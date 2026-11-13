// Package notes is the project's notes: its AGENTS.md files, which tell the agent what is already known of the
// project before it reads any code, so that each session does not find it all out again.
package notes

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"swcode/daemon/catalog"
	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
)

const (
	// FileName is the notes file's name, the one other coding agents read too.
	FileName = "AGENTS.md"
	// maxChars is the most of one file the model reads: the whole file goes in every request of the session.
	maxChars = 20_000
	// cutNote ends a file cut at maxChars: one with no notes to leave out, a single long text.
	cutNote = "\n\n[The file goes on: read it whole if you need the rest.]"
	// leftOutNote stands in a section for its oldest notes, left out of a file longer than maxChars.
	leftOutNote = "[%d older notes of this section are left out here: read the file for them.]"
	// The Markdown the notes are written in: a title, a heading per section, a bullet per note, its further lines
	// indented under it.
	titleMark   = "# "
	headingMark = "#"
	sectionMark = "## "
	bulletMark  = "- "
	moreIndent  = "  "
)

// Find is the project's notes on its host: the AGENTS.md of the repository's root first, then of each folder down to
// the project's, the general before the particular. A folder without one, or whose file cannot be read or is
// empty, adds nothing: the notes are a help, never a reason for a turn to fail.
func Find(ctx context.Context, host hosts.Host, workspace string) []protocol.ProjectNote {
	folders := catalog.ProjectFolders(ctx, host, workspace)
	found := []protocol.ProjectNote{}
	for index := len(folders) - 1; index >= 0; index-- {
		path := host.Paths().Join(folders[index], FileName)
		data, err := host.ReadFile(ctx, path)
		if err != nil || strings.TrimSpace(string(data)) == "" {
			continue
		}
		text, leftOut := Fit(string(data))
		found = append(found, protocol.ProjectNote{LeftOut: leftOut, Path: path, Text: text})
	}
	return found
}

// Target is the AGENTS.md the agent's notes go to: the nearest one from the project's folder up to the repository's
// root, or a new one at the root when there is none.
func Target(ctx context.Context, host hosts.Host, workspace string) string {
	folders := catalog.ProjectFolders(ctx, host, workspace)
	for _, folder := range folders {
		path := host.Paths().Join(folder, FileName)
		if info, err := host.FileInfo(ctx, path); err == nil && info.Kind == hosts.KindFile {
			return path
		}
	}
	return host.Paths().Join(folders[len(folders)-1], FileName)
}

// Add is the notes with one more, a bullet at the end of its section. A section the notes do not have yet is made at
// their end; notes that do not exist yet start with a title.
func Add(text, title, section, note string) string {
	lines := strings.Split(strings.TrimRight(startedWith(text, title), "\n"), "\n")
	bullet := bulletOf(note)
	heading := sectionAt(lines, section)
	if heading < 0 {
		return joined(slices.Concat(lines, []string{"", sectionMark + strings.TrimSpace(section), "", bullet}))
	}
	at := sectionEnd(lines, heading)
	if at == heading+1 {
		return joined(slices.Concat(lines[:at], []string{"", bullet}, lines[at:]))
	}
	return joined(slices.Concat(lines[:at], []string{bullet}, lines[at:]))
}

// startedWith is the notes as they are, or a title for notes that are still empty.
func startedWith(text, title string) string {
	if strings.TrimSpace(text) != "" {
		return text
	}
	return titleMark + title
}

// bulletOf is a note as a bullet: its first line after the mark, the others indented under it.
func bulletOf(note string) string {
	lines := strings.Split(strings.TrimSpace(note), "\n")
	for index, line := range lines {
		lines[index] = strings.TrimSpace(line)
	}
	return bulletMark + strings.TrimPrefix(strings.Join(lines, "\n"+moreIndent), bulletMark)
}

// sectionAt is the line of the section's heading, its name matched whatever its case; -1 when the notes have none.
func sectionAt(lines []string, section string) int {
	return slices.IndexFunc(lines, func(line string) bool {
		name, isSection := strings.CutPrefix(strings.TrimSpace(line), sectionMark)
		return isSection && strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(section))
	})
}

// sectionEnd is where a bullet goes in the section whose heading is at that line: after its last line that is not
// blank, before the next heading.
func sectionEnd(lines []string, heading int) int {
	end := heading + 1
	for index := heading + 1; index < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[index]), headingMark); index++ {
		if strings.TrimSpace(lines[index]) != "" {
			end = index + 1
		}
	}
	return end
}

func joined(lines []string) string { return strings.Join(lines, "\n") + "\n" }

// section is a part of the notes under one heading: the heading with what stands before its first note, then its
// notes, the oldest first, each with the lines under its bullet.
type section struct {
	lead    []string
	notes   [][]string
	leftOut int
}

// sectionsOf reads the notes into their sections; what comes before the first heading is a section without one.
func sectionsOf(text string) []*section {
	current := &section{}
	sections := []*section{current}
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, headingMark):
			current = &section{lead: []string{line}}
			sections = append(sections, current)
		case strings.HasPrefix(line, bulletMark):
			current.notes = append(current.notes, []string{line})
		case len(current.notes) > 0:
			last := len(current.notes) - 1
			current.notes[last] = append(current.notes[last], line)
		default:
			current.lead = append(current.lead, line)
		}
	}
	return sections
}

func (s *section) lines() []string {
	lines := slices.Clone(s.lead)
	if s.leftOut > 0 {
		lines = append(lines, fmt.Sprintf(leftOutNote, s.leftOut))
	}
	return append(lines, slices.Concat(s.notes...)...)
}

// notesChars is how long a section's notes are, which is what leaving one out can spare.
func (s *section) notesChars() int {
	total := 0
	for _, note := range s.notes {
		total += utf8.RuneCountInString(strings.Join(note, "\n"))
	}
	return total
}

func textOf(sections []*section) string {
	lines := []string{}
	for _, each := range sections {
		lines = append(lines, each.lines()...)
	}
	return strings.Join(lines, "\n")
}

// leaveOutOldest leaves out the oldest note of the section whose notes are the longest, keeping each section's
// newest one: false when no section has a note to spare.
func leaveOutOldest(sections []*section) bool {
	var longest *section
	for _, each := range sections {
		if len(each.notes) > 1 && (longest == nil || each.notesChars() > longest.notesChars()) {
			longest = each
		}
	}
	if longest == nil {
		return false
	}
	longest.notes = longest.notes[1:]
	longest.leftOut++
	return true
}

// Fit is a notes file as a session reads it, and how many of its characters were left out: whole up to maxChars.
// A longer one loses the oldest notes of its longest sections, a line in their place saying how many: notes are
// added at the end of their section, so the ones a session just wrote are the ones the next reads. A file with no
// notes to leave out is cut at maxChars.
func Fit(text string) (string, int) {
	whole := utf8.RuneCountInString(text)
	if whole <= maxChars {
		return text, 0
	}
	sections := sectionsOf(text)
	fitted := text
	for utf8.RuneCountInString(fitted) > maxChars {
		if !leaveOutOldest(sections) {
			return string([]rune(text)[:maxChars]) + cutNote, whole - maxChars
		}
		fitted = textOf(sections)
	}
	return fitted, whole - utf8.RuneCountInString(fitted)
}
