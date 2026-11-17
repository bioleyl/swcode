package workflows

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"swcode/daemon/protocol"
)

func testService(t *testing.T) *Service {
	t.Helper()
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	next := 0
	service := NewService(Deps{
		LogError: func(err error) { t.Fatal(err) },
		NewID: func() string {
			next++
			return "id-" + string(rune('0'+next))
		},
		Now:   func() time.Time { return now },
		Store: NewStore(filepath.Join(t.TempDir(), "workflows.json")),
	})
	return service
}

func step(id, name string) protocol.WorkflowStep {
	return protocol.WorkflowStep{ID: id, Name: name, Prompt: "Do the work.", Position: protocol.WorkflowPoint{}}
}

func withExits(step protocol.WorkflowStep, exits ...protocol.WorkflowExit) protocol.WorkflowStep {
	step.Exits = exits
	return step
}

func TestSaveConfirmAndRemove(t *testing.T) {
	service := testService(t)
	draft := protocol.WorkflowDraft{
		Draft: true, Edges: []protocol.WorkflowEdge{{From: "po", ID: "e1", To: "dev"}}, Name: "Team",
		Steps: []protocol.WorkflowStep{step("po", "PO"), step("dev", "Dev")},
	}
	saved, problem := service.Save("", draft)
	if problem != "" {
		t.Fatalf("Save: %s", problem)
	}
	if saved.ID == "" || !saved.Draft {
		t.Fatalf("saved = %+v", saved)
	}
	workflows, _ := service.List()
	if len(workflows) != 1 || workflows[0].Name != "Team" {
		t.Fatalf("list = %+v", workflows)
	}
	if problem := service.Confirm(saved.ID); problem != "" {
		t.Fatalf("Confirm: %s", problem)
	}
	confirmed, ok := service.Workflow(saved.ID)
	if !ok || confirmed.Draft {
		t.Fatalf("confirmed = %+v", confirmed)
	}
	if problem := service.Remove(saved.ID); problem != "" {
		t.Fatalf("Remove: %s", problem)
	}
	if _, ok := service.Workflow(saved.ID); ok {
		t.Fatal("the workflow stays after Remove")
	}
}

func TestSaveCreatesWithTheEditorsID(t *testing.T) {
	service := testService(t)
	saved, problem := service.Save("editor-1", protocol.WorkflowDraft{Name: "Team"})
	if problem != "" {
		t.Fatalf("Save: %s", problem)
	}
	if saved.ID != "editor-1" {
		t.Fatalf("id = %q", saved.ID)
	}
}

func TestGraphProblem(t *testing.T) {
	linear := []protocol.WorkflowStep{step("a", "A"), step("b", "B")}
	edge := protocol.WorkflowEdge{From: "a", ID: "e", To: "b"}
	cases := []struct {
		name  string
		steps []protocol.WorkflowStep
		edges []protocol.WorkflowEdge
		want  string
	}{
		{name: "empty", want: "at least one step"},
		{name: "linear", steps: linear, edges: []protocol.WorkflowEdge{edge}},
		{name: "two starts", steps: linear, want: "starts at one step"},
		{name: "two outputs", steps: []protocol.WorkflowStep{withExits(step("a", "A"), protocol.WorkflowExit{ID: "x", Label: "X", When: "it is x"}, protocol.WorkflowExit{ID: "y", Label: "Y", When: "it is y"}), step("b", "B"), step("c", "C")}, edges: []protocol.WorkflowEdge{{From: "a", ID: "e1", Port: "x", To: "b"}, {From: "a", ID: "e2", Port: "y", To: "c"}}},
		{name: "several ways without outputs", steps: []protocol.WorkflowStep{step("a", "A"), step("b", "B"), step("c", "C")}, edges: []protocol.WorkflowEdge{{From: "a", ID: "e1", To: "b"}, {From: "a", ID: "e2", To: "c"}}, want: "give it outputs"},
		{name: "a loop from a start", steps: []protocol.WorkflowStep{step("a", "A"), step("b", "B"), step("c", "C")}, edges: []protocol.WorkflowEdge{{From: "a", ID: "e1", To: "b"}, {From: "b", ID: "e2", To: "c"}, {From: "c", ID: "e3", To: "b"}}},
		{name: "a loop without a start", steps: linear, edges: []protocol.WorkflowEdge{edge, {From: "b", ID: "e2", To: "a"}}, want: "starts at one step"},
		{name: "unknown step", steps: linear, edges: []protocol.WorkflowEdge{{From: "a", ID: "e", To: "x"}}, want: "not in the workflow"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			problem := GraphProblem(test.steps, test.edges)
			if test.want == "" && problem != "" {
				t.Fatalf("problem = %q", problem)
			}
			if test.want != "" && !strings.Contains(problem, test.want) {
				t.Fatalf("problem = %q, want %q", problem, test.want)
			}
		})
	}
}

func TestRuns(t *testing.T) {
	service := testService(t)
	run := service.NewRun(protocol.WorkflowRun{
		Brief: "Build it", ProjectID: "p1", SessionID: "s0", WorkflowID: "w1", WorkflowName: "Team",
		Steps: []protocol.WorkflowRunStep{{Name: "PO", Status: protocol.WorkflowStepStatusPending, StepID: "po"}},
	})
	if run.ID == "" || run.Status != protocol.WorkflowRunStatusRunning {
		t.Fatalf("run = %+v", run)
	}
	run.Status = protocol.WorkflowRunStatusDone
	if problem := service.UpdateRun(run); problem != "" {
		t.Fatalf("UpdateRun: %s", problem)
	}
	kept, ok := service.Run(run.ID)
	if !ok || kept.Status != protocol.WorkflowRunStatusDone {
		t.Fatalf("kept = %+v", kept)
	}
	if problem := service.StopRun(run.ID); problem != "" {
		t.Fatalf("StopRun: %s", problem)
	}
	kept, _ = service.Run(run.ID)
	if kept.Status != protocol.WorkflowRunStatusStopped {
		t.Fatalf("stopped = %+v", kept)
	}
}
