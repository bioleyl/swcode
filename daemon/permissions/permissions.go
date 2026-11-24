// Package permissions decides which calls a person approves first: reading never, edits and commands as the
// session's mode and what people allowed for it say.
package permissions

import (
	"maps"
	"regexp"
	"strings"

	"swcode/daemon/protocol"
)

// Access is what a tool call touches: reading is always fine, changing a file or running a command may need a
// person.
type Access string

const (
	AccessRead    Access = "read"
	AccessEdit    Access = "edit"
	AccessCommand Access = "command"
	// AccessServer is a tool of an MCP server: what it does is the server's own, so it is asked about as a command
	// is, and allowed for a session by its server.
	AccessServer Access = "server"
	// AccessBrowser is a call that drives the daemon's browser: clicking, typing, or opening a private page.
	AccessBrowser Access = "browser"
)

// Subject is what a call touches: a file it edits, a command it runs, an MCP server it calls.
type Subject struct {
	Access  Access
	Path    string
	Command string
	Server  string
	// Outside: the command runs outside the sandbox the session's commands run in, with all the person can do.
	Outside bool
	// Browser is what a browser call does.
	Browser BrowserAction
}

// BrowserAction is what a browser call touches: the page it acts on, and whether it needs a person whatever the
// session's mode.
type BrowserAction struct {
	Action string
	URL    string
	// Approval: the call asks whatever the session's mode, as a password field, a form that leaves the page's
	// origin, or a private network address does.
	Approval bool
}

// Read is a call that only reads.
var Read = Subject{Access: AccessRead}

// Grants are what a session lets through without asking: its mode, plus what people allowed "for this session".
type Grants struct {
	Mode     protocol.PermissionMode
	Edits    bool
	Programs map[string]bool
	// Servers are the MCP servers whose tools were allowed.
	Servers map[string]bool
	// Outside are the programs allowed outside the sandbox: a mode that runs everything runs it in the sandbox,
	// and asks about what leaves it.
	Outside map[string]bool
}

// allowance is what a mode lets through by itself, before anything allowed for the session.
type allowance struct {
	edits    bool
	commands bool
}

var modeAllows = map[protocol.PermissionMode]allowance{
	protocol.PermissionModeAcceptEdits: {edits: true},
	protocol.PermissionModeAsk:         {},
	protocol.PermissionModeAuto:        {commands: true, edits: true},
}

// programsAllowed tells whether every program a command runs was allowed.
func programsAllowed(command string, allowed map[string]bool) bool {
	programs, known := CommandPrograms(command)
	if !known {
		return false
	}
	for _, program := range programs {
		if !allowed[program] {
			return false
		}
	}
	return true
}

func commandAllowed(subject Subject, grants Grants) bool {
	if subject.Outside {
		return programsAllowed(subject.Command, grants.Outside)
	}
	return modeAllows[grants.Mode].commands || programsAllowed(subject.Command, grants.Programs)
}

// NeedsPermission tells whether a person has to approve this call first.
func NeedsPermission(subject Subject, grants Grants) bool {
	switch subject.Access {
	case AccessRead:
		return false
	case AccessEdit:
		return !modeAllows[grants.Mode].edits && !grants.Edits
	case AccessServer:
		return !modeAllows[grants.Mode].commands && !grants.Servers[subject.Server]
	case AccessBrowser:
		return subject.Browser.Approval || !modeAllows[grants.Mode].commands
	}
	return !commandAllowed(subject, grants)
}

// GrantForSession is the grants once a person allowed this kind of call for the rest of the session.
func GrantForSession(subject Subject, grants Grants) Grants {
	next := Grants{
		Edits: grants.Edits, Mode: grants.Mode,
		Outside: maps.Clone(grants.Outside), Programs: maps.Clone(grants.Programs), Servers: maps.Clone(grants.Servers),
	}
	for _, held := range []*map[string]bool{&next.Outside, &next.Programs, &next.Servers} {
		if *held == nil {
			*held = map[string]bool{}
		}
	}
	switch subject.Access {
	case AccessEdit:
		next.Edits = true
	case AccessServer:
		next.Servers[subject.Server] = true
	case AccessCommand:
		allowed := next.Programs
		if subject.Outside {
			allowed = next.Outside
		}
		programs, _ := CommandPrograms(subject.Command)
		for _, program := range programs {
			allowed[program] = true
		}
	}
	return next
}

// Allowed is what a project lets through for every session of it, as people answered "in this project": programs
// by name, in the sandbox and outside it, and MCP servers.
type Allowed struct {
	Programs []string
	Outside  []string
	Servers  []string
}

// with is what was allowed, with more names on top.
func with(allowed map[string]bool, more []string) map[string]bool {
	if len(more) == 0 {
		return allowed
	}
	all := maps.Clone(allowed)
	if all == nil {
		all = map[string]bool{}
	}
	for _, name := range more {
		all[name] = true
	}
	return all
}

// With is the grants with what the session's project allows on top.
func (g Grants) With(allowed Allowed) Grants {
	g.Programs, g.Outside, g.Servers = with(g.Programs, allowed.Programs), with(g.Outside, allowed.Outside), with(g.Servers, allowed.Servers)
	return g
}

// AllowedBy is what allowing a call "in this project" adds to the project: the programs of a command, in the
// sandbox or outside it, or the server of an MCP tool. Nothing for an edit (a project's sessions start in the mode
// that accepts edits when it says so), nor for a command whose programs cannot be told, nor for a browser call,
// which a project keeps no rule about.
func AllowedBy(subject Subject) Allowed {
	switch subject.Access {
	case AccessServer:
		return Allowed{Servers: []string{subject.Server}}
	case AccessCommand:
		programs, _ := CommandPrograms(subject.Command)
		if subject.Outside {
			return Allowed{Outside: programs}
		}
		return Allowed{Programs: programs}
	}
	return Allowed{}
}

var (
	environmentAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	// runners are programs that run what their words say rather than a thing of their own: allowing one by its
	// name would allow anything after it.
	runners = map[string]bool{
		"bash": true, "bunx": true, "command": true, "dash": true, "doas": true, "env": true, "eval": true, "exec": true,
		"find": true, "fish": true, "nice": true, "nohup": true, "npx": true, "parallel": true, "pnpx": true, "sh": true,
		"ssh": true, "sudo": true, "time": true, "timeout": true, "watch": true, "xargs": true, "zsh": true,
	}
	// runningWords are the words that make a program named for itself run something else: git's own settings name
	// programs it runs, and npm runs any package's.
	runningWords = map[string][]string{
		"git": {"-c", "--config-env", "--exec-path"},
		"npm": {"exec", "x"},
	}
)

// runsItsWords tells whether a program runs what its words say, so that its name vouches for nothing.
func runsItsWords(program string, words []string) bool {
	if runners[program] {
		return true
	}
	for _, marker := range runningWords[program] {
		for _, word := range words {
			if word == marker || strings.HasPrefix(word, marker+"=") {
				return true
			}
		}
	}
	return false
}

// sshOptionsWithAWord are ssh's options whose value is a word of its own (`-p 2222`): skipping the option alone
// would make its value look like the host.
var sshOptionsWithAWord = map[byte]bool{
	'b': true, 'c': true, 'D': true, 'E': true, 'e': true, 'F': true, 'I': true, 'i': true, 'J': true, 'L': true,
	'l': true, 'm': true, 'O': true, 'o': true, 'p': true, 'Q': true, 'R': true, 'S': true, 'W': true, 'w': true,
}

// sshTarget is what an ssh command reaches: its host, which is the unit a person allows, since what runs there is
// that host's own shell and no list of programs can tell it. `ssh marvin 'cd x && ls'` names `ssh marvin`.
func sshTarget(words []string) (string, bool) {
	if len(words) == 0 || words[0] != "ssh" {
		return "", false
	}
	for i := 1; i < len(words); i++ {
		word := words[i]
		switch {
		case word == "--":
		case strings.HasPrefix(word, "-") && len(word) > 1:
			if len(word) == 2 && sshOptionsWithAWord[word[1]] {
				i++
			}
		default:
			return "ssh " + word, true
		}
	}
	return "", false
}

// programOf is the program a part of a command runs, when its name says what runs: not after a variable set for
// it (PATH, LD_PRELOAD and git's own change what a name means), and not a program that runs what its words say,
// unless its words name the host it reaches.
func programOf(part string) (string, bool) {
	words := strings.Fields(part)
	if len(words) == 0 || environmentAssignment.MatchString(words[0]) {
		return "", false
	}
	if runsItsWords(words[0], words[1:]) {
		return sshTarget(words)
	}
	return words[0], true
}

// splitParts are the statements of a command, split at its separators (`;`, `|`, `&&`, `||`) and not inside its
// quotes, or none when the command does more than a list of programs can tell: a substitution the shell runs (not
// one in single quotes, which quotes it), a redirection, a subshell, a background job or a second line. A separator
// inside quotes belongs to the argument, so `ssh marvin 'cd x && ls'` is one statement.
func splitParts(command string) ([]string, bool) {
	parts := []string{}
	var current strings.Builder
	var quote byte
	flush := func() {
		if part := strings.TrimSpace(current.String()); part != "" {
			parts = append(parts, part)
		}
		current.Reset()
	}
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch quote {
		case '\'':
			if c == '\'' {
				quote = 0
			} else {
				current.WriteByte(c)
			}
		case '"':
			switch {
			case c == '"':
				quote = 0
			case c == '\\' && i+1 < len(command):
				i++
				current.WriteByte(command[i])
			case c == '`', c == '$' && i+1 < len(command) && command[i+1] == '(':
				return nil, false
			default:
				current.WriteByte(c)
			}
		default:
			switch c {
			case '\'', '"':
				quote = c
			case '\\':
				if i+1 < len(command) {
					i++
					current.WriteByte(command[i])
				}
			case ';', '|':
				flush()
			case '&':
				if i+1 >= len(command) || command[i+1] != '&' {
					return nil, false
				}
				i++
				flush()
			case '`', '>', '<', '(', ')', '\n':
				return nil, false
			case '$':
				if i+1 < len(command) && command[i+1] == '(' {
					return nil, false
				}
				current.WriteByte(c)
			default:
				current.WriteByte(c)
			}
		}
	}
	if quote != 0 {
		return nil, false
	}
	flush()
	return parts, true
}

// CommandPrograms is the programs a shell command runs, one per statement (`npm test && git status` gives npm and
// git), or none known when the command does more than that list can tell: it sets a variable first, one of its
// programs runs what its words say (except ssh, which is named by the host it reaches), or it does something a list
// of programs cannot describe. shared/permissions decides with the same rule, in TypeScript: the tests of both hold
// the same cases.
func CommandPrograms(command string) ([]string, bool) {
	parts, known := splitParts(command)
	if !known {
		return nil, false
	}
	programs := []string{}
	for _, part := range parts {
		program, named := programOf(part)
		if !named {
			return nil, false
		}
		programs = append(programs, program)
	}
	return programs, len(programs) > 0
}
