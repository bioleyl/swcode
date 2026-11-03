//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"

	"swcode/desktop/selfinstall"
)

const appRunning = "swcode is open: quit it (Quit swcode, in its icon's menu), then open this one again to install it."

// runOrInstall runs the app its person opened, from the programs folder. Opened from anywhere else (a download), it
// installs itself there first, in place of an older one, and starts from there: the shortcuts it makes open that
// one. A test copy runs where it is with --here.
func runOrInstall() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	target := selfinstall.Target(os.Getenv("LOCALAPPDATA"))
	if selfinstall.Installed(exe, target) {
		return runApp(true)
	}
	// A program that runs cannot be replaced: the app's, or the daemon's alone.
	if running(instanceMutex) || running(daemonMutex) {
		messageBox(0, appRunning, mbIconInformation|mbSetForeground)
		return nil
	}
	if err := selfinstall.Copy(exe, target); err != nil {
		return err
	}
	// Out of any job this copy may be in: the installed app outlives it.
	return startOutside(`"` + target + `"`)
}

// running tells whether a program of swcode holds that mutex, without taking it.
func running(mutex string) bool {
	handle, err := windows.OpenMutex(windows.SYNCHRONIZE, false, utf16(mutex))
	if err != nil {
		return false
	}
	_ = windows.CloseHandle(handle)
	return true
}
