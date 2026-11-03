package app

import (
	"net/http"

	"swcode/daemon/protocol"
)

const (
	notStartable      = "Only swcode's Windows app starts with its computer."
	unreadableStartup = "The page sent a way to start that the daemon does not know."
)

// StartupPort starts the daemon when its person logs in to its computer: the Windows app does, nothing else.
type StartupPort interface {
	Mode() protocol.StartupMode
	SetMode(mode protocol.StartupMode) error
}

func (a *App) startupView() protocol.StartupView {
	if a.options.Startup == nil {
		return protocol.StartupView{Available: false, Mode: protocol.StartupModeOff}
	}
	return protocol.StartupView{Available: true, Mode: a.options.Startup.Mode()}
}

// startupRoutes tell how this daemon starts with its computer, and change it: the settings' Startup topic.
func (a *App) startupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+protocol.EndpointStartup, func(response http.ResponseWriter, _ *http.Request) {
		answer(response, a.startupView())
	})
	mux.HandleFunc("PUT "+protocol.EndpointStartup, func(response http.ResponseWriter, request *http.Request) {
		var change protocol.StartupChange
		if readBody(response, request, &change) != nil || !change.Mode.Valid() {
			fail(response, http.StatusBadRequest, unreadableStartup)
			return
		}
		if a.options.Startup == nil {
			fail(response, http.StatusNotImplemented, notStartable)
			return
		}
		if err := a.options.Startup.SetMode(change.Mode); err != nil {
			fail(response, http.StatusInternalServerError, err.Error())
			return
		}
		answer(response, a.startupView())
	})
}
