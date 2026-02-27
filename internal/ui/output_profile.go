package ui

import "strings"

func ApplyOutputProfile(frame string, ansiEnabled bool, encoding string) string {
	enc := strings.ToLower(strings.TrimSpace(encoding))
	if !ansiEnabled {
		frame = stripANSIEscapes(frame)
	}
	if !ansiEnabled || enc == "ascii" {
		frame = mapBoxDrawingToASCII(frame)
	}
	return frame
}

func mapBoxDrawingToASCII(value string) string {
	replacer := strings.NewReplacer(
		"╔", "+",
		"╗", "+",
		"╚", "+",
		"╝", "+",
		"═", "-",
		"║", "|",
		"╠", "+",
		"╣", "+",
		"╦", "+",
		"╩", "+",
		"╬", "+",
		"│", "|",
		"─", "-",
		"┌", "+",
		"┐", "+",
		"└", "+",
		"┘", "+",
		"┤", "+",
		"├", "+",
	)
	return replacer.Replace(value)
}

func stripANSIEscapes(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	inEscape := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if inEscape {
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				inEscape = false
			}
			continue
		}
		if ch == 0x1b {
			inEscape = true
			continue
		}
		out.WriteByte(ch)
	}
	return out.String()
}
