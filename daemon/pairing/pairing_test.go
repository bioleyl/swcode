package pairing

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func newPairing(t *testing.T) *Pairing {
	t.Helper()
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	return New(filepath.Join(t.TempDir(), "devices.json"), func() time.Time { return now }, func() string { return "phone" })
}

func TestAClientIsItsMachineWhateverTheConnection(t *testing.T) {
	cases := map[string]string{
		"192.168.1.7:51000":            "192.168.1.7",
		"192.168.1.7:51001":            "192.168.1.7",
		"[::ffff:192.168.1.7]:51000":   "192.168.1.7",
		"[2001:db8:1:2:aaaa::1]:51000": "2001:db8:1:2::/64",
		"[2001:db8:1:2:bbbb::2]:51000": "2001:db8:1:2::/64",
		"[fe80::1%eth0]:51000":         "fe80::/64",
		"not an address":               "not an address",
	}
	for remote, want := range cases {
		if got := ClientOf(remote); got != want {
			t.Errorf("%q: got %q, want %q", remote, got, want)
		}
	}
}

func TestOneMachineTriesNoMoreThanItsShareOverManyConnections(t *testing.T) {
	pairing := newPairing(t)
	for try := range TriesPerMinute {
		if !pairing.Allow(ClientOf("192.168.1.7:" + strconv.Itoa(40000+try))) {
			t.Fatalf("try %d was refused", try)
		}
	}
	if pairing.Allow(ClientOf("192.168.1.7:50000")) {
		t.Fatal("a new connection gave the same machine more tries")
	}
	if !pairing.Allow(ClientOf("192.168.1.8:50000")) {
		t.Fatal("another machine was refused")
	}
}

func TestTooManyWrongNumbersEndEveryWait(t *testing.T) {
	pairing := newPairing(t)
	pairing.Pend("123456")
	for range MissesPerWait {
		pairing.Redeem("000000", "", "")
	}
	if pairing.Redeem("123456", "", "") == "" {
		t.Fatal("the number stopped waiting before the limit")
	}

	pairing.Pend("123456")
	for range MissesPerWait + 1 {
		pairing.Redeem("000000", "", "")
	}
	if pairing.Redeem("123456", "", "") != "" {
		t.Fatal("the number still waited after more wrong numbers than a wait allows")
	}
}

func TestWrongNumbersAskedWhileNothingWaitsAreNotHeldAgainstTheNextWait(t *testing.T) {
	pairing := newPairing(t)
	// An app shows its number and asks until someone types it on the computer.
	for range MissesPerWait + 1 {
		pairing.Redeem("123456", "", "")
	}
	pairing.Pend("123456")
	if pairing.Redeem("123456", "", "") == "" {
		t.Fatal("the app's own asks before the number waited ended its wait")
	}
}
