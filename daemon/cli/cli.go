// Package cli reads the command line: open the app (starting the daemon when it does not run), run the daemon, stop
// it, install the program, run it on another machine.
package cli

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// DefaultLocalPort is where a remote daemon shows up on this machine.
const DefaultLocalPort = 4510

const maxPort = 65_535

// Kind is what the command line asks for.
type Kind string

const (
	KindOpen    Kind = "open"
	KindServe   Kind = "serve"
	KindStop    Kind = "stop"
	KindInstall Kind = "install"
	KindRemote  Kind = "remote"
	KindPair    Kind = "pair"
	KindVersion Kind = "version"
	KindHelp    Kind = "help"
)

// Mode is where the daemon runs: in a terminal, in the background (its log in a file), or inside the desktop app.
type Mode string

const (
	ModeTerminal   Mode = "terminal"
	ModeBackground Mode = "background"
	ModeDesktop    Mode = "desktop"
)

// Command is what the arguments ask for.
type Command struct {
	Kind Kind
	Mode Mode
	// Remote: the SSH destination, the folder to work in there, where it shows up here, and how the other machine
	// reaches llm-server itself (through this one when empty).
	Host      string
	Workspace string
	LocalPort int
	LlamaURL  string
	// Problem is why the arguments were not understood, with help.
	Problem string
}

// plainCommands are the commands that take no arguments.
var plainCommands = map[string]Command{
	"daemon":  {Kind: KindServe, Mode: ModeBackground},
	"install": {Kind: KindInstall},
	"open":    {Kind: KindOpen},
	"pair":    {Kind: KindPair},
	"serve":   {Kind: KindServe, Mode: ModeTerminal},
	"stop":    {Kind: KindStop},
}

var (
	versionFlags = map[string]bool{"--version": true, "-v": true, "version": true}
	helpFlags    = map[string]bool{"--help": true, "-h": true, "help": true}
)

const remoteWord = "remote"

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
	if versionFlags[first] {
		return Command{Kind: KindVersion}
	}
	if helpFlags[first] {
		return Command{Kind: KindHelp}
	}
	if first != remoteWord {
		return help(fmt.Sprintf("swcode does not know %s.", first))
	}
	return remoteCommand(rest)
}

// interleaved puts the flags before the destination, as Node's parseArgs takes them anywhere.
func interleaved(arguments []string) ([]string, []string) {
	flags, positionals := []string{}, []string{}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if !strings.HasPrefix(argument, "-") {
			positionals = append(positionals, argument)
			continue
		}
		flags = append(flags, argument)
		if !strings.Contains(argument, "=") && index+1 < len(arguments) {
			index++
			flags = append(flags, arguments[index])
		}
	}
	return flags, positionals
}

func remoteCommand(arguments []string) Command {
	set := flag.NewFlagSet(remoteWord, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	llamaURL := set.String("llama-url", "", "")
	port := set.String("port", "", "")
	set.StringVar(port, "p", "", "")
	workspace := set.String("workspace", "", "")
	set.StringVar(workspace, "w", "", "")
	flags, positionals := interleaved(arguments)
	if err := set.Parse(flags); err != nil {
		return help(err.Error())
	}
	if len(positionals) != 1 {
		return help("swcode remote takes one SSH destination, such as me@build-box.")
	}
	localPort := DefaultLocalPort
	if *port != "" {
		parsed, err := strconv.Atoi(*port)
		if err != nil || parsed <= 0 || parsed > maxPort {
			return help(*port + " is not a port.")
		}
		localPort = parsed
	}
	return Command{Host: positionals[0], Kind: KindRemote, LlamaURL: *llamaURL, LocalPort: localPort, Workspace: *workspace}
}

// Usage is what swcode --help prints.
var Usage = strings.Join([]string{
	"swcode: a coding agent for llm-server.",
	"",
	"Usage:",
	"  swcode                      Open swcode in the browser, starting its daemon in the background if needed.",
	"  swcode stop                 Stop the daemon.",
	"  swcode pair                 A code to pair another computer's swcode, or a phone, with this one.",
	"  swcode serve                Run the daemon here, in this terminal (Ctrl+C stops it).",
	"  swcode install              Install this program: the swcode command, its settings and, on WSL, a",
	"                              shortcut in Windows. The LLAMA_* and SWCODE_* settings set now are kept.",
	"  swcode remote <host>        Run it on another machine over SSH, reached from here.",
	"      -w, --workspace <dir>   The folder to work in there (the home folder by default).",
	"      -p, --port <port>       Where it shows up here (4510 by default).",
	"      --llama-url <url>       How that machine reaches llm-server itself; through this one by default.",
	"  swcode --version            The build.",
	"",
	"Settings come from ~/.swcode/settings.env, the environment on top: see the README.",
}, "\n")
