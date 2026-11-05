package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"swcode/daemon/llm"
	"swcode/daemon/protocol"
	"swcode/daemon/tools"
)

const (
	declinedOutput   = "The user declined this call. Wait for their instructions."
	declinedMessage  = "Stopped: a call was declined. Say how to go on."
	rawSummaryLength = 80
	// answerRequest: a small model can end with its findings left in its thoughts, and is asked once to give them.
	answerRequest = "Your last reply was empty. Give your answer now, in a few lines, with file paths."
	// emptyReplyRetries: a thinking model can end its turn inside its thoughts (the Qwen3.8 Coder at low effort did
	// it once in about twenty at some steps). Sampled again, it goes on.
	emptyReplyRetries = 2
)

var stopped = Ending{Outcome: protocol.TurnOutcomeStopped}

func stepLimitRequest(steps int) string {
	return fmt.Sprintf("You have used all %d steps of this turn: stop here, without calling any more tools. Report what you did, what you measured, what worked and what did not, and which of your changes should be kept or undone.", steps)
}

func endingOfError(ctx context.Context, err error) Ending {
	if ctx.Err() != nil {
		return stopped
	}
	var model *llm.ModelError
	if errors.As(err, &model) {
		return Ending{Message: model.Message, Outcome: protocol.TurnOutcomeFailed}
	}
	return Ending{Message: "Something went wrong: " + err.Error(), Outcome: protocol.TurnOutcomeFailed}
}

func parseArguments(text string) (any, string) {
	if jsonBlank(text) {
		return map[string]any{}, ""
	}
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return nil, "The arguments are not valid JSON: " + err.Error()
	}
	return value, ""
}

func jsonBlank(text string) bool {
	for _, char := range text {
		if char != ' ' && char != '\n' && char != '\t' && char != '\r' {
			return false
		}
	}
	return true
}

// UserMessage is the person's message as the session keeps it: its pictures named, when it has some.
func UserMessage(id string, message PromptMessage) protocol.SessionEventUserMessage {
	return protocol.SessionEventUserMessage{Concise: message.Concise, ID: id, Pictures: message.Pictures, Text: message.Text}
}

func finished(callID string, outcome tools.Outcome) protocol.SessionEventToolFinished {
	return protocol.SessionEventToolFinished{
		CallID: callID, Display: outcome.Display, Meta: outcome.Meta, OK: outcome.OK, Output: outcome.Output, Pictures: outcome.Pictures,
	}
}

func toolName(name protocol.ToolName) protocol.ToolName { return name }

// recordRefusal records a call that cannot run: the model reads why, and the turn goes on so it can correct itself.
func recordRefusal(host TurnHost, call protocol.ToolCall, summary, problem string, tool protocol.ToolName) {
	host.Record(protocol.SessionEventToolCall{CallID: call.ID, Summary: summary, Tool: tool})
	host.Record(finished(call.ID, tools.Outcome{Meta: "refused", Output: problem}))
}

// runOutcome runs a prepared call; a tool that breaks fails its call, never the turn.
func runOutcome(ctx context.Context, call tools.PreparedCall) (outcome tools.Outcome) {
	defer func() {
		if broken := recover(); broken != nil {
			outcome = tools.Outcome{Output: fmt.Sprintf("The tool failed: %v", broken)}
		}
	}()
	return call.Run(ctx)
}

// execute asks a person first when the call needs one, then runs it: false when the person declined it.
func execute(ctx context.Context, callID string, call tools.PreparedCall, host TurnHost) bool {
	if host.NeedsPermission(call.Subject) {
		decision := host.AskPermission(ctx, PermissionAsk{CallID: callID, Preview: call.Preview, Subject: call.Subject})
		if decision == protocol.PermissionDecisionDeny {
			host.Record(finished(callID, tools.Outcome{Meta: "declined", Output: declinedOutput}))
			return false
		}
	}
	host.Record(protocol.SessionEventToolRunning{CallID: callID})
	host.Record(finished(callID, runOutcome(ctx, call)))
	return true
}

func runCall(ctx context.Context, call protocol.ToolCall, host TurnHost, setup Setup) bool {
	tool, found := setup.Tools.Find(call.Name)
	if !found {
		recordRefusal(host, call, call.Name, "There is no tool named "+call.Name+".", "")
		return true
	}
	input, problem := parseArguments(call.Arguments)
	if problem != "" {
		recordRefusal(host, call, clip(call.Arguments, rawSummaryLength), problem, tool.Name)
		return true
	}
	context := *host.ToolContext()
	context.CallID = call.ID
	prepared := tool.Prepare(ctx, input, &context)
	if !prepared.OK {
		recordRefusal(host, call, prepared.Summary, prepared.Problem, tool.Name)
		return true
	}
	host.Record(protocol.SessionEventToolCall{CallID: call.ID, Summary: prepared.Call.Summary, Tool: toolName(tool.Name)})
	return execute(ctx, call.ID, prepared.Call, host)
}

// runCalls runs the calls of one reply, one after the other, subagents too: on one GPU, subagents at the same time
// share its speed and end later than one after another. A declined call ends the turn, and the calls after it stay
// unrun: false then.
func runCalls(ctx context.Context, calls []protocol.ToolCall, host TurnHost, setup Setup) bool {
	for _, call := range calls {
		if ctx.Err() != nil {
			return true
		}
		if !runCall(ctx, call, host, setup) {
			return false
		}
	}
	return true
}

// fixedChars is what every request carries besides the conversation: the system prompt and the tools' definitions.
func fixedChars(setup Setup) int {
	data, _ := json.Marshal(setup.Tools.Definitions())
	return chars(setup.SystemPrompt) + chars(string(data))
}

// askModel asks the model for its next reply, compacting the conversation first when it is about to fill the
// context, and once more if llm-server still finds it too long.
func askModel(ctx context.Context, host TurnHost, setup Setup) (llm.Reply, error) {
	if NeedsCompaction(host.History(), fixedChars(setup), setup.ContextTokens) {
		if err := Compact(ctx, host, setup); err != nil {
			return llm.Reply{}, err
		}
	}
	reply, err := complete(ctx, host, setup)
	if err == nil || !llm.IsContextFull(err) || ctx.Err() != nil {
		return reply, err
	}
	if err := Compact(ctx, host, setup); err != nil {
		return llm.Reply{}, err
	}
	return complete(ctx, host, setup)
}

func complete(ctx context.Context, host TurnHost, setup Setup) (llm.Reply, error) {
	messageID := setup.NewID()
	messages := append([]llm.Message{{Content: setup.SystemPrompt, Role: llm.RoleSystem}}, ConversationOf(host.History(), setup.Pictures)...)
	reply, err := setup.Model.Complete(ctx, llm.Request{
		Messages: messages,
		Target:   llm.Target{Identity: setup.Agent, Slot: llm.NoSlot},
		Tools:    setup.Tools.Definitions(),
	}, func(part llm.Part) { streamPart(host, setup.Tools, messageID, part) })
	if err != nil {
		return llm.Reply{}, err
	}
	host.Record(protocol.SessionEventAssistantMessage{
		ID: messageID, Reasoning: reply.Reasoning, Text: reply.Text, ToolCalls: reply.ToolCalls, Usage: reply.Usage,
	})
	return reply, nil
}

// streamPart shows a piece of the reply as it comes: its text, its reasoning, or the tool it started calling,
// when the agent has that tool.
func streamPart(host TurnHost, box tools.Box, messageID string, part llm.Part) {
	switch part.Kind {
	case llm.PartReasoning:
		host.StreamReasoning(messageID, part.Text)
	case llm.PartText:
		host.StreamText(messageID, part.Text)
	case llm.PartToolCall:
		if tool, found := box.Find(part.Text); found {
			host.StreamToolCall(messageID, tool.Name)
		}
	}
}

// nextReply is the model's next reply, asked for again while it comes back empty.
func nextReply(ctx context.Context, host TurnHost, setup Setup) (llm.Reply, error) {
	for retries := emptyReplyRetries; ; retries-- {
		reply, err := askModelAlone(ctx, host, setup)
		if err != nil || !IsEmptyReply(reply.Text, reply.ToolCalls) || retries == 0 || ctx.Err() != nil {
			return reply, err
		}
	}
}

// askModelAlone asks the model with no peek in between: a peek's question would take the agent's slot from under
// its compaction or its request, and their prompts would no longer be the one llm-server cached.
func askModelAlone(ctx context.Context, host TurnHost, setup Setup) (reply llm.Reply, err error) {
	host.UseModel(func() { reply, err = askModel(ctx, host, setup) })
	return reply, err
}

// takeInMessages has what the person sent to come in now join the conversation between two steps, after the
// results of the last calls: the model reads it with its next request.
func takeInMessages(host TurnHost, setup Setup) bool {
	message := host.TakeMessages()
	if message == nil {
		return false
	}
	host.Record(UserMessage(setup.NewID(), *message))
	return true
}

func runSteps(ctx context.Context, host TurnHost, setup Setup) (Ending, error) {
	for step := 0; step < setup.MaxSteps; step++ {
		if ctx.Err() != nil {
			return stopped, nil
		}
		takeInMessages(host, setup)
		reply, err := nextReply(ctx, host, setup)
		if err != nil {
			return Ending{}, err
		}
		if len(reply.ToolCalls) > 0 {
			if !runCalls(ctx, reply.ToolCalls, host, setup) {
				return declinedEnding(ctx), nil
			}
			continue
		}
		// A message that came while the model wrote its answer gets an answer in this turn.
		if takeInMessages(host, setup) {
			continue
		}
		reminder := PlanReminder(host.History())
		if reminder == "" {
			return Ending{Outcome: protocol.TurnOutcomeDone}, nil
		}
		host.Record(protocol.SessionEventReminder{ID: setup.NewID(), Reason: protocol.ReminderReasonPlan, Text: reminder})
	}
	return reportAtLimit(ctx, host, setup)
}

func declinedEnding(ctx context.Context) Ending {
	if ctx.Err() != nil {
		return stopped
	}
	return Ending{Message: declinedMessage, Outcome: protocol.TurnOutcomeStopped}
}

// reportAtLimit has the model report at the step limit instead of stopping in silence with its changes left as
// they are. Its tools are still listed, so the prompt stays the one llm-server cached, but no call of this last
// reply runs.
func reportAtLimit(ctx context.Context, host TurnHost, setup Setup) (Ending, error) {
	host.Record(protocol.SessionEventReminder{ID: setup.NewID(), Reason: protocol.ReminderReasonStepLimit, Text: stepLimitRequest(setup.MaxSteps)})
	if _, err := nextReply(ctx, host, setup); err != nil {
		return Ending{}, err
	}
	return Ending{Message: fmt.Sprintf("Stopped after %d steps: the agent was asked for its report.", setup.MaxSteps), Outcome: protocol.TurnOutcomeLimit}, nil
}

// lastAnswer is what the agent said last, its tool calls aside.
func lastAnswer(events []protocol.SessionEvent) string {
	for index := len(events) - 1; index >= 0; index-- {
		if message, ok := events[index].Value.(protocol.SessionEventAssistantMessage); ok && message.Text != "" {
			return message.Text
		}
	}
	return ""
}

func steps(ctx context.Context, host TurnHost, setup Setup) Ending {
	ending, err := runSteps(ctx, host, setup)
	if err != nil {
		return endingOfError(ctx, err)
	}
	return ending
}

// RunTask runs a task without a person, for a subagent: the prompt, then steps until an answer, which comes back.
func RunTask(ctx context.Context, prompt string, host TurnHost, setup Setup) TaskResult {
	result := taskStep(ctx, prompt, host, setup)
	if result.Answer != "" || result.Ending.Outcome != protocol.TurnOutcomeDone {
		return result
	}
	return taskStep(ctx, answerRequest, host, setup)
}

func taskStep(ctx context.Context, text string, host TurnHost, setup Setup) TaskResult {
	host.Record(protocol.SessionEventUserMessage{ID: setup.NewID(), Text: text})
	ending := steps(ctx, host, setup)
	return TaskResult{Answer: lastAnswer(host.History()), Ending: ending}
}

// RunTurn runs one turn: the person's message, then the model and its tools in turns until it answers without a
// tool call.
func RunTurn(ctx context.Context, message PromptMessage, host TurnHost, setup Setup) {
	turnID := setup.NewID()
	host.Record(protocol.SessionEventTurnStarted{ID: turnID})
	host.Record(UserMessage(setup.NewID(), message))
	ListSkills(ctx, host, setup)
	ending := steps(ctx, host, setup)
	host.Record(protocol.SessionEventTurnFinished{ID: turnID, Message: ending.Message, Outcome: ending.Outcome})
}
