package llm

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
)

func TestTheClientStreamsTextReasoningAndToolCalls(t *testing.T) {
	fake := fakellama.Start(fakellama.Options{Replies: []fakellama.Reply{{
		Reasoning: "Look first",
		Text:      "Reading it.",
		ToolCalls: []fakellama.ToolCall{{Arguments: map[string]any{"path": "a.txt"}, Name: "read"}},
	}}})
	defer fake.Stop()
	client := NewClient(Options{URL: fake.URL + "/"})
	parts := []Part{}
	reply, err := client.Complete(context.Background(), Request{
		Messages: []Message{{Content: "be brief", Role: RoleSystem}, {Content: "read a.txt", Role: RoleUser}},
		Target:   Target{Identity: Identity{AgentID: "a", Thinking: protocol.ThinkingModeLow}, Slot: 2},
		Tools:    []ToolDefinition{{Description: "Reads", Name: "read", Parameters: map[string]any{"type": "object"}}},
	}, func(part Part) { parts = append(parts, part) })
	if err != nil {
		t.Fatal(err)
	}
	want := []protocol.ToolCall{{Arguments: `{"path":"a.txt"}`, ID: "call-0", Name: "read"}}
	if reply.Text != "Reading it." || reply.Reasoning != "Look first" || !reflect.DeepEqual(reply.ToolCalls, want) {
		t.Fatalf("reply %+v", reply)
	}
	if reply.Usage == nil || reply.Usage.CompletionTokens != 40 {
		t.Fatalf("usage %+v", reply.Usage)
	}
	if len(parts) != 5 || parts[0].Kind != PartReasoning || parts[4] != (Part{Kind: PartToolCall, Text: "read"}) {
		t.Fatalf("parts %+v", parts)
	}
	body := fake.Requests()[0]
	if body["id_slot"] != float64(2) || body["cache_prompt"] != true {
		t.Fatalf("body %v", body)
	}
	if kwargs, _ := body["chat_template_kwargs"].(map[string]any); kwargs["reasoning_effort"] != "low" {
		t.Fatalf("kwargs %v", body["chat_template_kwargs"])
	}
}

func TestTheClientTellsAFullContextFromOtherRefusals(t *testing.T) {
	fake := fakellama.Start(fakellama.Options{Replies: []fakellama.Reply{
		{ErrorType: "exceed_context_size_error", FailWith: 400, Text: "too long"},
		{FailWith: 500, Text: "boom"},
	}})
	defer fake.Stop()
	client := NewClient(Options{URL: fake.URL})
	_, err := client.Complete(context.Background(), Request{Target: Target{Slot: NoSlot}}, func(Part) {})
	if !IsContextFull(err) {
		t.Fatalf("got %v", err)
	}
	_, err = client.Complete(context.Background(), Request{Target: Target{Slot: NoSlot}}, func(Part) {})
	if IsContextFull(err) || !strings.Contains(err.Error(), "llm-server answered 500: boom") {
		t.Fatalf("got %v", err)
	}
}

func TestTheStatusReadsARoutersLoadedModelsAndNeverAsksForAnUnloadedOne(t *testing.T) {
	fake := fakellama.Start(fakellama.Options{Router: []fakellama.RouterModel{
		{ContextTokens: 65_536, ID: "a-small", Loaded: true, Slots: 6},
		{ContextTokens: 131_072, ID: "b-main", Loaded: true, Slots: 4},
		{ContextTokens: 8192, ID: "c-old", Loaded: false, Slots: 1},
	}})
	defer fake.Stop()
	status := NewStatusReader(Options{URL: fake.URL})
	ctx := context.Background()
	served, reachable := status.Models(ctx)
	if !reachable || !reflect.DeepEqual(served, []string{"a-small", "b-main", "c-old"}) {
		t.Fatalf("models %v", served)
	}
	if main := status.FirstLoaded(ctx, "a-small"); main != "b-main" || status.ContextOf(ctx, main) != 131_072 {
		t.Fatalf("main %q", main)
	}
	if status.SlotsOf(ctx, "a-small") != 6 || status.ContextOf(ctx, "c-old") != 0 {
		t.Fatal("slots or context")
	}
	for _, asked := range fake.PropsAsked() {
		if asked == "c-old" {
			t.Fatal("asked for an unloaded model")
		}
	}
}

func TestARoutersModelSeesPicturesWhetherItIsLoadedOrNot(t *testing.T) {
	fake := fakellama.Start(fakellama.Options{Router: []fakellama.RouterModel{
		{ContextTokens: 131_072, ID: "b-main", Loaded: false, Slots: 4, Vision: true},
		{ContextTokens: 65_536, ID: "c-text", Loaded: true, Slots: 6},
	}})
	defer fake.Stop()
	status := NewStatusReader(Options{Model: "b-main", URL: fake.URL})
	ctx := context.Background()
	if !status.VisionOf(ctx, "b-main") || status.VisionOf(ctx, "c-text") || !status.VisionOf(ctx, "") {
		t.Fatal("vision")
	}
	if slices.Contains(fake.PropsAsked(), "b-main") {
		t.Fatal("asked for an unloaded model")
	}
}

func TestASingleModelLlamaServerSaysInItsPropsWhetherItSeesPictures(t *testing.T) {
	fake := fakellama.Start(fakellama.Options{Vision: true})
	defer fake.Stop()
	if !NewStatusReader(Options{URL: fake.URL}).VisionOf(context.Background(), "") {
		t.Fatal("vision")
	}
}

func TestTheStatusOfALlamaServerThatDoesNotAnswer(t *testing.T) {
	served, reachable := NewStatusReader(Options{URL: "http://127.0.0.1:1"}).Models(context.Background())
	if reachable || served == nil {
		t.Fatalf("models %v", served)
	}
}
