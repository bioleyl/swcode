package tools

import (
	"context"
	"slices"
	"strings"

	"swcode/daemon/protocol"
)

const (
	minDecisionOptions    = 2
	maxDecisionOptions    = 6
	maxDecisionQuestion   = 160
	maxDecisionOptionText = 80
	maxDecisionWhy        = 400
)

type decisionInput struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
	Chosen   string   `json:"chosen"`
	Why      string   `json:"why"`
}

// decisionText is the decision as the person reads it on the call, and on the map.
func decisionText(input decisionInput) string {
	lines := []string{
		"Question: " + input.Question,
		"Chosen: " + input.Chosen,
		"Options: " + strings.Join(input.Options, ", "),
	}
	if input.Why != "" {
		lines = append(lines, "Why: "+input.Why)
	}
	return strings.Join(lines, "\n")
}

// decisionTool records a choice that shapes the work, so the session's decision map shows it and the person can
// revisit it: the choice the model made without asking, such as a framework or a data model.
var decisionTool = Define(Spec[decisionInput]{
	Check: func(_ context.Context, input decisionInput, _ *Context) Check {
		if !slices.Contains(input.Options, input.Chosen) {
			return Refuse("The chosen option must be one of the options: " + strings.Join(input.Options, ", "))
		}
		return Ready(func(context.Context) Outcome {
			return Outcome{
				Display: &protocol.ToolDisplay{Value: protocol.ToolDisplayDecision{
					Chosen: input.Chosen, Options: input.Options, Question: input.Question, Why: input.Why,
				}},
				Meta: input.Chosen, OK: true, Output: "Recorded:\n" + decisionText(input),
			}
		}, nil)
	},
	Description: "Records a choice that shapes the work before you act on it: the question, the options you weighed, the one you chose, and why. Use it for choices the user did not make — a framework, a library, a data model, a protocol, a structure — not for each step of the plan. It shows on the session’s decision map, where the user can revisit it and replay from there.",
	Input: Object(
		Required("question", String().Trimmed().Min(1).Max(maxDecisionQuestion).Describe("The choice, as a question: “Which framework should the app use?”.")),
		Required("options", Array(String().Trimmed().Min(1).Max(maxDecisionOptionText)).Min(minDecisionOptions).Max(maxDecisionOptions).Describe("The options you weighed, the one you chose among them.")),
		Required("chosen", String().Trimmed().Min(1).Max(maxDecisionOptionText).Describe("The option you chose, word for word as it is in the options.")),
		Optional("why", String().Trimmed().Max(maxDecisionWhy).Describe("Why this one, in a sentence or two: what the project or the request ruled out.")),
	),
	Name:    protocol.ToolNameDecision,
	Subject: reads[decisionInput],
	Summarize: func(input decisionInput) string {
		return input.Question + " → " + input.Chosen
	},
})
