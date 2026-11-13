package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	"swcode/daemon/sessions"
	. "swcode/daemon/testapp"
)

// repository makes the test's workspace a git repository with its files committed; a machine without git has
// nothing to say about worktrees.
func repository(t *testing.T, folder string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git here")
	}
	for _, args := range [][]string{
		{"init", "-q"}, {"add", "-A"},
		{"-c", "user.name=T", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "first"},
	} {
		command := exec.Command("git", args...)
		command.Dir = folder
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
}

func TestASessionWorksInAGitWorktreeOfItsOwnWhichGoesWithIt(t *testing.T) {
	test := Start(t, Options{Files: map[string]string{"app.ts": "const answer = 41;\n"}, Replies: []fakellama.Reply{
		call("read", map[string]any{"path": "app.ts"}),
		call("edit", map[string]any{"newString": "42", "oldString": "41", "path": "app.ts"}),
		call("bash", map[string]any{"command": "git -c user.name=T -c user.email=t@example.com -c commit.gpgsign=false commit -q -am answer && git branch --show-current"}),
		text("Committed."),
	}})
	repository(t, test.Workspace)

	session := test.NewSession(sessions.Request{Prompt: "Fix the answer", Worktree: true})
	test.UntilIdle(session.ID)
	short := session.ID[:8]
	worktree := filepath.Join(filepath.Dir(test.Workspace), filepath.Base(test.Workspace)+".swcode-"+short)
	t.Cleanup(func() { _ = os.RemoveAll(worktree) })
	if session.Workspace != worktree || session.WorktreeOf != test.Workspace || session.Branch != "swcode/"+short {
		t.Fatalf("the session works in %+v", session)
	}

	// The worktree's file changed and was committed on its branch; the project's folder is as it was.
	if data, _ := os.ReadFile(filepath.Join(worktree, "app.ts")); string(data) != "const answer = 42;\n" {
		t.Fatalf("the worktree's file is %q", data)
	}
	if test.ReadFile("app.ts") != "const answer = 41;\n" {
		t.Fatalf("the project's own file changed: %q", test.ReadFile("app.ts"))
	}
	finished := Finished(test.Events(session.ID))
	if committed := finished[len(finished)-1]; !committed.OK || strings.TrimSpace(committed.Output) != "swcode/"+short {
		t.Fatalf("the commit: %+v", committed)
	}
	// It is a session of the project its worktree was made from, which has no project of its own.
	projects := test.App.Projects.List()
	if len(projects) != 1 || projects[0].Path != test.Workspace {
		t.Fatalf("projects %+v", projects)
	}

	if problem := test.App.Sessions.Remove(session.ID); problem != "" {
		t.Fatal(problem)
	}
	test.Until("the worktree to be removed", func() bool {
		_, err := os.Stat(worktree)
		return os.IsNotExist(err)
	})
	// Its branch stays: what was committed there is not lost.
	branches := exec.Command("git", "branch", "--list", "swcode/"+short)
	branches.Dir = test.Workspace
	if output, _ := branches.Output(); !strings.Contains(string(output), "swcode/"+short) {
		t.Fatalf("the branch went with the worktree: %q", output)
	}
}

func TestAProjectsSessionsInWorktreesGoWithTheProject(t *testing.T) {
	test := Start(t, Options{Files: map[string]string{"app.ts": "x\n"}, Replies: []fakellama.Reply{text("Hello.")}})
	repository(t, test.Workspace)
	session := test.NewSession(sessions.Request{Prompt: "Hi", Worktree: true})
	test.UntilIdle(session.ID)
	t.Cleanup(func() { _ = os.RemoveAll(session.Workspace) })
	if problem := test.App.Sessions.RemoveIn(test.Workspace, protocol.LocalHost()); problem != "" {
		t.Fatal(problem)
	}
	if left := test.App.Sessions.List(); len(left) != 0 {
		t.Fatalf("%d sessions left in the project", len(left))
	}
}

func TestAFolderThatIsNoRepositoryRefusesAWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git here")
	}
	test := Start(t, Options{Files: map[string]string{"app.ts": "x\n"}, Replies: []fakellama.Reply{}})
	_, problem := test.App.Sessions.Create(t.Context(), sessions.Request{Prompt: "Hi", Worktree: true})
	if !strings.Contains(problem, "not a git repository") {
		t.Fatalf("refused with %q", problem)
	}
	if len(test.App.Sessions.List()) != 0 {
		t.Fatal("a session was made all the same")
	}
}

func TestAWorktreesSessionCommitsFromItsSandbox(t *testing.T) {
	needsSandbox(t)
	test := Start(t, Options{Files: map[string]string{"app.ts": "x\n"}, Settings: sandboxOn, Replies: []fakellama.Reply{
		call("bash", map[string]any{"command": "echo y > app.ts && git -c user.name=T -c user.email=t@example.com -c commit.gpgsign=false commit -q -am change && echo hook > \"$(git rev-parse --git-common-dir)/hooks/pre-commit\""}),
		text("Committed."),
	}})
	repository(t, test.Workspace)
	session := test.NewSession(sessions.Request{Prompt: "Commit", Worktree: true})
	test.UntilIdle(session.ID)
	t.Cleanup(func() { _ = os.RemoveAll(session.Workspace) })
	// The commit went into the repository's own .git folder, which the sandbox opened; its hooks stayed closed.
	log := exec.Command("git", "log", "--oneline", "-1", session.Branch)
	log.Dir = test.Workspace
	if output, _ := log.Output(); !strings.Contains(string(output), "change") {
		t.Fatalf("the branch's last commit: %q (%+v)", output, Finished(test.Events(session.ID)))
	}
	if _, err := os.Stat(filepath.Join(test.Workspace, ".git", "hooks", "pre-commit")); err == nil {
		t.Fatal("a sandboxed command wrote a git hook")
	}
}
