package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const appPage = `<!doctype html><html><head><title>Counter</title></head><body>
<h1>Counter</h1>
<button id="inc">Increase</button>
<input id="name" aria-label="Name" type="text">
<select aria-label="Colour"><option>Red</option><option>Blue</option></select>
<p id="out">0</p>
<shadow-menu></shadow-menu>
<script>
class ShadowMenu extends HTMLElement {
  connectedCallback() {
    const root = this.attachShadow({mode: 'open'});
    root.innerHTML = '<button id="menu">Menu</button><input id="search" aria-label="Search">';
    root.getElementById('menu').onclick = () => { document.getElementById('out').textContent = 'menu'; };
  }
}
customElements.define('shadow-menu', ShadowMenu);
document.getElementById('inc').onclick = () => {
  const out = document.getElementById('out');
  out.textContent = String(Number(out.textContent) + 1);
};
fetch('http://127.0.0.1:9/nope').catch(() => {});
fetch('/missing').catch(() => {});
console.error('boom');
missing.call();
</script></body></html>`

func sessionsForTest(t *testing.T) *Sessions {
	t.Helper()
	home, _ := os.UserHomeDir()
	program := FindPath(home)
	if program == "" {
		t.Skip("no browser here: npm run e2e:install")
	}
	sessions := NewSessions(program, t.TempDir(), false)
	t.Cleanup(sessions.CloseAll)
	return sessions
}

func appServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("set-cookie", "kept=1; Path=/")
		if request.URL.Path == "/missing" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("content-type", "text/html")
		_, _ = response.Write([]byte(appPage))
	}))
	t.Cleanup(server.Close)
	return server
}

func itemOfKind(t *testing.T, snapshot Snapshot, kind string) SnapshotItem {
	t.Helper()
	for _, item := range snapshot.Items {
		if item.Kind == kind {
			return item
		}
	}
	t.Fatalf("no %s in %+v", kind, snapshot.Items)
	return SnapshotItem{}
}

func TestASessionKeepsItsPageAndStateAcrossCalls(t *testing.T) {
	sessions := sessionsForTest(t)
	server := appServer(t)
	ctx := context.Background()
	session := sessions.For("s1")

	opened, err := session.Open(ctx, server.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if opened.Title != "Counter" || opened.URL != server.URL+"/" || opened.Milliseconds < 0 {
		t.Fatalf("opened %+v", opened)
	}
	filtered, err := session.Snapshot(ctx, "Increase", "button")
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].Name != "Increase" {
		t.Fatalf("filtered snapshot %+v %v", filtered, err)
	}
	exerciseShadowRoot(t, session)
	exercisePage(t, session)
	capture(t, session)
	keepCookie(t, session, server.URL+"/second")
}

func exerciseShadowRoot(t *testing.T, session *Session) {
	t.Helper()
	ctx := context.Background()
	menu, err := session.Snapshot(ctx, "Menu", "button")
	if err != nil || len(menu.Items) != 1 {
		t.Fatalf("shadow menu %+v %v", menu, err)
	}
	search, err := session.Snapshot(ctx, "Search", "input")
	if err != nil || len(search.Items) != 1 {
		t.Fatalf("shadow search %+v %v", search, err)
	}
	if _, err := session.Click(ctx, menu.Items[0].Ref); err != nil {
		t.Fatal(err)
	}
	if value := eval(t, session, `document.getElementById('out').textContent`); value != "menu" {
		t.Fatalf("menu click out %q", value)
	}
	if _, err := session.page.Eval(`() => { document.getElementById('out').textContent = '0'; return 1 }`); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Type(ctx, search.Items[0].Ref, "hello"); err != nil {
		t.Fatal(err)
	}
	if value := eval(t, session, `document.querySelector('shadow-menu').shadowRoot.getElementById('search').value`); value != "hello" {
		t.Fatalf("shadow search value %q", value)
	}
}

func exercisePage(t *testing.T, session *Session) {
	t.Helper()
	ctx := context.Background()
	snapshot, err := session.Snapshot(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	input := itemOfKind(t, snapshot, "input")
	button := itemOfKind(t, snapshot, "button")
	selectItem := itemOfKind(t, snapshot, "select")
	if input.Ref == "" || button.Ref == "" || selectItem.Ref == "" || input.Name != "Name" || selectItem.Name != "Colour" {
		t.Fatalf("snapshot %+v", snapshot)
	}
	if _, err := session.Type(ctx, input.Ref, "Ada"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Type(ctx, selectItem.Ref, "Blue"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Click(ctx, button.Ref); err != nil {
		t.Fatal(err)
	}
	if value := eval(t, session, `document.getElementById('name').value`); value != "Ada" {
		t.Fatalf("name %q", value)
	}
	if value := eval(t, session, `document.querySelector('select').value`); value != "Blue" {
		t.Fatalf("colour %q", value)
	}
	if value := eval(t, session, `document.getElementById('out').textContent`); value != "1" {
		t.Fatalf("out %q", value)
	}
}

func capture(t *testing.T, session *Session) {
	t.Helper()
	shot, err := session.Screenshot(context.Background(), false)
	if err != nil || shot.Width != desktopWidth || shot.Height != desktopHeight || shot.Title != "Counter" {
		t.Fatalf("shot %+v %v", shot, err)
	}
}

func keepCookie(t *testing.T, session *Session, url string) {
	t.Helper()
	if _, err := session.Open(context.Background(), url); err != nil {
		t.Fatal(err)
	}
	if cookie := eval(t, session, `document.cookie`); !strings.Contains(cookie, "kept=1") {
		t.Fatalf("the session kept no cookie: %q", cookie)
	}
}

func eval(t *testing.T, session *Session, expression string) string {
	t.Helper()
	found, err := session.page.Eval(`() => ` + expression)
	if err != nil {
		t.Fatal(err)
	}
	return found.Value.Str()
}

func TestDiagnosticsAreCollectedWhileTheSessionRuns(t *testing.T) {
	sessions := sessionsForTest(t)
	server := appServer(t)
	session := sessions.For("s1")
	if _, err := session.Open(context.Background(), server.URL+"/"); err != nil {
		t.Fatal(err)
	}
	seen := waitForDiagnostics(session, func(report Report) bool {
		return hasDiagnostic(report, DiagnosticConsole, "boom") &&
			hasDiagnostic(report, DiagnosticException, "missing") &&
			hasDiagnostic(report, DiagnosticRequest, "127.0.0.1:9") &&
			hasDiagnostic(report, DiagnosticResponse, "404")
	})
	if !seen {
		t.Fatalf("diagnostics %+v", session.Diagnostics())
	}
}

func waitForDiagnostics(session *Session, wanted func(Report) bool) bool {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if wanted(session.Diagnostics()) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func hasDiagnostic(report Report, kind DiagnosticKind, text string) bool {
	for _, item := range report.Items {
		if item.Kind == kind && strings.Contains(item.Text, text) {
			return true
		}
	}
	return false
}

func TestAPageThatDisappearsSaysToOpenItAgain(t *testing.T) {
	sessions := sessionsForTest(t)
	server := appServer(t)
	session := sessions.For("s1")
	if _, err := session.Open(context.Background(), server.URL+"/"); err != nil {
		t.Fatal(err)
	}
	if err := session.page.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Snapshot(context.Background(), "", ""); err == nil || !strings.Contains(err.Error(), "disconnected") {
		t.Fatalf("err %v", err)
	}
}

func TestEachAgentSessionHasItsOwnBrowserProfile(t *testing.T) {
	sessions := sessionsForTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/set" {
			response.Header().Set("set-cookie", "kept=1; Path=/")
		}
		response.Header().Set("content-type", "text/html")
		_, _ = response.Write([]byte("<!doctype html><title>Cookie</title>"))
	}))
	t.Cleanup(server.Close)
	ctx := context.Background()

	first := sessions.For("s1")
	if _, err := first.Open(ctx, server.URL+"/set"); err != nil {
		t.Fatal(err)
	}
	second := sessions.For("s2")
	if _, err := second.Open(ctx, server.URL+"/"); err != nil {
		t.Fatal(err)
	}
	if cookie := eval(t, first, `document.cookie`); !strings.Contains(cookie, "kept=1") {
		t.Fatalf("the first session kept no cookie: %q", cookie)
	}
	if cookie := eval(t, second, `document.cookie`); strings.Contains(cookie, "kept=1") {
		t.Fatalf("the second session saw the first one's cookie: %q", cookie)
	}
}
