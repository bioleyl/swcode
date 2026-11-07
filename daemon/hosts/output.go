package hosts

import (
	"fmt"
	"strings"
	"sync"
)

// OutputBuffer keeps the start and the end of a long output and drops the middle: errors are usually at the end.
type OutputBuffer struct {
	mu        sync.Mutex
	headLimit int
	tailLimit int
	head      []byte
	tail      []byte
	dropped   int
	// written is all the command printed so far, kept or not: whether it printed something since a moment ago.
	written int
}

func NewOutputBuffer(headLimit, tailLimit int) *OutputBuffer {
	return &OutputBuffer{headLimit: headLimit, tailLimit: tailLimit}
}

// Write takes what a command printed; it never fails.
func (b *OutputBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	room := max(0, b.headLimit-len(b.head))
	taken := min(room, len(data))
	b.head = append(b.head, data[:taken]...)
	tail := append(b.tail, data[taken:]...)
	overflow := max(0, len(tail)-b.tailLimit)
	b.dropped += overflow
	b.tail = append([]byte(nil), tail[overflow:]...)
	b.written += len(data)
	return len(data), nil
}

// Written is how much the command printed so far, the middle cut included.
func (b *OutputBuffer) Written() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.written
}

// Text is the output kept, a note where its middle was cut.
func (b *OutputBuffer) Text() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dropped == 0 {
		return strings.ToValidUTF8(string(b.head)+string(b.tail), "�")
	}
	return strings.ToValidUTF8(fmt.Sprintf("%s\n[… %d characters cut …]\n%s", b.head, b.dropped, b.tail), "�")
}
