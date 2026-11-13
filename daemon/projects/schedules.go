package projects

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"swcode/daemon/protocol"
)

const (
	unknownSchedule  = "There is no such schedule."
	tooManySchedules = "A project keeps 50 schedules at most."
	hoursPerDay      = 24
	minutesPerHour   = 60
	daysPerWeek      = 7
	timeParts        = 2
)

// clockOf reads a schedule's time of the day, `08:00`: its hour and its minute, and whether it is one.
func clockOf(at string) (int, int, bool) {
	parts := strings.Split(at, ":")
	if len(parts) != timeParts {
		return 0, 0, false
	}
	hour, hourErr := strconv.Atoi(parts[0])
	minute, minuteErr := strconv.Atoi(parts[1])
	if hourErr != nil || minuteErr != nil || hour < 0 || hour >= hoursPerDay || minute < 0 || minute >= minutesPerHour {
		return 0, 0, false
	}
	return hour, minute, true
}

// lastSlot is the latest time, now or before, at which the schedule says to start a session, in now's own time
// zone: this hour's minute or the hour before's, today's time or yesterday's, this week's day or last week's.
func lastSlot(schedule protocol.ProjectSchedule, now time.Time) (time.Time, bool) {
	hour, minute, ok := clockOf(schedule.At)
	if !ok {
		return time.Time{}, false
	}
	year, month, day := now.Date()
	switch schedule.Every {
	case protocol.ScheduleEveryHour:
		slot := time.Date(year, month, day, now.Hour(), minute, 0, 0, now.Location())
		if slot.After(now) {
			slot = slot.Add(-time.Hour)
		}
		return slot, true
	case protocol.ScheduleEveryDay:
		slot := time.Date(year, month, day, hour, minute, 0, 0, now.Location())
		if slot.After(now) {
			slot = slot.AddDate(0, 0, -1)
		}
		return slot, true
	case protocol.ScheduleEveryWeek:
		back := (int(now.Weekday()) - schedule.Weekday + daysPerWeek) % daysPerWeek
		slot := time.Date(year, month, day-back, hour, minute, 0, 0, now.Location())
		if slot.After(now) {
			slot = slot.AddDate(0, 0, -daysPerWeek)
		}
		return slot, true
	}
	return time.Time{}, false
}

// Due tells whether a schedule starts a session now: it is on, and one of its times has come since last, which is
// when it last ran, or from when its times count for one that never did. Several times missed (the daemon was
// off) are one: it runs once, and not again until its next time.
func Due(schedule protocol.ProjectSchedule, last, now time.Time) bool {
	if !schedule.On {
		return false
	}
	slot, ok := lastSlot(schedule, now)
	return ok && slot.After(last)
}

// lastOf is the time a schedule's times count from: when it last ran, or when it was added or turned on, the later
// of the two.
func lastOf(schedule protocol.ProjectSchedule) time.Time {
	since, _ := protocol.ParseTimestamp(schedule.Since)
	ran, err := protocol.ParseTimestamp(schedule.LastRunAt)
	if err == nil && ran.After(since) {
		return ran
	}
	return since
}

// DueSchedule is a schedule that starts a session now, with its project.
type DueSchedule struct {
	Project  protocol.Project
	Schedule protocol.ProjectSchedule
}

// DueSchedules is the schedules of every project that start a session now.
func (s *Service) DueSchedules(now time.Time) []DueSchedule {
	s.mu.Lock()
	defer s.mu.Unlock()
	due := []DueSchedule{}
	for _, project := range s.projects {
		for _, schedule := range project.Schedules {
			if Due(schedule, lastOf(schedule), now) {
				due = append(due, DueSchedule{Project: project, Schedule: schedule})
			}
		}
	}
	return due
}

// scheduleAt finds a schedule of a project: their places in the lists, or why not.
func (s *Service) scheduleAt(projectID, scheduleID string) (int, int, string) {
	index := slices.IndexFunc(s.projects, func(project protocol.Project) bool { return project.ID == projectID })
	if index < 0 {
		return 0, 0, unknownProject
	}
	scheduleIndex := slices.IndexFunc(s.projects[index].Schedules, func(schedule protocol.ProjectSchedule) bool {
		return schedule.ID == scheduleID
	})
	if scheduleIndex < 0 {
		return 0, 0, unknownSchedule
	}
	return index, scheduleIndex, ""
}

// changeSchedule changes one schedule of a project, and tells the apps.
func (s *Service) changeSchedule(projectID, scheduleID string, change func(*protocol.ProjectSchedule)) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, scheduleIndex, problem := s.scheduleAt(projectID, scheduleID)
	if problem != "" {
		return problem
	}
	projects := append([]protocol.Project{}, s.projects...)
	projects[index].Schedules = slices.Clone(projects[index].Schedules)
	change(&projects[index].Schedules[scheduleIndex])
	s.change(projects)
	return ""
}

// AddSchedule has the project start a session by itself on a prompt, every hour, day or week at a time, from now on:
// a time already past today is not made up for.
func (s *Service) AddSchedule(projectID, prompt string, every protocol.ScheduleEvery, at string, weekday int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := slices.IndexFunc(s.projects, func(project protocol.Project) bool { return project.ID == projectID })
	if index < 0 {
		return unknownProject
	}
	if len(s.projects[index].Schedules) >= protocol.MaxSchedules {
		return tooManySchedules
	}
	schedule := protocol.ProjectSchedule{
		At: at, Every: every, ID: s.deps.NewID(), On: true, Prompt: prompt, Since: protocol.Timestamp(s.deps.Now()), Weekday: weekday,
	}
	projects := append([]protocol.Project{}, s.projects...)
	projects[index].Schedules = append(slices.Clone(projects[index].Schedules), schedule)
	s.change(projects)
	return ""
}

// SetScheduleOn turns a schedule on or off. Turned on, its times count from now: those that passed while it was off
// are not made up for.
func (s *Service) SetScheduleOn(projectID, scheduleID string, on bool) string {
	since := protocol.Timestamp(s.deps.Now())
	return s.changeSchedule(projectID, scheduleID, func(schedule *protocol.ProjectSchedule) {
		if on && !schedule.On {
			schedule.Since = since
		}
		schedule.On = on
	})
}

// RemoveSchedule deletes one of the project's schedules; the sessions it started stay.
func (s *Service) RemoveSchedule(projectID, scheduleID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, scheduleIndex, problem := s.scheduleAt(projectID, scheduleID)
	if problem != "" {
		return problem
	}
	projects := append([]protocol.Project{}, s.projects...)
	projects[index].Schedules = slices.Delete(slices.Clone(projects[index].Schedules), scheduleIndex, scheduleIndex+1)
	s.change(projects)
	return ""
}

// RanSchedule notes that a schedule ran at a time, and the session it started; none when the session could not
// start, which is not tried again before the schedule's next time.
func (s *Service) RanSchedule(projectID, scheduleID string, at time.Time, sessionID string) string {
	return s.changeSchedule(projectID, scheduleID, func(schedule *protocol.ProjectSchedule) {
		schedule.LastRunAt = protocol.Timestamp(at)
		schedule.LastSessionID = sessionID
	})
}
