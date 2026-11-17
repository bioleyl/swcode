package app_test

import (
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

// TestAWorkflowProposedByTheAgentIsADraft checks the tool an agent uses to offer a team: the graph becomes a project's
// draft, which its person confirms before a session can pick it.
func TestAWorkflowProposedByTheAgentIsADraft(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{
		call("workflow", map[string]any{
			"description": "A team that builds one feature.",
			"name":        "Team",
			"steps": []any{
				map[string]any{"id": "po", "name": "PO", "role": "You write the tickets.", "prompt": "Write the tickets."},
				map[string]any{"id": "dev", "name": "Dev", "prompt": "Implement one ticket."},
			},
			"edges": []any{map[string]any{"from": "po", "to": "dev"}},
		}),
		text("I proposed the team."),
	}})
	session := test.NewSession(sessions.Request{Prompt: "Make me a team of a PO and a dev"})
	test.UntilIdle(session.ID)

	workflows, _ := test.App.Workflows.List()
	if len(workflows) != 1 {
		t.Fatalf("workflows = %+v", workflows)
	}
	workflow := workflows[0]
	if !workflow.Draft || workflow.Name != "Team" || len(workflow.Steps) != 2 || len(workflow.Edges) != 1 {
		t.Fatalf("workflow = %+v", workflow)
	}
	if workflow.ProjectID == "" {
		t.Fatal("the proposed workflow is not a project's")
	}
	if problem := test.App.Workflows.Confirm(workflow.ID); problem != "" {
		t.Fatalf("Confirm: %s", problem)
	}
	confirmed, _ := test.App.Workflows.Workflow(workflow.ID)
	if confirmed.Draft {
		t.Fatal("the workflow stays a draft after Confirm")
	}
	proposed := false
	for _, finished := range Finished(test.Events(session.ID)) {
		if strings.Contains(finished.Output, "Proposed the workflow") {
			proposed = true
		}
	}
	if !proposed {
		t.Fatal("the agent's result does not say the workflow was proposed")
	}
}
