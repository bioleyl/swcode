package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"swcode/daemon/protocol"
)

const (
	githubJSON = "application/vnd.github+json"
	tagPrefix  = "v"
	// agentPrefix names swcode to GitHub, with its version.
	agentPrefix = "swcode/"
	// maxRelease is far more than a release's description and its files take.
	maxRelease = 1 << 20
	noVersion  = "GitHub's latest release has no version"
)

// Release is a version published on GitHub: what it brings, its page, and its files' addresses by name.
type Release struct {
	Version string
	Notes   string
	Page    string
	Files   map[string]string
}

func (r Release) public() protocol.Release {
	return protocol.Release{Notes: r.Notes, Page: r.Page, Version: r.Version}
}

// githubRelease is a release as GitHub's API describes it.
type githubRelease struct {
	Tag    string `json:"tag_name"`
	Body   string `json:"body"`
	Page   string `json:"html_url"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (g githubRelease) release() Release {
	files := map[string]string{}
	for _, asset := range g.Assets {
		files[asset.Name] = asset.URL
	}
	return Release{Files: files, Notes: g.Body, Page: g.Page, Version: strings.TrimPrefix(g.Tag, tagPrefix)}
}

// source asks GitHub's API for the latest release, saying what it answered last: an answer that it did not change
// does not count against the hourly limit GitHub sets for asking without an account.
type source struct {
	url    string
	client *http.Client
	agent  string
	etag   string
	last   Release
}

func newSource(url string, client *http.Client, version string) *source {
	return &source{agent: agentPrefix + version, client: client, url: url}
}

// latest is the latest release; a source asks once at a time.
func (s *source) latest(ctx context.Context) (Release, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return Release{}, err
	}
	request.Header.Set("accept", githubJSON)
	request.Header.Set("user-agent", s.agent)
	if s.etag != "" {
		request.Header.Set("if-none-match", s.etag)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return Release{}, fmt.Errorf("GitHub did not answer: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified {
		return s.last, nil
	}
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub answered %s", response.Status)
	}
	var found githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, maxRelease)).Decode(&found); err != nil {
		return Release{}, fmt.Errorf("GitHub's answer could not be read: %w", err)
	}
	release := found.release()
	if release.Version == "" {
		return Release{}, errors.New(noVersion)
	}
	s.etag, s.last = response.Header.Get("etag"), release
	return release, nil
}
