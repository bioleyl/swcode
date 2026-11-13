package permissions

import (
	"reflect"
	"testing"

	"swcode/daemon/protocol"
)

var ask = Grants{Mode: protocol.PermissionModeAsk, Programs: map[string]bool{}}

func edit(path string) Subject    { return Subject{Access: AccessEdit, Path: path} }
func command(text string) Subject { return Subject{Access: AccessCommand, Command: text} }
func withMode(mode protocol.PermissionMode) Grants {
	return Grants{Mode: mode, Programs: map[string]bool{}}
}

func TestReadingIsNeverAskedAbout(t *testing.T) {
	if NeedsPermission(Read, ask) {
		t.Fatal("reading asked")
	}
}

func TestEditsAreAskedAboutUnlessTheModeAcceptsThem(t *testing.T) {
	if !NeedsPermission(edit("a.ts"), ask) || NeedsPermission(edit("a.ts"), withMode(protocol.PermissionModeAcceptEdits)) {
		t.Fatal("edits")
	}
}

func TestCommandsAreAskedAboutUnlessTheModeRunsEverything(t *testing.T) {
	if !NeedsPermission(command("ls"), withMode(protocol.PermissionModeAcceptEdits)) {
		t.Fatal("ls passed in acceptEdits")
	}
	if NeedsPermission(command("rm -rf dist"), withMode(protocol.PermissionModeAuto)) || NeedsPermission(edit("a.ts"), withMode(protocol.PermissionModeAuto)) {
		t.Fatal("auto asked")
	}
}

func TestAnEditAllowedForTheSessionLetsEveryEditThrough(t *testing.T) {
	if NeedsPermission(edit("b.ts"), GrantForSession(edit("a.ts"), ask)) {
		t.Fatal("asked again")
	}
}

func TestTheSameProgramsAreLetThroughAndOnlyThem(t *testing.T) {
	grants := GrantForSession(command("npm test && git status"), ask)
	if NeedsPermission(command("git diff"), grants) || !NeedsPermission(command("npm test; rm -rf dist"), grants) {
		t.Fatal("programs")
	}
	grants = GrantForSession(command("echo hi"), ask)
	if !NeedsPermission(command("echo $(cat secret)"), grants) {
		t.Fatal("a substitution passed")
	}
}

func TestCommandPrograms(t *testing.T) {
	cases := map[string][]string{
		"npm test -- --run":                       {"npm"},
		"npm run build && git status; ls | wc -l": {"npm", "git", "ls", "wc"},
		"git commit -m fix && npm run build":      {"git", "npm"},
	}
	for text, want := range cases {
		if got, known := CommandPrograms(text); !known || !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v", text, got)
		}
	}
	// A program's name vouches for nothing after a variable set for it, nor when it runs what its words say.
	unnamed := []string{
		"PATH=.:$PATH ls", "LD_PRELOAD=./x.so ls", "git -c core.fsmonitor=./payload.sh status", "git --exec-path=. status",
		"npm exec -y -- some-package", "timeout 5 sh ./payload.sh", "bash ./payload.sh", "sudo rm -rf /", "find . -exec rm {} +",
	}
	for _, opaque := range append(unnamed, "echo $(rm -rf /)", "echo `whoami`", "npm test > out.txt", "sleep 10 & rm file", "(cd /tmp && rm x)", "ls\nrm x", "   ") {
		if _, known := CommandPrograms(opaque); known {
			t.Errorf("%q passed", opaque)
		}
	}
}

func TestAnMCPServersToolsAreAskedAboutUntilTheServerIsAllowed(t *testing.T) {
	tool := func(server string) Subject { return Subject{Access: AccessServer, Server: server} }
	if !NeedsPermission(tool("github"), ask) || !NeedsPermission(tool("github"), withMode(protocol.PermissionModeAcceptEdits)) {
		t.Fatal("a server's tool passed without asking")
	}
	if NeedsPermission(tool("github"), withMode(protocol.PermissionModeAuto)) {
		t.Fatal("auto asked")
	}
	granted := GrantForSession(tool("github"), ask)
	if NeedsPermission(tool("github"), granted) || !NeedsPermission(tool("files"), granted) {
		t.Fatal("allowing a server covers its tools, and only its")
	}
	if !NeedsPermission(command("github"), granted) {
		t.Fatal("allowing a server allowed a program of its name")
	}
}

func TestACommandThatLeavesTheSandboxIsAskedAboutEvenWhereEverythingRuns(t *testing.T) {
	outside := func(text string) Subject { return Subject{Access: AccessCommand, Command: text, Outside: true} }
	auto := withMode(protocol.PermissionModeAuto)
	if NeedsPermission(command("git push"), auto) || !NeedsPermission(outside("git push"), auto) {
		t.Fatal("running everything covers the sandbox, not what leaves it")
	}
	granted := GrantForSession(outside("git push"), auto)
	if NeedsPermission(outside("git fetch"), granted) || !NeedsPermission(outside("npm publish"), granted) {
		t.Fatal("a program allowed outside the sandbox goes through, and only it")
	}
	// What was allowed inside the sandbox says nothing of the outside, nor the other way round.
	inside := GrantForSession(command("git status"), ask)
	if !NeedsPermission(outside("git push"), inside) || !NeedsPermission(command("git status"), GrantForSession(outside("git push"), ask)) {
		t.Fatal("a grant crossed the sandbox's wall")
	}
}
