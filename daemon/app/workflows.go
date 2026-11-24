package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"swcode/daemon/agent"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	"swcode/daemon/workflows"
)

const (
	workflowHandoffFolder = ".swcode"
	workflowHandoffs      = "workflows"
	workflowInterrupted   = "swcode stopped while this run worked: its steps stopped too."
	// maxWorkflowRunSteps is how many step sessions a run may open: a loop that never says it is done stops here.
	maxWorkflowRunSteps = 100
)

// workflowRunner runs the workflows: a run session, and one session for each step it takes, each handed the previous
// step's report in a file. A step says which step it goes on to, so a workflow may loop until a step says it is done.
type workflowRunner struct {
	app      *App
	commands chan workflowCommand
	// steps finds the run step a session works on. Only the worker reads and writes it.
	steps  map[string]workflowStepRef
	states map[string]*workflowState
}

type workflowStepRef struct {
	runID  string
	stepID string
	// index is the run step in the run's steps: a step taken again has one of its own.
	index int
}

// workflowStepRun is what a step's session said before its turn ended.
type workflowStepRun struct {
	answer string
	report string
	exit   string
	chose  bool
}

type workflowState struct {
	run      protocol.WorkflowRun
	settings workflowSettings
	workflow protocol.Workflow
	project  protocol.Project
	// steps is each run step's answer and choice, by its index in the run.
	steps   map[int]*workflowStepRun
	stopped bool
}

type workflowCommand struct {
	run      protocol.WorkflowRun
	settings workflowSettings
	workflow protocol.Workflow
	project  protocol.Project
	message  *protocol.ServerMessage
	runID    string
	kind     string
}

// workflowSettings are how the run's sessions ask and think, as the person's composer picked them.
type workflowSettings struct {
	concise  bool
	mode     protocol.PermissionMode
	preset   string
	thinking protocol.ThinkingMode
}

const (
	commandStart = "start"
	commandEvent = "event"
	commandStop  = "stop"
)

func newWorkflowRunner(app *App) *workflowRunner {
	runner := &workflowRunner{
		app: app, commands: make(chan workflowCommand, 64), states: map[string]*workflowState{},
		steps: map[string]workflowStepRef{},
	}
	runner.stopInterrupted()
	go runner.work()
	return runner
}

// stopInterrupted marks the runs the last daemon left working as stopped: their turns are stopped with it, and a run
// starts again when the person asks for it.
func (r *workflowRunner) stopInterrupted() {
	_, runs := r.app.Workflows.List()
	for _, run := range runs {
		if run.Status != protocol.WorkflowRunStatusRunning && run.Status != protocol.WorkflowRunStatusWaiting {
			continue
		}
		run.Status = protocol.WorkflowRunStatusStopped
		run.Problem = workflowInterrupted
		for index, step := range run.Steps {
			if step.Status == protocol.WorkflowStepStatusRunning || step.Status == protocol.WorkflowStepStatusWaiting {
				run.Steps[index].Status = protocol.WorkflowStepStatusStopped
			}
		}
		_ = r.app.Workflows.UpdateRun(run)
	}
}

// observe hears the sessions' events; it only queues them, since it runs with the sessions' lock held.
func (r *workflowRunner) observe(message protocol.ServerMessage) {
	if _, ok := message.Value.(protocol.ServerMessageSessionEvent); !ok {
		return
	}
	select {
	case r.commands <- workflowCommand{kind: commandEvent, message: &message}:
	default:
		go func() { r.commands <- workflowCommand{kind: commandEvent, message: &message} }()
	}
}

func (r *workflowRunner) start(run protocol.WorkflowRun, workflow protocol.Workflow, project protocol.Project, settings workflowSettings) {
	r.commands <- workflowCommand{kind: commandStart, run: run, settings: settings, workflow: workflow, project: project}
}

func (r *workflowRunner) stop(runID string) {
	r.commands <- workflowCommand{kind: commandStop, runID: runID}
}

func (r *workflowRunner) work() {
	for command := range r.commands {
		r.handle(command)
	}
}

func (r *workflowRunner) handle(command workflowCommand) {
	switch command.kind {
	case commandStart:
		state := &workflowState{
			project: command.project, run: command.run, settings: command.settings,
			steps: map[int]*workflowStepRun{}, workflow: command.workflow,
		}
		r.states[state.run.ID] = state
		step, ok := firstStep(state.workflow)
		if !ok {
			r.fail(state, "This workflow has no step to start.")
			return
		}
		r.startStep(state, step, "")
	case commandEvent:
		if command.message != nil {
			r.event(*command.message)
		}
	case commandStop:
		state := r.states[command.runID]
		if state != nil {
			r.stopRun(state)
		}
	}
}

func (r *workflowRunner) event(message protocol.ServerMessage) {
	event, ok := message.Value.(protocol.ServerMessageSessionEvent)
	if !ok {
		return
	}
	ref, ok := r.steps[event.SessionID]
	if !ok {
		return
	}
	state := r.states[ref.runID]
	if state == nil || state.stopped {
		return
	}
	switch value := event.Event.Value.(type) {
	case protocol.SessionEventAssistantMessage:
		r.note(state, ref.index, value)
	case protocol.SessionEventTurnFinished:
		// A step that ended its turn only to wait for its subagents is still at work: their reports wake it for
		// the turn that hands off.
		if value.Outcome == protocol.TurnOutcomeDone && value.Message == agent.WaitingForReports {
			r.mark(state, ref.index, protocol.WorkflowStepStatusRunning, protocol.WorkflowRunStatusRunning)
			return
		}
		r.finish(state, ref, value)
	case protocol.SessionEventQuestionAsked, protocol.SessionEventPermissionRequested, protocol.SessionEventPasswordRequested:
		r.mark(state, ref.index, protocol.WorkflowStepStatusWaiting, protocol.WorkflowRunStatusWaiting)
	case protocol.SessionEventQuestionAnswered, protocol.SessionEventPermissionAnswered, protocol.SessionEventPasswordAnswered:
		r.mark(state, ref.index, protocol.WorkflowStepStatusRunning, protocol.WorkflowRunStatusRunning)
	}
}

// note keeps what a step's session answered and the handoff tool call it made, if one: the last one of a reply counts.
func (r *workflowRunner) note(state *workflowState, index int, message protocol.SessionEventAssistantMessage) {
	step := state.steps[index]
	if step == nil {
		step = &workflowStepRun{}
		state.steps[index] = step
	}
	step.answer = message.Text
	for _, call := range message.ToolCalls {
		if call.Name != string(protocol.ToolNameHandoff) {
			continue
		}
		var handoff struct {
			Exit   string `json:"exit"`
			Report string `json:"report"`
		}
		if json.Unmarshal([]byte(call.Arguments), &handoff) != nil {
			continue
		}
		step.exit = handoff.Exit
		step.report = handoff.Report
		step.chose = true
	}
}

func (r *workflowRunner) startStep(state *workflowState, step protocol.WorkflowStep, handoff string) {
	if state.stopped {
		return
	}
	if step.Kind == protocol.WorkflowStepKindStart {
		next := outgoingSteps(state.workflow, step.ID)
		if len(next) == 0 {
			r.fail(state, "The Start leads to no step.")
			return
		}
		r.startStep(state, next[0], handoff)
		return
	}
	if step.Kind == protocol.WorkflowStepKindEnd {
		state.run.Status = protocol.WorkflowRunStatusDone
		r.save(state)
		return
	}
	if len(state.run.Steps) >= maxWorkflowRunSteps {
		r.fail(state, fmt.Sprintf("The workflow ran %d steps: it may loop for good.", maxWorkflowRunSteps))
		return
	}
	request := stepRequest(state, step, handoff)
	session, refused := r.app.Sessions.Create(context.Background(), request)
	if refused != "" {
		r.fail(state, refused)
		return
	}
	if problem := r.app.Sessions.Rename(session.ID, step.Name); problem != "" {
		r.app.logger.Warn("the step's session could not be named", "problem", problem, "session", session.ID)
	}
	index := len(state.run.Steps)
	state.run.Steps = append(state.run.Steps, protocol.WorkflowRunStep{
		Name: step.Name, SessionID: session.ID, StartedAt: protocol.Timestamp(r.app.options.Now()),
		Status: protocol.WorkflowStepStatusRunning, StepID: step.ID,
	})
	state.steps[index] = &workflowStepRun{}
	r.steps[session.ID] = workflowStepRef{runID: state.run.ID, stepID: step.ID, index: index}
	state.run.Status = protocol.WorkflowRunStatusRunning
	r.save(state)
}

func (r *workflowRunner) finish(state *workflowState, ref workflowStepRef, event protocol.SessionEventTurnFinished) {
	index := ref.index
	if index < 0 || index >= len(state.run.Steps) {
		return
	}
	if state.stopped {
		r.closeStep(state, index, protocol.WorkflowStepStatusStopped)
		r.save(state)
		return
	}
	now := protocol.Timestamp(r.app.options.Now())
	state.run.Steps[index].FinishedAt = now
	answer := ""
	if said := state.steps[index]; said != nil {
		answer = said.report
		if answer == "" {
			answer = said.answer
		}
	}
	state.run.Steps[index].Summary = handoffSummary(answer)
	if event.Outcome != protocol.TurnOutcomeDone {
		status := protocol.WorkflowStepStatusFailed
		if event.Outcome == protocol.TurnOutcomeStopped {
			status = protocol.WorkflowStepStatusStopped
		}
		state.run.Steps[index].Status = status
		state.run.Status = protocol.WorkflowRunStatusFailed
		if event.Outcome == protocol.TurnOutcomeStopped {
			state.run.Status = protocol.WorkflowRunStatusStopped
		}
		state.run.Problem = event.Message
		if state.run.Problem == "" {
			state.run.Problem = fmt.Sprintf("%s did not finish.", state.run.Steps[index].Name)
		}
		r.save(state)
		return
	}
	step, ok := stepByID(state.workflow, ref.stepID)
	if !ok {
		state.run.Steps[index].Status = protocol.WorkflowStepStatusFailed
		r.fail(state, fmt.Sprintf("The workflow has no step %s.", ref.stepID))
		return
	}
	handoff, problem := r.writeHandoff(state, step, index, answer)
	if problem != "" {
		state.run.Steps[index].Status = protocol.WorkflowStepStatusFailed
		r.fail(state, problem)
		return
	}
	state.run.Steps[index].Status = protocol.WorkflowStepStatusDone
	state.run.Steps[index].Handoff = handoff
	next, more, problem := r.nextOf(state, step, index)
	if problem != "" {
		state.run.Steps[index].Status = protocol.WorkflowStepStatusFailed
		r.fail(state, problem)
		return
	}
	if !more {
		state.run.Status = protocol.WorkflowRunStatusDone
		r.save(state)
		return
	}
	state.run.Status = protocol.WorkflowRunStatusRunning
	r.save(state)
	r.startStep(state, next, handoff)
}

// nextOf is the step after this one: the one its handoff chose, or its only step after it when it chose none.
func (r *workflowRunner) nextOf(state *workflowState, step protocol.WorkflowStep, index int) (protocol.WorkflowStep, bool, string) {
	said := state.steps[index]
	if len(step.Exits) > 0 {
		if said == nil || !said.chose {
			return protocol.WorkflowStep{}, false, fmt.Sprintf("%s did not choose one of its outputs.", step.Name)
		}
		if said.exit == "" {
			return protocol.WorkflowStep{}, false, fmt.Sprintf("%s did not choose one of its outputs.", step.Name)
		}
		chosen := exitIDOf(step, said.exit)
		if chosen == "" {
			return protocol.WorkflowStep{}, false, fmt.Sprintf("%s chose %q, which is not one of its outputs.", step.Name, said.exit)
		}
		for _, edge := range state.workflow.Edges {
			if edge.From == step.ID && edge.Port == chosen {
				if next, ok := stepByID(state.workflow, edge.To); ok {
					if next.Kind == protocol.WorkflowStepKindEnd {
						return protocol.WorkflowStep{}, false, ""
					}
					return next, true, ""
				}
			}
		}
		return protocol.WorkflowStep{}, false, fmt.Sprintf("%s chose the output %s, which goes nowhere.", step.Name, chosen)
	}
	outgoing := outgoingSteps(state.workflow, step.ID)
	if said != nil && said.chose && said.exit != "" {
		return protocol.WorkflowStep{}, false, fmt.Sprintf("%s has no output %s.", step.Name, said.exit)
	}
	switch len(outgoing) {
	case 0:
		return protocol.WorkflowStep{}, false, ""
	case 1:
		if outgoing[0].Kind == protocol.WorkflowStepKindEnd {
			return protocol.WorkflowStep{}, false, ""
		}
		return outgoing[0], true, ""
	default:
		return protocol.WorkflowStep{}, false, fmt.Sprintf("%s did not choose where to go next.", step.Name)
	}
}

func (r *workflowRunner) mark(state *workflowState, index int, stepStatus protocol.WorkflowStepStatus, runStatus protocol.WorkflowRunStatus) {
	if index < 0 || index >= len(state.run.Steps) {
		return
	}
	state.run.Steps[index].Status = stepStatus
	state.run.Status = runStatus
	r.save(state)
}

// writeHandoff keeps a step's report in the project, where the next step reads it: a step taken again has its own file.
func (r *workflowRunner) writeHandoff(state *workflowState, step protocol.WorkflowStep, index int, answer string) (string, string) {
	if strings.TrimSpace(answer) == "" {
		answer = "This step ended without a report."
	}
	host := r.app.hosts.HostOf(state.project.Host)
	paths := host.Paths()
	folder := paths.Join(state.project.Path, workflowHandoffFolder, workflowHandoffs, state.run.ID)
	file := paths.Join(folder, fmt.Sprintf("%s-%d.md", step.ID, index+1))
	content := fmt.Sprintf("# %s\n\n%s\n", step.Name, answer)
	if err := host.WriteFile(context.Background(), file, []byte(content)); err != nil {
		return "", fmt.Sprintf("The handoff of %s could not be written: %s", step.Name, err.Error())
	}
	return paths.Rel(state.project.Path, file), ""
}

func (r *workflowRunner) fail(state *workflowState, problem string) {
	state.run.Status = protocol.WorkflowRunStatusFailed
	state.run.Problem = problem
	r.save(state)
}

func (r *workflowRunner) stopRun(state *workflowState) {
	state.stopped = true
	for index, step := range state.run.Steps {
		if step.Status != protocol.WorkflowStepStatusRunning && step.Status != protocol.WorkflowStepStatusWaiting {
			continue
		}
		if step.SessionID != "" {
			r.app.Sessions.Stop(step.SessionID)
		}
		r.closeStep(state, index, protocol.WorkflowStepStatusStopped)
	}
	state.run.Status = protocol.WorkflowRunStatusStopped
	r.save(state)
}

func (r *workflowRunner) closeStep(state *workflowState, index int, status protocol.WorkflowStepStatus) {
	state.run.Steps[index].Status = status
	state.run.Steps[index].FinishedAt = protocol.Timestamp(r.app.options.Now())
}

// save keeps the run as it is now; a run removed with its workflow leaves nothing to save.
func (r *workflowRunner) save(state *workflowState) {
	if problem := r.app.Workflows.UpdateRun(state.run); problem != "" {
		state.stopped = true
	}
}

// startWorkflowRun opens a run of a confirmed workflow: its run session, and the first step's session.
func (a *App) startWorkflowRun(ctx context.Context, message protocol.ClientMessageCreateSession) (protocol.SessionSummary, string) {
	host := protocol.LocalHost()
	if message.Host != nil {
		host = *message.Host
	}
	workspace := message.Workspace
	if workspace == "" {
		workspace = a.workspace
	}
	project, ok := a.Projects.Find(workspace, host)
	if !ok {
		return protocol.SessionSummary{}, "A workflow runs in one of this computer’s projects."
	}
	workflow, ok := a.Workflows.Workflow(message.WorkflowID)
	if !ok {
		return protocol.SessionSummary{}, "There is no such workflow."
	}
	if workflow.Draft {
		return protocol.SessionSummary{}, "This workflow is a draft: confirm it before a session uses it."
	}
	if workflow.ProjectID != "" && workflow.ProjectID != project.ID {
		return protocol.SessionSummary{}, "This workflow belongs to another project."
	}
	if problem := workflows.GraphProblem(workflow.Steps, workflow.Edges); problem != "" {
		return protocol.SessionSummary{}, problem
	}
	brief := strings.TrimSpace(message.Prompt)
	if brief == "" {
		return protocol.SessionSummary{}, "A workflow starts from a brief: write what the team has to do."
	}
	if message.Mode == "" {
		message.Mode = modeOf(project)
	}
	if message.Thinking == "" {
		message.Thinking = thinkingOf(project)
	}
	settings := workflowSettings{concise: message.Concise, mode: message.Mode, preset: message.Preset, thinking: message.Thinking}
	if settings.preset == "" {
		settings.preset = presetOf(project)
	}
	runID := a.options.NewID()
	runSession, refused := a.Sessions.Create(ctx, sessions.Request{
		WorkflowRunID: runID, Workspace: project.Path, Host: &project.Host, Kind: protocol.SessionKindCode,
		Concise: settings.concise, Mode: settings.mode, Preset: settings.preset, Thinking: settings.thinking,
	})
	if refused != "" {
		return protocol.SessionSummary{}, refused
	}
	if problem := a.Sessions.Rename(runSession.ID, "Workflow · "+workflow.Name); problem != "" {
		a.logger.Warn("the run's session could not be named", "problem", problem, "session", runSession.ID)
	}
	run := a.Workflows.NewRun(protocol.WorkflowRun{
		Brief: brief, ID: runID, ProjectID: project.ID, SessionID: runSession.ID,
		WorkflowID: workflow.ID, WorkflowName: workflow.Name,
	})
	a.workflowRuns.start(run, workflow, project, settings)
	return runSession, ""
}

// forgetWorkflowRun removes a run's record and its handoff files: its sessions are removed before, by RemoveRun.
func (a *App) forgetWorkflowRun(runID string) {
	if run, ok := a.Workflows.Run(runID); ok {
		if project, found := a.Projects.ByID(run.ProjectID); found {
			host := a.hosts.HostOf(project.Host)
			folder := host.Paths().Join(project.Path, workflowHandoffFolder, workflowHandoffs, run.ID)
			if err := host.RemoveFolder(context.Background(), folder); err != nil {
				a.logger.Warn("a run's handoff folder could not be removed", "problem", err.Error(), "run", run.ID)
			}
		}
	}
	_ = a.Workflows.ForgetRun(runID)
}

// stopWorkflowRun stops a run's step at work and starts no other.
func (a *App) stopWorkflowRun(runID string) {
	a.workflowRuns.stop(runID)
}

// proposeWorkflow keeps a workflow an agent proposed as a draft of its project, for the person to confirm.
func (a *App) proposeWorkflow(folder string, host protocol.HostRef, draft protocol.WorkflowDraft) (protocol.Workflow, string) {
	project, ok := a.Projects.Find(folder, host)
	if !ok {
		return protocol.Workflow{}, "This session’s folder is not a project."
	}
	if problem := workflows.GraphProblem(draft.Steps, draft.Edges); problem != "" {
		return protocol.Workflow{}, problem
	}
	draft.Draft = true
	draft.ProjectID = project.ID
	return a.Workflows.Save("", draft)
}

func stepRequest(state *workflowState, step protocol.WorkflowStep, handoff string) sessions.Request {
	preset := state.settings.preset
	if step.Preset != "" {
		preset = step.Preset
	}
	return sessions.Request{
		Concise: state.settings.concise, Host: &state.project.Host, Kind: protocol.SessionKindCode,
		Mode: state.settings.mode, Preset: preset, Thinking: state.settings.thinking,
		Workspace: state.project.Path, Prompt: stepPrompt(state.workflow, step, state.run.Brief, handoff),
		WorkflowRunID: state.run.ID, WorkflowStepID: step.ID,
	}
}

// stepPrompt is what a step's session starts on: its role, its task, the brief, where the previous step left its
// report, and the steps it may go on to.
func stepPrompt(workflow protocol.Workflow, step protocol.WorkflowStep, brief, handoff string) string {
	var text strings.Builder
	fmt.Fprintf(&text, "You are %s, a step of the workflow %s.\n", step.Name, workflow.Name)
	if step.Role != "" {
		fmt.Fprintf(&text, "\n%s\n", step.Role)
	}
	fmt.Fprintf(&text, "\n## Your task\n%s\n", step.Prompt)
	fmt.Fprintf(&text, "\n## The brief from the person\n%s\n", brief)
	if handoff != "" {
		fmt.Fprintf(&text, "\n## The previous step’s handoff\nRead %s: it holds the report and what that step made.\n", handoff)
	}
	if len(step.Exits) > 0 {
		text.WriteString("\n## Your outputs\nCall the `handoff` tool when you are done, with your report and `exit`, the output you take:\n")
		for _, exit := range step.Exits {
			target := ""
			for _, edge := range workflow.Edges {
				if edge.From == step.ID && edge.Port == exit.ID {
					if next, ok := stepByID(workflow, edge.To); ok {
						target = fmt.Sprintf(" → %s (%s)", next.Name, next.ID)
					}
				}
			}
			fmt.Fprintf(&text, "- %s — %q: take it when %s.%s\n", exit.ID, exit.Label, exit.When, target)
		}
		text.WriteString("\nChoose one of these outputs; the one that leads to End finishes the run.\n")
	} else {
		outgoing := outgoingSteps(workflow, step.ID)
		if len(outgoing) == 0 {
			text.WriteString("\n## Where to go next\nThis is the last step. Call the `handoff` tool when you are done, with your report and no `exit`.\n")
		} else {
			text.WriteString("\n## Where to go next\nCall the `handoff` tool when you are done, with your report and no `exit`. The workflow goes on to:\n")
			for _, next := range outgoing {
				fmt.Fprintf(&text, "- %s (%s)\n", next.Name, next.ID)
			}
		}
	}
	text.WriteString("\nWork in the project’s folder. Your report is what the next step reads.\n")
	return text.String()
}

func firstStep(workflow protocol.Workflow) (protocol.WorkflowStep, bool) {
	for _, step := range workflow.Steps {
		if step.Kind == protocol.WorkflowStepKindStart {
			return step, true
		}
	}
	incoming := map[string]int{}
	for _, edge := range workflow.Edges {
		incoming[edge.To]++
	}
	for _, step := range workflow.Steps {
		if step.Kind == protocol.WorkflowStepKindStep && incoming[step.ID] == 0 {
			return step, true
		}
	}
	return protocol.WorkflowStep{}, false
}

func outgoingSteps(workflow protocol.Workflow, stepID string) []protocol.WorkflowStep {
	next := []protocol.WorkflowStep{}
	for _, edge := range workflow.Edges {
		if edge.From != stepID {
			continue
		}
		if step, ok := stepByID(workflow, edge.To); ok {
			next = append(next, step)
		}
	}
	return next
}

// exitIDOf is the output an agent named, by its id or by the label it reads; empty when it names neither.
func exitIDOf(step protocol.WorkflowStep, name string) string {
	for _, exit := range step.Exits {
		if exit.ID == name || strings.EqualFold(exit.Label, name) {
			return exit.ID
		}
	}
	return ""
}

// handoffSummary is a step's report in one line for the run's list: the repeats of a step are told apart by what they
// said, not by their name alone.
func handoffSummary(report string) string {
	for _, line := range strings.Split(report, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		runes := []rune(trimmed)
		if len(runes) <= protocol.MaxWorkflowStepSummary {
			return trimmed
		}
		return string(runes[:protocol.MaxWorkflowStepSummary-1]) + "…"
	}
	return ""
}

func stepByID(workflow protocol.Workflow, stepID string) (protocol.WorkflowStep, bool) {
	for _, step := range workflow.Steps {
		if step.ID == stepID {
			return step, true
		}
	}
	return protocol.WorkflowStep{}, false
}

func presetOf(project protocol.Project) string {
	if project.Defaults == nil {
		return ""
	}
	return project.Defaults.Preset
}

func modeOf(project protocol.Project) protocol.PermissionMode {
	if project.Defaults == nil {
		return ""
	}
	return project.Defaults.Mode
}

func thinkingOf(project protocol.Project) protocol.ThinkingMode {
	if project.Defaults == nil {
		return ""
	}
	return project.Defaults.Thinking
}

func conciseOf(project protocol.Project) bool {
	return project.Defaults != nil && project.Defaults.Concise
}
