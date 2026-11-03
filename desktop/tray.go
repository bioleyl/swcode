//go:build windows

package main

import (
	"unsafe"
)

const (
	trayID = 1
	wmTray = wmApp + 3

	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4

	nifMessage  = 0x01
	nifIcon     = 0x02
	nifTip      = 0x04
	nifInfo     = 0x10
	nifShowTip  = 0x80
	iconVersion = 4

	// What a click on the icon or its notification sends, with NOTIFYICON_VERSION_4.
	ninSelect           = 0x0400
	ninKeySelect        = 0x0401
	ninBalloonUserClick = 0x0405

	niifRespectQuietTime = 0x80
	tip                  = "swcode"
	taskbarCreated       = "TaskbarCreated"
)

// notifyIconData is Win32's NOTIFYICONDATAW.
type notifyIconData struct {
	size            uint32
	hwnd            uintptr
	id              uint32
	flags           uint32
	callbackMessage uint32
	icon            uintptr
	tip             [128]uint16
	state           uint32
	stateMask       uint32
	info            [256]uint16
	versionOrTime   uint32
	infoTitle       [64]uint16
	infoFlags       uint32
	guidItem        [16]byte
	balloonIcon     uintptr
}

// trayIcon is the app's icon in the taskbar's notification area: the app lives there once its window is closed,
// and its notifications come from it.
type trayIcon struct {
	hwnd uintptr
	icon uintptr
}

// taskbarRestarted is the message Windows sends when Explorer starts again: the icons are gone and come back.
var taskbarRestarted = registeredMessage(taskbarCreated)

func registeredMessage(name string) uint32 {
	message, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(utf16(name))))
	return uint32(message)
}

func (t *trayIcon) data(flags uint32) notifyIconData {
	data := notifyIconData{flags: flags, hwnd: t.hwnd, id: trayID}
	data.size = uint32(unsafe.Sizeof(data))
	return data
}

// add shows the icon, at the size of the notification area's.
func (t *trayIcon) add(hwnd uintptr) {
	t.hwnd = hwnd
	if t.icon == 0 {
		instance, _, _ := procGetModuleHandleW.Call(0)
		width, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
		height, _, _ := procGetSystemMetrics.Call(smCySmIcon)
		t.icon, _, _ = procLoadImageW.Call(instance, iconResource, imageIcon, width, height, 0)
	}
	data := t.data(nifMessage | nifIcon | nifTip | nifShowTip)
	data.callbackMessage, data.icon = wmTray, t.icon
	fill(data.tip[:], tip)
	t.call(nimAdd, &data)
	data.versionOrTime = iconVersion
	t.call(nimSetVersion, &data)
}

// setTip changes what the icon says under the pointer.
func (t *trayIcon) setTip(text string) {
	data := t.data(nifTip | nifShowTip)
	fill(data.tip[:], text)
	t.call(nimModify, &data)
}

func (t *trayIcon) remove() {
	data := t.data(0)
	t.call(nimDelete, &data)
}

// notify shows a notification from the icon: Windows shows it as its own, and keeps quiet when the person asked
// it to.
func (t *trayIcon) notify(title, text string) {
	data := t.data(nifInfo)
	data.infoFlags = niifRespectQuietTime
	fill(data.infoTitle[:], title)
	fill(data.info[:], text)
	t.call(nimModify, &data)
}

func (t *trayIcon) call(action uintptr, data *notifyIconData) {
	_, _, _ = procShellNotifyIconW.Call(action, uintptr(unsafe.Pointer(data)))
}
