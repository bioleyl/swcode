package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const (
	desktopWidth  = 1440
	desktopHeight = 900

	navigationTimeout = 30 * time.Second
	actionTimeout     = 10 * time.Second
	elementTimeout    = 5 * time.Second
	probeTimeout      = 2 * time.Second
	cleanupTimeout    = 5 * time.Second
	settleAfterAction = 300 * time.Millisecond

	noBrowserFound = "No Chromium, Chrome or Edge was found: install one, set SWCODE_CHROMIUM to its program, or run npm run e2e:install."
	noPageOpened   = "No page is open in the browser: open one first with browser_open."
	browserGone    = "The browser disconnected: open the page again with browser_open."
)

var refPattern = regexp.MustCompile(`^e[1-9][0-9]*$`)

// DesktopViewport is the window a browser session opens its page in.
var DesktopViewport = Viewport{Height: desktopHeight, Width: desktopWidth}

// Sessions keeps one Chromium per agent session, started at its first browser tool and closed with the session, so
// cookies and the application's state last across the session's calls. Each browser has a profile folder of its
// own, removed when it closes.
type Sessions struct {
	bin      string
	folder   string
	headless bool
	mu       sync.Mutex
	live     map[string]*Session
}

// NewSessions keeps the browsers of the agent sessions under folder, started with the Chromium program at bin.
// headful shows a window, for a person watching the agent.
func NewSessions(bin, folder string, headful bool) *Sessions {
	return &Sessions{bin: bin, folder: folder, headless: !headful, live: map[string]*Session{}}
}

// For is a session's browser, made at its first call.
func (s *Sessions) For(sessionID string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if found, has := s.live[sessionID]; has {
		return found
	}
	opened := &Session{diagnostics: newDiagnostics(), manager: s, sessionID: sessionID}
	s.live[sessionID] = opened
	return opened
}

// Close ends a session's browser and removes its profile.
func (s *Sessions) Close(sessionID string) {
	s.mu.Lock()
	found, has := s.live[sessionID]
	delete(s.live, sessionID)
	s.mu.Unlock()
	if has {
		found.Close()
	}
}

// CloseAll ends every browser, when the daemon stops.
func (s *Sessions) CloseAll() {
	s.mu.Lock()
	opened := make([]*Session, 0, len(s.live))
	for _, found := range s.live {
		opened = append(opened, found)
	}
	s.live = map[string]*Session{}
	s.mu.Unlock()
	for _, found := range opened {
		found.Close()
	}
}

// Session is one agent session's browser: a page that stays open across the agent's calls.
type Session struct {
	mu          sync.Mutex
	manager     *Sessions
	sessionID   string
	browser     *rod.Browser
	page        *rod.Page
	launcher    *launcher.Launcher
	cancel      context.CancelFunc
	closed      bool
	diagnostics *diagnostics
}

// Open loads a page in the session's browser, and waits for it to load.
func (s *Session) Open(ctx context.Context, url string) (OpenResult, error) {
	page, err := s.ready(ctx)
	if err != nil {
		return OpenResult{}, err
	}
	started := time.Now()
	timed := page.Context(ctx).Timeout(navigationTimeout)
	if err := timed.Navigate(url); err != nil {
		return OpenResult{}, fmt.Errorf("%s could not be opened: %w", url, err)
	}
	if err := timed.WaitLoad(); err != nil {
		return OpenResult{}, fmt.Errorf("%s opened but did not finish loading: %w", url, err)
	}
	if err := pause(ctx, settleAfterAction); err != nil {
		return OpenResult{}, err
	}
	return resultOf(page, started), nil
}

// OpenResult is a page opened and the page it settled on.
type OpenResult struct {
	URL          string
	Title        string
	Milliseconds int
}

// Snapshot is the page as the agent reads it, narrowed to the controls whose text or role matches when asked.
func (s *Session) Snapshot(ctx context.Context, query, role string) (Snapshot, error) {
	page, err := s.openedPage()
	if err != nil {
		return Snapshot{}, err
	}
	found, err := page.Context(ctx).Timeout(actionTimeout).Eval(snapshotScript, query, role)
	if err != nil {
		return Snapshot{}, fmt.Errorf("the page could not be read: %w", err)
	}
	var snapshot Snapshot
	if err := found.Value.Unmarshal(&snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("the page gave no readable snapshot: %w", err)
	}
	return snapshot, nil
}

// Inspect is what an element is, before a call acts on it: the target is a ref from browser_snapshot or a CSS
// selector.
func (s *Session) Inspect(ctx context.Context, target string) (Target, error) {
	_, found, err := s.element(ctx, target)
	if err != nil {
		return Target{}, err
	}
	return found, nil
}

// Click clicks an element and answers the page it left behind.
func (s *Session) Click(ctx context.Context, target string) (ActionResult, error) {
	page, err := s.openedPage()
	if err != nil {
		return ActionResult{}, err
	}
	element, found, err := s.element(ctx, target)
	if err != nil {
		return ActionResult{}, err
	}
	if err := clickElement(element, ctx); err != nil {
		return ActionResult{}, fmt.Errorf("%s could not be clicked: %w", describe(target, found), err)
	}
	if err := pause(ctx, settleAfterAction); err != nil {
		return ActionResult{}, err
	}
	// A click that navigates waits for the new page; one that only changes it answers at once.
	_ = page.Context(ctx).Timeout(actionTimeout).WaitLoad()
	return resultOfAction(page), nil
}

// Type fills an input or textarea with text, replacing what it held; on a select it chooses the option that says
// the text.
func (s *Session) Type(ctx context.Context, target, text string) (ActionResult, error) {
	page, err := s.openedPage()
	if err != nil {
		return ActionResult{}, err
	}
	element, found, err := s.element(ctx, target)
	if err != nil {
		return ActionResult{}, err
	}
	switch found.Kind {
	case "select":
		if err := element.Context(ctx).Timeout(actionTimeout).Select([]string{text}, true, rod.SelectorTypeText); err != nil {
			return ActionResult{}, fmt.Errorf("%s has no option that says %q: %w", describe(target, found), text, err)
		}
	default:
		if err := fill(element, text, found.ContentEditable); err != nil {
			return ActionResult{}, fmt.Errorf("%s could not be filled: %w", describe(target, found), err)
		}
	}
	if err := pause(ctx, settleAfterAction); err != nil {
		return ActionResult{}, err
	}
	return resultOfAction(page), nil
}

// Screenshot captures the page's viewport, or its whole length when fullPage is set.
func (s *Session) Screenshot(ctx context.Context, fullPage bool) (Shot, error) {
	page, err := s.openedPage()
	if err != nil {
		return Shot{}, err
	}
	width := DesktopViewport.Width
	if measured, err := page.Context(ctx).Timeout(probeTimeout).Eval(`() => window.innerWidth`); err == nil && measured.Value.Int() > 0 {
		width = measured.Value.Int()
	}
	png, err := screenshot(page.Context(ctx).Timeout(actionTimeout), ShotRequest{
		FullPage: fullPage, Viewport: Viewport{Height: DesktopViewport.Height, Width: width},
	})
	if err != nil {
		return Shot{}, fmt.Errorf("the page could not be captured: %w", err)
	}
	info, _ := page.Info()
	width, height := PNGSize(png)
	title := ""
	if info != nil {
		title = info.Title
	}
	return Shot{Height: height, PNG: png, Title: title, URL: infoURL(info), Width: width}, nil
}

// Diagnostics is what the page has logged since it opened, collected as it happened.
func (s *Session) Diagnostics() Report {
	s.mu.Lock()
	opened := s.page != nil
	s.mu.Unlock()
	return s.diagnostics.report(opened)
}

// Close ends the browser and removes its profile.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.stopLocked()
}

// ready is the page, started when it is not there or its browser has gone.
func (s *Session) ready(ctx context.Context) (*rod.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("This browser session has ended.")
	}
	if s.page != nil && s.healthy(ctx) {
		return s.page, nil
	}
	s.stopLocked()
	if err := s.startLocked(); err != nil {
		return nil, err
	}
	return s.page, nil
}

// openedPage is the page, without starting a browser: a call that acts on a page does not open one.
func (s *Session) openedPage() (*rod.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.page == nil {
		return nil, errors.New(noPageOpened)
	}
	if !s.healthy(context.Background()) {
		s.stopLocked()
		return nil, errors.New(browserGone)
	}
	return s.page, nil
}

func (s *Session) healthy(ctx context.Context) bool {
	probe, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	if _, err := s.browser.Context(probe).Version(); err != nil {
		return false
	}
	_, err := s.page.Info()
	return err == nil
}

func (s *Session) startLocked() error {
	if s.manager.bin == "" {
		return errors.New(noBrowserFound)
	}
	profile, err := s.profileFolder()
	if err != nil {
		return err
	}
	life, cancel := context.WithCancel(context.Background())
	opened, err := launchBrowser(s.manager.bin, profile, s.manager.headless, life)
	if err != nil {
		cancel()
		return err
	}
	if err := s.configure(opened.page); err != nil {
		_ = opened.browser.Close()
		cancel()
		s.waitCleanup(opened.program)
		return err
	}
	s.browser, s.page, s.launcher, s.cancel = opened.browser, opened.page, opened.program, cancel
	return nil
}

// profileFolder is where the session's cookies and storage live, made private.
func (s *Session) profileFolder() (string, error) {
	profile := filepath.Join(s.manager.folder, s.sessionID)
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return "", fmt.Errorf("the browser's profile folder could not be made: %w", err)
	}
	return profile, nil
}

// browserPage is a Chromium started with one page, ready to navigate.
type browserPage struct {
	browser *rod.Browser
	page    *rod.Page
	program *launcher.Launcher
}

// launchBrowser starts a browser of its own, a separate Chromium from the desktop app's WebView. Connecting to an
// existing DevTools endpoint instead would replace the launcher here; the session needs only the browser and page.
func launchBrowser(bin, profile string, headless bool, life context.Context) (browserPage, error) {
	program := launcher.New().
		Bin(bin).
		Headless(headless).
		Leakless(false).
		UserDataDir(profile).
		Context(life)
	if !headless {
		program.Set("window-size", fmt.Sprintf("%d,%d", DesktopViewport.Width, DesktopViewport.Height))
	}
	started, err := program.Launch()
	if err != nil {
		return browserPage{}, fmt.Errorf("the browser could not start: %w", err)
	}
	opened := rod.New().ControlURL(started).Context(life)
	if err := opened.Connect(); err != nil {
		program.Kill()
		_ = os.RemoveAll(profile)
		return browserPage{}, fmt.Errorf("the browser could not start: %w", err)
	}
	page, err := opened.Page(proto.TargetCreateTarget{})
	if err != nil {
		_ = opened.Close()
		program.Kill()
		_ = os.RemoveAll(profile)
		return browserPage{}, fmt.Errorf("the browser could not open a page: %w", err)
	}
	if headless {
		if err := page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
			DeviceScaleFactor: 1, Height: DesktopViewport.Height, Width: DesktopViewport.Width,
		}); err != nil {
			_ = opened.Close()
			program.Kill()
			_ = os.RemoveAll(profile)
			return browserPage{}, fmt.Errorf("the browser could not size its page: %w", err)
		}
	} else if err := (proto.EmulationClearDeviceMetricsOverride{}).Call(page); err != nil {
		// The first target of a window started with --no-startup-window carries a 1280×720 emulated viewport:
		// left on, it ignores the window's size. Clearing it makes the page follow the window.
		_ = opened.Close()
		program.Kill()
		_ = os.RemoveAll(profile)
		return browserPage{}, fmt.Errorf("the browser could not size its page: %w", err)
	}
	return browserPage{browser: opened, page: page, program: program}, nil
}

// configure refuses downloads and starts collecting what the page logs as it happens. A browser that cannot do
// either is no use for testing, so the call fails rather than the page silently going unwatched.
func (s *Session) configure(page *rod.Page) error {
	// A download could run an application or fill the disk without the agent saying so: it is refused, and the
	// attempt is kept among the page's diagnostics.
	if err := (proto.BrowserSetDownloadBehavior{
		Behavior: proto.BrowserSetDownloadBehaviorBehaviorDeny, EventsEnabled: true,
	}).Call(page.Browser()); err != nil {
		return fmt.Errorf("the browser could not refuse downloads: %w", err)
	}
	// EachEvent enables these domains itself, but only once its goroutine runs: the page's first navigation must not
	// race that, or the errors it logs at load would never be collected.
	if err := (proto.RuntimeEnable{}).Call(page); err != nil {
		return fmt.Errorf("the browser could not start watching the page: %w", err)
	}
	if err := (proto.NetworkEnable{}).Call(page); err != nil {
		return fmt.Errorf("the browser could not start watching the page: %w", err)
	}
	if err := (proto.PageEnable{}).Call(page); err != nil {
		return fmt.Errorf("the browser could not start watching the page: %w", err)
	}
	s.watch(page)
	return nil
}

// watch collects what the page logs while the browser lives, not only during a tool call.
func (s *Session) watch(page *rod.Page) {
	d := s.diagnostics
	go page.EachEvent(
		d.console,
		d.exception,
		d.request,
		d.failed,
		d.response,
		func(event *proto.PageJavascriptDialogOpening) {
			d.dialog(event)
			_ = proto.PageHandleJavaScriptDialog{Accept: false}.Call(page)
		},
		func(event *proto.PageDownloadWillBegin) {
			d.download(event)
			_ = proto.BrowserCancelDownload{GUID: event.GUID}.Call(page.Browser())
		},
	)()
}

func (s *Session) stopLocked() {
	if s.page != nil {
		_ = s.page.Close()
	}
	if s.browser != nil {
		_ = s.browser.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.launcher != nil {
		s.waitCleanup(s.launcher)
	}
	s.page, s.browser, s.launcher, s.cancel = nil, nil, nil, nil
}

// waitCleanup waits for the browser to exit and its profile to go, but not forever: a stuck process must not keep
// a deleted session from going.
func (s *Session) waitCleanup(program *launcher.Launcher) {
	done := make(chan struct{})
	go func() {
		program.Cleanup()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(cleanupTimeout):
		program.Kill()
	}
}

func (s *Session) element(ctx context.Context, target string) (*rod.Element, Target, error) {
	page, err := s.openedPage()
	if err != nil {
		return nil, Target{}, err
	}
	element, err := elementOf(ctx, page, target)
	if err != nil {
		return nil, Target{}, err
	}
	found, err := element.Context(ctx).Timeout(elementTimeout).Eval(inspectScript)
	if err != nil {
		return nil, Target{}, fmt.Errorf("%s could not be read: %w", target, err)
	}
	var info Target
	if err := found.Value.Unmarshal(&info); err != nil {
		return nil, Target{}, fmt.Errorf("%s gave no readable element: %w", target, err)
	}
	if info.Kind == "" {
		return nil, Target{}, fmt.Errorf("The control %q is gone: run browser_snapshot again.", target)
	}
	return element, info, nil
}

// elementOf is the element a ref or a CSS selector names. A ref is a live element kept by the snapshot, so it can
// be inside an open shadow root, which a document selector cannot reach.
func elementOf(ctx context.Context, page *rod.Page, target string) (*rod.Element, error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return nil, errors.New("Give an element ref from browser_snapshot (e1, e2…) or a CSS selector.")
	}
	if !refPattern.MatchString(trimmed) {
		element, err := page.Context(ctx).Timeout(elementTimeout).Element(trimmed)
		if err != nil {
			return nil, fmt.Errorf("No element matches %q. Run browser_snapshot with query to find the control’s ref, or pass a CSS selector.", target)
		}
		return element, nil
	}
	known, err := page.Context(ctx).Timeout(elementTimeout).Eval(
		`(ref) => !!(window.__swcodeRefs && window.__swcodeRefs.get(ref))`, trimmed)
	if err != nil || !known.Value.Bool() {
		return nil, fmt.Errorf("The ref %q is gone: run browser_snapshot again to get the control’s new ref.", trimmed)
	}
	element, err := page.Context(ctx).Timeout(elementTimeout).ElementByJS(
		rod.Eval(`(ref) => (window.__swcodeRefs && window.__swcodeRefs.get(ref)) || null`, trimmed))
	if err != nil {
		return nil, fmt.Errorf("The ref %q could not be reached: run browser_snapshot again.", trimmed)
	}
	return element, nil
}

// clickElement clicks with the mouse, except inside an open shadow root: the mouse point there reaches the host,
// not the control, so the control's own click() is what fires its handlers.
func clickElement(element *rod.Element, ctx context.Context) error {
	inShadow, err := element.Context(ctx).Timeout(elementTimeout).Eval(`() => this.getRootNode() !== document`)
	if err != nil {
		return err
	}
	if !inShadow.Value.Bool() {
		return element.Context(ctx).Timeout(actionTimeout).Click(proto.InputMouseButtonLeft, 1)
	}
	_, err = element.Context(ctx).Timeout(actionTimeout).Eval(
		`() => { this.scrollIntoView({block: 'center'}); this.focus(); this.click(); }`)
	return err
}

// fill replaces what a field holds and types the text into it, as a person would: the events applications listen
// to fire. A contenteditable has no select(), so its old text goes with the keyboard.
func fill(element *rod.Element, text string, contentEditable bool) error {
	if !contentEditable {
		if err := element.SelectAllText(); err != nil {
			return err
		}
		return element.Input(text)
	}
	if err := clearEditable(element); err != nil {
		return err
	}
	return element.Input(text)
}

func clearEditable(element *rod.Element) error {
	if err := element.Focus(); err != nil {
		return err
	}
	modifier := input.ControlLeft
	if input.IsMac {
		modifier = input.MetaLeft
	}
	if err := withModifier(element.Page(), modifier, input.KeyA); err != nil {
		return err
	}
	page := element.Page()
	if err := page.Keyboard.Press(input.Backspace); err != nil {
		return err
	}
	return page.Keyboard.Release(input.Backspace)
}

func withModifier(page *rod.Page, modifier, key input.Key) (err error) {
	if err = page.Keyboard.Press(modifier); err != nil {
		return err
	}
	if err = page.Keyboard.Press(key); err == nil {
		err = page.Keyboard.Release(key)
	}
	if released := page.Keyboard.Release(modifier); err == nil {
		err = released
	}
	return err
}

func resultOf(page *rod.Page, started time.Time) OpenResult {
	info, _ := page.Info()
	title := ""
	if info != nil {
		title = info.Title
	}
	return OpenResult{Milliseconds: int(time.Since(started).Milliseconds()), Title: title, URL: infoURL(info)}
}

func resultOfAction(page *rod.Page) ActionResult {
	info, _ := page.Info()
	title := ""
	if info != nil {
		title = info.Title
	}
	return ActionResult{Title: title, URL: infoURL(info)}
}

func infoURL(info *proto.TargetTargetInfo) string {
	if info == nil {
		return ""
	}
	return info.URL
}

// ActionResult is where the page stands after a click or a typed text.
type ActionResult struct {
	URL   string
	Title string
}

func describe(target string, found Target) string {
	if found.Name == "" {
		return target
	}
	return fmt.Sprintf("%s %q", target, found.Name)
}

func pause(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
