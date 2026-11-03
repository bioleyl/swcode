// Package web is how the agents reach the web, from the daemon: DuckDuckGo's search, and pages turned into text for
// a model of their own to read.
package web

import (
	"context"

	"swcode/daemon/llm"
	"swcode/daemon/protocol"
)

// SearchResult is one result of a web search.
type SearchResult struct {
	Title string
	URL   string
	// Snippet is what the search engine quotes of the page.
	Snippet string
}

// SearchOutcome is the results, or why there are none.
type SearchOutcome struct {
	OK      bool
	Results []SearchResult
	Problem string
}

// Search is a web search, stopped with its context.
type Search func(ctx context.Context, query string) SearchOutcome

// Page is a web page as text, for a model to read.
type Page struct {
	// URL is where the page was found, after any redirection.
	URL   string
	Title string
	Text  string
	// Truncated: only the start of the page was kept.
	Truncated bool
}

// PageOutcome is a page, or why it cannot be read.
type PageOutcome struct {
	OK      bool
	Page    Page
	Problem string
}

// ReadOutcome is what the reader answered about a page, or why the page cannot be read.
type ReadOutcome struct {
	OK      bool
	Page    Page
	Answer  string
	Problem string
}

// Port is the web as the tools reach it, from the daemon.
type Port struct {
	Search Search
	// Read is a page's answer to a question: the page is read by a model of its own, so its text stays out of the
	// agent's.
	Read func(ctx context.Context, url, question string) ReadOutcome
}

// Reader is the model that reads pages, the subagents' one: faster, and thinking little.
type Reader struct {
	Model     llm.Model
	Source    string
	ModelName string
	Thinking  protocol.ThinkingMode
	NewID     func() string
}
