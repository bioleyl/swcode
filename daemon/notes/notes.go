// Package notes is the project's notes: its AGENTS.md files, which tell the agent what is already known of the
// project before it reads any code, so that each session does not find it all out again.
package notes

import (
	"context"
	"strings"

	"swcode/daemon/catalog"
	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
)

const (
	// FileName is the notes file's name, the one other coding agents read too.
	FileName = "AGENTS.md"
	// maxChars is the most of one file the model reads: the whole file goes in every request of the session.
	maxChars = 20_000
	// cutNote ends a file cut at maxChars.
	cutNote = "\n\n[The file goes on: read it whole if you need the rest.]"
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
		found = append(found, protocol.ProjectNote{Path: path, Text: clipped(string(data))})
	}
	return found
}

// clipped is a file's text, cut at maxChars characters.
func clipped(text string) string {
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + cutNote
}
