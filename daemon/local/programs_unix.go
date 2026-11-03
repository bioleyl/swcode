//go:build !windows

package local

import (
	"os/exec"
	"syscall"
)

func hidden(*exec.Cmd) {}

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
