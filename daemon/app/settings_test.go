package app_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"swcode/daemon/fakellama"
	"swcode/daemon/pairing"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

func TestTheSettingsAreSavedToTheFileThenTheDaemonRestartsOnThem(t *testing.T) {
	test := Start(t, Options{})
	// The llama-server from before models.json stays in the file, which the models start from.
	settingsFile := filepath.Join(test.DataDir, "settings.env")
	_ = os.WriteFile(settingsFile, []byte("LLAMA_URL=http://192.168.1.119:8080\nLLAMA_SLOT_SHARE=2/2\n"), 0o644)
	status, saved := test.Call("PUT", protocol.EndpointSettings, map[string]string{
		"SWCODE_IMAGE_URL": "http://garfield.home:8090", "SWCODE_THINKING": "low",
	})
	if status != 200 {
		t.Fatalf("status %d: %v", status, saved)
	}
	Expect(t, saved, `{"ok":true,"restart":"self"}`)
	test.WaitForRestart()
	file, _ := os.ReadFile(settingsFile)
	for _, line := range []string{"SWCODE_THINKING=low\n", "LLAMA_URL=http://192.168.1.119:8080\n", "LLAMA_SLOT_SHARE=2/2\n"} {
		if !strings.Contains(string(file), line) {
			t.Fatalf("file %s", file)
		}
	}
	_, view := test.Call("GET", protocol.EndpointSettings, nil)
	Expect(t, view, `{"environment":[],"restart":"self","values":{"SWCODE_IMAGE_URL":"http://garfield.home:8090","SWCODE_THINKING":"low"}}`)
}

func TestTheSettingsShowWhatTheEnvironmentSets(t *testing.T) {
	test := Start(t, Options{Environment: map[string]string{"SWCODE_THINKING": "medium"}})
	_, view := test.Call("GET", protocol.EndpointSettings, nil)
	Expect(t, view, `{"environment":["SWCODE_THINKING"],"values":{"SWCODE_THINKING":"medium"}}`)
}

func TestSettingsADaemonWouldNotStartWithAreRefusedSayingWhy(t *testing.T) {
	test := Start(t, Options{})
	status, refused := test.Call("PUT", protocol.EndpointSettings, map[string]string{"SWCODE_IMAGE_URL": "garfield"})
	if status != 400 || !regexp.MustCompile(`SWCODE_IMAGE_URL`).MatchString(refused.(map[string]any)["problem"].(string)) {
		t.Fatalf("status %d: %v", status, refused)
	}
}

func TestTheSettingsWaitWhileASessionWorks(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{call("bash", map[string]any{"command": "ls"})}})
	test.NewSession(sessions.Request{Mode: protocol.PermissionModeAsk, Prompt: "List"})
	test.WaitForEvent("permissionRequested", 1)
	_, refused := test.Call("PUT", protocol.EndpointSettings, map[string]string{"SWCODE_THINKING": "low"})
	if !strings.Contains(refused.(map[string]any)["problem"].(string), "is working") {
		t.Fatalf("got %v", refused)
	}
}

func TestTheModelsAtAnAddressAreListedBeforeItIsSaved(t *testing.T) {
	test := Start(t, Options{})
	status, found := test.Call("POST", protocol.EndpointSettingsModels, map[string]string{"url": test.Llama.URL})
	if status != 200 {
		t.Fatalf("status %d", status)
	}
	Expect(t, found, map[string]any{"models": []any{Model}, "ok": true})
	_, refused := test.Call("POST", protocol.EndpointSettingsModels, map[string]string{"url": "http://127.0.0.1:9"})
	Expect(t, refused, `{"ok":false}`)
}

func TestAPhoneTradesACodeOnceForATokenOfItsOwn(t *testing.T) {
	clock := time.Date(2026, 10, 24, 22, 0, 0, 0, time.UTC)
	ids := 0
	newPairing := func() *pairing.Pairing {
		return pairing.New(filepath.Join(t.TempDir(), "devices.json"), func() time.Time { return clock }, func() string {
			ids++
			return "phone-" + string(rune('0'+ids))
		})
	}
	daemon := newPairing()
	issued := daemon.Issue()
	if issued.ExpiresAt != "2026-10-24T22:01:00.000Z" {
		t.Fatalf("expires %s", issued.ExpiresAt)
	}
	token := daemon.Redeem(issued.Code, "iPhone")
	if !regexp.MustCompile(`^[\w-]{40,}$`).MatchString(token) || !daemon.Knows(token) || daemon.Knows("a guess") || daemon.Redeem(issued.Code, "iPhone") != "" {
		t.Fatal("pairing")
	}
	Expect(t, daemon.Devices(), []any{map[string]any{"id": "phone-1", "name": "iPhone", "pairedAt": "2026-10-24T22:00:00.000Z"}})
	late := daemon.Issue()
	clock = clock.Add(pairing.CodeLifetime)
	if daemon.Redeem(late.Code, "") != "" || daemon.Redeem("made up", "") != "" {
		t.Fatal("an old code passed")
	}
}

func TestPairedPhonesAreKeptAcrossRestartsTheirTokensAsHashesOnly(t *testing.T) {
	file := filepath.Join(t.TempDir(), "devices.json")
	now := func() time.Time { return time.Date(2026, 10, 24, 22, 0, 0, 0, time.UTC) }
	first := pairing.New(file, now, func() string { return "phone-1" })
	token := first.Redeem(first.Issue().Code, "")
	data, _ := os.ReadFile(file)
	if strings.Contains(string(data), token) {
		t.Fatal("the token is kept as it is")
	}
	second := pairing.New(file, now, func() string { return "phone-2" })
	Expect(t, second.Devices(), `[{"name":"A phone"}]`)
	if !second.Knows(token) || !second.Forget("phone-1") || second.Knows(token) || second.Forget("phone-1") {
		t.Fatal("forgetting")
	}
}
