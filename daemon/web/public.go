package web

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"syscall"
	"time"
)

const (
	dialLimit = 30 * time.Second
	notPublic = "the web tools read the public web only, and %s is this computer or a network of its own"
)

// proxyNames are the variables that send a program's requests through a proxy.
var proxyNames = []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"}

// sharedSpace is the addresses carriers and tailnets hand out (100.64.0.0/10): another machine of the person's, as a
// private address is.
var sharedSpace = netip.MustParsePrefix("100.64.0.0/10")

// isPublic tells an address of the public web from one of this computer or of a network around it.
func isPublic(address netip.Addr) bool {
	address = address.Unmap()
	inside := address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() ||
		address.IsMulticast() || address.IsUnspecified() || sharedSpace.Contains(address)
	return address.IsValid() && !inside
}

// refuseInside refuses a connection to anything but the public web. It is asked as the connection is made, once
// the name is an address: a name that leads inside is refused like an address written out, and so is a page that
// sends its reader there.
func refuseInside(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	parsed, err := netip.ParseAddr(host)
	if err != nil || !isPublic(parsed) {
		return fmt.Errorf(notPublic, host)
	}
	return nil
}

// usesProxy tells whether the environment sends requests through a proxy: the connections are then the proxy's to
// make, and to refuse.
func usesProxy() bool {
	return slices.ContainsFunc(proxyNames, func(name string) bool { return os.Getenv(name) != "" })
}

// PublicClient is what the web tools read pages and search with: an HTTP client that reaches the public web only.
// The model writes the addresses they read, and a page can send its reader anywhere: without this they would read
// llm-server, a router's page or a cloud's keys, and tell the agent what they found. Behind a proxy the client is
// the usual one: the proxy makes the connections.
func PublicClient() *http.Client {
	if usesProxy() {
		return http.DefaultClient
	}
	dialer := &net.Dialer{Control: refuseInside, Timeout: dialLimit}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer.DialContext
	return &http.Client{Transport: transport}
}
