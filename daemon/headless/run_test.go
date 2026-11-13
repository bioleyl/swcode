package headless_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"swcode/daemon/fakellama"
	"swcode/daemon/headless"
	"swcode/daemon/protocol"
	. "swcode/daemon/testapp"
)

func call(name string, arguments map[string]any) fakellama.Reply {
	return fakellama.Reply{ToolCalls: []fakellama.ToolCall{{Arguments: arguments, Name: name}}}
}

func text(answer string) fakellama.Reply { return fakellama.Reply{Text: answer} }

// ran is how a run went: its exit code, and what it printed.
type ran struct {
	code   int
	out    string
	errors string
}

// runIn runs swcode run's own function against a daemon under test, in its workspace.
func runIn(t *testing.T, test *App, options headless.Options) ran {
	t.Helper()
	out, errors := &bytes.Buffer{}, &bytes.Buffer{}
	options.Address, options.Token, options.Cwd = test.Server.URL, Token, test.Workspace
	options.Out, options.Err = out, errors
	if options.Timeout == 0 {
		options.Timeout = 20 * time.Second
	}
	if options.Mode == "" {
		options.Mode = protocol.PermissionModeAuto
	}
	code := headless.Run(context.Background(), options)
	return ran{code: code, errors: errors.String(), out: out.String()}
}

// lines reads a run's JSON output: an object a line.
func lines(t *testing.T, output string) []map[string]any {
	t.Helper()
	read := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		var object map[string]any
		if err := json.Unmarshal([]byte(line), &object); err != nil {
			t.Fatalf("not a JSON line: %q", line)
		}
		read = append(read, object)
	}
	return read
}

func TestARunPrintsTheAgentsAnswerAndItsCallsApart(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{
		call("bash", map[string]any{"command": "echo hi > said.txt"}), text("Said hi."),
	}})
	got := runIn(t, test, headless.Options{Prompt: "Say hi"})
	if got.code != 0 || got.out != "Said hi.\n" || got.errors != "> bash echo hi > said.txt\n" {
		t.Fatalf("the run: %+v", got)
	}
	if test.ReadFile("said.txt") != "hi\n" {
		t.Fatal("the command did not run in the folder given")
	}
	// The session stays in the daemon, for the app to show.
	if listed := test.App.Sessions.List(); len(listed) != 1 || listed[0].Workspace != test.Workspace {
		t.Fatalf("the daemon's sessions: %+v", listed)
	}
}

func TestARunPrintsTheSessionsEventsAsJSONLinesThenHowItWent(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{
		call("bash", map[string]any{"command": "echo hi"}), text("Said hi."),
	}})
	got := runIn(t, test, headless.Options{JSON: true, Prompt: "Say hi"})
	if got.code != 0 || got.errors != "" {
		t.Fatalf("the run: %d, %q", got.code, got.errors)
	}
	read := lines(t, got.out)
	kinds := []string{}
	for _, object := range read {
		kinds = append(kinds, object["kind"].(string))
	}
	want := "turnStarted userMessage assistantMessage toolCall toolRunning toolFinished assistantMessage turnFinished runFinished"
	if strings.Join(kinds, " ") != want {
		t.Fatalf("the lines: %v", kinds)
	}
	last := read[len(read)-1]
	sessions := test.App.Sessions.List()
	if last["outcome"] != "done" || last["sessionId"] != sessions[0].ID || last["steps"] != 2.0 || last["toolCalls"] != 1.0 {
		t.Fatalf("how it went: %v", last)
	}
	if last["promptTokens"].(float64) <= 0 || last["completionTokens"].(float64) <= 0 || last["seconds"].(float64) <= 0 {
		t.Fatalf("what it took: %v", last)
	}
}

func TestARunWhoseTurnFailsExitsWithOneAndSaysWhy(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{{FailWith: 500, Text: "tools param requires --jinja flag"}}})
	got := runIn(t, test, headless.Options{JSON: true, Prompt: "Say hi"})
	read := lines(t, got.out)
	if got.code != 1 || read[len(read)-1]["outcome"] != "failed" || !strings.Contains(got.errors, "jinja") {
		t.Fatalf("the run: %d, %q, %v", got.code, got.errors, read[len(read)-1])
	}
}

func TestARunDeclinesWhatAsksSinceNobodyIsThere(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{call("bash", map[string]any{"command": "echo hi > said.txt"})}})
	got := runIn(t, test, headless.Options{Mode: protocol.PermissionModeAsk, Prompt: "Say hi"})
	if got.code != 1 || test.ReadFile("said.txt") != "" {
		t.Fatalf("the run: %+v, the file %q", got, test.ReadFile("said.txt"))
	}
}

func TestARunThatTakesTooLongStopsItsTurn(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{{Delay: 30 * time.Second, Text: "Late."}}})
	got := runIn(t, test, headless.Options{JSON: true, Prompt: "Say hi", Timeout: 500 * time.Millisecond})
	read := lines(t, got.out)
	if got.code != 1 || read[len(read)-1]["outcome"] != "stopped" || !strings.Contains(got.errors, "longer than") {
		t.Fatalf("the run: %d, %q, %v", got.code, got.errors, read[len(read)-1])
	}
	// The session does not work on with nobody waiting for it.
	test.UntilIdle(test.App.Sessions.List()[0].ID)
}

func TestARunTheDaemonRefusesSaysWhy(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{}})
	out, errors := &bytes.Buffer{}, &bytes.Buffer{}
	code := headless.Run(context.Background(), headless.Options{
		Address: test.Server.URL, Cwd: "/no/such/folder/anywhere", Err: errors, Mode: protocol.PermissionModeAuto, Out: out,
		Prompt: "Say hi", Timeout: 20 * time.Second, Token: Token,
	})
	if code != 1 || errors.String() == "" || out.String() != "" {
		t.Fatalf("the run: %d, %q, %q", code, errors.String(), out.String())
	}
}
