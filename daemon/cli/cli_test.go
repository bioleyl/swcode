package cli

import (
	"strings"
	"testing"
)

func TestTheCommandLine(t *testing.T) {
	cases := map[string]Command{
		"":                               {Kind: KindOpen},
		"serve":                          {Kind: KindServe, Mode: ModeTerminal},
		"daemon":                         {Kind: KindServe, Mode: ModeBackground},
		"stop":                           {Kind: KindStop},
		"--version":                      {Kind: KindVersion},
		"remote me@box":                  {Host: "me@box", Kind: KindRemote, LocalPort: DefaultLocalPort},
		"remote -w ~/app me@box -p 4600": {Host: "me@box", Kind: KindRemote, LocalPort: 4600, Workspace: "~/app"},
		"remote me@box --llama-url http://x:8080": {Host: "me@box", Kind: KindRemote, LlamaURL: "http://x:8080", LocalPort: DefaultLocalPort},
	}
	for line, want := range cases {
		if got := Parse(strings.Fields(line)); got != want {
			t.Errorf("%q: got %+v", line, got)
		}
	}
	for _, line := range []string{"fly", "serve now", "remote", "remote a b", "remote me@box -p 99999"} {
		if got := Parse(strings.Fields(line)); got.Kind != KindHelp || got.Problem == "" {
			t.Errorf("%q: got %+v", line, got)
		}
	}
}
