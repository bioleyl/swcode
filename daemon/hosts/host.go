// Package hosts is where a project's files are and its commands run: this computer, or another one reached through
// its bash (a WSL distribution, an SSH endpoint). The tools and the folder browser go through a Host, never to the
// disk directly.
package hosts

import (
	"context"
	"time"
)

// FileKind tells what a path is.
type FileKind string

const (
	KindMissing   FileKind = "missing"
	KindDirectory FileKind = "directory"
	KindFile      FileKind = "file"
)

// FileInfo is what a path is; a file's modification time and size come with it.
type FileInfo struct {
	Kind FileKind
	// ModTimeMs is in milliseconds since 1970, as the edits compare it with what the agent last read.
	ModTimeMs int64
	Size      int64
}

// SearchRequest is a search of the files under a folder for a regular expression, given as its source and flags.
type SearchRequest struct {
	Root string
	// Include is a glob of the files to search, from the root.
	Include      string
	Source       string
	Flags        string
	MaxMatches   int
	MaxFiles     int
	MaxFileBytes int64
}

// SearchMatch is a line that matched, by its file and its number from 1.
type SearchMatch struct {
	File string
	Line int
	Text string
}

// Passwords answers what a command this computer ran asked for, and gives up when the turn stops before anyone
// does: the password typed, and whether one was (none closes the command's input).
type Passwords interface {
	Secret(ctx context.Context, prompt string) (text string, gave bool)
}

// CommandOptions is where a command runs and how long it may take.
type CommandOptions struct {
	Cwd     string
	Timeout time.Duration
	// Passwords answers what the command asks for when it stops to ask for a password: nil where nobody can, which
	// is how commands on another host run, and how a subagent's do.
	Passwords Passwords
}

// CommandResult is how a command ended: its exit code (none when a signal ended it), what it printed, and why it
// was ended early, when it was.
type CommandResult struct {
	ExitCode *int
	Output   string
	TimedOut bool
	Stopped  bool
}

// JobState is a command running on after the call that started it: a server, a watcher.
type JobState struct {
	Command string
	Running bool
	// ExitCode is how it ended, once it has; nil while it runs, or when a signal ended it.
	ExitCode *int
	// Output is what it printed since its output was last read.
	Output string
	// Dropped is how much of that output was dropped, keeping the newest.
	Dropped int
}

// FolderListing is a folder's subfolders, to pick a project's folder by browsing, or why it cannot be read.
type FolderListing struct {
	OK      bool
	Path    string
	Parent  string
	Folders []string
	Problem string
}

// FolderMaking is a new folder made, or why not.
type FolderMaking struct {
	OK      bool
	Path    string
	Problem string
}

// Host is where a project's files are and its commands run.
type Host interface {
	// Paths is how paths are written there: POSIX, or Windows.
	Paths() Paths
	// Platform is the system, for the system prompt: linux, windows.
	Platform(ctx context.Context) (string, error)
	Home(ctx context.Context) (string, error)
	FileInfo(ctx context.Context, path string) (FileInfo, error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
	// WriteFile writes a file, its folders made first.
	WriteFile(ctx context.Context, path string, data []byte) error
	// RemoveFile deletes a file; one that is not there is no failure.
	RemoveFile(ctx context.Context, path string) error
	// RealPath is where a path really leads, links followed; for one not made yet, from its nearest folder that
	// exists.
	RealPath(ctx context.Context, path string) (string, error)
	// ListFiles is the files under a folder matching a glob, up to a limit; in a git repository, the ones git shows.
	ListFiles(ctx context.Context, base, pattern string, limit int) ([]string, error)
	Search(ctx context.Context, request SearchRequest) ([]SearchMatch, error)
	// Run runs a command, ended when ctx is done (Stopped) or its time is up (TimedOut). When options.Passwords is
	// there, what it answers goes to the command if it stops to ask for a password: only this computer's commands
	// have an input to give.
	Run(ctx context.Context, command string, options CommandOptions) CommandResult
	// ListFolders is a folder's subfolders, to pick a project's folder by browsing; ~ is the home folder.
	ListFolders(ctx context.Context, path string) (FolderListing, error)
	// MakeFolder makes a new, empty folder in one that exists, for a new project.
	MakeFolder(ctx context.Context, parent, name string) (FolderMaking, error)
	// RemoveFolder deletes a folder and all it holds; one that is not there is no failure.
	RemoveFolder(ctx context.Context, path string) error
	// ScratchFolder is a session's scratch folder there, made if needed: where the agent keeps its experiments.
	ScratchFolder(ctx context.Context, sessionID string) (string, error)
	RemoveScratch(ctx context.Context, sessionID string) error
	// StartJob starts a command that runs on after this call (a server, a watcher) in a folder: its id there.
	StartJob(ctx context.Context, command, cwd string) (string, error)
	// ReadJob is a job's state, with what it printed since its output was last taken; consume takes it. Nil for a
	// job it does not know.
	ReadJob(ctx context.Context, id string, consume bool) (*JobState, error)
	// StopJob stops a job and everything it started.
	StopJob(ctx context.Context, id string) (*JobState, error)
	// Close lets go of the host when the daemon stops: ends its worker, stops its jobs.
	Close()
}
