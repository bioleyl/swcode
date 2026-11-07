package notes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestAFolderWithoutNotesHasNone(t *testing.T) {
	found := Find(context.Background(), hosts.NewLocalHost(hosts.LocalOptions{}), t.TempDir())
	if len(found) != 0 {
		t.Fatalf("%d notes", len(found))
	}
}
