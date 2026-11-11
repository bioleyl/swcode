package cli

import (
	"strings"
	"testing"
)

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
