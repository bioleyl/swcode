package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

func kwargsOf(test *App) []any {
	kwargs := []any{}
	for _, request := range test.Requests() {
		kwargs = append(kwargs, request["chat_template_kwargs"])
	}
	return kwargs
}

func TestThinkingAsksTheTemplateForTheSessionsModeAndNothingForTheModelsOwn(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("Yes."), text("Sure."), text("Fine.")}})
	instruct := test.NewSession(sessions.Request{Prompt: "One", Thinking: protocol.ThinkingModeOff})
	test.UntilIdle(instruct.ID)
	brief := test.NewSession(sessions.Request{Prompt: "Two", Thinking: protocol.ThinkingModeLow})
	test.UntilIdle(brief.ID)
	full := test.NewSession(sessions.Request{Prompt: "Three"})
	test.UntilIdle(full.ID)
	want := []any{map[string]any{"enable_thinking": false}, map[string]any{"reasoning_effort": "low"}, nil}
	if full.Thinking != protocol.ThinkingModeFull || !reflect.DeepEqual(kwargsOf(test), want) {
		t.Fatalf("kwargs %v", kwargsOf(test))
	}
}

func TestThinkingChangesFromTheNextRequestOn(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("Yes."), text("Sure.")}})
	session := test.NewSession(sessions.Request{Prompt: "One"})
	test.UntilIdle(session.ID)
	if problem := test.App.Sessions.SetThinking(session.ID, protocol.ThinkingModeMedium); problem != "" {
		t.Fatal(problem)
	}
	test.App.Sessions.SendPrompt(session.ID, "Two", nil)
	test.UntilIdle(session.ID)
	if !reflect.DeepEqual(kwargsOf(test), []any{nil, map[string]any{"reasoning_effort": "medium"}}) {
		t.Fatalf("kwargs %v", kwargsOf(test))
	}
	Expect(t, test.App.Sessions.List()[0], `{"thinking":"medium"}`)
}

func TestThinkingIsBriefForSubagents(t *testing.T) {
	test := Start(t, Options{
		Replies: []fakellama.Reply{call("agent", map[string]any{"description": "Look", "prompt": "TASK: look"}), text("Done.")},
		Scripts: map[string][]fakellama.Reply{"TASK": {slowly(text("Found it."))}},
	})
	session := test.NewSession(sessions.Request{Prompt: "Go"})
	test.UntilIdle(session.ID)
	if !reflect.DeepEqual(kwargsOf(test), []any{nil, map[string]any{"reasoning_effort": "low"}, nil}) {
		t.Fatalf("kwargs %v", kwargsOf(test))
	}
}

func TestThinkingIsTheModelsOwnForSessionsFromBeforeTheChoice(t *testing.T) {
	first := Start(t, Options{})
	session := first.NewSession(sessions.Request{})
	metaPath := filepath.Join(first.DataDir, "sessions", session.ID+".json")
	first.Stop()
	var older map[string]any
	data, _ := os.ReadFile(metaPath)
	_ = json.Unmarshal(data, &older)
	delete(older, "thinking")
	data, _ = json.Marshal(older)
	_ = os.WriteFile(metaPath, data, 0o644)
	again := Start(t, Options{DataDir: first.DataDir})
	Expect(t, again.App.Sessions.List()[0], map[string]any{"id": session.ID, "thinking": "full"})
}

const (
	mainModel    = "qwen3.8-flash-next"
	smallModel   = "qwen3.5-9b"
	mainContext  = 131_072
	smallContext = 65_536
	routerSlots  = 4
)

// router is what the GMKtec's router serves, in its alphabetical order: a model cached from Hugging Face but not
// loaded, then the subagents' small model before the main one.
var router = []fakellama.RouterModel{
	{ContextTokens: mainContext, ID: "ggml-org/gpt-oss-20b-GGUF:MXFP4", Loaded: false, Slots: routerSlots},
	{ContextTokens: smallContext, ID: smallModel, Loaded: true, Slots: routerSlots},
	{ContextTokens: mainContext, ID: mainModel, Loaded: true, Slots: routerSlots},
}

// presetStatus is the default preset's main model as GET /api/status says it.
func presetStatus(t *testing.T, test *App) map[string]any {
	t.Helper()
	status, body := test.Call("GET", protocol.EndpointStatus, nil)
	if status != 200 {
		t.Fatalf("status %d", status)
	}
	return body.(map[string]any)["llama"].(map[string]any)["presets"].([]any)[0].(map[string]any)
}

func TestARouterIsAskedForItsFirstLoadedModelWhenNoneIsSet(t *testing.T) {
	test := Start(t, Options{Router: router})
	Expect(t, presetStatus(t, test), map[string]any{"contextTokens": smallContext, "model": smallModel})
}

func TestTheMainAgentGetsALoadedModelTheSubagentsDoNotUse(t *testing.T) {
	test := Start(t, Options{Router: router, SubagentModel: smallModel})
	Expect(t, presetStatus(t, test), map[string]any{"contextTokens": mainContext, "model": mainModel})
}

func TestARouterIsNeverAskedAboutAModelItHasNotLoaded(t *testing.T) {
	unloaded := router[0].ID
	test := Start(t, Options{Model: unloaded, Router: router, SubagentModel: smallModel})
	preset := presetStatus(t, test)
	if preset["model"] != unloaded || preset["contextTokens"] != nil || slices.Contains(test.Llama.PropsAsked(), unloaded) {
		t.Fatalf("status %v, asked %v", preset, test.Llama.PropsAsked())
	}
}

func TestTheMainAgentAndItsSubagentsAreServedTheirOwnModels(t *testing.T) {
	test := Start(t, Options{
		Replies:       []fakellama.Reply{call("agent", map[string]any{"description": "Find it", "prompt": "TASK: where?"}), text("It is in src/a.ts.")},
		Router:        router,
		Scripts:       map[string][]fakellama.Reply{"TASK": {slowly(text("src/a.ts:1"))}},
		SubagentModel: smallModel,
	})
	session := test.NewSession(sessions.Request{})
	test.App.Sessions.SendPrompt(session.ID, "Where is it?", nil)
	Expect(t, test.WaitForEvent("turnFinished", 2), `{"outcome":"done"}`)
	// The subagent asks beside the main agent: its request may come before the main agent's next one, or after.
	models := []string{}
	for _, request := range test.Requests() {
		models = append(models, request["model"].(string))
	}
	slices.Sort(models)
	if !reflect.DeepEqual(models, []string{smallModel, mainModel, mainModel}) {
		t.Fatalf("models %v", models)
	}
}

// twoServers is the test's llm-server, and a second one serving a bigger model, as presets mixing them.
func twoServers(test *App, second *fakellama.Server) protocol.ModelSettings {
	return protocol.ModelSettings{
		DefaultPreset: "default",
		Presets: []protocol.Preset{
			{ID: "default", Main: protocol.ModelRef{Source: "first"}, Name: "Default", Workers: protocol.ModelRef{Source: "first"}},
			{ID: "mixed", Main: protocol.ModelRef{Model: "big", Source: "second"}, Name: "Mixed", Workers: protocol.ModelRef{Source: "first"}},
		},
		Sources: []protocol.LlmSource{{ID: "first", Name: "first", URL: test.Llama.URL}, {ID: "second", Name: "second", URL: second.URL}},
	}
}

func TestASessionsMainAgentAndWorkersAskTheLlamaServersOfItsPreset(t *testing.T) {
	test := Start(t, Options{Scripts: map[string][]fakellama.Reply{"TASK": {slowly(text("src/a.ts:1"))}}})
	second := fakellama.Start(fakellama.Options{Models: []string{"big"}, Replies: []fakellama.Reply{
		call("agent", map[string]any{"description": "Find it", "prompt": "TASK: where?"}), text("It is in src/a.ts."),
	}})
	defer second.Stop()
	told := make(chan protocol.ServerMessage, 1)
	test.App.Models.Subscribe(func(message protocol.ServerMessage) { told <- message })
	if status, body := test.Call("PUT", protocol.EndpointModels, twoServers(test, second)); status != 200 {
		t.Fatalf("status %d: %v", status, body)
	}
	session := test.NewSession(sessions.Request{Preset: "mixed"})
	test.App.Sessions.SendPrompt(session.ID, "Where is it?", nil)
	test.UntilIdle(session.ID)
	if len(second.Requests()) != 2 || second.Requests()[0]["model"] != "big" || len(test.Requests()) != 1 {
		t.Fatalf("main %d requests, workers %d", len(second.Requests()), len(test.Requests()))
	}
	Expect(t, test.App.Sessions.List()[0], `{"preset":"mixed"}`)
	if presets := (<-told).Value.(protocol.ServerMessagePresets).Choices.Presets; len(presets) != 2 {
		t.Fatalf("presets told %v", presets)
	}
	if _, err := os.Stat(filepath.Join(test.DataDir, "models.json")); err != nil {
		t.Fatal("the models were not kept")
	}
}

func TestASessionTakesTheDefaultPresetOnceItsOwnIsRemoved(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{text("From the first.")}})
	second := fakellama.Start(fakellama.Options{Models: []string{"big"}})
	defer second.Stop()
	settings := twoServers(test, second)
	test.Call("PUT", protocol.EndpointModels, settings)
	session := test.NewSession(sessions.Request{})
	if problem := test.App.Sessions.SetPreset(session.ID, "missing"); problem == "" {
		t.Fatal("an unknown preset was taken")
	}
	if problem := test.App.Sessions.SetPreset(session.ID, "mixed"); problem != "" {
		t.Fatal(problem)
	}
	settings.Presets = settings.Presets[:1]
	test.Call("PUT", protocol.EndpointModels, settings)
	test.App.Sessions.SendPrompt(session.ID, "Hi", nil)
	test.UntilIdle(session.ID)
	if len(test.Requests()) != 1 || len(second.Requests()) != 0 {
		t.Fatalf("first %d requests, second %d", len(test.Requests()), len(second.Requests()))
	}
}

func TestModelsThatCannotWorkAreRefused(t *testing.T) {
	test := Start(t, Options{})
	settings := twoServers(test, &fakellama.Server{URL: "http://127.0.0.1:1"})
	settings.Presets[1].Main.Source = "gone"
	status, body := test.Call("PUT", protocol.EndpointModels, settings)
	Expect(t, body, map[string]any{"ok": false, "problem": "A preset uses an llm-server that is not in the list."})
	if status != 400 {
		t.Fatalf("status %d", status)
	}
	_, saved := test.Call("GET", protocol.EndpointModels, nil)
	Expect(t, saved, map[string]any{"defaultPreset": "default", "sources": []any{map[string]any{"url": test.Llama.URL}}})
}

func TestAPresetsContextIsUsedUpToTheOneItsServerSays(t *testing.T) {
	test := Start(t, Options{ContextTokens: 65_536})
	contextOfStatus := func(asked int) any {
		settings := test.App.Models.Settings()
		settings.Presets[0].Main.ContextTokens = asked
		if status, body := test.Call("PUT", protocol.EndpointModels, settings); status != 200 {
			t.Fatalf("status %d: %v", status, body)
		}
		_, status := test.Call("GET", protocol.EndpointStatus, nil)
		return status.(map[string]any)["llama"].(map[string]any)["presets"].([]any)[0].(map[string]any)["contextTokens"]
	}
	if got := contextOfStatus(32_768); got != float64(32_768) {
		t.Fatalf("a smaller one: %v", got)
	}
	if got := contextOfStatus(131_072); got != float64(65_536) {
		t.Fatalf("a larger one: %v", got)
	}
}
