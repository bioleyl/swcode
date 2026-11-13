package catalog

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"swcode/daemon/hosts"
)

func putAgent(t *testing.T, root, name, text string) {
	t.Helper()
	folder := filepath.Join(root, ".agents", "agents")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAnAgentsToolsAreReadOnTheirLineOrAsAList(t *testing.T) {
	for text, want := range map[string][]string{
		"---\ntools: read, write\n---\nBody":                  {"read", "write"},
		"---\nname: a\ntools: [read, \"bash\"]\n---\nBody":    {"read", "bash"},
		"---\ntools:\n  - edit\n  - grep\nname: a\n---\nBody": {"edit", "grep"},
		"---\nname: a\n---\nBody":                             nil,
		"No frontmatter, tools: read":                         nil,
	} {
		if got := toolsOf(text); !slices.Equal(got, want) {
			t.Errorf("%q gives %v, not %v", text, got, want)
		}
	}
}

func TestAgentsAreThePersonsThenTheProjectsEachNameOnce(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	putAgent(t, home, "writer.md", "---\ndescription: Writes.\ntools: write\n---\nWrite it.")
	putAgent(t, home, "notes.txt", "not an agent")
	putAgent(t, workspace, "writer.md", "---\ndescription: The project's writer.\n---\nNot this one.")
	putAgent(t, workspace, "review.md", "---\nname: reviewer\ndescription: Reviews.\n---\nRead the diff.")
	host := hosts.NewLocalHost(hosts.LocalOptions{Home: home, ScratchRoot: t.TempDir()})
	defer host.Close()

	found := FindAgents(context.Background(), host, workspace, false)
	if len(found) != 2 || found[0].Name != "writer" || found[0].Description != "Writes." || found[0].Instructions != "Write it." ||
		!slices.Equal(found[0].Tools, []string{"write"}) || found[1].Name != "reviewer" {
		t.Fatalf("found %+v", found)
	}
	// A folder the person has not trusted brings none of its own.
	if own := FindAgents(context.Background(), host, workspace, true); len(own) != 1 || own[0].Name != "writer" {
		t.Fatalf("for an untrusted folder: %+v", own)
	}
}
