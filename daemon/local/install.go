package local

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"swcode"
	"swcode/daemon/app"
	"swcode/daemon/config"
)

const (
	binFolder    = ".local/bin"
	binaryName   = "swcode"
	executable   = 0o755
	newSuffix    = ".new"
	noWebApp     = "This program carries no web app: build it with npm run build:binary."
	shortcutFile = "shortcut.ps1"
	firewallFile = "firewall.ps1"
	iconFile     = "swcode.ico"
	windowsName  = "swcode"
	firewallRule = "swcode"
	wslProgram   = `C:\Windows\System32\wsl.exe`
	programsName = "Programs"
	exeName      = "swcode.exe"
)

// replaceFile copies a file in place of another, through a new name: a running program is never half written.
func replaceFile(source, target string, mode os.FileMode) error {
	if same, _ := filepath.Abs(source); same == target {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	from, err := os.Open(source)
	if err != nil {
		return err
	}
	defer from.Close()
	to, err := os.OpenFile(target+newSuffix, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(to, from); err != nil {
		to.Close()
		return err
	}
	if err := to.Close(); err != nil {
		return err
	}
	return os.Rename(target+newSuffix, target)
}

// firstSettings writes the settings file the first time only: after that, the settings page changes it.
func firstSettings(path string, environment map[string]string) {
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("Kept %s: change the settings in the app's settings page.\n", path)
		return
	}
	if err := config.WriteSettings(path, environment); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("Wrote %s.\n", path)
}

func onPath(folder string) bool {
	for _, each := range filepath.SplitList(os.Getenv("PATH")) {
		if each == folder {
			return true
		}
	}
	return false
}

// Install installs the running program: as ~/.local/bin/swcode, with the settings of this environment kept in the
// data folder, and on WSL a shortcut in Windows and the firewall rule for phones; on Windows, in the person's
// programs folder, with shortcuts and the PATH. Copied to another computer, the program installs itself there the
// same way.
func Install(environment map[string]string, place Place) int {
	if swcode.WebApp() == nil {
		fmt.Fprintln(os.Stderr, noWebApp)
		return failed
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	if place.Platform == "windows" {
		return installNative(self, environment, place)
	}
	target := filepath.Join(place.Home, binFolder, binaryName)
	if err := replaceFile(self, target, executable); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	fmt.Printf("Installed %s.\n", target)
	firstSettings(config.SettingsPathOf(environment, place.Home), environment)
	settings, err := config.Load(config.Environment(environment, place.Home), config.Defaults{Cwd: place.Home, Home: place.Home})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	if place.WSLDistro != "" {
		installForWindows(place.WSLDistro, target, settings.Port, app.ListensOnNetwork(settings.Host))
	}
	found := ""
	if !onPath(filepath.Dir(target)) {
		found = fmt.Sprintf(" (open a new terminal first: %s is not on its PATH yet)", filepath.Dir(target))
	}
	fmt.Printf("Run swcode to open it%s.\n", found)
	return 0
}

// windowsFolder puts what Windows runs in a folder of its own, from WSL: Windows reads it there without WSL
// running. It gives the folder as Windows names it and as WSL reaches it.
func windowsFolder() (string, string, bool) {
	local := Run(PowerShell("[Environment]::GetFolderPath('LocalApplicationData')"))
	if !local.OK || local.Output == "" {
		return "", "", false
	}
	windows := local.Output + `\` + windowsName
	unix := Run(Call{Args: []string{"-u", windows}, Program: "wslpath"})
	if !unix.OK || os.MkdirAll(unix.Output, 0o755) != nil {
		return "", "", false
	}
	for _, name := range []string{shortcutFile, firewallFile, iconFile} {
		data, _ := swcode.WindowsFile(name)
		_ = os.WriteFile(filepath.Join(unix.Output, name), data, 0o644)
	}
	return windows, unix.Output, true
}

func ruleOpens(port int) bool {
	ports := Run(PowerShell(fmt.Sprintf("(Get-NetFirewallHyperVRule -Name %s -ErrorAction SilentlyContinue).LocalPorts", firewallRule)))
	return ports.Output == fmt.Sprint(port)
}

// openFirewall asks, with a UAC prompt, for the rule that lets phones on private networks reach the daemon's port:
// WSL's own firewall (Hyper-V's) drops what comes in from the network, and Windows never asks about it.
func openFirewall(windows string, port int) {
	if ruleOpens(port) {
		fmt.Printf("WSL's firewall already lets phones on the network reach port %d.\n", port)
		return
	}
	fmt.Printf("Windows asks for an administrator: accept to let phones on the network reach port %d.\n", port)
	words := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", `"` + windows + `\` + firewallFile + `"`, "-Port", fmt.Sprint(port)}
	quoted := make([]string, len(words))
	for index, word := range words {
		quoted[index] = PowerShellString(word)
	}
	Run(PowerShell("Start-Process powershell.exe -Verb RunAs -Wait -WindowStyle Hidden -ArgumentList " + strings.Join(quoted, ",")))
	if ruleOpens(port) {
		fmt.Printf("Phones on the network can reach port %d now.\n", port)
		return
	}
	fmt.Println("The firewall rule was not made: run swcode install again to be asked again.")
}

// installForWindows makes, from WSL, a swcode shortcut in the Start menu and on the desktop, and the firewall rule
// for phones.
func installForWindows(distro, binary string, port int, open bool) {
	windows, _, ok := windowsFolder()
	if !ok {
		fmt.Fprintln(os.Stderr, "Windows did not answer: no shortcut, no firewall rule. Is WSL’s Windows interop on?")
		return
	}
	shortcuts := Run(PowerShellFile(windows+`\`+shortcutFile, "-Target", wslProgram, "-Arguments", "-d "+distro+" --exec "+binary+" open", "-Icon", windows+`\`+iconFile))
	fmt.Println(shortcuts.Output)
	if !open {
		fmt.Println("swcode listens on this computer only (SWCODE_HOST): no firewall rule needed.")
		return
	}
	openFirewall(windows, port)
}

// addToPath puts a folder on the person's PATH, for the swcode command in new terminals.
func addToPath(folder string) Call {
	return PowerShell(strings.Join([]string{
		"$path = [Environment]::GetEnvironmentVariable('Path', 'User')",
		"$folder = " + PowerShellString(folder),
		"if (-not (($path -split ';') -contains $folder)) { [Environment]::SetEnvironmentVariable('Path', ($path.TrimEnd(';') + ';' + $folder).TrimStart(';'), 'User'); 'added' }",
	}, "; "))
}

// installNative installs on Windows itself: swcode.exe in the person's programs folder, shortcuts in the Start menu
// and on the desktop, and the folder on the PATH. Windows asks about the firewall by itself the first time the
// daemon listens on the network.
func installNative(self string, environment map[string]string, place Place) int {
	local := environment["LOCALAPPDATA"]
	if local == "" {
		fmt.Fprintln(os.Stderr, "Windows did not say where programs go (LOCALAPPDATA).")
		return failed
	}
	folder := filepath.Join(local, programsName, windowsName)
	target := filepath.Join(folder, exeName)
	if err := replaceFile(self, target, executable); err != nil {
		fmt.Fprintln(os.Stderr, "swcode runs from there: stop it first (swcode stop), then install again. ("+err.Error()+")")
		return failed
	}
	for _, name := range []string{shortcutFile, iconFile} {
		data, _ := swcode.WindowsFile(name)
		_ = os.WriteFile(filepath.Join(folder, name), data, 0o644)
	}
	fmt.Println(Run(PowerShellFile(filepath.Join(folder, shortcutFile), "-Target", target, "-Icon", filepath.Join(folder, iconFile))).Output)
	if Run(addToPath(folder)).Output == "added" {
		fmt.Printf("Added %s to your PATH.\n", folder)
	} else {
		fmt.Printf("%s is on your PATH.\n", folder)
	}
	fmt.Printf("Installed %s.\n", target)
	firstSettings(config.SettingsPathOf(environment, place.Home), environment)
	fmt.Println("Run swcode to open it (in a new terminal), or its shortcut.")
	return 0
}
