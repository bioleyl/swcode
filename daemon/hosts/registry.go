package hosts

import (
	"sync"

	"swcode/daemon/protocol"
)

type keyOf struct{}

func (keyOf) Local(protocol.HostRefLocal) string { return "local" }
func (keyOf) SSH(ref protocol.HostRefSSH) string { return "ssh:" + ref.Endpoint }
func (keyOf) WSL(ref protocol.HostRefWSL) string { return "wsl:" + ref.Distro }

// Key is one string per host, to tell hosts apart and to know a project by its host and folder.
func Key(ref protocol.HostRef) string {
	return protocol.MapHostRef[string](ref, keyOf{})
}

// SameHost tells whether two references name the same host.
func SameHost(first, second protocol.HostRef) bool {
	return Key(first) == Key(second)
}

type labelOf struct{}

func (labelOf) Local(protocol.HostRefLocal) string { return "This computer" }
func (labelOf) SSH(ref protocol.HostRefSSH) string { return ref.Endpoint }
func (labelOf) WSL(ref protocol.HostRefWSL) string { return "WSL · " + ref.Distro }

// Label is how a host reads for people.
func Label(ref protocol.HostRef) string {
	return protocol.MapHostRef[string](ref, labelOf{})
}

// IsLocal tells this computer.
func IsLocal(ref protocol.HostRef) bool {
	_, local := ref.Value.(protocol.HostRefLocal)
	return local
}

// Registry is the hosts projects are on: this computer, and one connection to each other host, made when first
// needed and kept while the daemon runs.
type Registry struct {
	mu      sync.Mutex
	local   Host
	connect func(protocol.HostRef) Host
	remote  map[string]Host
}

func NewRegistry(local Host, connect func(protocol.HostRef) Host) *Registry {
	return &Registry{connect: connect, local: local, remote: map[string]Host{}}
}

func (r *Registry) HostOf(ref protocol.HostRef) Host {
	if IsLocal(ref) {
		return r.local
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := Key(ref)
	if known, ok := r.remote[key]; ok {
		return known
	}
	host := r.connect(ref)
	r.remote[key] = host
	return host
}

// Close ends the workers and stops every host's jobs, when the daemon stops.
func (r *Registry) Close() {
	r.local.Close()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, host := range r.remote {
		host.Close()
	}
}

// RemoteHost reaches a WSL distribution or an SSH endpoint through its bash.
func RemoteHost(ref protocol.HostRef) Host {
	return NewShellHost(TransportOf(ref))
}
