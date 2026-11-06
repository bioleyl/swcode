// Package catalog is the skills of every session: those of the .agents/skills folders on the session's host, the
// person's and the project's, read once a turn and loaded from there.
package catalog

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/skills"
)

const allFiles = "**/*"

var pathSeparators = regexp.MustCompile(`[\\/]`)

// Place is where a session's agent works, which decides the skills it has: they are read there.
type Place struct {
	Host      hosts.Host
	Workspace string
}

// Port is a session's skills, for the main agent's tools.
type Port interface {
	// List is what the agent is offered.
	List(ctx context.Context) []protocol.SkillSummary
	// Load is a skill's instructions as the agent reads them.
	Load(ctx context.Context, name string) skills.Text
	// ReadFile is a file of the skill's folder, by its path there: what the instructions name (references/api.md).
	ReadFile(ctx context.Context, name, path string) skills.Text
}

// Catalog is the skills of every session.
type Catalog struct {
	logError func(error)
}

func New(logError func(error)) *Catalog {
	return &Catalog{logError: logError}
}

// PortFor is the skills of a turn's place: they are read once, at the turn's start, and its loads find them there
// without going over an SSH connection again.
func (c *Catalog) PortFor(place Place) Port {
	return &port{catalog: c, place: place}
}

type port struct {
	catalog *Catalog
	place   Place
	once    sync.Once
	found   []HostSkill
}

// skills is each name once: the first of two skills of the same name wins, the person's before the project's.
func (p *port) skills(ctx context.Context) []HostSkill {
	p.once.Do(func() { p.found = distinct(p.catalog.hostSkills(ctx, p.place)) })
	return p.found
}

func distinct(found []HostSkill) []HostSkill {
	kept := []HostSkill{}
	seen := map[string]bool{}
	for _, skill := range found {
		if !seen[skill.Name] {
			seen[skill.Name] = true
			kept = append(kept, skill)
		}
	}
	return kept
}

func summaries(found []HostSkill) []protocol.SkillSummary {
	listed := []protocol.SkillSummary{}
	for _, skill := range found {
		listed = append(listed, protocol.SkillSummary{Description: skill.Description, Name: skill.Name})
	}
	return listed
}

func (p *port) List(ctx context.Context) []protocol.SkillSummary {
	return summaries(p.skills(ctx))
}

func (p *port) find(ctx context.Context, name string) (HostSkill, skills.Text) {
	found := p.skills(ctx)
	for _, skill := range found {
		if skill.Name == name {
			return skill, skills.Text{OK: true}
		}
	}
	return HostSkill{}, skills.Text{Problem: skills.UnknownSkill(name, summaries(found))}
}

// Load reads a skill on the session's host, where its folder and its scripts are.
func (p *port) Load(ctx context.Context, name string) skills.Text {
	skill, problem := p.find(ctx, name)
	if !problem.OK {
		return problem
	}
	host := p.place.Host
	data, err := host.ReadFile(ctx, skill.File)
	if err != nil {
		return skills.Text{Problem: skill.File + " could not be read."}
	}
	instructions := skills.Parse(string(data)).Instructions
	return skills.Text{OK: true, Text: skills.LoadedText(skill.Folder, instructions, hostSkillFiles(ctx, host, skill.Folder))}
}

func (p *port) ReadFile(ctx context.Context, name, path string) skills.Text {
	skill, problem := p.find(ctx, name)
	if !problem.OK {
		return problem
	}
	return hostSkillFile(ctx, p.place.Host, skill, path)
}

// hostSkills is the skills where the session works; none when that host cannot be read, which never stops a turn.
func (c *Catalog) hostSkills(ctx context.Context, place Place) []HostSkill {
	found, err := Find(ctx, place.Host, place.Workspace)
	if err != nil {
		c.logError(err)
		return nil
	}
	return found
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
func hostSkillFile(ctx context.Context, host hosts.Host, skill HostSkill, path string) skills.Text {
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
