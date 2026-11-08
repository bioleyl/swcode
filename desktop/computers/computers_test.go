package computers_test

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"swcode/daemon/protocol"
	. "swcode/daemon/testapp"
	"swcode/desktop/computers"
)

func TestAnAddressIsReadAsAPersonTypesIt(t *testing.T) {
	for typed, expected := range map[string]computers.Target{
		"gmktec":                           {Address: "http://gmktec:4500", Name: "gmktec"},
		" 192.168.1.20:4600 ":              {Address: "http://192.168.1.20:4600", Name: "192.168.1.20"},
		"https://gmktec.tailnet.ts.net/x":  {Address: "https://gmktec.tailnet.ts.net", Name: "gmktec.tailnet.ts.net"},
		"http://gmktec:4500/?token=abc123": {Address: "http://gmktec:4500", Name: "gmktec", Token: "abc123"},
	} {
		if found, err := computers.TargetOf(typed); err != nil || found != expected {
			t.Fatalf("%q: expected %+v, found %+v (%v)", typed, expected, found, err)
		}
	}
	if _, err := computers.TargetOf("  "); err == nil {
		t.Fatal("an empty address was taken")
	}
}

func issueCode(t *testing.T, test *App) string {
	t.Helper()
	_, issued := test.Call("GET", protocol.EndpointPairingCode, nil)
	return issued.(map[string]any)["code"].(string)
}

func TestACodeIsTradedForATokenOfTheAppsOwnOnce(t *testing.T) {
	test := Start(t, Options{})
	code := issueCode(t, test)
	computer, err := computers.Connect(context.Background(), http.DefaultClient, test.Server.URL, code, "swcode on LAPTOP")
	if err != nil || computer.Token == "" || computer.Token == Token || computer.Address != test.Server.URL {
		t.Fatalf("found %+v (%v)", computer, err)
	}
	_, devices := test.Call("GET", protocol.EndpointDevices, nil)
	Expect(t, devices, `{"devices":[{"name":"swcode on LAPTOP"}]}`)
	if _, err := computers.Connect(context.Background(), http.DefaultClient, test.Server.URL, code, "again"); err == nil || !strings.Contains(err.Error(), "works once") {
		t.Fatalf("a used code was taken: %v", err)
	}
}

func TestALinkWithItsTokenNeedsNoCodeAndAWrongOneIsRefused(t *testing.T) {
	test := Start(t, Options{})
	computer, err := computers.Connect(context.Background(), http.DefaultClient, test.Server.URL+"/?token="+Token, "", "")
	if err != nil || computer.Token != Token {
		t.Fatalf("found %+v (%v)", computer, err)
	}
	if _, err := computers.Connect(context.Background(), http.DefaultClient, test.Server.URL+"/?token=wrong", "", ""); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("a wrong token was taken: %v", err)
	}
	if _, err := computers.Connect(context.Background(), http.DefaultClient, test.Server.URL, "", ""); err == nil || !strings.Contains(err.Error(), "code") {
		t.Fatalf("an address without code nor token was taken: %v", err)
	}
}

func TestAComputerNoSwcodeAnswersOnSaysSo(t *testing.T) {
	_, err := computers.Connect(context.Background(), http.DefaultClient, "127.0.0.1:1", "123456", "")
	if err == nil || !strings.Contains(err.Error(), "127.0.0.1:1 did not answer") {
		t.Fatalf("found %v", err)
	}
}

func TestAComputerKeepsTheNameThePersonGivesIt(t *testing.T) {
	file := filepath.Join(t.TempDir(), "computers.json")
	store := computers.Load(file)
	_ = store.Add(computers.Computer{Address: "http://192.168.1.20:4500", ID: "a", Name: "192.168.1.20", Token: "t1"})
	if err := store.Rename("a", "gmktec"); err != nil {
		t.Fatal(err)
	}
	again := computers.Load(file)
	renamed, found := again.Find("a")
	if !found || renamed.Name != "gmktec" || renamed.Address != "http://192.168.1.20:4500" || renamed.Token != "t1" {
		t.Fatalf("found %+v, kept %v", renamed, found)
	}
	if err := again.Rename("missing", "other"); err == nil {
		t.Fatal("a computer that is not kept was renamed")
	}
}

func TestTheStoreKeepsTheComputersAndTheOneShownLast(t *testing.T) {
	file := filepath.Join(t.TempDir(), "computers.json")
	store := computers.Load(file)
	if store.Current() != protocol.ThisComputer || len(store.All()) != 0 {
		t.Fatalf("a new store has computers: %+v", store.All())
	}
	_ = store.Add(computers.Computer{Address: "http://gmktec:4500", ID: "a", Name: "gmktec", Token: "t1"})
	_ = store.Add(computers.Computer{Address: "http://gmktec:4500", ID: "b", Name: "gmktec", Token: "t2"})
	_ = store.SetCurrent("b")
	again := computers.Load(file)
	if all := again.All(); len(all) != 1 || all[0].Token != "t2" || again.Current() != "b" {
		t.Fatalf("found %+v, current %s", all, again.Current())
	}
	_ = again.Forget("b")
	if computers.Load(file).Current() != protocol.ThisComputer {
		t.Fatal("a forgotten computer is still the one shown")
	}
}
