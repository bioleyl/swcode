package agent

import (
	"context"
	"strings"
	"testing"

	"swcode/daemon/llm"
	"swcode/daemon/protocol"
)

// recording is a model that answers with its reply, streaming its text, and keeps the request it was asked.
type recording struct {
	reply   llm.Reply
	request llm.Request
}

func (m *recording) Complete(_ context.Context, request llm.Request, onPart func(llm.Part)) (llm.Reply, error) {
	m.request = request
	onPart(llm.Part{Kind: llm.PartText, Text: m.reply.Text})
	return m.reply, nil
}

func peekWith(model llm.Model) (Setup, *Peek, *string, *string) {
	answer, problem := "", "unanswered"
	peek := &Peek{At: at, Done: func(text string) { problem = text }, Question: "What are you doing?", Text: func(text string) { answer += text }}
	setup := Setup{Agent: llm.Identity{AgentID: "s1"}, Model: model, NewID: func() string { return "id" }, SystemPrompt: "You are swcode."}
	return setup, peek, &answer, &problem
}

func TestAPeekAsksAfterTheConversationAndACallOfTheTurnRunningIsStillRunning(t *testing.T) {
	read := protocol.ToolCall{Arguments: `{"path":"a"}`, ID: "c1", Name: "read"}
	grep := protocol.ToolCall{Arguments: `{"pattern":"b"}`, ID: "c2", Name: "grep"}
	history := events(
		protocol.SessionEventTurnStarted{ID: "t1"},
		protocol.SessionEventUserMessage{ID: "u1", Text: "Read a"},
		protocol.SessionEventAssistantMessage{ID: "a1", ToolCalls: []protocol.ToolCall{read}},
		protocol.SessionEventTurnFinished{ID: "t1", Outcome: protocol.TurnOutcomeStopped},
		protocol.SessionEventTurnStarted{ID: "t2"},
		protocol.SessionEventUserMessage{ID: "u2", Text: "Find b"},
		protocol.SessionEventAssistantMessage{ID: "a2", ToolCalls: []protocol.ToolCall{grep}},
	)
	model := &recording{reply: llm.Reply{Text: "Looking for b."}}
	setup, peek, answer, problem := peekWith(model)
	AnswerPeek(context.Background(), history, setup, *peek)
	messages := model.request.Messages
	results := map[string]string{}
	for _, message := range messages {
		if message.Role == llm.RoleTool {
			results[message.ToolCallID] = message.Content
		}
	}
	if results["c1"] != notRun || results["c2"] != stillRunning {
		t.Fatalf("results %v", results)
	}
	asked := messages[len(messages)-1]
	if asked.Role != llm.RoleUser || !strings.HasSuffix(asked.Content, "What are you doing?") {
		t.Fatalf("asked %+v", asked)
	}
	if model.request.Target.AgentID != "s1" || *answer != "Looking for b." || *problem != "" {
		t.Fatalf("target %+v, answer %q, problem %q", model.request.Target, *answer, *problem)
	}
}

func TestAPeekAnsweredWithACallInsteadOfWordsSaysSo(t *testing.T) {
	model := &recording{reply: llm.Reply{ToolCalls: []protocol.ToolCall{{ID: "c1", Name: "read"}}}}
	setup, peek, _, problem := peekWith(model)
	AnswerPeek(context.Background(), events(protocol.SessionEventUserMessage{ID: "u1", Text: "Hi"}), setup, *peek)
	if *problem != noPeekAnswer {
		t.Fatalf("problem %q", *problem)
	}
}
