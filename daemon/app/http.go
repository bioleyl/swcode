package app

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"swcode/daemon/pairing"
	"swcode/daemon/protocol"
)

const (
	maxBody = 64 << 20
	refusal = "Open swcode with the link the daemon printed when it started, or pair this phone with the QR code: either brings a token."
)

// openRoutes are the health, and the pairing of a phone, which brings the code as its secret.
var openRoutes = map[string]bool{protocol.EndpointHealth: true, protocol.EndpointPairing: true}

// message is an answer that says something to people.
type message struct {
	Message string `json:"message"`
}

func writeJSON(response http.ResponseWriter, status int, body any) {
	response.Header().Set("content-type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(body)
}

func answer(response http.ResponseWriter, body any) { writeJSON(response, http.StatusOK, body) }

func fail(response http.ResponseWriter, status int, text string) {
	writeJSON(response, status, message{Message: text})
}

// readBody reads a request's JSON into target: false, and the answer sent, when it is not what the route takes.
func readBody(response http.ResponseWriter, request *http.Request, target any) error {
	data, err := io.ReadAll(io.LimitReader(request.Body, maxBody))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func presentedToken(request *http.Request) string {
	if header := request.Header.Get("authorization"); strings.HasPrefix(header, protocol.BearerPrefix) {
		return strings.TrimPrefix(header, protocol.BearerPrefix)
	}
	return request.URL.Query().Get(protocol.TokenParam)
}

// withAuth makes every API route, the socket included, need the access token or a paired phone's: the daemon runs
// commands, so a page from another site must not reach it. The web app itself is served without one.
func withAuth(next http.Handler, token string, phones *pairing.Pairing) http.Handler {
	expected := []byte(token)
	accepted := func(given string) bool {
		return subtle.ConstantTimeCompare([]byte(given), expected) == 1 || (given != "" && phones.Knows(given))
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		path := request.URL.Path
		if !strings.HasPrefix(path, protocol.APIPrefix) || openRoutes[path] || accepted(presentedToken(request)) {
			next.ServeHTTP(response, request)
			return
		}
		fail(response, http.StatusUnauthorized, refusal)
	})
}
