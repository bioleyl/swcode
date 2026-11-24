package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"swcode/daemon/browser"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
)

type fakePages struct {
	clicked  []int
	filled   []string
	html     string
	opened   []string
	snapshot browser.PageState
	viewport browser.Viewport
	waited   string
}

func (p *fakePages) Open(_ context.Context, url string, viewport browser.Viewport, waitFor string) (browser.PageState, error) {
	p.opened = append(p.opened, url)
	p.viewport, p.waited = viewport, waitFor
	return p.snapshot, nil
}

func (p *fakePages) Snapshot(context.Context) (browser.PageState, error) { return p.snapshot, nil }

func (p *fakePages) Click(_ context.Context, index int) (browser.PageState, error) {
	p.clicked = append(p.clicked, index)
	return p.snapshot, nil
}

func (p *fakePages) Fill(_ context.Context, index int, value string) (browser.PageState, error) {
	p.filled = append(p.filled, value)
	return p.snapshot, nil
}

func (p *fakePages) Press(_ context.Context, _ string) (browser.PageState, error) {
	return p.snapshot, nil
}

func (p *fakePages) HTML(context.Context) (string, error) { return p.html, nil }

func orderPageState() browser.PageState {
	return browser.PageState{
		Elements: []browser.PageElement{
			{Name: "Menu", Role: "button"},
			{Name: "Email", Role: "textbox", Value: "a@b.c"},
			{Disabled: true, Name: "Envoyer", Role: "button"},
		},
		Title: "Order",
		Total: 4,
		URL:   "http://localhost:5173/",
	}
}

func TestBrowseOpensAPageAtTheSizeAskedAndListsItsNumberedElements(t *testing.T) {
	f := setUp(t)
	pages := &fakePages{snapshot: orderPageState()}
	f.tc.Pages = pages
	got := f.call("browse", `{"action":"open","size":"phone","url":"http://localhost:5173/","waitFor":"Ready"}`)
	want := strings.Join([]string{
		"Page: Order",
		"URL: http://localhost:5173/",
		"Interactive elements:",
		`1 button "Menu"`,
		`2 textbox "Email" = "a@b.c"`,
		`3 button "Envoyer" (disabled)`,
		"The page shows 4 interactive elements: these are the first 3.",
	}, "\n")
	if !got.OK || got.Output != want || got.Meta != "Order" {
		t.Fatalf("got %q, %+v", got.Output, got)
	}
	if len(pages.opened) != 1 || pages.viewport != (browser.Viewport{Height: 844, Mobile: true, Width: 390}) || pages.waited != "Ready" {
		t.Fatalf("opened %v, %+v, %q", pages.opened, pages.viewport, pages.waited)
	}
}

func TestBrowseClicksAndFillsTheElementASnapshotNumbered(t *testing.T) {
	f := setUp(t)
	pages := &fakePages{snapshot: orderPageState()}
	f.tc.Pages = pages
	f.call("browse", `{"action":"click","index":1}`)
	f.call("browse", `{"action":"fill","index":2,"value":""}`)
	if len(pages.clicked) != 1 || pages.clicked[0] != 1 || len(pages.filled) != 1 || pages.filled[0] != "" {
		t.Fatalf("clicked %v, filled %v", pages.clicked, pages.filled)
	}
}

func TestBrowseReadsTheHTMLCutWhereItGetsTooLongForTheModel(t *testing.T) {
	f := setUp(t)
	pages := &fakePages{html: strings.Repeat("a", maxHTML+10)}
	f.tc.Pages = pages
	got := f.call("browse", `{"action":"html"}`)
	if len([]rune(got.Output)) != maxHTML+1 || !strings.HasSuffix(got.Output, "…") || got.Meta != "html" {
		t.Fatalf("got %d runes, meta %q", len([]rune(got.Output)), got.Meta)
	}
}

func TestBrowseIsRefusedWithoutABrowserAndWithoutAURLToOpen(t *testing.T) {
	f := setUp(t)
	f.refused("browse", `{"action":"snapshot"}`, noPageBrowser)
	f.tc.Pages = &fakePages{}
	f.refused("browse", `{"action":"open"}`, urlNeeded)
}

func TestBrowseAsksAboutTheCallsThatChangeThePageAndNotAboutTheOnesThatReadIt(t *testing.T) {
	f := setUp(t)
	f.tc.Pages = &fakePages{}
	tool := all[protocol.ToolNameBrowse]
	prepare := func(input string) permissions.Subject {
		var raw any
		if err := json.Unmarshal([]byte(input), &raw); err != nil {
			t.Fatal(err)
		}
		prepared := tool.Prepare(context.Background(), raw, f.tc)
		if !prepared.OK {
			t.Fatalf("%s refused: %s", input, prepared.Problem)
		}
		return prepared.Call.Subject
	}
	if got := prepare(`{"action":"click","index":1}`); got != (permissions.Subject{Access: permissions.AccessCommand, Command: browseProgram}) {
		t.Fatalf("click subject %+v", got)
	}
	if got := prepare(`{"action":"snapshot"}`); got != permissions.Read {
		t.Fatalf("snapshot subject %+v", got)
	}
}
