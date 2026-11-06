package catalog

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
)

func skillFile(name, description, body string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body + "\n"
}

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

func localHost(t *testing.T, home string) hosts.Host {
	host := hosts.NewLocalHost(hosts.LocalOptions{Home: home, ScratchRoot: t.TempDir()})
	t.Cleanup(host.Close)
	return host
}

// personalHome is a home whose .agents/skills holds the deploy and pixel-art skills, and a folder that is no skill.
func personalHome(t *testing.T) string {
	home := t.TempDir()
	put(t, home, ".agents/skills/deploy/SKILL.md", skillFile("deploy", "The person’s deploy.", "Deploy."))
	put(t, home, ".agents/skills/pixel-art/SKILL.md", "---\ndescription: Makes sprites.\n---\nRun scripts/pixelize.mjs.")
	put(t, home, ".agents/skills/pixel-art/scripts/pixelize.mjs", "echo ok")
	put(t, home, ".agents/skills/notes/README.md", "not a skill")
	return home
}

// project is a repository whose root and apps/web have skills: the session works in apps/web.
func project(t *testing.T) string {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	put(t, root, ".agents/skills/deploy/SKILL.md", skillFile("deploy", "The project’s deploy.", "Not this one."))
	put(t, root, ".agents/skills/release-notes/SKILL.md", skillFile("release-notes", "Writes release notes.", "List the commits."))
	put(t, root, "apps/web/.agents/skills/web-build/SKILL.md", skillFile("web-build", "Builds the web app.", "Build."))
	return filepath.Join(root, "apps", "web")
}

func names(found []HostSkill) []string {
	list := []string{}
	for _, each := range found {
		list = append(list, each.Name)
	}
	return list
}

func TestThePersonsSkillsThenTheProjectsUpToTheRepositorysRoot(t *testing.T) {
	found, err := Find(context.Background(), localHost(t, personalHome(t)), project(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := names(found); !reflect.DeepEqual(got, []string{"deploy", "pixel-art", "web-build", "deploy", "release-notes"}) {
		t.Fatalf("got %v", got)
	}
}

func TestAProjectOutsideARepositoryHasItsOwnFolderAlone(t *testing.T) {
	outside := t.TempDir()
	put(t, outside, ".agents/skills/above/SKILL.md", skillFile("above", "Above.", "x"))
	put(t, outside, "app/.agents/skills/own/SKILL.md", skillFile("own", "Its own.", "x"))
	found, _ := Find(context.Background(), localHost(t, t.TempDir()), filepath.Join(outside, "app"))
	if got := names(found); !reflect.DeepEqual(got, []string{"own"}) {
		t.Fatalf("got %v", got)
	}
}

func TestTheAgentIsOfferedEachNameOnceThePersonsFirst(t *testing.T) {
	port := New(func(error) {}).PortFor(Place{Host: localHost(t, personalHome(t)), Workspace: project(t)})
	want := []protocol.SkillSummary{
		{Description: "The person’s deploy.", Name: "deploy"},
		{Description: "Makes sprites.", Name: "pixel-art"},
		{Description: "Builds the web app.", Name: "web-build"},
		{Description: "Writes release notes.", Name: "release-notes"},
	}
	if got := port.List(context.Background()); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestASkillIsLoadedWithItsFolderAndAFileOfIt(t *testing.T) {
	home := personalHome(t)
	port := New(func(error) {}).PortFor(Place{Host: localHost(t, home), Workspace: t.TempDir()})
	ctx := context.Background()
	folder := filepath.Join(home, ".agents", "skills", "pixel-art")
	want := "Base directory for this skill: " + folder + "\n\nRun scripts/pixelize.mjs.\n\n[Files of this skill, to read with skill and file: scripts/pixelize.mjs]"
	if got := port.Load(ctx, "pixel-art"); got.Text != want {
		t.Fatalf("got %q", got.Text)
	}
	if got := port.ReadFile(ctx, "pixel-art", "scripts/pixelize.mjs"); got.Text != "echo ok" {
		t.Fatalf("got %+v", got)
	}
	if got := port.ReadFile(ctx, "pixel-art", "../deploy/SKILL.md"); !strings.Contains(got.Problem, "outside") {
		t.Fatalf("got %+v", got)
	}
	if got := port.Load(ctx, "ship"); got.Problem != "There is no skill named ship. The skills: deploy, pixel-art." {
		t.Fatalf("got %+v", got)
	}
}

func TestAnotherMachineIsReadThroughItsBash(t *testing.T) {
	remote := hosts.NewShellHost(hosts.LocalTransport("build-box", personalHome(t)))
	defer remote.Close()
	found, err := Find(context.Background(), remote, project(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := names(found); !reflect.DeepEqual(got, []string{"deploy", "pixel-art", "web-build", "deploy", "release-notes"}) {
		t.Fatalf("got %v", got)
	}
}
