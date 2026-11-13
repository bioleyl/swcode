package app

import (
	"encoding/json"
	"sync"

	"swcode/daemon/protocol"
)

// everySession is what a socket follows until it says which session it shows: every session's events. The desktop
// app's watch of another computer says nothing, and tells of all of them.
const everySession = "*"

// hub sends what the services say to the open sockets. Each message is put in JSON once, however many sockets
// take it, and a session's own stream goes only where it is wanted: its answer as it is written to the sockets
// that have that session open, its events to those and to the sockets that follow every session. A phone left on
// one session is not sent every token of the others.
type hub struct {
	mu      sync.Mutex
	next    int
	sockets map[int]*listening
}

// listening is a socket as the hub knows it: where its messages wait, and the session it has open: none, one, or
// every session's events.
type listening struct {
	box     *outbox
	session string
}

func newHub() *hub {
	return &hub{sockets: map[int]*listening{}}
}

// join adds a socket: its number, to say which session it opens, and what takes it off again.
func (h *hub) join(box *outbox) (int, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := h.next
	h.next++
	h.sockets[id] = &listening{box: box, session: everySession}
	return id, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.sockets, id)
	}
}

// open notes the session a socket has open now: its stream goes there from now on, the one it had before no more.
func (h *hub) open(id int, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if socket, joined := h.sockets[id]; joined {
		socket.session = sessionID
	}
}

// close notes that a socket shows that session no more, or no session at all when none is named: a page that
// already opened another keeps the one it has.
func (h *hub) close(id int, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	socket, joined := h.sockets[id]
	if joined && (sessionID == "" || socket.session == sessionID || socket.session == everySession) {
		socket.session = ""
	}
}

// stream is which part of a session's stream a message is: none for a message every socket takes.
type stream int

const (
	noStream stream = iota
	// events are what a session recorded: they also go to the sockets that follow every session.
	events
	// writing is an answer as it is written, token by token: only for the sockets that show the session.
	writing
)

// streamOf is the session a message is the stream of, and which part of it.
func streamOf(message protocol.ServerMessage) (string, stream) {
	switch value := message.Value.(type) {
	case protocol.ServerMessageSessionEvent:
		return value.SessionID, events
	case protocol.ServerMessageAssistantDelta:
		return value.SessionID, writing
	case protocol.ServerMessageReasoningDelta:
		return value.SessionID, writing
	case protocol.ServerMessageToolCallWriting:
		return value.SessionID, writing
	}
	return "", noStream
}

// takes tells whether a socket is sent a message of a session's stream.
func (l *listening) takes(sessionID string, part stream) bool {
	return part == noStream || l.session == sessionID || (part == events && l.session == everySession)
}

// eventForPage is an event as a page takes it: without what only the model reads back. A tool's output is shown
// from its display, a reply's calls from their own events, and the Responses API's items by nobody: together
// they were more than half of what a session's history weighed on its way to a phone.
func eventForPage(event protocol.SessionEvent) protocol.SessionEvent {
	switch value := event.Value.(type) {
	case protocol.SessionEventToolFinished:
		value.Output = ""
		return protocol.SessionEvent{Value: value}
	case protocol.SessionEventAssistantMessage:
		value.ResponseItems, value.ToolCalls = "", []protocol.ToolCall{}
		return protocol.SessionEvent{Value: value}
	}
	return event
}

// forPage is a message as a page takes it: its events without what only the model reads back.
func forPage(message protocol.ServerMessage) protocol.ServerMessage {
	switch value := message.Value.(type) {
	case protocol.ServerMessageSessionEvent:
		value.Event = eventForPage(value.Event)
		return protocol.ServerMessage{Value: value}
	case protocol.ServerMessageSessionHistory:
		events := make([]protocol.SessionEvent, len(value.Events))
		for index, event := range value.Events {
			events[index] = eventForPage(event)
		}
		value.Events = events
		return protocol.ServerMessage{Value: value}
	}
	return message
}

// broadcast sends a message to the sockets that take it. The services call it with their lock held: it never
// waits for a socket.
func (h *hub) broadcast(message protocol.ServerMessage) {
	data, err := json.Marshal(forPage(message))
	if err != nil {
		return
	}
	sessionID, part := streamOf(message)
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, socket := range h.sockets {
		if socket.takes(sessionID, part) {
			socket.box.pushData(data)
		}
	}
}
