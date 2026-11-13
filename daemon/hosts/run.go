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

// RunCommand runs a shell command in a folder, with its output, stopped when it takes too long or ctx is done. The
// command has no input at all, which is how the commands of another host run: they cannot be answered.
func RunCommand(ctx context.Context, command string, options CommandOptions, shell Shell) CommandResult {
	options.Passwords = nil
	return runCommand(ctx, command, options, shell, "")
}

// RunCommandAsks runs a shell command the same way, and gives it what options.Passwords answers when it stops to ask
// for a password: only this computer's commands have an input to give. askpass is the daemon's password prompt,
// which sudo -A and ssh run (empty when it could not be written: they then cannot ask).
func RunCommandAsks(ctx context.Context, command string, options CommandOptions, shell Shell, askpass string) CommandResult {
	return runCommand(ctx, command, options, shell, askpass)
}

// runCommand is a command started, its output watched for a password prompt while it runs when someone can answer
// one, and ended when its time is up, ctx is done, or it came back.
func runCommand(ctx context.Context, command string, options CommandOptions, shell Shell, askpass string) CommandResult {
	output := NewOutputBuffer(headChars, tailChars)
	cmd := commandOf(shell, command, options.Cwd)
	cmd.Stdout = heard(output, options.Live)
	cmd.Stderr = cmd.Stdout
	cmd.WaitDelay = exitGrace
	// askedAbout is nil where nobody can answer what the command asks: its output is then never looked at, and it
	// has no input at all, as it always had.
	askedAbout := prompterOf(cmd, output, options.Passwords, askpass)
	defer askedAbout.close()
	if err := cmd.Start(); err != nil {
		output.Write([]byte(err.Error()))
		return CommandResult{Output: withoutNotes(output.Text())}
	}
	// What the command asked is asked no longer once it is over: the person's card goes with it.
	asking, stopAsking := context.WithCancel(ctx)
	defer stopAsking()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(options.Timeout)
	defer timer.Stop()
	for {
		select {
		case err := <-done:
			return CommandResult{ExitCode: exitCodeOf(err), Output: withoutNotes(output.Text())}
		case <-ctx.Done():
			killGroup(cmd.Process)
			<-done
			return CommandResult{Output: withoutNotes(output.Text()), Stopped: true}
		case <-timer.C:
			killGroup(cmd.Process)
			<-done
			return CommandResult{Output: withoutNotes(output.Text()), TimedOut: true}
		case <-askedAbout.looks():
			// Waiting for the person takes none of the command's time.
			stopWhile(askedAbout.look(asking), timer)
		case closed := <-askedAbout.answered():
			// The command has its whole time again from here.
			askedAbout.replied(closed)
			timer.Reset(options.Timeout)
		}
	}
}

// stopWhile stops the command's clock while the person is asked.
func stopWhile(asked bool, timer *time.Timer) {
	if !asked {
		return
	}
	timer.Stop()
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
