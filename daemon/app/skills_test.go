package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

const deploySkill = `---
name: deploy
description: Deploys the app to staging. Use when asked to deploy.
license: MIT
metadata:
  owner: syware
---

1. Run ` + "`npm run build`" + `.
2. Read references/hosts.md for the host.
`

func put(t *testing.T, root, path, content string) {
	t.Helper()
	target := filepath.Join(root, path)
	_ = os.MkdirAll(filepath.Dir(target), 0o755)
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// startWithDeploy is a daemon whose skills folder holds the deploy skill.
func startWithDeploy(t *testing.T, options Options) *App {
	dataDir := t.TempDir()
	put(t, dataDir, "skills/deploy/SKILL.md", deploySkill)
	put(t, dataDir, "skills/deploy/references/hosts.md", "staging.example.com")
	options.DataDir = dataDir
	return Start(t, options)
}

func TestTheSkillsPageListsOpensAndSavesASkillRenamed(t *testing.T) {
	test := startWithDeploy(t, Options{})
	_, list := test.Call("GET", protocol.EndpointSkills, nil)
	Expect(t, list, map[string]any{"folder": filepath.Join(test.DataDir, "skills"), "skills": []any{map[string]any{"description": "Deploys the app to staging. Use when asked to deploy.", "name": "deploy"}}})
	_, opened := test.Call("GET", protocol.EndpointSkills+"/deploy", nil)
	Expect(t, opened, `{"files":["references/hosts.md"],"name":"deploy"}`)
	_, saved := test.Call("PUT", protocol.EndpointSkills+"/deploy", map[string]string{"description": "Ships it.", "instructions": "Ship.", "name": "ship"})
	Expect(t, saved, `{"name":"ship","ok":true}`)
	if status, _ := test.Call("GET", protocol.EndpointSkills+"/deploy", nil); status != 404 {
		t.Fatalf("status %d", status)
	}
	if status, _ := test.Call("GET", protocol.EndpointSkills+"/ship", nil); status != 200 {
		t.Fatalf("status %d", status)
	}
}

func TestTheSkillsPageCreatesAndDeletesASkillAndSaysWhyItRefusesOne(t *testing.T) {
	test := Start(t, Options{})
	_, created := test.Call("POST", protocol.EndpointSkills, map[string]string{"description": "Writes release notes.", "instructions": "List the commits.", "name": "release-notes"})
	Expect(t, created, `{"name":"release-notes","ok":true}`)
	status, refused := test.Call("POST", protocol.EndpointSkills, map[string]string{"description": "", "instructions": "x", "name": "Release Notes"})
	if status != 400 || !regexp.MustCompile(`lowercase letters|must not be empty`).MatchString(refused.(map[string]any)["problem"].(string)) {
		t.Fatalf("status %d: %v", status, refused)
	}
	_, removed := test.Call("DELETE", protocol.EndpointSkills+"/release-notes", nil)
	Expect(t, removed, `{"name":"release-notes","ok":true}`)
	_, list := test.Call("GET", protocol.EndpointSkills, nil)
	Expect(t, list, `{"skills":[]}`)
}

func listings(test *App, sessionID string) []any {
	return OfKind(test.Events(sessionID), "skillsListed")
}

func TestTheModelIsToldTheSkillsAfterTheFirstMessageInTheSameUserMessage(t *testing.T) {
	test := startWithDeploy(t, Options{Replies: []fakellama.Reply{text("Done."), text("Again.")}})
	session := test.NewSession(sessions.Request{Prompt: "Deploy it"})
	test.UntilIdle(session.ID)
	Expect(t, Kinds(test.Events(session.ID)), []any{"turnStarted", "userMessage", "skillsListed", "assistantMessage", "turnFinished"})
	messages := fakellama.Messages(test.Requests()[0])
	if len(messages) != 2 || messages[0]["role"] != "system" || messages[1]["role"] != "user" {
		t.Fatalf("roles %v", messages)
	}
	if !regexp.MustCompile(`Deploy it\n\nThe user’s skills: [^\n]*\n- deploy: Deploys the app to staging\. Use when asked to deploy\.$`).MatchString(messages[1]["content"].(string)) {
		t.Fatalf("content %q", messages[1]["content"])
	}
	// Unchanged, the skills are not told again.
	test.App.Sessions.SendPrompt(session.ID, "Once more", nil)
	test.WaitForEvent("turnFinished", 2)
	if len(listings(test, session.ID)) != 1 {
		t.Fatal("told twice")
	}
}

func TestNothingIsToldWhenThereAreNoSkills(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("Done.")}})
	session := test.NewSession(sessions.Request{Prompt: "Hello"})
	test.UntilIdle(session.ID)
	if len(listings(test, session.ID)) != 0 {
		t.Fatal("skills told")
	}
}

func TestASkillIsLoadedAndAFileOfItsFolder(t *testing.T) {
	test := startWithDeploy(t, Options{Replies: []fakellama.Reply{
		call("skill", map[string]any{"name": "deploy"}),
		call("skill", map[string]any{"file": "references/hosts.md", "name": "deploy"}),
		call("skill", map[string]any{"name": "ship"}),
		text("Deployed."),
	}})
	session := test.NewSession(sessions.Request{Prompt: "Deploy it"})
	test.UntilIdle(session.ID)
	outputs := []string{}
	for _, finished := range Finished(test.Events(session.ID)) {
		outputs = append(outputs, finished.Output)
	}
	want := []string{
		"Base directory for this skill: " + filepath.Join(test.DataDir, "skills", "deploy") + "\n\n1. Run `npm run build`.\n2. Read references/hosts.md for the host.\n\n[Files of this skill, to read with skill and file: references/hosts.md]",
		"staging.example.com",
		"There is no skill named ship. The skills: deploy.",
	}
	if strings.Join(outputs, "|") != strings.Join(want, "|") {
		t.Fatalf("outputs %q", outputs)
	}
}

func TestASkillTheUserAsksForIsSavedAndToldAtTheNextTurn(t *testing.T) {
	skill := map[string]any{"description": "Writes release notes.", "instructions": "List the commits.", "name": "release-notes"}
	test := startWithDeploy(t, Options{Replies: []fakellama.Reply{call("saveskill", skill), text("Saved."), text("Hi.")}})
	session := test.NewSession(sessions.Request{Prompt: "Make a skill for release notes"})
	test.UntilIdle(session.ID)
	Expect(t, Finished(test.Events(session.ID))[0], map[string]any{
		"display": map[string]any{"added": float64(6), "kind": "diff", "path": "skills/release-notes/SKILL.md", "removed": float64(0)},
		"ok":      true, "output": "Created the skill release-notes. Sessions are told of it from their next turn on.",
	})
	data, _ := os.ReadFile(filepath.Join(test.DataDir, "skills", "release-notes", "SKILL.md"))
	if string(data) != "---\nname: release-notes\ndescription: Writes release notes.\n---\n\nList the commits.\n" {
		t.Fatalf("file %q", data)
	}
	test.App.Sessions.SendPrompt(session.ID, "Hello", nil)
	test.WaitForEvent("turnFinished", 2)
	told := listings(test, session.ID)
	Expect(t, told[len(told)-1], `{"skills":[{"name":"deploy"},{"description":"Writes release notes.","name":"release-notes"}]}`)
}

func TestTheSkillsAreToldAgainAfterACompaction(t *testing.T) {
	test := startWithDeploy(t, Options{
		// The system prompt and the tools take about 4,200 tokens; big.txt, about 4,000 more.
		ContextTokens: 6000,
		Files:         map[string]string{"big.txt": strings.Repeat(strings.Repeat("x", 60)+"\n", 200)},
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
	if strings.Join(kinds[at:], " ") != "contextCompacted skillsListed assistantMessage turnFinished" || len(listings(test, session.ID)) != 2 {
		t.Fatalf("kinds %v", kinds)
	}
	requests := test.Requests()
	if !strings.Contains(RequestText(requests[len(requests)-1]), "- deploy: Deploys the app") {
		t.Fatal("the skills were not told after the summary")
	}
}

func TestSavingASkillIsAskedAboutInASessionThatAsksBeforeEdits(t *testing.T) {
	skill := map[string]any{"description": "Writes release notes.", "instructions": "List the commits.", "name": "release-notes"}
	test := Start(t, Options{Replies: []fakellama.Reply{call("saveskill", skill)}})
	test.NewSession(sessions.Request{Mode: protocol.PermissionModeAsk, Prompt: "Make a skill"})
	Expect(t, test.WaitForEvent("permissionRequested", 1), `{"detail":{"kind":"diff","path":"skills/release-notes/SKILL.md"}}`)
}

// startWithClaude is a daemon whose home has Claude Code's skills and a plugin, its data folder swcode's own deploy
// skill.
func startWithClaude(t *testing.T, options Options) *App {
	home, dataDir := t.TempDir(), t.TempDir()
	put(t, home, ".claude/skills/fix-issue/SKILL.md", "---\ndescription: Fixes a GitHub issue.\nargument-hint: \"[issue-number]\"\n---\n\nFix issue $0 with ${CLAUDE_SKILL_DIR}/check.sh.")
	put(t, home, ".claude/skills/fix-issue/check.sh", "echo ok")
	put(t, home, ".claude/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Claude Code’s deploy.\n---\nNot this one.")
	put(t, home, ".claude/commands/commit.md", "---\ndescription: Commits.\ndisable-model-invocation: true\n---\nCommit.")
	installPath := filepath.Join(home, ".claude", "plugins", "cache", "jadis", "jadis", "1.0.0")
	put(t, installPath, "skills/jadis/SKILL.md", "---\nname: jadis\ndescription: Builds with Jadis.\n---\nJadis.")
	installed, _ := json.Marshal(map[string]any{"plugins": map[string]any{"jadis@jadis": []any{map[string]any{"installPath": installPath, "scope": "user"}}}, "version": 2})
	put(t, home, ".claude/plugins/installed_plugins.json", string(installed))
	put(t, dataDir, "skills/deploy/SKILL.md", "---\nname: deploy\ndescription: swcode’s deploy.\n---\nDeploy.")
	options.DataDir, options.Home = dataDir, home
	return Start(t, options)
}

func TestTheModelIsToldSwcodesSkillsThenClaudeCodesThoseForPeopleAside(t *testing.T) {
	test := startWithClaude(t, Options{Replies: []fakellama.Reply{text("Done.")}})
	session := test.NewSession(sessions.Request{Prompt: "Hello"})
	test.UntilIdle(session.ID)
	Expect(t, listings(test, session.ID)[0], `{"skills":[
		{"description":"swcode’s deploy.","name":"deploy"},
		{"description":"Fixes a GitHub issue. Arguments: [issue-number].","name":"fix-issue"},
		{"description":"Builds with Jadis.","name":"jadis:jadis"}]}`)
}

func TestAClaudeCodeSkillIsLoadedWithItsArgumentsAndItsFolder(t *testing.T) {
	test := startWithClaude(t, Options{Replies: []fakellama.Reply{
		call("skill", map[string]any{"arguments": "42", "name": "fix-issue"}),
		call("skill", map[string]any{"file": "check.sh", "name": "fix-issue"}),
		call("skill", map[string]any{"name": "commit"}),
		text("Fixed."),
	}})
	session := test.NewSession(sessions.Request{Prompt: "Fix issue 42"})
	test.UntilIdle(session.ID)
	folder := filepath.Join(test.Home, ".claude", "skills", "fix-issue")
	outputs := []string{}
	for _, finished := range Finished(test.Events(session.ID)) {
		outputs = append(outputs, finished.Output)
	}
	want := []string{
		"Base directory for this skill: " + folder + "\n\nFix issue 42 with " + folder + "/check.sh.\n\n[Files of this skill, to read with skill and file: check.sh]",
		"echo ok",
		"commit is for the person to type as a command in Claude Code: its author keeps it from agents.",
	}
	if strings.Join(outputs, "|") != strings.Join(want, "|") {
		t.Fatalf("outputs %q", outputs)
	}
}

func TestClaudeCodesSkillsAreLeftOutOnceTurnedOff(t *testing.T) {
	test := startWithClaude(t, Options{Replies: []fakellama.Reply{text("Done.")}})
	_, saved := test.Call("PUT", protocol.EndpointClaude, map[string]any{"personal": false, "plugins": map[string]bool{"jadis@jadis": false}})
	Expect(t, saved, `{"personal":false,"plugins":{"jadis@jadis":false},"project":true}`)
	session := test.NewSession(sessions.Request{Prompt: "Hello"})
	test.UntilIdle(session.ID)
	Expect(t, listings(test, session.ID)[0], `{"skills":[{"name":"deploy"}]}`)
}

func TestTheSettingsPageIsShownWhatClaudeCodeHasOnThisComputer(t *testing.T) {
	test := startWithClaude(t, Options{})
	_, view := test.Call("GET", protocol.EndpointClaude+"?host=local", nil)
	Expect(t, view, map[string]any{
		"folder": filepath.Join(test.Home, ".claude"),
		"hosts":  []any{map[string]any{"label": "This computer", "ref": map[string]any{"kind": "local"}}},
		"personal": []any{
			map[string]any{"forPeople": false, "name": "deploy"},
			map[string]any{"forPeople": false, "name": "fix-issue"},
			map[string]any{"forPeople": true, "name": "commit"},
		},
		"plugins": []any{map[string]any{"enabledInClaude": true, "id": "jadis@jadis", "on": true, "skills": []any{map[string]any{"name": "jadis:jadis"}}}},
		"sources": map[string]any{"personal": true, "plugins": map[string]any{}, "project": true},
	})
}
