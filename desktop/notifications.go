//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// notifications is what the app keeps of the tray's notifications: on, until the person turns them off.
type notifications struct {
	On bool `json:"on"`
}

// loadNotifications reads the app's answer, on when it never was asked.
func loadNotifications(file string) bool {
	data, err := os.ReadFile(file)
	if err != nil {
		return true
	}
	var kept notifications
	if json.Unmarshal(data, &kept) != nil {
		return true
	}
	return kept.On
}

// saveNotifications keeps the app's answer, so the icon's menu and the page's section say the same on the next start.
func saveNotifications(file string, on bool) {
	data, err := json.Marshal(notifications{On: on})
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	_ = os.WriteFile(file, data, 0o600)
}
