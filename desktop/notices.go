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
// the person comes to the window's thread, which shows it as Windows' notifications. The person turns those
// notifications on and off for every computer at once; a project that says nothing goes by that answer.
type noticeBoard struct {
	file      string
	hwnd      uintptr
	mu        sync.Mutex
	on        bool
	queue     []computerNotice
	reachable map[string]bool
	rules     map[string]*watch.Rules
	stops     map[string]context.CancelFunc
}

func newNoticeBoard(hwnd uintptr, file string) *noticeBoard {
	return &noticeBoard{
		file: file, hwnd: hwnd, on: loadNotifications(file),
		reachable: map[string]bool{}, rules: map[string]*watch.Rules{}, stops: map[string]context.CancelFunc{},
	}
}

// setNotifications keeps the person's answer, for every computer the app follows and for the next start.
func (b *noticeBoard) setNotifications(on bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.on = on
	saveNotifications(b.file, on)
	for _, rules := range b.rules {
		rules.SetOff(!on)
	}
}

// notifications is where the person left them, on for a app that never was asked.
func (b *noticeBoard) notifications() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.on
}

// follow follows a computer's swcode, in place of what it followed of it before (another port, another token).
func (b *noticeBoard) follow(computer, address, token string) {
	b.unfollow(computer)
	ctx, cancel := context.WithCancel(context.Background())
	b.mu.Lock()
	b.stops[computer] = cancel
	rules := b.rules[computer]
	if rules == nil {
		rules = watch.NewRules()
		rules.SetOff(!b.on)
		b.rules[computer] = rules
	}
	b.mu.Unlock()
	go watch.Follow(ctx, address, token, rules, watch.Handlers{
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
