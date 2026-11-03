//go:build embedweb

package swcode

import (
	"embed"
	"io/fs"
)

//go:embed all:dist/web
var webFiles embed.FS

// WebApp is the built web app inside the program (npm run build first).
func WebApp() fs.FS {
	web, err := fs.Sub(webFiles, "dist/web")
	if err != nil {
		panic(err)
	}
	return web
}
