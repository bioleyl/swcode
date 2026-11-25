package terminals

import (
	"context"

	"swcode/daemon/protocol"
)

// Request is a terminal to open: the folder and its host, what it is called (numbered when left out), and how much of
// it shows (the default size when left out).
type Request struct {
	Host    protocol.HostRef
	Cwd     string
	Title   string
	Columns int
	Rows    int
	// Agent: the agent opened it, so once its command ended the terminal leaves the side panel at the end of the
	// session's turn, and what the person never typed in it leaves nothing behind.
	Agent bool
	// SessionID is the session whose agent opened it, so deleting that session closes it.
	SessionID string
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
	// StartInBackground types a command in a terminal and lets it run on: once the command ends, the terminal is
	// finished. A refusal says why nothing ran.
	StartInBackground(id, command string) string
	// MarkFinished notes that the command typed in a terminal ended, so its terminal closes at the end of the
	// session's turn, or at once when no turn runs: a terminal the person typed in stays.
	MarkFinished(id string)
	// SessionWorking notes that a turn of the session works: a terminal whose command ends now stays until it ends.
	SessionWorking(sessionID string)
	// SessionIdle closes the terminals the agent opened for it whose commands ended, when its turn ends.
	SessionIdle(sessionID string)
	// CloseSession closes the terminals a session opened, when that session is deleted.
	CloseSession(sessionID string)
}

// Ran is how a command typed in a terminal went: what the terminal printed from its line on, and whether it ended
// (with which exit code), or the terminal closed first; neither when the wait for it was given up.
type Ran struct {
	Output []byte
	Code   int
	Ended  bool
	Closed bool
}
