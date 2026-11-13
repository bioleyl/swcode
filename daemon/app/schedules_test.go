package app_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	. "swcode/daemon/testapp"
)

// movingClock is a clock a test sets.
type movingClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *movingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *movingClock) set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

func schedulesOf(t *testing.T, test *App) []protocol.ProjectSchedule {
	t.Helper()
	project, found := test.App.Projects.Find(test.Workspace, protocol.LocalHost())
	if !found {
		t.Fatal("no project")
	}
	return project.Schedules
}

func TestAScheduleStartsASessionOnItsPromptAtItsTimeAndOnceOnly(t *testing.T) {
	morning := time.Date(2026, time.November, 13, 7, 0, 0, 0, time.Local)
	clock := &movingClock{now: morning}
	test := Start(t, Options{Now: clock.Now, Replies: []fakellama.Reply{text("The build is green."), text("Still green.")}})
	project := test.App.Projects.Ensure(test.Workspace, protocol.LocalHost())
	defaults := protocol.SessionDefaults{Concise: true, Mode: protocol.PermissionModeAsk, Thinking: protocol.ThinkingModeLow}
	if problem := test.App.Projects.SetDefaults(project.ID, &defaults); problem != "" {
		t.Fatal(problem)
	}
	if problem := test.App.Projects.AddSchedule(project.ID, "Check the build", protocol.ScheduleEveryDay, "08:00", 0); problem != "" {
		t.Fatal(problem)
	}
	if problem := test.App.Projects.AddSchedule(project.ID, "Never asked", protocol.ScheduleEveryDay, "08:00", 0); problem != "" {
		t.Fatal(problem)
	}
	if problem := test.App.Projects.SetScheduleOn(project.ID, schedulesOf(t, test)[1].ID, false); problem != "" {
		t.Fatal(problem)
	}

	// Before its time, nothing starts.
	test.App.RunDueSchedules(context.Background())
	if started := len(test.App.Sessions.List()); started != 0 {
		t.Fatalf("%d sessions before the schedule's time", started)
	}

	// At its time, the one that is on starts a session of the project, on its prompt, as the project's sessions start.
	clock.set(morning.Add(61 * time.Minute))
	test.App.RunDueSchedules(context.Background())
	started := test.App.Sessions.List()
	if len(started) != 1 || started[0].Workspace != test.Workspace || started[0].Mode != protocol.PermissionModeAsk || !started[0].Concise {
		t.Fatalf("the sessions started: %+v", started)
	}
	test.UntilIdle(started[0].ID)
	if said := fakellama.Messages(test.Requests()[0])[1]["content"].(string); !strings.Contains(said, "Check the build") {
		t.Fatalf("the session's first message: %q", said)
	}
	ran := schedulesOf(t, test)[0]
	if ran.LastSessionID != started[0].ID || ran.LastRunAt != protocol.Timestamp(clock.Now()) {
		t.Fatalf("the schedule notes %+v", ran)
	}
	if off := schedulesOf(t, test)[1]; off.LastRunAt != "" || off.LastSessionID != "" {
		t.Fatalf("a schedule that is off ran: %+v", off)
	}

	// The next looks start nothing more, until its next time: then once, however many days were missed.
	clock.set(morning.Add(3 * time.Hour))
	test.App.RunDueSchedules(context.Background())
	if count := len(test.App.Sessions.List()); count != 1 {
		t.Fatalf("%d sessions after a second look the same day", count)
	}
	clock.set(morning.AddDate(0, 0, 3).Add(2 * time.Hour))
	test.App.RunDueSchedules(context.Background())
	test.App.RunDueSchedules(context.Background())
	if count := len(test.App.Sessions.List()); count != 2 {
		t.Fatalf("%d sessions after three days off", count)
	}
}

func TestASchedulesTimesCountFromWhenItWasAddedOrTurnedOn(t *testing.T) {
	afternoon := time.Date(2026, time.November, 13, 15, 0, 0, 0, time.Local)
	clock := &movingClock{now: afternoon}
	test := Start(t, Options{Now: clock.Now, Replies: []fakellama.Reply{}})
	project := test.App.Projects.Ensure(test.Workspace, protocol.LocalHost())
	if problem := test.App.Projects.AddSchedule(project.ID, "Check the build", protocol.ScheduleEveryDay, "08:00", 0); problem != "" {
		t.Fatal(problem)
	}
	// Added after today's time: it waits for tomorrow's.
	clock.set(afternoon.Add(time.Hour))
	test.App.RunDueSchedules(context.Background())
	// Turned off over tomorrow's time and on again after it: it waits for the day after's.
	id := schedulesOf(t, test)[0].ID
	test.App.Projects.SetScheduleOn(project.ID, id, false)
	clock.set(afternoon.AddDate(0, 0, 1))
	test.App.Projects.SetScheduleOn(project.ID, id, true)
	test.App.RunDueSchedules(context.Background())
	if count := len(test.App.Sessions.List()); count != 0 {
		t.Fatalf("%d sessions for times that passed before the schedule counted", count)
	}
	if problem := test.App.Projects.RemoveSchedule(project.ID, id); problem != "" || len(schedulesOf(t, test)) != 0 {
		t.Fatalf("removing it: %q, %+v", problem, schedulesOf(t, test))
	}
}
