package push

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// browser is the side that reads: a key pair and a secret of its own, as a browser makes them when it subscribes.
type browser struct {
	key    *ecdh.PrivateKey
	secret []byte
}

func newBrowser(t *testing.T) browser {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, authLength)
	_, _ = rand.Read(secret)
	return browser{key: key, secret: secret}
}

func (b browser) keys() Keys {
	return Keys{Auth: base64.RawURLEncoding.EncodeToString(b.secret), P256dh: base64.RawURLEncoding.EncodeToString(b.key.PublicKey().Bytes())}
}

// read is the receiver's side of RFC 8291: the message a body holds, for the browser it was written for.
func (b browser) read(t *testing.T, body []byte) string {
	t.Helper()
	salt, size, length := body[:saltLength], binary.BigEndian.Uint32(body[saltLength:saltLength+4]), int(body[saltLength+4])
	if size != recordSize || length != pointLength {
		t.Fatalf("a record of %d bytes, a key of %d", size, length)
	}
	start := saltLength + 4 + 1
	sender := body[start : start+length]
	theirs, err := ecdh.P256().NewPublicKey(sender)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := b.key.ECDH(theirs)
	if err != nil {
		t.Fatal(err)
	}
	key, nonce, err := RecordKeys(shared, b.secret, b.key.PublicKey().Bytes(), sender, salt)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := aes.NewCipher(key)
	opener, _ := cipher.NewGCM(block)
	text, err := opener.Open(nil, nonce, body[start+length:], nil)
	if err != nil {
		t.Fatalf("the record does not open: %v", err)
	}
	if text[len(text)-1] != lastRecord {
		t.Fatalf("the record does not end as the last one: %x", text[len(text)-1])
	}
	return string(text[:len(text)-1])
}

func newService(t *testing.T) *Service {
	t.Helper()
	service, err := New(filepath.Join(t.TempDir(), File), http.DefaultClient, "https://example.com/contact", time.Now, func(err error) { t.Errorf("push: %v", err) })
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestAMessageIsReadByTheBrowserItWasWrittenForAndNoOther(t *testing.T) {
	reader, other := newBrowser(t), newBrowser(t)
	body, err := Encrypt(reader.keys(), []byte(`{"title":"Fix the build"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := reader.read(t, body); got != `{"title":"Fix the build"}` {
		t.Fatalf("read %q", got)
	}
	again, _ := Encrypt(reader.keys(), []byte(`{"title":"Fix the build"}`))
	if string(again) == string(body) {
		t.Fatal("two messages are the same bytes: the key or the salt is reused")
	}
	// Another browser's key opens nothing.
	theirs, _ := ecdh.P256().NewPublicKey(body[saltLength+5 : saltLength+5+pointLength])
	shared, _ := other.key.ECDH(theirs)
	key, nonce, _ := RecordKeys(shared, other.secret, other.key.PublicKey().Bytes(), body[saltLength+5:saltLength+5+pointLength], body[:saltLength])
	block, _ := aes.NewCipher(key)
	opener, _ := cipher.NewGCM(block)
	if _, err := opener.Open(nil, nonce, body[saltLength+5+pointLength:], nil); err == nil {
		t.Fatal("another browser read the message")
	}
	if _, err := Encrypt(reader.keys(), make([]byte, maxPayload+1)); err == nil {
		t.Fatal("a message longer than a record went through")
	}
}

// pushService is a push service: it keeps what it was sent, and answers as it is told to.
type pushService struct {
	mu       sync.Mutex
	server   *httptest.Server
	requests []*http.Request
	bodies   [][]byte
	status   int
}

func newPushService(t *testing.T) *pushService {
	t.Helper()
	service := &pushService{status: http.StatusCreated}
	service.server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		service.mu.Lock()
		defer service.mu.Unlock()
		service.requests = append(service.requests, request)
		service.bodies = append(service.bodies, body)
		response.WriteHeader(service.status)
	}))
	t.Cleanup(service.server.Close)
	return service
}

func decoded(t *testing.T, text string) []byte {
	t.Helper()
	data, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestABrowserIsToldThroughItsPushServiceSignedByTheDaemonsKey(t *testing.T) {
	service, carrier, reader := newService(t), newPushService(t), newBrowser(t)
	endpoint := carrier.server.URL + "/send/abc"
	if problem := service.Subscribe(Subscription{Endpoint: endpoint, Keys: reader.keys()}); problem != "" {
		t.Fatal(problem)
	}
	service.Send(context.Background(), []byte("needs you"))
	if len(carrier.requests) != 1 {
		t.Fatalf("%d requests", len(carrier.requests))
	}
	request := carrier.requests[0]
	if request.Method != http.MethodPost || request.URL.Path != "/send/abc" || request.Header.Get("content-encoding") != "aes128gcm" || request.Header.Get("ttl") == "" {
		t.Fatalf("the request: %s %s %v", request.Method, request.URL.Path, request.Header)
	}
	if got := reader.read(t, carrier.bodies[0]); got != "needs you" {
		t.Fatalf("read %q", got)
	}

	// Who sends: a JWT the daemon's public key verifies, for this push service, not for long.
	authorization := request.Header.Get("authorization")
	token, key, found := strings.Cut(strings.TrimPrefix(authorization, "vapid t="), ", k=")
	if !found || key != service.PublicKey() {
		t.Fatalf("authorization %q", authorization)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("the token has %d parts", len(parts))
	}
	signature := decoded(t, parts[2])
	if len(signature) != 2*half {
		t.Fatalf("a signature of %d bytes: JOSE writes the two numbers side by side", len(signature))
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	public, _ := ecdsa.ParseUncompressedPublicKey(service.key.Curve, decoded(t, key))
	if !ecdsa.Verify(public, digest[:], new(big.Int).SetBytes(signature[:half]), new(big.Int).SetBytes(signature[half:])) {
		t.Fatal("the daemon's key does not verify the token")
	}
	var claims struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(decoded(t, parts[1]), &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Aud != carrier.server.URL || claims.Sub != "https://example.com/contact" || time.Until(time.Unix(claims.Exp, 0)) > 24*time.Hour || time.Until(time.Unix(claims.Exp, 0)) <= 0 {
		t.Fatalf("claims %+v", claims)
	}
}

func TestASubscriptionItsPushServiceSaysIsGoneIsForgotten(t *testing.T) {
	service, carrier, reader := newService(t), newPushService(t), newBrowser(t)
	if problem := service.Subscribe(Subscription{Endpoint: carrier.server.URL + "/send/abc", Keys: reader.keys()}); problem != "" {
		t.Fatal(problem)
	}
	carrier.status = http.StatusGone
	service.Send(context.Background(), []byte("first"))
	if service.Any() {
		t.Fatal("a subscription that is gone was kept")
	}
	service.Send(context.Background(), []byte("second"))
	if len(carrier.requests) != 1 {
		t.Fatalf("%d requests: nothing is sent to nobody", len(carrier.requests))
	}
}

func TestTheKeyAndTheSubscriptionsAreKeptForThePersonAloneAndFoundAgain(t *testing.T) {
	file := filepath.Join(t.TempDir(), File)
	logged := func(err error) { t.Errorf("push: %v", err) }
	first, err := New(file, http.DefaultClient, "https://example.com", time.Now, logged)
	if err != nil {
		t.Fatal(err)
	}
	phone, laptop := newBrowser(t), newBrowser(t)
	first.Subscribe(Subscription{Device: "phone", Endpoint: "https://push.example.com/a", Keys: phone.keys()})
	first.Subscribe(Subscription{Endpoint: "https://push.example.com/b", Keys: laptop.keys()})
	// The same browser again takes its own place.
	first.Subscribe(Subscription{Device: "phone", Endpoint: "https://push.example.com/a", Keys: phone.keys()})
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != fileOnly {
		t.Fatalf("the file: %v, %v", info, err)
	}
	again, err := New(file, http.DefaultClient, "https://example.com", time.Now, logged)
	if err != nil || again.PublicKey() != first.PublicKey() || len(again.held) != 2 {
		t.Fatalf("after a restart: %v, %d subscriptions", err, len(again.held))
	}
	again.ForgetDevice("phone")
	if len(again.held) != 1 || again.held[0].Endpoint != "https://push.example.com/b" {
		t.Fatalf("after the phone was forgotten: %+v", again.held)
	}
	again.Unsubscribe("https://push.example.com/b")
	if again.Any() {
		t.Fatal("a browser that asked out is still told")
	}
}

func TestOnlyAPushSubscriptionIsKept(t *testing.T) {
	service, reader := newService(t), newBrowser(t)
	keys := reader.keys()
	for name, subscription := range map[string]Subscription{
		"an address in clear":    {Endpoint: "http://push.example.com/a", Keys: keys},
		"no address":             {Keys: keys},
		"a key that is no point": {Endpoint: "https://push.example.com/a", Keys: Keys{Auth: keys.Auth, P256dh: base64.RawURLEncoding.EncodeToString(make([]byte, pointLength))}},
		"a short secret":         {Endpoint: "https://push.example.com/a", Keys: Keys{Auth: "c2hvcnQ", P256dh: keys.P256dh}},
	} {
		if problem := service.Subscribe(subscription); problem == "" {
			t.Errorf("%s was kept", name)
		}
	}
	if service.Any() {
		t.Fatal("something was kept")
	}
	for index := range maxSubscriptions + 3 {
		service.Subscribe(Subscription{Endpoint: "https://push.example.com/" + string(rune('a'+index)), Keys: keys})
	}
	if len(service.held) != maxSubscriptions || service.held[0].Endpoint != "https://push.example.com/d" {
		t.Fatalf("%d kept, the oldest %s", len(service.held), service.held[0].Endpoint)
	}
}
