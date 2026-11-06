package app

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/coder/websocket"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
)

const (
	unknownSession = "There is no such session."
	notUnderstood  = "The daemon did not understand that message."
	maxMessage     = 64 << 20
)

// outbox is a socket's messages waiting to leave, in order, however many: the sessions tell it what happens with
// their lock held, so it never waits.
type outbox struct {
	mu      sync.Mutex
	waiting [][]byte
	ready   chan struct{}
}

func (o *outbox) push(message protocol.ServerMessage) {
	data, err := json.Marshal(message)
	if err != nil {
		return
	}
	o.mu.Lock()
	o.waiting = append(o.waiting, data)
	o.mu.Unlock()
	select {
	case o.ready <- struct{}{}:
	default:
	}
}

func (o *outbox) take() [][]byte {
	o.mu.Lock()
	defer o.mu.Unlock()
	taken := o.waiting
	o.waiting = nil
	return taken
}

func problem(text string) protocol.ServerMessage {
	return protocol.ServerMessage{Value: protocol.ServerMessageProblem{Message: text}}
}

// handlers has a method per message the app sends: a new kind does not compile until it has one.
type handlers struct {
	app   *App
	ctx   context.Context
	reply func(protocol.ServerMessage)
}

func (h handlers) problem(text string) struct{} {
	if text != "" {
		h.reply(problem(text))
	}
	return struct{}{}
}

func (h handlers) AnswerPermission(message protocol.ClientMessageAnswerPermission) struct{} {
	return h.problem(h.app.Sessions.AnswerPermission(message.SessionID, message.RequestID, message.Decision))
}

func (h handlers) AnswerQuestion(message protocol.ClientMessageAnswerQuestion) struct{} {
	return h.problem(h.app.Sessions.AnswerQuestion(message.SessionID, message.RequestID, message.Answers))
}

func (h handlers) ClearQueue(message protocol.ClientMessageClearQueue) struct{} {
	return h.problem(h.app.Sessions.ClearQueue(message.SessionID))
}

func (h handlers) CreateProject(message protocol.ClientMessageCreateProject) struct{} {
	go func() {
		project, refused := h.app.Projects.Create(h.ctx, message.Folder, message.Host, message.NewFolder)
		if refused != "" {
			h.reply(protocol.ServerMessage{Value: protocol.ServerMessageProjectRefused{Message: refused}})
			return
		}
		h.reply(protocol.ServerMessage{Value: protocol.ServerMessageProjectCreated{Project: project}})
	}()
	return struct{}{}
}

func (h handlers) CreateSession(message protocol.ClientMessageCreateSession) struct{} {
	go func() {
		session, refused := h.app.Sessions.Create(h.ctx, sessions.Request{
			Concise: message.Concise, Host: message.Host, Kind: message.Kind, Mode: message.Mode, Pictures: message.Pictures, Preset: message.Preset,
			Prompt: message.Prompt, Thinking: message.Thinking, Workspace: message.Workspace,
		})
		if refused != "" {
			h.reply(problem(refused))
			return
		}
		h.reply(protocol.ServerMessage{Value: protocol.ServerMessageSessionCreated{Session: session}})
	}()
	return struct{}{}
}

func (h handlers) DeleteSession(message protocol.ClientMessageDeleteSession) struct{} {
	return h.problem(h.app.Sessions.Remove(message.SessionID))
}

func folderMessage(listing hosts.FolderListing) protocol.ServerMessage {
	if !listing.OK {
		return protocol.ServerMessage{Value: protocol.ServerMessageFolders{Folders: []string{}, Path: listing.Path, Problem: listing.Problem}}
	}
	return protocol.ServerMessage{Value: protocol.ServerMessageFolders{Folders: listing.Folders, Parent: listing.Parent, Path: listing.Path}}
}

func (h handlers) ListFolders(message protocol.ClientMessageListFolders) struct{} {
	go func() { h.reply(folderMessage(h.app.Projects.Folders(h.ctx, message.Path, message.Host))) }()
	return struct{}{}
}

func (h handlers) ListHosts(protocol.ClientMessageListHosts) struct{} {
	go func() {
		h.reply(protocol.ServerMessage{Value: protocol.ServerMessageHosts{Hosts: h.app.Projects.HostOptions()}})
	}()
	return struct{}{}
}

func (h handlers) OpenSession(message protocol.ClientMessageOpenSession) struct{} {
	history, found := h.app.Sessions.History(message.SessionID)
	if !found {
		return h.problem(unknownSession)
	}
	h.reply(history)
	return struct{}{}
}

func (h handlers) RemoveProject(message protocol.ClientMessageRemoveProject) struct{} {
	return h.problem(h.app.Projects.Remove(message.ProjectID))
}

func (h handlers) RenameSession(message protocol.ClientMessageRenameSession) struct{} {
	return h.problem(h.app.Sessions.Rename(message.SessionID, message.Title))
}

func (h handlers) SendPrompt(message protocol.ClientMessageSendPrompt) struct{} {
	return h.problem(h.app.Sessions.SendPrompt(message.SessionID, message.Text, message.Pictures))
}

func (h handlers) SendQueueNow(message protocol.ClientMessageSendQueueNow) struct{} {
	return h.problem(h.app.Sessions.SendQueueNow(message.SessionID))
}

func (h handlers) SetMode(message protocol.ClientMessageSetMode) struct{} {
	return h.problem(h.app.Sessions.SetMode(message.SessionID, message.Mode))
}

func (h handlers) SetConcise(message protocol.ClientMessageSetConcise) struct{} {
	return h.problem(h.app.Sessions.SetConcise(message.SessionID, message.Concise))
}

func (h handlers) SetPreset(message protocol.ClientMessageSetPreset) struct{} {
	return h.problem(h.app.Sessions.SetPreset(message.SessionID, message.Preset))
}

func (h handlers) SetThinking(message protocol.ClientMessageSetThinking) struct{} {
	return h.problem(h.app.Sessions.SetThinking(message.SessionID, message.Thinking))
}

func (h handlers) StopJob(message protocol.ClientMessageStopJob) struct{} {
	return h.problem(h.app.Sessions.StopJob(message.SessionID, message.JobID))
}

func (h handlers) Peek(message protocol.ClientMessagePeek) struct{} {
	return h.problem(h.app.Sessions.Peek(message.SessionID, message.PeekID, message.Question, h.reply))
}

func (h handlers) SearchSessions(message protocol.ClientMessageSearchSessions) struct{} {
	go func() {
		h.reply(protocol.ServerMessage{Value: protocol.ServerMessageSessionMatches{Matches: h.app.Sessions.Search(message.Query), Query: message.Query}})
	}()
	return struct{}{}
}

func (h handlers) StopTurn(message protocol.ClientMessageStopTurn) struct{} {
	return h.problem(h.app.Sessions.Stop(message.SessionID))
}

// socket is the socket the app talks to the daemon through: commands one way, everything that happens the other.
func (a *App) socket(response http.ResponseWriter, request *http.Request) {
	connection, err := websocket.Accept(response, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	connection.SetReadLimit(maxMessage)
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	box := &outbox{ready: make(chan struct{}, 1)}
	unsubscribe := []func(){
		a.Sessions.Subscribe(func(message protocol.ServerMessage) { box.push(message) }),
		a.Projects.Subscribe(func(message protocol.ServerMessage) { box.push(message) }),
		a.Models.Subscribe(func(message protocol.ServerMessage) { box.push(message) }),
	}
	defer func() {
		for _, each := range unsubscribe {
			each()
		}
	}()
	box.push(protocol.ServerMessage{Value: protocol.ServerMessageProjects{Projects: a.Projects.List()}})
	box.push(protocol.ServerMessage{Value: protocol.ServerMessagePresets{Choices: a.Models.Choices()}})
	box.push(protocol.ServerMessage{Value: protocol.ServerMessageSessions{Sessions: a.Sessions.List()}})
	go a.send(ctx, cancel, connection, box)
	reply := handlers{app: a, ctx: context.WithoutCancel(ctx), reply: box.push}
	for {
		_, data, err := connection.Read(ctx)
		if err != nil {
			_ = connection.CloseNow()
			return
		}
		var message protocol.ClientMessage
		if json.Unmarshal(data, &message) != nil {
			box.push(problem(notUnderstood))
			continue
		}
		protocol.MapClientMessage[struct{}](message, reply)
	}
}

// send writes a socket's messages as they come, until the socket closes.
func (a *App) send(ctx context.Context, cancel context.CancelFunc, connection *websocket.Conn, box *outbox) {
	defer cancel()
	for {
		for _, data := range box.take() {
			if connection.Write(ctx, websocket.MessageText, data) != nil {
				return
			}
		}
		select {
		case <-box.ready:
		case <-ctx.Done():
			return
		}
	}
}
