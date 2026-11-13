package tailnet

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	connected = `{"BackendState":"Running","CertDomains":["laptop.tail1234.ts.net"]}`
	nothing   = `{}`
	serving   = `{"Web":{"laptop.tail1234.ts.net:4500":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:4500"}}},"laptop.tail1234.ts.net:443":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:8800"}}}}}`
)

// tailscale is a command line that answers each command as given, and keeps what it was asked.
type tailscale struct {
	answers map[string]string
	fails   map[string]error
	asked   []string
}

func (t *tailscale) run(_ context.Context, args ...string) ([]byte, error) {
	command := strings.Join(args, " ")
	t.asked = append(t.asked, command)
	return []byte(t.answers[command]), t.fails[command]
}

func TestAComputerWithoutTailscaleOrOffItsTailnetHasNoAddress(t *testing.T) {
	missing := &tailscale{fails: map[string]error{"status --json": exec.ErrNotFound}}
	if state := Read(context.Background(), missing.run, 4500); state.Available || state.Problem != notInstalledOn(runtime.GOOS) {
		t.Fatalf("without Tailscale: %+v", state)
	}
	stopped := &tailscale{answers: map[string]string{"status --json": `{"BackendState":"Stopped"}`}}
	if state := Read(context.Background(), stopped.run, 4500); state.Available || state.Problem != notConnected {
		t.Fatalf("off the tailnet: %+v", state)
	}
	plain := &tailscale{answers: map[string]string{"status --json": `{"BackendState":"Running"}`}}
	if state := Read(context.Background(), plain.run, 4500); state.Available || state.Problem != noHTTPS {
		t.Fatalf("without certificates: %+v", state)
	}
}

func TestTheAddressIsTheOneTailscaleServesThePortAtOrWould(t *testing.T) {
	served := &tailscale{answers: map[string]string{"status --json": connected, "serve status --json": serving}}
	state := Read(context.Background(), served.run, 4500)
	if !state.Available || !state.Served || state.Address != "https://laptop.tail1234.ts.net:4500" {
		t.Fatalf("served on its own port: %+v", state)
	}
	// The usual HTTPS port is left out of the address, and another port's site is not this swcode's.
	if state := Read(context.Background(), served.run, 8800); state.Address != "https://laptop.tail1234.ts.net" {
		t.Fatalf("served on the usual port: %+v", state)
	}
	if state := Read(context.Background(), served.run, 4600); state.Served || state.Address != "https://laptop.tail1234.ts.net:4600" {
		t.Fatalf("not served yet: %+v", state)
	}
}

func TestServingAsksTailscaleOnceAndSaysWhyItRefused(t *testing.T) {
	const serve = "serve --bg --https=4500 http://127.0.0.1:4500"
	refused := &tailscale{
		answers: map[string]string{"status --json": connected, "serve status --json": nothing, serve: "Access denied: serve config denied\nUse sudo."},
		fails:   map[string]error{serve: errors.New("exit status 1")},
	}
	state := Serve(context.Background(), refused.run, 4500)
	if state.Served || !strings.Contains(state.Problem, "Access denied: serve config denied") || strings.Contains(state.Problem, "sudo") {
		t.Fatalf("refused: %+v", state)
	}

	already := &tailscale{answers: map[string]string{"status --json": connected, "serve status --json": serving}}
	if state := Serve(context.Background(), already.run, 4500); !state.Served || strings.Join(already.asked, ", ") != "status --json, serve status --json" {
		t.Fatalf("already served: %+v after %v", state, already.asked)
	}
}

func TestTailscaleIsFoundWhereWindowsInstallsItWhenItIsNotOnThePath(t *testing.T) {
	nowhere := func(string) (string, error) { return "", exec.ErrNotFound }
	programFiles := func(name string) string {
		if name == "ProgramFiles" {
			return `C:\Program Files`
		}
		return ""
	}
	installed := filepath.Join(`C:\Program Files`, "Tailscale", "tailscale.exe")
	there := func(path string) bool { return path == installed }
	never := func(string) bool { return false }

	if found, err := programPath("linux", func(string) (string, error) { return "/usr/bin/tailscale", nil }, programFiles, never); err != nil || found != "/usr/bin/tailscale" {
		t.Fatalf("on the PATH: %q, %v", found, err)
	}
	if found, err := programPath("windows", nowhere, programFiles, there); err != nil || found != installed {
		t.Fatalf("installed on Windows, off the PATH: %q, %v", found, err)
	}
	if _, err := programPath("windows", nowhere, programFiles, never); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("not installed on Windows: %v", err)
	}
	// Another system has no such folder to look in.
	if _, err := programPath("linux", nowhere, programFiles, there); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("not installed on Linux: %v", err)
	}
}

func TestAWindowsComputerWithoutTailscaleIsToldWhatATailscaleInWSLDoesNotGiveIt(t *testing.T) {
	if said := notInstalledOn("windows"); !strings.Contains(said, "inside WSL") || !strings.Contains(said, "Settings → Computers") {
		t.Fatalf("on Windows: %q", said)
	}
	if said := notInstalledOn("linux"); said != notInstalled || strings.Contains(said, "WSL") {
		t.Fatalf("on Linux: %q", said)
	}
}
