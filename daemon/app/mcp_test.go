package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/mcp"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

// fakeServer is an MCP server at an address, of the kind that shakes hands first: its one tool, greet, says hello to
// the name it is given.
func fakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var asked struct {
			ID     *json.RawMessage `json:"id"`
			Method string           `json:"method"`
			Params struct {
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		_ = json.NewDecoder(request.Body).Decode(&asked)
		if asked.ID == nil {
			response.WriteHeader(http.StatusAccepted)
			return
		}
		result := map[string]any{}
		switch asked.Method {
		case "initialize":
			result = map[string]any{"capabilities": map[string]any{}, "protocolVersion": "2025-06-18"}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{
				"description": "Says hello to someone.",
				"inputSchema": map[string]any{"properties": map[string]any{"name": map[string]any{"type": "string"}}, "type": "object"},
				"name":        "greet",
			}}}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]any{"text": fmt.Sprint("Hello, ", asked.Params.Arguments["name"], "."), "type": "text"}}}
		}
		response.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{"id": asked.ID, "jsonrpc": "2.0", "result": result})
	}))
	t.Cleanup(server.Close)
	return server
}

// withServers is a data folder whose servers file lists these servers, by name.
func withServers(t *testing.T, addresses map[string]string) string {
	t.Helper()
	listed := map[string]any{}
	for name, address := range addresses {
		listed[name] = map[string]any{"url": address}
	}
	dataDir := t.TempDir()
	data, _ := json.Marshal(map[string]any{"mcpServers": listed})
	if err := os.WriteFile(filepath.Join(dataDir, mcp.File), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return dataDir
}

func serversOf(t *testing.T, test *App, method string) protocol.McpStatus {
	t.Helper()
	_, body := test.Call(method, protocol.EndpointMcp, nil)
	data, _ := json.Marshal(body)
	var status protocol.McpStatus
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatal(err)
	}
	return status
}

// untilReady waits for every server of the file to be reached.
func untilReady(t *testing.T, test *App) {
	t.Helper()
	test.Until("the MCP servers to be ready", func() bool {
		status := serversOf(t, test, "GET")
		for _, server := range status.Servers {
			if server.State != protocol.McpServerStateReady {
				return false
			}
		}
		return len(status.Servers) > 0
	})
}

func toolNamesOf(request map[string]any) []string {
	names := []string{}
	listed, _ := request["tools"].([]any)
	for _, each := range listed {
		function, _ := each.(map[string]any)["function"].(map[string]any)
		name, _ := function["name"].(string)
		names = append(names, name)
	}
	return names
}

func TestTheAgentHasTheToolsOfTheServersListedAndAsksBeforeItCallsOne(t *testing.T) {
	server := fakeServer(t)
	test := Start(t, Options{DataDir: withServers(t, map[string]string{"greeter": server.URL}), Replies: []fakellama.Reply{
		call("mcp_greeter_greet", map[string]any{"name": "Ada"}), call("mcp_greeter_greet", map[string]any{"name": "Grace"}), text("Greeted."),
	}})
	untilReady(t, test)

	sessionID := prompt(t, test, "Greet Ada and Grace")
	Expect(t, test.WaitForEvent("permissionRequested", 1), `{"detail":{"kind":"text"}}`)
	answerNext(t, test, sessionID, protocol.PermissionDecisionAllowSession, 1)
	test.UntilIdle(sessionID)

	events := test.Events(sessionID)
	if asked := len(OfKind(events, "permissionRequested")); asked != 1 {
		t.Fatalf("asked %d times: allowing a server for the session covers its next calls", asked)
	}
	Expect(t, OfKind(events, "toolCall")[0], `{"tool":"mcp","summary":"greeter · greet {\"name\":\"Ada\"}"}`)
	finished := Finished(events)
	if len(finished) != 2 || finished[0].Output != "Hello, Ada." || finished[1].Output != "Hello, Grace." || !finished[1].OK {
		t.Fatalf("the calls gave %+v", finished)
	}
	names := toolNamesOf(test.Requests()[0])
	if len(names) == 0 || names[len(names)-1] != "mcp_greeter_greet" {
		t.Fatalf("the model was offered %v", names)
	}
	// The model reads its own call back under the name it called.
	if later := RequestText(map[string]any{"messages": test.Requests()[1]["messages"]}); !strings.Contains(later, `"name":"mcp_greeter_greet"`) || !strings.Contains(later, "Hello, Ada.") {
		t.Fatalf("the conversation sent back: %s", later)
	}
}

func TestTheServersFileIsReadAgainWhenAsked(t *testing.T) {
	server := fakeServer(t)
	dataDir := withServers(t, map[string]string{})
	test := Start(t, Options{DataDir: dataDir, Replies: []fakellama.Reply{}})
	if status := serversOf(t, test, "GET"); len(status.Servers) != 0 || status.File != filepath.Join(dataDir, mcp.File) {
		t.Fatalf("before the file lists a server: %+v", status)
	}

	data, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"greeter": map[string]any{"url": server.URL}}})
	if err := os.WriteFile(filepath.Join(dataDir, mcp.File), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if status := serversOf(t, test, "POST"); len(status.Servers) != 1 || status.Servers[0].Name != "greeter" {
		t.Fatalf("once read again: %+v", status)
	}
	untilReady(t, test)
	if status := serversOf(t, test, "GET"); status.Servers[0].Tools != 1 || status.Servers[0].Target != server.URL {
		t.Fatalf("once reached: %+v", status)
	}

	if err := os.WriteFile(filepath.Join(dataDir, mcp.File), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if status := serversOf(t, test, "POST"); len(status.Servers) != 0 || !strings.Contains(status.Problem, "not valid JSON") {
		t.Fatalf("with a broken file: %+v", status)
	}
}

func TestAChatHasNoServerTools(t *testing.T) {
	server := fakeServer(t)
	test := Start(t, Options{DataDir: withServers(t, map[string]string{"greeter": server.URL}), Replies: []fakellama.Reply{text("Hello.")}})
	untilReady(t, test)
	session := test.NewSession(sessions.Request{Kind: protocol.SessionKindChat})
	if problem := test.App.Sessions.SendPrompt(session.ID, "Hi", nil); problem != "" {
		t.Fatal(problem)
	}
	test.UntilIdle(session.ID)
	for _, name := range toolNamesOf(test.Requests()[0]) {
		if strings.HasPrefix(name, "mcp_") {
			t.Fatalf("a chat was offered %s", name)
		}
	}
}
