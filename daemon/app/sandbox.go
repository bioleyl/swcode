package app

import (
	"net/http"
	"path/filepath"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
)

// sandboxOff is why no command is sandboxed when the settings say so.
const sandboxOff = "The sandbox is turned off in swcode's settings (SWCODE_SANDBOX=off)."

// realFolder is where a folder really is, links followed; the folder as given when it cannot be told.
func realFolder(folder string) string {
	resolved, err := filepath.EvalSymlinks(folder)
	if err != nil {
		return folder
	}
	return resolved
}

// sandboxRoutes tell whether a project's host can run its sessions' commands in a sandbox, for its page.
func (a *App) sandboxRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+protocol.EndpointSandbox, func(response http.ResponseWriter, request *http.Request) {
		project, found := a.Projects.ByID(request.URL.Query().Get(protocol.ProjectParam))
		if !found {
			fail(response, http.StatusNotFound, noProject)
			return
		}
		if a.sandboxes == nil {
			answer(response, protocol.SandboxView{Problem: sandboxOff})
			return
		}
		state := a.sandboxes.Of(request.Context(), hosts.Key(project.Host), a.hosts.HostOf(project.Host))
		answer(response, protocol.SandboxView{Available: state.Available, Problem: state.Problem})
	})
}
