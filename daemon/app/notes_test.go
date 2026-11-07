package app_test

import (
	"path/filepath"
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

const buildNotes = "# Commands\n- Build with make, never with go build: make embeds the app.\n"

func notesReadings(test *App, sessionID string) []any {
	return OfKind(test.Events(sessionID), "projectNotesRead")
}

func TestTheModelReadsTheProjectsNotesWithTheFirstMessageOnly(t *testing.T) {
	test := Start(t, Options{
		Files:   map[string]string{"AGENTS.md": buildNotes},
		Replies: []fakellama.Reply{text("Done."), text("Again.")},
	})
	session := test.NewSession(sessions.Request{Prompt: "Build it"})
	test.UntilIdle(session.ID)
	Expect(t, Kinds(test.Events(session.ID)), []any{"turnStarted", "userMessage", "projectNotesRead", "assistantMessage", "turnFinished"})
	messages := fakellama.Messages(test.Requests()[0])
	if len(messages) != 2 || messages[1]["role"] != "user" {
		t.Fatalf("roles %v", messages)
	}
	want := "Build it\n\nThe project’s notes (AGENTS.md)"
	file := filepath.Join(test.Workspace, "AGENTS.md") + ":\n" + buildNotes
	if content := messages[1]["content"].(string); !strings.Contains(content, want) || !strings.HasSuffix(content, file) {
		t.Fatalf("content %q", content)
	}
	// Read once: the next message does not bring them again.
	test.App.Sessions.SendPrompt(session.ID, "Once more", nil)
	test.WaitForEvent("turnFinished", 2)
	if len(notesReadings(test, session.ID)) != 1 {
		t.Fatal("read twice")
	}
}

func TestNothingIsReadWhereThereAreNoNotes(t *testing.T) {
	test := Start(t, Options{Files: map[string]string{"AGENTS.md": "  \n"}, Replies: []fakellama.Reply{text("Done.")}})
	session := test.NewSession(sessions.Request{Prompt: "Hello"})
	test.UntilIdle(session.ID)
	if len(notesReadings(test, session.ID)) != 0 {
		t.Fatal("an empty file was read as notes")
	}
}

func TestTheRepositorysNotesComeBeforeTheFoldersOwn(t *testing.T) {
	test := Start(t, Options{
		Files: map[string]string{
			".git/HEAD": "ref: refs/heads/main\n", "AGENTS.md": "Root notes.", "api/AGENTS.md": "Api notes.",
		},
		Replies: []fakellama.Reply{text("Done.")},
	})
	session := test.NewSession(sessions.Request{Prompt: "Hello", Workspace: filepath.Join(test.Workspace, "api")})
	test.UntilIdle(session.ID)
	request := RequestText(test.Requests()[0])
	root, api := strings.Index(request, "Root notes."), strings.Index(request, "Api notes.")
	if root < 0 || api < root {
		t.Fatalf("root at %d, api at %d", root, api)
	}
}

func TestTheNotesAreReadAgainAfterACompaction(t *testing.T) {
	test := Start(t, Options{
		// The system prompt and the tools take about 4,000 tokens; big.txt, about 4,000 more.
		ContextTokens: 6000,
		Files:         map[string]string{"AGENTS.md": buildNotes, "big.txt": strings.Repeat(strings.Repeat("x", 60)+"\n", 200)},
		Replies:       []fakellama.Reply{call("read", map[string]any{"path": "big.txt"}), text("Lines of x.")},
		Scripts:       map[string][]fakellama.Reply{summaryRequest: {text("The user asked what big.txt holds.")}},
	})
	session := test.NewSession(sessions.Request{Prompt: "What does big.txt hold?"})
	test.UntilIdle(session.ID)
	kinds := Kinds(test.Events(session.ID))
	at := 0
	for index, kind := range kinds {
		if kind == "contextCompacted" {
			at = index
		}
	}
	if strings.Join(kinds[at:], " ") != "contextCompacted projectNotesRead assistantMessage turnFinished" || len(notesReadings(test, session.ID)) != 2 {
		t.Fatalf("kinds %v", kinds)
	}
	requests := test.Requests()
	if !strings.Contains(RequestText(requests[len(requests)-1]), "Build with make") {
		t.Fatal("the notes were not read after the summary")
	}
}
