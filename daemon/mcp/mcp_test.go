package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"swcode/daemon/protocol"
)

// fakeKind is the variable that makes this test program a server instead: of the kind that shakes hands, or of
// the newer one.
const (
	fakeKind   = "SWCODE_FAKE_MCP"
	handshakes = "handshake"
	newer      = "newer"
)

// TestMain runs the tests, or plays an MCP server over its standard streams when asked to.
func TestMain(m *testing.M) {
	if kind := os.Getenv(fakeKind); kind != "" {
		serveStreams(kind)
		return
	}
	os.Exit(m.Run())
}

// asked is a request as a fake server reads it.
type asked struct {
	ID     *json.RawMessage `json:"id"`
	Method string           `json:"method"`
	Params struct {
		Name      string                     `json:"name"`
		Cursor    string                     `json:"cursor"`
		Arguments map[string]any             `json:"arguments"`
		Meta      map[string]json.RawMessage `json:"_meta"`
	} `json:"params"`
}

func rpcFailure(code int, text string) map[string]any {
	return map[string]any{"error": map[string]any{"code": code, "message": text}}
}

// answerTo is what a fake server answers a request: nil for a notification. It has two tools, listed a page each:
// echo, which says back what it is given, and fail, which says it failed.
func answerTo(request asked, kind string) map[string]any {
	said := func(result map[string]any) map[string]any { return map[string]any{"result": result} }
	if kind == newer && request.Method != methodInitialize && request.Params.Meta[versionMeta] == nil {
		return rpcFailure(-32602, "the request says no revision")
	}
	switch request.Method {
	case methodInitialize:
		if kind == newer {
			return rpcFailure(codeMethodNotFound, "Method not found")
		}
		return said(map[string]any{"capabilities": map[string]any{}, "protocolVersion": "2025-06-18"})
	case methodListTools:
		if request.Params.Cursor == "" {
			return said(map[string]any{"nextCursor": "page-2", "tools": []any{map[string]any{
				"description": "Says back what it is given.", "inputSchema": map[string]any{"type": "object"}, "name": "echo",
			}}})
		}
		return said(map[string]any{"tools": []any{map[string]any{"description": "Fails.", "inputSchema": map[string]any{"type": "object"}, "name": "fail"}}})
	case methodCallTool:
		if request.Params.Name == "fail" {
			return said(map[string]any{"content": []any{map[string]any{"text": "It broke.", "type": "text"}}, "isError": true})
		}
		return said(map[string]any{"content": []any{
			map[string]any{"text": fmt.Sprint("echo: ", request.Params.Arguments["text"]), "type": "text"},
			map[string]any{"data": "AAAA", "mimeType": "image/png", "type": "image"},
		}})
	}
	return nil
}

// serveStreams plays a server over this program's standard streams, a message a line, until its input closes.
func serveStreams(kind string) {
	lines := bufio.NewScanner(os.Stdin)
	for lines.Scan() {
		var request asked
		if json.Unmarshal(lines.Bytes(), &request) != nil || request.ID == nil {
			continue
		}
		answer := answerTo(request, kind)
		answer["id"], answer["jsonrpc"] = request.ID, rpcVersion
		data, _ := json.Marshal(answer)
		fmt.Println(string(data))
	}
}

// program is this test program as a server of that kind.
func program(kind string) ServerConfig {
	return ServerConfig{Command: os.Args[0], Env: map[string]string{fakeKind: kind}}
}

// useTools lists a server's tools and calls each, as a turn does.
func useTools(t *testing.T, config ServerConfig) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := Connect(ctx, config, http.DefaultClient, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tools, err := client.ListTools(ctx)
	if err != nil || len(tools) != 2 || tools[0].Name != "echo" || tools[1].Name != "fail" || tools[0].InputSchema["type"] != "object" {
		t.Fatalf("tools %+v, error %v", tools, err)
	}
	said, err := client.CallTool(ctx, "echo", map[string]any{"text": "hello"})
	if err != nil || said.IsError || said.Text != "echo: hello\n[image content, which swcode does not pass on]" {
		t.Fatalf("echo: %+v, error %v", said, err)
	}
	if failed, err := client.CallTool(ctx, "fail", nil); err != nil || !failed.IsError || failed.Text != "It broke." {
		t.Fatalf("fail: %+v, error %v", failed, err)
	}
}

func TestAProgramIsReachedOverItsStreamsWhetherItShakesHandsOrNot(t *testing.T) {
	useTools(t, program(handshakes))
	useTools(t, program(newer))
	if _, err := Connect(context.Background(), ServerConfig{Command: filepath.Join(t.TempDir(), "no-such-program")}, http.DefaultClient, "test"); err == nil {
		t.Fatal("a program that is not there was reached")
	}
	if _, err := Connect(context.Background(), ServerConfig{}, http.DefaultClient, "test"); err == nil {
		t.Fatal("a server with neither a command nor a url was reached")
	}
}

// address is a server at an address, of that kind: it answers at once, or as a stream of events for a tool's call,
// and one that shakes hands gives a session, which it wants back.
func address(t *testing.T, kind string) *httptest.Server {
	t.Helper()
	const session = "session-1"
	served := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, post *http.Request) {
		var request asked
		if json.NewDecoder(post.Body).Decode(&request) != nil || !strings.Contains(post.Header.Get("accept"), eventStream) {
			http.Error(response, "not a request", http.StatusBadRequest)
			return
		}
		if request.ID == nil {
			response.WriteHeader(http.StatusAccepted)
			return
		}
		if kind == handshakes && request.Method != methodInitialize && post.Header.Get(headerSession) != session {
			http.Error(response, "no session", http.StatusBadRequest)
			return
		}
		if kind == newer && request.Method == methodCallTool && post.Header.Get(headerName) != request.Params.Name {
			http.Error(response, "the call does not name its tool", http.StatusBadRequest)
			return
		}
		answer := answerTo(request, kind)
		answer["id"], answer["jsonrpc"] = request.ID, rpcVersion
		data, _ := json.Marshal(answer)
		if request.Method == methodInitialize && kind == handshakes {
			response.Header().Set(headerSession, session)
		}
		if request.Method != methodCallTool {
			response.Header().Set("content-type", "application/json")
			_, _ = response.Write(data)
			return
		}
		// A call is answered as a stream: a notification of the server's own first, then the answer.
		response.Header().Set("content-type", eventStream)
		fmt.Fprintf(response, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\nevent: message\ndata: %s\n\n", data)
	}))
	t.Cleanup(served.Close)
	return served
}

func TestAnAddressIsReachedOverHTTPWhetherItShakesHandsOrNot(t *testing.T) {
	useTools(t, ServerConfig{URL: address(t, handshakes).URL})
	useTools(t, ServerConfig{URL: address(t, newer).URL})
}

// managed is a manager on a servers file with that content, its servers being reached.
func managed(t *testing.T, content string) (*Manager, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), File)
	if content != "" {
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manager := NewManager(file, http.DefaultClient, "test")
	t.Cleanup(manager.Close)
	manager.Reload()
	return manager, file
}

// settled waits until no server is still being reached, and is where they stand.
func settled(t *testing.T, manager *Manager) protocol.McpStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		status := manager.Status()
		pending := false
		for _, each := range status.Servers {
			pending = pending || each.State == protocol.McpServerStateStarting
		}
		if !pending {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("still starting: %+v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestTheManagerReachesTheServersOfTheFileAndSaysWhereEachStands(t *testing.T) {
	entry, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{
		"tools":  program(handshakes),
		"broken": ServerConfig{Command: filepath.Join(t.TempDir(), "no-such-program")},
	}})
	manager, file := managed(t, string(entry))
	status := settled(t, manager)
	if len(status.Servers) != 2 || status.Servers[0].Name != "broken" || status.Servers[0].State != protocol.McpServerStateFailed || status.Servers[0].Problem == "" {
		t.Fatalf("the broken one: %+v", status.Servers)
	}
	if ready := status.Servers[1]; ready.Name != "tools" || ready.State != protocol.McpServerStateReady || ready.Tools != 2 {
		t.Fatalf("the ready one: %+v", ready)
	}

	// A turn takes the tools of the servers that are ready, in the same order each time.
	tools := manager.Tools()
	if len(tools) != 2 || tools[0].Server != "tools" || tools[0].Tool.Name != "echo" || tools[1].Tool.Name != "fail" {
		t.Fatalf("tools %+v", tools)
	}
	if said, err := manager.Call(context.Background(), "tools", "echo", map[string]any{"text": "hi"}); err != nil || !strings.HasPrefix(said.Text, "echo: hi") {
		t.Fatalf("called: %+v, error %v", said, err)
	}
	if _, err := manager.Call(context.Background(), "broken", "echo", nil); err == nil || !strings.Contains(err.Error(), "is not ready") {
		t.Fatalf("a call to the broken one: %v", err)
	}
	if _, err := manager.Call(context.Background(), "nope", "echo", nil); err == nil {
		t.Fatal("a call to a server that is not there went through")
	}

	// The file read again without them lets them go; one that is no JSON says so and has none.
	if err := os.WriteFile(file, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.Reload()
	if status := settled(t, manager); len(status.Servers) != 0 || len(manager.Tools()) != 0 {
		t.Fatalf("after the file emptied: %+v", status)
	}
	_ = os.WriteFile(file, []byte(`{not json`), 0o600)
	manager.Reload()
	if status := settled(t, manager); status.Problem == "" || len(status.Servers) != 0 {
		t.Fatalf("a file that is no JSON: %+v", status)
	}
}

func TestADaemonWithoutAServersFileHasNoServers(t *testing.T) {
	manager, _ := managed(t, "")
	if status := settled(t, manager); status.Problem != "" || len(status.Servers) != 0 {
		t.Fatalf("without a file: %+v", status)
	}
}
