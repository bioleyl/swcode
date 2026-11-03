package local

import (
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
	windowProfile = `swcode\window`
	windowSize    = "--window-size=1400,900"
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
func AppWindowCall(link, edge string, environment map[string]string) Call {
	data := environment["LOCALAPPDATA"]
	if data == "" {
		data = environment["USERPROFILE"]
	}
	return Call{Args: []string{
		"--app=" + link, "--user-data-dir=" + filepath.Join(data, windowProfile), "--no-first-run",
		"--no-default-browser-check", windowSize,
	}, Program: edge}
}
