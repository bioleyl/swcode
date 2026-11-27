package sessions

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"swcode/daemon/hosts"
)

const (
	gitStateTimeout = 15 * time.Second
	// What the script says of the workspace, a line each: the word, then its value.
	stateBranch   = "branch"
	stateDetached = "detached"
	stateChanged  = "changed"
	stateUpstream = "upstream"
	upstreamParts = 2
)

// GitState is where a workspace stands in git: in a repository or not, on which branch (a commit's short name when
// on none), with how many files uncommitted, and how far from the branch it follows when it follows one.
type GitState struct {
	Repository bool
	Branch     string
	Detached   bool
	Changed    int
	// Follows tells whether the branch follows another (its upstream): Ahead and Behind count from it.
	Follows bool
	Ahead   int
	Behind  int
	Problem string
}

// gitStateScript asks git, which only reads and runs no monitor of the repository's choosing. The commits of the
// branch followed are counted as git last fetched them: nothing is fetched here.
var gitStateScript = strings.Join([]string{
	`command -v git >/dev/null 2>&1 || { echo ` + reviewNoGit + `; exit 0; }`,
	`git rev-parse --is-inside-work-tree >/dev/null 2>&1 || { echo ` + reviewNoRepo + `; exit 0; }`,
	`echo ` + reviewReady,
	`if name=$(git symbolic-ref --short -q HEAD); then echo "` + stateBranch + ` $name"; else echo "` + stateDetached + ` $(git rev-parse --short HEAD 2>/dev/null)"; fi`,
	`echo "` + stateChanged + ` $(git -c core.fsmonitor=false status --porcelain=v1 --no-renames -- . | wc -l)"`,
	`counts=$(git rev-list --left-right --count '@{upstream}...HEAD' 2>/dev/null) && echo "` + stateUpstream + ` $counts"`,
	`exit 0`,
}, "\n")

// gitStateOf reads what the script said.
func gitStateOf(output string) GitState {
	said := strings.Split(strings.TrimSpace(output), "\n")
	switch strings.TrimSpace(said[0]) {
	case reviewNoGit:
		return GitState{Problem: noGitThere}
	case reviewNoRepo:
		return GitState{}
	case reviewReady:
	default:
		return GitState{Problem: fmt.Sprintf(reviewFailed, clippedText(output))}
	}
	state := GitState{Repository: true}
	for _, line := range said[1:] {
		word, value, _ := strings.Cut(strings.TrimSpace(line), " ")
		value = strings.TrimSpace(value)
		switch word {
		case stateBranch:
			state.Branch = value
		case stateDetached:
			state.Branch, state.Detached = value, true
		case stateChanged:
			state.Changed, _ = strconv.Atoi(value)
		case stateUpstream:
			// git counts the commits of each side: those only the branch followed has, then those only this one has.
			counts := strings.Fields(value)
			if len(counts) != upstreamParts {
				continue
			}
			state.Follows = true
			state.Behind, _ = strconv.Atoi(counts[0])
			state.Ahead, _ = strconv.Atoi(counts[1])
		}
	}
	return state
}

// GitState is where the session's workspace stands in git, for the page to say beside the session's folder. A
// refusal is for a session that is not there or works in no folder.
func (s *Service) GitState(sessionID string) (GitState, string) {
	host, workspace, refused := s.reviewPlace(sessionID)
	if refused != "" {
		return GitState{}, refused
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitStateTimeout)
	defer cancel()
	result := host.Run(ctx, gitStateScript, hosts.CommandOptions{Cwd: workspace, Timeout: gitStateTimeout})
	return gitStateOf(result.Output), ""
}
