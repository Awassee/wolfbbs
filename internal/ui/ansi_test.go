package ui

import (
	"strings"
	"testing"
)

func TestANSIHelpers(t *testing.T) {
	if got, want := ClearScreen(), "\x1b[2J\x1b[H"; got != want {
		t.Fatalf("ClearScreen = %q, want %q", got, want)
	}

	if got, want := CenterText(10, "abc"), "   abc    "; got != want {
		t.Fatalf("CenterText = %q, want %q", got, want)
	}

	if got, want := MoveCursor(5, 10), "\x1b[5;10H"; got != want {
		t.Fatalf("MoveCursor = %q, want %q", got, want)
	}

	if got, want := Color(FgRed, BgBlue, "x"), "\x1b[31m\x1b[44mx\x1b[0m"; got != want {
		t.Fatalf("Color = %q, want %q", got, want)
	}

	if got := CenterTextLine(6, FgGreen, BgBlack, "Y"); got != "\x1b[32m\x1b[40m  Y   \x1b[0m" {
		t.Fatalf("CenterTextLine = %q", got)
	}

	footer := FooterPrompt(20, "More")
	if got, want := footer, "     -- More --     "; got != want {
		t.Fatalf("FooterPrompt = %q, want %q", got, want)
	}

	box := DrawBox(8, 4, "Hi", []string{"a", "b"}, AsciiBox, FgGreen, BgBlack)
	plainBox := stripANSIEscapes(box)
	expected := []string{
		"+- Hi -+",
		"|a     |",
		"|b     |",
		"+------+",
	}
	for _, line := range expected {
		if !strings.Contains(plainBox, line) {
			t.Fatalf("DrawBox missing line: %q", line)
		}
	}
}

func TestApplyOutputProfileStripsANSIAndBoxes(t *testing.T) {
	raw := "\x1b[31m╔═╗\x1b[0m\r\n\x1b[32m║x║\x1b[0m\r\n\x1b[33m╚═╝\x1b[0m"
	got := ApplyOutputProfile(raw, false, "ascii")
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("expected ansi escapes removed, got %q", got)
	}
	for _, want := range []string{"+-+", "|x|", "+-+"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in %q", want, got)
		}
	}
}

func TestDrawBoxHandlesANSIContentPadding(t *testing.T) {
	content := []string{FgYellow + "Hotline" + FgCyan + " online"}
	box := DrawBox(24, 4, "Board", content, AsciiBox, FgGreen, BgBlack)
	plain := stripANSIEscapes(box)
	lines := strings.Split(strings.TrimSuffix(plain, "\r\n"), "\r\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(lines))
	}
	for _, line := range lines {
		if got := runeLen(line); got != 24 {
			t.Fatalf("expected visible line width 24, got %d for %q", got, line)
		}
	}
}
