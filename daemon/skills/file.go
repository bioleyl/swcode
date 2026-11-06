// Package skills is the user's instructions for kinds of tasks: SKILL.md files as Agent Skills write them, a folder
// each in the common .agents/skills folders.
package skills

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

// SkillFile is the file of a skill's folder.
const SkillFile = "SKILL.md"

const (
	nameKey        = "name"
	descriptionKey = "description"
	literalMark    = "|"
	doubleQuote    = `"`
	singleQuote    = "'"
	escapedQuote   = "''"
	// maxDescription is Agent Skills' longest description: what the agent reads of a skill in every conversation.
	maxDescription = 1024
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
)

// File is a SKILL.md as the daemon reads it: the frontmatter's name and description, then the instructions. Its
// other keys (license, metadata…) are left alone.
type File struct {
	// Name is the frontmatter's name, empty when it has none; the skill's name is then its folder's.
	Name         string
	Description  string
	Instructions string
}

// entry is a key of the frontmatter, with the lines its value takes: the key's own, and the indented ones under it.
type entry struct {
	key   string
	first string
	rest  []string
}

func entriesOf(block string) []*entry {
	entries := []*entry{}
	for _, line := range lineBreak.Split(block, -1) {
		if found := keyLine.FindStringSubmatch(line); found != nil {
			entries = append(entries, &entry{first: strings.TrimSpace(found[2]), key: found[1]})
			continue
		}
		if len(entries) > 0 {
			last := entries[len(entries)-1]
			last.rest = append(last.rest, line)
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

// clippedDescription is a description as long as Agent Skills allow one.
func clippedDescription(text string) string {
	if utf8.RuneCountInString(text) <= maxDescription {
		return text
	}
	return string([]rune(text)[:maxDescription])
}

// Parse reads a SKILL.md: the frontmatter between its --- lines, then the instructions; a file without one is all
// instructions.
func Parse(text string) File {
	match := frontmatter.FindStringSubmatchIndex(text)
	if match == nil {
		return File{Instructions: strings.TrimSpace(text)}
	}
	entries := entriesOf(text[match[2]:match[3]])
	value := func(key string) string {
		for _, each := range entries {
			if each.key == key {
				return each.value()
			}
		}
		return ""
	}
	return File{
		Description:  clippedDescription(value(descriptionKey)),
		Instructions: strings.TrimRight(leadingBlankLines.ReplaceAllString(text[match[1]:], ""), " \t\r\n"),
		Name:         value(nameKey),
	}
}
