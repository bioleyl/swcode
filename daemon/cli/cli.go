// Package cli reads the command line: open the app (starting the daemon when it does not run), start and stop it,
// install the program, run it as a service, update it, or run one turn of a session without the app.
package cli

import (
	"fmt"
	"strings"
	"time"
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
	KindRun              Kind = "run"
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

// RunOptions is what swcode run asks for: one turn of a new coding session, on a prompt, without the app.
type RunOptions struct {
	// Cwd is the folder the session works in; the current one when empty.
	Cwd string
	// Permissions is how the session asks: auto, acceptEdits or ask. Nobody is there to answer, so what asks is
	// declined.
	Permissions string
	// JSON prints the session's events, a line each, then how the run went.
	JSON    bool
	Timeout time.Duration
	Prompt  string
}

// Command is what the arguments ask for.
type Command struct {
	Kind Kind
	Mode Mode
	// Problem is why the arguments were not understood, with help.
	Problem string
	// Exit is the exit code a problem ends with, when it is not the usual one.
	Exit int
	// Run is what swcode run asks for.
	Run RunOptions
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
	// DefaultRunPermissions: a run has nobody to ask, so it runs everything unless told otherwise.
	DefaultRunPermissions = "auto"
	// DefaultRunTimeout is how long a run's turn may take.
	DefaultRunTimeout = 30 * time.Minute
	// UsageError is the exit code of a swcode run whose arguments were not understood.
	UsageError = 2
	runWord    = "run"
	// The flags of swcode run.
	cwdFlag     = "--cwd"
	modeFlag    = "--mode"
	jsonFlag    = "--json"
	timeoutFlag = "--timeout"
	flagMark    = "--"
	runUsage    = `swcode run takes one prompt: swcode run [--cwd <folder>] [--mode auto|acceptEdits|ask] [--json] [--timeout <duration>] "<prompt>".`
)

// runModes are the ways a run's session asks before it acts.
var runModes = map[string]bool{"auto": true, "acceptEdits": true, "ask": true}

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
	case runWord:
		return runCommand(rest)
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

// misused is a swcode run whose arguments were not understood: help, why, and its own exit code.
func misused(problem string) Command {
	return Command{Exit: UsageError, Kind: KindHelp, Problem: problem}
}

// runCommand reads swcode run's arguments: its flags, each with its value after it, and the prompt, one argument.
func runCommand(arguments []string) Command {
	options := RunOptions{Permissions: DefaultRunPermissions, Timeout: DefaultRunTimeout}
	prompts := []string{}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == jsonFlag:
			options.JSON = true
		case argument == cwdFlag || argument == modeFlag || argument == timeoutFlag:
			if index+1 >= len(arguments) {
				return misused(fmt.Sprintf("swcode run %s takes a value after it.", argument))
			}
			index++
			if problem := options.set(argument, arguments[index]); problem != "" {
				return misused(problem)
			}
		case strings.HasPrefix(argument, flagMark):
			return misused(fmt.Sprintf("swcode run does not know %s.", argument))
		default:
			prompts = append(prompts, argument)
		}
	}
	if len(prompts) != 1 || strings.TrimSpace(prompts[0]) == "" {
		return misused(runUsage)
	}
	options.Prompt = prompts[0]
	return Command{Kind: KindRun, Run: options}
}

// set takes a flag's value: why it was not understood, when it was not.
func (o *RunOptions) set(flag, value string) string {
	switch flag {
	case cwdFlag:
		o.Cwd = value
	case modeFlag:
		if !runModes[value] {
			return "swcode run --mode takes auto, acceptEdits or ask."
		}
		o.Permissions = value
	case timeoutFlag:
		limit, err := time.ParseDuration(value)
		if err != nil || limit <= 0 {
			return "swcode run --timeout takes a duration, such as 10m or 90s."
		}
		o.Timeout = limit
	}
	return ""
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
		`  swcode run "<prompt>"       Run one turn of a new session in this folder and print its answer, starting`,
		"                              the daemon if needed. --cwd <folder>, --mode auto|acceptEdits|ask (auto),",
		"                              --json (the session's events, a line each), --timeout <duration> (30m).",
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
