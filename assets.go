// Package swcode carries what the programs hold besides code: the files for Windows, and the built web app when
// built with -tags embedweb.
package swcode

import (
	"embed"
)

//go:embed scripts/wsl
var windowsFiles embed.FS

//go:embed web/public/swcode.ico
var icon []byte

// iconName is the icon's name among the files for Windows.
const iconName = "swcode.ico"

// WindowsFile is a file the program carries for Windows: the shortcut's script and icon, the firewall's script.
func WindowsFile(name string) ([]byte, error) {
	if name == iconName {
		return icon, nil
	}
	return windowsFiles.ReadFile("scripts/wsl/" + name)
}
