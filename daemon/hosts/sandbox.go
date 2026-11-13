package hosts

import (
	"context"
	"strings"
	"sync"
	"time"
)

const (
	sandboxProgram = "bwrap"
	sandboxReady   = "ok"
	sandboxMissing = "missing"
	foundFolder    = "dir"
	foundFile      = "file"
	foundCache     = "open"
	probeLimit     = 30 * time.Second
	// probeAgain is how long a host that has no sandbox is believed (bubblewrap may be installed meanwhile), and
	// sandboxAge how long what a host has to hide is: a key folder made since is hidden from then on.
	probeAgain = time.Minute
	sandboxAge = 10 * time.Minute

	NoSandboxHere    = "Commands are sandboxed on Linux only."
	NoSandboxProgram = "bubblewrap is not installed there (the bwrap command): commands run without a sandbox."
	NoSandboxAllowed = "bubblewrap cannot make a sandbox there (the system refuses it a namespace of its own): commands run without a sandbox."
	platformLinux    = "linux"

	// homesOf is the homes a command could read keys from: its own, and on WSL the Windows users' (their drives
	// are mounted there, with the same folders).
	homesOf = `"$HOME" /mnt/?/Users/*`
	// real is where a path really is, links followed: bubblewrap mounts on a real folder only. Only what is there
	// is asked about: each question starts a program.
	real = `[ -e "$at" ] && at=$(realpath -- "$at" 2>/dev/null)`
)

var (
	// hiddenFolders are what a sandboxed command never sees, under a home: keys that open other systems, and
	// swcode's own data (its token, the sessions).
	hiddenFolders = []string{".ssh", ".gnupg", ".aws", ".azure", ".kube", ".docker", ".config/gh", ".config/gcloud", ".swcode"}
	// hiddenFiles are the same, as files.
	hiddenFiles = []string{".netrc", ".git-credentials"}
	// cacheFolders are what a sandboxed command may write under its home beside its workspace: where package
	// managers and compilers keep what they fetched and built, without which no build goes through.
	cacheFolders = []string{
		".cache", ".npm", ".yarn", ".bun/install", ".local/share/pnpm", "go/pkg", ".cargo/registry", ".cargo/git",
		".gradle", ".m2", ".nuget", ".pub-cache",
	}
	// keptInGit are the parts of a repository a sandboxed command reads but never writes: what git itself runs
	// later, outside the sandbox, in the person's name.
	keptInGit = []string{".git/hooks", ".git/config"}
)

// KeptInGitFolder is the parts of a repository's .git folder a sandboxed command reads but never writes: its hooks
// and its settings, which git runs later, outside the sandbox, in the person's name.
func KeptInGitFolder(gitFolder string) []string {
	return []string{gitFolder + "/hooks", gitFolder + "/config"}
}

func shellWords(paths []string) string {
	quoted := make([]string, len(paths))
	for index, path := range paths {
		quoted[index] = ShellWord(path)
	}
	return strings.Join(quoted, " ")
}

// sandboxProbe says whether the host can make a sandbox (bubblewrap is there, and the system lets it make one:
// some forbid a namespace of one's own), then what there is to hide and to open, a line each.
var sandboxProbe = strings.Join([]string{
	`command -v bwrap >/dev/null 2>&1 || { echo ` + sandboxMissing + `; exit 0; }`,
	`bwrap --unshare-pid --ro-bind / / --dev /dev --proc /proc true >/dev/null 2>&1 || { echo refused; exit 0; }`,
	`echo ` + sandboxReady,
	`for home in ` + homesOf + `; do`,
	`  for name in ` + shellWords(hiddenFolders) + `; do at="$home/$name"; ` + real + ` && [ -d "$at" ] && echo "` + foundFolder + ` $at"; done`,
	`  for name in ` + shellWords(hiddenFiles) + `; do at="$home/$name"; ` + real + ` && [ -f "$at" ] && echo "` + foundFile + ` $at"; done`,
	`done`,
	`for name in ` + shellWords(cacheFolders) + `; do at="$HOME/$name"; ` + real + ` && [ -d "$at" ] && echo "` + foundCache + ` $at"; done`,
	`exit 0`,
}, "\n")

// SandboxState is whether a host can sandbox commands, and why not when it cannot; then what it has that a
// sandboxed command must not see, and the caches it may write.
type SandboxState struct {
	Available     bool
	Problem       string
	HiddenFolders []string
	HiddenFiles   []string
	Caches        []string
}

// Sandbox is where an agent's command runs on a Linux host: in bubblewrap, the whole system read-only but the
// folders it works in, the caches of package managers and a /tmp of the session's own; the person's keys and
// swcode's data are not there at all. The network stays: installs and tests need it.
type Sandbox struct {
	// Workspace is the project's folder: written freely, but for what git runs by itself.
	Workspace string
	// Scratch is the session's scratch folder, where its /tmp is kept from one command to the next.
	Scratch string
	// Hidden are more folders the command never sees: the daemon's data folder, when it is not ~/.swcode.
	Hidden []string
	// Writable are more folders the command may write.
	Writable []string
	// Kept are more paths the command reads but never writes, inside what it may write.
	Kept []string
	// Host is what the host has to hide and to open, as it said when asked.
	Host SandboxState
}

// Wrap is the command as it runs in the sandbox: a script for the host's bash, made of tests the shell does by
// itself, so a command starts about as fast as without. It looks at what is there first (bubblewrap cannot hide or
// open a folder that is gone). Later lines win over earlier ones, so the folders the command works in are opened
// last: a workspace under a hidden folder is still its workspace.
func (s Sandbox) Wrap(command string) string {
	tmp := s.Scratch + "/tmp"
	hidden := append(append([]string{}, s.Host.HiddenFolders...), s.Hidden...)
	open := append(append(append([]string{}, s.Host.Caches...), s.Workspace, s.Scratch), s.Writable...)
	kept := make([]string, len(keptInGit))
	for index, part := range keptInGit {
		kept[index] = s.Workspace + "/" + part
	}
	kept = append(kept, s.Kept...)
	lines := []string{
		"[ -d " + ShellWord(tmp) + " ] || mkdir -p -- " + ShellWord(tmp),
		"set -- --die-with-parent --unshare-pid --ro-bind / / --dev /dev --proc /proc --bind " + ShellWord(tmp) + " /tmp",
		"for at in " + shellWords(hidden) + `; do [ -d "$at" ] && set -- "$@" --tmpfs "$at"; done`,
		"for at in " + shellWords(s.Host.HiddenFiles) + `; do [ -f "$at" ] && set -- "$@" --ro-bind /dev/null "$at"; done`,
		"for at in " + shellWords(open) + `; do [ -d "$at" ] && set -- "$@" --bind "$at" "$at"; done`,
		"for at in " + shellWords(kept) + `; do [ -e "$at" ] && set -- "$@" --ro-bind "$at" "$at"; done`,
		sandboxProgram + ` "$@" -- bash -c ` + ShellWord(command),
	}
	return strings.Join(lines, "\n")
}

type probed struct {
	state SandboxState
	at    time.Time
}

// Sandboxes knows which hosts can sandbox commands and what each has to hide: a host is asked now and then, sooner
// when it could not.
type Sandboxes struct {
	mu    sync.Mutex
	known map[string]probed
	now   func() time.Time
}

func NewSandboxes(now func() time.Time) *Sandboxes {
	return &Sandboxes{known: map[string]probed{}, now: now}
}

// Of is whether the host can sandbox commands; key names the host among the others.
func (s *Sandboxes) Of(ctx context.Context, key string, host Host) SandboxState {
	s.mu.Lock()
	found, known := s.known[key]
	s.mu.Unlock()
	age := sandboxAge
	if !found.state.Available {
		age = probeAgain
	}
	if known && s.now().Sub(found.at) < age {
		return found.state
	}
	state := probeSandbox(ctx, host)
	s.mu.Lock()
	s.known[key] = probed{at: s.now(), state: state}
	s.mu.Unlock()
	return state
}

func probeSandbox(ctx context.Context, host Host) SandboxState {
	platform, err := host.Platform(ctx)
	if err != nil {
		return SandboxState{Problem: err.Error()}
	}
	if platform != platformLinux {
		return SandboxState{Problem: NoSandboxHere}
	}
	home, err := host.Home(ctx)
	if err != nil {
		return SandboxState{Problem: err.Error()}
	}
	lines := strings.Split(strings.TrimSpace(host.Run(ctx, sandboxProbe, CommandOptions{Cwd: home, Timeout: probeLimit}).Output), "\n")
	switch strings.TrimSpace(lines[0]) {
	case sandboxReady:
		return foundIn(lines[1:])
	case sandboxMissing:
		return SandboxState{Problem: NoSandboxProgram}
	}
	return SandboxState{Problem: NoSandboxAllowed}
}

// foundIn reads what a host that can sandbox has to hide and to open, a line each.
func foundIn(lines []string) SandboxState {
	state := SandboxState{Available: true}
	seen := map[string]bool{}
	for _, line := range lines {
		kind, path, _ := strings.Cut(strings.TrimRight(line, "\r"), " ")
		// A home's folder may be a link to another home's: it is hidden once.
		if seen[line] {
			continue
		}
		seen[line] = true
		switch kind {
		case foundFolder:
			state.HiddenFolders = append(state.HiddenFolders, path)
		case foundFile:
			state.HiddenFiles = append(state.HiddenFiles, path)
		case foundCache:
			state.Caches = append(state.Caches, path)
		}
	}
	return state
}
