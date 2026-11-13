// Package tailnet gives this computer's swcode an HTTPS address on the person's tailnet: Tailscale serves a local
// port under the machine's own name there, with a certificate every browser trusts. A phone installs swcode as an
// app, and its page uses the camera, only from such an address; and it then reaches swcode from anywhere.
package tailnet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"swcode/daemon/protocol"
)

const (
	program = "tailscale"
	// commandLimit is how long Tailscale's command line has to answer.
	commandLimit = 10 * time.Second
	running      = "Running"
	httpsScheme  = "https://"
	// Tailscale leaves its usual HTTPS port out of an address, as a browser does.
	usualPort    = ":443"
	notInstalled = "Tailscale is not on this computer. With it, swcode gets an HTTPS address on your tailnet, which a phone reaches from anywhere."
	notConnected = "Tailscale is on this computer, but not connected to a tailnet."
	noHTTPS      = "This tailnet gives no HTTPS certificates yet: turn them on in Tailscale’s admin console (DNS, then HTTPS Certificates)."
	notServed    = "Tailscale did not serve swcode: %s"
)

// Run runs Tailscale's command line with these arguments: what it printed.
type Run func(ctx context.Context, args ...string) ([]byte, error)

// Command is Tailscale's own command line, where this computer has it.
func Command(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, commandLimit)
	defer cancel()
	return exec.CommandContext(ctx, program, args...).CombinedOutput()
}

// The parts of `tailscale status --json` and `tailscale serve status --json` read here.
type status struct {
	BackendState string
	CertDomains  []string
}

type serveStatus struct {
	Web map[string]struct {
		Handlers map[string]struct{ Proxy string }
	}
}

// local is how Tailscale reaches a port of this computer, by each of its names.
func local(port int) []string {
	return []string{fmt.Sprintf("http://127.0.0.1:%d", port), fmt.Sprintf("http://localhost:%d", port)}
}

// servedAt is the tailnet address Tailscale serves a local port at; empty when it serves it nowhere.
func servedAt(ctx context.Context, run Run, port int) string {
	data, err := run(ctx, "serve", "status", "--json")
	var serving serveStatus
	if err != nil || json.Unmarshal(data, &serving) != nil {
		return ""
	}
	targets := local(port)
	for host, site := range serving.Web {
		for _, handler := range site.Handlers {
			if strings.TrimSuffix(handler.Proxy, "/") == targets[0] || strings.TrimSuffix(handler.Proxy, "/") == targets[1] {
				return httpsScheme + strings.TrimSuffix(host, usualPort)
			}
		}
	}
	return ""
}

// unavailable is why this computer's tailnet gives swcode no address.
func unavailable(problem string) protocol.Tailnet {
	return protocol.Tailnet{Problem: problem}
}

// Read tells whether this computer's tailnet can give the swcode on that port an HTTPS address, and the address:
// the one Tailscale already serves it at, or the one it would.
func Read(ctx context.Context, run Run, port int) protocol.Tailnet {
	data, err := run(ctx, "status", "--json")
	if errors.Is(err, exec.ErrNotFound) {
		return unavailable(notInstalled)
	}
	var found status
	if err != nil || json.Unmarshal(data, &found) != nil || found.BackendState != running {
		return unavailable(notConnected)
	}
	if len(found.CertDomains) == 0 {
		return unavailable(noHTTPS)
	}
	if served := servedAt(ctx, run, port); served != "" {
		return protocol.Tailnet{Address: served, Available: true, Served: true}
	}
	return protocol.Tailnet{Address: fmt.Sprintf("%s%s:%d", httpsScheme, found.CertDomains[0], port), Available: true}
}

// firstLine is what a command said, in a line.
func firstLine(output []byte, err error) string {
	line, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	if line == "" {
		return err.Error()
	}
	return line
}

// Serve has Tailscale serve the swcode on that port at its tailnet address, from now on and after a restart, and
// tells where it stands then: served, or why Tailscale refused.
func Serve(ctx context.Context, run Run, port int) protocol.Tailnet {
	state := Read(ctx, run, port)
	if !state.Available || state.Served {
		return state
	}
	output, err := run(ctx, "serve", "--bg", fmt.Sprintf("--https=%d", port), local(port)[0])
	if err != nil {
		state.Problem = fmt.Sprintf(notServed, firstLine(output, err))
		return state
	}
	return Read(ctx, run, port)
}
