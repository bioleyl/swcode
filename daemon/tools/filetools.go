package tools

import (
	"context"
	"fmt"
	"strings"

	"swcode/daemon/hosts"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
)

const (
	defaultReadLimit = 2000
	maxReadLineChars = 2000
	maxReadBytes     = 10_485_760
	lineNumberWidth  = 6
	firstLine        = 1
)

type readInput struct {
	Limit  *float64 `json:"limit"`
	Offset *float64 `json:"offset"`
	Path   string   `json:"path"`
}

func numberedLines(lines []string, first int) string {
	numbered := make([]string, len(lines))
	for index, line := range lines {
		numbered[index] = fmt.Sprintf("%*d\t%s", lineNumberWidth, first+index, clipped(line, maxReadLineChars))
	}
	return strings.Join(numbered, "\n")
}

// linesOf is a file's lines: one ending with a line break has no line after it.
func linesOf(text string) []string {
	if text == "" {
		return []string{}
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func whole(value *float64, fallback int) int {
	if value == nil {
		return fallback
	}
	return int(*value)
}

func excerpt(lines []string, input readInput) string {
	first := whole(input.Offset, firstLine)
	start := min(first-1, len(lines))
	shown := lines[start:min(len(lines), start+whole(input.Limit, defaultReadLimit))]
	if len(shown) == 0 {
		return fmt.Sprintf("The file has %d lines: there is nothing at line %d.", len(lines), first)
	}
	more := ""
	if after := len(lines) - (first - 1 + len(shown)); after > 0 {
		more = fmt.Sprintf("\n[%s: read on with offset %d]", counted(after, "more line", "more lines"), first+len(shown))
	}
	return numberedLines(shown, first) + more
}

func readLines(ctx context.Context, tc *Context, path string, input readInput) Outcome {
	content, err := tc.Host.ReadFile(ctx, path)
	if err != nil {
		return Failure(err.Error(), "failed")
	}
	if hosts.IsBinary(content) {
		return Failure(input.Path+" is a binary file.", "binary")
	}
	noteSeen(ctx, tc, path)
	lines := linesOf(string(content))
	output := "The file is empty."
	if len(lines) > 0 {
		output = excerpt(lines, input)
	}
	return Outcome{Meta: linesLabel(len(lines)), OK: true, Output: output}
}

var readTool = Define(Spec[readInput]{
	Check: func(ctx context.Context, input readInput, tc *Context) Check {
		path, problem := resolveTarget(ctx, tc, input.Path)
		if problem != "" {
			return Refuse(problem)
		}
		info, err := tc.Host.FileInfo(ctx, path)
		switch {
		case err != nil:
			return Refuse(err.Error())
		case info.Kind == hosts.KindMissing:
			return Refuse(input.Path + " does not exist.")
		case info.Kind == hosts.KindDirectory:
			return Refuse(input.Path + " is a folder: list its files with glob.")
		case info.Size > maxReadBytes:
			return Refuse(fmt.Sprintf("%s is too large to read (%d bytes): search it with grep.", input.Path, info.Size))
		}
		return Ready(func(ctx context.Context) Outcome { return readLines(ctx, tc, path, input) }, nil)
	},
	Description: "Reads a text file, with line numbers. Read a file before you edit it; for long files, read a part with offset and limit.",
	Input: Object(
		Optional("limit", Number().AtLeast(1).Describe("How many lines to read; up to 2000 when left out.")),
		Optional("offset", Number().AtLeast(1).Describe("The line to start from, counting from 1. For long files.")),
		Required("path", filePath),
	),
	Name:      protocol.ToolNameRead,
	Subject:   reads[readInput],
	Summarize: func(input readInput) string { return input.Path },
})

type writeInput struct {
	Content string `json:"content"`
	Path    string `json:"path"`
}

func edits(input string) permissions.Subject {
	return permissions.Subject{Access: permissions.AccessEdit, Path: input}
}

var writeTool = Define(Spec[writeInput]{
	Check: func(ctx context.Context, input writeInput, tc *Context) Check {
		path, problem := resolveTarget(ctx, tc, input.Path)
		if problem == "" {
			problem = gitFolderProblem(tc.Host.Paths(), path, input.Path)
		}
		if problem != "" {
			return Refuse(problem)
		}
		info, err := tc.Host.FileInfo(ctx, path)
		if err != nil {
			return Refuse(err.Error())
		}
		if info.Kind == hosts.KindDirectory {
			return Refuse(input.Path + " is a folder.")
		}
		before := ""
		if info.Kind == hosts.KindFile {
			if stale := stalenessProblem(tc, path, input.Path, info.ModTimeMs); stale != "" {
				return Refuse(stale)
			}
			if before, err = readText(ctx, tc.Host, path); err != nil {
				return Refuse(err.Error())
			}
		}
		label := WorkspacePath(tc.Host.Paths(), tc.Workspace, path)
		diff := diffDisplay(FileDiff(label, before, input.Content))
		verb := "Created"
		if info.Kind == hosts.KindFile {
			verb = "Wrote"
		}
		return Ready(func(ctx context.Context) Outcome {
			if problem := saveFile(ctx, tc, path, []byte(input.Content), info, label); problem != "" {
				return Failure(problem, "")
			}
			return Outcome{Display: diff, OK: true, Output: fmt.Sprintf("%s %s (%s).", verb, label, linesLabel(lineCount(input.Content)))}
		}, diff)
	},
	Description: "Writes a whole file: creates it, or replaces it after you read it. To change part of a file, use edit instead.",
	Input: Object(
		Required("content", String().Describe("The whole content of the file.")),
		Required("path", filePath),
	),
	Name:      protocol.ToolNameWrite,
	Subject:   func(input writeInput, _ *Context) permissions.Subject { return edits(input.Path) },
	Summarize: func(input writeInput) string { return input.Path },
})

type editInput struct {
	NewString  string `json:"newString"`
	OldString  string `json:"oldString"`
	Path       string `json:"path"`
	ReplaceAll bool   `json:"replaceAll"`
}

// editedOutput is what the model reads of an edit made: where, how many replacements, and how the text was found
// when not as it was written.
func editedOutput(label string, edit EditResult) string {
	said := fmt.Sprintf("Edited %s: %s", label, counted(edit.Replaced, "replacement", "replacements"))
	if edit.Note != "" {
		said += " (" + edit.Note + ")"
	}
	return said + "."
}

var editTool = Define(Spec[editInput]{
	Check: func(ctx context.Context, input editInput, tc *Context) Check {
		path, problem := resolveTarget(ctx, tc, input.Path)
		if problem == "" {
			problem = gitFolderProblem(tc.Host.Paths(), path, input.Path)
		}
		if problem != "" {
			return Refuse(problem)
		}
		info, err := tc.Host.FileInfo(ctx, path)
		if err != nil || info.Kind != hosts.KindFile {
			return Refuse(input.Path + " is not a file. To create one, use write.")
		}
		if stale := stalenessProblem(tc, path, input.Path, info.ModTimeMs); stale != "" {
			return Refuse(stale)
		}
		before, err := readText(ctx, tc.Host, path)
		if err != nil {
			return Refuse(err.Error())
		}
		edit := ApplyEdit(before, input.OldString, input.NewString, input.ReplaceAll)
		if !edit.OK {
			return Refuse(edit.Problem)
		}
		label := WorkspacePath(tc.Host.Paths(), tc.Workspace, path)
		diff := diffDisplay(FileDiff(label, before, edit.Text))
		return Ready(func(ctx context.Context) Outcome {
			if problem := saveFile(ctx, tc, path, []byte(edit.Text), info, label); problem != "" {
				return Failure(problem, "")
			}
			return Outcome{Display: diff, OK: true, Output: editedOutput(label, edit)}
		}, diff)
	},
	Description: "Replaces exact text in a file you read: oldString must appear once, unless replaceAll is set. Prefer it to write for changes.",
	Input: Object(
		Required("newString", String().Describe("The text to put in its place.")),
		Required("oldString", String().Describe("The exact text to replace, as the file has it, indentation included. Give enough lines to be unique.")),
		Required("path", filePath),
		Optional("replaceAll", Boolean().Describe("Replace every occurrence instead of exactly one.")),
	),
	Name:      protocol.ToolNameEdit,
	Subject:   func(input editInput, _ *Context) permissions.Subject { return edits(input.Path) },
	Summarize: func(input editInput) string { return input.Path },
})
