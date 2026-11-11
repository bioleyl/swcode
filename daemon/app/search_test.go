package app_test

import (
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

// talk starts a session of a kind on a message, and waits for the model's answer.
func talk(t *testing.T, test *App, kind protocol.SessionKind, message string) string {
	t.Helper()
	session := test.NewSession(sessions.Request{Kind: kind})
	test.App.Sessions.SendPrompt(session.ID, message, nil)
	test.UntilIdle(session.ID)
	return session.ID
}

func TestASearchFindsTheConversationsThatSayEveryWordNewestFirst(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{
		text("The cache keeps the prompt between two steps."),
		text("Paris is the capital of France."),
		text("The prompt CACHE is lost when the system prompt changes."),
	}})
	older := talk(t, test, protocol.SessionKindCode, "How does the prompt cache work?")
	talk(t, test, protocol.SessionKindChat, "What is the capital of France?")
	newer := talk(t, test, protocol.SessionKindChat, "When is it lost?")
	matches := test.App.Sessions.Search("Prompt cache")
	if len(matches) != 2 || matches[0].SessionID != newer || matches[1].SessionID != older {
		t.Fatalf("matches %+v", matches)
	}
	if matches[0].Snippet != "The prompt CACHE is lost when the system prompt changes." {
		t.Fatalf("snippet %q", matches[0].Snippet)
	}
	if answer := lastAnswer(test.Events(newer)); matches[0].EventID != answer.ID {
		t.Fatalf("event id %q, want the answer %q", matches[0].EventID, answer.ID)
	}
	if len(test.App.Sessions.Search("cache France")) != 0 {
		t.Fatal("found words said in two conversations")
	}
}

// lastAnswer is the model's last answer of a session.
func lastAnswer(events []protocol.SessionEvent) protocol.SessionEventAssistantMessage {
	var answer protocol.SessionEventAssistantMessage
	for _, event := range events {
		if value, ok := event.Value.(protocol.SessionEventAssistantMessage); ok {
			answer = value
		}
	}
	return answer
}

func TestASearchReadsTheToolCallsButNotWhatTheyGaveBack(t *testing.T) {
	test := Start(t, Options{
		Files:   map[string]string{"src/session-store.ts": "export const secretWord = 1;\n"},
		Replies: []fakellama.Reply{call("read", map[string]any{"path": "src/session-store.ts"}), text("Read.")},
	})
	sessionID := talk(t, test, protocol.SessionKindCode, "Look at the store")
	matches := test.App.Sessions.Search("session-store")
	if len(matches) != 1 || matches[0].SessionID != sessionID || !strings.Contains(matches[0].Snippet, "read src/session-store.ts") {
		t.Fatalf("matches %+v", matches)
	}
	if call := lastCall(test.Events(sessionID)); matches[0].EventID != call.CallID {
		t.Fatalf("event id %q, want the call %q", matches[0].EventID, call.CallID)
	}
	if len(test.App.Sessions.Search("secretWord")) != 0 {
		t.Fatal("found a word of a file the tool read")
	}
}

// lastCall is the agent's last tool call in a session.
func lastCall(events []protocol.SessionEvent) protocol.SessionEventToolCall {
	var call protocol.SessionEventToolCall
	for _, event := range events {
		if value, ok := event.Value.(protocol.SessionEventToolCall); ok {
			call = value
		}
	}
	return call
}

func TestASnippetKeepsAFewWordsAroundOnOneLine(t *testing.T) {
	long := strings.Repeat("filler ", 20) + "needle\nin the\nhaystack " + strings.Repeat("filler ", 20)
	test := Start(t, Options{Replies: []fakellama.Reply{text(long)}})
	talk(t, test, protocol.SessionKindChat, "Where is it?")
	snippet := test.App.Sessions.Search("NEEDLE")[0].Snippet
	if !strings.HasPrefix(snippet, "…") || !strings.HasSuffix(snippet, "…") || !strings.Contains(snippet, "needle in the haystack") {
		t.Fatalf("snippet %q", snippet)
	}
}
