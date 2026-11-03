package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"swcode/daemon/app"
	"swcode/daemon/config"
	"swcode/daemon/protocol"
)

const (
	notRunning      = "swcode does not run on this computer: start it first."
	pairingRefused  = "swcode refused this computer's token: did its settings change?"
	millisPerSecond = 1000
	noNetworkLink   = "  (none: swcode listens on this computer only, SWCODE_HOST)"
)

// Pairing is what pairs another computer, or a phone, with this computer's swcode: a code, how long it works, and
// the addresses it is reached at.
type Pairing struct {
	Code       string
	LifetimeMs int
	Links      []string
}

// PairingOf asks this computer's daemon for a pairing code: the daemon must run.
func PairingOf(settings config.Config) (Pairing, error) {
	token, err := app.AccessToken(settings.DataDir, settings.Token)
	if err != nil {
		return Pairing{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), answerLimit)
	defer cancel()
	base := app.ControlURL(settings.Host, settings.Port)
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+protocol.EndpointPairingCode, nil)
	request.Header.Set("authorization", protocol.BearerPrefix+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return Pairing{}, errors.New(notRunning)
	}
	defer response.Body.Close()
	var issued protocol.PairingCode
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&issued) != nil {
		return Pairing{}, errors.New(pairingRefused)
	}
	links := app.LANURLs(settings.Host, settings.Port, app.SystemInterfaces())
	if settings.PublicURL != "" {
		links = append([]string{settings.PublicURL}, links...)
	}
	return Pairing{Code: issued.Code, LifetimeMs: issued.LifetimeMs, Links: links}, nil
}

// PairingText says how to pair: the code, then the addresses to type with it on the other computer.
func PairingText(pairing Pairing) string {
	links := strings.Join(pairing.Links, "\n  ")
	if len(pairing.Links) == 0 {
		links = noNetworkLink
	}
	return fmt.Sprintf(
		"Pairing code: %s (it works once, for %d seconds)\n\nOn the other computer, add this one with the code, at:\n  %s",
		pairing.Code, pairing.LifetimeMs/millisPerSecond, links,
	)
}

// Pair prints a pairing code for this computer's swcode: swcode pair.
func Pair(settings config.Config) int {
	pairing, err := PairingOf(settings)
	if err != nil {
		fmt.Println(err)
		return failed
	}
	fmt.Println(PairingText(pairing))
	return 0
}
