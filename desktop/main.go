//go:build windows

// swcode's desktop app for Windows, one program: a window of its own, Edge's WebView2, on swcode's server, which
// runs inside it, and an icon in the notification area. Closing the window leaves the app there, the server working
// and telling what needs its person in Windows' notifications; Quit, in the icon's menu, stops it, asking first
// while a session works. With --daemon it is the server alone, which the app shows when it starts after it; the
// window also shows other computers' swcode, paired with it. It runs once: opened again, its window comes forward.
// Opened from anywhere but the programs folder (a download), it installs itself there first; --here runs a copy
// where it is, to try a build. A newer version installed from the app's Updates takes its place and starts again.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/wailsapp/wails/webview2/webviewloader"
	"golang.org/x/sys/windows"
)

const (
	appName = "swcode"
	// The app's own folders: the window's place and the desktop shortcut's mark where Electron kept them (roaming),
	// WebView2's profile on this computer only.
	dataFolderName    = "swcode"
	webviewFolderName = `swcode\webview`
	instanceMutex     = `Local\swcode-desktop`
	daemonMutex       = `Local\swcode-daemon`
	webviewDownload   = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"
	dataFolderMode    = 0o755
	dataFileMode      = 0o644
	opaque            = 255

	flagMinimized = "--minimized"
	flagDaemon    = "--daemon"
	flagPair      = "--pair"
	flagHere      = "--here"
	failedStart   = "swcode did not start.\n\n%s"
	failedPairing = "No pairing code.\n\n%s"
)

type appFolders struct {
	data    string
	webview string
}

// launch is how the program was started: the app (installed first when it is not), the app in the notification area
// only (at logon), the daemon alone, for a pairing code, or the app where it is, not installed.
type launch int

const (
	launchApp launch = iota
	launchTray
	launchDaemon
	launchPair
	launchHere
)

var launchFlags = map[string]launch{
	flagMinimized: launchTray, flagDaemon: launchDaemon, flagPair: launchPair, flagHere: launchHere,
}

// launches is what each launch runs, and how its failure is told.
var launches = map[launch]struct {
	run     func() error
	failure string
}{
	launchApp:    {run: runOrInstall, failure: failedStart},
	launchHere:   {run: func() error { return runApp(true) }, failure: failedStart},
	launchTray:   {run: func() error { return runApp(false) }, failure: failedStart},
	launchDaemon: {run: runDaemon, failure: failedStart},
	launchPair:   {run: showPairing, failure: failedPairing},
}

func launchOf(arguments []string) launch {
	if len(arguments) == 0 {
		return launchApp
	}
	if found, known := launchFlags[arguments[0]]; known {
		return found
	}
	return launchApp
}

func init() {
	// Windows belong to the thread that made them, and so does COM's apartment: the main goroutine keeps its thread.
	runtime.LockOSThread()
}

func main() {
	started := launches[launchOf(afterReplaced(os.Args[1:]))]
	if err := started.run(); err != nil {
		showError(fmt.Sprintf(started.failure, err))
		os.Exit(1)
	}
}

// runApp runs the app: its window opened, or only its icon, as at logon.
func runApp(opened bool) error {
	if !firstInstance() {
		if opened {
			bringForward()
		}
		return nil
	}
	if version, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString(""); err != nil || version == "" {
		return fmt.Errorf("it needs Microsoft Edge WebView2, which Windows 11 has: %s", webviewDownload)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	folders := appFolders{
		data:    filepath.Join(os.Getenv("APPDATA"), dataFolderName),
		webview: filepath.Join(os.Getenv("LOCALAPPDATA"), webviewFolderName),
	}
	server, err := newDaemon(exe)
	if err != nil {
		return err
	}
	first, err := server.local()
	if err != nil {
		return err
	}
	keepShortcuts(exe, folders)
	if err := openWindow(server, first, folders, opened); err != nil {
		server.stop(first)
		return err
	}
	messageLoop()
	return nil
}

// instance is held as long as the app runs: a second one finds it taken.
var instance windows.Handle

func firstInstance() bool {
	handle, err := windows.CreateMutex(nil, false, utf16(instanceMutex))
	instance = handle
	return !errors.Is(err, windows.ERROR_ALREADY_EXISTS)
}

// bringForward has the app already running open its window, rather than a second app: it may bring it to the front,
// which Windows allows the program the person just started.
func bringForward() {
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(utf16(windowClass))), 0)
	if hwnd == 0 {
		return
	}
	process := uint32(0)
	_, _, _ = procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&process)))
	_, _, _ = procAllowSetForegroundWindow.Call(uintptr(process))
	postMessage(hwnd, wmOpen, 0)
}
