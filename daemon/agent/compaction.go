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
	// summaryRoomTokens is what a summary asked at the end of the conversation takes after it: the request, the
	// model's thoughts, and the summary itself.
	summaryRoomTokens = 3000
)

// summaryParts is what a summary holds, in order, whichever way it is asked for.
var summaryParts = []string{
	"1. The user's latest request, quoted as they wrote it.",
	"2. What the user wants overall, and the decisions made so far.",
	"3. The files read and changed, with their paths and what changed.",
	"4. The commands run and what they showed.",
	"5. Where things stand, and what remains to do.",
	"6. The plan, when the agent wrote one with the plan tool: its latest list, each step with its status.",
	"Keep paths, names and code identifiers exact. Stay under 400 words.",
}

func instructions(first string, after ...string) string {
	return strings.Join(slices.Concat([]string{first}, summaryParts, after), "\n")
}

var (
	// compactionInstructions asks for the summary of a conversation given as a transcript, in a request of its own.
	compactionInstructions = instructions(
		"Summarize the conversation below so the agent can go on with the task. Write, in this order:", "", "The conversation:",
	)
	// continuedInstructions asks the agent itself, at the end of its conversation: the request is the one
	// llm-server cached plus these lines, so it only processes them.
	continuedInstructions = instructions(
		"Summarize the conversation so far, to go on with the task from your summary alone: it will stand in place of everything above. Call no tool now. Write, in this order:",
	)
)

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

// transcriptSummary is a summary of the conversation given as a transcript, each message cut short, in a request of
// its own: what still works once the conversation no longer fits the context. llm-server processes the whole
// transcript, and the agent's slot loses the conversation it had cached.
func transcriptSummary(ctx context.Context, host TurnHost, setup Setup) (string, error) {
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
	return reply.Text, err
}

// withRequest is a conversation with one more thing asked at its end, joined to a user message that ends it: some
// chat templates want the roles to alternate.
func withRequest(messages []llm.Message, text string) []llm.Message {
	last := len(messages) - 1
	if last < 0 || messages[last].Role != llm.RoleUser {
		return append(messages, llm.Message{Content: text, Role: llm.RoleUser})
	}
	asked := slices.Clone(messages)
	asked[last].Content += userSeparator + text
	return asked
}

// hasRoomForSummary tells whether a summary can be asked at the end of the conversation: it fits with what the
// summary takes after it. With a context of unknown size it is tried, and llm-server says when it does not fit.
func hasRoomForSummary(events []protocol.SessionEvent, setup Setup) bool {
	return setup.ContextTokens <= 0 || EstimatedTokens(events, fixedChars(setup))+summaryRoomTokens <= setup.ContextTokens
}

// continuedSummary is a summary the agent writes at the end of its own conversation: the request is the one
// llm-server cached, its tools listed still, with the request for a summary after it, so nothing but that request
// is processed and the model summarizes what it read whole. Empty when the model called a tool instead, or when
// there is no room for it.
func continuedSummary(ctx context.Context, host TurnHost, setup Setup) (string, error) {
	history := host.History()
	if !hasRoomForSummary(history, setup) {
		return "", nil
	}
	conversation := ConversationOf(history, setup.Pictures)
	reply, err := setup.Model.Complete(ctx, llm.Request{
		Messages: append([]llm.Message{{Content: setup.SystemPrompt, Role: llm.RoleSystem}}, withRequest(conversation, continuedInstructions)...),
		Target:   llm.Target{Identity: setup.Agent, Slot: llm.NoSlot},
		Tools:    setup.Tools.Definitions(),
	}, func(llm.Part) {})
	if llm.IsContextFull(err) {
		return "", nil
	}
	return reply.Text, err
}

// summaryOf is the summary of the conversation so far: asked at its end while it fits, from a transcript of it
// otherwise.
func summaryOf(ctx context.Context, host TurnHost, setup Setup) (string, error) {
	if summary, err := continuedSummary(ctx, host, setup); summary != "" || err != nil {
		return summary, err
	}
	return transcriptSummary(ctx, host, setup)
}

// compactWith replaces the conversation so far with a summary written by the agent's own model; the turn goes on
// from it.
func compactWith(ctx context.Context, host TurnHost, setup Setup, summarize func(context.Context, TurnHost, Setup) (string, error)) error {
	summary, err := summarize(ctx, host, setup)
	if err != nil {
		return err
	}
	if summary == "" {
		summary = noSummary
	}
	host.Record(protocol.SessionEventContextCompacted{ID: setup.NewID(), Summary: summary})
	// The notes were read and the skills listed before the summary, which the model no longer sees.
	ReadNotes(ctx, host, setup)
	ListSkills(ctx, host, setup)
	return nil
}

// Compact replaces the conversation so far with its summary, asked the cheapest way that fits.
func Compact(ctx context.Context, host TurnHost, setup Setup) error {
	return compactWith(ctx, host, setup, summaryOf)
}

// CompactFromTranscript replaces a conversation llm-server found too long for the context with its summary.
func CompactFromTranscript(ctx context.Context, host TurnHost, setup Setup) error {
	return compactWith(ctx, host, setup, transcriptSummary)
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
