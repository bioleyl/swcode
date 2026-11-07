package agent

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"swcode/daemon/llm"
	"swcode/daemon/protocol"
)

const (
	// Each message of the transcript is cut to this: the summary needs the gist, not whole files.
	messageChars = 2000
	callChars    = 300
	// transcriptShare: the transcript takes half the context, leaving room for the instructions and the summary.
	transcriptShare = 0.5
	// unknownContextTokens: when llm-server does not say how big the context is, the transcript stays small
	// enough for any model.
	unknownContextTokens = 8192
	noSummary            = "The earlier conversation could not be summarized."
	compactorPrompt      = "You summarize the conversation of a coding agent with its user, so the agent can go on with less context."
)

var compactionInstructions = strings.Join([]string{
	"Summarize the conversation below so the agent can go on with the task. Write, in this order:",
	"1. The user's latest request, quoted as they wrote it.",
	"2. What the user wants overall, and the decisions made so far.",
	"3. The files read and changed, with their paths and what changed.",
	"4. The commands run and what they showed.",
	"5. Where things stand, and what remains to do.",
	"6. The plan, when the agent wrote one with the plan tool: its latest list, each step with its status.",
	"Keep paths, names and code identifiers exact. Stay under 400 words.",
	"",
	"The conversation:",
}, "\n")

func clip(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

func lineOf(event protocol.SessionEvent) (string, bool) {
	switch value := event.Value.(type) {
	case protocol.SessionEventUserMessage:
		// The summary is text: it keeps that pictures were there, the agent's replies say what they showed.
		pictures := ""
		if len(value.Pictures) > 0 {
			pictures = fmt.Sprintf(" [%d pasted pictures]", len(value.Pictures))
		}
		return "User: " + clip(value.Text, messageChars) + pictures, true
	case protocol.SessionEventAssistantMessage:
		lines := []string{"Agent: " + clip(value.Text, messageChars)}
		for _, call := range value.ToolCalls {
			lines = append(lines, "[called "+call.Name+" "+clip(call.Arguments, callChars)+"]")
		}
		return strings.Join(lines, "\n"), true
	case protocol.SessionEventToolFinished:
		return "Result: " + clip(value.Output, messageChars), true
	case protocol.SessionEventReminder:
		return "swcode: " + value.Text, true
	case protocol.SessionEventSkillLoaded:
		return "swcode: the user ran the skill " + value.Name + ": " + clip(value.Text, messageChars), true
	case protocol.SessionEventContextCompacted:
		return "Summary of what came before: " + value.Summary, true
	}
	return "", false
}

// TranscriptOf is the conversation as text for the summary, the oldest lines dropped first when it is longer than
// the budget.
func TranscriptOf(events []protocol.SessionEvent, budgetChars int) string {
	lines := []string{}
	for _, event := range SinceCompaction(events) {
		if line, ok := lineOf(event); ok {
			lines = append(lines, line)
		}
	}
	kept := []string{}
	length := 0
	for index := len(lines) - 1; index >= 0; index-- {
		next := chars(lines[index])
		if len(kept) > 0 {
			next += len(userSeparator)
		}
		if length+next > budgetChars {
			break
		}
		kept = append([]string{lines[index]}, kept...)
		length += next
	}
	return strings.Join(kept, userSeparator)
}

// Compact replaces the conversation so far with a summary written by the agent's own model; the turn goes on from
// it.
func Compact(ctx context.Context, host TurnHost, setup Setup) error {
	contextTokens := setup.ContextTokens
	if contextTokens <= 0 {
		contextTokens = unknownContextTokens
	}
	transcript := TranscriptOf(host.History(), int(float64(contextTokens)*transcriptShare*CharsPerToken))
	reply, err := setup.Model.Complete(ctx, llm.Request{
		Messages: []llm.Message{
			{Content: compactorPrompt, Role: llm.RoleSystem},
			{Content: compactionInstructions + "\n\n" + transcript, Role: llm.RoleUser},
		},
		Target: llm.Target{Identity: setup.Agent, Slot: llm.NoSlot},
	}, func(llm.Part) {})
	if err != nil {
		return err
	}
	summary := reply.Text
	if summary == "" {
		summary = noSummary
	}
	host.Record(protocol.SessionEventContextCompacted{ID: setup.NewID(), Summary: summary})
	// The notes were read and the skills listed before the summary, which the model no longer sees.
	ReadNotes(ctx, host, setup)
	ListSkills(ctx, host, setup)
	return nil
}

// HasConversation tells whether there is anything to summarize: a message of the person's or of the model's since
// the last summary.
func HasConversation(events []protocol.SessionEvent) bool {
	return slices.ContainsFunc(SinceCompaction(events), func(event protocol.SessionEvent) bool {
		switch event.Value.(type) {
		case protocol.SessionEventUserMessage, protocol.SessionEventAssistantMessage:
			return true
		}
		return false
	})
}

// CompactTurn summarizes the conversation at the person's word (/compact), in a turn of its own, which shows it at
// work and can be stopped: the next turn starts from the summary.
func CompactTurn(ctx context.Context, host TurnHost, setup Setup) {
	turnID := setup.NewID()
	host.Record(protocol.SessionEventTurnStarted{ID: turnID})
	var err error
	// On the agent's slot, as its own compactions are: a peek waits for it.
	host.UseModel(func() { err = Compact(ctx, host, setup) })
	ending := compactionEnding(ctx, err)
	host.Record(protocol.SessionEventTurnFinished{ID: turnID, Message: ending.Message, Outcome: ending.Outcome})
}

func compactionEnding(ctx context.Context, err error) Ending {
	if err != nil {
		return endingOfError(ctx, err)
	}
	return Ending{Outcome: protocol.TurnOutcomeDone}
}
