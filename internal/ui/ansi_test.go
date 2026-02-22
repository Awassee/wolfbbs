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

	if got, want := CenterTextLine(6, FgGreen, BgBlack, "Y"); got != "\x1b[32m\x1b[40m  Y   \x1b[0m" {
		t.Fatalf("CenterTextLine = %q, want %q", got, want)
	}

	footer := FooterPrompt(20, "More")
	if got, want := footer, "     -- More --     "; got != want {
		t.Fatalf("FooterPrompt = %q, want %q", got, want)
	}

	box := DrawBox(8, 4, "Hi", []string{"a", "b"}, AsciiBox, FgGreen, BgBlack)
	expected := []string{
		"+--Hi--+",
		"|a      |",
		"|b      |",
		"+------+",
	}
	for _, line := range expected {
		if !strings.Contains(box, line) {
			t.Fatalf("DrawBox missing line: %q", line)
		}
	}
}
