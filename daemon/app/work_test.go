package app

import (
	"testing"

	"swcode/daemon/protocol"
)

func summaryOf(title string, status protocol.SessionStatus, commands ...string) protocol.SessionSummary {
	jobs := []protocol.SessionSummaryJob{}
	for index, command := range commands {
		jobs = append(jobs, protocol.SessionSummaryJob{Command: command, ID: string(rune('1' + index))})
	}
	return protocol.SessionSummary{ID: title, Jobs: jobs, Status: status, Title: title}
}

func TestClosingNeedsNoWarningWhenNothingRuns(t *testing.T) {
	if !RunningWorkOf([]protocol.SessionSummary{summaryOf("Idle one", protocol.SessionStatusIdle)}).Idle() {
		t.Fatal("an idle session is no work")
	}
}

func TestTheWarningNamesTheSessionsAtWorkAndTheCommandsLeftRunning(t *testing.T) {
	work := RunningWorkOf([]protocol.SessionSummary{
		summaryOf("Fix the build", protocol.SessionStatusWorking),
		summaryOf("Ask first", protocol.SessionStatusWaiting),
		summaryOf("Done", protocol.SessionStatusIdle, "npm run dev"),
	})
	expected := "2 sessions are working: Fix the build, Ask first.\n1 command runs in the background: npm run dev.\nClosing swcode stops them."
	if work.Idle() || work.Warning() != expected {
		t.Fatalf("expected %q, found %q", expected, work.Warning())
	}
}
