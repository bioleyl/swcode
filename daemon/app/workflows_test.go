package app_test

import (
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
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
	if !workflow.Draft || workflow.Name != "Team" || len(workflow.Steps) != 4 || len(workflow.Edges) != 3 {
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

// TestAStepWaitingForItsSubagentsStaysRunning: a step's turn ends before its subagents report, which wakes it for the
// turn that hands off; the run must not take that first end for the step's own.
func TestAStepWaitingForItsSubagentsStaysRunning(t *testing.T) {
	test := Start(t, Options{
		Files: subagentFiles,
		Replies: []fakellama.Reply{
			{ToolCalls: []fakellama.ToolCall{agentCall("Find the tools", "TASK-W: where are the tools?")}},
			call("handoff", map[string]any{"exit": "done", "report": "The tools are in src/tools.ts."}),
			text("Handed off."),
		},
		Scripts: map[string][]fakellama.Reply{"TASK-W": {text("src/tools.ts:1 lists them in TOOLS.")}},
		Slots:   2,
	})
	test.NewSession(sessions.Request{})
	project, ok := test.App.Projects.Find(test.Workspace, protocol.LocalHost())
	if !ok {
		t.Fatal("the session's folder is no project")
	}
	workflow, problem := test.App.Workflows.Save("", protocol.WorkflowDraft{
		Name:      "Team",
		ProjectID: project.ID,
		Steps: []protocol.WorkflowStep{
			{ID: "start", Kind: protocol.WorkflowStepKindStart, Name: "Start"},
			{
				Exits:  []protocol.WorkflowExit{{ID: "done", Label: "Done", When: "the tools are found"}},
				ID:     "work",
				Kind:   protocol.WorkflowStepKindStep,
				Name:   "Work",
				Prompt: "Find the tools.",
				Role:   "You look.",
			},
			{ID: "end", Kind: protocol.WorkflowStepKindEnd, Name: "End"},
		},
		Edges: []protocol.WorkflowEdge{
			{From: "start", ID: "e1", To: "work"},
			{From: "work", ID: "e2", Port: "done", To: "end"},
		},
	})
	if problem != "" {
		t.Fatal(problem)
	}
	if problem := test.App.Workflows.Confirm(workflow.ID); problem != "" {
		t.Fatal(problem)
	}

	socket := connect(t, test)
	socket.send(map[string]any{"prompt": "Find the tools", "type": "createSession", "workflowId": workflow.ID, "workspace": test.Workspace})
	runSession := socket.next("sessionCreated")["session"].(map[string]any)
	runSessionID, _ := runSession["id"].(string)

	var run protocol.WorkflowRun
	test.Until("the run to end", func() bool {
		_, runs := test.App.Workflows.List()
		for _, candidate := range runs {
			if candidate.SessionID == runSessionID {
				run = candidate
				return run.Status != protocol.WorkflowRunStatusRunning && run.Status != protocol.WorkflowRunStatusWaiting
			}
		}
		return false
	})
	if run.Status != protocol.WorkflowRunStatusDone {
		t.Fatalf("run %+v", run)
	}
	if len(run.Steps) != 1 || run.Steps[0].Status != protocol.WorkflowStepStatusDone {
		t.Fatalf("steps %+v", run.Steps)
	}
	if run.Steps[0].Summary != "The tools are in src/tools.ts." {
		t.Fatalf("summary %q", run.Steps[0].Summary)
	}
}
