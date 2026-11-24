//go:build !windows

package hosts

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// commandGroup is a command's own process group, which everything it starts joins: stop ends them all.
type commandGroup struct{}

// ownGroup is the attribute that gives a command a process group (a tree on Windows) of its own, and the group that
// then stops it all. The daemon's commands and its terminals both take one.
func ownGroup() (*syscall.SysProcAttr, *commandGroup) {
	return &syscall.SysProcAttr{Setsid: true}, &commandGroup{}
}

// started has nothing to do off Windows: the process group is known from the command's pid.
func (*commandGroup) started(*os.Process) {}

// stop stops a command and everything it started: politely, then for good a moment later. An interactive shell puts
// each of its jobs in a group of its own, so the group alone is not the tree: what is under it is stopped too.
func (*commandGroup) stop(process *os.Process) {
	if process == nil {
		return
	}
	group := -process.Pid
	started := descendants(process.Pid)
	_ = syscall.Kill(group, syscall.SIGTERM)
	for _, pid := range started {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	time.AfterFunc(killGrace, func() {
		for _, pid := range started {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		_ = syscall.Kill(group, syscall.SIGKILL)
	})
}

// descendants are the processes under pid as `ps` lists them, however deep: a shell's jobs are no longer its group.
func descendants(pid int) []int {
	out, err := exec.Command("ps", "-A", "-o", "pid=", "-o", "ppid=").Output()
	if err != nil {
		return nil
	}
	children := map[int][]int{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		child, childErr := strconv.Atoi(fields[0])
		parent, parentErr := strconv.Atoi(fields[1])
		if childErr != nil || parentErr != nil {
			continue
		}
		children[parent] = append(children[parent], child)
	}
	found := []int{}
	queue := []int{pid}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		found = append(found, children[next]...)
		queue = append(queue, children[next]...)
	}
	return found
}

// close has nothing to give back off Windows: the group is no handle of its own.
func (*commandGroup) close() {}

// hideWindow does nothing off Windows: only Windows opens a console for a program.
func hideWindow(*exec.Cmd) {}
