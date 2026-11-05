package sessions

import (
	"context"
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

// prepared is what the session's agent runs with now: its place, its models, and its setup.
type prepared struct {
	where    place
	main     llm.Choice
	subagent llm.Choice
	setup    agent.Setup
}

// prepare finds the models and context sizes llm-server gives now (a model may have been reloaded with another
// one, or a router may serve others), and where the session works.
func (s *Service) prepare(ctx context.Context, session *live) (prepared, error) {
	s.mu.Lock()
	meta := session.meta
	s.mu.Unlock()
	main, subagent := s.deps.Models.Choose(ctx, meta.Preset)
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
		Host: host, Local: hosts.IsLocal(meta.Host), SessionID: meta.ID, Workspace: meta.Workspace,
	})}
	return prepared{main: main, setup: s.setupFor(session, meta, main, where, main.Vision), subagent: subagent, where: where}, nil
}

// runTurnIn runs a turn with what the session's agent runs with now.
func (s *Service) runTurnIn(ctx context.Context, session *live, turn *runningTurn, message agent.PromptMessage) {
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

// failTurn records a turn whose host did not answer: the person's message, and why nothing came of it.
func (s *Service) failTurn(session *live, message agent.PromptMessage, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	turnID := s.deps.NewID()
	s.record(session, protocol.SessionEventTurnStarted{ID: turnID})
	s.record(session, agent.UserMessage(s.deps.NewID(), message))
	s.record(session, protocol.SessionEventTurnFinished{ID: turnID, Message: "The host did not answer: " + err.Error(), Outcome: protocol.TurnOutcomeFailed})
}

func (s *Service) setupFor(session *live, meta Meta, main llm.Choice, where place, sees bool) agent.Setup {
	return agent.Setup{
		Agent:         llmIdentity(meta, main),
		ContextTokens: main.ContextTokens,
		MaxSteps:      s.deps.MaxSteps,
		Model:         s.deps.Model,
		NewID:         s.deps.NewID,
		Pictures:      &agent.PictureView{Folder: s.picturesFolderOf(session), Seen: sees},
		Skills:        where.skills.List,
		SystemPrompt: agent.SystemPrompt(meta.Workspace, where.scratch, where.platform, agent.Servers{
			Browser: s.deps.Browser != nil && sees, Images: s.deps.ImageURL != "",
		}),
		Tools: s.deps.Tools,
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
		Subagents:   subagents{service: s, session: session, subagent: subagent, where: where},
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

// subagents start explore subagents for a session's main agent.
type subagents struct {
	service  *Service
	session  *live
	subagent llm.Choice
	where    place
}

func (a subagents) Run(ctx context.Context, task tools.SubagentTask) tools.Outcome {
	s := a.service
	return agent.RunSubagent(ctx, task, agent.SubagentDeps{
		ContextTokens: a.subagent.ContextTokens, Host: a.where.host, MaxSteps: s.deps.MaxSubagentSteps, Model: s.deps.Model,
		ModelName: a.subagent.Model, NewID: s.deps.NewID, Now: s.deps.Now, Platform: a.where.platform, Source: a.subagent.Source,
		Record: func(event protocol.SessionEventVariant) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.record(a.session, event)
		},
		Thinking: s.deps.Thinking.Subagents, Tools: s.deps.SubagentTools, Web: s.webFor(a.subagent), Workspace: a.session.meta.Workspace,
	})
}

func llmIdentity(meta Meta, main llm.Choice) llm.Identity {
	return llm.Identity{AgentID: meta.ID, Model: main.Model, Source: main.Source, Thinking: meta.Thinking}
}
