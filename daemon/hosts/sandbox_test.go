package hosts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// sandboxed are the hosts under test, each with what it has to hide: the tests have nothing to say where none can
// sandbox (no bubblewrap, or a system that refuses it a namespace).
func sandboxed(t *testing.T) map[string]sandboxedHost {
	t.Helper()
	found := map[string]sandboxedHost{}
	sandboxes := NewSandboxes(time.Now)
	for name, host := range hostsUnderTest(t) {
		state := sandboxes.Of(context.Background(), name, host)
		if !state.Available {
			t.Skip("no sandbox here: " + state.Problem)
		}
		found[name] = sandboxedHost{Host: host, state: state}
	}
	return found
}

type sandboxedHost struct {
	Host
	state SandboxState
}

func TestASandboxedCommandWritesItsWorkspaceItsScratchFolderAndItsOwnTmpOnly(t *testing.T) {
	for name, host := range sandboxed(t) {
		t.Run(name, func(t *testing.T) {
			root, folder := project(t)
			scratch, secrets := filepath.Join(root, "scratch"), filepath.Join(root, "secrets")
			write(t, filepath.Join(secrets, "key"), "the key\n")
			write(t, filepath.Join(folder, ".git", "config"), "[core]\n")
			if err := os.MkdirAll(scratch, 0o755); err != nil {
				t.Fatal(err)
			}
			sandbox := Sandbox{Hidden: []string{secrets}, Host: host.state, Scratch: scratch, Workspace: folder}
			run := func(command string) CommandResult {
				return host.Run(context.Background(), sandbox.Wrap(command), CommandOptions{Cwd: folder, Timeout: 20 * time.Second})
			}

			wrote := run("echo in > made.txt && echo out > " + ShellWord(scratch+"/probe.txt") + " && echo kept > /tmp/kept.txt && pwd && echo \"it's quoted\"")
			if wrote.ExitCode == nil || *wrote.ExitCode != 0 || wrote.Output != folder+"\nit's quoted\n" {
				t.Fatalf("writing where it may: %+v", wrote)
			}
			if data, _ := os.ReadFile(filepath.Join(folder, "made.txt")); string(data) != "in\n" {
				t.Fatal("the workspace was not written")
			}
			// Its /tmp is the session's own, kept from one command to the next, and not the system's.
			if again := run("cat /tmp/kept.txt"); again.Output != "kept\n" {
				t.Fatalf("its /tmp was not kept: %+v", again)
			}
			if _, err := os.Stat("/tmp/kept.txt"); err == nil {
				t.Fatal("the system's /tmp was written")
			}

			beside := filepath.Join(root, "beside.txt")
			refused := run("echo no > " + ShellWord(beside) + "; echo no >> .git/config; cat " + ShellWord(secrets+"/key") + "; exit 7")
			if refused.ExitCode == nil || *refused.ExitCode != 7 {
				t.Fatalf("the command's own exit code is lost: %+v", refused)
			}
			if _, err := os.Stat(beside); err == nil {
				t.Fatal("a file was written outside the workspace")
			}
			if data, _ := os.ReadFile(filepath.Join(folder, ".git", "config")); string(data) != "[core]\n" {
				t.Fatalf("git's settings were written: %q", data)
			}
			if strings.Contains(refused.Output, "the key") {
				t.Fatalf("a hidden folder was read: %s", refused.Output)
			}
			if !strings.Contains(refused.Output, "Read-only file system") {
				t.Fatalf("the refusal does not say why: %s", refused.Output)
			}
		})
	}
}

func TestASandboxedCommandIsStoppedWithAllItStarted(t *testing.T) {
	for name, host := range sandboxed(t) {
		t.Run(name, func(t *testing.T) {
			root, folder := project(t)
			sandbox := Sandbox{Host: host.state, Scratch: filepath.Join(root, "scratch"), Workspace: folder}
			marker := "sleep 4" + map[string]string{"local": "0", "shell": "1"}[name]
			ctx, stop := context.WithCancel(context.Background())
			time.AfterFunc(500*time.Millisecond, stop)
			result := host.Run(ctx, sandbox.Wrap("setsid "+marker+"7 & "+marker+"8; wait"), CommandOptions{Cwd: folder, Timeout: time.Minute})
			if !result.Stopped {
				t.Fatalf("got %+v", result)
			}
			// Even what left the command's own group ends: the sandbox's processes go with it.
			waitUntil(t, func() bool {
				output, _ := exec.Command("bash", "-c", "pgrep -f '"+marker+"[78]' || true").Output()
				return strings.TrimSpace(string(output)) == ""
			})
		})
	}
}

// noSandbox is a host of another system, or one whose bash has no bubblewrap.
type noSandbox struct {
	Host
	platform string
	says     string
	asked    int
}

func (h *noSandbox) Platform(context.Context) (string, error) { return h.platform, nil }
func (h *noSandbox) Home(context.Context) (string, error)     { return "/", nil }
func (h *noSandbox) Run(context.Context, string, CommandOptions) CommandResult {
	h.asked++
	return CommandResult{Output: h.says + "\n"}
}

func TestAHostWithoutBubblewrapOrOfAnotherSystemSandboxesNothingAndSaysWhy(t *testing.T) {
	now := time.Now()
	sandboxes := NewSandboxes(func() time.Time { return now })
	windows := &noSandbox{platform: "windows"}
	if state := sandboxes.Of(context.Background(), "windows", windows); state.Available || state.Problem != NoSandboxHere || windows.asked != 0 {
		t.Fatalf("on Windows: %+v", state)
	}
	if home, _ := os.UserHomeDir(); home != "" {
		if _, err := os.Stat(filepath.Join(home, ".ssh")); err == nil {
			real, _ := filepath.EvalSymlinks(filepath.Join(home, ".ssh"))
			state := probeSandbox(context.Background(), NewLocalHost(LocalOptions{Home: home, ScratchRoot: t.TempDir()}))
			if state.Available && !slices.Contains(state.HiddenFolders, real) {
				t.Fatalf("the keys in %s are not hidden: %+v", real, state)
			}
		}
	}
	bare := &noSandbox{platform: "linux", says: "missing"}
	if state := sandboxes.Of(context.Background(), "bare", bare); state.Available || state.Problem != NoSandboxProgram {
		t.Fatalf("without bubblewrap: %+v", state)
	}
	locked := &noSandbox{platform: "linux", says: "refused"}
	if state := sandboxes.Of(context.Background(), "locked", locked); state.Available || state.Problem != NoSandboxAllowed {
		t.Fatalf("where namespaces are refused: %+v", state)
	}

	// A host is asked once, and again after a while when it had none: bubblewrap may have been installed.
	sandboxes.Of(context.Background(), "bare", bare)
	if bare.asked != 1 {
		t.Fatalf("asked %d times within the minute", bare.asked)
	}
	now = now.Add(2 * probeAgain)
	bare.says = "ok"
	if state := sandboxes.Of(context.Background(), "bare", bare); !state.Available || bare.asked != 2 {
		t.Fatalf("once installed: %+v after %d asks", state, bare.asked)
	}
	now = now.Add(2 * probeAgain)
	sandboxes.Of(context.Background(), "bare", bare)
	if bare.asked != 2 {
		t.Fatal("a host that can sandbox was asked again so soon")
	}
	// What it has to hide is looked at again now and then: a key folder may have been made since.
	now = now.Add(2 * sandboxAge)
	bare.says = "ok\ndir /home/someone/.kube\nfile /home/someone/.netrc\nopen /home/someone/.cache"
	state := sandboxes.Of(context.Background(), "bare", bare)
	if bare.asked != 3 || !slices.Equal(state.HiddenFolders, []string{"/home/someone/.kube"}) ||
		!slices.Equal(state.HiddenFiles, []string{"/home/someone/.netrc"}) || !slices.Equal(state.Caches, []string{"/home/someone/.cache"}) {
		t.Fatalf("looked at again: %+v after %d asks", state, bare.asked)
	}
}
