package tools

import (
	"context"
	"fmt"

	"swcode/daemon/protocol"
	"swcode/daemon/workflows"
)

const (
	workflowStepsGap  = 136
	workflowStepIDFmt = "step-%d"
	workflowEdgeIDFmt = "edge-%d"
)

type workflowStepInput struct {
	ID     string              `json:"id"`
	Name   string              `json:"name"`
	Role   string              `json:"role"`
	Prompt string              `json:"prompt"`
	Preset string              `json:"preset,omitempty"`
	Exits  []workflowExitInput `json:"exits,omitempty"`
}

type workflowExitInput struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	When  string `json:"when"`
}

type workflowEdgeInput struct {
	From string `json:"from"`
	Port string `json:"port,omitempty"`
	To   string `json:"to"`
}

type workflowInput struct {
	Description string              `json:"description,omitempty"`
	Edges       []workflowEdgeInput `json:"edges"`
	Name        string              `json:"name"`
	Steps       []workflowStepInput `json:"steps"`
}

// draftOf builds the workflow the tool proposed, its nodes laid out one under the other.
func draftOf(input workflowInput) (protocol.WorkflowDraft, string) {
	steps := make([]protocol.WorkflowStep, len(input.Steps))
	for index, step := range input.Steps {
		id := step.ID
		if id == "" {
			id = fmt.Sprintf(workflowStepIDFmt, index+1)
		}
		exits := make([]protocol.WorkflowExit, len(step.Exits))
		for at, exit := range step.Exits {
			exits[at] = protocol.WorkflowExit{ID: exit.ID, Label: exit.Label, When: exit.When}
		}
		steps[index] = protocol.WorkflowStep{
			ID: id, Name: step.Name, Role: step.Role, Prompt: step.Prompt, Preset: step.Preset, Exits: exits,
			Position: protocol.WorkflowPoint{X: 0, Y: float64(index * workflowStepsGap)},
		}
	}
	edges := make([]protocol.WorkflowEdge, len(input.Edges))
	for index, edge := range input.Edges {
		edges[index] = protocol.WorkflowEdge{From: edge.From, ID: fmt.Sprintf(workflowEdgeIDFmt, index+1), Port: edge.Port, To: edge.To}
	}
	draft := protocol.WorkflowDraft{
		Description: input.Description, Draft: true, Edges: edges, Name: input.Name, Steps: steps,
	}
	if problem := workflows.GraphProblem(steps, edges); problem != "" {
		return protocol.WorkflowDraft{}, problem
	}
	return draft, ""
}

var workflowTool = Define(Spec[workflowInput]{
	Check: func(_ context.Context, input workflowInput, tc *Context) Check {
		if tc.ProposeWorkflow == nil {
			return Refuse("A workflow is proposed in a coding session of a project.")
		}
		draft, problem := draftOf(input)
		if problem != "" {
			return Refuse(problem)
		}
		return Ready(func(ctx context.Context) Outcome {
			proposed, problem := tc.ProposeWorkflow(tc.ProjectFolder, tc.HostRef, draft)
			if problem != "" {
				return Failure(problem, "failed")
			}
			return Outcome{
				Meta: "proposed",
				OK:   true,
				Output: fmt.Sprintf(
					"Proposed the workflow %q with %d steps. Its person confirms it in Workflows before a session can pick it.",
					proposed.Name, len(proposed.Steps),
				),
			}
		}, nil)
	},
	Description: "Proposes a workflow: roles of a team as sessions of their own, each handed the previous step’s report in a file. The steps are given by edges; a step may lead to several steps, and a loop may come back (QA to Dev for a fix, Dev to QA to test again). Each step names its role and what it does, and its session calls the handoff tool to choose one of its outputs, or empty to finish. A step with several ways on must name each output and when it is taken. It is kept as a draft, which the person confirms in Workflows before a session picks it. Use it when the person asks for a team: a PO who writes tickets, a dev who implements one, a QA who tests it, and so on.",
	Input: Object(
		Required("name", String().Trimmed().Min(1).Max(protocol.MaxWorkflowName).Describe("The workflow's name, as people read it.")),
		Optional("description", String().Trimmed().Max(protocol.MaxWorkflowDescription).Describe("What the team builds, in a sentence or two.")),
		Required("steps", Array(Object(
			Required("id", String().Trimmed().Min(1).Max(80).Matching("^[A-Za-z0-9_-]+$").Describe("The step's id, which the edges name: po, dev, qa. Letters, digits, - and _ only.")),
			Required("name", String().Trimmed().Min(1).Max(protocol.MaxWorkflowStepName).Describe("The role's name: PO, Dev, QA.")),
			Optional("role", String().Trimmed().Max(protocol.MaxWorkflowRole).Describe("The role's instructions, given to its session before its task: who it is and how it works.")),
			Required("prompt", String().Trimmed().Min(1).Max(protocol.MaxWorkflowPrompt).Describe("What this step has to do. The run adds the person's brief and the previous step's handoff.")),
			Optional("preset", String().Trimmed().Describe("The preset the step's session works with; the project's when left out.")),
			Optional("exits", Array(Object(
				Required("id", String().Trimmed().Min(1).Max(80).Matching("^[A-Za-z0-9_-]+$").Describe("The output's id, which the edge leaves by: fix, pass.")),
				Required("label", String().Trimmed().Min(1).Max(protocol.MaxWorkflowStepName).Describe("The output's name: Fix needed, Passed.")),
				Required("when", String().Trimmed().Min(1).Max(protocol.MaxWorkflowPrompt).Describe("When the step takes this output, as a condition its agent reads.")),
			)).Max(protocol.MaxWorkflowExits).Describe("The outputs of a step that may go several ways. Leave them out for a step with one way.")),
		)).Min(1).Max(protocol.MaxWorkflowSteps).Describe("The roles of the workflow, in the order they run.")),
		Required("edges", Array(Object(
			Required("from", String().Trimmed().Min(1).Describe("The step the work leaves.")),
			Optional("port", String().Trimmed().Describe("The output's id this edge leaves by, for a step with outputs.")),
			Required("to", String().Trimmed().Min(1).Describe("The step that takes it next.")),
		)).Max(protocol.MaxWorkflowEdges).Describe("How the steps follow each other: an edge from a step to each step it may go on to. A loop back is an edge too.")),
	),
	Name:      protocol.ToolNameWorkflow,
	Subject:   reads[workflowInput],
	Summarize: func(input workflowInput) string { return "Workflow: " + input.Name },
})
