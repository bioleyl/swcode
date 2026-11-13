// Package checkpoints keeps the files a turn changes as they were before it changed them, so a person can put them
// back: those its file tools write, copied before each first write, and in a git repository those its commands
// change, found by looking at the workspace before the turn and after it.
package checkpoints

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"swcode/daemon/hosts"
)

const (
	manifestName = "manifest.json"
	// keptTurns is how many restore points of a session are kept: older turns' files go as new turns come.
	keptTurns = 10
	ownerOnly = 0o700
	fileOnly  = 0o600
	// maxKeptBytes is the largest file a restore point keeps; a bigger one is left out, and the log says so.
	maxKeptBytes = 10 << 20
	blobFormat   = "file-%d"
	missingPath  = ".."
)

// ErrNoCheckpoint is a restore point that is not there: pruned, or from a turn that changed no file.
var ErrNoCheckpoint = errors.New("That restore point is gone: only the last turns' are kept.")

// KeptFile is a file a turn changed: where it is from the workspace, and the copy of it from before, when it existed.
type KeptFile struct {
	Existed bool   `json:"existed"`
	Path    string `json:"path"`
	Stored  string `json:"stored,omitempty"`
}

// Manifest is a turn's restore point, as its folder keeps it.
type Manifest struct {
	Files []KeptFile `json:"files"`
}

// Store keeps the restore points, a folder per session and turn.
type Store struct {
	folder   string
	logError func(error)
}

func NewStore(folder string, logError func(error)) *Store {
	return &Store{folder: folder, logError: logError}
}

// Recorder takes the files a turn changes, once each, into its restore point.
type Recorder struct {
	store     *Store
	host      hosts.Host
	sessionID string
	turnID    string
	workspace string
	mu        sync.Mutex
	kept      map[string]bool
	files     []KeptFile
	closed    bool
	// The turn's commands are watched through git: scratch is the session's scratch folder on the host, looked is
	// closed once the workspace was looked at first, and before is its tree then; empty where git has none to give.
	scratch string
	looked  chan struct{}
	before  string
}

// Begin takes the restore point of one turn: files outside the workspace, and the scratch folder's, are not kept.
func (s *Store) Begin(host hosts.Host, sessionID, turnID, workspace string) *Recorder {
	return &Recorder{
		store: s, host: host, sessionID: sessionID, turnID: turnID, workspace: workspace, kept: map[string]bool{},
	}
}

// Host is the turn's host, with its files copied into the restore point before each write.
func (r *Recorder) Host() hosts.Host { return recordingHost{Host: r.host, recorder: r} }

type recordingHost struct {
	hosts.Host
	recorder *Recorder
}

func (h recordingHost) WriteFile(ctx context.Context, path string, data []byte) error {
	h.recorder.keep(ctx, path)
	return h.Host.WriteFile(ctx, path, data)
}

// Close writes the restore point, keeps the session's newest ones, and says which files it holds.
func (r *Recorder) Close() ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.paths(), nil
	}
	r.closed = true
	r.keepCommandFiles()
	if len(r.files) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(r.folder(), ownerOnly); err != nil {
		return nil, err
	}
	data, err := json.Marshal(Manifest{Files: r.files})
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(r.folder(), manifestName), data, fileOnly); err != nil {
		return nil, err
	}
	// The folder's own time says how new its restore point is, for the ones kept.
	now := time.Now()
	_ = os.Chtimes(r.folder(), now, now)
	r.store.prune(r.sessionID)
	return r.paths(), nil
}

// keep copies a file as it is now, once: later writes to it keep the version from before the turn's first write.
func (r *Recorder) keep(ctx context.Context, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.kept[path] || r.workspace == "" {
		return
	}
	paths := r.host.Paths()
	if !inside(paths, r.workspace, path) {
		return
	}
	relative := paths.Rel(r.workspace, path)
	if relative == "" || relative == "." {
		return
	}
	info, err := r.host.FileInfo(ctx, path)
	if err != nil {
		r.store.log(err)
		return
	}
	if info.Kind == hosts.KindDirectory {
		return
	}
	kept := KeptFile{Existed: info.Kind == hosts.KindFile, Path: relative}
	if kept.Existed {
		if info.Size > maxKeptBytes {
			r.store.log(fmt.Errorf("%s is larger than %d bytes: it is not kept for a restore", relative, maxKeptBytes))
			return
		}
		data, err := r.host.ReadFile(ctx, path)
		if err != nil {
			r.store.log(err)
			return
		}
		kept.Stored = fmt.Sprintf(blobFormat, len(r.files))
		if err := os.MkdirAll(r.folder(), ownerOnly); err != nil {
			r.store.log(err)
			return
		}
		if err := os.WriteFile(filepath.Join(r.folder(), kept.Stored), data, fileOnly); err != nil {
			r.store.log(err)
			return
		}
	}
	r.kept[path] = true
	r.files = append(r.files, kept)
}

// Snapshot is a file of a restore point: where it is from the workspace, and what it held before the turn.
type Snapshot struct {
	Existed bool
	Path    string
	Data    []byte
}

// Snapshot is a turn's restore point: its files as they were before it, ErrNoCheckpoint when it is gone.
func (s *Store) Snapshot(sessionID, turnID string) ([]Snapshot, error) {
	folder := filepath.Join(s.folder, sessionID, turnID)
	data, err := os.ReadFile(filepath.Join(folder, manifestName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoCheckpoint
	}
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	snapshots := make([]Snapshot, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		snapshot := Snapshot{Existed: file.Existed, Path: file.Path}
		if file.Existed {
			if file.Stored == "" || filepath.Base(file.Stored) != file.Stored {
				return nil, fmt.Errorf("%s has no copy to show", file.Path)
			}
			before, err := os.ReadFile(filepath.Join(folder, file.Stored))
			if err != nil {
				return nil, err
			}
			snapshot.Data = before
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

// Restore puts a turn's files back as they were before it, and deletes those it made.
func (s *Store) Restore(ctx context.Context, host hosts.Host, sessionID, turnID, workspace string) ([]string, error) {
	folder := filepath.Join(s.folder, sessionID, turnID)
	data, err := os.ReadFile(filepath.Join(folder, manifestName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoCheckpoint
	}
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	paths := host.Paths()
	root, err := host.RealPath(ctx, workspace)
	if err != nil {
		return nil, err
	}
	restored := []string{}
	for _, file := range manifest.Files {
		path := paths.Resolve(workspace, file.Path)
		if path == workspace || !inside(paths, workspace, path) {
			return restored, fmt.Errorf("%s is outside the workspace: not restored", file.Path)
		}
		real, err := host.RealPath(ctx, path)
		if err != nil {
			return restored, err
		}
		if real == root || !inside(paths, root, real) {
			return restored, fmt.Errorf("%s leads outside the workspace through a link: not restored", file.Path)
		}
		if !file.Existed {
			if err := host.RemoveFile(ctx, path); err != nil {
				return restored, err
			}
			restored = append(restored, file.Path)
			continue
		}
		if file.Stored == "" || filepath.Base(file.Stored) != file.Stored {
			return restored, fmt.Errorf("%s has no copy to restore", file.Path)
		}
		data, err := os.ReadFile(filepath.Join(folder, file.Stored))
		if err != nil {
			return restored, err
		}
		if err := host.WriteFile(ctx, path, data); err != nil {
			return restored, err
		}
		restored = append(restored, file.Path)
	}
	return restored, nil
}

// Remove deletes a session's restore points, with the session.
func (s *Store) Remove(sessionID string) {
	if err := os.RemoveAll(filepath.Join(s.folder, sessionID)); err != nil {
		s.log(err)
	}
}

// prune keeps a session's newest restore points, dropping the others.
func (s *Store) prune(sessionID string) {
	folder := filepath.Join(s.folder, sessionID)
	entries, err := os.ReadDir(folder)
	if err != nil {
		return
	}
	type turn struct {
		path string
		at   time.Time
	}
	turns := make([]turn, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		turns = append(turns, turn{path: filepath.Join(folder, entry.Name()), at: info.ModTime()})
	}
	sort.Slice(turns, func(first, second int) bool { return turns[first].at.After(turns[second].at) })
	for _, old := range turns[min(keptTurns, len(turns)):] {
		if err := os.RemoveAll(old.path); err != nil {
			s.log(err)
		}
	}
}

func (r *Recorder) folder() string {
	return filepath.Join(r.store.folder, r.sessionID, r.turnID)
}

func (r *Recorder) paths() []string {
	files := make([]string, 0, len(r.files))
	for _, file := range r.files {
		files = append(files, file.Path)
	}
	return files
}

func (s *Store) log(err error) {
	if s.logError != nil {
		s.logError(err)
	}
}

// inside tells whether a path is the root or under it, in the host's path style.
func inside(paths hosts.Paths, root, path string) bool {
	fromRoot := paths.Rel(root, path)
	return !paths.IsAbs(fromRoot) && fromRoot != missingPath && !strings.HasPrefix(fromRoot, missingPath+paths.Separator())
}
