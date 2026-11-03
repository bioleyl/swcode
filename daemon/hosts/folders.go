package hosts

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	homeMark     = "~"
	hiddenPrefix = "."
	fullPathHint = "Give the folder as a full path, such as /home/me or ~/projects."
)

var homePrefixes = []string{homeMark + "/", homeMark + `\`}

var relativeNames = map[string]bool{".": true, "..": true}

// ExpandHome writes a path given as ~ or ~/project as a full path, in the host's path style.
func ExpandHome(path, home string, paths Paths) string {
	if path == homeMark {
		return home
	}
	for _, prefix := range homePrefixes {
		if strings.HasPrefix(path, prefix) {
			return paths.Join(home, path[len(prefix):])
		}
	}
	return path
}

func localFullPath(input, home string) (string, bool) {
	expanded := ExpandHome(input, home, NativePaths)
	if !filepath.IsAbs(expanded) {
		return "", false
	}
	return filepath.Clean(expanded), true
}

// subfolders is the subfolders to offer, by name: hidden ones (.git, .cache) left out, in alphabetical order.
func subfolders(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), hiddenPrefix) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// localListFolders is a folder's subfolders, to pick a project's folder by browsing.
func localListFolders(input, home string) FolderListing {
	path, ok := localFullPath(input, home)
	if !ok {
		return FolderListing{Path: input, Problem: fullPathHint}
	}
	if info, _ := localFileInfo(path); info.Kind != KindDirectory {
		return FolderListing{Path: input, Problem: fmt.Sprintf("%s is not a folder on the daemon's computer.", input)}
	}
	real, err := filepath.EvalSymlinks(path)
	if err == nil {
		folders, listErr := subfolders(real)
		if listErr == nil {
			return FolderListing{Folders: folders, OK: true, Parent: parentOf(real), Path: real}
		}
		err = listErr
	}
	return FolderListing{Path: input, Problem: fmt.Sprintf("%s cannot be read: %v.", input, err)}
}

// parentOf is a folder's parent; none at the root.
func parentOf(folder string) string {
	parent := filepath.Dir(folder)
	if parent == folder {
		return ""
	}
	return parent
}

// localMakeFolder makes a new, empty folder inside one that exists, for a new project.
func localMakeFolder(parentInput, name, home string) FolderMaking {
	parent, ok := localFullPath(parentInput, home)
	if info, _ := localFileInfo(parent); !ok || info.Kind != KindDirectory {
		return FolderMaking{Problem: fmt.Sprintf("%s is not a folder on the daemon's computer.", parentInput)}
	}
	if relativeNames[name] {
		return FolderMaking{Problem: fmt.Sprintf("%s cannot be a folder's name.", name)}
	}
	real, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return FolderMaking{Problem: fmt.Sprintf("%s cannot be read: %v.", parentInput, err)}
	}
	path := filepath.Join(real, name)
	if err := os.Mkdir(path, 0o755); err != nil {
		reason := err.Error()
		if errors.Is(err, fs.ErrExist) {
			reason = "it already exists"
		}
		return FolderMaking{Problem: fmt.Sprintf("%s cannot be made: %s.", path, reason)}
	}
	return FolderMaking{OK: true, Path: path}
}

// ChooseFolder is the folder a new session or project works in, on its host: the fallback when none is given (the
// daemon's own folder, on this computer), else a folder that exists there.
func ChooseFolder(ctx context.Context, host Host, input, fallback string) (string, string) {
	if input == "" {
		if fallback == "" {
			return "", "Give the folder to work in."
		}
		return fallback, ""
	}
	home, err := host.Home(ctx)
	if err != nil {
		return "", "The host did not answer: " + err.Error()
	}
	expanded := ExpandHome(input, home, host.Paths())
	if !host.Paths().IsAbs(expanded) {
		return "", "Give the folder as a full path, such as /home/me/project or ~/project."
	}
	path := host.Paths().Clean(expanded)
	if info, err := host.FileInfo(ctx, path); err != nil || info.Kind != KindDirectory {
		return "", input + " is not a folder there."
	}
	real, err := host.RealPath(ctx, path)
	if err != nil {
		return "", "The host did not answer: " + err.Error()
	}
	return real, ""
}
