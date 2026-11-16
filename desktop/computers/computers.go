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

	noAddress    = "Type that computer's address: its name or IP address, and the port when it is not 4500."
	noComputer   = "That computer is not kept here."
	noToken      = "That computer has to be paired first: its app gives this one a token."
	tokenRefused = "That computer's swcode refused the token this app paired with."
	notSwcode    = "%s answered, but not as swcode does."
	unreachable  = "%s did not answer: is swcode running there, and listening on the network (SWCODE_HOST)?"

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

// AddressOf reads an address as a person types it: a name or an IP address, with or without the scheme and the port
// (http and 4500 when left out).
func AddressOf(typed string) (string, error) {
	text := strings.TrimSpace(typed)
	if text == "" {
		return "", errors.New(noAddress)
	}
	if !strings.Contains(text, schemeMark) {
		text = plainScheme + schemeMark + text
	}
	link, err := url.Parse(text)
	if err != nil || link.Hostname() == "" {
		return "", errors.New(noAddress)
	}
	host := link.Host
	if link.Scheme == plainScheme && link.Port() == "" {
		host = net.JoinHostPort(link.Hostname(), strconv.Itoa(config.DefaultPort))
	}
	address := url.URL{Host: host, Scheme: link.Scheme}
	return address.String(), nil
}

// Connect reaches another computer's swcode with the token its pairing gave the page: it checks that swcode answers
// with that token, and keeps the computer under the name the person gave it.
func Connect(ctx context.Context, client *http.Client, typed, name, token string) (Computer, error) {
	address, err := AddressOf(typed)
	if err != nil {
		return Computer{}, err
	}
	if token == "" {
		return Computer{}, errors.New(noToken)
	}
	if err := check(ctx, client, address, token); err != nil {
		return Computer{}, err
	}
	if name == "" {
		name = nameOf(address)
	}
	return Computer{Address: address, ID: newID(), Name: name, Token: token}, nil
}

// nameOf is an address's own name, for a computer the person gave none: its host, without the port.
func nameOf(address string) string {
	link, err := url.Parse(address)
	if err != nil {
		return address
	}
	return link.Hostname()
}

// check asks the daemon's status with a token: whether that swcode takes it.
func check(ctx context.Context, client *http.Client, address, token string) error {
	status, _, err := call(ctx, client, http.MethodGet, address+protocol.EndpointStatus, token, nil)
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized {
		return errors.New(tokenRefused)
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
