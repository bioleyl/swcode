package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"swcode/daemon/protocol"
)

// MaxFiles: a skill's folder may hold many files, and the agent is told of the first ones.
const MaxFiles = 50

// Draft is what saving a skill would change in its file, for the person to see before it is saved.
type Draft struct {
	// Label is the file, as the conversation names it: skills/<name>/SKILL.md.
	Label string
	// Before is empty for a new skill.
	Before string
	After  string
}

// Source is one of swcode's own skills as the agent loads it: its file as read, its folder, and the other files.
type Source struct {
	File   File
	Folder string
	Files  []string
}

// Text is a file of a skill's folder, or why it cannot be read.
type Text struct {
	OK      bool
	Text    string
	Problem string
}

func changed(name string) protocol.SkillChange {
	return protocol.SkillChange{Value: protocol.SkillChangeOk{Name: name}}
}

func refused(problem string) protocol.SkillChange {
	return protocol.SkillChange{Value: protocol.SkillChangeFailed{Problem: problem}}
}

func taken(name string) protocol.SkillChange {
	return refused(fmt.Sprintf("There is already a skill named %s.", name))
}

func isInside(root, path string) bool {
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

// Store is the skills, a folder each in the data folder's skills/: its SKILL.md (a name and a description in the
// frontmatter, then the instructions), and the files the instructions name, as Agent Skills have them.
type Store struct {
	Folder string
}

func NewStore(folder string) *Store { return &Store{Folder: folder} }

// Seed starts a new skills folder with these files, by path inside it; one that exists stays as the person left
// it.
func (s *Store) Seed(files fs.FS) error {
	if _, err := os.Stat(s.Folder); err == nil {
		return nil
	}
	if err := os.MkdirAll(s.Folder, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(files, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := fs.ReadFile(files, path)
		if err != nil {
			return err
		}
		target := filepath.Join(s.Folder, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func (s *Store) List() []protocol.SkillSummary {
	entries, err := os.ReadDir(s.Folder)
	if err != nil {
		return []protocol.SkillSummary{}
	}
	summaries := []protocol.SkillSummary{}
	for _, entry := range entries {
		if entry.IsDir() && s.exists(entry.Name()) {
			summaries = append(summaries, protocol.SkillSummary{Description: s.fileOf(entry.Name()).Description, Name: entry.Name()})
		}
	}
	sort.Slice(summaries, func(first, second int) bool { return summaries[first].Name < summaries[second].Name })
	return summaries
}

func (s *Store) Read(name string) (protocol.SkillView, bool) {
	if !s.exists(name) {
		return protocol.SkillView{}, false
	}
	file := s.fileOf(name)
	return protocol.SkillView{Description: file.Description, Files: s.filesOf(name), Instructions: file.Instructions, Name: name}, true
}

// Source is a skill as the agent loads it.
func (s *Store) Source(name string) (Source, bool) {
	if !s.exists(name) {
		return Source{}, false
	}
	return Source{File: s.fileOf(name), Files: s.filesOf(name), Folder: s.folderOf(name)}, true
}

// ReadFile is a file of a skill's folder, never one outside it, through a path or a link.
func (s *Store) ReadFile(name, path string) Text {
	if !s.exists(name) {
		return Text{Problem: NoSkill(name)}
	}
	root, err := filepath.EvalSymlinks(s.folderOf(name))
	if err != nil {
		return Text{Problem: NoSkill(name)}
	}
	target := filepath.Join(root, path)
	if filepath.IsAbs(path) {
		target = filepath.Clean(path)
	}
	outside := Text{Problem: path + " is outside the skill's folder."}
	if !isInside(root, target) {
		return outside
	}
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		return Text{Problem: fmt.Sprintf("The skill %s has no file %s.", name, path)}
	}
	// A link in the folder may lead out of it.
	if real, err := filepath.EvalSymlinks(target); err != nil || !isInside(root, real) {
		return outside
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return Text{Problem: fmt.Sprintf("The skill %s has no file %s.", name, path)}
	}
	return Text{OK: true, Text: Clipped(string(data))}
}

func (s *Store) Draft(skill protocol.Skill) Draft {
	before := ""
	if s.exists(skill.Name) {
		data, _ := os.ReadFile(s.pathOf(skill.Name))
		before = string(data)
	}
	extra := ""
	if before != "" {
		extra = Parse(before).Extra
	}
	return Draft{After: FileText(skill, extra), Before: before, Label: Label(skill.Name)}
}

// Save creates the skill, or replaces the one of that name.
func (s *Store) Save(skill protocol.Skill) error {
	draft := s.Draft(skill)
	if err := os.MkdirAll(s.folderOf(skill.Name), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.pathOf(skill.Name), []byte(draft.After), 0o644)
}

// Create makes a new skill, from the page: a name already taken is refused.
func (s *Store) Create(skill protocol.Skill) protocol.SkillChange {
	if s.exists(skill.Name) {
		return taken(skill.Name)
	}
	if err := s.Save(skill); err != nil {
		return refused(err.Error())
	}
	return changed(skill.Name)
}

// Update saves a skill changed on the page: a new name moves its folder, with its other files.
func (s *Store) Update(name string, skill protocol.Skill) protocol.SkillChange {
	if !s.exists(name) {
		return refused(NoSkill(name))
	}
	if skill.Name != name && s.exists(skill.Name) {
		return taken(skill.Name)
	}
	if skill.Name != name {
		if err := os.Rename(s.folderOf(name), s.folderOf(skill.Name)); err != nil {
			return refused(err.Error())
		}
	}
	if err := s.Save(skill); err != nil {
		return refused(err.Error())
	}
	return changed(skill.Name)
}

// Remove deletes a skill's folder, with every file in it.
func (s *Store) Remove(name string) protocol.SkillChange {
	if !s.exists(name) {
		return refused(NoSkill(name))
	}
	if err := os.RemoveAll(s.folderOf(name)); err != nil {
		return refused(err.Error())
	}
	return changed(name)
}

func (s *Store) exists(name string) bool {
	if !protocol.SkillNamePattern.MatchString(name) {
		return false
	}
	info, err := os.Stat(s.pathOf(name))
	return err == nil && !info.IsDir()
}

func (s *Store) folderOf(name string) string { return filepath.Join(s.Folder, name) }

func (s *Store) pathOf(name string) string { return filepath.Join(s.folderOf(name), SkillFile) }

func (s *Store) fileOf(name string) File {
	data, _ := os.ReadFile(s.pathOf(name))
	return Parse(string(data))
}

// filesOf is the skill's other files, by their path in its folder, written with slashes.
func (s *Store) filesOf(name string) []string {
	root := s.folderOf(name)
	files := []string{}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		if relative = filepath.ToSlash(relative); relative != SkillFile {
			files = append(files, relative)
		}
		return nil
	})
	sort.Strings(files)
	return files[:min(len(files), MaxFiles)]
}
