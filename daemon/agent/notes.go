package agent

import (
	"context"
	"slices"
	"strings"

	"swcode/daemon/protocol"
)

const notesHeader = "The project’s notes (AGENTS.md), left by the user and by earlier sessions: what is already known of the project. Go by them rather than finding it all out again."

// NotesText is what the model reads of the project's notes: each file under its path.
func NotesText(notes []protocol.ProjectNote) string {
	parts := []string{notesHeader}
	for _, note := range notes {
		parts = append(parts, note.Path+":\n"+note.Text)
	}
	return strings.Join(parts, userSeparator)
}

// notesRead is whether the model was given the project's notes since the last compaction.
func notesRead(events []protocol.SessionEvent) bool {
	return slices.ContainsFunc(SinceCompaction(events), func(event protocol.SessionEvent) bool {
		_, read := event.Value.(protocol.SessionEventProjectNotesRead)
		return read
	})
}

// ReadNotes gives the model the project's notes once, with the first message that finds some, and again after a
// compaction: a message at the end of the conversation, as the skills are, so the system prompt and what came before
// stay as llm-server cached them. Notes changed later are not given again: the agent mostly changed them itself, and
// the whole file again would fill the context.
func ReadNotes(ctx context.Context, host TurnHost, setup Setup) {
	if setup.Notes == nil || notesRead(host.History()) {
		return
	}
	notes := setup.Notes(ctx)
	if len(notes) == 0 {
		return
	}
	host.Record(protocol.SessionEventProjectNotesRead{ID: setup.NewID(), Notes: notes})
}
