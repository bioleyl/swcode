package local_test

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"swcode/daemon/config"
	"swcode/daemon/local"
	"swcode/daemon/testapp"
)

func TestPairingAsksTheDaemonForACodeAndSaysWhereItIsReached(t *testing.T) {
	test := testapp.Start(t, testapp.Options{})
	address, _ := url.Parse(test.Server.URL)
	port, _ := strconv.Atoi(address.Port())
	settings := config.Config{DataDir: test.DataDir, Host: address.Hostname(), Port: port, Token: testapp.Token}
	pairing, err := local.PairingOf(settings)
	if err != nil || len(pairing.Code) == 0 || pairing.LifetimeMs <= 0 {
		t.Fatalf("found %+v (%v)", pairing, err)
	}
	text := local.PairingText(pairing)
	if !strings.HasPrefix(text, "Pairing code: "+pairing.Code) || !strings.Contains(text, "listens on this computer only") {
		t.Fatalf("found %s", text)
	}
	if !strings.Contains(local.PairingText(local.Pairing{Code: "1", LifetimeMs: 60000, Links: []string{"http://192.168.1.20:4500"}}), "\n  http://192.168.1.20:4500") {
		t.Fatal("the addresses are not listed")
	}
}

func TestPairingSaysWhenSwcodeDoesNotRun(t *testing.T) {
	_, err := local.PairingOf(config.Config{DataDir: t.TempDir(), Host: "127.0.0.1", Port: 1, Token: testapp.Token})
	if err == nil || !strings.Contains(err.Error(), "does not run") {
		t.Fatalf("found %v", err)
	}
}
