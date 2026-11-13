package hosts

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
)

// Shell is the program a command line runs with, and how the system prompt names it.
type Shell struct {
	Program string
	Args    func(command string) []string
	// Interactive starts it for a person to type in, with no command: a terminal of the app runs one.
	Interactive []string
	// Label is said to the model beside the platform; empty when it is plain bash.
	Label string
	// Announce is typed once in a terminal's shell, before the first command the agent waits for there: it defines
	// the function a command's line ends with (ended), which prints the mark it is given and the exit code of the
	// command before it, in an escape no terminal shows. Wipe clears the view after it.
	Announce string
	Wipe     string
}

// ended is the name of the function Announce defines in a terminal's shell.
const ended = "swdone"

// What a shell prints once a command typed in a terminal ended: an operating system command escape, which a
// terminal's view keeps to itself, carrying the mark the command was typed with and its exit code.
const (
	endedStart = "\x1b]777;swcode;"
	endedStop  = '\x07'
)

// CommandEnd finds, in what a terminal printed, where its shell said the command typed with that mark ended, and
// the command's exit code: 0 when the shell knew none (PowerShell, after a command that is no program).
func CommandEnd(output []byte, mark string) (at, code int, ended bool) {
	start := []byte(endedStart + mark + ";")
	at = bytes.Index(output, start)
	if at < 0 {
		return 0, 0, false
	}
	said := output[at+len(start):]
	stop := bytes.IndexByte(said, endedStop)
	if stop < 0 {
		return 0, 0, false
	}
	code, _ = strconv.Atoi(string(said[:stop]))
	return at, code, true
}

// typedEnd is what follows a command on its line in a terminal's shell, for the shell to say when it ended.
func typedEnd(mark string) string {
	return "; " + ended + " " + mark
}

// AfterEcho is what a terminal printed for a command, without the line it was typed on, which the shell echoes:
// what comes after the last place its end shows as typed.
func AfterEcho(output []byte, mark string) []byte {
	typed := []byte(typedEnd(mark))
	at := bytes.LastIndex(output, typed)
	if at < 0 {
		return output
	}
	rest := output[at+len(typed):]
	return rest[bytes.IndexByte(rest, '\n')+1:]
}

// Bash runs commands with bash -c, and starts bash -i for a terminal.
var Bash = Shell{
	// printf writes the escape from its octal form, so the line that defines the function shows none of its own.
	Announce:    ended + `() { printf '\033]777;swcode;%s;%s\007' "$1" "$?"; }`,
	Args:        func(command string) []string { return []string{"-c", command} },
	Interactive: []string{"-i"},
	Program:     "bash",
	Wipe:        "clear",
}

// Windows PowerShell 5.1 is on every Windows; its output is UTF-8 only when asked.
var powerShell = Shell{
	Args: func(command string) []string {
		return []string{
			"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command",
			"[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " + command,
		}
	},
	Announce:    "function " + ended + `($mark) { Write-Host -NoNewline ("{0}]777;swcode;{1};{2}{3}" -f [char]27, $mark, $LASTEXITCODE, [char]7) }`,
	Interactive: []string{"-NoLogo"},
	Label:       "commands run in Windows PowerShell 5.1: there is no bash here",
	Program:     "powershell.exe",
	Wipe:        "Clear-Host",
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
			return Shell{
				Announce: Bash.Announce, Args: Bash.Args, Interactive: Bash.Interactive, Label: "commands run in Git Bash", Program: place,
				Wipe: Bash.Wipe,
			}
		}
	}
	return powerShell
}
