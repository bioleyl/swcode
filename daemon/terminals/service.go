// Package terminals runs the shells people type in from the app: a terminal per shell, in a project's folder, on its
// host, kept open and kept in a scrollback while the daemon runs. The agent opens them too, titled, for what people
// should watch.
package terminals

import (
	"context"
	"fmt"
	"sync"
	"time"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
)

const (
	// maxScrollback is what a terminal keeps for a page that comes late: enough to redraw a screen and read back.
	maxScrollback = 256 << 10
	// readSize is how much of its output one message carries at most.
	readSize = 32 << 10
	// drainDelay is how long what a shell printed as it ended is read, before its terminal is let go.
	drainDelay = 100 * time.Millisecond
	// clearScreen resets a view before a scrollback whose start was dropped: its escapes would be cut.
	clearScreen = "\x1b[0m\x1b[2J\x1b[H"
	// terminalName is what a terminal the person opens is called, and numbered: Terminal, Terminal 2…
	terminalName    = "Terminal"
	unknownTerminal = "There is no such terminal."
	endedTerminal   = "This terminal has ended."
)

// Hosts is where terminals run: the one of each project.
type Hosts interface {
	HostOf(ref protocol.HostRef) hosts.Host
}

// Deps is what the service needs.
type Deps struct {
	Hosts Hosts
	NewID func() string
}

// terminal is one shell: what it is, what it printed kept for a page that joins, and who hears what comes next.
type terminal struct {
	info    protocol.Terminal
	process *hosts.Terminal
	running bool
	buffer  []byte
	// cut says the start of its output was dropped: a view clears itself before drawing what is left.
	cut         bool
	watchers    map[int]func([]byte)
	nextWatcher int
}

// view is what a page draws to have the terminal as it is: its scrollback, cleared first when its start is gone.
func (t *terminal) view() []byte {
	if !t.cut {
		return append([]byte(nil), t.buffer...)
	}
	return append([]byte(clearScreen), t.buffer...)
}

// Service is the terminals of the daemon: each on its project's host, in its folder.
type Service struct {
	deps      Deps
	mu        sync.Mutex
	terminals map[string]*terminal
	order     []string
	listeners map[int]func(protocol.ServerMessage)
	next      int
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps, listeners: map[int]func(protocol.ServerMessage){}, terminals: map[string]*terminal{}}
}

// Open starts a shell in the folder, on the host, on a terminal of its own; a title is given it when none is, and
// the agent's command is typed in it once it starts. A refusal says why.
func (s *Service) Open(ctx context.Context, request Request) (protocol.Terminal, string) {
	host := s.deps.Hosts.HostOf(request.Host)
	if refused := s.checkFolder(ctx, host, request); refused != "" {
		return protocol.Terminal{}, refused
	}
	process, err := host.StartTerminal(hosts.TerminalOptions{Cwd: request.Cwd, Columns: request.Columns, Rows: request.Rows})
	if err != nil {
		return protocol.Terminal{}, "The terminal could not be opened: " + err.Error()
	}
	opened, started := s.remember(request, process)
	go s.read(started, process)
	go s.wait(started, process)
	if request.Command != "" {
		if _, err := process.Write([]byte(request.Command + "\n")); err != nil {
			s.Close(opened.ID)
			return protocol.Terminal{}, "The command could not be typed in the terminal: " + err.Error()
		}
	}
	s.publish()
	return opened, ""
}

// checkFolder tells why a terminal cannot run there: the folder is not on that host, or not a folder.
func (s *Service) checkFolder(ctx context.Context, host hosts.Host, request Request) string {
	found, err := host.FileInfo(ctx, request.Cwd)
	if err != nil {
		return "The terminal could not be opened: " + err.Error()
	}
	if found.Kind != hosts.KindDirectory {
		return fmt.Sprintf("There is no folder %s on %s.", request.Cwd, hosts.Label(request.Host))
	}
	return ""
}

// remember keeps a terminal opened, named by its number when the request names it not: what it is, and the terminal.
func (s *Service) remember(request Request, process *hosts.Terminal) (protocol.Terminal, *terminal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	title := request.Title
	if title == "" {
		title = s.nextTitle(request.Cwd, request.Host)
	}
	info := protocol.Terminal{
		Cwd: request.Cwd, Host: request.Host, ID: s.deps.NewID(), State: runningState(), Title: title,
	}
	started := &terminal{info: info, process: process, running: true, watchers: map[int]func([]byte){}}
	s.terminals[info.ID] = started
	s.order = append(s.order, info.ID)
	return info, started
}

// nextTitle is the first of Terminal, Terminal 2… the folder's terminals do not take.
func (s *Service) nextTitle(cwd string, host protocol.HostRef) string {
	taken := map[string]bool{}
	for _, id := range s.order {
		each := s.terminals[id]
		if each.info.Cwd == cwd && hosts.SameHost(each.info.Host, host) {
			taken[each.info.Title] = true
		}
	}
	title := terminalName
	for number := 2; taken[title]; number++ {
		title = fmt.Sprintf("%s %d", terminalName, number)
	}
	return title
}

func runningState() protocol.TerminalState {
	return protocol.TerminalState{Value: protocol.TerminalStateRunning{}}
}

// read takes what the shell prints, until its terminal is let go.
func (s *Service) read(t *terminal, process *hosts.Terminal) {
	data := make([]byte, readSize)
	for {
		n, err := process.Read(data)
		if n > 0 {
			s.deliver(t, data[:n])
		}
		if err != nil {
			return
		}
	}
}

// deliver keeps what came in the scrollback and hands it to the pages showing the terminal, which must not keep it.
func (s *Service) deliver(t *terminal, chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.buffer = append(t.buffer, chunk...)
	if over := len(t.buffer) - maxScrollback; over > 0 {
		t.buffer = append([]byte(nil), t.buffer[over:]...)
		t.cut = true
	}
	for _, watcher := range t.watchers {
		watcher(chunk)
	}
}

// wait is how the shell ended: what it printed as it did is read first, then its terminal shows it ended.
func (s *Service) wait(t *terminal, process *hosts.Terminal) {
	code, exited := process.Wait()
	time.Sleep(drainDelay)
	process.Close()
	s.mu.Lock()
	t.running = false
	t.info.State = endedState(code, exited)
	s.mu.Unlock()
	s.publish()
}

func endedState(code int, exited bool) protocol.TerminalState {
	if !exited {
		return protocol.TerminalState{Value: protocol.TerminalStateSignalled{}}
	}
	return protocol.TerminalState{Value: protocol.TerminalStateExited{Code: code}}
}

// List is every terminal, in the order they were opened.
func (s *Service) List() []protocol.Terminal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list()
}

func (s *Service) list() []protocol.Terminal {
	list := make([]protocol.Terminal, 0, len(s.order))
	for _, id := range s.order {
		list = append(list, s.terminals[id].info)
	}
	return list
}

// Attach is a terminal and what it printed so far, for a page that shows it: false for one it does not know.
func (s *Service) Attach(id string) (protocol.Terminal, []byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.terminals[id]
	if !ok {
		return protocol.Terminal{}, nil, false
	}
	return t.info, t.view(), true
}

// Terminal is one of them as it stands now: false for one it does not know.
func (s *Service) Terminal(id string) (protocol.Terminal, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.terminals[id]
	if !ok {
		return protocol.Terminal{}, false
	}
	return t.info, true
}

// Output is what the terminal printed so far, for the agent's call that started it.
func (s *Service) Output(id string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.terminals[id]
	if !ok {
		return nil
	}
	return t.view()
}

// Watch hears what the terminal prints from now on, until the function it answers is called. The watcher must not
// keep the data it is given.
func (s *Service) Watch(id string, watcher func([]byte)) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.terminals[id]
	if !ok {
		return func() {}
	}
	key := t.nextWatcher
	t.nextWatcher++
	t.watchers[key] = watcher
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(t.watchers, key)
	}
}

// Write is what the person typed, as the shell reads it: a refusal says a terminal it does not know or one that ended.
func (s *Service) Write(id, data string) string {
	s.mu.Lock()
	t, ok := s.terminals[id]
	alive := ok && t.running
	s.mu.Unlock()
	if !ok {
		return unknownTerminal
	}
	if !alive {
		return endedTerminal
	}
	if _, err := t.process.Write([]byte(data)); err != nil {
		return "The terminal could not take what was typed: " + err.Error()
	}
	return ""
}

// Resize keeps the shell's terminal as wide and as tall as the view that types in it.
func (s *Service) Resize(id string, columns, rows int) string {
	s.mu.Lock()
	t, ok := s.terminals[id]
	s.mu.Unlock()
	if !ok {
		return unknownTerminal
	}
	if err := t.process.Resize(columns, rows); err != nil {
		return "The terminal could not be resized: " + err.Error()
	}
	return ""
}

// Rename is the terminal under another name, as its page lists it: a refusal says a terminal it does not know.
func (s *Service) Rename(id, title string) string {
	s.mu.Lock()
	t, ok := s.terminals[id]
	if ok {
		t.info.Title = title
	}
	s.mu.Unlock()
	if !ok {
		return unknownTerminal
	}
	s.publish()
	return ""
}

// Close ends the shell and takes the terminal out of the list; one that is not there is no failure.
func (s *Service) Close(id string) {
	s.mu.Lock()
	t, ok := s.terminals[id]
	delete(s.terminals, id)
	for index, each := range s.order {
		if each == id {
			s.order = append(s.order[:index], s.order[index+1:]...)
			break
		}
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	t.process.Close()
	s.publish()
}

// CloseAll ends every terminal: the daemon stops.
func (s *Service) CloseAll() {
	s.mu.Lock()
	all := make([]*hosts.Terminal, 0, len(s.order))
	for _, id := range s.order {
		all = append(all, s.terminals[id].process)
	}
	s.terminals, s.order = map[string]*terminal{}, nil
	s.mu.Unlock()
	for _, process := range all {
		process.Close()
	}
}

// Subscribe hears the list of terminals whenever it changes: at open, at end, at close.
func (s *Service) Subscribe(listener func(protocol.ServerMessage)) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.next
	s.next++
	s.listeners[key] = listener
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.listeners, key)
	}
}

func (s *Service) publish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	message := protocol.ServerMessage{Value: protocol.ServerMessageTerminals{Terminals: s.list()}}
	for _, listener := range s.listeners {
		listener(message)
	}
}
