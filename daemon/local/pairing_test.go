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

func TestPairingMakesTheNumberWaitAndSaysWhereItIsReached(t *testing.T) {
	test := testapp.Start(t, testapp.Options{})
	address, _ := url.Parse(test.Server.URL)
	port, _ := strconv.Atoi(address.Port())
	settings := config.Config{DataDir: test.DataDir, Host: address.Hostname(), Port: port, Token: testapp.Token}
	expecting, err := local.Expect(settings, "123456")
	if err != nil || expecting.Number != "123456" || expecting.LifetimeMs <= 0 {
		t.Fatalf("found %+v (%v)", expecting, err)
	}
	text := local.PairingText(expecting)
	if !strings.HasPrefix(text, "Pairing number 123456 waits for 5 minutes.") || !strings.Contains(text, "listens on this computer only") {
		t.Fatalf("found %s", text)
	}
	if !strings.Contains(local.PairingText(local.Expecting{Number: "1", LifetimeMs: 300000, Links: []string{"http://192.168.1.20:4500"}}), "\n  http://192.168.1.20:4500") {
		t.Fatal("the addresses are not listed")
	}
}

func TestPairingSaysWhenSwcodeDoesNotRun(t *testing.T) {
	_, err := local.Expect(config.Config{DataDir: t.TempDir(), Host: "127.0.0.1", Port: 1, Token: testapp.Token}, "123456")
	if err == nil || !strings.Contains(err.Error(), "does not run") {
		t.Fatalf("found %v", err)
	}
}
