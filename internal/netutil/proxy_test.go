package netutil

import (
	"net/http"
	"testing"
)

func TestProxyResolver_UsesRemoteForUntrustedPeer(t *testing.T) {
	resolver, err := NewProxyResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	headers := http.Header{}
	headers.Set("X-Forwarded-For", "203.0.113.9")
	got := resolver.Resolve("198.51.100.4:1234", headers)
	if got != "198.51.100.4" {
		t.Fatalf("expected direct remote ip, got %q", got)
	}
}

func TestProxyResolver_UsesForwardedForTrustedProxy(t *testing.T) {
	resolver, err := NewProxyResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	headers := http.Header{}
	headers.Set("X-Forwarded-For", "203.0.113.9, 10.1.2.3")
	got := resolver.Resolve("10.1.2.3:4321", headers)
	if got != "203.0.113.9" {
		t.Fatalf("expected x-forwarded-for client ip, got %q", got)
	}
}

func TestProxyResolver_FallsBackToRealIP(t *testing.T) {
	resolver, err := NewProxyResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	headers := http.Header{}
	headers.Set("X-Real-IP", "203.0.113.10")
	got := resolver.Resolve("10.1.2.3:4321", headers)
	if got != "203.0.113.10" {
		t.Fatalf("expected x-real-ip fallback, got %q", got)
	}
}
