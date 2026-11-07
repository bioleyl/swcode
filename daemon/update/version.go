// Package update finds a newer swcode on GitHub and puts it in place of the program that runs: the settings'
// Updates topic, and the offer the app makes on a computer.
package update

import (
	"regexp"
	"slices"
	"strconv"
)

// versionNumbers is a release's version (0.4.0, its tag v0.4.0) or a build's (0.3.0+7bd0cd6): a build from the
// sources says dev.
var versionNumbers = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

// numbers is a version's three numbers, when it has them.
func numbers(version string) ([]int, bool) {
	found := versionNumbers.FindStringSubmatch(version)
	if found == nil {
		return nil, false
	}
	parsed := make([]int, 0, len(found)-1)
	for _, part := range found[1:] {
		number, _ := strconv.Atoi(part)
		parsed = append(parsed, number)
	}
	return parsed, true
}

// fromSources tells whether a build comes from the sources (dev) rather than a release: it never updates itself.
func fromSources(version string) bool {
	_, released := numbers(version)
	return !released
}

// Newer tells whether a release's version (v0.4.0) comes after the build that runs (0.3.0+7bd0cd6).
func Newer(release, running string) bool {
	latest, published := numbers(release)
	current, released := numbers(running)
	return published && released && slices.Compare(latest, current) > 0
}
