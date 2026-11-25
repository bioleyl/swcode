package sessions

import (
	"context"

	"swcode/daemon/agent"
	"swcode/daemon/protocol"
)

// peekStopped answers a peek the daemon stopped before it could be asked.
const peekStopped = "The daemon stopped before this peek was answered."

// Peek answers a question beside the session (/peek), to the app that asked only (tell): on the agent's own slot,
// as soon as its request running ends, or at once while it asks nothing. The answer never enters the session.
func (s *Service) Peek(sessionID, peekID, question string, tell func(protocol.ServerMessage)) string {
	s.mu.Lock()
	session, found := s.sessions[sessionID]
	var setup *agent.Setup
	if found && session.turn != nil {
		setup = session.turn.setup
	}
	s.mu.Unlock()
	if !found {
		return unknownSession
	}
	reply := peekOf(sessionID, peekID, tell)
	reply.Question, reply.At = question, protocol.Timestamp(s.deps.Now())
	go s.answerPeek(s.life, session, setup, reply)
	return ""
}

// answerPeek waits for the agent's model, then asks with the turn's setup, or with one made now while no turn runs.
// It asks nothing once the daemon stops.
func (s *Service) answerPeek(ctx context.Context, session *live, setup *agent.Setup, peek peekReply) {
	if setup == nil {
		ready, err := s.prepare(ctx, session)
		if err != nil {
			peek.state(protocol.PeekStateFailed, "The host did not answer: "+err.Error())
			return
		}
		setup = &ready.setup
	}
	if !tryTakeModel(session) {
		peek.state(protocol.PeekStateWaiting, "")
		if !takeModel(ctx, session) {
			peek.state(protocol.PeekStateFailed, peekStopped)
			return
		}
	}
	defer releaseModel(session)
	peek.state(protocol.PeekStateAnswering, "")
	s.mu.Lock()
	history := append([]protocol.SessionEvent{}, session.events...)
	s.mu.Unlock()
	agent.AnswerPeek(ctx, history, *setup, peek.Peek)
}

// tryTakeModel holds the session's model slot when it is free.
func tryTakeModel(session *live) bool {
	select {
	case session.model <- struct{}{}:
		return true
	default:
		return false
	}
}

// takeModel holds the session's model slot, waiting for the agent's request to end; false when ctx ends first.
func takeModel(ctx context.Context, session *live) bool {
	select {
	case session.model <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func releaseModel(session *live) { <-session.model }

// peekReply is a peek, and how its states reach the app that asked.
type peekReply struct {
	agent.Peek
	state func(state protocol.PeekState, message string)
}

func peekOf(sessionID, peekID string, tell func(protocol.ServerMessage)) peekReply {
	state := func(state protocol.PeekState, message string) {
		tell(protocol.ServerMessage{Value: protocol.ServerMessagePeekState{Message: message, PeekID: peekID, SessionID: sessionID, State: state}})
	}
	return peekReply{
		Peek: agent.Peek{
			Done: func(problem string) {
				if problem == "" {
					state(protocol.PeekStateDone, "")
					return
				}
				state(protocol.PeekStateFailed, problem)
			},
			Text: func(text string) {
				tell(protocol.ServerMessage{Value: protocol.ServerMessagePeekDelta{PeekID: peekID, SessionID: sessionID, Text: text}})
			},
		},
		state: state,
	}
}
