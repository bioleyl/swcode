//go:build windows

package main

import (
	"errors"
	"strings"

	"golang.org/x/sys/windows/registry"

	"swcode/daemon/protocol"
)

const (
	runKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue = "swcode"
)

// startupFlags is what each way to start adds to the program's line; off has no line.
var startupFlags = map[protocol.StartupMode]string{
	protocol.StartupModeTray:   flagMinimized,
	protocol.StartupModeDaemon: flagDaemon,
}

// startupEntry is the app's line among the person's startup programs (HKCU's Run key): Windows starts the app in
// the tray, or the daemon alone, when they log in. It is the daemon's StartupPort, which the settings' Startup topic
// changes.
type startupEntry struct {
	exe string
}

func (s startupEntry) Mode() protocol.StartupMode {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return protocol.StartupModeOff
	}
	defer key.Close()
	line, _, err := key.GetStringValue(runValue)
	if err != nil {
		return protocol.StartupModeOff
	}
	for mode, flag := range startupFlags {
		if strings.HasSuffix(line, " "+flag) {
			return mode
		}
	}
	return protocol.StartupModeOff
}

// SetMode writes the line for this program where it is now, or takes it out.
func (s startupEntry) SetMode(mode protocol.StartupMode) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	flag, starts := startupFlags[mode]
	if !starts {
		return withoutMissing(key.DeleteValue(runValue))
	}
	return key.SetStringValue(runValue, `"`+s.exe+`" `+flag)
}

func withoutMissing(err error) error {
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}
