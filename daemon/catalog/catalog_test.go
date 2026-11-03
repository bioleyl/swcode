package catalog

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"swcode/daemon/claude"
	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/skills"
)

func put(t *testing.T, root, path, text string) {
	t.Helper()
	target := filepath.Join(root, path)
	_ = os.MkdirAll(filepath.Dir(target), 0o755)
	if err := os.WriteFile(target, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

type sources struct{ value protocol.ClaudeSources }

func (s *sources) Load() protocol.ClaudeSources            { return s.value }
func (s *sources) Save(value protocol.ClaudeSources) error { s.value = value; return nil }

// setUp is a home with Claude Code's skills, and swcode's own deploy skill, which wins over Claude Code's.
func setUp(t *testing.T) (*Catalog, Place, string) {
	home, data := t.TempDir(), t.TempDir()
	put(t, home, ".claude/skills/fix-issue/SKILL.md", "---\ndescription: Fixes a GitHub issue.\nargument-hint: \"[issue-number]\"\n---\n\nFix issue $0 with ${CLAUDE_SKILL_DIR}/check.sh.")
	put(t, home, ".claude/skills/fix-issue/check.sh", "echo ok")
	put(t, home, ".claude/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Claude Code’s deploy.\n---\nNot this one.")
	put(t, home, ".claude/commands/commit.md", "---\ndescription: Commits.\ndisable-model-invocation: true\n---\nCommit.")
	put(t, data, "skills/deploy/SKILL.md", "---\nname: deploy\ndescription: swcode’s deploy.\n---\nDeploy $ARGUMENTS.")
	host := hosts.NewLocalHost(hosts.LocalOptions{Home: home, ScratchRoot: t.TempDir()})
	t.Cleanup(host.Close)
	catalog := New(skills.NewStore(filepath.Join(data, "skills")), &sources{value: claude.AllSources()}, func(error) {})
	return catalog, Place{Host: host, Local: true, SessionID: "s1", Workspace: t.TempDir()}, home
}

func TestTheAgentIsOfferedSwcodesSkillsThenClaudeCodesThoseForPeopleAside(t *testing.T) {
	catalog, place, _ := setUp(t)
	got := catalog.PortFor(place).List(context.Background())
	want := []protocol.SkillSummary{
		{Description: "swcode’s deploy.", Name: "deploy"},
		{Description: "Fixes a GitHub issue. Arguments: [issue-number].", Name: "fix-issue"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestASkillIsLoadedWithItsArgumentsAndItsFolder(t *testing.T) {
	catalog, place, home := setUp(t)
	port := catalog.PortFor(place)
	ctx := context.Background()
	folder := filepath.Join(home, ".claude", "skills", "fix-issue")
	want := "Base directory for this skill: " + folder + "\n\nFix issue 42 with " + folder + "/check.sh.\n\n[Files of this skill, to read with skill and file: check.sh]"
	if got := port.Load(ctx, "fix-issue", "42"); got.Text != want {
		t.Fatalf("got %q", got.Text)
	}
	if got := port.ReadFile(ctx, "fix-issue", "check.sh"); got.Text != "echo ok" {
		t.Fatalf("got %+v", got)
	}
	if got := port.ReadFile(ctx, "fix-issue", "../deploy/SKILL.md"); !strings.Contains(got.Problem, "outside") {
		t.Fatalf("got %+v", got)
	}
	if got := port.Load(ctx, "commit", ""); got.Problem != skills.ForPeopleOnly("commit") {
		t.Fatalf("got %+v", got)
	}
	if got := port.Load(ctx, "deploy", "staging"); !strings.HasSuffix(got.Text, "Deploy staging.") {
		t.Fatalf("got %+v", got)
	}
	if got := port.Load(ctx, "ship", ""); got.Problem != "There is no skill named ship. The skills: deploy, fix-issue." {
		t.Fatalf("got %+v", got)
	}
}

func TestSwcodesOwnSkillsNameNoFolderWhereCommandsCannotReachIt(t *testing.T) {
	catalog, place, _ := setUp(t)
	place.Local = false
	if got := catalog.PortFor(place).Load(context.Background(), "deploy", ""); got.Text != "Deploy ." {
		t.Fatalf("got %q", got.Text)
	}
}
