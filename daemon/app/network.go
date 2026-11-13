package app

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	tokenFile      = "token"
	tokenBytes     = 24
	tokenMinLength = 16
	ownerOnly      = 0o600
	ownerFolder    = 0o700
	localName      = "localhost"
	loopback       = "127.0.0.1"
)

var (
	// virtualPrefixes are bridges of containers and virtual machines: their addresses lead nowhere a phone is.
	virtualPrefixes = []string{"br-", "cni", "docker", "flannel", "lxdbr", "podman", "veth", "vEthernet", "virbr"}
	// preferredPrefixes are the usual home and office networks first, then the others (a VPN's, say).
	preferredPrefixes = []string{"192.168.", "10.", "172."}
	allInterfaces     = map[string]bool{"0.0.0.0": true, "::": true}
	loopbackHosts     = map[string]bool{"127.0.0.1": true, "::1": true, "localhost": true}
)

// Interface is a network interface of this machine: its name, and its addresses.
type Interface struct {
	Name      string
	Addresses []InterfaceAddress
}

// InterfaceAddress is an address of an interface.
type InterfaceAddress struct {
	Address  string
	IPv4     bool
	Internal bool
}

// SystemInterfaces is this machine's network interfaces.
func SystemInterfaces() []Interface {
	found, err := net.Interfaces()
	if err != nil {
		return nil
	}
	interfaces := []Interface{}
	for _, each := range found {
		addresses, _ := each.Addrs()
		known := Interface{Name: each.Name}
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err != nil {
				continue
			}
			known.Addresses = append(known.Addresses, InterfaceAddress{Address: ip.String(), Internal: ip.IsLoopback(), IPv4: ip.To4() != nil})
		}
		interfaces = append(interfaces, known)
	}
	return interfaces
}

func isVirtual(name string) bool {
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func rank(address string) int {
	for index, prefix := range preferredPrefixes {
		if strings.HasPrefix(address, prefix) {
			return index
		}
	}
	return len(preferredPrefixes)
}

// LANAddresses is the addresses other machines may reach this one at: IPv4, not loopback, not a container's bridge.
func LANAddresses(interfaces []Interface) []string {
	seen := map[string]bool{}
	addresses := []string{}
	for _, each := range interfaces {
		if isVirtual(each.Name) {
			continue
		}
		for _, address := range each.Addresses {
			if address.IPv4 && !address.Internal && !seen[address.Address] {
				seen[address.Address] = true
				addresses = append(addresses, address.Address)
			}
		}
	}
	sort.SliceStable(addresses, func(first, second int) bool { return rank(addresses[first]) < rank(addresses[second]) })
	return addresses
}

// LANURLs is the links other machines open to reach a daemon listening on host: none when it listens on loopback
// only.
func LANURLs(host string, port int, interfaces []Interface) []string {
	if loopbackHosts[host] {
		return []string{}
	}
	addresses := []string{host}
	if allInterfaces[host] {
		addresses = LANAddresses(interfaces)
	}
	links := []string{}
	for _, address := range addresses {
		links = append(links, fmt.Sprintf("http://%s:%d", address, port))
	}
	return links
}

// ListensOnNetwork tells whether a daemon listening on host can be reached from other machines.
func ListensOnNetwork(host string) bool { return !loopbackHosts[host] }

// ControlURL is where this computer's own commands reach the daemon (swcode open, stop): loopback when it listens
// there.
func ControlURL(host string, port int) string {
	if allInterfaces[host] || loopbackHosts[host] {
		host = loopback
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}

// LocalURL is the link on this computer itself: localhost, unless the daemon listens on one address only.
func LocalURL(host string, port int) string {
	if allInterfaces[host] || loopbackHosts[host] {
		host = localName
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}

// AccessToken is the token from the settings, or the one kept in the data folder, created on the first start.
func AccessToken(dataDir, token string) (string, error) {
	if token != "" {
		return token, nil
	}
	path := filepath.Join(dataDir, tokenFile)
	if data, err := os.ReadFile(path); err == nil {
		stored := strings.TrimSpace(string(data))
		if len(stored) >= tokenMinLength {
			return stored, nil
		}
	}
	secret := make([]byte, tokenBytes)
	_, _ = rand.Read(secret)
	created := base64.RawURLEncoding.EncodeToString(secret)
	if err := os.MkdirAll(dataDir, ownerFolder); err != nil {
		return "", err
	}
	return created, os.WriteFile(path, []byte(created), ownerOnly)
}

// keepPrivate makes the data folder the person's alone: it holds the sessions with all the agents read, the
// settings with their keys, and the log with the link that opens swcode. A folder made by an earlier version was
// readable by every account of the machine.
func keepPrivate(dataDir string) error {
	if err := os.MkdirAll(dataDir, ownerFolder); err != nil {
		return err
	}
	return os.Chmod(dataDir, ownerFolder)
}
