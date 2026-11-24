//go:build windows

package hosts

import (
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

const createNoWindow = 0x08000000

// commandGroup is a command's own Job Object: what the command starts joins the job, and a job is not left by its
// children, so stopping it ends the whole tree however deep it went. A command that could not be put in one falls
// back to taskkill.
type commandGroup struct {
	mu  sync.Mutex
	job windows.Handle
}

// ownGroup is the attribute that gives a command a job (a tree) of its own, and the job that then stops it all. The
// daemon's commands and its terminals both take one.
func ownGroup() (*syscall.SysProcAttr, *commandGroup) {
	attr := &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | createNoWindow}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return attr, &commandGroup{}
	}
	return attr, &commandGroup{job: job}
}

// started puts the command in its job: only then does the job hold what the command starts. A job that could not
// hold it is given back, and the tree is then stopped by taskkill alone.
func (g *commandGroup) started(process *os.Process) {
	job := g.jobHandle()
	if job == 0 || process == nil {
		return
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err != nil {
		g.close()
		return
	}
	defer windows.CloseHandle(handle)
	if err := windows.AssignProcessToJobObject(job, handle); err != nil {
		g.close()
	}
}

// stop ends every process in the job, then whatever taskkill still finds under the command.
func (g *commandGroup) stop(process *os.Process) {
	if job := g.take(); job != 0 {
		_ = windows.TerminateJobObject(job, 1)
		_ = windows.CloseHandle(job)
	}
	killTree(process)
}

// close gives the job handle back without ending anything: what the command left behind with `&` keeps running.
func (g *commandGroup) close() {
	if job := g.take(); job != 0 {
		_ = windows.CloseHandle(job)
	}
}

// take is the job handle, once, and the group then holds none. Stopping and ending race on a job that is still
// finishing its command.
func (g *commandGroup) take() windows.Handle {
	g.mu.Lock()
	defer g.mu.Unlock()
	job := g.job
	g.job = 0
	return job
}

// jobHandle is the job, if the group still holds one.
func (g *commandGroup) jobHandle() windows.Handle {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.job
}

// killTree ends a process and its children: Windows has no process groups to signal, taskkill walks the tree.
func killTree(process *os.Process) {
	if process == nil {
		return
	}
	kill := exec.Command("taskkill", "/pid", strconv.Itoa(process.Pid), "/T", "/F")
	kill.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	_ = kill.Run()
}

// hideWindow keeps a console program (wsl.exe, ssh) from opening a window of its own.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
}
