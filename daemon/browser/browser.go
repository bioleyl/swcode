// Package browser draws web pages for the screenshot tool, in a headless Chromium: Playwright's when it is there,
// else the system's Chrome, Chromium or Edge (Windows 11 has Edge).
package browser

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const (
	// settle is what a page gets after it loads, for its fonts and its first effects.
	settle = 300 * time.Millisecond
	// MaxPageHeight: the whole page stops there, a longer one would take thousands of the model's tokens.
	MaxPageHeight = 4000
	maxErrors     = 20
	// phoneAgent is a phone's Chrome, as the engine drawing it is Chromium.
	phoneAgent = "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Mobile Safari/537.36"
	pageHeight = `() => Math.max(document.documentElement.scrollHeight, document.body ? document.body.scrollHeight : 0)`
	showsText  = `(text) => document.body !== null && document.body.innerText.includes(text)`
	// A PNG's width and height sit in its header chunk, at these offsets.
	widthOffset    = 16
	heightOffset   = 20
	chromiumEnv    = "SWCODE_CHROMIUM"
	headlessPrefix = "chromium_headless_shell-"
)

var errorName = regexp.MustCompile(`^\w*Error: `)

// Viewport is the window a page is drawn in: a computer's, or a phone's.
type Viewport struct {
	Width  int
	Height int
	Mobile bool
}

type ShotRequest struct {
	URL      string
	Viewport Viewport
	// FullPage: the whole page, scrolled, rather than what the window shows.
	FullPage bool
	// WaitFor is text the page shows once it is ready: the picture waits for it.
	WaitFor string
	Timeout time.Duration
}

// Shot is a page as the browser drew it: the picture, its size, and what the page logged as errors.
type Shot struct {
	PNG    []byte
	Width  int
	Height int
	Title  string
	Errors []string
}

type ShotOutcome struct {
	OK      bool
	Shot    Shot
	Problem string
}

// Browser draws pages.
type Browser interface {
	Capture(ctx context.Context, request ShotRequest) ShotOutcome
}

// PNGSize is a PNG's size in pixels, from its header.
func PNGSize(png []byte) (int, int) {
	if len(png) < heightOffset+4 {
		return 0, 0
	}
	return int(binary.BigEndian.Uint32(png[widthOffset:])), int(binary.BigEndian.Uint32(png[heightOffset:]))
}

// playwrightFolders is where Playwright keeps its browsers on each system.
func playwrightFolders(home string) []string {
	if cache := os.Getenv("PLAYWRIGHT_BROWSERS_PATH"); cache != "" {
		return []string{cache}
	}
	folders := map[string][]string{
		"darwin":  {filepath.Join(home, "Library", "Caches", "ms-playwright")},
		"windows": {filepath.Join(os.Getenv("LOCALAPPDATA"), "ms-playwright")},
	}
	if found, ok := folders[runtime.GOOS]; ok {
		return found
	}
	return []string{filepath.Join(home, ".cache", "ms-playwright")}
}

// playwrightShell is the newest headless shell Playwright installed (npm run e2e:install).
func playwrightShell(home string) string {
	for _, folder := range playwrightFolders(home) {
		installs, _ := filepath.Glob(filepath.Join(folder, headlessPrefix+"*"))
		sort.Sort(sort.Reverse(sort.StringSlice(installs)))
		for _, install := range installs {
			shells, _ := filepath.Glob(filepath.Join(install, "*", "chrome-headless-shell*"))
			shells = append(shells, filepath.Join(install, "chrome-linux", "headless_shell"))
			for _, shell := range shells {
				if info, err := os.Stat(shell); err == nil && !info.IsDir() {
					return shell
				}
			}
		}
	}
	return ""
}

// Find is the browser to draw pages with: SWCODE_CHROMIUM, Playwright's, or the system's; none when there is none.
func Find(home string) Browser {
	if bin := os.Getenv(chromiumEnv); bin != "" {
		return &Chromium{bin: bin}
	}
	if shell := playwrightShell(home); shell != "" {
		return &Chromium{bin: shell}
	}
	if system, found := launcher.LookPath(); found {
		return &Chromium{bin: system}
	}
	return nil
}

// Chromium is a Chromium program, started for each picture and closed once it is taken, or when the turn stops.
type Chromium struct {
	bin string
}

// NewChromium draws with the browser at this path.
func NewChromium(bin string) *Chromium { return &Chromium{bin: bin} }

func (c *Chromium) Capture(ctx context.Context, request ShotRequest) ShotOutcome {
	control, err := launcher.New().Bin(c.bin).Headless(true).Leakless(false).Context(ctx).Launch()
	if err != nil {
		return ShotOutcome{Problem: "The browser could not start: " + err.Error()}
	}
	browser := rod.New().ControlURL(control).Context(ctx)
	if err := browser.Connect(); err != nil {
		return ShotOutcome{Problem: "The browser could not start: " + err.Error()}
	}
	defer func() { _ = browser.Close() }()
	shot, err := draw(browser, request)
	if err != nil {
		return ShotOutcome{Problem: fmt.Sprintf("%s could not be drawn: %v", request.URL, err)}
	}
	return ShotOutcome{OK: true, Shot: shot}
}

// pageErrors keeps what the page logged as errors: console.error, and the exceptions nothing caught.
type pageErrors struct {
	mu   sync.Mutex
	list []string
}

func (p *pageErrors) add(text string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.list = append(p.list, text)
}

func (p *pageErrors) first() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string{}, p.list[:min(len(p.list), maxErrors)]...)
}

// exceptionText is an exception as its message says it, without its name and its stack.
func exceptionText(details *proto.RuntimeExceptionDetails) string {
	if details.Exception == nil || details.Exception.Description == "" {
		return details.Text
	}
	line, _, _ := strings.Cut(details.Exception.Description, "\n")
	return errorName.ReplaceAllString(line, "")
}

func consoleText(event *proto.RuntimeConsoleAPICalled) string {
	parts := []string{}
	for _, argument := range event.Args {
		text := argument.Value.Str()
		if text == "" {
			text = argument.Description
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " ")
}

func draw(browser *rod.Browser, request ShotRequest) (Shot, error) {
	page, err := browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		return Shot{}, err
	}
	viewport := request.Viewport
	if err := page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		DeviceScaleFactor: 1, Height: viewport.Height, Mobile: viewport.Mobile, Width: viewport.Width,
	}); err != nil {
		return Shot{}, err
	}
	if viewport.Mobile {
		_ = page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: phoneAgent})
		_ = proto.EmulationSetTouchEmulationEnabled{Enabled: true}.Call(page)
	}
	errors := &pageErrors{}
	go page.EachEvent(func(event *proto.RuntimeConsoleAPICalled) {
		if event.Type == proto.RuntimeConsoleAPICalledTypeError {
			errors.add(consoleText(event))
		}
	}, func(event *proto.RuntimeExceptionThrown) {
		errors.add(exceptionText(event.ExceptionDetails))
	})()
	timed := page.Timeout(request.Timeout)
	if err := timed.Navigate(request.URL); err != nil {
		return Shot{}, err
	}
	if err := timed.WaitLoad(); err != nil {
		return Shot{}, err
	}
	if request.WaitFor != "" {
		if err := timed.Wait(rod.Eval(showsText, request.WaitFor)); err != nil {
			return Shot{}, fmt.Errorf("it never showed %q: %w", request.WaitFor, err)
		}
	}
	time.Sleep(settle)
	png, err := screenshot(page, request)
	if err != nil {
		return Shot{}, err
	}
	title, _ := page.Eval(`() => document.title`)
	width, height := PNGSize(png)
	return Shot{Errors: errors.first(), Height: height, PNG: png, Title: title.Value.Str(), Width: width}, nil
}

func screenshot(page *rod.Page, request ShotRequest) ([]byte, error) {
	capture := proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng}
	if request.FullPage {
		measured, err := page.Eval(pageHeight)
		if err != nil {
			return nil, err
		}
		height := min(measured.Value.Int(), MaxPageHeight)
		capture.CaptureBeyondViewport = true
		capture.Clip = &proto.PageViewport{Height: float64(height), Scale: 1, Width: float64(request.Viewport.Width)}
	}
	shot, err := capture.Call(page)
	if err != nil {
		return nil, err
	}
	if len(shot.Data) == 0 {
		return nil, errors.New("the browser gave no picture")
	}
	return shot.Data, nil
}
