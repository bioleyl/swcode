package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"swcode/daemon/browser"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
)

const (
	actionOpen     = "open"
	actionClick    = "click"
	actionFill     = "fill"
	actionPress    = "press"
	actionHTML     = "html"
	actionSnapshot = "snapshot"
	// maxHTML is how much of a page's markup the model reads at once.
	maxHTML = 20000
	// browseProgram is the program a person allows: the browse tool, as a session's command.
	browseProgram = "browse"
	noPageBrowser = "No browser to act on pages with here."
	urlNeeded     = "action open needs the page's url."
)

// browseInput is what the model says to act on a page.
type browseInput struct {
	Action  string `json:"action"`
	Index   int    `json:"index"`
	Key     string `json:"key"`
	Size    string `json:"size"`
	URL     string `json:"url"`
	Value   string `json:"value"`
	WaitFor string `json:"waitFor"`
}

// size is the window the page is opened in: a desktop's unless asked otherwise.
func (input browseInput) size() string {
	if input.Size == "" {
		return desktopSize
	}
	return input.Size
}

// browseActions is what each action does, by the word the model calls it by.
var browseActions = map[string]func(context.Context, BrowserPages, browseInput) Outcome{
	actionClick:    clickPage,
	actionFill:     fillPage,
	actionHTML:     readPageHTML,
	actionOpen:     openPage,
	actionPress:    pressPageKey,
	actionSnapshot: snapshotPage,
}

// browseActs are the actions that change the page: a person may be asked about them.
var browseActs = map[string]bool{actionClick: true, actionFill: true, actionPress: true}

// browseSubject is what a browse call touches: a changing page is a side effect, as a command is; reading it is
// not.
func browseSubject(input browseInput, _ *Context) permissions.Subject {
	if browseActs[input.Action] {
		return permissions.Subject{Access: permissions.AccessCommand, Command: browseProgram}
	}
	return permissions.Read
}

// browseSummary is the call in a few words, for people.
func browseSummary(input browseInput) string {
	parts := nonEmpty(input.Action, input.URL, input.Key)
	if input.Index > 0 {
		parts = append(parts, strconv.Itoa(input.Index))
	}
	return strings.Join(parts, " ")
}

// browseTool acts on a web page kept between calls: the model numbers its elements, then clicks, fills or presses
// one, or reads its markup. A menu it opened stays open for the next call.
var browseTool = Define(Spec[browseInput]{
	Check: func(_ context.Context, input browseInput, tc *Context) Check {
		if tc.Pages == nil {
			return Refuse(noPageBrowser)
		}
		if input.Action == actionOpen && input.URL == "" {
			return Refuse(urlNeeded)
		}
		run, known := browseActions[input.Action]
		if !known {
			return Refuse(fmt.Sprintf("There is no action %q.", input.Action))
		}
		pages := tc.Pages
		return Ready(func(ctx context.Context) Outcome { return run(ctx, pages, input) }, nil)
	},
	Description: "Acts on a web page in a browser, at a desktop or a phone size: opens it, then clicks, fills or presses on the numbered elements a snapshot lists, reads its HTML, or lists it again. The page stays open between calls and turns, so an opened menu stays open: act by the numbers of the last snapshot, and take a snapshot again after the page changed. Start a dev server first, with bash in the background.",
	Input: Object(
		Required("action", Enum(actionOpen, actionClick, actionFill, actionPress, actionHTML, actionSnapshot).Describe("open a page, then click, fill, press, read its html, or list it again.")),
		Optional("url", String().Trimmed().Min(1).Describe("open: the page, a dev server running in the background (http://localhost:5173/) or any web page.")),
		Optional("index", Number().AtLeast(1).Describe("click and fill: the element's number in the last snapshot.")),
		Optional("value", String().Describe("fill: what to write in the element. Empty clears it.")),
		Optional("key", Enum("enter", "escape", "tab", "backspace", "delete", "arrowup", "arrowdown", "arrowleft", "arrowright").Describe("press: the key to press on the page.")),
		Optional("size", Enum(desktopSize, phoneSize).Describe("open: desktop: 1440 × 900, the default; phone: 390 × 844.")),
		Optional("waitFor", String().Trimmed().Min(1).Describe("open: text the page shows once ready: the call waits for it.")),
	),
	Name:      protocol.ToolNameBrowse,
	Subject:   browseSubject,
	Summarize: browseSummary,
})

// openPage goes to a page and lists what it shows.
func openPage(ctx context.Context, pages BrowserPages, input browseInput) Outcome {
	state, err := pages.Open(ctx, input.URL, viewports[input.size()], input.WaitFor)
	if err != nil {
		return Failure(err.Error(), "failed")
	}
	return pageOutcome(state)
}

// snapshotPage lists the page as the last action left it.
func snapshotPage(ctx context.Context, pages BrowserPages, _ browseInput) Outcome {
	state, err := pages.Snapshot(ctx)
	if err != nil {
		return Failure(err.Error(), "failed")
	}
	return pageOutcome(state)
}

// clickPage clicks the element a snapshot numbered.
func clickPage(ctx context.Context, pages BrowserPages, input browseInput) Outcome {
	state, err := pages.Click(ctx, input.Index)
	if err != nil {
		return Failure(err.Error(), "failed")
	}
	return pageOutcome(state)
}

// fillPage writes a value into the element a snapshot numbered.
func fillPage(ctx context.Context, pages BrowserPages, input browseInput) Outcome {
	state, err := pages.Fill(ctx, input.Index, input.Value)
	if err != nil {
		return Failure(err.Error(), "failed")
	}
	return pageOutcome(state)
}

// pressPageKey presses a key on the page.
func pressPageKey(ctx context.Context, pages BrowserPages, input browseInput) Outcome {
	state, err := pages.Press(ctx, input.Key)
	if err != nil {
		return Failure(err.Error(), "failed")
	}
	return pageOutcome(state)
}

// readPageHTML is the page's own markup, cut where it gets too long for the model.
func readPageHTML(ctx context.Context, pages BrowserPages, _ browseInput) Outcome {
	markup, err := pages.HTML(ctx)
	if err != nil {
		return Failure(err.Error(), "failed")
	}
	return Outcome{OK: true, Output: clipped(markup, maxHTML), Meta: "html"}
}

// pageOutcome is a page as the model reads it: where it is and the numbered elements it shows.
func pageOutcome(state browser.PageState) Outcome {
	lines := nonEmpty("Page: "+state.Title, "URL: "+state.URL)
	if len(state.Elements) == 0 {
		return Outcome{OK: true, Output: strings.Join(append(lines, "It shows no interactive element."), "\n"), Meta: state.Title}
	}
	lines = append(lines, "Interactive elements:")
	for index, element := range state.Elements {
		lines = append(lines, elementLine(index+1, element))
	}
	if state.Total > len(state.Elements) {
		lines = append(lines, fmt.Sprintf("The page shows %d interactive elements: these are the first %d.", state.Total, len(state.Elements)))
	}
	return Outcome{OK: true, Output: strings.Join(lines, "\n"), Meta: state.Title}
}

// elementLine is one element as a snapshot lists it: its number, role, name, value, and whether it is disabled.
func elementLine(index int, element browser.PageElement) string {
	parts := []string{strconv.Itoa(index), element.Role, strconv.Quote(element.Name)}
	if element.Value != "" {
		parts = append(parts, "= "+strconv.Quote(element.Value))
	}
	if element.Disabled {
		parts = append(parts, "(disabled)")
	}
	return strings.Join(parts, " ")
}
