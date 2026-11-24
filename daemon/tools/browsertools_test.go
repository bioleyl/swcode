package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"swcode/daemon/browser"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
)

type fakeBrowserPage struct {
	target     browser.Target
	snapshot   browser.Snapshot
	report     browser.Report
	openResult browser.OpenResult
	action     browser.ActionResult
	shot       browser.Shot
	opened     []string
	clicked    []string
	typed      [][2]string
	err        error
}

func (p *fakeBrowserPage) Open(_ context.Context, url string) (browser.OpenResult, error) {
	p.opened = append(p.opened, url)
	return p.openResult, p.err
}

func (p *fakeBrowserPage) Snapshot(context.Context, string, string) (browser.Snapshot, error) {
	return p.snapshot, p.err
}

func (p *fakeBrowserPage) Inspect(context.Context, string) (browser.Target, error) {
	return p.target, p.err
}

func (p *fakeBrowserPage) Click(_ context.Context, target string) (browser.ActionResult, error) {
	p.clicked = append(p.clicked, target)
	return p.action, p.err
}

func (p *fakeBrowserPage) Type(_ context.Context, target, text string) (browser.ActionResult, error) {
	p.typed = append(p.typed, [2]string{target, text})
	return p.action, p.err
}

func (p *fakeBrowserPage) Screenshot(context.Context, bool) (browser.Shot, error) {
	return p.shot, p.err
}

func (p *fakeBrowserPage) Diagnostics() browser.Report { return p.report }

func (f *fixture) prepare(name, input string) Preparation {
	f.t.Helper()
	tool, ok := all[protocol.ToolName(name)]
	if !ok {
		f.t.Fatalf("no tool %s", name)
	}
	var raw any
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		f.t.Fatal(err)
	}
	return tool.Prepare(context.Background(), raw, f.tc)
}

func TestTheBrowserToolsKeepTheNamesTheModelCalls(t *testing.T) {
	names := map[string]protocol.ToolName{
		"browser_open":        protocol.ToolNameBrowseropen,
		"browser_snapshot":    protocol.ToolNameBrowsersnapshot,
		"browser_click":       protocol.ToolNameBrowserclick,
		"browser_type":        protocol.ToolNameBrowsertype,
		"browser_screenshot":  protocol.ToolNameBrowserscreenshot,
		"browser_diagnostics": protocol.ToolNameBrowserdiagnostics,
	}
	for model, expected := range names {
		tool, found := Main.Find(model)
		if !found || tool.Name != expected {
			t.Fatalf("%s: %+v, found %v", model, tool.Name, found)
		}
	}
}

func TestASnapshotNamesControlsWithRefsAndTreatsThePageAsData(t *testing.T) {
	f := setUp(t)
	f.tc.Browser = &fakeBrowserPage{report: browser.Report{Opened: true}, snapshot: browser.Snapshot{
		URL: "http://localhost:5173/", Title: "App",
		Items: []browser.SnapshotItem{
			{Kind: "heading", Level: 1, Name: "Welcome", Ref: "e1"},
			{Kind: "input", Name: "Password", Ref: "e2", Sensitive: true, Type: "password", Value: "secret"},
			{Kind: "button", Name: "Save", Ref: "e3"},
		},
	}}
	got := f.call("browsersnapshot", `{}`)
	if !got.OK || !strings.Contains(got.Output, "untrusted data") || !strings.Contains(got.Output, `e3  button  "Save"`) {
		t.Fatalf("output %q", got.Output)
	}
	if strings.Contains(got.Output, "secret") || !strings.Contains(got.Output, "value=<hidden>") {
		t.Fatalf("a password was read: %q", got.Output)
	}
}

func TestAPrivateAddressIsAskedAboutAndOnlyWebAddressesOpen(t *testing.T) {
	f := setUp(t)
	f.tc.Browser = &fakeBrowserPage{openResult: browser.OpenResult{URL: "http://127.0.0.1:5173/", Title: "App"}}

	local := f.prepare("browseropen", `{"url":"http://127.0.0.1:5173/"}`)
	if !local.OK || local.Call.Subject.Access != permissions.AccessRead {
		t.Fatalf("local page: %+v", local)
	}
	private := f.prepare("browseropen", `{"url":"http://10.0.0.5:8080/"}`)
	if !private.OK || private.Call.Subject.Access != permissions.AccessBrowser || !private.Call.Subject.Browser.Approval {
		t.Fatalf("private page: %+v", private)
	}
	f.refused("browseropen", `{"url":"file:///etc/passwd"}`, "URL|http")
}

func TestTypingIntoAPasswordFieldIsAskedAboutAndChoicesAreClicked(t *testing.T) {
	f := setUp(t)
	f.tc.Browser = &fakeBrowserPage{target: browser.Target{
		Kind: "input", Name: "Password", Ref: "e1", Sensitive: true, URL: "http://localhost:5173/",
	}}
	typed := f.prepare("browsertype", `{"target":"e1","text":"hunter2"}`)
	if !typed.OK || typed.Call.Subject.Access != permissions.AccessBrowser || !typed.Call.Subject.Browser.Approval {
		t.Fatalf("password: %+v", typed)
	}

	f.tc.Browser = &fakeBrowserPage{target: browser.Target{Kind: "checkbox", Name: "Remember", Ref: "e1"}}
	f.refused("browsertype", `{"target":"e1","text":"yes"}`, "click it with browser_click")

	// inspectScript names an <input> by its type, so a text field arrives as "text" and not "input".
	f.tc.Browser = &fakeBrowserPage{target: browser.Target{Kind: "text", Name: "Username", Ref: "e1"}}
	if text := f.prepare("browsertype", `{"target":"e1","text":"standard_user"}`); !text.OK {
		t.Fatalf("text field: %+v", text)
	}

	f.tc.Browser = &fakeBrowserPage{target: browser.Target{Kind: "input", Name: "Name", Ref: "e1", Disabled: true}}
	f.refused("browsertype", `{"target":"e1","text":"Ada"}`, "disabled")
}

func TestAClickThatSubmitsToAnotherOriginIsAskedAbout(t *testing.T) {
	f := setUp(t)
	f.tc.Browser = &fakeBrowserPage{
		target: browser.Target{
			Kind: "button", Name: "Pay", Ref: "e1", URL: "http://localhost:5173/",
			SubmitsTo: "https://pay.example/checkout", CrossOrigin: true,
		},
		action: browser.ActionResult{URL: "https://pay.example/checkout", Title: "Pay"},
	}
	pay := f.prepare("browserclick", `{"target":"e1"}`)
	if !pay.OK || pay.Call.Subject.Access != permissions.AccessBrowser || !pay.Call.Subject.Browser.Approval {
		t.Fatalf("pay: %+v", pay)
	}
	if pay.Call.Preview == nil {
		t.Fatal("no preview explains the other origin")
	}

	f.tc.Browser = &fakeBrowserPage{target: browser.Target{Kind: "button", Name: "Save", Ref: "e1", URL: "http://localhost:5173/"}}
	save := f.prepare("browserclick", `{"target":"e1"}`)
	if !save.OK || save.Call.Subject.Browser.Approval {
		t.Fatalf("save: %+v", save)
	}

	f.tc.Browser = &fakeBrowserPage{target: browser.Target{Kind: "link", Name: "Export", Ref: "e1", Download: true}}
	f.refused("browserclick", `{"target":"e1"}`, "downloads are blocked")

	f.tc.Browser = &fakeBrowserPage{target: browser.Target{Kind: "link", Name: "Router", Ref: "e1", Href: "http://192.168.1.1/"}}
	privateLink := f.prepare("browserclick", `{"target":"e1"}`)
	if !privateLink.OK || !privateLink.Call.Subject.Browser.Approval {
		t.Fatalf("private link: %+v", privateLink)
	}

	f.tc.Browser = &fakeBrowserPage{target: browser.Target{Kind: "link", Name: "Script", Ref: "e1", Href: "javascript:alert(1)"}}
	f.refused("browserclick", `{"target":"e1"}`, "not an http or https page")
}

func TestDiagnosticsNameEachKindThePageLogged(t *testing.T) {
	f := setUp(t)
	f.tc.Browser = &fakeBrowserPage{report: browser.Report{Opened: true, Items: []browser.Diagnostic{
		{Kind: browser.DiagnosticConsole, Text: "boom"},
		{Kind: browser.DiagnosticException, Text: "missing is not defined"},
		{Kind: browser.DiagnosticRequest, Text: "GET http://127.0.0.1:9/ — net::ERR_CONNECTION_REFUSED"},
		{Kind: browser.DiagnosticResponse, Text: "404 GET http://localhost:5173/missing"},
	}}}
	got := f.call("browserdiagnostics", `{}`)
	for _, wanted := range []string{"Console errors", "Uncaught exceptions", "Failed network requests", "HTTP error responses", "boom", "404"} {
		if !got.OK || !strings.Contains(got.Output, wanted) {
			t.Fatalf("output %q lacks %q", got.Output, wanted)
		}
	}
	empty := &fakeBrowserPage{report: browser.Report{Opened: true}}
	f.tc.Browser = empty
	if got := f.call("browserdiagnostics", `{}`); !strings.Contains(got.Output, "no console errors") {
		t.Fatalf("empty report: %q", got.Output)
	}
	f.tc.Browser = &fakeBrowserPage{}
	f.refused("browserdiagnostics", `{}`, "open one with browser_open")
}

func TestABrowserScreenshotIsShownAndCanComeIntoTheContext(t *testing.T) {
	f := setUp(t)
	shots := &fakeScreenshots{seen: true}
	f.tc.Screenshots = shots
	f.tc.Browser = &fakeBrowserPage{report: browser.Report{Opened: true}, shot: browser.Shot{Height: 900, PNG: []byte("png"), Title: "App", URL: "http://localhost:5173/", Width: 1440}}
	got := f.call("browserscreenshot", `{"see":true}`)
	if !got.OK || got.Display == nil || len(got.Pictures) != 1 || !strings.Contains(got.Output, "Screenshot of http://localhost:5173/") {
		t.Fatalf("got %+v", got)
	}
	if !strings.Contains(got.Output, "It follows as a picture.") {
		t.Fatalf("output %q", got.Output)
	}
}

func TestTheBrowserToolsAreRefusedWithoutABrowser(t *testing.T) {
	f := setUp(t)
	inputs := map[string]string{
		"browseropen":        `{"url":"http://localhost:5173/"}`,
		"browsersnapshot":    `{}`,
		"browserclick":       `{"target":"e1"}`,
		"browsertype":        `{"target":"e1","text":"Ada"}`,
		"browserscreenshot":  `{}`,
		"browserdiagnostics": `{}`,
	}
	for name, input := range inputs {
		f.refused(name, input, "No browser")
	}
}

func TestAClickOrTypedTextCanFindAControlByItsText(t *testing.T) {
	f := setUp(t)
	page := &fakeBrowserPage{
		report:   browser.Report{Opened: true},
		snapshot: browser.Snapshot{Items: []browser.SnapshotItem{{Kind: "button", Name: "Save", Ref: "e2"}}},
		target:   browser.Target{Kind: "button", Name: "Save", Ref: "e2", URL: "http://localhost:5173/"},
		action:   browser.ActionResult{URL: "http://localhost:5173/"},
	}
	f.tc.Browser = page
	clicked := f.call("browserclick", `{"text":"Save"}`)
	if !clicked.OK || !reflect.DeepEqual(page.clicked, []string{"e2"}) || !strings.Contains(clicked.Output, `e2 "Save"`) {
		t.Fatalf("clicked %+v, page %+v", clicked, page.clicked)
	}

	page.target = browser.Target{Kind: "input", Name: "Email", Ref: "e3", URL: "http://localhost:5173/"}
	page.snapshot = browser.Snapshot{Items: []browser.SnapshotItem{{Kind: "input", Name: "Email", Ref: "e3"}}}
	typed := f.call("browsertype", `{"label":"Email","text":"ada@example.com"}`)
	if !typed.OK || !reflect.DeepEqual(page.typed, [][2]string{{"e3", "ada@example.com"}}) {
		t.Fatalf("typed %+v, page %+v", typed, page.typed)
	}

	f.refused("browserclick", `{"target":"e1","text":"Save"}`, "not both")
	f.refused("browserclick", `{}`, "Give a ref")
}
