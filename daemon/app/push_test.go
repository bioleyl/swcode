package app_test

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	"swcode/daemon/push"
	. "swcode/daemon/testapp"
)

// The parts of a push's body before its text: a salt, the record's size, the length of the sender's key, the key.
const (
	pushSalt   = 16
	pushHeader = pushSalt + 4 + 1
	pushPoint  = 65
)

// pushed is a push service that keeps what the daemon sent a browser, and the browser's side to read it.
type pushed struct {
	t      *testing.T
	server *httptest.Server
	key    *ecdh.PrivateKey
	secret []byte
	mu     sync.Mutex
	bodies [][]byte
}

func newPushed(t *testing.T) *pushed {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	service := &pushed{key: key, secret: make([]byte, pushSalt), t: t}
	_, _ = rand.Read(service.secret)
	service.server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		service.mu.Lock()
		defer service.mu.Unlock()
		service.bodies = append(service.bodies, body)
		response.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(service.server.Close)
	return service
}

// subscription is what the browser posts to the daemon.
func (p *pushed) subscription() map[string]any {
	return map[string]any{"endpoint": p.server.URL + "/send/abc", "keys": map[string]any{
		"auth": base64.RawURLEncoding.EncodeToString(p.secret), "p256dh": base64.RawURLEncoding.EncodeToString(p.key.PublicKey().Bytes()),
	}}
}

func (p *pushed) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.bodies)
}

// notice is one of the pushes as the browser reads it.
func (p *pushed) notice(index int) protocol.PushNotice {
	p.t.Helper()
	p.mu.Lock()
	body := p.bodies[index]
	p.mu.Unlock()
	sender := body[pushHeader : pushHeader+pushPoint]
	theirs, err := ecdh.P256().NewPublicKey(sender)
	if err != nil {
		p.t.Fatal(err)
	}
	shared, _ := p.key.ECDH(theirs)
	key, nonce, err := push.RecordKeys(shared, p.secret, p.key.PublicKey().Bytes(), sender, body[:pushSalt])
	if err != nil {
		p.t.Fatal(err)
	}
	block, _ := aes.NewCipher(key)
	opener, _ := cipher.NewGCM(block)
	text, err := opener.Open(nil, nonce, body[pushHeader+pushPoint:], nil)
	if err != nil {
		p.t.Fatalf("the push does not open: %v", err)
	}
	var notice protocol.PushNotice
	if err := json.Unmarshal(text[:len(text)-1], &notice); err != nil {
		p.t.Fatal(err)
	}
	return notice
}

func TestABrowserThatSubscribedIsToldOnceWhenASessionWaitsAndWhenItsTurnEnds(t *testing.T) {
	service := newPushed(t)
	test := Start(t, Options{Replies: []fakellama.Reply{
		call("bash", map[string]any{"command": "echo one"}), text("Said one."), text("Once more."),
	}})
	_, key := test.Call("GET", protocol.EndpointPush, nil)
	if text, _ := key.(map[string]any)["key"].(string); len(text) < pushPoint {
		t.Fatalf("the daemon's push key: %v", key)
	}
	if status, _ := test.Call("POST", protocol.EndpointPush, service.subscription()); status != http.StatusOK {
		t.Fatalf("subscribing answered %d", status)
	}
	if status, _ := test.Call("POST", protocol.EndpointPush, map[string]any{"endpoint": "http://example.com/a", "keys": map[string]any{"auth": "x", "p256dh": "y"}}); status != http.StatusBadRequest {
		t.Fatalf("what is no subscription answered %d", status)
	}

	sessionID := prompt(t, test, "Say one")
	test.WaitForEvent("permissionRequested", 1)
	test.Until("the push that the session waits", func() bool { return service.count() == 1 })
	waits := service.notice(0)
	if waits.SessionID != sessionID || !strings.Contains(waits.Body, "echo one") || waits.Title == "" {
		t.Fatalf("the push says %+v", waits)
	}

	answerNext(t, test, sessionID, protocol.PermissionDecisionAllowOnce, 1)
	test.UntilIdle(sessionID)
	test.Until("the push that the turn ended", func() bool { return service.count() == 2 })
	if ended := service.notice(1); ended.SessionID != sessionID || !strings.Contains(ended.Body, "Said one.") {
		t.Fatalf("the push says %+v", ended)
	}

	// A browser that asked out is told no more.
	if status, _ := test.Call("DELETE", protocol.EndpointPush, map[string]any{"endpoint": service.server.URL + "/send/abc"}); status != http.StatusOK {
		t.Fatalf("asking out answered %d", status)
	}
	if problem := test.App.Sessions.SendPrompt(sessionID, "Again", nil); problem != "" {
		t.Fatal(problem)
	}
	test.WaitForEvent("turnFinished", 2)
	test.UntilIdle(sessionID)
	if service.count() != 2 {
		t.Fatalf("%d pushes after asking out", service.count())
	}
}
