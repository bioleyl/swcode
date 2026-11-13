// Package permissions decides which calls a person approves first: reading never, edits and commands as the
// session's mode and what people allowed for it say.
package permissions

import (
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
)

// Subject is what a call touches: a file it edits, a command it runs.
type Subject struct {
	Access  Access
	Path    string
	Command string
}

// Read is a call that only reads.
var Read = Subject{Access: AccessRead}

// Grants are what a session lets through without asking: its mode, plus what people allowed "for this session".
type Grants struct {
	Mode     protocol.PermissionMode
	Edits    bool
	Programs map[string]bool
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

func commandAllowed(command string, grants Grants) bool {
	if modeAllows[grants.Mode].commands {
		return true
	}
	programs, known := CommandPrograms(command)
	if !known {
		return false
	}
	for _, program := range programs {
		if !grants.Programs[program] {
			return false
		}
	}
	return true
}

// NeedsPermission tells whether a person has to approve this call first.
func NeedsPermission(subject Subject, grants Grants) bool {
	switch subject.Access {
	case AccessRead:
		return false
	case AccessEdit:
		return !modeAllows[grants.Mode].edits && !grants.Edits
	}
	return !commandAllowed(subject.Command, grants)
}

// GrantForSession is the grants once a person allowed this kind of call for the rest of the session.
func GrantForSession(subject Subject, grants Grants) Grants {
	next := Grants{Edits: grants.Edits, Mode: grants.Mode, Programs: map[string]bool{}}
	for program := range grants.Programs {
		next.Programs[program] = true
	}
	switch subject.Access {
	case AccessEdit:
		next.Edits = true
	case AccessCommand:
		programs, _ := CommandPrograms(subject.Command)
		for _, program := range programs {
			next.Programs[program] = true
		}
	}
	return next
}

// opaqueMarks are the marks of a command that does more than run a list of programs: a substitution, a
// redirection, a subshell, a background job or a second line. A rule about programs cannot vouch for those, so
// they are always asked about.
var opaqueMarks = []string{"`", "$(", ">", "<", "(", ")", "\n", "&"}

var (
	chains                = regexp.MustCompile(`&&|\|\|`)
	separators            = regexp.MustCompile(`[;|]`)
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

// programOf is the program a part of a command runs, when its name says what runs: not after a variable set for
// it (PATH, LD_PRELOAD and git's own change what a name means), and not a program that runs what its words say.
func programOf(part string) (string, bool) {
	words := strings.Fields(part)
	if len(words) == 0 || environmentAssignment.MatchString(words[0]) || runsItsWords(words[0], words[1:]) {
		return "", false
	}
	return words[0], true
}

const statementSeparator = ";"

// CommandPrograms is the programs a shell command runs, one per part (`npm test && git status` gives npm and git),
// or none known when the command does more than that list can tell: it sets a variable first, or one of its
// programs runs what its words say. Quotes are not understood: a separator inside quotes only makes a part that
// no rule allows, so the command is asked about. shared/permissions words the app's button with the same rule,
// in TypeScript: the tests of both hold the same cases.
func CommandPrograms(command string) ([]string, bool) {
	statements := chains.ReplaceAllString(command, statementSeparator)
	for _, mark := range opaqueMarks {
		if strings.Contains(statements, mark) {
			return nil, false
		}
	}
	programs := []string{}
	for _, part := range separators.Split(statements, -1) {
		if strings.TrimSpace(part) == "" {
			continue
		}
		program, named := programOf(part)
		if !named {
			return nil, false
		}
		programs = append(programs, program)
	}
	return programs, len(programs) > 0
}
