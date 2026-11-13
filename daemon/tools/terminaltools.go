package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"swcode/daemon/hosts"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
	"swcode/daemon/terminals"
)

const (
	// The size a terminal the agent starts has until a page shows it: the app keeps it the size of its own view after.
	terminalColumns  = 100
	terminalRows     = 30
	terminalTitleMax = 60
	// The most of what a terminal printed that the model reads: a server's first seconds, or a build's last lines.
	maxTerminalOutput = 4000
	maxTerminalRead   = 12_000
	// How many lines terminal_output reads at most, from the end: a build's error comes last.
	defaultReadLines = 100
	minReadLines     = 1
	maxReadLines     = 500
	noTerminalAgent  = "Subagents do not open terminals: the main agent runs what the person watches."
	// waitLimit is how long a command the person may have to answer is waited for: they may be away.
	waitLimit    = 30 * time.Minute
	stillRunning = "[the command still runs in the terminal %q after %s: the person may not have answered it yet. Tell them, and read the terminal later with readterminal.]"
	closedFirst  = "[the terminal %q was closed before the command ended]"
	exitCodeLine = "[exit code %d]"
)

// escapeSequence is what a terminal draws with rather than says: the colours and cursor moves the model cannot read.
var escapeSequence = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b[()][A-Za-z0-9]|\x1b[=>]`)

// plainTerminal is what a terminal printed, without the escapes that draw it, its end when there is more than most
// characters: what the model can read.
func plainTerminal(output []byte, most int) string {
	text := strings.ReplaceAll(escapeSequence.ReplaceAllString(string(output), ""), "\r", "")
	runes := []rune(strings.TrimRight(text, " \t\r\n"))
	if len(runes) > most {
		return "…" + string(runes[len(runes)-most:])
	}
	return string(runes)
}

// lastLines is the end of what a terminal printed, the lines asked for at most, and the characters.
func lastLines(output []byte, count, most int) string {
	lines := strings.Split(plainTerminal(output, most), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, "\n")
}

// terminalStanding is how the model reads where a terminal is, and whether it still runs there.
type terminalStanding struct {
	text    string
	running bool
	exited  bool
	code    int
}

// standingOf maps a terminal's state to that: a new state does not compile until it is here.
type standingOf struct {
	id    string
	title string
}

func (s standingOf) Running(protocol.TerminalStateRunning) terminalStanding {
	return terminalStanding{running: true, text: fmt.Sprintf("[terminal %q (id %s) runs in the app beside the session]", s.title, s.id)}
}

func (s standingOf) Exited(state protocol.TerminalStateExited) terminalStanding {
	return terminalStanding{code: state.Code, exited: true, text: fmt.Sprintf("[terminal %q ended with exit code %d]", s.title, state.Code)}
}

func (s standingOf) Signalled(protocol.TerminalStateSignalled) terminalStanding {
	return terminalStanding{text: fmt.Sprintf("[terminal %q was stopped]", s.title)}
}

type terminalInput struct {
	Command string `json:"command"`
	Title   string `json:"title"`
	Wait    bool   `json:"wait"`
}

var terminalTool = Define(Spec[terminalInput]{
	Check: func(_ context.Context, input terminalInput, tc *Context) Check {
		if tc.Terminals == nil {
			return Refuse(noTerminalAgent)
		}
		if input.Wait {
			return Ready(func(ctx context.Context) Outcome { return runInTerminal(ctx, input, tc) }, Text(input.Command))
		}
		return Ready(func(ctx context.Context) Outcome { return startTerminal(ctx, input, tc) }, Text(input.Command))
	},
	Description: "Runs a command in a terminal of the app, in the workspace, which the person watches beside the session and can type in. Two uses. A long command (a dev server, a watcher): the call returns after a few seconds with what it printed so far, the terminal runs on, and readterminal reads it later. A command the person has to answer (sudo or ssh asking for a password, a login, an installer's questions): set wait, and the call waits until the command ends while the person types in the terminal, then gives what it printed and its exit code; say in your answer nothing of the password, the app tells the person where to type it. Give the terminal a short title people know it by.",
	Input: Object(
		Required("command", String().Min(1).Describe("The command, run by a shell in the workspace.")),
		Required("title", String().Min(1).Max(terminalTitleMax).Describe("What the terminal is called in the app, in a few words: dev server, setup. With wait, a title already in use runs the command in that terminal: a password sudo took there is not asked for again for a while.")),
		Optional("wait", Boolean().Describe("Wait until the command ends, while the person answers what it asks in the terminal: for sudo, ssh with a password, a login, or any command that asks questions.")),
	),
	Name: protocol.ToolNameTerminal,
	// A terminal is the person's own shell: what runs there is outside the session's sandbox, when it has one.
	Subject: func(input terminalInput, tc *Context) permissions.Subject {
		return permissions.Subject{Access: permissions.AccessCommand, Command: input.Command, Outside: tc.Sandbox != nil}
	},
	Summarize: func(input terminalInput) string { return input.Title + ": " + input.Command },
})

type terminalOutputInput struct {
	Lines    *float64 `json:"lines"`
	Terminal string   `json:"terminal"`
}

var terminalOutputTool = Define(Spec[terminalOutputInput]{
	Check: func(_ context.Context, input terminalOutputInput, tc *Context) Check {
		if tc.Terminals == nil {
			return Refuse(noTerminalAgent)
		}
		return Ready(func(_ context.Context) Outcome { return readTerminal(tc, input) }, nil)
	},
	Description: "Reads the last lines a terminal of the workspace printed, such as the error a build failed with, and how the terminal stands. A terminal the person opened in the app's side panel is read the same way.",
	Input: Object(
		Optional("terminal", String().Describe("The terminal's name (dev server) or its id, as terminal gave it; the workspace's only terminal when left out, and a list of them when there are several.")),
		Optional("lines", Number().Between(minReadLines, maxReadLines).Describe("How many lines to read from its end; 100 when left out.")),
	),
	Name:    protocol.ToolNameReadterminal,
	Subject: reads,
	Summarize: func(input terminalOutputInput) string {
		if input.Terminal == "" {
			return "the terminal"
		}
		return "terminal " + input.Terminal
	},
})

// readTerminal is the end of a terminal's output, the one named or the workspace's only one.
func readTerminal(tc *Context, input terminalOutputInput) Outcome {
	found := workspaceTerminals(tc)
	if len(found) == 0 {
		return Failure("This workspace has no terminal: open one in the app's side panel, or start one with terminal.", "no terminal")
	}
	chosen, named := namedTerminal(found, input.Terminal)
	if !named && len(found) > 1 {
		return Failure("There are several terminals: "+terminalNames(found)+". Name the one to read.", "choose a terminal")
	}
	lines := whole(input.Lines, defaultReadLines)
	return terminalLines(tc.Terminals, chosen, int(lines), maxTerminalRead)
}

// workspaceTerminals are the terminals of the session's folder on its host.
func workspaceTerminals(tc *Context) []protocol.Terminal {
	found := []protocol.Terminal{}
	for _, each := range tc.Terminals.List() {
		if each.Cwd == tc.Workspace && hosts.SameHost(each.Host, tc.HostRef) {
			found = append(found, each)
		}
	}
	return found
}

// namedTerminal is the terminal the name or id tells, if any: a title is not case-sensitive.
func namedTerminal(found []protocol.Terminal, name string) (protocol.Terminal, bool) {
	wanted := strings.ToLower(strings.TrimSpace(name))
	for _, each := range found {
		if each.ID == name || (wanted != "" && strings.ToLower(each.Title) == wanted) {
			return each, true
		}
	}
	return protocol.Terminal{}, false
}

func terminalNames(found []protocol.Terminal) string {
	names := make([]string, 0, len(found))
	for _, each := range found {
		names = append(names, fmt.Sprintf("%q", each.Title))
	}
	return strings.Join(names, ", ")
}

// terminalLines is what a terminal printed last, and how it stands, as the model reads it: the lines and characters
// asked for at most.
func terminalLines(port terminals.Port, chosen protocol.Terminal, count, most int) Outcome {
	state, found := port.Terminal(chosen.ID)
	if !found {
		return Failure(fmt.Sprintf("The terminal %q closed: its output is gone.", chosen.Title), "closed")
	}
	output := lastLines(port.Output(chosen.ID), count, most)
	if output == "" {
		output = noOutput
	}
	standing := protocol.MapTerminalState[terminalStanding](state.State, standingOf{id: chosen.ID, title: chosen.Title})
	meta := "ended"
	if standing.running {
		meta = "running"
	}
	succeeded := standing.running || (standing.exited && standing.code == 0)
	text := joined(output, standing.text)
	return Outcome{Display: Text(text), Meta: meta, OK: succeeded, Output: text}
}

// startTerminal opens the terminal, waits for the command's first seconds, and gives what it printed and how it
// stands.
func startTerminal(ctx context.Context, input terminalInput, tc *Context) Outcome {
	info, refused := tc.Terminals.Open(ctx, terminals.Request{
		Columns: terminalColumns, Command: input.Command, Cwd: tc.Workspace, Host: tc.HostRef, Rows: terminalRows,
		Title: input.Title,
	})
	if refused != "" {
		return Failure(refused, "failed")
	}
	waitForStart(ctx)
	return terminalOutcome(tc.Terminals, info)
}

// waitingTerminal is the terminal a command the person answers runs in: the workspace's running one of that title,
// which keeps what sudo was told, else a new one.
func waitingTerminal(ctx context.Context, title string, tc *Context) (protocol.Terminal, string) {
	if found, named := namedTerminal(workspaceTerminals(tc), title); named {
		if _, running := found.State.Value.(protocol.TerminalStateRunning); running {
			return found, ""
		}
	}
	return tc.Terminals.Open(ctx, terminals.Request{
		Columns: terminalColumns, Cwd: tc.Workspace, Host: tc.HostRef, Rows: terminalRows, Title: title,
	})
}

// runInTerminal types the command in a terminal of the app and waits until it ends, while the person answers
// what it asks there: a password is typed in the terminal itself, and never comes here.
func runInTerminal(ctx context.Context, input terminalInput, tc *Context) Outcome {
	terminal, refused := waitingTerminal(ctx, input.Title, tc)
	if refused != "" {
		return Failure(refused, "failed")
	}
	asks := tc.InTerminal(tc.CallID, terminal)
	defer asks(false)
	waiting, giveUp := context.WithTimeout(ctx, waitLimit)
	defer giveUp()
	ran, refused := tc.Terminals.Run(waiting, terminal.ID, input.Command, asks)
	if refused != "" {
		return Failure(refused, "failed")
	}
	return ranOutcome(ctx, terminal, ran)
}

// ranOutcome is how a command the person could answer went, as the model reads it: what it printed, then its exit
// code, or why it has none.
func ranOutcome(ctx context.Context, terminal protocol.Terminal, ran terminals.Ran) Outcome {
	printed := plainTerminal(ran.Output, maxTerminalRead)
	result := func(standing, meta string) Outcome {
		text := joined(printed, standing)
		return Outcome{Display: Text(text), Meta: meta, OK: ran.Ended && ran.Code == 0, Output: text}
	}
	switch {
	case ran.Ended && ran.Code == 0:
		return result("", "done")
	case ran.Ended:
		return result(fmt.Sprintf(exitCodeLine, ran.Code), fmt.Sprintf("exit code %d", ran.Code))
	case ran.Closed:
		return result(fmt.Sprintf(closedFirst, terminal.Title), "closed")
	case ctx.Err() != nil:
		return result("[stopped]", "stopped")
	}
	return result(fmt.Sprintf(stillRunning, terminal.Title, waitLimit), "still running")
}

// waitForStart gives the command its first seconds, as a background job's call does.
func waitForStart(ctx context.Context) {
	deadline := time.Now().Add(startWait)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		select {
		case <-ctx.Done():
		case <-time.After(startPoll):
		}
	}
}

// terminalOutcome is what the terminal printed in its first seconds, and how it stands, as the model reads it.
func terminalOutcome(port terminals.Port, opened protocol.Terminal) Outcome {
	return terminalLines(port, opened, defaultReadLines, maxTerminalOutput)
}
