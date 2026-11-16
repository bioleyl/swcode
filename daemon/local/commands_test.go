package local

import (
	"slices"
	"strings"
	"testing"
)

var (
	wsl   = Place{Home: "/home/me", Platform: "linux", WSLDistro: "Ubuntu-26.04"}
	linux = Place{Home: "/home/me", Platform: "linux"}
	mac   = Place{Home: "/Users/me", Platform: "darwin"}
)

func TestTheAppOpensInThePersonsBrowserWindowsFromWSLTheDesktopsElsewhere(t *testing.T) {
	fromWSL := BrowserCall("http://localhost:4500/?token=t", wsl)
	if fromWSL.Program != "powershell.exe" || !slices.Contains(fromWSL.Args, "Start-Process 'http://localhost:4500/?token=t'") {
		t.Fatalf("WSL opens Windows' browser, found %+v", fromWSL)
	}
	onLinux := BrowserCall("http://localhost:4500/", linux)
	if onLinux.Program != "xdg-open" || !slices.Equal(onLinux.Args, []string{"http://localhost:4500/"}) {
		t.Fatalf("Linux opens the desktop's browser, found %+v", onLinux)
	}
	if onMac := BrowserCall("http://localhost:4500/", mac); onMac.Program != "open" {
		t.Fatalf("a Mac opens with open, found %+v", onMac)
	}
}

func TestTheWSLDaemonStartsInAHiddenWSLExeThatWindowsMakes(t *testing.T) {
	call := WSLDaemonCall("Ubuntu-26.04", "/home/me/.local/bin/swcode")
	script := call.Args[len(call.Args)-1]
	for _, part := range []string{
		"Invoke-CimMethod -ClassName Win32_Process -MethodName Create",
		"ShowWindow = [uint16]0",
		`$line = '"' + $wsl + '" ' + '-d Ubuntu-26.04 --exec /home/me/.local/bin/swcode daemon'`,
		"CommandLine = $line",
	} {
		if !strings.Contains(script, part) {
			t.Fatalf("the script has no %q:\n%s", part, script)
		}
	}
}

func TestAnInstalledAppWindowOpensByItsAppIDAndAPlainOneByItsLink(t *testing.T) {
	environment := map[string]string{"LOCALAPPDATA": `C:\Users\me\AppData\Local`}
	plain := AppWindowCall("http://127.0.0.1:4500/?token=t", `C:\Edge\msedge.exe`, environment, "")
	if !slices.Contains(plain.Args, "--app=http://127.0.0.1:4500/?token=t") {
		t.Fatalf("a window that is no app opens by its link, found %+v", plain)
	}
	if slices.Contains(plain.Args, "--profile-directory=Default") {
		t.Fatalf("a window that is no app opens no profile of its own, found %+v", plain)
	}
	installed := AppWindowCall("http://127.0.0.1:4500/?token=t", `C:\Edge\msedge.exe`, environment, "abcdefghijklmnopabcdefghijklmnop")
	if !slices.Contains(installed.Args, "--app-id=abcdefghijklmnopabcdefghijklmnop") {
		t.Fatalf("an installed app opens by its id, found %+v", installed)
	}
	if !slices.Contains(installed.Args, "--profile-directory=Default") {
		t.Fatalf("an installed app opens the profile it is installed in, found %+v", installed)
	}
}

func TestAWebAppsIDIsChromiumsLettersOfTheOrigin(t *testing.T) {
	id := webAppID("http://127.0.0.1:4500/")
	if len(id) != 32 || strings.Trim(id, "abcdefghijklmnop") != "" {
		t.Fatalf("an app id is 32 letters a to p, found %q", id)
	}
	if again := webAppID("http://127.0.0.1:4500/"); again != id {
		t.Fatalf("the same origin keeps its id, found %q then %q", id, again)
	}
	if other := webAppID("http://127.0.0.1:4501/"); other == id {
		t.Fatalf("another origin has another id, found %q", other)
	}
}

func TestAWindowIsNeverAnInstalledAppOffWindows(t *testing.T) {
	for _, place := range []Place{wsl, linux, mac} {
		if id := installedAppID("http://127.0.0.1:4500/", place); id != "" {
			t.Fatalf("%s found an app id %q", place.Platform, id)
		}
	}
}

func TestPowerShellStringsDoubleTheQuotesInside(t *testing.T) {
	if found := PowerShellString("it's"); found != "'it''s'" {
		t.Fatalf("found %s", found)
	}
}
