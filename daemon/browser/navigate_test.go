package browser

import (
	"strings"
	"testing"
)

func TestOnlyWebAddressesOpenAndPrivateOnesNeedAPerson(t *testing.T) {
	cases := []struct {
		url      string
		approval bool
		problem  bool
	}{
		{"http://127.0.0.1:5173/", false, false},
		{"http://localhost:5173/", false, false},
		{"http://app.localhost/", false, false},
		{"http://[::1]:3000/", false, false},
		{"https://8.8.8.8/", false, false},
		{"http://10.1.2.3/", true, false},
		{"http://192.168.1.10:8080/", true, false},
		{"http://172.16.0.5/", true, false},
		{"http://169.254.1.1/", true, false},
		{"http://100.101.102.103/", true, false},
		// 192.168.1.1 as Chromium also reads it.
		{"http://3232235777/", true, false},
		{"http://0xC0A80101/", true, false},
		{"http://192.168.257/", true, false},
		{"http://0300.0250.1.1/", true, false},
		{"file:///etc/passwd", false, true},
		{"javascript:alert(1)", false, true},
		{"http://user:secret@example.com/", false, true},
		{"http:///nohost", false, true},
	}
	for _, each := range cases {
		got := CheckNavigation(each.url)
		if each.problem {
			if got.Problem == "" {
				t.Fatalf("%s opened", each.url)
			}
			continue
		}
		if got.Problem != "" || !got.Allowed || got.Approval != each.approval {
			t.Fatalf("%s: %+v", each.url, got)
		}
	}
}

func TestTheBrowserSaysWhenThereIsNoChromium(t *testing.T) {
	sessions := NewSessions("", t.TempDir(), false)
	_, err := sessions.For("s1").Open(t.Context(), "http://127.0.0.1/")
	if err == nil || !strings.Contains(err.Error(), "No Chromium") {
		t.Fatalf("err %v", err)
	}
}
