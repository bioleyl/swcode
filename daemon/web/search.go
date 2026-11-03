package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	duckDuckGo = "duckduckgo.com"
	// Its links to a result go through its own address, the result's in this parameter.
	redirectPath = "/l/"
	targetParam  = "uddg"
	// The status of its "are you a robot?" page.
	robotCheckStatus = http.StatusAccepted
	adClass          = "result--ad"
	formType         = "application/x-www-form-urlencoded"
	maxResults       = 8
	maxSnippetChars  = 300
	searchTimeout    = 30 * time.Second
	htmlPage         = "https://html.duckduckgo.com/html/"
	litePage         = "https://lite.duckduckgo.com/lite/"
	robotChecked     = "DuckDuckGo takes these searches for a robot’s for now: search again in a minute, or read the pages you know with webfetch."
)

var spaces = regexp.MustCompile(`\s+`)

// found is a result as the page shows it: its link still DuckDuckGo's.
type found struct {
	href    string
	title   string
	snippet string
}

// resultsPage is one of DuckDuckGo's pages without scripts: how to ask it, and where its results are.
type resultsPage struct {
	request func(ctx context.Context, query string) (*http.Request, error)
	found   func(document *goquery.Document) []found
}

func plain(text string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(text, " "))
}

func isDuckDuckGo(link *url.URL) bool {
	return link.Hostname() == duckDuckGo || strings.HasSuffix(link.Hostname(), "."+duckDuckGo)
}

// targetOf is where a result's link leads: its own address, or none for DuckDuckGo's own links (its ads).
func targetOf(href string) string {
	base, _ := url.Parse("https://" + duckDuckGo + "/")
	link, err := base.Parse(href)
	if err != nil || href == "" {
		return ""
	}
	target := link
	if isDuckDuckGo(link) && link.Path == redirectPath {
		if target, err = url.Parse(link.Query().Get(targetParam)); err != nil {
			return ""
		}
	}
	if (target.Scheme != "http" && target.Scheme != "https") || isDuckDuckGo(target) {
		return ""
	}
	return target.String()
}

// htmlResults is html.duckduckgo.com: a block for each result, its ads marked.
var htmlResults = resultsPage{
	found: func(document *goquery.Document) []found {
		results := []found{}
		document.Find(".result").Each(func(_ int, result *goquery.Selection) {
			if result.HasClass(adClass) {
				return
			}
			link := result.Find("a.result__a").First()
			href, _ := link.Attr("href")
			results = append(results, found{href: href, snippet: result.Find(".result__snippet").First().Text(), title: link.Text()})
		})
		return results
	},
	request: func(ctx context.Context, query string) (*http.Request, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, htmlPage, strings.NewReader(url.Values{"q": {query}}.Encode()))
		if err == nil {
			request.Header.Set("content-type", formType)
		}
		return request, err
	},
}

// liteResults is lite.duckduckgo.com: a table, a row for each result's link, then one for its snippet.
var liteResults = resultsPage{
	found: func(document *goquery.Document) []found {
		results := []found{}
		document.Find("tr").Each(func(_ int, row *goquery.Selection) {
			link := row.Find("a.result-link").First()
			snippet := row.Find("td.result-snippet").First()
			if link.Length() > 0 {
				href, _ := link.Attr("href")
				results = append(results, found{href: href, title: link.Text()})
				return
			}
			if snippet.Length() > 0 && len(results) > 0 {
				results[len(results)-1].snippet = snippet.Text()
			}
		})
		return results
	},
	request: func(ctx context.Context, query string) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, litePage+"?"+url.Values{"q": {query}}.Encode(), nil)
	},
}

// pages: the lite page is turned away apart from the other, and answers when the first takes swcode for a robot.
var pages = []resultsPage{htmlResults, liteResults}

// resultsOf is the first few results that lead somewhere, one per page.
func resultsOf(found []found) []SearchResult {
	results := []SearchResult{}
	seen := map[string]bool{}
	for _, result := range found {
		target := targetOf(result.href)
		if target == "" || seen[target] || len(results) >= maxResults {
			continue
		}
		seen[target] = true
		snippet, _ := clip(plain(result.snippet), maxSnippetChars)
		results = append(results, SearchResult{Snippet: snippet, Title: plain(result.title), URL: target})
	}
	return results
}

func ask(ctx context.Context, page resultsPage, query string, client *http.Client) SearchOutcome {
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	request, err := page.request(ctx, query)
	if err != nil {
		return SearchOutcome{Problem: "DuckDuckGo could not be asked: " + err.Error()}
	}
	request.Header.Set("user-agent", UserAgent)
	response, err := client.Do(request)
	if err != nil {
		return SearchOutcome{Problem: "DuckDuckGo could not be asked: " + err.Error()}
	}
	defer response.Body.Close()
	if response.StatusCode == robotCheckStatus {
		return SearchOutcome{Problem: robotChecked}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return SearchOutcome{Problem: fmt.Sprintf("DuckDuckGo answered %d.", response.StatusCode)}
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return SearchOutcome{Problem: "DuckDuckGo could not be asked: " + err.Error()}
	}
	document, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return SearchOutcome{Problem: "DuckDuckGo could not be asked: " + err.Error()}
	}
	return SearchOutcome{OK: true, Results: resultsOf(page.found(document))}
}

// SearchWeb searches the web with DuckDuckGo, from the daemon: no server to set up. A page without results goes on
// to the next one; none found on either is the answer.
func SearchWeb(ctx context.Context, query string, client *http.Client) SearchOutcome {
	outcomes := []SearchOutcome{}
	for _, page := range pages {
		outcome := ask(ctx, page, query, client)
		if outcome.OK && len(outcome.Results) > 0 {
			return outcome
		}
		outcomes = append(outcomes, outcome)
	}
	for _, outcome := range outcomes {
		if outcome.OK {
			return outcome
		}
	}
	return outcomes[len(outcomes)-1]
}

// SearchGap: DuckDuckGo turns away a few searches in a row, but not searches 5 s apart.
const SearchGap = 5 * time.Second

var stopped = SearchOutcome{Problem: "The search was stopped."}

// Paced makes searches one at a time, some time apart, whichever session asks: each waits for its turn.
func Paced(search Search, gap time.Duration) Search {
	turn := make(chan time.Time, 1)
	turn <- time.Time{}
	return func(ctx context.Context, query string) SearchOutcome {
		var last time.Time
		select {
		case last = <-turn:
		case <-ctx.Done():
			return stopped
		}
		wait := time.NewTimer(time.Until(last.Add(gap)))
		defer wait.Stop()
		select {
		case <-wait.C:
		case <-ctx.Done():
			turn <- last
			return stopped
		}
		outcome := search(ctx, query)
		turn <- time.Now()
		return outcome
	}
}
