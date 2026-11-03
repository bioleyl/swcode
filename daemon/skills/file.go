// Package skills is the user's instructions for kinds of tasks: SKILL.md files as Agent Skills (and Claude Code)
// write them, swcode's own kept in a folder of the daemon.
package skills

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"swcode/daemon/protocol"
)

// SkillFile is the file of a skill's folder.
const SkillFile = "SKILL.md"

const (
	nameKey        = "name"
	descriptionKey = "description"
	// Claude Code's keys swcode acts on; it keeps the others (allowed-tools, model, hooks…) and ignores them.
	forPeopleKey    = "disable-model-invocation"
	whenToUseKey    = "when_to_use"
	argumentsKey    = "arguments"
	argumentHintKey = "argument-hint"
	literalMark     = "|"
	doubleQuote     = `"`
	singleQuote     = "'"
	escapedQuote    = "''"
	listSeparator   = ","
	// Claude Code cuts a skill's line in the listing there, its description and when_to_use together.
	maxListedDescription = 1536
)

var (
	frontmatter       = regexp.MustCompile(`^---\r?\n([\s\S]*?)\r?\n---[ \t]*(?:\r?\n|$)`)
	lineBreak         = regexp.MustCompile(`\r?\n`)
	keyLine           = regexp.MustCompile(`^([A-Za-z0-9_-]+):(.*)$`)
	blockScalar       = regexp.MustCompile(`^[|>][-+0-9]*$`)
	indent            = regexp.MustCompile(`^[ \t]*`)
	leadingBlankLines = regexp.MustCompile(`^(?:[ \t]*\r?\n)+`)
	// A line break between two lines of text folds to a space; a blank line stays a line break.
	foldedBreak = regexp.MustCompile(`([^\n])\n([^\n])`)
	blankLine   = regexp.MustCompile(`\n\n`)
	listItem    = regexp.MustCompile(`^\s*-\s+(.*)$`)
	flowList    = regexp.MustCompile(`^\[(.*)\]$`)
	quotes      = regexp.MustCompile(`^(["'])(.*)["']$`)
	headingMark = regexp.MustCompile(`^#+\s*`)
	// trueWords are YAML's words for true, in any case, as Claude Code reads them.
	trueWords = map[string]bool{"true": true, "yes": true, "on": true, "1": true}
	ownKeys   = map[string]bool{nameKey: true, descriptionKey: true}
)

// File is a SKILL.md as the daemon reads it: the frontmatter's name and description, then the instructions.
type File struct {
	// Name is the frontmatter's name, empty when it has none; the skill's name is its folder's.
	Name         string
	Description  string
	Instructions string
	// Extra is the frontmatter's other lines (license, metadata…), kept as they are when the skill is saved again.
	Extra string
	// ForPeople is disable-model-invocation: for Claude Code's person to type, not for the agent to load.
	ForPeople bool
	// WhenToUse is when_to_use: more of what the skill is for, after its description in the list.
	WhenToUse string
	// ArgumentNames are the arguments' names in order, which the instructions use as $name.
	ArgumentNames []string
	// ArgumentHint is what the skill is given, such as [issue-number].
	ArgumentHint string
}

// entry is a key of the frontmatter, with the lines its value takes: the key's own, and the indented ones under it.
type entry struct {
	key   string
	first string
	rest  []string
	raw   []string
}

// Label is the file a skill's name has, as the conversation names it.
func Label(name string) string {
	return "skills/" + name + "/" + SkillFile
}

func entriesOf(block string) []*entry {
	entries := []*entry{}
	for _, line := range lineBreak.Split(block, -1) {
		if found := keyLine.FindStringSubmatch(line); found != nil {
			entries = append(entries, &entry{first: strings.TrimSpace(found[2]), key: found[1], raw: []string{line}})
			continue
		}
		if len(entries) > 0 {
			last := entries[len(entries)-1]
			last.rest = append(last.rest, line)
			last.raw = append(last.raw, line)
		}
	}
	return entries
}

func folded(lines []string) string {
	trimmed := make([]string, len(lines))
	for index, line := range lines {
		trimmed[index] = strings.TrimSpace(line)
	}
	text := strings.Join(trimmed, "\n")
	// Twice: a match takes the character after the break, so breaks one line apart need a second pass.
	text = foldedBreak.ReplaceAllString(foldedBreak.ReplaceAllString(text, "$1 $2"), "$1 $2")
	return strings.TrimSpace(blankLine.ReplaceAllString(text, "\n"))
}

func literal(lines []string) string {
	least := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		width := len(indent.FindString(line))
		if least < 0 || width < least {
			least = width
		}
	}
	dedented := make([]string, len(lines))
	for index, line := range lines {
		dedented[index] = line[min(max(least, 0), len(line)):]
	}
	return strings.TrimSpace(strings.Join(dedented, "\n"))
}

func doubleQuoted(text string) string {
	var value string
	if json.Unmarshal([]byte(text), &value) == nil {
		return value
	}
	return strings.TrimSuffix(strings.TrimPrefix(text, doubleQuote), doubleQuote)
}

func singleQuoted(text string) string {
	inner := strings.TrimSuffix(strings.TrimPrefix(text, singleQuote), singleQuote)
	return strings.ReplaceAll(inner, escapedQuote, singleQuote)
}

// value reads a value as YAML reads the few forms frontmatter uses: plain, quoted, or a block of lines (|, >).
func (e *entry) value() string {
	if blockScalar.MatchString(e.first) {
		if strings.HasPrefix(e.first, literalMark) {
			return literal(e.rest)
		}
		return folded(e.rest)
	}
	parts := []string{e.first}
	for _, line := range e.rest {
		parts = append(parts, strings.TrimSpace(line))
	}
	text := strings.TrimSpace(strings.Join(parts, " "))
	if strings.HasPrefix(text, doubleQuote) {
		return doubleQuoted(text)
	}
	if strings.HasPrefix(text, singleQuote) {
		return singleQuoted(text)
	}
	return folded(append([]string{e.first}, e.rest...))
}

func unquoted(text string) string {
	if found := quotes.FindStringSubmatch(text); found != nil {
		return found[2]
	}
	return text
}

// list reads a list as frontmatter writes one: `- item` lines under the key, [a, b], or words on its line.
func (e *entry) list() []string {
	items := []string{}
	for _, line := range e.rest {
		if found := listItem.FindStringSubmatch(line); found != nil {
			items = append(items, strings.TrimSpace(found[1]))
		}
	}
	if len(items) == 0 {
		text := e.value()
		if flow := flowList.FindStringSubmatch(text); flow != nil {
			items = strings.Split(flow[1], listSeparator)
		} else {
			items = strings.Fields(text)
		}
	}
	kept := []string{}
	for _, item := range items {
		if value := unquoted(strings.TrimSpace(item)); value != "" {
			kept = append(kept, value)
		}
	}
	return kept
}

// Parse reads a SKILL.md: the frontmatter between its --- lines, then the instructions; a file without one is all
// instructions.
func Parse(text string) File {
	match := frontmatter.FindStringSubmatchIndex(text)
	if match == nil {
		return File{ArgumentNames: []string{}, Instructions: strings.TrimSpace(text)}
	}
	entries := entriesOf(text[match[2]:match[3]])
	find := func(key string) *entry {
		for _, each := range entries {
			if each.key == key {
				return each
			}
		}
		return nil
	}
	value := func(key string) string {
		if found := find(key); found != nil {
			return found.value()
		}
		return ""
	}
	extra := []string{}
	for _, each := range entries {
		if !ownKeys[each.key] {
			extra = append(extra, each.raw...)
		}
	}
	names := []string{}
	if found := find(argumentsKey); found != nil {
		names = found.list()
	}
	return File{
		ArgumentHint:  value(argumentHintKey),
		ArgumentNames: names,
		Description:   value(descriptionKey),
		Extra:         strings.Join(extra, "\n"),
		ForPeople:     trueWords[strings.ToLower(value(forPeopleKey))],
		Instructions:  strings.TrimRight(leadingBlankLines.ReplaceAllString(text[match[1]:], ""), " \t\r\n"),
		Name:          value(nameKey),
		WhenToUse:     value(whenToUseKey),
	}
}

func firstLine(text string) string {
	for _, line := range lineBreak.Split(text, -1) {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(headingMark.ReplaceAllString(line, ""))
		}
	}
	return ""
}

// ListedDescription is a Claude Code skill's line in the list the agent reads: its description, or its first line
// without one, then when_to_use, then the arguments it takes.
func ListedDescription(file File) string {
	description := file.Description
	if description == "" {
		description = firstLine(file.Instructions)
	}
	hint := ""
	if file.ArgumentHint != "" {
		hint = "Arguments: " + file.ArgumentHint + "."
	}
	parts := []string{}
	for _, part := range []string{description, file.WhenToUse, hint} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	listed := strings.Join(parts, " ")
	if utf8.RuneCountInString(listed) > maxListedDescription {
		return string([]rune(listed)[:maxListedDescription])
	}
	return listed
}

// plainValue: a plain YAML value needs no quotes, no indicator first, no ": " or " #" inside, nothing blank at its
// end.
var (
	plainStart  = regexp.MustCompile(`^[-?:,\[\]{}#&*!|>'"%@` + "`" + `\s]`)
	plainInside = regexp.MustCompile(`: | #|\s$`)
)

func yamlValue(text string) string {
	if text != "" && !plainStart.MatchString(text) && !plainInside.MatchString(text) {
		return text
	}
	// JSON's escapes are YAML's in double quotes.
	var quoted strings.Builder
	encoder := json.NewEncoder(&quoted)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(text)
	return strings.TrimSuffix(quoted.String(), "\n")
}

// FileText is the SKILL.md of a skill, with the other lines its frontmatter had.
func FileText(skill protocol.Skill, extra string) string {
	lines := []string{"---", nameKey + ": " + skill.Name, descriptionKey + ": " + yamlValue(skill.Description)}
	if extra != "" {
		lines = append(lines, extra)
	}
	lines = append(lines, "---", "", strings.TrimSpace(skill.Instructions), "")
	return strings.Join(lines, "\n")
}
