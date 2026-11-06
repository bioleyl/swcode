package agent

import (
	"context"
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

const noAnswer = "The subagent ended without an answer."

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
}

func (h *subagentHost) ToolContext() *tools.Context { return h.context }

func (h *subagentHost) History() []protocol.SessionEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]protocol.SessionEvent{}, h.events...)
}

func (h *subagentHost) Record(event protocol.SessionEventVariant) {
	h.mu.Lock()
	h.events = append(h.events, protocol.SessionEvent{Value: protocol.Dated(event, protocol.Timestamp(h.deps.Now()))})
	h.mu.Unlock()
	h.report(event)
}

// StreamText does nothing: nobody reads a subagent's answer as it streams, the main agent gets it whole.
func (h *subagentHost) StreamText(string, string) {}

// StreamReasoning does nothing either.
func (h *subagentHost) StreamReasoning(string, string) {}

// StreamToolCall does nothing: the subagent's calls show once they are made.
func (h *subagentHost) StreamToolCall(string, protocol.ToolName) {}

func (h *subagentHost) NeedsPermission(subject permissions.Subject) bool {
	return subject.Access != permissions.AccessRead
}

func (h *subagentHost) AskPermission(context.Context, PermissionAsk) protocol.PermissionDecision {
	return protocol.PermissionDecisionDeny
}

// TakeMessages is none: the person talks to the main agent only.
func (h *subagentHost) TakeMessages() *PromptMessage { return nil }

// TakeReports is none: a subagent starts none of its own.
func (h *subagentHost) TakeReports() []protocol.SessionEventSubagentReport { return nil }

// UseModel: a subagent's requests take no peek, and wait for none.
func (h *subagentHost) UseModel(use func()) { use() }

func (h *subagentHost) report(event protocol.SessionEventVariant) {
	switch value := event.(type) {
	case protocol.SessionEventToolCall:
		h.calls[value.CallID] = subagentCall{summary: value.Summary, tool: value.Tool}
	case protocol.SessionEventToolFinished:
		call := h.calls[value.CallID]
		h.deps.Record(protocol.SessionEventSubagentActivity{
			AgentID: h.agentID, Meta: value.Meta, OK: value.OK, Summary: call.summary, Tool: call.tool,
		})
	}
}

// RunSubagent runs one explore subagent to its report. Its start and its end are its session's to record: the
// session hands the report to the main agent.
func RunSubagent(ctx context.Context, agentID string, task tools.SubagentTask, deps SubagentDeps) Report {
	host := &subagentHost{
		agentID: agentID,
		calls:   map[string]subagentCall{},
		// Its own reading does not count for the main agent's edits: the main agent reads a file itself.
		context: &tools.Context{Host: deps.Host, ReadFiles: tools.NewReadFiles(), Web: deps.Web, Workspace: deps.Workspace},
		deps:    deps,
	}
	setup := Setup{
		Agent:         llm.Identity{AgentID: agentID, Model: deps.ModelName, Source: deps.Source, Thinking: deps.Thinking},
		ContextTokens: deps.ContextTokens,
		MaxSteps:      deps.MaxSteps,
		Model:         deps.Model,
		NewID:         deps.NewID,
		SystemPrompt:  ExplorePrompt(deps.Workspace, deps.Platform),
		Tools:         deps.Tools,
	}
	result := RunTask(ctx, task.Prompt, host, setup)
	// Its conversation ends here: its slot goes to the next agent, before its main agent's.
	llm.AgentDone(deps.Model, setup.Agent)
	return Report{Outcome: result.Ending.Outcome, Text: reports[result.Ending.Outcome](result)}
}
