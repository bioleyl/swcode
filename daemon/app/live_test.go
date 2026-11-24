package app_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"swcode/daemon/browser"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

// liveSession is a session of the test daemon, as a page names it in a live view address: the start of its id.
func liveSession(t *testing.T, test *App) string {
	t.Helper()
	summary, problem := test.App.Sessions.Create(t.Context(), sessions.Request{})
	if problem != "" {
		t.Fatal(problem)
	}
	return summary.ID[:8]
}

func TestTheLivePageNeedsNoTokenButItsFramesDo(t *testing.T) {
	test := startHello(t)
	short := liveSession(t, test)
	status, body, header := raw(t, test, "GET", protocol.EndpointLive+"/"+short, "", nil)
	if status != 200 || !strings.Contains(string(body), "swcode · live page") {
		t.Fatalf("status %d, %q", status, body)
	}
	if header.Get("content-type") != "text/html; charset=utf-8" || !strings.Contains(string(body), protocol.TokenStorageKey) {
		t.Fatalf("headers %v, page %q", header, body)
	}
	if status, body, _ := raw(t, test, "GET", protocol.EndpointLive+"/bad!name", "", nil); status != 404 || !strings.Contains(string(body), "names no session") {
		t.Fatalf("status %d, %q outside a session", status, body)
	}
	if status, body, _ := raw(t, test, "GET", protocol.EndpointLive+"/"+short+"-again", "", nil); status != 404 || !strings.Contains(string(body), "names no session") {
		t.Fatalf("status %d, %q for a name no session starts with", status, body)
	}
	if status, _, _ := raw(t, test, "GET", protocol.EndpointLiveFrames+"/"+short+"/frames", "", nil); status != 401 {
		t.Fatalf("status %d without a token", status)
	}
	// The test rig has no browser: a watcher that is allowed to look is told so, before the stream starts.
	if status, _, _ := raw(t, test, "GET", protocol.EndpointLiveFrames+"/"+short+"/frames?token="+Token, "", nil); status != 404 {
		t.Fatalf("status %d with no browser", status)
	}
}

// keptBrowser is a browser whose live pages the test can drive itself, as an agent's browse calls would.
type keptBrowser struct {
	pages *browser.Pages
}

func (keptBrowser) Capture(context.Context, browser.ShotRequest) browser.ShotOutcome {
	return browser.ShotOutcome{}
}

func (k keptBrowser) Pages() *browser.Pages { return k.pages }

func TestTheLiveFramesFollowThePageASessionOpened(t *testing.T) {
	home, _ := os.UserHomeDir()
	found := browser.Find(home)
	if found == nil {
		t.Skip("no browser here: npm run e2e:install")
	}
	pages := found.Pages()
	t.Cleanup(pages.CloseAll)
	pageServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("content-type", "text/html")
		_, _ = response.Write([]byte("<!doctype html><title>Watched</title><h1>Watched</h1>"))
	}))
	t.Cleanup(pageServer.Close)
	test := Start(t, Options{Browser: keptBrowser{pages: pages}})
	short := liveSession(t, test)
	summaryID, ok := test.App.Sessions.Resolve(short)
	if !ok {
		t.Fatalf("the short name %q resolves no session", short)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := pages.Session(summaryID).Open(ctx, pageServer.URL+"/", browser.Viewport{Height: 900, Width: 1440}, "Watched"); err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequestWithContext(ctx, "GET", test.Server.URL+protocol.EndpointLiveFrames+"/"+short+"/frames?token="+Token, nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("content-type"), "text/event-stream") {
		t.Fatalf("status %d, %v", response.StatusCode, response.Header)
	}
	lines := bufio.NewScanner(response.Body)
	for lines.Scan() {
		if strings.HasPrefix(lines.Text(), "data: data:image/jpeg;base64,") {
			return
		}
	}
	t.Fatalf("no frame came: %v", lines.Err())
}
