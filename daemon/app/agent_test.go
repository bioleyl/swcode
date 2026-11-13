package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

var questionCall = fakellama.ToolCall{Name: "question", Arguments: map[string]any{"questions": []any{map[string]any{
	"header": "Approach",
	"options": []any{
		map[string]any{"description": "A map from tag to number.", "label": "Map"},
		map[string]any{"description": "Sort the tags once.", "label": "Sort"},
	},
	"question": "Map or sort?",
}}}}

// planCall is a plan call, its steps as content and status in turn.
func planCall(steps ...string) fakellama.ToolCall {
	list := []any{}
	for index := 0; index < len(steps); index += 2 {
		list = append(list, map[string]any{"content": steps[index], "status": steps[index+1]})
	}
	return fakellama.ToolCall{Arguments: map[string]any{"steps": list}, Name: "plan"}
}

func call(name string, arguments map[string]any) fakellama.Reply {
	return fakellama.Reply{ToolCalls: []fakellama.ToolCall{{Arguments: arguments, Name: name}}}
}

func text(answer string) fakellama.Reply { return fakellama.Reply{Text: answer} }

// prompt sends a prompt in a new session that asks before edits and commands: the session's id.
func prompt(t *testing.T, test *App, text string) string {
	t.Helper()
	session := test.NewSession(sessions.Request{Mode: protocol.PermissionModeAsk})
	if problem := test.App.Sessions.SendPrompt(session.ID, text, nil); problem != "" {
		t.Fatal(problem)
	}
	return session.ID
}

func answerNext(t *testing.T, test *App, sessionID string, decision protocol.PermissionDecision, count int) {
	t.Helper()
	request := test.WaitForEvent("permissionRequested", count).Value.(protocol.SessionEventPermissionRequested)
	if problem := test.App.Sessions.Answer(sessionID, request.RequestID, sessions.Permission(decision)); problem != "" {
		t.Fatal(problem)
	}
}

func last(events []protocol.SessionEvent) any { return events[len(events)-1] }

func TestATurnReadsAFileWithoutAskingAndGivesTheModelWhatItRead(t *testing.T) {
	test := Start(t, Options{Files: map[string]string{"notes.txt": "hello from the notes\n"}, Replies: []fakellama.Reply{
		call("read", map[string]any{"path": "notes.txt"}), text("It says hello."),
	}})
	sessionID := prompt(t, test, "What do the notes say?")
	test.UntilIdle(sessionID)
	events := test.Events(sessionID)
	want := []string{"turnStarted", "userMessage", "assistantMessage", "toolCall", "toolRunning", "toolFinished", "assistantMessage", "turnFinished"}
	if got := Kinds(events); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds %v", got)
	}
	Expect(t, Finished(events)[0], `{"meta":"1 line","ok":true}`)
	Expect(t, last(events), `{"outcome":"done"}`)
	if !strings.Contains(RequestText(test.Requests()[1]), "hello from the notes") {
		t.Fatal("the model did not get what it read")
	}
	Expect(t, test.Requests()[0], `{"cache_prompt":true,"stream":true}`)
}

func TestATurnStreamsTheAnswerAsItComesThenRecordsItWhole(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("All good here.")}})
	sessionID := prompt(t, test, "Hi")
	test.UntilIdle(sessionID)
	streamed := ""
	for _, message := range test.Messages() {
		if delta, ok := message.Value.(protocol.ServerMessageAssistantDelta); ok {
			streamed += delta.Text
		}
	}
	if streamed != "All good here." {
		t.Fatalf("streamed %q", streamed)
	}
	reply := OfKind(test.Events(sessionID), "assistantMessage")[0].(map[string]any)
	if reply["text"] != "All good here." || reply["usage"].(map[string]any)["completionTokens"] != float64(40) {
		t.Fatalf("reply %v", reply)
	}
}

func TestATurnSaysWhichToolTheModelCallsWhileItWritesTheCall(t *testing.T) {
	test := Start(t, Options{Files: map[string]string{"notes.txt": "hello\n"}, Replies: []fakellama.Reply{
		call("read", map[string]any{"path": "notes.txt"}), call("nosuchtool", map[string]any{}), text("Done."),
	}})
	sessionID := prompt(t, test, "Read the notes")
	test.UntilIdle(sessionID)
	writing := []protocol.ToolName{}
	for _, message := range test.Messages() {
		if call, ok := message.Value.(protocol.ServerMessageToolCallWriting); ok && call.SessionID == sessionID {
			writing = append(writing, call.Tool)
		}
	}
	if !reflect.DeepEqual(writing, []protocol.ToolName{protocol.ToolNameRead}) {
		t.Fatalf("tools %v", writing)
	}
}

func TestATurnAsksAgainWhenTheModelEndsInsideItsThoughtsWithTheSameRequest(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{{Reasoning: "Let me read the README first."}, text("All good here.")}})
	sessionID := prompt(t, test, "Hi")
	test.UntilIdle(sessionID)
	replies := OfKind(test.Events(sessionID), "assistantMessage")
	Expect(t, replies, []any{map[string]any{"reasoning": "Let me read the README first.", "text": "", "toolCalls": []any{}}, map[string]any{"text": "All good here."}})
	requests := test.Requests()
	if !reflect.DeepEqual(requests[1]["messages"], requests[0]["messages"]) {
		t.Fatal("the second request is not the first one")
	}
}

func TestATurnEndsAfterTwoMoreEmptyReplies(t *testing.T) {
	empty := fakellama.Reply{Reasoning: "Let me think."}
	test := Start(t, Options{Replies: []fakellama.Reply{empty, empty, empty, text("Never asked for.")}})
	sessionID := prompt(t, test, "Hi")
	test.UntilIdle(sessionID)
	Expect(t, last(test.Events(sessionID)), `{"outcome":"done"}`)
	if len(test.Requests()) != 3 {
		t.Fatalf("%d requests", len(test.Requests()))
	}
}

func TestTheModelGetsItsPastThoughtsBackWithEachReply(t *testing.T) {
	test := Start(t, Options{Files: map[string]string{"notes.txt": "hello\n"}, Replies: []fakellama.Reply{
		{Reasoning: "The notes should say hello.", ToolCalls: []fakellama.ToolCall{{Arguments: map[string]any{"path": "notes.txt"}, Name: "read"}}},
		text("They say hello."),
	}})
	sessionID := prompt(t, test, "What do the notes say?")
	test.UntilIdle(sessionID)
	found := false
	for _, message := range fakellama.Messages(test.Requests()[1]) {
		found = found || (message["role"] == "assistant" && message["reasoning_content"] == "The notes should say hello.")
	}
	if !found {
		t.Fatal("no past thoughts")
	}
}

func TestEachSessionHasAScratchFolderNamedInTheSystemPrompt(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("Hello.")}})
	sessionID := prompt(t, test, "Hi")
	test.UntilIdle(sessionID)
	scratch := filepath.Join(test.DataDir, "scratch", sessionID)
	if entries, err := os.ReadDir(scratch); err != nil || len(entries) != 0 {
		t.Fatalf("scratch %v %v", entries, err)
	}
	if !strings.Contains(RequestText(test.Requests()[0]), "Scratch folder: "+scratch) {
		t.Fatal("no scratch folder in the system prompt")
	}
}

func TestTheModelIsRemindedOfItsUnfinishedPlanAndTheTurnGoesOn(t *testing.T) {
	test := Start(t, Options{Files: map[string]string{"notes.txt": "hello\n"}, Replies: []fakellama.Reply{
		{ToolCalls: []fakellama.ToolCall{planCall("Read the notes", "in_progress", "Answer", "pending")}},
		text("I will read the notes now:"),
		call("read", map[string]any{"path": "notes.txt"}),
		{ToolCalls: []fakellama.ToolCall{planCall("Read the notes", "completed", "Answer", "completed")}},
		text("They say hello."),
	}})
	sessionID := prompt(t, test, "What do the notes say?")
	test.UntilIdle(sessionID)
	reminders := OfKind(test.Events(sessionID), "reminder")
	if len(reminders) != 1 || !strings.Contains(reminders[0].(map[string]any)["text"].(string), "- Read the notes\n- Answer") {
		t.Fatalf("reminders %v", reminders)
	}
	if !strings.Contains(RequestText(test.Requests()[2]), "Your plan still has steps") {
		t.Fatal("the model was not reminded")
	}
	Expect(t, last(test.Events(sessionID)), `{"outcome":"done"}`)
	if len(test.Requests()) != 5 {
		t.Fatalf("%d requests", len(test.Requests()))
	}
}

func TestTheTurnEndsWhenTheModelStopsAgainAfterAReminderWithoutDoingAnything(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{
		{ToolCalls: []fakellama.ToolCall{planCall("Look", "in_progress")}}, text("Now I will look."), text("Which folder do you mean?"),
	}})
	sessionID := prompt(t, test, "Look around")
	test.UntilIdle(sessionID)
	if len(OfKind(test.Events(sessionID), "reminder")) != 1 || len(test.Requests()) != 3 {
		t.Fatalf("%d requests", len(test.Requests()))
	}
	Expect(t, last(test.Events(sessionID)), `{"outcome":"done"}`)
}

func TestTheModelReportsAtTheStepLimitAndNoneOfItsLastCallsRun(t *testing.T) {
	read := call("read", map[string]any{"path": "notes.txt"})
	test := Start(t, Options{Files: map[string]string{"notes.txt": "hello\n"}, MaxSteps: 2, Replies: []fakellama.Reply{
		read, read,
		{Text: "I read the notes twice; nothing to undo.", ToolCalls: []fakellama.ToolCall{{Arguments: map[string]any{"command": "touch created.txt"}, Name: "bash"}}},
	}})
	sessionID := prompt(t, test, "Read the notes")
	test.UntilIdle(sessionID)
	events := test.Events(sessionID)
	Expect(t, OfKind(events, "reminder"), []any{map[string]any{"reason": "stepLimit"}})
	if !strings.Contains(RequestText(test.Requests()[2]), "You have used all 2 steps of this turn") {
		t.Fatal("no step limit request")
	}
	Expect(t, events[len(events)-2], `{"kind":"assistantMessage","text":"I read the notes twice; nothing to undo."}`)
	Expect(t, last(events), `{"outcome":"limit"}`)
	if _, err := os.Stat(filepath.Join(test.Workspace, "created.txt")); err == nil {
		t.Fatal("a call of the report ran")
	}
}

func TestTheAgentAsksThePersonItsQuestionsWaitsAndGivesTheModelTheirAnswers(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{{ToolCalls: []fakellama.ToolCall{questionCall}}, text("Going with the map.")}})
	sessionID := prompt(t, test, "Speed up the tag lookup")
	asked := test.WaitForEvent("questionAsked", 1).Value.(protocol.SessionEventQuestionAsked)
	Expect(t, asked.Questions, `[{"header":"Approach","multiSelect":false,"question":"Map or sort?"}]`)
	if test.App.Sessions.List()[0].Status != protocol.SessionStatusWaiting {
		t.Fatal("not waiting")
	}
	if problem := test.App.Sessions.Answer(sessionID, "another", sessions.Answers([]string{"Map"})); !strings.Contains(problem, "answered") {
		t.Fatalf("got %q", problem)
	}
	if problem := test.App.Sessions.Answer(sessionID, asked.RequestID, sessions.Answers([]string{"Map"})); problem != "" {
		t.Fatal(problem)
	}
	test.UntilIdle(sessionID)
	Expect(t, OfKind(test.Events(sessionID), "questionAnswered"), `[{"answers":["Map"]}]`)
	if !strings.Contains(RequestText(test.Requests()[1]), `Map or sort?\n→ Map`) {
		t.Fatal("the model did not get the answers")
	}
}

// TestACommandThatAsksForAPasswordWaitsForThePersonAndGetsIt: a command this computer runs that stops on a
// password prompt asks the person, and the password they typed goes to the command, not to the session.
func TestACommandThatAsksForAPasswordWaitsForThePersonAndGetsIt(t *testing.T) {
	askpass := filepath.Join(t.TempDir(), "askpass.sh")
	if err := os.WriteFile(askpass, []byte("#!/bin/sh\nprintf 'Password: '\nread -r line\necho got\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	test := Start(t, Options{Replies: []fakellama.Reply{
		call("bash", map[string]any{"command": "sh " + askpass}), text("It took it."),
	}})
	session := test.NewSession(sessions.Request{})
	if problem := test.App.Sessions.SendPrompt(session.ID, "Run the command that asks", nil); problem != "" {
		t.Fatal(problem)
	}
	sessionID := session.ID
	asked := test.WaitForEvent("passwordRequested", 1).Value.(protocol.SessionEventPasswordRequested)
	Expect(t, asked.Prompt, "Password:")
	// The card goes under the call whose command asked.
	Expect(t, asked.CallID, test.WaitForEvent("toolCall", 1).Value.(protocol.SessionEventToolCall).CallID)
	if test.App.Sessions.List()[0].Status != protocol.SessionStatusWaiting {
		t.Fatal("not waiting")
	}
	if problem := test.App.Sessions.Answer(sessionID, asked.RequestID, sessions.Answers([]string{"no"})); !strings.Contains(problem, "answered") {
		t.Fatalf("an answer of the other kind: %q", problem)
	}
	if problem := test.App.Sessions.Answer(sessionID, asked.RequestID, sessions.Secret("hunter2")); problem != "" {
		t.Fatal(problem)
	}
	test.UntilIdle(sessionID)
	Expect(t, OfKind(test.Events(sessionID), "passwordAnswered"), `[{"gave":true}]`)
	// The command printed `got` once it had read a line: the password itself never reaches the model.
	if !strings.Contains(RequestText(test.Requests()[1]), "got") {
		t.Fatal("the command did not get the password")
	}
}

func TestTheModelGetsNoAnswerWhenTheTurnStopsWhileItWaits(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{{ToolCalls: []fakellama.ToolCall{questionCall}}}})
	sessionID := prompt(t, test, "Speed up the tag lookup")
	test.WaitForEvent("questionAsked", 1)
	test.App.Sessions.Stop(sessionID)
	test.UntilIdle(sessionID)
	Expect(t, Finished(test.Events(sessionID))[0], `{"meta":"unanswered","ok":false}`)
	Expect(t, last(test.Events(sessionID)), `{"outcome":"stopped"}`)
}

func TestAnEditIsAskedAboutAndMadeOnceAllowed(t *testing.T) {
	test := Start(t, Options{Files: map[string]string{"app.ts": "const answer = 41;\n"}, Replies: []fakellama.Reply{
		call("read", map[string]any{"path": "app.ts"}),
		call("edit", map[string]any{"newString": "42", "oldString": "41", "path": "app.ts"}),
		text("Fixed."),
	}})
	sessionID := prompt(t, test, "Fix the answer")
	Expect(t, test.WaitForEvent("permissionRequested", 1), `{"detail":{"added":1,"kind":"diff","path":"app.ts","removed":1}}`)
	if test.ReadFile("app.ts") != "const answer = 41;\n" {
		t.Fatal("changed before it was allowed")
	}
	answerNext(t, test, sessionID, protocol.PermissionDecisionAllowOnce, 1)
	test.UntilIdle(sessionID)
	if test.ReadFile("app.ts") != "const answer = 42;\n" {
		t.Fatal("not changed")
	}
	Expect(t, last(test.Events(sessionID)), `{"outcome":"done"}`)
}

func TestAnEditToAFileNotReadIsRefusedWithoutAskingAnyone(t *testing.T) {
	test := Start(t, Options{Files: map[string]string{"app.ts": "const answer = 41;\n"}, Replies: []fakellama.Reply{
		call("edit", map[string]any{"newString": "42", "oldString": "41", "path": "app.ts"}), text("I will read it first."),
	}})
	sessionID := prompt(t, test, "Fix the answer")
	test.UntilIdle(sessionID)
	events := test.Events(sessionID)
	for _, kind := range Kinds(events) {
		if kind == "permissionRequested" {
			t.Fatal("asked")
		}
	}
	Expect(t, Finished(events)[0], `{"meta":"refused","ok":false}`)
}

func TestTheTurnStopsWhenAPersonDeclinesACommand(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{call("bash", map[string]any{"command": "touch created.txt"})}})
	sessionID := prompt(t, test, "Make a file")
	answerNext(t, test, sessionID, protocol.PermissionDecisionDeny, 1)
	test.UntilIdle(sessionID)
	events := test.Events(sessionID)
	Expect(t, Finished(events)[0], `{"meta":"declined","ok":false}`)
	Expect(t, last(events), `{"outcome":"stopped"}`)
	if len(test.Requests()) != 1 {
		t.Fatalf("%d requests", len(test.Requests()))
	}
}

func TestAProgramAllowedForTheSessionGoesThroughAfter(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{
		call("bash", map[string]any{"command": "echo one"}), call("bash", map[string]any{"command": "echo two"}), text("Done."),
	}})
	sessionID := prompt(t, test, "Say one and two")
	answerNext(t, test, sessionID, protocol.PermissionDecisionAllowSession, 1)
	test.UntilIdle(sessionID)
	events := test.Events(sessionID)
	if len(OfKind(events, "permissionRequested")) != 1 {
		t.Fatal("asked twice")
	}
	outputs := []string{}
	for _, finished := range Finished(events) {
		outputs = append(outputs, finished.Output)
	}
	if !reflect.DeepEqual(outputs, []string{"one", "two"}) {
		t.Fatalf("outputs %v", outputs)
	}
}

func TestEditsGoThroughWithoutAskingInTheAcceptEditsMode(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{call("write", map[string]any{"content": "new\n", "path": "fresh.txt"}), text("Ok.")}})
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SetMode(session.ID, protocol.PermissionModeAcceptEdits)
	test.App.Sessions.SendPrompt(session.ID, "Write a file", nil)
	test.UntilIdle(session.ID)
	if len(OfKind(test.Events(session.ID), "permissionRequested")) != 0 || test.ReadFile("fresh.txt") != "new\n" {
		t.Fatal("asked, or not written")
	}
}

func TestARestorePointPutsBackTheFilesATurnChanged(t *testing.T) {
	test := Start(t, Options{Files: map[string]string{"app.ts": "answer = 41\n"}, Replies: []fakellama.Reply{
		call("read", map[string]any{"path": "app.ts"}),
		call("edit", map[string]any{"newString": "42", "oldString": "41", "path": "app.ts"}),
		call("write", map[string]any{"content": "new\n", "path": "made.ts"}),
		text("Done."),
	}})
	session := test.NewSession(sessions.Request{Mode: protocol.PermissionModeAcceptEdits})
	if problem := test.App.Sessions.SendPrompt(session.ID, "Change it", nil); problem != "" {
		t.Fatal(problem)
	}
	test.UntilIdle(session.ID)
	saved := test.WaitForEvent("checkpointSaved", 1).Value.(protocol.SessionEventCheckpointSaved)
	if !reflect.DeepEqual(saved.Files, []string{"app.ts", "made.ts"}) || saved.TurnID == "" {
		t.Fatalf("restore point %v", saved)
	}
	if test.ReadFile("app.ts") != "answer = 42\n" {
		t.Fatalf("app.ts is %q", test.ReadFile("app.ts"))
	}

	if problem := test.App.Sessions.RestoreCheckpoint(session.ID, saved.TurnID); problem != "" {
		t.Fatal(problem)
	}
	if test.ReadFile("app.ts") != "answer = 41\n" {
		t.Fatalf("app.ts is %q", test.ReadFile("app.ts"))
	}
	if _, err := os.Stat(filepath.Join(test.Workspace, "made.ts")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("made.ts survived")
	}
	restored := test.WaitForEvent("checkpointRestored", 1).Value.(protocol.SessionEventCheckpointRestored)
	if !reflect.DeepEqual(restored.Files, saved.Files) || restored.TurnID != saved.TurnID {
		t.Fatalf("restored %v", restored)
	}
}

func TestARestorePointSaysWhatEachFileHeldBeforeTheTurn(t *testing.T) {
	test := Start(t, Options{Files: map[string]string{"app.ts": "answer = 41\n"}, Replies: []fakellama.Reply{
		call("read", map[string]any{"path": "app.ts"}),
		call("edit", map[string]any{"newString": "42", "oldString": "41", "path": "app.ts"}),
		call("write", map[string]any{"content": "new\n", "path": "made.ts"}),
		text("Done."),
	}})
	session := test.NewSession(sessions.Request{Mode: protocol.PermissionModeAcceptEdits})
	if problem := test.App.Sessions.SendPrompt(session.ID, "Change it", nil); problem != "" {
		t.Fatal(problem)
	}
	test.UntilIdle(session.ID)
	saved := test.WaitForEvent("checkpointSaved", 1).Value.(protocol.SessionEventCheckpointSaved)
	files, problem := test.App.Sessions.CheckpointDiff(session.ID, saved.TurnID)
	if problem != "" {
		t.Fatal(problem)
	}
	if len(files) != 2 {
		t.Fatalf("%d files", len(files))
	}
	changed := files[0].Value.(protocol.ToolDisplayDiff)
	if changed.Path != "app.ts" || changed.Added != 1 || changed.Removed != 1 {
		t.Fatalf("app.ts diff %v", changed)
	}
	made := files[1].Value.(protocol.ToolDisplayDiff)
	if made.Path != "made.ts" || made.Added != 1 || made.Removed != 0 {
		t.Fatalf("made.ts diff %v", made)
	}
	if _, problem := test.App.Sessions.CheckpointDiff(session.ID, "no-such-turn"); problem == "" {
		t.Fatal("a restore point that is gone answered")
	}
}

func TestCommandsRunWithoutAskingByDefault(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{call("bash", map[string]any{"command": "echo ran"}), text("Done.")}})
	session := test.NewSession(sessions.Request{Prompt: "Run it"})
	test.WaitForEvent("turnFinished", 1)
	if session.Mode != protocol.PermissionModeAuto || len(OfKind(test.Events(session.ID), "permissionRequested")) != 0 {
		t.Fatal("asked")
	}
	Expect(t, Finished(test.Events(session.ID)), `[{"ok":true,"output":"ran"}]`)
}

func TestATurnStoppedWhileItWaitsForAPersonEndsAsStopped(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{call("bash", map[string]any{"command": "ls"})}})
	sessionID := prompt(t, test, "List")
	test.WaitForEvent("permissionRequested", 1)
	test.App.Sessions.Stop(sessionID)
	test.UntilIdle(sessionID)
	Expect(t, last(test.Events(sessionID)), `{"outcome":"stopped"}`)
}

func TestTheModelIsToldOfAToolThatDoesNotExistAndGoesOn(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{call("teleport", map[string]any{}), text("Sorry, I cannot.")}})
	sessionID := prompt(t, test, "Teleport")
	test.UntilIdle(sessionID)
	Expect(t, Finished(test.Events(sessionID))[0], `{"ok":false,"output":"There is no tool named teleport."}`)
	Expect(t, last(test.Events(sessionID)), `{"outcome":"done"}`)
}

func TestATurnFailsWithLlamaServersOwnMessage(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{{FailWith: 500, Text: "tools param requires --jinja flag"}}})
	sessionID := prompt(t, test, "Hi")
	test.UntilIdle(sessionID)
	Expect(t, last(test.Events(sessionID)), `{"message":"llm-server answered 500: tools param requires --jinja flag","outcome":"failed"}`)
}

func TestSessionsAreNamedAfterTheirFirstMessageAndFoundAgainAfterARestart(t *testing.T) {
	first := Start(t, Options{Replies: []fakellama.Reply{text("Hello!")}})
	sessionID := prompt(t, first, "Say hello to the team\nand more")
	first.UntilIdle(sessionID)
	first.Stop()
	second := Start(t, Options{DataDir: first.DataDir})
	Expect(t, second.App.Sessions.List(), []any{map[string]any{"id": sessionID, "status": "idle", "title": "Say hello to the team"}})
	if !strings.Contains(strings.Join(Kinds(second.Events(sessionID)), " "), "assistantMessage") {
		t.Fatal("no reply after the restart")
	}
}

func TestSessionsGoOnWhenTheirFileCannotBeWritten(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("Hello!"), text("Still here.")}})
	sessionID := prompt(t, test, "Hi")
	test.UntilIdle(sessionID)
	_ = os.Chmod(filepath.Join(test.DataDir, "sessions", sessionID+".jsonl"), 0o444)
	test.App.Sessions.SendPrompt(sessionID, "Again", nil)
	Expect(t, test.WaitForEvent("turnFinished", 2), `{"outcome":"done"}`)
	events := test.Events(sessionID)
	Expect(t, events[len(events)-2], `{"kind":"assistantMessage","text":"Still here."}`)
}

func userTexts(events []protocol.SessionEvent) []string {
	texts := []string{}
	for _, event := range events {
		if message, ok := event.Value.(protocol.SessionEventUserMessage); ok {
			texts = append(texts, message.Text)
		}
	}
	return texts
}

func TestAMessageSentWhileATurnRunsStartsTheNextTurn(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{call("bash", map[string]any{"command": "ls"}), text("Listed."), text("Sure.")}})
	sessionID := prompt(t, test, "List")
	test.WaitForEvent("permissionRequested", 1)
	if problem := test.App.Sessions.SendPrompt(sessionID, "Then say sure", nil); problem != "" {
		t.Fatal(problem)
	}
	Expect(t, test.App.Sessions.List()[0], `{"queued":["Then say sure"],"status":"waiting"}`)
	answerNext(t, test, sessionID, protocol.PermissionDecisionAllowOnce, 1)
	test.WaitForEvent("turnFinished", 2)
	test.UntilIdle(sessionID)
	if got := userTexts(test.Events(sessionID)); !reflect.DeepEqual(got, []string{"List", "Then say sure"}) {
		t.Fatalf("messages %v", got)
	}
	Expect(t, test.App.Sessions.List()[0], `{"queued":[]}`)
}

func TestWhatWasQueuedSentNowJoinsTheRunningTurnAtItsNextStep(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{call("bash", map[string]any{"command": "ls"}), text("Listed, and sure.")}})
	sessionID := prompt(t, test, "List")
	test.WaitForEvent("permissionRequested", 1)
	test.App.Sessions.SendPrompt(sessionID, "Then say sure", nil)
	if problem := test.App.Sessions.SendQueueNow(sessionID); problem != "" {
		t.Fatal(problem)
	}
	Expect(t, test.App.Sessions.List()[0], `{"queued":["Then say sure"],"queuedNow":true}`)
	answerNext(t, test, sessionID, protocol.PermissionDecisionAllowOnce, 1)
	test.UntilIdle(sessionID)
	steps := []string{}
	for _, event := range test.Events(sessionID) {
		switch value := event.Value.(type) {
		case protocol.SessionEventUserMessage:
			steps = append(steps, value.Text)
		case protocol.SessionEventToolFinished, protocol.SessionEventAssistantMessage, protocol.SessionEventTurnStarted:
			steps = append(steps, protocol.Kind(event))
		}
	}
	want := []string{"turnStarted", "List", "assistantMessage", "toolFinished", "Then say sure", "assistantMessage"}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("steps %v", steps)
	}
	Expect(t, test.App.Sessions.List()[0], `{"queued":[],"queuedNow":false}`)
}

func TestTheCommandsLeftRunningAreListedUntilThePersonStopsThem(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{call("bash", map[string]any{"background": true, "command": "sleep 30"}), text("The server runs.")}})
	sessionID := prompt(t, test, "Start the server")
	answerNext(t, test, sessionID, protocol.PermissionDecisionAllowOnce, 1)
	test.UntilIdle(sessionID)
	Expect(t, test.App.Sessions.List()[0], `{"jobs":[{"command":"sleep 30","id":"1"}]}`)
	if problem := test.App.Sessions.StopJob(sessionID, "1"); problem != "" {
		t.Fatal(problem)
	}
	test.Until("the job to leave the list", func() bool { return len(test.App.Sessions.List()[0].Jobs) == 0 })
}

func TestWhatWasQueuedIsForgottenWhenTheTurnIsStopped(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{call("bash", map[string]any{"command": "ls"})}})
	sessionID := prompt(t, test, "List")
	test.WaitForEvent("permissionRequested", 1)
	test.App.Sessions.SendPrompt(sessionID, "Later", nil)
	test.App.Sessions.Stop(sessionID)
	test.UntilIdle(sessionID)
	Expect(t, test.App.Sessions.List()[0], `{"queued":[]}`)
	if len(OfKind(test.Events(sessionID), "turnStarted")) != 1 {
		t.Fatal("a second turn started")
	}
}

func TestASessionIsRenamedAndDeletedOnceIdle(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{call("bash", map[string]any{"command": "ls"})}})
	sessionID := prompt(t, test, "List")
	test.WaitForEvent("permissionRequested", 1)
	if problem := test.App.Sessions.Rename(sessionID, "Listing"); problem != "" || test.App.Sessions.List()[0].Title != "Listing" {
		t.Fatal("not renamed")
	}
	if problem := test.App.Sessions.Remove(sessionID); !strings.Contains(problem, "stop it") {
		t.Fatalf("got %q", problem)
	}
	test.App.Sessions.Stop(sessionID)
	test.UntilIdle(sessionID)
	if problem := test.App.Sessions.Remove(sessionID); problem != "" || len(test.App.Sessions.List()) != 0 {
		t.Fatal("not removed")
	}
	if entries, _ := os.ReadDir(filepath.Join(test.DataDir, "sessions")); len(entries) != 0 {
		t.Fatalf("files left %v", entries)
	}
	test.Until("the scratch folder to go", func() bool {
		entries, _ := os.ReadDir(filepath.Join(test.DataDir, "scratch"))
		return len(entries) == 0
	})
}

func TestASessionStartsInTheFolderAskedForOrRefusesOneThatDoesNotExist(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("Hello.")}})
	folder := filepath.Join(test.Workspace, "nested")
	_ = os.Mkdir(folder, 0o755)
	if session := test.NewSession(sessions.Request{Workspace: folder}); session.Workspace != folder {
		t.Fatalf("workspace %s", session.Workspace)
	}
	if _, problem := test.App.Sessions.Create(t.Context(), sessions.Request{Workspace: filepath.Join(test.Workspace, "missing")}); !strings.Contains(problem, "not a folder") {
		t.Fatalf("got %q", problem)
	}
	if _, problem := test.App.Sessions.Create(t.Context(), sessions.Request{Workspace: "relative/path"}); problem == "" {
		t.Fatal("a relative path passed")
	}
}

func TestASessionStartsOnItsFirstMessage(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("Hello.")}})
	session := test.NewSession(sessions.Request{Prompt: "Say hello"})
	test.WaitForEvent("turnFinished", 1)
	if got := userTexts(test.Events(session.ID)); !reflect.DeepEqual(got, []string{"Say hello"}) || test.App.Sessions.List()[0].Title != "Say hello" {
		t.Fatalf("messages %v", got)
	}
}

// lastUserText is what the model read last from the person, in a request.
func lastUserText(t *testing.T, request map[string]any) string {
	t.Helper()
	messages := request["messages"].([]any)
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index].(map[string]any)
		if text, ok := message["content"].(string); ok && message["role"] == "user" {
			return text
		}
	}
	t.Fatal("no user message")
	return ""
}

func TestTokenEconomyAsksTheModelToBeConciseAndShowsTheMessageAsWritten(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("Short."), text("Long, as it likes.")}})
	session := test.NewSession(sessions.Request{Concise: true, Prompt: "Explain the plan"})
	test.UntilIdle(session.ID)
	if got := lastUserText(t, test.Requests()[0]); !strings.HasSuffix(got, "Explain the plan\n\nBe concise.") {
		t.Fatalf("the model read %q", got)
	}
	Expect(t, OfKind(test.Events(session.ID), "userMessage"), `[{"concise":true,"text":"Explain the plan"}]`)
	if problem := test.App.Sessions.SetConcise(session.ID, false); problem != "" {
		t.Fatal(problem)
	}
	test.App.Sessions.SendPrompt(session.ID, "And now in detail", nil)
	test.UntilIdle(session.ID)
	if got := lastUserText(t, test.Requests()[1]); strings.Contains(got, "Be concise.") {
		t.Fatalf("the model read %q", got)
	}
	// The first message still reads as it was sent: the start of the conversation stays the same for the cache.
	if !strings.Contains(RequestText(test.Requests()[1]), "Explain the plan\\n\\nBe concise.") {
		t.Fatal("the first message changed")
	}
	Expect(t, test.App.Sessions.List()[0], `{"concise":false}`)
}

func TestTheReadsOfOneReplyRunTogetherAndTheirResultsStayUnderTheirCalls(t *testing.T) {
	test := Start(t, Options{
		Files: map[string]string{"a.txt": "alpha\n", "b.txt": "bravo\n", "c.txt": "charlie\n"},
		Replies: []fakellama.Reply{
			{ToolCalls: []fakellama.ToolCall{
				{Arguments: map[string]any{"path": "a.txt"}, Name: "read"},
				{Arguments: map[string]any{"path": "missing.txt"}, Name: "read"},
				{Arguments: map[string]any{"path": "b.txt"}, Name: "read"},
				{Arguments: map[string]any{"path": "c.txt"}, Name: "read"},
			}},
			text("Read."),
		},
	})
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "Read them", nil)
	test.WaitForEvent("turnFinished", 1)

	// Every call is recorded in the order the model wrote them, before any runs; the one that cannot run is refused.
	events := test.Events(session.ID)
	summaries := []any{}
	for _, call := range OfKind(events, "toolCall") {
		summaries = append(summaries, call.(map[string]any)["summary"])
	}
	Expect(t, summaries, []any{"a.txt", "missing.txt", "b.txt", "c.txt"})

	// The model reads each result under its own call, in the order of the calls.
	results := []string{}
	for _, message := range fakellama.Messages(test.Requests()[1]) {
		if message["role"] == "tool" {
			results = append(results, fakellama.TextOf(message["content"]))
		}
	}
	if len(results) != 4 || !strings.Contains(results[0], "alpha") || !strings.Contains(results[1], "does not exist") || !strings.Contains(results[2], "bravo") || !strings.Contains(results[3], "charlie") {
		t.Fatalf("the results the model read: %q", results)
	}
}

func TestWhatACommandPrintsShowsWhileItStillRuns(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{
		call("bash", map[string]any{"command": "echo first; sleep 0.5; echo second"}), text("Ran."),
	}})
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "Run it", nil)
	test.WaitForEvent("turnFinished", 1)

	shown, finished := -1, -1
	for index, message := range test.Messages() {
		switch value := message.Value.(type) {
		case protocol.ServerMessageToolOutput:
			if shown < 0 && strings.Contains(value.Text, "first") && !strings.Contains(value.Text, "second") {
				shown = index
			}
		case protocol.ServerMessageSessionEvent:
			if _, done := value.Event.Value.(protocol.SessionEventToolFinished); done {
				finished = index
			}
		}
	}
	if shown < 0 || finished < shown {
		t.Fatalf("the first line showed at message %d, the result came at %d", shown, finished)
	}
	// The result still holds all the command printed.
	Expect(t, OfKind(test.Events(session.ID), "toolFinished"), `[{"output":"first\nsecond"}]`)
}
