package computers

import (
	"net/url"
	"strings"
)

// OriginOf is where a page or a computer's address is from (scheme, host and port), lower-cased; empty for what is no
// address of a page.
func OriginOf(link string) string {
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

// FromOneOf tells whether a page is a page of one of these addresses: the app answers the pages of the computers it
// knows, and no other page that came to be in its window.
func FromOneOf(page string, addresses []string) bool {
	origin := OriginOf(page)
	if origin == "" {
		return false
	}
	for _, address := range addresses {
		if OriginOf(address) == origin {
			return true
		}
	}
	return false
}
