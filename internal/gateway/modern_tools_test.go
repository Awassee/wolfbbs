package gateway

import (
	"context"
	"strings"
	"testing"
)

func TestParseFeedBodyRSS(t *testing.T) {
	raw := `<?xml version="1.0"?><rss version="2.0"><channel><title>Wolf Feed</title><item><title>Entry One</title><link>https://example.com/1</link><description>Hello world</description><pubDate>Thu, 01 Jan 1970 00:00:00 GMT</pubDate></item><item><title>Entry Two</title><link>https://example.com/2</link><description>Second entry</description></item></channel></rss>`
	parsed, ok := parseRSS([]byte(raw), 5)
	if !ok {
		t.Fatalf("parse rss failed")
	}
	if parsed.Title != "Wolf Feed" {
		t.Fatalf("unexpected title %q", parsed.Title)
	}
	if len(parsed.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(parsed.Items))
	}
	if parsed.Items[0].Title != "Entry One" || parsed.Items[0].Link != "https://example.com/1" {
		t.Fatalf("unexpected first item: %+v", parsed.Items[0])
	}
	if !strings.Contains(parsed.Items[0].Summary, "Hello world") {
		t.Fatalf("unexpected summary %q", parsed.Items[0].Summary)
	}
}

func TestParseFeedBodyAtomNamespaced(t *testing.T) {
	raw := `<?xml version="1.0" encoding="utf-8"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Wolf Atom</title><entry><title>Atom One</title><link href="https://example.com/a1" rel="alternate"/><updated>2026-03-01T10:00:00Z</updated><summary>First atom item</summary></entry></feed>`
	parsed, err := parseFeedBody([]byte(raw), 5)
	if err != nil {
		t.Fatalf("parse atom: %v", err)
	}
	if parsed.Title != "Wolf Atom" {
		t.Fatalf("unexpected title %q", parsed.Title)
	}
	if len(parsed.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(parsed.Items))
	}
	if parsed.Items[0].Link != "https://example.com/a1" {
		t.Fatalf("unexpected atom link: %+v", parsed.Items[0])
	}
}

func TestFetchJSONRejectsUnsafeURL(t *testing.T) {
	_, err := FetchJSON(context.Background(), "http://127.0.0.1", DefaultFetchConfig)
	if err == nil {
		t.Fatalf("expected blocked localhost url")
	}
}

func TestSummarizeURLRejectsUnsafeURL(t *testing.T) {
	_, err := SummarizeURL(context.Background(), "http://localhost", DefaultFetchConfig, 3)
	if err == nil {
		t.Fatalf("expected blocked localhost url")
	}
}

func TestSummarizeTextBulletsFallback(t *testing.T) {
	bullets := summarizeTextBullets("tiny", 3)
	if len(bullets) != 1 {
		t.Fatalf("expected fallback bullet, got %d", len(bullets))
	}
	if !strings.Contains(strings.ToLower(bullets[0]), "tiny") {
		t.Fatalf("unexpected fallback bullet %q", bullets[0])
	}
}
