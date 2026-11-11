//go:build linux

package local

import (
	"strings"
	"testing"
)

func TestTheUnitRunsTheDaemonAsAService(t *testing.T) {
	unit := serviceUnit("/home/me/.local/bin/swcode")
	for _, line := range []string{
		"ExecStart=/home/me/.local/bin/swcode serve --service",
		"Restart=on-failure",
		"WantedBy=default.target",
	} {
		if !strings.Contains(unit, line) {
			t.Fatalf("the unit has no %q:\n%s", line, unit)
		}
	}
}

func TestAPathWithASpaceIsQuotedInTheUnit(t *testing.T) {
	if got := execWord("/home/me/My Programs/swcode"); got != `"/home/me/My Programs/swcode"` {
		t.Fatalf("found %s", got)
	}
}

func TestTheUnitLivesWhereSystemdLooksForIt(t *testing.T) {
	place := Place{Home: "/home/me", Environment: map[string]string{}}
	if unitPath(place) != "/home/me/.config/systemd/user/swcode.service" {
		t.Fatalf("found %s", unitPath(place))
	}
	withXDG := Place{Home: "/home/me", Environment: map[string]string{"XDG_CONFIG_HOME": "/home/me/.config-xdg"}}
	if unitPath(withXDG) != "/home/me/.config-xdg/systemd/user/swcode.service" {
		t.Fatalf("found %s", unitPath(withXDG))
	}
}
