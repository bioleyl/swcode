package sessions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/tools"
)

const (
	// worktreeLimit is how long git has to make or remove a worktree: a large repository checks out many files.
	worktreeLimit = 2 * time.Minute
	// shortID is how much of a session's id names its branch and its folder: enough to tell sessions apart.
	shortID          = 8
	branchPrefix     = "swcode/"
	worktreeSuffix   = ".swcode-"
	commonGitKey     = "git-common-dir"
	noWorktreeHere   = "A git worktree needs a POSIX shell on the project's host: Windows' own shell has none."
	worktreeFailed   = "git made no worktree"
	worktreeNotGone  = "the worktree %s was not removed: %s"
	commonGitCommand = `cd -- "$(git rev-parse --git-common-dir)" 2>/dev/null && pwd -P`
)

// madeWorktree is a session's own worktree: the folder it works in there, and its branch.
type madeWorktree struct {
	workspace string
	branch    string
}

// worktreeScript makes a worktree of the repository a folder is in, on a branch of its own, beside the repository:
// it prints the folder there that is the one asked for (a project may be a folder inside its repository), or git's
// own words when git refuses.
func worktreeScript(short string) string {
	return strings.Join([]string{
		`top=$(git rev-parse --show-toplevel 2>&1) || { printf '%s\n' "$top"; exit 1; }`,
		`prefix=$(git rev-parse --show-prefix)`,
		`made="$(dirname -- "$top")/$(basename -- "$top")` + worktreeSuffix + short + `"`,
		`said=$(git worktree add -q -b ` + hosts.ShellWord(branchPrefix+short) + ` "$made" 2>&1) || { printf '%s\n' "$said"; exit 1; }`,
		`printf '%s\n' "$made${prefix:+/${prefix%/}}"`,
	}, "\n")
}

// makeWorktree gives a new session a git worktree of its own, made from the repository its folder is in: the
// problem, in git's own words, when the folder is in no repository or git refuses.
func makeWorktree(ctx context.Context, host hosts.Host, folder, sessionID string) (madeWorktree, string) {
	platform, err := host.Platform(ctx)
	if err != nil {
		return madeWorktree{}, "The host did not answer: " + err.Error()
	}
	if platform == hosts.PlatformWindows {
		return madeWorktree{}, noWorktreeHere
	}
	short := sessionID[:min(len(sessionID), shortID)]
	result := host.Run(ctx, worktreeScript(short), hosts.CommandOptions{Cwd: folder, Timeout: worktreeLimit})
	said := strings.TrimSpace(result.Output)
	if result.ExitCode == nil || *result.ExitCode != 0 || said == "" {
		if said == "" {
			said = worktreeFailed
		}
		return madeWorktree{}, said
	}
	lines := strings.Split(said, "\n")
	return madeWorktree{branch: branchPrefix + short, workspace: strings.TrimSpace(lines[len(lines)-1])}, ""
}

// removeWorktree removes a deleted session's worktree, when it had one: its branch stays, so what was committed
// there is not lost.
func removeWorktree(ctx context.Context, host hosts.Host, meta Meta) error {
	if meta.WorktreeOf == "" {
		return nil
	}
	script := `top=$(git rev-parse --show-toplevel) && cd -- ` + hosts.ShellWord(meta.WorktreeOf) + ` && git worktree remove --force "$top"`
	result := host.Run(ctx, script, hosts.CommandOptions{Cwd: meta.Workspace, Timeout: worktreeLimit})
	if result.ExitCode != nil && *result.ExitCode == 0 {
		return nil
	}
	return fmt.Errorf(worktreeNotGone, meta.Workspace, strings.TrimSpace(result.Output))
}

// commonGitFolder is the .git folder of the repository a session's worktree belongs to, where its commits go: asked
// of the host once. Empty for a session that works in its project's own folder, whose .git is in its workspace.
func (s *Service) commonGitFolder(ctx context.Context, session *live, host hosts.Host, meta Meta) string {
	if meta.WorktreeOf == "" {
		return ""
	}
	s.mu.Lock()
	known, found := session.realPaths[commonGitKey]
	s.mu.Unlock()
	if found {
		return known
	}
	result := host.Run(ctx, commonGitCommand, hosts.CommandOptions{Cwd: meta.Workspace, Timeout: worktreeLimit})
	folder := strings.TrimSpace(result.Output)
	if result.ExitCode == nil || *result.ExitCode != 0 || folder == "" {
		return ""
	}
	s.mu.Lock()
	if session.realPaths == nil {
		session.realPaths = map[string]string{}
	}
	session.realPaths[commonGitKey] = folder
	s.mu.Unlock()
	return folder
}

// projectTodos is the todos of a session's project, whatever folder the session works in: a worktree's session
// opens and closes the todos of the project its worktree was made from.
type projectTodos struct {
	todos  tools.ProjectTodos
	folder string
}

func (p projectTodos) OpenTodo(_ string, host protocol.HostRef, text string) ([]protocol.ProjectTodo, string) {
	return p.todos.OpenTodo(p.folder, host, text)
}

func (p projectTodos) CloseTodo(_ string, host protocol.HostRef, text string) ([]protocol.ProjectTodo, string) {
	return p.todos.CloseTodo(p.folder, host, text)
}

// todosOf is the project's todos as the session's agent opens and closes them; none where the daemon keeps none.
func (s *Service) todosOf(session *live) tools.ProjectTodos {
	if s.deps.Todos == nil {
		return nil
	}
	return projectTodos{folder: session.meta.projectFolder(), todos: s.deps.Todos}
}
