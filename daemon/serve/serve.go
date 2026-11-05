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
)

// Outcome is how the daemon ended: stopped, or to start again on new settings.
type Outcome int

const (
	Stopped Outcome = iota
	Restart
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
}

func logOf(options Options) (io.Writer, func(), error) {
	if options.Mode == cli.ModeTerminal || options.LogFile == "" {
		return os.Stderr, func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(options.LogFile), 0o755); err != nil {
		return nil, nil, err
	}
	file, err := os.Create(options.LogFile)
	if err != nil {
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
		OnRestart:   restartOf(options, ended),
		OnStop:      func() { ended <- Stopped },
		Startup:     options.Startup,
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
	server := &http.Server{BaseContext: func(net.Listener) context.Context { return base }, Handler: daemon.Handler}
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

// restartOf is what new settings do once the daemon closed: in a terminal, the person restarts it.
func restartOf(options Options, ended chan Outcome) func() {
	if options.Mode == cli.ModeTerminal {
		return nil
	}
	return func() { ended <- Restart }
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
