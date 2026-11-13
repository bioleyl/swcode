package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

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

// shortRetryWaits makes the waits before a request goes again short, for the test.
func shortRetryWaits(t *testing.T, waits ...time.Duration) {
	saved := retryWaits
	retryWaits = waits
	t.Cleanup(func() { retryWaits = saved })
}

func TestARequestGoesAgainWhileLlmServerIsOutOfReachForAMoment(t *testing.T) {
	shortRetryWaits(t, time.Millisecond, time.Millisecond)
	fake := fakellama.Start(fakellama.Options{Replies: []fakellama.Reply{
		{FailWith: http.StatusBadGateway},
		{FailWith: http.StatusServiceUnavailable},
		{Text: "Back."},
	}})
	defer fake.Stop()
	reply, err := NewClient(Options{URL: fake.URL}).Complete(context.Background(), Request{Target: Target{Slot: NoSlot}}, func(Part) {})
	if err != nil || reply.Text != "Back." || len(fake.Requests()) != 3 {
		t.Fatalf("reply %+v, %v, %d requests", reply, err, len(fake.Requests()))
	}
}

func TestTheTurnFailsOnceTheRequestWentAgainAfterEveryWait(t *testing.T) {
	shortRetryWaits(t, time.Millisecond, time.Millisecond)
	gone := fakellama.Reply{FailWith: http.StatusBadGateway, Text: "Bad Gateway"}
	fake := fakellama.Start(fakellama.Options{Replies: []fakellama.Reply{gone, gone, gone, {Text: "Too late."}}})
	defer fake.Stop()
	_, err := NewClient(Options{URL: fake.URL}).Complete(context.Background(), Request{Target: Target{Slot: NoSlot}}, func(Part) {})
	if err == nil || err.Error() != "llm-server answered 502: Bad Gateway" || len(fake.Requests()) != 3 {
		t.Fatalf("got %v, %d requests", err, len(fake.Requests()))
	}
}

func TestStoppingTheTurnEndsTheWaitBeforeARequestGoesAgain(t *testing.T) {
	shortRetryWaits(t, time.Hour)
	fake := fakellama.Start(fakellama.Options{Replies: []fakellama.Reply{{FailWith: http.StatusBadGateway}, {Text: "Too late."}}})
	defer fake.Stop()
	ctx, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	_, err := NewClient(Options{URL: fake.URL}).Complete(ctx, Request{Target: Target{Slot: NoSlot}}, func(Part) {})
	if !errors.Is(err, context.DeadlineExceeded) || len(fake.Requests()) != 1 {
		t.Fatalf("got %v, %d requests", err, len(fake.Requests()))
	}
}

func TestAServerThatRefusesLlamaCppsFieldsGetsRequestsWithoutThem(t *testing.T) {
	fake := fakellama.Start(fakellama.Options{
		Refuses: []string{"cache_prompt", "id_slot", "chat_template_kwargs", "reasoning_content"},
		Replies: []fakellama.Reply{{Text: "First."}, {Text: "Second."}},
	})
	defer fake.Stop()
	client := NewClient(Options{URL: fake.URL})
	request := Request{
		Messages: []Message{
			{Content: "hello", Role: RoleUser},
			{Content: "Hi.", Reasoning: "Greet back", Role: RoleAssistant},
			{Content: "again", Role: RoleUser},
		},
		Target: Target{Identity: Identity{Model: "gpt-5", Thinking: protocol.ThinkingModeOff}, Slot: 1},
	}
	for _, want := range []string{"First.", "Second."} {
		reply, err := client.Complete(context.Background(), request, func(Part) {})
		if err != nil || reply.Text != want {
			t.Fatalf("reply %+v, %v", reply, err)
		}
	}
	// The first request is refused and sent again plain; the next one goes plain at once.
	requests := fake.Requests()
	if len(requests) != 3 || requests[0]["cache_prompt"] != true {
		t.Fatalf("requests %v", requests)
	}
	for _, body := range requests[1:] {
		messages, _ := body["messages"].([]any)
		reply, _ := messages[1].(map[string]any)
		if body["cache_prompt"] != nil || body["id_slot"] != nil || body["chat_template_kwargs"] != nil ||
			reply["reasoning_content"] != nil || body["model"] != "gpt-5" {
			t.Fatalf("body %v", body)
		}
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

func TestTheContextOfAServerWithoutPropsIsReadFromItsModelsList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/models" {
			http.NotFound(response, request)
			return
		}
		_, _ = response.Write([]byte(`{"data":[{"id":"qwen3-coder","max_model_len":32768}]}`))
	}))
	defer server.Close()
	status := NewStatusReader(Options{URL: server.URL})
	ctx := context.Background()
	if status.ContextOf(ctx, "qwen3-coder") != 32_768 || status.ContextOf(ctx, "") != 32_768 || status.ContextOf(ctx, "other") != 0 {
		t.Fatal("context")
	}
}

func TestTheContextIsReadAsHostedAPIsListIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/models" {
			http.NotFound(response, request)
			return
		}
		_, _ = response.Write([]byte(`{"data":[
			{"id":"router","context_length":200000},
			{"id":"groq","context_window":131072},
			{"id":"mistral","max_context_length":128000},
			{"id":"gpt-5","object":"model","owned_by":"openai"}
		]}`))
	}))
	defer server.Close()
	status := NewStatusReader(Options{URL: server.URL})
	ctx := context.Background()
	got := []int{status.ContextOf(ctx, "router"), status.ContextOf(ctx, "groq"), status.ContextOf(ctx, "mistral"), status.ContextOf(ctx, "gpt-5")}
	if !reflect.DeepEqual(got, []int{200_000, 131_072, 128_000, 0}) {
		t.Fatalf("contexts %v", got)
	}
}

// lines is a stream as llm-server writes it: an event a line.
func lines(events ...string) string {
	return "data: " + strings.Join(events, "\n\ndata: ") + "\n\ndata: [DONE]\n\n"
}

func TestAReplyKeepsHowLlamaCppSpentItsTimeAndHowMuchOfThePromptItHadKept(t *testing.T) {
	const answer = `{"choices":[{"delta":{"content":"Hi."}}]}`
	replies := map[string]*protocol.Usage{
		// llama.cpp's timings beside its usage, in its last event.
		lines(answer, `{"choices":[],"usage":{"prompt_tokens":1200,"completion_tokens":40},"timings":{"cache_n":1100,"prompt_n":100,"prompt_ms":250.4,"predicted_n":40,"predicted_ms":999.6}}`): {
			CachedTokens: 1100, CompletionTokens: 40, PredictedMs: 1000, PromptMs: 250, PromptTokens: 1200,
		},
		// Timings alone count the tokens too: the prompt is what was kept and what was processed.
		lines(answer, `{"choices":[],"timings":{"cache_n":900,"prompt_n":300,"prompt_ms":10,"predicted_n":7,"predicted_ms":70}}`): {
			CachedTokens: 900, CompletionTokens: 7, PredictedMs: 70, PromptMs: 10, PromptTokens: 1200,
		},
		// OpenAI's API says what it had kept in its usage, and no time.
		lines(answer, `{"choices":[],"usage":{"prompt_tokens":500,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":400}}}`): {
			CachedTokens: 400, CompletionTokens: 5, PromptTokens: 500,
		},
		lines(answer, `{"choices":[],"usage":{"prompt_tokens":500,"completion_tokens":5}}`): {CompletionTokens: 5, PromptTokens: 500},
	}
	for stream, expected := range replies {
		reply, err := readReply(context.Background(), strings.NewReader(stream), func(Part) {})
		if err != nil || !reflect.DeepEqual(reply.Usage, expected) {
			t.Fatalf("usage %+v, error %v, for %s", reply.Usage, err, stream)
		}
	}
}
