package app_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"swcode/daemon/fakellama"
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

// startWithDeploy is a daemon whose home's .agents/skills holds the deploy skill.
func startWithDeploy(t *testing.T, options Options) *App {
	home := t.TempDir()
	put(t, home, ".agents/skills/deploy/SKILL.md", deploySkill)
	put(t, home, ".agents/skills/deploy/references/hosts.md", "staging.example.com")
	options.Home = home
	return Start(t, options)
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

func TestTheModelIsToldThePersonsSkillsThenTheProjects(t *testing.T) {
	test := startWithDeploy(t, Options{
		Files: map[string]string{
			".agents/skills/deploy/SKILL.md":        "---\nname: deploy\ndescription: The project’s deploy.\n---\nNot this one.",
			".agents/skills/release-notes/SKILL.md": "---\nname: release-notes\ndescription: Writes release notes.\n---\nList the commits.",
		},
		Replies: []fakellama.Reply{text("Done.")},
	})
	session := test.NewSession(sessions.Request{Prompt: "Hello"})
	test.UntilIdle(session.ID)
	Expect(t, listings(test, session.ID)[0], `{"skills":[
		{"description":"Deploys the app to staging. Use when asked to deploy.","name":"deploy"},
		{"description":"Writes release notes.","name":"release-notes"}]}`)
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
		"Base directory for this skill: " + filepath.Join(test.Home, ".agents", "skills", "deploy") + "\n\n1. Run `npm run build`.\n2. Read references/hosts.md for the host.\n\n[Files of this skill, to read with skill and file: references/hosts.md]",
		"staging.example.com",
		"There is no skill named ship. The skills: deploy.",
	}
	if strings.Join(outputs, "|") != strings.Join(want, "|") {
		t.Fatalf("outputs %q", outputs)
	}
}

func TestSaveskillIsOfferedNoLonger(t *testing.T) {
	test := startWithDeploy(t, Options{Replies: []fakellama.Reply{text("Done.")}})
	session := test.NewSession(sessions.Request{Prompt: "Hello"})
	test.UntilIdle(session.ID)
	offered := toolNames(test.Requests()[0])
	if !slices.Contains(offered, "skill") || slices.Contains(offered, "saveskill") {
		t.Fatalf("tools %v", offered)
	}
}

func TestTheSkillsAreToldAgainAfterACompaction(t *testing.T) {
	test := startWithDeploy(t, Options{
		// The system prompt and the tools take about 4,000 tokens; big.txt, about 4,000 more.
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
