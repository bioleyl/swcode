package hosts

import (
	"strconv"
	"sync"
)

// The newest output a job keeps until it is read: a dev server's log can grow for hours.
const maxUnreadBytes = 50_000

type job struct {
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

func (o jobOutput) Write(data []byte) (int, error) {
	o.jobs.mu.Lock()
	defer o.jobs.mu.Unlock()
	unread := append(o.job.unread, data...)
	overflow := max(0, len(unread)-maxUnreadBytes)
	o.job.dropped += overflow
	o.job.unread = append([]byte(nil), unread[overflow:]...)
	return len(data), nil
}

// Jobs are the commands of this computer that run on after the call that started them, each in its own process
// group (its own tree on Windows), their output kept until it is read. They end when stopped, or with the host.
type Jobs struct {
	mu    sync.Mutex
	shell Shell
	jobs  map[string]*job
	count int
}

func NewJobs(shell Shell) *Jobs {
	return &Jobs{jobs: map[string]*job{}, shell: shell}
}

// Start starts a command in a folder: its id.
func (j *Jobs) Start(command, cwd string) string {
	j.mu.Lock()
	j.count++
	id := strconv.Itoa(j.count)
	started := &job{command: command, running: true}
	j.jobs[id] = started
	j.mu.Unlock()
	cmd := commandOf(j.shell, command, cwd)
	cmd.Stdout = jobOutput{jobs: j, job: started}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		cmd.Stdout.Write([]byte(err.Error()))
		j.ended(started, nil)
		return id
	}
	started.process = &jobProcess{stop: func() { killGroup(cmd.Process) }}
	go func() { j.ended(started, exitCodeOf(cmd.Wait())) }()
	return id
}

func (j *Jobs) ended(ended *job, code *int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	ended.running = false
	ended.exitCode = code
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
