package web

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html/charset"
)

const (
	pageTimeout  = 30 * time.Second
	maxPageBytes = 5 * 1024 * 1024
	// About 15,000 tokens: the reader answers in seconds, and documentation pages mostly fit.
	MaxTextChars = 60_000
	accept       = "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.5"
	// UserAgent: some sites turn away what does not look like a browser.
	UserAgent   = "Mozilla/5.0 (compatible; swcode)"
	mainContent = "main, article, [role=main]"
	// leftOut is what surrounds a page's content: menus, footers, scripts, pictures.
	leftOut = "aside, audio, button, canvas, footer, form, iframe, img, nav, noscript, picture, script, style, svg, template, video"
)

var (
	htmlType = regexp.MustCompile(`^(text/html|application/xhtml\+xml)`)
	// textType is read as it is: plain text, markdown, source code, JSON, XML, YAML.
	textType  = regexp.MustCompile(`^(text/|application/(json|xml|javascript|yaml|x-yaml|toml)|application/[^;]*\+(json|xml))`)
	blankRuns = regexp.MustCompile(`\n{3,}`)
)

type pageKind int

const (
	kindHTML pageKind = iota
	kindText
	kindOther
)

func kindOf(contentType string) pageKind {
	if htmlType.MatchString(contentType) {
		return kindHTML
	}
	if textType.MatchString(contentType) {
		return kindText
	}
	return kindOther
}

// HTMLText is a page's title, and its content as markdown: the main part when the page marks it, without menus or
// scripts.
func HTMLText(page string) (string, string) {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(page))
	if err != nil {
		return "", ""
	}
	title := strings.TrimSpace(document.Find("title").First().Text())
	content := document.Find(mainContent).First()
	if content.Length() == 0 {
		content = document.Find("body").First()
	}
	if content.Length() == 0 {
		content = document.Selection
	}
	content.Find(leftOut).Remove()
	inner, err := goquery.OuterHtml(content)
	if err != nil {
		return title, ""
	}
	text, err := htmltomarkdown.ConvertString(inner)
	if err != nil {
		return title, ""
	}
	return title, strings.TrimSpace(blankRuns.ReplaceAllString(text, "\n\n"))
}

// decode reads the body as text, in the charset the server names when it is one the decoder knows.
func decode(body []byte, contentType string) string {
	_, params, _ := mime.ParseMediaType(contentType)
	if label := params["charset"]; label != "" {
		if reader, err := charset.NewReaderLabel(label, strings.NewReader(string(body))); err == nil {
			if text, err := io.ReadAll(reader); err == nil {
				return string(text)
			}
		}
	}
	return strings.ToValidUTF8(string(body), "�")
}

// clip keeps the start of a text, in characters.
func clip(text string, chars int) (string, bool) {
	if utf8.RuneCountInString(text) <= chars {
		return text, false
	}
	return string([]rune(text)[:chars]), true
}

// FetchPage downloads a page and turns it into text: HTML into markdown, text as it is; anything else is refused.
func FetchPage(ctx context.Context, address string, client *http.Client) PageOutcome {
	ctx, cancel := context.WithTimeout(ctx, pageTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return PageOutcome{Problem: fmt.Sprintf("%s could not be read: %v", address, err)}
	}
	request.Header.Set("accept", accept)
	request.Header.Set("user-agent", UserAgent)
	response, err := client.Do(request)
	if err != nil {
		return PageOutcome{Problem: fmt.Sprintf("%s could not be read: %v", address, err)}
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return PageOutcome{Problem: fmt.Sprintf("%s answered %d.", address, response.StatusCode)}
	}
	contentType := strings.ToLower(response.Header.Get("content-type"))
	kind := kindOf(contentType)
	if kind == kindOther {
		described := contentType
		if described == "" {
			described = "of no stated type"
		}
		return PageOutcome{Problem: fmt.Sprintf("%s is %s: not a page to read.", address, described)}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPageBytes))
	if err != nil {
		return PageOutcome{Problem: fmt.Sprintf("%s could not be read: %v", address, err)}
	}
	text, title := decode(body, contentType), ""
	if kind == kindHTML {
		title, text = HTMLText(text)
	}
	clipped, cut := clip(text, MaxTextChars)
	return PageOutcome{OK: true, Page: Page{
		Text: clipped, Title: title, Truncated: cut || len(body) >= maxPageBytes, URL: response.Request.URL.String(),
	}}
}
