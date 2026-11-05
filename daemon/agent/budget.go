package agent

import (
	"encoding/json"
	"unicode/utf8"

	"swcode/daemon/protocol"
)

const (
	// CharsPerToken: code runs about 3 characters a token, prose about 4, and counting 3 errs on the side of
	// compacting early.
	CharsPerToken = 3
	// compactShare: compacted once the conversation fills four fifths of a slot, the reply needs room too.
	compactShare = 0.8
	// pictureTilePx: a Qwen vision encoder takes a token for each 32 × 32 pixels of a picture.
	pictureTilePx = 32
)

func chars(text string) int { return utf8.RuneCountInString(text) }

func ceilDiv(value, by int) int { return (value + by - 1) / by }

func picturesChars(pictures []protocol.PictureRef) int {
	tokens := 0
	for _, picture := range pictures {
		tokens += ceilDiv(picture.Width, pictureTilePx) * ceilDiv(picture.Height, pictureTilePx)
	}
	return tokens * CharsPerToken
}

func charsOf(event protocol.SessionEvent) int {
	switch value := event.Value.(type) {
	case protocol.SessionEventUserMessage:
		return chars(value.Text) + picturesChars(value.Pictures)
	case protocol.SessionEventAssistantMessage:
		total := chars(value.Text) + chars(value.Reasoning)
		for _, call := range value.ToolCalls {
			total += chars(call.Name) + chars(call.Arguments)
		}
		return total
	case protocol.SessionEventToolFinished:
		return chars(value.Output) + picturesChars(value.Pictures)
	case protocol.SessionEventReminder:
		return chars(value.Text)
	case protocol.SessionEventSkillsListed:
		// About as long as the lines the model reads.
		data, _ := json.Marshal(value.Skills)
		return chars(string(data))
	case protocol.SessionEventContextCompacted:
		return chars(value.Summary)
	}
	return 0
}

// SinceCompaction is the events the model still sees: from the last compaction on, the compaction itself included.
func SinceCompaction(events []protocol.SessionEvent) []protocol.SessionEvent {
	for index := len(events) - 1; index >= 0; index-- {
		if _, compacted := events[index].Value.(protocol.SessionEventContextCompacted); compacted {
			return events[index:]
		}
	}
	return events
}

func measuredTokens(event protocol.SessionEvent) (int, bool) {
	message, ok := event.Value.(protocol.SessionEventAssistantMessage)
	if !ok || message.Usage == nil {
		return 0, false
	}
	return message.Usage.PromptTokens + message.Usage.CompletionTokens, true
}

// EstimatedTokens is how many tokens the next request takes: the last reply's count, as llm-server gave it, plus
// a guess for what came after it (tool results, a new message). fixedChars is what every request carries: the
// system prompt and the tools.
func EstimatedTokens(events []protocol.SessionEvent, fixedChars int) int {
	recent := SinceCompaction(events)
	base, last := ceilDiv(fixedChars, CharsPerToken), -1
	for index := len(recent) - 1; index >= 0; index-- {
		if tokens, ok := measuredTokens(recent[index]); ok {
			base, last = tokens, index
			break
		}
	}
	after := 0
	for _, event := range recent[last+1:] {
		after += charsOf(event)
	}
	return base + ceilDiv(after, CharsPerToken)
}

// NeedsCompaction tells whether the conversation should be compacted before the next request; never when the
// context is unknown.
func NeedsCompaction(events []protocol.SessionEvent, fixedChars, contextTokens int) bool {
	return contextTokens > 0 && float64(EstimatedTokens(events, fixedChars)) > float64(contextTokens)*compactShare
}
