package terminals

import (
	"context"

	"swcode/daemon/protocol"
)

// Request is a terminal to open: the folder and its host, what it is called (numbered when left out), the command
// typed in it once it starts, and how much of it shows (the default size when left out).
type Request struct {
	Host    protocol.HostRef
	Cwd     string
	Title   string
	Command string
	Columns int
	Rows    int
}

// Port is what a session's tools do with terminals: open one in its folder, read what it printed so far, and tell how
// it stands now.
type Port interface {
	Open(ctx context.Context, request Request) (protocol.Terminal, string)
	// List is every terminal, in the order they were opened.
	List() []protocol.Terminal
	Output(id string) []byte
	Terminal(id string) (protocol.Terminal, bool)
	// Run types a command in a terminal's shell and waits for it to end, while the person answers what it asks
	// there; asks hears whether it waits on a password now.
	Run(ctx context.Context, id, command string, asks func(waits bool)) (Ran, string)
}

// Ran is how a command typed in a terminal went: what the terminal printed from its line on, and whether it ended
// (with which exit code), or the terminal closed first; neither when the wait for it was given up.
type Ran struct {
	Output []byte
	Code   int
	Ended  bool
	Closed bool
}
