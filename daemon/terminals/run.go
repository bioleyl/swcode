package terminals

import (
	"bytes"
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
)

// since is what a terminal printed from a point of all it printed, and whether it still runs; the caller holds
// the lock. What the scrollback dropped since is gone.
func (t *terminal) since(start int) ([]byte, bool) {
	dropped := t.printed - len(t.buffer)
	return append([]byte(nil), t.buffer[max(0, start-dropped):]...), t.running
}

// Run types a command in a terminal's shell and waits until the shell says it ended: the person answers what it
// asks in the terminal itself, a password among other things, which swcode never sees. The terminal stays open
// after it: a password sudo took there is good for the next commands, as in any terminal. asks hears whether the
// command waits on a password now, each time that changes. A refusal says why nothing ran.
func (s *Service) Run(ctx context.Context, id, command string, asks func(waits bool)) (Ran, string) {
	s.mu.Lock()
	t, ok := s.terminals[id]
	if !ok || !t.running {
		s.mu.Unlock()
		return Ran{}, endedTerminal
	}
	start, announce, wipe := t.printed, !t.announced, !t.typed
	t.announced, t.typed = true, true
	s.mu.Unlock()
	mark := markOf(s.deps.NewID())
	if err := typeIn(t.process, command, mark, announce, wipe); err != nil {
		return Ran{}, "The command could not be typed in the terminal: " + err.Error()
	}
	ran := s.await(ctx, t, start, mark, asks)
	ran.Output = hosts.AfterEcho(ran.Output, mark)
	return ran, ""
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
	for {
		s.mu.Lock()
		output, running := t.since(start)
		s.mu.Unlock()
		if at, code, ended := hosts.CommandEnd(output, mark); ended {
			return Ran{Code: code, Ended: true, Output: output[:at]}
		}
		if !running {
			return Ran{Closed: true, Output: output}
		}
		if waits := hosts.PromptOf(string(bytes.TrimRight(output, " "))) != ""; waits != asking {
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
