package cli

import (
	"strings"
	"testing"
	"time"
)

func TestSwcodeRunTakesItsFlagsAndOnePrompt(t *testing.T) {
	plain := Parse([]string{"run", "Fix the failing test"})
	if want := (Command{Kind: KindRun, Run: RunOptions{Permissions: "auto", Prompt: "Fix the failing test", Timeout: 30 * time.Minute}}); plain != want {
		t.Fatalf("got %+v", plain)
	}
	full := Parse([]string{"run", "--json", "--cwd", "/srv/app", "--mode", "ask", "--timeout", "90s", "Fix it"})
	want := Command{Kind: KindRun, Run: RunOptions{Cwd: "/srv/app", JSON: true, Permissions: "ask", Prompt: "Fix it", Timeout: 90 * time.Second}}
	if full != want {
		t.Fatalf("got %+v", full)
	}
	// What is not understood asks for help, and ends with the exit code of a usage error.
	for _, arguments := range [][]string{
		{"run"}, {"run", " "}, {"run", "one", "two"}, {"run", "--mode", "yolo", "Fix it"}, {"run", "--timeout", "soon", "Fix it"},
		{"run", "--cwd"}, {"run", "--fast", "Fix it"}, {"run", "--timeout", "-5s", "Fix it"},
	} {
		if got := Parse(arguments); got.Kind != KindHelp || got.Problem == "" || got.Exit != UsageError {
			t.Errorf("%q: got %+v", arguments, got)
		}
	}
}

func TestTheCommandLine(t *testing.T) {
	cases := map[string]Command{
		"":                  {Kind: KindOpen},
		"serve":             {Kind: KindServe, Mode: ModeTerminal},
		"serve --service":   {Kind: KindServe, Mode: ModeService},
		"daemon":            {Kind: KindServe, Mode: ModeBackground},
		"start":             {Kind: KindStart},
		"stop":              {Kind: KindStop},
		"service install":   {Kind: KindServiceInstall},
		"service uninstall": {Kind: KindServiceUninstall},
		"--update":          {Kind: KindUpdate},
		"--version":         {Kind: KindVersion},
	}
	for line, want := range cases {
		if got := Parse(strings.Fields(line)); got != want {
			t.Errorf("%q: got %+v", line, got)
		}
	}
	for _, line := range []string{
		"fly", "serve now", "start now", "service", "service restart", "--update now", "remote me@box",
	} {
		if got := Parse(strings.Fields(line)); got.Kind != KindHelp || got.Problem == "" {
			t.Errorf("%q: got %+v", line, got)
		}
	}
}
