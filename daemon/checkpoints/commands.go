package checkpoints

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"swcode/daemon/hosts"
)

const (
	// treeLimit is how long looking at the workspace may take: a repository with a huge folder git does not ignore
	// is given up on, and the turn's restore point then holds what the file tools wrote, as before.
	treeLimit = 30 * time.Second
	// maxCommandFiles is how many files changed by commands one restore point keeps.
	maxCommandFiles = 500
	gitState        = "git"
	changesFolder   = "changes"
	changesList     = "list"
	added           = "A"
	fields          = 3
)

// treeScript writes the workspace's files as a git tree and prints its name: every file git does not ignore,
// committed or not. Its index and its objects are the session's own, in its scratch folder, so the repository's
// index, objects and refs stay as they are; nothing is printed outside a repository, or when git fails. git runs
// no monitor of the repository's choosing while it looks.
func treeScript(scratch string) string {
	state := hosts.ShellWord(scratch + "/" + gitState)
	return strings.Join([]string{
		`git rev-parse --is-inside-work-tree >/dev/null 2>&1 || exit 0`,
		`state=` + state,
		`mkdir -p "$state/objects" || exit 0`,
		// The repository's own index knows which files have not changed since git last read them: starting from
		// it, only the others are read again. The session's index then remembers its own.
		`[ -f "$state/index" ] || cp "$(git rev-parse --git-path index)" "$state/index" 2>/dev/null`,
		`objects=$(cd "$(git rev-parse --git-path objects)" 2>/dev/null && pwd) || exit 0`,
		`export GIT_INDEX_FILE="$state/index" GIT_OBJECT_DIRECTORY="$state/objects" GIT_ALTERNATE_OBJECT_DIRECTORIES="$objects"`,
		`git -c core.fsmonitor=false add -A -- . >/dev/null 2>&1 || exit 0`,
		`tree=$(git write-tree 2>/dev/null) || exit 0`,
	}, "\n")
}

// changesScript looks at the workspace again and writes, in the scratch folder, what changed since a tree: a list
// (a line per file: A for one that was not there, else M with the name of the copy of what it held), and those
// copies. Links and repositories inside the repository are left out: a restore writes files.
func changesScript(scratch, before string) string {
	out := hosts.ShellWord(scratch + "/" + gitState + "/" + changesFolder)
	return strings.Join([]string{
		treeScript(scratch),
		`out=` + out,
		`rm -rf -- "$out" && mkdir -p "$out" || exit 0`,
		`tab=$(printf '\t')`,
		`n=0`,
		`git -c core.quotePath=false diff --raw --no-abbrev --no-renames --relative ` + before + ` "$tree" -- . | while IFS="$tab" read -r meta path; do`,
		`  set -- $meta`,
		`  case "$1$2" in *120000*|*160000*) continue ;; esac`,
		fmt.Sprintf(`  [ "$n" -lt %d ] || break`, maxCommandFiles),
		`  if [ "$5" = A ]; then`,
		`    printf 'A\t-\t%s\n' "$path"`,
		`  else`,
		`    size=$(git cat-file -s "$3" 2>/dev/null) || continue`,
		fmt.Sprintf(`    [ "$size" -le %d ] || continue`, maxKeptBytes),
		`    git cat-file blob "$3" > "$out/$n" || continue`,
		`    printf 'M\t%s\t%s\n' "$n" "$path"`,
		`  fi`,
		`  n=$((n+1))`,
		`done > "$out/list"`,
	}, "\n")
}

// treeNow is the workspace's files as a git tree, as the script printed it: empty where there is none to have.
func (r *Recorder) run(script string) string {
	ctx, cancel := context.WithTimeout(context.Background(), treeLimit)
	defer cancel()
	result := r.host.Run(ctx, script, hosts.CommandOptions{Cwd: r.workspace, Timeout: treeLimit})
	if result.ExitCode == nil || *result.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(result.Output)
}

// WatchCommands keeps what the turn's commands change too, in a git repository: the workspace's files are looked at
// now, while the model writes its first reply, and again when the turn ends. scratch is the session's scratch
// folder on the host. The turn's first command waits for the first look.
func (r *Recorder) WatchCommands(scratch string) {
	if r.workspace == "" || scratch == "" {
		return
	}
	r.scratch = scratch
	r.looked = make(chan struct{})
	go func() {
		defer close(r.looked)
		r.before = r.run(treeScript(scratch) + "\necho \"$tree\"")
	}()
}

// wait waits for the first look at the workspace, when the turn's commands are watched.
func (r *Recorder) wait() {
	if r.looked != nil {
		<-r.looked
	}
}

func (h recordingHost) Run(ctx context.Context, command string, options hosts.CommandOptions) hosts.CommandResult {
	h.recorder.wait()
	return h.Host.Run(ctx, command, options)
}

func (h recordingHost) StartJob(ctx context.Context, command, cwd string) (string, error) {
	h.recorder.wait()
	return h.Host.StartJob(ctx, command, cwd)
}

// keepCommandFiles adds the files the turn's commands changed to the restore point: what git says changed between
// its two looks. A file the tools wrote too is kept as git had it, which is how it was before the turn, whatever a
// command did to it before the tool's first write.
func (r *Recorder) keepCommandFiles() {
	r.wait()
	if r.before == "" {
		return
	}
	paths := r.host.Paths()
	out := paths.Join(r.scratch, gitState, changesFolder)
	r.run(changesScript(r.scratch, r.before))
	ctx, cancel := context.WithTimeout(context.Background(), treeLimit)
	defer cancel()
	defer func() { _ = r.host.RemoveFolder(context.WithoutCancel(ctx), out) }()
	list, err := r.host.ReadFile(ctx, paths.Join(out, changesList))
	if err != nil {
		return
	}
	known := map[string]int{}
	for index, file := range r.files {
		known[file.Path] = index
	}
	for _, line := range strings.Split(strings.TrimRight(string(list), "\n"), "\n") {
		parts := strings.SplitN(line, "\t", fields)
		if len(parts) != fields || parts[2] == "" {
			continue
		}
		kept := KeptFile{Existed: parts[0] != added, Path: parts[2]}
		index, tooled := known[kept.Path]
		if !tooled {
			index = len(r.files)
		}
		if kept.Existed {
			data, err := r.host.ReadFile(ctx, paths.Join(out, parts[1]))
			if err != nil {
				r.store.log(err)
				continue
			}
			kept.Stored = fmt.Sprintf(blobFormat, index)
			if err := os.MkdirAll(r.folder(), ownerOnly); err != nil {
				r.store.log(err)
				continue
			}
			if err := os.WriteFile(filepath.Join(r.folder(), kept.Stored), data, fileOnly); err != nil {
				r.store.log(err)
				continue
			}
		}
		if tooled {
			r.files[index] = kept
			continue
		}
		known[kept.Path] = index
		r.files = append(r.files, kept)
	}
}
