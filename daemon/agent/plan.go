package agent

import (
	"strings"

	"swcode/daemon/protocol"
)

// maxReminders is enough for a long task that stops early more than once, never a loop.
const maxReminders = 3

// lastTurnStart is where the session's last turn started in the events, -1 when none did.
func lastTurnStart(events []protocol.SessionEvent) int {
	for index := len(events) - 1; index >= 0; index-- {
		if _, started := events[index].Value.(protocol.SessionEventTurnStarted); started {
			return index
		}
	}
	return -1
}

// LastTurn is the events of the session's last turn, from its start; a subagent's own conversation has no turns:
// all of it.
func LastTurn(events []protocol.SessionEvent) []protocol.SessionEvent {
	if start := lastTurnStart(events); start >= 0 {
		return events[start:]
	}
	return events
}

// TurnRunning is the turn the events leave unfinished, when the daemon stopped in the middle of it: its id, or ""
// when the last turn ended, or there is none.
func TurnRunning(events []protocol.SessionEvent) string {
	start := lastTurnStart(events)
	if start < 0 {
		return ""
	}
	turn := events[start:]
	for _, event := range turn {
		if _, finished := event.Value.(protocol.SessionEventTurnFinished); finished {
			return ""
		}
	}
	started, _ := turn[0].Value.(protocol.SessionEventTurnStarted)
	return started.ID
}

func planOf(event protocol.SessionEvent) []protocol.PlanStep {
	finished, ok := event.Value.(protocol.SessionEventToolFinished)
	if !ok || finished.Display == nil {
		return nil
	}
	plan, ok := finished.Display.Value.(protocol.ToolDisplayPlan)
	if !ok {
		return nil
	}
	return plan.Steps
}

// CurrentPlan is the plan of the session's last turn, as the agent's latest plan call left it: none until it
// writes one.
func CurrentPlan(events []protocol.SessionEvent) []protocol.PlanStep {
	turn := LastTurn(events)
	for index := len(turn) - 1; index >= 0; index-- {
		if steps := planOf(turn[index]); len(steps) > 0 {
			return steps
		}
	}
	return nil
}

func unfinished(steps []protocol.PlanStep) []protocol.PlanStep {
	left := []protocol.PlanStep{}
	for _, step := range steps {
		if step.Status != protocol.PlanStepStatusCompleted {
			left = append(left, step)
		}
	}
	return left
}

func reminderText(steps []protocol.PlanStep) string {
	lines := []string{"Your plan still has steps that are not completed:"}
	for _, step := range steps {
		lines = append(lines, "- "+step.Content)
	}
	lines = append(lines, "Go on with them. If some are no longer needed, write the plan again without them, then give your answer. If you need the user to answer something first, say so in one line and stop.")
	return strings.Join(lines, "\n")
}

// PlanReminder is what to tell a model that ended its turn with its plan unfinished, or nothing: some models
// announce their next step, then stop without doing it. A reminder comes only when the model did something since
// the last one, so one it ignores ends the turn.
func PlanReminder(events []protocol.SessionEvent) string {
	turn := LastTurn(events)
	left := unfinished(CurrentPlan(turn))
	reminders, lastReminder, lastCall := 0, -1, -1
	for index, event := range turn {
		switch value := event.Value.(type) {
		case protocol.SessionEventReminder:
			if value.Reason == protocol.ReminderReasonPlan {
				reminders, lastReminder = reminders+1, index
			}
		case protocol.SessionEventToolCall:
			lastCall = index
		}
	}
	if len(left) == 0 || reminders >= maxReminders || lastCall <= lastReminder {
		return ""
	}
	return reminderText(left)
}
