package main

import "testing"

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
