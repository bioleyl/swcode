package app

import (
	"context"
	"net/http"
	"sync"
	"time"

	"swcode/daemon/protocol"
	"swcode/daemon/tailnet"
)

// tailnetAge is how long what Tailscale said is kept: every page asks the daemon's status four times a minute, and
// Tailscale's command line is asked no more than once.
const tailnetAge = time.Minute

// tailnetAddress is where this daemon's tailnet address stands, as Tailscale last said.
type tailnetAddress struct {
	mu    sync.Mutex
	read  time.Time
	state protocol.Tailnet
}

// tailnetState is whether the tailnet can give this swcode an HTTPS address, and the address; nothing without a
// Tailscale to ask (the tests).
func (a *App) tailnetState(ctx context.Context) protocol.Tailnet {
	if a.options.Tailscale == nil {
		return protocol.Tailnet{}
	}
	a.tailnet.mu.Lock()
	defer a.tailnet.mu.Unlock()
	if a.options.Now().Sub(a.tailnet.read) > tailnetAge {
		a.tailnet.state = tailnet.Read(ctx, a.options.Tailscale, a.config.Port)
		a.tailnet.read = a.options.Now()
	}
	return a.tailnet.state
}

// serveOnTailnet has Tailscale serve this swcode, and keeps where it stands then.
func (a *App) serveOnTailnet(ctx context.Context) protocol.Tailnet {
	if a.options.Tailscale == nil {
		return protocol.Tailnet{}
	}
	a.tailnet.mu.Lock()
	defer a.tailnet.mu.Unlock()
	a.tailnet.state = tailnet.Serve(ctx, a.options.Tailscale, a.config.Port)
	a.tailnet.read = a.options.Now()
	return a.tailnet.state
}

// publicURL is the address a phone opens: the one the settings give, else the tailnet's once Tailscale serves
// swcode there, so the QR code leads to HTTPS without anyone typing an address.
func (a *App) publicURL(ctx context.Context) string {
	if a.config.PublicURL != "" {
		return a.config.PublicURL
	}
	if state := a.tailnetState(ctx); state.Served {
		return state.Address
	}
	return ""
}

// tailnetRoutes tell whether the tailnet can give swcode an HTTPS address, and have Tailscale serve it there.
func (a *App) tailnetRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+protocol.EndpointTailnet, func(response http.ResponseWriter, request *http.Request) {
		answer(response, a.tailnetState(request.Context()))
	})
	mux.HandleFunc("POST "+protocol.EndpointTailnet, func(response http.ResponseWriter, request *http.Request) {
		answer(response, a.serveOnTailnet(request.Context()))
	})
}
