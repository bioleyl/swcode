//go:build windows

package hosts

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

const createNoWindow = 0x08000000

func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | createNoWindow}
}

// killGroup ends a process and its children: Windows has no process groups to signal, taskkill walks the tree.
func killGroup(process *os.Process) {
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
