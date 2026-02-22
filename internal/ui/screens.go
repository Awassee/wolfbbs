package ui

import (
	"fmt"
	"strings"
	"time"
)

const (
	DefaultWidth  = 80
	DefaultHeight = 25
)

type Theme struct {
	StatusFg string
	StatusBg string
	BodyFg   string
	AccentFg string
	WarnFg   string
	ErrorFg  string
	MutedFg  string
}

func DefaultTheme() Theme {
	return Theme{
		StatusFg: FgWhite,
		StatusBg: BgBlue,
		BodyFg:   FgCyan,
		AccentFg: FgYellow,
		WarnFg:   FgRed,
		ErrorFg:  FgMagenta,
		MutedFg:  FgGreen,
	}
}

func RenderTopBar(width int, boardName, user string, now time.Time, node string, th Theme) string {
	line := fmt.Sprintf(" %s | User: %s | %s | %s ", boardName, user, now.Format("2006-01-02 15:04"), node)
	if len(line) > width {
		line = line[:width]
	}
	if len(line) < width {
		line += strings.Repeat(" ", width-len(line))
	}
	return th.StatusBg + th.StatusFg + Bold + line + Reset
}

func RenderWelcome(width int) string {
	art := []string{
		"┌──────────────────────────────────────────────────────────────────────────┐",
		"│   ██╗    ██╗██╗    ██╗███████╗██████╗ ██████╗ ███████╗                │",
		"│   ██║    ██║██║    ██║██╔════╝██╔══██╗██╔══██╗██╔════╝                │",
		"│   ██║ █╗ ██║██║ █╗██║███████╗██████╔╝██████╔╝███████╗                │",
		"│   ██║███╗██║██║███╗██║╚════██║██╔═══╝ ██╔══██╗╚════██║                │",
		"│   ╚███╔███╔╝╚███╔███╔╝███████║██║     ██████╔╝███████║                │",
		"│    ╚══╝╚══╝  ╚══╝╚══╝ ╚══════╝╚═╝     ╚═════╝ ╚══════╝                │",
		"├──────────────────────────────────────────────────────────────────────────┤",
		"│ WolfBBS  -  modern ANSI frontier, Wildcat-inspired, no copied assets      │",
		"├──────────────────────────────────────────────────────────────────────────┤",
		"│ Press ESC to quit, any other key to continue.                          │",
		"└──────────────────────────────────────────────────────────────────────────┘",
	}
	var b strings.Builder
	for _, l := range art {
		if len(l) > width {
			l = l[:width]
		}
		b.WriteString(FgYellow)
		b.WriteString(l)
		b.WriteString(Reset)
		b.WriteString("\r\n")
	}
	b.WriteString(FgGreen + "Welcome back to the terminal. Stay sharp, stay polite." + Reset + "\r\n")
	return b.String()
}

func RenderMainMenu(width int) string {
	lines := []string{
		"╔══════════════════════════════════ Wildcat Menu ══════════════════════════╗",
		"║ [M]essage Boards    [P]rivate Mail      [F]iles / Areas              ║",
		"║ [C]hat               [G]ateways          [S]ettings                  ║",
		"║ [D]oors              [W]ho's Online      [L]ast Callers              ║",
		"║ [A]dmin   (Sysop)                                                ║",
		"║ [Q]uit to previous menu / disconnect                                      ║",
		"╚═══════════════════════════════════════════════════════════════════════╝",
		"Selection:",
	}
	for i, l := range lines {
		if len(l) > width {
			lines[i] = l[:width]
		}
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

func RenderRegisterPrompt(width int) string {
	lines := []string{
		"┌───────────────────── New User Registration ─────────────────────┐",
		"│ Enter your desired handle and a secure password.                │",
		"│ Passwords should be 8+ characters and not all whitespace.       │",
		"└────────────────────────────────────────────────────────────────┘",
	}
	return clampLines(width, lines) + "\r\n"
}

func RenderBulletinList(width int, titles []string) string {
	lines := []string{
		"┌──────────────────────────── Bulletins ───────────────────────────┐",
	}
	for _, t := range titles {
		lines = append(lines, "│  * "+padOrTrim(t, width-5, " ")+ "│")
	}
	if len(lines) == 1 {
		lines = append(lines, "│ No bulletins today.                                        │")
	}
	lines = append(lines, "└──────────────────────────────────────────────────────────────┘")
	lines = append(lines, "Any key to return.")
	return clampLines(width, lines) + "\r\n"
}

func RenderMessageBoardList(width int, boards []string) string {
	lines := []string{
		"┌──────────────────── Message Boards ───────────────────────────────┐",
		"│ [1] Select board   [N] Next page   [P] Previous page            │",
	}
	for i, b := range boards {
		lines = append(lines, fmt.Sprintf("│ %-3d %-74s│", i+1, padOrTrim(b, 71, " ")))
	}
	lines = append(lines, "│ [Q]uit                                                  [ ]     │")
	lines = append(lines, "└──────────────────────────────────────────────────────────────┘")
	return clampLines(width, lines) + "\r\n"
}

func RenderMessageReader(width int, subject string, body []string, index, total int) string {
	lines := []string{
		"╔══════════════════════════ Message Reader ═════════════════════════╗",
		fmt.Sprintf("║ %-66s║", padOrTrim(subject, 66, " ")),
		"╠═════════════════════════════════════════════════════════════════════╣",
	}
	for _, b := range body {
		lines = append(lines, "║ "+padOrTrim(b, 67, " ")+"║")
	}
	lines = append(lines, "╠═════════════════════════════════════════════════════════════════════╣")
	lines = append(lines, fmt.Sprintf("║ (R)eply (N)ext (P)rev (Q)uit  (%d/%d)                               ║", index, total))
	lines = append(lines, "╚═════════════════════════════════════════════════════════════════════╝")
	lines = append(lines, "Press any key to continue, space/enter for pager next page.")
	return clampLines(width, lines) + "\r\n"
}

func RenderPostEditor(width int, subject string) string {
	lines := []string{
		"╔═══════════════ Post Editor ───────────────────────────────────────╗",
		"║ Compose area opens in plain text mode. Quote with: >                ║",
		fmt.Sprintf("║ Subject: %-60s║", padOrTrim(subject, 60, " ")),
		"╠═════════════════════════════════════════════════════════════════════╣",
		"║ Use Ctrl+W to save draft, Ctrl+X to send, Esc to cancel.        ║",
		"╚═════════════════════════════════════════════════════════════════════╝",
		"Enter text, finish with a blank line then Ctrl+X.",
	}
	return clampLines(width, lines) + "\r\n"
}

func RenderGatewayMenu(width int) string {
	lines := []string{
		"┌────────────────────────── Gateway Menu ───────────────────────────┐",
		"│ [E]mail gateway     Send external mail from BBS compose           │",
		"│ [W]eb gateway       Read and save URLs in offline reader        │",
		"│ [R]eturn                                                       │",
		"└───────────────────────────────────────────────────────────────────┘",
	}
	return clampLines(width, lines) + "\r\n"
}

func RenderDoorMenu(width int, options []string) string {
	if len(options) == 0 {
		lines := []string{
			"┌────────────────────────── Doors Menu ────────────────────────────┐",
			"│ No doors configured.                                             │",
			"│ [R]eturn                                                       │",
			"└────────────────────────────────────────────────────────────────┘",
		}
		return clampLines(width, lines) + "\r\n"
	}
	lines := []string{
		"┌────────────────────────── Doors Menu ────────────────────────────┐",
		"│ [R]eturn                                                       │",
		"├────────────────────────────────────────────────────────────────┤",
	}
	for _, hotkey := range options {
		lines = append(lines, "│ [ "+strings.ToUpper(hotkey)+" ] door entry (sample placeholder)                        │")
	}
	lines = append(lines, "└────────────────────────────────────────────────────────────────┘")
	return clampLines(width, lines) + "\r\n"
}

func RenderLastCallers(width int, users []string) string {
	lines := []string{
		"╔══════════════ Last Callers and Recent Traffic ═════════════╗",
	}
	for _, u := range users {
		lines = append(lines, "│ "+padOrTrim(u, 58, " ")+"│")
	}
	if len(lines) == 1 {
		lines = append(lines, "│ No caller history yet.                                   │")
	}
	lines = append(lines, "╚══════════════════════════════════════════════════════════════╝")
	lines = append(lines, "Press any key to return.")
	return clampLines(width, lines) + "\r\n"
}

func RenderWhoOnline(width int, users []string) string {
	header := "┌────────────── Who's Online / Presence ───────────────┐"
	lines := []string{header}
	for _, u := range users {
		lines = append(lines, "│ "+padOrTrim(u, len(header)-4, " ")+"│")
	}
	lines = append(lines, "├────────────────────────────────────────────────────┤")
	lines = append(lines, "│  node  user            login              area     idle │")
	for _, u := range users {
		lines = append(lines, "│  " + padOrTrim(u, len(header)-4, " ") + "│")
	}
	lines = append(lines, "└────────────────────────────────────────────────────┘")
	lines = append(lines, "Press any key to return.")
	return clampLines(width, lines) + "\r\n"
}

func clampLines(width int, lines []string) string {
	for i, line := range lines {
		if len(line) > width {
			lines[i] = line[:width]
		}
	}
	return strings.Join(lines, "\r\n")
}

func padOrTrim(value string, width int, pad string) string {
	if width <= 0 {
		return ""
	}
	if len(value) > width {
		return value[:width]
	}
	return value + strings.Repeat(pad, width-len(value))
}
