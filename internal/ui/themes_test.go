package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestThemeByNameKnownAndFallback(t *testing.T) {
	resetThemesForTest()
	if got := ThemeByName("ice-blue"); got.StatusBg != BgBlue {
		t.Fatalf("expected ice-blue status background to be blue, got %q", got.StatusBg)
	}
	if got := ThemeByName("unknown-theme"); got.BodyFg != ThemeByName("retro-amber").BodyFg {
		t.Fatalf("expected unknown theme fallback to retro-amber")
	}
}

func TestThemeNamesStable(t *testing.T) {
	resetThemesForTest()
	names := ThemeNames()
	if len(names) < 3 {
		t.Fatalf("expected at least 3 themes, got %d", len(names))
	}
	if names[0] != "retro-amber" {
		t.Fatalf("expected first theme to be retro-amber, got %q", names[0])
	}
}

func TestLoadThemeFile(t *testing.T) {
	resetThemesForTest()
	dir := t.TempDir()
	path := filepath.Join(dir, "themes.hjson")
	body := `{
  themes: [
    {
      name: "night-drive"
      status_fg: "fg-white"
      status_bg: "bg-blue"
      body_fg: "fg-cyan"
      accent_fg: "fg-yellow"
      warn_fg: "fg-red"
      error_fg: "fg-magenta"
      muted_fg: "fg-green"
    }
  ]
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write theme file: %v", err)
	}
	loaded, err := LoadThemeFile(path)
	if err != nil {
		t.Fatalf("load theme file: %v", err)
	}
	if loaded != 1 {
		t.Fatalf("expected 1 loaded theme, got %d", loaded)
	}
	got := ThemeByName("night-drive")
	if got.StatusBg != BgBlue || got.BodyFg != FgCyan {
		t.Fatalf("unexpected custom theme %+v", got)
	}
}

func TestLoadThemesFromEnv(t *testing.T) {
	resetThemesForTest()
	dir := t.TempDir()
	path := filepath.Join(dir, "themes.hjson")
	body := `{
  themes: [{ name: "forest", body_fg: "fg-green", status_bg: "bg-cyan" }]
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write theme file: %v", err)
	}
	t.Setenv("WOLFBBS_THEME_FILE", path)
	if err := LoadThemesFromEnv(); err != nil {
		t.Fatalf("load themes from env: %v", err)
	}
	if ThemeByName("forest").BodyFg != FgGreen {
		t.Fatalf("expected env-loaded theme body color")
	}
}
