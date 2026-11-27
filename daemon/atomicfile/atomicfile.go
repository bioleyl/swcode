// Package atomicfile writes a data file whole or not at all: the daemon's files (a session's record, the projects,
// the workflows) are rewritten often, and one cut short by a full disk or a crash would read as no data at the next
// start.
package atomicfile

import (
	"os"
	"path/filepath"
)

// draftSuffix ends the name a file is written under before it takes its own: nothing reads a file of that name.
const draftSuffix = ".tmp"

// Write puts data in the file at path: written beside it first, then renamed over it, so a reader finds the old
// file or the new one and never a part of one. The folder must be there.
func Write(path string, data []byte, mode os.FileMode) error {
	draft, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*"+draftSuffix)
	if err != nil {
		return err
	}
	name := draft.Name()
	_, writeErr := draft.Write(data)
	closeErr := draft.Close()
	for _, failure := range []error{writeErr, closeErr, os.Chmod(name, mode)} {
		if failure != nil {
			_ = os.Remove(name)
			return failure
		}
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}
