package gateway

import (
	"strings"
	"testing"
)

func TestFetchTextWrapsAndStrips(t *testing.T) {
	html := "<html><body><p>Hello <b>BBS</b> world</p><script>bad()</script><a href=\"https://example.com\">Example</a></body></html>"
	got := sanitizeAndWrap(html, defaultLineWidth)
	if !strings.Contains(got, "Hello BBS world") {
		t.Fatalf("expected body text in output: %q", got)
	}
	if !strings.Contains(got, "Example [https://example.com]") {
		t.Fatalf("expected link text in output: %q", got)
	}
}

func TestValidateGatewayURLRejectsLocalhost(t *testing.T) {
	if err := ValidateSafeHTTPURL("http://127.0.0.1", false); err == nil {
		t.Fatal("expected error for localhost URL")
	}
}

func TestValidateGatewayURLAllowsPrivateWhenExplicit(t *testing.T) {
	if err := ValidateSafeHTTPURL("http://127.0.0.1", true); err != nil {
		t.Fatalf("expected private url override to allow localhost, got %v", err)
	}
}
