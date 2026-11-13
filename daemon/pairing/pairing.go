// Package pairing pairs phones by a QR code: codes that last a minute and serve once, traded for a token of the
// phone's own.
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
	// CodeLifetime: a code shows for a minute, long enough to scan, too short to be of use to whoever sees it
	// later.
	CodeLifetime = 60 * time.Second
	codeBytes    = 16
	tokenBytes   = 32
	defaultName  = "A phone"
	ownerOnly    = 0o600
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

func secret(size int) string {
	data := make([]byte, size)
	_, _ = rand.Read(data)
	return base64.RawURLEncoding.EncodeToString(data)
}

func hashOf(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Pairing is the phones paired, and the codes that pair them.
type Pairing struct {
	mu      sync.Mutex
	file    string
	now     func() time.Time
	newID   func() string
	codes   map[string]time.Time
	devices []storedDevice
}

// New reads the paired phones from their file, devices.json in the data folder.
func New(file string, now func() time.Time, newID func() string) *Pairing {
	pairing := &Pairing{codes: map[string]time.Time{}, devices: []storedDevice{}, file: file, newID: newID, now: now}
	if data, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(data, &pairing.devices)
	}
	return pairing
}

// Issue makes a new code for the QR code.
func (p *Pairing) Issue() protocol.PairingCode {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	p.dropExpired(now)
	code := secret(codeBytes)
	expires := now.Add(CodeLifetime)
	p.codes[code] = expires
	return protocol.PairingCode{Code: code, ExpiresAt: protocol.Timestamp(expires), LifetimeMs: int(CodeLifetime.Milliseconds())}
}

// Redeem is a phone's token for a live code, which then stops working; empty for a code expired, used or unknown.
// origin is where the page that asks came from, as its browser says, empty for anything but a page.
func (p *Pairing) Redeem(code, name, origin string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dropExpired(p.now())
	if _, live := p.codes[code]; !live {
		return ""
	}
	delete(p.codes, code)
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
	for code, expires := range p.codes {
		if !expires.After(now) {
			delete(p.codes, code)
		}
	}
}

func (p *Pairing) save() {
	_ = os.MkdirAll(filepath.Dir(p.file), 0o700)
	data, _ := json.MarshalIndent(p.devices, "", "  ")
	_ = os.WriteFile(p.file, data, ownerOnly)
}
