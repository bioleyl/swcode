package app_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

var projectFiles = map[string]string{".cache/x": "", "api/index.ts": "", "web/index.ts": ""}

func projectNames(test *App) []string {
	names := []string{}
	for _, project := range test.App.Projects.List() {
		names = append(names, project.Name)
	}
	return names
}

func TestProjectsAreMadeOfAFolderNamedAfterItOnceForEachFolder(t *testing.T) {
	test := Start(t, Options{Files: projectFiles})
	ctx := context.Background()
	first, problem := test.App.Projects.Create(ctx, filepath.Join(test.Workspace, "web"), nil, "")
	again, _ := test.App.Projects.Create(ctx, filepath.Join(test.Workspace, "web"), nil, "")
	if problem != "" || first.Name != "web" || first.Path != filepath.Join(test.Workspace, "web") || again.ID != first.ID || len(test.App.Projects.List()) != 1 {
		t.Fatalf("first %+v, again %+v", first, again)
	}
}

func TestAProjectCanStartInANewFolder(t *testing.T) {
	test := Start(t, Options{})
	ctx := context.Background()
	created, problem := test.App.Projects.Create(ctx, test.Workspace, nil, "shop")
	if problem != "" || created.Name != "shop" || created.Path != filepath.Join(test.Workspace, "shop") {
		t.Fatalf("created %+v, %q", created, problem)
	}
	if _, err := os.Stat(filepath.Join(test.Workspace, "shop")); err != nil {
		t.Fatal("no folder")
	}
	if _, problem := test.App.Projects.Create(ctx, test.Workspace, nil, "shop"); !strings.Contains(problem, "already exists") {
		t.Fatalf("got %q", problem)
	}
}

func TestAProjectRefusesAFolderNotThereOrNotAFullPath(t *testing.T) {
	test := Start(t, Options{})
	ctx := context.Background()
	for _, attempt := range [][2]string{{filepath.Join(test.Workspace, "nope"), ""}, {"relative/path", ""}, {test.Workspace, ".."}} {
		if _, problem := test.App.Projects.Create(ctx, attempt[0], nil, attempt[1]); problem == "" {
			t.Errorf("%v passed", attempt)
		}
	}
}

func TestProjectsComeForEveryFolderASessionWorksInAndGoWithTheirSessions(t *testing.T) {
	test := Start(t, Options{Files: projectFiles})
	test.NewSession(sessions.Request{Workspace: filepath.Join(test.Workspace, "api")})
	other := test.NewSession(sessions.Request{Workspace: filepath.Join(test.Workspace, "web")})
	var api protocol.Project
	for _, project := range test.App.Projects.List() {
		if project.Name == "api" {
			api = project
		}
	}
	if api.Path != filepath.Join(test.Workspace, "api") || test.App.Projects.Remove(api.ID) != "" {
		t.Fatalf("project %+v", api)
	}
	if !reflect.DeepEqual(projectNames(test), []string{"web"}) || len(test.App.Sessions.List()) != 1 || test.App.Sessions.List()[0].ID != other.ID {
		t.Fatal("not removed with its sessions")
	}
	if _, err := os.Stat(filepath.Join(test.Workspace, "api", "index.ts")); err != nil {
		t.Fatal("the folder went too")
	}
}

func TestAProjectStaysWhileOneOfItsSessionsIsWorking(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{call("bash", map[string]any{"command": "ls"})}})
	session := test.NewSession(sessions.Request{Mode: protocol.PermissionModeAsk, Prompt: "List"})
	test.WaitForEvent("permissionRequested", 1)
	if problem := test.App.Projects.Remove(test.App.Projects.List()[0].ID); !strings.Contains(problem, "is working") {
		t.Fatalf("got %q", problem)
	}
	if test.App.Sessions.List()[0].ID != session.ID {
		t.Fatal("the session went")
	}
}

func TestProjectsAreFoundAgainAfterARestart(t *testing.T) {
	first := Start(t, Options{})
	first.App.Projects.Create(context.Background(), first.Workspace, nil, "kept")
	first.Stop()
	again := Start(t, Options{DataDir: first.DataDir})
	if !reflect.DeepEqual(projectNames(again), []string{"kept"}) {
		t.Fatalf("projects %v", projectNames(again))
	}
}

func TestFoldersAreListedByNameWithoutTheHiddenOnesWithTheParent(t *testing.T) {
	test := Start(t, Options{Files: projectFiles})
	ctx := context.Background()
	listing := test.App.Projects.Folders(ctx, test.Workspace, nil)
	if !reflect.DeepEqual(listing.Folders, []string{"api", "web"}) || listing.Parent != filepath.Dir(test.Workspace) || listing.Path != test.Workspace {
		t.Fatalf("listing %+v", listing)
	}
	if start := test.App.Projects.Folders(ctx, "", nil); !start.OK || start.Path != test.Workspace {
		t.Fatalf("start %+v", start)
	}
	if missing := test.App.Projects.Folders(ctx, filepath.Join(test.Workspace, "nope"), nil); missing.OK || !strings.Contains(missing.Problem, "not a folder") {
		t.Fatalf("missing %+v", missing)
	}
}

func TestAProjectOnAnotherHostRunsItsSessionsToolsThere(t *testing.T) {
	remoteHome := t.TempDir()
	_ = os.MkdirAll(filepath.Join(remoteHome, "app"), 0o755)
	_ = os.WriteFile(filepath.Join(remoteHome, "app", "notes.txt"), []byte("built on the box\n"), 0o644)
	host := protocol.HostRef{Value: protocol.HostRefSSH{Endpoint: "build-box"}}
	remote := hosts.NewShellHost(hosts.LocalTransport("build-box", remoteHome))
	test := Start(t, Options{
		ConnectHost: func(protocol.HostRef) hosts.Host { return remote },
		Replies:     []fakellama.Reply{call("bash", map[string]any{"command": "cat notes.txt"}), text("It says so.")},
	})
	ctx := context.Background()
	listing := test.App.Projects.Folders(ctx, "", &host)
	if !listing.OK || !reflect.DeepEqual(listing.Folders, []string{"app"}) || listing.Path != remoteHome {
		t.Fatalf("listing %+v", listing)
	}
	project, problem := test.App.Projects.Create(ctx, "~/app", &host, "")
	if problem != "" || project.Name != "app" || project.Path != filepath.Join(remoteHome, "app") || !hosts.SameHost(project.Host, host) {
		t.Fatalf("project %+v %q", project, problem)
	}
	session := test.NewSession(sessions.Request{Host: &host, Prompt: "What do the notes say?", Workspace: "~/app"})
	if !hosts.SameHost(session.Host, host) {
		t.Fatalf("host %+v", session.Host)
	}
	test.UntilIdle(session.ID)
	Expect(t, Finished(test.Events(session.ID)), `[{"ok":true,"output":"built on the box"}]`)
	if !strings.Contains(RequestText(test.Requests()[0]), "Scratch folder: "+filepath.Join(remoteHome, ".swcode", "scratch")) {
		t.Fatal("the scratch folder is not the other host's")
	}
}

func TestAProjectKeepsWhatItsNewSessionsStartWithUntilItGoesBackToTheAppsDefaults(t *testing.T) {
	first := Start(t, Options{})
	project := first.App.Projects.Ensure(first.Workspace, protocol.LocalHost())
	defaults := &protocol.SessionDefaults{Concise: false, Mode: protocol.PermissionModeAsk, Preset: "fast", Thinking: protocol.ThinkingModeLow}
	if problem := first.App.Projects.SetDefaults(project.ID, defaults); problem != "" {
		t.Fatal(problem)
	}
	if problem := first.App.Projects.SetDefaults("nope", defaults); problem == "" {
		t.Fatal("an unknown project took defaults")
	}
	first.Stop()

	again := Start(t, Options{DataDir: first.DataDir})
	kept := again.App.Projects.List()
	if len(kept) != 1 || !reflect.DeepEqual(kept[0].Defaults, defaults) {
		t.Fatalf("got %+v", kept)
	}
	again.App.Projects.SetDefaults(project.ID, nil)
	if got := again.App.Projects.List()[0].Defaults; got != nil {
		t.Fatalf("still %+v", got)
	}
}
