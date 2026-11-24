package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const (
	// actionSettle is what a page gets after an action, for its menu or its request to show.
	actionSettle = 300 * time.Millisecond
	// actionTimeout bounds one action: a page that never answers fails the call.
	actionTimeout = 20 * time.Second
	// maxElements is how many interactive elements one snapshot lists: enough to act on, few enough to read.
	maxElements = 120
	// maxNameLength is how much of an element's name a snapshot keeps.
	maxNameLength = 80
	// noPageOpen is the error of an action before a page was opened.
	noPageOpen = "No page is open: open one first."
	// frameQuality is how sharp a page is in the live view: small enough to send a few times a second.
	frameQuality = 60
	// readyStateComplete is a document that finished loading.
	readyStateComplete = "complete"
	// headlessEnv asks for a browser with no window: set to 1 on a computer whose screen should stay free, or where
	// there is none.
	headlessEnv = "SWCODE_BROWSER_HEADLESS"
	// windowSize is the window a browser visible to its person opens at: a little taller than the page's 1440 × 900
	// viewport, for the browser's own bars.
	windowSize = "1440,1000"
	// fillOK, fillMissing and fillOption are what the page answers a fill with.
	fillOK      = "ok"
	fillMissing = "missing"
	fillOption  = "option"
)

// candidatesJS is the interactive elements of a page, in document order, visible ones only: the numbering a
// snapshot lists and an action picks from.
const candidatesJS = `(() => {
	const selector = 'a[href],button,input,textarea,select,summary,[role="button"],[role="link"],[role="menuitem"],[role="menuitemcheckbox"],[role="menuitemradio"],[role="tab"],[role="checkbox"],[role="radio"],[role="switch"],[role="combobox"],[role="textbox"],[role="searchbox"],[contenteditable="true"]';
	return Array.from(document.querySelectorAll(selector)).filter((element) => {
		const box = element.getBoundingClientRect();
		const style = getComputedStyle(element);
		return box.width >= 1 && box.height >= 1 && style.visibility !== 'hidden' && style.display !== 'none';
	});
})()`

// snapshotScript is a page as a numbered list of its interactive elements, with the values they hold.
const snapshotScript = `(limit, nameLength) => {
	const items = ` + candidatesJS + `;
	const squash = (text) => (text || '').replace(/\s+/g, ' ').trim().slice(0, nameLength);
	const textOf = (element) => element.innerText || element.textContent || '';
	const nameOf = (element) => {
		const byLabel = element.getAttribute('aria-labelledby');
		const labelled = byLabel ? byLabel.split(/\s+/).map((id) => document.getElementById(id)).filter(Boolean).map((each) => textOf(each)).join(' ') : '';
		const labels = element.labels ? Array.from(element.labels).map((each) => textOf(each)).join(' ') : '';
		const own = element.innerText || (element.tagName === 'INPUT' ? element.value : '');
		const rest = element.getAttribute('placeholder') || element.getAttribute('title') || element.getAttribute('alt') || element.getAttribute('name') || '';
		return squash(element.getAttribute('aria-label') || labelled || labels || own || rest);
	};
	const roleOf = (element) => {
		const role = element.getAttribute('role');
		if (role) return role;
		const tag = element.tagName.toLowerCase();
		if (tag === 'a') return 'link';
		if (tag === 'button' || tag === 'summary') return 'button';
		if (tag === 'select') return 'combobox';
		if (tag === 'textarea') return 'textbox';
		if (tag === 'input') {
			const roles = {button: 'button', checkbox: 'checkbox', color: 'color', file: 'file', radio: 'radio', range: 'slider', reset: 'button', submit: 'button'};
			return roles[(element.getAttribute('type') || 'text').toLowerCase()] || 'textbox';
		}
		return 'editable';
	};
	const valueOf = (element) => {
		const type = (element.getAttribute('type') || '').toLowerCase();
		if (type === 'password') return element.value ? '•••' : '';
		if (type === 'checkbox' || type === 'radio') return element.checked ? 'checked' : 'unchecked';
		if (element.tagName === 'SELECT') return Array.from(element.selectedOptions).map((option) => option.text.trim()).join(', ');
		if (element.isContentEditable) return squash(element.innerText);
		return element.value || '';
	};
	return JSON.stringify({
		url: location.href,
		title: document.title,
		total: items.length,
		items: items.slice(0, limit).map((element) => ({disabled: !!element.disabled, name: nameOf(element), role: roleOf(element), value: valueOf(element)})),
	});
}`

// elementScript is the element a snapshot numbered, as a reference to click.
const elementScript = `(index) => {
	const items = ` + candidatesJS + `;
	return items[index - 1] || null;
}`

// fillScript writes a value into the element a snapshot numbered, as typing and choosing would: a field's value
// set through its own setter, so a page that watches the field sees the change.
const fillScript = `(index, value) => {
	const items = ` + candidatesJS + `;
	const element = items[index - 1];
	if (!element) return 'missing';
	element.scrollIntoView({block: 'center'});
	element.focus();
	if (element.tagName === 'SELECT') {
		const option = Array.from(element.options).find((each) => each.value === value || each.text.trim() === value);
		if (!option) return 'option';
		element.value = option.value;
	} else if (element.isContentEditable) {
		element.innerText = value;
	} else if (element.tagName === 'INPUT' || element.tagName === 'TEXTAREA') {
		const proto = element.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
		Object.getOwnPropertyDescriptor(proto, 'value').set.call(element, value);
	} else {
		return 'notfillable';
	}
	element.dispatchEvent(new Event('input', {bubbles: true}));
	element.dispatchEvent(new Event('change', {bubbles: true}));
	return 'ok';
}`

// errNoPage is an action before the session opened a page.
var errNoPage = errors.New(noPageOpen)

var pressable = map[string]input.Key{
	"arrowdown": input.ArrowDown, "arrowleft": input.ArrowLeft, "arrowright": input.ArrowRight, "arrowup": input.ArrowUp,
	"backspace": input.Backspace, "delete": input.Delete, "enter": input.Enter, "escape": input.Escape, "tab": input.Tab,
}

// PageElement is one interactive element of a page, as its snapshot lists it.
type PageElement struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled"`
}

// PageState is a page as an action left it: where it is, and the numbered elements it shows.
type PageState struct {
	URL      string        `json:"url"`
	Title    string        `json:"title"`
	Elements []PageElement `json:"items"`
	Total    int           `json:"total"`
}

// Pages keeps one page per session, open between the agent's calls and turns, drawn by a browser started on the
// first page and closed with the daemon.
type Pages struct {
	bin      string
	headless bool
	mu       sync.Mutex
	browser  *rod.Browser
	pages    map[string]*keptPage
}

// keptPage is a session's page.
type keptPage struct {
	page *rod.Page
}

// NewPages keeps pages drawn by the browser at this path.
func NewPages(bin string) *Pages {
	return &Pages{bin: bin, headless: headlessBrowser(), pages: map[string]*keptPage{}}
}

// headlessBrowser tells whether the managed browser runs without a window: a computer with a screen shows it, so a
// person can use the very page the agent acts on, unless SWCODE_BROWSER_HEADLESS asks otherwise.
func headlessBrowser() bool {
	if forced, err := strconv.ParseBool(os.Getenv(headlessEnv)); err == nil && forced {
		return true
	}
	return runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
}

// Session is one session's page, by its id.
func (p *Pages) Session(id string) *Session { return &Session{id: id, pages: p} }

// Close closes a session's page, when it has one.
func (p *Pages) Close(id string) {
	p.mu.Lock()
	kept, ok := p.pages[id]
	delete(p.pages, id)
	p.mu.Unlock()
	if ok {
		_ = kept.page.Close()
	}
}

// CloseAll closes every page and the browser, for shutting down.
func (p *Pages) CloseAll() {
	p.mu.Lock()
	pages, browser := p.pages, p.browser
	p.pages, p.browser = map[string]*keptPage{}, nil
	p.mu.Unlock()
	for _, kept := range pages {
		_ = kept.page.Close()
	}
	if browser != nil {
		_ = browser.Close()
	}
}

// Session is a page an agent acts on: opened once, then clicked, filled and read by number.
type Session struct {
	id    string
	pages *Pages
}

// Open goes to a page, at the size asked, and lists what it shows.
func (s *Session) Open(ctx context.Context, url string, viewport Viewport, waitFor string) (PageState, error) {
	page, err := s.pages.pageFor(s.id)
	if err != nil {
		return PageState{}, err
	}
	timed := page.Context(ctx).Timeout(actionTimeout)
	if err := timed.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		DeviceScaleFactor: 1, Height: viewport.Height, Mobile: viewport.Mobile, Width: viewport.Width,
	}); err != nil {
		return PageState{}, err
	}
	if viewport.Mobile {
		_ = timed.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: phoneAgent})
		_ = proto.EmulationSetTouchEmulationEnabled{Enabled: true}.Call(timed)
	}
	if err := timed.Navigate(url); err != nil {
		return PageState{}, err
	}
	if err := timed.WaitLoad(); err != nil {
		return PageState{}, err
	}
	if waitFor != "" {
		if err := timed.Wait(rod.Eval(showsText, waitFor)); err != nil {
			return PageState{}, fmt.Errorf("it never showed %q: %w", waitFor, err)
		}
	}
	return settled(ctx, page)
}

// Snapshot lists the page again, as it is now.
func (s *Session) Snapshot(ctx context.Context) (PageState, error) {
	page, err := s.kept()
	if err != nil {
		return PageState{}, err
	}
	return snapshotOf(ctx, page)
}

// Click clicks the element a snapshot numbered, then lists the page again.
func (s *Session) Click(ctx context.Context, index int) (PageState, error) {
	page, err := s.kept()
	if err != nil {
		return PageState{}, err
	}
	found, err := page.Context(ctx).Timeout(actionTimeout).Evaluate(&rod.EvalOptions{JS: elementScript, JSArgs: []any{index}})
	if err != nil {
		return PageState{}, err
	}
	if found.ObjectID == "" {
		return PageState{}, fmt.Errorf("the page has no element %d: take a snapshot to number it again", index)
	}
	element, err := page.ElementFromObject(found)
	if err != nil {
		return PageState{}, err
	}
	if err := element.Context(ctx).Timeout(actionTimeout).Click(proto.InputMouseButtonLeft, 1); err != nil {
		return PageState{}, fmt.Errorf("element %d could not be clicked: %w", index, err)
	}
	return settled(ctx, page)
}

// Fill writes a value into the element a snapshot numbered, as typing and choosing do, then lists the page again.
func (s *Session) Fill(ctx context.Context, index int, value string) (PageState, error) {
	page, err := s.kept()
	if err != nil {
		return PageState{}, err
	}
	answer, err := page.Context(ctx).Timeout(actionTimeout).Eval(fillScript, index, value)
	if err != nil {
		return PageState{}, err
	}
	if problem := fillProblem(answer.Value.Str(), index, value); problem != "" {
		return PageState{}, errors.New(problem)
	}
	return settled(ctx, page)
}

// Press presses a key on the page, then lists it again.
func (s *Session) Press(ctx context.Context, key string) (PageState, error) {
	page, err := s.kept()
	if err != nil {
		return PageState{}, err
	}
	pressed, known := pressable[strings.ToLower(key)]
	if !known {
		return PageState{}, fmt.Errorf("there is no key %q to press", key)
	}
	if err := page.Keyboard.Type(pressed); err != nil {
		return PageState{}, err
	}
	return settled(ctx, page)
}

// HTML is the page's own markup.
func (s *Session) HTML(ctx context.Context) (string, error) {
	page, err := s.kept()
	if err != nil {
		return "", err
	}
	return page.Context(ctx).Timeout(actionTimeout).HTML()
}

// Frame is the session's page as a JPEG picture, for a person watching it: false when no page is open yet.
func (s *Session) Frame(ctx context.Context) ([]byte, bool, error) {
	page, err := s.kept()
	if errors.Is(err, errNoPage) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	quality := frameQuality
	shot, err := proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatJpeg, Quality: &quality}.Call(page.Context(ctx).Timeout(actionTimeout))
	if err != nil {
		return nil, true, err
	}
	return shot.Data, true, nil
}

// fillProblem says why a fill did not happen, or nothing when it did.
func fillProblem(answer string, index int, value string) string {
	switch answer {
	case fillOK:
		return ""
	case fillMissing:
		return fmt.Sprintf("the page has no element %d: take a snapshot to number it again", index)
	case fillOption:
		return fmt.Sprintf("element %d has no option %q", index, value)
	default:
		return fmt.Sprintf("element %d cannot be filled", index)
	}
}

// settled waits for the page an action left: its next load, when one started, then its menus and requests; and
// lists it.
func settled(ctx context.Context, page *rod.Page) (PageState, error) {
	time.Sleep(actionSettle)
	timed := page.Context(ctx).Timeout(actionTimeout)
	state, err := timed.Eval(`() => document.readyState`)
	if err != nil {
		return PageState{}, err
	}
	if state.Value.Str() != readyStateComplete {
		if err := timed.WaitLoad(); err != nil {
			return PageState{}, err
		}
	}
	return snapshotOf(ctx, timed)
}

// snapshotOf lists a page's interactive elements, numbered from 1.
func snapshotOf(ctx context.Context, page *rod.Page) (PageState, error) {
	raw, err := page.Context(ctx).Timeout(actionTimeout).Eval(snapshotScript, maxElements, maxNameLength)
	if err != nil {
		return PageState{}, err
	}
	var state PageState
	if err := json.Unmarshal([]byte(raw.Value.Str()), &state); err != nil {
		return PageState{}, err
	}
	return state, nil
}

// pageFor is a session's page, made on its first open.
func (p *Pages) pageFor(id string) (*rod.Page, error) {
	p.mu.Lock()
	kept, ok := p.pages[id]
	p.mu.Unlock()
	if ok {
		return kept.page, nil
	}
	browser, err := p.browserOrStart()
	if err != nil {
		return nil, err
	}
	page, err := browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		return nil, err
	}
	kept = &keptPage{page: page}
	p.mu.Lock()
	p.pages[id] = kept
	p.mu.Unlock()
	return page, nil
}

// browserOrStart is the browser the pages are drawn in, started on the first page and kept.
func (p *Pages) browserOrStart() (*rod.Browser, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.browser != nil {
		return p.browser, nil
	}
	launch := launcher.New().Bin(p.bin).Headless(p.headless).Leakless(false)
	if !p.headless {
		launch = launch.Set("window-size", windowSize)
	}
	control, err := launch.Launch()
	if err != nil {
		return nil, fmt.Errorf("the browser could not start: %w", err)
	}
	started := rod.New().ControlURL(control)
	if err := started.Connect(); err != nil {
		return nil, fmt.Errorf("the browser could not start: %w", err)
	}
	p.browser = started
	return started, nil
}

// kept is the session's page: an error before it was opened.
func (s *Session) kept() (*rod.Page, error) {
	p := s.pages
	p.mu.Lock()
	kept, ok := p.pages[s.id]
	p.mu.Unlock()
	if !ok {
		return nil, errNoPage
	}
	return kept.page, nil
}
