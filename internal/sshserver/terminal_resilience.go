package sshserver

import (
	"fmt"
	"os"
	"strings"

	gssh "github.com/gliderlabs/ssh"
	"wolfbbs/internal/term"
	"wolfbbs/internal/ui"
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

func currentSessionLayout(sess gssh.Session) (int, int, term.Profile) {
	width := ui.DefaultWidth
	height := 25
	termName := ""
	if pty, _, ok := sess.Pty(); ok {
		if pty.Window.Width > 0 {
			width = pty.Window.Width
		}
		if pty.Window.Height > 0 {
			height = pty.Window.Height
		}
		termName = pty.Term
	}
	if width < ui.MinWidth {
		width = ui.MinWidth
	}
	if height < 20 {
		height = 20
	}
	renderWidth := width
	if renderWidth > ui.DefaultWidth {
		renderWidth = ui.DefaultWidth
	}
	profile := term.DetectProfile(termName, os.Getenv("LANG"), strings.TrimSpace(os.Getenv("WOLFBBS_TERM_ENCODING")), width, height, true)
	return width, renderWidth, profile
}

func pagerPageSizeForHeight(height int) int {
	if height <= 0 {
		return 16
	}
	pageSize := height - 8
	if pageSize < 8 {
		pageSize = 8
	}
	if pageSize > 28 {
		pageSize = 28
	}
	return pageSize
}
