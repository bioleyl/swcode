package hosts

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"

	"swcode/daemon/protocol"
)

// PlatformWindows is how a host on Windows names its system.
const PlatformWindows = "windows"

var (
	hostLine  = regexp.MustCompile(`(?i)^\s*host\s+(.+)$`)
	wildcards = regexp.MustCompile(`[*?!]`)
	// Docker Desktop's own distributions hold no projects.
	notForProjects = regexp.MustCompile(`^docker-desktop`)
	sshConfig      = filepath.Join(".ssh", "config")
	listDistros    = []string{"-l", "-q"}
)

func optionOf(ref protocol.HostRef, label string) protocol.HostOption {
	return protocol.HostOption{Label: label, Ref: ref}
}

// Named is a setting entry's name and endpoint: `marvin=me@marvin` names the endpoint `me@marvin`, and an entry
// without a name is named by its endpoint.
func Named(entry string) (string, string) {
	if name, endpoint, found := strings.Cut(entry, "="); found && name != "" && endpoint != "" {
		return name, endpoint
	}
	return entry, entry
}

// wslDistros is the WSL distributions of this Windows: wsl.exe lists them in UTF-16, one a line.
func wslDistros() []string {
	cmd := exec.Command(wslProgram, listDistros...)
	hideWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	names := []string{}
	for _, name := range strings.Split(utf16Text(output), "\n") {
		name = strings.TrimSpace(strings.ReplaceAll(name, "\x00", ""))
		if name != "" && !notForProjects.MatchString(name) {
			names = append(names, name)
		}
	}
	return names
}

func utf16Text(data []byte) string {
	data = bytes.TrimPrefix(data, []byte{0xff, 0xfe})
	units := make([]uint16, len(data)/2)
	for index := range units {
		units[index] = uint16(data[2*index]) | uint16(data[2*index+1])<<8
	}
	return string(utf16.Decode(units))
}

// SSHConfigHosts is the machines ~/.ssh/config names: its Host aliases, the patterns aside.
func SSHConfigHosts(home string) []string {
	text, _ := os.ReadFile(filepath.Join(home, sshConfig))
	names := []string{}
	for _, line := range strings.Split(string(text), "\n") {
		found := hostLine.FindStringSubmatch(line)
		if found == nil {
			continue
		}
		for _, name := range strings.Fields(found[1]) {
			if !wildcards.MatchString(name) {
				names = append(names, name)
			}
		}
	}
	return names
}

// Options is where a new project can be: this computer, its WSL distributions when it is Windows, and the SSH
// endpoints of the settings and of ~/.ssh/config. A setting's entry may name its endpoint (`marvin=me@marvin`), and
// the name is what people read; the endpoint names itself otherwise. One endpoint, one option: the settings' name
// wins over the config's alias.
func Options(platform, home string, endpoints []string) []protocol.HostOption {
	options := []protocol.HostOption{optionOf(protocol.LocalHost(), Label(protocol.LocalHost()))}
	if platform == PlatformWindows {
		for _, distro := range wslDistros() {
			ref := protocol.HostRef{Value: protocol.HostRefWSL{Distro: distro}}
			options = append(options, optionOf(ref, Label(ref)))
		}
	}
	seen := map[string]bool{}
	for _, entry := range append(append([]string{}, endpoints...), SSHConfigHosts(home)...) {
		name, endpoint := Named(entry)
		if !seen[endpoint] {
			seen[endpoint] = true
			options = append(options, optionOf(protocol.HostRef{Value: protocol.HostRefSSH{Endpoint: endpoint}}, name))
		}
	}
	return options
}
