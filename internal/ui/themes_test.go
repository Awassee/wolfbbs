package ui

import "testing"

func TestThemeByNameKnownAndFallback(t *testing.T) {
	if got := ThemeByName("ice-blue"); got.StatusBg != BgBlue {
		t.Fatalf("expected ice-blue status background to be blue, got %q", got.StatusBg)
	}
	if got := ThemeByName("unknown-theme"); got.BodyFg != ThemeByName("retro-amber").BodyFg {
		t.Fatalf("expected unknown theme fallback to retro-amber")
	}
}

func TestThemeNamesStable(t *testing.T) {
	names := ThemeNames()
	if len(names) < 3 {
		t.Fatalf("expected at least 3 themes, got %d", len(names))
	}
	if names[0] != "retro-amber" {
		t.Fatalf("expected first theme to be retro-amber, got %q", names[0])
	}
}
