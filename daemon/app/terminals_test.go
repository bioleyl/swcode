package app_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	. "swcode/daemon/testapp"
)

const terminalWait = 5 * time.Second

// terminalText reads what the daemon sends of a terminal until it says the text.
func terminalText(t *testing.T, socket *client, id, text string) string {
	t.Helper()
	said := ""
	deadline := time.Now().Add(terminalWait)
	for !strings.Contains(said, text) {
		if time.Now().After(deadline) {
			t.Fatalf("the terminal said %q, not %q", said, text)
		}
		message := socket.next("terminalOutput")
		if message["terminalId"] != id {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(message["data"].(string))
		if err != nil {
			t.Fatal(err)
		}
		said += string(data)
	}
	return said
}

func TestASocketOpensATerminalTypesInItAndClosesIt(t *testing.T) {
	test := startHello(t)
	socket := connect(t, test)
	if list := socket.next("terminals")["terminals"].([]any); len(list) != 0 {
		t.Fatalf("%d terminals at connection", len(list))
	}
	socket.send(map[string]any{"columns": 100, "cwd": test.Workspace, "rows": 30, "type": "openTerminal"})
	if list := socket.next("terminals")["terminals"].([]any); len(list) != 1 {
		t.Fatalf("%d terminals after opening one", len(list))
	}
	opened := socket.next("terminalOpened")
	terminal := opened["terminal"].(map[string]any)
	if terminal["title"] != "Terminal" || terminal["cwd"] != test.Workspace {
		t.Fatalf("opened %v", terminal)
	}
	id := terminal["id"].(string)
	socket.send(map[string]any{"data": "echo hello-from-the-socket\n", "terminalId": id, "type": "terminalInput"})
	terminalText(t, socket, id, "hello-from-the-socket")
	socket.send(map[string]any{"terminalId": id, "type": "closeTerminal"})
	if list := socket.next("terminals")["terminals"].([]any); len(list) != 0 {
		t.Fatalf("%d terminals after closing the only one", len(list))
	}
	if list := test.App.Terminals.List(); len(list) != 0 {
		t.Fatalf("the daemon still lists %d terminals", len(list))
	}
}

func TestASocketIsRefusedATerminalInAFolderThatIsNotThere(t *testing.T) {
	test := startHello(t)
	socket := connect(t, test)
	socket.next("terminals")
	socket.send(map[string]any{"columns": 80, "cwd": test.Workspace + "/no-such-folder", "rows": 24, "type": "openTerminal"})
	if problem := socket.next("problem")["message"].(string); !strings.Contains(problem, "no-such-folder") {
		t.Fatalf("refused with %q", problem)
	}
}

func TestATurnOpensATerminalWithATitleThePersonWatches(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{
		call("terminal", map[string]any{"command": "echo dev-server-here; sleep 30", "title": "dev server"}),
		text("The dev server runs in its terminal."),
	}})
	sessionID := prompt(t, test, "Run the dev server")
	answerNext(t, test, sessionID, protocol.PermissionDecisionAllowOnce, 1)
	test.UntilIdle(sessionID)
	list := test.App.Terminals.List()
	if len(list) != 1 || list[0].Title != "dev server" {
		t.Fatalf("terminals %v", list)
	}
	if output := string(test.App.Terminals.Output(list[0].ID)); !strings.Contains(output, "dev-server-here") {
		t.Fatalf("the terminal said %q", output)
	}
	if !strings.Contains(RequestText(test.Requests()[1]), "dev-server-here") {
		t.Fatal("the model did not get what the terminal printed")
	}
}

func TestATurnReadsTheLastLinesATerminalPrinted(t *testing.T) {
	test := Start(t, Options{Replies: []fakellama.Reply{
		call("terminal", map[string]any{"command": "echo build-error-here; exit 1", "title": "build"}),
		call("readterminal", map[string]any{"lines": 5, "terminal": "build"}),
		text("The build failed with build-error-here."),
	}})
	sessionID := prompt(t, test, "Build it in a terminal")
	answerNext(t, test, sessionID, protocol.PermissionDecisionAllowOnce, 1)
	test.UntilIdle(sessionID)
	read := Finished(test.Events(sessionID))
	if len(read) != 2 || !strings.Contains(read[1].Output, "build-error-here") {
		t.Fatalf("the terminal output read %v", read)
	}
	if !strings.Contains(RequestText(test.Requests()[2]), "build-error-here") {
		t.Fatal("the model did not get the terminal's last lines")
	}
}
