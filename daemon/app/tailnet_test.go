package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"swcode/daemon/config"
	"swcode/daemon/fakellama"
	"swcode/daemon/protocol"
	. "swcode/daemon/testapp"
)

// fakeTailscale is a Tailscale on a tailnet with certificates, which serves a port once it was asked to.
type fakeTailscale struct {
	serving string
}

func (f *fakeTailscale) run(_ context.Context, args ...string) ([]byte, error) {
	switch command := strings.Join(args, " "); {
	case command == "status --json":
		return []byte(`{"BackendState":"Running","CertDomains":["laptop.tail1234.ts.net"]}`), nil
	case command == "serve status --json" && f.serving != "":
		return []byte(`{"Web":{"laptop.tail1234.ts.net:` + f.serving + `":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:` + f.serving + `"}}}}}`), nil
	case strings.HasPrefix(command, "serve --bg --https="):
		f.serving = strings.TrimPrefix(args[2], "--https=")
	}
	return []byte(`{}`), nil
}

func tailnetOf(t *testing.T, body any) protocol.Tailnet {
	t.Helper()
	data, _ := json.Marshal(body)
	var state protocol.Tailnet
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

// publicURLOf is the address the daemon's status gives a phone to open; empty when it gives none.
func publicURLOf(test *App) string {
	_, status := test.Call("GET", protocol.EndpointStatus, nil)
	address, _ := status.(map[string]any)["publicUrl"].(string)
	return address
}

func TestTheTailnetGivesTheQRCodeAnHTTPSAddressOnceTailscaleServesSwcodeThere(t *testing.T) {
	tailscale := &fakeTailscale{}
	test := Start(t, Options{Replies: []fakellama.Reply{}, Tailscale: tailscale.run})
	address := fmt.Sprintf("https://laptop.tail1234.ts.net:%d", config.DefaultPort)

	_, body := test.Call("GET", protocol.EndpointTailnet, nil)
	if state := tailnetOf(t, body); !state.Available || state.Served || state.Address != address {
		t.Fatalf("before serving: %+v", state)
	}
	if public := publicURLOf(test); public != "" {
		t.Fatalf("a public address before serving: %q", public)
	}

	_, body = test.Call("POST", protocol.EndpointTailnet, nil)
	if state := tailnetOf(t, body); !state.Served || state.Address != address {
		t.Fatalf("once served: %+v", state)
	}
	if public := publicURLOf(test); public != address {
		t.Fatalf("the public address once served: %q", public)
	}
}

func TestADaemonWithoutTailscaleHasNoTailnetAddress(t *testing.T) {
	test := startHello(t)
	_, body := test.Call("GET", protocol.EndpointTailnet, nil)
	if state := tailnetOf(t, body); state.Available || state.Served || state.Address != "" {
		t.Fatalf("without Tailscale: %+v", state)
	}
}
