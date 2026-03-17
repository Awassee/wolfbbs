package menu

import (
	"fmt"
	"strings"

	hjson "github.com/hjson/hjson-go/v4"
)

func ParseHJSON(data []byte) (Screen, error) {
	var screen Screen
	if err := hjson.Unmarshal(data, &screen); err != nil {
		return Screen{}, err
	}
	if strings.TrimSpace(screen.ID) == "" {
		screen.ID = "main"
	}
	if strings.TrimSpace(screen.Title) == "" {
		return Screen{}, fmt.Errorf("%w: title is required", ErrInvalidMenu)
	}
	if len(screen.Entries) == 0 {
		return Screen{}, fmt.Errorf("%w: at least one menu entry is required", ErrInvalidMenu)
	}

	seenHotkeys := map[string]struct{}{}
	for i := range screen.Entries {
		entry := &screen.Entries[i]
		entry.Hotkey = strings.ToUpper(strings.TrimSpace(entry.Hotkey))
		entry.Label = strings.TrimSpace(entry.Label)
		entry.Action = normalizeAction(entry.Action)
		entry.Target = strings.TrimSpace(entry.Target)
		entry.ACS = strings.TrimSpace(entry.ACS)

		if len(entry.Hotkey) != 1 {
			return Screen{}, fmt.Errorf("%w: entry %d has invalid hotkey %q", ErrInvalidMenu, i, entry.Hotkey)
		}
		if entry.Label == "" {
			return Screen{}, fmt.Errorf("%w: entry %d label is required", ErrInvalidMenu, i)
		}
		if entry.Action == "" {
			return Screen{}, fmt.Errorf("%w: entry %d action is required", ErrInvalidMenu, i)
		}
		if _, exists := seenHotkeys[entry.Hotkey]; exists {
			return Screen{}, fmt.Errorf("%w: duplicate hotkey %q", ErrInvalidMenu, entry.Hotkey)
		}
		seenHotkeys[entry.Hotkey] = struct{}{}
	}
	return screen, nil
}
