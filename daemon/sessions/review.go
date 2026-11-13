package sessions

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	"swcode/daemon/tools"
)

const (
	// maxReviewFiles is how many files a review shows: the others are counted.
	maxReviewFiles = 50
	// maxReviewBytes is the largest file a review shows the lines of.
	maxReviewBytes = 1 << 20
	reviewTimeout  = 30 * time.Second
	commitTimeout  = 2 * time.Minute
	reviewFolder   = "review"
	reviewList     = "list"
	reviewCount    = "count"
	// What the review's script says first, then the branch on a line of its own.
	reviewReady  = "ok"
	reviewNoGit  = "nogit"
	reviewNoRepo = "norepo"
	// What a line of the list says of a file's side: it is not there, it is too large to show, or (after) it is a
	// file to read where it is. Before, a number names the copy of what HEAD holds.
	sideAbsent = "-"
	sideLarge  = "big"
	listFields = 3
	maxGitSaid = 2000

	reviewNoFolder = "This session works in no folder: it has nothing to commit."
	noGitThere     = "git is not installed there."
	noRepository   = "This folder is not in a git repository."
	notShown       = "Not shown: a binary file, or one over 1 MiB."
	noMessage      = "A commit needs a message."
	reviewFailed   = "git did not answer: %s"
	commitFailed   = "The commit did not go through."
)

// Review is what a session's workspace has uncommitted: the branch it is on, a diff a file (what HEAD holds, and
// what the file is now), how many more files changed than are shown, or why there is nothing to show.
type Review struct {
	Branch  string
	Files   []protocol.ToolDisplay
	More    int
	Problem string
}

// reviewScript lists what the workspace has that HEAD has not: modified, added (not ignored) and deleted files, a
// line each in the scratch folder's list, with a copy of what HEAD holds of each beside it. git reads only, and
// runs no monitor of the repository's choosing; -z keeps every name as it is, spaces included.
func reviewScript(out string) string {
	return strings.Join([]string{
		`command -v git >/dev/null 2>&1 || { echo ` + reviewNoGit + `; exit 0; }`,
		`git rev-parse --is-inside-work-tree >/dev/null 2>&1 || { echo ` + reviewNoRepo + `; exit 0; }`,
		`out=` + hosts.ShellWord(out),
		`rm -rf -- "$out" && mkdir -p "$out" && : > "$out/` + reviewList + `" || exit 1`,
		`prefix=$(git rev-parse --show-prefix)`,
		`git -c core.fsmonitor=false status --porcelain=v1 --no-renames --untracked-files=all -z -- . | tr '\0' '\n' | {`,
		`  n=0`,
		`  while IFS= read -r entry; do`,
		`    full=${entry#???}`,
		`    path=${full#"$prefix"}`,
		// A repository inside the repository is listed as a folder: a review shows files.
		`    case "$path" in ''|*/) continue ;; esac`,
		`    n=$((n+1))`,
		fmt.Sprintf(`    [ "$n" -le %d ] || continue`, maxReviewFiles),
		`    before=` + sideAbsent,
		`    if size=$(git cat-file -s "HEAD:$full" 2>/dev/null); then`,
		fmt.Sprintf(`      if [ "$size" -le %d ] && git cat-file blob "HEAD:$full" > "$out/$n" 2>/dev/null; then before=$n; else before=%s; fi`, maxReviewBytes, sideLarge),
		`    fi`,
		`    now=` + sideAbsent,
		`    if [ -f "$path" ] && [ ! -L "$path" ]; then`,
		fmt.Sprintf(`      if [ "$(wc -c < "$path")" -le %d ]; then now=file; else now=%s; fi`, maxReviewBytes, sideLarge),
		`    fi`,
		`    printf '%s\t%s\t%s\n' "$before" "$now" "$path" >> "$out/` + reviewList + `"`,
		`  done`,
		`  echo "$n" > "$out/` + reviewCount + `"`,
		`}`,
		`echo ` + reviewReady,
		`git symbolic-ref --short -q HEAD || git rev-parse --short HEAD 2>/dev/null || true`,
	}, "\n")
}

// reviewPlace is where a session works and on which host; a refusal for a session that is not there or works nowhere.
func (s *Service) reviewPlace(sessionID string) (hosts.Host, string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, "", unknownSession
	}
	if session.meta.Workspace == "" {
		return nil, "", reviewNoFolder
	}
	return s.hostOf(session), session.meta.Workspace, ""
}

// reviewSide reads one side of a file's diff: its text, and whether a diff can show it (not too large, no binary).
func reviewSide(ctx context.Context, host hosts.Host, side, path string) (string, bool) {
	switch side {
	case sideAbsent:
		return "", true
	case sideLarge:
		return "", false
	}
	data, err := host.ReadFile(ctx, path)
	if err != nil || bytes.Contains(data, []byte{0}) {
		return "", false
	}
	return string(data), true
}

// clippedText is what git said, short enough to show.
func clippedText(text string) string {
	said := []rune(strings.TrimSpace(text))
	return string(said[:min(len(said), maxGitSaid)])
}

// Review is what the session's workspace has uncommitted, for the person to look at before they commit it. A refusal
// is for a session that is not there or works in no folder; why git has nothing to show is the review's own problem.
func (s *Service) Review(sessionID string) (Review, string) {
	host, workspace, refused := s.reviewPlace(sessionID)
	if refused != "" {
		return Review{}, refused
	}
	ctx, cancel := context.WithTimeout(context.Background(), reviewTimeout)
	defer cancel()
	review := Review{Files: []protocol.ToolDisplay{}}
	scratch, err := host.ScratchFolder(ctx, sessionID)
	if err != nil {
		review.Problem = fmt.Sprintf(reviewFailed, err)
		return review, ""
	}
	paths := host.Paths()
	out := paths.Join(scratch, reviewFolder)
	defer func() { _ = host.RemoveFolder(context.WithoutCancel(ctx), out) }()
	result := host.Run(ctx, reviewScript(out), hosts.CommandOptions{Cwd: workspace, Timeout: reviewTimeout})
	said := strings.Split(strings.TrimSpace(result.Output), "\n")
	switch strings.TrimSpace(said[0]) {
	case reviewNoGit:
		review.Problem = noGitThere
		return review, ""
	case reviewNoRepo:
		review.Problem = noRepository
		return review, ""
	case reviewReady:
	default:
		review.Problem = fmt.Sprintf(reviewFailed, clippedText(result.Output))
		return review, ""
	}
	if len(said) > 1 {
		review.Branch = strings.TrimSpace(said[1])
	}
	list, err := host.ReadFile(ctx, paths.Join(out, reviewList))
	if err != nil {
		review.Problem = fmt.Sprintf(reviewFailed, err)
		return review, ""
	}
	for _, line := range strings.Split(strings.TrimRight(string(list), "\n"), "\n") {
		parts := strings.SplitN(line, "\t", listFields)
		if len(parts) != listFields {
			continue
		}
		path := parts[2]
		before, beforeShown := reviewSide(ctx, host, parts[0], paths.Join(out, parts[0]))
		after, afterShown := reviewSide(ctx, host, parts[1], paths.Resolve(workspace, path))
		diff := protocol.ToolDisplayDiff{Lines: []protocol.DiffLine{{Kind: protocol.DiffLineKindGap, Text: notShown}}, Path: path}
		if beforeShown && afterShown {
			diff = tools.FileDiff(path, before, after)
		}
		review.Files = append(review.Files, protocol.ToolDisplay{Value: diff})
	}
	if count, err := host.ReadFile(ctx, paths.Join(out, reviewCount)); err == nil {
		total, _ := strconv.Atoi(strings.TrimSpace(string(count)))
		review.More = max(0, total-maxReviewFiles)
	}
	return review, ""
}

// Commit commits everything the session's workspace has uncommitted, with the person's message: it is their own
// action, so it runs as they would run it, outside any sandbox. What git said when it refused (nothing to commit, no
// identity) is the problem; empty when the commit was made.
func (s *Service) Commit(sessionID, message string) string {
	host, workspace, refused := s.reviewPlace(sessionID)
	if refused != "" {
		return refused
	}
	if strings.TrimSpace(message) == "" {
		return noMessage
	}
	ctx, cancel := context.WithTimeout(context.Background(), commitTimeout)
	defer cancel()
	script := strings.Join([]string{
		`command -v git >/dev/null 2>&1 || { echo ` + hosts.ShellWord(noGitThere) + `; exit 1; }`,
		`git rev-parse --is-inside-work-tree >/dev/null 2>&1 || { echo ` + hosts.ShellWord(noRepository) + `; exit 1; }`,
		`git add -A -- . && git commit -q -m ` + hosts.ShellWord(strings.TrimSpace(message)),
	}, "\n")
	result := host.Run(ctx, script, hosts.CommandOptions{Cwd: workspace, Timeout: commitTimeout})
	if result.ExitCode != nil && *result.ExitCode == 0 {
		return ""
	}
	if said := clippedText(result.Output); said != "" {
		return said
	}
	return commitFailed
}
