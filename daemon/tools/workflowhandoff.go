package tools

import (
	"context"

	"swcode/daemon/protocol"
)

const (
	// maxHandoffReport is the most a step's report may say: the next step reads it in a file.
	maxHandoffReport = 20_000
)

type handoffInput struct {
	Exit   string `json:"exit,omitempty"`
	Report string `json:"report"`
}

// Handoff ends a workflow step: the step it goes on to, and the report the next step reads. The runner follows the
// step it names; empty finishes the workflow. Only a step's session has it.
var Handoff = Define(Spec[handoffInput]{
	Check: func(_ context.Context, _ handoffInput, _ *Context) Check {
		// Offered only in a step's session (toolsFor adds it by its WorkflowStepID): the session's Meta may be
		// older than the Context, so its presence is enough.
		return Ready(func(context.Context) Outcome {
			return Outcome{Meta: "handoff", OK: true, Output: "The workflow goes on from your report."}
		}, nil)
	},
	Description: "Ends a step of a workflow: says which output it takes, and the report the next step reads. Call it when the step is done. `exit` is the id (or the label) of one of the step's outputs; it is required when the step has outputs. Leave it empty only for a step with one way on.",
	Input: Object(
		Optional("exit", String().Trimmed().Describe("The id or label of the step's output to take; required when the step has outputs.")),
		Required("report", String().Trimmed().Min(1).Max(maxHandoffReport).Describe("What the next step needs to know: the report it reads.")),
	),
	Name:      protocol.ToolNameHandoff,
	Subject:   reads[handoffInput],
	Summarize: func(input handoffInput) string { return "Handoff: " + input.Report },
})
