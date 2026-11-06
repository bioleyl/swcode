package app_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

// toolNames is the tools a request to the model offered, by name.
func toolNames(request map[string]any) []string {
	names := []string{}
	for _, tool := range request["tools"].([]any) {
		names = append(names, tool.(map[string]any)["function"].(map[string]any)["name"].(string))
	}
	return names
}

func TestAChatWorksInNoFolderAndMakesNoProject(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("Paris.")}})
	before := len(test.App.Projects.List())
	chat := test.NewSession(sessions.Request{Kind: protocol.SessionKindChat})
	if chat.Kind != protocol.SessionKindChat || chat.Workspace != "" || !hosts.IsLocal(chat.Host) {
		t.Fatalf("chat %+v", chat)
	}
	if len(test.App.Projects.List()) != before {
		t.Fatal("the chat made a project")
	}
}

func TestAChatReadsTheWebAndNothingElse(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("Paris.")}})
	chat := test.NewSession(sessions.Request{Kind: protocol.SessionKindChat})
	test.App.Sessions.SendPrompt(chat.ID, "What is the capital of France?", nil)
	test.UntilIdle(chat.ID)
	request := test.Requests()[0]
	Expect(t, toolNames(request), []string{"websearch", "webfetch"})
	text := RequestText(request)
	if !strings.Contains(text, "You work in no project and run nothing") || strings.Contains(text, "Workspace:") {
		t.Fatalf("the chat's prompt: %s", text)
	}
}

func TestASessionIsACodingSessionWhenItsKindIsLeftOut(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("Done.")}})
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "Hi", nil)
	test.UntilIdle(session.ID)
	if session.Kind != protocol.SessionKindCode || session.Workspace == "" {
		t.Fatalf("session %+v", session)
	}
	if !slices.Contains(toolNames(test.Requests()[0]), "bash") {
		t.Fatal("a coding session cannot run commands")
	}
}

func TestARestartedDaemonMakesNoProjectForAChat(t *testing.T) {
	first := Start(t, Options{Replies: []fakellama.Reply{text("Paris.")}})
	chat := first.NewSession(sessions.Request{Kind: protocol.SessionKindChat})
	first.App.Sessions.SendPrompt(chat.ID, "What is the capital of France?", nil)
	first.UntilIdle(chat.ID)
	before := projectNames(first)
	first.Stop()
	again := Start(t, Options{DataDir: first.DataDir})
	if !reflect.DeepEqual(projectNames(again), before) {
		t.Fatalf("projects %v, before %v", projectNames(again), before)
	}
}

func TestAProjectWithoutAFolderIsDroppedAtAStart(t *testing.T) {
	first := Start(t, Options{})
	first.Stop()
	pathless := `[{"createdAt":"2026-11-06T08:00:00.000Z","host":{"kind":"local"},"id":"p0","name":".","path":""}]`
	if err := os.WriteFile(filepath.Join(first.DataDir, "projects.json"), []byte(pathless), 0o644); err != nil {
		t.Fatal(err)
	}
	again := Start(t, Options{DataDir: first.DataDir})
	if slices.Contains(projectNames(again), ".") {
		t.Fatalf("projects %v", projectNames(again))
	}
}
