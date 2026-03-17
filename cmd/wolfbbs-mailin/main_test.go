package main

import (
	"net/http/httptest"
	"testing"
)

func TestParseAllowDomains(t *testing.T) {
	out := parseAllowDomains("example.com, mail.example.org ,")
	if len(out) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(out))
	}
	if _, ok := out["example.com"]; !ok {
		t.Fatalf("missing example.com")
	}
	if _, ok := out["mail.example.org"]; !ok {
		t.Fatalf("missing mail.example.org")
	}
}

func TestSenderDomain(t *testing.T) {
	cases := map[string]string{
		"user@example.com":            "example.com",
		`"Name" <sender@example.org>`: "example.org",
		`bad`:                         "",
		`sender+tag@example.net`:      "example.net",
	}
	for input, want := range cases {
		got := senderDomain(input)
		if got != want {
			t.Fatalf("senderDomain(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestInboundAuthTokenSources(t *testing.T) {
	req := httptest.NewRequest("POST", "/ingest?token=query-token", nil)
	if got := inboundAuthToken(req); got != "query-token" {
		t.Fatalf("expected query token, got %q", got)
	}

	req = httptest.NewRequest("POST", "/ingest", nil)
	req.Header.Set("X-Inbound-Token", "header-token")
	if got := inboundAuthToken(req); got != "header-token" {
		t.Fatalf("expected header token, got %q", got)
	}

	req = httptest.NewRequest("POST", "/ingest", nil)
	req.Header.Set("Authorization", "Bearer bearer-token")
	if got := inboundAuthToken(req); got != "bearer-token" {
		t.Fatalf("expected bearer token, got %q", got)
	}
}

func TestIngestAuthorizedRejectsPublicDefaultToken(t *testing.T) {
	req := httptest.NewRequest("POST", "/ingest", nil)
	req.RemoteAddr = "198.51.100.4:1234"
	req.Header.Set("X-Inbound-Token", defaultInboundToken)
	if ingestAuthorized(req, defaultInboundToken) {
		t.Fatal("expected public default token to be rejected")
	}
}

func TestIngestAuthorizedAllowsLocalDefaultTokenAndCustomToken(t *testing.T) {
	req := httptest.NewRequest("POST", "/ingest", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Inbound-Token", defaultInboundToken)
	if !ingestAuthorized(req, defaultInboundToken) {
		t.Fatal("expected loopback default token to be allowed")
	}

	req = httptest.NewRequest("POST", "/ingest", nil)
	req.RemoteAddr = "198.51.100.4:1234"
	req.Header.Set("X-Inbound-Token", "custom-secret")
	if !ingestAuthorized(req, "custom-secret") {
		t.Fatal("expected custom token to be allowed")
	}
}
