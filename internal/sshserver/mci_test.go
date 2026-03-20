package sshserver

import (
	"os"
	"path/filepath"
	"testing"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/mci"
)

func TestBuildSettingsMCIViewDefault(t *testing.T) {
	s := &Server{}
	user := &domain.User{
		Handle:        "tester",
		Theme:         "retro-amber",
		ANSIEnabled:   true,
		PagingEnabled: true,
		TimeFormat24h: true,
	}
	view := s.buildSettingsMCIView(user, []string{"retro-amber", "ice-blue"}, 0)
	if view.ID != "settings" {
		t.Fatalf("expected settings id, got %q", view.ID)
	}
	if view.Title != "My Settings" {
		t.Fatalf("expected default title, got %q", view.Title)
	}
	normalized, err := mci.Normalize(view)
	if err != nil {
		t.Fatalf("normalize view: %v", err)
	}
	if len(normalized.Controls) < 6 {
		t.Fatalf("expected controls, got %d", len(normalized.Controls))
	}
}

func TestBuildSettingsMCIViewTemplate(t *testing.T) {
	s := &Server{}
	user := &domain.User{
		Handle:        "tester",
		Theme:         "ice-blue",
		ANSIEnabled:   false,
		PagingEnabled: true,
		TimeFormat24h: false,
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.hjson")
	body := `{
  id: "custom-settings"
  title: "Custom Settings"
  controls: [
    { type: "label", id: "header", label: "Template Header" }
    { type: "input", id: "theme", label: "Palette", value: "retro-amber" }
    { type: "toggle", id: "ansi", label: "ANSI Mode", value: "true" }
    { type: "button", id: "save", label: "Commit" }
  ]
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	t.Setenv("WOLFBBS_MCI_SETTINGS_FILE", path)
	view := s.buildSettingsMCIView(user, []string{"retro-amber", "ice-blue"}, 1)
	if view.Title != "Custom Settings" {
		t.Fatalf("expected template title, got %q", view.Title)
	}
	normalized, err := mci.Normalize(view)
	if err != nil {
		t.Fatalf("normalize template view: %v", err)
	}
	foundThemeList := false
	for _, control := range normalized.Controls {
		if control.ID == "theme_list" {
			foundThemeList = true
			if control.Selected != 1 {
				t.Fatalf("expected selected theme index 1, got %d", control.Selected)
			}
		}
	}
	if !foundThemeList {
		t.Fatal("expected injected theme_list control")
	}
}
