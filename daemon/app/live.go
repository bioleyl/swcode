package app

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"swcode/daemon/protocol"
)

const (
	// liveFrameEvery is how often a watcher gets the page again: often enough to follow a menu, few enough to keep
	// the daemon light.
	liveFrameEvery = 500 * time.Millisecond
	// noLivePage is what a watcher hears when there is no browser to watch a page with.
	noLivePage = "No browser to watch a page with here."
	// liveWaiting is the note a watcher reads before the agent opened a page.
	liveWaiting = "No page is open yet: the agent opens one with browse."
	// liveProblem is the note a watcher reads when the page cannot be drawn now, as it navigates.
	liveProblem = "The page could not be drawn: "
)

// livePageHTML is the page a person opens to watch a session's browser page: a picture the daemon sends again a
// few times a second, over a stream. It takes the token from the app's storage, so the person opens it in the
// browser they use swcode with.
const livePageHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>swcode · live page</title>
<style>
html, body { margin: 0; height: 100%; background: #111; color: #ddd; font: 14px system-ui, sans-serif; }
body { display: grid; place-items: center; }
img { max-width: 100vw; max-height: 100vh; display: block; }
p { padding: 1rem 2rem; text-align: center; background: #000a; border-radius: .5rem; }
</style></head><body>
<img id="page" alt="The page the agent acts on" hidden>
<p id="note">Connecting…</p>
<script>
(() => {
	const token = localStorage.getItem('{{token}}');
	const note = document.getElementById('note');
	const page = document.getElementById('page');
	const session = encodeURIComponent(location.pathname.split('/').pop());
	if (!token) {
		note.textContent = 'Open swcode with its link first: this window needs the token it keeps.';
		return;
	}
	const source = new EventSource('{{frames}}/' + session + '/frames?{{param}}=' + encodeURIComponent(token));
	source.addEventListener('frame', (event) => {
		page.src = event.data;
		page.hidden = false;
		note.hidden = true;
	});
	source.addEventListener('waiting', (event) => {
		page.hidden = true;
		note.hidden = false;
		note.textContent = event.data;
	});
	source.onerror = () => {
		page.hidden = true;
		note.hidden = false;
		note.textContent = 'The stream stopped: reopening it…';
	};
})();
</script>
</body></html>`

// livePage is the page with the daemon's own names in it, the token's key among them.
var livePage = strings.NewReplacer(
	"{{token}}", protocol.TokenStorageKey, "{{frames}}", protocol.EndpointLiveFrames, "{{param}}", protocol.TokenParam,
).Replace(livePageHTML)

// liveViewURL is where a person watches a session's page: this computer's own address, so the browser finds the
// token the app kept there.
func (a *App) liveViewURL(sessionID string) string {
	return LocalURL(a.config.Host, a.config.Port) + protocol.EndpointLive + "/" + sessionID
}

// liveRoutes serve the window a person watches a session's browser page in: the page, and its pictures as they
// come.
func (a *App) liveRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+protocol.EndpointLive+"/{session}", func(response http.ResponseWriter, request *http.Request) {
		if !sessionName.MatchString(request.PathValue("session")) {
			fail(response, http.StatusNotFound, noLivePage)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.Header().Set("cache-control", "no-store")
		_, _ = response.Write([]byte(livePage))
	})
	mux.HandleFunc("GET "+protocol.EndpointLiveFrames+"/{session}/frames", a.liveFrames)
}

// liveFrames sends a session's page as JPEG pictures, a few times a second, until the watcher leaves: server-sent
// events, so the page needs no script of its own to keep the stream.
func (a *App) liveFrames(response http.ResponseWriter, request *http.Request) {
	sessionID := request.PathValue("session")
	if !sessionName.MatchString(sessionID) || a.browserPages == nil {
		fail(response, http.StatusNotFound, noLivePage)
		return
	}
	response.Header().Set("content-type", "text/event-stream")
	response.Header().Set("cache-control", "no-store")
	response.WriteHeader(http.StatusOK)
	flusher := http.NewResponseController(response)
	frame := a.browserPages.Session(sessionID)
	lastNote := ""
	for {
		jpeg, open, err := frame.Frame(request.Context())
		note := liveWaiting
		if err != nil {
			note = liveProblem + err.Error()
		}
		if err != nil || !open {
			if note != lastNote {
				if _, err := fmt.Fprintf(response, "event: waiting\ndata: %s\n\n", strings.ReplaceAll(note, "\n", " ")); err != nil {
					return
				}
				if err := flusher.Flush(); err != nil {
					return
				}
				lastNote = note
			}
		} else {
			lastNote = ""
			if _, err := fmt.Fprintf(response, "event: frame\ndata: data:image/jpeg;base64,%s\n\n", base64.StdEncoding.EncodeToString(jpeg)); err != nil {
				return
			}
			if err := flusher.Flush(); err != nil {
				return
			}
		}
		select {
		case <-request.Context().Done():
			return
		case <-time.After(liveFrameEvery):
		}
	}
}
