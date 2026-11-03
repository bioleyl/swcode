package sessions

import (
	"encoding/json"
	"fmt"
	"strings"

	"swcode/daemon/agent"
	"swcode/daemon/protocol"
)

const (
	titleLength = 60
	ellipsis    = "…"
	// Untitled is a session's title until its first message names it.
	Untitled         = "New session"
	messageSeparator = "\n\n"
)

// TitleFrom is a session's title: the first line of its first message, cut to fit a list.
func TitleFrom(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	line = strings.TrimSpace(line)
	if line == "" {
		return Untitled
	}
	runes := []rune(line)
	if len(runes) <= titleLength {
		return line
	}
	return string(runes[:titleLength-1]) + ellipsis
}

// unfinishedTurn is the turn that started and never finished, when the daemon stopped in the middle of it.
func unfinishedTurn(events []protocol.SessionEvent) string {
	finished := map[string]bool{}
	started := ""
	for _, event := range events {
		switch value := event.Value.(type) {
		case protocol.SessionEventTurnFinished:
			finished[value.ID] = true
		case protocol.SessionEventTurnStarted:
			started = value.ID
		}
	}
	if started == "" || finished[started] {
		return ""
	}
	return started
}

// lastActivity is when a session last did something: its last event, or its making.
func lastActivity(meta Meta, events []protocol.SessionEvent) string {
	if len(events) == 0 {
		return meta.CreatedAt
	}
	data, _ := events[len(events)-1].MarshalJSON()
	var dated struct {
		At string `json:"at"`
	}
	_ = json.Unmarshal(data, &dated)
	if dated.At == "" {
		return meta.CreatedAt
	}
	return dated.At
}

// joinMessages makes messages sent while a turn ran one: their words one after the other, all their pictures.
func joinMessages(messages []agent.PromptMessage) agent.PromptMessage {
	joined := agent.PromptMessage{}
	texts := []string{}
	for _, message := range messages {
		joined.Pictures = append(joined.Pictures, message.Pictures...)
		joined.Concise = joined.Concise || message.Concise
		if message.Text != "" {
			texts = append(texts, message.Text)
		}
	}
	joined.Text = strings.Join(texts, messageSeparator)
	return joined
}

// queuedText is a waiting message as the app lists it: its words, and how many pictures go with them.
func queuedText(message agent.PromptMessage) string {
	parts := []string{}
	if message.Text != "" {
		parts = append(parts, message.Text)
	}
	switch count := len(message.Pictures); {
	case count == 1:
		parts = append(parts, "[1 picture]")
	case count > 1:
		parts = append(parts, fmt.Sprintf("[%d pictures]", count))
	}
	return strings.Join(parts, " ")
}
