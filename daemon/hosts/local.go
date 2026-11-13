package hosts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

// LocalOptions is what this computer's host needs.
type LocalOptions struct {
	// Helpers is where the daemon writes the programs its commands run: the password prompt sudo -A and ssh ask
	// with, in the data folder. Empty, sudo and ssh cannot ask, though a command reading its input still can.
	Helpers string
	Home    string
	// ScratchRoot is where the sessions' scratch folders go: in the data folder.
	ScratchRoot string
	// Shell is what commands run with: bash, unless said (Git Bash or PowerShell on Windows).
	Shell *Shell
}

// LocalHost is this computer: the files on its disk, the commands in its shell.
type LocalHost struct {
	options LocalOptions
	shell   Shell
	jobs    *Jobs
	// askpass is the password prompt's path, written the first time a command may ask: empty when it could not be.
	askpass func() string
}

func NewLocalHost(options LocalOptions) *LocalHost {
	shell := Bash
	if options.Shell != nil {
		shell = *options.Shell
	}
	askpass := sync.OnceValue(func() string { return WriteAskpass(options.Helpers) })
	return &LocalHost{askpass: askpass, jobs: NewJobs(shell), options: options, shell: shell}
}

func (h *LocalHost) Paths() Paths { return NativePaths }

// Platform is the system, and the shell when it is not plain bash: the model writes its commands for it.
func (h *LocalHost) Platform(context.Context) (string, error) {
	if h.shell.Label == "" {
		return runtime.GOOS, nil
	}
	return runtime.GOOS + ", " + h.shell.Label, nil
}

func (h *LocalHost) Home(context.Context) (string, error) { return h.options.Home, nil }

func (h *LocalHost) FileInfo(_ context.Context, path string) (FileInfo, error) {
	return localFileInfo(path)
}

func (h *LocalHost) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (h *LocalHost) WriteFile(_ context.Context, path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (h *LocalHost) RemoveFile(_ context.Context, path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (h *LocalHost) RealPath(_ context.Context, path string) (string, error) {
	return localRealPath(path)
}

func (h *LocalHost) ListFiles(ctx context.Context, base, pattern string, limit int) ([]string, error) {
	return localListFiles(ctx, base, pattern, limit)
}

func (h *LocalHost) Search(ctx context.Context, request SearchRequest) ([]SearchMatch, error) {
	return localSearch(ctx, request)
}

func (h *LocalHost) Run(ctx context.Context, command string, options CommandOptions) CommandResult {
	if options.Passwords == nil {
		return RunCommand(ctx, command, options, h.shell)
	}
	return RunCommandAsks(ctx, command, options, h.shell, h.askpass())
}

func (h *LocalHost) ListFolders(_ context.Context, path string) (FolderListing, error) {
	return localListFolders(path, h.options.Home), nil
}

func (h *LocalHost) MakeFolder(_ context.Context, parent, name string) (FolderMaking, error) {
	return localMakeFolder(parent, name, h.options.Home), nil
}

func (h *LocalHost) RemoveFolder(_ context.Context, path string) error {
	return os.RemoveAll(path)
}

func (h *LocalHost) ScratchFolder(_ context.Context, sessionID string) (string, error) {
	folder := filepath.Join(h.options.ScratchRoot, sessionID)
	return folder, os.MkdirAll(folder, 0o755)
}

func (h *LocalHost) RemoveScratch(_ context.Context, sessionID string) error {
	return os.RemoveAll(filepath.Join(h.options.ScratchRoot, sessionID))
}

func (h *LocalHost) StartJob(_ context.Context, command, cwd string) (string, error) {
	return h.jobs.Start(command, cwd), nil
}

// StartTerminal starts this computer's shell in the folder, on a pseudo-terminal of its own.
func (h *LocalHost) StartTerminal(options TerminalOptions) (*Terminal, error) {
	command := exec.Command(h.shell.Program, h.shell.Interactive...)
	command.Dir = options.Cwd
	return StartTerminal(command, options, h.shell)
}

func (h *LocalHost) ReadJob(_ context.Context, id string, consume bool) (*JobState, error) {
	return h.jobs.Read(id, consume), nil
}

func (h *LocalHost) StopJob(_ context.Context, id string) (*JobState, error) {
	return h.jobs.Stop(id), nil
}

func (h *LocalHost) Close() { h.jobs.StopAll() }
