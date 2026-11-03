package hosts

import (
	"os"
	"path/filepath"
)

// Shell is the program a command line runs with, and how the system prompt names it.
type Shell struct {
	Program string
	Args    func(command string) []string
	// Label is said to the model beside the platform; empty when it is plain bash.
	Label string
}

// Bash runs commands with bash -c.
var Bash = Shell{Args: func(command string) []string { return []string{"-c", command} }, Program: "bash"}

// Windows PowerShell 5.1 is on every Windows; its output is UTF-8 only when asked.
var powerShell = Shell{
	Args: func(command string) []string {
		return []string{
			"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command",
			"[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " + command,
		}
	},
	Label:   "commands run in Windows PowerShell 5.1: there is no bash here",
	Program: "powershell.exe",
}

// Where Git for Windows puts its bash: for everyone, for the 32-bit Program Files, or for this person only.
const (
	gitBash      = `Git\bin\bash.exe`
	userPrograms = "Programs"
)

var programFolders = []string{"ProgramFiles", "ProgramFiles(x86)"}

func gitBashPlaces(environment map[string]string) []string {
	places := []string{}
	for _, name := range programFolders {
		if folder := environment[name]; folder != "" {
			places = append(places, filepath.Join(folder, gitBash))
		}
	}
	if local := environment["LOCALAPPDATA"]; local != "" {
		places = append(places, filepath.Join(local, userPrograms, gitBash))
	}
	return places
}

// WindowsShell is the shell of commands on Windows: Git Bash when Git for Windows is installed (models know bash
// best), else Windows PowerShell. Never the bash.exe of System32, which is WSL's.
func WindowsShell(environment map[string]string) Shell {
	for _, place := range gitBashPlaces(environment) {
		if _, err := os.Stat(place); err == nil {
			return Shell{Args: Bash.Args, Label: "commands run in Git Bash", Program: place}
		}
	}
	return powerShell
}
