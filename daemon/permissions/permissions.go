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
)

const statementSeparator = ";"

// CommandPrograms is the programs a shell command runs, one per part (`npm test && git status` gives npm and git),
// or none known when the command does more than that list can tell. Quotes are not understood: a separator inside
// quotes only makes a part that no rule allows, so the command is asked about.
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
		program := ""
		for _, word := range strings.Fields(part) {
			if !environmentAssignment.MatchString(word) {
				program = word
				break
			}
		}
		if program == "" {
			return nil, false
		}
		programs = append(programs, program)
	}
	return programs, len(programs) > 0
}
