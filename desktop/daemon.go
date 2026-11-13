//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	daemonapp "swcode/daemon/app"
	"swcode/daemon/cli"
	"swcode/daemon/config"
	"swcode/daemon/local"
	"swcode/daemon/protocol"
	"swcode/daemon/serve"
	"swcode/daemon/update"
	"swcode/desktop/selfinstall"
)

const (
	// The server listens in a second or two; a daemon from before to stop first takes longer.
	startLimit = 60 * time.Second
	// Stopping waits for the sessions to close, up to the daemon's own shutdown deadline.
	stopLimit   = 15 * time.Second
	answerLimit = 2 * time.Second
	bearer      = "Bearer "
	// portTaken: another swcode answers on the port, most likely one in WSL, whose ports Windows reaches at localhost.
	portTaken = "Another swcode answers on port %d, with an access token of its own: most likely one running in WSL, " +
		"whose ports Windows reaches at localhost.\n\nStop it, or give one of them another port (SWCODE_PORT in its " +
		"settings), then open swcode again."
)

// daemon is swcode's server, run inside this program: started with it, stopped with it, started again on new
// settings. A job holds this program and the commands the agent starts, so none outlives it; what should (a browser
// a link opens) is started from outside it (startOutside).
type daemon struct {
	stopping atomic.Bool
	startup  startupEntry
	// exe is this program: an update puts its newer version in its place.
	exe string
}

// daemonRun is one run of the server, once it listens: run inside this program, or the daemon alone (--daemon),
// started before the app, which then only shows it.
type daemonRun struct {
	link     *url.URL
	attached bool
	stop     chan struct{}
	once     sync.Once
	// done is closed once the run ended: outcome and err then say how.
	done    chan struct{}
	outcome serve.Outcome
	err     error
}

func newDaemon(exe string) (*daemon, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	// No breakaway: Git Bash's runtime starts every process out of a job that allows it.
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits)),
	)
	if err != nil {
		return nil, err
	}
	// The job's handle closes with this program, however it ends: Windows then ends every process in the job.
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		return nil, err
	}
	return &daemon{exe: exe, startup: startupEntry{exe: exe}}, nil
}

// loadSettings is the settings saved now, the environment's on top, and that environment.
func loadSettings() (config.Config, map[string]string, error) {
	environment := config.ProcessEnvironment()
	home := homeFolder()
	if err := config.ListenAsBefore(environment, home); err != nil {
		return config.Config{}, environment, err
	}
	settings, err := config.Load(config.Environment(environment, home), config.Defaults{Cwd: home, Home: home})
	return settings, environment, err
}

// local is this computer's server for the window: the daemon alone when it runs already, else one run here.
func (d *daemon) local() (*daemonRun, error) {
	running, found, err := d.attach()
	if err != nil {
		return nil, err
	}
	if found {
		return running, nil
	}
	return d.start()
}

// attach finds the daemon alone (--daemon), which answers on this computer's port with its token. Another swcode
// answering there is an error: the window would show a page that has no token for it.
func (d *daemon) attach() (*daemonRun, bool, error) {
	settings, _, err := loadSettings()
	if err != nil {
		return nil, false, nil
	}
	token, err := daemonapp.AccessToken(settings.DataDir, settings.Token)
	control := daemonapp.ControlURL(settings.Host, settings.Port)
	if err != nil || !local.IsUp(control) {
		return nil, false, nil
	}
	if !local.Accepts(control, token) {
		return nil, false, fmt.Errorf(portTaken, settings.Port)
	}
	link, err := url.Parse(daemonapp.LocalURL(settings.Host, settings.Port) + "/?" + protocol.TokenParam + "=" + url.QueryEscape(token))
	if err != nil {
		return nil, false, nil
	}
	return &daemonRun{attached: true, done: make(chan struct{}), link: link, stop: make(chan struct{})}, true, nil
}

// start runs the server on the settings saved now, and waits for its link.
func (d *daemon) start() (*daemonRun, error) {
	settings, environment, err := loadSettings()
	if err != nil {
		return nil, err
	}
	home := homeFolder()
	links := make(chan string, 1)
	run := &daemonRun{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(run.done)
		run.outcome, run.err = serve.Run(settings, serve.Options{
			Cwd:         home,
			Environment: environment,
			Home:        home,
			LogFile:     local.LogFileOf(settings),
			Mode:        cli.ModeDesktop,
			Program:     update.Program{File: selfinstall.ProgramName, Path: d.exe},
			Ready:       func(link string) { links <- link },
			Startup:     d.startup,
			Stop:        run.stop,
		})
	}()
	select {
	case line := <-links:
		link, err := url.Parse(line)
		if err != nil {
			run.end()
			return nil, fmt.Errorf("the server gave a link that is none: %q", line)
		}
		run.link = link
		return run, nil
	case <-run.done:
		return nil, fmt.Errorf("the server ended before it listened: %v", run.err)
	case <-time.After(startLimit):
		run.end()
		return nil, fmt.Errorf("the server did not listen within %s; its log is %s", startLimit, local.LogFileOf(settings))
	}
}

// watch tells the window when a run ends by itself: not when the window stopped it. The daemon alone ends apart.
func (d *daemon) watch(run *daemonRun, hwnd uintptr, message uint32) {
	if run.attached {
		return
	}
	go func() {
		<-run.done
		if d.stopping.Load() {
			return
		}
		postMessage(hwnd, message, 0)
	}()
}

// stop stops the server run here as on Ctrl+C, and waits for it, up to a limit: the daemon alone keeps running.
func (d *daemon) stop(run *daemonRun) {
	if run.attached {
		return
	}
	d.stopping.Store(true)
	run.end()
	select {
	case <-run.done:
	case <-time.After(stopLimit):
	}
}

func (r *daemonRun) end() { r.once.Do(func() { close(r.stop) }) }

// address is where the run answers, without its token: the base of the window's links.
func (r *daemonRun) address() string {
	base := url.URL{Host: r.link.Host, Scheme: r.link.Scheme}
	return base.String()
}

func (r *daemonRun) token() string { return r.link.Query().Get(protocol.TokenParam) }

// restarts tells whether the run ended for new settings, to start again on them.
func (r *daemonRun) restarts() bool {
	<-r.done
	return r.err == nil && r.outcome == serve.Restart
}

// updated tells whether the run ended for a newer version, put in place of this program: the new one starts then.
func (r *daemonRun) updated() bool {
	<-r.done
	return r.err == nil && r.outcome == serve.Updated
}

// problem is why the run ended, when it failed.
func (r *daemonRun) problem() error {
	<-r.done
	return r.err
}

// closing is the daemon's answer to GET /api/closing: what closing the window would stop.
type closing struct {
	Idle    bool   `json:"idle"`
	Warning string `json:"warning"`
}

// closingWarning says what closing the window would stop: nothing when nothing runs, or the daemon does not answer.
func (r *daemonRun) closingWarning() string {
	body, err := r.call(http.MethodGet, protocol.EndpointClosing)
	if err != nil {
		return ""
	}
	answer := closing{}
	if json.Unmarshal(body, &answer) != nil || answer.Idle {
		return ""
	}
	return answer.Warning
}

// call sends a request to the daemon, with the token of its link.
func (r *daemonRun) call(method, path string) ([]byte, error) {
	address := url.URL{Scheme: r.link.Scheme, Host: r.link.Host, Path: path}
	request, err := http.NewRequest(method, address.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", bearer+r.link.Query().Get(protocol.TokenParam))
	client := http.Client{Timeout: answerLimit}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New(response.Status)
	}
	var body bytes.Buffer
	_, err = body.ReadFrom(response.Body)
	return body.Bytes(), err
}

func homeFolder() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
