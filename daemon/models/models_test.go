package models

import (
	"testing"

	"swcode/daemon/config"
	"swcode/daemon/protocol"
)

func TestTheSettingsFilesLlamaServerIsASourceAndAPresetOfItsModels(t *testing.T) {
	settings := FromConfig(config.Config{
		LlamaAPIKey: "key", LlamaModel: "big", LlamaSubagentModel: "small", LlamaURL: "https://garfield.example:8443",
	})
	source, preset := settings.Sources[0], settings.Presets[0]
	if source.Name != "garfield.example" || source.APIKey != "key" {
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
		return FromConfig(config.Config{LlamaURL: "http://127.0.0.1:8080"})
	}
	cases := map[string]func(*protocol.ModelSettings){
		badAddress:    func(settings *protocol.ModelSettings) { settings.Sources[0].URL = "garfield" },
		noDefault:     func(settings *protocol.ModelSettings) { settings.DefaultPreset = "gone" },
		unknownSource: func(settings *protocol.ModelSettings) { settings.Presets[0].Workers.Source = "gone" },
		twice: func(settings *protocol.ModelSettings) {
			settings.Presets = append(settings.Presets, protocol.Preset{ID: "other", Main: settings.Presets[0].Main, Name: settings.Presets[0].Name, Workers: settings.Presets[0].Workers})
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

func TestAPresetsContextIsTheServersAtMostAndItsOwnWhenTheServerSaysNone(t *testing.T) {
	cases := []struct{ served, asked, want int }{
		{131_072, 0, 131_072},
		{131_072, 65_536, 65_536},
		{65_536, 131_072, 65_536},
		{0, 32_768, 32_768},
		{0, 0, 0},
	}
	for _, each := range cases {
		if got := contextFor(each.served, each.asked); got != each.want {
			t.Errorf("served %d, asked %d: got %d", each.served, each.asked, got)
		}
	}
}
