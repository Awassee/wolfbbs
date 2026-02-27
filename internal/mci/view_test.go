package mci

import (
	"strings"
	"testing"
)

func TestParseHJSONAndRender(t *testing.T) {
	raw := []byte(`
{
  id: "settings"
  title: "Settings"
  footer: "Press Q to return"
  controls: [
    { type: "label", label: "Profile" }
    { type: "input", id: "theme", label: "Theme", value: "retro-amber" }
    { type: "toggle", id: "ansi", label: "ANSI Enabled", value: "true" }
    { type: "lightbar", id: "themes", label: "Themes", options: ["retro-amber","ice-blue"], selected: 1 }
  ]
}
`)
	view, err := ParseHJSON(raw)
	if err != nil {
		t.Fatalf("parse mci view: %v", err)
	}
	lines := RenderLines(view)
	rendered := strings.Join(lines, "\n")
	if !strings.Contains(rendered, "Theme: retro-amber") {
		t.Fatalf("missing input render: %q", rendered)
	}
	if !strings.Contains(rendered, "[x] ANSI Enabled") {
		t.Fatalf("missing toggle render: %q", rendered)
	}
	if !strings.Contains(rendered, "> ice-blue") {
		t.Fatalf("missing selected lightbar render: %q", rendered)
	}
}

func TestNormalizeRejectsUnknownType(t *testing.T) {
	_, err := Normalize(View{
		Title: "Bad",
		Controls: []Control{
			{Type: "unknown", Label: "Bad"},
		},
	})
	if err == nil {
		t.Fatal("expected unknown control type error")
	}
}
