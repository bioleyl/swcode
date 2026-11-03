// Package config reads the daemon's settings: the environment, on top of the settings file.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"swcode/daemon/protocol"
)

const (
	DataFolder       = ".swcode"
	DefaultPort      = 4500
	defaultHost      = "0.0.0.0"
	defaultLlamaURL  = "http://127.0.0.1:8080"
	maxPort          = 65_535
	minTokenLength   = 16
	defaultShutdown  = 10_000
	minShutdown      = 100
	maxShutdown      = 120_000
	shareSeparator   = "/"
	defaultThinking  = protocol.ThinkingModeFull
	subagentThinking = protocol.ThinkingModeLow
)

// The environment's names of the settings.
const (
	envDataDir          = "SWCODE_DATA_DIR"
	envHost             = "SWCODE_HOST"
	envImageURL         = "SWCODE_IMAGE_URL"
	envLlamaAPIKey      = "LLAMA_API_KEY"
	envLlamaModel       = "LLAMA_MODEL"
	envLlamaSlotShare   = "LLAMA_SLOT_SHARE"
	envLlamaSubagent    = "LLAMA_SUBAGENT_MODEL"
	envLlamaURL         = "LLAMA_URL"
	envEnv              = "SWCODE_ENV"
	envNodeEnv          = "NODE_ENV"
	envPort             = "SWCODE_PORT"
	envPublicURL        = "SWCODE_PUBLIC_URL"
	envShutdownTimeout  = "SWCODE_SHUTDOWN_TIMEOUT_MS"
	envSSHEndpoints     = "SWCODE_SSH_ENDPOINTS"
	envSubagentThinking = "SWCODE_SUBAGENT_THINKING"
	envThinking         = "SWCODE_THINKING"
	envToken            = "SWCODE_TOKEN"
	envWebDir           = "SWCODE_WEB_DIR"
	envWorkspace        = "SWCODE_WORKSPACE"
)

// Env is what the daemon runs for: development logs a lot, tests nothing.
type Env string

const (
	EnvDevelopment Env = "development"
	EnvTest        Env = "test"
	EnvProduction  Env = "production"
)

var (
	slotSharePattern  = regexp.MustCompile(`^[1-9]\d*/[1-9]\d*$`)
	endpointSeparator = regexp.MustCompile(`[\s,]+`)
)

// SlotShare is this daemon's part of each model's slots: part 2 of 2 is the second half. When people share a
// llama-server with a daemon each, every daemon keeps to its own slots, so to its own prompt caches.
type SlotShare struct {
	Part  int
	Parts int
}

// WholeShare is every slot: one daemon on its llama-server.
var WholeShare = SlotShare{Part: 1, Parts: 1}

// Defaults is what the settings fall back on: the folder the daemon starts in, and the home folder.
type Defaults struct {
	Cwd  string
	Home string
}

// Config is the daemon's settings, as read.
type Config struct {
	// Where the sessions and the access token are kept.
	DataDir string
	// Every interface: a phone on the network opens the app too, with the token. 127.0.0.1 keeps it here.
	Host string
	// A stable-diffusion.cpp sd-server (its OpenAI routes): the agent makes pictures with it.
	ImageURL string
	// Only when llama-server was started with --api-key.
	LlamaAPIKey string
	// The model to ask for: needed when llama-server runs in router mode with several models.
	LlamaModel     string
	LlamaSlotShare SlotShare
	// The model explore subagents ask for, a faster one; the main model when left out.
	LlamaSubagentModel string
	LlamaURL           string
	Env                Env
	Port               int
	// The address people open, when it differs from host and port (the Vite server in development).
	PublicURL         string
	ShutdownTimeoutMs int
	// Machines projects can be on, reached over SSH: ssh destinations, by commas or spaces.
	SSHEndpoints []string
	// How much subagents think: briefly, their answers being for the main agent.
	SubagentThinking protocol.ThinkingMode
	// How much the main agent of a new session thinks: as the model does by itself, unless said.
	Thinking protocol.ThinkingMode
	// The access token; without it, one is created once and kept in the data folder.
	Token string
	// The built web app on the disk; empty for the one inside the binary, or dist/web beside the sources.
	WebDir string
	// The folder the agent works in.
	Workspace string
}

// reader reads settings one by one, keeping the first problem.
type reader struct {
	environment map[string]string
	problems    []error
}

func (r *reader) text(name string) string {
	return strings.TrimSpace(r.environment[name])
}

func (r *reader) or(name, fallback string) string {
	if value := r.text(name); value != "" {
		return value
	}
	return fallback
}

func (r *reader) fail(name, problem string) {
	r.problems = append(r.problems, fmt.Errorf("%s: %s", name, problem))
}

func (r *reader) url(name, fallback string) string {
	value := r.or(name, fallback)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		r.fail(name, "an address, such as http://127.0.0.1:8080")
	}
	return value
}

func (r *reader) number(name string, fallback, min, max int) int {
	value := r.text(name)
	if value == "" {
		return fallback
	}
	number, err := strconv.Atoi(value)
	if err != nil || number < min || number > max {
		r.fail(name, fmt.Sprintf("a whole number from %d to %d", min, max))
	}
	return number
}

func (r *reader) thinking(name string, fallback protocol.ThinkingMode) protocol.ThinkingMode {
	mode := protocol.ThinkingMode(r.or(name, string(fallback)))
	if !mode.Valid() {
		r.fail(name, "full, medium, low or off")
	}
	return mode
}

func (r *reader) share(name string) SlotShare {
	share, problem := ParseSlotShare(r.text(name))
	if problem != "" {
		r.fail(name, problem)
	}
	return share
}

// ParseSlotShare reads a part of the slots, as 2/3: every slot when empty, and why not when it is no such part.
func ParseSlotShare(value string) (SlotShare, string) {
	if value == "" {
		return WholeShare, ""
	}
	if !slotSharePattern.MatchString(value) {
		return WholeShare, "a part of the slots, such as 1/2"
	}
	part, parts, _ := strings.Cut(value, shareSeparator)
	share := SlotShare{Part: atoi(part), Parts: atoi(parts)}
	if share.Part > share.Parts {
		return WholeShare, "the part cannot be more than the parts, as in 3/2"
	}
	return share, ""
}

func (r *reader) env() Env {
	env := Env(r.or(envEnv, r.or(envNodeEnv, string(EnvDevelopment))))
	if env != EnvDevelopment && env != EnvTest && env != EnvProduction {
		r.fail(envEnv, "development, test or production")
	}
	return env
}

func (r *reader) token() string {
	token := r.text(envToken)
	if token != "" && len(token) < minTokenLength {
		r.fail(envToken, fmt.Sprintf("at least %d characters", minTokenLength))
	}
	return token
}

func atoi(text string) int {
	number, _ := strconv.Atoi(text)
	return number
}

func endpoints(text string) []string {
	return slicesWithout(endpointSeparator.Split(text, -1), "")
}

func slicesWithout(items []string, unwanted string) []string {
	kept := []string{}
	for _, item := range items {
		if item != unwanted {
			kept = append(kept, item)
		}
	}
	return kept
}

// Load reads the settings from an environment: names as the README lists them, an empty one as left out.
func Load(environment map[string]string, defaults Defaults) (Config, error) {
	r := &reader{environment: environment}
	config := Config{
		DataDir:            r.or(envDataDir, filepath.Join(defaults.Home, DataFolder)),
		Env:                r.env(),
		Host:               r.or(envHost, defaultHost),
		ImageURL:           r.url(envImageURL, ""),
		LlamaAPIKey:        r.text(envLlamaAPIKey),
		LlamaModel:         r.text(envLlamaModel),
		LlamaSlotShare:     r.share(envLlamaSlotShare),
		LlamaSubagentModel: r.text(envLlamaSubagent),
		LlamaURL:           r.url(envLlamaURL, defaultLlamaURL),
		Port:               r.number(envPort, DefaultPort, 1, maxPort),
		PublicURL:          r.url(envPublicURL, ""),
		ShutdownTimeoutMs:  r.number(envShutdownTimeout, defaultShutdown, minShutdown, maxShutdown),
		SSHEndpoints:       endpoints(r.text(envSSHEndpoints)),
		SubagentThinking:   r.thinking(envSubagentThinking, subagentThinking),
		Thinking:           r.thinking(envThinking, defaultThinking),
		Token:              r.token(),
		WebDir:             r.text(envWebDir),
		Workspace:          r.or(envWorkspace, defaults.Cwd),
	}
	return config, errors.Join(r.problems...)
}
