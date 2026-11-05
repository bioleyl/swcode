// Package models keeps the llm-servers the agents ask and the presets sessions pick from them: models.json in the
// data folder or, until it is saved once, the settings file's one llm-server (LLAMA_URL and the like).
package models

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"

	"swcode/daemon/config"
	"swcode/daemon/llm"
	"swcode/daemon/protocol"
)

const (
	// File is where the models are kept, in the data folder: it holds the servers' keys, for its owner only.
	File      = "models.json"
	ownerOnly = 0o600
	// The settings file's llm-server, as a source and the preset that uses it.
	settingsSource     = "llama-server"
	settingsPreset     = "default"
	settingsPresetName = "Default"
	// Why models cannot be used.
	unknownSource = "A preset uses an llm-server that is not in the list."
	noDefault     = "The default preset is not in the list."
	badAddress    = "An llm-server’s address is not a web address, as http://127.0.0.1:8080."
	twice         = "Two llm-servers or two presets have the same name."
)

// FromConfig is the settings file's llm-server as a source, and a preset of its models: the models a daemon uses
// until they are saved once.
func FromConfig(settings config.Config) protocol.ModelSettings {
	source := protocol.LlmSource{
		APIKey: settings.LlamaAPIKey, ID: settingsSource, Name: hostOf(settings.LlamaURL),
		SlotShare: shareText(settings.LlamaSlotShare), URL: settings.LlamaURL,
	}
	preset := protocol.Preset{
		ID: settingsPreset, Main: protocol.ModelRef{Model: settings.LlamaModel, Source: settingsSource}, Name: settingsPresetName,
		Workers: protocol.ModelRef{Model: settings.LlamaSubagentModel, Source: settingsSource},
	}
	return protocol.ModelSettings{DefaultPreset: settingsPreset, Presets: []protocol.Preset{preset}, Sources: []protocol.LlmSource{source}}
}

func hostOf(address string) string {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Hostname() == "" {
		return settingsSource
	}
	return parsed.Hostname()
}

func shareText(share config.SlotShare) string {
	if share == config.WholeShare {
		return ""
	}
	return strconv.Itoa(share.Part) + "/" + strconv.Itoa(share.Parts)
}

// Problem is why the models cannot be used, if they cannot: each name given once, addresses that read, each preset's
// models on a source of the list, and the default preset among the presets.
func Problem(settings protocol.ModelSettings) string {
	sources := []string{}
	names := []string{}
	for _, source := range settings.Sources {
		if _, problem := config.ParseSlotShare(source.SlotShare); problem != "" {
			return source.Name + ": " + problem + "."
		}
		if !isWebAddress(source.URL) {
			return badAddress
		}
		if slices.Contains(sources, source.ID) || slices.Contains(names, source.Name) {
			return twice
		}
		sources, names = append(sources, source.ID), append(names, source.Name)
	}
	presets := []string{}
	names = []string{}
	for _, preset := range settings.Presets {
		if !slices.Contains(sources, preset.Main.Source) || !slices.Contains(sources, preset.Workers.Source) {
			return unknownSource
		}
		if slices.Contains(presets, preset.ID) || slices.Contains(names, preset.Name) {
			return twice
		}
		presets, names = append(presets, preset.ID), append(names, preset.Name)
	}
	if !slices.Contains(presets, settings.DefaultPreset) {
		return noDefault
	}
	return ""
}

func isWebAddress(address string) bool {
	parsed, err := url.Parse(address)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

// Store is models.json.
type Store struct {
	file string
}

func NewStore(file string) *Store { return &Store{file: file} }

// Load is the models saved, when they were.
func (s *Store) Load() (protocol.ModelSettings, bool, error) {
	data, err := os.ReadFile(s.file)
	if errors.Is(err, os.ErrNotExist) {
		return protocol.ModelSettings{}, false, nil
	}
	if err != nil {
		return protocol.ModelSettings{}, false, err
	}
	var settings protocol.ModelSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return protocol.ModelSettings{}, false, errors.New(s.file + ": " + err.Error())
	}
	return settings, true, nil
}

func (s *Store) Save(settings protocol.ModelSettings) error {
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.file, data, ownerOnly)
}

// Service is the models in use: it gives the sources to the llm-servers, picks a preset's models for each turn,
// says where each source and preset stands, and tells every open app when the presets change.
type Service struct {
	mu        sync.Mutex
	settings  protocol.ModelSettings
	store     *Store
	servers   *llm.Servers
	listeners map[int]func(protocol.ServerMessage)
	next      int
}

// NewService runs on the models saved, else on fallback, the settings file's.
func NewService(store *Store, servers *llm.Servers, fallback protocol.ModelSettings) (*Service, error) {
	settings, saved, err := store.Load()
	if err != nil {
		return nil, err
	}
	if !saved {
		settings = fallback
	}
	if problem := Problem(settings); problem != "" {
		return nil, errors.New(store.file + ": " + problem)
	}
	servers.Configure(settings.Sources)
	return &Service{listeners: map[int]func(protocol.ServerMessage){}, servers: servers, settings: settings, store: store}, nil
}

// Settings is the models in use, with the servers' keys.
func (s *Service) Settings() protocol.ModelSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// Save keeps new models and uses them at once: a turn running goes on with the models it started with.
func (s *Service) Save(settings protocol.ModelSettings) string {
	if problem := Problem(settings); problem != "" {
		return problem
	}
	if err := s.store.Save(settings); err != nil {
		return err.Error()
	}
	s.mu.Lock()
	s.settings = settings
	s.mu.Unlock()
	s.servers.Configure(settings.Sources)
	s.publish(s.Choices())
	return ""
}

// Choices is the presets as the app picks them: the sources' names, without their addresses and keys.
func (s *Service) Choices() protocol.PresetChoices {
	settings := s.Settings()
	sources := []protocol.PresetChoicesSource{}
	for _, source := range settings.Sources {
		sources = append(sources, protocol.PresetChoicesSource{ID: source.ID, Name: source.Name})
	}
	return protocol.PresetChoices{DefaultPreset: settings.DefaultPreset, Presets: settings.Presets, Sources: sources}
}

// Has tells whether there is a preset of this id.
func (s *Service) Has(preset string) bool {
	_, found := s.presetOf(preset)
	return found
}

// presetOf is the preset of this id, else the default one.
func (s *Service) presetOf(id string) (protocol.Preset, bool) {
	settings := s.Settings()
	fallback := settings.Presets[0]
	for _, preset := range settings.Presets {
		if preset.ID == id {
			return preset, true
		}
		if preset.ID == settings.DefaultPreset {
			fallback = preset
		}
	}
	return fallback, false
}

// Choose is a preset's models as their sources serve them now, the default preset's when it is not there: the main
// agent's, and the workers'. An unnamed main model is a router's first loaded one other than the workers'; unnamed
// workers' model is the main agent's when they share a source.
func (s *Service) Choose(ctx context.Context, presetID string) (llm.Choice, llm.Choice) {
	preset, _ := s.presetOf(presetID)
	main := s.mainOf(ctx, preset)
	workers := llm.Choice{Model: preset.Workers.Model, Source: preset.Workers.Source}
	server, found := s.servers.Server(workers.Source)
	if !found {
		return main, workers
	}
	if workers.Model == "" && workers.Source == main.Source {
		workers.Model = main.Model
	}
	if workers.Model == "" {
		workers.Model = server.Status.FirstLoaded(ctx, "")
	}
	workers.ContextTokens = server.Status.ContextOf(ctx, workers.Model)
	return main, workers
}

func (s *Service) mainOf(ctx context.Context, preset protocol.Preset) llm.Choice {
	main := llm.Choice{Model: preset.Main.Model, Source: preset.Main.Source}
	server, found := s.servers.Server(main.Source)
	if !found {
		return main
	}
	if main.Model == "" {
		avoid := ""
		if preset.Workers.Source == preset.Main.Source {
			avoid = preset.Workers.Model
		}
		main.Model = server.Status.FirstLoaded(ctx, avoid)
	}
	main.ContextTokens = server.Status.ContextOf(ctx, main.Model)
	main.Vision = server.Status.VisionOf(ctx, main.Model)
	return main
}

// Status is whether each source answers and what it serves, and each preset's main model as its source says now:
// a preset on a source that does not answer is not asked further.
func (s *Service) Status(ctx context.Context) protocol.StatusLlama {
	settings := s.Settings()
	sources := []protocol.SourceStatus{}
	served := map[string]protocol.SourceStatus{}
	for _, source := range settings.Sources {
		status := protocol.SourceStatus{ID: source.ID, Models: []string{}, Name: source.Name, URL: source.URL}
		if server, found := s.servers.Server(source.ID); found {
			status.Models, status.Reachable = server.Status.Models(ctx)
		}
		served[source.ID] = status
		sources = append(sources, status)
	}
	presets := []protocol.PresetStatus{}
	for _, preset := range settings.Presets {
		source := served[preset.Main.Source]
		if !source.Reachable {
			presets = append(presets, protocol.PresetStatus{ID: preset.ID})
			continue
		}
		main := s.mainOf(ctx, preset)
		presets = append(presets, protocol.PresetStatus{
			ContextTokens: main.ContextTokens, ID: preset.ID, Model: shownModel(main.Model, source.Models), Vision: main.Vision,
		})
	}
	return protocol.StatusLlama{Presets: presets, Sources: sources}
}

// shownModel names the model asked: a single-model llm-server's, asked for by no name, is the one it lists.
func shownModel(asked string, served []string) string {
	if asked != "" || len(served) == 0 {
		return asked
	}
	return served[0]
}

// Subscribe tells listener the presets each time they change, until the returned function is called.
func (s *Service) Subscribe(listener func(protocol.ServerMessage)) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.next
	s.next++
	s.listeners[id] = listener
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.listeners, id)
	}
}

func (s *Service) publish(choices protocol.PresetChoices) {
	s.mu.Lock()
	listeners := []func(protocol.ServerMessage){}
	for _, listener := range s.listeners {
		listeners = append(listeners, listener)
	}
	s.mu.Unlock()
	message := protocol.ServerMessage{Value: protocol.ServerMessagePresets{Choices: choices}}
	for _, listener := range listeners {
		listener(message)
	}
}
