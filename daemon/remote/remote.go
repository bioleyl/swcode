// Package remote runs the daemon on another machine over SSH: this very program copied there when it lacks it,
// started with its port forwarded here, and llama-server forwarded there.
package remote

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"swcode/daemon/cli"
	"swcode/daemon/config"
	"swcode/daemon/version"
)

const (
	// Folder is where swcode lives on a remote machine; ssh runs commands from the home folder.
	Folder = ".swcode/bin"
	Binary = Folder + "/swcode"
	// The remote daemon's port, and the one llama-server comes through, on the remote machine.
	remoteDaemonPort = 4500
	remoteLlamaPort  = 18080
	loopback         = "127.0.0.1"
	homeMark         = "~"
	homePrefix       = "~/"
	sshProgram       = "ssh"
	scpProgram       = "scp"
	failed           = 1
	// ProbeCommand asks the remote machine its platform and the swcode it has, in one round trip.
	ProbeCommand = `uname -sm; "$HOME"/` + Binary + ` --version 2>/dev/null || true`
)

// platforms is what uname -sm says, as Go names the platform its programs are built for.
var platforms = map[string]string{
	"Darwin arm64": "darwin-arm64", "Darwin x86_64": "darwin-amd64", "Linux aarch64": "linux-arm64", "Linux x86_64": "linux-amd64",
}

var defaultPorts = map[string]int{"http": 80, "https": 443}

// ShellWord is a value as one shell word: single-quoted, a ~ at its start still meaning the remote home.
func ShellWord(value string) string {
	quoted := func(text string) string { return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'" }
	if value == homeMark {
		return `"$HOME"`
	}
	if rest, ok := strings.CutPrefix(value, homePrefix); ok {
		return `"$HOME"/` + quoted(rest)
	}
	return quoted(value)
}

// Probe is what the remote machine said about itself.
type Probe struct {
	Platform string
	Version  string
}

func ParseProbe(output string) Probe {
	lines := strings.Split(output, "\n")
	probe := Probe{Platform: platforms[strings.TrimSpace(lines[0])]}
	if len(lines) > 1 {
		probe.Version = strings.TrimSpace(lines[1])
	}
	return probe
}

func llamaEndpoint(address string) (string, int) {
	parsed, err := url.Parse(address)
	if err != nil {
		return loopback, defaultPorts["http"]
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		port = defaultPorts[parsed.Scheme]
	}
	return parsed.Hostname(), port
}

// Environment is the remote daemon's settings: this machine's models, and addresses that work through the tunnels.
func Environment(command cli.Command, settings config.Config) map[string]string {
	environment := map[string]string{
		"LLAMA_URL":         fmt.Sprintf("http://%s:%d", loopback, remoteLlamaPort),
		"SWCODE_ENV":        string(config.EnvProduction),
		"SWCODE_HOST":       loopback,
		"SWCODE_PORT":       strconv.Itoa(remoteDaemonPort),
		"SWCODE_PUBLIC_URL": fmt.Sprintf("http://%s:%d", loopback, command.LocalPort),
		"SWCODE_WORKSPACE":  homeMark,
	}
	if command.LlamaURL != "" {
		environment["LLAMA_URL"] = command.LlamaURL
	}
	if command.Workspace != "" {
		environment["SWCODE_WORKSPACE"] = command.Workspace
	}
	optional := map[string]string{"LLAMA_API_KEY": settings.LlamaAPIKey, "LLAMA_MODEL": settings.LlamaModel, "LLAMA_SUBAGENT_MODEL": settings.LlamaSubagentModel}
	// The whole of the slots is the remote daemon's default: only a part needs saying.
	if settings.LlamaSlotShare.Parts > 1 {
		optional["LLAMA_SLOT_SHARE"] = fmt.Sprintf("%d/%d", settings.LlamaSlotShare.Part, settings.LlamaSlotShare.Parts)
	}
	for name, value := range optional {
		if value != "" {
			environment[name] = value
		}
	}
	return environment
}

// ServeCommand is the remote daemon started with its settings, in the order of their names.
func ServeCommand(environment map[string]string) string {
	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)
	words := []string{"env"}
	for _, name := range names {
		words = append(words, name+"="+ShellWord(environment[name]))
	}
	return strings.Join(append(words, `"$HOME"/`+Binary, "serve"), " ")
}

// SSHServeArguments is the ssh command that runs the remote daemon: its port forwarded here, and, unless the
// remote reaches llama-server itself, llama-server forwarded there. A terminal is asked for so that stopping here
// stops it there.
func SSHServeArguments(command cli.Command, settings config.Config) []string {
	host, port := llamaEndpoint(settings.LlamaURL)
	arguments := []string{"-tt", "-L", fmt.Sprintf("%d:%s:%d", command.LocalPort, loopback, remoteDaemonPort)}
	if command.LlamaURL == "" {
		arguments = append(arguments, "-R", fmt.Sprintf("%d:%s:%d", remoteLlamaPort, host, port))
	}
	return append(arguments, command.Host, ServeCommand(Environment(command, settings)))
}

func capture(program string, arguments ...string) (string, error) {
	cmd := exec.Command(program, arguments...)
	cmd.Stderr = os.Stderr
	var output bytes.Buffer
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s ended: %w", program, err)
	}
	return output.String(), nil
}

// programFor is the program to copy for a platform: this one, or one built for it beside this one
// (swcode-linux-arm64…).
func programFor(platform string) string {
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	if platform == runtime.GOOS+"-"+runtime.GOARCH {
		return self
	}
	sibling := filepath.Join(filepath.Dir(self), "swcode-"+platform)
	if _, err := os.Stat(sibling); err == nil {
		return sibling
	}
	return ""
}

// upload puts the program on the remote machine, in place of an older one: written aside, then moved.
func upload(host, program string) error {
	fmt.Printf("Copying swcode %s to %s…\n", version.Build, host)
	if _, err := capture(sshProgram, host, `mkdir -p "$HOME"/`+Folder); err != nil {
		return err
	}
	if _, err := capture(scpProgram, "-q", program, host+":"+Binary+".new"); err != nil {
		return err
	}
	_, err := capture(sshProgram, host, `chmod +x "$HOME"/`+Binary+`.new && mv "$HOME"/`+Binary+`.new "$HOME"/`+Binary)
	return err
}

// Run runs the daemon on another machine: copies the program there when it lacks it or has another build, then
// starts it over SSH with its port forwarded here. Stopping here (Ctrl+C) stops it there.
func Run(command cli.Command, settings config.Config) int {
	output, err := capture(sshProgram, command.Host, ProbeCommand)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	probe := ParseProbe(output)
	program := programFor(probe.Platform)
	if program == "" {
		fmt.Fprintf(os.Stderr, "%s runs %s: build swcode for it (npm run build:binary) and put it beside this one.\n", command.Host, orUnknown(probe.Platform))
		return failed
	}
	if probe.Version != version.Build {
		if err := upload(command.Host, program); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return failed
		}
	}
	ssh := exec.Command(sshProgram, SSHServeArguments(command, settings)...)
	ssh.Stdin, ssh.Stdout, ssh.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := ssh.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		return failed
	}
	return 0
}

func orUnknown(platform string) string {
	if platform == "" {
		return "an unknown platform"
	}
	return platform
}
