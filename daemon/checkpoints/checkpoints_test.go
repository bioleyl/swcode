package checkpoints

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"swcode/daemon/hosts"
)

// local makes a host of this computer, and a workspace with a file in it.
func local(t *testing.T) (hosts.Host, string) {
	t.Helper()
	host := hosts.NewLocalHost(hosts.LocalOptions{Home: t.TempDir(), ScratchRoot: t.TempDir()})
	t.Cleanup(host.Close)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "app.ts"), []byte("answer = 41\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return host, workspace
}

func newStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(t.TempDir(), func(err error) { t.Errorf("checkpoints: %v", err) })
}

func TestARestorePointKeepsTheFilesATurnWritesAndPutsThemBack(t *testing.T) {
	host, workspace := local(t)
	store := newStore(t)
	recorder := store.Begin(host, "session", "turn", workspace)
	recording := recorder.Host()
	if err := recording.WriteFile(context.Background(), filepath.Join(workspace, "app.ts"), []byte("answer = 42\n")); err != nil {
		t.Fatal(err)
	}
	if err := recording.WriteFile(context.Background(), filepath.Join(workspace, "made.ts"), []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "elsewhere.ts")
	if err := recording.WriteFile(context.Background(), outside, []byte("x\n")); err != nil {
		t.Fatal(err)
	}
	files, err := recorder.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0] != "app.ts" || files[1] != "made.ts" {
		t.Fatalf("kept %v", files)
	}
	if data, _ := os.ReadFile(filepath.Join(workspace, "app.ts")); string(data) != "answer = 42\n" {
		t.Fatalf("write did not go through: %q", data)
	}

	restored, err := store.Restore(context.Background(), host, "session", "turn", workspace)
	if err != nil || len(restored) != 2 {
		t.Fatalf("restored %v, %v", restored, err)
	}
	if data, _ := os.ReadFile(filepath.Join(workspace, "app.ts")); string(data) != "answer = 41\n" {
		t.Fatalf("app.ts is %q", data)
	}
	if _, err := os.Stat(filepath.Join(workspace, "made.ts")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("made.ts survived")
	}
}

func TestARestorePointKnowsWhenItIsGone(t *testing.T) {
	host, workspace := local(t)
	store := newStore(t)
	if _, err := store.Restore(context.Background(), host, "session", "missing", workspace); !errors.Is(err, ErrNoCheckpoint) {
		t.Fatalf("err %v", err)
	}
}

func TestASessionKeepsItsNewestRestorePointsOnly(t *testing.T) {
	host, workspace := local(t)
	store := newStore(t)
	for turn := 0; turn < keptTurns+1; turn++ {
		recorder := store.Begin(host, "session", string(rune('a'+turn)), workspace)
		if err := recorder.Host().WriteFile(context.Background(), filepath.Join(workspace, "app.ts"), []byte("x\n")); err != nil {
			t.Fatal(err)
		}
		if _, err := recorder.Close(); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(store.folder, "session"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != keptTurns {
		t.Fatalf("%d restore points kept", len(entries))
	}
}
