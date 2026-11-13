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

func TestACommandTypedInATerminalIsWaitedForWhileThePersonAnswersItThere(t *testing.T) {
	service, folder := local(t)
	opened, refused := service.Open(context.Background(), terminals.Request{Cwd: folder, Host: protocol.LocalHost(), Title: "setup"})
	if refused != "" {
		t.Fatal(refused)
	}
	// A command that asks for a password, as sudo does: without echo, on a line of its own.
	asked := make(chan bool, 8)
	ran := make(chan terminals.Ran, 1)
	go func() {
		got, _ := service.Run(context.Background(), opened.ID, `read -r -s -p "Password: " secret; echo; echo "it was $secret"; (exit 4)`, func(waits bool) { asked <- waits })
		ran <- got
	}()
	select {
	case waits := <-asked:
		if !waits {
			t.Fatal("the command was said not to wait before it asked")
		}
	case <-time.After(waitLimit):
		t.Fatal("the password prompt was not seen")
	}
	if refusal := service.Write(opened.ID, "hunter2\n"); refusal != "" {
		t.Fatal(refusal)
	}
	select {
	case got := <-ran:
		output := string(got.Output)
		if !got.Ended || got.Code != 4 || !strings.Contains(output, "it was hunter2") || strings.Contains(output, "\x1b]777") {
			t.Fatalf("ended %v with code %d: %q", got.Ended, got.Code, output)
		}
	case <-time.After(waitLimit):
		t.Fatal("the command was not seen to end")
	}

	// The terminal runs on: the next command is typed in the same shell, which kept what the first one set.
	next, refusal := service.Run(context.Background(), opened.ID, `echo "still $secret"`, func(bool) {})
	if refusal != "" || !next.Ended || next.Code != 0 || !strings.Contains(string(next.Output), "still hunter2") {
		t.Fatalf("the next command: %+v, %q", next, refusal)
	}
}

func TestAWaitForACommandEndsWithItsTerminalOrWhenGivenUp(t *testing.T) {
	service, folder := local(t)
	opened, _ := service.Open(context.Background(), terminals.Request{Cwd: folder, Host: protocol.LocalHost(), Title: "setup"})
	closed := make(chan terminals.Ran, 1)
	go func() {
		got, _ := service.Run(context.Background(), opened.ID, "sleep 30", func(bool) {})
		closed <- got
	}()
	until(t, "the command to be typed", func() bool { return strings.Contains(string(service.Output(opened.ID)), "sleep 30") })
	service.Close(opened.ID)
	select {
	case got := <-closed:
		if got.Ended || !got.Closed {
			t.Fatalf("after its terminal closed: %+v", got)
		}
	case <-time.After(waitLimit):
		t.Fatal("the wait went on after its terminal closed")
	}

	again, _ := service.Open(context.Background(), terminals.Request{Cwd: folder, Host: protocol.LocalHost(), Title: "setup"})
	short, stop := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer stop()
	if got, refusal := service.Run(short, again.ID, "sleep 30", func(bool) {}); refusal != "" || got.Ended || got.Closed {
		t.Fatalf("given up: %+v, %q", got, refusal)
	}
	if _, refusal := service.Run(context.Background(), "nope", "true", func(bool) {}); refusal == "" {
		t.Fatal("a command ran in a terminal that is not there")
	}
}
