package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	hjson "github.com/hjson/hjson-go/v4"
)

type themeFile struct {
	Themes []themeFileEntry `json:"themes"`
}

type themeFileEntry struct {
	Name     string `json:"name"`
	StatusFg string `json:"status_fg"`
	StatusBg string `json:"status_bg"`
	BodyFg   string `json:"body_fg"`
	AccentFg string `json:"accent_fg"`
	WarnFg   string `json:"warn_fg"`
	ErrorFg  string `json:"error_fg"`
	MutedFg  string `json:"muted_fg"`
}

var (
	themeMu       sync.RWMutex
	themeRegistry = defaultThemeRegistry()
	themeNames    = orderedThemeNames(themeRegistry)
)

func defaultThemeRegistry() map[string]Theme {
	return map[string]Theme{
		"retro-amber": {
			StatusFg: FgBlack,
			StatusBg: BgGray,
			BodyFg:   FgYellow,
			AccentFg: FgOrange,
			WarnFg:   FgRed,
			ErrorFg:  FgMagenta,
			MutedFg:  FgGreen,
		},
		"ice-blue": {
			StatusFg: FgWhite,
			StatusBg: BgBlue,
			BodyFg:   FgCyan,
			AccentFg: FgYellow,
			WarnFg:   FgRed,
			ErrorFg:  FgMagenta,
			MutedFg:  FgGreen,
		},
		"emerald": {
			StatusFg: FgBlack,
			StatusBg: BgCyan,
			BodyFg:   FgGreen,
			AccentFg: FgYellow,
			WarnFg:   FgRed,
			ErrorFg:  FgMagenta,
			MutedFg:  FgWhite,
		},
	}
}

func ThemeByName(name string) Theme {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		key = "retro-amber"
	}
	themeMu.RLock()
	defer themeMu.RUnlock()
	if th, ok := themeRegistry[key]; ok {
		return th
	}
	return themeRegistry["retro-amber"]
}

func ThemeNames() []string {
	themeMu.RLock()
	defer themeMu.RUnlock()
	out := make([]string, len(themeNames))
	copy(out, themeNames)
	return out
}

func LoadThemesFromEnv() error {
	path := strings.TrimSpace(os.Getenv("WOLFBBS_THEME_FILE"))
	if path == "" {
		return nil
	}
	_, err := LoadThemeFile(path)
	return err
}

func LoadThemeFile(path string) (int, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, fmt.Errorf("theme file path is required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var parsed themeFile
	if err := hjson.Unmarshal(body, &parsed); err != nil {
		return 0, err
	}
	if len(parsed.Themes) == 0 {
		return 0, fmt.Errorf("theme file has no themes")
	}

	themeMu.Lock()
	defer themeMu.Unlock()
	merged := defaultThemeRegistry()
	loaded := 0
	for _, row := range parsed.Themes {
		name := strings.ToLower(strings.TrimSpace(row.Name))
		if name == "" {
			continue
		}
		base := merged["retro-amber"]
		if existing, ok := merged[name]; ok {
			base = existing
		}
		var resolveErr error
		base.StatusFg, resolveErr = resolveColor(row.StatusFg, base.StatusFg)
		if resolveErr != nil {
			return loaded, fmt.Errorf("theme %q status_fg: %w", name, resolveErr)
		}
		base.StatusBg, resolveErr = resolveColor(row.StatusBg, base.StatusBg)
		if resolveErr != nil {
			return loaded, fmt.Errorf("theme %q status_bg: %w", name, resolveErr)
		}
		base.BodyFg, resolveErr = resolveColor(row.BodyFg, base.BodyFg)
		if resolveErr != nil {
			return loaded, fmt.Errorf("theme %q body_fg: %w", name, resolveErr)
		}
		base.AccentFg, resolveErr = resolveColor(row.AccentFg, base.AccentFg)
		if resolveErr != nil {
			return loaded, fmt.Errorf("theme %q accent_fg: %w", name, resolveErr)
		}
		base.WarnFg, resolveErr = resolveColor(row.WarnFg, base.WarnFg)
		if resolveErr != nil {
			return loaded, fmt.Errorf("theme %q warn_fg: %w", name, resolveErr)
		}
		base.ErrorFg, resolveErr = resolveColor(row.ErrorFg, base.ErrorFg)
		if resolveErr != nil {
			return loaded, fmt.Errorf("theme %q error_fg: %w", name, resolveErr)
		}
		base.MutedFg, resolveErr = resolveColor(row.MutedFg, base.MutedFg)
		if resolveErr != nil {
			return loaded, fmt.Errorf("theme %q muted_fg: %w", name, resolveErr)
		}
		merged[name] = base
		loaded++
	}
	if loaded == 0 {
		return 0, fmt.Errorf("theme file has no valid theme entries")
	}
	themeRegistry = merged
	themeNames = orderedThemeNames(merged)
	return loaded, nil
}

func resolveColor(raw, fallback string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return fallback, nil
	}
	if strings.HasPrefix(key, "\x1b[") {
		return key, nil
	}
	if code, ok := themeColorCode[key]; ok {
		return code, nil
	}
	return "", fmt.Errorf("unknown color token %q", raw)
}

func orderedThemeNames(reg map[string]Theme) []string {
	names := make([]string, 0, len(reg))
	for name := range reg {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	if len(names) == 0 {
		return []string{"retro-amber"}
	}
	idx := -1
	for i, name := range names {
		if name == "retro-amber" {
			idx = i
			break
		}
	}
	if idx > 0 {
		names[0], names[idx] = names[idx], names[0]
	} else if idx == -1 {
		names = append([]string{"retro-amber"}, names...)
	}
	return names
}

var themeColorCode = map[string]string{
	"fg-black":   FgBlack,
	"fg-red":     FgRed,
	"fg-green":   FgGreen,
	"fg-yellow":  FgYellow,
	"fg-blue":    FgBlue,
	"fg-magenta": FgMagenta,
	"fg-cyan":    FgCyan,
	"fg-white":   FgWhite,
	"fg-orange":  FgOrange,
	"bg-black":   BgBlack,
	"bg-blue":    BgBlue,
	"bg-cyan":    BgCyan,
	"bg-gray":    BgGray,
}

func resetThemesForTest() {
	themeMu.Lock()
	defer themeMu.Unlock()
	themeRegistry = defaultThemeRegistry()
	themeNames = orderedThemeNames(themeRegistry)
}
