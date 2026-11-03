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

func TestPowerShellStringsDoubleTheQuotesInside(t *testing.T) {
	if found := PowerShellString("it's"); found != "'it''s'" {
		t.Fatalf("found %s", found)
	}
}
