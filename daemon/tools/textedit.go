package tools

import (
	"fmt"
	"strings"
)

const (
	lf       = "\n"
	crlf     = "\r\n"
	notFound = "oldString was not found in the file. Read the file again and copy the text exactly, with its indentation."
	// nearLine follows notFound when the file has oldString's first line and not what follows it.
	nearLine = " Its first line is at line %d of the file, and the lines after it differ: read from there."
	// indentNote says an edit matched the file's lines by their words alone, and took their indentation.
	indentNote = "matched without regard to indentation, which was kept as the file has it"
	// maxNearLines is how many places the refusal names at most.
	maxNearLines = 3
)

// EditResult is the file's text once edited, and how many replacements were made; or why it cannot be. Note says
// how the text was found, when not as it was written.
type EditResult struct {
	OK       bool
	Text     string
	Replaced int
	Problem  string
	Note     string
}

func replaceExactly(text, oldString, newString string, replaceAll bool) EditResult {
	count := strings.Count(text, oldString)
	if count == 0 {
		return EditResult{Problem: notFound}
	}
	if count > 1 && !replaceAll {
		return EditResult{Problem: fmt.Sprintf("oldString appears %d times. Add the lines around it to pick one, or set replaceAll.", count)}
	}
	if replaceAll {
		return EditResult{OK: true, Replaced: count, Text: strings.ReplaceAll(text, oldString, newString)}
	}
	return EditResult{OK: true, Replaced: 1, Text: strings.Replace(text, oldString, newString, 1)}
}

// usesCrlf: models write \n, and a file with Windows line ends still takes the edit, and keeps its line ends.
func usesCrlf(text, oldString string) bool {
	return strings.Contains(text, crlf) && strings.Contains(oldString, lf) && !strings.Contains(oldString, crlf)
}

func toCrlf(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, crlf, lf), lf, crlf)
}

// indentOf is the spaces and tabs a line starts with.
func indentOf(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// sameWords tells whether lines of the file say what the lines looked for say, whatever starts and ends them.
func sameWords(file, wanted []string) bool {
	for index, line := range wanted {
		if strings.TrimSpace(file[index]) != strings.TrimSpace(line) {
			return false
		}
	}
	return true
}

// placesOf is where the lines looked for are in the file, by their words alone: the index of each place's first
// line. A place that overlaps the one before it is not another.
func placesOf(file, wanted []string) []int {
	places := []int{}
	for start := 0; start+len(wanted) <= len(file); start++ {
		if sameWords(file[start:], wanted) {
			places = append(places, start)
			start += len(wanted) - 1
		}
	}
	return places
}

// indentations is how the file indents what the lines looked for indent their own way: each indentation they
// were written with, and the one the file has on the same line.
func indentations(wanted, inFile []string) map[string]string {
	known := map[string]string{}
	for index, line := range wanted {
		if _, seen := known[indentOf(line)]; !seen && strings.TrimSpace(line) != "" {
			known[indentOf(line)] = indentOf(inFile[index])
		}
	}
	return known
}

// fileIndent is a written indentation as the file has it: the one the file has for it, else the one it has for
// the longest part of it, with the rest as written.
func fileIndent(written string, known map[string]string) string {
	for length := len(written); length >= 0; length-- {
		if inFile, seen := known[written[:length]]; seen {
			return inFile + written[length:]
		}
	}
	return written
}

// indented is the new lines as they go in the file: each takes the indentation the file has for the one it was
// written with.
func indented(lines []string, known map[string]string) []string {
	moved := make([]string, len(lines))
	for index, line := range lines {
		moved[index] = line
		if strings.TrimSpace(line) != "" {
			moved[index] = fileIndent(indentOf(line), known) + strings.TrimLeft(line, " \t")
		}
	}
	return moved
}

// firstWritten is the first line that says something, for its indentation.
func firstWritten(lines []string) string {
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}

// replaceByWords replaces oldString where the file says the same words on the same lines with another
// indentation: a model that copies a block from memory gets the words right far more often than the tabs. The
// new text takes the indentation the file has there. One place, unless replaceAll.
func replaceByWords(text, oldString, newString string, replaceAll bool) EditResult {
	file, wanted := strings.Split(text, lf), strings.Split(strings.Trim(oldString, lf), lf)
	places := placesOf(file, wanted)
	if len(places) == 0 || strings.TrimSpace(oldString) == "" {
		return EditResult{Problem: notFound + nearLines(file, wanted)}
	}
	if len(places) > 1 && !replaceAll {
		return EditResult{Problem: fmt.Sprintf("oldString appears %d times, indentation aside. Add the lines around it to pick one, or set replaceAll.", len(places))}
	}
	edited := []string{}
	next := 0
	for _, place := range places {
		known := indentations(wanted, file[place:place+len(wanted)])
		edited = append(edited, file[next:place]...)
		edited = append(edited, indented(strings.Split(strings.Trim(newString, lf), lf), known)...)
		next = place + len(wanted)
	}
	edited = append(edited, file[next:]...)
	return EditResult{Note: indentNote, OK: true, Replaced: len(places), Text: strings.Join(edited, lf)}
}

// nearLines names where the file has oldString's first line, when it has it and not the rest: the model reads from
// there instead of the whole file again.
func nearLines(file, wanted []string) string {
	first := strings.TrimSpace(firstWritten(wanted))
	if first == "" {
		return ""
	}
	said := ""
	found := 0
	for index, line := range file {
		if strings.TrimSpace(line) == first && found < maxNearLines {
			said += fmt.Sprintf(nearLine, index+1)
			found++
		}
	}
	return said
}

// ApplyEdit is the file's text with oldString replaced: exactly one occurrence, or all of them with replaceAll. Text
// that is not in the file as written is looked for by its words, line by line, whatever its indentation.
func ApplyEdit(text, oldString, newString string, replaceAll bool) EditResult {
	if oldString == "" {
		return EditResult{Problem: "oldString is empty. To create a file, use write."}
	}
	if oldString == newString {
		return EditResult{Problem: "oldString and newString are the same: there is nothing to change."}
	}
	if usesCrlf(text, oldString) {
		oldString, newString = toCrlf(oldString), toCrlf(newString)
	}
	if exact := replaceExactly(text, oldString, newString, replaceAll); exact.OK || exact.Problem != notFound {
		return exact
	}
	return replaceByWords(text, oldString, newString, replaceAll)
}
