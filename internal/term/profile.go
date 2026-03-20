package term

import "strings"

type Encoding string

const (
	EncodingUTF8  Encoding = "utf8"
	EncodingCP437 Encoding = "cp437"
	EncodingASCII Encoding = "ascii"
)

type Profile struct {
	TermName  string
	Width     int
	Height    int
	ANSI      bool
	Encoding  Encoding
	SyncTERM  bool
	Narrow    bool
	LowHeight bool
	CompactUI bool
	Degraded  bool
}

func DetectProfile(termName, locale, explicitEncoding string, width, height int, ansi bool) Profile {
	termName = strings.ToLower(strings.TrimSpace(termName))
	locale = strings.ToLower(strings.TrimSpace(locale))
	if isPlainTerminal(termName) {
		ansi = false
	}
	explicit := parseEncoding(explicitEncoding)
	encoding := explicit
	if encoding == "" {
		encoding = detectEncoding(termName, locale)
	}
	if encoding == "" {
		encoding = EncodingUTF8
	}
	if !ansi && encoding != EncodingASCII {
		encoding = EncodingASCII
	}
	narrow := width > 0 && width < 56
	lowHeight := height > 0 && height < 22
	degraded := isPlainTerminal(termName) || !ansi || (width > 0 && width < 40) || (height > 0 && height < 18)
	return Profile{
		TermName:  termName,
		Width:     width,
		Height:    height,
		ANSI:      ansi,
		Encoding:  encoding,
		SyncTERM:  strings.Contains(termName, "syncterm"),
		Narrow:    narrow,
		LowHeight: lowHeight,
		CompactUI: degraded || narrow || lowHeight,
		Degraded:  degraded,
	}
}

func ProfileHints(p Profile) []string {
	out := make([]string, 0, 4)
	if isPlainTerminal(p.TermName) {
		out = append(out, "Plain terminal fallback active.")
	}
	if !p.ANSI {
		out = append(out, "ANSI color is disabled for this client.")
	}
	if p.Narrow {
		out = append(out, "Compact layout enabled for narrow width.")
	}
	if p.LowHeight {
		out = append(out, "Short-page mode enabled for low terminal height.")
	}
	if len(out) == 0 && p.CompactUI {
		out = append(out, "Compact layout enabled for this session.")
	}
	return out
}

func parseEncoding(value string) Encoding {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "utf8", "utf-8":
		return EncodingUTF8
	case "cp437", "ibm437", "dos":
		return EncodingCP437
	case "ascii", "plain":
		return EncodingASCII
	default:
		return ""
	}
}

func detectEncoding(termName, locale string) Encoding {
	if strings.Contains(termName, "syncterm") || strings.Contains(termName, "ansi-bbs") || strings.Contains(termName, "pcansi") {
		if strings.Contains(locale, "utf-8") || strings.Contains(locale, "utf8") {
			return EncodingUTF8
		}
		return EncodingCP437
	}
	if strings.Contains(locale, "utf-8") || strings.Contains(locale, "utf8") {
		return EncodingUTF8
	}
	if strings.Contains(termName, "vt100") || strings.Contains(termName, "ansi") {
		return EncodingCP437
	}
	return EncodingUTF8
}

func SupportsUnicode(p Profile) bool {
	return p.ANSI && p.Encoding == EncodingUTF8
}

func isPlainTerminal(termName string) bool {
	switch strings.ToLower(strings.TrimSpace(termName)) {
	case "", "dumb", "unknown":
		return true
	default:
		return false
	}
}
