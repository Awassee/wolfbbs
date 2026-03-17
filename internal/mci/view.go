package mci

import (
	"fmt"
	"strings"

	hjson "github.com/hjson/hjson-go/v4"
)

type ControlType string

const (
	ControlLabel    ControlType = "label"
	ControlInput    ControlType = "input"
	ControlToggle   ControlType = "toggle"
	ControlLightbar ControlType = "lightbar"
	ControlButton   ControlType = "button"
)

type Control struct {
	Type     ControlType `json:"type"`
	ID       string      `json:"id,omitempty"`
	Label    string      `json:"label"`
	Value    string      `json:"value,omitempty"`
	Options  []string    `json:"options,omitempty"`
	Selected int         `json:"selected,omitempty"`
}

type View struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Footer   string    `json:"footer,omitempty"`
	Controls []Control `json:"controls"`
}

func ParseHJSON(data []byte) (View, error) {
	var view View
	if err := hjson.Unmarshal(data, &view); err != nil {
		return View{}, err
	}
	return Normalize(view)
}

func Normalize(view View) (View, error) {
	view.ID = strings.TrimSpace(view.ID)
	if view.ID == "" {
		view.ID = "view"
	}
	view.Title = strings.TrimSpace(view.Title)
	if view.Title == "" {
		return View{}, fmt.Errorf("view title is required")
	}
	if len(view.Controls) == 0 {
		return View{}, fmt.Errorf("at least one control is required")
	}

	for i := range view.Controls {
		control := &view.Controls[i]
		control.Type = ControlType(strings.ToLower(strings.TrimSpace(string(control.Type))))
		control.ID = strings.TrimSpace(control.ID)
		control.Label = strings.TrimSpace(control.Label)
		control.Value = strings.TrimSpace(control.Value)
		if control.Label == "" {
			return View{}, fmt.Errorf("control %d label is required", i)
		}
		switch control.Type {
		case ControlLabel, ControlInput, ControlToggle, ControlLightbar, ControlButton:
		default:
			return View{}, fmt.Errorf("unsupported control type %q", control.Type)
		}
		if control.Type == ControlLightbar {
			if len(control.Options) == 0 {
				return View{}, fmt.Errorf("lightbar %q requires options", control.ID)
			}
			if control.Selected < 0 {
				control.Selected = 0
			}
			if control.Selected >= len(control.Options) {
				control.Selected = len(control.Options) - 1
			}
		}
	}
	return view, nil
}

func RenderLines(view View) []string {
	lines := make([]string, 0, len(view.Controls)*2)
	for _, control := range view.Controls {
		switch control.Type {
		case ControlLabel:
			lines = append(lines, control.Label)
		case ControlInput:
			lines = append(lines, fmt.Sprintf("%s: %s", control.Label, control.Value))
		case ControlToggle:
			lines = append(lines, fmt.Sprintf("[%s] %s", boolGlyph(control.Value), control.Label))
		case ControlLightbar:
			lines = append(lines, control.Label+":")
			for idx, option := range control.Options {
				prefix := "  "
				if idx == control.Selected {
					prefix = "> "
				}
				lines = append(lines, prefix+option)
			}
		case ControlButton:
			lines = append(lines, fmt.Sprintf("[ %s ]", control.Label))
		}
	}
	if footer := strings.TrimSpace(view.Footer); footer != "" {
		lines = append(lines, "")
		lines = append(lines, footer)
	}
	return lines
}

func boolGlyph(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return "x"
	default:
		return " "
	}
}
