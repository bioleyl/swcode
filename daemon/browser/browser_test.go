package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

const tallPage = `<!doctype html><html><head><title>Tall page</title></head><body style="margin:0">
<div style="height:2500px;background:linear-gradient(#0aa,#a0a)">Welcome</div>
<script>setTimeout(() => document.body.insertAdjacentHTML('beforeend', '<p>Ready now</p>'), 200);
console.error('Failed to load the avatar'); missing.call();</script></body></html>`

func setUp(t *testing.T) (Browser, string) {
	t.Helper()
	home, _ := os.UserHomeDir()
	found := Find(home)
	if found == nil {
		t.Skip("no browser here: npm run e2e:install")
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("content-type", "text/html")
		_, _ = response.Write([]byte(tallPage))
	}))
	t.Cleanup(server.Close)
	return found, server.URL + "/"
}

func request(url string) ShotRequest {
	return ShotRequest{Timeout: 15 * time.Second, URL: url, Viewport: Viewport{Height: 900, Width: 1440}}
}

func TestTheBrowserDrawsAPageAtTheSizeAskedWithTheErrorsItLogged(t *testing.T) {
	browser, url := setUp(t)
	asked := request(url)
	asked.WaitFor = "Ready now"
	drawn := browser.Capture(context.Background(), asked)
	if !drawn.OK || drawn.Shot.Width != 1440 || drawn.Shot.Height != 900 || drawn.Shot.Title != "Tall page" {
		t.Fatalf("drawn %+v", drawn.Problem)
	}
	if width, height := PNGSize(drawn.Shot.PNG); width != 1440 || height != 900 {
		t.Fatalf("png %dx%d", width, height)
	}
	if !reflect.DeepEqual(drawn.Shot.Errors, []string{"Failed to load the avatar", "missing is not defined"}) {
		t.Fatalf("errors %q", drawn.Shot.Errors)
	}
}

func TestTheBrowserDrawsAPhonesWindowAndAWholePageUpToItsLimit(t *testing.T) {
	browser, url := setUp(t)
	asked := request(url)
	asked.FullPage = true
	asked.Viewport = Viewport{Height: 844, Mobile: true, Width: 390}
	drawn := browser.Capture(context.Background(), asked)
	if !drawn.OK || drawn.Shot.Width != 390 || drawn.Shot.Height <= 2400 {
		t.Fatalf("drawn %dx%d %s", drawn.Shot.Width, drawn.Shot.Height, drawn.Problem)
	}
}

func TestTheBrowserSaysWhatWentWrongWithAPageThatDoesNotAnswer(t *testing.T) {
	browser, _ := setUp(t)
	drawn := browser.Capture(context.Background(), request("http://127.0.0.1:9/"))
	if drawn.OK || !strings.Contains(drawn.Problem, "could not be drawn") {
		t.Fatalf("drawn %+v", drawn)
	}
}

const orderPage = `<!doctype html><html><head><title>Order</title></head><body>
<button id="menu" onclick="document.getElementById('items').hidden = false">Menu</button>
<ul id="items" hidden><li><button>Item</button></li></ul>
<label for="mail">Email</label><input id="mail">
<button id="send" disabled>Envoyer</button>
<script>
document.getElementById('mail').addEventListener('input', (event) => {
	document.getElementById('send').disabled = event.target.value === '';
});
</script></body></html>`

// pageBrowser is the browser to act on pages with, or a skip where there is none.
func pageBrowser(t *testing.T) *Pages {
	t.Helper()
	home, _ := os.UserHomeDir()
	found := Find(home)
	if found == nil {
		t.Skip("no browser here: npm run e2e:install")
	}
	pages := found.Pages()
	t.Cleanup(pages.CloseAll)
	return pages
}

// numbered is the number a snapshot gave the element with that name.
func numbered(t *testing.T, state PageState, name string) int {
	t.Helper()
	for index, element := range state.Elements {
		if element.Name == name {
			return index + 1
		}
	}
	t.Fatalf("no element %q in %+v", name, state.Elements)
	return 0
}

func TestAPageStaysOpenWhileItsElementsAreClickedAndFilledByNumber(t *testing.T) {
	pages := pageBrowser(t)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("content-type", "text/html")
		_, _ = response.Write([]byte(orderPage))
	}))
	t.Cleanup(server.Close)
	session := pages.Session("s1")
	ctx := context.Background()
	state, err := session.Open(ctx, server.URL+"/", Viewport{Height: 900, Width: 1440}, "Menu")
	if err != nil {
		t.Fatal(err)
	}
	if state.Title != "Order" || numbered(t, state, "Menu") == 0 || numbered(t, state, "Email") == 0 {
		t.Fatalf("state %+v", state)
	}
	if state.Elements[numbered(t, state, "Envoyer")-1].Disabled != true {
		t.Fatalf("Envoyer is enabled: %+v", state.Elements)
	}
	opened, err := session.Click(ctx, numbered(t, state, "Menu"))
	if err != nil {
		t.Fatal(err)
	}
	if numbered(t, opened, "Item") == 0 {
		t.Fatalf("the menu did not open: %+v", opened.Elements)
	}
	filled, err := session.Fill(ctx, numbered(t, opened, "Email"), "a@b.c")
	if err != nil {
		t.Fatal(err)
	}
	send := filled.Elements[numbered(t, filled, "Envoyer")-1]
	if send.Disabled || filled.Elements[numbered(t, filled, "Email")-1].Value != "a@b.c" {
		t.Fatalf("filled %+v", filled.Elements)
	}
	markup, err := session.HTML(ctx)
	if err != nil || !strings.Contains(markup, `id="mail"`) {
		t.Fatalf("html %q, %v", markup, err)
	}
	frame, open, err := session.Frame(ctx)
	if err != nil || !open || len(frame) < 2 || frame[0] != 0xff || frame[1] != 0xd8 {
		t.Fatalf("frame %d bytes, open %v, %v", len(frame), open, err)
	}
}

func TestAPageSaysWhenNothingIsOpenYet(t *testing.T) {
	pages := NewPages("")
	session := pages.Session("s1")
	if _, err := session.Snapshot(context.Background()); err == nil || !strings.Contains(err.Error(), "No page is open") {
		t.Fatalf("snapshot %v", err)
	}
	if _, err := session.Click(context.Background(), 1); err == nil || !strings.Contains(err.Error(), "No page is open") {
		t.Fatalf("click %v", err)
	}
	if frame, open, err := session.Frame(context.Background()); frame != nil || open || err != nil {
		t.Fatalf("frame %v, %v, %v", frame, open, err)
	}
}
