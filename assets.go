// Package swcode carries what the programs hold besides code: the files for Windows, and the built web app when
// built with -tags embedweb.
package swcode

import (
	"embed"
	"io/fs"
)

//go:embed scripts/wsl
var windowsFiles embed.FS

//go:embed all:skills
var swcodeBase embed.FS

// SwcodeBaseSkills is the swcode-base skills the program carries, as a folder of skills each with its SKILL.md; the app
// offers them to a person whose machine has none.
func SwcodeBaseSkills() fs.FS {
	skills, err := fs.Sub(swcodeBase, "skills")
	if err != nil {
		panic(err)
	}
	return skills
}

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
