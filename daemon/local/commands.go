package local

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	powershellProgram = "powershell.exe"
	quote             = "'"
	daemonCommand     = "daemon"
	edgePath          = `Microsoft\Edge\Application\msedge.exe`
	// windowProfile is the window's own browser profile: it keeps the access token and the window's size, apart
	// from the person's.
	windowProfile     = `swcode\window`
	windowProfileName = "Default"
	webAppFolder      = "Web Applications"
	windowSize        = "--window-size=1400,900"
)

var (
	powershellFlags = []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass"}
	edgeFolders     = []string{"ProgramFiles(x86)", "ProgramFiles"}
)

// Place is where swcode runs: the system, the WSL distribution when it runs in one, the home folder, and the
// environment (where Windows keeps programs and app data).
type Place struct {
	Platform    string
	WSLDistro   string
	Home        string
	Environment map[string]string
}

// PowerShellString is a value as a PowerShell string: single quotes, a quote inside doubled.
func PowerShellString(value string) string {
	return quote + strings.ReplaceAll(value, quote, quote+quote) + quote
}

// PowerShell runs a command, from WSL or Windows.
func PowerShell(command string) Call {
	return Call{Args: append(append([]string{}, powershellFlags...), "-Command", command), Program: powershellProgram}
}

// PowerShellFile runs a script file, with its parameters.
func PowerShellFile(file string, parameters ...string) Call {
	return Call{Args: append(append(append([]string{}, powershellFlags...), "-File", file), parameters...), Program: powershellProgram}
}

// BrowserCall opens a link in the person's browser: Windows' from WSL, the desktop's elsewhere.
func BrowserCall(link string, place Place) Call {
	switch {
	case place.WSLDistro != "":
		// The browser of Windows: the one the person uses, WSL having none.
		return PowerShell("Start-Process " + PowerShellString(link))
	case place.Platform == "darwin":
		return Call{Args: []string{link}, Program: "open"}
	case place.Platform == "windows":
		return Call{Args: []string{"/c", "start", `""`, link}, Program: "cmd.exe"}
	}
	return Call{Args: []string{link}, Program: "xdg-open"}
}

// WSLDaemonCall starts the daemon in the background from the WSL program: through a wsl.exe of its own on the
// Windows side, since WSL stops a distribution once no wsl.exe uses it, whatever runs inside. Windows makes that
// wsl.exe itself (WMI's Win32_Process.Create), hidden: one started from WSL would be in the job of the session that
// started it, and end with it, as a shortcut's session ends at once.
func WSLDaemonCall(distro, binary string) Call {
	args := strings.Join([]string{"-d", distro, "--exec", binary, daemonCommand}, " ")
	return PowerShell(strings.Join([]string{
		`$wsl = Join-Path $env:WINDIR 'System32\wsl.exe'`,
		`$line = '"' + $wsl + '" ' + ` + PowerShellString(args),
		"$startup = New-CimInstance -ClassName Win32_ProcessStartup -ClientOnly -Property @{ ShowWindow = [uint16]0 }",
		"$made = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{ CommandLine = $line; ProcessStartupInformation = $startup }",
		"if ($made.ReturnValue -ne 0) { exit $made.ReturnValue }",
	}, "; "))
}

// DetachedDaemonCall starts the daemon in the background elsewhere: the same program, detached.
func DetachedDaemonCall(executable string) Call {
	return Call{Args: []string{daemonCommand}, Program: executable}
}

// EdgeOf is Edge, where Windows 10 and 11 put it; empty when it is not there.
func EdgeOf(environment map[string]string) string {
	for _, name := range edgeFolders {
		if folder := environment[name]; folder != "" {
			path := filepath.Join(folder, edgePath)
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}
	return ""
}

// AppWindowCall is swcode in a window of its own, as an app: Edge in app mode, with its own profile, so no tabs, no
// address bar and its own icon in the taskbar. It is started apart: the swcode command ends while the window stays.
// Installed as an app, the window is that app's: Edge knows its icon and the taskbar lists it by name.
func AppWindowCall(link, edge string, environment map[string]string, appID string) Call {
	target := "--app=" + link
	args := []string{"--user-data-dir=" + windowFolder(environment), "--no-first-run", "--no-default-browser-check"}
	if appID == "" {
		args = append(args, target, windowSize)
	} else {
		args = append(args, "--app-id="+appID, "--profile-directory="+windowProfileName)
	}
	return Call{Args: args, Program: edge}
}

// windowFolder is the folder Edge keeps the window's own profile in.
func windowFolder(environment map[string]string) string {
	data := environment["LOCALAPPDATA"]
	if data == "" {
		data = environment["USERPROFILE"]
	}
	return filepath.Join(data, windowProfile)
}

// installedAppID is the id Edge installed swcode's window as an app under, empty when it did not: Chromium files an
// installed web app under its manifest's id, here the origin the page is served at, hashed into a's and p's.
func installedAppID(link string, place Place) string {
	if place.Platform != "windows" || place.WSLDistro != "" {
		return ""
	}
	address, err := url.Parse(link)
	if err != nil || address.Host == "" {
		return ""
	}
	id := webAppID(address.Scheme + "://" + address.Host + "/")
	installed := filepath.Join(windowFolder(place.Environment), windowProfileName, webAppFolder, id)
	if _, err := os.Stat(installed); err != nil {
		return ""
	}
	return id
}

// webAppID is Chromium's app id for a manifest id: the SHA-256's first 32 hexadecimal digits, each a letter, 0 to 9
// being a to j and a to f being k to p (web_app::GenerateAppIdFromManifestId).
func webAppID(manifestID string) string {
	digest := sha256.Sum256([]byte(manifestID))
	digits := hex.EncodeToString(digest[:])
	id := make([]byte, 0, 32)
	for _, digit := range digits[:32] {
		if digit <= '9' {
			id = append(id, byte('a'+digit-'0'))
			continue
		}
		id = append(id, byte('k'+digit-'a'))
	}
	return string(id)
}
