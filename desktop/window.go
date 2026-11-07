//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/wailsapp/wails/webview2/pkg/edge"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"swcode/daemon/protocol"
	"swcode/desktop/computers"
)

const (
	windowClass = "swcode-desktop"
	// The executable's icon, as go-winres names it.
	iconResource = 1
	// The window's first size, and the smallest it gets, in pixels at 96 dpi.
	defaultWidth  = 1400
	defaultHeight = 900
	minWidth      = 380
	minHeight     = 560
	// The first size leaves this share of the screen around it, on a small one.
	screenShare = 0.9
	placeFile   = "window.json"
	// The window was maximized before it was minimized.
	wpfRestoreToMaximized = 0x0002

	wmDaemonEnded   = wmApp + 1
	wmDaemonStarted = wmApp + 2
	// A second start of the app asks the first to open its window.
	wmOpen = wmApp + 6

	computersFile = "computers.json"
	// The app told once that it stays in the notification area.
	trayHintShown = "tray-hint"
	trayHintTitle = "swcode keeps running"
	trayHintText  = "Its sessions go on, and tell you here when they need you. Right-click its icon to quit."
	sessionsPath  = "/sessions/"
	// The page's router follows the address's hash.
	routeMark = "#"
	// The window's title and the icon's tip name the other computer the window shows.
	placeSeparator = " · "

	personalizeKey = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
	lightAppsValue = "AppsUseLightTheme"
)

// theme is the page's own background, so the window does not flash another colour before it draws: tokens.css's
// --surface.
type theme struct {
	dark             bool
	red, green, blue uint8
}

var themes = map[bool]theme{
	false: {dark: false, red: 0xff, green: 0xff, blue: 0xff},
	true:  {dark: true, red: 0x0f, green: 0x11, blue: 0x13},
}

// place is where the window was last, so it opens there again: its normal bounds, in pixels.
type place struct {
	Left      int32 `json:"left"`
	Top       int32 `json:"top"`
	Right     int32 `json:"right"`
	Bottom    int32 `json:"bottom"`
	Maximized bool  `json:"maximized"`
}

// window is the app's one window, what it shows, and its icon; Win32 calls back into it through windowProc.
type window struct {
	hwnd    uintptr
	look    theme
	folders appFolders
	// The page, made the first time the window opens: an app started at logon may never need it.
	webview *edge.Chromium
	sender  *edge.ICoreWebView2
	// Minimized, the page cannot take the keyboard focus: it takes it once the window is restored.
	minimized bool
	daemon    *daemon
	// This computer's server: none while it restarts.
	current   *daemonRun
	restarted chan restart
	places    string
	computers *computers.Store
	pairings  chan added
	// The computer the page shows: this one, or another paired with the app.
	shown   string
	tray    trayIcon
	notices *noticeBoard
	// What a click on the last notification opens.
	lastNotice computerNotice
}

type restart struct {
	run *daemonRun
	err error
}

var app *window

var handlers = map[uint32]func(w *window, hwnd, wParam, lParam uintptr) uintptr{
	wmSize:            (*window).resized,
	wmMove:            (*window).moved,
	wmActivate:        (*window).activated,
	wmGetMinMaxInfo:   (*window).sizeLimits,
	wmDpiChanged:      (*window).dpiChanged,
	wmClose:           (*window).closing,
	wmQueryEndSession: (*window).endingSession,
	wmEndSession:      (*window).sessionEnds,
	wmDestroy:         (*window).destroyed,
	wmDaemonEnded:     (*window).daemonEnded,
	wmDaemonStarted:   (*window).daemonStarted,
	wmTray:            (*window).trayEvent,
	wmNotice:          (*window).noticesCame,
	wmComputerAdded:   (*window).computerAdded,
	wmOpen:            (*window).openAsked,
}

func windowProc(hwnd, message, wParam, lParam uintptr) uintptr {
	if uint32(message) == taskbarRestarted && app != nil {
		app.tray.add(hwnd)
		app.display(app.shown)
		return 0
	}
	handle, known := handlers[uint32(message)]
	if !known || app == nil {
		result, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
		return result
	}
	return handle(app, hwnd, wParam, lParam)
}

// systemTheme is Windows' own choice of light or dark for apps.
func systemTheme() theme {
	key, err := registry.OpenKey(registry.CURRENT_USER, personalizeKey, registry.QUERY_VALUE)
	if err != nil {
		return themes[false]
	}
	defer key.Close()
	light, _, err := key.GetIntegerValue(lightAppsValue)
	return themes[err == nil && light == 0]
}

// openWindow makes the window and the app's icon, and follows every computer's sessions; the window opens where it
// was last, on the computer it showed, unless the app starts in the notification area.
func openWindow(d *daemon, first *daemonRun, folders appFolders, opened bool) error {
	look := systemTheme()
	app = &window{
		computers: computers.Load(filepath.Join(folders.data, computersFile)), current: first, daemon: d,
		folders: folders, look: look, pairings: make(chan added, 1), places: filepath.Join(folders.data, placeFile),
		restarted: make(chan restart, 1),
	}
	hwnd, err := createWindow(look)
	if err != nil {
		return err
	}
	app.hwnd = hwnd
	if look.dark {
		darkTitleBar(hwnd)
	}
	app.tray.add(hwnd)
	app.display(app.computers.Current())
	app.notices = newNoticeBoard(hwnd)
	app.notices.follow(protocol.ThisComputer, first.address(), first.token())
	for _, computer := range app.computers.All() {
		app.notices.follow(computer.ID, computer.Address, computer.Token)
	}
	d.watch(first, hwnd, wmDaemonEnded)
	if !opened {
		return nil
	}
	return app.open()
}

// open opens the window: the first time, where it was last, with its page on the computer it shows.
func (w *window) open() error {
	if w.webview != nil {
		w.bringUp()
		return nil
	}
	w.show()
	webview, err := embedWebview(w.hwnd, w.folders.webview, w.look)
	if err != nil {
		return err
	}
	w.webview = webview
	w.navigate(w.shown, "")
	return nil
}

func (w *window) bringUp() {
	if isMinimized(w.hwnd) {
		showWindow(w.hwnd, swRestore)
	}
	showWindow(w.hwnd, swShow)
	_, _, _ = procSetForegroundWindow.Call(w.hwnd)
}

func (w *window) openAsked(_, _, _ uintptr) uintptr {
	w.openOrSay()
	return 0
}

// openOrSay opens the window, and says why when the page cannot be made.
func (w *window) openOrSay() {
	if err := w.open(); err != nil {
		showError(err.Error())
	}
}

// display makes a computer the one the window shows: its title and the icon's tip say which, when it is another one.
func (w *window) display(id string) {
	w.shown = id
	title := appName
	if computer, other := w.computers.Find(id); other {
		title += placeSeparator + computer.Name
	}
	_, _, _ = procSetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(utf16(title))))
	w.tray.setTip(title)
}

// showComputer opens the window on a computer's swcode, at a page of it.
func (w *window) showComputer(id, path string) {
	link, known := w.linkOf(id, path)
	if !known {
		return
	}
	w.display(id)
	_ = w.computers.SetCurrent(id)
	if w.webview == nil {
		w.openOrSay()
		return
	}
	w.bringUp()
	w.webview.Navigate(link)
}

// navigate has the page show a computer's swcode, falling back on this one's when that computer is gone.
func (w *window) navigate(id, path string) {
	link, known := w.linkOf(id, path)
	if !known {
		w.display(protocol.ThisComputer)
		link, known = w.linkOf(w.shown, path)
	}
	if known && w.webview != nil {
		w.webview.Navigate(link)
	}
}

// linkOf is the address of a page of a computer's swcode (its home without a path), with the token the page takes
// from it.
func (w *window) linkOf(id, path string) (string, bool) {
	address, token, known := w.reachOf(id)
	if !known {
		return "", false
	}
	route := ""
	if path != "" {
		route = routeMark + path
	}
	return address + "/?" + protocol.TokenParam + "=" + url.QueryEscape(token) + route, true
}

func (w *window) reachOf(id string) (string, string, bool) {
	if id != protocol.ThisComputer {
		computer, known := w.computers.Find(id)
		return computer.Address, computer.Token, known
	}
	if w.current == nil {
		return "", "", false
	}
	return w.current.address(), w.current.token(), true
}

func createWindow(look theme) (uintptr, error) {
	instance, _, _ := procGetModuleHandleW.Call(0)
	icon, _, _ := procLoadImageW.Call(instance, iconResource, imageIcon, 0, 0, lrDefaultSize|lrShared)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	colour := uintptr(look.red) | uintptr(look.green)<<8 | uintptr(look.blue)<<16
	brush, _, _ := procCreateSolidBrush.Call(colour)
	class := wndClassEx{
		wndProc:    windows.NewCallback(windowProc),
		instance:   instance,
		icon:       icon,
		cursor:     cursor,
		background: brush,
		className:  utf16(windowClass),
	}
	class.size = uint32(unsafe.Sizeof(class))
	if registered, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); registered == 0 {
		return 0, fmt.Errorf("the window class: %w", err)
	}
	width, height := firstSize()
	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16(windowClass))),
		uintptr(unsafe.Pointer(utf16(appName))),
		wsOverlappedWindow,
		cwUseDefault,
		cwUseDefault,
		uintptr(width),
		uintptr(height),
		0,
		0,
		instance,
		0,
	)
	if hwnd == 0 {
		return 0, fmt.Errorf("the window: %w", err)
	}
	return hwnd, nil
}

// firstSize is the window's size the first time: the default at the screen's scale, within the screen.
func firstSize() (int32, int32) {
	dpi, _, _ := procGetDpiForSystem.Call()
	screenWidth, _, _ := procGetSystemMetrics.Call(smCxFullscreen)
	screenHeight, _, _ := procGetSystemMetrics.Call(smCyFullscreen)
	width := min(scaled(defaultWidth, dpi), int32(float64(screenWidth)*screenShare))
	height := min(scaled(defaultHeight, dpi), int32(float64(screenHeight)*screenShare))
	return width, height
}

func darkTitleBar(hwnd uintptr) {
	enabled := int32(1)
	_, _, _ = procDwmSetWindowAttribute.Call(
		hwnd,
		dwmwaUseImmersiveDarkMode,
		uintptr(unsafe.Pointer(&enabled)),
		unsafe.Sizeof(enabled),
	)
}

// show opens the window where it was last, at the default size the first time or when that place is off screen.
func (w *window) show() {
	last, err := w.lastPlace()
	if err != nil {
		showWindow(w.hwnd, swShowNormal)
		return
	}
	shown := map[bool]uint32{false: swShowNormal, true: swShowMaximized}
	placement := windowPlacement{
		showCmd:        shown[last.Maximized],
		normalPosition: rect{left: last.Left, top: last.Top, right: last.Right, bottom: last.Bottom},
	}
	placement.length = uint32(unsafe.Sizeof(placement))
	_, _, _ = procSetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&placement)))
}

func (w *window) lastPlace() (place, error) {
	data, err := os.ReadFile(w.places)
	if err != nil {
		return place{}, err
	}
	last := place{}
	if err := json.Unmarshal(data, &last); err != nil {
		return place{}, err
	}
	bounds := rect{left: last.Left, top: last.Top, right: last.Right, bottom: last.Bottom}
	monitor, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&bounds)), monitorDefaultToNull)
	if bounds.right-bounds.left < minWidth || bounds.bottom-bounds.top < minHeight || monitor == 0 {
		return place{}, errors.New("no screen shows that place")
	}
	return last, nil
}

func (w *window) keepPlace() {
	placement := windowPlacement{}
	placement.length = uint32(unsafe.Sizeof(placement))
	if read, _, _ := procGetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&placement))); read == 0 {
		return
	}
	bounds := placement.normalPosition
	data, err := json.Marshal(place{
		Left:      bounds.left,
		Top:       bounds.top,
		Right:     bounds.right,
		Bottom:    bounds.bottom,
		Maximized: placement.showCmd == swShowMaximized || placement.flags&wpfRestoreToMaximized != 0,
	})
	if err != nil {
		return
	}
	// Next time, it opens at the default size.
	_ = os.MkdirAll(filepath.Dir(w.places), dataFolderMode)
	_ = os.WriteFile(w.places, data, dataFileMode)
}

func (w *window) resized(_, wParam, _ uintptr) uintptr {
	if w.webview == nil {
		return 0
	}
	w.webview.Resize()
	restored := w.minimized && wParam != sizeMinimized
	w.minimized = wParam == sizeMinimized
	if restored {
		w.webview.Focus()
	}
	return 0
}

func (w *window) moved(_, _, _ uintptr) uintptr {
	if w.webview != nil {
		_ = w.webview.NotifyParentWindowPositionChanged()
	}
	return 0
}

// activated gives the page the keyboard focus. WebView2 refuses it while the window is minimized (a click on its
// taskbar button activates it before restoring it) or hidden (the tray icon's menu activates the window first).
func (w *window) activated(hwnd, wParam, _ uintptr) uintptr {
	if w.webview == nil || wParam&0xffff == waInactive || isMinimized(hwnd) || !isVisible(hwnd) {
		return 0
	}
	w.webview.Focus()
	return 0
}

// Windows hands its structures to the window procedure as numbers: go vet's unsafeptr check is off for this
// package (npm run check:go vets Windows with it off).
func (w *window) sizeLimits(hwnd, _, lParam uintptr) uintptr {
	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	limits := (*minMaxInfo)(unsafe.Pointer(lParam))
	limits.minTrackSize = point{x: scaled(minWidth, dpi), y: scaled(minHeight, dpi)}
	return 0
}

// dpiChanged takes the size Windows suggests on a screen of another scale.
func (w *window) dpiChanged(hwnd, _, lParam uintptr) uintptr {
	suggested := (*rect)(unsafe.Pointer(lParam))
	_, _, _ = procSetWindowPos.Call(
		hwnd,
		0,
		uintptr(suggested.left),
		uintptr(suggested.top),
		uintptr(suggested.right-suggested.left),
		uintptr(suggested.bottom-suggested.top),
		swpNoZOrder|swpNoActivate,
	)
	return 0
}

// closing hides the window: the app stays in the notification area, its sessions working. It says so the first
// time.
func (w *window) closing(_, _, _ uintptr) uintptr {
	w.keepPlace()
	showWindow(w.hwnd, swHide)
	hint := filepath.Join(w.folders.data, trayHintShown)
	if _, err := os.Stat(hint); err == nil {
		return 0
	}
	w.tray.notify(trayHintTitle, trayHintText)
	_ = os.MkdirAll(w.folders.data, dataFolderMode)
	_ = os.WriteFile(hint, nil, dataFileMode)
	return 0
}

// quit ends the app, the server run here with it, asking first while it works: the daemon alone keeps running.
func (w *window) quit() {
	if w.current != nil && !w.current.attached && !w.quitAnyway() {
		return
	}
	if isVisible(w.hwnd) {
		w.keepPlace()
	}
	showWindow(w.hwnd, swHide)
	w.end()
}

// end stops following the computers and the server run here, and takes the icon away with the window.
func (w *window) end() {
	w.notices.stopAll()
	if w.current != nil {
		w.daemon.stop(w.current)
	}
	w.tray.remove()
	_, _, _ = procDestroyWindow.Call(w.hwnd)
}

func (w *window) quitAnyway() bool {
	warning := w.current.closingWarning()
	if warning == "" {
		return true
	}
	text := fmt.Sprintf("swcode is still working.\n\n%s\n\nQuit anyway?", warning)
	return messageBox(w.hwnd, text, mbYesNo|mbIconWarning|mbDefButton2|mbSetForeground) == idYes
}

// endingSession lets Windows log off or shut down: the server stops when it does.
func (w *window) endingSession(_, _, _ uintptr) uintptr {
	return 1
}

func (w *window) sessionEnds(_, wParam, _ uintptr) uintptr {
	if wParam == 0 {
		return 0
	}
	if isVisible(w.hwnd) {
		w.keepPlace()
	}
	w.notices.stopAll()
	w.tray.remove()
	if w.current != nil {
		w.daemon.stop(w.current)
	}
	return 0
}

func (w *window) destroyed(_, _, _ uintptr) uintptr {
	_, _, _ = procPostQuitMessage.Call(0)
	return 0
}

// daemonEnded: the server ended by itself. On new settings it starts again; for a newer version, the new program
// starts in place of this one; asked to stop from elsewhere, the window closes with it; a failure is said first.
func (w *window) daemonEnded(_, _, _ uintptr) uintptr {
	ended := w.current
	w.current = nil
	if ended.restarts() {
		go w.startAgain()
		return 0
	}
	if ended.updated() {
		w.startUpdated()
		return 0
	}
	if err := ended.problem(); err != nil {
		showError(fmt.Sprintf("swcode stopped.\n\n%s", err))
	}
	if isVisible(w.hwnd) {
		w.keepPlace()
	}
	w.end()
	return 0
}

// startAgain runs on its own goroutine: the window stays responsive while the server starts.
func (w *window) startAgain() {
	started, err := w.daemon.start()
	w.restarted <- restart{run: started, err: err}
	postMessage(w.hwnd, wmDaemonStarted, 0)
}

// daemonStarted: the server listens again, maybe at another address: the page opens there.
func (w *window) daemonStarted(_, _, _ uintptr) uintptr {
	again := <-w.restarted
	if again.err != nil {
		showError(fmt.Sprintf("swcode did not start again.\n\n%s", again.err))
		w.end()
		return 0
	}
	w.current = again.run
	w.daemon.watch(again.run, w.hwnd, wmDaemonEnded)
	w.notices.follow(protocol.ThisComputer, again.run.address(), again.run.token())
	if w.shown == protocol.ThisComputer {
		w.navigate(w.shown, "")
	}
	return 0
}

// noticesCame shows the notices that came as notifications, but those of the page the person looks at.
func (w *window) noticesCame(_, _, _ uintptr) uintptr {
	for _, came := range w.notices.take() {
		if w.sees(came.computer) {
			continue
		}
		w.lastNotice = came
		w.tray.notify(w.noticeTitle(came), came.notice.Text)
	}
	return 0
}

// sees tells whether the person looks at a computer's page: the window in front, showing it.
func (w *window) sees(computer string) bool {
	foreground, _, _ := procGetForegroundWindow.Call()
	return foreground == w.hwnd && isVisible(w.hwnd) && w.shown == computer
}

// noticeTitle names what happened and the session, and the computer when it is another one.
func (w *window) noticeTitle(came computerNotice) string {
	title := noticeTitles[came.notice.Kind]
	if computer, other := w.computers.Find(came.computer); other {
		title += " on " + computer.Name
	}
	return title + ": " + came.notice.Title
}

// openNotice opens the session of the last notification.
func (w *window) openNotice() {
	if w.lastNotice.notice.SessionID == "" {
		w.openOrSay()
		return
	}
	w.showComputer(w.lastNotice.computer, sessionsPath+url.PathEscape(w.lastNotice.notice.SessionID))
}

// messageLoop runs the window until it is gone.
func messageLoop() {
	message := msg{}
	for {
		got, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(got) <= 0 {
			return
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
}
