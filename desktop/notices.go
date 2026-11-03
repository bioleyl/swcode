//go:build windows

package main

import (
	"context"
	"sync"

	"swcode/desktop/watch"
)

const wmNotice = wmApp + 4

// noticeTitles is how each kind of notice starts its notification's title.
var noticeTitles = map[watch.Kind]string{
	watch.NeedsYou: "Needs you",
	watch.Finished: "Finished",
	watch.Failed:   "Failed",
}

// computerNotice is a notice, and the computer whose swcode it comes from.
type computerNotice struct {
	computer string
	notice   watch.Notice
}

// noticeBoard follows the sessions of every computer the app knows, a socket each, as the web app has: what needs
// the person comes to the window's thread, which shows it as Windows' notifications.
type noticeBoard struct {
	hwnd      uintptr
	mu        sync.Mutex
	queue     []computerNotice
	reachable map[string]bool
	stops     map[string]context.CancelFunc
}

func newNoticeBoard(hwnd uintptr) *noticeBoard {
	return &noticeBoard{hwnd: hwnd, reachable: map[string]bool{}, stops: map[string]context.CancelFunc{}}
}

// follow follows a computer's swcode, in place of what it followed of it before (another port, another token).
func (b *noticeBoard) follow(computer, address, token string) {
	b.unfollow(computer)
	ctx, cancel := context.WithCancel(context.Background())
	b.mu.Lock()
	b.stops[computer] = cancel
	b.mu.Unlock()
	go watch.Follow(ctx, address, token, watch.Handlers{
		Connected: func(open bool) { b.setReachable(computer, open) },
		Notice: func(notice watch.Notice) {
			b.mu.Lock()
			b.queue = append(b.queue, computerNotice{computer: computer, notice: notice})
			b.mu.Unlock()
			postMessage(b.hwnd, wmNotice, 0)
		},
	})
}

func (b *noticeBoard) unfollow(computer string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if stop := b.stops[computer]; stop != nil {
		stop()
	}
	delete(b.stops, computer)
	delete(b.reachable, computer)
}

func (b *noticeBoard) stopAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, stop := range b.stops {
		stop()
	}
	b.stops = map[string]context.CancelFunc{}
}

func (b *noticeBoard) setReachable(computer string, open bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reachable[computer] = open
}

func (b *noticeBoard) isReachable(computer string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reachable[computer]
}

// take is the notices that came since the last time.
func (b *noticeBoard) take() []computerNotice {
	b.mu.Lock()
	defer b.mu.Unlock()
	taken := b.queue
	b.queue = nil
	return taken
}
