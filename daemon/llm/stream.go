package llm

import (
	"sort"
	"strings"

	"swcode/daemon/protocol"
)

const (
	dataField  = "data:"
	errorField = "error:"
	// DoneMarker is the last event of a Chat Completions stream.
	DoneMarker = "[DONE]"
)

// SSEKind is what a line of a server-sent event stream holds.
type SSEKind string

const (
	SSEData  SSEKind = "data"
	SSEError SSEKind = "error"
	SSEOther SSEKind = "other"
)

// SSELine is one line of a server-sent event stream: its data, an error reported mid-stream, or anything else.
type SSELine struct {
	Kind SSEKind
	Data string
}

// ReadSSELine reads one line of the stream.
func ReadSSELine(line string) SSELine {
	if data, ok := strings.CutPrefix(line, dataField); ok {
		return SSELine{Data: strings.TrimSpace(data), Kind: SSEData}
	}
	if data, ok := strings.CutPrefix(line, errorField); ok {
		return SSELine{Data: strings.TrimSpace(data), Kind: SSEError}
	}
	return SSELine{Kind: SSEOther}
}

// ToolCallDelta is one streamed tool-call fragment: the first one names the call, the next ones add to its
// arguments.
type ToolCallDelta struct {
	Index     int
	ID        string
	Name      string
	Arguments string
}

// StreamChunk is one chunk of the stream, read from the API's shape.
type StreamChunk struct {
	Text      string
	Reasoning string
	ToolCalls []ToolCallDelta
	Usage     *protocol.Usage
}

type partialCall struct {
	index     int
	id        string
	name      string
	arguments strings.Builder
}

// ReplyBuilder gathers a streamed reply chunk by chunk, handing back the pieces worth showing as they come.
type ReplyBuilder struct {
	text      strings.Builder
	reasoning strings.Builder
	calls     []*partialCall
	usage     *protocol.Usage
}

// Add takes a chunk: the parts to show now.
func (b *ReplyBuilder) Add(chunk StreamChunk) []Part {
	b.text.WriteString(chunk.Text)
	b.reasoning.WriteString(chunk.Reasoning)
	if chunk.Usage != nil {
		b.usage = chunk.Usage
	}
	parts := []Part{}
	if chunk.Reasoning != "" {
		parts = append(parts, Part{Kind: PartReasoning, Text: chunk.Reasoning})
	}
	if chunk.Text != "" {
		parts = append(parts, Part{Kind: PartText, Text: chunk.Text})
	}
	for _, delta := range chunk.ToolCalls {
		if name := b.merge(delta); name != "" {
			parts = append(parts, Part{Kind: PartToolCall, Text: name})
		}
	}
	return parts
}

// merge adds a fragment: a new index starts a call, a known one adds to its arguments. It returns the call's
// name when this fragment is the one that gave it.
func (b *ReplyBuilder) merge(delta ToolCallDelta) string {
	for _, call := range b.calls {
		if call.index != delta.Index {
			continue
		}
		named := call.name
		call.arguments.WriteString(delta.Arguments)
		call.id = firstNonEmpty(call.id, delta.ID)
		call.name = firstNonEmpty(call.name, delta.Name)
		if named != "" {
			return ""
		}
		return call.name
	}
	call := &partialCall{id: delta.ID, index: delta.Index, name: delta.Name}
	call.arguments.WriteString(delta.Arguments)
	b.calls = append(b.calls, call)
	return call.name
}

func firstNonEmpty(first, second string) string {
	if first != "" {
		return first
	}
	return second
}

// Build is the reply whole: the calls in the order the model wrote them, one the server gave no id getting one.
func (b *ReplyBuilder) Build(newID func() string) Reply {
	calls := append([]*partialCall(nil), b.calls...)
	sort.SliceStable(calls, func(first, second int) bool { return calls[first].index < calls[second].index })
	toolCalls := []protocol.ToolCall{}
	for _, call := range calls {
		id := call.id
		if id == "" {
			id = newID()
		}
		toolCalls = append(toolCalls, protocol.ToolCall{Arguments: call.arguments.String(), ID: id, Name: call.name})
	}
	return Reply{
		Reasoning: strings.TrimSpace(b.reasoning.String()),
		Text:      strings.TrimSpace(b.text.String()),
		ToolCalls: toolCalls,
		Usage:     b.usage,
	}
}
