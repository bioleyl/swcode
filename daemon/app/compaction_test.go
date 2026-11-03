package app_test

import (
	"strings"
	"testing"

	"swcode/daemon/agent"
	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

const (
	summaryRequest = "Summarize the conversation below"
	summary        = "The user asked what big.txt holds; it holds many lines of x. Nothing remains to do but answer."
)

func TestTheBudgetStartsFromTheLastMeasureAndAddsWhatCameAfter(t *testing.T) {
	events := []protocol.SessionEvent{
		{Value: protocol.SessionEventUserMessage{ID: "u", Text: "Read it"}},
		{Value: protocol.SessionEventAssistantMessage{ID: "a1", ToolCalls: []protocol.ToolCall{{Arguments: "{}", ID: "c1", Name: "read"}}, Usage: &protocol.Usage{CompletionTokens: 20, PromptTokens: 1000}}},
		{Value: protocol.SessionEventToolFinished{CallID: "c1", OK: true, Output: strings.Repeat("x", 3000)}},
	}
	if got := agent.EstimatedTokens(events, 0); got != 2020 {
		t.Fatalf("got %d", got)
	}
	if !agent.NeedsCompaction(events, 0, 2400) || agent.NeedsCompaction(events, 0, 4000) {
		t.Fatal("compaction")
	}
	long := []protocol.SessionEvent{{Value: protocol.SessionEventUserMessage{Text: strings.Repeat("x", 100_000)}}}
	if agent.NeedsCompaction(long, 0, 0) {
		t.Fatal("compacted without knowing the context")
	}
	compacted := append(long, protocol.SessionEvent{Value: protocol.SessionEventContextCompacted{ID: "s1", Summary: "short"}})
	if got := agent.EstimatedTokens(compacted, 300); got != 102 {
		t.Fatalf("got %d", got)
	}
}

func TestTheConversationAfterACompactionStartsWithTheSummary(t *testing.T) {
	events := []protocol.SessionEvent{
		{Value: protocol.SessionEventUserMessage{At: "2026-10-23T12:00:00.000Z", ID: "u1", Text: "old request"}},
		{Value: protocol.SessionEventContextCompacted{ID: "s1", Summary: "What happened so far."}},
		{Value: protocol.SessionEventUserMessage{At: "2026-10-23T12:00:00.000Z", ID: "u2", Text: "new request"}},
	}
	messages := agent.ConversationOf(events, nil)
	if len(messages) != 1 || !strings.HasPrefix(messages[0].Content, "The conversation so far was summarized") || !strings.HasSuffix(messages[0].Content, "new request") || strings.Contains(messages[0].Content, "old request") {
		t.Fatalf("got %+v", messages)
	}
	transcript := agent.TranscriptOf([]protocol.SessionEvent{
		{Value: protocol.SessionEventUserMessage{Text: "first"}}, {Value: protocol.SessionEventUserMessage{Text: "second"}}, {Value: protocol.SessionEventUserMessage{Text: "third"}},
	}, 30)
	if transcript != "User: second\n\nUser: third" {
		t.Fatalf("got %q", transcript)
	}
}

func TestATurnSummarizesOnceTheConversationNearsTheEndOfTheContext(t *testing.T) {
	// The system prompt and the tools take about 4,200 tokens; big.txt, about 4,000 more.
	test := Start(t, Options{
		ContextTokens: 6000,
		Files:         map[string]string{"big.txt": strings.Repeat(strings.Repeat("x", 60)+"\n", 200)},
		Replies:       []fakellama.Reply{call("read", map[string]any{"path": "big.txt"}), text("It holds lines of x.")},
		Scripts:       map[string][]fakellama.Reply{summaryRequest: {{Text: summary}}},
	})
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "What does big.txt hold?", nil)
	test.WaitForEvent("turnFinished", 1)
	events := test.Events(session.ID)
	Expect(t, OfKind(events, "contextCompacted"), []any{map[string]any{"summary": summary}})
	Expect(t, last(events), `{"outcome":"done"}`)
	requests := test.Requests()
	for _, request := range requests {
		if strings.Contains(RequestText(request), summaryRequest) && request["tools"] != nil {
			t.Fatal("the summary request has tools")
		}
	}
	final := RequestText(map[string]any{"messages": requests[len(requests)-1]["messages"]})
	if !strings.Contains(final, summary) || strings.Contains(final, strings.Repeat("x", 60)) {
		t.Fatal("the last request did not go on from the summary")
	}
}

func TestATurnCompactsAndTriesAgainWhenLlamaServerFindsTheContextFull(t *testing.T) {
	test := Start(t, Options{
		Replies: []fakellama.Reply{{ErrorType: "exceed_context_size_error", FailWith: 400, Text: "the request exceeds the available context size"}, text("Recovered.")},
		Scripts: map[string][]fakellama.Reply{summaryRequest: {{Text: summary}}},
	})
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "Go on", nil)
	test.WaitForEvent("turnFinished", 1)
	Expect(t, Kinds(test.Events(session.ID)), []any{"turnStarted", "userMessage", "contextCompacted", "assistantMessage", "turnFinished"})
	Expect(t, last(test.Events(session.ID)), `{"outcome":"done"}`)
}

func TestReasoningStreamsApartFromTheAnswerAndStaysWithTheReply(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{{Reasoning: "The user greets me.", Text: "Hello!"}}})
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "Hi", nil)
	test.WaitForEvent("turnFinished", 1)
	streamed := ""
	for _, message := range test.Messages() {
		if delta, ok := message.Value.(protocol.ServerMessageReasoningDelta); ok {
			streamed += delta.Text
		}
	}
	if streamed != "The user greets me." {
		t.Fatalf("streamed %q", streamed)
	}
	Expect(t, OfKind(test.Events(session.ID), "assistantMessage"), `[{"reasoning":"The user greets me.","text":"Hello!"}]`)
}
