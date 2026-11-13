package app

import (
	"net/http"

	"swcode/daemon/pairing"
	"swcode/daemon/protocol"
)

const (
	originHeader = "origin"
	// What a page from an allowed origin may send: the API's methods, a token and a JSON body.
	corsMethods = "GET, POST, PUT, DELETE"
	corsHeaders = "authorization, content-type"
	// How long a browser keeps the answer to its question before it asks again, in seconds.
	corsMaxAge = "600"
)

// isPreflight tells the question a browser asks before a page from another origin sends a request: may it?
func isPreflight(request *http.Request) bool {
	return request.Method == http.MethodOptions && request.Header.Get("access-control-request-method") != ""
}

// withCORS lets a swcode page served by another computer's swcode talk to this one: the app a phone installed
// from one computer shows the others it paired with. A browser lets a page read another origin's answers only
// when that origin says so: this daemon says so to the origins its paired phones came from, and to anyone who
// pairs, the code being the secret. No cookie opens anything here: an origin allowed still needs its token.
func withCORS(next http.Handler, phones *pairing.Pairing) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get(originHeader)
		allowed := origin != "" && (request.URL.Path == protocol.EndpointPairing || phones.AllowsOrigin(origin))
		if allowed {
			response.Header().Set("access-control-allow-origin", origin)
			response.Header().Add("vary", originHeader)
		}
		if !isPreflight(request) {
			next.ServeHTTP(response, request)
			return
		}
		if !allowed {
			fail(response, http.StatusForbidden, refusal)
			return
		}
		response.Header().Set("access-control-allow-methods", corsMethods)
		response.Header().Set("access-control-allow-headers", corsHeaders)
		response.Header().Set("access-control-max-age", corsMaxAge)
		// Chrome asks this too of a page that reaches an address of a private network from a public one.
		response.Header().Set("access-control-allow-private-network", "true")
		response.WriteHeader(http.StatusNoContent)
	})
}
