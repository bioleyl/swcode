package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	"swcode/daemon/terminals"
)

const (
	unknownSession  = "There is no such session."
	unknownTerminal = "There is no such terminal."
	notUnderstood   = "The daemon did not understand that message."
	maxMessage      = 64 << 20
	// maxWaiting is how much may wait for a socket that takes nothing: past it the socket is closed, and its page
	// asks for what it missed once it is back.
	maxWaiting = 32 << 20
	// writeLimit is how long a socket has to take a message.
	writeLimit = 30 * time.Second
	// pingEvery is how often a socket is asked whether it is still there, and pingLimit how long it has to say so:
	// a phone that left the network without a word would hold its socket, and what waits for it, for hours.
	pingEvery = 30 * time.Second
	pingLimit = 10 * time.Second
)

// outbox is a socket's messages waiting to leave, in order: the sessions tell it what happens with their lock
// held, so it never waits. What waits is bounded: full is called once when it overflows, to close the socket.
type outbox struct {
	mu      sync.Mutex
	waiting [][]byte
	bytes   int
	ready   chan struct{}
	full    func()
}

// push sends the socket an answer of its own, as a page takes it.
func (o *outbox) push(message protocol.ServerMessage) {
	data, err := json.Marshal(forPage(message))
	if err != nil {
		return
	}
	o.pushData(data)
}

// pushData adds a message already in JSON, unless too much waits already: the socket is then closed.
func (o *outbox) pushData(data []byte) {
	o.mu.Lock()
	if o.bytes+len(data) > maxWaiting {
		o.mu.Unlock()
		o.full()
		return
	}
	o.waiting = append(o.waiting, data)
	o.bytes += len(data)
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
	o.waiting, o.bytes = nil, 0
	return taken
}

func problem(text string) protocol.ServerMessage {
	return protocol.ServerMessage{Value: protocol.ServerMessageProblem{Message: text}}
}

// terminalWatchers is the terminals one socket hears from, each watched until the person goes back to the
// conversation, closes it, or the socket ends.
type terminalWatchers struct {
	mu     sync.Mutex
	off    map[string]func()
	closed bool
}

func newTerminalWatchers() *terminalWatchers {
	return &terminalWatchers{off: map[string]func(){}}
}

// watch makes the socket hear a terminal's output from now on: the output it had before came with its attach.
func (w *terminalWatchers) watch(service *terminals.Service, id string, deliver func([]byte)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	if _, watching := w.off[id]; watching {
		return
	}
	w.off[id] = service.Watch(id, deliver)
}

func (w *terminalWatchers) drop(id string) {
	w.mu.Lock()
	off, watching := w.off[id]
	delete(w.off, id)
	w.mu.Unlock()
	if watching {
		off()
	}
}

func (w *terminalWatchers) close() {
	w.mu.Lock()
	off := w.off
	w.off, w.closed = map[string]func(){}, true
	w.mu.Unlock()
	for _, each := range off {
		each()
	}
}

// handlers has a method per message the app sends: a new kind does not compile until it has one.
type handlers struct {
	app      *App
	ctx      context.Context
	reply    func(protocol.ServerMessage)
	watchers *terminalWatchers
	// socket is this socket's number in the hub, which sends it the stream of the session it opens.
	socket int
}

func (h handlers) problem(text string) struct{} {
	if text != "" {
		h.reply(problem(text))
	}
	return struct{}{}
}

func (h handlers) AnswerPermission(message protocol.ClientMessageAnswerPermission) struct{} {
	return h.problem(h.app.Sessions.Answer(message.SessionID, message.RequestID, sessions.Permission(message.Decision)))
}

func (h handlers) AnswerQuestion(message protocol.ClientMessageAnswerQuestion) struct{} {
	return h.problem(h.app.Sessions.Answer(message.SessionID, message.RequestID, sessions.Answers(message.Answers)))
}

func (h handlers) AnswerPassword(message protocol.ClientMessageAnswerPassword) struct{} {
	return h.problem(h.app.Sessions.Answer(message.SessionID, message.RequestID, sessions.Secret(message.Secret)))
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
			Concise: message.Concise, Host: message.Host, Kind: message.Kind, Mode: message.Mode, Pictures: message.Pictures, PinnedTodos: message.PinnedTodos, Preset: message.Preset,
			Prompt: message.Prompt, Thinking: message.Thinking, Workspace: message.Workspace, Worktree: message.Worktree,
		})
		if refused != "" {
			h.reply(problem(refused))
			return
		}
		h.linkPinnedTodos(session, message.PinnedTodoIds)
		h.reply(protocol.ServerMessage{Value: protocol.ServerMessageSessionCreated{Session: session}})
	}()
	return struct{}{}
}

// linkPinnedTodos points the project's todos the session started on at it, for the project's page to link back.
func (h handlers) linkPinnedTodos(session protocol.SessionSummary, todoIDs []string) {
	if len(todoIDs) == 0 {
		return
	}
	project, ok := h.app.Projects.Find(projectFolderOf(session), session.Host)
	if !ok {
		return
	}
	h.problem(h.app.Projects.LinkTodos(project.ID, todoIDs, session.ID))
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

// OpenSession answers a page with the end of a session's conversation, or, to a page back online that holds its
// first events, with what came after them.
func (h handlers) OpenSession(message protocol.ClientMessageOpenSession) struct{} {
	h.app.hub.open(h.socket, message.SessionID)
	if message.Since > 0 {
		if since, ok := h.app.Sessions.Resume(message.SessionID, message.Since); ok {
			h.reply(since)
			return struct{}{}
		}
	}
	history, found := h.app.Sessions.History(message.SessionID, message.Events)
	if !found {
		return h.problem(unknownSession)
	}
	h.reply(history)
	return struct{}{}
}

// OlderEvents answers a page that scrolled to the top of what it shows with the events before them.
func (h handlers) OlderEvents(message protocol.ClientMessageOlderEvents) struct{} {
	history, found := h.app.Sessions.OlderEvents(message.SessionID, message.KeptBack, message.Events)
	if !found {
		return h.problem(unknownSession)
	}
	h.reply(history)
	return struct{}{}
}

func (h handlers) RemoveProject(message protocol.ClientMessageRemoveProject) struct{} {
	return h.problem(h.app.Projects.Remove(message.ProjectID))
}

func (h handlers) SetProjectDefaults(message protocol.ClientMessageSetProjectDefaults) struct{} {
	return h.problem(h.app.Projects.SetDefaults(message.ProjectID, message.Defaults))
}

func (h handlers) RemoveProjectAllowed(message protocol.ClientMessageRemoveProjectAllowed) struct{} {
	return h.problem(h.app.Projects.RemoveAllowed(message.ProjectID, message.Rule, message.Name))
}

func (h handlers) SetProjectTrust(message protocol.ClientMessageSetProjectTrust) struct{} {
	return h.problem(h.app.Projects.SetTrust(message.ProjectID, message.Trusted))
}

func (h handlers) SetProjectSandbox(message protocol.ClientMessageSetProjectSandbox) struct{} {
	return h.problem(h.app.Projects.SetSandbox(message.ProjectID, message.On))
}

func (h handlers) SetProjectAfterEdit(message protocol.ClientMessageSetProjectAfterEdit) struct{} {
	return h.problem(h.app.Projects.SetAfterEdit(message.ProjectID, message.Command))
}

func (h handlers) SetProjectSkill(message protocol.ClientMessageSetProjectSkill) struct{} {
	return h.problem(h.app.Projects.SetSkill(message.ProjectID, message.Name, message.On))
}

func (h handlers) AddProjectTodo(message protocol.ClientMessageAddProjectTodo) struct{} {
	return h.problem(h.app.Projects.AddTodo(message.ProjectID, message.Text))
}

func (h handlers) SetProjectTodoDone(message protocol.ClientMessageSetProjectTodoDone) struct{} {
	return h.problem(h.app.Projects.SetTodoDone(message.ProjectID, message.TodoID, message.Done))
}

func (h handlers) SetProjectTodoText(message protocol.ClientMessageSetProjectTodoText) struct{} {
	return h.problem(h.app.Projects.SetTodoText(message.ProjectID, message.TodoID, message.Text))
}

func (h handlers) RemoveProjectTodo(message protocol.ClientMessageRemoveProjectTodo) struct{} {
	return h.problem(h.app.Projects.RemoveTodo(message.ProjectID, message.TodoID))
}

func (h handlers) RenameSession(message protocol.ClientMessageRenameSession) struct{} {
	return h.problem(h.app.Sessions.Rename(message.SessionID, message.Title))
}

func (h handlers) PinEvent(message protocol.ClientMessagePinEvent) struct{} {
	return h.problem(h.app.Sessions.Pin(message.SessionID, message.EventID))
}

func (h handlers) UnpinEvent(message protocol.ClientMessageUnpinEvent) struct{} {
	return h.problem(h.app.Sessions.Unpin(message.SessionID, message.EventID))
}

func (h handlers) RestoreCheckpoint(message protocol.ClientMessageRestoreCheckpoint) struct{} {
	return h.problem(h.app.Sessions.RestoreCheckpoint(message.SessionID, message.TurnID))
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

func (h handlers) MarkSeen(message protocol.ClientMessageMarkSeen) struct{} {
	return h.problem(h.app.Sessions.MarkSeen(message.SessionID, message.SeenAt))
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

// terminalOpened is a terminal and what it printed so far, as its bytes: a terminal's output is not always UTF-8.
func terminalOpened(info protocol.Terminal, output []byte) protocol.ServerMessage {
	return protocol.ServerMessage{Value: protocol.ServerMessageTerminalOpened{
		Output: base64.StdEncoding.EncodeToString(output), Terminal: info,
	}}
}

// watchTerminal makes this socket hear what the terminal prints from now on, until it is detached or closes.
func (h handlers) watchTerminal(id string) {
	h.watchers.watch(h.app.Terminals, id, func(data []byte) {
		h.reply(protocol.ServerMessage{Value: protocol.ServerMessageTerminalOutput{
			Data: base64.StdEncoding.EncodeToString(data), TerminalID: id,
		}})
	})
}

func (h handlers) AttachTerminal(message protocol.ClientMessageAttachTerminal) struct{} {
	info, output, found := h.app.Terminals.Attach(message.TerminalID)
	if !found {
		return h.problem(unknownTerminal)
	}
	h.problem(h.app.Terminals.Resize(message.TerminalID, message.Columns, message.Rows))
	h.watchTerminal(message.TerminalID)
	h.reply(terminalOpened(info, output))
	return struct{}{}
}

func (h handlers) DetachTerminal(message protocol.ClientMessageDetachTerminal) struct{} {
	h.watchers.drop(message.TerminalID)
	return struct{}{}
}

func (h handlers) OpenTerminal(message protocol.ClientMessageOpenTerminal) struct{} {
	go func() {
		host := protocol.LocalHost()
		if message.Host != nil {
			host = *message.Host
		}
		info, refused := h.app.Terminals.Open(h.ctx, terminals.Request{
			Columns: message.Columns, Cwd: message.Cwd, Host: host, Rows: message.Rows, Title: message.Title,
		})
		if refused != "" {
			h.reply(problem(refused))
			return
		}
		h.watchTerminal(info.ID)
		h.reply(terminalOpened(info, h.app.Terminals.Output(info.ID)))
	}()
	return struct{}{}
}

func (h handlers) TerminalInput(message protocol.ClientMessageTerminalInput) struct{} {
	return h.problem(h.app.Terminals.Write(message.TerminalID, message.Data))
}

func (h handlers) TerminalResize(message protocol.ClientMessageTerminalResize) struct{} {
	return h.problem(h.app.Terminals.Resize(message.TerminalID, message.Columns, message.Rows))
}

func (h handlers) RenameTerminal(message protocol.ClientMessageRenameTerminal) struct{} {
	return h.problem(h.app.Terminals.Rename(message.TerminalID, message.Title))
}

// ReviewChanges answers a page with what the session's workspace has uncommitted, a diff a file.
func (h handlers) ReviewChanges(message protocol.ClientMessageReviewChanges) struct{} {
	go func() {
		review, refused := h.app.Sessions.Review(message.SessionID)
		if refused != "" {
			h.reply(problem(refused))
			return
		}
		h.reply(protocol.ServerMessage{Value: protocol.ServerMessageReviewChanges{
			Branch: review.Branch, Files: review.Files, More: review.More, Problem: review.Problem, SessionID: message.SessionID,
		}})
	}()
	return struct{}{}
}

// CommitChanges commits what the session's workspace has uncommitted, and tells the page how it went.
func (h handlers) CommitChanges(message protocol.ClientMessageCommitChanges) struct{} {
	go func() {
		h.reply(protocol.ServerMessage{Value: protocol.ServerMessageChangesCommitted{
			Problem: h.app.Sessions.Commit(message.SessionID, message.Message), SessionID: message.SessionID,
		}})
	}()
	return struct{}{}
}

// CheckpointDiff answers a page with the files of a turn's restore point, a diff each: what they were before the
// turn, and what they are now.
func (h handlers) CheckpointDiff(message protocol.ClientMessageCheckpointDiff) struct{} {
	go func() {
		files, refused := h.app.Sessions.CheckpointDiff(message.SessionID, message.TurnID)
		if refused != "" {
			h.reply(problem(refused))
			return
		}
		h.reply(protocol.ServerMessage{Value: protocol.ServerMessageCheckpointDiff{
			Files: files, SessionID: message.SessionID, TurnID: message.TurnID,
		}})
	}()
	return struct{}{}
}

func (h handlers) CloseTerminal(message protocol.ClientMessageCloseTerminal) struct{} {
	h.watchers.drop(message.TerminalID)
	h.app.Terminals.Close(message.TerminalID)
	return struct{}{}
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

func (h handlers) CompactSession(message protocol.ClientMessageCompactSession) struct{} {
	return h.problem(h.app.Sessions.Compact(message.SessionID))
}

// CloseSession notes that the page shows that session no more, or none: its stream stops coming to this socket.
func (h handlers) CloseSession(message protocol.ClientMessageCloseSession) struct{} {
	h.app.hub.close(h.socket, message.SessionID)
	return struct{}{}
}

// Ping answers a page that asks whether the daemon still hears it: a socket can go without either end being told.
func (h handlers) Ping(protocol.ClientMessagePing) struct{} {
	h.reply(protocol.ServerMessage{Value: protocol.ServerMessagePong{}})
	return struct{}{}
}

// socket is the socket the app talks to the daemon through: commands one way, everything that happens the other.
// What it sends is compressed, each message by itself: a session's history is text that shrinks several times.
func (a *App) socket(response http.ResponseWriter, request *http.Request) {
	connection, err := websocket.Accept(response, request, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionNoContextTakeover, InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	connection.SetReadLimit(maxMessage)
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	if device, paired := a.pairing.DeviceOf(presentedToken(request)); paired {
		defer a.deviceSockets.open(device, cancel)()
	}
	box := &outbox{full: cancel, ready: make(chan struct{}, 1)}
	socketID, leave := a.hub.join(box)
	defer leave()
	box.push(protocol.ServerMessage{Value: protocol.ServerMessageProjects{Projects: a.Projects.List()}})
	box.push(protocol.ServerMessage{Value: protocol.ServerMessagePresets{Choices: a.Models.Choices()}})
	box.push(protocol.ServerMessage{Value: protocol.ServerMessageSessions{Sessions: a.Sessions.List()}})
	box.push(protocol.ServerMessage{Value: protocol.ServerMessageTerminals{Terminals: a.Terminals.List()}})
	go a.send(ctx, cancel, connection, box)
	go keepAlive(ctx, cancel, connection)
	watchers := newTerminalWatchers()
	defer watchers.close()
	reply := handlers{app: a, ctx: context.WithoutCancel(ctx), reply: box.push, socket: socketID, watchers: watchers}
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

// write sends one message, given writeLimit to leave.
func write(ctx context.Context, connection *websocket.Conn, data []byte) error {
	ctx, done := context.WithTimeout(ctx, writeLimit)
	defer done()
	return connection.Write(ctx, websocket.MessageText, data)
}

// keepAlive asks a socket now and then whether it is still there, and ends it when it does not say so in time.
func keepAlive(ctx context.Context, cancel context.CancelFunc, connection *websocket.Conn) {
	ticker := time.NewTicker(pingEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		asked, done := context.WithTimeout(ctx, pingLimit)
		err := connection.Ping(asked)
		done()
		if err != nil {
			cancel()
			return
		}
	}
}

// send writes a socket's messages as they come, until the socket closes.
func (a *App) send(ctx context.Context, cancel context.CancelFunc, connection *websocket.Conn, box *outbox) {
	defer cancel()
	for {
		for _, data := range box.take() {
			if write(ctx, connection, data) != nil {
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
