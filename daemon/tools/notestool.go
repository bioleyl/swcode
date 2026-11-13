package tools

import (
	"context"
	"fmt"

	"swcode/daemon/hosts"
	"swcode/daemon/notes"
	"swcode/daemon/permissions"
	"swcode/daemon/protocol"
)

const (
	// noteSummaryLength is how much of a note its call shows in a few words.
	noteSummaryLength = 60
	// tidyRequest follows a note added to notes longer than a session reads: the agent that fills them is the one
	// that can shorten them.
	tidyRequest = " The notes are now longer than a session reads: %d of their characters are left out, the oldest notes of their longest sections. Edit %s: merge the notes that say the same, and remove those that are no longer true."
)

// addedOutput is what the model reads once its notes are in: where, how many, and that the file needs tidying when
// it outgrew what a session reads.
func addedOutput(label string, count int, after string) string {
	output := fmt.Sprintf("Added to %s: %s.", label, counted(count, "note", "notes"))
	if _, leftOut := notes.Fit(after); leftOut > 0 {
		output += fmt.Sprintf(tidyRequest, leftOut, label)
	}
	return output
}

type noteInput struct {
	Section string `json:"section"`
	Text    string `json:"text"`
}

type notesInput struct {
	Notes []noteInput `json:"notes"`
}

// notesTool adds to the project's AGENTS.md without the agent reading it first: the notes came with the session's
// first message, and a note goes at the end of its section. Writing them must cost the agent as little as possible,
// or it does not do it.
var notesTool = Define(Spec[notesInput]{
	Check: func(ctx context.Context, input notesInput, tc *Context) Check {
		path := notes.Target(ctx, tc.Host, tc.Workspace)
		if notes.Linked(ctx, tc.Host, path) {
			return Refuse(path + " is a link to another file: the notes are written to a file of the project only.")
		}
		info, err := tc.Host.FileInfo(ctx, path)
		if err != nil {
			return Refuse(err.Error())
		}
		before, problem := notesText(ctx, tc.Host, path, info)
		if problem != "" {
			return Refuse(problem)
		}
		after := withNotes(before, tc.Host.Paths().Base(tc.Host.Paths().Dir(path)), input.Notes)
		label := WorkspacePath(tc.Host.Paths(), tc.Workspace, path)
		diff := diffDisplay(FileDiff(label, before, after))
		return Ready(func(ctx context.Context) Outcome {
			if problem := saveFile(ctx, tc, path, []byte(after), info, label); problem != "" {
				return Failure(problem, "")
			}
			return Outcome{Display: diff, OK: true, Output: addedOutput(label, len(input.Notes), after)}
		}, diff)
	},
	Description: "Adds to the project’s notes, its AGENTS.md, which every session starts from: what you had to find out about the project and the next session would too. A command and what matters about it, a trap and its way around, a convention the code doesn’t show, how the user wants something done. Use it without being asked, before you end a task that taught you such a thing. When the project has no notes yet (none came with the first message), add its first ones at the end of your first task: how to build, test and run it, and what you found out. One checked fact per note, never a guess, nor what the code or the README already says, nor what only mattered to this task. To fix or remove a note, edit AGENTS.md.",
	Input: Object(Required("notes", Array(Object(
		Required("section", String().Trimmed().Min(1).Describe("The heading it goes under, such as Commands, Conventions or Traps: one the notes have when it fits.")),
		Required("text", String().Trimmed().Min(1).Describe("The fact in a line or two, then its reason or its way around.")),
	)).Min(1))),
	Name:    protocol.ToolNameNotes,
	Subject: func(notesInput, *Context) permissions.Subject { return edits(notes.FileName) },
	Summarize: func(input notesInput) string {
		return clipped(input.Notes[0].Section+": "+input.Notes[0].Text, noteSummaryLength)
	},
})

// notesText is what the notes say now: nothing when the file is not there yet.
func notesText(ctx context.Context, host hosts.Host, path string, info hosts.FileInfo) (string, string) {
	if info.Kind == hosts.KindMissing {
		return "", ""
	}
	if info.Kind != hosts.KindFile {
		return "", path + " is not a file."
	}
	text, err := readText(ctx, host, path)
	if err != nil {
		return "", err.Error()
	}
	return text, ""
}

// withNotes is the notes with each new one at the end of its section, in the order given.
func withNotes(text, title string, added []noteInput) string {
	for _, note := range added {
		text = notes.Add(text, title, note.Section, note.Text)
	}
	return text
}
