// Package version tells which build runs.
package version

// Build is set when the binary is built (version and commit, with -ldflags -X), dev when run from the sources.
var Build = "dev"
