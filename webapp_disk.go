//go:build !embedweb

package swcode

import "io/fs"

// WebApp is nil when the program does not carry the web app: it is served from the disk (dist/web, or
// SWCODE_WEB_DIR).
func WebApp() fs.FS {
	return nil
}
