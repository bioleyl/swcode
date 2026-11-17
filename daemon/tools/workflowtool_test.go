package tools

import (
	"strings"
	"testing"

	"swcode/daemon/protocol"
)

func TestDraftOfLaysTheStepsOut(t *testing.T) {
	draft, problem := draftOf(workflowInput{
		Name: "Team",
		Steps: []workflowStepInput{
			{ID: "po", Name: "PO", Prompt: "Write the tickets."},
			{ID: "dev", Name: "Dev", Prompt: "Implement one."},
		},
		Edges: []workflowEdgeInput{{From: "po", To: "dev"}},
	})
	if problem != "" {
		t.Fatalf("draftOf: %s", problem)
	}
	if !draft.Draft || len(draft.Steps) != 4 || len(draft.Edges) != 3 {
		t.Fatalf("draft = %+v", draft)
	}
	if draft.Steps[2].Position.Y <= draft.Steps[1].Position.Y {
		t.Fatalf("steps are not laid out: %+v", draft.Steps)
	}
	if draft.Edges[0].ID == "" || draft.Edges[0].To != "dev" {
		t.Fatalf("edge = %+v", draft.Edges[0])
	}
}

func TestDraftOfRefusesAGraphWithoutOnePath(t *testing.T) {
	_, problem := draftOf(workflowInput{
		Name:  "Team",
		Steps: []workflowStepInput{{ID: "po", Name: "PO", Prompt: "Write."}, {ID: "dev", Name: "Dev", Prompt: "Build."}},
	})
	if !strings.Contains(problem, "Start leads to one step") {
		t.Fatalf("problem = %q", problem)
	}
}

func TestWorkflowToolIsOffered(t *testing.T) {
	tool, ok := all[protocol.ToolNameWorkflow]
	if !ok || tool.Name != protocol.ToolNameWorkflow {
		t.Fatalf("workflow tool = %+v, %v", tool, ok)
	}
}
