package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
)

const (
	clientName = "swcode"
	// What stands, in a tool's result, for what this client does not pass on: a picture, a sound.
	otherContent = "[%s content, which swcode does not pass on]"
	needsInput   = "The tool asked for more input from the person, which swcode cannot give it."
)

// Client is one server reached and ready: its tools can be listed and called.
type Client struct {
	transport transport
	next      atomic.Int64
	// version is the revision spoken with the server; modern, that it is one without a handshake, where every
	// request says its revision itself.
	version string
	modern  bool
}

// Connect reaches a server and readies it: its program started, or its address asked, then the handshake where
// the server has one. clientVersion is swcode's, which a server is told.
func Connect(ctx context.Context, config ServerConfig, httpClient *http.Client, clientVersion string) (*Client, error) {
	carrier, err := transportOf(config, httpClient)
	if err != nil {
		return nil, err
	}
	client := &Client{transport: carrier}
	if err := client.shakeHands(ctx, clientVersion); err != nil {
		carrier.close()
		return nil, err
	}
	return client, nil
}

func transportOf(config ServerConfig, httpClient *http.Client) (transport, error) {
	if config.URL != "" {
		return &overHTTP{client: httpClient, headers: config.Headers, url: config.URL}, nil
	}
	if config.Command == "" {
		return nil, errors.New("it has neither a command nor a url")
	}
	return startStdio(config)
}

// shakeHands tells the server who asks and which revision, as servers up to legacyVersion want before anything
// else. A server of the newer kind knows no such request, or not that revision: with it, there is nothing to shake.
func (c *Client) shakeHands(ctx context.Context, clientVersion string) error {
	c.version = legacyVersion
	result, err := c.ask(ctx, methodInitialize, "", map[string]any{
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": clientName, "version": clientVersion},
		"protocolVersion": legacyVersion,
	})
	var refused *rpcError
	if errors.As(err, &refused) && (refused.Code == codeMethodNotFound || refused.Code == codeUnsupportedVersion) {
		c.modern, c.version = true, modernVersion
		return nil
	}
	if err != nil {
		return err
	}
	var agreed initializeResult
	if json.Unmarshal(result, &agreed) == nil && agreed.ProtocolVersion != "" {
		c.version = agreed.ProtocolVersion
	}
	return c.transport.notify(ctx, sent{request: request{Method: methodInitialized, Version: rpcVersion}, version: c.version})
}

// ask sends a request and is its result, or why the server refused it. name is the tool a call is for.
func (c *Client) ask(ctx context.Context, method, name string, params map[string]any) (json.RawMessage, error) {
	id := c.next.Add(1)
	if c.modern {
		params["_meta"] = map[string]any{versionMeta: c.version}
	}
	// Only once the handshake is over does a request say its revision beside its body.
	version := c.version
	if method == methodInitialize {
		version = ""
	}
	answer, err := c.transport.call(ctx, sent{
		name: name, request: request{ID: &id, Method: method, Params: params, Version: rpcVersion}, version: version,
	})
	if err != nil {
		return nil, err
	}
	if answer.Error != nil {
		return nil, answer.Error
	}
	return answer.Result, nil
}

// ListTools is every tool the server has, page after page.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	tools := []Tool{}
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		result, err := c.ask(ctx, methodListTools, "", params)
		if err != nil {
			return nil, err
		}
		var page listResult
		if err := json.Unmarshal(result, &page); err != nil {
			return nil, err
		}
		tools = append(tools, page.Tools...)
		if page.NextCursor == "" || page.NextCursor == cursor {
			return tools, nil
		}
		cursor = page.NextCursor
	}
}

// textOf is a tool's result as text: its text parts, and a word for each part that is something else.
func textOf(blocks []contentBlock) string {
	parts := []string{}
	for _, block := range blocks {
		switch {
		case block.Type == "text":
			parts = append(parts, block.Text)
		case block.Resource != nil && block.Resource.Text != "":
			parts = append(parts, block.Resource.Text)
		case block.URI != "":
			parts = append(parts, block.URI)
		default:
			parts = append(parts, fmt.Sprintf(otherContent, block.Type))
		}
	}
	return strings.Join(parts, "\n")
}

// CallTool calls one of the server's tools. A tool that fails says so in its result, which the model reads; an
// error is the call itself refused, or the server gone.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (Result, error) {
	result, err := c.ask(ctx, methodCallTool, name, map[string]any{"arguments": arguments, "name": name})
	if err != nil {
		return Result{}, err
	}
	var called callResult
	if err := json.Unmarshal(result, &called); err != nil {
		return Result{}, err
	}
	// A server of the newer kind may answer that it needs the person first: nobody here can answer it.
	if called.ResultType != "" && called.ResultType != "complete" {
		return Result{IsError: true, Text: needsInput}, nil
	}
	return Result{IsError: called.IsError, Text: textOf(called.Content)}, nil
}

// Close lets the server go: its program ends.
func (c *Client) Close() { c.transport.close() }
