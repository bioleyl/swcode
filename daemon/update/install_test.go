package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsMovesTheRunningProgramAsideUntilTheNewOneStarts(t *testing.T) {
	folder := t.TempDir()
	program := filepath.Join(folder, "swcode-desktop.exe")
	downloaded := program + newSuffix
	_ = os.WriteFile(program, []byte("old"), programMode)
	_ = os.WriteFile(downloaded, []byte("new"), programMode)
	if err := replaceAside(program, downloaded); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(program); string(data) != "new" {
		t.Fatalf("the program is %q", data)
	}
	if data, _ := os.ReadFile(program + oldSuffix); string(data) != "old" {
		t.Fatalf("the old program is %q", data)
	}
	removeLeftover(program)
	if _, err := os.Stat(program + oldSuffix); !os.IsNotExist(err) {
		t.Fatal("the old program should go once the new one starts")
	}
}
