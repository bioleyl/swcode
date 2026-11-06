package skills

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
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

func TestASkillFileHasItsNameDescriptionThenItsInstructions(t *testing.T) {
	want := File{
		Description:  "Deploys the app to staging. Use when asked to deploy.",
		Instructions: "1. Run `npm run build`.\n2. Read references/hosts.md for the host.",
		Name:         "deploy",
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

func TestADescriptionIsAsLongAsAgentSkillsAllow(t *testing.T) {
	got := Parse("---\ndescription: " + strings.Repeat("é", maxDescription+10) + "\n---\nDo it.").Description
	if utf8.RuneCountInString(got) != maxDescription {
		t.Fatalf("got %d characters", utf8.RuneCountInString(got))
	}
}

func TestAFileWithoutFrontmatterIsAllInstructions(t *testing.T) {
	if got := Parse("Just do it.\n"); got.Instructions != "Just do it." || got.Name != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestALoadedSkillNamesItsFolderThenItsFiles(t *testing.T) {
	got := LoadedText("/home/me/.agents/skills/deploy", "Deploy.", []string{"references/hosts.md"})
	want := "Base directory for this skill: /home/me/.agents/skills/deploy\n\nDeploy.\n\n[Files of this skill, to read with skill and file: references/hosts.md]"
	if got != want {
		t.Fatalf("got %q", got)
	}
}
