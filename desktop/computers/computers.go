// Package computers keeps the other computers the desktop app shows besides its own: their swcode's address, and the
// token pairing gave this app, in a file of the app's folder. Their daemons work on whether this computer is on.
package computers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"swcode/daemon/config"
	"swcode/daemon/protocol"
)

const (
	plainScheme  = "http"
	schemeMark   = "://"
	idBytes      = 8
	answerLimit  = 5 * time.Second
	maxAnswer    = 1 << 16
	folderMode   = 0o755
	ownerOnly    = 0o600
	bearerHeader = "authorization"

	noAddress   = "Type that computer's address: its name or IP address, and the port when it is not 4500."
	noComputer  = "That computer is not kept here."
	noCode      = "Type the code that computer's swcode shows to pair a device, or paste its link with the token."
	codeRefused = "The code was refused: it works once, for a minute. Ask that computer's swcode for a new one."
	linkRefused = "That computer's swcode refused the link's token."
	notSwcode   = "%s answered, but not as swcode does."
	unreachable = "%s did not answer: is swcode running there, and listening on the network (SWCODE_HOST)?"

	// probeLimit is how long swcode at another computer has to answer before the app says it does not.
	probeLimit = 2 * time.Second
)

// Computer is another computer's swcode, as this app reaches it.
type Computer struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Color   protocol.ComputerColor `json:"color,omitempty"`
	Address string                 `json:"address"`
	Token   string                 `json:"token"`
}

// Store is the computers this app knows, and which one its window showed last, kept in a file.
type Store struct {
	file string
	kept kept
}

type kept struct {
	Current   string     `json:"current"`
	Computers []Computer `json:"computers"`
}

// Load reads the computers kept in file: none when it is not there yet.
func Load(file string) *Store {
	store := &Store{file: file, kept: kept{Computers: []Computer{}, Current: protocol.ThisComputer}}
	if data, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(data, &store.kept)
	}
	return store
}

func (s *Store) All() []Computer { return slices.Clone(s.kept.Computers) }

func (s *Store) Find(id string) (Computer, bool) {
	index := slices.IndexFunc(s.kept.Computers, func(computer Computer) bool { return computer.ID == id })
	if index < 0 {
		return Computer{}, false
	}
	return s.kept.Computers[index], true
}

// Current is the computer the window showed last: this one, or another that is still kept.
func (s *Store) Current() string {
	if _, known := s.Find(s.kept.Current); known {
		return s.kept.Current
	}
	return protocol.ThisComputer
}

func (s *Store) SetCurrent(id string) error {
	s.kept.Current = id
	return s.save()
}

// Add keeps a computer, in place of one at the same address: pairing again gives it a new token.
func (s *Store) Add(computer Computer) error {
	s.kept.Computers = slices.DeleteFunc(s.kept.Computers, func(known Computer) bool { return known.Address == computer.Address })
	s.kept.Computers = append(s.kept.Computers, computer)
	return s.save()
}

// Edit gives a kept computer the name and colour the person chose: its address and token stay.
func (s *Store) Edit(id, name string, color protocol.ComputerColor) error {
	index := slices.IndexFunc(s.kept.Computers, func(known Computer) bool { return known.ID == id })
	if index < 0 {
		return errors.New(noComputer)
	}
	s.kept.Computers[index].Name = name
	s.kept.Computers[index].Color = color
	return s.save()
}

func (s *Store) Forget(id string) error {
	s.kept.Computers = slices.DeleteFunc(s.kept.Computers, func(known Computer) bool { return known.ID == id })
	return s.save()
}

// save writes the file for its person alone: it holds the tokens.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.kept, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.file), folderMode); err != nil {
		return err
	}
	return os.WriteFile(s.file, data, ownerOnly)
}

// Target is what a person typed to reach another computer's swcode: its address, its name, and the token when they
// pasted a link that carries one.
type Target struct {
	Address string
	Name    string
	Token   string
}

// TargetOf reads an address as a person types it: a name or an IP address, with or without the scheme and the port
// (http and 4500 when left out), or a whole link from that swcode, its token included.
func TargetOf(typed string) (Target, error) {
	text := strings.TrimSpace(typed)
	if text == "" {
		return Target{}, errors.New(noAddress)
	}
	if !strings.Contains(text, schemeMark) {
		text = plainScheme + schemeMark + text
	}
	link, err := url.Parse(text)
	if err != nil || link.Hostname() == "" {
		return Target{}, errors.New(noAddress)
	}
	host := link.Host
	if link.Scheme == plainScheme && link.Port() == "" {
		host = net.JoinHostPort(link.Hostname(), strconv.Itoa(config.DefaultPort))
	}
	address := url.URL{Host: host, Scheme: link.Scheme}
	return Target{Address: address.String(), Name: link.Hostname(), Token: link.Query().Get(protocol.TokenParam)}, nil
}

// Connect reaches another computer's swcode: it trades the code for a token of this app's own, named after this
// computer in that swcode's list of devices, or checks the token of the link it was given.
func Connect(ctx context.Context, client *http.Client, typed, code, deviceName string) (Computer, error) {
	target, err := TargetOf(typed)
	if err != nil {
		return Computer{}, err
	}
	token, err := tokenFor(ctx, client, target, strings.TrimSpace(code), deviceName)
	if err != nil {
		return Computer{}, err
	}
	return Computer{Address: target.Address, ID: newID(), Name: target.Name, Token: token}, nil
}

func tokenFor(ctx context.Context, client *http.Client, target Target, code, deviceName string) (string, error) {
	if target.Token != "" {
		return target.Token, check(ctx, client, target.Address, target.Token)
	}
	if code == "" {
		return "", errors.New(noCode)
	}
	return pair(ctx, client, target.Address, code, deviceName)
}

// pair trades a pairing code for a token, as a phone does.
func pair(ctx context.Context, client *http.Client, address, code, deviceName string) (string, error) {
	body, _ := json.Marshal(protocol.PairRequest{Code: code, Name: deviceName})
	status, answer, err := call(ctx, client, http.MethodPost, address+protocol.EndpointPairing, "", body)
	if err != nil {
		return "", err
	}
	if status == http.StatusForbidden || status == http.StatusBadRequest {
		return "", errors.New(codeRefused)
	}
	var paired protocol.PairAnswer
	if status != http.StatusOK || json.Unmarshal(answer, &paired) != nil || paired.Token == "" {
		return "", fmt.Errorf(notSwcode, address)
	}
	return paired.Token, nil
}

// check asks the daemon's status with a token: whether that swcode takes it.
func check(ctx context.Context, client *http.Client, address, token string) error {
	status, _, err := call(ctx, client, http.MethodGet, address+protocol.EndpointStatus, token, nil)
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized {
		return errors.New(linkRefused)
	}
	if status != http.StatusOK {
		return fmt.Errorf(notSwcode, address)
	}
	return nil
}

// Answers tells whether swcode at another computer answers within probeLimit: its health route, which needs no
// token. The window asks before it shows that computer, whose swcode may be off.
func Answers(ctx context.Context, client *http.Client, address string) bool {
	ctx, cancel := context.WithTimeout(ctx, probeLimit)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address+protocol.EndpointHealth, nil)
	if err != nil {
		return false
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func call(ctx context.Context, client *http.Client, method, target, token string, body []byte) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, answerLimit)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	if token != "" {
		request.Header.Set(bearerHeader, protocol.BearerPrefix+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, fmt.Errorf(unreachable, request.URL.Host)
	}
	defer response.Body.Close()
	answer, _ := io.ReadAll(io.LimitReader(response.Body, maxAnswer))
	return response.StatusCode, answer, nil
}

func newID() string {
	random := make([]byte, idBytes)
	_, _ = rand.Read(random)
	return hex.EncodeToString(random)
}
