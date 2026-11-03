//go:build windows

package main

import (
	"os"
	"path/filepath"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
)

const (
	shortcutName = "swcode.lnk"
	// The app made the desktop's shortcut once: removed, it stays removed.
	desktopShortcutMade = "desktop-shortcut"
)

// keepShortcuts makes the Start menu's shortcut at each start, so it opens this program; the desktop's, the first
// time only.
func keepShortcuts(exe string, folders appFolders) {
	if programs, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0); err == nil {
		start := filepath.Join(programs, shortcutName)
		// A new one: what an earlier app wrote in it (its own app ID) would keep the taskbar from grouping it.
		_ = os.Remove(start)
		_ = makeShortcut(start, exe)
	}
	made := filepath.Join(folders.data, desktopShortcutMade)
	if _, err := os.Stat(made); err == nil {
		return
	}
	desktop, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
	if err != nil || makeShortcut(filepath.Join(desktop, shortcutName), exe) != nil {
		return
	}
	_ = os.MkdirAll(folders.data, dataFolderMode)
	_ = os.WriteFile(made, nil, dataFileMode)
}

// makeShortcut writes a shortcut to the program, with its icon, through Windows' scripting shell.
func makeShortcut(path, target string) error {
	object, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return err
	}
	defer object.Release()
	shell, err := object.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer shell.Release()
	created, err := oleutil.CallMethod(shell, "CreateShortcut", path)
	if err != nil {
		return err
	}
	link := created.ToIDispatch()
	defer link.Release()
	properties := map[string]string{
		"Description":      appName,
		"IconLocation":     target + ",0",
		"TargetPath":       target,
		"WorkingDirectory": filepath.Dir(target),
	}
	for name, value := range properties {
		if _, err := oleutil.PutProperty(link, name, value); err != nil {
			return err
		}
	}
	_, err = oleutil.CallMethod(link, "Save")
	return err
}
