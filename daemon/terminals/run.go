package terminals

import (
	"context"
	"strings"
	"time"

	"swcode/daemon/hosts"
)

const (
	// lookEvery is how often a terminal is looked at while a command typed in it runs.
	lookEvery = 100 * time.Millisecond
	// markLength is how long the mark a command's line ends with is: enough to tell its end from any other's.
	markLength = 8
	// markWindow is how much of what a background command prints is kept to find its mark: what bridges a chunk.
	markWindow = 4 << 10
)

// readFrom is what a terminal printed after a point of all it printed, and whether it still runs; the caller holds
// the lock. What the scrollback dropped since that point is gone: the read starts at what is left.
func (t *terminal) readFrom(after int) ([]byte, int, bool) {
	dropped := t.printed - len(t.buffer)
	return append([]byte(nil), t.buffer[max(after, dropped)-dropped:]...), t.printed, t.running
}

// Run types a command in a terminal's shell and waits until the shell says it ended: the person answers what it
// asks in the terminal itself, a password among other things, which swcode never sees. The terminal stays open
// after it: a password sudo took there is good for the next commands, as in any terminal. asks hears whether the
// command waits on a password now, each time that changes. A refusal says why nothing ran.
func (s *Service) Run(ctx context.Context, id, command string, asks func(waits bool)) (Ran, string) {
	t, start, mark, refused := s.beginCommand(id, command)
	if refused != "" {
		return Ran{}, refused
	}
	ran := s.await(ctx, t, start, mark, asks)
	ran.Output = hosts.AfterEcho(ran.Output, mark)
	return ran, ""
}

// StartInBackground types a command in a terminal and lets it run on: once the shell says it ended, the terminal is
// finished and closes at the end of its session's turn. What it prints is not kept here.
func (s *Service) StartInBackground(id, command string) string {
	t, start, mark, refused := s.beginCommand(id, command)
	if refused != "" {
		// The command never ran: the terminal has nothing to show, so it leaves now.
		s.Close(id)
		return refused
	}
	go func() {
		if s.commandEnding(t, start, mark) {
			s.MarkFinished(id)
		}
	}()
	return ""
}

// beginCommand readies a terminal's shell and types the command with a mark it says it ended by: the terminal,
// where its output starts, and the mark; a refusal says why nothing ran.
func (s *Service) beginCommand(id, command string) (t *terminal, start int, mark, refused string) {
	s.mu.Lock()
	found, ok := s.terminals[id]
	if !ok || !found.running {
		s.mu.Unlock()
		return nil, 0, "", endedTerminal
	}
	start, announce, wipe := found.printed, !found.announced, !found.typed
	found.announced, found.typed, found.finished = true, true, false
	s.mu.Unlock()
	mark = markOf(s.deps.NewID())
	if err := typeIn(found.process, command, mark, announce, wipe); err != nil {
		return nil, 0, "", "The command could not be typed in the terminal: " + err.Error()
	}
	return found, start, mark, ""
}

// commandEnding watches what a background command prints until the shell says it ended (true), or the terminal does
// (false): only enough of it to find the mark is kept.
func (s *Service) commandEnding(t *terminal, start int, mark string) bool {
	ticker := time.NewTicker(lookEvery)
	defer ticker.Stop()
	tail, cursor := []byte{}, start
	for {
		s.mu.Lock()
		chunk, next, running := t.readFrom(cursor)
		s.mu.Unlock()
		cursor = next
		seen := append(tail, chunk...)
		if _, _, ended := hosts.CommandEnd(seen, mark); ended {
			return true
		}
		if !running {
			return false
		}
		tail = seen[max(0, len(seen)-markWindow):]
		<-ticker.C
	}
}

// markOf is a few characters no other command of the terminal ends with, from an id.
func markOf(id string) string {
	letters := strings.ReplaceAll(id, "-", "")
	return letters[:min(len(letters), markLength)]
}

// typeIn types a command in a terminal's shell, readied first when no command was waited for there yet.
func typeIn(process *hosts.Terminal, command, mark string, announce, wipe bool) error {
	if announce {
		if err := process.Announce(wipe); err != nil {
			return err
		}
	}
	return process.Type(command, mark)
}

// await looks at what the terminal prints until the command ended, the terminal did, or the wait is given up.
func (s *Service) await(ctx context.Context, t *terminal, start int, mark string, asks func(waits bool)) Ran {
	ticker := time.NewTicker(lookEvery)
	defer ticker.Stop()
	asking := false
	output, cursor := []byte{}, start
	for {
		s.mu.Lock()
		chunk, next, running := t.readFrom(cursor)
		s.mu.Unlock()
		output = append(output, chunk...)
		cursor = next
		// Keep what a command printed as bounded as the scrollback: a command that floods its terminal does not
		// keep growing the wait's copy of it.
		if over := len(output) - maxScrollback; over > 0 {
			output = append(output[:0], output[over:]...)
		}
		// A terminal that closed is looked at first: closing it ends the command, and its shell may still say so
		// (ended, by a signal) before it goes itself. What happened is that the terminal closed.
		if !running {
			return Ran{Closed: true, Output: output}
		}
		if at, code, ended := hosts.CommandEnd(output, mark); ended {
			return Ran{Code: code, Ended: true, Output: output[:at]}
		}
		if waits := hosts.PromptAtEnd(output) != ""; waits != asking {
			asking = waits
			asks(waits)
		}
		select {
		case <-ctx.Done():
			return Ran{Output: output}
		case <-ticker.C:
		}
	}
}
