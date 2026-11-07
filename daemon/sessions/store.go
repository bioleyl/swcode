// Package sessions is the daemon's sessions: each one is its list of events, kept in a file, and the turns it runs.
package sessions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"swcode/daemon/protocol"
)

const (
	metaSuffix   = ".json"
	eventsSuffix = ".jsonl"
)

// Meta is what a session is: its folder on its host, its title, and how it asks and thinks.
type Meta struct {
	Concise   bool                    `json:"concise,omitempty"`
	CreatedAt string                  `json:"createdAt"`
	Host      protocol.HostRef        `json:"host"`
	ID        string                  `json:"id"`
	Kind      protocol.SessionKind    `json:"kind"`
	Mode      protocol.PermissionMode `json:"mode"`
	Preset    string                  `json:"preset,omitempty"`
	// SeenAt is the session's last activity the person saw, on any device; empty in a session from before it was kept.
	SeenAt    string                `json:"seenAt,omitempty"`
	Thinking  protocol.ThinkingMode `json:"thinking"`
	Title     string                `json:"title"`
	Workspace string                `json:"workspace"`
}

// UnmarshalJSON reads sessions from before hosts as on this computer, from before the choice as thinking as the
// model does by itself, and from before chats as coding sessions.
func (m *Meta) UnmarshalJSON(data []byte) error {
	type plain Meta
	read := plain{Host: protocol.LocalHost(), Kind: protocol.SessionKindCode, Thinking: protocol.ThinkingModeFull}
	if err := json.Unmarshal(data, &read); err != nil {
		return err
	}
	*m = Meta(read)
	return nil
}

// Stored is a session as its files keep it.
type Stored struct {
	Meta   Meta
	Events []protocol.SessionEvent
}

// StorePort keeps the sessions.
type StorePort interface {
	LoadAll() []Stored
	SaveMeta(meta Meta) error
	Append(sessionID string, event protocol.SessionEvent) error
	Remove(sessionID string)
}

// Store keeps each session in two files: <id>.json for what it is, <id>.jsonl for what happened, one event a line.
type Store struct {
	folder string
}

func NewStore(folder string) *Store {
	_ = os.MkdirAll(folder, 0o755)
	return &Store{folder: folder}
}

func (s *Store) LoadAll() []Stored {
	entries, err := os.ReadDir(s.folder)
	if err != nil {
		return nil
	}
	stored := []Stored{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), metaSuffix) {
			continue
		}
		if session, ok := s.load(entry.Name()); ok {
			stored = append(stored, session)
		}
	}
	return stored
}

func (s *Store) load(fileName string) (Stored, bool) {
	data, err := os.ReadFile(filepath.Join(s.folder, fileName))
	if err != nil {
		return Stored{}, false
	}
	var meta Meta
	if json.Unmarshal(data, &meta) != nil || meta.ID == "" {
		return Stored{}, false
	}
	events, _ := os.ReadFile(s.eventsPath(meta.ID))
	return Stored{Events: eventsOf(events), Meta: meta}, true
}

// eventsOf reads the events, one a line: a line cut short by a crash is skipped, not the whole session.
func eventsOf(data []byte) []protocol.SessionEvent {
	events := []protocol.SessionEvent{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64<<10), 256<<20)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event protocol.SessionEvent
		if json.Unmarshal(line, &event) == nil {
			events = append(events, event)
		}
	}
	return events
}

func (s *Store) SaveMeta(meta Meta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.metaPath(meta.ID), data, 0o644)
}

// Append adds an event to a session's file: the event is the one it holds, so its kind and its time come from it.
func (s *Store) Append(sessionID string, event protocol.SessionEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(s.eventsPath(sessionID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(data, '\n'))
	return err
}

func (s *Store) Remove(sessionID string) {
	_ = os.Remove(s.metaPath(sessionID))
	_ = os.Remove(s.eventsPath(sessionID))
}

func (s *Store) metaPath(sessionID string) string {
	return filepath.Join(s.folder, sessionID+metaSuffix)
}

func (s *Store) eventsPath(sessionID string) string {
	return filepath.Join(s.folder, sessionID+eventsSuffix)
}
