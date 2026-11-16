package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"swcode/daemon/app"
	"swcode/daemon/config"
	"swcode/daemon/protocol"
)

const (
	notRunning       = "swcode does not run on this computer: start it first."
	pairingRefused   = "swcode refused this computer's token: did its settings change?"
	millisPerSecond  = 1000
	secondsPerMinute = 60
	noNetworkLink    = "  (none: swcode listens on this computer only, SWCODE_HOST)"
)

// Expecting is a pairing number made to wait for the app that shows it, and the addresses this computer is reached at.
type Expecting struct {
	Number     string
	LifetimeMs int
	Links      []string
}

// said is what the daemon answers a request it refuses.
type said struct {
	Message string `json:"message"`
}

// Expect asks this computer's daemon to wait for the app that shows number: the daemon must run.
func Expect(settings config.Config, number string) (Expecting, error) {
	token, err := app.AccessToken(settings.DataDir, settings.Token)
	if err != nil {
		return Expecting{}, err
	}
	body, _ := json.Marshal(protocol.PairPendingRequest{Number: number})
	ctx, cancel := context.WithTimeout(context.Background(), answerLimit)
	defer cancel()
	base := app.ControlURL(settings.Host, settings.Port)
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+protocol.EndpointPairingPending, bytes.NewReader(body))
	request.Header.Set("authorization", protocol.BearerPrefix+token)
	request.Header.Set("content-type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return Expecting{}, errors.New(notRunning)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		var refused said
		if json.Unmarshal(data, &refused) == nil && refused.Message != "" {
			return Expecting{}, errors.New(refused.Message)
		}
		return Expecting{}, errors.New(pairingRefused)
	}
	var pending protocol.PairPending
	if json.Unmarshal(data, &pending) != nil {
		return Expecting{}, errors.New(pairingRefused)
	}
	links := app.LANURLs(settings.Host, settings.Port, app.SystemInterfaces())
	if settings.PublicURL != "" {
		links = append([]string{settings.PublicURL}, links...)
	}
	return Expecting{Number: number, LifetimeMs: pending.LifetimeMs, Links: links}, nil
}

// PairingText says what a waiting number does, and the addresses to type on the device that shows it.
func PairingText(expecting Expecting) string {
	links := strings.Join(expecting.Links, "\n  ")
	if len(expecting.Links) == 0 {
		links = noNetworkLink
	}
	return fmt.Sprintf(
		"Pairing number %s waits for %d minutes.\n\nThe device that shows it pairs with this swcode, reached at:\n  %s",
		expecting.Number, expecting.LifetimeMs/millisPerSecond/secondsPerMinute, links,
	)
}

// Pair makes the number an app shows wait on this computer's swcode: swcode pair <number>. The daemon starts in the
// background when it does not run, since the app has to reach it.
func Pair(settings config.Config, place Place, number string) int {
	control := app.ControlURL(settings.Host, settings.Port)
	if !IsUp(control) && !(startDaemon(place) && WaitFor(func() bool { return IsUp(control) }, startLimit)) {
		fmt.Fprintf(os.Stderr, "swcode did not start: its log is %s.\n", LogFileOf(settings))
		return failed
	}
	expecting, err := Expect(settings, number)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	fmt.Println(PairingText(expecting))
	return 0
}
