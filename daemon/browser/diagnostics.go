package browser

import (
	"fmt"
	"sync"

	"github.com/go-rod/rod/lib/proto"
)

const (
	// maxDiagnostics bounds what a long-lived page keeps: a page that logs in a loop must not grow the daemon.
	maxDiagnostics = 200
	// maxTrackedRequests bounds the requests remembered for their URL, so a failure can be named.
	maxTrackedRequests = 500
)

// DiagnosticKind is what a page did that the agent should know about.
type DiagnosticKind string

const (
	DiagnosticConsole   DiagnosticKind = "console"
	DiagnosticException DiagnosticKind = "exception"
	DiagnosticRequest   DiagnosticKind = "request"
	DiagnosticResponse  DiagnosticKind = "response"
	DiagnosticDialog    DiagnosticKind = "dialog"
	DiagnosticDownload  DiagnosticKind = "download"
)

// Diagnostic is one thing the page logged or failed at, in the order it happened.
type Diagnostic struct {
	Kind DiagnosticKind
	Text string
}

// Report is what a page has logged since it was opened: the newest diagnostics kept, and how many older ones were
// dropped.
type Report struct {
	Opened  bool
	Items   []Diagnostic
	Dropped int
}

type requestInfo struct {
	method string
	url    string
}

// diagnostics collects a page's console errors, exceptions, failed requests, HTTP errors, dialogs and blocked
// downloads as they happen, rather than only while a tool runs.
type diagnostics struct {
	mu       sync.Mutex
	items    []Diagnostic
	dropped  int
	requests map[proto.NetworkRequestID]requestInfo
}

func newDiagnostics() *diagnostics {
	return &diagnostics{requests: map[proto.NetworkRequestID]requestInfo{}}
}

func (d *diagnostics) add(kind DiagnosticKind, text string) {
	if text == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.items) >= maxDiagnostics {
		d.items = d.items[1:]
		d.dropped++
	}
	d.items = append(d.items, Diagnostic{Kind: kind, Text: text})
}

func (d *diagnostics) console(event *proto.RuntimeConsoleAPICalled) {
	if event.Type == proto.RuntimeConsoleAPICalledTypeError {
		d.add(DiagnosticConsole, consoleText(event))
	}
}

func (d *diagnostics) exception(event *proto.RuntimeExceptionThrown) {
	d.add(DiagnosticException, exceptionText(event.ExceptionDetails))
}

func (d *diagnostics) request(event *proto.NetworkRequestWillBeSent) {
	if event.Request == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.requests) >= maxTrackedRequests {
		d.requests = map[proto.NetworkRequestID]requestInfo{}
	}
	d.requests[event.RequestID] = requestInfo{method: event.Request.Method, url: event.Request.URL}
}

func (d *diagnostics) failed(event *proto.NetworkLoadingFailed) {
	d.mu.Lock()
	known, has := d.requests[event.RequestID]
	d.mu.Unlock()
	if event.Canceled {
		return
	}
	what := string(event.Type)
	if has {
		what = known.method + " " + known.url
	}
	d.add(DiagnosticRequest, fmt.Sprintf("%s — %s", what, event.ErrorText))
}

func (d *diagnostics) response(event *proto.NetworkResponseReceived) {
	if event.Response == nil || event.Response.Status < 400 {
		return
	}
	d.mu.Lock()
	known, has := d.requests[event.RequestID]
	d.mu.Unlock()
	method := known.method
	if !has {
		method = string(event.Type)
	}
	d.add(DiagnosticResponse, fmt.Sprintf("%d %s %s", event.Response.Status, method, event.Response.URL))
}

func (d *diagnostics) dialog(event *proto.PageJavascriptDialogOpening) {
	d.add(DiagnosticDialog, fmt.Sprintf("%s dialog: %s", event.Type, event.Message))
}

func (d *diagnostics) download(event *proto.PageDownloadWillBegin) {
	d.add(DiagnosticDownload, fmt.Sprintf("%s (%s)", event.SuggestedFilename, event.URL))
}

// report is what the page has logged so far.
func (d *diagnostics) report(opened bool) Report {
	d.mu.Lock()
	defer d.mu.Unlock()
	return Report{Opened: opened, Items: append([]Diagnostic{}, d.items...), Dropped: d.dropped}
}
