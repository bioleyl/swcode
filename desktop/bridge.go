//go:build windows

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/wailsapp/wails/webview2/pkg/edge"

	"swcode/daemon/protocol"
	"swcode/desktop/computers"
)

const (
	wmComputerAdded = wmApp + 5
	thisComputer    = "This computer"
	deviceName      = "swcode on "
)

// added is how pairing with another computer ended, back on the window's thread.
type added struct {
	computer computers.Computer
	err      error
}

// pageMessage is what the page in the window asks the app (protocol.DesktopRequest), a JSON string, answered the
// same way. WebView2 sends a page's message back to it too: the page tells the answers by their type.
func (w *window) pageMessage(message string, sender *edge.ICoreWebView2) {
	var request protocol.DesktopRequest
	if json.Unmarshal([]byte(message), &request) != nil {
		return
	}
	w.sender = sender
	protocol.MapDesktopRequest[struct{}](request, bridge{w: w})
}

// bridge does what the page asks, on the window's thread.
type bridge struct{ w *window }

func (b bridge) ListComputers(protocol.DesktopRequestListComputers) struct{} {
	b.w.answerComputers()
	return struct{}{}
}

func (b bridge) AddComputer(request protocol.DesktopRequestAddComputer) struct{} {
	go b.w.pair(request.Address, request.Code)
	return struct{}{}
}

func (b bridge) OpenComputer(request protocol.DesktopRequestOpenComputer) struct{} {
	b.w.showComputer(request.ID, "")
	return struct{}{}
}

func (b bridge) EditComputer(request protocol.DesktopRequestEditComputer) struct{} {
	b.w.editComputer(request.ID, request.Name, request.Color)
	return struct{}{}
}

func (b bridge) ForgetComputer(request protocol.DesktopRequestForgetComputer) struct{} {
	b.w.forgetComputer(request.ID)
	b.w.answerComputers()
	return struct{}{}
}

func (b bridge) OpenOutside(request protocol.DesktopRequestOpenOutside) struct{} {
	openOutside(request.URL)
	return struct{}{}
}

// pair reaches another computer's swcode, away from the window's thread: it takes a few seconds when it does not
// answer.
func (w *window) pair(address, code string) {
	host, _ := os.Hostname()
	computer, err := computers.Connect(context.Background(), http.DefaultClient, address, code, deviceName+host)
	w.pairings <- added{computer: computer, err: err}
	postMessage(w.hwnd, wmComputerAdded, 0)
}

// computerAdded keeps the computer pairing gave, follows its sessions, and tells the page.
func (w *window) computerAdded(_, _, _ uintptr) uintptr {
	result := <-w.pairings
	if result.err != nil {
		w.answer(protocol.DesktopAnswer{Value: protocol.DesktopAnswerComputerRefused{Message: result.err.Error()}})
		return 0
	}
	if err := w.computers.Add(result.computer); err != nil {
		w.answer(protocol.DesktopAnswer{Value: protocol.DesktopAnswerComputerRefused{Message: err.Error()}})
		return 0
	}
	w.notices.follow(result.computer.ID, result.computer.Address, result.computer.Token)
	w.answerComputers()
	return 0
}

// editComputer gives another computer the name and colour the person chose.
func (w *window) editComputer(id, name string, color protocol.ComputerColor) {
	if id == protocol.ThisComputer {
		return
	}
	if err := w.computers.Edit(id, name, color); err != nil {
		w.answer(protocol.DesktopAnswer{Value: protocol.DesktopAnswerComputerRefused{Message: err.Error()}})
		return
	}
	w.answerComputers()
}

// forgetComputer forgets a computer: the window shows this one instead when it showed that one.
func (w *window) forgetComputer(id string) {
	if id == protocol.ThisComputer {
		return
	}
	if w.shown == id {
		w.showComputer(protocol.ThisComputer, "")
	}
	w.notices.unfollow(id)
	_ = w.computers.Forget(id)
}

// computerViews is the computers as the page lists them: this one first, never a token.
func (w *window) computerViews() []protocol.Computer {
	address := ""
	if w.current != nil {
		address = w.current.address()
	}
	views := []protocol.Computer{{Address: address, ID: protocol.ThisComputer, Name: thisComputer, Reachable: w.current != nil}}
	for _, computer := range w.computers.All() {
		views = append(views, protocol.Computer{
			Address: computer.Address, Color: computer.Color, ID: computer.ID, Name: computer.Name,
			Reachable: w.notices.isReachable(computer.ID),
		})
	}
	return views
}

func (w *window) answerComputers() {
	w.answer(protocol.DesktopAnswer{Value: protocol.DesktopAnswerComputers{Computers: w.computerViews(), Current: w.shown}})
}

// answer sends the page an answer: none before it asked anything.
func (w *window) answer(message protocol.DesktopAnswer) {
	data, err := json.Marshal(message)
	if err != nil || w.sender == nil {
		return
	}
	_ = w.sender.PostWebMessageAsString(string(data))
}
