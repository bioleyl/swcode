package tools

import (
	"io"
	"strings"
	"sync"
	"time"
)

const (
	// liveEvery is how often what a command prints is shown while it runs: a build prints thousands of lines a
	// second, and the page draws a few times.
	liveEvery = 100 * time.Millisecond
	// liveChars is the most shown at a time: the end of what came, where a command says how it stands.
	liveChars = 4000
)

// liveOutput shows the person what a command prints while it runs, a piece every liveEvery at most. A command that
// nobody watches (a subagent's) has one that shows nothing.
type liveOutput struct {
	show    func(text string)
	mu      sync.Mutex
	waiting []byte
	timer   *time.Timer
	closed  bool
}

// liveOutputOf is the live output of the call being run; one that shows nothing where nobody watches.
func liveOutputOf(tc *Context) *liveOutput {
	if tc.ShowOutput == nil {
		return &liveOutput{}
	}
	callID, show := tc.CallID, tc.ShowOutput
	return &liveOutput{show: func(text string) { show(callID, text) }}
}

// writer is what the command's output is written to as it comes; nil when nobody watches.
func (l *liveOutput) writer() io.Writer {
	if l.show == nil {
		return nil
	}
	return l
}

// Write keeps what came for the next showing, which it schedules when none is. It never fails.
func (l *liveOutput) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return len(data), nil
	}
	l.waiting = append(l.waiting, data...)
	if l.timer == nil {
		l.timer = time.AfterFunc(liveEvery, l.flush)
	}
	return len(data), nil
}

// flush shows what waits, its end when it is long.
func (l *liveOutput) flush() {
	l.mu.Lock()
	text := strings.ToValidUTF8(string(l.waiting[max(0, len(l.waiting)-liveChars):]), "")
	l.waiting, l.timer = nil, nil
	l.mu.Unlock()
	if text != "" {
		l.show(text)
	}
}

// close ends the showing with the command: what still waits is dropped, the call's result holds it all.
func (l *liveOutput) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	if l.timer != nil {
		l.timer.Stop()
	}
}
