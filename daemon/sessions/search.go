package sessions

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"swcode/daemon/protocol"
)

const (
	// maxMatches is how many sessions a search lists: the palette shows a few, the newest.
	maxMatches = 20
	// snippetSide is how many letters of the text a snippet keeps on each side of the words found.
	snippetSide = 40
)

// Search is the sessions whose conversation says every word of the query, the newest first, each with the event
// that says them last and a line around the words in it. It reads the person's messages, the model's answers, the
// summaries of compacted conversations and the tool calls' few words (a file, a command), not the tools' results:
// they hold whole files and outputs. The sessions are in memory already, so a search takes no model and no disk.
func (s *Service) Search(query string) []protocol.SessionMatch {
	words := strings.Fields(lowered(query))
	matches := []protocol.SessionMatch{}
	if len(words) == 0 {
		return matches
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, summary := range s.list() {
		eventID, snippet, found := lastSaying(s.sessions[summary.ID].events, words)
		if !found {
			continue
		}
		matches = append(matches, protocol.SessionMatch{EventID: eventID, SessionID: summary.ID, Snippet: snippet})
		if len(matches) == maxMatches {
			break
		}
	}
	return matches
}

// lastSaying is the event that says the words last, and a line around them in it.
func lastSaying(events []protocol.SessionEvent, words []string) (string, string, bool) {
	for index := len(events) - 1; index >= 0; index-- {
		rowID, text := searched(events[index])
		if text == "" {
			continue
		}
		if snippet, found := snippetOf(text, words); found {
			return rowID, snippet, true
		}
	}
	return "", "", false
}

// searched is what a search reads of an event, and the id the app draws its row by: a tool call's row is its call's,
// the other events' their own. Nothing of the events that only say how a turn went.
func searched(event protocol.SessionEvent) (string, string) {
	switch value := event.Value.(type) {
	case protocol.SessionEventUserMessage:
		return value.ID, value.Text
	case protocol.SessionEventAssistantMessage:
		return value.ID, value.Text
	case protocol.SessionEventContextCompacted:
		return value.ID, value.Summary
	case protocol.SessionEventToolCall:
		return value.CallID, strings.TrimSpace(string(value.Tool) + " " + value.Summary)
	}
	return "", ""
}

// lowered is text in small letters, letter for letter, so that a place in it is the same place in the text.
func lowered(text string) string {
	return strings.Map(unicode.ToLower, text)
}

// snippetOf is the text around the first word, on one line, when the text says every word.
func snippetOf(text string, words []string) (string, bool) {
	small := lowered(text)
	for _, word := range words {
		if !strings.Contains(small, word) {
			return "", false
		}
	}
	runes := []rune(text)
	start := utf8.RuneCountInString(small[:strings.Index(small, words[0])])
	from := wordStart(runes, max(0, start-snippetSide))
	to := wordEnd(runes, min(len(runes), start+utf8.RuneCountInString(words[0])+snippetSide))
	snippet := strings.Join(strings.Fields(string(runes[from:to])), " ")
	if from > 0 {
		snippet = ellipsis + snippet
	}
	if to < len(runes) {
		snippet += ellipsis
	}
	return snippet, true
}

// wordStart is where the word at a place of the text starts, at most snippetSide letters before: a snippet starts on a
// whole word, unless the text has no space for that long.
func wordStart(runes []rune, at int) int {
	for start := at; start > max(0, at-snippetSide); start-- {
		if unicode.IsSpace(runes[start-1]) {
			return start
		}
	}
	return at
}

// wordEnd is where the word at a place of the text ends, at most snippetSide letters after.
func wordEnd(runes []rune, at int) int {
	for end := at; end < min(len(runes), at+snippetSide); end++ {
		if unicode.IsSpace(runes[end]) {
			return end
		}
	}
	return min(len(runes), at+snippetSide)
}
