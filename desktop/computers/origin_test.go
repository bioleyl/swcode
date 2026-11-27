package computers

import "testing"

func TestOnlyThePagesOfKnownComputersAreAnswered(t *testing.T) {
	known := []string{"http://localhost:4500", "http://gmktec:4500", "https://marvin.tail1234.ts.net"}
	for page, want := range map[string]bool{
		"http://localhost:4500/?token=abc#/sessions/1": true,
		"http://GMKTEC:4500/":                          true,
		"https://marvin.tail1234.ts.net/#/settings":    true,
		"http://localhost:4501/":                       false,
		"https://gmktec:4500/":                         false,
		"https://example.com/":                         false,
		"about:blank":                                  false,
		"":                                             false,
	} {
		if got := FromOneOf(page, known); got != want {
			t.Errorf("%q: %v", page, got)
		}
	}
}
