package hosts

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/dlclark/regexp2"
)

const (
	binaryProbeBytes = 8000
	lineBreak        = "\n"
	ignoreCaseFlag   = "i"
)

// ignoredFolders are folders of dependencies, builds and caches: searching them buries what the agent looks for.
var ignoredFolders = map[string]bool{
	".cache": true, ".git": true, ".next": true, ".venv": true, "__pycache__": true, "build": true,
	"coverage": true, "dist": true, "node_modules": true, "target": true, "venv": true,
}

// gitVisibleFiles: tracked files, and untracked ones the .gitignore files do not exclude, separated by NUL. The
// repository's own fsmonitor setting is left out: it names a program git would run, and listing files runs nothing
// the repository says.
var gitVisibleFiles = []string{"-c", "core.fsmonitor=false", "ls-files", "--cached", "--others", "--exclude-standard", "-z"}

// IsBinary tells a binary file: a NUL byte near the start.
func IsBinary(content []byte) bool {
	return bytes.IndexByte(content[:min(len(content), binaryProbeBytes)], 0) >= 0
}

func localFileInfo(path string) (FileInfo, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return FileInfo{Kind: KindMissing}, nil
	}
	if err != nil {
		return FileInfo{}, err
	}
	if info.IsDir() {
		return FileInfo{Kind: KindDirectory}, nil
	}
	return FileInfo{Kind: KindFile, ModTimeMs: info.ModTime().UnixMilli(), Size: info.Size()}, nil
}

// localRealPath is where a path really leads, links followed: for a file not created yet, from the nearest folder
// that exists.
func localRealPath(path string) (string, error) {
	real, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Abs(real)
	}
	parent := filepath.Dir(path)
	if !errors.Is(err, fs.ErrNotExist) || parent == path {
		return "", err
	}
	realParent, err := localRealPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(realParent, filepath.Base(path)), nil
}

// gitFiles is the files git shows under a folder, relative to it with slashes; nil outside a repository or without
// git.
func gitFiles(ctx context.Context, folder string) []string {
	cmd := exec.CommandContext(ctx, "git", gitVisibleFiles...)
	cmd.Dir = folder
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	files := []string{}
	for _, file := range strings.Split(string(output), "\x00") {
		if file != "" {
			files = append(files, file)
		}
	}
	return files
}

func isIgnored(relative string) bool {
	for _, part := range strings.Split(relative, "/") {
		if ignoredFolders[part] {
			return true
		}
	}
	return false
}

// isRegularFile leaves links out, as listings do.
func isRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

// localListFiles is the files under a folder that match a glob, up to a limit: in a git repository the ones git
// shows, so the .gitignore files apply; elsewhere all but dependency, build and cache folders. Links are left out.
func localListFiles(ctx context.Context, base, pattern string, limit int) ([]string, error) {
	if git := gitFiles(ctx, base); git != nil {
		return matchingFiles(base, pattern, limit, git), nil
	}
	walked := []string{}
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return walkProblem(path, base, err)
		}
		if entry.IsDir() && path != base && ignoredFolders[entry.Name()] {
			return filepath.SkipDir
		}
		if entry.Type().IsRegular() {
			relative, _ := filepath.Rel(base, path)
			walked = append(walked, filepath.ToSlash(relative))
		}
		return ctx.Err()
	})
	return matchingFiles(base, pattern, limit, walked), err
}

// walkProblem lets a listing go on past a folder it cannot read; a base that is not there lists nothing.
func walkProblem(path, base string, err error) error {
	if path == base {
		return filepath.SkipAll
	}
	if errors.Is(err, fs.ErrPermission) {
		return nil
	}
	return err
}

func matchingFiles(base, pattern string, limit int, relatives []string) []string {
	found := []string{}
	for _, relative := range relatives {
		if len(found) >= limit {
			break
		}
		if isIgnored(relative) || !doublestar.MatchUnvalidated(pattern, relative) {
			continue
		}
		path := filepath.Join(base, filepath.FromSlash(relative))
		if isRegularFile(path) {
			found = append(found, path)
		}
	}
	return found
}

// CompilePattern reads a search's expression as JavaScript does (lookarounds included), its flags too.
func CompilePattern(source, flags string) (*regexp2.Regexp, error) {
	options := regexp2.RegexOptions(regexp2.ECMAScript)
	if strings.Contains(flags, ignoreCaseFlag) {
		options |= regexp2.IgnoreCase
	}
	return regexp2.Compile(source, options)
}

// matchesIn is the lines of a text file that match, unless it is too large or binary.
func matchesIn(file string, pattern *regexp2.Regexp, maxFileBytes int64) []SearchMatch {
	info, err := localFileInfo(file)
	if err != nil || info.Kind != KindFile || info.Size > maxFileBytes {
		return nil
	}
	content, err := os.ReadFile(file)
	if err != nil || IsBinary(content) {
		return nil
	}
	matches := []SearchMatch{}
	for index, text := range strings.Split(string(content), lineBreak) {
		if found, _ := pattern.MatchString(text); found {
			matches = append(matches, SearchMatch{File: file, Line: index + 1, Text: text})
		}
	}
	return matches
}

// localSearch is the matching lines under a folder, or in one file, up to the request's limits.
func localSearch(ctx context.Context, request SearchRequest) ([]SearchMatch, error) {
	pattern, err := CompilePattern(request.Source, request.Flags)
	if err != nil {
		return nil, err
	}
	files := []string{request.Root}
	if info, _ := localFileInfo(request.Root); info.Kind != KindFile {
		if files, err = localListFiles(ctx, request.Root, request.Include, request.MaxFiles); err != nil {
			return nil, err
		}
	}
	matches := []SearchMatch{}
	for _, file := range files {
		if len(matches) >= request.MaxMatches {
			break
		}
		found := matchesIn(file, pattern, request.MaxFileBytes)
		matches = append(matches, found[:min(len(found), request.MaxMatches-len(matches))]...)
	}
	return matches, nil
}
