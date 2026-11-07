package app_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"swcode"

	"swcode/daemon/hosts"
	"swcode/daemon/protocol"
	. "swcode/daemon/testapp"
)

// packSkill is a skill of the pack the program carries, read the way the program does: the pack is what
// swcode.SwcodeBaseSkills gives, whatever folder the test runs from.
func packSkill(t *testing.T) string {
	t.Helper()
	text, err := fs.ReadFile(swcode.SwcodeBaseSkills(), "swcode-base/SKILL.md")
	if err != nil {
		t.Fatalf("the program carries no swcode-base skill: %v", err)
	}
	return string(text)
}

func TestTheDaemonSaysWhereTheSwcodeBaseSkillsAre(t *testing.T) {
	test := Start(t, Options{})
	project := test.App.Projects.Ensure(test.Workspace, protocol.LocalHost())
	_, before := test.Call("GET", protocol.EndpointSwcodeBase+"?project="+project.ID, nil)
	Expect(t, before, `{"carried":true,"inHome":false,"inProject":false,"installed":false,"names":["swcode-base"]}`)
}

func TestTheSwcodeBaseSkillsAreInstalledInTheHomeAndTakenOutOfIt(t *testing.T) {
	test := Start(t, Options{})
	_, installed := test.Call("PUT", protocol.EndpointSwcodeBaseHome, nil)
	Expect(t, installed, `{"carried":true,"inHome":true,"inProject":false,"installed":true,"names":["swcode-base"]}`)
	written, err := os.ReadFile(filepath.Join(test.Home, ".agents", "skills", "swcode-base", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != packSkill(t) {
		t.Fatal("the skill was written changed")
	}
	if _, err := os.Stat(filepath.Join(test.Home, ".agents", "skills", "swcode-base", "references", "solid.md")); err != nil {
		t.Fatal("a file of its folder was not written:", err)
	}
	_, removed := test.Call("DELETE", protocol.EndpointSwcodeBaseHome, nil)
	Expect(t, removed, `{"carried":true,"inHome":false,"inProject":false,"installed":false,"names":["swcode-base"]}`)
	if _, err := os.Stat(filepath.Join(test.Home, ".agents", "skills", "swcode-base")); !os.IsNotExist(err) {
		t.Fatal("the folder stayed")
	}
}

func TestTheSwcodeBaseSkillsAreInstalledInTheProjectAndTakenOutOfIt(t *testing.T) {
	test := Start(t, Options{})
	project := test.App.Projects.Ensure(test.Workspace, protocol.LocalHost())
	_, installed := test.Call("POST", protocol.EndpointSwcodeBase+"?project="+project.ID, nil)
	Expect(t, installed, `{"carried":true,"inHome":false,"inProject":true,"installed":true,"names":["swcode-base"]}`)
	inProject := filepath.Join(test.Workspace, ".agents", "skills", "swcode-base", "SKILL.md")
	writtenToProject, err := os.ReadFile(inProject)
	if err != nil {
		t.Fatal(err)
	}
	if string(writtenToProject) != packSkill(t) {
		t.Fatal("the project's copy differs")
	}
	_, removed := test.Call("DELETE", protocol.EndpointSwcodeBase+"?project="+project.ID, nil)
	Expect(t, removed, `{"carried":true,"inHome":false,"inProject":false,"installed":false,"names":["swcode-base"]}`)
	if _, err := os.Stat(inProject); !os.IsNotExist(err) {
		t.Fatal("the project kept its copy")
	}
}

func TestTheSwcodeBaseSkillsGoOnTheHostTheProjectIsOn(t *testing.T) {
	remoteHome := t.TempDir()
	put(t, remoteHome, "app/README.md", "built on the box")
	remote := hosts.NewShellHost(hosts.LocalTransport("build-box", remoteHome))
	test := Start(t, Options{ConnectHost: func(protocol.HostRef) hosts.Host { return remote }})
	project := test.App.Projects.Ensure(filepath.Join(remoteHome, "app"), protocol.HostRef{Value: protocol.HostRefSSH{Endpoint: "build-box"}})
	query := "?project=" + project.ID
	_, installed := test.Call("PUT", protocol.EndpointSwcodeBaseHome+query, nil)
	Expect(t, installed, `{"carried":true,"inHome":true,"inProject":false,"installed":true,"names":["swcode-base"]}`)
	inHome := filepath.Join(remoteHome, ".agents", "skills", "swcode-base")
	if _, err := os.Stat(filepath.Join(inHome, "references", "solid.md")); err != nil {
		t.Fatal("the other host's home did not get the skill:", err)
	}
	if _, err := os.Stat(filepath.Join(test.Home, ".agents")); !os.IsNotExist(err) {
		t.Fatal("this computer's home got the skill meant for the other host's")
	}
	_, both := test.Call("POST", protocol.EndpointSwcodeBase+query, nil)
	Expect(t, both, `{"carried":true,"inHome":true,"inProject":true,"installed":true,"names":["swcode-base"]}`)
	_, removed := test.Call("DELETE", protocol.EndpointSwcodeBaseHome+query, nil)
	Expect(t, removed, `{"carried":true,"inHome":false,"inProject":true,"installed":true,"names":["swcode-base"]}`)
	if _, err := os.Stat(inHome); !os.IsNotExist(err) {
		t.Fatal("the other host's home kept the skill")
	}
	_, gone := test.Call("DELETE", protocol.EndpointSwcodeBase+query, nil)
	Expect(t, gone, `{"carried":true,"inHome":false,"inProject":false,"installed":false,"names":["swcode-base"]}`)
}

func TestRemovingTheSwcodeBaseSkillsLeavesThePersonsOwnSkills(t *testing.T) {
	home := t.TempDir()
	// A skill of the person's that calls itself swcode-base, in a folder of its own.
	put(t, home, ".agents/skills/my-base/SKILL.md", "---\nname: swcode-base\ndescription: Mine.\n---\nMy own rules.\n")
	test := Start(t, Options{Home: home})
	_, before := test.Call("GET", protocol.EndpointSwcodeBase, nil)
	Expect(t, before, `{"carried":true,"inHome":false,"inProject":false,"installed":false,"names":["swcode-base"]}`)
	test.Call("PUT", protocol.EndpointSwcodeBaseHome, nil)
	_, removed := test.Call("DELETE", protocol.EndpointSwcodeBaseHome, nil)
	Expect(t, removed, `{"carried":true,"inHome":false,"inProject":false,"installed":false,"names":["swcode-base"]}`)
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "my-base", "SKILL.md")); err != nil {
		t.Fatal("the person's own skill went with the pack:", err)
	}
}

func TestNoSwcodeBaseSkillsAreOfferedWhereAnySkillIs(t *testing.T) {
	home := t.TempDir()
	put(t, home, ".agents/skills/deploy/SKILL.md", deploySkill)
	test := Start(t, Options{Home: home})
	project := test.App.Projects.Ensure(test.Workspace, protocol.LocalHost())
	_, status := test.Call("GET", protocol.EndpointSwcodeBase+"?project="+project.ID, nil)
	// The person has a skill of their own: the pack is still nowhere, so it is offered, with the home taken.
	Expect(t, status, `{"carried":true,"inHome":false,"inProject":false,"installed":false,"names":["swcode-base"]}`)
}
