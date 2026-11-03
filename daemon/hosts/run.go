package hosts

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

const (
	headChars = 10_000
	tailChars = 20_000
	// A process that ignores SIGTERM this long is killed.
	killGrace = 2 * time.Second
	// A command that leaves a process behind (`server &`) exits while that process still holds its output: the call
	// ends this long after the shell exits, without waiting for the output to close.
	exitGrace = time.Second
)

// quietEnvironment: commands run without a terminal, so no pager waiting for a key, no colour codes in the output.
var quietEnvironment = []string{"FORCE_COLOR=0", "GIT_PAGER=cat", "NO_COLOR=1", "PAGER=cat"}

// commandOf is a shell command in a folder, in a process group (a tree on Windows) of its own, so it is stopped
// with all it started.
func commandOf(shell Shell, command, cwd string) *exec.Cmd {
	cmd := exec.Command(shell.Program, shell.Args(command)...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), quietEnvironment...)
	ownGroup(cmd)
	return cmd
}

// RunCommand runs a shell command in a folder, with its output, stopped when it takes too long or ctx is done.
func RunCommand(ctx context.Context, command string, options CommandOptions, shell Shell) CommandResult {
	output := NewOutputBuffer(headChars, tailChars)
	cmd := commandOf(shell, command, options.Cwd)
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.WaitDelay = exitGrace
	if err := cmd.Start(); err != nil {
		output.Write([]byte(err.Error()))
		return CommandResult{Output: output.Text()}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(options.Timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return CommandResult{ExitCode: exitCodeOf(err), Output: output.Text()}
	case <-ctx.Done():
		killGroup(cmd.Process)
		<-done
		return CommandResult{Output: output.Text(), Stopped: true}
	case <-timer.C:
		killGroup(cmd.Process)
		<-done
		return CommandResult{Output: output.Text(), TimedOut: true}
	}
}

// exitCodeOf is how a command ended: nil when a signal ended it.
func exitCodeOf(err error) *int {
	if err == nil {
		code := 0
		return &code
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() >= 0 {
		code := exit.ExitCode()
		return &code
	}
	// The output stayed open past the grace after the shell ended (a process left behind): it did end.
	if errors.Is(err, exec.ErrWaitDelay) {
		code := 0
		return &code
	}
	return nil
}
