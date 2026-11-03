package app

import (
	"fmt"
	"strings"

	"swcode/daemon/protocol"
)

// RunningWork is what closing the desktop app would stop: the sessions at work, and the commands running in the
// background.
type RunningWork struct {
	Sessions []string
	Commands []string
}

func RunningWorkOf(sessions []protocol.SessionSummary) RunningWork {
	work := RunningWork{Commands: []string{}, Sessions: []string{}}
	for _, session := range sessions {
		for _, job := range session.Jobs {
			work.Commands = append(work.Commands, job.Command)
		}
		if session.Status != protocol.SessionStatusIdle {
			work.Sessions = append(work.Sessions, session.Title)
		}
	}
	return work
}

func (w RunningWork) Idle() bool { return len(w.Sessions) == 0 && len(w.Commands) == 0 }

func counted(count int, one, many string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, one)
	}
	return fmt.Sprintf("%d %s", count, many)
}

// Warning is the warning before closing: what runs, by name, and that closing stops it.
func (w RunningWork) Warning() string {
	parts := []string{}
	if len(w.Sessions) > 0 {
		parts = append(parts, fmt.Sprintf("%s working: %s.", counted(len(w.Sessions), "session is", "sessions are"), strings.Join(w.Sessions, ", ")))
	}
	if len(w.Commands) > 0 {
		parts = append(parts, fmt.Sprintf("%s in the background: %s.", counted(len(w.Commands), "command runs", "commands run"), strings.Join(w.Commands, ", ")))
	}
	return strings.Join(append(parts, "Closing swcode stops them."), "\n")
}
