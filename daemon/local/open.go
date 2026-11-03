package local

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"swcode/daemon/app"
	"swcode/daemon/config"
	"swcode/daemon/protocol"
)

const (
	failed  = 1
	logName = "swcode.log"
	// startLimit: the program starts in a second or two, a slow disk gets more.
	startLimit = 30 * time.Second
	// stopLimit: stopping waits for the sessions to close, up to the shutdown deadline.
	stopLimit = 15 * time.Second
)

// LogFileOf is where the daemon in the background writes its log.
func LogFileOf(settings config.Config) string {
	return filepath.Join(settings.DataDir, logName)
}

// linkOf is the link that opens the app: it carries the access token, which the app then keeps.
func linkOf(settings config.Config) (string, error) {
	token, err := app.AccessToken(settings.DataDir, settings.Token)
	if err != nil {
		return "", err
	}
	address := settings.PublicURL
	if address == "" {
		address = app.LocalURL(settings.Host, settings.Port)
	}
	return address + "/?" + protocol.TokenParam + "=" + token, nil
}

// openApp opens the app: on Windows, in a window of its own (Edge in app mode); elsewhere, or without Edge, in the
// browser.
func openApp(link string, place Place) Result {
	edge := ""
	if place.Platform == "windows" && place.WSLDistro == "" {
		edge = EdgeOf(place.Environment)
	}
	if edge == "" {
		return Run(BrowserCall(link, place))
	}
	if err := StartDetached(AppWindowCall(link, edge, place.Environment)); err != nil {
		return Result{Output: err.Error()}
	}
	return Result{OK: true}
}

// Open opens the app in the browser. The daemon starts in the background first when it does not answer, so opening
// it again only opens the browser, and it keeps running with no browser open.
func Open(settings config.Config, place Place) int {
	control := app.ControlURL(settings.Host, settings.Port)
	running := IsUp(control)
	if !running && !(StartInBackground(place) && WaitFor(func() bool { return IsUp(control) }, startLimit)) {
		fmt.Fprintf(os.Stderr, "swcode did not start: its log is %s.\n", LogFileOf(settings))
		return failed
	}
	link, err := linkOf(settings)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	if running {
		fmt.Println("swcode runs at " + link)
	} else {
		fmt.Println("swcode started at " + link)
	}
	if opened := openApp(link, place); !opened.OK {
		fmt.Fprintf(os.Stderr, "No browser opened (%s): open the link above.\n", opened.Output)
	}
	return 0
}

var stopMessages = map[StopAnswer]string{
	StopAsked:      "swcode stopped.",
	StopNotRunning: "swcode is not running.",
	StopOlder:      "This daemon is an older build, which cannot be stopped this way: stop it where it runs.",
	StopRefused:    "The daemon refused: it runs with another access token (SWCODE_TOKEN or another data folder).",
}

// Stop stops the daemon in the background, once its sessions are closed.
func Stop(settings config.Config) int {
	token, err := app.AccessToken(settings.DataDir, settings.Token)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	control := app.ControlURL(settings.Host, settings.Port)
	answer := AskToStop(control, token)
	fmt.Println(stopMessages[answer])
	switch answer {
	case StopNotRunning:
		return 0
	case StopAsked:
		if WaitFor(func() bool { return !IsUp(control) }, stopLimit) {
			return 0
		}
	}
	return failed
}
