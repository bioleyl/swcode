package sessions

import (
	"context"
	"fmt"
	"time"

	"swcode/daemon/agent"
	"swcode/daemon/browser"
	"swcode/daemon/catalog"
	"swcode/daemon/checkpoints"
	"swcode/daemon/hosts"
	"swcode/daemon/images"
	"swcode/daemon/llm"
	"swcode/daemon/notes"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
	"swcode/daemon/tools"
	"swcode/daemon/web"
)

// place is where a turn works: its host, that host's system, the session's scratch folder there, and its skills.
type place struct {
	host     hosts.Host
	platform string
	scratch  string
	skills   catalog.Port
	// sandbox is where the session's commands run; nil where they run as the person does.
	sandbox *hosts.Sandbox
}

// The agent call's result: the subagent works, or waits its turn, and its report comes later in any case.
const (
	subagentStarted = "Started the subagent “%s” in the background."
	subagentQueued  = "Queued the subagent “%s”: %d work at once, so it starts by itself once one of them reports (%d waiting before it)."
	reportComes     = " Your turn ends after this reply's calls: the reports of your subagents wake you, all together once the last is done."
	// notStartedReport is the report of a subagent stopped while it waited its turn.
	notStartedReport = "[Stopped before it started.]"
)

// prepared is what the session's agent runs with now: its place, its models, how many subagents a turn may start,
// and its setup.
type prepared struct {
	where     place
	main      llm.Choice
	subagent  llm.Choice
	subagents *protocol.SubagentLimit
	setup     agent.Setup
}

// prepare finds the models and context sizes llm-server gives now (a model may have been reloaded with another
// one, or a router may serve others), and where the session works: a chat works nowhere.
func (s *Service) prepare(ctx context.Context, session *live) (prepared, error) {
	s.mu.Lock()
	meta := session.meta
	s.mu.Unlock()
	main, subagent := s.deps.Models.Choose(ctx, meta.Preset)
	if meta.Kind == protocol.SessionKindChat {
		return prepared{main: main, setup: s.chatSetup(session, meta, main), subagent: subagent}, nil
	}
	limit := s.deps.Models.SubagentLimit(meta.Preset)
	host := s.deps.Hosts.HostOf(meta.Host)
	scratch, err := host.ScratchFolder(ctx, meta.ID)
	if err != nil {
		return prepared{}, err
	}
	platform, err := host.Platform(ctx)
	if err != nil {
		return prepared{}, err
	}
	where := place{host: host, platform: platform, sandbox: s.sandboxFor(ctx, session, meta, host, scratch), scratch: scratch, skills: s.deps.Skills.PortFor(catalog.Place{
		Host: host, Off: s.deps.SkillsOff(meta.Workspace, meta.Host), Workspace: meta.Workspace,
	})}
	return prepared{
		main: main, setup: s.setupFor(session, meta, main, where, main.Vision, limit), subagent: subagent, subagents: limit, where: where,
	}, nil
}

// sandboxFor is where the session's commands run: in a sandbox, when its host can make one and neither the daemon
// nor the session's project turned it off; nil otherwise, and they run as the person does.
func (s *Service) sandboxFor(ctx context.Context, session *live, meta Meta, host hosts.Host, scratch string) *hosts.Sandbox {
	if s.deps.Sandboxes == nil || (s.deps.SandboxOff != nil && s.deps.SandboxOff(meta.Workspace, meta.Host)) {
		return nil
	}
	state := s.deps.Sandboxes.Of(ctx, hosts.Key(meta.Host), host)
	if !state.Available {
		return nil
	}
	sandbox := &hosts.Sandbox{Host: state, Scratch: s.realPath(ctx, session, host, scratch), Workspace: s.realPath(ctx, session, host, meta.Workspace)}
	if hosts.IsLocal(meta.Host) {
		sandbox.Hidden = s.deps.SandboxHidden
	}
	return sandbox
}

// realPath is where a folder of the session really is, links followed, asked of its host once: a sandbox opens a
// folder by its real path only.
func (s *Service) realPath(ctx context.Context, session *live, host hosts.Host, path string) string {
	s.mu.Lock()
	known, found := session.realPaths[path]
	s.mu.Unlock()
	if found {
		return known
	}
	resolved, err := host.RealPath(ctx, path)
	if err != nil || resolved == "" {
		return path
	}
	s.mu.Lock()
	if session.realPaths == nil {
		session.realPaths = map[string]string{}
	}
	session.realPaths[path] = resolved
	s.mu.Unlock()
	return resolved
}

// runTurnIn runs a turn with what the session's agent runs with now.
func (s *Service) runTurnIn(ctx context.Context, session *live, turn *runningTurn, message *agent.PromptMessage) {
	ready, err := s.prepare(ctx, session)
	if err != nil {
		s.failTurn(session, message, err)
		return
	}
	recorder := s.keepFilesOf(&ready, session, turn)
	ready.setup.TurnID = turn.id
	s.mu.Lock()
	turn.setup = &ready.setup
	turn.recorder = recorder
	s.mu.Unlock()
	agent.RunTurn(ctx, message, s.hostFor(session, turn, ready, ready.main.Vision), ready.setup)
}

// keepFilesOf takes the restore point of a coding turn, and works through a host that keeps each file before it is
// first written; nil for a chat, and where no restore points are kept.
func (s *Service) keepFilesOf(ready *prepared, session *live, turn *runningTurn) *checkpoints.Recorder {
	if ready.where.host == nil || s.deps.Checkpoints == nil || session.meta.Workspace == "" {
		return nil
	}
	recorder := s.deps.Checkpoints.Begin(ready.where.host, session.meta.ID, turn.id, session.meta.Workspace)
	ready.where.host = recorder.Host()
	return recorder
}

// compactIn summarizes the conversation with what the session's agent runs with now, its model and its skills.
func (s *Service) compactIn(ctx context.Context, session *live, turn *runningTurn) {
	ready, err := s.prepare(ctx, session)
	if err != nil {
		s.failTurn(session, nil, err)
		return
	}
	s.mu.Lock()
	turn.setup = &ready.setup
	s.mu.Unlock()
	agent.CompactTurn(ctx, s.hostFor(session, turn, ready, ready.main.Vision), ready.setup)
}

// failTurn records a turn whose host did not answer: the person's message, and why nothing came of it.
func (s *Service) failTurn(session *live, message *agent.PromptMessage, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	turnID := s.deps.NewID()
	s.record(session, protocol.SessionEventTurnStarted{ID: turnID})
	if message != nil {
		s.record(session, agent.UserMessage(s.deps.NewID(), *message))
	}
	s.record(session, protocol.SessionEventTurnFinished{ID: turnID, Message: "The host did not answer: " + err.Error(), Outcome: protocol.TurnOutcomeFailed})
}

// toolsFor is the main agent's tools: without agent when the preset allows no subagents, and with the tools of the
// MCP servers ready now.
func (s *Service) toolsFor(limit *protocol.SubagentLimit) tools.Box {
	box := s.deps.Tools
	if limit != nil && limit.AtOnce == 0 {
		box = box.Without(protocol.ToolNameAgent)
	}
	if s.deps.ServerTools == nil {
		return box
	}
	return box.With(s.deps.ServerTools())
}

func (s *Service) setupFor(session *live, meta Meta, main llm.Choice, where place, sees bool, limit *protocol.SubagentLimit) agent.Setup {
	return agent.Setup{
		Agent:         llmIdentity(meta, main),
		ContextTokens: main.ContextTokens,
		MaxSteps:      s.deps.MaxSteps,
		Model:         s.deps.Model,
		NewID:         s.deps.NewID,
		Pictures:      &agent.PictureView{Folder: s.picturesFolderOf(session), Seen: sees},
		LoadSkill:     where.skills.Load,
		Notes: func(ctx context.Context) []protocol.ProjectNote {
			return notes.Find(ctx, where.host, meta.Workspace)
		},
		Skills: where.skills.List,
		SystemPrompt: agent.SystemPrompt(meta.Workspace, where.scratch, where.platform, agent.Servers{
			Browser: s.deps.Browser != nil && sees, Images: s.deps.ImageURL != "", Sandbox: where.sandbox != nil,
		}, limit),
		Tools: s.toolsFor(limit),
	}
}

// chatSetup is a chat's: its own prompt, the web tools and drawing alone, and no skills. The subagent model reads the
// pages.
func (s *Service) chatSetup(session *live, meta Meta, main llm.Choice) agent.Setup {
	return agent.Setup{
		Agent:         llmIdentity(meta, main),
		ContextTokens: main.ContextTokens,
		MaxSteps:      s.deps.MaxSteps,
		Model:         s.deps.Model,
		NewID:         s.deps.NewID,
		Pictures:      &agent.PictureView{Folder: s.picturesFolderOf(session), Seen: main.Vision},
		SystemPrompt:  agent.ChatPrompt(s.deps.ImageURL != ""),
		Tools:         s.deps.ChatTools,
	}
}

// turnHost is what a turn needs from its session.
type turnHost struct {
	service *Service
	session *live
	turn    *runningTurn
	context *tools.Context
}

func (s *Service) hostFor(session *live, turn *runningTurn, ready prepared, sees bool) agent.TurnHost {
	where, main, subagent := ready.where, ready.main, ready.subagent
	host := &turnHost{service: s, session: session, turn: turn}
	host.context = &tools.Context{
		AskQuestions: func(ctx context.Context, callID string, questions []protocol.Question) []string {
			return s.ask(ctx, session, turn, questionsAsked{callID: callID, questions: questions, service: s}).answers
		},
		AskPassword: func(ctx context.Context, callID, prompt string) (string, bool) {
			got := s.ask(ctx, session, turn, passwordAsk{callID: callID, prompt: prompt, service: s}).secret
			return got.text, got.gave
		},
		Host:      where.host,
		HostRef:   session.meta.Host,
		AfterEdit: s.afterEditOf(session),
		InTerminal: func(callID string, terminal protocol.Terminal) func(asks bool) {
			s.mu.Lock()
			s.record(session, protocol.SessionEventTerminalWaiting{CallID: callID, TerminalID: terminal.ID, Title: terminal.Title})
			s.mu.Unlock()
			return func(asks bool) { s.asksInTerminal(turn, asks) }
		},
		ShowOutput: func(callID, text string) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.publish(protocol.ServerMessage{Value: protocol.ServerMessageToolOutput{CallID: callID, SessionID: session.meta.ID, Text: text}})
		},
		Images:      s.imagesFor(session),
		Jobs:        &sessionJobs{host: where.host, service: s, session: session},
		ReadFiles:   session.readFiles,
		Sandbox:     where.sandbox,
		Scratch:     where.scratch,
		Screenshots: s.screenshotsFor(session, main, sees),
		Skills:      where.skills,
		Subagents:   subagents{limit: ready.subagents, service: s, session: session, subagent: subagent, where: where},
		Terminals:   s.deps.Terminals,
		Todos:       s.deps.Todos,
		Web:         s.webFor(subagent),
		Workspace:   session.meta.Workspace,
	}
	return host
}

// afterEditOf is the command the session's project runs after each edit, as its settings say now; none for a
// daemon without projects (the tests of the sessions alone).
func (s *Service) afterEditOf(session *live) string {
	if s.deps.AfterEdit == nil {
		return ""
	}
	return s.deps.AfterEdit(session.meta.Workspace, session.meta.Host)
}

// asksInTerminal notes whether a command of the turn waits for the person in a terminal of the app: the session
// then waits for them, as it does on a request, and every app says so.
func (s *Service) asksInTerminal(turn *runningTurn, asks bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if turn.asksInTerminal == asks {
		return
	}
	turn.asksInTerminal = asks
	s.publishSessions()
}

func (h *turnHost) ToolContext() *tools.Context { return h.context }

func (h *turnHost) History() []protocol.SessionEvent {
	h.service.mu.Lock()
	defer h.service.mu.Unlock()
	return append([]protocol.SessionEvent{}, h.session.events...)
}

func (h *turnHost) Record(event protocol.SessionEventVariant) {
	h.service.mu.Lock()
	defer h.service.mu.Unlock()
	h.service.record(h.session, event)
}

// TakeCheckpoint closes the turn's restore point, once: the files it kept, for the turn's own event.
func (h *turnHost) TakeCheckpoint() []string {
	recorder := h.turn.recorder
	h.turn.recorder = nil
	if recorder == nil {
		return nil
	}
	files, err := recorder.Close()
	if err != nil {
		h.service.deps.LogError(err)
		return nil
	}
	return files
}

func (h *turnHost) StreamText(messageID, text string) {
	h.service.mu.Lock()
	defer h.service.mu.Unlock()
	sofar := ""
	if h.turn.streaming != nil && h.turn.streaming.MessageID == messageID {
		sofar = h.turn.streaming.Text
	}
	h.turn.streaming = &protocol.Streaming{MessageID: messageID, Text: sofar + text}
	h.service.publish(protocol.ServerMessage{Value: protocol.ServerMessageAssistantDelta{MessageID: messageID, SessionID: h.session.meta.ID, Text: text}})
}

func (h *turnHost) StreamReasoning(messageID, text string) {
	h.service.mu.Lock()
	defer h.service.mu.Unlock()
	h.service.publish(protocol.ServerMessage{Value: protocol.ServerMessageReasoningDelta{MessageID: messageID, SessionID: h.session.meta.ID, Text: text}})
}

func (h *turnHost) StreamToolCall(messageID string, tool protocol.ToolName) {
	h.service.mu.Lock()
	defer h.service.mu.Unlock()
	h.service.publish(protocol.ServerMessage{Value: protocol.ServerMessageToolCallWriting{MessageID: messageID, SessionID: h.session.meta.ID, Tool: tool}})
}

func (h *turnHost) NeedsPermission(subject permissions.Subject) bool {
	h.service.mu.Lock()
	defer h.service.mu.Unlock()
	return permissions.NeedsPermission(subject, h.service.grantsOf(h.session))
}

func (h *turnHost) AskPermission(ctx context.Context, ask agent.PermissionAsk) protocol.PermissionDecision {
	return h.service.ask(ctx, h.session, h.turn, permissionAsk{ask: ask, service: h.service}).decision
}

func (h *turnHost) AskQuestions(ctx context.Context, callID string, questions []protocol.Question) []string {
	return h.service.ask(ctx, h.session, h.turn, questionsAsked{callID: callID, questions: questions, service: h.service}).answers
}

// asked is what a turn asks a person, and how the turn takes the answer once it comes: the service records the
// request and the answer, publishes the session, and does what the answer means for the session.
type asked interface {
	// request is the event of asking, with what the person is asked in it.
	request(id string) protocol.SessionEventVariant
	// no is the answer of a turn that stopped before anyone answered.
	no() answer
	// settled is the turn taking the answer: recording it, and acting on it. The service holds its lock.
	settled(session *live, id string, got answer)
	// waitsFor says whether an answer is one of this kind, so a question's answer never settles a permission and
	// the answer of a turn that stopped is taken whichever way it comes.
	waitsFor(got answer) bool
}

// answer is what a person gives what they were asked: a permission, answers to the agent's questions, or the
// password a command asked for. What it holds says which kind of request it answers, so a question's answer never
// settles a permission.
type answer struct {
	// decision is a permission's answer, empty for the agent's questions.
	decision protocol.PermissionDecision
	// answers is the agent's questions' answers, nil for a permission.
	answers []string
	// secret is the password a command asked for, nil for the other kinds.
	secret *typedSecret
}

// typedSecret is the password a person typed for a command, or that they gave none (gave false).
type typedSecret struct {
	text string
	gave bool
}

// permissionAsk is a permission the agent asked for; allowed for the session, it grants what was asked for the
// session's whole life.
type permissionAsk struct {
	ask     agent.PermissionAsk
	service *Service
}

func (a permissionAsk) request(id string) protocol.SessionEventVariant {
	return protocol.SessionEventPermissionRequested{CallID: a.ask.CallID, Detail: a.ask.Preview, Outside: a.ask.Subject.Outside, RequestID: id}
}

func (permissionAsk) no() answer { return answer{decision: protocol.PermissionDecisionDeny} }

func (permissionAsk) waitsFor(got answer) bool { return got.answers == nil && got.secret == nil }

func (a permissionAsk) settled(session *live, id string, got answer) {
	a.service.record(session, protocol.SessionEventPermissionAnswered{Decision: got.decision, RequestID: id})
	switch got.decision {
	case protocol.PermissionDecisionAllowSession:
		granted := permissions.GrantForSession(a.ask.Subject, session.grants)
		session.grants = permissions.Grants{Edits: granted.Edits, Outside: granted.Outside, Programs: granted.Programs, Servers: granted.Servers}
	case protocol.PermissionDecisionAllowProject:
		// The project keeps it, and every session of it reads the project's rules at each call: this one too.
		if a.service.deps.AllowInProject != nil {
			a.service.deps.AllowInProject(session.meta.Workspace, session.meta.Host, permissions.AllowedBy(a.ask.Subject))
		}
	case protocol.PermissionDecisionAllowOnce, protocol.PermissionDecisionDeny:
	}
}

// questionsAsked are the agent's questions; stopping the turn answers none of them.
type questionsAsked struct {
	callID    string
	questions []protocol.Question
	service   *Service
}

func (q questionsAsked) request(id string) protocol.SessionEventVariant {
	return protocol.SessionEventQuestionAsked{CallID: q.callID, Questions: q.questions, RequestID: id}
}

func (questionsAsked) no() answer { return answer{answers: []string{}} }

func (questionsAsked) waitsFor(got answer) bool { return got.answers != nil }

func (q questionsAsked) settled(session *live, id string, got answer) {
	q.service.record(session, protocol.SessionEventQuestionAnswered{Answers: append([]string{}, got.answers...), RequestID: id})
}

// passwordAsk is the password a command this computer ran asked for: the person types it in, and it goes to the
// command, never kept in the session. Stopping the turn before anyone answers gives the command none.
type passwordAsk struct {
	callID  string
	prompt  string
	service *Service
}

func (a passwordAsk) request(id string) protocol.SessionEventVariant {
	return protocol.SessionEventPasswordRequested{CallID: a.callID, Prompt: a.prompt, RequestID: id}
}

func (passwordAsk) no() answer { return Secret("") }

func (passwordAsk) waitsFor(got answer) bool { return got.secret != nil }

func (a passwordAsk) settled(session *live, id string, got answer) {
	a.service.record(session, protocol.SessionEventPasswordAnswered{Gave: got.secret.gave, RequestID: id})
}

// ask asks a person something and waits: it records the request, waits for the answer or the turn's stopping, and
// records the answer it got either way. The service's lock is not held while it waits.
func (s *Service) ask(ctx context.Context, session *live, turn *runningTurn, what asked) answer {
	answered := make(chan answer, 1)
	s.mu.Lock()
	requestID := s.deps.NewID()
	settle := func(got answer) {
		if turn.pending == nil || turn.pending.requestID != requestID {
			return
		}
		turn.pending = nil
		what.settled(session, requestID, got)
		s.publishSessions()
		answered <- got
	}
	turn.pending = &pending{requestID: requestID, settle: settle, waitsFor: what.waitsFor}
	s.record(session, what.request(requestID))
	s.publishSessions()
	s.mu.Unlock()
	select {
	case got := <-answered:
		return got
	case <-ctx.Done():
		s.mu.Lock()
		settle(what.no())
		s.mu.Unlock()
		return <-answered
	}
}

func (h *turnHost) TakeMessages() *agent.PromptMessage { return h.service.takeQueued(h.session) }

func (h *turnHost) TakeReports() []protocol.SessionEventSubagentReport {
	return h.service.takeReports(h.session)
}

func (h *turnHost) UseModel(use func()) {
	h.session.model.Lock()
	defer h.session.model.Unlock()
	use()
}

// grantsOf is what the session lets through without asking: its mode, what people allowed for it, and what its
// project allows every session of it.
func (s *Service) grantsOf(session *live) permissions.Grants {
	grants := session.grants
	grants.Mode = session.meta.Mode
	if s.deps.Allowed == nil {
		return grants
	}
	return grants.With(s.deps.Allowed(session.meta.Workspace, session.meta.Host))
}

// sessionJobs is a session's background commands, on its host, in its folder: the session lists them while they
// run.
type sessionJobs struct {
	host    hosts.Host
	service *Service
	session *live
}

func (j *sessionJobs) Start(ctx context.Context, shown, command string) (string, error) {
	j.service.mu.Lock()
	workspace := j.session.meta.Workspace
	j.service.mu.Unlock()
	id, err := j.host.StartJob(ctx, command, workspace)
	if err != nil {
		return "", err
	}
	j.service.mu.Lock()
	j.session.startedJobs[id] = true
	j.session.jobs = append(j.session.jobs, protocol.SessionSummaryJob{Command: shown, ID: id})
	j.service.publishSessions()
	j.service.watchJobs(j.session, j.host)
	j.service.mu.Unlock()
	return id, nil
}

// started tells whether a job is one this session started: another session's is not there for it.
func (j *sessionJobs) started(id string) bool {
	j.service.mu.Lock()
	defer j.service.mu.Unlock()
	return j.session.startedJobs[id]
}

func (j *sessionJobs) Read(ctx context.Context, id string, consume bool) (*hosts.JobState, error) {
	if !j.started(id) {
		return nil, nil
	}
	state, err := j.host.ReadJob(ctx, id, consume)
	if err == nil {
		j.service.noteJob(j.session, id, state)
	}
	return state, err
}

// Stop stops a job: it leaves the list at once, its group ends within a moment.
func (j *sessionJobs) Stop(ctx context.Context, id string) (*hosts.JobState, error) {
	if !j.started(id) {
		return nil, nil
	}
	state, err := j.host.StopJob(ctx, id)
	j.service.mu.Lock()
	j.service.dropJob(j.session, id)
	j.service.mu.Unlock()
	return state, err
}

// noteJob drops a job seen ended, or unknown to its host, from the session's list.
func (s *Service) noteJob(session *live, id string, state *hosts.JobState) {
	if state != nil && state.Running {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropJob(session, id)
}

// dropJob takes a job off the session's list; the caller holds the lock.
func (s *Service) dropJob(session *live, id string) {
	kept := []protocol.SessionSummaryJob{}
	for _, job := range session.jobs {
		if job.ID != id {
			kept = append(kept, job)
		}
	}
	if len(kept) == len(session.jobs) {
		return
	}
	session.jobs = kept
	s.publishSessions()
}

// watchJobs looks at a session's jobs now and then, so the list shows the ones that ended by themselves; the caller
// holds the lock.
func (s *Service) watchJobs(session *live, host hosts.Host) {
	if session.watching {
		return
	}
	session.watching = true
	go func() {
		ticker := time.NewTicker(jobWatch)
		defer ticker.Stop()
		for range ticker.C {
			s.mu.Lock()
			_, exists := s.sessions[session.meta.ID]
			jobs := append([]protocol.SessionSummaryJob{}, session.jobs...)
			if !exists || len(jobs) == 0 {
				session.watching = false
				s.mu.Unlock()
				return
			}
			s.mu.Unlock()
			for _, job := range jobs {
				state, err := host.ReadJob(context.Background(), job.ID, false)
				if err != nil {
					s.deps.LogError(err)
					continue
				}
				s.noteJob(session, job.ID, state)
			}
		}
	}()
}

// StopJob stops a command the agent left running, at the person's word.
func (s *Service) StopJob(sessionID, jobID string) string {
	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		return unknownSession
	}
	go func() {
		jobs := &sessionJobs{host: s.hostOf(session), service: s, session: session}
		if _, err := jobs.Stop(context.Background(), jobID); err != nil {
			s.deps.LogError(err)
		}
	}()
	return ""
}

// screenshotsFor is a session's screenshots, when the daemon has a browser: looked at by the main model, which may
// see.
func (s *Service) screenshotsFor(session *live, main llm.Choice, sees bool) tools.Screenshots {
	if s.deps.Browser == nil {
		return nil
	}
	return &browser.Screenshots{
		Browser: s.deps.Browser, Folder: s.picturesFolderOf(session), Model: s.deps.Model, ModelName: main.Model,
		NewID: s.deps.NewID, Sees: sees, SessionID: session.meta.ID, Source: main.Source, Thinking: s.deps.Thinking.Subagents,
	}
}

// imagesFor is a session's pictures, when there is a picture server.
func (s *Service) imagesFor(session *live) images.Port {
	if s.deps.ImageURL == "" {
		return nil
	}
	return images.NewPort(s.deps.ImageURL, s.deps.ImagesFolder, s.deps.HTTP, session.meta.ID, s.deps.NewID)
}

// webFor is the web for a turn's tools: pages are read by the subagents' model, which is faster.
func (s *Service) webFor(subagent llm.Choice) *web.Port {
	port := web.NewPort(s.deps.Search, s.deps.Pages, web.Reader{
		Model: s.deps.Model, ModelName: subagent.Model, NewID: s.deps.NewID, Source: subagent.Source, Thinking: s.deps.Thinking.Subagents,
	})
	return &port
}

// subagents start explore subagents for a session's main agent, each in the background, as many at once as its
// preset lets work at once; the others wait their turn.
type subagents struct {
	service  *Service
	session  *live
	subagent llm.Choice
	where    place
	limit    *protocol.SubagentLimit
}

// runningSubagent is a subagent at work in the background: what it was asked, and how to stop it.
type runningSubagent struct {
	description string
	cancel      context.CancelFunc
}

// waitingSubagent is a subagent past its preset's limit: it starts once one at work reports.
type waitingSubagent struct {
	agentID     string
	callID      string
	description string
	model       string
	run         func(ctx context.Context) agent.Report
}

// roomFor tells whether one more subagent of a session may work now. A preset that came to allow none since lets one
// work, so those waiting still end.
func roomFor(limit *protocol.SubagentLimit, session *live) bool {
	return limit == nil || len(session.subagents) < max(limit.AtOnce, 1)
}

// Start starts the subagent in the background when the preset lets one more work, and has it wait its turn
// otherwise; its row shows at once, under the agent call. The call's result says which, and that the report comes
// later.
func (a subagents) Start(task tools.SubagentTask) tools.Outcome {
	s := a.service
	agentID := s.deps.NewID()
	deps := agent.SubagentDeps{
		ContextTokens: a.subagent.ContextTokens, Host: a.where.host, MaxSteps: s.deps.MaxSubagentSteps, Model: s.deps.Model,
		ModelName: a.subagent.Model, NewID: s.deps.NewID, Now: s.deps.Now, Platform: a.where.platform, Source: a.subagent.Source,
		Record: func(event protocol.SessionEventVariant) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.record(a.session, event)
		},
		Thinking: s.deps.Thinking.Subagents, Tools: s.deps.SubagentTools, Web: s.webFor(a.subagent), Workspace: a.session.meta.Workspace,
	}
	subagent := waitingSubagent{
		agentID: agentID, callID: task.CallID, description: task.Description, model: a.subagent.Model,
		run: func(ctx context.Context) agent.Report { return agent.RunSubagent(ctx, agentID, task, deps) },
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if roomFor(a.limit, a.session) {
		s.startSubagent(a.session, subagent)
		return tools.Outcome{Meta: "started", OK: true, Output: fmt.Sprintf(subagentStarted, task.Description) + reportComes}
	}
	s.record(a.session, protocol.SessionEventSubagentQueued{AgentID: agentID, CallID: task.CallID, Description: task.Description, Model: a.subagent.Model})
	a.session.waiting = append(a.session.waiting, subagent)
	s.publishSessions()
	before := len(a.session.waiting) - 1
	return tools.Outcome{Meta: "queued", OK: true, Output: fmt.Sprintf(subagentQueued, task.Description, a.limit.AtOnce, before) + reportComes}
}

// startSubagent records a subagent's start and runs it in the background; the caller holds the lock.
func (s *Service) startSubagent(session *live, subagent waitingSubagent) {
	s.record(session, protocol.SessionEventSubagentStarted{AgentID: subagent.agentID, CallID: subagent.callID, Description: subagent.description, Model: subagent.model})
	s.inBackground(session, subagent.agentID, subagent.description, subagent.run)
}

// startWaiting starts the subagents waiting their turn, first come first served, as many as the preset lets work at
// once now; the caller holds the lock.
func (s *Service) startWaiting(session *live) {
	if s.stopping {
		return
	}
	limit := s.deps.Models.SubagentLimit(session.meta.Preset)
	for len(session.waiting) > 0 && roomFor(limit, session) {
		next := session.waiting[0]
		session.waiting = session.waiting[1:]
		s.startSubagent(session, next)
	}
}

// dropWaiting ends the subagents still waiting their turn, as stopped: they never started. The caller holds the lock.
func (s *Service) dropWaiting(session *live) {
	for _, waiting := range session.waiting {
		s.finishSubagent(session, waiting.agentID, waiting.description, agent.Report{Outcome: protocol.TurnOutcomeStopped, Text: notStartedReport})
	}
	session.waiting = nil
}

// inBackground runs a subagent beside its session's turn, which may end first; the caller holds the lock. Once it is
// done, its end is recorded with its report, which waits for the main agent, and the next waiting subagent starts.
func (s *Service) inBackground(session *live, agentID, description string, run func(ctx context.Context) agent.Report) {
	ctx, cancel := context.WithCancel(context.Background())
	session.subagents[agentID] = runningSubagent{cancel: cancel, description: description}
	s.publishSessions()
	s.turns.Add(1)
	go func() {
		defer s.turns.Done()
		defer cancel()
		report := run(ctx)
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(session.subagents, agentID)
		s.finishSubagent(session, agentID, description, report)
		s.startWaiting(session)
		s.wakeForReports(session)
		s.publishSessions()
	}()
}

// finishSubagent records a subagent's end and keeps its report for the main agent; the caller holds the lock.
func (s *Service) finishSubagent(session *live, agentID, description string, report agent.Report) {
	s.record(session, protocol.SessionEventSubagentFinished{AgentID: agentID, Outcome: report.Outcome, Report: report.Text})
	session.reports = append(session.reports, pendingReport{
		report: protocol.SessionEventSubagentReport{AgentID: agentID, Description: description, Text: report.Text},
		wakes:  report.Outcome != protocol.TurnOutcomeStopped,
	})
}

// closeSubagents ends, as stopped, the subagents a restart left halfway, and keeps for the main agent the reports it
// never got; they wake nobody, they go with the person's next message.
func (s *Service) closeSubagents(session *live) {
	descriptions, running, reported := map[string]string{}, map[string]bool{}, map[string]bool{}
	finished := []protocol.SessionEventSubagentFinished{}
	for _, event := range session.events {
		switch value := event.Value.(type) {
		case protocol.SessionEventSubagentQueued:
			descriptions[value.AgentID], running[value.AgentID] = value.Description, true
		case protocol.SessionEventSubagentStarted:
			descriptions[value.AgentID], running[value.AgentID] = value.Description, true
		case protocol.SessionEventSubagentFinished:
			finished = append(finished, value)
			delete(running, value.AgentID)
		case protocol.SessionEventSubagentReport:
			reported[value.AgentID] = true
		}
	}
	for _, end := range finished {
		if end.Report == "" || reported[end.AgentID] {
			continue
		}
		session.reports = append(session.reports, pendingReport{report: protocol.SessionEventSubagentReport{
			AgentID: end.AgentID, Description: descriptions[end.AgentID], Text: end.Report,
		}})
	}
	for agentID := range running {
		s.finishSubagent(session, agentID, descriptions[agentID], agent.Report{Outcome: protocol.TurnOutcomeStopped, Text: restartedReport})
	}
}

func llmIdentity(meta Meta, main llm.Choice) llm.Identity {
	return llm.Identity{AgentID: meta.ID, Model: main.Model, Source: main.Source, Thinking: meta.Thinking}
}
