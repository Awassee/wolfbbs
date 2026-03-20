package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
)

var reFeedTag = regexp.MustCompile(`(?is)<[^>]+>`)

type FeedItem struct {
	Title     string
	Link      string
	Published string
	Summary   string
}

type FeedResult struct {
	SourceURL string
	Title     string
	Items     []FeedItem
}

type URLSummary struct {
	URL       string
	Title     string
	Bullets   []string
	WordCount int
	Excerpt   string
}

// FetchFeed reads an RSS/Atom feed URL and returns normalized entries.
func FetchFeed(ctx context.Context, rawURL string, cfg FetchConfig, maxItems int) (FeedResult, error) {
	if maxItems <= 0 {
		maxItems = 12
	}
	body, _, finalURL, err := fetchGatewayBody(ctx, rawURL, cfg, []string{
		"application/rss+xml",
		"application/atom+xml",
		"application/xml",
		"text/xml",
		"text/plain",
		"text/html",
	})
	if err != nil {
		return FeedResult{}, err
	}
	parsed, err := parseFeedBody(body, maxItems)
	if err != nil {
		return FeedResult{}, err
	}
	parsed.SourceURL = finalURL
	if strings.TrimSpace(parsed.Title) == "" {
		parsed.Title = finalURL
	}
	return parsed, nil
}

// FetchJSON returns pretty-printed JSON from a URL protected by gateway safety checks.
func FetchJSON(ctx context.Context, rawURL string, cfg FetchConfig) (string, error) {
	body, _, _, err := fetchGatewayBody(ctx, rawURL, cfg, []string{
		"application/json",
		"application/problem+json",
		"text/json",
		"text/plain",
	})
	if err != nil {
		return "", err
	}
	var payload interface{}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return "", fmt.Errorf("invalid json response: %w", err)
	}
	pretty, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(pretty), nil
}

// SummarizeURL extracts readable text and emits short bullets suitable for BBS sessions.
func SummarizeURL(ctx context.Context, rawURL string, cfg FetchConfig, maxBullets int) (URLSummary, error) {
	if maxBullets <= 0 {
		maxBullets = 5
	}
	text, err := FetchText(ctx, rawURL, cfg)
	if err != nil {
		return URLSummary{}, err
	}
	bullets := summarizeTextBullets(text, maxBullets)
	title := extractSummaryTitle(text)
	if title == "" {
		title = rawURL
	}
	excerpt := strings.TrimSpace(strings.ReplaceAll(text, "\r\n", " "))
	if len(excerpt) > 220 {
		excerpt = strings.TrimSpace(excerpt[:220]) + "..."
	}
	return URLSummary{
		URL:       strings.TrimSpace(rawURL),
		Title:     title,
		Bullets:   bullets,
		WordCount: len(strings.Fields(text)),
		Excerpt:   excerpt,
	}, nil
}

func fetchGatewayBody(ctx context.Context, rawURL string, cfg FetchConfig, allowedTypes []string) ([]byte, http.Header, string, error) {
	cfg = sanitizeFetchConfig(cfg)
	if err := ValidateSafeHTTPURL(rawURL, false); err != nil {
		return nil, nil, "", err
	}
	client := &http.Client{
		Timeout: cfg.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > cfg.MaxRedirects {
				return fmt.Errorf("too many redirects (max=%d)", cfg.MaxRedirects)
			}
			return ValidateSafeHTTPURL(req.URL.String(), false)
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, "", err
	}
	req.Header.Set("User-Agent", cfg.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, "", fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, "", fmt.Errorf("http status %s", resp.Status)
	}
	if len(allowedTypes) > 0 && !isAllowedContentTypeStrict(resp.Header.Get("Content-Type"), allowedTypes) {
		return nil, nil, "", fmt.Errorf("content type blocked: %s", resp.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, cfg.MaxBodyBytes+1))
	if err != nil {
		return nil, nil, "", err
	}
	if int64(len(body)) > cfg.MaxBodyBytes {
		return nil, nil, "", fmt.Errorf("response body exceeds limit (%d bytes)", cfg.MaxBodyBytes)
	}
	finalURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return body, resp.Header, finalURL, nil
}

func isAllowedContentTypeStrict(header string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil {
		mediaType = strings.ToLower(strings.TrimSpace(strings.SplitN(header, ";", 2)[0]))
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	for _, a := range allowed {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		if mediaType == a {
			return true
		}
	}
	return false
}

func parseFeedBody(body []byte, maxItems int) (FeedResult, error) {
	if maxItems <= 0 {
		maxItems = 12
	}
	if parsed, ok := parseRSS(body, maxItems); ok {
		return parsed, nil
	}
	if parsed, ok := parseAtom(body, maxItems); ok {
		return parsed, nil
	}
	return FeedResult{}, fmt.Errorf("unsupported feed format")
}

type rssDoc struct {
	Channel struct {
		Title string `xml:"title"`
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			PubDate     string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

func parseRSS(body []byte, maxItems int) (FeedResult, bool) {
	var doc rssDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return FeedResult{}, false
	}
	if len(doc.Channel.Items) == 0 && strings.TrimSpace(doc.Channel.Title) == "" {
		return FeedResult{}, false
	}
	items := make([]FeedItem, 0, minInt(maxItems, len(doc.Channel.Items)))
	for _, row := range doc.Channel.Items {
		if len(items) >= maxItems {
			break
		}
		items = append(items, FeedItem{
			Title:     strings.TrimSpace(row.Title),
			Link:      strings.TrimSpace(row.Link),
			Published: strings.TrimSpace(row.PubDate),
			Summary:   compactSnippet(row.Description, 220),
		})
	}
	return FeedResult{Title: strings.TrimSpace(doc.Channel.Title), Items: items}, true
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomFeedNS struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title   string `xml:"title"`
	Summary string `xml:"summary"`
	Content string `xml:"content"`
	Updated string `xml:"updated"`
	Links   []struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
	} `xml:"link"`
}

func parseAtom(body []byte, maxItems int) (FeedResult, bool) {
	result, ok := parseAtomWithStruct(body, maxItems, false)
	if ok {
		return result, true
	}
	result, ok = parseAtomWithStruct(body, maxItems, true)
	if ok {
		return result, true
	}
	return FeedResult{}, false
}

func parseAtomWithStruct(body []byte, maxItems int, namespaced bool) (FeedResult, bool) {
	entries := []atomEntry{}
	title := ""
	if namespaced {
		var doc atomFeedNS
		if err := xml.Unmarshal(body, &doc); err != nil {
			return FeedResult{}, false
		}
		title = doc.Title
		entries = doc.Entries
	} else {
		var doc atomFeed
		if err := xml.Unmarshal(body, &doc); err != nil {
			return FeedResult{}, false
		}
		title = doc.Title
		entries = doc.Entries
	}
	if len(entries) == 0 && strings.TrimSpace(title) == "" {
		return FeedResult{}, false
	}
	items := make([]FeedItem, 0, minInt(maxItems, len(entries)))
	for _, row := range entries {
		if len(items) >= maxItems {
			break
		}
		link := ""
		for _, l := range row.Links {
			if strings.TrimSpace(l.Href) == "" {
				continue
			}
			if strings.TrimSpace(l.Rel) == "" || strings.EqualFold(strings.TrimSpace(l.Rel), "alternate") {
				link = strings.TrimSpace(l.Href)
				break
			}
		}
		if link == "" && len(row.Links) > 0 {
			link = strings.TrimSpace(row.Links[0].Href)
		}
		summary := row.Summary
		if strings.TrimSpace(summary) == "" {
			summary = row.Content
		}
		items = append(items, FeedItem{
			Title:     strings.TrimSpace(row.Title),
			Link:      link,
			Published: strings.TrimSpace(row.Updated),
			Summary:   compactSnippet(summary, 220),
		})
	}
	return FeedResult{Title: strings.TrimSpace(title), Items: items}, true
}

func compactSnippet(value string, maxLen int) string {
	value = reFeedTag.ReplaceAllString(value, " ")
	value = html.UnescapeString(value)
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	return strings.TrimSpace(value[:maxLen]) + "..."
}

func summarizeTextBullets(text string, maxBullets int) []string {
	if maxBullets <= 0 {
		maxBullets = 5
	}
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	paragraphs := strings.Split(normalized, "\n")
	seen := map[string]struct{}{}
	bullets := make([]string, 0, maxBullets)
	for _, raw := range paragraphs {
		line := strings.TrimSpace(raw)
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		if len(strings.Fields(line)) < 6 {
			continue
		}
		key := strings.ToLower(line)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if len(line) > 180 {
			line = strings.TrimSpace(line[:180]) + "..."
		}
		bullets = append(bullets, line)
		if len(bullets) >= maxBullets {
			break
		}
	}
	if len(bullets) > 0 {
		return bullets
	}
	fallback := strings.TrimSpace(strings.Join(strings.Fields(normalized), " "))
	if fallback == "" {
		return []string{"No readable summary available."}
	}
	if len(fallback) > 180 {
		fallback = strings.TrimSpace(fallback[:180]) + "..."
	}
	return []string{fallback}
}

func extractSummaryTitle(text string) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	for _, line := range strings.Split(normalized, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.Join(strings.Fields(line), " ")
		if len(line) > 100 {
			line = strings.TrimSpace(line[:100]) + "..."
		}
		return line
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
