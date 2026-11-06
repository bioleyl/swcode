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
	if !strings.Contains(prompt(nil), subagentLine) || strings.Contains(prompt(nil), "at most") {
		t.Fatal("no limit")
	}
	if strings.Contains(prompt(&protocol.SubagentLimit{PerTurn: 0}), "subagent") {
		t.Fatal("none allowed, yet told of them")
	}
	if !strings.Contains(prompt(&protocol.SubagentLimit{PerTurn: 2}), "You may start 2 in a turn at most.") {
		t.Fatal("the limit is not said")
	}
}
