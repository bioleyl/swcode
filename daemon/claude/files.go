// Package claude reads the skills and plugins installed for Claude Code, on the host a project is on, as Claude
// Code run there would find them.
package claude

import (
	"encoding/json"
	"sort"
	"strings"
)

const (
	userScope   = "user"
	idSeparator = "@"
)

// InstalledPlugin is a plugin Claude Code installed, as a project sees it: by name@marketplace, and the folder of
// its version.
type InstalledPlugin struct {
	ID          string
	InstallPath string
	Version     string
}

// Claude Code's files, read leniently: a key swcode does not know is left alone, a file it cannot read is empty.
type install struct {
	InstallPath string `json:"installPath"`
	ProjectPath string `json:"projectPath"`
	Scope       string `json:"scope"`
	Version     string `json:"version"`
}

// installs is one install or a list of them, as the versions of installed_plugins.json write them.
type installs []install

func (i *installs) UnmarshalJSON(data []byte) error {
	var list []install
	if json.Unmarshal(data, &list) == nil {
		*i = list
		return nil
	}
	var one install
	if err := json.Unmarshal(data, &one); err != nil {
		return err
	}
	*i = installs{one}
	return nil
}

// PluginManifest is a plugin's .claude-plugin/plugin.json: its name, and where its skills and commands are when
// not the default.
type PluginManifest struct {
	Commands       json.RawMessage `json:"commands"`
	DefaultEnabled *bool           `json:"defaultEnabled"`
	Description    string          `json:"description"`
	Name           string          `json:"name"`
	Skills         json.RawMessage `json:"skills"`
	Version        string          `json:"version"`
}

// PluginNameOf is the plugin's name in its id: jadis of jadis@jadis.
func PluginNameOf(id string) string {
	name, _, _ := strings.Cut(id, idSeparator)
	return name
}

// InstalledPlugins is the plugins Claude Code installed that a project sees: those installed for the person, and
// those installed for that project's folder.
func InstalledPlugins(text string, workspace string) []InstalledPlugin {
	var file struct {
		Plugins map[string]installs `json:"plugins"`
	}
	if json.Unmarshal([]byte(text), &file) != nil {
		return []InstalledPlugin{}
	}
	plugins := []InstalledPlugin{}
	for _, id := range sortedKeys(file.Plugins) {
		for _, each := range file.Plugins[id] {
			scope := each.Scope
			if scope == "" {
				scope = userScope
			}
			if each.InstallPath != "" && (scope == userScope || (workspace != "" && each.ProjectPath == workspace)) {
				plugins = append(plugins, InstalledPlugin{ID: id, InstallPath: each.InstallPath, Version: each.Version})
				break
			}
		}
	}
	return plugins
}

// EnabledPlugins is what a settings file says of each plugin: on, off, or nothing for those it does not name.
func EnabledPlugins(text string) map[string]bool {
	var file struct {
		EnabledPlugins map[string]any `json:"enabledPlugins"`
	}
	enabled := map[string]bool{}
	if json.Unmarshal([]byte(text), &file) != nil {
		return enabled
	}
	for id, value := range file.EnabledPlugins {
		if on, ok := value.(bool); ok {
			enabled[id] = on
		}
	}
	return enabled
}

func ParseManifest(text string) PluginManifest {
	var manifest PluginManifest
	_ = json.Unmarshal([]byte(text), &manifest)
	return manifest
}

// ManifestPaths is a manifest's paths for skills or commands: one, or several; none for an object of commands,
// which swcode does not read.
func ManifestPaths(value json.RawMessage) ([]string, bool) {
	var one string
	if json.Unmarshal(value, &one) == nil {
		return []string{one}, true
	}
	var several []string
	if json.Unmarshal(value, &several) == nil {
		return several, true
	}
	return nil, false
}

func sortedKeys[Value any](items map[string]Value) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
