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

// gitIn runs git in a folder, as a person with a name would.
func gitIn(t *testing.T, folder string, args ...string) string {
	t.Helper()
	identity := []string{"-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}
	command := exec.Command("git", append(identity, args...)...)
	command.Dir = folder
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

// startInRepository is a daemon whose workspace is a git repository with a first commit, and a session in it.
func startInRepository(t *testing.T, files map[string]string) (*App, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git here")
	}
	test := Start(t, Options{Files: files, Replies: []fakellama.Reply{}})
	gitIn(t, test.Workspace, "init", "-q", "-b", "main")
	gitIn(t, test.Workspace, "config", "user.name", "Test")
	gitIn(t, test.Workspace, "config", "user.email", "test@example.com")
	gitIn(t, test.Workspace, "config", "commit.gpgsign", "false")
	gitIn(t, test.Workspace, "add", "-A")
	gitIn(t, test.Workspace, "commit", "-q", "-m", "first")
	return test, test.NewSession(sessions.Request{}).ID
}

func diffOf(t *testing.T, review sessions.Review, path string) protocol.ToolDisplayDiff {
	t.Helper()
	for _, file := range review.Files {
		if diff, ok := file.Value.(protocol.ToolDisplayDiff); ok && diff.Path == path {
			return diff
		}
	}
	t.Fatalf("the review has no %s: %+v", path, review.Files)
	return protocol.ToolDisplayDiff{}
}

func TestAReviewShowsWhatTheWorkspaceHasUncommittedAndACommitTakesItAll(t *testing.T) {
	test, sessionID := startInRepository(t, map[string]string{
		"app.ts": "const answer = 41;\n", "old.ts": "gone soon\n", "kept.ts": "as it is\n", ".gitignore": "dist/\n",
	})
	put(t, test.Workspace, "app.ts", "const answer = 42;\n")
	put(t, test.Workspace, "docs/a new file.md", "# New\n")
	put(t, test.Workspace, "dist/out.js", "built\n")
	put(t, test.Workspace, "logo.bin", "PNG\x00\x01\x02")
	if err := os.Remove(filepath.Join(test.Workspace, "old.ts")); err != nil {
		t.Fatal(err)
	}

	review, refused := test.App.Sessions.Review(sessionID)
	if refused != "" || review.Problem != "" || review.Branch != "main" || review.More != 0 {
		t.Fatalf("the review: %+v, %q", review, refused)
	}
	// A file modified, one added (its name has a space), one deleted; not what git ignores, nor the file left as it is.
	if len(review.Files) != 4 {
		t.Fatalf("%d files in the review: %+v", len(review.Files), review.Files)
	}
	if changed := diffOf(t, review, "app.ts"); changed.Added != 1 || changed.Removed != 1 {
		t.Fatalf("app.ts: %+v", changed)
	}
	if made := diffOf(t, review, "docs/a new file.md"); made.Added != 1 || made.Removed != 0 {
		t.Fatalf("the new file: %+v", made)
	}
	if gone := diffOf(t, review, "old.ts"); gone.Added != 0 || gone.Removed != 1 {
		t.Fatalf("the deleted file: %+v", gone)
	}
	// A binary file is listed, its bytes not shown.
	if binary := diffOf(t, review, "logo.bin"); binary.Added != 0 || len(binary.Lines) != 1 || binary.Lines[0].Kind != protocol.DiffLineKindGap {
		t.Fatalf("the binary file: %+v", binary)
	}

	if problem := test.App.Sessions.Commit(sessionID, "  "); !strings.Contains(problem, "message") {
		t.Fatalf("a commit without a message: %q", problem)
	}
	if problem := test.App.Sessions.Commit(sessionID, "Answer 42, it's \"final\""); problem != "" {
		t.Fatal(problem)
	}
	if status := gitIn(t, test.Workspace, "status", "--porcelain"); status != "" {
		t.Fatalf("the tree after the commit: %q", status)
	}
	if log := gitIn(t, test.Workspace, "log", "-1", "--format=%s"); strings.TrimSpace(log) != `Answer 42, it's "final"` {
		t.Fatalf("the commit says %q", log)
	}
	after, _ := test.App.Sessions.Review(sessionID)
	if len(after.Files) != 0 || after.Problem != "" {
		t.Fatalf("the review after the commit: %+v", after)
	}
	// With nothing left to commit, git's own words say so.
	if problem := test.App.Sessions.Commit(sessionID, "Again"); !strings.Contains(problem, "nothing to commit") {
		t.Fatalf("a commit of nothing: %q", problem)
	}
}

func TestAReviewShowsItsFirstFilesAndCountsTheOthers(t *testing.T) {
	test, sessionID := startInRepository(t, map[string]string{"app.ts": "one\n"})
	for index := 0; index < 53; index++ {
		put(t, test.Workspace, filepath.Join("made", "file-"+string(rune('a'+index/26))+string(rune('a'+index%26))+".txt"), "new\n")
	}
	review, refused := test.App.Sessions.Review(sessionID)
	if refused != "" || len(review.Files) != 50 || review.More != 3 {
		t.Fatalf("%d files shown, %d more, %q", len(review.Files), review.More, refused)
	}
}

func TestAFolderThatIsNoRepositoryHasNothingToReviewAndSaysSo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git here")
	}
	test := Start(t, Options{Files: map[string]string{"app.ts": "one\n"}, Replies: []fakellama.Reply{}})
	session := test.NewSession(sessions.Request{})
	review, refused := test.App.Sessions.Review(session.ID)
	if refused != "" || !strings.Contains(review.Problem, "not in a git repository") || len(review.Files) != 0 {
		t.Fatalf("outside a repository: %+v, %q", review, refused)
	}
	if problem := test.App.Sessions.Commit(session.ID, "First"); !strings.Contains(problem, "not in a git repository") {
		t.Fatalf("a commit outside a repository: %q", problem)
	}
	chat := test.NewSession(sessions.Request{Kind: protocol.SessionKindChat})
	if _, refused := test.App.Sessions.Review(chat.ID); !strings.Contains(refused, "no folder") {
		t.Fatalf("a chat's review: %q", refused)
	}
	if _, refused := test.App.Sessions.Review("no-such-session"); refused == "" {
		t.Fatal("a session that is not there was reviewed")
	}
}
