// Package push tells the browsers that asked for it what a session needs, the app closed: Web Push. The daemon
// signs who it is with a key of its own (VAPID, RFC 8292) and encrypts each message for the one browser that reads
// it (RFC 8291), so the push service that carries it learns nothing of the sessions.
package push

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sync"
	"time"
)

const (
	// File is the daemon's push key and the browsers that subscribed, in the data folder.
	File     = "push.json"
	fileOnly = 0o600
	// maxSubscriptions is how many browsers are kept: the oldest goes when another comes.
	maxSubscriptions = 20
	// A message is one record: its size is the least a browser must take.
	recordSize   = 4096
	saltLength   = 16
	authLength   = 16
	pointLength  = 65
	keyLength    = 16
	nonceLength  = 12
	secretLength = 32
	half         = 32
	// lastRecord ends the text of the only record.
	lastRecord = 0x02
	// maxPayload leaves room for the record's header, the delimiter and the tag.
	maxPayload  = 3000
	keyInfo     = "WebPush: info\x00"
	cekInfo     = "Content-Encoding: aes128gcm\x00"
	nonceInfo   = "Content-Encoding: nonce\x00"
	encoding    = "aes128gcm"
	jwtHeader   = `{"typ":"JWT","alg":"ES256"}`
	tokenLife   = 12 * time.Hour
	secondsKept = "3600"
	sendLimit   = 20 * time.Second
	secure      = "https"
	plain       = "http"

	notASubscription = "This is not a push subscription: an HTTPS address, a key and a secret."
)

// Keys are what a browser gives to be written to: its public key and its secret, base64url.
type Keys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// Subscription is a browser that asked to be told: where its push service takes messages for it, and its keys.
// Device is the paired phone it is, when it is one: forgetting the phone forgets it too.
type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     Keys   `json:"keys"`
	Device   string `json:"device,omitempty"`
}

// stored is the file's content.
type stored struct {
	// Key is the daemon's private key, PKCS #8, base64.
	Key           string         `json:"key"`
	Subscriptions []Subscription `json:"subscriptions"`
}

// Service keeps the daemon's key and the browsers to tell, and sends them messages.
type Service struct {
	file     string
	http     *http.Client
	subject  string
	now      func() time.Time
	logError func(error)
	mu       sync.Mutex
	key      *ecdsa.PrivateKey
	held     []Subscription
}

// New reads the file, or makes the daemon's key at its first start. subject is who the push services may write to
// about this sender: an https or mailto address.
func New(file string, client *http.Client, subject string, now func() time.Time, logError func(error)) (*Service, error) {
	service := &Service{file: file, http: client, logError: logError, now: now, subject: subject}
	if data, err := os.ReadFile(file); err == nil {
		var kept stored
		if json.Unmarshal(data, &kept) == nil {
			if key := keyOf(kept.Key); key != nil {
				service.key, service.held = key, kept.Subscriptions
				return service, nil
			}
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	service.key = key
	return service, service.save()
}

func keyOf(text string) *ecdsa.PrivateKey {
	der, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil
	}
	key, _ := parsed.(*ecdsa.PrivateKey)
	return key
}

// save writes the file; the caller holds the lock, or is alone.
func (s *Service) save() error {
	der, err := x509.MarshalPKCS8PrivateKey(s.key)
	if err != nil {
		return err
	}
	data, err := json.Marshal(stored{Key: base64.StdEncoding.EncodeToString(der), Subscriptions: s.held})
	if err != nil {
		return err
	}
	return os.WriteFile(s.file, data, fileOnly)
}

func (s *Service) log(err error) {
	if err != nil && s.logError != nil {
		s.logError(err)
	}
}

// PublicKey is the daemon's public key as a browser takes it to subscribe: the point, base64url.
func (s *Service) PublicKey() string {
	public, err := s.key.PublicKey.ECDH()
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(public.Bytes())
}

// local tells whether an address is this computer's: a push service of the tests.
func local(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

// checked is why a subscription cannot be kept; nothing when it can. Its address is a push service's: HTTPS, so the
// daemon is never made to write anywhere in clear.
func checked(subscription Subscription) string {
	address, err := url.Parse(subscription.Endpoint)
	if err != nil || address.Host == "" {
		return notASubscription
	}
	if address.Scheme != secure && (address.Scheme != plain || !local(address.Hostname())) {
		return notASubscription
	}
	point, err := base64.RawURLEncoding.DecodeString(subscription.Keys.P256dh)
	if err != nil || len(point) != pointLength {
		return notASubscription
	}
	if _, err := ecdh.P256().NewPublicKey(point); err != nil {
		return notASubscription
	}
	secret, err := base64.RawURLEncoding.DecodeString(subscription.Keys.Auth)
	if err != nil || len(secret) != authLength {
		return notASubscription
	}
	return ""
}

// Subscribe keeps a browser to tell: why not, when what it gave is no subscription. One that subscribes again takes
// its own place; the oldest goes past the most kept.
func (s *Service) Subscribe(subscription Subscription) string {
	if problem := checked(subscription); problem != "" {
		return problem
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.held = append(without(s.held, subscription.Endpoint), subscription)
	if len(s.held) > maxSubscriptions {
		s.held = s.held[len(s.held)-maxSubscriptions:]
	}
	s.log(s.save())
	return ""
}

func without(held []Subscription, endpoint string) []Subscription {
	return slices.DeleteFunc(slices.Clone(held), func(each Subscription) bool { return each.Endpoint == endpoint })
}

// Unsubscribe forgets a browser, by the address of its subscription.
func (s *Service) Unsubscribe(endpoint string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.held = without(s.held, endpoint)
	s.log(s.save())
}

// ForgetDevice forgets the browsers of a paired phone, with the phone.
func (s *Service) ForgetDevice(device string) {
	if device == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.held = slices.DeleteFunc(slices.Clone(s.held), func(each Subscription) bool { return each.Device == device })
	s.log(s.save())
}

// Any tells whether any browser asked to be told: nothing is worded for nobody.
func (s *Service) Any() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.held) > 0
}

// Send tells every browser kept, each through its push service; one whose service says it is gone is forgotten.
func (s *Service) Send(ctx context.Context, payload []byte) {
	s.mu.Lock()
	held := slices.Clone(s.held)
	s.mu.Unlock()
	for _, subscription := range held {
		gone, err := s.sendTo(ctx, subscription, payload)
		if gone {
			s.Unsubscribe(subscription.Endpoint)
			continue
		}
		s.log(err)
	}
}

// sendTo sends one browser a message: whether its subscription is gone, and why the message did not go otherwise.
func (s *Service) sendTo(ctx context.Context, subscription Subscription, payload []byte) (bool, error) {
	body, err := Encrypt(subscription.Keys, payload)
	if err != nil {
		return false, err
	}
	token, err := s.token(subscription.Endpoint)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(ctx, sendLimit)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, subscription.Endpoint, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	request.Header.Set("authorization", "vapid t="+token+", k="+s.PublicKey())
	request.Header.Set("content-encoding", encoding)
	request.Header.Set("content-type", "application/octet-stream")
	request.Header.Set("ttl", secondsKept)
	request.Header.Set("urgency", "high")
	response, err := s.http.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, recordSize))
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
		return true, nil
	}
	if response.StatusCode >= http.StatusBadRequest {
		return false, fmt.Errorf("the push service answered %d", response.StatusCode)
	}
	return false, nil
}

func part(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }

// token says who sends, signed with the daemon's key, for the push service of one address: a JWT, whose signature
// is the two numbers side by side, 32 bytes each, as JOSE writes them.
func (s *Service) token(endpoint string) (string, error) {
	address, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{
		"aud": address.Scheme + "://" + address.Host, "exp": s.now().Add(tokenLife).Unix(), "sub": s.subject,
	})
	if err != nil {
		return "", err
	}
	signed := part([]byte(jwtHeader)) + "." + part(claims)
	digest := sha256.Sum256([]byte(signed))
	first, second, err := ecdsa.Sign(rand.Reader, s.key, digest[:])
	if err != nil {
		return "", err
	}
	signature := make([]byte, 2*half)
	first.FillBytes(signature[:half])
	second.FillBytes(signature[half:])
	return signed + "." + part(signature), nil
}

// Encrypt writes a message for one browser, as RFC 8291 says: a key made for this message alone agrees on a secret
// with the browser's, the browser's own secret mixed in, and the text goes as one aes128gcm record.
func Encrypt(keys Keys, payload []byte) ([]byte, error) {
	if len(payload) > maxPayload {
		return nil, errors.New("the message is too long for a push")
	}
	point, err := base64.RawURLEncoding.DecodeString(keys.P256dh)
	if err != nil {
		return nil, err
	}
	secret, err := base64.RawURLEncoding.DecodeString(keys.Auth)
	if err != nil {
		return nil, err
	}
	theirs, err := ecdh.P256().NewPublicKey(point)
	if err != nil {
		return nil, err
	}
	ours, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := ours.ECDH(theirs)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	sender := ours.PublicKey().Bytes()
	key, nonce, err := RecordKeys(shared, secret, point, sender, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	sealer, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	header := make([]byte, 0, saltLength+4+1+len(sender))
	header = append(header, salt...)
	header = binary.BigEndian.AppendUint32(header, recordSize)
	header = append(header, byte(len(sender)))
	header = append(header, sender...)
	return sealer.Seal(header, nonce, append(slices.Clone(payload), lastRecord), nil), nil
}

// RecordKeys are the key and the nonce of a message's record, from the secret both sides agreed on, the browser's
// own secret, both public keys and the message's salt: the same on the side that reads.
func RecordKeys(shared, secret, receiver, sender, salt []byte) ([]byte, []byte, error) {
	agreed, err := hkdf.Extract(sha256.New, shared, secret)
	if err != nil {
		return nil, nil, err
	}
	input, err := hkdf.Expand(sha256.New, agreed, keyInfo+string(receiver)+string(sender), secretLength)
	if err != nil {
		return nil, nil, err
	}
	record, err := hkdf.Extract(sha256.New, input, salt)
	if err != nil {
		return nil, nil, err
	}
	key, err := hkdf.Expand(sha256.New, record, cekInfo, keyLength)
	if err != nil {
		return nil, nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, record, nonceInfo, nonceLength)
	if err != nil {
		return nil, nil, err
	}
	return key, nonce, nil
}
