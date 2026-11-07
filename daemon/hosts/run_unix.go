//go:build !windows

package hosts

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// ownGroup gives a command a session of its own, which is also its process group: it has no terminal, even when
// the daemon has one, so sudo and ssh never stop to read a password from the daemon's.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// killGroup stops a command and everything it started: politely, then for good a moment later.
func killGroup(process *os.Process) {
	if process == nil {
		return
	}
	group := -process.Pid
	_ = syscall.Kill(group, syscall.SIGTERM)
	time.AfterFunc(killGrace, func() { _ = syscall.Kill(group, syscall.SIGKILL) })
}

// hideWindow does nothing off Windows: only Windows opens a console for a program.
func hideWindow(*exec.Cmd) {}
