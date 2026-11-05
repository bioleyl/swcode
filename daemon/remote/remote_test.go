package remote

import (
	"reflect"
	"testing"

	"swcode/daemon/cli"
	"swcode/daemon/config"
)

func settings(t *testing.T, environment map[string]string) config.Config {
	loaded, err := config.Load(environment, config.Defaults{Cwd: "/work", Home: "/home/me"})
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func remote() cli.Command {
	return cli.Command{Host: "me@build-box", Kind: cli.KindRemote, LocalPort: cli.DefaultLocalPort}
}

func TestValuesAreQuotedAsOneShellWordALeadingTildeStillTheRemoteHome(t *testing.T) {
	cases := map[string]string{"it's": `'it'\''s'`, "~/projects/app": `"$HOME"/'projects/app'`, "~": `"$HOME"`}
	for value, want := range cases {
		if got := ShellWord(value); got != want {
			t.Errorf("%q: got %q", value, got)
		}
	}
}

func TestWhatTheRemoteMachineSaidAboutItselfIsRead(t *testing.T) {
	cases := map[string]Probe{
		"Linux x86_64\n0.1.0+abc\n": {Platform: "linux-amd64", Version: "0.1.0+abc"},
		"Darwin arm64\n":            {Platform: "darwin-arm64"},
		"Plan9 mips\n":              {},
	}
	for output, want := range cases {
		if got := ParseProbe(output); got != want {
			t.Errorf("%q: got %+v", output, got)
		}
	}
}

func TestTheRemoteDaemonGetsThisMachinesModelsAndAddressesThroughTheTunnels(t *testing.T) {
	loaded := settings(t, map[string]string{"LLAMA_API_KEY": "it's secret", "LLAMA_MODEL": "gpt-oss-120b", "LLAMA_URL": "http://gmktec:8080"})
	command := remote()
	command.Workspace = "~/app"
	want := map[string]string{
		"LLAMA_API_KEY": "it's secret", "LLAMA_MODEL": "gpt-oss-120b", "LLAMA_URL": "http://127.0.0.1:18080",
		"SWCODE_ENV": "production", "SWCODE_HOST": "127.0.0.1", "SWCODE_PORT": "4500",
		"SWCODE_PUBLIC_URL": "http://127.0.0.1:4510", "SWCODE_WORKSPACE": "~/app",
	}
	if got := Environment(command, loaded); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestTheDaemonIsForwardedHereAndLlamaServerThere(t *testing.T) {
	loaded := settings(t, map[string]string{"LLAMA_URL": "http://gmktec:8080"})
	arguments := SSHServeArguments(remote(), loaded)
	if !reflect.DeepEqual(arguments[:5], []string{"-tt", "-L", "4510:127.0.0.1:4500", "-R", "18080:gmktec:8080"}) || arguments[5] != "me@build-box" {
		t.Fatalf("got %v", arguments)
	}
	reaching := remote()
	reaching.LlamaURL = "http://gmktec:8080"
	if arguments := SSHServeArguments(reaching, loaded); arguments[3] != "me@build-box" {
		t.Fatalf("got %v", arguments)
	}
}
