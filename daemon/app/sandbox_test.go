package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"swcode/daemon/fakellama"
	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

var sandboxOn = map[string]string{"SWCODE_SANDBOX": "on"}

// needsSandbox skips a test on a machine that cannot make a sandbox: no bubblewrap, or a system that refuses it.
func needsSandbox(t *testing.T) {
	t.Helper()
	local := hosts.NewLocalHost(hosts.LocalOptions{Home: t.TempDir(), ScratchRoot: t.TempDir()})
	defer local.Close()
	if state := hosts.NewSandboxes(time.Now).Of(context.Background(), "local", local); !state.Available {
		t.Skip("no sandbox here: " + state.Problem)
	}
}

// outsideFile is a file no sandboxed command can write, and no test leaves behind.
func outsideFile(t *testing.T) string {
	t.Helper()
	file := filepath.Join("/var/tmp", "swcode-sandbox-test-"+filepath.Base(t.TempDir())+"-"+strings.ReplaceAll(t.Name(), "/", "-"))
	t.Cleanup(func() { _ = os.Remove(file) })
	return file
}

func TestCommandsRunInASandboxAndAskBeforeLeavingItEvenWhereEverythingRuns(t *testing.T) {
	needsSandbox(t)
	file := outsideFile(t)
	test := Start(t, Options{Settings: sandboxOn, Replies: []fakellama.Reply{
		call("bash", map[string]any{"command": "echo inside > made.txt && echo no > " + file}),
		call("bash", map[string]any{"command": "echo yes > " + file, "unsandboxed": true}),
		text("Written."),
	}})
	session := test.NewSession(sessions.Request{Mode: protocol.PermissionModeAuto})
	if problem := test.App.Sessions.SendPrompt(session.ID, "Write the file", nil); problem != "" {
		t.Fatal(problem)
	}

	// The mode runs everything, in the sandbox: only the command that leaves it is asked about.
	asked := test.WaitForEvent("permissionRequested", 1).Value.(protocol.SessionEventPermissionRequested)
	if !asked.Outside {
		t.Fatalf("the request does not say the command leaves the sandbox: %+v", asked)
	}
	if _, err := os.Stat(file); err == nil {
		t.Fatal("the sandboxed command wrote outside its workspace")
	}
	answerNext(t, test, session.ID, protocol.PermissionDecisionAllowOnce, 1)
	test.UntilIdle(session.ID)

	finished := Finished(test.Events(session.ID))
	if finished[0].OK || !strings.Contains(finished[0].Output, "Read-only file system") || !strings.Contains(finished[0].Output, "unsandboxed") {
		t.Fatalf("the sandboxed command: %+v", finished[0])
	}
	if test.ReadFile("made.txt") != "inside\n" {
		t.Fatal("the workspace was not written")
	}
	if data, _ := os.ReadFile(file); string(data) != "yes\n" || !finished[1].OK {
		t.Fatalf("outside the sandbox, once allowed: %q, %+v", data, finished[1])
	}
	if prompt := RequestText(map[string]any{"messages": test.Requests()[0]["messages"]}); !strings.Contains(prompt, "Commands run in a sandbox") {
		t.Fatal("the model is not told its commands run in a sandbox")
	}
}

func TestAProjectTurnsTheSandboxOffForItsSessions(t *testing.T) {
	needsSandbox(t)
	file := outsideFile(t)
	test := Start(t, Options{Settings: sandboxOn, Replies: []fakellama.Reply{
		call("bash", map[string]any{"command": "echo free > " + file}), text("Written."),
	}})
	project := test.App.Projects.Ensure(test.Workspace, protocol.HostRef{Value: protocol.HostRefLocal{}})
	status, view := test.Call("GET", protocol.EndpointSandbox+"?"+protocol.ProjectParam+"="+project.ID, nil)
	Expect(t, view, `{"available":true}`)
	if status != 200 {
		t.Fatalf("status %d", status)
	}
	if problem := test.App.Projects.SetSandbox(project.ID, false); problem != "" {
		t.Fatal(problem)
	}

	session := test.NewSession(sessions.Request{Mode: protocol.PermissionModeAuto})
	if problem := test.App.Sessions.SendPrompt(session.ID, "Write the file", nil); problem != "" {
		t.Fatal(problem)
	}
	test.UntilIdle(session.ID)
	if data, _ := os.ReadFile(file); string(data) != "free\n" {
		t.Fatalf("the command did not run as the person: %q", data)
	}
	if prompt := RequestText(map[string]any{"messages": test.Requests()[0]["messages"]}); strings.Contains(prompt, "Commands run in a sandbox") {
		t.Fatal("the model is told of a sandbox its commands do not run in")
	}
}

func TestADaemonWhoseSettingsTurnTheSandboxOffSaysSo(t *testing.T) {
	test := startHello(t)
	project := test.App.Projects.Ensure(test.Workspace, protocol.HostRef{Value: protocol.HostRefLocal{}})
	_, view := test.Call("GET", protocol.EndpointSandbox+"?"+protocol.ProjectParam+"="+project.ID, nil)
	Expect(t, view, `{"available":false}`)
	if problem, _ := view.(map[string]any)["problem"].(string); !strings.Contains(problem, "SWCODE_SANDBOX") {
		t.Fatalf("why: %q", problem)
	}
}
