package local

import (
	"context"
	"net/http"
	"os"
	"time"

	"swcode/daemon/protocol"
)

const (
	answerLimit = time.Second
	pollEvery   = 250 * time.Millisecond
)

// StopAnswer is how the daemon took a request to stop.
type StopAnswer string

const (
	StopAsked      StopAnswer = "asked"
	StopNotRunning StopAnswer = "notRunning"
	StopRefused    StopAnswer = "refused"
	StopOlder      StopAnswer = "older"
)

// IsUp tells whether the daemon answers at this address.
func IsUp(base string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), answerLimit)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+protocol.EndpointHealth, nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK
}

// WaitFor asks again until the answer is yes, or the time is up.
func WaitFor(check func() bool, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if check() {
			return true
		}
		time.Sleep(pollEvery)
	}
	return false
}

// StartInBackground starts the daemon in the background: through a hidden wsl.exe for the WSL program, else as a
// detached process.
func StartInBackground(place Place) bool {
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	if place.WSLDistro != "" {
		return Run(WSLDaemonCall(place.WSLDistro, executable)).OK
	}
	return StartDetached(DetachedDaemonCall(executable)) == nil
}

// AskToStop asks the daemon to stop: it answers, then closes. An older build has no such request.
func AskToStop(base, token string) StopAnswer {
	ctx, cancel := context.WithTimeout(context.Background(), answerLimit)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+protocol.EndpointStop, nil)
	request.Header.Set("authorization", protocol.BearerPrefix+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return StopNotRunning
	}
	response.Body.Close()
	switch {
	case response.StatusCode == http.StatusUnauthorized:
		return StopRefused
	case response.StatusCode == http.StatusOK:
		return StopAsked
	}
	return StopOlder
}
