//go:build windows

package main

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"

	"swcode/daemon/local"
)

// daemonInstance is held as long as the daemon alone runs: a second one finds it taken.
var daemonInstance windows.Handle

// runDaemon is swcode-desktop.exe --daemon: the daemon alone, without window nor icon, until it is stopped (swcode
// stop, or the app's Quit does not: it only shows it). The app, opened later, shows it; new settings restart it.
func runDaemon() error {
	handle, err := windows.CreateMutex(nil, false, utf16(daemonMutex))
	daemonInstance = handle
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	server, err := newDaemon(exe)
	if err != nil {
		return err
	}
	for {
		run, err := server.start()
		if err != nil {
			return err
		}
		if !run.restarts() {
			return run.problem()
		}
	}
}

// showPairing is swcode-desktop.exe --pair: a code that pairs another computer's swcode with this one's, which runs.
func showPairing() error {
	settings, _, err := loadSettings()
	if err != nil {
		return err
	}
	pairing, err := local.PairingOf(settings)
	if err != nil {
		return err
	}
	messageBox(0, local.PairingText(pairing), mbIconInformation|mbSetForeground)
	return nil
}
