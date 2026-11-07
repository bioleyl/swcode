// Package selfinstall puts the desktop app where Windows keeps a person's own programs when it was opened from
// anywhere else: a downloaded swcode-desktop.exe installs itself the first time it opens, and a newer one opened later
// takes the older one's place.
package selfinstall

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	programsFolder = "Programs"
	appFolder      = "swcode"
	// ProgramName is the desktop app's program, as its releases name it too.
	ProgramName = "swcode-desktop.exe"
	// The new program is written beside the old one first: a copy that fails leaves the old one whole.
	partialSuffix = ".new"
	folderMode    = 0o755
	programMode   = 0o755
)

// Target is where the app lives: %LOCALAPPDATA%\Programs\swcode\swcode-desktop.exe, as per-user installers put them.
func Target(localAppData string) string {
	return filepath.Join(localAppData, programsFolder, appFolder, ProgramName)
}

// Installed tells whether the program runs from there already: Windows paths ignore case.
func Installed(exe, target string) bool {
	return strings.EqualFold(filepath.Clean(exe), filepath.Clean(target))
}

// Copy puts the program at target, in place of an older one.
func Copy(exe, target string) error {
	data, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), folderMode); err != nil {
		return err
	}
	partial := target + partialSuffix
	if err := os.WriteFile(partial, data, programMode); err != nil {
		return err
	}
	if err := os.Rename(partial, target); err != nil {
		_ = os.Remove(partial)
		return err
	}
	return nil
}
