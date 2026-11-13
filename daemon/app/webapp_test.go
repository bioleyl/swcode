package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// builtApp is a web app as the build leaves it: the page, a file named after its content with its compressed
// copies, and one the build left no copy of.
var builtApp = fstest.MapFS{
	"index.html":             {Data: []byte("<!doctype html><title>swcode</title>")},
	"assets/app-1a2b.js":     {Data: []byte("console.log('the app')")},
	"assets/app-1a2b.js.br":  {Data: []byte("brotli")},
	"assets/app-1a2b.js.gz":  {Data: []byte("gzip")},
	"assets/font-3c4d.woff2": {Data: []byte("a font")},
}

func getFile(t *testing.T, address string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, address, nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	webApp(builtApp).ServeHTTP(response, request)
	return response
}

func TestAFileNamedAfterItsContentTravelsAsTheSmallestCopyTheBrowserReadsAndIsKept(t *testing.T) {
	copies := map[string][2]string{
		"gzip, deflate, br": {"br", "brotli"},
		"gzip, deflate":     {"gzip", "gzip"},
		"br;q=0, gzip":      {"gzip", "gzip"},
		"":                  {"", "console.log('the app')"},
	}
	for accepted, expected := range copies {
		response := getFile(t, "/assets/app-1a2b.js", map[string]string{"accept-encoding": accepted})
		if response.Header().Get("content-encoding") != expected[0] || response.Body.String() != expected[1] {
			t.Fatalf("accepting %q: %q, encoded %q", accepted, response.Body.String(), response.Header().Get("content-encoding"))
		}
		if response.Header().Get("cache-control") != keepCached || response.Header().Get("vary") != "accept-encoding" {
			t.Fatalf("accepting %q: cache-control %q, vary %q", accepted, response.Header().Get("cache-control"), response.Header().Get("vary"))
		}
	}
	font := getFile(t, "/assets/font-3c4d.woff2", map[string]string{"accept-encoding": "br"})
	if font.Header().Get("content-encoding") != "" || font.Body.String() != "a font" {
		t.Fatalf("a file without a copy: %q, encoded %q", font.Body.String(), font.Header().Get("content-encoding"))
	}
}

func TestThePageIsAskedAboutEachTimeAndSentAgainOnlyOnceItChanged(t *testing.T) {
	for _, address := range []string{"/", "/sessions/abc", "/index.html"} {
		first := getFile(t, address, nil)
		etag := first.Header().Get("etag")
		if first.Code != http.StatusOK || etag == "" || first.Header().Get("cache-control") != revalidate {
			t.Fatalf("%s: status %d, etag %q, cache-control %q", address, first.Code, etag, first.Header().Get("cache-control"))
		}
		again := getFile(t, address, map[string]string{"if-none-match": etag})
		if again.Code != http.StatusNotModified || again.Body.Len() != 0 {
			t.Fatalf("%s with its etag: status %d, %d bytes", address, again.Code, again.Body.Len())
		}
		if changed := getFile(t, address, map[string]string{"if-none-match": `"another"`}); changed.Code != http.StatusOK {
			t.Fatalf("%s with another etag: status %d", address, changed.Code)
		}
	}
}

func TestThePageSaysWhatItMayLoadAndNoFileLetsItsTypeBeGuessed(t *testing.T) {
	page := getFile(t, "/sessions/abc", nil)
	policy := page.Header().Get("content-security-policy")
	for _, rule := range []string{"script-src 'self'", "frame-ancestors 'none'", "object-src 'none'"} {
		if !strings.Contains(policy, rule) {
			t.Fatalf("the page's policy lacks %q: %q", rule, policy)
		}
	}
	script := getFile(t, "/assets/app-1a2b.js", nil)
	if page.Header().Get("x-content-type-options") != "nosniff" || script.Header().Get("x-content-type-options") != "nosniff" {
		t.Fatal("a file's type may be guessed")
	}
	if script.Header().Get("content-security-policy") != "" {
		t.Fatal("a script carries the page's policy")
	}
}
