// Package workflows is the workflows of this daemon, global or a project's, and the runs of them: a run session that
// starts one session per step, each a role of the workflow.
package workflows

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"

	"swcode/daemon/protocol"
)

const unknownWorkflow = "There is no such workflow."
const unknownRun = "There is no such workflow run."

// File is what workflows.json holds.
type File struct {
	Runs      []protocol.WorkflowRun `json:"runs"`
	Workflows []protocol.Workflow    `json:"workflows"`
}

// Store keeps the workflows and their runs in one file, workflows.json in the data folder.
type Store struct {
	file string
}

func NewStore(file string) *Store { return &Store{file: file} }

func (s *Store) Load() File {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return File{}
	}
	var kept File
	if json.Unmarshal(data, &kept) != nil {
		return File{}
	}
	return kept
}

func (s *Store) Save(kept File) error {
	if err := os.MkdirAll(filepath.Dir(s.file), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(kept, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.file, data, 0o600)
}

// Deps is what the workflows work with.
type Deps struct {
	Store    *Store
	Now      func() time.Time
	NewID    func() string
	LogError func(error)
}

// Listener hears the workflows and their runs whenever they change; it must not call back into the service.
type Listener func(message protocol.ServerMessage)

// Service is the workflows and runs kept on disk.
type Service struct {
	deps      Deps
	mu        sync.Mutex
	workflows []protocol.Workflow
	runs      []protocol.WorkflowRun
	listeners map[int]Listener
	next      int
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps, listeners: map[int]Listener{}, runs: []protocol.WorkflowRun{}, workflows: []protocol.Workflow{}}
}

func (s *Service) Load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.deps.Store.Load()
	s.workflows = kept.Workflows
	s.runs = kept.Runs
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

// List gives every workflow and every run, for the app at connection.
func (s *Service) List() ([]protocol.Workflow, []protocol.WorkflowRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked()
}

func (s *Service) listLocked() ([]protocol.Workflow, []protocol.WorkflowRun) {
	workflows := append([]protocol.Workflow{}, s.workflows...)
	sort.SliceStable(workflows, func(first, second int) bool { return workflows[first].Name < workflows[second].Name })
	return workflows, append([]protocol.WorkflowRun{}, s.runs...)
}

func (s *Service) Workflow(id string) (protocol.Workflow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, workflow := range s.workflows {
		if workflow.ID == id {
			return workflow, true
		}
	}
	return protocol.Workflow{}, false
}

// Save writes a workflow as the editor holds it: a new one when its id is unknown, the one it names otherwise. A
// draft may be incomplete; confirmWorkflow checks it. The problem is what the person has to fix.
func (s *Service) Save(id string, draft protocol.WorkflowDraft) (protocol.Workflow, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.deps.Now()
	if id == "" {
		id = s.deps.NewID()
	}
	index := slices.IndexFunc(s.workflows, func(workflow protocol.Workflow) bool { return workflow.ID == id })
	if index < 0 {
		if len(s.workflows) >= protocol.MaxWorkflows {
			return protocol.Workflow{}, fmt.Sprintf("There are already %d workflows: remove one first.", protocol.MaxWorkflows)
		}
		workflow := protocol.Workflow{
			CreatedAt:   protocol.Timestamp(now),
			Description: draft.Description,
			Draft:       draft.Draft,
			Edges:       draft.Edges,
			ID:          id,
			Name:        draft.Name,
			ProjectID:   draft.ProjectID,
			Steps:       draft.Steps,
			UpdatedAt:   protocol.Timestamp(now),
		}
		s.workflows = append(s.workflows, workflow)
		s.change()
		return workflow, ""
	}
	workflow := s.workflows[index]
	workflow.Description = draft.Description
	workflow.Draft = draft.Draft
	workflow.Edges = draft.Edges
	workflow.Name = draft.Name
	workflow.ProjectID = draft.ProjectID
	workflow.Steps = draft.Steps
	workflow.UpdatedAt = protocol.Timestamp(now)
	s.workflows[index] = workflow
	s.change()
	return workflow, ""
}

// Confirm takes a draft into the workflows a session can pick: a workflow whose steps run one after another, from one
// start to one end.
func (s *Service) Confirm(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := slices.IndexFunc(s.workflows, func(workflow protocol.Workflow) bool { return workflow.ID == id })
	if index < 0 {
		return unknownWorkflow
	}
	workflow := s.workflows[index]
	if problem := GraphProblem(workflow.Steps, workflow.Edges); problem != "" {
		return problem
	}
	workflow.Draft = false
	workflow.UpdatedAt = protocol.Timestamp(s.deps.Now())
	s.workflows[index] = workflow
	s.change()
	return ""
}

// Remove forgets a workflow and its runs.
func (s *Service) Remove(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := slices.IndexFunc(s.workflows, func(workflow protocol.Workflow) bool { return workflow.ID == id })
	if index < 0 {
		return unknownWorkflow
	}
	s.workflows = slices.Delete(s.workflows, index, index+1)
	s.runs = slices.DeleteFunc(s.runs, func(run protocol.WorkflowRun) bool { return run.WorkflowID == id })
	s.change()
	return ""
}

// NewRun keeps a run of a workflow, its steps pending, and gives it its id and dates.
func (s *Service) NewRun(run protocol.WorkflowRun) protocol.WorkflowRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.deps.Now()
	run.CreatedAt = protocol.Timestamp(now)
	if run.ID == "" {
		run.ID = s.deps.NewID()
	}
	run.Status = protocol.WorkflowRunStatusRunning
	run.UpdatedAt = run.CreatedAt
	s.runs = append(s.runs, run)
	if len(s.runs) > protocol.MaxWorkflowRuns {
		s.runs = slices.Delete(s.runs, 0, len(s.runs)-protocol.MaxWorkflowRuns)
	}
	s.change()
	return run
}

// UpdateRun keeps the run as the runner has it now. The problem is when it has gone, removed with its workflow.
func (s *Service) UpdateRun(run protocol.WorkflowRun) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := slices.IndexFunc(s.runs, func(each protocol.WorkflowRun) bool { return each.ID == run.ID })
	if index < 0 {
		return unknownRun
	}
	run.UpdatedAt = protocol.Timestamp(s.deps.Now())
	s.runs[index] = run
	s.change()
	return ""
}

func (s *Service) Run(id string) (protocol.WorkflowRun, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, run := range s.runs {
		if run.ID == id {
			return run, true
		}
	}
	return protocol.WorkflowRun{}, false
}

// StopRun marks a run stopped: the runner stops its running step and starts no other.
func (s *Service) StopRun(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := slices.IndexFunc(s.runs, func(run protocol.WorkflowRun) bool { return run.ID == id })
	if index < 0 {
		return unknownRun
	}
	run := s.runs[index]
	run.Status = protocol.WorkflowRunStatusStopped
	run.UpdatedAt = protocol.Timestamp(s.deps.Now())
	s.runs[index] = run
	s.change()
	return ""
}

// ForgetProject removes a project's workflows and their runs: its sessions go with it.
func (s *Service) ForgetProject(projectID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := map[string]bool{}
	s.workflows = slices.DeleteFunc(s.workflows, func(workflow protocol.Workflow) bool {
		if workflow.ProjectID != projectID {
			return false
		}
		removed[workflow.ID] = true
		return true
	})
	s.runs = slices.DeleteFunc(s.runs, func(run protocol.WorkflowRun) bool {
		return run.ProjectID == projectID || removed[run.WorkflowID]
	})
	s.change()
}

// GraphProblem says why a workflow cannot run, or nothing when it can: one start, every step reached from it, and no step handing its work to itself. A step may have several steps after it, and a loop may come back: each step chooses where it goes on.
func GraphProblem(steps []protocol.WorkflowStep, edges []protocol.WorkflowEdge) string {
	if len(steps) == 0 {
		return "A workflow needs at least one step."
	}
	ids := map[string]bool{}
	stepOf := map[string]protocol.WorkflowStep{}
	for _, step := range steps {
		stepOf[step.ID] = step
		if step.ID == "" || ids[step.ID] {
			return "Each step needs an id of its own."
		}
		ids[step.ID] = true
		if step.Name == "" {
			return "Each step needs a name."
		}
		if step.Prompt == "" {
			return fmt.Sprintf("The step %s needs what it has to do.", step.Name)
		}
	}
	for _, step := range steps {
		if len(step.Exits) == 0 {
			continue
		}
		exits := map[string]bool{}
		for _, exit := range step.Exits {
			if exit.ID == "" || exits[exit.ID] {
				return fmt.Sprintf("The step %s has two outputs with one id.", step.Name)
			}
			exits[exit.ID] = true
			if exit.Label == "" || exit.When == "" {
				return fmt.Sprintf("The output %s of %s needs a name and when it is taken.", exit.ID, step.Name)
			}
		}
		for _, edge := range edges {
			if edge.From != step.ID {
				continue
			}
			if !exits[edge.Port] {
				return fmt.Sprintf("The step %s has no output %s.", step.Name, edge.Port)
			}
		}
		for id := range exits {
			used := 0
			for _, edge := range edges {
				if edge.From == step.ID && edge.Port == id {
					used++
				}
			}
			if used != 1 {
				return fmt.Sprintf("The output %s of %s goes to one step only.", id, step.Name)
			}
		}
	}
	for _, step := range steps {
		if len(step.Exits) == 0 && outgoingCount(edges, step.ID) > 1 {
			return fmt.Sprintf("The step %s goes several ways: give it outputs.", step.Name)
		}
	}
	incoming := map[string]int{}
	after := map[string][]string{}
	for _, edge := range edges {
		if !ids[edge.From] || !ids[edge.To] {
			return "An edge joins a step that is not in the workflow."
		}
		if edge.Port != "" {
			source := stepOf[edge.From]
			known := false
			for _, exit := range source.Exits {
				if exit.ID == edge.Port {
					known = true
				}
			}
			if !known {
				return fmt.Sprintf("The step %s has no output %s.", source.Name, edge.Port)
			}
		}
		if edge.From == edge.To {
			return "A step cannot hand its work to itself."
		}
		incoming[edge.To]++
		after[edge.From] = append(after[edge.From], edge.To)
	}
	starts := []string{}
	for _, step := range steps {
		if incoming[step.ID] == 0 {
			starts = append(starts, step.ID)
		}
	}
	if len(starts) != 1 {
		return "A workflow starts at one step: it has " + fmt.Sprint(len(starts)) + "."
	}
	seen := map[string]bool{starts[0]: true}
	queue := []string{starts[0]}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		for _, next := range after[at] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	if len(seen) != len(steps) {
		return "Every step is reached from the start: one of them is off the path."
	}
	return ""
}

func outgoingCount(edges []protocol.WorkflowEdge, stepID string) int {
	count := 0
	for _, edge := range edges {
		if edge.From == stepID {
			count++
		}
	}
	return count
}

// change keeps the workflows and runs and tells the listeners; the caller holds the lock.
func (s *Service) change() {
	kept := File{Runs: s.runs, Workflows: s.workflows}
	if err := s.deps.Store.Save(kept); err != nil {
		s.deps.LogError(err)
	}
	workflows, runs := s.listLocked()
	message := protocol.ServerMessage{Value: protocol.ServerMessageWorkflows{Runs: runs, Workflows: workflows}}
	for _, listener := range s.listeners {
		listener(message)
	}
}
