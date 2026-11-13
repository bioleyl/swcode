package hosts

import (
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/aymanbagabas/go-pty"
)

const (
	defaultTerminalColumns = 80
	defaultTerminalRows    = 24
	terminalType           = "TERM=xterm-256color"
	terminalColors         = "COLORTERM=truecolor"
)

// TerminalOptions is where a terminal's shell runs, and how much of it shows: the app keeps it the size of its view.
type TerminalOptions struct {
	Cwd     string
	Columns int
	Rows    int
}

func terminalSize(options TerminalOptions) (columns, rows int) {
	columns, rows = options.Columns, options.Rows
	if columns <= 0 {
		columns = defaultTerminalColumns
	}
	if rows <= 0 {
		rows = defaultTerminalRows
	}
	return columns, rows
}

// terminalEnvironment is the daemon's environment with the terminal the shell draws in: what it says of its own,
// which has none, leaves.
func terminalEnvironment(environment []string) []string {
	kept := make([]string, 0, len(environment)+2)
	for _, entry := range environment {
		if strings.HasPrefix(entry, "TERM=") || strings.HasPrefix(entry, "COLORTERM=") {
			continue
		}
		kept = append(kept, entry)
	}
	return append(kept, terminalType, terminalColors)
}

// Terminal is a shell kept open on a pseudo-terminal, which people type in from the app: what it prints reads on
// until it ends, and its size follows the view.
type Terminal struct {
	terminal pty.Pty
	command  *pty.Cmd
	// close is once: the person closing the terminal and its shell ending both let it go, and go-pty keeps no lock.
	close sync.Once
	// shell is the shell that runs in it: how a command typed there says it ended.
	shell Shell
}

// Announce readies the terminal's shell for commands the agent waits for: typed once, before the first one. wipe
// clears the view after it, in a terminal nobody typed in yet. The space before it keeps it out of the shell's
// history.
func (t *Terminal) Announce(wipe bool) error {
	line := " " + t.shell.Announce
	if wipe {
		line += "; " + t.shell.Wipe
	}
	_, err := t.Write([]byte(line + "\n"))
	return err
}

// Type types a command in the terminal's shell, which says when it ended, with that mark and its exit code
// (CommandEnd finds it in what the terminal prints).
func (t *Terminal) Type(command, mark string) error {
	_, err := t.Write([]byte(command + typedEnd(mark) + "\n"))
	return err
}

// StartTerminal starts the command attached to a pseudo-terminal of its own: the same program, folder and
// environment, with a terminal it can draw in.
func StartTerminal(command *exec.Cmd, options TerminalOptions, shell Shell) (*Terminal, error) {
	terminal, err := pty.New()
	if err != nil {
		return nil, err
	}
	environment := command.Env
	if environment == nil {
		environment = os.Environ()
	}
	started := terminal.Command(command.Path, command.Args[1:]...)
	started.Dir = command.Dir
	started.Env = terminalEnvironment(environment)
	if err := started.Start(); err != nil {
		_ = terminal.Close()
		return nil, err
	}
	columns, rows := terminalSize(options)
	if err := terminal.Resize(columns, rows); err != nil {
		_ = terminal.Close()
		return nil, err
	}
	return &Terminal{command: started, shell: shell, terminal: terminal}, nil
}

func (t *Terminal) Read(data []byte) (int, error) {
	return t.terminal.Read(data)
}

func (t *Terminal) Write(data []byte) (int, error) {
	return t.terminal.Write(data)
}

// Resize keeps the shell's terminal as wide and as tall as the view that types in it.
func (t *Terminal) Resize(columns, rows int) error {
	return t.terminal.Resize(columns, rows)
}

// Wait is how the shell ended: its exit code, and whether it exited at all (a signal ends one without).
func (t *Terminal) Wait() (code int, exited bool) {
	_ = t.command.Wait()
	if t.command.ProcessState == nil {
		return 0, false
	}
	code = t.command.ProcessState.ExitCode()
	return code, code >= 0
}

// Close lets go of the terminal: its shell is hung up, and what it holds freed. It may come from either side, the
// person closing the terminal or its shell ending, so only the first one does it.
func (t *Terminal) Close() {
	t.close.Do(func() { _ = t.terminal.Close() })
}
