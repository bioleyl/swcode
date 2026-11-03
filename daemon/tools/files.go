package tools

import (
	"context"
	"fmt"
	"strings"

	"swcode/daemon/hosts"
)

const (
	parentName = ".."
	hereName   = "."
)

// isInside tells whether a path is the root or under it, in the host's path style.
func isInside(paths hosts.Paths, root, path string) bool {
	fromRoot := paths.Rel(root, path)
	return !paths.IsAbs(fromRoot) && fromRoot != parentName && !strings.HasPrefix(fromRoot, parentName+paths.Separator())
}

// rootOf is the root a path is in: the workspace, or the scratch folder.
func rootOf(tc *Context, path string) string {
	for _, root := range []string{tc.Workspace, tc.Scratch} {
		if root != "" && isInside(tc.Host.Paths(), root, path) {
			return root
		}
	}
	return ""
}

func outsideProblem(tc *Context, input string) string {
	if tc.Scratch == "" {
		return fmt.Sprintf("%s is outside the workspace (%s).", input, tc.Workspace)
	}
	return fmt.Sprintf("%s is outside the workspace (%s) and the scratch folder (%s).", input, tc.Workspace, tc.Scratch)
}

// WorkspacePath is a path as people and the model read it: from the workspace, or whole when it is elsewhere (the
// scratch folder).
func WorkspacePath(paths hosts.Paths, workspace, path string) string {
	if !isInside(paths, workspace, path) {
		return path
	}
	if relative := paths.Rel(workspace, path); relative != "" {
		return relative
	}
	return hereName
}

// resolveTarget makes a path the model gave absolute from the workspace, refused when it is outside the workspace
// and the scratch folder, or leads outside its folder through a symbolic link: checked on the project's host.
func resolveTarget(ctx context.Context, tc *Context, input string) (string, string) {
	paths := tc.Host.Paths()
	path := paths.Resolve(tc.Workspace, input)
	folder := rootOf(tc, path)
	if folder == "" {
		return "", outsideProblem(tc, input)
	}
	root, err := tc.Host.RealPath(ctx, folder)
	if err != nil {
		return "", err.Error()
	}
	real, err := tc.Host.RealPath(ctx, path)
	if err != nil {
		return "", err.Error()
	}
	if !isInside(paths, root, real) {
		return "", fmt.Sprintf("%s leads outside %s through a link.", input, folder)
	}
	return path, ""
}

func readText(ctx context.Context, host hosts.Host, path string) (string, error) {
	data, err := host.ReadFile(ctx, path)
	return string(data), err
}

// noteSeen notes the file as the agent last saw it: the next change to it must start from this version.
func noteSeen(ctx context.Context, tc *Context, path string) {
	if info, err := tc.Host.FileInfo(ctx, path); err == nil && info.Kind == hosts.KindFile {
		tc.ReadFiles.Set(path, info.ModTimeMs)
	}
}

// stalenessProblem is why the agent may not change an existing file yet: never read, or changed since.
func stalenessProblem(tc *Context, path, label string, modTimeMs int64) string {
	seen, ok := tc.ReadFiles.Get(path)
	if !ok {
		return fmt.Sprintf("Read %s before changing it: changes only go to a file as you last read it.", label)
	}
	if seen != modTimeMs {
		return fmt.Sprintf("%s changed since you read it. Read it again first.", label)
	}
	return ""
}

func changedSince(before, now hosts.FileInfo) bool {
	return before.Kind != now.Kind || (before.Kind == hosts.KindFile && before.ModTimeMs != now.ModTimeMs)
}

// saveFile writes a file the agent checked earlier, unless it changed in the meantime: a person may edit it while
// the agent waits for approval. It returns the problem when it did not write.
func saveFile(ctx context.Context, tc *Context, path string, data []byte, checked hosts.FileInfo, label string) string {
	now, err := tc.Host.FileInfo(ctx, path)
	if err != nil {
		return err.Error()
	}
	if changedSince(checked, now) {
		return label + " changed while the change waited for approval. Read it again."
	}
	if err := tc.Host.WriteFile(ctx, path, data); err != nil {
		return err.Error()
	}
	noteSeen(ctx, tc, path)
	return ""
}

// lineCount is how many lines a text has: none when it is empty.
func lineCount(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

// counted is a count with its noun: "1 line", "3 lines".
func counted(count int, one, many string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, one)
	}
	return fmt.Sprintf("%d %s", count, many)
}

func linesLabel(count int) string { return counted(count, "line", "lines") }

// The path every file tool takes, and the folder the search tools start from.
var (
	filePath   = String().Min(1).Describe("The file, relative to the workspace.")
	searchRoot = String().Describe("The folder to search, relative to the workspace; the whole workspace when left out.")
)

// clipped cuts a line of a file or a match where it gets too long for the model.
func clipped(line string, limit int) string {
	runes := []rune(line)
	if len(runes) <= limit {
		return line
	}
	return string(runes[:limit]) + "…"
}
