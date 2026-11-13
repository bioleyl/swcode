package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestOnlyAddressesOfThePublicWebAreOnes(t *testing.T) {
	inside := []string{
		"127.0.0.1", "::1", "10.1.2.3", "172.16.0.9", "192.168.1.20", "169.254.169.254", "fe80::1", "0.0.0.0", "100.101.102.103",
		"::ffff:127.0.0.1", "224.0.0.1",
	}
	for _, address := range inside {
		if isPublic(netip.MustParseAddr(address)) {
			t.Errorf("%s counts as the public web", address)
		}
	}
	for _, address := range []string{"93.184.216.34", "2606:2800:220:1:248:1893:25c8:1946", "100.63.255.255", "172.32.0.1"} {
		if !isPublic(netip.MustParseAddr(address)) {
			t.Errorf("%s is refused", address)
		}
	}
}

func TestThePublicClientRefusesAPageOfThisComputer(t *testing.T) {
	for _, name := range proxyNames {
		t.Setenv(name, "")
	}
	local := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("the llm-server's own page"))
	}))
	defer local.Close()
	fetched := FetchPage(context.Background(), local.URL, PublicClient())
	if fetched.OK || !strings.Contains(fetched.Problem, "public web only") {
		t.Fatalf("a page of this computer: %+v", fetched)
	}
	// The usual client still reads it: only the web tools are held to the public web.
	if usual := FetchPage(context.Background(), local.URL, http.DefaultClient); !usual.OK {
		t.Fatalf("with the usual client: %+v", usual)
	}
}
