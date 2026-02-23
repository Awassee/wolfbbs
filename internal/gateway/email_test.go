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
}
