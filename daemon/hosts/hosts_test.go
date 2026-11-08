package hosts

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"swcode/daemon/protocol"
)

// project makes a folder with src/tags.ts, the project of these tests.
func project(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	folder := filepath.Join(root, "project")
	write(t, filepath.Join(folder, "src", "tags.ts"), "export const tags = new Map();\nconst slow = [];\n")
	return root, folder
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// hostsUnderTest are this computer, and a host reached through a bash, as wsl.exe or ssh reach one: this
// computer's own, here.
func hostsUnderTest(t *testing.T) map[string]Host {
	t.Helper()
	local := NewLocalHost(LocalOptions{Home: t.TempDir(), ScratchRoot: t.TempDir()})
	shell := NewShellHost(LocalTransport("build-box", ""))
	t.Cleanup(func() {
		local.Close()
		shell.Close()
	})
	return map[string]Host{"local": local, "shell": shell}
}

func TestAHostReadsAndWritesFilesBytesAndOddNamesIncluded(t *testing.T) {
	ctx := context.Background()
	for name, host := range hostsUnderTest(t) {
		t.Run(name, func(t *testing.T) {
			_, folder := project(t)
			if info, _ := host.FileInfo(ctx, filepath.Join(folder, "src")); info.Kind != KindDirectory {
				t.Fatalf("src is %v", info)
			}
			if info, _ := host.FileInfo(ctx, filepath.Join(folder, "nope.ts")); info.Kind != KindMissing {
				t.Fatalf("nope.ts is %v", info)
			}
			if info, _ := host.FileInfo(ctx, filepath.Join(folder, "src", "tags.ts")); info.Kind != KindFile || info.Size != 48 {
				t.Fatalf("tags.ts is %v", info)
			}
			binary := []byte{0x61, 0x00, 0xff, 0x0a, 0x27}
			odd := filepath.Join(folder, "it's new", "blob $(rm -rf).bin")
			if err := host.WriteFile(ctx, odd, binary); err != nil {
				t.Fatal(err)
			}
			if written, _ := os.ReadFile(odd); !bytes.Equal(written, binary) {
				t.Fatalf("wrote %v", written)
			}
			if read, _ := host.ReadFile(ctx, odd); !bytes.Equal(read, binary) {
				t.Fatalf("read %v", read)
			}
			if err := host.RemoveFile(ctx, odd); err != nil {
				t.Fatal(err)
			}
			if info, _ := host.FileInfo(ctx, odd); info.Kind != KindMissing {
				t.Fatalf("removed file is %v", info)
			}
			if err := host.RemoveFile(ctx, odd); err != nil {
				t.Fatalf("removing a missing file: %v", err)
			}
			real, err := host.RealPath(ctx, filepath.Join(folder, "src", "..", "later.ts"))
			if err != nil || real != filepath.Join(folder, "later.ts") {
				t.Fatalf("real path %q, %v", real, err)
			}
		})
	}
}

func TestAHostListsFilesOutsideARepositoryInOrder(t *testing.T) {
	ctx := context.Background()
	for name, host := range hostsUnderTest(t) {
		t.Run(name, func(t *testing.T) {
			folder := t.TempDir()
			names := []string{"zeta.md", "frontend/component.md", "commit.md", "alpha.md"}
			for _, file := range names {
				write(t, filepath.Join(folder, file), "x\n")
			}
			files, err := host.ListFiles(ctx, folder, "**/*.md", 10)
			want := []string{}
			for _, file := range []string{"alpha.md", "commit.md", "frontend/component.md", "zeta.md"} {
				want = append(want, filepath.Join(folder, file))
			}
			if err != nil || !reflect.DeepEqual(files, want) {
				t.Fatalf("listed %v, %v", files, err)
			}
		})
	}
}

func TestAHostListsAndSearchesGitsFilesAGlobAndLookaheads(t *testing.T) {
	ctx := context.Background()
	for name, host := range hostsUnderTest(t) {
		t.Run(name, func(t *testing.T) {
			_, folder := project(t)
			write(t, filepath.Join(folder, "src", "readme.md"), "slow down\n")
			write(t, filepath.Join(folder, "node_modules", "lib", "x.ts"), "const slow = 1;\n")
			files, err := host.ListFiles(ctx, folder, "**/*.ts", 10)
			if err != nil || !reflect.DeepEqual(files, []string{filepath.Join(folder, "src", "tags.ts")}) {
				t.Fatalf("listed %v, %v", files, err)
			}
			if output, err := exec.Command("git", "init", "-q", folder).CombinedOutput(); err != nil {
				t.Fatalf("git init: %s", output)
			}
			write(t, filepath.Join(folder, ".gitignore"), "generated/\n")
			write(t, filepath.Join(folder, "generated", "out.ts"), "const slow = 2;\n")
			files, _ = host.ListFiles(ctx, folder, "src/**", 10)
			sort.Strings(files)
			want := []string{filepath.Join(folder, "src", "readme.md"), filepath.Join(folder, "src", "tags.ts")}
			if !reflect.DeepEqual(files, want) {
				t.Fatalf("listed %v", files)
			}
			search := SearchRequest{Include: "**/*", MaxFileBytes: 100_000, MaxFiles: 50, MaxMatches: 10, Root: folder}
			search.Source = "const (?=slow)"
			matches, err := host.Search(ctx, search)
			wantMatch := []SearchMatch{{File: filepath.Join(folder, "src", "tags.ts"), Line: 2, Text: "const slow = [];"}}
			if err != nil || !reflect.DeepEqual(matches, wantMatch) {
				t.Fatalf("found %v, %v", matches, err)
			}
			search.Source, search.Flags = "SLOW", "i"
			if matches, _ := host.Search(ctx, search); len(matches) != 2 {
				t.Fatalf("found %v", matches)
			}
		})
	}
}

func TestAHostBrowsesFoldersAndMakesOneItsScratchFolderToo(t *testing.T) {
	ctx := context.Background()
	for name, host := range hostsUnderTest(t) {
		t.Run(name, func(t *testing.T) {
			root, folder := project(t)
			if err := os.Mkdir(filepath.Join(folder, ".hidden"), 0o755); err != nil {
				t.Fatal(err)
			}
			listing, _ := host.ListFolders(ctx, folder)
			want := FolderListing{Folders: []string{"src"}, OK: true, Parent: root, Path: folder}
			if !reflect.DeepEqual(listing, want) {
				t.Fatalf("listed %+v", listing)
			}
			if listing, _ := host.ListFolders(ctx, "relative/path"); listing.OK {
				t.Fatal("a relative path was listed")
			}
			if made, _ := host.MakeFolder(ctx, folder, "fresh"); !made.OK || made.Path != filepath.Join(folder, "fresh") {
				t.Fatalf("made %+v", made)
			}
			if made, _ := host.MakeFolder(ctx, folder, "fresh"); made.OK || !strings.Contains(made.Problem, "already exists") {
				t.Fatalf("made again %+v", made)
			}
			scratch, err := host.ScratchFolder(ctx, "session-1")
			if err != nil || !strings.HasSuffix(filepath.ToSlash(scratch), "session-1") {
				t.Fatalf("scratch %q, %v", scratch, err)
			}
			if err := host.RemoveScratch(ctx, "session-1"); err != nil {
				t.Fatal(err)
			}
			if info, _ := host.FileInfo(ctx, scratch); info.Kind != KindMissing {
				t.Fatalf("scratch is %v", info)
			}
		})
	}
}

func TestAHostRunsACommandAndStopsItWithAllItStarted(t *testing.T) {
	for name, host := range hostsUnderTest(t) {
		t.Run(name, func(t *testing.T) {
			_, folder := project(t)
			options := CommandOptions{Cwd: folder, Timeout: 5 * time.Second}
			result := host.Run(context.Background(), "echo $((40 + 2)); echo oops >&2; exit 3", options)
			if result.ExitCode == nil || *result.ExitCode != 3 || result.Output != "42\noops\n" {
				t.Fatalf("got %+v", result)
			}
			marker := "sleep 3" + map[string]string{"local": "0", "shell": "1"}[name]
			ctx, stop := context.WithCancel(context.Background())
			time.AfterFunc(500*time.Millisecond, stop)
			started := time.Now()
			result = host.Run(ctx, marker+"7 & "+marker+"8; wait", CommandOptions{Cwd: folder, Timeout: time.Minute})
			if !result.Stopped || result.TimedOut || time.Since(started) > 10*time.Second {
				t.Fatalf("got %+v after %v", result, time.Since(started))
			}
			waitUntil(t, func() bool {
				output, _ := exec.Command("bash", "-c", "pgrep -f '"+marker+"[78]' || true").Output()
				return strings.TrimSpace(string(output)) == ""
			})
		})
	}
}

func TestAHostTimesACommandOut(t *testing.T) {
	for name, host := range hostsUnderTest(t) {
		t.Run(name, func(t *testing.T) {
			_, folder := project(t)
			result := host.Run(context.Background(), "echo started; sleep 20", CommandOptions{Cwd: folder, Timeout: 300 * time.Millisecond})
			if !result.TimedOut || result.Stopped || result.Output != "started\n" {
				t.Fatalf("got %+v", result)
			}
		})
	}
}

func TestAHostKeepsABackgroundJobsOutputUntilReadAndStopsIt(t *testing.T) {
	ctx := context.Background()
	for name, host := range hostsUnderTest(t) {
		t.Run(name, func(t *testing.T) {
			_, folder := project(t)
			id, err := host.StartJob(ctx, "echo listening; sleep 30", folder)
			if err != nil {
				t.Fatal(err)
			}
			waitUntil(t, func() bool {
				state, _ := host.ReadJob(ctx, id, false)
				return state != nil && state.Output == "listening\n"
			})
			state, _ := host.ReadJob(ctx, id, true)
			if state.Command != "echo listening; sleep 30" || !state.Running || state.Output != "listening\n" {
				t.Fatalf("got %+v", state)
			}
			if state, _ := host.ReadJob(ctx, id, true); state.Output != "" {
				t.Fatalf("read twice %+v", state)
			}
			if _, err := host.StopJob(ctx, id); err != nil {
				t.Fatal(err)
			}
			waitUntil(t, func() bool {
				state, _ := host.ReadJob(ctx, id, false)
				return !state.Running
			})
			quick, _ := host.StartJob(ctx, "echo built; exit 4", folder)
			waitUntil(t, func() bool {
				state, _ := host.ReadJob(ctx, quick, false)
				return !state.Running
			})
			state, _ = host.ReadJob(ctx, quick, true)
			if state.ExitCode == nil || *state.ExitCode != 4 || state.Output != "built\n" {
				t.Fatalf("got %+v", state)
			}
			if state, _ := host.ReadJob(ctx, "nope", true); state != nil {
				t.Fatalf("an unknown job read %+v", state)
			}
		})
	}
}

func TestHostOptionsTakeTheHostAliasesOfSSHConfigItsPatternsAside(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".ssh", "config"), "Host gmktec build\n  User me\nHost *.corp\nhost  laptop\n")
	if got := SSHConfigHosts(home); !reflect.DeepEqual(got, []string{"gmktec", "build", "laptop"}) {
		t.Fatalf("got %v", got)
	}
}

func TestHostOptionsAreThisComputerThenTheSSHEndpointsOnceEach(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".ssh", "config"), "Host gmktec\n")
	want := []protocol.HostOption{
		{Label: "This computer", Ref: protocol.LocalHost()},
		{Label: "me@box", Ref: protocol.HostRef{Value: protocol.HostRefSSH{Endpoint: "me@box"}}},
		{Label: "gmktec", Ref: protocol.HostRef{Value: protocol.HostRefSSH{Endpoint: "gmktec"}}},
	}
	if got := Options("linux", home, []string{"me@box", "gmktec"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestPosixPathsRelative(t *testing.T) {
	cases := map[[2]string]string{
		{"/a/b", "/a/b/c/d"}: "c/d",
		{"/a/b", "/a/x"}:     "../x",
		{"/a/b", "/a/b"}:     "",
	}
	for input, want := range cases {
		if got := PosixPaths.Rel(input[0], input[1]); got != want {
			t.Errorf("%v: got %q", input, got)
		}
	}
}

func waitUntil(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("it never happened")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
