package agent

import (
	"context"
	"errors"
	"strings"

	"swcode/daemon/llm"
	"swcode/daemon/protocol"
)

const (
	// peekRequest frames the person's question beside the work: the agent answers in words, from what it knows
	// now, and its turn goes on as it was.
	peekRequest = "A quick question from the person, beside your work: answer it in a few sentences, from what you know now, without calling any tool. Your work then goes on as it was.\n\n"
	// stillRunning is a call of the turn running whose result has not come yet.
	stillRunning = "Still running: its result comes once it ends."
	noPeekAnswer = "No answer came: the agent wanted to call a tool. Ask again in other words."
	peekStopped  = "Stopped before it answered."
)

// Peek is a question asked beside the session (/peek): answered from the conversation as it is, never kept in it.
type Peek struct {
	Question string
	// At is when it was asked, as the session dates its events: the model reads it before the question.
	At string
	// Text streams the answer as it comes; Done says it ended, with why when it failed.
	Text func(text string)
	Done func(problem string)
}

// AnswerPeek answers a peek with the agent's own request, its system prompt, its tools and its slot, the question
// after the conversation: llm-server goes on from the prompt it cached, and only the question is new. A call of the
// turn running whose result has not come says so, and the agent's next request takes up again from there.
func AnswerPeek(ctx context.Context, history []protocol.SessionEvent, setup Setup, peek Peek) {
	events := append(append([]protocol.SessionEvent{}, history...), stillRunningResults(history)...)
	question := protocol.SessionEventUserMessage{ID: setup.NewID(), Text: peekRequest + peek.Question}
	events = append(events, protocol.SessionEvent{Value: protocol.Dated(question, peek.At)})
	messages := append([]llm.Message{{Content: setup.SystemPrompt, Role: llm.RoleSystem}}, ConversationOf(events, setup.Pictures)...)
	reply, err := setup.Model.Complete(ctx, llm.Request{
		Messages: messages,
		Target:   llm.Target{Identity: setup.Agent, Slot: llm.NoSlot},
		Tools:    setup.Tools.Definitions(),
	}, func(part llm.Part) {
		if part.Kind == llm.PartText {
			peek.Text(part.Text)
		}
	})
	peek.Done(peekProblem(ctx, reply, err))
}

// stillRunningResults stands in for the results of the turn running that have not come; a call an earlier turn
// left unrun keeps its own, as the agent's prompts have it.
func stillRunningResults(events []protocol.SessionEvent) []protocol.SessionEvent {
	start := lastTurnStart(events)
	if start < 0 {
		return nil
	}
	finished := map[string]bool{}
	calls := []string{}
	for _, event := range events[start:] {
		switch value := event.Value.(type) {
		case protocol.SessionEventTurnFinished:
			return nil
		case protocol.SessionEventToolFinished:
			finished[value.CallID] = true
		case protocol.SessionEventAssistantMessage:
			for _, call := range value.ToolCalls {
				calls = append(calls, call.ID)
			}
		}
	}
	results := []protocol.SessionEvent{}
	for _, id := range calls {
		if !finished[id] {
			results = append(results, protocol.SessionEvent{Value: protocol.SessionEventToolFinished{CallID: id, Output: stillRunning}})
		}
	}
	return results
}

// peekProblem is why a peek has no answer: stopped, refused by llm-server, or a call instead of words.
func peekProblem(ctx context.Context, reply llm.Reply, err error) string {
	if ctx.Err() != nil {
		return peekStopped
	}
	var model *llm.ModelError
	if errors.As(err, &model) {
		return model.Message
	}
	if err != nil {
		return "Something went wrong: " + err.Error()
	}
	if strings.TrimSpace(reply.Text) == "" {
		return noPeekAnswer
	}
	return ""
}
