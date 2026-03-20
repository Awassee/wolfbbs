package sshserver

import (
	"fmt"
	"strings"

	"wolfbbs/internal/term"
)

func resolveSessionOutput(profile term.Profile, ansiPreference bool) (bool, string) {
	sessionANSI := ansiPreference && profile.ANSI
	if !sessionANSI {
		return false, string(term.EncodingASCII)
	}
	encoding := strings.TrimSpace(string(profile.Encoding))
	if encoding == "" {
		encoding = string(term.EncodingUTF8)
	}
	return true, encoding
}

func sessionProfileStatusLines(profile term.Profile, sessionANSI bool, encoding string) []string {
	termName := strings.TrimSpace(profile.TermName)
	if termName == "" {
		termName = "(unknown)"
	}
	lines := []string{
		fmt.Sprintf("Terminal type: %s", termName),
		fmt.Sprintf("Terminal size: %dx%d", profile.Width, profile.Height),
		fmt.Sprintf("Session ANSI: %s", boolText(sessionANSI)),
		fmt.Sprintf("Session encoding: %s", strings.ToLower(strings.TrimSpace(encoding))),
		fmt.Sprintf("Compact mode: %s", boolText(profile.CompactUI)),
		fmt.Sprintf("Degraded fallback: %s", boolText(profile.Degraded)),
	}
	for _, hint := range term.ProfileHints(profile) {
		lines = append(lines, "  - "+hint)
	}
	return lines
}
