// Package catalog is the skills of every session: swcode's own, kept by the daemon, then Claude Code's on the
// session's host (the person's, the project's, the plugins'), as the settings page turns them on.
package catalog

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"swcode/daemon/claude"
	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/skills"
)

const allFiles = "**/*"

var pathSeparators = regexp.MustCompile(`[\\/]`)

// Place is where a session's agent works, which decides the skills it has: Claude Code's are read there.
type Place struct {
	Host      hosts.Host
	Workspace string
	SessionID string
	// Local: its commands run on the daemon's computer, where swcode's own skills' folders are.
	Local bool
}

// Port is a session's skills, for the main agent's tools: swcode's own, and Claude Code's where the project is.
type Port interface {
	// List is what the agent is offered: those for people only aside.
	List(ctx context.Context) []protocol.SkillSummary
	// Load is a skill's instructions as the agent reads them, given the arguments it passed.
	Load(ctx context.Context, name, given string) skills.Text
	// ReadFile is a file of the skill's folder, by its path there: what the instructions name (references/api.md).
	ReadFile(ctx context.Context, name, path string) skills.Text
	Draft(skill protocol.Skill) skills.Draft
	// Save creates one of swcode's skills, or replaces the one of that name.
	Save(skill protocol.Skill) error
}

// SourcesPort is which of Claude Code's skills swcode reads, kept in the data folder.
type SourcesPort interface {
	Load() protocol.ClaudeSources
	Save(sources protocol.ClaudeSources) error
}

// entry is a skill a session's agent has: one of swcode's own, or one of Claude Code's on its host.
type entry struct {
	own     *protocol.SkillSummary
	claude  *claude.HostSkill
	offered bool
}

func (e entry) name() string {
	if e.own != nil {
		return e.own.Name
	}
	return e.claude.Name
}

func (e entry) summary() protocol.SkillSummary {
	if e.own != nil {
		return *e.own
	}
	return protocol.SkillSummary{Description: e.claude.Description, Name: e.claude.Name}
}

func offered(entries []entry) []protocol.SkillSummary {
	summaries := []protocol.SkillSummary{}
	for _, each := range entries {
		if each.offered {
			summaries = append(summaries, each.summary())
		}
	}
	return summaries
}

// Catalog is the skills of every session. Claude Code's are read where they are, once a turn, and loaded from there.
type Catalog struct {
	store    *skills.Store
	sources  SourcesPort
	logError func(error)
}

func New(store *skills.Store, sources SourcesPort, logError func(error)) *Catalog {
	return &Catalog{logError: logError, sources: sources, store: store}
}

// ClaudeOn is what swcode finds of Claude Code on a host, the skills of the plugins that are off too: for the
// settings page.
func (c *Catalog) ClaudeOn(ctx context.Context, host hosts.Host) (claude.Findings, error) {
	return claude.Read(ctx, claude.Query{EveryPlugin: true, Host: host, Sources: c.sources.Load()})
}

// PortFor is the skills of a turn's place: Claude Code's are read once, at the turn's start, and its loads find
// them there without going over an SSH connection again.
func (c *Catalog) PortFor(place Place) Port {
	return &port{catalog: c, place: place}
}

type port struct {
	catalog *Catalog
	place   Place
	once    sync.Once
	claude  []claude.HostSkill
}

func (p *port) claudeSkills(ctx context.Context) []claude.HostSkill {
	p.once.Do(func() { p.claude = p.catalog.claudeSkills(ctx, p.place) })
	return p.claude
}

// entries is each name once: the first of two skills of the same name wins, swcode's own before Claude Code's.
func (p *port) entries(ctx context.Context) []entry {
	entries := []entry{}
	seen := map[string]bool{}
	add := func(each entry) {
		if !seen[each.name()] {
			seen[each.name()] = true
			entries = append(entries, each)
		}
	}
	for _, own := range p.catalog.store.List() {
		add(entry{offered: true, own: &own})
	}
	for _, skill := range p.claudeSkills(ctx) {
		add(entry{claude: &skill, offered: !skill.ForPeople})
	}
	return entries
}

func (p *port) List(ctx context.Context) []protocol.SkillSummary {
	return offered(p.entries(ctx))
}

func (p *port) find(ctx context.Context, name string) (entry, skills.Text) {
	entries := p.entries(ctx)
	for _, each := range entries {
		if each.name() != name {
			continue
		}
		if !each.offered {
			return entry{}, skills.Text{Problem: skills.ForPeopleOnly(name)}
		}
		return each, skills.Text{OK: true}
	}
	return entry{}, skills.Text{Problem: skills.UnknownSkill(name, offered(entries))}
}

func (p *port) Load(ctx context.Context, name, given string) skills.Text {
	found, problem := p.find(ctx, name)
	if !problem.OK {
		return problem
	}
	if found.own != nil {
		return p.loadOwn(name, given)
	}
	return p.loadClaude(ctx, *found.claude, given)
}

func (p *port) values() skills.Values {
	return skills.Values{skills.VariableProjectDir: p.place.Workspace, skills.VariableSessionID: p.place.SessionID}
}

// loadOwn loads one of swcode's skills: its folder is named when the session's commands run on the daemon's
// computer.
func (p *port) loadOwn(name, given string) skills.Text {
	source, ok := p.catalog.store.Source(name)
	if !ok {
		return skills.Text{Problem: skills.UnknownSkill(name, nil)}
	}
	folder := ""
	if p.place.Local {
		folder = source.Folder
	}
	values := p.values()
	values[skills.VariableSkillDir] = folder
	instructions := skills.WithArguments(source.File.Instructions, given, source.File.ArgumentNames, values)
	return skills.Text{OK: true, Text: skills.LoadedText(folder, instructions, source.Files)}
}

// loadClaude loads one of Claude Code's, read on the session's host, where its folder and its scripts are.
func (p *port) loadClaude(ctx context.Context, skill claude.HostSkill, given string) skills.Text {
	host := p.place.Host
	data, err := host.ReadFile(ctx, skill.File)
	if err != nil {
		return skills.Text{Problem: skill.File + " could not be read."}
	}
	file := skills.Parse(string(data))
	values := p.values()
	values[skills.VariableSkillDir] = skill.Folder
	if skill.Plugin != nil {
		values[skills.VariablePluginData] = skill.Plugin.Data
		values[skills.VariablePluginRoot] = skill.Plugin.Root
	}
	instructions := skills.WithArguments(file.Instructions, given, file.ArgumentNames, values)
	files := []string{}
	if skill.Folder != "" {
		files = hostSkillFiles(ctx, host, skill.Folder)
	}
	return skills.Text{OK: true, Text: skills.LoadedText(skill.Folder, instructions, files)}
}

func (p *port) ReadFile(ctx context.Context, name, path string) skills.Text {
	found, problem := p.find(ctx, name)
	if !problem.OK {
		return problem
	}
	if found.own != nil {
		return p.catalog.store.ReadFile(name, path)
	}
	return hostSkillFile(ctx, p.place.Host, *found.claude, path)
}

func (p *port) Draft(skill protocol.Skill) skills.Draft { return p.catalog.store.Draft(skill) }

func (p *port) Save(skill protocol.Skill) error { return p.catalog.store.Save(skill) }

// claudeSkills is Claude Code's skills where the session works; none when that host cannot be read, which never
// stops a turn.
func (c *Catalog) claudeSkills(ctx context.Context, place Place) []claude.HostSkill {
	found, err := claude.Read(ctx, claude.Query{Host: place.Host, Sources: c.sources.Load(), Workspace: place.Workspace})
	if err != nil {
		c.logError(err)
		return nil
	}
	all := append(append([]claude.HostSkill{}, found.Personal...), found.Project...)
	for _, plugin := range found.Plugins {
		all = append(all, plugin.Skills...)
	}
	return all
}

// hostSkillFiles is the files of a skill's folder on its host, by their path there, written with slashes.
func hostSkillFiles(ctx context.Context, host hosts.Host, folder string) []string {
	listed, _ := host.ListFiles(ctx, folder, allFiles, skills.MaxFiles+1)
	files := []string{}
	for _, file := range listed {
		relative := strings.Join(pathSeparators.Split(host.Paths().Rel(folder, file), -1), "/")
		if relative != skills.SkillFile {
			files = append(files, relative)
		}
	}
	sort.Strings(files)
	return files[:min(len(files), skills.MaxFiles)]
}

func isInside(host hosts.Host, root, path string) bool {
	return strings.HasPrefix(path, root+host.Paths().Separator())
}

// hostSkillFile is a file of a skill's folder on its host, never one outside it, through a path or a link.
func hostSkillFile(ctx context.Context, host hosts.Host, skill claude.HostSkill, path string) skills.Text {
	if skill.Folder == "" {
		return skills.Text{Problem: skill.Name + " is a command: it has no folder of files."}
	}
	unreadable := skills.Text{Problem: path + " could not be read."}
	root, err := host.RealPath(ctx, skill.Folder)
	if err != nil {
		return unreadable
	}
	target := host.Paths().Resolve(root, path)
	outside := skills.Text{Problem: path + " is outside the skill's folder."}
	if !isInside(host, root, target) {
		return outside
	}
	if info, err := host.FileInfo(ctx, target); err != nil || info.Kind != hosts.KindFile {
		return skills.Text{Problem: fmt.Sprintf("The skill %s has no file %s.", skill.Name, path)}
	}
	if real, err := host.RealPath(ctx, target); err != nil || !isInside(host, root, real) {
		return outside
	}
	data, err := host.ReadFile(ctx, target)
	if err != nil {
		return unreadable
	}
	return skills.Text{OK: true, Text: skills.Clipped(string(data))}
}
