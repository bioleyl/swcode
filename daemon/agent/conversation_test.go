package agent

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"swcode/daemon/llm"
	"swcode/daemon/protocol"
	"swcode/daemon/tools"
)

const at = "2026-10-23T10:00:00.000Z"

func events(variants ...protocol.SessionEventVariant) []protocol.SessionEvent {
	list := []protocol.SessionEvent{}
	for _, variant := range variants {
		list = append(list, protocol.SessionEvent{Value: protocol.Dated(variant, at)})
	}
	return list
}

func TestEachToolResultComesRightAfterTheMessageThatAskedForIt(t *testing.T) {
	read := protocol.ToolCall{Arguments: `{"path":"a"}`, ID: "c1", Name: "read"}
	got := ConversationOf(events(
		protocol.SessionEventTurnStarted{ID: "t1"},
		protocol.SessionEventUserMessage{ID: "u1", Text: "Read it"},
		protocol.SessionEventAssistantMessage{ID: "a1", ToolCalls: []protocol.ToolCall{read}},
		protocol.SessionEventToolCall{CallID: "c1", Summary: "a", Tool: protocol.ToolNameRead},
		protocol.SessionEventToolFinished{CallID: "c1", OK: true, Output: "content of a"},
		protocol.SessionEventAssistantMessage{ID: "a2", Text: "Done"},
		protocol.SessionEventTurnFinished{ID: "t1", Outcome: protocol.TurnOutcomeDone},
	), nil)
	want := []llm.Message{
		{Content: "[" + LocalTime(at, nil) + "]\nRead it", Role: llm.RoleUser},
		{Role: llm.RoleAssistant, ToolCalls: []protocol.ToolCall{read}},
		{Content: "content of a", Role: llm.RoleTool, ToolCallID: "c1"},
		{Content: "Done", Role: llm.RoleAssistant},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestTheThoughtsBeforeEachReplyAreKeptAndAnEmptyReplyLeftOut(t *testing.T) {
	got := ConversationOf(events(
		protocol.SessionEventUserMessage{ID: "u1", Text: "Hi"},
		protocol.SessionEventAssistantMessage{ID: "a1", Reasoning: "Let me think."},
		protocol.SessionEventAssistantMessage{ID: "a2", Reasoning: "A greeting.", Text: "Hello"},
	), nil)
	if len(got) != 2 || got[1].Reasoning != "A greeting." || got[1].Content != "Hello" {
		t.Fatalf("got %+v", got)
	}
}

func TestACallThatNeverRanGetsAResult(t *testing.T) {
	got := ConversationOf(events(
		protocol.SessionEventAssistantMessage{ID: "a1", ToolCalls: []protocol.ToolCall{{Arguments: "{}", ID: "c1", Name: "bash"}, {Arguments: "{}", ID: "c2", Name: "bash"}}},
		protocol.SessionEventToolFinished{CallID: "c1", Output: "declined"},
	), nil)
	want := []llm.Message{
		{Content: "declined", Role: llm.RoleTool, ToolCallID: "c1"},
		{Content: "Not run: the turn stopped before this call.", Role: llm.RoleTool, ToolCallID: "c2"},
	}
	if !reflect.DeepEqual(got[1:], want) {
		t.Fatalf("got %+v", got[1:])
	}
}

func TestLocalTimeReadsLikeAClockInTheTimeZoneNamed(t *testing.T) {
	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Skip("no time zone database")
	}
	if got := LocalTime("2026-10-24T21:05:00.000Z", zurich); got != "Sat 2026-10-24 23:05" {
		t.Fatalf("got %q", got)
	}
	if got := LocalTime("2026-10-24T21:05:00.000Z", time.UTC); got != "Sat 2026-10-24 21:05" {
		t.Fatalf("got %q", got)
	}
}

func shotEvents() []protocol.SessionEvent {
	picture := protocol.PictureRef{File: "shot.png", Height: 900, Type: protocol.PictureTypeImagePng, Width: 1440}
	return events(
		protocol.SessionEventUserMessage{ID: "u1", Text: "Check the page"},
		protocol.SessionEventAssistantMessage{ID: "a1", ToolCalls: []protocol.ToolCall{{Arguments: `{"see":true}`, ID: "c1", Name: "screenshot"}}},
		protocol.SessionEventToolFinished{CallID: "c1", OK: true, Output: "Screenshot of …", Pictures: []protocol.PictureRef{picture}},
	)
}

func TestScreenshotsTheModelAskedToSeeFollowTheResultsAsPictures(t *testing.T) {
	got := ConversationOf(shotEvents(), &PictureView{Folder: "/data/images/s1", Seen: true})
	want := []llm.Message{
		{Content: "Screenshot of …", Role: llm.RoleTool, ToolCallID: "c1"},
		{Content: "[The screenshots you asked to see, in the order of your calls.]", Pictures: []llm.Picture{{Path: "/data/images/s1/shot.png", Type: "image/png"}}, Role: llm.RoleUser},
	}
	if !reflect.DeepEqual(got[2:], want) {
		t.Fatalf("got %+v", got[2:])
	}
	unseen := ConversationOf(shotEvents(), &PictureView{Folder: "/data/images/s1"})
	last := unseen[len(unseen)-1]
	if !strings.Contains(last.Content, "cannot see now") || last.Pictures != nil {
		t.Fatalf("got %+v", last)
	}
}

func TestTwoUserMessagesInARowBecomeOne(t *testing.T) {
	got := ConversationOf(events(
		protocol.SessionEventUserMessage{ID: "u1", Text: "Deploy it"},
		protocol.SessionEventSkillsListed{ID: "s1", Skills: []protocol.SkillSummary{{Description: "Deploys.", Name: "deploy"}}},
	), nil)
	if len(got) != 1 || !strings.HasSuffix(got[0].Content, "Deploy it\n\n"+SkillsText([]protocol.SkillSummary{{Description: "Deploys.", Name: "deploy"}})) {
		t.Fatalf("got %+v", got)
	}
}

func TestTheBudgetCountsFromTheLastMeasureOn(t *testing.T) {
	list := events(
		protocol.SessionEventUserMessage{ID: "u1", Text: "Hi"},
		protocol.SessionEventAssistantMessage{ID: "a1", Text: "Hello", Usage: &protocol.Usage{CompletionTokens: 10, PromptTokens: 90}},
		protocol.SessionEventToolFinished{CallID: "c1", Output: strings.Repeat("x", 30)},
	)
	if got := EstimatedTokens(list, 3000); got != 110 {
		t.Fatalf("got %d", got)
	}
	if !NeedsCompaction(list, 3000, 130) || NeedsCompaction(list, 3000, 0) {
		t.Fatal("compaction")
	}
}

func TestThePlanReminderComesOnlyWhenTheModelActedSinceTheLastOne(t *testing.T) {
	plan := protocol.ToolDisplay{Value: protocol.ToolDisplayPlan{Steps: []protocol.PlanStep{
		{Content: "Fix it", Status: protocol.PlanStepStatusCompleted}, {Content: "Test it", Status: protocol.PlanStepStatusPending},
	}}}
	list := events(
		protocol.SessionEventTurnStarted{ID: "t1"},
		protocol.SessionEventToolCall{CallID: "c1", Summary: "Fix it", Tool: protocol.ToolNamePlan},
		protocol.SessionEventToolFinished{CallID: "c1", Display: &plan, OK: true},
	)
	reminder := PlanReminder(list)
	if !strings.Contains(reminder, "- Test it") || strings.Contains(reminder, "- Fix it") {
		t.Fatalf("got %q", reminder)
	}
	list = append(list, events(protocol.SessionEventReminder{ID: "r1", Reason: protocol.ReminderReasonPlan, Text: reminder})...)
	if PlanReminder(list) != "" {
		t.Fatal("reminded twice without anything done")
	}
}

func TestATranscriptDropsTheOldestLinesFirst(t *testing.T) {
	list := events(
		protocol.SessionEventUserMessage{ID: "u1", Text: "first"},
		protocol.SessionEventUserMessage{ID: "u2", Text: "second"},
	)
	if got := TranscriptOf(list, 15); got != "User: second" {
		t.Fatalf("got %q", got)
	}
}

func TestASummaryIsAskedAtTheEndOfTheConversationOnlyWhileItFitsWithWhatItTakes(t *testing.T) {
	events := []protocol.SessionEvent{
		{Value: protocol.SessionEventUserMessage{ID: "u", Text: "Read it"}},
		{Value: protocol.SessionEventAssistantMessage{ID: "a", Text: "Done.", Usage: &protocol.Usage{CompletionTokens: 20, PromptTokens: 9980}}},
	}
	fits := map[int]bool{10_000 + summaryRoomTokens: true, 10_000 + summaryRoomTokens - 1: false, 0: true}
	for contextTokens, expected := range fits {
		if got := hasRoomForSummary(events, Setup{ContextTokens: contextTokens}); got != expected {
			t.Fatalf("a context of %d tokens: room %v", contextTokens, got)
		}
	}
}

func TestARequestAtTheEndOfAConversationJoinsTheUserMessageThatEndsIt(t *testing.T) {
	ended := withRequest([]llm.Message{{Content: "Hi", Role: llm.RoleUser}}, "Summarize.")
	if len(ended) != 1 || ended[0].Content != "Hi\n\nSummarize." {
		t.Fatalf("after a user message: %+v", ended)
	}
	answered := withRequest([]llm.Message{{Content: "Hi", Role: llm.RoleUser}, {Content: "Hello.", Role: llm.RoleAssistant}}, "Summarize.")
	if len(answered) != 3 || answered[2].Role != llm.RoleUser || answered[2].Content != "Summarize." {
		t.Fatalf("after an answer: %+v", answered)
	}
	if empty := withRequest(nil, "Summarize."); len(empty) != 1 {
		t.Fatalf("an empty conversation: %+v", empty)
	}
}

func TestOnlyTheReadsThatFollowOneAnotherAtTheStartRunTogether(t *testing.T) {
	named := func(names ...string) []protocol.ToolCall {
		calls := []protocol.ToolCall{}
		for _, name := range names {
			calls = append(calls, protocol.ToolCall{Name: name})
		}
		return calls
	}
	counts := map[int][]protocol.ToolCall{
		3: named("read", "grep", "webfetch", "edit", "read"),
		0: named("bash", "read", "read"),
		1: named("glob"),
	}
	for expected, calls := range counts {
		if got := readsAtStart(calls, tools.Main); got != expected {
			t.Fatalf("%d of %v", got, calls)
		}
	}
	// A tool the agent does not have runs with nothing: its refusal is the call's own.
	if got := readsAtStart(named("websearch", "read"), tools.Main.Without(protocol.ToolNameWebsearch)); got != 0 {
		t.Fatalf("%d with a tool the agent lacks", got)
	}
}
