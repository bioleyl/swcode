package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"swcode/daemon/protocol"
)

// checkEvery is how often swcode looks for a newer version, while it is told to.
const checkEvery = 10 * time.Minute

const (
	folderMode   = 0o755
	settingsMode = 0o644
	noProgram    = "This swcode cannot replace itself: install the new version with the install command."
	nothingNewer = "There is no newer version to install: look for one first."
	installing   = "The new version is being installed already."
)

// Deps is what the service works with.
type Deps struct {
	// Version is the build that runs: 0.3.0+7bd0cd6, or dev from the sources, which never looks.
	Version string
	// Source is the latest release's address in GitHub's API.
	Source string
	HTTP   *http.Client
	Now    func() time.Time
	// File keeps whether to look every 10 minutes.
	File string
	// Program is what an install replaces.
	Program Program
	// Restart is how an install applies: the daemon restarts by itself, or the person restarts it.
	Restart  protocol.Restart
	LogError func(error)
}

// settings is what File keeps.
type settings struct {
	Checks bool `json:"checks"`
}

// Service looks for a newer version every 10 minutes while it is told to, and installs the one it found.
type Service struct {
	deps   Deps
	source *source
	// One look at a time, and one install.
	looking    sync.Mutex
	installing sync.Mutex
	// Told when looking is turned on: it looks at once.
	wake chan struct{}

	mu     sync.Mutex
	checks bool
	state  protocol.UpdateState
	// The newer release the state offers, with its files.
	found Release
}

// NewService is the service of the build that runs, looking or not as its file keeps.
func NewService(deps Deps) *Service {
	return &Service{
		checks: readChecks(deps.File), deps: deps, source: newSource(deps.Source, deps.HTTP, deps.Version),
		state: initialState(deps.Version), wake: make(chan struct{}, 1),
	}
}

func initialState(version string) protocol.UpdateState {
	if fromSources(version) {
		return protocol.UpdateState{Value: protocol.UpdateStateSource{}}
	}
	return protocol.UpdateState{Value: protocol.UpdateStateUnchecked{}}
}

// readChecks is whether to look every 10 minutes: on until the person turns it off, so a file that cannot be read
// keeps it on.
func readChecks(file string) bool {
	kept := settings{Checks: true}
	data, err := os.ReadFile(file)
	if err != nil {
		return true
	}
	_ = json.Unmarshal(data, &kept)
	return kept.Checks
}

// Watch looks now, then every 10 minutes while it is told to, until ctx ends; turned on, it looks at once. A build
// from the sources never looks.
func (s *Service) Watch(ctx context.Context) {
	if fromSources(s.deps.Version) {
		return
	}
	removeLeftover(s.deps.Program.Path)
	ticker := time.NewTicker(checkEvery)
	defer ticker.Stop()
	for {
		s.lookIfOn(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		}
	}
}

func (s *Service) lookIfOn(ctx context.Context) {
	if !s.View().Checks {
		return
	}
	s.Check(ctx)
}

// Check looks for a newer version now, and says what it found. A build from the sources never looks, and a newer
// version installed waits for the restart: nothing to look for until then.
func (s *Service) Check(ctx context.Context) protocol.UpdateView {
	s.looking.Lock()
	defer s.looking.Unlock()
	if fromSources(s.deps.Version) || s.installed() {
		return s.View()
	}
	release, err := s.source.latest(ctx)
	s.mu.Lock()
	s.state = s.stateAfter(release, err)
	s.mu.Unlock()
	return s.View()
}

func (s *Service) installed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, installed := s.state.Value.(protocol.UpdateStateInstalled)
	return installed
}

// stateAfter is what a look found, with s.mu held. An install done meanwhile stays.
func (s *Service) stateAfter(release Release, err error) protocol.UpdateState {
	at := protocol.Timestamp(s.deps.Now())
	if _, installed := s.state.Value.(protocol.UpdateStateInstalled); installed {
		return s.state
	}
	if err != nil {
		return s.failed(at, err)
	}
	if !Newer(release.Version, s.deps.Version) {
		return protocol.UpdateState{Value: protocol.UpdateStateCurrent{At: at}}
	}
	s.found = release
	return protocol.UpdateState{Value: protocol.UpdateStateNewer{At: at, Release: release.public()}}
}

// failed is a look that failed, with s.mu held: a newer version found before stays on offer.
func (s *Service) failed(at string, err error) protocol.UpdateState {
	s.deps.LogError(err)
	if _, offered := s.state.Value.(protocol.UpdateStateNewer); offered {
		return s.state
	}
	return protocol.UpdateState{Value: protocol.UpdateStateFailed{At: at, Problem: err.Error()}}
}

// View is what the settings' Updates topic shows.
func (s *Service) View() protocol.UpdateView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return protocol.UpdateView{Checks: s.checks, Restart: s.deps.Restart, State: s.state, Version: s.deps.Version}
}

// Offer is the newer version found and not installed yet, for the app to offer: nothing when there is none.
func (s *Service) Offer() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, offered := s.state.Value.(protocol.UpdateStateNewer); !offered {
		return ""
	}
	return s.found.Version
}

// SetChecks turns looking every 10 minutes on or off, kept for the next starts; turned on, it looks at once.
func (s *Service) SetChecks(on bool) error {
	data, err := json.Marshal(settings{Checks: on})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.deps.File), folderMode); err != nil {
		return err
	}
	if err := os.WriteFile(s.deps.File, data, settingsMode); err != nil {
		return err
	}
	s.mu.Lock()
	s.checks = on
	s.mu.Unlock()
	if on {
		s.wakeUp()
	}
	return nil
}

func (s *Service) wakeUp() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Install puts the newer version found in place of the program that runs, once checked against the release's
// checksums, and gives its version: it runs once the program starts again.
func (s *Service) Install(ctx context.Context) (string, error) {
	if s.deps.Program.Path == "" {
		return "", errors.New(noProgram)
	}
	if !s.installing.TryLock() {
		return "", errors.New(installing)
	}
	defer s.installing.Unlock()
	release, offered := s.offered()
	if !offered {
		return "", errors.New(nothingNewer)
	}
	if err := install(ctx, s.deps.HTTP, release, s.deps.Program); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.state = protocol.UpdateState{Value: protocol.UpdateStateInstalled{Release: release.public()}}
	s.mu.Unlock()
	return release.Version, nil
}

func (s *Service) offered() (Release, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, offered := s.state.Value.(protocol.UpdateStateNewer)
	return s.found, offered
}
