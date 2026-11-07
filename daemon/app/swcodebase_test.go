package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	. "swcode/daemon/testapp"
)

// fakeNpx stands in for npx running the skills CLI: it writes each command line and the folder it ran in to its log,
// then does what the line asks, in $HOME/.agents/skills with --global, else in the folder's.
const fakeNpx = `#!/bin/sh
printf '%s | %s\n' "$*" "$PWD" >> "$FAKE_NPX_LOG"
folder="$PWD/.agents/skills/swcode-base"
case " $* " in *" --global "*) folder="$HOME/.agents/skills/swcode-base" ;; esac
case " $* " in
  *" add "*) mkdir -p "$folder" && printf -- '---\nname: swcode-base\ndescription: Coding rules.\n---\n' > "$folder/SKILL.md" ;;
  *" remove "*) rm -rf "$folder" ;;
esac
`

// failingNpx is the skills CLI failing as it does when it cannot clone the repository: its frame, its terminal codes,
// its error line and the one that explains it.
const failingNpx = `#!/bin/sh
printf '\033[?25l\342\224\202\n\342\226\240  Failed to clone repository\n\342\224\202  Authentication failed for https://github.com/bioleyl/swcode.git.\n\342\224\224  Installation failed\n'
exit 1
`

// The command lines the daemon gives the skills CLI.
const (
	addToHome         = "--yes skills@1.7.6 add bioleyl/swcode --skill swcode-base --agent universal --yes --global"
	addToProject      = "--yes skills@1.7.6 add bioleyl/swcode --skill swcode-base --agent universal --yes"
	removeFromHome    = "--yes skills@1.7.6 remove swcode-base --agent universal --yes --global"
	removeFromProject = "--yes skills@1.7.6 remove swcode-base --agent universal --yes"
)

// withNpx puts a stand-in for npx first on the PATH, and makes home the HOME the commands run with, as it is this
// computer's home for the daemon: the skills CLI installs in $HOME with --global. What it ran is in the log it gives.
func withNpx(t *testing.T, script, home string) func() []string {
	t.Helper()
	bin := t.TempDir()
	put(t, bin, "npx", script)
	if err := os.Chmod(filepath.Join(bin, "npx"), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "npx.log")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", home)
	t.Setenv("FAKE_NPX_LOG", log)
	return func() []string {
		text, _ := os.ReadFile(log)
		return strings.Split(strings.TrimSpace(string(text)), "\n")
	}
}

func ran(command, folder string) string { return command + " | " + folder }

func TestTheDaemonSaysWhereSwcodeBaseIs(t *testing.T) {
	test := Start(t, Options{})
	project := test.App.Projects.Ensure(test.Workspace, protocol.LocalHost())
	_, before := test.Call("GET", protocol.EndpointSwcodeBase+"?project="+project.ID, nil)
	Expect(t, before, `{"inHome":false,"inProject":false,"installed":false}`)
}

func TestTheSkillsCLIInstallsSwcodeBaseInTheHomeAndTakesItOut(t *testing.T) {
	home := t.TempDir()
	commands := withNpx(t, fakeNpx, home)
	test := Start(t, Options{Home: home})
	_, installed := test.Call("PUT", protocol.EndpointSwcodeBaseHome, nil)
	Expect(t, installed, `{"inHome":true,"inProject":false,"installed":true}`)
	_, removed := test.Call("DELETE", protocol.EndpointSwcodeBaseHome, nil)
	Expect(t, removed, `{"inHome":false,"inProject":false,"installed":false}`)
	Expect(t, commands(), []string{ran(addToHome, home), ran(removeFromHome, home)})
}

func TestTheSkillsCLIInstallsSwcodeBaseInTheProjectAndTakesItOut(t *testing.T) {
	home := t.TempDir()
	commands := withNpx(t, fakeNpx, home)
	test := Start(t, Options{Home: home})
	project := test.App.Projects.Ensure(test.Workspace, protocol.LocalHost())
	_, installed := test.Call("POST", protocol.EndpointSwcodeBase+"?project="+project.ID, nil)
	Expect(t, installed, `{"inHome":false,"inProject":true,"installed":true}`)
	_, removed := test.Call("DELETE", protocol.EndpointSwcodeBase+"?project="+project.ID, nil)
	Expect(t, removed, `{"inHome":false,"inProject":false,"installed":false}`)
	Expect(t, commands(), []string{ran(addToProject, test.Workspace), ran(removeFromProject, test.Workspace)})
}

func TestTheSkillsCLIRunsOnTheHostTheProjectIsOn(t *testing.T) {
	remoteHome := t.TempDir()
	put(t, remoteHome, "app/README.md", "built on the box")
	home := t.TempDir()
	commands := withNpx(t, fakeNpx, home)
	remote := hosts.NewShellHost(hosts.LocalTransport("build-box", remoteHome))
	test := Start(t, Options{Home: home, ConnectHost: func(protocol.HostRef) hosts.Host { return remote }})
	folder := filepath.Join(remoteHome, "app")
	project := test.App.Projects.Ensure(folder, protocol.HostRef{Value: protocol.HostRefSSH{Endpoint: "build-box"}})
	query := "?project=" + project.ID
	_, installed := test.Call("PUT", protocol.EndpointSwcodeBaseHome+query, nil)
	Expect(t, installed, `{"inHome":true,"inProject":false,"installed":true}`)
	if _, err := os.Stat(filepath.Join(home, ".agents")); !os.IsNotExist(err) {
		t.Fatal("this computer's home got the skill meant for the other host's")
	}
	_, both := test.Call("POST", protocol.EndpointSwcodeBase+query, nil)
	Expect(t, both, `{"inHome":true,"inProject":true,"installed":true}`)
	_, removed := test.Call("DELETE", protocol.EndpointSwcodeBaseHome+query, nil)
	Expect(t, removed, `{"inHome":false,"inProject":true,"installed":true}`)
	_, gone := test.Call("DELETE", protocol.EndpointSwcodeBase+query, nil)
	Expect(t, gone, `{"inHome":false,"inProject":false,"installed":false}`)
	Expect(t, commands(), []string{
		ran(addToHome, remoteHome), ran(addToProject, folder), ran(removeFromHome, remoteHome), ran(removeFromProject, folder),
	})
}

func TestWhatTheSkillsCLISaysWhenItFailsIsTheProblem(t *testing.T) {
	home := t.TempDir()
	withNpx(t, failingNpx, home)
	test := Start(t, Options{Home: home})
	_, failed := test.Call("PUT", protocol.EndpointSwcodeBaseHome, nil)
	Expect(t, failed, map[string]any{
		"installed": false,
		"problem":   "npx skills could not install swcode-base on this computer: Failed to clone repository: Authentication failed for https://github.com/bioleyl/swcode.git.",
	})
}

func TestSwcodeBaseIsOnlyTheFolderOfItsName(t *testing.T) {
	home := t.TempDir()
	put(t, home, ".agents/skills/deploy/SKILL.md", deploySkill)
	// A skill of the person's that calls itself swcode-base, in a folder of its own.
	put(t, home, ".agents/skills/my-base/SKILL.md", "---\nname: swcode-base\ndescription: Mine.\n---\nMy own rules.\n")
	test := Start(t, Options{Home: home})
	project := test.App.Projects.Ensure(test.Workspace, protocol.LocalHost())
	_, status := test.Call("GET", protocol.EndpointSwcodeBase+"?project="+project.ID, nil)
	// The person has skills of their own: swcode-base is still nowhere, so the app offers it.
	Expect(t, status, `{"inHome":false,"inProject":false,"installed":false}`)
}
