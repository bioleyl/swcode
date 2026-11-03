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
		"CI=1 NODE_ENV=test npx vitest run":       {"npx"},
	}
	for text, want := range cases {
		if got, known := CommandPrograms(text); !known || !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v", text, got)
		}
	}
	for _, opaque := range []string{"echo $(rm -rf /)", "echo `whoami`", "npm test > out.txt", "sleep 10 & rm file", "(cd /tmp && rm x)", "ls\nrm x", "   "} {
		if _, known := CommandPrograms(opaque); known {
			t.Errorf("%q passed", opaque)
		}
	}
}
