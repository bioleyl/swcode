package sessions

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"swcode/daemon/agent"
	"swcode/daemon/browser"
	"swcode/daemon/catalog"
	"swcode/daemon/checkpoints"
	"swcode/daemon/hosts"
	"swcode/daemon/images"
	"swcode/daemon/llm"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
	"swcode/daemon/terminals"
	"swcode/daemon/tools"
	"swcode/daemon/web"
)

const (
	unknownSession = "There is no such session."
	unknownPreset  = "There is no such preset: it may have just been removed."
	stillWorking   = "This session is working: stop it before deleting it."
	allReported    = "All your subagents have reported."
	othersWorking  = "Not reported yet: %s. Their reports wake you once they are done: do not look into what they look into meanwhile."
	// restartedReport is the report of a subagent the daemon stopped by restarting.
	restartedReport    = "[The daemon restarted before this subagent finished.]"
	projectWorking     = "A session of this project is working: stop it before removing the project."
	blankMessage       = "Write a message or paste a picture first."
	pinnedTodosHeading = "Todos pinned to this session:"
	// pinnedTodosHint tells the model how to close one it did, or open one the work needs: it has the tools for both.
	pinnedTodosHint = "Close each with closetodo when it is done, and open another with todo when the work needs it."
	alreadyAnswered = "This request was already answered."
	// compactWorking: a turn compacts by itself when the context fills.
	compactWorking   = "This session is working: wait for its turn to end, or stop it, before compacting."
	nothingToCompact = "There is nothing new to summarize."
	restarted        = "The daemon restarted before the turn ended."
	notATime         = "%q is not a time the daemon gave."
	defaultMode      = protocol.PermissionModeAuto
	// restoreWorking: a turn may still write the files a restore would put back.
	restoreWorking = "This session is working: wait for its turn to end, or stop it, before restoring files."
	restoreNoFiles = "This session works in no folder: it has no files to restore."
	restoreTimeout = 30 * time.Second
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
	// SubagentLimit is how many subagents of the preset work at once: nil for all those the agent starts.
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
	ChatTools     tools.Box
	// Search is the daemon's one web search, which keeps every session's searches apart in time.
	Search web.Search
	HTTP   *http.Client
	// Terminals are the app's shells, which the agent opens in its folder for what people should watch.
	Terminals terminals.Port
	// ImageURL is the picture server, when there is one; ImagesFolder where the daemon keeps the pictures it shows,
	// a folder per session.
	ImageURL     string
	ImagesFolder string
	// Browser draws screenshots, when the daemon has one.
	Browser browser.Browser
	Skills  *catalog.Catalog
	// SkillsOff is the skills a folder's project turned off, by name: its sessions are neither told of them nor run them.
	SkillsOff func(path string, host protocol.HostRef) []string
	// Todos are the projects' todos, which the agent opens and closes.
	Todos     tools.ProjectTodos
	Workspace string
	Thinking  Thinking
	// OnWorkspace is told the folder of each new session, and its host: every folder sessions work in has a project.
	OnWorkspace      func(path string, host protocol.HostRef)
	Hosts            *hosts.Registry
	MaxSteps         int
	MaxSubagentSteps int
	// Checkpoints keeps the files a turn changes, as they were before it; nil keeps none.
	Checkpoints *checkpoints.Store
	Now         func() time.Time
	NewID       func() string
	LogError    func(error)
}

// Request is a new session: a chat, or a coding session in a folder on a host; its first message, how it asks and
// thinks.
type Request struct {
	// Kind is a coding session when empty.
	Kind      protocol.SessionKind
	Host      *protocol.HostRef
	Workspace string
	Prompt    string
	Pictures  []protocol.PictureUpload
	Mode      protocol.PermissionMode
	Thinking  protocol.ThinkingMode
	// Preset is the models it works with: the default preset when empty.
	Preset string
	// PinnedTodos are the project's todos this session starts on: their text opens the first message.
	PinnedTodos []string
	// Concise: token economy from the first message on.
	Concise bool
}

// pending is what a turn waits for from a person: a permission, or answers to the agent's questions. The answer
// comes back on waiting, whichever way it comes: the person's, or the turn's stopping, which settles it with a no.
type pending struct {
	requestID string
	// waitsFor says whether an answer is the kind this request waits for, so a question's answer never settles a
	// permission.
	waitsFor func(answer) bool
	// settle takes the answer on the turn's behalf: it clears the request, records the answer, and acts on a
	// permission allowed for the session. The service calls it with its lock held; it is also how the turn's own
	// stopping settles the request with a no.
	settle func(answer)
}

type runningTurn struct {
	cancel context.CancelFunc
	// id is the turn's own, made before the agent runs so its restore point bears the same id as its events.
	id string
	// recorder keeps the files the turn changes, before it changes them; closed when the turn ends.
	recorder  *checkpoints.Recorder
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
	// subagents is the subagents at work in the background, to stop each: they outlive the turn that started them.
	subagents map[string]runningSubagent
	// waiting is the subagents past the preset's limit, in the order they start.
	waiting []waitingSubagent
	// reports is the finished subagents' reports, waiting for the main agent's next step.
	reports  []pendingReport
	watching bool
	// model is held by the agent's requests and by the peeks: one at a time on the agent's slot.
	model sync.Mutex
}

// pendingReport is a subagent's report waiting for the main agent; one that wakes it starts a turn when the session
// has none, once no other subagent of it is at work. A stopped subagent's never does: the person stopped it.
type pendingReport struct {
	report protocol.SessionEventSubagentReport
	wakes  bool
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
		s.seenSoFar(session)
		s.closeSubagents(session)
		if turnID := unfinishedTurn(session.events); turnID != "" {
			s.record(session, protocol.SessionEventTurnFinished{ID: turnID, Message: restarted, Outcome: protocol.TurnOutcomeStopped})
		}
	}
}

// seenSoFar counts a session from before the daemon kept what was seen as seen as it is, once and for good: it would
// show done on every device otherwise. What the restart then closes (a turn, subagents) comes after, unseen.
func (s *Service) seenSoFar(session *live) {
	if session.meta.SeenAt != "" {
		return
	}
	session.meta.SeenAt = lastActivity(session.meta, session.events)
	s.saveMeta(session.meta)
}

func revive(stored Stored) *live {
	return &live{
		events: append([]protocol.SessionEvent{}, stored.Events...), grants: permissions.Grants{Programs: map[string]bool{}},
		jobs: []protocol.SessionSummaryJob{}, meta: stored.Meta, readFiles: tools.NewReadFiles(),
		subagents: map[string]runningSubagent{},
	}
}

// working tells whether a session's agent works: its turn, or subagents in the background, at work or waiting.
func working(session *live) bool {
	return session.turn != nil || subagentsLeft(session) > 0
}

// subagentsLeft is how many subagents of a session have not reported yet: at work, or waiting their turn.
func subagentsLeft(session *live) int {
	return len(session.subagents) + len(session.waiting)
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
		if working(session) {
			return projectWorking
		}
	}
	for _, session := range found {
		s.forget(session)
	}
	s.publishSessions()
	return ""
}

// Create makes a new session, started on its first message if given: a chat, or a coding session in the daemon's
// workspace or the folder asked for.
func (s *Service) Create(ctx context.Context, request Request) (protocol.SessionSummary, string) {
	kind := orKind(request.Kind)
	hostRef, workspace, problem := s.placeOf(ctx, kind, request)
	if problem != "" {
		return protocol.SessionSummary{}, problem
	}
	createdAt := protocol.Timestamp(s.deps.Now())
	meta := Meta{
		CreatedAt: createdAt, Host: hostRef, ID: s.deps.NewID(), Kind: kind, Mode: orMode(request.Mode),
		Concise: request.Concise, Preset: request.Preset, SeenAt: createdAt,
		Thinking: orThinking(request.Thinking, s.deps.Thinking.Session), Title: Untitled, Workspace: workspace,
	}
	s.saveMeta(meta)
	session := revive(Stored{Meta: meta})
	s.mu.Lock()
	s.sessions[meta.ID] = session
	s.publishSessions()
	s.mu.Unlock()
	// A session may start without its first message: the problem of a blank one goes nowhere.
	s.SendPrompt(meta.ID, withPinnedTodos(request.Prompt, request.PinnedTodos), request.Pictures)
	s.mu.Lock()
	defer s.mu.Unlock()
	return summaryOf(session), ""
}

// placeOf is where a new session works: a chat in no folder, on this computer; a coding session in the folder asked
// for, or the daemon's on this computer, which then has a project. The problem when the folder is not there.
func (s *Service) placeOf(ctx context.Context, kind protocol.SessionKind, request Request) (protocol.HostRef, string, string) {
	hostRef := protocol.LocalHost()
	if kind == protocol.SessionKindChat {
		return hostRef, "", ""
	}
	if request.Host != nil {
		hostRef = *request.Host
	}
	fallback := ""
	if hosts.IsLocal(hostRef) {
		fallback = s.deps.Workspace
	}
	workspace, problem := hosts.ChooseFolder(ctx, s.deps.Hosts.HostOf(hostRef), request.Workspace, fallback)
	if problem != "" {
		return hostRef, "", problem
	}
	s.deps.OnWorkspace(workspace, hostRef)
	return hostRef, workspace, ""
}

func orKind(kind protocol.SessionKind) protocol.SessionKind {
	if kind == "" {
		return protocol.SessionKindCode
	}
	return kind
}

// withPinnedTodos puts the project's pinned todos after the first message, so the agent reads what was asked first
// and the session is named after the start of that message.
func withPinnedTodos(prompt string, todos []string) string {
	if len(todos) == 0 {
		return prompt
	}
	lines := make([]string, 0, len(todos)+5)
	if prompt != "" {
		lines = append(lines, prompt, "")
	}
	lines = append(lines, pinnedTodosHeading)
	for _, todo := range todos {
		lines = append(lines, "- "+todo)
	}
	lines = append(lines, "", pinnedTodosHint)
	return strings.Join(lines, "\n")
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
		if working(session) {
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
	if s.deps.Checkpoints != nil {
		s.deps.Checkpoints.Remove(session.meta.ID)
	}
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

// History is the end of a session's conversation, and the answer being written, for a page that opens it: at most
// `limit` events, all of them when the limit is none. `keptBack` says how many older ones the session holds, so the
// page knows whether scrolling up has anything more to find.
func (s *Service) History(sessionID string, limit int) (protocol.ServerMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return protocol.ServerMessage{}, false
	}
	kept, older := lastEvents(session.events, limit)
	history := protocol.ServerMessageSessionHistory{
		Events:    append([]protocol.SessionEvent{}, kept...),
		KeptBack:  older,
		SessionID: sessionID,
	}
	if session.turn != nil && session.turn.streaming != nil {
		streaming := *session.turn.streaming
		history.Streaming = &streaming
	}
	return protocol.ServerMessage{Value: history}, true
}

// lastEvents is the last `limit` events of a session, and how many come before them.
func lastEvents(events []protocol.SessionEvent, limit int) ([]protocol.SessionEvent, int) {
	if limit <= 0 || limit >= len(events) {
		return events, 0
	}
	return events[len(events)-limit:], len(events) - limit
}

// OlderEvents is the page of events before the ones a page shows: the `limit` that come right before the
// `keptBack` older ones its last history reported, and how many are left before those. The streaming answer is not
// repeated: the page holds it.
func (s *Service) OlderEvents(sessionID string, keptBack, limit int) (protocol.ServerMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return protocol.ServerMessage{}, false
	}
	end := len(session.events) - keptBack
	if end < 0 {
		end = 0
	}
	start := max(end-limit, 0)
	history := protocol.ServerMessageSessionHistory{
		Events:    append([]protocol.SessionEvent{}, session.events[start:end]...),
		KeptBack:  start,
		SessionID: sessionID,
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
		s.startTurn(session, &message)
		return ""
	})
}

// Permission is a person's answer to a permission the agent asked for.
func Permission(decision protocol.PermissionDecision) answer { return answer{decision: decision} }

// Answers is a person's answers to the agent's questions.
func Answers(answers []string) answer { return answer{answers: append([]string{}, answers...)} }

// Secret is a person's answer to a password a command asked for: the words they typed, or empty for none, which
// closes the command's input. It is never kept in the session.
func Secret(secret string) answer {
	return answer{secret: &typedSecret{gave: secret != "", text: secret}}
}

// Answer answers what the turn of a session asked a person: a permission, or the agent's questions. The turn takes
// it through the request it holds, which records the answer and lets the turn go on.
func (s *Service) Answer(sessionID, requestID string, answer answer) string {
	return s.withSession(sessionID, func(session *live) string {
		settle := session.turn.settleOf(requestID, answer)
		if settle == nil {
			return alreadyAnswered
		}
		settle(answer)
		return ""
	})
}

// settleOf is how the turn takes an answer: the request it still waits for, when that request takes an answer of
// this kind and bears this id; nil otherwise, which is how an answer too late or of the wrong kind is refused. The
// caller holds the service's lock.
func (t *runningTurn) settleOf(requestID string, got answer) func(answer) {
	if t == nil || t.pending == nil || t.pending.requestID != requestID || !t.pending.waitsFor(got) {
		return nil
	}
	return t.pending.settle
}

// Stop stops the running turn; what was queued for after it goes too, since people stop to think again.
// Stop stops a session's turn and its subagents; the reports waiting no longer wake the agent, it gets them with
// the person's next message.
func (s *Service) Stop(sessionID string) string {
	return s.withSession(sessionID, func(session *live) string {
		session.queue, session.queueNow = nil, false
		if session.turn != nil {
			session.turn.cancel()
		}
		for _, running := range session.subagents {
			running.cancel()
		}
		s.dropWaiting(session)
		for index := range session.reports {
			session.reports[index].wakes = false
		}
		s.publishSessions()
		return ""
	})
}

// RestoreCheckpoint puts back the files a turn wrote or edited, as they were before it, and deletes those it made.
// It holds the service while the host writes them, so no turn starts in the middle of a restore.
func (s *Service) RestoreCheckpoint(sessionID, turnID string) string {
	return s.withSession(sessionID, func(session *live) string {
		if session.turn != nil {
			return restoreWorking
		}
		if s.deps.Checkpoints == nil || session.meta.Workspace == "" {
			return restoreNoFiles
		}
		ctx, cancel := context.WithTimeout(context.Background(), restoreTimeout)
		defer cancel()
		files, err := s.deps.Checkpoints.Restore(ctx, s.hostOf(session), sessionID, turnID, session.meta.Workspace)
		if err != nil {
			return err.Error()
		}
		s.record(session, protocol.SessionEventCheckpointRestored{Files: files, TurnID: turnID})
		return ""
	})
}

// CheckpointDiff is a turn's restore point, a diff a file: what it was before the turn and what it is now. A refusal
// says why there is none.
func (s *Service) CheckpointDiff(sessionID, turnID string) ([]protocol.ToolDisplay, string) {
	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return nil, unknownSession
	}
	workspace, host := session.meta.Workspace, s.hostOf(session)
	s.mu.Unlock()
	if s.deps.Checkpoints == nil || workspace == "" {
		return nil, restoreNoFiles
	}
	snapshots, err := s.deps.Checkpoints.Snapshot(sessionID, turnID)
	if err != nil {
		return nil, err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), restoreTimeout)
	defer cancel()
	paths := host.Paths()
	diffs := make([]protocol.ToolDisplay, 0, len(snapshots))
	for _, snapshot := range snapshots {
		after, refused := currentText(ctx, host, paths.Resolve(workspace, snapshot.Path))
		if refused != "" {
			return nil, refused
		}
		diff := tools.FileDiff(snapshot.Path, string(snapshot.Data), after)
		diffs = append(diffs, protocol.ToolDisplay{Value: diff})
	}
	return diffs, ""
}

// currentText is what a file says now, empty when it is gone: a refusal says why it cannot be read.
func currentText(ctx context.Context, host hosts.Host, path string) (string, string) {
	info, err := host.FileInfo(ctx, path)
	if err != nil {
		return "", err.Error()
	}
	if info.Kind != hosts.KindFile {
		return "", ""
	}
	data, err := host.ReadFile(ctx, path)
	if err != nil {
		return "", err.Error()
	}
	return string(data), ""
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

// MarkSeen keeps that the person saw the session as it was at an activity, for every device to show it seen. What
// a device saw before another already saw more changes nothing.
func (s *Service) MarkSeen(sessionID, seenAt string) string {
	seen, err := protocol.ParseTimestamp(seenAt)
	if err != nil {
		return fmt.Sprintf(notATime, seenAt)
	}
	return s.withSession(sessionID, func(session *live) string {
		if !seenBefore(session.meta.SeenAt, seen) {
			return ""
		}
		session.meta.SeenAt = seenAt
		s.saveMeta(session.meta)
		s.publishSessions()
		return ""
	})
}

// seenBefore tells whether what was seen so far comes before a newer sight: always when nothing was kept, or when
// what was kept is not a time.
func seenBefore(kept string, seen time.Time) bool {
	before, err := protocol.ParseTimestamp(kept)
	return err != nil || before.Before(seen)
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
		for _, running := range session.subagents {
			running.cancel()
		}
		// Those waiting their turn are closed as stopped at the next start, with the others the restart stopped.
		session.waiting = nil
	}
	s.mu.Unlock()
	s.turns.Wait()
}

// Compact summarizes the session's conversation now, at the person's word, in a turn of its own: the next request
// starts from the summary. Not while a turn runs, which compacts by itself when the context fills.
func (s *Service) Compact(sessionID string) string {
	return s.withSession(sessionID, func(session *live) string {
		if session.turn != nil {
			return compactWorking
		}
		if !agent.HasConversation(session.events) {
			return nothingToCompact
		}
		s.inTurn(session, func(ctx context.Context, turn *runningTurn) { s.compactIn(ctx, session, turn) })
		return ""
	})
}

// startTurn starts a turn on the person's message, or on none when the subagents' reports wake the agent: they
// join at its first step. The caller holds the lock.
func (s *Service) startTurn(session *live, message *agent.PromptMessage) {
	if message != nil && session.meta.Title == Untitled {
		session.meta.Title = TitleFrom(message.Text)
		s.saveMeta(session.meta)
	}
	s.inTurn(session, func(ctx context.Context, turn *runningTurn) { s.runTurnIn(ctx, session, turn, message) })
}

// inTurn runs work as the session's turn, in its own goroutine: the session works until it ends, then the messages
// that waited start the next one. The caller holds the lock.
func (s *Service) inTurn(session *live, work func(ctx context.Context, turn *runningTurn)) {
	if s.stopping {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	turn := &runningTurn{cancel: cancel, id: s.deps.NewID()}
	session.turn = turn
	s.publishSessions()
	s.turns.Add(1)
	go func() {
		defer s.turns.Done()
		defer cancel()
		work(ctx, turn)
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
// startQueued starts the next turn once one ends: on the messages that waited, else on the reports that wake the
// agent.
func (s *Service) startQueued(session *live) {
	if _, exists := s.sessions[session.meta.ID]; !exists {
		return
	}
	if len(session.queue) == 0 {
		s.wakeForReports(session)
		return
	}
	message := joinMessages(session.queue)
	session.queue, session.queueNow = nil, false
	s.startTurn(session, &message)
}

// wakeForReports starts a turn for the reports waiting, when one of them wakes the agent, the session has no turn
// (its next step takes them) and no other subagent is at work: the agent gets the reports together.
func (s *Service) wakeForReports(session *live) {
	if session.turn != nil || subagentsLeft(session) > 0 || !slices.ContainsFunc(session.reports, func(pending pendingReport) bool { return pending.wakes }) {
		return
	}
	s.startTurn(session, nil)
}

// takeReports is the reports waiting, handed to the main agent's next step; the last says what is still to come.
func (s *Service) takeReports(session *live) []protocol.SessionEventSubagentReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	taken := []protocol.SessionEventSubagentReport{}
	for _, pending := range session.reports {
		taken = append(taken, pending.report)
	}
	session.reports = nil
	if len(taken) > 0 {
		taken[len(taken)-1].Note = stillToCome(session)
	}
	return taken
}

// stillToCome tells the main agent which subagents have not reported yet, or that all have.
func stillToCome(session *live) string {
	left := []string{}
	for _, running := range session.subagents {
		left = append(left, "“"+running.description+"”")
	}
	sort.Strings(left)
	for _, waiting := range session.waiting {
		left = append(left, "“"+waiting.description+"”")
	}
	if len(left) == 0 {
		return allReported
	}
	return fmt.Sprintf(othersWorking, strings.Join(left, ", "))
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
		Host: session.meta.Host, ID: session.meta.ID, Jobs: append([]protocol.SessionSummaryJob{}, session.jobs...), Kind: session.meta.Kind,
		Concise: session.meta.Concise, Mode: session.meta.Mode, Preset: session.meta.Preset, Queued: queued, QueuedNow: session.queueNow,
		SeenAt: session.meta.SeenAt, Status: status,
		Subagents: subagentsLeft(session),
		Thinking:  session.meta.Thinking, Title: session.meta.Title, UpdatedAt: lastActivity(session.meta, session.events),
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
