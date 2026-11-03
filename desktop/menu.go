//go:build windows

package main

import (
	"unsafe"

	"swcode/daemon/protocol"
)

const (
	menuOpen = 1
	menuQuit = 2
	// The computers' items follow, in the order the menu lists them.
	menuFirstComputer = 100

	openLabel   = "Open swcode"
	quitLabel   = "Quit swcode"
	unreachable = " (not reachable)"
)

// trayEvents is what each click on the icon or its notification does.
var trayEvents = map[uint32]func(w *window){
	ninSelect:           (*window).openOrSay,
	ninKeySelect:        (*window).openOrSay,
	wmContextMenu:       (*window).showMenu,
	ninBalloonUserClick: (*window).openNotice,
}

func (w *window) trayEvent(_, _, lParam uintptr) uintptr {
	if event, known := trayEvents[uint32(lParam&0xffff)]; known {
		event(w)
	}
	return 0
}

// menuComputer is a computer as the icon's menu lists it.
type menuComputer struct {
	id    string
	label string
	flags uintptr
}

// menuComputers is the computers the window can show, the one it shows checked: none when there is only this one.
func (w *window) menuComputers() []menuComputer {
	views := w.computerViews()
	if len(views) < 2 {
		return nil
	}
	items := []menuComputer{}
	for _, view := range views {
		item := menuComputer{flags: mfString, id: view.ID, label: view.Name}
		if view.ID != protocol.ThisComputer && !w.notices.isReachable(view.ID) {
			item.label += unreachable
		}
		if view.ID == w.shown {
			item.flags |= mfChecked
		}
		items = append(items, item)
	}
	return items
}

// showMenu is the icon's menu, where the pointer is: open the window, show another computer, or quit.
func (w *window) showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	defer func() { _, _, _ = procDestroyMenu.Call(menu) }()
	appendItem(menu, mfString, menuOpen, openLabel)
	_, _, _ = procSetMenuDefaultItem.Call(menu, menuOpen, 0)
	choices := w.menuComputers()
	if len(choices) > 0 {
		appendItem(menu, mfSeparator, 0, "")
	}
	for index, choice := range choices {
		appendItem(menu, choice.flags, uintptr(menuFirstComputer+index), choice.label)
	}
	appendItem(menu, mfSeparator, 0, "")
	appendItem(menu, mfString, menuQuit, quitLabel)
	pointer := point{}
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pointer)))
	// The menu closes when the person clicks elsewhere only if the window is in front.
	_, _, _ = procSetForegroundWindow.Call(w.hwnd)
	chosen, _, _ := procTrackPopupMenu.Call(menu, tpmRightButton|tpmReturnCmd|tpmNoNotify, uintptr(pointer.x), uintptr(pointer.y), 0, w.hwnd, 0)
	postMessage(w.hwnd, wmNull, 0)
	w.menuChosen(int(chosen), choices)
}

func (w *window) menuChosen(chosen int, choices []menuComputer) {
	if chosen == menuOpen {
		w.openOrSay()
		return
	}
	if chosen == menuQuit {
		w.quit()
		return
	}
	if index := chosen - menuFirstComputer; index >= 0 && index < len(choices) {
		w.showComputer(choices[index].id, "")
	}
}

func appendItem(menu, flags, id uintptr, label string) {
	text := uintptr(0)
	if label != "" {
		text = uintptr(unsafe.Pointer(utf16(label)))
	}
	_, _, _ = procAppendMenuW.Call(menu, flags, id, text)
}
