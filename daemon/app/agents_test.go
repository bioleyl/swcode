package app_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

const writerAgent = `---
name: writer
description: Writes the files it is asked for.
tools: [read, write]
---

Write what you are asked, and nothing else.
`

// definedCall is a main agent giving a task to one of the person's agents.
func definedCall(agent, description, prompt string) fakellama.Reply {
	return fakellama.Reply{ToolCalls: []fakellama.ToolCall{{
		Arguments: map[string]any{"agent": agent, "description": description, "prompt": prompt}, Name: "agent",
	}}}
}

// startWithWriter is a daemon whose home's .agents/agents holds the writer agent; its main agent gives it a task,
// then answers from its report.
func startWithWriter(t *testing.T, agent string) *App {
	t.Helper()
	home := t.TempDir()
	put(t, home, ".agents/agents/writer.md", writerAgent)
	return Start(t, Options{
		Home:    home,
		Replies: []fakellama.Reply{definedCall(agent, "Write the notes", "TASK-W: write notes.txt"), text("Done.")},
		Scripts: map[string][]fakellama.Reply{
			"TASK-W": {call("write", map[string]any{"content": "hello\n", "path": "notes.txt"}), text("Wrote notes.txt.")},
		},
		Slots: 4,
	})
}

// runWith sends a prompt in a session of that mode, and waits for the turn its subagent's report wakes.
func runWith(t *testing.T, test *App, mode protocol.PermissionMode, turns int) string {
	t.Helper()
	session := test.NewSession(sessions.Request{Mode: mode})
	if problem := test.App.Sessions.SendPrompt(session.ID, "Write the notes", nil); problem != "" {
		t.Fatal(problem)
	}
	test.WaitForEvent("turnFinished", turns)
	test.UntilIdle(session.ID)
	return session.ID
}

// reportOf is the first subagent report the session's main agent was handed.
func reportOf(t *testing.T, test *App, sessionID string) string {
	t.Helper()
	for _, event := range test.Events(sessionID) {
		if report, ok := event.Value.(protocol.SessionEventSubagentReport); ok {
			return report.Text
		}
	}
	t.Fatal("no report")
	return ""
}

func TestAnAgentThePersonDefinedWritesWhereTheSessionAcceptsEdits(t *testing.T) {
	test := startWithWriter(t, "writer")
	sessionID := runWith(t, test, protocol.PermissionModeAcceptEdits, 2)
	if test.ReadFile("notes.txt") != "hello\n" {
		t.Fatalf("notes.txt is %q", test.ReadFile("notes.txt"))
	}
	if report := reportOf(t, test, sessionID); report != "Wrote notes.txt." {
		t.Fatalf("the report: %q", report)
	}
	events := test.Events(sessionID)
	Expect(t, OfKind(events, "subagentStarted")[0], `{"description":"writer: Write the notes"}`)
	Expect(t, OfKind(events, "subagentActivity")[0], `{"ok":true,"summary":"notes.txt","tool":"write"}`)
	// Its own calls never enter the session's list: only the main agent's agent call is there.
	if calls := OfKind(events, "toolCall"); len(calls) != 1 {
		t.Fatalf("%d calls in the session's list", len(calls))
	}
	// The model was told of the agent with the first message, and the agent starts from the person's instructions
	// with the tools they gave it.
	if said := firstMessage(test); !strings.Contains(said, "The user’s agents") || !strings.Contains(said, "- writer: Writes the files it is asked for.") {
		t.Fatalf("the first message: %q", said)
	}
	for _, request := range test.Requests() {
		messages := fakellama.Messages(request)
		system, _ := messages[0]["content"].(string)
		if !strings.HasPrefix(system, "Write what you are asked, and nothing else.") {
			continue
		}
		if names := toolNamesOf(request); strings.Join(names, ",") != "read,write" {
			t.Fatalf("the agent was offered %v", names)
		}
		return
	}
	t.Fatal("no request started from the agent's instructions")
}

func TestAnAgentThePersonDefinedIsRefusedWhatTheSessionAsksAbout(t *testing.T) {
	test := startWithWriter(t, "writer")
	sessionID := runWith(t, test, protocol.PermissionModeAsk, 2)
	if test.ReadFile("notes.txt") != "" {
		t.Fatal("the agent wrote a file the session asks about")
	}
	events := test.Events(sessionID)
	if asked := len(OfKind(events, "permissionRequested")); asked != 0 {
		t.Fatalf("the person was asked %d times for a subagent", asked)
	}
	if report := reportOf(t, test, sessionID); !strings.Contains(report, "has no one to ask: write notes.txt") {
		t.Fatalf("the report does not say why: %q", report)
	}
}

func TestAnAgentNobodyDefinedIsRefusedWithTheNamesOfThoseThereAre(t *testing.T) {
	test := startWithWriter(t, "nobody")
	sessionID := runWith(t, test, protocol.PermissionModeAcceptEdits, 1)
	finished := Finished(test.Events(sessionID))[0]
	if finished.OK || !strings.Contains(finished.Output, "There is no agent named nobody") || !strings.Contains(finished.Output, "writer") {
		t.Fatalf("the call: %+v", finished)
	}
	if started := len(OfKind(test.Events(sessionID), "subagentStarted")); started != 0 {
		t.Fatal("a subagent started")
	}
}

func TestAnUntrustedFoldersOwnAgentIsNotOffered(t *testing.T) {
	home := t.TempDir()
	put(t, home, ".agents/agents/writer.md", writerAgent)
	test := Start(t, Options{Home: home, Replies: []fakellama.Reply{text("Looked."), text("Looked again.")}})
	folder := filepath.Join(test.Workspace, "cloned")
	put(t, folder, ".agents/agents/local.md", "---\ndescription: The folder’s own agent.\ntools:\n  - bash\n---\nRun what you like.\n")
	project, problem := test.App.Projects.Create(context.Background(), folder, nil, "")
	if problem != "" {
		t.Fatal(problem)
	}
	session := test.NewSession(sessions.Request{Prompt: "Look around", Workspace: folder})
	test.UntilIdle(session.ID)
	if said := firstMessage(test); strings.Contains(said, "local:") || !strings.Contains(said, "- writer:") {
		t.Fatalf("an untrusted folder's session was told: %q", said)
	}
	if problem := test.App.Projects.SetTrust(project.ID, true); problem != "" {
		t.Fatal(problem)
	}
	trusted := test.NewSession(sessions.Request{Prompt: "Look again", Workspace: folder})
	test.UntilIdle(trusted.ID)
	if said := fakellama.Messages(test.Requests()[1])[1]["content"].(string); !strings.Contains(said, "- local: The folder’s own agent.") {
		t.Fatalf("a trusted folder's session was told: %q", said)
	}
}
