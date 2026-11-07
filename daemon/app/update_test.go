package app_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
	"swcode/daemon/update"
)

const (
	releaseFile = "swcode-test"
	oldProgram  = "swcode 0.3.0"
	newProgram  = "swcode 0.4.0"
)

// releases is GitHub's API for a latest release 0.4.0, and its files: the program and its checksum.
func releases(t *testing.T) string {
	sum := sha256.Sum256([]byte(newProgram))
	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /latest", func(response http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(response).Encode(map[string]any{
			"assets": []map[string]string{
				{"browser_download_url": server.URL + "/program", "name": releaseFile},
				{"browser_download_url": server.URL + "/sums", "name": "SHA256SUMS"},
			},
			"body": "Updates itself.", "html_url": "https://github.com/bioleyl/swcode/releases/tag/v0.4.0",
			"tag_name": "v0.4.0",
		})
	})
	mux.HandleFunc("GET /program", func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(newProgram))
	})
	mux.HandleFunc("GET /sums", func(response http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(response, "%s  %s\n", hex.EncodeToString(sum[:]), releaseFile)
	})
	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server.URL + "/latest"
}

// released is a daemon of release 0.3.0, whose program is a file of the test's.
func released(t *testing.T, replies []fakellama.Reply) (*App, string) {
	program := filepath.Join(t.TempDir(), "swcode")
	if err := os.WriteFile(program, []byte(oldProgram), 0o755); err != nil {
		t.Fatal(err)
	}
	test := Start(t, Options{
		Program: update.Program{File: releaseFile, Path: program}, Replies: replies,
		Settings: map[string]string{"SWCODE_RELEASES_URL": releases(t)}, Version: "0.3.0+7bd0cd6",
	})
	test.Until("the newer version to be found", func() bool {
		_, status := test.Call("GET", protocol.EndpointStatus, nil)
		return status.(map[string]any)["update"] == "0.4.0"
	})
	return test, program
}

func TestANewerReleaseIsOfferedThenInstalledAndTheDaemonStartsAgainOnIt(t *testing.T) {
	test, program := released(t, nil)
	_, view := test.Call("GET", protocol.EndpointUpdate, nil)
	Expect(t, view, `{"checks":true,"restart":"self","version":"0.3.0+7bd0cd6",`+
		`"state":{"kind":"newer","release":{"notes":"Updates itself.","version":"0.4.0"}}}`)
	status, installed := test.Call("POST", protocol.EndpointUpdateInstall, nil)
	if status != 200 {
		t.Fatalf("status %d: %v", status, installed)
	}
	Expect(t, installed, `{"ok":true,"restart":"self","version":"0.4.0"}`)
	test.WaitForUpdated()
	if data, _ := os.ReadFile(program); string(data) != newProgram {
		t.Fatalf("the program is %q", data)
	}
	_, after := test.Call("GET", protocol.EndpointStatus, nil)
	if _, offered := after.(map[string]any)["update"]; offered {
		t.Fatal("installed, the newer version is offered no more")
	}
}

func TestInstallingWaitsWhileASessionWorks(t *testing.T) {
	test, program := released(t, []fakellama.Reply{call("bash", map[string]any{"command": "ls"})})
	test.NewSession(sessions.Request{Mode: protocol.PermissionModeAsk, Prompt: "List"})
	test.WaitForEvent("permissionRequested", 1)
	status, refused := test.Call("POST", protocol.EndpointUpdateInstall, nil)
	if status != 400 || !strings.Contains(refused.(map[string]any)["problem"].(string), "is working") {
		t.Fatalf("status %d: %v", status, refused)
	}
	if data, _ := os.ReadFile(program); string(data) != oldProgram {
		t.Fatalf("the program is %q", data)
	}
}

func TestLookingEveryTenMinutesTurnsOffAndABuildFromTheSourcesNeverLooks(t *testing.T) {
	test := Start(t, Options{})
	_, view := test.Call("GET", protocol.EndpointUpdate, nil)
	Expect(t, view, `{"checks":true,"state":{"kind":"source"},"version":"dev"}`)
	_, changed := test.Call("PUT", protocol.EndpointUpdate, map[string]bool{"checks": false})
	Expect(t, changed, `{"checks":false}`)
	_, looked := test.Call("POST", protocol.EndpointUpdateCheck, nil)
	Expect(t, looked, `{"state":{"kind":"source"}}`)
	if _, err := os.Stat(filepath.Join(test.DataDir, "updates.json")); err != nil {
		t.Fatal("turning it off is kept for the next starts")
	}
}
