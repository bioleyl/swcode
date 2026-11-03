package models

import (
	"testing"

	"swcode/daemon/config"
	"swcode/daemon/protocol"
)

func TestTheSettingsFilesLlamaServerIsASourceAndAPresetOfItsModels(t *testing.T) {
	settings := FromConfig(config.Config{
		LlamaAPIKey: "key", LlamaModel: "big", LlamaSlotShare: config.SlotShare{Part: 2, Parts: 2},
		LlamaSubagentModel: "small", LlamaURL: "https://garfield.example:8443",
	})
	source, preset := settings.Sources[0], settings.Presets[0]
	if source.Name != "garfield.example" || source.SlotShare != "2/2" || source.APIKey != "key" {
		t.Fatalf("source %+v", source)
	}
	if preset.Main.Model != "big" || preset.Workers.Model != "small" || settings.DefaultPreset != preset.ID {
		t.Fatalf("preset %+v", preset)
	}
	if problem := Problem(settings); problem != "" {
		t.Fatal(problem)
	}
}

func TestModelsThatCannotWorkSayWhy(t *testing.T) {
	valid := func() protocol.ModelSettings {
		return FromConfig(config.Config{LlamaSlotShare: config.WholeShare, LlamaURL: "http://127.0.0.1:8080"})
	}
	cases := map[string]func(*protocol.ModelSettings){
		badAddress:    func(settings *protocol.ModelSettings) { settings.Sources[0].URL = "garfield" },
		noDefault:     func(settings *protocol.ModelSettings) { settings.DefaultPreset = "gone" },
		unknownSource: func(settings *protocol.ModelSettings) { settings.Presets[0].Workers.Source = "gone" },
		twice: func(settings *protocol.ModelSettings) {
			settings.Presets = append(settings.Presets, protocol.Preset{ID: "other", Main: settings.Presets[0].Main, Name: settings.Presets[0].Name, Workers: settings.Presets[0].Workers})
		},
		"127.0.0.1: the part cannot be more than the parts, as in 3/2.": func(settings *protocol.ModelSettings) {
			settings.Sources[0].SlotShare = "3/2"
		},
	}
	for want, change := range cases {
		settings := valid()
		change(&settings)
		if got := Problem(settings); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
