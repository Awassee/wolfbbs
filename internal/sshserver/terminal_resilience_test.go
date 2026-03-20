package sshserver

import (
	"strings"
	"testing"

	"wolfbbs/internal/term"
)

func TestResolveSessionOutputHonorsTerminalCapability(t *testing.T) {
	profile := term.DetectProfile("dumb", "en_US.UTF-8", "", 80, 25, true)
	ansi, encoding := resolveSessionOutput(profile, true)
	if ansi {
		t.Fatal("expected ansi disabled for plain terminal profile")
	}
	if encoding != string(term.EncodingASCII) {
		t.Fatalf("expected ascii encoding, got %q", encoding)
	}
}

func TestSessionProfileStatusLinesIncludeTerminalHints(t *testing.T) {
	profile := term.DetectProfile("xterm-256color", "en_US.UTF-8", "", 48, 20, true)
	lines := sessionProfileStatusLines(profile, true, string(profile.Encoding))
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Terminal size: 48x20", "Compact mode: true", "Compact layout enabled for narrow width.", "Short-page mode enabled for low terminal height."} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in session profile lines: %s", want, joined)
		}
	}
}

func TestPagerPageSizeForHeight(t *testing.T) {
	cases := []struct {
		height int
		want   int
	}{
		{0, 16},
		{10, 8},
		{20, 12},
		{28, 20},
		{48, 28},
	}
	for _, tc := range cases {
		if got := pagerPageSizeForHeight(tc.height); got != tc.want {
			t.Fatalf("height %d: expected %d, got %d", tc.height, tc.want, got)
		}
	}
}
