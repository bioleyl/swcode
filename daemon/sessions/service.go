package sessions

import (
	"context"
	"net/http"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"swcode/daemon/agent"
	"swcode/daemon/browser"
	"swcode/daemon/catalog"
	"swcode/daemon/hosts"
	"swcode/daemon/images"
	"swcode/daemon/llm"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
	"swcode/daemon/tools"
	"swcode/daemon/web"
)

const (
	unknownSession  = "There is no such session."
	unknownPreset   = "There is no such preset: it may have just been removed."
	stillWorking    = "This session is working: stop it before deleting it."
	projectWorking  = "A session of this project is working: stop it before removing the project."
	blankMessage    = "Write a message or paste a picture first."
	alreadyAnswered = "This request was already answered."
	restarted       = "The daemon restarted before the turn ended."
	defaultMode     = protocol.PermissionModeAuto
	// jobWatch is how often the jobs of a session are looked at, for the list to drop the ones that ended.
	jobWatch = 5 * time.Second
)

// Listener hears every message for the apps. It is called with the service's lock held, so the messages come in
// order: it must not call back into the service.
type Listener func(message protocol.ServerMessage)

// ModelsPort is the presets: a preset's models as their sources serve them now, the main agent's and the workers'
// (the default preset's when it is not there), and whether there is a preset of an id.
type ModelsPort interface {
	Choose(ctx context.Context, preset string) (llm.Choice, llm.Choice)
	// SubagentLimit is how many subagents a turn of the preset may start: nil for as many as the agent likes.
	SubagentLimit(preset string) *protocol.SubagentLimit
	Has(preset string) bool
}

// Thinking is how much the main agent of a new session thinks, and how much subagents do.
type Thinking struct {
	Session   protocol.ThinkingMode
	Subagents protocol.ThinkingMode
}

// Deps is what the sessions work with.
type Deps struct {
	Store StorePort
	// Model asks the llm-server a request's target names.
	Model llm.Model
	// Models picks a session's models at each turn, from its preset.
	Models        ModelsPort
	Tools         tools.Box
	SubagentTools tools.Box
	// Search is the daemon's one web search, which keeps every session's searches apart in time.
	Search web.Search
	HTTP   *http.Client
	// ImageURL is the picture server, when there is one; ImagesFolder where the daemon keeps the pictures it shows,
	// a folder per session.
	ImageURL     string
	ImagesFolder string
	// Browser draws screenshots, when the daemon has one.
	Browser   browser.Browser
	Skills    *catalog.Catalog
	Workspace string
	Thinking  Thinking
	// OnWorkspace is told the folder of each new session, and its host: every folder sessions work in has a project.
	OnWorkspace      func(path string, host protocol.HostRef)
	Hosts            *hosts.Registry
	MaxSteps         int
	MaxSubagentSteps int
	Now              func() time.Time
	NewID            func() string
	LogError         func(error)
}

// Request is a new session: its folder on a host, its first message, how it asks and thinks.
type Request struct {
	Host      *protocol.HostRef
	Workspace string
	Prompt    string
	Pictures  []protocol.PictureUpload
	Mode      protocol.PermissionMode
	Thinking  protocol.ThinkingMode
	// Preset is the models it works with: the default preset when empty.
	Preset string
	// Concise: token economy from the first message on.
	Concise bool
}

// pending is what a turn waits for from a person: a permission, or answers to the agent's questions.
type pending struct {
	requestID string
	question  bool
	decide    func(protocol.PermissionDecision)
	answer    func([]string)
}

type runningTurn struct {
	cancel    context.CancelFunc
	streaming *protocol.Streaming
	pending   *pending
	// setup is what the turn runs with once ready: a peek asks with the same, for the prompt llm-server cached.
	setup *agent.Setup
}

// live is a session as the daemon holds it while it runs: its file's content, and what lives only in memory.
type live struct {
	meta   Meta
	events []protocol.SessionEvent
	// queue is the messages sent while a turn ran, for the next one; queueNow, the person asked for them now.
	queue     []agent.PromptMessage
	queueNow  bool
	jobs      []protocol.SessionSummaryJob
	grants    permissions.Grants
	readFiles *tools.ReadFiles
	turn      *runningTurn
	watching  bool
	// model is held by the agent's requests and by the peeks: one at a time on the agent's slot.
	model sync.Mutex
}

// Service is the sessions of this daemon: it starts their turns, holds their permission requests until someone
// answers, keeps their files, and tells every open app what happens.
type Service struct {
	deps Deps
	// turns are the turns running, which stopping the daemon waits for.
	turns sync.WaitGroup
	// stopping: the daemon is shutting down, and no turn starts any more.
	stopping  bool
	mu        sync.Mutex
	sessions  map[string]*live
	listeners map[int]Listener
	next      int
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps, listeners: map[int]Listener{}, sessions: map[string]*live{}}
}

// Load reads the sessions kept on disk; a turn the daemon left halfway is closed as stopped.
func (s *Service) Load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, stored := range s.deps.Store.LoadAll() {
		session := revive(stored)
		s.sessions[session.meta.ID] = session
		if turnID := unfinishedTurn(session.events); turnID != "" {
			s.record(session, protocol.SessionEventTurnFinished{ID: turnID, Message: restarted, Outcome: protocol.TurnOutcomeStopped})
		}
	}
}

func revive(stored Stored) *live {
	return &live{
		events: append([]protocol.SessionEvent{}, stored.Events...), grants: permissions.Grants{Programs: map[string]bool{}},
		jobs: []protocol.SessionSummaryJob{}, meta: stored.Meta, readFiles: tools.NewReadFiles(),
	}
}

// Subscribe adds a listener of every message for the apps, until the returned function is called.
func (s *Service) Subscribe(listener Listener) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.next
	s.next++
	s.listeners[id] = listener
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.listeners, id)
	}
}

func (s *Service) List() []protocol.SessionSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list()
}

func (s *Service) list() []protocol.SessionSummary {
	summaries := []protocol.SessionSummary{}
	for _, session := range s.sessions {
		summaries = append(summaries, summaryOf(session))
	}
	sort.SliceStable(summaries, func(first, second int) bool { return summaries[first].UpdatedAt > summaries[second].UpdatedAt })
	return summaries
}

// RemoveIn deletes every session working in a folder of a host, or none while one of them runs a turn.
func (s *Service) RemoveIn(path string, host protocol.HostRef) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := []*live{}
	for _, session := range s.sessions {
		if session.meta.Workspace == path && hosts.SameHost(session.meta.Host, host) {
			found = append(found, session)
		}
	}
	for _, session := range found {
		if session.turn != nil {
			return projectWorking
		}
	}
	for _, session := range found {
		s.forget(session)
	}
	s.publishSessions()
	return ""
}

// Create makes a new session, in the daemon's workspace or the folder asked for, started on its first message if
// given.
func (s *Service) Create(ctx context.Context, request Request) (protocol.SessionSummary, string) {
	hostRef := protocol.LocalHost()
	if request.Host != nil {
		hostRef = *request.Host
	}
	fallback := ""
	if hosts.IsLocal(hostRef) {
		fallback = s.deps.Workspace
	}
	workspace, problem := hosts.ChooseFolder(ctx, s.deps.Hosts.HostOf(hostRef), request.Workspace, fallback)
	if problem != "" {
		return protocol.SessionSummary{}, problem
	}
	s.deps.OnWorkspace(workspace, hostRef)
	meta := Meta{
		CreatedAt: protocol.Timestamp(s.deps.Now()), Host: hostRef, ID: s.deps.NewID(), Mode: orMode(request.Mode),
		Concise: request.Concise, Preset: request.Preset, Thinking: orThinking(request.Thinking, s.deps.Thinking.Session), Title: Untitled,
		Workspace: workspace,
	}
	s.saveMeta(meta)
	session := revive(Stored{Meta: meta})
	s.mu.Lock()
	s.sessions[meta.ID] = session
	s.publishSessions()
	s.mu.Unlock()
	// A session may start without its first message: the problem of a blank one goes nowhere.
	s.SendPrompt(meta.ID, request.Prompt, request.Pictures)
	s.mu.Lock()
	defer s.mu.Unlock()
	return summaryOf(session), ""
}

func orMode(mode protocol.PermissionMode) protocol.PermissionMode {
	if mode == "" {
		return defaultMode
	}
	return mode
}

func orThinking(thinking, fallback protocol.ThinkingMode) protocol.ThinkingMode {
	if thinking == "" {
		return fallback
	}
	return thinking
}

func (s *Service) saveMeta(meta Meta) {
	if err := s.deps.Store.SaveMeta(meta); err != nil {
		s.deps.LogError(err)
	}
}

// withSession runs change on a session, under the lock: the problem when there is no such session.
func (s *Service) withSession(sessionID string, change func(session *live) string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return unknownSession
	}
	return change(session)
}

func (s *Service) Rename(sessionID, title string) string {
	return s.withSession(sessionID, func(session *live) string {
		session.meta.Title = title
		s.saveMeta(session.meta)
		s.publishSessions()
		return ""
	})
}

func (s *Service) Remove(sessionID string) string {
	return s.withSession(sessionID, func(session *live) string {
		if session.turn != nil {
			return stillWorking
		}
		s.forget(session)
		s.publishSessions()
		return ""
	})
}

// forget deletes a session: its files, its pictures, and its scratch folder on its host when the host answers.
func (s *Service) forget(session *live) {
	delete(s.sessions, session.meta.ID)
	s.deps.Store.Remove(session.meta.ID)
	host := s.hostOf(session)
	jobs := append([]protocol.SessionSummaryJob{}, session.jobs...)
	go func() {
		ctx := context.Background()
		if err := host.RemoveScratch(ctx, session.meta.ID); err != nil {
			s.deps.LogError(err)
		}
		for _, job := range jobs {
			if _, err := host.StopJob(ctx, job.ID); err != nil {
				s.deps.LogError(err)
			}
		}
	}()
	if err := images.Remove(s.deps.ImagesFolder, session.meta.ID); err != nil {
		s.deps.LogError(err)
	}
}

// picturesFolderOf is where a session's pictures are: those its agent made, and those the person pasted.
func (s *Service) picturesFolderOf(session *live) string {
	return filepath.Join(s.deps.ImagesFolder, session.meta.ID)
}

func (s *Service) hostOf(session *live) hosts.Host {
	return s.deps.Hosts.HostOf(session.meta.Host)
}

func (s *Service) ClearQueue(sessionID string) string {
	return s.withSession(sessionID, func(session *live) string {
		session.queue, session.queueNow = nil, false
		s.publishSessions()
		return ""
	})
}

// SendQueueNow sends the queued messages to the model at the running turn's next step: the person steers it as it
// works.
func (s *Service) SendQueueNow(sessionID string) string {
	return s.withSession(sessionID, func(session *live) string {
		session.queueNow = session.turn != nil && len(session.queue) > 0
		s.publishSessions()
		return ""
	})
}

// History is a session's events, and the answer being written, for a page that opens it.
func (s *Service) History(sessionID string) (protocol.ServerMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return protocol.ServerMessage{}, false
	}
	history := protocol.ServerMessageSessionHistory{Events: append([]protocol.SessionEvent{}, session.events...), SessionID: sessionID}
	if session.turn != nil && session.turn.streaming != nil {
		streaming := *session.turn.streaming
		history.Streaming = &streaming
	}
	return protocol.ServerMessage{Value: history}, true
}

// SendPrompt starts a turn with the person's message, or queues it for the next turn while one runs. Its pictures
// are kept at once, with the session's.
func (s *Service) SendPrompt(sessionID, text string, uploads []protocol.PictureUpload) string {
	return s.withSession(sessionID, func(session *live) string {
		if text == "" && len(uploads) == 0 {
			return blankMessage
		}
		pictures, err := images.KeepPictures(s.picturesFolderOf(session), uploads, s.deps.NewID)
		if err != nil {
			return err.Error()
		}
		message := agent.PromptMessage{Concise: session.meta.Concise, Pictures: pictures, Text: text}
		if session.turn != nil {
			session.queue = append(session.queue, message)
			s.publishSessions()
			return ""
		}
		s.startTurn(session, message)
		return ""
	})
}

func (s *Service) AnswerPermission(sessionID, requestID string, decision protocol.PermissionDecision) string {
	return s.withSession(sessionID, func(session *live) string {
		if session.turn == nil || session.turn.pending == nil || session.turn.pending.question || session.turn.pending.requestID != requestID {
			return alreadyAnswered
		}
		session.turn.pending.decide(decision)
		return ""
	})
}

func (s *Service) AnswerQuestion(sessionID, requestID string, answers []string) string {
	return s.withSession(sessionID, func(session *live) string {
		if session.turn == nil || session.turn.pending == nil || !session.turn.pending.question || session.turn.pending.requestID != requestID {
			return alreadyAnswered
		}
		session.turn.pending.answer(answers)
		return ""
	})
}

// Stop stops the running turn; what was queued for after it goes too, since people stop to think again.
func (s *Service) Stop(sessionID string) string {
	return s.withSession(sessionID, func(session *live) string {
		session.queue, session.queueNow = nil, false
		if session.turn != nil {
			session.turn.cancel()
		}
		s.publishSessions()
		return ""
	})
}

func (s *Service) SetMode(sessionID string, mode protocol.PermissionMode) string {
	return s.withSession(sessionID, func(session *live) string {
		session.meta.Mode = mode
		s.saveMeta(session.meta)
		s.publishSessions()
		return ""
	})
}

// SetThinking sets how much the session's main agent thinks from its next request on.
func (s *Service) SetThinking(sessionID string, thinking protocol.ThinkingMode) string {
	return s.withSession(sessionID, func(session *live) string {
		session.meta.Thinking = thinking
		s.saveMeta(session.meta)
		s.publishSessions()
		return ""
	})
}

// SetConcise sets token economy from the next message sent on: the model reads "Be concise." after each.
func (s *Service) SetConcise(sessionID string, concise bool) string {
	return s.withSession(sessionID, func(session *live) string {
		session.meta.Concise = concise
		s.saveMeta(session.meta)
		s.publishSessions()
		return ""
	})
}

// SetPreset sets the models the session works with from its next request on.
func (s *Service) SetPreset(sessionID, preset string) string {
	if !s.deps.Models.Has(preset) {
		return unknownPreset
	}
	return s.withSession(sessionID, func(session *live) string {
		session.meta.Preset = preset
		s.saveMeta(session.meta)
		s.publishSessions()
		return ""
	})
}

// StopAll stops every running turn and waits for them to end, for shutting down: their files stay consistent.
func (s *Service) StopAll() {
	s.mu.Lock()
	s.stopping = true
	for _, session := range s.sessions {
		if session.turn != nil {
			session.turn.cancel()
		}
	}
	s.mu.Unlock()
	s.turns.Wait()
}

// startTurn runs a turn in its own goroutine; the caller holds the lock.
func (s *Service) startTurn(session *live, message agent.PromptMessage) {
	if s.stopping {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	turn := &runningTurn{cancel: cancel}
	session.turn = turn
	if session.meta.Title == Untitled {
		session.meta.Title = TitleFrom(message.Text)
		s.saveMeta(session.meta)
	}
	s.publishSessions()
	s.turns.Add(1)
	go func() {
		defer s.turns.Done()
		defer cancel()
		s.runTurnIn(ctx, session, turn, message)
		s.mu.Lock()
		defer s.mu.Unlock()
		session.turn = nil
		s.startQueued(session)
		s.publishSessions()
	}()
}

// takeQueued is the queued messages, once the person asked for them now: they go into the running turn, together.
func (s *Service) takeQueued(session *live) *agent.PromptMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !session.queueNow || len(session.queue) == 0 {
		return nil
	}
	message := joinMessages(session.queue)
	session.queue, session.queueNow = nil, false
	s.publishSessions()
	return &message
}

// startQueued starts the next turn with the messages queued during a turn, together, as one message; the caller
// holds the lock.
func (s *Service) startQueued(session *live) {
	if _, exists := s.sessions[session.meta.ID]; !exists || len(session.queue) == 0 {
		return
	}
	message := joinMessages(session.queue)
	session.queue, session.queueNow = nil, false
	s.startTurn(session, message)
}

func summaryOf(session *live) protocol.SessionSummary {
	queued := []string{}
	for _, message := range session.queue {
		queued = append(queued, queuedText(message))
	}
	status := protocol.SessionStatusIdle
	if session.turn != nil {
		status = protocol.SessionStatusWorking
		if session.turn.pending != nil {
			status = protocol.SessionStatusWaiting
		}
	}
	return protocol.SessionSummary{
		Host: session.meta.Host, ID: session.meta.ID, Jobs: append([]protocol.SessionSummaryJob{}, session.jobs...),
		Concise: session.meta.Concise, Mode: session.meta.Mode, Preset: session.meta.Preset, Queued: queued, QueuedNow: session.queueNow, Status: status,
		Thinking: session.meta.Thinking, Title: session.meta.Title, UpdatedAt: lastActivity(session.meta, session.events),
		Workspace: session.meta.Workspace,
	}
}

// record dates an event, keeps it in the session and its file, and tells the apps; the caller holds the lock.
func (s *Service) record(session *live, event protocol.SessionEventVariant) {
	dated := protocol.SessionEvent{Value: protocol.Dated(event, protocol.Timestamp(s.deps.Now()))}
	session.events = append(session.events, dated)
	// A full disk (an agent's runaway output, say) loses the event from the session's file, not from the turn: it
	// goes on in memory and in the app, and the agent can see what filled the disk.
	if err := s.deps.Store.Append(session.meta.ID, dated); err != nil {
		s.deps.LogError(err)
	}
	if _, reply := event.(protocol.SessionEventAssistantMessage); reply && session.turn != nil {
		session.turn.streaming = nil
	}
	s.publish(protocol.ServerMessage{Value: protocol.ServerMessageSessionEvent{Event: dated, SessionID: session.meta.ID}})
}

func (s *Service) publishSessions() {
	s.publish(protocol.ServerMessage{Value: protocol.ServerMessageSessions{Sessions: s.list()}})
}

// publish tells every listener; the caller holds the lock.
func (s *Service) publish(message protocol.ServerMessage) {
	for _, listener := range s.listeners {
		listener(message)
	}
}
