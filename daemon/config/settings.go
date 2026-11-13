package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"swcode/daemon/protocol"
)

const (
	SettingsFile = "settings.env"
	// The settings hold the keys of the llm-servers and may hold swcode's own token: they are the person's alone.
	settingsFolderMode = 0o700
	settingsFileMode   = 0o600
	tokenSetting       = "SWCODE_TOKEN"
	singleQuote        = `'`
	doubleQuote        = `"`
	backQuote          = "`"
	commentMark        = "#"
	exportPrefix       = "export "
	escapedNewline     = `\n`
	settingsHeader     = "# swcode's settings: the settings page writes them, the README lists them. The environment's own come on top."
)

var plainValue = regexp.MustCompile(`^[\w./:~@+-]*$`)

// SavedSettings are what a settings file keeps: the llm-server from before models.json, which the models start
// from until they are saved once, what the settings page changes, and the access token when one was given.
var SavedSettings = append(append(
	[]string{envLlamaURL, envLlamaAPIKey, envLlamaModel, envLlamaSubagent},
	settingNames(protocol.EditableSettings)...), tokenSetting)

func settingNames(names []protocol.SettingName) []string {
	texts := make([]string, len(names))
	for index, name := range names {
		texts[index] = string(name)
	}
	return texts
}

// quoted writes a value: single quotes keep backslashes as they are (Windows paths: in double quotes, \n would be
// a line break); a value with a single quote of its own gets double quotes.
func quoted(value string) string {
	if plainValue.MatchString(value) {
		return value
	}
	if strings.Contains(value, singleQuote) {
		return doubleQuote + value + doubleQuote
	}
	return singleQuote + value + singleQuote
}

// DataDirOf is the data folder, where the settings file is: SWCODE_DATA_DIR, else ~/.swcode.
func DataDirOf(environment map[string]string, home string) string {
	if dir := environment[envDataDir]; dir != "" {
		return dir
	}
	return filepath.Join(home, DataFolder)
}

// SettingsPathOf is where the settings file of this environment is.
func SettingsPathOf(environment map[string]string, home string) string {
	return filepath.Join(DataDirOf(environment, home), SettingsFile)
}

// ReadSettings reads the settings a file keeps, by name: none when there is no file.
func ReadSettings(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	return ParseEnv(string(data))
}

// ParseEnv reads `KEY=value` lines as Node's parseEnv does: comments, `export`, and quoted values, double quotes
// reading \n as a line break, single quotes keeping every character.
func ParseEnv(text string) map[string]string {
	values := map[string]string{}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if line == "" || strings.HasPrefix(line, commentMark) {
			continue
		}
		key, value, found := strings.Cut(strings.TrimPrefix(line, exportPrefix), "=")
		if !found {
			continue
		}
		value, index = valueFrom(strings.TrimSpace(value), lines, index)
		values[strings.TrimSpace(key)] = value
	}
	return values
}

// valueFrom reads a value from where it starts: a quoted one may go on over the next lines.
func valueFrom(value string, lines []string, index int) (string, int) {
	quote := quoteOf(value)
	if quote == "" {
		comment := strings.Index(value, " "+commentMark)
		if comment >= 0 {
			value = value[:comment]
		}
		return strings.TrimSpace(value), index
	}
	text := value[1:]
	for !strings.Contains(text, quote) && index+1 < len(lines) {
		index++
		text += "\n" + lines[index]
	}
	end := strings.Index(text, quote)
	if end < 0 {
		return strings.TrimSpace(value), index
	}
	inner := text[:end]
	if quote == doubleQuote {
		inner = strings.ReplaceAll(inner, escapedNewline, "\n")
	}
	return inner, index
}

func quoteOf(value string) string {
	for _, quote := range []string{singleQuote, doubleQuote, backQuote} {
		if strings.HasPrefix(value, quote) {
			return quote
		}
	}
	return ""
}

// SettingsText is a settings file's text: the settings it keeps that have a value, one a line.
func SettingsText(values map[string]string) string {
	lines := []string{settingsHeader}
	for _, name := range SavedSettings {
		value := strings.TrimSpace(values[name])
		if value != "" {
			lines = append(lines, name+"="+quoted(value))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// WriteSettings writes a settings file, its folder made first.
func WriteSettings(path string, values map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), settingsFolderMode); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(SettingsText(values)), settingsFileMode)
}

// Environment is what the daemon starts with: the settings file's (`KEY=value` lines, as `swcode install` writes
// them), the environment's own on top, so one command can change one for once.
func Environment(environment map[string]string, home string) map[string]string {
	merged := ReadSettings(SettingsPathOf(environment, home))
	for name, value := range environment {
		merged[name] = value
	}
	return merged
}

// ProcessEnvironment is this process's environment, by name.
func ProcessEnvironment() map[string]string {
	environment := map[string]string{}
	for _, entry := range os.Environ() {
		name, value, _ := strings.Cut(entry, "=")
		environment[name] = value
	}
	return environment
}

const (
	// DevicesFile is the paired phones and computers, in the data folder.
	DevicesFile = "devices.json"
	// listenMarker says an install's address was looked at once, now that the default is this computer alone.
	listenMarker = "listen-checked"
)

// hasDevices tells whether phones or computers are paired with the daemon of a data folder.
func hasDevices(dataDir string) bool {
	data, err := os.ReadFile(filepath.Join(dataDir, DevicesFile))
	var devices []json.RawMessage
	return err == nil && json.Unmarshal(data, &devices) == nil && len(devices) > 0
}

// ListenAsBefore keeps an install from before swcode listened on its own computer alone reachable by what was
// paired with it. Looked at once, at its first start since: with phones or computers paired and no address chosen,
// its settings now choose every interface, which was the default; a choice of this computer says what the default
// now says, and its line goes. An install nobody paired with becomes this computer's alone.
func ListenAsBefore(environment map[string]string, home string) error {
	dataDir := DataDirOf(environment, home)
	marker := filepath.Join(dataDir, listenMarker)
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	path := SettingsPathOf(environment, home)
	values := ReadSettings(path)
	chosen, set := values[envHost]
	switch {
	case set && chosen == defaultHost:
		delete(values, envHost)
	case !set && environment[envHost] == "" && hasDevices(dataDir):
		values[envHost] = everyInterface
	default:
		return os.WriteFile(marker, nil, settingsFileMode)
	}
	if err := WriteSettings(path, values); err != nil {
		return err
	}
	return os.WriteFile(marker, nil, settingsFileMode)
}
