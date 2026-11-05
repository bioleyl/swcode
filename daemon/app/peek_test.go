package app_test

import (
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	. "swcode/daemon/testapp"
)

const peekQuestion = "PEEK: what did you change?"

// peekAnswer keeps what a peek tells the app that asked.
type peekAnswer struct {
	mu     sync.Mutex
	states []protocol.PeekState
	text   string
}

func (p *peekAnswer) tell(message protocol.ServerMessage) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch value := message.Value.(type) {
	case protocol.ServerMessagePeekState:
		p.states = append(p.states, value.State)
	case protocol.ServerMessagePeekDelta:
		p.text += value.Text
	}
}

func (p *peekAnswer) ended() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Contains(p.states, protocol.PeekStateDone) || slices.Contains(p.states, protocol.PeekStateFailed)
}

// peekOptions is a turn's replies, then the peek's.
func peekOptions(turn fakellama.Reply) Options {
	return Options{Replies: []fakellama.Reply{turn, text("Nothing yet.")}, Slots: 2}
}

func TestAPeekGoesOnTheAgentsSlotWithItsPromptAndNeverEntersTheSession(t *testing.T) {
	test := Start(t, peekOptions(text("Done.")))
	sessionID := prompt(t, test, "Hi")
	test.UntilIdle(sessionID)
	before := len(test.Events(sessionID))
	answer := &peekAnswer{}
	if problem := test.App.Sessions.Peek(sessionID, "p1", peekQuestion, answer.tell); problem != "" {
		t.Fatal(problem)
	}
	test.Until("the peek ends", answer.ended)
	if !reflect.DeepEqual(answer.states, []protocol.PeekState{protocol.PeekStateAnswering, protocol.PeekStateDone}) || answer.text != "Nothing yet." {
		t.Fatalf("states %v, text %q", answer.states, answer.text)
	}
	turn, peek := test.Requests()[0], test.Requests()[1]
	if peek["id_slot"] != turn["id_slot"] || !reflect.DeepEqual(peek["tools"], turn["tools"]) {
		t.Fatalf("slot %v against %v, or other tools", peek["id_slot"], turn["id_slot"])
	}
	asked := peek["messages"].([]any)
	if !reflect.DeepEqual(asked[:2], turn["messages"].([]any)) || !strings.Contains(RequestText(peek), peekQuestion) {
		t.Fatalf("messages %v", asked)
	}
	if got := len(test.Events(sessionID)); got != before {
		t.Fatalf("%d events, %d before", got, before)
	}
}

func TestAPeekWaitsForTheAgentsRequestRunningThenTheTurnGoesOn(t *testing.T) {
	test := Start(t, peekOptions(fakellama.Reply{Delay: 300 * time.Millisecond, Text: "Done."}))
	sessionID := prompt(t, test, "Hi")
	test.Until("the agent asks", func() bool { return len(test.Requests()) == 1 })
	answer := &peekAnswer{}
	test.App.Sessions.Peek(sessionID, "p1", peekQuestion, answer.tell)
	test.Until("the peek ends", answer.ended)
	test.UntilIdle(sessionID)
	want := []protocol.PeekState{protocol.PeekStateWaiting, protocol.PeekStateAnswering, protocol.PeekStateDone}
	if !reflect.DeepEqual(answer.states, want) {
		t.Fatalf("states %v", answer.states)
	}
	Expect(t, last(test.Events(sessionID)), `{"outcome":"done"}`)
	if strings.Contains(RequestText(test.Requests()[0]), peekQuestion) {
		t.Fatal("the agent was asked the peek")
	}
}
