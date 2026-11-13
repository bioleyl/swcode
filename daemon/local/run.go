package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"swcode/daemon/app"
	"swcode/daemon/cli"
	"swcode/daemon/config"
	"swcode/daemon/headless"
	"swcode/daemon/protocol"
)

// RunSession runs one turn of a new session without the app: swcode run. The daemon starts in the background first
// when it does not answer, as when the app is opened, and the session stays in it afterwards for the app to show.
func RunSession(settings config.Config, place Place, options cli.RunOptions) int {
	cwd, err := filepath.Abs(options.Cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	control := app.ControlURL(settings.Host, settings.Port)
	if !IsUp(control) && !(startDaemon(place) && WaitFor(func() bool { return IsUp(control) }, startLimit)) {
		fmt.Fprintf(os.Stderr, "swcode did not start: its log is %s.\n", LogFileOf(settings))
		return failed
	}
	token, err := app.AccessToken(settings.DataDir, settings.Token)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	return headless.Run(context.Background(), headless.Options{
		Address: control, Cwd: cwd, Err: os.Stderr, JSON: options.JSON, Mode: protocol.PermissionMode(options.Permissions),
		Out: os.Stdout, Prompt: options.Prompt, Timeout: options.Timeout, Token: token,
	})
}
