package app

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"swcode/daemon/protocol"
)

const (
	indexPage = "index.html"
	// assetsFolder holds the files the build names after their content: a new build names them anew, so a browser
	// may keep one for good.
	assetsFolder = "assets/"
	// revalidate has the browser ask each time whether its copy still stands: the page and the files beside it keep
	// their names from one build to the next.
	revalidate = "no-cache"
)

// fileTypes are the types of the web app's files. The system's own table is not asked for them: Windows keeps
// it in the registry, where a program may have changed what a script is, and a browser runs no module sent as
// text.
var fileTypes = map[string]string{
	".css":         "text/css; charset=utf-8",
	".html":        "text/html; charset=utf-8",
	".ico":         "image/x-icon",
	".js":          "text/javascript; charset=utf-8",
	".json":        "application/json",
	".png":         "image/png",
	".svg":         "image/svg+xml",
	".webmanifest": "application/manifest+json",
	".woff2":       "font/woff2",
}

// typeOf is a file's type: the web app's own table first, the system's for anything else.
func typeOf(name string) string {
	if kind, known := fileTypes[path.Ext(name)]; known {
		return kind
	}
	return mime.TypeByExtension(path.Ext(name))
}

// encoding is a compressed copy the build leaves beside a file, and how a browser says it reads it.
type encoding struct {
	name   string
	suffix string
}

// encodings are the copies the build makes (web/vite.config.ts), the smallest first.
var encodings = []encoding{{name: "br", suffix: ".br"}, {name: "gzip", suffix: ".gz"}}

// accepts tells whether a browser reads an encoding, by its accept-encoding header.
func accepts(header, name string) bool {
	for _, each := range strings.Split(header, ",") {
		coding, quality, _ := strings.Cut(strings.TrimSpace(each), ";")
		if strings.EqualFold(strings.TrimSpace(coding), name) && strings.TrimSpace(quality) != "q=0" {
			return true
		}
	}
	return false
}

// served is a file of the web app as it travels: its smallest copy the browser reads, and that copy's encoding,
// empty for the file itself.
func served(files fs.FS, name, acceptEncoding string, plain []byte) ([]byte, string) {
	for _, each := range encodings {
		if !accepts(acceptEncoding, each.name) {
			continue
		}
		if data, err := fs.ReadFile(files, name+each.suffix); err == nil {
			return data, each.name
		}
	}
	return plain, ""
}

// etagOf names a file's content, for a browser to ask whether its copy still stands.
func etagOf(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

// cached answers a browser that already holds the file: true when nothing more is to be sent. A file named after
// its content is kept for good; any other is asked about each time, and sent again only once it changed.
func cached(response http.ResponseWriter, request *http.Request, name string, plain []byte) bool {
	if strings.HasPrefix(name, assetsFolder) {
		response.Header().Set("cache-control", keepCached)
		return false
	}
	etag := etagOf(plain)
	response.Header().Set("cache-control", revalidate)
	response.Header().Set("etag", etag)
	if request.Header.Get("if-none-match") != etag {
		return false
	}
	response.WriteHeader(http.StatusNotModified)
	return true
}

// pagePolicy is what the app's page may load and run: its own files, nothing written into the page, and nothing
// in a frame. It talks to this daemon and to the other computers its browser paired with, at their own addresses,
// and shows their pictures. Styles written into the page stay allowed: the terminal draws with them.
const pagePolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob: http: https:; font-src 'self' data:; connect-src 'self' http: https: ws: wss:; " +
	"object-src 'none'; base-uri 'none'; frame-ancestors 'none'"

// protect says what a browser may do with a file of the app: never guess its type, and for the page, never run a
// script from elsewhere nor show it in another site's frame, where a click could be led astray.
func protect(header http.Header, name string) {
	header.Set("x-content-type-options", "nosniff")
	header.Set("referrer-policy", "no-referrer")
	if name == indexPage {
		header.Set("content-security-policy", pagePolicy)
	}
}

// webApp serves the built web app, every address that is not the API answered by its page; nothing when there is
// none (the tests). A file travels compressed when the build left a copy the browser reads.
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
		if kind := typeOf(name); kind != "" {
			response.Header().Set("content-type", kind)
		}
		response.Header().Set("vary", "accept-encoding")
		protect(response.Header(), name)
		if cached(response, request, name, data) {
			return
		}
		body, encoded := served(files, name, request.Header.Get("accept-encoding"), data)
		if encoded != "" {
			response.Header().Set("content-encoding", encoded)
		}
		_, _ = response.Write(body)
	})
}
