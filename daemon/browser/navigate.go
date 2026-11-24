package browser

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"
)

const (
	lookupTimeout = 3 * time.Second
	httpScheme    = "http"
	httpsScheme   = "https"
)

// Navigation is how a page may be opened: on its own, once a person approves it, or not at all.
type Navigation struct {
	Allowed  bool
	Approval bool
	Problem  string
}

// CheckNavigation guards browser_open: only http and https pages open by themselves, a private address or a name
// that resolves to one needs a person's approval, and every other scheme is refused. A name that does not resolve
// is let through, so the browser's own error tells the agent what happened.
func CheckNavigation(rawURL string) Navigation {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return Navigation{Problem: "That is not a web address: " + err.Error()}
	}
	if parsed.Scheme != httpScheme && parsed.Scheme != httpsScheme {
		return Navigation{Problem: "Only http and https addresses open in the browser."}
	}
	if parsed.Hostname() == "" {
		return Navigation{Problem: "The address has no host."}
	}
	if parsed.User != nil {
		return Navigation{Problem: "An address with a password in it is not opened: sign in on the page instead."}
	}
	if reachesPrivate(parsed.Hostname()) {
		return Navigation{Allowed: true, Approval: true}
	}
	return Navigation{Allowed: true}
}

// reachesPrivate tells whether a host is this machine's loopback (which is free), or a private or link-local
// address, which the daemon reaches but the public web does not.
func reachesPrivate(host string) bool {
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	if name == "localhost" || strings.HasSuffix(name, ".localhost") {
		return false
	}
	if ip := net.ParseIP(name); ip != nil {
		return !ip.IsLoopback() && !ip.IsUnspecified() && privateOrLinkLocal(ip)
	}
	ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", name)
	if err != nil {
		return false
	}
	private := false
	for _, ip := range ips {
		if ip.IsLoopback() {
			continue
		}
		private = private || privateOrLinkLocal(ip)
	}
	return private
}

func privateOrLinkLocal(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
