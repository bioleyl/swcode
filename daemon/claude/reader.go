package claude

import (
	"context"
	"regexp"
	"strings"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/skills"
)

const (
	claudeFolder   = ".claude"
	skillsFolder   = "skills"
	commandsFolder = "commands"
	pluginsFolder  = "plugins"
	pluginCache    = "cache"
	pluginData     = "data"
	installedFile  = "installed_plugins.json"
	settingsFile   = "settings.json"
	localSettings  = "settings.local.json"
	manifestFolder = ".claude-plugin"
	manifestFile   = "plugin.json"
	// syncedFolder holds claude.ai's own skills, which Claude Code keeps in the person's skills folder: they need
	// claude.ai's tools.
	syncedFolder     = "synced"
	gitFolder        = ".git"
	commandExtension = ".md"
	commandFiles     = "**/*" + commandExtension
	maxCommands      = 200
	nameSeparator    = ":"
	// A plugin's data folder is named after its id, anything but letters, digits, _ and - made a -.
	dataNameFiller = "-"
)

var (
	pathSeparators = regexp.MustCompile(`[\\/]`)
	notInDataName  = regexp.MustCompile(`[^A-Za-z0-9_-]`)
)

// Origin is where a skill comes from: swcode's own folder, Claude Code's for the person, the project's, or a
// plugin. Two of the same name, the first in this order wins; a plugin's are named after it, so they never meet the
// others.
type Origin string

const (
	OriginSwcode  Origin = "swcode"
	OriginClaude  Origin = "claude"
	OriginProject Origin = "project"
	OriginPlugin  Origin = "plugin"
)

// Plugin is a plugin's folder and its data folder, for ${CLAUDE_PLUGIN_ROOT} and ${CLAUDE_PLUGIN_DATA}.
type Plugin struct {
	Root string
	Data string
}

// HostSkill is a skill or a command of Claude Code, found on the project's host, and read from there when loaded.
type HostSkill struct {
	// Name is as the agent calls it: deploy, frontend:component, jadis:jadis.
	Name string
	// Description is its line in the list: the description, or its first line, then when_to_use.
	Description string
	// ForPeople is disable-model-invocation: for a person to type in Claude Code; the agent is not offered it.
	ForPeople bool
	Origin    Origin
	// File is its SKILL.md, or the command's file.
	File string
	// Folder is a skill's folder, with the files its instructions name; empty for a command.
	Folder string
	Plugin *Plugin
}

// FoundPlugin is a plugin as swcode found it: whether Claude Code has it enabled, whether swcode reads it, and its
// skills.
type FoundPlugin struct {
	ID              string
	Name            string
	Description     string
	Version         string
	EnabledInClaude bool
	On              bool
	// Skills are its skills and commands; those of a plugin that is off only when asked for (the settings page).
	Skills []HostSkill
}

// Findings is what swcode finds of Claude Code on a host, for a project there or for none (the settings page).
type Findings struct {
	// Folder is ~/.claude there.
	Folder   string
	Personal []HostSkill
	Project  []HostSkill
	Plugins  []FoundPlugin
}

// Query is what to read of Claude Code on a host.
type Query struct {
	Host hosts.Host
	// Workspace is the project's folder there, for its own skills and plugins; empty for the settings page.
	Workspace string
	Sources   protocol.ClaudeSources
	// EveryPlugin reads the skills of the plugins that are off too, to show what turning one on would bring.
	EveryPlugin bool
}

func readText(ctx context.Context, host hosts.Host, path string) (string, bool) {
	data, err := host.ReadFile(ctx, path)
	return string(data), err == nil
}

func kindOf(ctx context.Context, host hosts.Host, path string) hosts.FileKind {
	info, err := host.FileInfo(ctx, path)
	if err != nil {
		return hosts.KindMissing
	}
	return info.Kind
}

// shelf is where a skill or a command is found, and what its name starts with: a plugin's name, or nothing.
type shelf struct {
	ctx    context.Context
	host   hosts.Host
	origin Origin
	prefix string
	plugin *Plugin
}

// skillAt is a skill's folder, named by its frontmatter or its folder; nothing when it holds no SKILL.md.
func (s shelf) skillAt(folder, folderName string) (HostSkill, bool) {
	file := s.host.Paths().Join(folder, skills.SkillFile)
	text, ok := readText(s.ctx, s.host, file)
	if !ok {
		return HostSkill{}, false
	}
	parsed := skills.Parse(text)
	name := strings.TrimSpace(parsed.Name)
	if name == "" {
		name = folderName
	}
	return HostSkill{
		Description: skills.ListedDescription(parsed), File: file, Folder: folder, ForPeople: parsed.ForPeople,
		Name: s.prefix + name, Origin: s.origin, Plugin: s.plugin,
	}, true
}

// skillsIn is the skills of a folder: itself when it holds a SKILL.md, else its subfolders that do.
func (s shelf) skillsIn(root string) []HostSkill {
	if itself, ok := s.skillAt(root, s.host.Paths().Base(root)); ok {
		return []HostSkill{itself}
	}
	listing, err := s.host.ListFolders(s.ctx, root)
	if err != nil || !listing.OK {
		return nil
	}
	found := []HostSkill{}
	for _, name := range listing.Folders {
		if strings.ToLower(name) == syncedFolder {
			continue
		}
		if skill, ok := s.skillAt(s.host.Paths().Join(root, name), name); ok {
			found = append(found, skill)
		}
	}
	return found
}

// commandAt is a command's file: its name is its path in the commands folder, the subfolders before it
// (frontend:component).
func (s shelf) commandAt(root, file string) (HostSkill, bool) {
	text, ok := readText(s.ctx, s.host, file)
	if !ok {
		return HostSkill{}, false
	}
	parsed := skills.Parse(text)
	relative := s.host.Paths().Rel(root, file)
	if relative == "" {
		relative = s.host.Paths().Base(file)
	}
	name := strings.Join(pathSeparators.Split(strings.TrimSuffix(relative, commandExtension), -1), nameSeparator)
	return HostSkill{
		Description: skills.ListedDescription(parsed), File: file, ForPeople: parsed.ForPeople,
		Name: s.prefix + name, Origin: s.origin, Plugin: s.plugin,
	}, true
}

// commandsIn is the commands of a folder (commands/*.md, Claude Code's older skills), or the one a path names.
func (s shelf) commandsIn(path string) []HostSkill {
	if strings.HasSuffix(path, commandExtension) {
		if command, ok := s.commandAt(s.host.Paths().Dir(path), path); ok {
			return []HostSkill{command}
		}
		return nil
	}
	files, err := s.host.ListFiles(s.ctx, path, commandFiles, maxCommands)
	if err != nil {
		return nil
	}
	found := []HostSkill{}
	for _, file := range files {
		if command, ok := s.commandAt(path, file); ok {
			found = append(found, command)
		}
	}
	return found
}

// claudeFolderSkills is a .claude folder's skills, then its commands: a skill wins over a command of the same name.
func (s shelf) claudeFolderSkills(folder string) []HostSkill {
	join := s.host.Paths().Join
	return append(s.skillsIn(join(folder, skillsFolder)), s.commandsIn(join(folder, commandsFolder))...)
}

// ancestors is a folder, then its parents up to the root.
func ancestors(paths hosts.Paths, folder string) []string {
	chain := []string{folder}
	for parent := paths.Dir(folder); parent != chain[len(chain)-1]; parent = paths.Dir(parent) {
		chain = append(chain, parent)
	}
	return chain
}

// projectFolders is the project's folder, then its parents up to the repository's root (its .git, a folder or a
// worktree's file), as Claude Code reads their .claude; outside a repository, the project's folder alone.
func projectFolders(ctx context.Context, host hosts.Host, workspace string) []string {
	chain := ancestors(host.Paths(), workspace)
	for index, folder := range chain {
		if kindOf(ctx, host, host.Paths().Join(folder, gitFolder)) != hosts.KindMissing {
			return chain[:index+1]
		}
	}
	return []string{workspace}
}

func projectSkills(ctx context.Context, host hosts.Host, workspace string) []HostSkill {
	project := shelf{ctx: ctx, host: host, origin: OriginProject}
	found := []HostSkill{}
	for _, folder := range projectFolders(ctx, host, workspace) {
		found = append(found, project.claudeFolderSkills(host.Paths().Join(folder, claudeFolder))...)
	}
	return found
}

// pluginRoot is the folder of a plugin's version: where Claude Code installed it, or the same place under this
// ~/.claude when that path is another machine's (a folder copied, CLAUDE_CONFIG_DIR).
func pluginRoot(ctx context.Context, host hosts.Host, folder, installPath string) string {
	if kindOf(ctx, host, installPath) == hosts.KindDirectory {
		return installPath
	}
	segments := pathSeparators.Split(installPath, -1)
	for index := 1; index < len(segments); index++ {
		if segments[index] == pluginCache && segments[index-1] == pluginsFolder {
			return host.Paths().Join(append([]string{folder, pluginsFolder}, segments[index:]...)...)
		}
	}
	return installPath
}

// pluginOwnSkills is a plugin's skills: those of skills/ and of the folders its manifest names, together; without
// either, a SKILL.md at its root makes it one skill, named after the plugin.
func (s shelf) pluginOwnSkills(root, name string, manifest PluginManifest) []HostSkill {
	join := s.host.Paths().Join
	named, hasNamed := ManifestPaths(manifest.Skills)
	if !hasNamed && kindOf(s.ctx, s.host, join(root, skillsFolder)) != hosts.KindDirectory {
		if skill, ok := s.skillAt(root, name); ok {
			return []HostSkill{skill}
		}
		return nil
	}
	found := []HostSkill{}
	seen := map[string]bool{}
	for _, path := range append([]string{skillsFolder}, named...) {
		if !seen[path] {
			seen[path] = true
			found = append(found, s.skillsIn(join(root, path))...)
		}
	}
	return found
}

// pluginSkills is a plugin's skills, then its commands: those the manifest names, which replace commands/.
func (s shelf) pluginSkills(root, name string, manifest PluginManifest) []HostSkill {
	found := s.pluginOwnSkills(root, name, manifest)
	commandRoots, named := ManifestPaths(manifest.Commands)
	if !named {
		commandRoots = []string{commandsFolder}
	}
	for _, path := range commandRoots {
		found = append(found, s.commandsIn(s.host.Paths().Join(root, path))...)
	}
	return found
}

func foundPlugin(ctx context.Context, query Query, folder string, plugin InstalledPlugin, enabled map[string]bool) FoundPlugin {
	host := query.Host
	join := host.Paths().Join
	root := pluginRoot(ctx, host, folder, plugin.InstallPath)
	text, _ := readText(ctx, host, join(root, manifestFolder, manifestFile))
	manifest := ParseManifest(text)
	// A plugin no settings file names is on, unless its manifest says otherwise.
	enabledInClaude, named := enabled[plugin.ID]
	if !named {
		enabledInClaude = manifest.DefaultEnabled == nil || *manifest.DefaultEnabled
	}
	// swcode reads a plugin as the person set it here, else as Claude Code has it.
	on, set := query.Sources.Plugins[plugin.ID]
	if !set {
		on = enabledInClaude
	}
	name := manifest.Name
	if name == "" {
		name = PluginNameOf(plugin.ID)
	}
	data := join(folder, pluginsFolder, pluginData, notInDataName.ReplaceAllString(plugin.ID, dataNameFiller))
	pluginShelf := shelf{ctx: ctx, host: host, origin: OriginPlugin, plugin: &Plugin{Data: data, Root: root}, prefix: name + nameSeparator}
	found := FoundPlugin{
		Description: manifest.Description, EnabledInClaude: enabledInClaude, ID: plugin.ID, Name: name, On: on,
		Skills: []HostSkill{}, Version: firstOf(manifest.Version, plugin.Version),
	}
	if on || query.EveryPlugin {
		found.Skills = pluginShelf.pluginSkills(root, name, manifest)
	}
	return found
}

func firstOf(texts ...string) string {
	for _, text := range texts {
		if text != "" {
			return text
		}
	}
	return ""
}

// plugins is the plugins Claude Code installed there: each enabled or not by its settings files merged key by key
// (the person's, then the project's, then the project's local one), then turned on or off here.
func plugins(ctx context.Context, query Query, folder string) []FoundPlugin {
	join := query.Host.Paths().Join
	installed, _ := readText(ctx, query.Host, join(folder, pluginsFolder, installedFile))
	user, _ := readText(ctx, query.Host, join(folder, settingsFile))
	enabled := EnabledPlugins(user)
	if query.Workspace != "" {
		for _, name := range []string{settingsFile, localSettings} {
			text, _ := readText(ctx, query.Host, join(query.Workspace, claudeFolder, name))
			for id, on := range EnabledPlugins(text) {
				enabled[id] = on
			}
		}
	}
	found := []FoundPlugin{}
	for _, plugin := range InstalledPlugins(installed, query.Workspace) {
		found = append(found, foundPlugin(ctx, query, folder, plugin, enabled))
	}
	return found
}

// Read is what swcode reads of Claude Code on a host, as Claude Code run there would find it: the person's skills
// and commands in ~/.claude, the project's in its .claude folders, and the plugins. Nothing is copied: it is read
// where it is, at each turn.
func Read(ctx context.Context, query Query) (Findings, error) {
	home, err := query.Host.Home(ctx)
	if err != nil {
		return Findings{}, err
	}
	folder := query.Host.Paths().Join(home, claudeFolder)
	findings := Findings{Folder: folder, Personal: []HostSkill{}, Project: []HostSkill{}}
	if query.Sources.Personal {
		findings.Personal = shelf{ctx: ctx, host: query.Host, origin: OriginClaude}.claudeFolderSkills(folder)
	}
	if query.Sources.Project && query.Workspace != "" {
		findings.Project = projectSkills(ctx, query.Host, query.Workspace)
	}
	findings.Plugins = plugins(ctx, query, folder)
	return findings, ctx.Err()
}
