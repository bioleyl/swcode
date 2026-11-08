package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"swcode/daemon/agent"
	"swcode/daemon/hosts"
	"swcode/daemon/llm"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
	"swcode/daemon/tools"
)

const (
	at                 = "2026-10-23T10:00:00.000Z"
	folder             = "/work/swcode"
	restartAt          = "2026-10-23T11:00:00.000Z"
	restartedOnRestart = restarted
)

// events makes session events out of variants, all at the same time.
func events(variants ...protocol.SessionEventVariant) []protocol.SessionEvent {
	list := []protocol.SessionEvent{}
	for _, variant := range variants {
		list = append(list, protocol.SessionEvent{Value: protocol.Dated(variant, at)})
	}
	return list
}

// kindsOf is the kind of each event, to see what a turn wrote, in order.
func kindsOf(list []protocol.SessionEvent) []string {
	kinds := []string{}
	for _, event := range list {
		kinds = append(kinds, protocol.Kind(event))
	}
	return kinds
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// memory is the sessions kept in memory: the store the service works through, so a test sees what would be on disk
// without a folder of its own.
type memory struct {
	// mu: the service writes from its turns' goroutines while a test reads.
	mu     sync.Mutex
	metas  map[string]Meta
	events map[string][]protocol.SessionEvent
	// broken: saving what a session is fails, as on a disk that will not take it.
	broken bool
}

func newMemory() *memory {
	return &memory{events: map[string][]protocol.SessionEvent{}, metas: map[string]Meta{}}
}

func (m *memory) LoadAll() []Stored {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored := []Stored{}
	for id, meta := range m.metas {
		stored = append(stored, Stored{Events: append([]protocol.SessionEvent{}, m.events[id]...), Meta: meta})
	}
	return stored
}

func (m *memory) SaveMeta(meta Meta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.broken {
		return errors.New("the folder is not there")
	}
	m.metas[meta.ID] = meta
	return nil
}

func (m *memory) Append(sessionID string, event protocol.SessionEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events[sessionID] = append(m.events[sessionID], event)
	return nil
}

func (m *memory) Remove(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.metas, sessionID)
	delete(m.events, sessionID)
}

// eventsOf is a copy of a session's events as the store holds them now.
func (m *memory) eventsOf(sessionID string) []protocol.SessionEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]protocol.SessionEvent{}, m.events[sessionID]...)
}

// metaOf is what the store holds of a session now, and whether it holds it.
func (m *memory) metaOf(sessionID string) (Meta, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	meta, ok := m.metas[sessionID]
	return meta, ok
}

// words is a model that answers each request with the next of its texts, then fails with its problem.
type words struct {
	mu      sync.Mutex
	replies []string
	problem string
	asked   int
}

func (w *words) Complete(ctx context.Context, _ llm.Request, onPart func(llm.Part)) (llm.Reply, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if ctx.Err() != nil {
		return llm.Reply{}, ctx.Err()
	}
	if w.asked < len(w.replies) {
		reply := w.replies[w.asked]
		w.asked++
		onPart(llm.Part{Kind: llm.PartText, Text: reply})
		return llm.Reply{Text: reply}, nil
	}
	if w.problem != "" {
		return llm.Reply{}, errors.New(w.problem)
	}
	return llm.Reply{Text: "Nothing more to say."}, nil
}

// waiter is a model that answers when it is told to, so a turn waits in the middle of it.
type waiter struct {
	release chan struct{}
}

func (w *waiter) Complete(ctx context.Context, _ llm.Request, _ func(llm.Part)) (llm.Reply, error) {
	select {
	case <-w.release:
		return llm.Reply{Text: "Answered."}, nil
	case <-ctx.Done():
		return llm.Reply{}, ctx.Err()
	}
}

// presets is the models the service picks for a session, the same for every preset.
type presets struct{ main, subagent llm.Choice }

func (p presets) Choose(context.Context, string) (llm.Choice, llm.Choice) { return p.main, p.subagent }

func (presets) SubagentLimit(string) *protocol.SubagentLimit { return nil }

func (presets) Has(preset string) bool { return preset == "" || preset == "default" }

// heard keeps every message the service sends to the apps.
type heard struct {
	mu       sync.Mutex
	messages []protocol.ServerMessage
}

func (h *heard) listen(message protocol.ServerMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.messages = append(h.messages, message)
}

// ofType is the payload of each message of a type, as JSON.
func (h *heard) ofType(kind string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	found := []string{}
	for _, message := range h.messages {
		data, err := json.Marshal(message.Value)
		if err != nil {
			continue
		}
		var head struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &head) == nil && head.Type == kind {
			found = append(found, string(data))
		}
	}
	return found
}

// chats starts a service of chats with the test's model, on a store of memory, and returns it with what it wrote.
func chats(t *testing.T, model llm.Model) (*Service, *memory, *heard) {
	t.Helper()
	return onStore(t, newMemory(), model)
}

// onStore is the same on a store the test filled first, as after a restart.
func onStore(t *testing.T, store *memory, model llm.Model) (*Service, *memory, *heard) {
	t.Helper()
	listener := &heard{}
	service := NewService(Deps{
		ChatTools: tools.Main, Hosts: hosts.NewRegistry(hosts.NewLocalHost(hosts.LocalOptions{ScratchRoot: t.TempDir()}), nil),
		ImagesFolder: t.TempDir(), LogError: func(error) {}, MaxSteps: 4, Model: model,
		Models: presets{main: llm.Choice{ContextTokens: 8192, Model: "tester"}}, NewID: ids(),
		Now: func() time.Time { return time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC) }, Store: store,
		Thinking: Thinking{Session: protocol.ThinkingModeOff, Subagents: protocol.ThinkingModeOff}, Workspace: folder,
	})
	service.Subscribe(listener.listen)
	return service, store, listener
}

// ids gives a new id for each call, so requests and events can be told apart.
func ids() func() string {
	next := 0
	return func() string {
		next++
		return string(rune('a' + next))
	}
}

// newChat makes a chat with no first message, and returns its id.
func newChat(t *testing.T, service *Service) string {
	t.Helper()
	session, problem := service.Create(context.Background(), Request{Kind: protocol.SessionKindChat})
	if problem != "" {
		t.Fatal(problem)
	}
	return session.ID
}

// until waits for the session to reach a status.
func until(t *testing.T, service *Service, sessionID string, want protocol.SessionStatus) {
	t.Helper()
	for attempt := 0; attempt < 400; attempt++ {
		for _, summary := range service.List() {
			if summary.ID == sessionID && summary.Status == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the session never became %s", want)
}

func history(t *testing.T, service *Service, sessionID string) []protocol.SessionEvent {
	t.Helper()
	message, ok := service.History(sessionID, 0)
	if !ok {
		t.Fatal("no such session")
	}
	list, ok := message.Value.(protocol.ServerMessageSessionHistory)
	if !ok {
		t.Fatalf("a history of another kind: %T", message.Value)
	}
	return list.Events
}

// lastRequestID is the id of the last request event of a kind the store holds, "" when there is none yet.
func lastRequestID(store *memory, sessionID, kind string) string {
	events := store.eventsOf(sessionID)
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if protocol.Kind(event) != kind {
			continue
		}
		data, err := json.Marshal(event.Value)
		if err != nil {
			continue
		}
		var request struct {
			RequestID string `json:"requestId"`
		}
		if json.Unmarshal(data, &request) == nil && request.RequestID != "" {
			return request.RequestID
		}
	}
	return ""
}

// askedID waits for the turn's request to reach the store, and returns its id.
func askedID(t *testing.T, store *memory, sessionID, kind string) string {
	t.Helper()
	for attempt := 0; attempt < 400; attempt++ {
		if id := lastRequestID(store, sessionID, kind); id != "" {
			return id
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no %s event", kind)
	return ""
}

func TestANewChatIsIdleTitledAndKept(t *testing.T) {
	service, store, _ := chats(t, &words{})
	sessionID := newChat(t, service)
	if meta, _ := store.metaOf(sessionID); meta.Kind != protocol.SessionKindChat {
		t.Fatalf("kept as %+v", meta)
	}
	summaries := service.List()
	if len(summaries) != 1 || summaries[0].Title != Untitled || summaries[0].Status != protocol.SessionStatusIdle {
		t.Fatalf("summaries %+v", summaries)
	}
	if problem := service.Rename("nope", "x"); problem != unknownSession {
		t.Fatalf("renaming another session: %q", problem)
	}
}

func TestTheFirstMessageNamesTheSessionAndTheTurnWritesItsEvents(t *testing.T) {
	service, store, listener := chats(t, &words{replies: []string{"All good."}})
	sessionID := newChat(t, service)
	if problem := service.SendPrompt(sessionID, "Fix the notes\nand say why", nil); problem != "" {
		t.Fatal(problem)
	}
	until(t, service, sessionID, protocol.SessionStatusIdle)
	if meta, _ := store.metaOf(sessionID); meta.Title != "Fix the notes" {
		t.Fatalf("title %q", meta.Title)
	}
	want := []string{"turnStarted", "userMessage", "assistantMessage", "turnFinished"}
	if got := kindsOf(history(t, service, sessionID)); !reflect.DeepEqual(got, want) {
		t.Fatalf("events %v", got)
	}
	if got := kindsOf(store.eventsOf(sessionID)); !reflect.DeepEqual(got, want) {
		t.Fatalf("the file holds %v", got)
	}
	if got := service.List()[0]; got.UpdatedAt != at {
		t.Fatalf("updated %q", got.UpdatedAt)
	}
	if len(listener.ofType("sessionEvent")) != len(want) {
		t.Fatalf("the apps got %d events", len(listener.ofType("sessionEvent")))
	}
}

func TestAMessageOfNothingIsRefused(t *testing.T) {
	service, _, _ := chats(t, &words{})
	sessionID := newChat(t, service)
	if problem := service.SendPrompt(sessionID, "", nil); problem != blankMessage {
		t.Fatalf("problem %q", problem)
	}
	if len(history(t, service, sessionID)) != 0 {
		t.Fatal("a turn started")
	}
}

func TestWhatIsSentWhileATurnRunsWaitsForTheNextOne(t *testing.T) {
	service, store, _ := chats(t, &words{replies: []string{"First.", "Second."}})
	sessionID := newChat(t, service)
	if problem := service.SendPrompt(sessionID, "one", nil); problem != "" {
		t.Fatal(problem)
	}
	if problem := service.SendPrompt(sessionID, "two", nil); problem != "" {
		t.Fatal(problem)
	}
	if got := service.List()[0]; len(got.Queued) != 1 || got.Queued[0] != "two" {
		t.Fatalf("queued %+v", got)
	}
	until(t, service, sessionID, protocol.SessionStatusIdle)
	turns := 0
	for _, kind := range kindsOf(store.eventsOf(sessionID)) {
		if kind == "turnStarted" {
			turns++
		}
	}
	if turns != 2 {
		t.Fatalf("%d turns", turns)
	}
	if problem := service.ClearQueue(sessionID); problem != "" {
		t.Fatal(problem)
	}
}

func TestStoppingClearsTheQueue(t *testing.T) {
	release := make(chan struct{})
	service, _, _ := chats(t, &waiter{release: release})
	defer close(release)
	sessionID := newChat(t, service)
	if problem := service.SendPrompt(sessionID, "one", nil); problem != "" {
		t.Fatal(problem)
	}
	if problem := service.SendPrompt(sessionID, "two", nil); problem != "" {
		t.Fatal(problem)
	}
	if problem := service.Stop(sessionID); problem != "" {
		t.Fatal(problem)
	}
	if got := service.List()[0]; len(got.Queued) != 0 || got.QueuedNow {
		t.Fatalf("summary %+v", got)
	}
}

func TestTheSettingsAreSavedAndToldToTheApps(t *testing.T) {
	service, store, listener := chats(t, &words{})
	sessionID := newChat(t, service)
	if problem := service.SetMode(sessionID, protocol.PermissionModeAsk); problem != "" {
		t.Fatal(problem)
	}
	if problem := service.SetThinking(sessionID, protocol.ThinkingModeLow); problem != "" {
		t.Fatal(problem)
	}
	if problem := service.SetConcise(sessionID, true); problem != "" {
		t.Fatal(problem)
	}
	if problem := service.SetPreset(sessionID, "other"); problem != unknownPreset {
		t.Fatalf("preset %q", problem)
	}
	if problem := service.SetPreset(sessionID, "default"); problem != "" {
		t.Fatal(problem)
	}
	meta, _ := store.metaOf(sessionID)
	if meta.Mode != protocol.PermissionModeAsk || meta.Thinking != protocol.ThinkingModeLow || !meta.Concise || meta.Preset != "default" {
		t.Fatalf("meta %+v", meta)
	}
	if got := service.List()[0]; got.Mode != protocol.PermissionModeAsk || !got.Concise {
		t.Fatalf("summary %+v", got)
	}
	if len(listener.ofType("sessions")) == 0 {
		t.Fatal("the apps were never told")
	}
	if problem := service.SetMode("nope", protocol.PermissionModeAuto); problem != unknownSession {
		t.Fatalf("another session: %q", problem)
	}
}

func TestDeletingAWorkingSessionIsRefused(t *testing.T) {
	release := make(chan struct{})
	service, store, _ := chats(t, &waiter{release: release})
	defer close(release)
	sessionID := newChat(t, service)
	if problem := service.SendPrompt(sessionID, "one", nil); problem != "" {
		t.Fatal(problem)
	}
	if problem := service.Remove(sessionID); problem != stillWorking {
		t.Fatalf("while working: %q", problem)
	}
	service.StopAll()
	if problem := service.Remove(sessionID); problem != "" {
		t.Fatal(problem)
	}
	if _, ok := store.metaOf(sessionID); ok {
		t.Fatal("its file stayed")
	}
	if len(service.List()) != 0 {
		t.Fatal("still listed")
	}
}

func TestWhatThePersonSawIsKeptForEveryDevice(t *testing.T) {
	service, store, listener := chats(t, &words{})
	sessionID := newChat(t, service)
	if got := service.List()[0]; got.SeenAt != at {
		t.Fatalf("a new session is seen as made: %+v", got)
	}
	const later = "2026-10-23T10:05:00.000Z"
	if problem := service.MarkSeen(sessionID, later); problem != "" {
		t.Fatal(problem)
	}
	meta, _ := store.metaOf(sessionID)
	if meta.SeenAt != later || service.List()[0].SeenAt != later {
		t.Fatalf("seen at %q, listed %q", meta.SeenAt, service.List()[0].SeenAt)
	}
	told := len(listener.ofType("sessions"))
	// A device that saw less than another already did changes nothing, and tells nothing.
	if problem := service.MarkSeen(sessionID, at); problem != "" {
		t.Fatal(problem)
	}
	if meta, _ := store.metaOf(sessionID); meta.SeenAt != later || len(listener.ofType("sessions")) != told {
		t.Fatalf("went back to %q", meta.SeenAt)
	}
	if problem := service.MarkSeen(sessionID, "yesterday"); problem == "" {
		t.Fatal("took a word for a time")
	}
	if problem := service.MarkSeen("nope", later); problem != unknownSession {
		t.Fatalf("another session: %q", problem)
	}
}

func TestASessionFromBeforeWhatWasSeenIsSeenAsItWasOnce(t *testing.T) {
	const earlier = "2026-10-23T09:00:00.000Z"
	store := newMemory()
	meta := Meta{
		CreatedAt: earlier, Host: protocol.LocalHost(), ID: "s1", Kind: protocol.SessionKindChat, Mode: protocol.PermissionModeAuto,
		Thinking: protocol.ThinkingModeOff, Title: "From before", Workspace: folder,
	}
	store.metas["s1"] = meta
	store.events["s1"] = events(protocol.SessionEventTurnStarted{ID: "t1"}, protocol.SessionEventTurnFinished{ID: "t1", Outcome: protocol.TurnOutcomeDone})
	// Another left halfway: the restart closes its turn after what was seen, which is then news.
	meta.ID, meta.CreatedAt = "s2", earlier
	store.metas["s2"] = meta
	store.events["s2"] = []protocol.SessionEvent{{Value: protocol.Dated(protocol.SessionEventTurnStarted{ID: "t2"}, earlier)}}
	service, _, _ := onStore(t, store, &words{})
	service.Load()
	if kept, _ := store.metaOf("s1"); kept.SeenAt != at {
		t.Fatalf("seen at %q, kept for the next start", kept.SeenAt)
	}
	for _, summary := range service.List() {
		seen := summary.SeenAt == summary.UpdatedAt
		if seen != (summary.ID == "s1") {
			t.Fatalf("%s seen %v: %+v", summary.ID, seen, summary)
		}
	}
}

func TestADaemonThatStoppedInATurnClosesItAsStopped(t *testing.T) {
	store := newMemory()
	store.metas["s1"] = Meta{
		CreatedAt: at, Host: protocol.LocalHost(), ID: "s1", Kind: protocol.SessionKindChat, Mode: protocol.PermissionModeAuto,
		Thinking: protocol.ThinkingModeOff, Title: "Half a turn", Workspace: folder,
	}
	store.events["s1"] = events(
		protocol.SessionEventTurnStarted{ID: "t1"},
		protocol.SessionEventUserMessage{ID: "u1", Text: "Work"},
		protocol.SessionEventAssistantMessage{ID: "a1", Text: "I am working"},
	)
	service, _, _ := onStore(t, store, &words{})
	service.Load()
	list := history(t, service, "s1")
	if got := kindsOf(list); len(got) != 4 || got[3] != "turnFinished" {
		t.Fatalf("events %v", got)
	}
	last := list[3].Value.(protocol.SessionEventTurnFinished)
	if last.ID != "t1" || last.Outcome != protocol.TurnOutcomeStopped || !strings.Contains(last.Message, restartedOnRestart) {
		t.Fatalf("closed as %+v", last)
	}
	if service.List()[0].Status != protocol.SessionStatusIdle {
		t.Fatal("working after a restart")
	}
}

func TestATurnWhoseModelDoesNotAnswerStillEnds(t *testing.T) {
	service, _, _ := chats(t, &words{problem: "llm-server is not answering"})
	sessionID := newChat(t, service)
	if problem := service.SendPrompt(sessionID, "one", nil); problem != "" {
		t.Fatal(problem)
	}
	until(t, service, sessionID, protocol.SessionStatusIdle)
	list := history(t, service, sessionID)
	last := list[len(list)-1].Value.(protocol.SessionEventTurnFinished)
	if last.Outcome != protocol.TurnOutcomeFailed {
		t.Fatalf("ended as %+v", last)
	}
	if problem := service.SendPrompt(sessionID, "again", nil); problem != "" {
		t.Fatalf("the session is stuck: %q", problem)
	}
}

// texts is the text of each user message of a history, to see which events a page holds.
func texts(t *testing.T, list []protocol.SessionEvent) []string {
	t.Helper()
	got := []string{}
	for _, event := range list {
		if message, ok := event.Value.(protocol.SessionEventUserMessage); ok {
			got = append(got, message.Text)
		}
	}
	return got
}

// pageOf is what a page of history holds: the text of its messages, and how many older events the daemon kept
// back. The tests ask about a session they made, so it answers.
func pageOf(t *testing.T, message protocol.ServerMessage) ([]string, int) {
	t.Helper()
	history, ok := message.Value.(protocol.ServerMessageSessionHistory)
	if !ok {
		t.Fatalf("a history of another kind: %T", message.Value)
	}
	return texts(t, history.Events), history.KeptBack
}

// pagesOf is the same for what History or OlderEvents answered, which the tests expect to be there.
func pagesOf(t *testing.T, message protocol.ServerMessage, found bool) ([]string, int) {
	t.Helper()
	if !found {
		t.Fatal("the session answered no history")
	}
	return pageOf(t, message)
}

func TestAHistoryOfTheEndKeepsTheOlderEventsBack(t *testing.T) {
	service, _, _ := chats(t, &words{replies: []string{"One.", "Two.", "Three.", "Four.", "Five."}})
	sessionID := newChat(t, service)
	for index := 1; index <= 5; index++ {
		if problem := service.SendPrompt(sessionID, string(rune('a'+index)), nil); problem != "" {
			t.Fatal(problem)
		}
		until(t, service, sessionID, protocol.SessionStatusIdle)
	}
	all, _ := service.History(sessionID, 0)
	if got, kept := pageOf(t, all); len(got) != 5 || kept != 0 {
		t.Fatalf("without a limit: %v, %d kept back", got, kept)
	}
	// Each turn writes four events: its start, the person's message, the model's answer, its end.
	last, _ := service.History(sessionID, 2)
	if got, kept := pageOf(t, last); len(got) != 0 || kept != 18 {
		t.Fatalf("the last two events, an answer and its end: %v, %d kept back", got, kept)
	}
	turn, _ := service.History(sessionID, 4)
	if got, kept := pageOf(t, turn); !reflect.DeepEqual(got, []string{"f"}) || kept != 16 {
		t.Fatalf("the last turn: %v, %d kept back", got, kept)
	}
	over, _ := service.History(sessionID, 50)
	if got, kept := pageOf(t, over); len(got) != 5 || kept != 0 {
		t.Fatalf("a limit past the session: %v, %d kept back", got, kept)
	}
}

func TestTheOlderEventsComeUpFromTheEndOfTheHistory(t *testing.T) {
	service, _, _ := chats(t, &words{replies: []string{"One.", "Two.", "Three.", "Four.", "Five."}})
	sessionID := newChat(t, service)
	for index := 1; index <= 5; index++ {
		if problem := service.SendPrompt(sessionID, string(rune('a'+index)), nil); problem != "" {
			t.Fatal(problem)
		}
		until(t, service, sessionID, protocol.SessionStatusIdle)
	}
	// The page a person opens with: the last four events, and as many as the session holds before them.
	end, found := service.History(sessionID, 4)
	if _, kept := pagesOf(t, end, found); kept == 0 {
		t.Fatal("a session of five turns has events to keep back")
	}
	// The page above it holds what the session had before those, and how many are left before that.
	next, found := service.OlderEvents(sessionID, 16, 8)
	if got, older := pagesOf(t, next, found); len(got) == 0 || older >= 16 {
		t.Fatalf("the next page up: %v, %d kept back", got, older)
	}
	// A page that runs past the beginning holds what is left and says nothing is.
	first, found := service.OlderEvents(sessionID, 2, 40)
	if got, older := pagesOf(t, first, found); len(got) == 0 || older != 0 {
		t.Fatalf("past the beginning: %v, %d kept back", got, older)
	}
	// Nothing is older than the whole conversation, and another session has none of its own.
	none, found := service.OlderEvents(sessionID, 200, 4)
	if got, older := pagesOf(t, none, found); len(got) != 0 || older != 0 {
		t.Fatalf("older than all of it: %v, %d kept back", got, older)
	}
	if _, found := service.OlderEvents("nope", 4, 4); found {
		t.Fatal("another session's events")
	}
}

func TestCompactingNeedsAConversationAndNoTurn(t *testing.T) {
	service, store, _ := chats(t, &words{replies: []string{"A summary of the work."}})
	sessionID := newChat(t, service)
	if problem := service.Compact(sessionID); problem != nothingToCompact {
		t.Fatalf("without a conversation: %q", problem)
	}
	if problem := service.SendPrompt(sessionID, "one", nil); problem != "" {
		t.Fatal(problem)
	}
	until(t, service, sessionID, protocol.SessionStatusIdle)
	if problem := service.Compact(sessionID); problem != "" {
		t.Fatal(problem)
	}
	until(t, service, sessionID, protocol.SessionStatusIdle)
	if got := kindsOf(store.eventsOf(sessionID)); !contains(got, "contextCompacted") {
		t.Fatalf("events %v", got)
	}
}

// TestTheKindOfAnAnswerOnlySettlesTheRequestOfThatKind checks the seam the apps use: an answer of the agent's
// questions never settles a permission, nor the other way round, and only the request the turn waits for is taken.
func TestTheKindOfAnAnswerOnlySettlesTheRequestOfThatKind(t *testing.T) {
	questions := questionsAsked{callID: "c1", questions: []protocol.Question{{Header: "Approach", Question: "Map or sort?"}}}
	permission := permissionAsk{ask: agent.PermissionAsk{CallID: "c1", Subject: permissions.Read}}
	password := passwordAsk{callID: "c1", prompt: "Password: "}
	for _, test := range []struct {
		name    string
		pending asked
		got     answer
		want    bool
	}{
		{"a question's answer settles a question", questions, answer{answers: []string{"Map"}}, true},
		{"a question's empty answer settles a question", questions, answer{answers: []string{}}, true},
		{"a permission does not settle a question", questions, Permission(protocol.PermissionDecisionAllowOnce), false},
		{"answers do not settle a permission", permission, answer{answers: []string{"Map"}}, false},
		{"a permission settles a permission", permission, Permission(protocol.PermissionDecisionDeny), true},
		{"a password settles a password", password, Secret("hunter2"), true},
		{"no password settles a password", password, Secret(""), true},
		{"a permission does not settle a password", password, Permission(protocol.PermissionDecisionAllowOnce), false},
		{"a password does not settle a permission", permission, Secret("hunter2"), false},
		{"a password does not settle a question", questions, Secret("hunter2"), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			turn := &runningTurn{pending: &pending{requestID: "r1", settle: func(answer) {}, waitsFor: test.pending.waitsFor}}
			if got := turn.settleOf("r1", test.got) != nil; got != test.want {
				t.Fatalf("took it: %v", got)
			}
			if turn.settleOf("other", test.got) != nil {
				t.Fatal("another request's answer was taken")
			}
			if (&runningTurn{}).settleOf("r1", test.got) != nil {
				t.Fatal("a turn that waits for nothing took an answer")
			}
		})
	}
}

// TestAnAnswerAllowedForTheSessionGrantsWhatItLets: a permission the turn waits for, answered through the service,
// is recorded, and allowed for the session it was asked in.
func TestAnAnswerAllowedForTheSessionGrantsWhatItLets(t *testing.T) {
	service, store, _ := chats(t, &words{})
	sessionID := newChat(t, service)
	service.mu.Lock()
	session := service.sessions[sessionID]
	turn := &runningTurn{}
	// The turn holds the request the way a running one does, so Service.Answer reaches it.
	session.turn = turn
	service.mu.Unlock()
	asked := permissionAsk{ask: agent.PermissionAsk{CallID: "c1", Subject: permissions.Subject{Access: permissions.AccessCommand, Command: "npm test"}}, service: service}
	got := make(chan answer, 1)
	go func() { got <- service.ask(context.Background(), session, turn, asked) }()
	requestID := askedID(t, store, sessionID, "permissionRequested")
	if problem := service.Answer(sessionID, "not it", Permission(protocol.PermissionDecisionAllowSession)); problem != alreadyAnswered {
		t.Fatalf("another request: %q", problem)
	}
	if problem := service.Answer(sessionID, requestID, answer{answers: []string{"Map"}}); problem != alreadyAnswered {
		t.Fatalf("an answer of the other kind: %q", problem)
	}
	if problem := service.Answer(sessionID, requestID, Permission(protocol.PermissionDecisionAllowSession)); problem != "" {
		t.Fatal(problem)
	}
	select {
	case decision := <-got:
		if decision.decision != protocol.PermissionDecisionAllowSession {
			t.Fatalf("answer %+v", decision)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the turn never got its answer")
	}
	if !session.grants.Programs["npm"] {
		t.Fatalf("grants %+v", session.grants)
	}
	kinds := kindsOf(store.eventsOf(sessionID))
	if !contains(kinds, "permissionRequested") || !contains(kinds, "permissionAnswered") {
		t.Fatalf("events %v", kinds)
	}
	if turn.pending != nil {
		t.Fatal("the turn still waits")
	}
}

// TestAPasswordAnsweredThroughTheServiceReachesTheTurnAndIsNotKept: a command's password request, answered through
// the service, lets the turn go on with the words typed, and the session keeps only that a password came.
func TestAPasswordAnsweredThroughTheServiceReachesTheTurnAndIsNotKept(t *testing.T) {
	service, store, _ := chats(t, &words{})
	sessionID := newChat(t, service)
	service.mu.Lock()
	session := service.sessions[sessionID]
	turn := &runningTurn{}
	session.turn = turn
	service.mu.Unlock()
	asked := passwordAsk{callID: "c1", prompt: "[sudo] password for leal:", service: service}
	got := make(chan answer, 1)
	go func() { got <- service.ask(context.Background(), session, turn, asked) }()
	requestID := askedID(t, store, sessionID, "passwordRequested")
	if problem := service.Answer(sessionID, requestID, Permission(protocol.PermissionDecisionAllowOnce)); problem != alreadyAnswered {
		t.Fatalf("an answer of the other kind: %q", problem)
	}
	if problem := service.Answer(sessionID, requestID, Secret("hunter2")); problem != "" {
		t.Fatal(problem)
	}
	select {
	case typed := <-got:
		if typed.secret == nil || *typed.secret != (typedSecret{gave: true, text: "hunter2"}) {
			t.Fatalf("answer %+v", typed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the turn never got its answer")
	}
	kinds := kindsOf(store.eventsOf(sessionID))
	if !contains(kinds, "passwordRequested") || !contains(kinds, "passwordAnswered") {
		t.Fatalf("events %v", kinds)
	}
	for _, event := range store.eventsOf(sessionID) {
		if data, err := json.Marshal(event.Value); err == nil && strings.Contains(string(data), "hunter2") {
			t.Fatalf("the password was kept: %s", data)
		}
	}
	if turn.pending != nil {
		t.Fatal("the turn still waits")
	}
}

// TestAStoppedTurnSettlesWhatItAskedWithANo: the turn's stopping answers its own request, so the agent is let go
// and the answer it gets says nobody answered.
func TestAStoppedTurnSettlesWhatItAskedWithANo(t *testing.T) {
	service, store, _ := chats(t, &words{})
	sessionID := newChat(t, service)
	service.mu.Lock()
	session := service.sessions[sessionID]
	turn := &runningTurn{}
	service.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	asked := questionsAsked{callID: "c1", questions: []protocol.Question{{Header: "Approach", Question: "Map or sort?"}}, service: service}
	got := make(chan answer, 1)
	go func() { got <- service.ask(ctx, session, turn, asked) }()
	for attempt := 0; attempt < 400; attempt++ {
		if len(store.eventsOf(sessionID)) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case answers := <-got:
		if answers.answers == nil || len(answers.answers) != 0 {
			t.Fatalf("answer %+v", answers)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the stopped turn never let go")
	}
	if kinds := kindsOf(store.eventsOf(sessionID)); !contains(kinds, "questionAsked") || !contains(kinds, "questionAnswered") {
		t.Fatalf("events %v", kinds)
	}
	if turn.pending != nil {
		t.Fatal("the turn still waits")
	}
}
