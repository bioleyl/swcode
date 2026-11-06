package agent

import (
	"strings"
	"testing"

	"swcode/daemon/protocol"
)

func TestThePromptSaysOfSubagentsWhatThePresetAllows(t *testing.T) {
	prompt := func(limit *protocol.SubagentLimit) string {
		return SystemPrompt("/w", "/s", "linux", Servers{}, limit)
	}
	if !strings.Contains(prompt(nil), subagentLine) || strings.Contains(prompt(nil), "work at once:") {
		t.Fatal("no limit")
	}
	if strings.Contains(prompt(&protocol.SubagentLimit{AtOnce: 0}), "subagent") {
		t.Fatal("none allowed, yet told of them")
	}
	if !strings.Contains(prompt(&protocol.SubagentLimit{AtOnce: 2}), "2 work at once: swcode starts the others by itself as those report.") {
		t.Fatal("the limit is not said")
	}
}

func TestAChatDrawsOnlyWhenThereIsAPictureServer(t *testing.T) {
	if strings.Contains(ChatPrompt(false), "draw") || !strings.Contains(ChatPrompt(true), chatDrawLine) {
		t.Fatal("the chat's prompt tells of draw without a picture server, or not with one")
	}
}
