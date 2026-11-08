package app_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

const subagentModel = "qwen3-coder-30b"

var subagentFiles = map[string]string{
	"src/session-store.ts": "export const save = () => writeFileSync(path, data);\n",
	"src/tools.ts":         "export const TOOLS = { read, write };\n",
}

func agentCall(description, prompt string) fakellama.ToolCall {
	return fakellama.ToolCall{Arguments: map[string]any{"description": description, "prompt": prompt}, Name: "agent"}
}

// thinking is how long a subagent's model takes over its first reply: the main agent's turn ends before any report.
const thinking = 300 * time.Millisecond

func slowly(reply fakellama.Reply) fakellama.Reply {
	reply.Delay = thinking
	return reply
}

// twoSubagents is a main agent that sends two explore subagents in one reply, which ends its turn; their reports wake
// it, and it answers from them.
func twoSubagents() Options {
	return Options{
		Files: subagentFiles,
		Replies: []fakellama.Reply{
			{ToolCalls: []fakellama.ToolCall{agentCall("Find the session store", "TASK-A: where are sessions saved?"), agentCall("Find the tools", "TASK-B: where are the tools listed?")}},
			text("Sessions are saved in src/session-store.ts, tools are listed in src/tools.ts."),
		},
		Scripts: map[string][]fakellama.Reply{
			"TASK-A": {slowly(call("grep", map[string]any{"pattern": "writeFileSync"})), text("src/session-store.ts:1 saves them with writeFileSync.")},
			"TASK-B": {slowly(call("glob", map[string]any{"pattern": "**/*.ts"})), text("src/tools.ts:1 lists them in TOOLS.")},
		},
		Slots:         4,
		SubagentModel: subagentModel,
	}
}

func runTwoSubagents(t *testing.T) (*App, string) {
	test := Start(t, twoSubagents())
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "Where are sessions saved, and where are the tools?", nil)
	test.WaitForEvent("turnFinished", 2)
	return test, session.ID
}

// kindsOf is the events of those kinds, in order.
func kindsOf(events []protocol.SessionEvent, kinds ...string) []string {
	found := []string{}
	for _, kind := range Kinds(events) {
		if slices.Contains(kinds, kind) {
			found = append(found, kind)
		}
	}
	return found
}

func summaryOf(test *App, sessionID string) protocol.SessionSummary {
	for _, session := range test.App.Sessions.List() {
		if session.ID == sessionID {
			return session
		}
	}
	return protocol.SessionSummary{}
}

func TestSubagentsWorkAtOnceInTheBackgroundAndTheirReportsWakeTheMainAgent(t *testing.T) {
	test, sessionID := runTwoSubagents(t)
	events := test.Events(sessionID)
	order := kindsOf(events, "subagentStarted", "subagentFinished", "turnFinished", "subagentReport", "turnStarted")
	want := []string{"turnStarted", "subagentStarted", "subagentStarted", "turnFinished", "subagentFinished", "subagentFinished", "turnStarted", "subagentReport", "subagentReport", "turnFinished"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order %v", order)
	}
	Expect(t, OfKind(events, "subagentStarted"), []any{
		map[string]any{"description": "Find the session store", "model": subagentModel},
		map[string]any{"description": "Find the tools", "model": subagentModel},
	})
	tools := []string{}
	for _, activity := range OfKind(events, "subagentActivity") {
		if activity.(map[string]any)["ok"] == true {
			tools = append(tools, activity.(map[string]any)["tool"].(string))
		}
	}
	if !slices.Contains(tools, "grep") || !slices.Contains(tools, "glob") {
		t.Fatalf("tools %v", tools)
	}
	for _, finished := range Finished(events) {
		if finished.Meta != "started" || !strings.HasPrefix(finished.Output, "Started the subagent “Find the") {
			t.Fatalf("an agent call's result %+v", finished)
		}
	}
	reports := map[string]string{}
	for _, report := range OfKind(events, "subagentReport") {
		fields := report.(map[string]any)
		reports[fields["description"].(string)] = fields["text"].(string)
	}
	if !reflect.DeepEqual(reports, map[string]string{"Find the session store": "src/session-store.ts:1 saves them with writeFileSync.", "Find the tools": "src/tools.ts:1 lists them in TOOLS."}) {
		t.Fatalf("reports %v", reports)
	}
	Expect(t, OfKind(events, "turnFinished")[0], `{"message":"Waiting for the subagents' reports: they wake the agent.","outcome":"done"}`)
	Expect(t, last(events), `{"outcome":"done"}`)
	Expect(t, OfKind(events, "assistantMessage")[1], `{"text":"Sessions are saved in src/session-store.ts, tools are listed in src/tools.ts."}`)
	if summaryOf(test, sessionID).Subagents != 0 {
		t.Fatal("subagents still counted at work")
	}
}

func TestSubagentsKeepTheirOwnWorkOutOfTheMainConversation(t *testing.T) {
	test, sessionID := runTwoSubagents(t)
	requests := test.Requests()
	lastMain := RequestText(map[string]any{"messages": requests[len(requests)-1]["messages"]})
	if !strings.Contains(lastMain, "The report of your subagent “Find the session store”:") || !strings.Contains(lastMain, "saves them with writeFileSync") || strings.Contains(lastMain, `"name":"grep"`) {
		t.Fatal("the reports did not reach the main conversation, or the subagents' work did")
	}
	Expect(t, OfKind(test.Events(sessionID), "toolCall"), `[{"tool":"agent"},{"tool":"agent"}]`)
}

func TestSubagentsEachAskForTheirSlotAndForTheSubagentModel(t *testing.T) {
	test, _ := runTwoSubagents(t)
	slotsOf := map[string]map[float64]bool{"TASK-A": {}, "TASK-B": {}}
	count := 0
	for _, request := range test.Requests() {
		if request["model"] != subagentModel {
			continue
		}
		count++
		for task, slots := range slotsOf {
			if strings.Contains(RequestText(request), task) {
				slots[request["id_slot"].(float64)] = true
			}
		}
	}
	if count != 4 || len(slotsOf["TASK-A"]) != 1 || len(slotsOf["TASK-B"]) != 1 || reflect.DeepEqual(slotsOf["TASK-A"], slotsOf["TASK-B"]) {
		t.Fatalf("%d requests, slots %v", count, slotsOf)
	}
	Expect(t, test.Requests()[0], `{"id_slot":0}`)
}

// oneSlowSubagent is a main agent that starts a subagent which takes its time, which ends its turn.
func oneSlowSubagent(delay time.Duration, replies ...fakellama.Reply) Options {
	return Options{
		Files: subagentFiles,
		Replies: append([]fakellama.Reply{
			{ToolCalls: []fakellama.ToolCall{agentCall("Find the tools", "TASK-E: where are the tools listed?")}},
		}, replies...),
		Scripts: map[string][]fakellama.Reply{"TASK-E": {{Delay: delay, Text: "src/tools.ts:1 lists them in TOOLS."}}},
	}
}

// TestASubagentStopsAtItsOwnStepLimitAndReports: the setting limits subagents apart from the main agent, which
// gets what the subagent found before it stopped.
func TestASubagentStopsAtItsOwnStepLimitAndReports(t *testing.T) {
	test := Start(t, Options{
		Files: subagentFiles,
		Replies: []fakellama.Reply{
			{ToolCalls: []fakellama.ToolCall{agentCall("Find the tools", "TASK-L: where are the tools listed?")}},
			text("It found src/tools.ts before its steps ran out."),
		},
		Scripts:  map[string][]fakellama.Reply{"TASK-L": {call("glob", map[string]any{"pattern": "**/*.ts"}), text("src/tools.ts, as far as I got.")}},
		Settings: map[string]string{"SWCODE_SUBAGENT_MAX_STEPS": "1"},
	})
	sessionID := prompt(t, test, "Where are the tools?")
	test.WaitForEvent("turnFinished", 2)
	events := test.Events(sessionID)
	Expect(t, OfKind(events, "subagentFinished"), `[{"outcome":"limit"}]`)
	Expect(t, OfKind(events, "subagentReport"), `[{"text":"src/tools.ts, as far as I got.\n[Stopped after 1 steps: the agent was asked for its report.]"}]`)
}

func TestThePersonTalksWithTheMainAgentWhileItsSubagentWorks(t *testing.T) {
	test := Start(t, oneSlowSubagent(time.Second, text("Hello! It is still looking."), text("It found them in src/tools.ts.")))
	sessionID := prompt(t, test, "Where are the tools?")
	test.WaitForEvent("turnFinished", 1)
	if summary := summaryOf(test, sessionID); summary.Subagents != 1 || summary.Status != protocol.SessionStatusIdle {
		t.Fatalf("summary %+v", summary)
	}
	test.App.Sessions.SendPrompt(sessionID, "Hi, how is it going?", nil)
	test.WaitForEvent("turnFinished", 2)
	if len(OfKind(test.Events(sessionID), "subagentFinished")) != 0 {
		t.Fatal("the subagent finished before the person's answer")
	}
	test.WaitForEvent("turnFinished", 3)
	order := kindsOf(test.Events(sessionID), "userMessage", "subagentFinished", "subagentReport", "turnFinished")
	if !reflect.DeepEqual(order, []string{"userMessage", "turnFinished", "userMessage", "turnFinished", "subagentFinished", "subagentReport", "turnFinished"}) {
		t.Fatalf("order %v", order)
	}
}

func TestStoppingASessionStopsItsSubagentsWhoseReportsWakeNobody(t *testing.T) {
	test := Start(t, oneSlowSubagent(time.Minute, text("It was stopped.")))
	sessionID := prompt(t, test, "Where are the tools?")
	test.WaitForEvent("turnFinished", 1)
	test.App.Sessions.Stop(sessionID)
	test.WaitForEvent("subagentFinished", 1)
	test.Until("no subagent at work", func() bool { return summaryOf(test, sessionID).Subagents == 0 })
	if summaryOf(test, sessionID).Status != protocol.SessionStatusIdle || len(OfKind(test.Events(sessionID), "turnStarted")) != 1 {
		t.Fatal("the stopped subagent's report woke the agent")
	}
	test.App.Sessions.SendPrompt(sessionID, "What happened?", nil)
	test.WaitForEvent("turnFinished", 2)
	order := kindsOf(test.Events(sessionID), "userMessage", "subagentReport")
	if !reflect.DeepEqual(order, []string{"userMessage", "userMessage", "subagentReport"}) {
		t.Fatalf("order %v", order)
	}
	Expect(t, OfKind(test.Events(sessionID), "subagentFinished"), `[{"outcome":"stopped"}]`)
}

func TestSubagentsCannotChangeFiles(t *testing.T) {
	test := Start(t, Options{
		Files:   subagentFiles,
		Replies: []fakellama.Reply{{ToolCalls: []fakellama.ToolCall{agentCall("Try to edit", "TASK-C: change something")}}, text("It could not.")},
		Scripts: map[string][]fakellama.Reply{"TASK-C": {slowly(call("bash", map[string]any{"command": "rm -rf src"})), text("I can only read.")}},
	})
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "Go", nil)
	test.WaitForEvent("turnFinished", 2)
	events := test.Events(session.ID)
	Expect(t, OfKind(events, "subagentActivity"), `[{"ok":false,"summary":"bash"}]`)
	if len(OfKind(events, "permissionRequested")) != 0 || test.ReadFile("src/tools.ts") != subagentFiles["src/tools.ts"] {
		t.Fatal("asked, or changed")
	}
}

func TestSubagentsAreAskedForTheirAnswerWhenTheyKeepEndingEmpty(t *testing.T) {
	answer := "src/tools.ts:1 lists them in TOOLS."
	empty := fakellama.Reply{Reasoning: answer}
	test := Start(t, Options{
		Files:         subagentFiles,
		Replies:       []fakellama.Reply{{ToolCalls: []fakellama.ToolCall{agentCall("Find the tools", "TASK-D: where are the tools listed?")}}, text("There.")},
		Scripts:       map[string][]fakellama.Reply{"TASK-D": {slowly(empty), empty, empty, {Text: answer}}},
		SubagentModel: subagentModel,
	})
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "Where are the tools?", nil)
	test.WaitForEvent("turnFinished", 2)
	Expect(t, OfKind(test.Events(session.ID), "subagentFinished"), []any{map[string]any{"outcome": "done", "report": answer}})
	asked := []map[string]any{}
	for _, request := range test.Requests() {
		if request["model"] == subagentModel {
			asked = append(asked, request)
		}
	}
	if len(asked) != 4 || !strings.Contains(RequestText(asked[3]), "Your last reply was empty.") {
		t.Fatalf("%d requests", len(asked))
	}
}

// limitTo has the default preset let that many subagents work at once.
func limitTo(t *testing.T, test *App, atOnce int) {
	t.Helper()
	settings := test.App.Models.Settings()
	settings.Presets[0].SubagentLimit = &protocol.SubagentLimit{AtOnce: atOnce}
	if status, body := test.Call("PUT", protocol.EndpointModels, settings); status != 200 {
		t.Fatalf("status %d: %v", status, body)
	}
}

func TestAPresetThatAllowsNoSubagentsTakesTheAgentToolAway(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("Done.")}})
	limitTo(t, test, 0)
	sessionID := prompt(t, test, "Hi")
	test.UntilIdle(sessionID)
	request := test.Requests()[0]
	for _, tool := range request["tools"].([]any) {
		if tool.(map[string]any)["function"].(map[string]any)["name"] == "agent" {
			t.Fatal("the agent tool is offered")
		}
	}
	if strings.Contains(RequestText(request), "explore subagent") {
		t.Fatal("the prompt tells of subagents")
	}
}

func TestSubagentsPastThePresetsLimitWaitTheirTurnAndStartByThemselves(t *testing.T) {
	test := Start(t, twoSubagents())
	limitTo(t, test, 1)
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "Where are sessions saved, and where are the tools?", nil)
	test.WaitForEvent("turnFinished", 2)
	events := test.Events(session.ID)
	agents := Finished(events)
	if len(agents) != 2 || agents[0].Meta != "started" || agents[1].Meta != "queued" || !strings.HasPrefix(agents[1].Output, "Queued the subagent “Find the tools”: 1 work at once, so it starts by itself once one of them reports (0 waiting before it). Your turn ends after this reply") {
		t.Fatalf("results %+v", agents)
	}
	order := kindsOf(events, "subagentQueued", "subagentStarted", "subagentFinished", "subagentReport", "turnStarted", "turnFinished")
	want := []string{"turnStarted", "subagentStarted", "subagentQueued", "turnFinished", "subagentFinished", "subagentStarted", "subagentFinished", "turnStarted", "subagentReport", "subagentReport", "turnFinished"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order %v", order)
	}
	Expect(t, OfKind(events, "subagentReport"), `[{"description":"Find the session store"},{"description":"Find the tools","note":"All your subagents have reported."}]`)
}

func TestSubagentsOnTheMainAgentsModelLeaveItsSlotAlone(t *testing.T) {
	options := twoSubagents()
	options.Slots, options.SubagentModel = 2, ""
	test := Start(t, options)
	limitTo(t, test, 1)
	question := "Where are sessions saved, and where are the tools?"
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, question, nil)
	test.WaitForEvent("turnFinished", 2)
	mainSlots, subagentSlots := map[any]bool{}, map[any]bool{}
	for _, request := range test.Requests() {
		slots := map[bool]map[any]bool{true: mainSlots, false: subagentSlots}[strings.Contains(RequestText(request), question)]
		slots[request["id_slot"]] = true
	}
	if len(mainSlots) != 1 || len(subagentSlots) != 1 || reflect.DeepEqual(mainSlots, subagentSlots) {
		t.Fatalf("the main agent asked on %v, its subagents on %v", mainSlots, subagentSlots)
	}
}

func TestAReportSaysWhichSubagentsHaveNotReportedYet(t *testing.T) {
	test := Start(t, Options{
		Files: subagentFiles,
		Replies: []fakellama.Reply{
			{ToolCalls: []fakellama.ToolCall{agentCall("Look quickly", "TASK-F: quick"), agentCall("Look slowly", "TASK-G: slow")}},
			text("The quick one reported; the slow one still looks."),
			text("Both reported."),
		},
		Scripts: map[string][]fakellama.Reply{
			"TASK-F": {{Text: "Quick answer."}},
			"TASK-G": {{Delay: 1500 * time.Millisecond, Text: "Slow answer."}},
		},
		Slots: 4,
	})
	sessionID := prompt(t, test, "Look")
	test.WaitForEvent("subagentFinished", 1)
	test.App.Sessions.SendPrompt(sessionID, "How is it going?", nil)
	test.WaitForEvent("turnFinished", 3)
	Expect(t, OfKind(test.Events(sessionID), "subagentReport"), `[{"description":"Look quickly","note":"Not reported yet: “Look slowly”. Their reports wake you once they are done: do not look into what they look into meanwhile."},{"description":"Look slowly","note":"All your subagents have reported."}]`)
}

func TestAReplyThatStartsSubagentsEndsTheTurnWhateverElseItDoes(t *testing.T) {
	test := Start(t, Options{
		Files: subagentFiles,
		Replies: []fakellama.Reply{
			{ToolCalls: []fakellama.ToolCall{{Arguments: map[string]any{"path": "src/tools.ts"}, Name: "read"}, agentCall("Find the store", "TASK-H: where?")}},
			text("It is in src/session-store.ts."),
		},
		Scripts: map[string][]fakellama.Reply{"TASK-H": {slowly(text("src/session-store.ts:1"))}},
	})
	sessionID := prompt(t, test, "Where is the store?")
	test.UntilIdle(sessionID)
	order := kindsOf(test.Events(sessionID), "assistantMessage", "turnFinished", "subagentReport")
	if !reflect.DeepEqual(order, []string{"assistantMessage", "turnFinished", "subagentReport", "assistantMessage", "turnFinished"}) {
		t.Fatalf("order %v", order)
	}
}
