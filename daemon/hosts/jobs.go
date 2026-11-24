package hosts

import (
	"strconv"
	"sync"
)

// The newest output a job keeps until it is read: a dev server's log can grow for hours.
const maxUnreadBytes = 50_000

// maxEndedJobs is how many jobs that ended are kept for a read that comes late: the oldest record goes first, so a
// daemon running for months holds no more.
const maxEndedJobs = 32

type job struct {
	id       string
	command  string
	unread   []byte
	dropped  int
	running  bool
	exitCode *int
	process  *jobProcess
}

// jobProcess is a started command's process, kept to stop it.
type jobProcess struct {
	stop func()
}

// jobOutput collects what a job prints, keeping the newest.
type jobOutput struct {
	jobs *Jobs
	job  *job
}

// Write keeps the newest output only: a job that prints faster than it is read drops the oldest of it, in one copy
// rather than one per write.
func (o jobOutput) Write(data []byte) (int, error) {
	o.jobs.mu.Lock()
	defer o.jobs.mu.Unlock()
	o.job.unread = append(o.job.unread, data...)
	if over := len(o.job.unread) - maxUnreadBytes; over > 0 {
		o.job.dropped += over
		o.job.unread = append([]byte(nil), o.job.unread[over:]...)
	}
	return len(data), nil
}

// Jobs are the commands of this computer that run on after the call that started them, each in its own process
// group (its own tree on Windows), their output kept until it is read. They end when stopped, or with the host.
type Jobs struct {
	mu    sync.Mutex
	shell Shell
	jobs  map[string]*job
	// finished are the ids of the jobs that ended, oldest first: their records go at maxEndedJobs.
	finished []string
	count    int
}

func NewJobs(shell Shell) *Jobs {
	return &Jobs{jobs: map[string]*job{}, shell: shell}
}

// Start starts a command in a folder: its id.
func (j *Jobs) Start(command, cwd string) string {
	j.mu.Lock()
	j.count++
	id := strconv.Itoa(j.count)
	started := &job{id: id, command: command, running: true}
	j.jobs[id] = started
	j.mu.Unlock()
	cmd, group := commandOf(j.shell, command, cwd)
	cmd.Stdout = jobOutput{jobs: j, job: started}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		cmd.Stdout.Write([]byte(err.Error()))
		j.ended(started, nil)
		return id
	}
	group.started(cmd.Process)
	started.process = &jobProcess{stop: func() { group.stop(cmd.Process) }}
	go func() {
		j.ended(started, exitCodeOf(cmd.Wait()))
		group.close()
	}()
	return id
}

func (j *Jobs) ended(done *job, code *int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	done.running = false
	done.exitCode = code
	if _, kept := j.jobs[done.id]; !kept {
		return
	}
	j.finished = append(j.finished, done.id)
	for len(j.finished) > maxEndedJobs {
		delete(j.jobs, j.finished[0])
		j.finished = j.finished[1:]
	}
}

// Read is a job's state; consume takes the output read, so the next read gives what came after.
func (j *Jobs) Read(id string, consume bool) *JobState {
	j.mu.Lock()
	defer j.mu.Unlock()
	found, ok := j.jobs[id]
	if !ok {
		return nil
	}
	state := &JobState{
		Command:  found.command,
		Dropped:  found.dropped,
		ExitCode: found.exitCode,
		Output:   string(found.unread),
		Running:  found.running,
	}
	if consume {
		found.unread = nil
		found.dropped = 0
	}
	return state
}

// Stop stops a job; what it printed before is still there to read once.
func (j *Jobs) Stop(id string) *JobState {
	j.mu.Lock()
	found, ok := j.jobs[id]
	j.mu.Unlock()
	if !ok {
		return nil
	}
	j.stopOne(found)
	return j.Read(id, true)
}

// StopAll stops every job still running: the host is closing.
func (j *Jobs) StopAll() {
	j.mu.Lock()
	running := []*job{}
	for _, each := range j.jobs {
		running = append(running, each)
	}
	j.mu.Unlock()
	for _, each := range running {
		j.stopOne(each)
	}
}

func (j *Jobs) stopOne(target *job) {
	j.mu.Lock()
	process, running := target.process, target.running
	j.mu.Unlock()
	if running && process != nil {
		process.stop()
	}
}
