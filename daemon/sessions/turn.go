package sessions

import (
	"context"
	"fmt"
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

// place is where a turn works: its host, that host's system, the session's scratch folder there, and its skills.
type place struct {
	host     hosts.Host
	platform string
	scratch  string
	skills   catalog.Port
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
	where := place{host: host, platform: platform, scratch: scratch, skills: s.deps.Skills.PortFor(catalog.Place{
		Host: host, Off: s.deps.SkillsOff(meta.Workspace, meta.Host), Workspace: meta.Workspace,
	})}
	return prepared{
		main: main, setup: s.setupFor(session, meta, main, where, main.Vision, limit), subagent: subagent, subagents: limit, where: where,
	}, nil
}

// runTurnIn runs a turn with what the session's agent runs with now.
func (s *Service) runTurnIn(ctx context.Context, session *live, turn *runningTurn, message *agent.PromptMessage) {
	ready, err := s.prepare(ctx, session)
	if err != nil {
		s.failTurn(session, message, err)
		return
	}
	s.mu.Lock()
	turn.setup = &ready.setup
	s.mu.Unlock()
	agent.RunTurn(ctx, message, s.hostFor(session, turn, ready, ready.main.Vision), ready.setup)
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

// toolsFor is the main agent's tools: without agent when the preset allows no subagents.
func (s *Service) toolsFor(limit *protocol.SubagentLimit) tools.Box {
	if limit != nil && limit.AtOnce == 0 {
		return s.deps.Tools.Without(protocol.ToolNameAgent)
	}
	return s.deps.Tools
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
		Skills:        where.skills.List,
		SystemPrompt: agent.SystemPrompt(meta.Workspace, where.scratch, where.platform, agent.Servers{
			Browser: s.deps.Browser != nil && sees, Images: s.deps.ImageURL != "",
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
			return s.askQuestions(ctx, session, turn, callID, questions)
		},
		Host:        where.host,
		Images:      s.imagesFor(session),
		Jobs:        &sessionJobs{host: where.host, service: s, session: session},
		ReadFiles:   session.readFiles,
		Scratch:     where.scratch,
		Screenshots: s.screenshotsFor(session, main, sees),
		Skills:      where.skills,
		Subagents:   subagents{limit: ready.subagents, service: s, session: session, subagent: subagent, where: where},
		Web:         s.webFor(subagent),
		Workspace:   session.meta.Workspace,
	}
	return host
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
	return h.service.askPermission(ctx, h.session, h.turn, ask)
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

func (s *Service) grantsOf(session *live) permissions.Grants {
	grants := session.grants
	grants.Mode = session.meta.Mode
	return grants
}

// askPermission waits for a person's answer; stopping the turn counts as a no.
func (s *Service) askPermission(ctx context.Context, session *live, turn *runningTurn, ask agent.PermissionAsk) protocol.PermissionDecision {
	decided := make(chan protocol.PermissionDecision, 1)
	s.mu.Lock()
	requestID := s.deps.NewID()
	settle := func(decision protocol.PermissionDecision) {
		if turn.pending == nil || turn.pending.requestID != requestID {
			return
		}
		turn.pending = nil
		s.record(session, protocol.SessionEventPermissionAnswered{Decision: decision, RequestID: requestID})
		if decision == protocol.PermissionDecisionAllowSession {
			granted := permissions.GrantForSession(ask.Subject, s.grantsOf(session))
			session.grants = permissions.Grants{Edits: granted.Edits, Programs: granted.Programs}
		}
		s.publishSessions()
		decided <- decision
	}
	turn.pending = &pending{decide: settle, requestID: requestID}
	s.record(session, protocol.SessionEventPermissionRequested{CallID: ask.CallID, Detail: ask.Preview, RequestID: requestID})
	s.publishSessions()
	s.mu.Unlock()
	select {
	case decision := <-decided:
		return decision
	case <-ctx.Done():
		s.mu.Lock()
		settle(protocol.PermissionDecisionDeny)
		s.mu.Unlock()
		return <-decided
	}
}

// askQuestions waits for a person's answers to the agent's questions; stopping the turn gives none.
func (s *Service) askQuestions(ctx context.Context, session *live, turn *runningTurn, callID string, questions []protocol.Question) []string {
	answered := make(chan []string, 1)
	s.mu.Lock()
	requestID := s.deps.NewID()
	settle := func(answers []string) {
		if turn.pending == nil || turn.pending.requestID != requestID {
			return
		}
		turn.pending = nil
		s.record(session, protocol.SessionEventQuestionAnswered{Answers: append([]string{}, answers...), RequestID: requestID})
		s.publishSessions()
		answered <- answers
	}
	turn.pending = &pending{answer: settle, question: true, requestID: requestID}
	s.record(session, protocol.SessionEventQuestionAsked{CallID: callID, Questions: questions, RequestID: requestID})
	s.publishSessions()
	s.mu.Unlock()
	select {
	case answers := <-answered:
		return answers
	case <-ctx.Done():
		s.mu.Lock()
		settle([]string{})
		s.mu.Unlock()
		return <-answered
	}
}

// sessionJobs is a session's background commands, on its host, in its folder: the session lists them while they
// run.
type sessionJobs struct {
	host    hosts.Host
	service *Service
	session *live
}

func (j *sessionJobs) Start(ctx context.Context, command string) (string, error) {
	j.service.mu.Lock()
	workspace := j.session.meta.Workspace
	j.service.mu.Unlock()
	id, err := j.host.StartJob(ctx, command, workspace)
	if err != nil {
		return "", err
	}
	j.service.mu.Lock()
	j.session.jobs = append(j.session.jobs, protocol.SessionSummaryJob{Command: command, ID: id})
	j.service.publishSessions()
	j.service.watchJobs(j.session, j.host)
	j.service.mu.Unlock()
	return id, nil
}

func (j *sessionJobs) Read(ctx context.Context, id string, consume bool) (*hosts.JobState, error) {
	state, err := j.host.ReadJob(ctx, id, consume)
	if err == nil {
		j.service.noteJob(j.session, id, state)
	}
	return state, err
}

// Stop stops a job: it leaves the list at once, its group ends within a moment.
func (j *sessionJobs) Stop(ctx context.Context, id string) (*hosts.JobState, error) {
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
	port := web.NewPort(s.deps.Search, s.deps.HTTP, web.Reader{
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
