package tools

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
)

const (
	// The name the model calls an MCP server's tool by: the server's and the tool's, in the letters every model's
	// API takes, and no longer than they take.
	serverToolPrefix  = "mcp_"
	maxFunctionName   = 64
	maxServerToolText = 1000
	// maxServerOutput is the most of a server's answer the model reads: a tool can give back a whole file.
	maxServerOutput = 30_000
	notAnObject     = "The input is not an object."
)

var notInAName = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// ServerCall calls a tool of an MCP server: what it gave back, whether the tool said it failed, or why the call
// itself did not go through.
type ServerCall func(ctx context.Context, server, tool string, arguments map[string]any) (text string, failed bool, err error)

// serverFunction is the name the model calls a server's tool by.
func serverFunction(server, tool string) string {
	name := serverToolPrefix + notInAName.ReplaceAllString(server, "_") + "_" + notInAName.ReplaceAllString(tool, "_")
	return name[:min(len(name), maxFunctionName)]
}

// ServerTool is a tool of an MCP server as an agent has it: named for the model after its server, described and
// shaped as the server says, and asked about like a command, since what it does is the server's own.
func ServerTool(server, name, description string, schema map[string]any, call ServerCall) Tool {
	return Tool{
		Description: clipped(description, maxServerToolText) + " (A tool of the MCP server " + server + ".)",
		Function:    serverFunction(server, name),
		Name:        protocol.ToolNameMcp,
		Parameters:  schema,
		Prepare: func(_ context.Context, raw any, _ *Context) Preparation {
			arguments, isObject := raw.(map[string]any)
			summary := server + " · " + name + " " + rawSummary(raw)
			if !isObject {
				return Preparation{Problem: notAnObject, Summary: summary}
			}
			shown, _ := json.MarshalIndent(arguments, "", "  ")
			return Preparation{OK: true, Summary: summary, Call: PreparedCall{
				Preview: Text(string(shown)),
				Run:     func(ctx context.Context) Outcome { return serverOutcome(ctx, server, name, arguments, call) },
				Subject: permissions.Subject{Access: permissions.AccessServer, Server: server},
				Summary: summary,
			}}
		},
	}
}

// serverOutcome is how a call to a server's tool went, as the model reads it.
func serverOutcome(ctx context.Context, server, name string, arguments map[string]any, call ServerCall) Outcome {
	text, failed, err := call(ctx, server, name, arguments)
	if err != nil {
		return Failure("The MCP server "+server+" did not take the call: "+err.Error(), "failed")
	}
	said := clipped(strings.TrimSpace(text), maxServerOutput)
	if said == "" {
		said = noOutput
	}
	if failed {
		return Outcome{Display: Text(said), Meta: "failed", Output: said}
	}
	return Outcome{Display: Text(said), Meta: "done", OK: true, Output: said}
}
