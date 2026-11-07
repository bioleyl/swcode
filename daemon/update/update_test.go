package update_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"swcode/daemon/protocol"
	"swcode/daemon/update"
)

const (
	programFile = "swcode-linux-amd64"
	releaseTag  = "v0.4.0"
	etag        = `"release-0.4.0"`
	notes       = "## Highlights\n\n- Updates itself."
	waitLimit   = 5 * time.Second
)

var newProgram = []byte("swcode 0.4.0")

// fakeGitHub is GitHub's API for the latest release, and the release's files.
type fakeGitHub struct {
	server *httptest.Server
	mu     sync.Mutex
	// What the API answers: the release, or this status when it is not 0.
	tag    string
	status int
	// The checksum the release lists for the program, its own when empty.
	listed string
	// Each request to the API, by the tag it said it had.
	asked chan string
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	fake := &fakeGitHub{asked: make(chan string, 16), tag: releaseTag}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /latest", fake.latest)
	mux.HandleFunc("GET /files/"+programFile, func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write(newProgram)
	})
	mux.HandleFunc("GET /files/SHA256SUMS", func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(fake.checksum() + "  " + programFile + "\n"))
	})
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeGitHub) latest(response http.ResponseWriter, request *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked <- request.Header.Get("if-none-match")
	if f.status != 0 {
		response.WriteHeader(f.status)
		return
	}
	if request.Header.Get("if-none-match") == etag {
		response.WriteHeader(http.StatusNotModified)
		return
	}
	response.Header().Set("etag", etag)
	_ = json.NewEncoder(response).Encode(map[string]any{
		"assets": []map[string]string{
			{"browser_download_url": f.server.URL + "/files/" + programFile, "name": programFile},
			{"browser_download_url": f.server.URL + "/files/SHA256SUMS", "name": "SHA256SUMS"},
		},
		"body": notes, "html_url": "https://github.com/bioleyl/swcode/releases/tag/" + f.tag, "tag_name": f.tag,
	})
}

func (f *fakeGitHub) checksum() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listed != "" {
		return f.listed
	}
	sum := sha256.Sum256(newProgram)
	return hex.EncodeToString(sum[:])
}

func (f *fakeGitHub) list(checksum string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listed = checksum
}

func (f *fakeGitHub) answer(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = status
}

// installedProgram is the program an update replaces, in a folder of its own.
func installedProgram(t *testing.T) update.Program {
	path := filepath.Join(t.TempDir(), "swcode")
	if err := os.WriteFile(path, []byte("swcode 0.3.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	return update.Program{File: programFile, Path: path}
}

func newService(t *testing.T, fake *fakeGitHub, version string, program update.Program) *update.Service {
	return update.NewService(update.Deps{
		File: filepath.Join(t.TempDir(), "updates.json"), HTTP: fake.server.Client(), LogError: func(error) {},
		Now:     func() time.Time { return time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC) },
		Program: program, Restart: protocol.RestartSelf, Source: fake.server.URL + "/latest", Version: version,
	})
}

func TestAReleaseIsNewerByItsNumbersNotItsLetters(t *testing.T) {
	cases := []struct {
		release, running string
		newer            bool
	}{
		{"v0.4.0", "0.3.0+7bd0cd6", true},
		{"v0.3.0", "0.3.0+7bd0cd6", false},
		{"v0.3.1", "0.3.0", true},
		{"v0.10.0", "0.9.0", true},
		{"v1.0.0", "0.10.2", true},
		{"v0.2.9", "0.3.0", false},
		{"v0.4.0", "dev", false},
		{"nightly", "0.3.0", false},
	}
	for _, each := range cases {
		if update.Newer(each.release, each.running) != each.newer {
			t.Errorf("Newer(%s, %s) should be %v", each.release, each.running, each.newer)
		}
	}
}

func TestABuildFromTheSourcesNeverLooks(t *testing.T) {
	fake := newFakeGitHub(t)
	service := newService(t, fake, "dev", installedProgram(t))
	view := service.Check(context.Background())
	if _, source := view.State.Value.(protocol.UpdateStateSource); !source {
		t.Fatalf("state %#v", view.State.Value)
	}
	if len(fake.asked) != 0 {
		t.Fatal("a build from the sources asked GitHub")
	}
}

func TestALookOffersTheNewerReleaseWithItsNotes(t *testing.T) {
	fake := newFakeGitHub(t)
	service := newService(t, fake, "0.3.0+7bd0cd6", installedProgram(t))
	if service.Offer() != "" {
		t.Fatal("nothing is offered before a look")
	}
	newer, found := service.Check(context.Background()).State.Value.(protocol.UpdateStateNewer)
	if !found || newer.Release.Version != "0.4.0" || newer.Release.Notes != notes || newer.At != "2026-11-07T10:00:00.000Z" {
		t.Fatalf("state %#v", newer)
	}
	if service.Offer() != "0.4.0" {
		t.Fatalf("offer %q", service.Offer())
	}
}

func TestTheLatestReleaseRunningIsCurrent(t *testing.T) {
	fake := newFakeGitHub(t)
	service := newService(t, fake, "0.4.0+1a2b3c4", installedProgram(t))
	if _, current := service.Check(context.Background()).State.Value.(protocol.UpdateStateCurrent); !current {
		t.Fatal("the latest release running should be current")
	}
	if service.Offer() != "" {
		t.Fatal("nothing newer to offer")
	}
}

func TestAnUnchangedReleaseIsAskedForByTheTagGitHubGave(t *testing.T) {
	fake := newFakeGitHub(t)
	service := newService(t, fake, "0.3.0", installedProgram(t))
	service.Check(context.Background())
	service.Check(context.Background())
	if first, second := <-fake.asked, <-fake.asked; first != "" || second != etag {
		t.Fatalf("asked with %q, then %q", first, second)
	}
	if service.Offer() != "0.4.0" {
		t.Fatal("an unchanged answer keeps the release found")
	}
}

func TestAFailedLookSaysWhyAndKeepsANewerVersionOnOffer(t *testing.T) {
	fake := newFakeGitHub(t)
	service := newService(t, fake, "0.3.0", installedProgram(t))
	service.Check(context.Background())
	fake.answer(http.StatusForbidden)
	if service.Check(context.Background()); service.Offer() != "0.4.0" {
		t.Fatal("one failed look took the newer version back")
	}
	failed, found := newService(t, fake, "0.3.0", installedProgram(t)).Check(context.Background()).State.Value.(protocol.UpdateStateFailed)
	if !found || failed.Problem != "GitHub answered 403 Forbidden" {
		t.Fatalf("state %#v", failed)
	}
}

func TestInstallPutsTheCheckedDownloadInPlaceOfTheProgram(t *testing.T) {
	fake := newFakeGitHub(t)
	program := installedProgram(t)
	service := newService(t, fake, "0.3.0", program)
	service.Check(context.Background())
	version, err := service.Install(context.Background())
	if err != nil || version != "0.4.0" {
		t.Fatalf("installed %q: %v", version, err)
	}
	if data, _ := os.ReadFile(program.Path); string(data) != string(newProgram) {
		t.Fatalf("the program is %q", data)
	}
	if _, installed := service.View().State.Value.(protocol.UpdateStateInstalled); !installed || service.Offer() != "" {
		t.Fatalf("state %#v", service.View().State.Value)
	}
	<-fake.asked
	service.Check(context.Background())
	if len(fake.asked) != 0 {
		t.Fatal("installed, it waits for the restart rather than looking again")
	}
}

func TestADownloadThatDoesNotMatchItsChecksumChangesNothing(t *testing.T) {
	fake := newFakeGitHub(t)
	fake.list(hex.EncodeToString(make([]byte, sha256.Size)))
	program := installedProgram(t)
	service := newService(t, fake, "0.3.0", program)
	service.Check(context.Background())
	if _, err := service.Install(context.Background()); err == nil {
		t.Fatal("a download that does not match its checksum was installed")
	}
	if data, _ := os.ReadFile(program.Path); string(data) != "swcode 0.3.0" {
		t.Fatalf("the program is %q", data)
	}
	if entries, _ := os.ReadDir(filepath.Dir(program.Path)); len(entries) != 1 {
		t.Fatalf("the download was left beside the program: %v", entries)
	}
	if service.Offer() != "0.4.0" {
		t.Fatal("the newer version should stay on offer")
	}
}

func TestInstallNeedsANewerVersionFoundAndAProgramToReplace(t *testing.T) {
	fake := newFakeGitHub(t)
	if _, err := newService(t, fake, "0.3.0", installedProgram(t)).Install(context.Background()); err == nil {
		t.Fatal("installed without a newer version found")
	}
	service := newService(t, fake, "0.3.0", update.Program{})
	service.Check(context.Background())
	if _, err := service.Install(context.Background()); err == nil {
		t.Fatal("installed without a program to replace")
	}
}

func TestLookingTurnedOffStaysOffAndTurnedOnLooksAtOnce(t *testing.T) {
	fake := newFakeGitHub(t)
	file := filepath.Join(t.TempDir(), "data", "updates.json")
	deps := update.Deps{
		File: file, HTTP: fake.server.Client(), LogError: func(error) {}, Now: time.Now,
		Restart: protocol.RestartPerson, Source: fake.server.URL + "/latest", Version: "0.3.0",
	}
	if err := update.NewService(deps).SetChecks(false); err != nil {
		t.Fatal(err)
	}
	service := update.NewService(deps)
	if service.View().Checks {
		t.Fatal("looking should stay off from one start to the next")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go service.Watch(ctx)
	if err := service.SetChecks(true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fake.asked:
	case <-time.After(waitLimit):
		t.Fatal("turned on, it did not look")
	}
}
