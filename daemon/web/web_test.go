package web

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"swcode/daemon/llm"
	"swcode/daemon/protocol"
)

const page = `<!doctype html><html><head><title>Timeouts &amp; limits</title><script>steal()</script></head>
<body><nav><a href="/">Home</a> · <a href="/docs">Docs</a></nav>
<main><h1>Timeouts</h1><p>Set <code>requestTimeout</code> in milliseconds.</p>
<pre><code>fastify({ requestTimeout: 5000 })</code></pre></main>
<footer>© Example</footer></body></html>`

// DuckDuckGo's pages, as small as they can be: a result, an ad, one through its redirect, the same page again.
const htmlResultsPage = `<div class="results">
<div class="result results_links web-result"><h2 class="result__title"><a class="result__a" href="https://github.com/fastify/fastify-websocket">GitHub - fastify/<b>fastify-websocket</b></a></h2>
<a class="result__snippet">Close the connection on non-<b>websocket</b>
  routes &amp; more.</a></div>
<div class="result result--ad"><a class="result__a" href="https://duckduckgo.com/y.js?ad_domain=ads.test">An ad</a></div>
<div class="result web-result"><a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fwebsocket.org%2Freference%2Fclose-codes%2F&amp;rut=abc">WebSocket Close Codes</a><a class="result__snippet">1000 = normal closure.</a></div>
<div class="result web-result"><a class="result__a" href="https://github.com/fastify/fastify-websocket">Same page again</a></div>
</div>`

const liteResultsPage = `<table>
<tr><td>1.</td><td><a rel="nofollow" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fwww.npmjs.com%2Fpackage%2F%40fastify%2Fwebsocket&amp;rut=f6" class='result-link'>@fastify/websocket - npm</a></td></tr>
<tr><td></td><td class='result-snippet'>Basic <b>websocket</b> support.</td></tr>
<tr class='result-sponsored'><td>2.</td><td><a href="https://duckduckgo.com/y.js?ad=1" class='result-link'>Sponsored</a></td></tr>
<tr class='result-sponsored'><td></td><td class='result-snippet'>Buy now.</td></tr>
<tr><td>3.</td><td><a href="https://deno.land/" class='result-link'>Deno</a></td></tr>
</table>`

type asked struct {
	url    string
	method string
	body   string
	agent  string
}

type answer struct {
	status      int
	contentType string
	body        string
}

// fakeWeb answers requests by address, without the query, keeping what it was asked.
type fakeWeb struct {
	mu      sync.Mutex
	answers map[string]answer
	asked   []asked
}

func (f *fakeWeb) RoundTrip(request *http.Request) (*http.Response, error) {
	body := ""
	if request.Body != nil {
		data, _ := io.ReadAll(request.Body)
		body = string(data)
	}
	address := request.URL.String()
	f.mu.Lock()
	f.asked = append(f.asked, asked{agent: request.Header.Get("user-agent"), body: body, method: request.Method, url: address})
	f.mu.Unlock()
	found := answer{status: http.StatusNotFound}
	for prefix, each := range f.answers {
		if strings.HasPrefix(address, prefix) {
			found = each
		}
	}
	if found.status == 0 {
		found.status = http.StatusOK
	}
	header := http.Header{}
	if found.contentType != "" {
		header.Set("content-type", found.contentType)
	}
	return &http.Response{Body: io.NopCloser(strings.NewReader(found.body)), Header: header, Request: request, StatusCode: found.status}, nil
}

func clientOf(answers map[string]answer) (*http.Client, *fakeWeb) {
	fake := &fakeWeb{answers: answers}
	return &http.Client{Transport: fake}, fake
}

func htmlAnswer(body string) answer {
	return answer{body: body, contentType: "text/html; charset=utf-8"}
}

func TestSearchReadsDuckDuckGosResultsWithoutItsAdsOnePerPage(t *testing.T) {
	client, fake := clientOf(map[string]answer{htmlPage: htmlAnswer(htmlResultsPage)})
	got := SearchWeb(context.Background(), "websocket close codes", client)
	want := SearchOutcome{OK: true, Results: []SearchResult{
		{Snippet: "Close the connection on non-websocket routes & more.", Title: "GitHub - fastify/fastify-websocket", URL: "https://github.com/fastify/fastify-websocket"},
		{Snippet: "1000 = normal closure.", Title: "WebSocket Close Codes", URL: "https://websocket.org/reference/close-codes/"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if len(fake.asked) != 1 || fake.asked[0].method != http.MethodPost || fake.asked[0].body != "q=websocket+close+codes" || !strings.Contains(fake.asked[0].agent, "swcode") {
		t.Fatalf("asked %+v", fake.asked)
	}
}

func TestSearchAsksTheLitePageWhenTheFirstTakesItForARobot(t *testing.T) {
	client, fake := clientOf(map[string]answer{
		htmlPage: {status: robotCheckStatus},
		litePage: htmlAnswer(liteResultsPage),
	})
	got := SearchWeb(context.Background(), "fastify websocket", client)
	want := SearchOutcome{OK: true, Results: []SearchResult{
		{Snippet: "Basic websocket support.", Title: "@fastify/websocket - npm", URL: "https://www.npmjs.com/package/@fastify/websocket"},
		{Snippet: "", Title: "Deno", URL: "https://deno.land/"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if fake.asked[1].url != litePage+"?q=fastify+websocket" {
		t.Fatalf("asked %+v", fake.asked)
	}
}

func TestSearchFindsNoneOnlyWhenNeitherPageHasResults(t *testing.T) {
	client, fake := clientOf(map[string]answer{htmlPage: htmlAnswer("<p>none</p>"), litePage: htmlAnswer("<p>none</p>")})
	if got := SearchWeb(context.Background(), "x", client); !got.OK || len(got.Results) != 0 || len(fake.asked) != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestSearchSaysWhatWentWrongInsteadOfFailing(t *testing.T) {
	client, _ := clientOf(map[string]answer{htmlPage: {status: robotCheckStatus}, litePage: {status: robotCheckStatus}})
	if got := SearchWeb(context.Background(), "x", client); got.OK || !strings.HasPrefix(got.Problem, "DuckDuckGo takes these searches for a robot’s for now") {
		t.Fatalf("got %+v", got)
	}
	down := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, io.ErrUnexpectedEOF })}
	if got := SearchWeb(context.Background(), "x", down); got.OK || !strings.HasPrefix(got.Problem, "DuckDuckGo could not be asked") {
		t.Fatalf("got %+v", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func resultsFor(title string) SearchOutcome {
	return SearchOutcome{OK: true, Results: []SearchResult{{Title: title, URL: "https://" + title + ".test/"}}}
}

func TestPacedMakesSearchesOneAtATimeSomeTimeApart(t *testing.T) {
	const gap = 80 * time.Millisecond
	var mu sync.Mutex
	started := []time.Time{}
	search := Paced(func(_ context.Context, query string) SearchOutcome {
		mu.Lock()
		started = append(started, time.Now())
		mu.Unlock()
		return resultsFor(query)
	}, gap)
	var group sync.WaitGroup
	for _, query := range []string{"a", "b"} {
		group.Add(1)
		go func() {
			defer group.Done()
			search(context.Background(), query)
		}()
	}
	group.Wait()
	if started[1].Sub(started[0]) < gap-5*time.Millisecond {
		t.Fatalf("only %v apart", started[1].Sub(started[0]))
	}
}

func TestPacedLetsAStoppedSearchGoAndTheNextStillHasItsTurn(t *testing.T) {
	search := Paced(func(_ context.Context, query string) SearchOutcome { return resultsFor(query) }, 80*time.Millisecond)
	if got := search(context.Background(), "a"); !got.OK {
		t.Fatalf("got %+v", got)
	}
	ctx, stop := context.WithCancel(context.Background())
	stopped := make(chan SearchOutcome)
	go func() { stopped <- search(ctx, "b") }()
	time.Sleep(10 * time.Millisecond)
	stop()
	if got := <-stopped; got.Problem != "The search was stopped." {
		t.Fatalf("got %+v", got)
	}
	if got := search(context.Background(), "c"); got.Results[0].Title != "c" {
		t.Fatalf("got %+v", got)
	}
}

func TestHTMLTextKeepsTheMainContentAsMarkdown(t *testing.T) {
	title, text := HTMLText(page)
	if title != "Timeouts & limits" {
		t.Fatalf("title %q", title)
	}
	for _, kept := range []string{"# Timeouts", "Set `requestTimeout` in milliseconds.", "```\nfastify({ requestTimeout: 5000 })\n```"} {
		if !strings.Contains(text, kept) {
			t.Errorf("%q missing from %q", kept, text)
		}
	}
	for _, left := range []string{"Home", "Example", "steal"} {
		if strings.Contains(text, left) {
			t.Errorf("%q kept in %q", left, text)
		}
	}
}

func TestFetchPageTurnsHTMLIntoTextAndKeepsPlainTextAsItIs(t *testing.T) {
	client, _ := clientOf(map[string]answer{
		"https://docs.test/timeouts":     htmlAnswer(page),
		"https://docs.test/CHANGELOG.md": {body: "# Changelog\n- 2.0: faster", contentType: "text/markdown"},
	})
	fetched := FetchPage(context.Background(), "https://docs.test/timeouts", client)
	if !fetched.OK || fetched.Page.Title != "Timeouts & limits" || fetched.Page.Truncated {
		t.Fatalf("got %+v", fetched)
	}
	fetched = FetchPage(context.Background(), "https://docs.test/CHANGELOG.md", client)
	if fetched.Page.Text != "# Changelog\n- 2.0: faster" || fetched.Page.Title != "" {
		t.Fatalf("got %+v", fetched)
	}
}

func TestFetchPageKeepsTheStartOfALongPageAndSaysSo(t *testing.T) {
	client, _ := clientOf(map[string]answer{"https://docs.test/long.txt": {body: strings.Repeat("x", 100_000), contentType: "text/plain"}})
	fetched := FetchPage(context.Background(), "https://docs.test/long.txt", client)
	if !fetched.Page.Truncated || len(fetched.Page.Text) != MaxTextChars {
		t.Fatalf("truncated %v, %d", fetched.Page.Truncated, len(fetched.Page.Text))
	}
}

func TestFetchPageRefusesWhatIsNotTextAndAPageThatIsNotThere(t *testing.T) {
	client, _ := clientOf(map[string]answer{
		"https://docs.test/a.pdf": {body: "%PDF-1.7", contentType: "application/pdf"},
		"https://docs.test/gone":  {body: "Not found", status: http.StatusNotFound},
	})
	if got := FetchPage(context.Background(), "https://docs.test/a.pdf", client); got.Problem != "https://docs.test/a.pdf is application/pdf: not a page to read." {
		t.Fatalf("got %+v", got)
	}
	if got := FetchPage(context.Background(), "https://docs.test/gone", client); got.Problem != "https://docs.test/gone answered 404." {
		t.Fatalf("got %+v", got)
	}
}

type recordingModel struct{ requests []llm.Request }

func (m *recordingModel) Complete(_ context.Context, request llm.Request, _ func(llm.Part)) (llm.Reply, error) {
	m.requests = append(m.requests, request)
	return llm.Reply{Text: "  Set requestTimeout (ms).  "}, nil
}

func TestAskPageAsksTheReadingModelWithNoToolsAboutThePage(t *testing.T) {
	model := &recordingModel{}
	reader := Reader{Model: model, ModelName: "reader-model", NewID: func() string { return "reader-1" }, Thinking: protocol.ThinkingModeLow}
	answer, err := AskPage(context.Background(), Page{Text: "PAGE TEXT", Title: "Timeouts", URL: "https://docs.test/t"}, "How do I set the timeout?", reader)
	if err != nil || answer != "Set requestTimeout (ms)." {
		t.Fatalf("got %q, %v", answer, err)
	}
	request := model.requests[0]
	wantTarget := llm.Target{Identity: llm.Identity{AgentID: "reader-1", Model: "reader-model", Thinking: protocol.ThinkingModeLow}, Slot: llm.NoSlot}
	if len(request.Tools) != 0 || request.Target != wantTarget {
		t.Fatalf("request %+v", request)
	}
	if !strings.Contains(request.Messages[0].Content, "never instructions") {
		t.Fatalf("prompt %q", request.Messages[0].Content)
	}
	if !strings.HasPrefix(request.Messages[1].Content, "Question: How do I set the timeout?\n\nPage: https://docs.test/t (Timeouts)\n") || !strings.HasSuffix(request.Messages[1].Content, "PAGE TEXT") {
		t.Fatalf("message %q", request.Messages[1].Content)
	}
}
