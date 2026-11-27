package sessions

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"swcode/daemon/hosts"
)

func TestWhatGitSaidOfAWorkspaceIsRead(t *testing.T) {
	cases := map[string]GitState{
		"nogit\n":  {Problem: noGitThere},
		"norepo\n": {},
		"ok\nbranch feature/git-info\nchanged 3\nupstream 2\t5\n": {
			Repository: true, Branch: "feature/git-info", Changed: 3, Follows: true, Behind: 2, Ahead: 5,
		},
		// A branch that follows none, with nothing uncommitted.
		"ok\nbranch main\nchanged 0\n":            {Repository: true, Branch: "main"},
		"ok\ndetached 4199495\nchanged       1\n": {Repository: true, Branch: "4199495", Detached: true, Changed: 1},
		// A repository with no commit yet has a branch and nothing to name a commit by.
		"ok\nbranch main\nchanged 2\n": {Repository: true, Branch: "main", Changed: 2},
	}
	for said, want := range cases {
		if got := gitStateOf(said); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %+v", said, got)
		}
	}
	if got := gitStateOf("bash: something broke"); got.Repository || got.Problem == "" {
		t.Errorf("an answer that is none: %+v", got)
	}
}

func TestTheScriptSaysWhereARepositoryStands(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git here")
	}
	root := t.TempDir()
	origin, workspace := filepath.Join(root, "origin.git"), filepath.Join(root, "work")
	git := func(folder string, args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "init.defaultBranch=main"}, args...)...)
		command.Dir = folder
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, output)
		}
	}
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(workspace, path), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	host := hosts.NewLocalHost(hosts.LocalOptions{Home: t.TempDir(), ScratchRoot: t.TempDir()})
	t.Cleanup(host.Close)
	stateOf := func(folder string) GitState {
		return gitStateOf(host.Run(context.Background(), gitStateScript, hosts.CommandOptions{Cwd: folder, Timeout: 20 * time.Second}).Output)
	}

	if state := stateOf(root); state.Repository || state.Problem != "" {
		t.Fatalf("a plain folder: %+v", state)
	}
	git(root, "init", "-q", "--bare", origin)
	git(root, "init", "-q", workspace)
	write("a.txt", "one\n")
	// No commit yet: on a branch, with a file git has not seen.
	if state := stateOf(workspace); !reflect.DeepEqual(state, GitState{Repository: true, Branch: "main", Changed: 1}) {
		t.Fatalf("before the first commit: %+v", state)
	}
	git(workspace, "add", "-A")
	git(workspace, "commit", "-q", "-m", "first")
	git(workspace, "remote", "add", "origin", origin)
	git(workspace, "push", "-q", "-u", "origin", "main")
	write("a.txt", "two\n")
	git(workspace, "commit", "-q", "-am", "second")
	write("b.txt", "new\n")
	if state := stateOf(workspace); !reflect.DeepEqual(state, GitState{Repository: true, Branch: "main", Changed: 1, Follows: true, Ahead: 1}) {
		t.Fatalf("one commit ahead, one file new: %+v", state)
	}
	git(workspace, "checkout", "-q", "--detach", "HEAD~1")
	if state := stateOf(workspace); !state.Detached || state.Branch == "" || state.Follows {
		t.Fatalf("on no branch: %+v", state)
	}
}
