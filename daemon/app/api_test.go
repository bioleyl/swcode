package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	. "swcode/daemon/testapp"
)

func startHello(t *testing.T) *App {
	return Start(t, Options{Replies: []fakellama.Reply{text("Hello there.")}})
}

// raw calls a route with the token given, or none: the status, and the body.
func raw(t *testing.T, test *App, method, path, token string, body any) (int, []byte, http.Header) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request, _ := http.NewRequest(method, test.Server.URL+path, reader)
	if token != "" {
		request.Header.Set("authorization", protocol.BearerPrefix+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, data, response.Header
}

func TestTheAPIAnswersItsHealthWithoutAToken(t *testing.T) {
	test := startHello(t)
	if status, _, _ := raw(t, test, "GET", protocol.EndpointHealth, "", nil); status != 200 {
		t.Fatalf("status %d", status)
	}
}

func TestTheAPIServesThePicturesWithTheTokenAndNothingOutsideTheirFolder(t *testing.T) {
	test := startHello(t)
	picture := []byte{0x89, 0x50, 0x4e, 0x47}
	_ = os.MkdirAll(filepath.Join(test.DataDir, "images", "session-1"), 0o755)
	_ = os.WriteFile(filepath.Join(test.DataDir, "images", "session-1", "p1.png"), picture, 0o644)
	status, data, header := raw(t, test, "GET", protocol.EndpointImages+"/session-1/p1.png?token="+Token, "", nil)
	if status != 200 || header.Get("content-type") != "image/png" || !bytes.Equal(data, picture) {
		t.Fatalf("status %d", status)
	}
	if status, _, _ := raw(t, test, "GET", protocol.EndpointImages+"/session-1/p1.png", "", nil); status != 401 {
		t.Fatalf("status %d without a token", status)
	}
	if status, _, _ := raw(t, test, "GET", protocol.EndpointImages+"/session-1/..%2Fsettings.env?token="+Token, "", nil); status != 404 {
		t.Fatalf("status %d outside the folder", status)
	}
}

func TestAPhonePairedWithACodeOpensTheAPIUntilItIsForgotten(t *testing.T) {
	test := startHello(t)
	if status, _, _ := raw(t, test, "GET", protocol.EndpointPairingCode, "", nil); status != 401 {
		t.Fatalf("status %d", status)
	}
	_, data, _ := raw(t, test, "GET", protocol.EndpointPairingCode, Token, nil)
	var code protocol.PairingCode
	_ = json.Unmarshal(data, &code)
	pair := func() (int, []byte) {
		status, body, _ := raw(t, test, "POST", protocol.EndpointPairing, "", map[string]string{"code": code.Code, "name": "iPhone"})
		return status, body
	}
	status, body := pair()
	var paired protocol.PairAnswer
	_ = json.Unmarshal(body, &paired)
	if status != 200 || paired.Token == "" {
		t.Fatalf("status %d", status)
	}
	if status, _, _ := raw(t, test, "GET", protocol.EndpointStatus, paired.Token, nil); status != 200 {
		t.Fatalf("status %d with the phone's token", status)
	}
	if status, _ := pair(); status != 403 {
		t.Fatalf("status %d for a code used", status)
	}
	_, data, _ = raw(t, test, "GET", protocol.EndpointDevices, Token, nil)
	var devices protocol.Devices
	_ = json.Unmarshal(data, &devices)
	raw(t, test, "DELETE", protocol.EndpointDevices+"/"+devices.Devices[0].ID, Token, nil)
	if status, _, _ := raw(t, test, "GET", protocol.EndpointStatus, paired.Token, nil); status != 401 {
		t.Fatalf("status %d once forgotten", status)
	}
}

// pairedPhone pairs a phone with a fresh code: its token, and the id the daemon lists it under.
func pairedPhone(t *testing.T, test *App) (string, string) {
	t.Helper()
	_, data, _ := raw(t, test, "GET", protocol.EndpointPairingCode, Token, nil)
	var code protocol.PairingCode
	_ = json.Unmarshal(data, &code)
	_, data, _ = raw(t, test, "POST", protocol.EndpointPairing, "", map[string]string{"code": code.Code, "name": "iPhone"})
	var paired protocol.PairAnswer
	_ = json.Unmarshal(data, &paired)
	_, data, _ = raw(t, test, "GET", protocol.EndpointDevices, Token, nil)
	var devices protocol.Devices
	_ = json.Unmarshal(data, &devices)
	if paired.Token == "" || len(devices.Devices) != 1 {
		t.Fatalf("%d phones paired", len(devices.Devices))
	}
	return paired.Token, devices.Devices[0].ID
}

func TestForgettingAPhoneEndsTheSocketItHadOpen(t *testing.T) {
	test := startHello(t)
	token, device := pairedPhone(t, test)
	address := "ws" + strings.TrimPrefix(test.Server.URL, "http") + protocol.EndpointSocket + "?token=" + token
	connection, _, err := websocket.Dial(context.Background(), address, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	connection.SetReadLimit(64 << 20)
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		for {
			if _, _, err := connection.Read(context.Background()); err != nil {
				return
			}
		}
	}()
	raw(t, test, "DELETE", protocol.EndpointDevices+"/"+device, Token, nil)
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the forgotten phone's socket is still open")
	}
}

func TestTheStatusNeedsTheTokenAndSaysTheWorkspaceAndWhatLlamaServerServes(t *testing.T) {
	test := startHello(t)
	if status, _, _ := raw(t, test, "GET", protocol.EndpointStatus, "", nil); status != 401 {
		t.Fatalf("status %d", status)
	}
	if status, _, _ := raw(t, test, "GET", protocol.EndpointStatus+"?token="+Token, "", nil); status != 200 {
		t.Fatalf("status %d with the token in the query", status)
	}
	status, body := test.Call("GET", protocol.EndpointStatus, nil)
	if status != 200 {
		t.Fatalf("status %d", status)
	}
	want := map[string]any{
		"llama": map[string]any{
			"presets": []any{map[string]any{"contextTokens": float64(32_768), "id": "default", "model": Model}},
			"sources": []any{map[string]any{"models": []any{Model}, "reachable": true, "url": test.Llama.URL}},
		},
		"thinking": "full", "version": "dev", "workspace": test.Workspace,
	}
	Expect(t, body, want)
	if _, public := body.(map[string]any)["publicUrl"]; public {
		t.Fatal("a public address without one set")
	}
}

func TestTheDaemonStopsWhenAskedWithTheToken(t *testing.T) {
	test := startHello(t)
	if status, _, _ := raw(t, test, "POST", protocol.EndpointStop, "", nil); status != 401 {
		t.Fatalf("status %d", status)
	}
	_, body := test.Call("POST", protocol.EndpointStop, nil)
	Expect(t, body, `{"ok":true}`)
	test.WaitForStop()
}

func TestTheDesktopWindowIsToldWhetherClosingWouldStopAnything(t *testing.T) {
	test := startHello(t)
	if status, _, _ := raw(t, test, "GET", protocol.EndpointClosing, "", nil); status != 401 {
		t.Fatalf("status %d", status)
	}
	_, body := test.Call("GET", protocol.EndpointClosing, nil)
	Expect(t, body, `{"idle":true,"warning":""}`)
}

// client is a socket to the daemon, with the messages it receives and a way to wait for the next of a type.
type client struct {
	t          *testing.T
	connection *websocket.Conn
	mu         sync.Mutex
	received   []map[string]any
}

func connect(t *testing.T, test *App) *client {
	t.Helper()
	address := "ws" + strings.TrimPrefix(test.Server.URL, "http") + protocol.EndpointSocket + "?token=" + Token
	connection, _, err := websocket.Dial(context.Background(), address, nil)
	if err != nil {
		t.Fatal(err)
	}
	connection.SetReadLimit(64 << 20)
	socket := &client{connection: connection, t: t}
	go func() {
		for {
			_, data, err := connection.Read(context.Background())
			if err != nil {
				return
			}
			var message map[string]any
			_ = json.Unmarshal(data, &message)
			socket.mu.Lock()
			socket.received = append(socket.received, message)
			socket.mu.Unlock()
		}
	}()
	t.Cleanup(func() { _ = connection.CloseNow() })
	return socket
}

func (c *client) send(message any) {
	data, _ := json.Marshal(message)
	if err := c.connection.Write(context.Background(), websocket.MessageText, data); err != nil {
		c.t.Fatal(err)
	}
}

func (c *client) next(kind string) map[string]any {
	c.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		for index, message := range c.received {
			if message["type"] == kind {
				c.received = append(c.received[:index], c.received[index+1:]...)
				c.mu.Unlock()
				return message
			}
		}
		c.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	c.t.Fatalf("no %s message came", kind)
	return nil
}

func TestTheSocketRefusesAConnectionWithoutTheToken(t *testing.T) {
	test := startHello(t)
	address := "ws" + strings.TrimPrefix(test.Server.URL, "http") + protocol.EndpointSocket
	if _, _, err := websocket.Dial(context.Background(), address, nil); err == nil {
		t.Fatal("connected without a token")
	}
}

func TestTheSocketListsTheSessionsCreatesOneAndRunsATurnInIt(t *testing.T) {
	test := startHello(t)
	socket := connect(t, test)
	Expect(t, socket.next("sessions"), `{"sessions":[],"type":"sessions"}`)
	socket.send(map[string]any{"type": "createSession"})
	created := socket.next("sessionCreated")["session"].(map[string]any)
	socket.send(map[string]any{"sessionId": created["id"], "text": "Hi", "type": "sendPrompt"})
	test.WaitForEvent("turnFinished", 1)
	socket.send(map[string]any{"sessionId": created["id"], "type": "openSession"})
	kinds := []string{}
	for _, event := range socket.next("sessionHistory")["events"].([]any) {
		kinds = append(kinds, event.(map[string]any)["kind"].(string))
	}
	if strings.Join(kinds, " ") != "turnStarted userMessage assistantMessage turnFinished" {
		t.Fatalf("kinds %v", kinds)
	}
}

// historyKinds is the kinds of a history answer's events, in order.
func historyKinds(history map[string]any) string {
	kinds := []string{}
	for _, event := range history["events"].([]any) {
		kinds = append(kinds, event.(map[string]any)["kind"].(string))
	}
	return strings.Join(kinds, " ")
}

func TestAPageBackOnlineGetsOnlyWhatCameAfterTheEventsItHolds(t *testing.T) {
	test := startHello(t)
	socket := connect(t, test)
	socket.send(map[string]any{"type": "createSession"})
	created := socket.next("sessionCreated")["session"].(map[string]any)
	socket.send(map[string]any{"sessionId": created["id"], "text": "Hi", "type": "sendPrompt"})
	test.WaitForEvent("turnFinished", 1)

	socket.send(map[string]any{"sessionId": created["id"], "since": 2, "type": "openSession"})
	since := socket.next("sessionHistory")
	if since["part"] != "since" || since["keptBack"] != float64(2) || historyKinds(since) != "assistantMessage turnFinished" {
		t.Fatalf("part %v, kept back %v, kinds %q", since["part"], since["keptBack"], historyKinds(since))
	}

	// A page that says it holds more events than the session has is another daemon's: it gets the end, to draw.
	socket.send(map[string]any{"events": 3, "sessionId": created["id"], "since": 9, "type": "openSession"})
	end := socket.next("sessionHistory")
	if end["part"] != "end" || end["keptBack"] != float64(1) || historyKinds(end) != "userMessage assistantMessage turnFinished" {
		t.Fatalf("part %v, kept back %v, kinds %q", end["part"], end["keptBack"], historyKinds(end))
	}

	socket.send(map[string]any{"events": 1, "keptBack": 1, "sessionId": created["id"], "type": "olderEvents"})
	if older := socket.next("sessionHistory"); older["part"] != "older" || historyKinds(older) != "turnStarted" {
		t.Fatalf("part %v, kinds %q", older["part"], historyKinds(older))
	}
}

func TestTheSocketListsTheProjectsTheFoldersAndMakesOne(t *testing.T) {
	test := startHello(t)
	socket := connect(t, test)
	Expect(t, socket.next("projects"), `{"projects":[],"type":"projects"}`)
	socket.send(map[string]any{"type": "listFolders"})
	Expect(t, socket.next("folders"), map[string]any{"folders": []any{}, "path": test.Workspace, "type": "folders"})
	socket.send(map[string]any{"folder": test.Workspace, "newFolder": "notes", "type": "createProject"})
	Expect(t, socket.next("projectCreated"), `{"project":{"name":"notes"}}`)
	Expect(t, socket.next("projects"), `{"projects":[{"name":"notes"}]}`)
	socket.send(map[string]any{"folder": test.Workspace, "newFolder": "notes", "type": "createProject"})
	if message := socket.next("projectRefused")["message"].(string); !strings.Contains(message, "already exists") {
		t.Fatalf("message %q", message)
	}
}

func TestTheSocketAnswersAMessageItDoesNotUnderstandWithAProblem(t *testing.T) {
	test := startHello(t)
	socket := connect(t, test)
	socket.send(map[string]any{"type": "launchRocket"})
	socket.next("problem")
}

// fromOrigin sends a request as a page from that origin would: its status, and the origin the daemon lets read
// the answer.
func fromOrigin(t *testing.T, test *App, method, path, origin string, headers map[string]string, body any) (int, string, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request, _ := http.NewRequest(method, test.Server.URL+path, reader)
	request.Header.Set("origin", origin)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, response.Header.Get("access-control-allow-origin"), data
}

func TestAPageFromAnotherSwcodeTalksToThisOneOnceItsPhonePairedFromThere(t *testing.T) {
	test := startHello(t)
	const home, other = "https://laptop.tailnet.ts.net", "https://elsewhere.example"
	asks := map[string]string{"access-control-request-method": "GET", "access-control-request-headers": "authorization"}

	// Before any phone paired from there, a page of that origin may only pair.
	if status, allowed, _ := fromOrigin(t, test, "OPTIONS", protocol.EndpointStatus, home, asks, nil); status != 403 || allowed != "" {
		t.Fatalf("status before pairing: %d, allowed %q", status, allowed)
	}
	if status, allowed, _ := fromOrigin(t, test, "OPTIONS", protocol.EndpointPairing, home, asks, nil); status != 204 || allowed != home {
		t.Fatalf("pairing: %d, allowed %q", status, allowed)
	}
	_, data, _ := raw(t, test, "GET", protocol.EndpointPairingCode, Token, nil)
	var code protocol.PairingCode
	_ = json.Unmarshal(data, &code)
	status, allowed, body := fromOrigin(t, test, "POST", protocol.EndpointPairing, home, nil, map[string]string{"code": code.Code, "name": "iPhone"})
	var paired protocol.PairAnswer
	_ = json.Unmarshal(body, &paired)
	if status != 200 || allowed != home || paired.Token == "" {
		t.Fatalf("pairing from the page: %d, allowed %q", status, allowed)
	}

	// Its origin may now ask, and read what the phone's token opens; no other origin may.
	if status, allowed, _ := fromOrigin(t, test, "OPTIONS", protocol.EndpointStatus, home, asks, nil); status != 204 || allowed != home {
		t.Fatalf("status once paired: %d, allowed %q", status, allowed)
	}
	token := map[string]string{"authorization": protocol.BearerPrefix + paired.Token}
	if status, allowed, _ := fromOrigin(t, test, "GET", protocol.EndpointStatus, home, token, nil); status != 200 || allowed != home {
		t.Fatalf("the status read from the page: %d, allowed %q", status, allowed)
	}
	if status, allowed, _ := fromOrigin(t, test, "GET", protocol.EndpointStatus, home, nil, nil); status != 401 {
		t.Fatalf("without the token: %d, allowed %q", status, allowed)
	}
	if status, allowed, _ := fromOrigin(t, test, "OPTIONS", protocol.EndpointStatus, other, asks, nil); status != 403 || allowed != "" {
		t.Fatalf("another origin: %d, allowed %q", status, allowed)
	}
	if _, allowed, _ := fromOrigin(t, test, "GET", protocol.EndpointStatus, other, token, nil); allowed != "" {
		t.Fatalf("another origin reads the answer: allowed %q", allowed)
	}
}
