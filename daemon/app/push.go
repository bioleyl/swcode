package app

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync"

	"swcode/daemon/protocol"
	"swcode/daemon/push"
	"swcode/desktop/watch"
)

const (
	// pushContact is who a push service may write to about this sender: swcode's own page.
	pushContact = "https://github.com/bioleyl/swcode"
	noPush      = "This swcode sends no push notifications."
)

// startPush makes the daemon's push key at its first start, and tells the browsers that subscribed what the desktop
// app's notifications tell: a session waits for its person, ended its turn, or failed. A daemon that cannot keep
// its key sends none, and says so in its log.
func (a *App) startPush(logError func(error)) {
	service, err := push.New(filepath.Join(a.config.DataDir, push.File), a.options.HTTP, pushContact, a.options.Now, logError)
	if err != nil {
		logError(err)
		return
	}
	a.push = service
	// The sessions tell one listener at a time, but nothing says they always will.
	var mu sync.Mutex
	tracker := watch.NewTracker()
	a.Sessions.Subscribe(func(message protocol.ServerMessage) {
		mu.Lock()
		notice, told := tracker.Take(message)
		mu.Unlock()
		if !told || !service.Any() {
			return
		}
		payload, err := json.Marshal(protocol.PushNotice{Body: notice.Text, SessionID: notice.SessionID, Title: notice.Title})
		if err != nil {
			return
		}
		// The sessions wait for no push service.
		go service.Send(context.Background(), payload)
	})
}

// pushRoutes give a browser the daemon's push key, keep its subscription, and let it go.
func (a *App) pushRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+protocol.EndpointPush, func(response http.ResponseWriter, _ *http.Request) {
		if a.push == nil {
			fail(response, http.StatusServiceUnavailable, noPush)
			return
		}
		answer(response, protocol.PushKey{Key: a.push.PublicKey()})
	})
	mux.HandleFunc("POST "+protocol.EndpointPush, func(response http.ResponseWriter, request *http.Request) {
		var body protocol.PushSubscription
		if a.push == nil || readBody(response, request, &body) != nil {
			fail(response, http.StatusBadRequest, noPush)
			return
		}
		// A paired phone's subscription goes with the phone when it is forgotten.
		device, _ := a.pairing.DeviceOf(presentedToken(request))
		problem := a.push.Subscribe(push.Subscription{
			Device: device, Endpoint: body.Endpoint, Keys: push.Keys{Auth: body.Keys.Auth, P256dh: body.Keys.P256dh},
		})
		if problem != "" {
			fail(response, http.StatusBadRequest, problem)
			return
		}
		answer(response, map[string]bool{"ok": true})
	})
	mux.HandleFunc("DELETE "+protocol.EndpointPush, func(response http.ResponseWriter, request *http.Request) {
		var body protocol.PushRemoval
		if a.push == nil || readBody(response, request, &body) != nil {
			fail(response, http.StatusBadRequest, noPush)
			return
		}
		a.push.Unsubscribe(body.Endpoint)
		answer(response, map[string]bool{"ok": true})
	})
}

// forgetPushOf forgets the browsers of a paired phone, with the phone.
func (a *App) forgetPushOf(device string) {
	if a.push != nil {
		a.push.ForgetDevice(device)
	}
}
