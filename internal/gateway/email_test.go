package gateway

import (
	"strings"
	"testing"
)

func TestParseInboundRecipient(t *testing.T) {
	tests := map[string]string{
		"user@example.com":            "user",
		"user+wolfbbs@example.com":    "user",
		`"Name" <reader@bbs.example>`: "reader",
	}
	for input, want := range tests {
		got := ParseInboundRecipient(input)
		if got != want {
			t.Fatalf("recipient parse mismatch for %q: got %q want %q", input, got, want)
		}
	}
}

func TestEmailGatewayValidateOutbound(t *testing.T) {
	gw := NewEmailGateway(EmailConfig{
		MaxRecipients:    1,
		MaxMessageBytes:  64,
		RateLimitPerHour: 2,
	})
	if err := gw.ValidateOutbound([]string{"reader@example.com"}, "hello", "world"); err != nil {
		t.Fatalf("validate outbound: %v", err)
	}
	if err := gw.ValidateOutbound([]string{"bad-address"}, "hello", "world"); err == nil {
		t.Fatalf("expected invalid address error")
	}
	if err := gw.ValidateOutbound([]string{"reader@example.com", "two@example.com"}, "hello", "world"); err == nil {
		t.Fatalf("expected max recipients error")
	}
	if err := gw.ValidateOutbound([]string{"reader@example.com"}, "hello", strings.Repeat("a", 100)); err == nil {
		t.Fatalf("expected max message bytes error")
	}
	if err := gw.ValidateOutbound([]string{"reader@example.com"}, "hello\r\nBcc:evil@example.com", "world"); err == nil {
		t.Fatalf("expected header injection subject to be rejected")
	}
}

func TestLoadEmailConfigFromWolfbbsPrefixedEnv(t *testing.T) {
	t.Setenv("WOLFBBS_SMTP_HOST", "smtp.example.com")
	t.Setenv("WOLFBBS_SMTP_PORT", "2525")
	t.Setenv("WOLFBBS_SMTP_USER", "mailer")
	t.Setenv("WOLFBBS_SMTP_PASS", "secret")
	t.Setenv("WOLFBBS_FROM_DOMAIN", "bbs.example.com")

	cfg := LoadEmailConfigFromEnv()
	if cfg.Host != "smtp.example.com" {
		t.Fatalf("unexpected host %q", cfg.Host)
	}
	if cfg.Port != 2525 {
		t.Fatalf("unexpected port %d", cfg.Port)
	}
	if cfg.User != "mailer" {
		t.Fatalf("unexpected user %q", cfg.User)
	}
	if cfg.Pass != "secret" {
		t.Fatalf("unexpected pass")
	}
	if cfg.FromDomain != "bbs.example.com" {
		t.Fatalf("unexpected from domain %q", cfg.FromDomain)
	}
}
