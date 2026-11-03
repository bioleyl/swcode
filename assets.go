// Package swcode carries what the programs hold besides code: the skills a new skills folder starts with, and the
// built web app when built with -tags embedweb.
package swcode

import (
	"embed"
	"io/fs"
)

//go:embed skills
var skillFiles embed.FS

//go:embed scripts/wsl
var windowsFiles embed.FS

//go:embed web/public/swcode.ico
var icon []byte

// iconName is the icon's name among the files for Windows.
const iconName = "swcode.ico"

// DefaultSkills is the skills a new skills folder starts with: the user's coding style and Jadis.
func DefaultSkills() fs.FS {
	skills, err := fs.Sub(skillFiles, "skills")
	if err != nil {
		panic(err)
	}
	return skills
}

// WindowsFile is a file the program carries for Windows: the shortcut's script and icon, the firewall's script.
func WindowsFile(name string) ([]byte, error) {
	if name == iconName {
		return icon, nil
	}
	return windowsFiles.ReadFile("scripts/wsl/" + name)
}
