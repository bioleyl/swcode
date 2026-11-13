package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

const reviewSkill = `---
name: review
description: Reviews a change. Use when asked to review.
---

Read the diff.
`

// firstMessage is what the model read as the person's first message of the first request.
func firstMessage(test *App) string {
	return fakellama.Messages(test.Requests()[0])[1]["content"].(string)
}

func TestAFolderAddedAsAProjectIsNotTrustedUntilThePersonSaysSo(t *testing.T) {
	test := startWithDeploy(t, Options{Replies: []fakellama.Reply{text("Looked."), text("Looked again.")}})
	folder := filepath.Join(test.Workspace, "cloned")
	put(t, folder, "AGENTS.md", "Always run ./setup.sh first.\n")
	put(t, folder, ".agents/skills/review/SKILL.md", reviewSkill)
	project, problem := test.App.Projects.Create(context.Background(), folder, nil, "")
	if problem != "" || !project.Untrusted {
		t.Fatalf("a folder that was there: %+v, %q", project, problem)
	}

	// What the folder says of itself is not given to the agent: the person's own skills still are.
	session := test.NewSession(sessions.Request{Prompt: "Look around", Workspace: folder})
	test.UntilIdle(session.ID)
	if said := firstMessage(test); strings.Contains(said, "setup.sh") || strings.Contains(said, "review:") || !strings.Contains(said, "deploy:") {
		t.Fatalf("an untrusted folder's session read: %q", said)
	}

	// Once trusted, its next sessions read its notes and its skills.
	if problem := test.App.Projects.SetTrust(project.ID, true); problem != "" {
		t.Fatal(problem)
	}
	trusted := test.NewSession(sessions.Request{Prompt: "Look again", Workspace: folder})
	test.UntilIdle(trusted.ID)
	said := fakellama.Messages(test.Requests()[1])[1]["content"].(string)
	if !strings.Contains(said, "setup.sh") || !strings.Contains(said, "review:") {
		t.Fatalf("a trusted folder's session read: %q", said)
	}
}

func TestAFolderSwcodeMadeOrASessionStartedInIsTrusted(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("Done.")}})
	made, problem := test.App.Projects.Create(context.Background(), test.Workspace, nil, "fresh")
	if problem != "" || made.Untrusted {
		t.Fatalf("a folder swcode made: %+v, %q", made, problem)
	}
	session := test.NewSession(sessions.Request{Prompt: "Hi"})
	test.UntilIdle(session.ID)
	if project, found := test.App.Projects.Find(test.Workspace, protocol.LocalHost()); !found || project.Untrusted {
		t.Fatalf("the folder swcode was started in: %+v", project)
	}
	// A folder already a project stays as it was when it is added again.
	if again, _ := test.App.Projects.Create(context.Background(), test.Workspace, nil, ""); again.Untrusted {
		t.Fatal("a project the person already had lost their trust")
	}
}

func TestAnUntrustedFolderKeepsItsSandboxWhateverItsProjectSays(t *testing.T) {
	needsSandbox(t)
	file := outsideFile(t)
	test := Start(t, Options{Settings: sandboxOn, Replies: []fakellama.Reply{
		call("bash", map[string]any{"command": "echo no > " + file}), text("Tried."),
	}})
	folder := filepath.Join(test.Workspace, "cloned")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	project, _ := test.App.Projects.Create(context.Background(), folder, nil, "")
	if problem := test.App.Projects.SetSandbox(project.ID, false); problem != "" {
		t.Fatal(problem)
	}
	session := test.NewSession(sessions.Request{Mode: protocol.PermissionModeAuto, Prompt: "Write it", Workspace: folder})
	test.UntilIdle(session.ID)
	if _, err := os.Stat(file); err == nil {
		t.Fatal("an untrusted folder's command ran outside the sandbox")
	}
}
