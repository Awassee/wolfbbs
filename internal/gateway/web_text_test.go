package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchTextWrapsAndStrips(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body><p>Hello <b>BBS</b> world</p><script>bad()</script><a href=\"https://example.com\">Example</a></body></html>"))
	}))
	defer srv.Close()

	cfg := DefaultFetchConfig
	cfg.Timeout = 2 * time.Second
	cfg.MaxBodyBytes = 1024
	got, err := FetchText(context.Background(), srv.URL, cfg)
	if err != nil {
		t.Fatalf("FetchText: %v", err)
	}
	if !strings.Contains(got, "Hello BBS world") {
		t.Fatalf("expected body text in output: %q", got)
	}
	if !strings.Contains(got, "Example [https://example.com]") {
		t.Fatalf("expected link text in output: %q", got)
	}
}

func TestValidateGatewayURLRejectsLocalhost(t *testing.T) {
	if err := validateGatewayURL("http://127.0.0.1"); err == nil {
		t.Fatal("expected error for localhost URL")
	}
}
