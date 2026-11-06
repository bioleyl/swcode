// Package testapp runs the daemon on temporary folders for the tests, talking to a fake llm-server that plays
// the scripted replies.
package testapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"swcode/daemon/app"
	"swcode/daemon/browser"
	"swcode/daemon/config"
	"swcode/daemon/fakellama"
	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
)

const (
	Token         = "test-token-0123456789abcdef"
	Model         = "qwen3-coder-test"
	contextTokens = 32_768
	waitLimit     = 5 * time.Second
	poll          = 10 * time.Millisecond
)

// Options is what a test sets up.
type Options struct {
	Browser       browser.Browser
	Replies       []fakellama.Reply
	Scripts       map[string][]fakellama.Reply
	Slots         int
	Vision        bool
	ContextTokens int
	SubagentModel string
	Model         string
	// Files are put in the workspace, by path.
	Files map[string]string
	// DataDir is a data folder to reuse, to see what a restarted daemon finds.
	DataDir string
	Router  []fakellama.RouterModel
	// ConnectHost is how the app reaches other hosts: a fake remote, say.
	ConnectHost func(protocol.HostRef) hosts.Host
	MaxSteps    int
	Environment map[string]string
	// Home is this computer's home folder, where Claude Code's ~/.claude is: an empty one of its own otherwise.
	Home string
	// Settings are more settings, as the environment names them.
	Settings map[string]string
	// Startup starts the daemon with its computer, as the Windows app does.
	Startup app.StartupPort
}

// App is a daemon under test.
type App struct {
	t         *testing.T
	App       *app.App
	Llama     *fakellama.Server
	Server    *httptest.Server
	Workspace string
	DataDir   string
	Home      string
	mu        sync.Mutex
	messages  []protocol.ServerMessage
	restarts  chan struct{}
	stops     chan struct{}
}

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for path, text := range files {
		target := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func orTemp(t *testing.T, folder string) string {
	if folder != "" {
		return folder
	}
	return t.TempDir()
}

// Start starts a daemon for a test; it stops when the test ends.
func Start(t *testing.T, options Options) *App {
	t.Helper()
	workspace := t.TempDir()
	dataDir, home := orTemp(t, options.DataDir), orTemp(t, options.Home)
	write(t, workspace, options.Files)
	if options.ContextTokens == 0 {
		options.ContextTokens = contextTokens
	}
	llama := fakellama.Start(fakellama.Options{
		ContextTokens: options.ContextTokens, Models: []string{Model}, Replies: options.Replies, Router: options.Router,
		Scripts: options.Scripts, Slots: options.Slots, Vision: options.Vision,
	})
	environment := map[string]string{
		"LLAMA_MODEL": options.Model, "LLAMA_SUBAGENT_MODEL": options.SubagentModel, "LLAMA_URL": llama.URL,
		"SWCODE_DATA_DIR": dataDir, "SWCODE_ENV": "test", "SWCODE_HOST": "127.0.0.1", "SWCODE_TOKEN": Token,
		"SWCODE_WORKSPACE": workspace,
	}
	for name, value := range options.Settings {
		environment[name] = value
	}
	settings, err := config.Load(environment, config.Defaults{Cwd: workspace, Home: home})
	if err != nil {
		t.Fatal(err)
	}
	test := &App{DataDir: dataDir, Home: home, Llama: llama, Workspace: workspace, restarts: make(chan struct{}, 1), stops: make(chan struct{}, 1), t: t}
	environmentOf := options.Environment
	if environmentOf == nil {
		environmentOf = map[string]string{}
	}
	daemon, err := app.New(settings, app.Options{
		Browser: options.Browser, ConnectHost: options.ConnectHost, Defaults: config.Defaults{Cwd: workspace, Home: home},
		// A test starts without skills, so the model's prompts are only what the test sets up.
		DefaultSkills: fstest.MapFS{}, Environment: environmentOf, Home: home, MaxSteps: options.MaxSteps,
		OnRestart: func() { test.restarts <- struct{}{} }, OnStop: func() { test.stops <- struct{}{} },
		Startup: options.Startup,
	})
	if err != nil {
		t.Fatal(err)
	}
	test.App = daemon
	daemon.Sessions.Subscribe(func(message protocol.ServerMessage) {
		test.mu.Lock()
		defer test.mu.Unlock()
		test.messages = append(test.messages, message)
	})
	test.Server = httptest.NewServer(daemon.Handler)
	t.Cleanup(test.Stop)
	return test
}

// Stop stops the daemon; the folders stay with the test's.
func (a *App) Stop() {
	a.Server.Close()
	a.App.Close()
	a.Llama.Stop()
}

// Messages is every message the apps were told, in order.
func (a *App) Messages() []protocol.ServerMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]protocol.ServerMessage{}, a.messages...)
}

// NewSession makes a session; a refused request fails the test.
func (a *App) NewSession(request sessions.Request) protocol.SessionSummary {
	a.t.Helper()
	session, problem := a.App.Sessions.Create(context.Background(), request)
	if problem != "" {
		a.t.Fatal(problem)
	}
	return session
}

// Events is a session's events.
func (a *App) Events(sessionID string) []protocol.SessionEvent {
	history, ok := a.App.Sessions.History(sessionID)
	if !ok {
		return nil
	}
	return history.Value.(protocol.ServerMessageSessionHistory).Events
}

// Until waits for found to say yes.
func (a *App) Until(what string, found func() bool) {
	a.t.Helper()
	deadline := time.Now().Add(waitLimit)
	for !found() {
		if time.Now().After(deadline) {
			a.t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(poll)
	}
}

// UntilIdle waits for the session to have no running turn.
func (a *App) UntilIdle(sessionID string) {
	a.t.Helper()
	a.Until("the session to be idle", func() bool {
		for _, session := range a.App.Sessions.List() {
			if session.ID == sessionID {
				return session.Status == protocol.SessionStatusIdle
			}
		}
		return false
	})
}

// WaitForEvent waits for count events of this kind, in any session: the last of them.
func (a *App) WaitForEvent(kind string, count int) protocol.SessionEvent {
	a.t.Helper()
	var found protocol.SessionEvent
	a.Until(fmt.Sprintf("%d %s", count, kind), func() bool {
		seen := 0
		for _, message := range a.Messages() {
			event, ok := message.Value.(protocol.ServerMessageSessionEvent)
			if ok && protocol.Kind(event.Event) == kind {
				seen++
				if seen == count {
					found = event.Event
					return true
				}
			}
		}
		return false
	})
	return found
}

// WaitForRestart waits for saved settings to ask the daemon to restart.
func (a *App) WaitForRestart() {
	a.t.Helper()
	select {
	case <-a.restarts:
	case <-time.After(waitLimit):
		a.t.Fatal("no restart")
	}
}

// WaitForStop waits for someone to ask the daemon to stop.
func (a *App) WaitForStop() {
	a.t.Helper()
	select {
	case <-a.stops:
	case <-time.After(waitLimit):
		a.t.Fatal("no stop")
	}
}

// ReadFile is a file of the workspace.
func (a *App) ReadFile(path string) string {
	data, _ := os.ReadFile(filepath.Join(a.Workspace, path))
	return string(data)
}

// Call calls a route with the token: the status, and the answer as JSON values.
func (a *App) Call(method, path string, body any) (int, any) {
	a.t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request, _ := http.NewRequest(method, a.Server.URL+path, reader)
	request.Header.Set("authorization", protocol.BearerPrefix+Token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		a.t.Fatal(err)
	}
	defer response.Body.Close()
	var answer any
	_ = json.NewDecoder(response.Body).Decode(&answer)
	return response.StatusCode, answer
}

// Requests is what the agent sent llm-server, in order.
func (a *App) Requests() []map[string]any { return a.Llama.Requests() }

// Kinds is the kinds of events, in order.
func Kinds(events []protocol.SessionEvent) []string {
	kinds := []string{}
	for _, event := range events {
		kinds = append(kinds, protocol.Kind(event))
	}
	return kinds
}

// Finished is the calls that finished, in order.
func Finished(events []protocol.SessionEvent) []protocol.SessionEventToolFinished {
	finished := []protocol.SessionEventToolFinished{}
	for _, event := range events {
		if call, ok := event.Value.(protocol.SessionEventToolFinished); ok {
			finished = append(finished, call)
		}
	}
	return finished
}

// OfKind is the events of a kind, as JSON values.
func OfKind(events []protocol.SessionEvent, kind string) []any {
	found := []any{}
	for _, event := range events {
		if protocol.Kind(event) == kind {
			found = append(found, Value(event))
		}
	}
	return found
}

// Value is anything as JSON values, to match with Matches.
func Value(value any) any {
	data, _ := json.Marshal(value)
	var decoded any
	_ = json.Unmarshal(data, &decoded)
	return decoded
}

// Matches tells whether got has what want has, as toMatchObject does: objects may hold more keys, lists are
// matched item by item, and a want written as a JSON text is read first.
func Matches(got, want any) bool {
	if text, ok := want.(string); ok && (strings.HasPrefix(strings.TrimSpace(text), "{") || strings.HasPrefix(strings.TrimSpace(text), "[")) {
		var decoded any
		if json.Unmarshal([]byte(text), &decoded) == nil {
			want = decoded
		}
	}
	return matches(Value(got), Value(want))
}

func matches(got, want any) bool {
	switch expected := want.(type) {
	case map[string]any:
		object, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for key, value := range expected {
			if !matches(object[key], value) {
				return false
			}
		}
		return true
	case []any:
		list, ok := got.([]any)
		if !ok || len(list) != len(expected) {
			return false
		}
		for index := range expected {
			if !matches(list[index], expected[index]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(got, want)
}

// Expect fails the test when got does not match want.
func Expect(t *testing.T, got, want any) {
	t.Helper()
	if !Matches(got, want) {
		data, _ := json.MarshalIndent(Value(got), "", "  ")
		t.Fatalf("got %s\nwant %v", data, want)
	}
}

// RequestText is a request as JSON text, to look for what it holds.
func RequestText(request map[string]any) string {
	return fakellama.JSONText(request)
}
