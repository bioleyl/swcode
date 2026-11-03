// Package local is what the swcode command does on this computer: open the app, start and stop the daemon in the
// background, install the program.
package local

import (
	"os"
	"os/exec"
	"strings"
)

// Call is a program to run, with its arguments.
type Call struct {
	Program string
	Args    []string
}

// Result is what a program said, once it ended.
type Result struct {
	OK     bool
	Output string
}

// Run runs a program to its end; what Windows programs print comes without their carriage returns.
func Run(call Call) Result {
	cmd := exec.Command(call.Program, call.Args...)
	hidden(cmd)
	output, err := cmd.CombinedOutput()
	return Result{OK: err == nil, Output: strings.TrimSpace(strings.ReplaceAll(string(output), "\r", ""))}
}

// StartDetached starts a program that outlives this one: its own session, no terminal, nothing waiting for it.
func StartDetached(call Call) error {
	cmd := exec.Command(call.Program, call.Args...)
	detach(cmd)
	devnull, err := os.Open(os.DevNull)
	if err == nil {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
