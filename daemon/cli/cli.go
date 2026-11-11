// Package cli reads the command line: open the app (starting the daemon when it does not run), start and stop it,
// install the program, run it as a service, update it.
package cli

import (
	"fmt"
	"strings"
)

// Kind is what the command line asks for.
type Kind string

const (
	KindOpen             Kind = "open"
	KindServe            Kind = "serve"
	KindStart            Kind = "start"
	KindStop             Kind = "stop"
	KindInstall          Kind = "install"
	KindServiceInstall   Kind = "service-install"
	KindServiceUninstall Kind = "service-uninstall"
	KindPair             Kind = "pair"
	KindUpdate           Kind = "update"
	KindVersion          Kind = "version"
	KindHelp             Kind = "help"
)

// Mode is where the daemon runs: in a terminal, in the background (its log in a file), as a service (which starts it
// again), or inside the desktop app.
type Mode string

const (
	ModeTerminal   Mode = "terminal"
	ModeBackground Mode = "background"
	ModeService    Mode = "service"
	ModeDesktop    Mode = "desktop"
)

// Command is what the arguments ask for.
type Command struct {
	Kind Kind
	Mode Mode
	// Problem is why the arguments were not understood, with help.
	Problem string
}

// plainCommands are the commands that take no arguments.
var plainCommands = map[string]Command{
	"daemon":  {Kind: KindServe, Mode: ModeBackground},
	"install": {Kind: KindInstall},
	"open":    {Kind: KindOpen},
	"pair":    {Kind: KindPair},
	"start":   {Kind: KindStart},
	"stop":    {Kind: KindStop},
}

var (
	versionFlags = map[string]bool{"--version": true, "-v": true, "version": true}
	helpFlags    = map[string]bool{"--help": true, "-h": true, "help": true}
	updateFlags  = map[string]bool{"--update": true}
)

const (
	serveWord   = "serve"
	serviceWord = "service"
	// serviceFlag runs the daemon so that its service manager starts it again after a restart or an update.
	serviceFlag = "--service"
)

func help(problem string) Command { return Command{Kind: KindHelp, Problem: problem} }

// Parse reads the arguments; anything not understood asks for help, saying why.
func Parse(arguments []string) Command {
	if len(arguments) == 0 {
		return Command{Kind: KindOpen}
	}
	first, rest := arguments[0], arguments[1:]
	if plain, ok := plainCommands[first]; ok {
		if len(rest) > 0 {
			return help(fmt.Sprintf("swcode %s takes nothing after it.", first))
		}
		return plain
	}
	switch first {
	case serveWord:
		return serveCommand(rest)
	case serviceWord:
		return serviceCommand(rest)
	}
	if versionFlags[first] {
		return Command{Kind: KindVersion}
	}
	if helpFlags[first] {
		return Command{Kind: KindHelp}
	}
	if updateFlags[first] {
		if len(rest) > 0 {
			return help("swcode --update takes nothing after it.")
		}
		return Command{Kind: KindUpdate}
	}
	return help(fmt.Sprintf("swcode does not know %s.", first))
}

func serveCommand(arguments []string) Command {
	if len(arguments) == 0 {
		return Command{Kind: KindServe, Mode: ModeTerminal}
	}
	if len(arguments) == 1 && arguments[0] == serviceFlag {
		return Command{Kind: KindServe, Mode: ModeService}
	}
	return help("swcode serve takes nothing, or --service when a service manager runs it.")
}

func serviceCommand(arguments []string) Command {
	if len(arguments) != 1 {
		return help("swcode service takes install or uninstall.")
	}
	switch arguments[0] {
	case "install":
		return Command{Kind: KindServiceInstall}
	case "uninstall":
		return Command{Kind: KindServiceUninstall}
	}
	return help("swcode service takes install or uninstall.")
}

// Usage is what swcode --help prints, for the build that runs.
func Usage(build string) string {
	return strings.Join([]string{
		"swcode " + build,
		"A coding agent for llm-server.",
		"",
		"Usage:",
		"  swcode                      Open swcode in the browser, starting its daemon if needed.",
		"  swcode start                Start the daemon in the background.",
		"  swcode stop                 Stop the daemon.",
		"  swcode serve                Run the daemon in this terminal (Ctrl+C stops it).",
		"  swcode pair                 A code to pair another computer's swcode, or a phone, with this one.",
		"  swcode install              Install this program: the swcode command, its settings and, on WSL, a",
		"                              shortcut in Windows. The LLAMA_* and SWCODE_* settings set now are kept.",
		"  swcode service install      Run swcode as a service: it starts when you log in.",
		"  swcode service uninstall    Remove the service; settings and sessions are kept.",
		"  swcode --update             Install the latest release, then restart the daemon if it runs.",
		"  swcode --version            The build.",
		"",
		"Settings come from ~/.swcode/settings.env, the environment on top: see the README.",
	}, "\n")
}
