package app

import (
	"context"
	"time"

	"swcode/daemon/projects"
	"swcode/daemon/sessions"
)

// scheduleLook is how often the daemon looks for schedules that are due: their times are minutes.
const scheduleLook = time.Minute

// watchSchedules starts the sessions the projects' schedules ask for, looking every minute until ctx is done.
func (a *App) watchSchedules(ctx context.Context) {
	ticker := time.NewTicker(scheduleLook)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.RunDueSchedules(ctx)
		}
	}
}

// requestOf is the session a schedule starts: on its prompt, in its project's folder, with what the project's new
// sessions start with.
func requestOf(due projects.DueSchedule) sessions.Request {
	host := due.Project.Host
	request := sessions.Request{Host: &host, Prompt: due.Schedule.Prompt, Workspace: due.Project.Path}
	if defaults := due.Project.Defaults; defaults != nil {
		request.Concise, request.Mode, request.Preset, request.Thinking = defaults.Concise, defaults.Mode, defaults.Preset, defaults.Thinking
	}
	return request
}

// RunDueSchedules starts a session for each schedule due now, and notes it on the schedule. One whose session
// could not start is noted too, without a session: it is tried again at its next time, not every minute.
func (a *App) RunDueSchedules(ctx context.Context) {
	now := a.options.Now()
	for _, due := range a.Projects.DueSchedules(now) {
		session, refused := a.Sessions.Create(ctx, requestOf(due))
		if refused != "" {
			a.logger.Error("the schedule “" + due.Schedule.Prompt + "” of " + due.Project.Name + " started no session: " + refused)
		}
		a.Projects.RanSchedule(due.Project.ID, due.Schedule.ID, now, session.ID)
	}
}
