// Package watch follows a daemon's sessions through its socket, as the web app does, for the desktop app's
// notifications: a session needs its person, finished its turn, or failed.
package watch

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"

	"swcode/daemon/protocol"
)

// Kind is what a notice tells.
type Kind string

const (
	NeedsYou Kind = "needsYou"
	Finished Kind = "finished"
	Failed   Kind = "failed"
)

const (
	// A notification shows a line or two.
	maxText  = 160
	ellipsis = "…"
	// A session event carries a tool's output, which can be long.
	readLimit  = 64 << 20
	firstRetry = time.Second
	lastRetry  = 30 * time.Second

	untitled   = "A session"
	askedFirst = "Asks you first: "
	turnOver   = "Its turn is over."
	turnFailed = "Its turn failed."

	secureScheme = "https"
	plainSocket  = "ws"
	secureSocket = "wss"
)

// outcomes is what each way a turn ends tells: a turn its person stopped tells nothing.
var outcomes = map[protocol.TurnOutcome]Kind{
	protocol.TurnOutcomeDone:   Finished,
	protocol.TurnOutcomeLimit:  Finished,
	protocol.TurnOutcomeFailed: Failed,
}

// Notice is what a notification says, and the session it opens.
type Notice struct {
	Kind      Kind
	SessionID string
	Title     string
	Text      string
}

// Tracker turns what a daemon's socket says into notices. It keeps what words them: the sessions' titles, their
// last answers, and their calls, which an approval names.
type Tracker struct {
	titles  map[string]string
	replies map[string]string
	calls   map[string]map[string]string
}

func NewTracker() *Tracker {
	return &Tracker{titles: map[string]string{}, replies: map[string]string{}, calls: map[string]map[string]string{}}
}

// Take reads one message of the socket: a notice, when it is one.
func (t *Tracker) Take(message protocol.ServerMessage) (Notice, bool) {
	switch value := message.Value.(type) {
	case protocol.ServerMessageSessions:
		for _, session := range value.Sessions {
			t.titles[session.ID] = session.Title
		}
	case protocol.ServerMessageSessionEvent:
		return t.event(value.SessionID, value.Event)
	}
	return Notice{}, false
}

func (t *Tracker) event(sessionID string, event protocol.SessionEvent) (Notice, bool) {
	notice := Notice{SessionID: sessionID, Title: t.titleOf(sessionID)}
	switch value := event.Value.(type) {
	case protocol.SessionEventToolCall:
		t.callsOf(sessionID)[value.CallID] = value.Summary
	case protocol.SessionEventAssistantMessage:
		t.keepReply(sessionID, value.Text)
	case protocol.SessionEventPermissionRequested:
		notice.Kind, notice.Text = NeedsYou, clipped(askedFirst+t.callsOf(sessionID)[value.CallID])
		return notice, true
	case protocol.SessionEventQuestionAsked:
		notice.Kind, notice.Text = NeedsYou, clipped(firstQuestion(value.Questions))
		return notice, true
	case protocol.SessionEventTurnFinished:
		return t.finished(notice, value)
	}
	return Notice{}, false
}

func (t *Tracker) titleOf(sessionID string) string {
	if title := t.titles[sessionID]; title != "" {
		return title
	}
	return untitled
}

func (t *Tracker) callsOf(sessionID string) map[string]string {
	if t.calls[sessionID] == nil {
		t.calls[sessionID] = map[string]string{}
	}
	return t.calls[sessionID]
}

// keepReply keeps a session's last words to its person: what a finished turn's notice says.
func (t *Tracker) keepReply(sessionID, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	t.replies[sessionID] = text
}

// finished is the notice a turn's end makes; what the tracker kept of that turn goes with it.
func (t *Tracker) finished(notice Notice, turn protocol.SessionEventTurnFinished) (Notice, bool) {
	reply := t.replies[notice.SessionID]
	delete(t.replies, notice.SessionID)
	delete(t.calls, notice.SessionID)
	kind, told := outcomes[turn.Outcome]
	if !told {
		return Notice{}, false
	}
	notice.Kind, notice.Text = kind, clipped(endingText(kind, reply, turn.Message))
	return notice, true
}

func endingText(kind Kind, reply, problem string) string {
	if kind == Failed {
		return firstOf(problem, turnFailed)
	}
	return firstOf(reply, turnOver)
}

func firstOf(text, otherwise string) string {
	if strings.TrimSpace(text) == "" {
		return otherwise
	}
	return text
}

func firstQuestion(questions []protocol.Question) string {
	if len(questions) == 0 {
		return ""
	}
	return questions[0].Question
}

// clipped is a text on one line, cut to what a notification shows.
func clipped(text string) string {
	line := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(line) <= maxText {
		return line
	}
	return string([]rune(line)[:maxText-1]) + ellipsis
}

// SocketURL is the socket at a daemon's address: ws, or wss behind https, with the token.
func SocketURL(address, token string) (string, error) {
	target, err := url.Parse(address)
	if err != nil {
		return "", err
	}
	scheme := plainSocket
	if target.Scheme == secureScheme {
		scheme = secureSocket
	}
	target.Scheme, target.Path = scheme, protocol.EndpointSocket
	target.RawQuery = url.Values{protocol.TokenParam: {token}}.Encode()
	return target.String(), nil
}

// Handlers are told what following a daemon finds: each notice, and whether its socket is open.
type Handlers struct {
	Notice    func(Notice)
	Connected func(connected bool)
}

// Follow reads the socket of the daemon at address and tells each notice, connecting again when the connection
// drops, until ctx ends.
func Follow(ctx context.Context, address, token string, handlers Handlers) {
	wait := firstRetry
	for ctx.Err() == nil {
		if listen(ctx, address, token, handlers) {
			wait = firstRetry
			handlers.Connected(false)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, lastRetry)
	}
}

// listen reads the socket until it closes: false when it could not connect. The daemon speaks first, once it
// follows its sessions for this socket: the socket counts as open from then on.
func listen(ctx context.Context, address, token string, handlers Handlers) bool {
	target, err := SocketURL(address, token)
	if err != nil {
		return false
	}
	connection, _, err := websocket.Dial(ctx, target, nil)
	if err != nil {
		return false
	}
	defer func() { _ = connection.CloseNow() }()
	connection.SetReadLimit(readLimit)
	tracker := NewTracker()
	_, data, err := connection.Read(ctx)
	if err != nil {
		return false
	}
	handlers.Connected(true)
	for ; err == nil; _, data, err = connection.Read(ctx) {
		var message protocol.ServerMessage
		if json.Unmarshal(data, &message) != nil {
			continue
		}
		if notice, ok := tracker.Take(message); ok {
			handlers.Notice(notice)
		}
	}
	return true
}
