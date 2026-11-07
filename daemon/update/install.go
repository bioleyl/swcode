package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
)

const (
	checksumsFile = "SHA256SUMS"
	// filePrefix starts the name of a release's command line, its system and processor after it.
	filePrefix = "swcode-"
	// The new program is written beside the old one, on the same disk, for a rename to put it in place.
	newSuffix = ".new"
	// Where Windows' old program goes while it still runs: the new one deletes it as it starts.
	oldSuffix   = ".old"
	programMode = 0o755
	// maxProgram and maxChecksums are far more than the program and its checksums weigh.
	maxProgram   = 512 << 20
	maxChecksums = 64 << 10
	mismatch     = "the download does not match the release's checksum: try again"
	// binaryMark is how sha256sum marks a file it read as binary, before its name.
	binaryMark = "*"
)

// extensions is what the release's file for a system ends with.
var extensions = map[string]string{"windows": ".exe"}

// replacements is how a system puts a new program in place of the one that runs, a rename over it otherwise.
var replacements = map[string]func(program, downloaded string) error{"windows": replaceAside}

// Program is what an update replaces: the program the daemon runs in, by the release's file built for it, and
// where it is. Without a path, nothing is replaced.
type Program struct {
	File string
	Path string
}

// CommandLine is the command line at path: its release's file is the one built for this system and processor.
func CommandLine(path string) Program {
	return Program{File: filePrefix + runtime.GOOS + "-" + runtime.GOARCH + extensions[runtime.GOOS], Path: path}
}

// install puts the release's file for the program in its place, once checked against the release's checksums.
func install(ctx context.Context, client *http.Client, release Release, program Program) error {
	downloaded, err := download(ctx, client, release, program)
	if err != nil {
		return err
	}
	if err := replace(program.Path, downloaded); err != nil {
		_ = os.Remove(downloaded)
		return err
	}
	return nil
}

// download saves the release's file for the program beside it, and gives where, once checked.
func download(ctx context.Context, client *http.Client, release Release, program Program) (string, error) {
	address, found := release.Files[program.File]
	if !found {
		return "", fmt.Errorf("version %s has no %s", release.Version, program.File)
	}
	expected, err := checksumOf(ctx, client, release, program.File)
	if err != nil {
		return "", err
	}
	partial := program.Path + newSuffix
	sum, err := save(ctx, client, address, partial)
	if err != nil {
		_ = os.Remove(partial)
		return "", err
	}
	if sum != expected {
		_ = os.Remove(partial)
		return "", errors.New(mismatch)
	}
	return partial, nil
}

// checksumOf is a file's checksum, as the release's SHA256SUMS lists it.
func checksumOf(ctx context.Context, client *http.Client, release Release, name string) (string, error) {
	address, found := release.Files[checksumsFile]
	if !found {
		return "", fmt.Errorf("version %s has no checksums", release.Version)
	}
	body, err := fetch(ctx, client, address)
	if err != nil {
		return "", err
	}
	defer body.Close()
	listed, err := io.ReadAll(io.LimitReader(body, maxChecksums))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(listed), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], binaryMark) == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("version %s's checksums do not list %s", release.Version, name)
}

// fetch is what an address serves: an answer other than 200 is an error.
func fetch(ctx context.Context, client *http.Client, address string) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("the download did not start: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("the download failed: %s answered %s", address, response.Status)
	}
	return response.Body, nil
}

// save writes what an address serves to a file, and gives its checksum.
func save(ctx context.Context, client *http.Client, address, path string) (string, error) {
	body, err := fetch(ctx, client, address)
	if err != nil {
		return "", err
	}
	defer body.Close()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, programMode)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copied := io.Copy(io.MultiWriter(file, hash), io.LimitReader(body, maxProgram))
	if err := errors.Join(copied, file.Close()); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// replace puts the downloaded program in place of the one that runs: a program goes on from the file it started
// from, whatever takes its name.
func replace(program, downloaded string) error {
	if strategy, found := replacements[runtime.GOOS]; found {
		return strategy(program, downloaded)
	}
	return os.Rename(downloaded, program)
}

// replaceAside is Windows' way: a program that runs cannot be replaced or deleted, but it can be renamed, so it
// moves aside first, and goes when the new one starts (removeLeftover).
func replaceAside(program, downloaded string) error {
	old := program + oldSuffix
	_ = os.Remove(old)
	if err := os.Rename(program, old); err != nil {
		return err
	}
	if err := os.Rename(downloaded, program); err != nil {
		_ = os.Rename(old, program)
		return err
	}
	return nil
}

// removeLeftover deletes the program an update moved aside on Windows, once the new one runs; it may still be
// ending, and goes at a later start then.
func removeLeftover(program string) {
	if program == "" {
		return
	}
	_ = os.Remove(program + oldSuffix)
}
