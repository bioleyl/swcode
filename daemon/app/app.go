// Package app puts the daemon together: its hosts, llm-server, the tools, the sessions and projects, and the
// routes the web app calls.
package app

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"swcode"
	"swcode/daemon/browser"
	"swcode/daemon/catalog"
	"swcode/daemon/checkpoints"
	"swcode/daemon/config"
	"swcode/daemon/hosts"
	"swcode/daemon/llm"
	"swcode/daemon/models"
	"swcode/daemon/pairing"
	"swcode/daemon/projects"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	"swcode/daemon/swcodebase"
	"swcode/daemon/terminals"
	"swcode/daemon/tools"
	"swcode/daemon/update"
	"swcode/daemon/version"
	"swcode/daemon/web"
)

const (
	sessionsFolder = "sessions"
	scratchFolder  = "scratch"
	// checkpointsFolder is the files turns wrote or edited, as they were before, a folder per session and turn.
	checkpointsFolder = "checkpoints"
	// imagesFolder is the pictures the agents made, as the app shows them: a folder per session.
	imagesFolder = "images"
	// helpersFolder is the programs the daemon's commands run: the password prompt sudo -A and ssh ask with.
	helpersFolder = "helpers"
	// devicesFile is the paired phones, their tokens kept as hashes.
	devicesFile  = "devices.json"
	projectsFile = "projects.json"
	// updatesFile keeps whether to look for a newer version every 10 minutes.
	updatesFile   = "updates.json"
	defaultWebDir = "dist/web"
)

// Options is what a daemon may be given instead of what it finds by itself: the tests' stand-ins, and how it stops.
type Options struct {
	// Browser draws screenshots; FindBrowser is called when it is nil, and none when that is nil too.
	Browser     browser.Browser
	FindBrowser func(home string) browser.Browser
	HTTP        *http.Client
	Now         func() time.Time
	NewID       func() string
	// ConnectHost is how other hosts are reached: through their bash otherwise.
	ConnectHost func(protocol.HostRef) hosts.Host
	// MaxSteps, when set, stands in for the setting (the tests give one).
	MaxSteps   int
	Interfaces func() []Interface
	// OnStop is what POST /api/stop does: the daemon shuts down. Without it, the route is not there.
	OnStop func()
	// OnRestart is what PUT /api/settings does once the settings are saved: the daemon restarts. Without it, the
	// person does.
	OnRestart func()
	// Environment is the daemon's own, which comes on top of the settings file: the page cannot change those.
	Environment map[string]string
	// Defaults is what settings fall back on, to check new ones.
	Defaults config.Defaults
	// Home is this computer's home folder, where the person's skills are (~/.agents/skills): a test's own, without
	// the person's.
	Home string
	// Log writes the daemon's log; nothing for the tests.
	Log io.Writer
	// Busy tells whether a session works: settings restart the daemon, so they wait.
	Busy func() bool
	// Startup starts the daemon with its computer: only the Windows app has one.
	Startup StartupPort
	// Program is what installing a newer version replaces: the program the daemon runs in.
	Program update.Program
	// OnUpdated is what installing a newer version does once it is in place: the program starts again on it.
	// Without it, the person restarts it.
	OnUpdated func()
	// Version stands in for the build's (the tests give a release's).
	Version string
}

// App is a daemon, ready to serve.
type App struct {
	Handler  http.Handler
	Sessions *sessions.Service
	Projects *projects.Service
	// Models is the llm-servers and the presets sessions pick from them.
	Models *models.Service
	// Updates looks for a newer version every 10 minutes, and installs it.
	Updates *update.Service
	// SwcodeBase is where the swcode-base skill is, and the skills CLI that installs it.
	SwcodeBase *swcodebase.Service
	// Terminals are the shells people type in from the app, in their projects' folders.
	Terminals *terminals.Service
	Token     string
	LanURLs   []string

	config       config.Config
	options      Options
	workspace    string
	imagesFolder string
	http         *http.Client
	hosts        *hosts.Registry
	pairing      *pairing.Pairing
	// deviceSockets is the sockets each paired phone has open, ended with it when it is forgotten.
	deviceSockets *deviceSockets
	logger        *slog.Logger
	stopWatching  context.CancelFunc
}

// NewID is a random UUID, as sessions and projects are named.
func NewID() string {
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	id[6] = id[6]&0x0f | 0x40
	id[8] = id[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:])
}

func (o *Options) fill() {
	if o.HTTP == nil {
		o.HTTP = http.DefaultClient
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.NewID == nil {
		o.NewID = NewID
	}
	if o.ConnectHost == nil {
		o.ConnectHost = hosts.RemoteHost
	}
	if o.Interfaces == nil {
		o.Interfaces = SystemInterfaces
	}
	if o.Home == "" {
		o.Home, _ = os.UserHomeDir()
	}
	if o.Log == nil {
		o.Log = io.Discard
	}
	if o.Version == "" {
		o.Version = version.Build
	}
}

func localShell(environment map[string]string) *hosts.Shell {
	if runtime.GOOS != "windows" {
		return nil
	}
	shell := hosts.WindowsShell(environment)
	return &shell
}

func (o Options) browser() browser.Browser {
	if o.Browser != nil || o.FindBrowser == nil {
		return o.Browser
	}
	return o.FindBrowser(o.Home)
}

// New puts a daemon together on its settings.
func New(settings config.Config, options Options) (*App, error) {
	options.fill()
	token, err := AccessToken(settings.DataDir, settings.Token)
	if err != nil {
		return nil, err
	}
	workspace, err := filepath.Abs(settings.Workspace)
	if err != nil {
		return nil, err
	}
	logger := slog.New(slog.NewTextHandler(options.Log, nil))
	logError := func(err error) { logger.Error(err.Error()) }
	servers := llm.NewServers(options.HTTP)
	modelService, err := models.NewService(models.NewStore(filepath.Join(settings.DataDir, models.File)), servers, models.FromConfig(settings))
	if err != nil {
		return nil, err
	}
	local := hosts.NewLocalHost(hosts.LocalOptions{
		Helpers: filepath.Join(settings.DataDir, helpersFolder), Home: options.Home,
		ScratchRoot: filepath.Join(settings.DataDir, scratchFolder), Shell: localShell(options.Environment),
	})
	registry := hosts.NewRegistry(local, options.ConnectHost)
	terminalService := terminals.NewService(terminals.Deps{Hosts: registry, NewID: options.NewID})
	found := options.browser()
	mainTools, exploreTools, chatTools := tools.Boxes(tools.Servers{Browser: found != nil, Images: settings.ImageURL != ""})
	app := &App{
		LanURLs: LANURLs(settings.Host, settings.Port, options.Interfaces()), Token: token,
		Models: modelService, config: settings, hosts: registry, http: options.HTTP,
		SwcodeBase:   swcodebase.NewService(registry, logger),
		Terminals:    terminalService,
		imagesFolder: filepath.Join(settings.DataDir, imagesFolder), logger: logger, options: options, workspace: workspace,
		pairing:       pairing.New(filepath.Join(settings.DataDir, devicesFile), options.Now, options.NewID),
		deviceSockets: newDeviceSockets(),
	}
	var service *sessions.Service
	app.Projects = projects.NewService(projects.Deps{
		ForgetSessions: func(path string, host protocol.HostRef) string { return service.RemoveIn(path, host) },
		HostOptions: func() []protocol.HostOption {
			return hosts.Options(runtime.GOOS, options.Home, settings.SSHEndpoints)
		},
		Hosts: registry, LogError: logError, NewID: options.NewID, Now: options.Now, ProjectsFolder: workspace,
		Store: projects.NewStore(filepath.Join(settings.DataDir, projectsFile)),
	})
	app.Projects.Load()
	maxSteps := settings.MaxSteps
	if options.MaxSteps != 0 {
		maxSteps = options.MaxSteps
	}
	service = sessions.NewService(sessions.Deps{
		Browser: found, HTTP: options.HTTP, Hosts: registry, ImageURL: settings.ImageURL,
		ImagesFolder: app.imagesFolder, LogError: logError, MaxSteps: maxSteps, MaxSubagentSteps: settings.SubagentMaxSteps,
		Model: servers, Models: modelService, NewID: options.NewID, Now: options.Now,
		OnWorkspace: func(path string, host protocol.HostRef) { app.Projects.Ensure(path, host) },
		Search: web.Paced(func(ctx context.Context, query string) web.SearchOutcome {
			return web.SearchWeb(ctx, query, options.HTTP)
		}, web.SearchGap),
		Skills: catalog.New(logError), SkillsOff: func(path string, host protocol.HostRef) []string {
			project, _ := app.Projects.Find(path, host)
			return project.SkillsOff
		},
		Store:       sessions.NewStore(filepath.Join(settings.DataDir, sessionsFolder)),
		Checkpoints: checkpoints.NewStore(filepath.Join(settings.DataDir, checkpointsFolder), logError),
		ChatTools:   chatTools, SubagentTools: exploreTools, Thinking: sessions.Thinking{Session: settings.Thinking, Subagents: settings.SubagentThinking},
		Terminals: terminalService, Todos: app.Projects, Tools: mainTools, Workspace: workspace,
	})
	app.Sessions = service
	service.Load()
	// Sessions from before projects, or whose project was lost, get one; a chat works in no folder.
	for _, session := range service.List() {
		if session.Kind == protocol.SessionKindChat {
			continue
		}
		app.Projects.Ensure(session.Workspace, session.Host)
	}
	if app.options.Busy == nil {
		app.options.Busy = func() bool { return anyWorking(service.List()) }
	}
	app.watchUpdates(settings, logError)
	app.Handler = app.routes()
	return app, nil
}

// watchUpdates looks for a newer version now and every 10 minutes, until the daemon closes.
func (a *App) watchUpdates(settings config.Config, logError func(error)) {
	a.Updates = update.NewService(update.Deps{
		File: filepath.Join(settings.DataDir, updatesFile), HTTP: a.options.HTTP, LogError: logError,
		Now: a.options.Now, Program: a.options.Program, Restart: restartOf(a.options.OnUpdated),
		Source: settings.ReleasesURL, Version: a.options.Version,
	})
	watching, stop := context.WithCancel(context.Background())
	a.stopWatching = stop
	go a.Updates.Watch(watching)
}

func anyWorking(summaries []protocol.SessionSummary) bool {
	for _, summary := range summaries {
		if summary.Status != protocol.SessionStatusIdle {
			return true
		}
	}
	return false
}

func (a *App) webFiles() fs.FS {
	if a.config.WebDir != "" {
		return os.DirFS(a.config.WebDir)
	}
	if embedded := swcode.WebApp(); embedded != nil {
		return embedded
	}
	if _, err := os.Stat(filepath.Join(defaultWebDir, indexPage)); err == nil {
		return os.DirFS(defaultWebDir)
	}
	return nil
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	a.statusRoutes(mux)
	a.imageRoutes(mux)
	a.pairingRoutes(mux)
	a.settingsRoutes(mux)
	a.modelRoutes(mux)
	a.skillRoutes(mux)
	a.closingRoutes(mux)
	a.startupRoutes(mux)
	a.updateRoutes(mux)
	mux.HandleFunc("GET "+protocol.EndpointSocket, a.socket)
	mux.Handle("/", webApp(a.webFiles()))
	return withAuth(mux, a.Token, a.pairing)
}

// Close stops every session's turn and lets go of the hosts, when the daemon stops.
func (a *App) Close() {
	a.stopWatching()
	a.Sessions.StopAll()
	a.Terminals.CloseAll()
	a.hosts.Close()
}

// Logger is the daemon's log.
func (a *App) Logger() *slog.Logger { return a.logger }
