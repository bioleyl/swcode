// Package headless runs one turn of a session without the app, as a client of the daemon's socket: swcode run. It
// makes the session, follows its events until the turn is over, and prints its answer, or its events as JSON lines
// for a program to read.
package headless

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"

	"swcode/daemon/protocol"
)

const (
	// Exit codes: the turn was done, or it was not.
	exitDone   = 0
	exitFailed = 1
	// A session event carries a tool's output, which can be long.
	readLimit    = 64 << 20
	secureScheme = "https"
	plainSocket  = "ws"
	secureSocket = "wss"
	// maxEarlyEvents is how many events are kept while the session's id is not known yet: the daemon starts the turn
	// before it answers which session it made, and other sessions' events come on the same socket.
	maxEarlyEvents = 10_000
	// stopGrace is how long a run that ran out of time has to tell the daemon to stop its turn.
	stopGrace = 10 * time.Second
	// nobodyHere is what the agent's questions are answered: a run has nobody to ask.
	nobodyHere   = "Nobody is here to answer: decide by yourself, and say what you chose."
	toolLine     = "> %s %s\n"
	finishedKind = "runFinished"
	noConnection = "swcode's daemon did not answer: %v\n"
	timedOut     = "The run took longer than %s: its turn was stopped.\n"
)

// Options is one run: the daemon to ask, the folder and the prompt, and where it prints.
type Options struct {
	// Address is the daemon's, such as http://127.0.0.1:4500, and Token its access token.
	Address string
	Token   string
	Cwd     string
	Mode    protocol.PermissionMode
	// JSON prints each event of the session as the protocol encodes it, a line each, then a runFinished line.
	JSON    bool
	Timeout time.Duration
	Prompt  string
	Out     io.Writer
	Err     io.Writer
}

// finished is the last line of a run's JSON: how the turn ended and what it took.
type finished struct {
	Kind             string  `json:"kind"`
	Outcome          string  `json:"outcome"`
	SessionID        string  `json:"sessionId"`
	Steps            int     `json:"steps"`
	ToolCalls        int     `json:"toolCalls"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	Seconds          float64 `json:"seconds"`
}

// run is a run under way: what it counted, and what it knows of its session.
type run struct {
	options    Options
	connection *websocket.Conn
	sessionID  string
	// early are the events that came before the session's id was known.
	early []protocol.ServerMessageSessionEvent
	// answer is the agent's last words; problem, why its turn failed.
	answer  string
	problem string
	outcome protocol.TurnOutcome
	// resting: the session has no turn and no subagent at work, as the daemon last listed it.
	resting bool
	summary finished
}

// socketURL is the socket at the daemon's address: ws, or wss behind https, with the token.
func socketURL(address, token string) (string, error) {
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

// Run runs one turn of a new session on the prompt and waits for it: 0 when the turn was done, 1 otherwise.
func Run(ctx context.Context, options Options) int {
	started := time.Now()
	target, err := socketURL(options.Address, options.Token)
	if err != nil {
		fmt.Fprintf(options.Err, noConnection, err)
		return exitFailed
	}
	limited, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	connection, _, err := websocket.Dial(limited, target, nil)
	if err != nil {
		fmt.Fprintf(options.Err, noConnection, err)
		return exitFailed
	}
	defer func() { _ = connection.CloseNow() }()
	connection.SetReadLimit(readLimit)
	current := &run{connection: connection, options: options, summary: finished{Kind: finishedKind}}
	if !current.send(limited, protocol.ClientMessageCreateSession{Mode: options.Mode, Prompt: options.Prompt, Workspace: options.Cwd}) {
		return exitFailed
	}
	if err := current.follow(limited); err != nil {
		current.ranOut(ctx, limited, err)
	}
	current.summary.Outcome, current.summary.SessionID = string(current.outcome), current.sessionID
	current.summary.Seconds = time.Since(started).Seconds()
	return current.report()
}

// send sends the daemon a message; false when the connection is gone.
func (r *run) send(ctx context.Context, message any) bool {
	data, err := json.Marshal(message)
	if err == nil {
		err = r.connection.Write(ctx, websocket.MessageText, data)
	}
	if err != nil {
		fmt.Fprintf(r.options.Err, noConnection, err)
		return false
	}
	return true
}

// follow reads the socket until the session's turn is over and the session rests, or the daemon refuses it.
func (r *run) follow(ctx context.Context) error {
	for !r.over() {
		_, data, err := r.connection.Read(ctx)
		if err != nil {
			return err
		}
		var message protocol.ServerMessage
		if json.Unmarshal(data, &message) != nil {
			continue
		}
		r.take(ctx, message)
	}
	return nil
}

// over tells whether the run is: its turn ended, and no subagent is left whose report would start another.
func (r *run) over() bool { return r.outcome != "" && r.resting }

// take reads one message of the socket.
func (r *run) take(ctx context.Context, message protocol.ServerMessage) {
	switch value := message.Value.(type) {
	case protocol.ServerMessageProblem:
		// Before any session was made, the daemon refused this one: a folder that is not there, say.
		if r.sessionID == "" {
			r.outcome, r.problem, r.resting = protocol.TurnOutcomeFailed, value.Message, true
		}
	case protocol.ServerMessageSessionCreated:
		r.sessionID = value.Session.ID
		r.rest(value.Session)
		for _, early := range r.early {
			r.event(ctx, early)
		}
		r.early = nil
	case protocol.ServerMessageSessions:
		for _, session := range value.Sessions {
			if session.ID == r.sessionID {
				r.rest(session)
			}
		}
	case protocol.ServerMessageSessionEvent:
		r.event(ctx, value)
	}
}

func (r *run) rest(session protocol.SessionSummary) {
	r.resting = session.Status == protocol.SessionStatusIdle && session.Subagents == 0
}

// event takes one event: the run's own session's, once its id is known; kept until then.
func (r *run) event(ctx context.Context, message protocol.ServerMessageSessionEvent) {
	if r.sessionID == "" {
		if len(r.early) < maxEarlyEvents {
			r.early = append(r.early, message)
		}
		return
	}
	if message.SessionID != r.sessionID {
		return
	}
	if r.options.JSON {
		if line, err := json.Marshal(message.Event); err == nil {
			fmt.Fprintln(r.options.Out, string(line))
		}
	}
	switch value := message.Event.Value.(type) {
	case protocol.SessionEventTurnStarted:
		// A subagent's report woke the agent: the run goes on with that turn.
		r.outcome = ""
	case protocol.SessionEventToolCall:
		r.summary.ToolCalls++
		if !r.options.JSON {
			fmt.Fprintf(r.options.Err, toolLine, value.Tool, value.Summary)
		}
	case protocol.SessionEventAssistantMessage:
		r.summary.Steps++
		if strings.TrimSpace(value.Text) != "" {
			r.answer = value.Text
		}
		if value.Usage != nil {
			r.summary.PromptTokens = value.Usage.PromptTokens
			r.summary.CompletionTokens += value.Usage.CompletionTokens
		}
	case protocol.SessionEventPermissionRequested:
		// Nobody is there to approve it.
		r.send(ctx, protocol.ClientMessageAnswerPermission{Decision: protocol.PermissionDecisionDeny, RequestID: value.RequestID, SessionID: r.sessionID})
	case protocol.SessionEventQuestionAsked:
		answers := make([]string, len(value.Questions))
		for index := range answers {
			answers[index] = nobodyHere
		}
		r.send(ctx, protocol.ClientMessageAnswerQuestion{Answers: answers, RequestID: value.RequestID, SessionID: r.sessionID})
	case protocol.SessionEventPasswordRequested:
		// No password comes: the command's input closes.
		r.send(ctx, protocol.ClientMessageAnswerPassword{RequestID: value.RequestID, SessionID: r.sessionID})
	case protocol.SessionEventTerminalWaiting:
		// A command waits for someone to type in a terminal of the app: nobody will.
		r.send(ctx, protocol.ClientMessageStopTurn{SessionID: r.sessionID})
	case protocol.SessionEventTurnFinished:
		r.outcome, r.problem = value.Outcome, value.Message
	}
}

// ranOut ends a run whose connection ended or whose time is up: its turn is stopped, so the session does not work
// on with nobody waiting for it.
func (r *run) ranOut(ctx, limited context.Context, err error) {
	if limited.Err() == nil {
		fmt.Fprintf(r.options.Err, noConnection, err)
		r.outcome = protocol.TurnOutcomeFailed
		return
	}
	fmt.Fprintf(r.options.Err, timedOut, r.options.Timeout)
	r.outcome = protocol.TurnOutcomeStopped
	if r.sessionID == "" {
		return
	}
	// The read that ran out of time closed the connection: the turn is stopped on a new one.
	stopping, cancel := context.WithTimeout(context.WithoutCancel(ctx), stopGrace)
	defer cancel()
	target, _ := socketURL(r.options.Address, r.options.Token)
	connection, _, dialErr := websocket.Dial(stopping, target, nil)
	if dialErr != nil {
		return
	}
	defer func() { _ = connection.CloseNow() }()
	data, _ := json.Marshal(protocol.ClientMessageStopTurn{SessionID: r.sessionID})
	_ = connection.Write(stopping, websocket.MessageText, data)
	_ = connection.Close(websocket.StatusNormalClosure, "")
}

// report prints how the run went: its JSON line, or the agent's answer, and why the turn failed.
func (r *run) report() int {
	done := r.outcome == protocol.TurnOutcomeDone
	if r.options.JSON {
		if line, err := json.Marshal(r.summary); err == nil {
			fmt.Fprintln(r.options.Out, string(line))
		}
	} else if r.answer != "" {
		fmt.Fprintln(r.options.Out, r.answer)
	}
	if r.problem != "" && !done {
		fmt.Fprintln(r.options.Err, r.problem)
	}
	if done {
		return exitDone
	}
	return exitFailed
}
