package tools

import (
	"fmt"
	"strings"
)

const (
	lf       = "\n"
	crlf     = "\r\n"
	notFound = "oldString was not found in the file. Read the file again and copy the text exactly, with its indentation."
)

// EditResult is the file's text once edited, and how many replacements were made; or why it cannot be.
type EditResult struct {
	OK       bool
	Text     string
	Replaced int
	Problem  string
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

// ApplyEdit is the file's text with oldString replaced: exactly one occurrence, or all of them with replaceAll.
func ApplyEdit(text, oldString, newString string, replaceAll bool) EditResult {
	if oldString == "" {
		return EditResult{Problem: "oldString is empty. To create a file, use write."}
	}
	if oldString == newString {
		return EditResult{Problem: "oldString and newString are the same: there is nothing to change."}
	}
	if usesCrlf(text, oldString) {
		return replaceExactly(text, toCrlf(oldString), toCrlf(newString), replaceAll)
	}
	return replaceExactly(text, oldString, newString, replaceAll)
}
