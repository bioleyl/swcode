// Package pairing pairs a phone or another computer by a number its app shows: `swcode pair <number>` on this
// computer makes the number wait a few minutes, and the app trades it once for a token of its own.
package pairing

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"swcode/daemon/protocol"
)

const (
	// NumberLifetime: a number waits long enough to walk to the computer and type it, and no longer: anyone who sees
	// it later cannot use it.
	NumberLifetime = 5 * time.Minute
	tokenBytes     = 32
	defaultName    = "A phone"
	ownerOnly      = 0o600
	// TriesPerMinute is how often one client may ask to redeem a number: a six-digit number is only safe from
	// guessing while no machine can try millions of them within the minutes it waits.
	TriesPerMinute = 60
	tryWindow      = time.Minute
)

// storedDevice is a phone as devices.json keeps it: its token only as its hash, so the file gives no access.
// Origin is where the page that paired came from, when a browser said so: the app a phone installed from another
// computer's swcode talks to this one from that origin.
type storedDevice struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Origin    string `json:"origin,omitempty"`
	PairedAt  string `json:"pairedAt"`
	TokenHash string `json:"tokenHash"`
}

// tries counts one client's asks within a window, which the next ask starts again.
type tries struct {
	count int
	reset time.Time
}

func secret(size int) string {
	data := make([]byte, size)
	_, _ = rand.Read(data)
	return base64.RawURLEncoding.EncodeToString(data)
}

func hashOf(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Pairing is the phones paired, and the numbers that are waiting to pair one.
type Pairing struct {
	mu      sync.Mutex
	file    string
	now     func() time.Time
	newID   func() string
	pending map[string]time.Time
	tries   map[string]tries
	devices []storedDevice
}

// New reads the paired phones from their file, devices.json in the data folder.
func New(file string, now func() time.Time, newID func() string) *Pairing {
	pairing := &Pairing{
		devices: []storedDevice{},
		file:    file,
		newID:   newID,
		now:     now,
		pending: map[string]time.Time{},
		tries:   map[string]tries{},
	}
	if data, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(data, &pairing.devices)
	}
	return pairing
}

// Pend makes a number wait for its app to redeem it, for a few minutes: what `swcode pair <number>` asks for. A number
// made to wait again waits from now on.
func (p *Pairing) Pend(number string) protocol.PairPending {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	p.dropExpired(now)
	expires := now.Add(NumberLifetime)
	p.pending[number] = expires
	return protocol.PairPending{ExpiresAt: protocol.Timestamp(expires), LifetimeMs: int(NumberLifetime.Milliseconds())}
}

// Redeem is an app's token for a number waiting here, which then stops working; empty for a number expired, used or
// never made to wait. origin is where the page that asks came from, as its browser says, empty for anything but a
// page.
func (p *Pairing) Redeem(number, name, origin string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dropExpired(p.now())
	if _, waiting := p.pending[number]; !waiting {
		return ""
	}
	delete(p.pending, number)
	token := secret(tokenBytes)
	if name == "" {
		name = defaultName
	}
	p.devices = append(p.devices, storedDevice{
		ID: p.newID(), Name: name, Origin: origin, PairedAt: protocol.Timestamp(p.now()), TokenHash: hex.EncodeToString(hashOf(token)),
	})
	p.save()
	return token
}

// Allow says whether a client may ask to redeem a number now: TriesPerMinute within any minute, so that a machine
// cannot try enough of the million numbers to find the one waiting.
func (p *Pairing) Allow(client string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	asked, counted := p.tries[client]
	if !counted || !now.Before(asked.reset) {
		p.forgetTries(now)
		p.tries[client] = tries{count: 1, reset: now.Add(tryWindow)}
		return true
	}
	if asked.count >= TriesPerMinute {
		return false
	}
	asked.count++
	p.tries[client] = asked
	return true
}

// Knows tells whether a token is a paired phone's.
func (p *Pairing) Knows(token string) bool {
	_, known := p.DeviceOf(token)
	return known
}

// DeviceOf is the paired phone a token belongs to, by its id: every phone is compared, so the time taken says
// nothing of which one matched.
func (p *Pairing) DeviceOf(token string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	given := hashOf(token)
	id, known := "", false
	for _, device := range p.devices {
		stored, _ := hex.DecodeString(device.TokenHash)
		if subtle.ConstantTimeCompare(stored, given) == 1 {
			id, known = device.ID, true
		}
	}
	return id, known
}

// AllowsOrigin tells whether a paired phone's page came from that origin: a page from there may talk to this
// daemon, with its phone's token.
func (p *Pairing) AllowsOrigin(origin string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return origin != "" && slices.ContainsFunc(p.devices, func(device storedDevice) bool { return device.Origin == origin })
}

func (p *Pairing) Devices() []protocol.Device {
	p.mu.Lock()
	defer p.mu.Unlock()
	devices := []protocol.Device{}
	for _, device := range p.devices {
		devices = append(devices, protocol.Device{ID: device.ID, Name: device.Name, PairedAt: device.PairedAt})
	}
	return devices
}

// Forget forgets a phone: its token stops working at once.
func (p *Pairing) Forget(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	kept := []storedDevice{}
	for _, device := range p.devices {
		if device.ID != id {
			kept = append(kept, device)
		}
	}
	if len(kept) == len(p.devices) {
		return false
	}
	p.devices = kept
	p.save()
	return true
}

func (p *Pairing) dropExpired(now time.Time) {
	for number, expires := range p.pending {
		if !expires.After(now) {
			delete(p.pending, number)
		}
	}
}

// forgetTries drops the counts of windows that are over, so that a daemon seen by many clients does not keep them all.
func (p *Pairing) forgetTries(now time.Time) {
	for client, asked := range p.tries {
		if !now.Before(asked.reset) {
			delete(p.tries, client)
		}
	}
}

func (p *Pairing) save() {
	_ = os.MkdirAll(filepath.Dir(p.file), 0o700)
	data, _ := json.MarshalIndent(p.devices, "", "  ")
	_ = os.WriteFile(p.file, data, ownerOnly)
}
