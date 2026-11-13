package app

import "sync"

// deviceSockets is the sockets each paired phone has open. A phone's token is checked when its socket opens, so
// forgetting the phone has to end the sockets it opened before: they would go on running commands otherwise.
type deviceSockets struct {
	mu   sync.Mutex
	next int
	ends map[string]map[int]func()
}

func newDeviceSockets() *deviceSockets {
	return &deviceSockets{ends: map[string]map[int]func(){}}
}

// open notes a socket a phone opened, with what ends it; the returned function takes it off once it closed.
func (d *deviceSockets) open(device string, end func()) func() {
	d.mu.Lock()
	defer d.mu.Unlock()
	id := d.next
	d.next++
	if d.ends[device] == nil {
		d.ends[device] = map[int]func(){}
	}
	d.ends[device][id] = end
	return func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		delete(d.ends[device], id)
	}
}

// end ends every socket a phone has open.
func (d *deviceSockets) end(device string) {
	d.mu.Lock()
	ends := d.ends[device]
	delete(d.ends, device)
	d.mu.Unlock()
	for _, each := range ends {
		each()
	}
}
