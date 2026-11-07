package catalog

import (
	"context"
	"strings"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/skills"
)

const (
	// agentsFolder and skillsFolder are where agents that share the convention look for skills: .agents/skills, in
	// the person's home and in a project.
	agentsFolder = ".agents"
	skillsFolder = "skills"
	gitFolder    = ".git"
)

// HostSkill is a skill found on the project's host, and read from there when loaded.
type HostSkill struct {
	Name        string
	Description string
	Origin      protocol.SkillOrigin
	// File is its SKILL.md, in Folder with the files its instructions name.
	File   string
	Folder string
}

// shelf is the .agents/skills folders of a host of one origin: the person's, or the project's.
type shelf struct {
	ctx    context.Context
	host   hosts.Host
	origin protocol.SkillOrigin
}

func (s shelf) readText(path string) (string, bool) {
	data, err := s.host.ReadFile(s.ctx, path)
	return string(data), err == nil
}

// skillAt is a skill's folder, named by its frontmatter or its folder; nothing when it holds no SKILL.md.
func (s shelf) skillAt(folder, folderName string) (HostSkill, bool) {
	file := s.host.Paths().Join(folder, skills.SkillFile)
	text, ok := s.readText(file)
	if !ok {
		return HostSkill{}, false
	}
	parsed := skills.Parse(text)
	name := strings.TrimSpace(parsed.Name)
	if name == "" {
		name = folderName
	}
	return HostSkill{Description: parsed.Description, File: file, Folder: folder, Name: name, Origin: s.origin}, true
}

// skillsIn is the skills of a .agents/skills folder: its subfolders that hold a SKILL.md.
func (s shelf) skillsIn(root string) []HostSkill {
	listing, err := s.host.ListFolders(s.ctx, root)
	if err != nil || !listing.OK {
		return nil
	}
	found := []HostSkill{}
	for _, name := range listing.Folders {
		if skill, ok := s.skillAt(s.host.Paths().Join(root, name), name); ok {
			found = append(found, skill)
		}
	}
	return found
}

func (s shelf) skillsUnder(folder string) []HostSkill {
	return s.skillsIn(s.host.Paths().Join(folder, agentsFolder, skillsFolder))
}

// ancestors is a folder, then its parents up to the root.
func ancestors(paths hosts.Paths, folder string) []string {
	chain := []string{folder}
	for parent := paths.Dir(folder); parent != chain[len(chain)-1]; parent = paths.Dir(parent) {
		chain = append(chain, parent)
	}
	return chain
}

func exists(ctx context.Context, host hosts.Host, path string) bool {
	info, err := host.FileInfo(ctx, path)
	return err == nil && info.Kind != hosts.KindMissing
}

// ProjectFolders is the project's folder, then its parents up to the repository's root (its .git, a folder or a
// worktree's file); outside a repository, the project's folder alone.
func ProjectFolders(ctx context.Context, host hosts.Host, workspace string) []string {
	chain := ancestors(host.Paths(), workspace)
	for index, folder := range chain {
		if exists(ctx, host, host.Paths().Join(folder, gitFolder)) {
			return chain[:index+1]
		}
	}
	return []string{workspace}
}

// Find is the skills on a host, read where they are: the person's in ~/.agents/skills, then the project's in the
// .agents/skills of its folder and of its parents up to the repository's root. Nothing is copied.
func Find(ctx context.Context, host hosts.Host, workspace string) ([]HostSkill, error) {
	home, err := host.Home(ctx)
	if err != nil {
		return nil, err
	}
	all := shelf{ctx: ctx, host: host, origin: protocol.SkillOriginPersonal}.skillsUnder(home)
	if workspace == "" {
		return all, ctx.Err()
	}
	project := shelf{ctx: ctx, host: host, origin: protocol.SkillOriginProject}
	for _, folder := range ProjectFolders(ctx, host, workspace) {
		all = append(all, project.skillsUnder(folder)...)
	}
	return all, ctx.Err()
}
