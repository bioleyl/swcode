package app

import (
	"slices"
	"testing"
)

func interfaceOf(name string, internal bool, addresses ...string) Interface {
	known := Interface{Name: name}
	for _, address := range addresses {
		known.Addresses = append(known.Addresses, InterfaceAddress{Address: address, IPv4: true, Internal: internal})
	}
	return known
}

// What WSL showed on the laptop: loopback, the Wi-Fi, two Docker bridges, and a VPN.
var laptop = []Interface{
	interfaceOf("br-6c9de9406bb7", false, "172.19.0.1"),
	interfaceOf("docker0", false, "172.17.0.1"),
	interfaceOf("lo", true, "127.0.0.1"),
	interfaceOf("tailscale0", false, "100.101.102.103"),
	interfaceOf("eth2", false, "192.168.1.121"),
}

func expectLinks(t *testing.T, found, expected []string) {
	t.Helper()
	if !slices.Equal(found, expected) {
		t.Fatalf("expected %q, found %q", expected, found)
	}
}

func TestTheLANAddressesPutTheHomeNetworkFirstWithoutLoopbackOrBridges(t *testing.T) {
	expectLinks(t, LANAddresses(laptop), []string{"192.168.1.121", "100.101.102.103"})
}

func TestTheLANLinksAreOnePerAddressWhenTheDaemonListensEverywhere(t *testing.T) {
	expectLinks(t, LANURLs("0.0.0.0", 4500, laptop), []string{"http://192.168.1.121:4500", "http://100.101.102.103:4500"})
}

func TestThereAreNoLANLinksOnLoopbackAndTheDaemonsOwnAddressOtherwise(t *testing.T) {
	expectLinks(t, LANURLs("127.0.0.1", 4500, laptop), []string{})
	expectLinks(t, LANURLs("192.168.1.121", 4500, laptop), []string{"http://192.168.1.121:4500"})
}

func TestTheLocalLinkIsLocalhostUnlessTheDaemonListensOnOneAddress(t *testing.T) {
	expectLinks(t, []string{LocalURL("0.0.0.0", 4500), LocalURL("127.0.0.1", 4500), LocalURL("192.168.1.121", 4500)},
		[]string{"http://localhost:4500", "http://localhost:4500", "http://192.168.1.121:4500"})
}

func TestTheDaemonIsReachedOnLoopbackUnlessItListensOnOneAddress(t *testing.T) {
	expectLinks(t, []string{ControlURL("0.0.0.0", 4500), ControlURL("192.168.1.121", 4500)},
		[]string{"http://127.0.0.1:4500", "http://192.168.1.121:4500"})
	if !ListensOnNetwork("0.0.0.0") || ListensOnNetwork("127.0.0.1") {
		t.Fatal("only a daemon listening beyond loopback is on the network")
	}
}
