package skills

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"swcode/daemon/protocol"
)

const deploy = `---
name: deploy
description: Deploys the app to staging. Use when asked to deploy.
license: MIT
metadata:
  owner: syware
---

1. Run ` + "`npm run build`" + `.
2. Read references/hosts.md for the host.
`

func put(t *testing.T, root, path, text string) {
	t.Helper()
	target := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestASkillFileHasItsNameDescriptionThenItsInstructions(t *testing.T) {
	want := File{
		ArgumentNames: []string{},
		Description:   "Deploys the app to staging. Use when asked to deploy.",
		Extra:         "license: MIT\nmetadata:\n  owner: syware",
		Instructions:  "1. Run `npm run build`.\n2. Read references/hosts.md for the host.",
		Name:          "deploy",
	}
	if got := Parse(deploy); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestADescriptionMayBeQuotedOrFoldedOverSeveralLines(t *testing.T) {
	description := func(frontmatter string) string { return Parse("---\n" + frontmatter + "\n---\nDo it.").Description }
	cases := map[string]string{
		`description: "Says \"hi\": politely"`:                `Says "hi": politely`,
		`description: 'It''s short'`:                          "It's short",
		"description: >\n  Builds the docs.\n  Use for docs.": "Builds the docs. Use for docs.",
		"description: Builds the docs\n  over two lines.":     "Builds the docs over two lines.",
		"description: |\n  One\n  Two":                        "One\nTwo",
	}
	for frontmatter, want := range cases {
		if got := description(frontmatter); got != want {
			t.Errorf("%q: got %q", frontmatter, got)
		}
	}
}

func TestAFileWithoutFrontmatterIsAllInstructions(t *testing.T) {
	if got := Parse("Just do it.\n"); got.Instructions != "Just do it." || got.Name != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestASkillIsWrittenBackWithItsOtherKeysQuotingADescriptionYAMLWouldMisread(t *testing.T) {
	text := FileText(protocol.Skill{Description: "Deploys: to staging", Instructions: "Run it.", Name: "deploy"}, "license: MIT")
	if text != "---\nname: deploy\ndescription: \"Deploys: to staging\"\nlicense: MIT\n---\n\nRun it.\n" {
		t.Fatalf("got %q", text)
	}
	if back := Parse(text); back.Description != "Deploys: to staging" || back.Instructions != "Run it." {
		t.Fatalf("read back %+v", back)
	}
}

func TestClaudeCodesFrontmatterKeys(t *testing.T) {
	file := Parse(strings.Join([]string{
		"---", "description: Fixes an issue.", "when_to_use: When an issue number is given.",
		`argument-hint: "[issue-number]"`, "disable-model-invocation: Yes", "arguments:", "  - issue", "  - branch",
		"---", "Fix it.",
	}, "\n"))
	if file.ArgumentHint != "[issue-number]" || !reflect.DeepEqual(file.ArgumentNames, []string{"issue", "branch"}) || !file.ForPeople {
		t.Fatalf("got %+v", file)
	}
	if got := ListedDescription(file); got != "Fixes an issue. When an issue number is given. Arguments: [issue-number]." {
		t.Fatalf("listed %q", got)
	}
	if got := Parse("---\narguments: [a, \"b\"]\n---\nx").ArgumentNames; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("got %v", got)
	}
	if got := Parse("---\narguments: a b\n---\nx").ArgumentNames; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("got %v", got)
	}
	if got := ListedDescription(Parse("# Release notes\n\nList the commits.")); got != "Release notes" {
		t.Fatalf("got %q", got)
	}
}

func TestTheArgumentsAreSplitAsAShellDoes(t *testing.T) {
	got := SplitArguments(`123 "two words" 'single quoted' "say \"hi\""`)
	if !reflect.DeepEqual(got, []string{"123", "two words", "single quoted", `say "hi"`}) {
		t.Fatalf("got %q", got)
	}
}

func TestTheArgumentsGoInPlaceOfArgumentsTheirPositionsAndTheirNames(t *testing.T) {
	values := Values{VariableSkillDir: "/skills/fix"}
	cases := []struct {
		text, given string
		names       []string
		want        string
	}{
		{"Fix issue $ARGUMENTS.", "123 urgent", nil, "Fix issue 123 urgent."},
		{"Fix $0, then $ARGUMENTS[1].", `123 "the docs"`, nil, "Fix 123, then the docs."},
		{"Fix $issue in $branch.", "123", []string{"issue", "branch"}, "Fix 123 in ."},
		{"Run ${CLAUDE_SKILL_DIR}/fix.sh $0", "7", nil, "Run /skills/fix/fix.sh 7"},
		// A position with no argument, an escaped dollar and $HOME stay as they are.
		{"awk '{print $1}' in $HOME", "", nil, "awk '{print $1}' in $HOME"},
		{`Costs \$1.00, for $0`, "you", nil, "Costs $1.00, for you"},
		{"Unknown ${CLAUDE_NOTHING}", "", nil, "Unknown ${CLAUDE_NOTHING}"},
		// Without a placeholder, the arguments come at the end.
		{"Review the change.", "src/app.ts", nil, "Review the change.\n\nARGUMENTS: src/app.ts"},
		{"Review the change.", "", nil, "Review the change."},
		// Claude Code's commands are left for the agent to run.
		{"Changes:\n!`git diff --stat`\nKEY=!`no`", "", nil, "Changes:\n[the output of `git diff --stat`: run it with bash]\nKEY=!`no`"},
	}
	for _, each := range cases {
		if got := WithArguments(each.text, each.given, each.names, values); got != each.want {
			t.Errorf("%q with %q: got %q", each.text, each.given, got)
		}
	}
	if got := WithArguments("```!\nnpm test\n```", "", nil, values); !strings.Contains(got, "```bash\nnpm test\n```") {
		t.Errorf("got %q", got)
	}
}

func TestTheStoreStartsANewFolderWithTheDefaultSkillsAndLeavesOneThatExists(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "skills"))
	files := fstest.MapFS{"deploy/SKILL.md": {Data: []byte(deploy)}}
	if err := store.Seed(files); err != nil {
		t.Fatal(err)
	}
	want := []protocol.SkillSummary{{Description: "Deploys the app to staging. Use when asked to deploy.", Name: "deploy"}}
	if got := store.List(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	store.Remove("deploy")
	_ = store.Seed(files)
	if got := store.List(); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestTheStoreListsFoldersWithASkillFileAndReadsOneWithItsFiles(t *testing.T) {
	folder := t.TempDir()
	put(t, folder, "deploy/SKILL.md", deploy)
	put(t, folder, "deploy/references/hosts.md", "staging.example.com")
	put(t, folder, "notes/README.md", "not a skill")
	put(t, folder, "Bad Name/SKILL.md", deploy)
	store := NewStore(folder)
	if got := store.List(); len(got) != 1 || got[0].Name != "deploy" {
		t.Fatalf("got %+v", got)
	}
	if view, _ := store.Read("deploy"); !reflect.DeepEqual(view.Files, []string{"references/hosts.md"}) {
		t.Fatalf("got %+v", view)
	}
	if got := store.ReadFile("deploy", "references/hosts.md"); got != (Text{OK: true, Text: "staging.example.com"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestTheStoreReadsNoFileOutsideASkillsFolder(t *testing.T) {
	folder := t.TempDir()
	put(t, folder, "deploy/SKILL.md", deploy)
	put(t, folder, "secret.txt", "no")
	if err := os.Symlink(filepath.Join(folder, "secret.txt"), filepath.Join(folder, "deploy", "link.txt")); err != nil {
		t.Fatal(err)
	}
	store := NewStore(folder)
	for _, path := range []string{"../secret.txt", "link.txt"} {
		if got := store.ReadFile("deploy", path); !strings.Contains(got.Problem, "outside") {
			t.Errorf("%s: got %+v", path, got)
		}
	}
	if got := store.ReadFile("deploy", "nope.md"); got.Problem != "The skill deploy has no file nope.md." {
		t.Fatalf("got %+v", got)
	}
	if _, found := store.Read("../deploy"); found {
		t.Fatal("read outside the folder")
	}
}

func TestTheStoreRenamesASkillWithItsFolderKeepingItsOtherKeys(t *testing.T) {
	folder := t.TempDir()
	put(t, folder, "deploy/SKILL.md", deploy)
	put(t, folder, "deploy/references/hosts.md", "staging.example.com")
	store := NewStore(folder)
	change := store.Update("deploy", protocol.Skill{Description: "Ships it.", Instructions: "Ship.", Name: "ship"})
	if done, ok := change.Value.(protocol.SkillChangeOk); !ok || done.Name != "ship" {
		t.Fatalf("got %+v", change.Value)
	}
	if _, err := os.Stat(filepath.Join(folder, "deploy")); err == nil {
		t.Fatal("the old folder stayed")
	}
	data, _ := os.ReadFile(filepath.Join(folder, "ship", "SKILL.md"))
	if !strings.Contains(string(data), "license: MIT\nmetadata:\n  owner: syware") {
		t.Fatalf("got %s", data)
	}
}

func TestTheStoreRefusesANameTakenAndASkillThatIsNotThere(t *testing.T) {
	folder := t.TempDir()
	put(t, folder, "deploy/SKILL.md", deploy)
	put(t, folder, "ship/SKILL.md", deploy)
	store := NewStore(folder)
	skill := protocol.Skill{Description: "Ships it.", Instructions: "Ship.", Name: "ship"}
	problem := func(change protocol.SkillChange) string { return change.Value.(protocol.SkillChangeFailed).Problem }
	if got := problem(store.Create(skill)); got != "There is already a skill named ship." {
		t.Errorf("create: %s", got)
	}
	if got := problem(store.Update("deploy", skill)); got != "There is already a skill named ship." {
		t.Errorf("update: %s", got)
	}
	if got := problem(store.Remove("nope")); got != "There is no skill named nope." {
		t.Errorf("remove: %s", got)
	}
}

func TestTheDefaultSkillsAreTheCodingStyleAndJadisWithTheirReferences(t *testing.T) {
	files := []string{}
	_ = fs.WalkDir(os.DirFS("../../skills"), ".", func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	want := []string{"coding-style/SKILL.md", "jadis/SKILL.md", "jadis/references/api.md", "jadis/references/routing.md", "jadis/references/setup.md"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("got %v", files)
	}
}
