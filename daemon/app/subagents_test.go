package app_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

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

// twoSubagents is a main agent that sends two explore subagents in one reply, then answers from what they found.
func twoSubagents() Options {
	return Options{
		Files: subagentFiles,
		Replies: []fakellama.Reply{
			{ToolCalls: []fakellama.ToolCall{agentCall("Find the session store", "TASK-A: where are sessions saved?"), agentCall("Find the tools", "TASK-B: where are the tools listed?")}},
			text("Sessions are saved in src/session-store.ts, tools are listed in src/tools.ts."),
		},
		Scripts: map[string][]fakellama.Reply{
			"TASK-A": {call("grep", map[string]any{"pattern": "writeFileSync"}), text("src/session-store.ts:1 saves them with writeFileSync.")},
			"TASK-B": {call("glob", map[string]any{"pattern": "**/*.ts"}), text("src/tools.ts:1 lists them in TOOLS.")},
		},
		Slots:         4,
		SubagentModel: subagentModel,
	}
}

func runTwoSubagents(t *testing.T) (*App, string) {
	test := Start(t, twoSubagents())
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "Where are sessions saved, and where are the tools?", nil)
	test.WaitForEvent("turnFinished", 1)
	return test, session.ID
}

func TestSubagentsRunOneAfterTheOtherOnTheirModelAndGiveTheirAnswers(t *testing.T) {
	test, sessionID := runTwoSubagents(t)
	events := test.Events(sessionID)
	order := []string{}
	for _, kind := range Kinds(events) {
		if kind == "subagentStarted" || kind == "subagentFinished" {
			order = append(order, kind)
		}
	}
	if !reflect.DeepEqual(order, []string{"subagentStarted", "subagentFinished", "subagentStarted", "subagentFinished"}) {
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
	outputs := []string{}
	for _, finished := range Finished(events) {
		outputs = append(outputs, finished.Output)
	}
	if !reflect.DeepEqual(outputs, []string{"src/session-store.ts:1 saves them with writeFileSync.", "src/tools.ts:1 lists them in TOOLS."}) {
		t.Fatalf("outputs %v", outputs)
	}
	Expect(t, last(events), `{"outcome":"done"}`)
}

func TestSubagentsKeepTheirOwnWorkOutOfTheMainConversation(t *testing.T) {
	test, sessionID := runTwoSubagents(t)
	requests := test.Requests()
	lastMain := RequestText(map[string]any{"messages": requests[len(requests)-1]["messages"]})
	if !strings.Contains(lastMain, "saves them with writeFileSync") || strings.Contains(lastMain, `"name":"grep"`) {
		t.Fatal("the subagents' work reached the main conversation")
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

func TestSubagentsCannotChangeFiles(t *testing.T) {
	test := Start(t, Options{
		Files:   subagentFiles,
		Replies: []fakellama.Reply{{ToolCalls: []fakellama.ToolCall{agentCall("Try to edit", "TASK-C: change something")}}, text("It could not.")},
		Scripts: map[string][]fakellama.Reply{"TASK-C": {call("bash", map[string]any{"command": "rm -rf src"}), text("I can only read.")}},
	})
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "Go", nil)
	test.WaitForEvent("turnFinished", 1)
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
		Scripts:       map[string][]fakellama.Reply{"TASK-D": {empty, empty, empty, {Text: answer}}},
		SubagentModel: subagentModel,
	})
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "Where are the tools?", nil)
	test.WaitForEvent("turnFinished", 1)
	Expect(t, Finished(test.Events(session.ID)), []any{map[string]any{"ok": true, "output": answer}})
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

// limitTo has the default preset let a turn start that many subagents.
func limitTo(t *testing.T, test *App, perTurn int) {
	t.Helper()
	settings := test.App.Models.Settings()
	settings.Presets[0].SubagentLimit = &protocol.SubagentLimit{PerTurn: perTurn}
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

func TestATurnStartsNoMoreSubagentsThanItsPresetAllows(t *testing.T) {
	test := Start(t, twoSubagents())
	limitTo(t, test, 1)
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "Where are sessions saved, and where are the tools?", nil)
	test.WaitForEvent("turnFinished", 1)
	finished := Finished(test.Events(session.ID))
	agents := []protocol.SessionEventToolFinished{}
	for _, each := range finished {
		if each.Meta == "refused" || strings.Contains(each.Output, "session-store") {
			agents = append(agents, each)
		}
	}
	if len(agents) != 2 || agents[0].Meta == "refused" || !strings.HasPrefix(agents[1].Output, "No more subagents in this turn: the preset allows 1.") {
		t.Fatalf("results %+v", agents)
	}
	if len(OfKind(test.Events(session.ID), "subagentStarted")) != 1 {
		t.Fatal("a second subagent started")
	}
}
