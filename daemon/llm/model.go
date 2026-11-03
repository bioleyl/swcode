// Package llm talks to llama-server: the Chat Completions API streamed, its status, and its slots shared out
// between the agents.
package llm

import (
	"context"

	"swcode/daemon/protocol"
)

// Role is who says a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Picture is a picture of a message: its file on the daemon, read when the request is sent.
type Picture struct {
	Path string
	Type string
}

// Message is a message of the conversation as the agent keeps it; the client turns it into the API's shape.
type Message struct {
	Role    Role
	Content string
	// Pictures of a user message, which the model sees after the text.
	Pictures []Picture
	// ToolCalls of an assistant message.
	ToolCalls []protocol.ToolCall
	// Reasoning is what the model thought before an assistant message.
	Reasoning string
	// ToolCallID is the call a tool message answers.
	ToolCallID string
}

// ToolDefinition is a tool as the model sees it: its name, what it does, and the JSON schema of its input.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Identity is which agent asks, the llama-server it asks (a source's id), the model it asks there (empty for the one a
// single-model llama-server serves), and how much it thinks: the scheduler gives it a slot of that model.
type Identity struct {
	AgentID  string
	Source   string
	Model    string
	Thinking protocol.ThinkingMode
}

// Choice is a model picked for an agent: its source and its name there, a slot's context (0 when the server does
// not say), and whether it sees pictures.
type Choice struct {
	Source        string
	Model         string
	ContextTokens int
	Vision        bool
}

// Target is where a request goes: the agent's model, on the slot the scheduler gave it (-1 for none).
type Target struct {
	Identity
	Slot int
}

// NoSlot leaves the slot to llama-server.
const NoSlot = -1

type Request struct {
	Messages []Message
	Tools    []ToolDefinition
	Target   Target
}

// PartKind tells the answer from the model's reasoning before it, and from the calls it writes.
type PartKind string

const (
	PartText      PartKind = "text"
	PartReasoning PartKind = "reasoning"
	// PartToolCall says the model started writing a call: its text is the tool's name, the arguments not shown.
	PartToolCall PartKind = "toolCall"
)

// Part is a piece of the reply as it streams in.
type Part struct {
	Kind PartKind
	Text string
}

type Reply struct {
	Text string
	// Reasoning is what the model thought before answering; empty when it does not say.
	Reasoning string
	ToolCalls []protocol.ToolCall
	Usage     *protocol.Usage
}

// Model answers a conversation, streaming its reply.
type Model interface {
	Complete(ctx context.Context, request Request, onPart func(Part)) (Reply, error)
}

// ModelError is llama-server not reached, refusing the request or breaking off: the message says which, for people.
type ModelError struct {
	Message string
	// ContextFull: the conversation no longer fits in the model's context, and compacting it lets the turn go on.
	ContextFull bool
}

func (e *ModelError) Error() string { return e.Message }
