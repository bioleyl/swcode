// Package projects is the folders the daemon's sessions work in, each on a host, with a name for people.
package projects

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
)

const (
	unknownProject = "There is no such project."
	unknownTodo    = "There is no such todo."
	homeMark       = "~"
)

// Store keeps the projects in one file, projects.json in the data folder.
type Store struct {
	file string
}

func NewStore(file string) *Store { return &Store{file: file} }

func (s *Store) Load() []protocol.Project {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return []protocol.Project{}
	}
	var projects []protocol.Project
	if json.Unmarshal(data, &projects) != nil {
		return []protocol.Project{}
	}
	return projects
}

func (s *Store) Save(projects []protocol.Project) error {
	if err := os.MkdirAll(filepath.Dir(s.file), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.file, data, 0o600)
}

// Deps is what the projects work with.
type Deps struct {
	Store *Store
	Hosts *hosts.Registry
	// HostOptions is where a new project can be, for the project dialog.
	HostOptions func() []protocol.HostOption
	// ProjectsFolder is where new projects usually go on this computer: the folder browser starts there.
	ProjectsFolder string
	// ForgetSessions deletes the sessions working in this folder of a host, or says why not: one of them is
	// working.
	ForgetSessions func(path string, host protocol.HostRef) string
	Now            func() time.Time
	NewID          func() string
	LogError       func(error)
}

// Listener hears the projects whenever they change; it must not call back into the service.
type Listener func(message protocol.ServerMessage)

// Service is the projects of this daemon: the folders its sessions work in. Removing one deletes its sessions and
// forgets it, its folder stays; every folder a session works in has one, made when needed.
type Service struct {
	deps      Deps
	mu        sync.Mutex
	projects  []protocol.Project
	listeners map[int]Listener
	next      int
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps, listeners: map[int]Listener{}, projects: []protocol.Project{}}
}

// Load reads the projects kept on disk, but those without a folder: chats made them at a start, before a start told
// them apart from coding sessions. The file loses them at the next change.
func (s *Service) Load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects = slices.DeleteFunc(s.deps.Store.Load(), func(project protocol.Project) bool { return project.Path == "" })
}

func (s *Service) Subscribe(listener Listener) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.next
	s.next++
	s.listeners[id] = listener
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.listeners, id)
	}
}

func (s *Service) List() []protocol.Project {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list()
}

func (s *Service) list() []protocol.Project {
	sorted := append([]protocol.Project{}, s.projects...)
	sort.SliceStable(sorted, func(first, second int) bool { return sorted[first].Name < sorted[second].Name })
	return sorted
}

// Create makes a project of a folder; picking the folder of a project there already is gives that project.
func (s *Service) Create(ctx context.Context, folder string, host *protocol.HostRef, newFolder string) (protocol.Project, string) {
	ref := protocol.LocalHost()
	if host != nil {
		ref = *host
	}
	target := s.deps.Hosts.HostOf(ref)
	if newFolder != "" {
		made, err := target.MakeFolder(ctx, folder, newFolder)
		if err != nil {
			return protocol.Project{}, "The host did not answer: " + err.Error()
		}
		if !made.OK {
			return protocol.Project{}, made.Problem
		}
		return s.Ensure(made.Path, ref), ""
	}
	path, problem := hosts.ChooseFolder(ctx, target, folder, "")
	if problem != "" {
		return protocol.Project{}, problem
	}
	return s.Ensure(path, ref), ""
}

func (s *Service) Remove(projectID string) string {
	s.mu.Lock()
	var found *protocol.Project
	for index := range s.projects {
		if s.projects[index].ID == projectID {
			found = &s.projects[index]
		}
	}
	s.mu.Unlock()
	if found == nil {
		return unknownProject
	}
	if refused := s.deps.ForgetSessions(found.Path, found.Host); refused != "" {
		return refused
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := []protocol.Project{}
	for _, project := range s.projects {
		if project.ID != projectID {
			kept = append(kept, project)
		}
	}
	s.change(kept)
	return ""
}

// SetDefaults sets what the project's new sessions start with: nil for the app's own defaults again.
func (s *Service) SetDefaults(projectID string, defaults *protocol.SessionDefaults) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := slices.IndexFunc(s.projects, func(project protocol.Project) bool { return project.ID == projectID })
	if index < 0 {
		return unknownProject
	}
	projects := append([]protocol.Project{}, s.projects...)
	projects[index].Defaults = defaults
	s.change(projects)
	return ""
}

// SetSkill turns one of the skills the project's sessions find on or off for them: they read it from their next turn.
func (s *Service) SetSkill(projectID, name string, on bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := slices.IndexFunc(s.projects, func(project protocol.Project) bool { return project.ID == projectID })
	if index < 0 {
		return unknownProject
	}
	projects := append([]protocol.Project{}, s.projects...)
	projects[index].SkillsOff = withSkill(projects[index].SkillsOff, name, !on)
	s.change(projects)
	return ""
}

// withSkill is the names turned off, with this one among them or not, sorted.
func withSkill(off []string, name string, turnedOff bool) []string {
	kept := slices.DeleteFunc(slices.Clone(off), func(each string) bool { return each == name })
	if turnedOff {
		kept = append(kept, name)
	}
	slices.Sort(kept)
	return kept
}

// AddTodo adds a todo to the project, at the end of its list.
func (s *Service) AddTodo(projectID, text string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := slices.IndexFunc(s.projects, func(project protocol.Project) bool { return project.ID == projectID })
	if index < 0 {
		return unknownProject
	}
	projects := append([]protocol.Project{}, s.projects...)
	todo := protocol.ProjectTodo{CreatedAt: protocol.Timestamp(s.deps.Now()), ID: s.deps.NewID(), Text: text}
	projects[index].Todos = append(slices.Clone(projects[index].Todos), todo)
	s.change(projects)
	return ""
}

// SetTodoDone marks one of the project's todos done or not.
func (s *Service) SetTodoDone(projectID, todoID string, done bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, todoIndex, problem := s.todoAt(projectID, todoID)
	if problem != "" {
		return problem
	}
	projects := append([]protocol.Project{}, s.projects...)
	projects[index].Todos = slices.Clone(projects[index].Todos)
	projects[index].Todos[todoIndex].Done = done
	s.change(projects)
	return ""
}

// SetTodoText changes what one of the project's todos says.
func (s *Service) SetTodoText(projectID, todoID, text string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, todoIndex, problem := s.todoAt(projectID, todoID)
	if problem != "" {
		return problem
	}
	projects := append([]protocol.Project{}, s.projects...)
	projects[index].Todos = slices.Clone(projects[index].Todos)
	projects[index].Todos[todoIndex].Text = text
	s.change(projects)
	return ""
}

// LinkTodos makes the todos of the project that are in the list point at the session they started, and leaves the
// others: one may have been deleted while the session was made.
func (s *Service) LinkTodos(projectID string, todoIDs []string, sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := slices.IndexFunc(s.projects, func(project protocol.Project) bool { return project.ID == projectID })
	if index < 0 {
		return unknownProject
	}
	todos := slices.Clone(s.projects[index].Todos)
	for todoIndex := range todos {
		if slices.Contains(todoIDs, todos[todoIndex].ID) {
			todos[todoIndex].SessionID = sessionID
		}
	}
	projects := append([]protocol.Project{}, s.projects...)
	projects[index].Todos = todos
	s.change(projects)
	return ""
}

// RemoveTodo deletes one of the project's todos.
func (s *Service) RemoveTodo(projectID, todoID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, todoIndex, problem := s.todoAt(projectID, todoID)
	if problem != "" {
		return problem
	}
	projects := append([]protocol.Project{}, s.projects...)
	projects[index].Todos = slices.Delete(slices.Clone(projects[index].Todos), todoIndex, todoIndex+1)
	s.change(projects)
	return ""
}

// OpenTodo adds a todo to the project of a folder on a host, at the end of its list, and answers its todos.
func (s *Service) OpenTodo(folder string, host protocol.HostRef, text string) ([]protocol.ProjectTodo, string) {
	project, found := s.Find(folder, host)
	if !found {
		return nil, unknownProject
	}
	if slices.ContainsFunc(project.Todos, func(todo protocol.ProjectTodo) bool { return todo.Text == text }) {
		return project.Todos, "The project already has a todo “" + text + "”."
	}
	if problem := s.AddTodo(project.ID, text); problem != "" {
		return nil, problem
	}
	return s.todosOf(project.ID)
}

// CloseTodo marks done the todo of the project of a folder on a host that says text, and answers its todos.
func (s *Service) CloseTodo(folder string, host protocol.HostRef, text string) ([]protocol.ProjectTodo, string) {
	project, found := s.Find(folder, host)
	if !found {
		return nil, unknownProject
	}
	index := slices.IndexFunc(project.Todos, func(todo protocol.ProjectTodo) bool { return todo.Text == text })
	if index < 0 {
		return project.Todos, "The project has no todo “" + text + "”."
	}
	if problem := s.SetTodoDone(project.ID, project.Todos[index].ID, true); problem != "" {
		return nil, problem
	}
	return s.todosOf(project.ID)
}

// todosOf is the project's todos by its id, or why none.
func (s *Service) todosOf(projectID string) ([]protocol.ProjectTodo, string) {
	project, found := s.ByID(projectID)
	if !found {
		return nil, unknownProject
	}
	return project.Todos, ""
}

// todoAt is the project's index, and its todo's, or the problem; the caller holds the lock.
func (s *Service) todoAt(projectID, todoID string) (int, int, string) {
	index := slices.IndexFunc(s.projects, func(project protocol.Project) bool { return project.ID == projectID })
	if index < 0 {
		return 0, 0, unknownProject
	}
	todoIndex := slices.IndexFunc(s.projects[index].Todos, func(todo protocol.ProjectTodo) bool { return todo.ID == todoID })
	if todoIndex < 0 {
		return 0, 0, unknownTodo
	}
	return index, todoIndex, ""
}

// Find is the project of a folder on a host, if it has one.
func (s *Service) Find(folder string, host protocol.HostRef) (protocol.Project, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, project := range s.projects {
		if project.Path == folder && hosts.SameHost(project.Host, host) {
			return project, true
		}
	}
	return protocol.Project{}, false
}

// ByID is a project by its id, if there is one.
func (s *Service) ByID(projectID string) (protocol.Project, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := slices.IndexFunc(s.projects, func(project protocol.Project) bool { return project.ID == projectID })
	if index < 0 {
		return protocol.Project{}, false
	}
	return s.projects[index], true
}

// Ensure is the project of a folder on a host, made for it when there is none yet.
func (s *Service) Ensure(folder string, host protocol.HostRef) protocol.Project {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, project := range s.projects {
		if project.Path == folder && hosts.SameHost(project.Host, host) {
			return project
		}
	}
	project := protocol.Project{
		CreatedAt: protocol.Timestamp(s.deps.Now()), Host: host, ID: s.deps.NewID(), Name: baseName(folder), Path: folder,
	}
	s.change(append(append([]protocol.Project{}, s.projects...), project))
	return project
}

// baseName is a folder's name, whichever way its path is written.
func baseName(folder string) string {
	if strings.Contains(folder, `\`) {
		return path.Base(strings.ReplaceAll(folder, `\`, "/"))
	}
	return path.Base(folder)
}

// HostOptions is where a new project can be: this computer, its WSL distributions, the SSH endpoints.
func (s *Service) HostOptions() []protocol.HostOption {
	return s.deps.HostOptions()
}

// Folders is the subfolders of a folder on a host: when none is given, the projects' usual folder on this
// computer, the home folder elsewhere.
func (s *Service) Folders(ctx context.Context, folder string, host *protocol.HostRef) hosts.FolderListing {
	ref := protocol.LocalHost()
	if host != nil {
		ref = *host
	}
	start := folder
	if start == "" {
		start = homeMark
		if hosts.IsLocal(ref) {
			start = s.deps.ProjectsFolder
		}
	}
	listing, err := s.deps.Hosts.HostOf(ref).ListFolders(ctx, start)
	if err != nil {
		return hosts.FolderListing{Path: start, Problem: "The host did not answer: " + err.Error()}
	}
	return listing
}

// change keeps the projects and tells the listeners; the caller holds the lock.
func (s *Service) change(projects []protocol.Project) {
	s.projects = projects
	if err := s.deps.Store.Save(projects); err != nil {
		s.deps.LogError(err)
	}
	message := protocol.ServerMessage{Value: protocol.ServerMessageProjects{Projects: s.list()}}
	for _, listener := range s.listeners {
		listener(message)
	}
}
