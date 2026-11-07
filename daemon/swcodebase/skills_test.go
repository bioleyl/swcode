package swcodebase

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"swcode/daemon/hosts"
)

// root is the repository's folder, from this package's, where go test runs.
var root = filepath.Join("..", "..")

func readJSON(t *testing.T, file string, value any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, file))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
}

// The repository publishes what the CLI installs: the marketplace lists the plugin of the skill's name, which holds
// the skill, and the plugin's version is the program's, since Claude Code updates a plugin only when it changes.
func TestTheRepositoryPublishesTheSkill(t *testing.T) {
	var marketplace struct {
		Plugins []struct{ Name, Source string }
	}
	readJSON(t, ".claude-plugin/marketplace.json", &marketplace)
	if len(marketplace.Plugins) != 1 || marketplace.Plugins[0].Name != Name || marketplace.Plugins[0].Source != "./plugins/"+Name {
		t.Fatalf("the marketplace lists %+v, not the plugin %s", marketplace.Plugins, Name)
	}
	var plugin, program struct{ Name, Version string }
	readJSON(t, filepath.Join("plugins", Name, ".claude-plugin", "plugin.json"), &plugin)
	readJSON(t, "package.json", &program)
	if plugin.Name != Name || plugin.Version != program.Version {
		t.Fatalf("plugin.json is %s %s: it should be %s %s, as package.json", plugin.Name, plugin.Version, Name, program.Version)
	}
	skill, err := os.ReadFile(filepath.Join(root, "plugins", Name, "skills", Name, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skill), "\nname: "+Name+"\n") {
		t.Fatalf("the plugin's SKILL.md is not named %s", Name)
	}
}

func exited(code int, output string) hosts.CommandResult {
	return hosts.CommandResult{ExitCode: &code, Output: output}
}

func TestTheCommandLinesNeedNoQuotes(t *testing.T) {
	cases := map[string]string{
		commandOf(adding, true):    "npx --yes skills@1.7.6 add bioleyl/swcode --skill swcode-base --agent universal --yes --global",
		commandOf(removing, false): "npx --yes skills@1.7.6 remove swcode-base --agent universal --yes",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestAProblemSaysWhyTheSkillsCLIDidNotDoIt(t *testing.T) {
	cases := []struct {
		name   string
		result hosts.CommandResult
		want   string
	}{
		{name: "done", result: exited(0, "Done!"), want: ""},
		{
			name:   "no npx",
			result: exited(commandNotFound, "bash: line 1: npx: command not found"),
			want:   "npx was not found on build-box: swcode-base is installed with the skills CLI, which needs Node.js 22.20 or later there.",
		},
		{
			name:   "timed out",
			result: hosts.CommandResult{TimedOut: true},
			want:   "npx skills could not remove swcode-base on build-box: it took longer than 5m0s.",
		},
		{
			name:   "its error",
			result: exited(1, "◇  Source: x\n│\n■  Failed to clone repository\n│  Authentication failed.\n│    - Retry with SSH\n└  Installation failed\n■  Canceled\n"),
			want:   "npx skills could not remove swcode-base on build-box: Failed to clone repository: Authentication failed.",
		},
		{
			name:   "no mark",
			result: exited(1, "\nnpx : The term 'npx' is not recognized\nAt line:1\n"),
			want:   "npx skills could not remove swcode-base on build-box: npx : The term 'npx' is not recognized",
		},
	}
	for _, test := range cases {
		if got := problemOf(test.result, removing, "build-box"); got != test.want {
			t.Errorf("%s: got %q, want %q", test.name, got, test.want)
		}
	}
}
