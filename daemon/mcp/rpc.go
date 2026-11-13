package mcp

import (
	"encoding/json"
	"fmt"
)

const (
	rpcVersion = "2.0"
	// The revisions of the protocol this client speaks. Up to legacyVersion, a client and a server shake hands
	// first (initialize) and an HTTP server may give the client a session; from modernVersion on there is neither,
	// and every request says its revision itself.
	legacyVersion = "2025-11-25"
	modernVersion = "2026-07-28"
	// versionMeta is where a request says its revision, from modernVersion on.
	versionMeta = "io.modelcontextprotocol/protocolVersion"
	// The errors that tell a server of the newer kind from one that shakes hands: it knows no initialize, or not
	// the revision asked.
	codeMethodNotFound     = -32601
	codeUnsupportedVersion = -32022

	methodInitialize  = "initialize"
	methodInitialized = "notifications/initialized"
	methodListTools   = "tools/list"
	methodCallTool    = "tools/call"
	methodPing        = "ping"
)

// request is a JSON-RPC request, or a notification when it has no id.
type request struct {
	Version string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// rpcError is why a server refused a request.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("%s (code %d)", e.Message, e.Code) }

// message is anything a server sends: the answer to a request (an id, with a result or an error), or a request or
// a notification of its own (a method).
type message struct {
	ID     *json.RawMessage `json:"id,omitempty"`
	Method string           `json:"method,omitempty"`
	Result json.RawMessage  `json:"result,omitempty"`
	Error  *rpcError        `json:"error,omitempty"`
}

// answers tells whether a message is the answer to the request of that id.
func (m message) answers(id int64) bool {
	if m.ID == nil || m.Method != "" {
		return false
	}
	var got int64
	return json.Unmarshal(*m.ID, &got) == nil && got == id
}

// Tool is a tool a server has: its name there, what it does, and the JSON schema of what it takes.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Result is what a tool gave back, as the model reads it: its text, and whether the tool said it failed.
type Result struct {
	Text    string
	IsError bool
}

// The parts of the protocol's own shapes this client reads.
type listResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
	// A link to a resource names it; a resource given whole carries its text.
	URI      string `json:"uri"`
	Resource *struct {
		URI  string `json:"uri"`
		Text string `json:"text"`
	} `json:"resource"`
}

type callResult struct {
	Content    []contentBlock `json:"content"`
	IsError    bool           `json:"isError"`
	ResultType string         `json:"resultType"`
}

type initializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
}
