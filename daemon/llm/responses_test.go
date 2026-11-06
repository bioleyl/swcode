package llm

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
)

func inputTypes(body map[string]any) []string {
	input, _ := body["input"].([]any)
	types := []string{}
	for _, each := range input {
		item, _ := each.(map[string]any)
		kind, _ := item["type"].(string)
		role, _ := item["role"].(string)
		types = append(types, strings.TrimSpace(kind+" "+role))
	}
	return types
}

func TestTheResponsesAPIStreamsAReplyAndGetsItsItemsBackWithTheEncryptedReasoning(t *testing.T) {
	fake := fakellama.Start(fakellama.Options{Replies: []fakellama.Reply{
		{Reasoning: "Look first", Text: "Reading it.", ToolCalls: []fakellama.ToolCall{{Arguments: map[string]any{"path": "a.txt"}, Name: "read"}}},
		{Text: "It says hello."},
	}})
	defer fake.Stop()
	client := NewResponsesClient(Options{URL: fake.URL})
	target := Target{Identity: Identity{Model: "gpt-5.6-luna", Thinking: protocol.ThinkingModeLow}, Slot: NoSlot}
	tools := []ToolDefinition{{Description: "Reads", Name: "read", Parameters: map[string]any{"type": "object"}}}
	messages := []Message{{Content: "be brief", Role: RoleSystem}, {Content: "read a.txt", Role: RoleUser}}
	parts := []Part{}
	reply, err := client.Complete(context.Background(), Request{Messages: messages, Target: target, Tools: tools}, func(part Part) { parts = append(parts, part) })
	if err != nil {
		t.Fatal(err)
	}
	want := []protocol.ToolCall{{Arguments: `{"path":"a.txt"}`, ID: "call-0", Name: "read"}}
	if reply.Text != "Reading it." || reply.Reasoning != "Look first" || !reflect.DeepEqual(reply.ToolCalls, want) {
		t.Fatalf("reply %+v", reply)
	}
	if reply.Usage == nil || reply.Usage.CompletionTokens != 40 || !strings.Contains(reply.Items, fakellama.EncryptedReasoning) {
		t.Fatalf("usage %+v, items %s", reply.Usage, reply.Items)
	}
	if len(parts) != 5 || parts[0].Kind != PartReasoning || parts[4] != (Part{Kind: PartToolCall, Text: "read"}) {
		t.Fatalf("parts %+v", parts)
	}
	first := fake.Requests()[0]
	wantReasoning := map[string]any{"effort": "low", "summary": "auto"}
	if first["model"] != "gpt-5.6-luna" || first["store"] != false || !reflect.DeepEqual(first["reasoning"], wantReasoning) ||
		!reflect.DeepEqual(first["include"], []any{"reasoning.encrypted_content"}) {
		t.Fatalf("body %v", first)
	}
	if tool, _ := first["tools"].([]any)[0].(map[string]any); tool["name"] != "read" || tool["strict"] != false || tool["type"] != "function" {
		t.Fatalf("tools %v", first["tools"])
	}

	messages = append(messages,
		Message{Content: reply.Text, Items: reply.Items, Reasoning: reply.Reasoning, Role: RoleAssistant, ToolCalls: reply.ToolCalls},
		Message{Content: "hello", Role: RoleTool, ToolCallID: "call-0"},
	)
	if _, err := client.Complete(context.Background(), Request{Messages: messages, Target: target, Tools: tools}, func(Part) {}); err != nil {
		t.Fatal(err)
	}
	second := fake.Requests()[1]
	wantTypes := []string{"system", "user", "reasoning", "message assistant", "function_call", "function_call_output"}
	if got := inputTypes(second); !reflect.DeepEqual(got, wantTypes) {
		t.Fatalf("input %v", got)
	}
	input, _ := second["input"].([]any)
	reasoning, _ := input[2].(map[string]any)
	output, _ := input[5].(map[string]any)
	if reasoning["encrypted_content"] != fakellama.EncryptedReasoning || output["call_id"] != "call-0" || output["output"] != "hello" {
		t.Fatalf("input %v", input)
	}
}

func TestAReplyOfAnotherServerGoesToTheResponsesAPIAsItsTextAndCalls(t *testing.T) {
	calls := []protocol.ToolCall{{Arguments: `{}`, ID: "call_1", Name: "plan"}}
	got, _ := json.Marshal(responsesInput([]Message{{Content: "On it.", Reasoning: "Plan", Role: RoleAssistant, ToolCalls: calls}}))
	want := `[{"content":"On it.","role":"assistant"},{"arguments":"{}","call_id":"call_1","name":"plan","type":"function_call"}]`
	if string(got) != want {
		t.Fatalf("input %s", got)
	}
}

func TestAModelThatRefusesReasoningSettingsGetsFewerUntilItTakesTheRequest(t *testing.T) {
	fake := fakellama.Start(fakellama.Options{
		Refuses: []string{"reasoning"},
		Replies: []fakellama.Reply{{Text: "First."}, {Text: "Second."}},
	})
	defer fake.Stop()
	client := NewResponsesClient(Options{URL: fake.URL})
	request := Request{
		Messages: []Message{{Content: "hello", Role: RoleUser}},
		Target:   Target{Identity: Identity{Model: "gpt-4.1", Thinking: protocol.ThinkingModeLow}, Slot: NoSlot},
	}
	for _, want := range []string{"First.", "Second."} {
		reply, err := client.Complete(context.Background(), request, func(Part) {})
		if err != nil || reply.Text != want {
			t.Fatalf("reply %+v, %v", reply, err)
		}
	}
	// The effort and its summary, then the effort alone, then none: the next request goes without at once.
	requests := fake.Requests()
	efforts := []any{}
	for _, body := range requests {
		efforts = append(efforts, body["reasoning"])
	}
	wantEfforts := []any{map[string]any{"effort": "low", "summary": "auto"}, map[string]any{"effort": "low"}, nil, nil}
	if !reflect.DeepEqual(efforts, wantEfforts) || requests[3]["include"] != nil {
		t.Fatalf("requests %v", requests)
	}
}

func TestTheResponsesAPITellsAFullContext(t *testing.T) {
	fake := fakellama.Start(fakellama.Options{Replies: []fakellama.Reply{
		{FailWith: 400, Text: "Your input exceeds the context window of this model. Please adjust your input and try again."},
	}})
	defer fake.Stop()
	client := NewResponsesClient(Options{URL: fake.URL})
	_, err := client.Complete(context.Background(), Request{Target: Target{Slot: NoSlot}}, func(Part) {})
	if !IsContextFull(err) || len(fake.Requests()) != 1 {
		t.Fatalf("got %v after %d requests", err, len(fake.Requests()))
	}
}

func TestOnlyOpenAIsOwnServerIsAskedThroughTheResponsesAPI(t *testing.T) {
	for address, want := range map[string]bool{
		"https://api.openai.com": true, "https://api.openai.com/": true, "http://garfield.home:8080": false, "": false,
	} {
		if UsesResponses(address) != want {
			t.Fatalf("%q: %v", address, !want)
		}
	}
}
