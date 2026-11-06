// Package agent is the agent loop: the person's message, then the model and its tools in turns until it answers
// without a tool call; compaction when the context fills up; explore subagents; the system prompts.
package agent

import (
	"context"

	"swcode/daemon/llm"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
	"swcode/daemon/tools"
)

// PermissionAsk is a call waiting for a person: what it touches, and what they see of it first.
type PermissionAsk struct {
	CallID  string
	Subject permissions.Subject
	Preview *protocol.ToolDisplay
}

// PromptMessage is what the person sent: their words, and the pictures they pasted, kept as files of the session.
// Concise: sent in token economy, the model is asked to be concise.
type PromptMessage struct {
	Text     string
	Pictures []protocol.PictureRef
	Concise  bool
}

// PictureView is how the model gets a session's pictures: as pictures when it sees them, from the session's
// folder.
type PictureView struct {
	Folder string
	Seen   bool
}

// TurnHost is what a turn needs from its session: the history, a place to record what happens, and a person to
// ask.
type TurnHost interface {
	ToolContext() *tools.Context
	History() []protocol.SessionEvent
	// Record keeps an event; the host dates it.
	Record(event protocol.SessionEventVariant)
	StreamText(messageID, text string)
	StreamReasoning(messageID, text string)
	// StreamToolCall says the model started writing a call to one of its tools.
	StreamToolCall(messageID string, tool protocol.ToolName)
	NeedsPermission(subject permissions.Subject) bool
	AskPermission(ctx context.Context, ask PermissionAsk) protocol.PermissionDecision
	// TakeMessages is what the person sent to come in now, during the turn: once, then it is the turn's.
	TakeMessages() *PromptMessage
	// TakeReports is the reports of the subagents that finished since: once, then they are the turn's.
	TakeReports() []protocol.SessionEventSubagentReport
	// UseModel runs use while the agent has its model to itself: a peek waits for it to end, then goes on the
	// agent's slot, where its conversation is cached.
	UseModel(use func())
}

// Setup is what an agent runs with.
type Setup struct {
	Agent        llm.Identity
	Model        llm.Model
	Tools        tools.Box
	SystemPrompt string
	NewID        func() string
	// MaxSteps is how many times the model may answer in one turn, so a model going round in circles stops.
	MaxSteps int
	// ContextTokens is how many tokens a slot of the agent's model holds (0 when unknown): past most of it, the
	// conversation is compacted.
	ContextTokens int
	// Skills is the user's skills as they are now, which the model is told of; nil for subagents.
	Skills func(ctx context.Context) []protocol.SkillSummary
	// Pictures is the session's pasted pictures; nil for a subagent, and the summary of a compaction.
	Pictures *PictureView
}

// Ending is how a turn ended, and what to tell people.
type Ending struct {
	Outcome protocol.TurnOutcome
	Message string
}

// TaskResult is how a task without a person ended, and what the agent answered last.
type TaskResult struct {
	Ending Ending
	Answer string
}
