package agent

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"swcode/daemon/llm"
	"swcode/daemon/protocol"
)

const (
	notRun        = "Not run: the turn stopped before this call."
	summaryHeader = "The conversation so far was summarized to save context. The summary:"
	// reportHeader opens a subagent's report, which comes as a message once the subagent is done.
	reportHeader  = "The report of your subagent “%s”:"
	userSeparator = "\n\n"
	// shownNote: the screenshots a model asked to see follow the results of its calls, a tool result is text only.
	shownNote = "[The screenshots you asked to see, in the order of your calls.]"
	// conciseNote follows the words of a message sent in token economy.
	conciseNote = "Be concise."
	// timeLayout is when a message was sent, as the person reads a clock.
	timeLayout = "Mon 2006-01-02 15:04"
)

// IsEmptyReply tells a reply with neither text nor a tool call: the model ended its turn inside its thoughts. The
// turn asks again, and the model never sees it, so the next request is the same one, its prompt already cached.
func IsEmptyReply(text string, calls []protocol.ToolCall) bool {
	return text == "" && len(calls) == 0
}

// LocalTime is when a message was sent, as the person reads a clock: "Sat 2026-10-24 23:05", in the daemon's time
// zone unless another is given. The model knows no date of its own, and the system prompt carries none, to stay
// cached.
func LocalTime(iso string, zone *time.Location) string {
	at, err := protocol.ParseTimestamp(iso)
	if err != nil {
		return iso
	}
	if zone == nil {
		zone = time.Local
	}
	return at.In(zone).Format(timeLayout)
}

// callResult is what each call gave back: its output, and the pictures the model asked to see.
type callResult struct {
	output   string
	pictures []protocol.PictureRef
}

func resultsOf(events []protocol.SessionEvent) map[string]callResult {
	results := map[string]callResult{}
	for _, event := range events {
		if finished, ok := event.Value.(protocol.SessionEventToolFinished); ok {
			results[finished.CallID] = callResult{output: finished.Output, pictures: finished.Pictures}
		}
	}
	return results
}

func pictureFiles(folder string, pictures []protocol.PictureRef) []llm.Picture {
	files := make([]llm.Picture, len(pictures))
	for index, picture := range pictures {
		files[index] = llm.Picture{Path: filepath.Join(folder, picture.File), Type: string(picture.Type)}
	}
	return files
}

// plural is "a picture" for one, "3 pictures" for more.
func plural(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return strconv.Itoa(count) + " " + many
}

// shownMessage is the screenshots the calls of a reply showed, after their results: as pictures when the model
// sees them.
func shownMessage(shown []protocol.PictureRef, pictures *PictureView) llm.Message {
	if pictures == nil || !pictures.Seen {
		return llm.Message{Content: "[" + plural(len(shown), "A screenshot", "screenshots") + " you asked to see, which you cannot see now: your model is loaded without vision.]", Role: llm.RoleUser}
	}
	return llm.Message{Content: shownNote, Pictures: pictureFiles(pictures.Folder, shown), Role: llm.RoleUser}
}

// userMessageOf is the person's message: the pictures after its text when the model sees them, read from the
// session's folder as the request leaves; otherwise a word that they are there.
func userMessageOf(event protocol.SessionEventUserMessage, pictures *PictureView) llm.Message {
	// Each message carries its time, which never changes: the model knows the date, and the cache stays valid.
	content := "[" + LocalTime(event.At, nil) + "]\n" + event.Text
	if event.Concise {
		content += userSeparator + conciseNote
	}
	if len(event.Pictures) == 0 {
		return llm.Message{Content: content, Role: llm.RoleUser}
	}
	if pictures == nil || !pictures.Seen {
		note := "[The user pasted " + plural(len(event.Pictures), "a picture", "pictures") + " here, which you cannot see: your model was loaded without vision.]"
		return llm.Message{Content: content + userSeparator + note, Role: llm.RoleUser}
	}
	return llm.Message{Content: content, Pictures: pictureFiles(pictures.Folder, event.Pictures), Role: llm.RoleUser}
}

func messagesOf(event protocol.SessionEvent, results map[string]callResult, pictures *PictureView) []llm.Message {
	switch value := event.Value.(type) {
	case protocol.SessionEventUserMessage:
		return []llm.Message{userMessageOf(value, pictures)}
	case protocol.SessionEventContextCompacted:
		return []llm.Message{{Content: summaryHeader + "\n\n" + value.Summary, Role: llm.RoleUser}}
	case protocol.SessionEventReminder:
		return []llm.Message{{Content: value.Text, Role: llm.RoleUser}}
	case protocol.SessionEventSkillsListed:
		return []llm.Message{{Content: SkillsText(value.Skills), Role: llm.RoleUser}}
	case protocol.SessionEventSkillLoaded:
		return []llm.Message{{Content: LoadedSkillText(value), Role: llm.RoleUser}}
	case protocol.SessionEventSubagentReport:
		return []llm.Message{{Content: reportOf(value), Role: llm.RoleUser}}
	case protocol.SessionEventAssistantMessage:
		return replyMessages(value, results, pictures)
	}
	return nil
}

// reportOf is a subagent's report as the main agent reads it: whose it is, what it says, then what is still to come.
func reportOf(report protocol.SessionEventSubagentReport) string {
	parts := []string{fmt.Sprintf(reportHeader, report.Description), report.Text}
	if report.Note != "" {
		parts = append(parts, report.Note)
	}
	return strings.Join(parts, userSeparator)
}

// replyMessages is a reply, then each of its calls' results, then the screenshots they showed.
func replyMessages(reply protocol.SessionEventAssistantMessage, results map[string]callResult, pictures *PictureView) []llm.Message {
	if IsEmptyReply(reply.Text, reply.ToolCalls) {
		return nil
	}
	messages := []llm.Message{{
		Content: reply.Text, Items: reply.ResponseItems, Reasoning: reply.Reasoning, Role: llm.RoleAssistant,
		ToolCalls: reply.ToolCalls,
	}}
	shown := []protocol.PictureRef{}
	for _, call := range reply.ToolCalls {
		result, ran := results[call.ID]
		output := notRun
		if ran {
			output = result.output
			shown = append(shown, result.pictures...)
		}
		messages = append(messages, llm.Message{Content: output, Role: llm.RoleTool, ToolCallID: call.ID})
	}
	if len(shown) > 0 {
		messages = append(messages, shownMessage(shown, pictures))
	}
	return messages
}

// joinUserMessages makes two user messages in a row one: the skills listed after the person's message, say. Some
// chat templates want the roles to alternate.
func joinUserMessages(messages []llm.Message) []llm.Message {
	joined := []llm.Message{}
	for _, message := range messages {
		last := len(joined) - 1
		if last >= 0 && joined[last].Role == llm.RoleUser && message.Role == llm.RoleUser {
			joined[last].Content += userSeparator + message.Content
			joined[last].Pictures = append(joined[last].Pictures, message.Pictures...)
			continue
		}
		joined = append(joined, message)
	}
	return joined
}

// ConversationOf is the model's view of a session, rebuilt from its events: from the last compaction on, its
// summary first. Each tool result comes right after the message that asked for it, and a call that never ran gets a
// result too, so the conversation is always one the API accepts. Each reply keeps the thoughts before it: Qwen's
// templates show them (preserved thinking), so the model goes on from its plan instead of working it out again at
// every step, and the prompt matches what llama-server cached while the reply was written.
func ConversationOf(events []protocol.SessionEvent, pictures *PictureView) []llm.Message {
	recent := SinceCompaction(events)
	results := resultsOf(recent)
	messages := []llm.Message{}
	for _, event := range recent {
		messages = append(messages, messagesOf(event, results, pictures)...)
	}
	return joinUserMessages(messages)
}
