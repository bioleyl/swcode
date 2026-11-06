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
}

func TestABadValueSaysWhichSetting(t *testing.T) {
	_, err := Load(map[string]string{"SWCODE_PORT": "99999", "LLAMA_URL": "gmktec", "SWCODE_THINKING": "lots"}, place)
	for _, name := range []string{"SWCODE_PORT", "LLAMA_URL", "SWCODE_THINKING"} {
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
