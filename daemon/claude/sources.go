package claude

import (
	"encoding/json"
	"os"
	"path/filepath"

	"swcode/daemon/protocol"
)

// SourcesStore keeps which of Claude Code's skills swcode reads, in one file, claude.json in the data folder: all,
// at first.
type SourcesStore struct {
	file string
}

func NewSourcesStore(file string) *SourcesStore { return &SourcesStore{file: file} }

// AllSources is every source on, each plugin as Claude Code has it.
func AllSources() protocol.ClaudeSources {
	var sources protocol.ClaudeSources
	_ = json.Unmarshal([]byte("{}"), &sources)
	return sources
}

func (s *SourcesStore) Load() protocol.ClaudeSources {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return AllSources()
	}
	var sources protocol.ClaudeSources
	if json.Unmarshal(data, &sources) != nil {
		return AllSources()
	}
	return sources
}

func (s *SourcesStore) Save(sources protocol.ClaudeSources) error {
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.file, data, 0o644)
}
