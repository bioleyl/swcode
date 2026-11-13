package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"swcode/daemon/hosts"
	"swcode/daemon/llm"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
	"swcode/daemon/tools"
	"swcode/daemon/web"
)

const (
	noAnswer = "The subagent ended without an answer."
	// declinedNote tells the main agent why a subagent's task ended on a call: nobody approves a subagent's calls.
	declinedNote = "[The subagent’s task ended on a call the session asks about before it runs, and a subagent has no one to ask: %s. Do it yourself, so the person is asked.]"
)

// SubagentDeps is what a subagent runs with: its own model and tools, the main turn's context, and the session to
// report to.
type SubagentDeps struct {
	Model llm.Model
	// Source is the llm-server it asks, and ModelName the model there: a smaller, faster one than the main
	// agent's, typically.
	Source        string
	ModelName     string
	Thinking      protocol.ThinkingMode
	ContextTokens int
	Tools         tools.Box
	// Host is the session's host: a subagent reads where its main agent works.
	Host      hosts.Host
	Workspace string
	Platform  string
	MaxSteps  int
	Now       func() time.Time
	NewID     func() string
	// Record records an event in the main session: a line for each call the subagent made.
	Record func(event protocol.SessionEventVariant)
	Web    *web.Port
	// Defined is the agent the person defined that does the task; nil for an explore subagent.
	Defined *DefinedAgent
}

func answered(result TaskResult) string {
	if result.Answer == "" {
		return noAnswer
	}
	return result.Answer
}

func cutShort(note string) func(TaskResult) string {
	return func(result TaskResult) string {
		why := result.Ending.Message
		if why == "" {
			why = note
		}
		return strings.Join(nonEmpty(result.Answer, "["+why+"]"), "\n")
	}
}

func nonEmpty(parts ...string) []string {
	kept := []string{}
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return kept
}

// reports is what the main agent reads: the answer, and why it is short when the subagent did not finish.
var reports = map[protocol.TurnOutcome]func(TaskResult) string{
	protocol.TurnOutcomeDone:    answered,
	protocol.TurnOutcomeFailed:  cutShort("Failed"),
	protocol.TurnOutcomeLimit:   cutShort("Too many steps"),
	protocol.TurnOutcomeStopped: cutShort("Stopped"),
}

// Report is what a subagent gives its main agent: how it ended, and what it found or why it stopped short.
type Report struct {
	Outcome protocol.TurnOutcome
	Text    string
}

type subagentCall struct {
	tool    protocol.ToolName
	summary string
}

// subagentHost is where a subagent works: its conversation stays its own, in memory; the session only gets a line
// for each call it made. It has no one to ask, so anything beyond reading is declined.
type subagentHost struct {
	agentID string
	deps    SubagentDeps
	context *tools.Context
	mu      sync.Mutex
	events  []protocol.SessionEvent
	calls   map[string]subagentCall
	// declined is the call that needed a person's approval, which ended the task; empty when none did.
	declined string
}

func (h *subagentHost) ToolContext() *tools.Context { return h.context }

func (h *subagentHost) History() []protocol.SessionEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]protocol.SessionEvent{}, h.events...)
}

// Record keeps an event in the subagent's own conversation, and tells the session of a call that finished. Calls
// that only read run together, so it may be called from several of them at once.
func (h *subagentHost) Record(event protocol.SessionEventVariant) {
	h.mu.Lock()
	h.events = append(h.events, protocol.SessionEvent{Value: protocol.Dated(event, protocol.Timestamp(h.deps.Now()))})
	activity, finished := h.activityOf(event)
	h.mu.Unlock()
	if finished {
		h.deps.Record(activity)
	}
}

// StreamText does nothing: nobody reads a subagent's answer as it streams, the main agent gets it whole.
func (h *subagentHost) StreamText(string, string) {}

// StreamReasoning does nothing either.
func (h *subagentHost) StreamReasoning(string, string) {}

// StreamToolCall does nothing: the subagent's calls show once they are made.
func (h *subagentHost) StreamToolCall(string, protocol.ToolName) {}

// NeedsPermission: an explore subagent only reads. An agent the person defined works under its session's
// permissions: what the session lets through without asking, it does.
func (h *subagentHost) NeedsPermission(subject permissions.Subject) bool {
	if h.deps.Defined == nil || h.deps.Defined.Grants == nil {
		return subject.Access != permissions.AccessRead
	}
	return permissions.NeedsPermission(subject, h.deps.Defined.Grants())
}

// AskPermission declines: a subagent has no one to ask. The call is kept, for the report to say why the task ended.
func (h *subagentHost) AskPermission(_ context.Context, ask PermissionAsk) protocol.PermissionDecision {
	h.mu.Lock()
	defer h.mu.Unlock()
	call := h.calls[ask.CallID]
	h.declined = strings.TrimSpace(string(call.tool) + " " + call.summary)
	return protocol.PermissionDecisionDeny
}

// TakeCheckpoint is none: a subagent writes no file, and the turn's restore point is the main agent's.
func (h *subagentHost) TakeCheckpoint() []string { return nil }

// TakeMessages is none: the person talks to the main agent only.
func (h *subagentHost) TakeMessages() *PromptMessage { return nil }

// TakeReports is none: a subagent starts none of its own.
func (h *subagentHost) TakeReports() []protocol.SessionEventSubagentReport { return nil }

// UseModel: a subagent's requests take no peek, and wait for none.
func (h *subagentHost) UseModel(use func()) { use() }

// activityOf keeps a call's few words when it is made, and gives the line the session shows once it finished:
// false for any other event. The caller holds the lock.
func (h *subagentHost) activityOf(event protocol.SessionEventVariant) (protocol.SessionEventSubagentActivity, bool) {
	switch value := event.(type) {
	case protocol.SessionEventToolCall:
		h.calls[value.CallID] = subagentCall{summary: value.Summary, tool: value.Tool}
	case protocol.SessionEventToolFinished:
		call := h.calls[value.CallID]
		return protocol.SessionEventSubagentActivity{
			AgentID: h.agentID, Meta: value.Meta, OK: value.OK, Summary: call.summary, Tool: call.tool,
		}, true
	}
	return protocol.SessionEventSubagentActivity{}, false
}

// RunSubagent runs one subagent to its report: an explore one, or an agent the person defined. Its start and its end
// are its session's to record: the session hands the report to the main agent.
func RunSubagent(ctx context.Context, agentID string, task tools.SubagentTask, deps SubagentDeps) Report {
	host := &subagentHost{
		agentID: agentID,
		calls:   map[string]subagentCall{},
		// Its own reading does not count for the main agent's edits: the main agent reads a file itself.
		context: &tools.Context{Host: deps.Host, ReadFiles: tools.NewReadFiles(), Web: deps.Web, Workspace: deps.Workspace},
		deps:    deps,
	}
	prompt := ExplorePrompt(deps.Workspace, deps.Platform)
	if deps.Defined != nil {
		// Its commands run where the session's do, in its sandbox when it has one.
		host.context.Sandbox, host.context.Scratch = deps.Defined.Sandbox, deps.Defined.Scratch
		prompt = DefinedAgentPrompt(deps.Defined.Instructions, deps.Workspace, deps.Platform)
	}
	setup := Setup{
		Agent:         llm.Identity{AgentID: agentID, Model: deps.ModelName, Source: deps.Source, Thinking: deps.Thinking},
		ContextTokens: deps.ContextTokens,
		MaxSteps:      deps.MaxSteps,
		Model:         deps.Model,
		NewID:         deps.NewID,
		SystemPrompt:  prompt,
		Tools:         deps.Tools,
	}
	result := RunTask(ctx, task.Prompt, host, setup)
	// Its conversation ends here: its slot goes to the next agent, before its main agent's.
	llm.AgentDone(deps.Model, setup.Agent)
	text := reports[result.Ending.Outcome](result)
	host.mu.Lock()
	declined := host.declined
	host.mu.Unlock()
	if declined != "" {
		// Nobody stopped it: its task failed on that call, and its report wakes the main agent like any other.
		return Report{Outcome: protocol.TurnOutcomeFailed, Text: strings.Join(nonEmpty(result.Answer, fmt.Sprintf(declinedNote, declined)), "\n")}
	}
	return Report{Outcome: result.Ending.Outcome, Text: text}
}
