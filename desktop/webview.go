//go:build windows

package main

import (
	_ "embed"
	"errors"
	"net/url"
	"strings"

	"github.com/wailsapp/wails/webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

//go:embed outside-links.js
var outsideLinks string

// The pages a link may open in the person's browser.
var outsideSchemes = map[string]bool{"http": true, "https": true}

// Windows' own opener of a link in the default browser: the link goes to it as it is, commas included.
var linkOpener = []string{"rundll32.exe", "url.dll,FileProtocolHandler"}

// embedWebview puts Edge's WebView2 in the window, on its own profile, in the page's colours.
func embedWebview(hwnd uintptr, folder string, look theme) (*edge.Chromium, error) {
	chromium := edge.NewChromium()
	chromium.DataPath = folder
	chromium.SetErrorCallback(func(err error) { showError(err.Error()) })
	chromium.MessageCallback = func(message string, sender *edge.ICoreWebView2, _ *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		app.pageMessage(message, sender)
	}
	// Pictures pasted in the composer.
	chromium.SetPermission(edge.CoreWebView2PermissionKindClipboardRead, edge.CoreWebView2PermissionStateAllow)
	if !chromium.Embed(hwnd) {
		return nil, errors.New("Edge's WebView2 did not start")
	}
	chromium.SetBackgroundColour(look.red, look.green, look.blue, opaque)
	// No address at the bottom when the pointer is on a link: it is an app, not a browser.
	if settings, err := chromium.GetSettings(); err == nil {
		_ = settings.PutIsStatusBarEnabled(false)
	}
	chromium.Init(outsideLinks)
	chromium.Resize()
	return chromium, nil
}

// openOutside opens a page of another site in the person's browser, started out of this program's job: a browser
// it starts stays open once swcode closes. When WMI fails, ShellExecute starts it in the job. The window waits for
// neither.
func openOutside(address string) {
	link, err := url.Parse(address)
	if err != nil || !outsideSchemes[link.Scheme] {
		return
	}
	go func() {
		if startOutside(strings.Join(append(append([]string{}, linkOpener...), link.String()), " ")) == nil {
			return
		}
		_ = windows.ShellExecute(0, utf16("open"), utf16(link.String()), nil, nil, windows.SW_SHOWNORMAL)
	}()
}
