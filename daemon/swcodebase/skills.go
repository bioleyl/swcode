// Package swcodebase is the swcode-base skills the program carries: the ones a person gets with swcode, offered
// when their machine has none, written in their home or in a project's .agents/skills when they ask for them.
package swcodebase

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"swcode"
	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/skills"
)

const (
	// agentsFolder and skillsFolder are where the agent looks for skills, in a home or in a project.
	agentsFolder = ".agents"
	skillsFolder = "skills"
	// homeUnread is why nothing is known of a home: its host did not say where it is.
	homeUnread = "The home folder could not be read: %v."
	// noProject is why a project's skills cannot be written: the app named no project it knows.
	noProject = "There is no such project."
	// carriesNone is why nothing can be installed: this build embedded no swcode-base skills.
	carriesNone = "This swcode carries no swcode-base skills."
)

// Pack is the swcode-base skills, as the build embedded them.
type Pack struct {
	files fs.FS
}

// Carried is the skills the program holds; none when the build embedded none.
func Carried() Pack { return Pack{files: swcode.SwcodeBaseSkills()} }

// skillName is a skill's name in the pack: its folder's, the pack already being the skills folder.

func (p Pack) offered() bool { return p.files != nil }

// skillFolders is what the pack holds: the folders of the pack that hold a SKILL.md; fs.ReadDir lists them sorted.
func (p Pack) skillFolders() []string {
	if p.files == nil {
		return nil
	}
	entries, err := fs.ReadDir(p.files, ".")
	if err != nil {
		return nil
	}
	folders := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := fs.Stat(p.files, path.Join(entry.Name(), skills.SkillFile)); err == nil {
			folders = append(folders, entry.Name())
		}
	}
	return folders
}

// folderOf is where a skill of this name would be under a folder's .agents/skills.
func folderOf(paths hosts.Paths, folder, name string) string {
	return paths.Join(folder, agentsFolder, skillsFolder, name)
}

// shelf is a .agents/skills folder on a host: the person's home, or a project's folder.
type shelf struct {
	ctx    context.Context
	host   hosts.Host
	folder string
}

// skillFolder is where a skill of this name would be in this shelf.
func (s shelf) skillFolder(name string) string {
	return folderOf(s.host.Paths(), s.folder, name)
}

// present is whether each named skill is in this shelf: its SKILL.md is there, as the host reads it.
func (s shelf) present(names []string) map[string]bool {
	found := map[string]bool{}
	for _, name := range names {
		found[name] = s.holds(name)
	}
	return found
}

// holds is whether one skill is in this shelf: the folder of the pack's name for it holds a SKILL.md. Only that
// folder counts, so a skill of the person's, in a folder of its own, is never taken for the pack's.
func (s shelf) holds(name string) bool {
	info, err := s.host.FileInfo(s.ctx, s.host.Paths().Join(s.skillFolder(name), skills.SkillFile))
	return err == nil && info.Kind == hosts.KindFile
}

// remove deletes one skill's folder from this shelf, when it holds that skill.
func (s shelf) remove(name string) error {
	if !s.holds(name) {
		return nil
	}
	return s.host.RemoveFolder(s.ctx, s.skillFolder(name))
}

// write puts the pack's skills in this shelf, each file at a time: the first failure says why.
func (s shelf) write(p Pack, folders []string) string {
	for _, name := range folders {
		target := s.skillFolder(name)
		err := fs.WalkDir(p.files, name, func(file string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			data, err := fs.ReadFile(p.files, file)
			if err != nil {
				return err
			}
			relative, cut := strings.CutPrefix(file, name+"/")
			if !cut {
				return fmt.Errorf("%s is not in %s.", file, name)
			}
			return s.host.WriteFile(s.ctx, s.host.Paths().Resolve(target, strings.ReplaceAll(relative, "/", s.host.Paths().Separator())), data)
		})
		if err != nil {
			return fmt.Sprintf("%s could not be written: %v.", name, err)
		}
	}
	return ""
}

// Service knows where the swcode-base skills are, and puts them where a person asks: a home, or a project. Both are
// on the host the project is on, where its sessions read their skills.
type Service struct {
	pack  Pack
	hosts *hosts.Registry
}

func NewService(registry *hosts.Registry) *Service { return &Service{hosts: registry, pack: Carried()} }

// hostOf is the host the project is on; this computer when there is no project.
func (s *Service) hostOf(project *protocol.Project) hosts.Host {
	if project == nil {
		return s.hosts.HostOf(protocol.LocalHost())
	}
	return s.hosts.HostOf(project.Host)
}

// Status answers what GET /api/swcode-base says: whether this swcode carries these skills, and which of them are
// already in the home or in the project, both read on the project's host (this computer's home without a project),
// so the app offers them when neither has any.
func (s *Service) Status(ctx context.Context, project *protocol.Project) protocol.SwcodeBaseStatus {
	folders := s.pack.skillFolders()
	host := s.hostOf(project)
	home, err := host.Home(ctx)
	if err != nil {
		return s.nowhere(fmt.Sprintf(homeUnread, err))
	}
	inHome := shelf{ctx: ctx, host: host, folder: home}.present(folders)
	inProject := map[string]bool{}
	if project != nil {
		inProject = shelf{ctx: ctx, host: host, folder: project.Path}.present(folders)
	}
	return protocol.SwcodeBaseStatus{
		Carried: true, Installed: everyPlaced(folders, inHome, inProject), InHome: anyPlaced(inHome),
		InProject: anyPlaced(inProject), Names: folders,
	}
}

func anyPlaced(present map[string]bool) bool {
	for _, there := range present {
		if there {
			return true
		}
	}
	return false
}

// everyPlaced is whether the pack is where it is said to be: each of its skills in one place or the other.
func everyPlaced(folders []string, inHome, inProject map[string]bool) bool {
	for _, name := range folders {
		if !inHome[name] && !inProject[name] {
			return false
		}
	}
	return true
}

// Install writes the pack's skills in the home or in the project's .agents/skills, on the project's host (this
// computer's home without a project); then what is there.
func (s *Service) Install(ctx context.Context, project *protocol.Project, inHome bool) protocol.SwcodeBaseStatus {
	if problem := s.canWrite(project, inHome); problem != "" {
		return s.nowhere(problem)
	}
	target, problem := s.shelfFor(ctx, project, inHome)
	if problem != "" {
		return s.nowhere(problem)
	}
	if problem := target.write(s.pack, s.pack.skillFolders()); problem != "" {
		return s.nowhere(problem)
	}
	return s.Status(ctx, project)
}

// Remove deletes the pack's skills from the home or from the project, on the project's host; what the other place
// holds stays, and so does a skill of the person's in a folder of another name. Then what is there.
func (s *Service) Remove(ctx context.Context, project *protocol.Project, inHome bool) protocol.SwcodeBaseStatus {
	if problem := s.canWrite(project, inHome); problem != "" {
		return s.nowhere(problem)
	}
	target, problem := s.shelfFor(ctx, project, inHome)
	if problem != "" {
		return s.nowhere(problem)
	}
	for _, name := range s.pack.skillFolders() {
		if err := target.remove(name); err != nil {
			return s.nowhere(fmt.Sprintf("%s could not be removed: %v.", name, err))
		}
	}
	return s.Status(ctx, project)
}

// shelfFor is the .agents/skills folder the request means, on the project's host: its home's, or the project's.
func (s *Service) shelfFor(ctx context.Context, project *protocol.Project, inHome bool) (shelf, string) {
	host := s.hostOf(project)
	if !inHome {
		return shelf{ctx: ctx, folder: project.Path, host: host}, ""
	}
	home, err := host.Home(ctx)
	if err != nil {
		return shelf{}, fmt.Sprintf(homeUnread, err)
	}
	return shelf{ctx: ctx, folder: home, host: host}, ""
}

// nowhere is the answer of a request that could not be answered: what the pack holds, and why.
func (s *Service) nowhere(problem string) protocol.SwcodeBaseStatus {
	return protocol.SwcodeBaseStatus{Carried: s.pack.offered(), Names: s.pack.skillFolders(), Problem: problem}
}

// canWrite is why these skills cannot be written or removed here: this swcode carries none, or there is no such
// project.
func (s *Service) canWrite(project *protocol.Project, inHome bool) string {
	if !s.pack.offered() {
		return carriesNone
	}
	if !inHome && project == nil {
		return noProject
	}
	return ""
}
