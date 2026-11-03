package hosts

import (
	"path"
	"path/filepath"
	"strings"
)

// Paths is how a host writes paths: this computer's way, or POSIX for a host reached through its bash.
type Paths interface {
	Join(elements ...string) string
	Dir(name string) string
	Base(name string) string
	IsAbs(name string) bool
	Clean(name string) string
	// Rel is target from base, as Node's path.relative gives it: empty for the same path.
	Rel(base, target string) string
	// Resolve is name from base when it is relative, cleaned.
	Resolve(base, name string) string
	Separator() string
}

type nativePaths struct{}

// NativePaths are this computer's paths.
var NativePaths Paths = nativePaths{}

func (nativePaths) Join(elements ...string) string { return filepath.Join(elements...) }
func (nativePaths) Dir(name string) string         { return filepath.Dir(name) }
func (nativePaths) Base(name string) string        { return filepath.Base(name) }
func (nativePaths) IsAbs(name string) bool         { return filepath.IsAbs(name) }
func (nativePaths) Clean(name string) string       { return filepath.Clean(name) }
func (nativePaths) Separator() string              { return string(filepath.Separator) }

func (nativePaths) Rel(base, target string) string {
	relative, err := filepath.Rel(base, target)
	if err != nil || relative == "." {
		return relativeFallback(err, target)
	}
	return relative
}

func (nativePaths) Resolve(base, name string) string {
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}
	return filepath.Join(base, name)
}

func relativeFallback(err error, target string) string {
	if err != nil {
		return target
	}
	return ""
}

type posixPaths struct{}

// PosixPaths are the paths of a host reached through its bash.
var PosixPaths Paths = posixPaths{}

func (posixPaths) Join(elements ...string) string { return path.Join(elements...) }
func (posixPaths) Dir(name string) string         { return path.Dir(name) }
func (posixPaths) Base(name string) string        { return path.Base(name) }
func (posixPaths) IsAbs(name string) bool         { return path.IsAbs(name) }
func (posixPaths) Clean(name string) string       { return path.Clean(name) }
func (posixPaths) Separator() string              { return "/" }

func (posixPaths) Resolve(base, name string) string {
	if path.IsAbs(name) {
		return path.Clean(name)
	}
	return path.Join(base, name)
}

// Rel walks up from base to what it shares with target, then down to target.
func (posixPaths) Rel(base, target string) string {
	from := pathParts(path.Clean(base))
	to := pathParts(path.Clean(target))
	shared := 0
	for shared < len(from) && shared < len(to) && from[shared] == to[shared] {
		shared++
	}
	parts := []string{}
	for range from[shared:] {
		parts = append(parts, "..")
	}
	return strings.Join(append(parts, to[shared:]...), "/")
}

func pathParts(name string) []string {
	parts := []string{}
	for _, part := range strings.Split(name, "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}
