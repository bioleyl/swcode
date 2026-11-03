//go:build windows

package main

import "golang.org/x/sys/windows"

// The Win32 calls golang.org/x/sys/windows does not have.
var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")

	procRegisterClassExW      = user32.NewProc("RegisterClassExW")
	procCreateWindowExW       = user32.NewProc("CreateWindowExW")
	procDefWindowProcW        = user32.NewProc("DefWindowProcW")
	procDestroyWindow         = user32.NewProc("DestroyWindow")
	procShowWindow            = user32.NewProc("ShowWindow")
	procGetMessageW           = user32.NewProc("GetMessageW")
	procTranslateMessage      = user32.NewProc("TranslateMessage")
	procDispatchMessageW      = user32.NewProc("DispatchMessageW")
	procPostQuitMessage       = user32.NewProc("PostQuitMessage")
	procPostMessageW          = user32.NewProc("PostMessageW")
	procLoadImageW            = user32.NewProc("LoadImageW")
	procLoadCursorW           = user32.NewProc("LoadCursorW")
	procFindWindowW           = user32.NewProc("FindWindowW")
	procSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	procIsIconic              = user32.NewProc("IsIconic")
	procGetWindowPlacement    = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement    = user32.NewProc("SetWindowPlacement")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
	procMonitorFromRect       = user32.NewProc("MonitorFromRect")
	procGetDpiForWindow       = user32.NewProc("GetDpiForWindow")
	procGetDpiForSystem       = user32.NewProc("GetDpiForSystem")
	procGetSystemMetrics      = user32.NewProc("GetSystemMetrics")
	procCreateSolidBrush      = gdi32.NewProc("CreateSolidBrush")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	procGetModuleHandleW      = kernel32.NewProc("GetModuleHandleW")

	procShellNotifyIconW         = shell32.NewProc("Shell_NotifyIconW")
	procCreatePopupMenu          = user32.NewProc("CreatePopupMenu")
	procAppendMenuW              = user32.NewProc("AppendMenuW")
	procSetMenuDefaultItem       = user32.NewProc("SetMenuDefaultItem")
	procTrackPopupMenu           = user32.NewProc("TrackPopupMenu")
	procDestroyMenu              = user32.NewProc("DestroyMenu")
	procGetCursorPos             = user32.NewProc("GetCursorPos")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procRegisterWindowMessageW   = user32.NewProc("RegisterWindowMessageW")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procSetWindowTextW           = user32.NewProc("SetWindowTextW")
)

const (
	wmDestroy         = 0x0002
	wmMove            = 0x0003
	wmSize            = 0x0005
	wmActivate        = 0x0006
	wmClose           = 0x0010
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	wmGetMinMaxInfo   = 0x0024
	wmDpiChanged      = 0x02E0
	wmApp             = 0x8000
	wmNull            = 0x0000
	wmContextMenu     = 0x007B

	waInactive         = 0
	sizeMinimized      = 1
	wsOverlappedWindow = 0x00CF0000
	cwUseDefault       = 0x80000000

	swHide          = 0
	swShowNormal    = 1
	swShowMaximized = 3
	swShow          = 5
	swRestore       = 9

	imageIcon     = 1
	lrDefaultSize = 0x00000040
	lrShared      = 0x00008000
	idcArrow      = 32512

	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010

	smCxFullscreen = 16
	smCyFullscreen = 17
	smCxSmIcon     = 49
	smCySmIcon     = 50

	mfString       = 0x0000
	mfChecked      = 0x0008
	mfSeparator    = 0x0800
	tpmRightButton = 0x0002
	tpmNoNotify    = 0x0080
	tpmReturnCmd   = 0x0100

	monitorDefaultToNull      = 0
	dwmwaUseImmersiveDarkMode = 20
	defaultDpi                = 96

	mbYesNo           = 0x00000004
	mbIconError       = 0x00000010
	mbIconWarning     = 0x00000030
	mbIconInformation = 0x00000040
	mbDefButton2      = 0x00000100
	mbSetForeground   = 0x00010000
	idYes             = 6
)

type point struct{ x, y int32 }

type rect struct{ left, top, right, bottom int32 }

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	private uint32
}

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	iconSm     uintptr
}

type windowPlacement struct {
	length         uint32
	flags          uint32
	showCmd        uint32
	minPosition    point
	maxPosition    point
	normalPosition rect
}

type minMaxInfo struct {
	reserved     point
	maxSize      point
	maxPosition  point
	minTrackSize point
	maxTrackSize point
}

func utf16(text string) *uint16 {
	pointer, _ := windows.UTF16PtrFromString(text)
	return pointer
}

func postMessage(hwnd uintptr, message uint32, wParam uintptr) {
	_, _, _ = procPostMessageW.Call(hwnd, uintptr(message), wParam, 0)
}

func showWindow(hwnd uintptr, command int) {
	_, _, _ = procShowWindow.Call(hwnd, uintptr(command))
}

// scaled turns a size in pixels at 96 dpi into the window's own.
func scaled(size int32, dpi uintptr) int32 {
	return size * int32(dpi) / defaultDpi
}

func messageBox(hwnd uintptr, text string, style uint32) int32 {
	answer, _ := windows.MessageBox(windows.HWND(hwnd), utf16(text), utf16(appName), style)
	return answer
}

func showError(text string) {
	messageBox(0, text, mbIconError|mbSetForeground)
}

// fill copies a text into a fixed UTF-16 field of a Win32 structure, cut to fit with its terminating zero.
func fill(field []uint16, text string) {
	encoded := windows.StringToUTF16(text)
	if len(encoded) > len(field) {
		encoded = append(encoded[:len(field)-1], 0)
	}
	copy(field, encoded)
}

func isVisible(hwnd uintptr) bool {
	visible, _, _ := procIsWindowVisible.Call(hwnd)
	return visible != 0
}

func isMinimized(hwnd uintptr) bool {
	minimized, _, _ := procIsIconic.Call(hwnd)
	return minimized != 0
}
