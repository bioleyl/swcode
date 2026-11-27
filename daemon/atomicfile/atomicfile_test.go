package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAFileIsReplacedWholeAndLeavesNoDraft(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "projects.json")
	for _, text := range []string{"first", "second, longer"} {
		if err := Write(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		if data, _ := os.ReadFile(path); string(data) != text {
			t.Fatalf("holds %q", data)
		}
	}
	entries, _ := os.ReadDir(folder)
	if len(entries) != 1 {
		t.Fatalf("%d files are left", len(entries))
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("its mode is %v", info.Mode().Perm())
	}
}

func TestAFileThatCannotBeWrittenKeepsWhatItHeld(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "kept.json")
	if err := Write(path, []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(filepath.Join(folder, "missing", "kept.json"), []byte("x"), 0o600); err == nil {
		t.Fatal("a file in a folder that is not there was written")
	}
	if data, _ := os.ReadFile(path); string(data) != "kept" {
		t.Fatalf("holds %q", data)
	}
}
