package notes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"swcode/daemon/hosts"
)

func notesIn(t *testing.T, text string) []string {
	t.Helper()
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, FileName), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	found := Find(context.Background(), hosts.NewLocalHost(hosts.LocalOptions{Home: folder}), folder)
	texts := []string{}
	for _, note := range found {
		texts = append(texts, note.Text)
	}
	return texts
}

func TestAFileUpToTheLimitIsReadWhole(t *testing.T) {
	whole := strings.Repeat("é", maxChars)
	if found := notesIn(t, whole); len(found) != 1 || found[0] != whole {
		t.Fatalf("%d notes", len(found))
	}
}

func TestAFileOnePastTheLimitIsCutWithANote(t *testing.T) {
	found := notesIn(t, strings.Repeat("é", maxChars+1))
	if len(found) != 1 || found[0] != strings.Repeat("é", maxChars)+cutNote {
		t.Fatalf("%d notes", len(found))
	}
}

// longNotes is notes past the limit: a short section, then a long one of numbered notes, the oldest first.
func longNotes(count int) string {
	lines := []string{"# Project", "", "## Conventions", "", "- Tabs, not spaces.", "", "## Traps", ""}
	for index := range count {
		lines = append(lines, fmt.Sprintf("- Trap %d: %s", index, strings.Repeat("x", 190)), "  Its way around.")
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestNotesPastTheLimitLoseTheOldestOfTheirLongestSection(t *testing.T) {
	const count = 200
	text := longNotes(count)
	fitted, leftOut := Fit(text)
	if length := utf8.RuneCountInString(fitted); length > maxChars || leftOut != utf8.RuneCountInString(text)-length {
		t.Fatalf("%d characters kept, %d left out", length, leftOut)
	}
	for _, kept := range []string{"- Tabs, not spaces.", fmt.Sprintf("- Trap %d:", count-1), "## Traps"} {
		if !strings.Contains(fitted, kept) {
			t.Fatalf("%q is gone", kept)
		}
	}
	if strings.Contains(fitted, "- Trap 0:") {
		t.Fatal("the oldest note is still there")
	}
	gone := count - strings.Count(fitted, "- Trap ")
	if !strings.Contains(fitted, "## Traps\n\n"+fmt.Sprintf(leftOutNote, gone)+"\n- Trap ") {
		t.Fatalf("no line says %d notes are left out where they were", gone)
	}
}

func TestANoteAddedToNotesPastTheLimitIsStillRead(t *testing.T) {
	added := Add(longNotes(200), "Project", "Traps", "The newest trap.")
	if fitted, _ := Fit(added); !strings.Contains(fitted, "- The newest trap.") {
		t.Fatal("the note just added is left out")
	}
}

func TestAFolderWithoutNotesHasNone(t *testing.T) {
	found := Find(context.Background(), hosts.NewLocalHost(hosts.LocalOptions{}), t.TempDir())
	if len(found) != 0 {
		t.Fatalf("%d notes", len(found))
	}
}

func TestTheFirstNoteMakesTheNotesWithATitle(t *testing.T) {
	got := Add("", "app", "Commands", "make test runs everything.")
	if got != "# app\n\n## Commands\n\n- make test runs everything.\n" {
		t.Fatalf("%q", got)
	}
}

func TestANoteGoesAtTheEndOfItsSectionBeforeTheNext(t *testing.T) {
	notes := "# app\n\n## Commands\n\n- make build.\n\n## Traps\n\n- /tmp is in RAM.\n"
	got := Add(notes, "app", "commands", "make test.")
	if got != "# app\n\n## Commands\n\n- make build.\n- make test.\n\n## Traps\n\n- /tmp is in RAM.\n" {
		t.Fatalf("%q", got)
	}
}

func TestANoteOfANewSectionGoesAtTheEnd(t *testing.T) {
	got := Add("# app\n\n## Commands\n\n- make build.\n", "app", "Traps", "- /tmp is in RAM:\nkeep outputs on the disk.")
	if got != "# app\n\n## Commands\n\n- make build.\n\n## Traps\n\n- /tmp is in RAM:\n  keep outputs on the disk.\n" {
		t.Fatalf("%q", got)
	}
}

func TestANoteInAnEmptySectionIsSetApartFromItsHeading(t *testing.T) {
	got := Add("# app\n\n## Commands\n## Traps\n\n- x\n", "app", "Commands", "make build.")
	if got != "# app\n\n## Commands\n\n- make build.\n## Traps\n\n- x\n" {
		t.Fatalf("%q", got)
	}
}

func TestANotesFileThatIsALinkIsNeitherReadNorTakenForTheProjects(t *testing.T) {
	folder, outside := t.TempDir(), filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("a key the repository must not read"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(folder, FileName)); err != nil {
		t.Skip("links cannot be made here")
	}
	host := hosts.NewLocalHost(hosts.LocalOptions{Home: folder})
	if found := Find(context.Background(), host, folder); len(found) != 0 {
		t.Fatalf("a linked file was read as the project's notes: %+v", found)
	}
	if !Linked(context.Background(), host, filepath.Join(folder, FileName)) || Linked(context.Background(), host, outside) {
		t.Fatal("a link was not told from a file")
	}
	// A file not made yet is no link: the first notes are written there.
	if Linked(context.Background(), host, filepath.Join(t.TempDir(), FileName)) {
		t.Fatal("a file not made yet counts as a link")
	}
}
