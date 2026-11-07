//go:build windows

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"swcode/desktop/selfinstall"
)

const (
	// flagReplaces starts the newer version an update put in place of this program, once this one ended: it holds
	// the app's place (its mutex, its port, WebView2's profile) until then.
	flagReplaces = "--replaces"
	// The program replaced ends in a few seconds, its sessions stopped.
	replacedLimit = 60 * time.Second
	failedUpdate  = "swcode was updated, but did not start again: open it from the Start menu.\n\n%s"
)

// startReplacing starts the newer version, now in this program's place, with these arguments once this program
// ended: out of this one's job, which ends everything in it with this one.
func startReplacing(exe string, arguments ...string) error {
	words := append([]string{`"` + exe + `"`, flagReplaces, strconv.Itoa(os.Getpid())}, arguments...)
	return startOutside(strings.Join(words, " "))
}

// afterReplaced waits, for a program an update started, for the one it replaces to end, and gives the arguments
// that follow; others are given as they are.
func afterReplaced(arguments []string) []string {
	if len(arguments) < 2 || arguments[0] != flagReplaces {
		return arguments
	}
	if pid, err := strconv.ParseUint(arguments[1], 10, 32); err == nil {
		waitForEnd(uint32(pid))
	}
	return arguments[2:]
}

// waitForEnd waits for a process to end, up to a limit: it may have ended already.
func waitForEnd(pid uint32) {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return
	}
	defer windows.CloseHandle(process)
	_, _ = windows.WaitForSingleObject(process, uint32(replacedLimit.Milliseconds()))
}

// startUpdated starts the newer version in place of the app, as the app shows now (its window open, or only its
// icon), and ends this one.
func (w *window) startUpdated() {
	if err := startReplacing(w.daemon.exe, w.relaunchArguments()...); err != nil {
		showError(fmt.Sprintf(failedUpdate, err))
	}
	if isVisible(w.hwnd) {
		w.keepPlace()
	}
	w.end()
}

// relaunchArguments start the app as it shows now: a test copy where it is (--here), the installed one with its
// window open, or only its icon.
func (w *window) relaunchArguments() []string {
	if !selfinstall.Installed(w.daemon.exe, selfinstall.Target(os.Getenv("LOCALAPPDATA"))) {
		return []string{flagHere}
	}
	if isVisible(w.hwnd) {
		return nil
	}
	return []string{flagMinimized}
}
