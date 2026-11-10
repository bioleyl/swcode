package terminals_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/terminals"
)

const waitLimit = 5 * time.Second

// oneHost hands every terminal this computer's host: what the service needs, without a project.
type oneHost struct{ host hosts.Host }

func (h oneHost) HostOf(protocol.HostRef) hosts.Host { return h.host }

// local is a service on this computer, in a folder of the test's.
func local(t *testing.T) (*terminals.Service, string) {
	t.Helper()
	home := t.TempDir()
	host := hosts.NewLocalHost(hosts.LocalOptions{Home: home, ScratchRoot: t.TempDir()})
	ids := 0
	service := terminals.NewService(terminals.Deps{Hosts: oneHost{host}, NewID: func() string {
		ids++
		return fmt.Sprintf("terminal-%d", ids)
	}})
	t.Cleanup(service.CloseAll)
	return service, t.TempDir()
}

// until waits for found to say yes, failing the test after a moment.
func until(t *testing.T, what string, found func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitLimit)
	for !found() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestATerminalRunsItsCommandAndEndsWithItsExitCode(t *testing.T) {
	service, folder := local(t)
	opened, refused := service.Open(context.Background(), terminals.Request{
		Cwd: folder, Command: "echo hello-from-the-terminal; exit 3", Host: protocol.LocalHost(), Title: "test",
	})
	if refused != "" {
		t.Fatal(refused)
	}
	until(t, "the command's output", func() bool {
		return strings.Contains(string(service.Output(opened.ID)), "hello-from-the-terminal")
	})
	until(t, "the shell to end", func() bool {
		state, found := service.Terminal(opened.ID)
		if !found {
			return false
		}
		_, exited := state.State.Value.(protocol.TerminalStateExited)
		return exited
	})
	state, _ := service.Terminal(opened.ID)
	if code := state.State.Value.(protocol.TerminalStateExited).Code; code != 3 {
		t.Fatalf("exit code %d, want 3", code)
	}
	if refusal := service.Write(opened.ID, "echo again\n"); refusal == "" {
		t.Fatal("an ended terminal took what was typed")
	}
}

func TestATerminalIsNamedByItsNumberInItsFolder(t *testing.T) {
	service, folder := local(t)
	first, _ := service.Open(context.Background(), terminals.Request{Cwd: folder, Host: protocol.LocalHost()})
	second, _ := service.Open(context.Background(), terminals.Request{Cwd: folder, Host: protocol.LocalHost()})
	if first.Title != "Terminal" || second.Title != "Terminal 2" {
		t.Fatalf("titles %q, %q", first.Title, second.Title)
	}
	service.Close(second.ID)
	third, _ := service.Open(context.Background(), terminals.Request{Cwd: folder, Host: protocol.LocalHost()})
	if third.Title != "Terminal 2" {
		t.Fatalf("title %q after the second closed, want Terminal 2", third.Title)
	}
}

func TestATerminalResizesKeepsWhatItPrintedAndIsClosedForGood(t *testing.T) {
	service, folder := local(t)
	opened, refused := service.Open(context.Background(), terminals.Request{
		Columns: 100, Cwd: folder, Host: protocol.LocalHost(), Rows: 30, Title: "resize",
	})
	if refused != "" {
		t.Fatal(refused)
	}
	if refusal := service.Resize(opened.ID, 120, 40); refusal != "" {
		t.Fatal(refusal)
	}
	until(t, "the prompt", func() bool { return len(service.Output(opened.ID)) > 0 })
	service.Close(opened.ID)
	if list := service.List(); len(list) != 0 {
		t.Fatalf("%d terminals after closing the only one", len(list))
	}
	if output := service.Output(opened.ID); output != nil {
		t.Fatalf("output of a closed terminal: %q", output)
	}
	if refusal := service.Resize(opened.ID, 80, 24); refusal == "" {
		t.Fatal("resizing a closed terminal said nothing")
	}
}

func TestATerminalTakesAnotherName(t *testing.T) {
	service, folder := local(t)
	opened, _ := service.Open(context.Background(), terminals.Request{Cwd: folder, Host: protocol.LocalHost()})
	if refusal := service.Rename(opened.ID, "dev server"); refusal != "" {
		t.Fatal(refusal)
	}
	if title := service.List()[0].Title; title != "dev server" {
		t.Fatalf("title %q", title)
	}
	if refusal := service.Rename("no-such-terminal", "dev server"); refusal == "" {
		t.Fatal("an unknown terminal took a name")
	}
}
