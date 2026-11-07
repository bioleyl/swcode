package app

import (
	"context"
	"net/http"

	"swcode/daemon/protocol"
)

const busyUpdating = "A session is working: let it finish or stop it, then install. swcode restarts on the new version."

// updateRoutes are the settings' Updates topic's: the version that runs and the newer one found, whether to look
// every 10 minutes, looking now, and installing the newer one, after which the daemon restarts on it.
func (a *App) updateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+protocol.EndpointUpdate, func(response http.ResponseWriter, _ *http.Request) {
		answer(response, a.Updates.View())
	})
	mux.HandleFunc("PUT "+protocol.EndpointUpdate, func(response http.ResponseWriter, request *http.Request) {
		var change protocol.UpdateChange
		if readBody(response, request, &change) != nil {
			fail(response, http.StatusBadRequest, unreadable)
			return
		}
		if err := a.Updates.SetChecks(change.Checks); err != nil {
			fail(response, http.StatusInternalServerError, err.Error())
			return
		}
		answer(response, a.Updates.View())
	})
	mux.HandleFunc("POST "+protocol.EndpointUpdateCheck, func(response http.ResponseWriter, request *http.Request) {
		answer(response, a.Updates.Check(request.Context()))
	})
	mux.HandleFunc("POST "+protocol.EndpointUpdateInstall, func(response http.ResponseWriter, request *http.Request) {
		installed := a.installUpdate(request.Context())
		status := http.StatusOK
		if _, refused := installed.Value.(protocol.UpdateInstalledFailed); refused {
			status = http.StatusBadRequest
		}
		writeJSON(response, status, installed)
	})
}

func refusedUpdate(problem string) protocol.UpdateInstalled {
	return protocol.UpdateInstalled{Value: protocol.UpdateInstalledFailed{Problem: problem}}
}

// installUpdate installs the newer version found, then restarts on it; a restart stops the sessions' turns, so it
// waits while one works. Run in a terminal, the daemon goes on until the person restarts it.
func (a *App) installUpdate(ctx context.Context) protocol.UpdateInstalled {
	restart := restartOf(a.options.OnUpdated)
	if restart == protocol.RestartSelf && a.options.Busy() {
		return refusedUpdate(busyUpdating)
	}
	version, err := a.Updates.Install(ctx)
	if err != nil {
		return refusedUpdate(err.Error())
	}
	if restart == protocol.RestartSelf {
		// The answer leaves before the daemon closes.
		go a.options.OnUpdated()
	}
	return protocol.UpdateInstalled{Value: protocol.UpdateInstalledOk{Restart: restart, Version: version}}
}
