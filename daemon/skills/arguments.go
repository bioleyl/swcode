package skills

import (
	"regexp"
	"strconv"
	"strings"
)

// Variables are Claude Code's variables a skill's instructions may use, written ${CLAUDE_SKILL_DIR}.
const (
	VariablePluginData = "CLAUDE_PLUGIN_DATA"
	VariablePluginRoot = "CLAUDE_PLUGIN_ROOT"
	VariableProjectDir = "CLAUDE_PROJECT_DIR"
	VariableSessionID  = "CLAUDE_SESSION_ID"
	VariableSkillDir   = "CLAUDE_SKILL_DIR"
)

const (
	argumentsLine    = "ARGUMENTS:"
	commandBlockNote = "[Run these with bash for their output: swcode runs nothing before you read a skill.]\n```bash"
)

var (
	// argument is one argument as a shell reads it: in double quotes (backslash escapes), in single quotes, or a word.
	argument = regexp.MustCompile(`"((?:\\.|[^"\\])*)"|'([^']*)'|(\S+)`)
	escape   = regexp.MustCompile(`\\(.)`)
	// placeholder is a $… with the backslashes before it: $ARGUMENTS[1], $ARGUMENTS, $1, $name.
	placeholder = regexp.MustCompile(`(\\*)\$(?:ARGUMENTS\[(\d+)\]|(ARGUMENTS)\b|(\d+)|([A-Za-z_]\w*))`)
	variable    = regexp.MustCompile(`\$\{(CLAUDE_[A-Z_]+)\}`)
	// Claude Code runs these before the model reads the skill: !`git status`, or a ```! block.
	inlineCommand = regexp.MustCompile("(?m)(^|\\s)!`([^`\\n]+)`")
	commandBlock  = regexp.MustCompile("(?m)^```!\\s*$")
)

// Values are the ${CLAUDE_…} variables a skill's instructions may use, by name; one with no value stays as it is.
type Values map[string]string

// SplitArguments splits the arguments a skill was given as a shell splits a command line.
func SplitArguments(text string) []string {
	words := []string{}
	for _, found := range argument.FindAllStringSubmatch(text, -1) {
		switch {
		case strings.HasPrefix(found[0], `"`):
			words = append(words, escape.ReplaceAllString(found[1], "$1"))
		case strings.HasPrefix(found[0], "'"):
			words = append(words, found[2])
		default:
			words = append(words, found[3])
		}
	}
	return words
}

// withCommandsLeft leaves Claude Code's commands that run before the model reads a skill for the agent to run.
func withCommandsLeft(text string) string {
	text = commandBlock.ReplaceAllString(text, commandBlockNote)
	return inlineCommand.ReplaceAllString(text, "$1[the output of `$2`: run it with bash]")
}

// found is a $… the instructions hold, when it is one of the placeholders: $HOME in a shell example is not.
type found struct {
	match   string
	slashes string
	// at is the argument's position, or -1 for all of them.
	at int
	// named by the skill's arguments: one with no argument is left empty, not as it is.
	named bool
}

func placeholderOf(groups []string, names []string) (found, bool) {
	match, slashes, index, all, position, name := groups[0], groups[1], groups[2], groups[3], groups[4], groups[5]
	if name == "" {
		at := -1
		if all == "" {
			at, _ = strconv.Atoi(index + position)
		}
		return found{at: at, match: match, slashes: slashes}, true
	}
	for at, each := range names {
		if each == name {
			return found{at: at, match: match, named: true, slashes: slashes}, true
		}
	}
	return found{}, false
}

func (f found) value(words []string, given string) string {
	// \$1 writes $1; \\$1 keeps its backslashes, and $1 still counts.
	if len(f.slashes) == 1 {
		return f.match[1:]
	}
	if f.at < 0 {
		return f.slashes + given
	}
	if f.at < len(words) {
		return f.slashes + words[f.at]
	}
	if f.named {
		return ""
	}
	return f.match
}

// WithArguments is a skill's instructions with what it was given, as Claude Code substitutes them: $ARGUMENTS,
// the arguments by position ($ARGUMENTS[0], $0) or by the names of its arguments, then the ${CLAUDE_…} variables.
// A position with no argument stays as it is ($1 in a shell example); without any placeholder, the arguments come
// at the end.
func WithArguments(instructions, given string, names []string, values Values) string {
	words := SplitArguments(given)
	placed := false
	for _, groups := range placeholder.FindAllStringSubmatch(instructions, -1) {
		if _, ok := placeholderOf(groups, names); ok {
			placed = true
		}
	}
	substituted := placeholder.ReplaceAllStringFunc(instructions, func(match string) string {
		each, ok := placeholderOf(placeholder.FindStringSubmatch(match), names)
		if !ok {
			return match
		}
		return each.value(words, given)
	})
	if !placed && strings.TrimSpace(given) != "" {
		substituted += "\n\n" + argumentsLine + " " + given
	}
	return variable.ReplaceAllStringFunc(withCommandsLeft(substituted), func(match string) string {
		if value := values[variable.FindStringSubmatch(match)[1]]; value != "" {
			return value
		}
		return match
	})
}
