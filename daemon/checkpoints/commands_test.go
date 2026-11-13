package checkpoints

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"swcode/daemon/hosts"
)

func put(t *testing.T, folder, path, text string) {
	t.Helper()
	target := filepath.Join(folder, path)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, folder string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = folder
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

// repository is a workspace that is a git repository: a committed file, one git has never seen, one it ignores.
func repository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git here")
	}
	workspace := t.TempDir()
	put(t, workspace, "app.ts", "answer = 41\n")
	put(t, workspace, "both.ts", "one\n")
	put(t, workspace, ".gitignore", "dist/\n")
	git(t, workspace, "init", "-q")
	git(t, workspace, "add", "-A")
	git(t, workspace, "commit", "-q", "-m", "first")
	put(t, workspace, "notes.txt", "draft\n")
	put(t, workspace, "dist/out.js", "old build\n")
	return workspace
}

// hostsOf are this computer, and a host reached through a bash, as wsl.exe or ssh reach one.
func hostsOf(t *testing.T) map[string]hosts.Host {
	t.Helper()
	local := hosts.NewLocalHost(hosts.LocalOptions{Home: t.TempDir(), ScratchRoot: t.TempDir()})
	shell := hosts.NewShellHost(hosts.LocalTransport("build-box", ""))
	t.Cleanup(func() {
		local.Close()
		shell.Close()
	})
	return map[string]hosts.Host{"local": local, "shell": shell}
}

func read(t *testing.T, folder, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(folder, path))
	if err != nil {
		return "(none)"
	}
	return string(data)
}

func TestARestorePointKeepsWhatTheTurnsCommandsChangedInARepository(t *testing.T) {
	for name, host := range hostsOf(t) {
		t.Run(name, func(t *testing.T) {
			workspace, scratch, store := repository(t), t.TempDir(), newStore(t)
			status, index := git(t, workspace, "status", "--porcelain"), read(t, workspace, ".git/index")
			objects := git(t, workspace, "count-objects")

			recorder := store.Begin(host, "session", "turn", workspace)
			recorder.WatchCommands(scratch)
			recording := recorder.Host()
			ran := recording.Run(context.Background(), strings.Join([]string{
				"sed -i s/41/42/ app.ts", "rm notes.txt", "echo new > made.ts", "echo built > dist/out.js",
				"mkdir -p deep/er && echo x > 'deep/er/a file.txt'", "ln -s app.ts link.ts", "echo two > both.ts",
			}, " && "), hosts.CommandOptions{Cwd: workspace, Timeout: 20 * time.Second})
			if ran.ExitCode == nil || *ran.ExitCode != 0 {
				t.Fatalf("the command: %+v", ran)
			}
			// A file a command changed first and a tool wrote after is kept as it was before the turn.
			if err := recording.WriteFile(context.Background(), filepath.Join(workspace, "both.ts"), []byte("three\n")); err != nil {
				t.Fatal(err)
			}
			if err := recording.WriteFile(context.Background(), filepath.Join(workspace, "dist/tool.js"), []byte("tool\n")); err != nil {
				t.Fatal(err)
			}
			files, err := recorder.Close()
			if err != nil {
				t.Fatal(err)
			}
			slices.Sort(files)
			// Not what git ignores unless a tool wrote it, nor a link.
			if want := []string{"app.ts", "both.ts", "deep/er/a file.txt", "dist/tool.js", "made.ts", "notes.txt"}; !slices.Equal(files, want) {
				t.Fatalf("kept %v", files)
			}
			// The repository itself is as it was: its index, its objects, what it says of its files but for the turn's.
			if read(t, workspace, ".git/index") != index || git(t, workspace, "count-objects") != objects {
				t.Fatal("the repository's index or objects were written")
			}

			if _, err := store.Restore(context.Background(), host, "session", "turn", workspace); err != nil {
				t.Fatal(err)
			}
			for path, want := range map[string]string{
				"app.ts": "answer = 41\n", "both.ts": "one\n", "deep/er/a file.txt": "(none)", "dist/tool.js": "(none)",
				"made.ts": "(none)", "notes.txt": "draft\n", "dist/out.js": "built\n",
			} {
				if got := read(t, workspace, path); got != want {
					t.Errorf("%s is %q after the restore, not %q", path, got, want)
				}
			}
			_ = os.Remove(filepath.Join(workspace, "link.ts"))
			_ = os.RemoveAll(filepath.Join(workspace, "deep"))
			if now := git(t, workspace, "status", "--porcelain"); now != status {
				t.Fatalf("git sees %q, not %q as before the turn", now, status)
			}
		})
	}
}

func TestEachTurnKeepsWhatItsOwnCommandsChanged(t *testing.T) {
	host := hostsOf(t)["local"]
	workspace, scratch, store := repository(t), t.TempDir(), newStore(t)
	turn := func(id, command string) []string {
		recorder := store.Begin(host, "session", id, workspace)
		recorder.WatchCommands(scratch)
		recorder.Host().Run(context.Background(), command, hosts.CommandOptions{Cwd: workspace, Timeout: 20 * time.Second})
		files, err := recorder.Close()
		if err != nil {
			t.Fatal(err)
		}
		return files
	}
	if files := turn("first", "echo one > first.txt"); !slices.Equal(files, []string{"first.txt"}) {
		t.Fatalf("the first turn kept %v", files)
	}
	// What an earlier turn made is no change of this one, nor what the person did in between.
	put(t, workspace, "mine.txt", "by hand\n")
	if files := turn("second", "echo two >> first.txt && echo 42 > app.ts"); !slices.Equal(files, []string{"app.ts", "first.txt"}) {
		t.Fatalf("the second turn kept %v", files)
	}
	if files := turn("third", "true"); len(files) != 0 {
		t.Fatalf("a turn that changed nothing kept %v", files)
	}
	if _, err := store.Restore(context.Background(), host, "session", "second", workspace); err != nil {
		t.Fatal(err)
	}
	if read(t, workspace, "first.txt") != "one\n" || read(t, workspace, "app.ts") != "answer = 41\n" || read(t, workspace, "mine.txt") != "by hand\n" {
		t.Fatalf("after restoring the second turn: %q, %q", read(t, workspace, "first.txt"), read(t, workspace, "app.ts"))
	}
}

func TestOutsideARepositoryARestorePointKeepsWhatTheToolsWroteOnly(t *testing.T) {
	host, workspace := local(t)
	store := newStore(t)
	recorder := store.Begin(host, "session", "turn", workspace)
	recorder.WatchCommands(t.TempDir())
	recording := recorder.Host()
	recording.Run(context.Background(), "echo 42 > app.ts", hosts.CommandOptions{Cwd: workspace, Timeout: 20 * time.Second})
	if err := recording.WriteFile(context.Background(), filepath.Join(workspace, "made.ts"), []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	files, err := recorder.Close()
	if err != nil || !slices.Equal(files, []string{"made.ts"}) {
		t.Fatalf("kept %v, %v", files, err)
	}
}
