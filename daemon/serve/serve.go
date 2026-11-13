// Package serve runs the daemon until it is told to stop: a signal, or POST /api/stop.
package serve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"swcode/daemon/app"
	"swcode/daemon/browser"
	"swcode/daemon/cli"
	"swcode/daemon/config"
	"swcode/daemon/protocol"
	"swcode/daemon/tailnet"
	"swcode/daemon/update"
)

// Outcome is how the daemon ended: stopped, to start again on new settings, or on a newer version put in place of
// the program.
type Outcome int

const (
	Stopped Outcome = iota
	Restart
	Updated
)

// Options is how the daemon runs.
type Options struct {
	Mode cli.Mode
	// LogFile is where the log goes out of a terminal: it starts empty.
	LogFile     string
	Environment map[string]string
	Home        string
	Cwd         string
	// Stop ends the daemon from outside, when it is not nil: the desktop app's window closing.
	Stop <-chan struct{}
	// Ready is told the link of this computer once the daemon listens: the desktop app's window opens it.
	Ready func(link string)
	// Startup starts the daemon with its computer: the Windows app's.
	Startup app.StartupPort
	// Program is what installing a newer version replaces: the program the daemon runs in.
	Program update.Program
}

// The log says the link that opens swcode, its token in it: it is the person's alone, as its folder is.
const (
	logFolderMode = 0o700
	logFileMode   = 0o600
	// headerLimit is how long a connection has to say what it asks, and idleLimit how long one that asks nothing
	// more is kept: a daemon on a network is not held open by whoever connects and stays silent. A request's body
	// and its answer have no limit: the socket and a model's answer last as long as they last.
	headerLimit = 10 * time.Second
	idleLimit   = 2 * time.Minute
)

func logOf(options Options) (io.Writer, func(), error) {
	if options.Mode == cli.ModeTerminal || options.LogFile == "" {
		return os.Stderr, func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(options.LogFile), logFolderMode); err != nil {
		return nil, nil, err
	}
	// A service starts again on new settings and new versions: its log keeps what the runs before said.
	flags := os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	if options.Mode == cli.ModeService {
		flags = os.O_CREATE | os.O_APPEND | os.O_WRONLY
	}
	file, err := os.OpenFile(options.LogFile, flags, logFileMode)
	if err != nil {
		return nil, nil, err
	}
	// A log from before it was the person's alone becomes so: the mode given above only makes a new file.
	if err := file.Chmod(logFileMode); err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, func() { _ = file.Close() }, nil
}

// Run runs the daemon until it is told to stop. In a terminal it prints the links people open; elsewhere it writes
// everything to its log file instead.
func Run(settings config.Config, options Options) (Outcome, error) {
	log, closeLog, err := logOf(options)
	if err != nil {
		return Stopped, err
	}
	defer closeLog()
	ended := make(chan Outcome, 2)
	daemon, err := app.New(settings, app.Options{
		Defaults:    config.Defaults{Cwd: options.Cwd, Home: options.Home},
		Environment: options.Environment,
		FindBrowser: browser.Find,
		Home:        options.Home,
		Log:         log,
		OnRestart:   endingOf(options, ended, Restart),
		OnStop:      func() { ended <- Stopped },
		OnUpdated:   endingOf(options, ended, Updated),
		Program:     options.Program,
		Startup:     options.Startup,
		Tailscale:   tailnet.Command,
	})
	if err != nil {
		return Stopped, err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(settings.Host, strconv.Itoa(settings.Port)))
	if err != nil {
		daemon.Close()
		return Stopped, err
	}
	base, cancel := context.WithCancel(context.Background())
	server := &http.Server{
		BaseContext: func(net.Listener) context.Context { return base }, Handler: daemon.Handler,
		IdleTimeout: idleLimit, ReadHeaderTimeout: headerLimit,
	}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			daemon.Logger().Error(err.Error())
		}
	}()
	announce(settings, options, daemon, log)
	outcome := wait(ended, options.Stop)
	daemon.Logger().Info("Shutting down")
	shutdown, done := context.WithTimeout(context.Background(), time.Duration(settings.ShutdownTimeoutMs)*time.Millisecond)
	defer done()
	// A socket stays open: its context ends with the server's, so the pages let go.
	cancel()
	_ = server.Shutdown(shutdown)
	daemon.Close()
	return outcome, nil
}

// endingOf ends the daemon for new settings or a newer version, to start again on them: in a terminal, the person
// restarts it.
func endingOf(options Options, ended chan Outcome, outcome Outcome) func() {
	if options.Mode == cli.ModeTerminal {
		return nil
	}
	return func() { ended <- outcome }
}

func wait(ended <-chan Outcome, stop <-chan struct{}) Outcome {
	signals := make(chan os.Signal, 1)
	// A remote daemon started over SSH gets a hangup when the connection drops.
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	select {
	case outcome := <-ended:
		return outcome
	case <-signals:
		return Stopped
	case <-stop:
		return Stopped
	}
}

// readyOut is where the daemon says it is ready: the terminal it runs in, else its log.
func readyOut(mode cli.Mode, log io.Writer) io.Writer {
	if mode == cli.ModeTerminal {
		return os.Stdout
	}
	return log
}

// announce says the daemon is ready: in the terminal, else in the log; the desktop window is told its link.
func announce(settings config.Config, options Options, daemon *app.App, log io.Writer) {
	access := "/?" + protocol.TokenParam + "=" + daemon.Token
	address := settings.PublicURL
	if address == "" {
		address = app.LocalURL(settings.Host, settings.Port)
	}
	network := ""
	for _, link := range daemon.LanURLs {
		network += "\n  On the network:  " + link + access
	}
	ready := fmt.Sprintf("\n  swcode is ready: %s%s%s\n", address, access, network)
	daemon.Logger().Info(fmt.Sprintf("Working in %s, with llm-server at %s", settings.Workspace, settings.LlamaURL))
	link := app.LocalURL(settings.Host, settings.Port) + access
	fmt.Fprintln(readyOut(options.Mode, log), ready)
	if options.Ready != nil {
		options.Ready(link)
	}
}
