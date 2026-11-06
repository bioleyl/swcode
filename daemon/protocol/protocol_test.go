package protocol

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEventsCarryTheirKindAndNeverWriteNullForAList(t *testing.T) {
	event := SessionEvent{Value: SessionEventAssistantMessage{At: "2026-11-02T10:00:00.000Z", ID: "m1", Text: "Hi"}}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"kind":"assistantMessage","at":"2026-11-02T10:00:00.000Z","id":"m1","text":"Hi","toolCalls":[]}`
	if string(data) != want {
		t.Fatalf("got %s, want %s", data, want)
	}
	var back SessionEvent
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	message, ok := back.Value.(SessionEventAssistantMessage)
	if !ok || message.Text != "Hi" {
		t.Fatalf("read back %#v", back.Value)
	}
}

func TestAMessageFromTheAppIsTrimmedAndChecked(t *testing.T) {
	var message ClientMessage
	if err := json.Unmarshal([]byte(`{"type":"renameSession","sessionId":"s","title":"  New title  "}`), &message); err != nil {
		t.Fatal(err)
	}
	if rename := message.Value.(ClientMessageRenameSession); rename.Title != "New title" {
		t.Fatalf("title %q", rename.Title)
	}
	err := json.Unmarshal([]byte(`{"type":"renameSession","sessionId":"s","title":"   "}`), &message)
	if err == nil || !strings.Contains(err.Error(), "title: must not be empty") {
		t.Fatalf("an empty title passed: %v", err)
	}
	err = json.Unmarshal([]byte(`{"type":"setMode","sessionId":"s","mode":"yolo"}`), &message)
	if err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("an unknown mode passed: %v", err)
	}
	if err := json.Unmarshal([]byte(`{"type":"nothing"}`), &message); err == nil {
		t.Fatal("an unknown type passed")
	}
}

func TestAMissingFieldTakesItsDefault(t *testing.T) {
	var project Project
	if err := json.Unmarshal([]byte(`{"createdAt":"2026-11-02T10:00:00.000Z","id":"p","name":"app","path":"/app"}`), &project); err != nil {
		t.Fatal(err)
	}
	if _, local := project.Host.Value.(HostRefLocal); !local {
		t.Fatalf("host %#v", project.Host.Value)
	}
}

func TestANewFolderNameSaysWhatItWants(t *testing.T) {
	var create ClientMessageCreateProject
	err := json.Unmarshal([]byte(`{"folder":"/work","newFolder":"apps/web"}`), &create)
	if err == nil || !strings.Contains(err.Error(), "a folder name, without / or") {
		t.Fatalf("got %v", err)
	}
}

type kindNames struct{}

func (kindNames) Local(HostRefLocal) string   { return "local" }
func (kindNames) WSL(value HostRefWSL) string { return "wsl " + value.Distro }
func (kindNames) SSH(value HostRefSSH) string { return "ssh " + value.Endpoint }

func TestAMapperHasAMethodPerVariant(t *testing.T) {
	if got := MapHostRef[string](HostRef{Value: HostRefWSL{Distro: "Ubuntu"}}, kindNames{}); got != "wsl Ubuntu" {
		t.Fatalf("got %q", got)
	}
}

func TestTimestampsAreWrittenAsJavaScriptDoes(t *testing.T) {
	at := time.Date(2026, 11, 2, 10, 5, 3, 120_000_000, time.FixedZone("CET", 3600))
	if got := Timestamp(at); got != "2026-11-02T09:05:03.120Z" {
		t.Fatalf("got %s", got)
	}
}
