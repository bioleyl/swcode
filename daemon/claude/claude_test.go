package claude

import (
	"context"
	"encoding/json"
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

// InstallPlugin installs a plugin in Claude Code's cache, recorded in installed_plugins.json as Claude Code writes
// it; projectPath installs it for that project only.
func InstallPlugin(t *testing.T, home, id string, files map[string]string, projectPath string) string {
	t.Helper()
	name, marketplace, _ := strings.Cut(id, "@")
	installPath := filepath.Join(home, ".claude", "plugins", "cache", marketplace, name, "1.0.0")
	for path, text := range files {
		put(t, installPath, path, text)
	}
	file := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	recorded := map[string]any{"plugins": map[string]any{}, "version": 2}
	if data, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(data, &recorded)
	}
	scope := "user"
	if projectPath != "" {
		scope = "project"
	}
	recorded["plugins"].(map[string]any)[id] = []any{map[string]any{"installPath": installPath, "projectPath": projectPath, "scope": scope, "version": "1.0.0"}}
	data, _ := json.Marshal(recorded)
	put(t, home, ".claude/plugins/installed_plugins.json", string(data))
	return installPath
}

func localHost(t *testing.T, home string) hosts.Host {
	host := hosts.NewLocalHost(hosts.LocalOptions{Home: home, ScratchRoot: t.TempDir()})
	t.Cleanup(host.Close)
	return host
}

func all() protocol.ClaudeSources { return AllSources() }

func names(found []HostSkill) []string {
	list := []string{}
	for _, each := range found {
		list = append(list, each.Name)
	}
	return list
}

func personalHome(t *testing.T) string {
	home := t.TempDir()
	put(t, home, ".claude/skills/pixel-art/SKILL.md", skillFile("pixel-art", "Makes sprites.", "Run scripts/pixelize.mjs."))
	put(t, home, ".claude/skills/pixel-art/scripts/pixelize.mjs", "")
	put(t, home, ".claude/skills/notes/README.md", "not a skill")
	put(t, home, ".claude/skills/synced/docx/SKILL.md", skillFile("docx", "claude.ai only.", "x"))
	put(t, home, ".claude/commands/frontend/component.md", "---\ndescription: Makes a component.\n---\nMake $0.")
	put(t, home, ".claude/commands/commit.md", "---\ndisable-model-invocation: true\n---\nCommit.")
	return home
}

func TestThePersonsSkillsAndCommandsAreFoundSyncedOnesAside(t *testing.T) {
	home := personalHome(t)
	found, err := Read(context.Background(), Query{Host: localHost(t, home), Sources: all()})
	if err != nil {
		t.Fatal(err)
	}
	if found.Folder != filepath.Join(home, ".claude") {
		t.Fatalf("folder %s", found.Folder)
	}
	type line struct {
		name, description string
		forPeople         bool
	}
	got := []line{}
	for _, each := range found.Personal {
		got = append(got, line{each.Name, each.Description, each.ForPeople})
	}
	want := []line{{"pixel-art", "Makes sprites.", false}, {"commit", "Commit.", true}, {"frontend:component", "Makes a component.", false}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if found.Personal[0].Folder != filepath.Join(home, ".claude/skills/pixel-art") || found.Personal[0].Origin != OriginClaude {
		t.Fatalf("got %+v", found.Personal[0])
	}
}

func TestAProjectsSkillsAreFoundUpToTheRepositorysRoot(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	put(t, project, ".claude/skills/deploy/SKILL.md", skillFile("deploy", "Deploys.", "Deploy."))
	put(t, project, "apps/web/.claude/skills/web-build/SKILL.md", skillFile("web-build", "Builds the web app.", "Build."))
	found, _ := Read(context.Background(), Query{Host: localHost(t, t.TempDir()), Sources: all(), Workspace: filepath.Join(project, "apps", "web")})
	if got := names(found.Project); !reflect.DeepEqual(got, []string{"web-build", "deploy"}) {
		t.Fatalf("got %v", got)
	}
}

func TestThePluginsEnabledByClaudeCodesSettingsMergedFromThePersonsToTheProjects(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	InstallPlugin(t, home, "jadis@jadis", map[string]string{
		".claude-plugin/plugin.json": `{"description":"Jadis apps.","name":"jadis","version":"1.6.0"}`,
		"commands/db/migrate.md":     "---\ndescription: Migrates.\n---\nMigrate.",
		"skills/jadis/SKILL.md":      skillFile("jadis", "Builds with Jadis.", "Use ${CLAUDE_PLUGIN_ROOT}."),
	}, "")
	InstallPlugin(t, home, "extra@market", map[string]string{
		".claude-plugin/plugin.json": `{"name":"extra","skills":"./more"}`,
		"more/lint/SKILL.md":         skillFile("lint", "Lints.", "Lint."),
	}, "")
	InstallPlugin(t, home, "theirs@market", map[string]string{"skills/x/SKILL.md": skillFile("x", "X.", "x")}, "/another/project")
	put(t, home, ".claude/settings.json", `{"enabledPlugins":{"extra@market":true,"jadis@jadis":true}}`)
	put(t, project, ".claude/settings.json", `{"enabledPlugins":{"extra@market":false}}`)
	host := localHost(t, home)
	found, _ := Read(context.Background(), Query{Host: host, Sources: all(), Workspace: project})
	type state struct {
		id          string
		claude, on  bool
		skillsFound []string
	}
	got := []state{}
	for _, plugin := range found.Plugins {
		got = append(got, state{plugin.ID, plugin.EnabledInClaude, plugin.On, names(plugin.Skills)})
	}
	want := []state{{"extra@market", false, false, []string{}}, {"jadis@jadis", true, true, []string{"jadis:jadis", "jadis:db:migrate"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if jadis := found.Plugins[1]; jadis.Description != "Jadis apps." || jadis.Name != "jadis" || jadis.Version != "1.6.0" {
		t.Fatalf("got %+v", jadis)
	}
	// Turned on here, whatever Claude Code says; and the page sees the skills of those that are off.
	sources := all()
	sources.Plugins = map[string]bool{"extra@market": true, "jadis@jadis": false}
	turned, _ := Read(context.Background(), Query{EveryPlugin: true, Host: host, Sources: sources, Workspace: project})
	if turned.Plugins[0].On != true || !reflect.DeepEqual(names(turned.Plugins[0].Skills), []string{"extra:lint"}) {
		t.Fatalf("got %+v", turned.Plugins[0])
	}
	if turned.Plugins[1].On || len(turned.Plugins[1].Skills) != 2 {
		t.Fatalf("got %+v", turned.Plugins[1])
	}
}

func TestAPluginIsFoundUnderThisClaudeFolderWhenRecordedUnderAnother(t *testing.T) {
	home := t.TempDir()
	InstallPlugin(t, home, "jadis@jadis", map[string]string{"skills/jadis/SKILL.md": skillFile("jadis", "Jadis.", "x")}, "")
	file := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	data, _ := os.ReadFile(file)
	_ = os.WriteFile(file, []byte(strings.ReplaceAll(string(data), home, "/home/someone-else")), 0o644)
	found, _ := Read(context.Background(), Query{Host: localHost(t, home), Sources: all()})
	if got := found.Plugins[0].Skills[0].Folder; got != filepath.Join(home, ".claude/plugins/cache/jadis/jadis/1.0.0/skills/jadis") {
		t.Fatalf("got %s", got)
	}
}

func TestNothingTurnedOffIsRead(t *testing.T) {
	home, project := personalHome(t), t.TempDir()
	put(t, project, ".claude/skills/deploy/SKILL.md", skillFile("deploy", "Deploys.", "Deploy."))
	sources := all()
	sources.Personal, sources.Project = false, false
	found, _ := Read(context.Background(), Query{Host: localHost(t, home), Sources: sources, Workspace: project})
	if len(found.Personal)+len(found.Project) != 0 {
		t.Fatalf("got %+v", found)
	}
}

func TestAnotherMachineIsReadThroughItsBash(t *testing.T) {
	home := personalHome(t)
	InstallPlugin(t, home, "jadis@jadis", map[string]string{"skills/jadis/SKILL.md": skillFile("jadis", "Jadis.", "x")}, "")
	remote := hosts.NewShellHost(hosts.LocalTransport("build-box", home))
	defer remote.Close()
	found, err := Read(context.Background(), Query{Host: remote, Sources: all()})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(found.Personal); !reflect.DeepEqual(got, []string{"pixel-art", "commit", "frontend:component"}) {
		t.Fatalf("got %v", got)
	}
	if got := names(found.Plugins[0].Skills); !reflect.DeepEqual(got, []string{"jadis:jadis"}) {
		t.Fatalf("got %v", got)
	}
}

func TestTheSourcesStoreStartsWithEverythingOn(t *testing.T) {
	store := NewSourcesStore(filepath.Join(t.TempDir(), "claude.json"))
	if got := store.Load(); !got.Personal || !got.Project || got.Plugins == nil {
		t.Fatalf("got %+v", got)
	}
	_ = store.Save(protocol.ClaudeSources{Personal: false, Plugins: map[string]bool{"a@b": false}, Project: true})
	if got := store.Load(); got.Personal || !got.Project || got.Plugins["a@b"] {
		t.Fatalf("got %+v", got)
	}
}
