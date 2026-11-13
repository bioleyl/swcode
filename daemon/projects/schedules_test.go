package projects

import (
	"testing"
	"time"

	"swcode/daemon/protocol"
)

// at is a time of Friday 13 November 2026 or a day near it, in this machine's time zone.
func at(day, hour, minute int) time.Time {
	return time.Date(2026, time.November, day, hour, minute, 0, 0, time.Local)
}

func schedule(every protocol.ScheduleEvery, when string, weekday int) protocol.ProjectSchedule {
	return protocol.ProjectSchedule{At: when, Every: every, ID: "s1", On: true, Prompt: "Check the build", Weekday: weekday}
}

func TestAScheduleIsDueOnceItsTimeHasComeSinceItLastRan(t *testing.T) {
	friday := 13
	cases := []struct {
		name     string
		schedule protocol.ProjectSchedule
		last     time.Time
		now      time.Time
		due      bool
	}{
		{"every day, before its time", schedule(protocol.ScheduleEveryDay, "08:00", 0), at(friday, 7, 0), at(friday, 7, 59), false},
		{"every day, at its time", schedule(protocol.ScheduleEveryDay, "08:00", 0), at(friday, 7, 0), at(friday, 8, 0), true},
		{"every day, already run today", schedule(protocol.ScheduleEveryDay, "08:00", 0), at(friday, 8, 0), at(friday, 9, 30), false},
		{"every day, the day after", schedule(protocol.ScheduleEveryDay, "08:00", 0), at(friday, 8, 0), at(friday+1, 8, 1), true},
		{"every day, added after today's time", schedule(protocol.ScheduleEveryDay, "08:00", 0), at(friday, 10, 0), at(friday, 23, 0), false},
		{"every hour, before its minute", schedule(protocol.ScheduleEveryHour, "00:30", 0), at(friday, 9, 0), at(friday, 9, 29), false},
		{"every hour, at its minute", schedule(protocol.ScheduleEveryHour, "00:30", 0), at(friday, 9, 0), at(friday, 9, 30), true},
		{"every hour, already run this hour", schedule(protocol.ScheduleEveryHour, "00:30", 0), at(friday, 9, 30), at(friday, 10, 29), false},
		{"every hour, the hour after", schedule(protocol.ScheduleEveryHour, "00:30", 0), at(friday, 9, 30), at(friday, 10, 30), true},
		{"every week, another day", schedule(protocol.ScheduleEveryWeek, "08:00", int(time.Monday)), at(friday, 7, 0), at(friday+1, 9, 0), false},
		{"every week, its day", schedule(protocol.ScheduleEveryWeek, "08:00", int(time.Monday)), at(friday, 7, 0), at(friday+3, 8, 0), true},
		{"every week, already run this week", schedule(protocol.ScheduleEveryWeek, "08:00", int(time.Monday)), at(friday+3, 8, 0), at(friday+6, 8, 0), false},
		{"every week, the week after", schedule(protocol.ScheduleEveryWeek, "08:00", int(time.Monday)), at(friday+3, 8, 0), at(friday+10, 8, 5), true},
		{"a time that is none", schedule(protocol.ScheduleEveryDay, "8 o'clock", 0), at(friday, 7, 0), at(friday+1, 9, 0), false},
	}
	for _, each := range cases {
		if got := Due(each.schedule, each.last, each.now); got != each.due {
			t.Errorf("%s: due %v, want %v", each.name, got, each.due)
		}
	}
}

func TestAScheduleThatIsOffIsNeverDue(t *testing.T) {
	off := schedule(protocol.ScheduleEveryDay, "08:00", 0)
	off.On = false
	if Due(off, at(13, 7, 0), at(14, 9, 0)) {
		t.Fatal("a schedule that is off was due")
	}
}

func TestTimesMissedWhileTheDaemonWasOffAreOneAndNeverRunTwice(t *testing.T) {
	daily := schedule(protocol.ScheduleEveryDay, "08:00", 0)
	last := at(13, 8, 0)
	// Five days later: due, once.
	now := at(18, 12, 0)
	if !Due(daily, last, now) {
		t.Fatal("not due after days off")
	}
	// It ran now: not due again at the next looks, until its next time.
	if Due(daily, now, now.Add(time.Minute)) || Due(daily, now, at(18, 23, 59)) {
		t.Fatal("due twice for the same time")
	}
	if !Due(daily, now, at(19, 8, 0)) {
		t.Fatal("not due at its next time")
	}
}
