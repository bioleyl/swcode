package local

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"swcode/daemon/protocol"
)

func TestOnlyTheDaemonOfThisTokenIsTakenForThisComputers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == protocol.EndpointHealth || request.Header.Get("authorization") == protocol.BearerPrefix+"mine" {
			response.WriteHeader(http.StatusOK)
			return
		}
		response.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	if !IsUp(server.URL) || !Accepts(server.URL, "mine") || Accepts(server.URL, "another") {
		t.Fatal("accepts")
	}
}
