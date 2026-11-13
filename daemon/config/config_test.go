package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var place = Defaults{Cwd: "/work", Home: "/home/me"}

func TestTheDefaults(t *testing.T) {
	config, err := Load(map[string]string{"SWCODE_SSH_ENDPOINTS": "me@box, gmktec  laptop"}, place)
	if err != nil {
		t.Fatal(err)
	}
	if config.DataDir != filepath.Join("/home/me", ".swcode") || config.Port != 4500 || config.Workspace != "/work" {
		t.Fatalf("got %+v", config)
	}
	if !reflect.DeepEqual(config.SSHEndpoints, []string{"me@box", "gmktec", "laptop"}) {
		t.Fatalf("endpoints %v", config.SSHEndpoints)
	}
	if config.Thinking != "full" || config.SubagentThinking != "low" || config.LlamaURL != "http://127.0.0.1:8080" {
		t.Fatalf("got %+v", config)
	}
	if config.MaxSteps != 150 || config.SubagentMaxSteps != 30 {
		t.Fatalf("steps %d, subagents' %d", config.MaxSteps, config.SubagentMaxSteps)
	}
}

func TestABadValueSaysWhichSetting(t *testing.T) {
	_, err := Load(map[string]string{"SWCODE_PORT": "99999", "LLAMA_URL": "gmktec", "SWCODE_THINKING": "lots", "SWCODE_SUBAGENT_MAX_STEPS": "0"}, place)
	for _, name := range []string{"SWCODE_PORT", "LLAMA_URL", "SWCODE_THINKING", "SWCODE_SUBAGENT_MAX_STEPS"} {
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s not named in %v", name, err)
		}
	}
}

func TestTheSettingsFileGivesTheDaemonItsSettingsTheEnvironmentsOwnOnTop(t *testing.T) {
	data := t.TempDir()
	text := "# kept by swcode install\nLLAMA_URL=http://gmktec:8080\nSWCODE_PORT=4500\n"
	if err := os.WriteFile(filepath.Join(data, "settings.env"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	merged := Environment(map[string]string{"SWCODE_DATA_DIR": data, "SWCODE_PORT": "4600"}, "/home/me")
	if merged["LLAMA_URL"] != "http://gmktec:8080" || merged["SWCODE_PORT"] != "4600" {
		t.Fatalf("got %v", merged)
	}
	missing := Environment(map[string]string{"SWCODE_DATA_DIR": "/nowhere"}, "/home/me")
	if !reflect.DeepEqual(missing, map[string]string{"SWCODE_DATA_DIR": "/nowhere"}) {
		t.Fatalf("got %v", missing)
	}
}

func TestTheSettingsFileIsWrittenQuotedWhenAValueNeedsIt(t *testing.T) {
	text := SettingsText(map[string]string{
		"HOME":         "/home/me",
		"LLAMA_MODEL":  "gpt-oss-120b",
		"LLAMA_URL":    "http://gmktec:8080",
		"SWCODE_TOKEN": "a b",
	})
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	want := []string{"LLAMA_URL=http://gmktec:8080", "LLAMA_MODEL=gpt-oss-120b", "SWCODE_TOKEN='a b'", ""}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("got %q", lines)
	}
}

func TestAWindowsPathKeepsItsBackslashes(t *testing.T) {
	path := `C:\Users\me\new\tools`
	if got := ParseEnv(SettingsText(map[string]string{"SWCODE_WORKSPACE": path}))["SWCODE_WORKSPACE"]; got != path {
		t.Fatalf("got %q", got)
	}
}

func TestParseEnvReadsQuotesAsNodeDoes(t *testing.T) {
	values := ParseEnv("export A=1 # one\nB=\"two\\nlines\"\nC='it\\n stays'\nD=\"it's\"\n#E=no\n")
	want := map[string]string{"A": "1", "B": "two\nlines", "C": `it\n stays`, "D": "it's"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("got %q", values)
	}
}

func TestAnInstallDevicesArePairedWithKeepsListeningOnTheNetworkOnce(t *testing.T) {
	// listens is the address the settings choose after the look: empty when they choose none.
	listens := func(home string) string {
		return ReadSettings(SettingsPathOf(map[string]string{}, home))[envHost]
	}
	dataDirOf := func(t *testing.T, devices, settings string) string {
		home := t.TempDir()
		dataDir := filepath.Join(home, DataFolder)
		if err := os.MkdirAll(dataDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if devices != "" {
			_ = os.WriteFile(filepath.Join(dataDir, DevicesFile), []byte(devices), 0o600)
		}
		if settings != "" {
			_ = os.WriteFile(filepath.Join(dataDir, SettingsFile), []byte(settings), 0o600)
		}
		return home
	}
	const paired = `[{"id":"p1","name":"iPhone","pairedAt":"2026-11-01T10:00:00.000Z","tokenHash":"aa"}]`

	// Paired with a phone, and no address chosen: it listened on the network, and goes on.
	kept := dataDirOf(t, paired, "")
	if err := ListenAsBefore(map[string]string{}, kept); err != nil || listens(kept) != everyInterface {
		t.Fatalf("paired: listens on %q, error %v", listens(kept), err)
	}
	// The person then chooses this computer: the next start leaves that alone.
	_ = WriteSettings(SettingsPathOf(map[string]string{}, kept), map[string]string{})
	if err := ListenAsBefore(map[string]string{}, kept); err != nil || listens(kept) != "" {
		t.Fatalf("after the person's choice: listens on %q, error %v", listens(kept), err)
	}

	// Nobody paired: it becomes this computer's alone, and a phone paired later over a tailnet does not open it.
	alone := dataDirOf(t, "", "")
	if err := ListenAsBefore(map[string]string{}, alone); err != nil || listens(alone) != "" {
		t.Fatalf("nobody paired: listens on %q, error %v", listens(alone), err)
	}
	_ = os.WriteFile(filepath.Join(alone, DataFolder, DevicesFile), []byte(paired), 0o600)
	if err := ListenAsBefore(map[string]string{}, alone); err != nil || listens(alone) != "" {
		t.Fatalf("paired after the look: listens on %q, error %v", listens(alone), err)
	}

	// A choice of this computer says what the default now says: its line goes. Another choice stays, and so does
	// one the environment makes.
	explicit := dataDirOf(t, paired, "SWCODE_HOST=127.0.0.1\n")
	if err := ListenAsBefore(map[string]string{}, explicit); err != nil || listens(explicit) != "" {
		t.Fatalf("this computer chosen: listens on %q, error %v", listens(explicit), err)
	}
	other := dataDirOf(t, paired, "SWCODE_HOST=192.168.1.20\n")
	if err := ListenAsBefore(map[string]string{}, other); err != nil || listens(other) != "192.168.1.20" {
		t.Fatalf("another address chosen: listens on %q, error %v", listens(other), err)
	}
	fromEnvironment := dataDirOf(t, paired, "")
	if err := ListenAsBefore(map[string]string{envHost: "127.0.0.1"}, fromEnvironment); err != nil || listens(fromEnvironment) != "" {
		t.Fatalf("chosen by the environment: listens on %q, error %v", listens(fromEnvironment), err)
	}
}
