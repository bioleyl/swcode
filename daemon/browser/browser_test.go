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
