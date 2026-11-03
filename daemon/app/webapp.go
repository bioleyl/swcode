package app

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"swcode/daemon/protocol"
)

const indexPage = "index.html"

// webApp serves the built web app, every address that is not the API answered by its page; nothing when there is
// none (the tests).
func webApp(files fs.FS) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || strings.HasPrefix(request.URL.Path, protocol.APIPrefix) || files == nil {
			fail(response, http.StatusNotFound, "Not found.")
			return
		}
		name := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
		data, err := fs.ReadFile(files, name)
		if name == "" || err != nil {
			name = indexPage
			if data, err = fs.ReadFile(files, indexPage); err != nil {
				fail(response, http.StatusNotFound, "Not found.")
				return
			}
		}
		if kind := mime.TypeByExtension(path.Ext(name)); kind != "" {
			response.Header().Set("content-type", kind)
		}
		_, _ = response.Write(data)
	})
}
