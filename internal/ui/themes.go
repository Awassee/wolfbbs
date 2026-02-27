package ui

import "strings"

var themeRegistry = map[string]Theme{
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

func ThemeByName(name string) Theme {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		key = "retro-amber"
	}
	if th, ok := themeRegistry[key]; ok {
		return th
	}
	return themeRegistry["retro-amber"]
}

func ThemeNames() []string {
	return []string{"retro-amber", "ice-blue", "emerald"}
}
