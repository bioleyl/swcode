package mcp

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"swcode/daemon/protocol"
)

const (
	// connectLimit is how long a server has to start and list its tools: npx may fetch its package first.
	connectLimit = 90 * time.Second
	unknown      = "There is no MCP server named %s."
	notReady     = "The MCP server %s is not ready: %s"
	starting     = "it is still starting"
)

// server is one server of the file as the manager holds it: reached or not, with its tools or why it has none.
type server struct {
	config  ServerConfig
	client  *Client
	tools   []Tool
	problem string
	ready   bool
}

// ServerTool is a tool of a server, with the name the person gave that server.
type ServerTool struct {
	Server string
	Tool   Tool
}

// Manager keeps the servers of the file reached: started when the daemon starts and when the file is read again,
// each by itself, so one that is slow or broken holds nothing back. A turn takes the tools of those ready then.
type Manager struct {
	file    string
	http    *http.Client
	version string
	mu      sync.Mutex
	servers map[string]*server
	// problem is why the file itself could not be read.
	problem string
}

func NewManager(file string, httpClient *http.Client, version string) *Manager {
	return &Manager{file: file, http: httpClient, servers: map[string]*server{}, version: version}
}

// Reload reads the file again and reaches its servers: those it no longer has, or has changed, are let go first.
func (m *Manager) Reload() {
	configs, err := Load(m.file)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.problem = ""
	if err != nil {
		m.problem = err.Error()
		configs = map[string]ServerConfig{}
	}
	for name, held := range m.servers {
		if config, kept := configs[name]; kept && sameConfig(config, held.config) && (held.ready || held.problem == "") {
			delete(configs, name)
			continue
		}
		if held.client != nil {
			go held.client.Close()
		}
		delete(m.servers, name)
	}
	for name, config := range configs {
		started := &server{config: config}
		m.servers[name] = started
		go m.reach(name, started)
	}
}

func sameConfig(a, b ServerConfig) bool {
	return a.Command == b.Command && a.URL == b.URL && slices.Equal(a.Args, b.Args) && maps.Equal(a.Env, b.Env) &&
		maps.Equal(a.Headers, b.Headers)
}

// reach connects one server and lists its tools; what it found, or why not, is kept for the turns and the page.
func (m *Manager) reach(name string, started *server) {
	ctx, cancel := context.WithTimeout(context.Background(), connectLimit)
	defer cancel()
	client, tools, err := connected(ctx, started.config, m.http, m.version)
	m.mu.Lock()
	defer m.mu.Unlock()
	// The file was read again meanwhile: this server is no longer the one held.
	if m.servers[name] != started {
		if client != nil {
			go client.Close()
		}
		return
	}
	if err != nil {
		started.problem = err.Error()
		return
	}
	started.client, started.tools, started.ready = client, tools, true
}

func connected(ctx context.Context, config ServerConfig, httpClient *http.Client, version string) (*Client, []Tool, error) {
	client, err := Connect(ctx, config, httpClient, version)
	if err != nil {
		return nil, nil, err
	}
	tools, err := client.ListTools(ctx)
	if err != nil {
		client.Close()
		return nil, nil, err
	}
	return client, tools, nil
}

// Tools is the tools of the servers ready now, by server then by tool: the same order from one turn to the next, so
// the prompt llm-server cached stays the same.
func (m *Manager) Tools() []ServerTool {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := []ServerTool{}
	for _, name := range slices.Sorted(maps.Keys(m.servers)) {
		held := m.servers[name]
		tools := slices.Clone(held.tools)
		slices.SortFunc(tools, func(a, b Tool) int { return strings.Compare(a.Name, b.Name) })
		for _, tool := range tools {
			found = append(found, ServerTool{Server: name, Tool: tool})
		}
	}
	return found
}

// Call calls a tool of a server; an error when the server is not there, not ready, or refused the call.
func (m *Manager) Call(ctx context.Context, name, tool string, arguments map[string]any) (Result, error) {
	m.mu.Lock()
	held, known := m.servers[name]
	var client *Client
	why := starting
	if known {
		client = held.client
		if held.problem != "" {
			why = held.problem
		}
	}
	m.mu.Unlock()
	if !known {
		return Result{}, fmt.Errorf(unknown, name)
	}
	if client == nil {
		return Result{}, fmt.Errorf(notReady, name, why)
	}
	return client.CallTool(ctx, tool, arguments)
}

// Status is the servers of the file as the settings list them: each one's kind, where it stands and how many
// tools it has, and why the file could not be read when it could not.
func (m *Manager) Status() protocol.McpStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := protocol.McpStatus{File: m.file, Problem: m.problem, Servers: []protocol.McpServer{}}
	for _, name := range slices.Sorted(maps.Keys(m.servers)) {
		held := m.servers[name]
		status.Servers = append(status.Servers, protocol.McpServer{
			Name: name, Problem: held.problem, State: stateOf(held), Target: targetOf(held.config), Tools: len(held.tools),
		})
	}
	return status
}

func stateOf(held *server) protocol.McpServerState {
	switch {
	case held.ready:
		return protocol.McpServerStateReady
	case held.problem != "":
		return protocol.McpServerStateFailed
	}
	return protocol.McpServerStateStarting
}

// targetOf is what a server is, in a line: its address, or its command.
func targetOf(config ServerConfig) string {
	if config.URL != "" {
		return config.URL
	}
	return strings.Join(append([]string{config.Command}, config.Args...), " ")
}

// Close lets every server go: the daemon stops.
func (m *Manager) Close() {
	m.mu.Lock()
	held := slices.Collect(maps.Values(m.servers))
	m.servers = map[string]*server{}
	m.mu.Unlock()
	for _, each := range held {
		if each.client != nil {
			each.client.Close()
		}
	}
}
