package app

import (
	"context"
	"net/http"

	"swcode/daemon/protocol"
	"swcode/daemon/tools"
)

// serverTools is the tools of the MCP servers ready now, as the main agent of a coding session has them.
func (a *App) serverTools() []tools.Tool {
	found := []tools.Tool{}
	for _, each := range a.servers.Tools() {
		found = append(found, tools.ServerTool(each.Server, each.Tool.Name, each.Tool.Description, each.Tool.InputSchema, a.callServer))
	}
	return found
}

// callServer calls a tool of an MCP server for the agent.
func (a *App) callServer(ctx context.Context, server, tool string, arguments map[string]any) (string, bool, error) {
	result, err := a.servers.Call(ctx, server, tool, arguments)
	return result.Text, result.IsError, err
}

// mcpRoutes list the MCP servers of the file with where each stands, and read the file again.
func (a *App) mcpRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+protocol.EndpointMcp, func(response http.ResponseWriter, _ *http.Request) {
		answer(response, a.servers.Status())
	})
	mux.HandleFunc("POST "+protocol.EndpointMcp, func(response http.ResponseWriter, _ *http.Request) {
		a.servers.Reload()
		answer(response, a.servers.Status())
	})
}
