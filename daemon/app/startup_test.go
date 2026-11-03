package app_test

import (
	"testing"

	"swcode/daemon/protocol"
	. "swcode/daemon/testapp"
)

type fakeStartup struct{ mode protocol.StartupMode }

func (f *fakeStartup) Mode() protocol.StartupMode { return f.mode }

func (f *fakeStartup) SetMode(mode protocol.StartupMode) error {
	f.mode = mode
	return nil
}

func TestTheWindowsAppSaysHowItStartsAndChangesIt(t *testing.T) {
	startup := &fakeStartup{mode: protocol.StartupModeOff}
	test := Start(t, Options{Startup: startup})
	_, view := test.Call("GET", protocol.EndpointStartup, nil)
	Expect(t, view, `{"available":true,"mode":"off"}`)
	status, changed := test.Call("PUT", protocol.EndpointStartup, map[string]string{"mode": "tray"})
	if status != 200 || startup.mode != protocol.StartupModeTray {
		t.Fatalf("status %d, mode %s: %v", status, startup.mode, changed)
	}
	Expect(t, changed, `{"available":true,"mode":"tray"}`)
	status, _ = test.Call("PUT", protocol.EndpointStartup, map[string]string{"mode": "always"})
	if status != 400 || startup.mode != protocol.StartupModeTray {
		t.Fatalf("an unknown way to start was taken: status %d, mode %s", status, startup.mode)
	}
}

func TestADaemonWithoutTheWindowsAppDoesNotStartWithItsComputer(t *testing.T) {
	test := Start(t, Options{})
	_, view := test.Call("GET", protocol.EndpointStartup, nil)
	Expect(t, view, `{"available":false,"mode":"off"}`)
	if status, _ := test.Call("PUT", protocol.EndpointStartup, map[string]string{"mode": "tray"}); status != 501 {
		t.Fatalf("status %d", status)
	}
}
