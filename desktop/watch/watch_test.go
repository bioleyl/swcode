package watch_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
	"swcode/desktop/watch"
)

const noticeWait = 10 * time.Second

func sessionsMessage(id, title string) protocol.ServerMessage {
	return protocol.ServerMessage{Value: protocol.ServerMessageSessions{Sessions: []protocol.SessionSummary{{ID: id, Title: title}}}}
}

func eventMessage(id string, event protocol.SessionEventVariant) protocol.ServerMessage {
	return protocol.ServerMessage{Value: protocol.ServerMessageSessionEvent{Event: protocol.SessionEvent{Value: event}, SessionID: id}}
}

func expectNotice(t *testing.T, found watch.Notice, told bool, expected watch.Notice) {
	t.Helper()
	if !told || found != expected {
		t.Fatalf("expected %+v, found %+v (told: %t)", expected, found, told)
	}
}

func TestAnApprovalNamesTheCallItWaitsFor(t *testing.T) {
	tracker := watch.NewTracker()
	tracker.Take(sessionsMessage("s1", "Fix the build"))
	tracker.Take(eventMessage("s1", protocol.SessionEventToolCall{CallID: "c1", Summary: "npm test"}))
	found, told := tracker.Take(eventMessage("s1", protocol.SessionEventPermissionRequested{CallID: "c1", RequestID: "r1"}))
	expectNotice(t, found, told, watch.Notice{Kind: watch.NeedsYou, SessionID: "s1", Text: "Asks you first: npm test", Title: "Fix the build"})
}

func TestAQuestionSaysTheFirstOne(t *testing.T) {
	tracker := watch.NewTracker()
	found, told := tracker.Take(eventMessage("s1", protocol.SessionEventQuestionAsked{Questions: []protocol.Question{{Question: "Which database?"}}}))
	expectNotice(t, found, told, watch.Notice{Kind: watch.NeedsYou, SessionID: "s1", Text: "Which database?", Title: "A session"})
}

func TestAFinishedTurnSaysTheLastAnswerOnOneShortLine(t *testing.T) {
	tracker := watch.NewTracker()
	tracker.Take(sessionsMessage("s1", "Docs"))
	tracker.Take(eventMessage("s1", protocol.SessionEventAssistantMessage{Text: "Done:\n\nthe README " + strings.Repeat("says more ", 30)}))
	tracker.Take(eventMessage("s1", protocol.SessionEventAssistantMessage{Text: ""}))
	found, told := tracker.Take(eventMessage("s1", protocol.SessionEventTurnFinished{Outcome: protocol.TurnOutcomeDone}))
	if !told || found.Kind != watch.Finished || !strings.HasPrefix(found.Text, "Done: the README says more") || len([]rune(found.Text)) != 160 || !strings.HasSuffix(found.Text, "…") {
		t.Fatalf("found %+v", found)
	}
	found, _ = tracker.Take(eventMessage("s1", protocol.SessionEventTurnFinished{Outcome: protocol.TurnOutcomeDone}))
	if found.Text != "Its turn is over." {
		t.Fatalf("the next turn said the last one's answer: %+v", found)
	}
}

func TestAFailedTurnSaysWhyAndAStoppedOneNothing(t *testing.T) {
	tracker := watch.NewTracker()
	found, told := tracker.Take(eventMessage("s1", protocol.SessionEventTurnFinished{Message: "llm-server did not answer.", Outcome: protocol.TurnOutcomeFailed}))
	expectNotice(t, found, told, watch.Notice{Kind: watch.Failed, SessionID: "s1", Text: "llm-server did not answer.", Title: "A session"})
	if _, told := tracker.Take(eventMessage("s1", protocol.SessionEventTurnFinished{Outcome: protocol.TurnOutcomeStopped})); told {
		t.Fatal("a stopped turn made a notice")
	}
}

func TestTheSocketsAddressFollowsTheDaemons(t *testing.T) {
	for address, expected := range map[string]string{
		"http://gmktec:4500":            "ws://gmktec:4500/api/socket?token=t+1",
		"https://gmktec.tailnet.ts.net": "wss://gmktec.tailnet.ts.net/api/socket?token=t+1",
	} {
		if found, err := watch.SocketURL(address, "t 1"); err != nil || found != expected {
			t.Fatalf("%s: expected %s, found %s (%v)", address, expected, found, err)
		}
	}
}

func TestFollowingADaemonTellsWhenASessionNeedsItsPersonThenFinishes(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{
		{ToolCalls: []fakellama.ToolCall{{Arguments: map[string]any{"command": "echo hello"}, Name: "bash"}}},
		{Text: "It printed hello."},
	}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	notices := make(chan watch.Notice, 4)
	connected := make(chan bool, 4)
	go watch.Follow(ctx, test.Server.URL, Token, watch.Handlers{
		Connected: func(open bool) { connected <- open },
		Notice:    func(notice watch.Notice) { notices <- notice },
	})
	if open := <-connected; !open {
		t.Fatal("the socket did not open")
	}
	session := test.NewSession(sessions.Request{Mode: protocol.PermissionModeAsk})
	if problem := test.App.Sessions.SendPrompt(session.ID, "Say hello", nil); problem != "" {
		t.Fatal(problem)
	}
	asked := next(t, notices)
	if asked.Kind != watch.NeedsYou || asked.SessionID != session.ID || !strings.HasPrefix(asked.Text, "Asks you first: ") {
		t.Fatalf("found %+v", asked)
	}
	request := test.WaitForEvent("permissionRequested", 1).Value.(protocol.SessionEventPermissionRequested)
	if problem := test.App.Sessions.AnswerPermission(session.ID, request.RequestID, protocol.PermissionDecisionAllowOnce); problem != "" {
		t.Fatal(problem)
	}
	if done := next(t, notices); done.Kind != watch.Finished || done.Text != "It printed hello." {
		t.Fatalf("found %+v", done)
	}
}

func next(t *testing.T, notices <-chan watch.Notice) watch.Notice {
	t.Helper()
	select {
	case notice := <-notices:
		return notice
	case <-time.After(noticeWait):
		t.Fatal("no notice came")
		return watch.Notice{}
	}
}

func TestATurnThatEndsWhileSubagentsWorkTellsNothingTheNextOneDoes(t *testing.T) {
	tracker := watch.NewTracker()
	working := protocol.SessionSummary{ID: "s1", Subagents: 2, Title: "Review"}
	tracker.Take(protocol.ServerMessage{Value: protocol.ServerMessageSessions{Sessions: []protocol.SessionSummary{working}}})
	if _, told := tracker.Take(eventMessage("s1", protocol.SessionEventTurnFinished{Outcome: protocol.TurnOutcomeDone})); told {
		t.Fatal("told while its subagents work")
	}
	tracker.Take(sessionsMessage("s1", "Review"))
	if found, told := tracker.Take(eventMessage("s1", protocol.SessionEventTurnFinished{Outcome: protocol.TurnOutcomeDone})); !told || found.Kind != watch.Finished {
		t.Fatalf("found %+v", found)
	}
}
