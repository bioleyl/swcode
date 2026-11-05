package llm

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"swcode/daemon/config"
	"swcode/daemon/protocol"
)

const testModel = "qwen3-coder-30b"

func schedulerWith(slots int) *Scheduler {
	return NewScheduler(func(context.Context, string) int { return slots }, config.WholeShare)
}

func acquire(t *testing.T, slots *Scheduler, model, agent string) *Lease {
	t.Helper()
	lease, err := slots.Acquire(context.Background(), model, agent)
	if err != nil {
		t.Fatal(err)
	}
	return lease
}

func TestSharedSlotsSplitsTheSlotsBetweenTheDaemons(t *testing.T) {
	cases := []struct {
		total int
		share config.SlotShare
		want  []int
	}{
		{4, config.SlotShare{Part: 1, Parts: 2}, []int{0, 1}},
		{4, config.SlotShare{Part: 2, Parts: 2}, []int{2, 3}},
		{3, config.SlotShare{Part: 2, Parts: 2}, []int{1, 2}},
		{6, config.WholeShare, []int{0, 1, 2, 3, 4, 5}},
		// Every share gets a slot, sharing one when there are fewer slots than shares.
		{1, config.SlotShare{Part: 1, Parts: 2}, []int{0}},
		{1, config.SlotShare{Part: 2, Parts: 2}, []int{0}},
	}
	for _, each := range cases {
		if got := SharedSlots(each.total, each.share); !reflect.DeepEqual(got, each.want) {
			t.Errorf("%d %v: got %v", each.total, each.share, got)
		}
	}
}

func TestTheSchedulerHandsOutItsShareOnlyThenMakesRequestsWait(t *testing.T) {
	slots := NewScheduler(func(context.Context, string) int { return 4 }, config.SlotShare{Part: 2, Parts: 2})
	first := acquire(t, slots, testModel, "main")
	second := acquire(t, slots, testModel, "explore-1")
	if first.Slot != 2 || second.Slot != 3 {
		t.Fatalf("slots %d %d", first.Slot, second.Slot)
	}
	want := []protocol.SlotUsage{{Busy: []int{2, 3}, Model: testModel, Own: []int{2, 3}, Total: 4}}
	if got := slots.Usage(); !reflect.DeepEqual(got, want) {
		t.Fatalf("usage %+v", got)
	}
	third := make(chan *Lease)
	go func() { third <- acquire(t, slots, testModel, "explore-2") }()
	time.Sleep(20 * time.Millisecond)
	first.Release()
	if lease := <-third; lease.Slot != 2 {
		t.Fatalf("slot %d", lease.Slot)
	}
}

func TestTheSchedulerGivesAnAgentBackTheSlotItHeld(t *testing.T) {
	slots := schedulerWith(3)
	main := acquire(t, slots, testModel, "main")
	other := acquire(t, slots, testModel, "explore-1")
	main.Release()
	other.Release()
	if got := acquire(t, slots, testModel, "explore-1").Slot; got != other.Slot {
		t.Fatalf("slot %d", got)
	}
}

func TestTheSchedulerKeepsASlotWhoseCacheAnotherAgentWillWant(t *testing.T) {
	slots := schedulerWith(2)
	acquire(t, slots, testModel, "main").Release()
	if got := acquire(t, slots, testModel, "explore-1").Slot; got != 1 {
		t.Fatalf("slot %d", got)
	}
}

func TestTheSchedulerServesWaitingRequestsFirstComeFirstServed(t *testing.T) {
	slots := schedulerWith(1)
	holding := acquire(t, slots, testModel, "main")
	var mu sync.Mutex
	order := []string{}
	var group sync.WaitGroup
	for _, agent := range []string{"explore-1", "explore-2"} {
		group.Add(1)
		go func() {
			defer group.Done()
			lease := acquire(t, slots, testModel, agent)
			mu.Lock()
			order = append(order, agent)
			mu.Unlock()
			lease.Release()
		}()
		time.Sleep(20 * time.Millisecond)
	}
	holding.Release()
	group.Wait()
	if !reflect.DeepEqual(order, []string{"explore-1", "explore-2"}) {
		t.Fatalf("order %v", order)
	}
}

func TestTheSchedulerLetsAWaitingRequestGoWhenItsTurnStops(t *testing.T) {
	slots := schedulerWith(1)
	acquire(t, slots, testModel, "main")
	ctx, stop := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, stop)
	if _, err := slots.Acquire(ctx, testModel, "explore-1"); !errors.Is(err, ErrStopped) {
		t.Fatalf("got %v", err)
	}
}

func TestTheSchedulerKeepsTheModelsApart(t *testing.T) {
	slots := schedulerWith(1)
	acquire(t, slots, "gpt-oss-120b", "main")
	if got := acquire(t, slots, testModel, "explore-1").Slot; got != 0 {
		t.Fatalf("slot %d", got)
	}
}

func TestTheSchedulerAsksAgainOnceAGuessedPoolIsIdle(t *testing.T) {
	answers := []int{0, 4}
	slots := NewScheduler(func(context.Context, string) int {
		answer := answers[0]
		answers = answers[1:]
		return answer
	}, config.WholeShare)
	acquire(t, slots, testModel, "main").Release()
	want := []protocol.SlotUsage{{Busy: []int{}, Model: testModel, Own: []int{0}, Total: 1}}
	if got := slots.Usage(); !reflect.DeepEqual(got, want) {
		t.Fatalf("usage %+v", got)
	}
	acquire(t, slots, testModel, "main")
	want = []protocol.SlotUsage{{Busy: []int{0}, Model: testModel, Own: []int{0, 1, 2, 3}, Total: 4}}
	if got := slots.Usage(); !reflect.DeepEqual(got, want) {
		t.Fatalf("usage %+v", got)
	}
}

func TestTheSchedulerCountsAReleaseOnce(t *testing.T) {
	slots := schedulerWith(2)
	lease := acquire(t, slots, testModel, "main")
	acquire(t, slots, testModel, "explore-1")
	lease.Release()
	lease.Release()
	want := []protocol.SlotUsage{{Busy: []int{1}, Model: testModel, Own: []int{0, 1}, Total: 2}}
	if got := slots.Usage(); !reflect.DeepEqual(got, want) {
		t.Fatalf("usage %+v", got)
	}
}

func TestReadSSELineReadsDataErrorsAndAnythingElse(t *testing.T) {
	cases := map[string]SSELine{
		`data: {"x":1}`:             {Data: `{"x":1}`, Kind: SSEData},
		`error: {"message":"boom"}`: {Data: `{"message":"boom"}`, Kind: SSEError},
		": keep-alive":              {Kind: SSEOther},
	}
	for line, want := range cases {
		if got := ReadSSELine(line); got != want {
			t.Errorf("%s: got %+v", line, got)
		}
	}
}

func TestTheReplyBuilderHandsBackTheTextAsItComes(t *testing.T) {
	builder := &ReplyBuilder{}
	if parts := builder.Add(StreamChunk{Text: "Hel"}); !reflect.DeepEqual(parts, []Part{{Kind: PartText, Text: "Hel"}}) {
		t.Fatalf("parts %v", parts)
	}
	builder.Add(StreamChunk{Text: "lo"})
	if reply := builder.Build(func() string { return "id" }); reply.Text != "Hello" {
		t.Fatalf("text %q", reply.Text)
	}
	if parts := (&ReplyBuilder{}).Add(StreamChunk{Reasoning: "Let me think"}); !reflect.DeepEqual(parts, []Part{{Kind: PartReasoning, Text: "Let me think"}}) {
		t.Fatalf("parts %v", parts)
	}
}

func TestTheReplyBuilderSaysOnceWhichToolACallIsFor(t *testing.T) {
	builder := &ReplyBuilder{}
	if parts := builder.Add(StreamChunk{ToolCalls: []ToolCallDelta{{Index: 0}}}); len(parts) != 0 {
		t.Fatalf("parts before the name %v", parts)
	}
	if parts := builder.Add(StreamChunk{ToolCalls: []ToolCallDelta{{Index: 0, Name: "write"}}}); !reflect.DeepEqual(parts, []Part{{Kind: PartToolCall, Text: "write"}}) {
		t.Fatalf("parts with the name %v", parts)
	}
	if parts := builder.Add(StreamChunk{ToolCalls: []ToolCallDelta{{Arguments: `{"path"`, Index: 0, Name: "write"}}}); len(parts) != 0 {
		t.Fatalf("parts of the arguments %v", parts)
	}
	if parts := builder.Add(StreamChunk{ToolCalls: []ToolCallDelta{{Index: 1, Name: "read"}}}); !reflect.DeepEqual(parts, []Part{{Kind: PartToolCall, Text: "read"}}) {
		t.Fatalf("parts of the next call %v", parts)
	}
}

func TestTheReplyBuilderAssemblesToolCallsInTheOrderTheModelWroteThem(t *testing.T) {
	builder := &ReplyBuilder{}
	builder.Add(StreamChunk{ToolCalls: []ToolCallDelta{{ID: "b", Index: 1, Name: "grep"}}})
	builder.Add(StreamChunk{ToolCalls: []ToolCallDelta{{Arguments: `{"pa`, ID: "a", Index: 0, Name: "read"}}})
	builder.Add(StreamChunk{ToolCalls: []ToolCallDelta{{Arguments: `th":"x"}`, Index: 0}}})
	builder.Add(StreamChunk{ToolCalls: []ToolCallDelta{{Arguments: "{}", Index: 1}}})
	want := []protocol.ToolCall{{Arguments: `{"path":"x"}`, ID: "a", Name: "read"}, {Arguments: "{}", ID: "b", Name: "grep"}}
	if got := builder.Build(func() string { return "generated" }).ToolCalls; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls %+v", got)
	}
}

func TestTheReplyBuilderGivesACallWithoutAnIDOneAndKeepsTheUsage(t *testing.T) {
	builder := &ReplyBuilder{}
	builder.Add(StreamChunk{ToolCalls: []ToolCallDelta{{Arguments: "{}", Index: 0, Name: "glob"}}})
	builder.Add(StreamChunk{Usage: &protocol.Usage{CompletionTokens: 5, PromptTokens: 100}})
	reply := builder.Build(func() string { return "generated" })
	if reply.ToolCalls[0].ID != "generated" || *reply.Usage != (protocol.Usage{CompletionTokens: 5, PromptTokens: 100}) {
		t.Fatalf("reply %+v", reply)
	}
}

func TestARefusalTellsAFullContextApart(t *testing.T) {
	refusal := refusalOf(400, []byte(`{"error":{"message":"the request exceeds the available context size","type":"exceed_context_size_error"}}`), "Bad Request")
	if !refusal.ContextFull || refusal.Message != "llm-server answered 400: the request exceeds the available context size" {
		t.Fatalf("got %+v", refusal)
	}
	if refusalOf(500, []byte(`{"error":"boom"}`), "").ContextFull {
		t.Fatal("a plain error counted as a full context")
	}
}
