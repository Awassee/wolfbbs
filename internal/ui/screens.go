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
	width = normalizeScreenWidth(width)
	area := strings.TrimSpace(boardName)
	if area == "" {
		area = "WolfBBS"
	}
	if strings.TrimSpace(user) == "" {
		user = "Guest"
	}
	line := fmt.Sprintf(" %s | User: %-12s | %s | %s ", area, user, now.Format("2006-01-02 15:04"), node)
	line = padOrTrim(line, width, " ")
	return th.StatusBg + th.StatusFg + Bold + line + Reset
}

func RenderWelcome(width int) string {
	width = normalizeScreenWidth(width)
	lines := []string{
		" __      __      ______  ______  ____   ____   _____ ",
		" \\ \\ /\\ / /___  / / __ )/ __ ) \\/ / /  / __ ) / ___/ ",
		"  \\ V  V / __ \\/ / __  / __  |\\  / /  / __  | \\__ \\  ",
		"   \\_/\\_/ /_/ / / /_/ / /_/ / / / /__/ /_/ / ___/ /  ",
		"       \\____/_/_____/_____/ /_/\\____/_____/ /____/   ",
		"",
		"Node-ready ANSI board with classic flow and modern plumbing.",
		"Original art + text. Wildcat-era feel, not copied assets.",
		"",
		"Press ESC to quit, any other key to continue.",
	}
	panel := renderPanel(width, "WolfBBS Welcome", lines, FgYellow)
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(panel, "\r\n"), "\r\n") {
		b.WriteString(FgYellow)
		b.WriteString(line)
		b.WriteString(Reset)
		b.WriteString("\r\n")
	}
	b.WriteString(FgGreen + CenterText(width, "Welcome back to the terminal frontier.") + Reset + "\r\n")
	return b.String()
}

func RenderMainMenu(width int) string {
	lines := []string{
		"[M]essage Boards   [P]rivate Mail   [F]iles",
		"[C]hat             [G]ateways       [D]oors",
		"[S]ettings         [A]dmin          [L]ast Callers",
		"[W]ho's Online     [Q]uit to caller prompt",
		"",
		"Single-letter hotkeys only. Esc = Back. ? = Help.",
	}
	return renderPanel(width, "Main Menu", lines, FgCyan)
}

func RenderLoginPrompt(width int) string {
	lines := []string{
		"Enter handle and password to continue.",
		"Unknown handle may create a new account after login attempt.",
		"Passwords are never stored in plaintext.",
	}
	return renderPanel(width, "Login", lines, FgGreen) + "\r\n"
}

func RenderRegisterPrompt(width int) string {
	lines := []string{
		"New user registration:",
		"- Handle should be short and unique.",
		"- Password should be at least 8 characters.",
		"- Keep ANSI enabled for full board visuals.",
	}
	return renderPanel(width, "New User", lines, FgGreen) + "\r\n"
}

func RenderBulletinList(width int, titles []string) string {
	lines := []string{}
	for i, t := range titles {
		lines = append(lines, fmt.Sprintf("%2d) %s", i+1, t))
	}
	if len(lines) == 0 {
		lines = append(lines, "No bulletins today.")
	}
	lines = append(lines, "Any key to return.")
	return renderPanel(width, "Bulletins", lines, FgYellow) + "\r\n"
}

func RenderMessageBoardList(width int, boards []string) string {
	lines := []string{
		"[ID] Open board      [Q] Return      [PgUp/PgDn] Page",
		"",
	}
	for i, b := range boards {
		lines = append(lines, fmt.Sprintf("%3d  %s", i+1, b))
	}
	if len(boards) == 0 {
		lines = append(lines, "No boards available.")
	}
	lines = append(lines, "")
	lines = append(lines, "Inside board: (N)ew (R)ead (Q)uit (?)Help")
	return renderPanel(width, "Message Boards", lines, FgCyan) + "\r\n"
}

func RenderMessageReader(width int, subject string, body []string, index, total int) string {
	lines := []string{
		subject,
		strings.Repeat("-", 30),
	}
	for _, b := range body {
		lines = append(lines, b)
	}
	lines = append(lines, strings.Repeat("-", 30))
	lines = append(lines, fmt.Sprintf("(R)eply (N)ext (P)rev (Q)uit (?)Help   Msg %d/%d", index, total))
	lines = append(lines, "More: Space/Enter next page, Q/Esc exits pager.")
	return renderPanel(width, "Message Reader", lines, FgCyan) + "\r\n"
}

func RenderPostEditor(width int, subject string) string {
	lines := []string{
		"Compose area opens in plain text mode. Quote with: >",
		fmt.Sprintf("Subject: %s", subject),
		"Finish with a line containing a single period: .",
		"Esc or Q backs out at the next prompt.",
	}
	return renderPanel(width, "Post Editor", lines, FgGreen) + "\r\n"
}

func RenderGatewayMenu(width int) string {
	lines := []string{
		"[E]mail gateway   Send external mail via SMTP relay",
		"[W]eb gateway     Read URL in ANSI pager + offline save",
		"[R]eturn / [Q]uit",
	}
	return renderPanel(width, "Gateway Menu", lines, FgCyan) + "\r\n"
}

func RenderDoorMenu(width int, options []string) string {
	lines := []string{"[R]eturn / [Q]uit", ""}
	if len(options) == 0 {
		lines = append(lines, "No doors configured.")
		return renderPanel(width, "Doors Menu", lines, FgYellow) + "\r\n"
	}
	for _, hotkey := range options {
		lines = append(lines, fmt.Sprintf("[%s] Launch configured door", strings.ToUpper(hotkey)))
	}
	return renderPanel(width, "Doors Menu", lines, FgYellow) + "\r\n"
}

func RenderLastCallers(width int, users []string) string {
	lines := []string{
		"Node User         Login Time         Area         Idle",
		strings.Repeat("-", 56),
	}
	for _, u := range users {
		lines = append(lines, u)
	}
	if len(users) == 0 {
		lines = append(lines, "No caller history yet.")
	}
	lines = append(lines, "Press any key to return.")
	return renderPanel(width, "Last Callers", lines, FgCyan) + "\r\n"
}

func RenderWhoOnline(width int, users []string) string {
	lines := []string{
		"Node User         Login Time         Area         Idle",
		strings.Repeat("-", 56),
	}
	for _, u := range users {
		lines = append(lines, u)
	}
	if len(users) == 0 {
		lines = append(lines, "None online.")
	}
	lines = append(lines, "Press any key to return.")
	return renderPanel(width, "Who's Online", lines, FgCyan) + "\r\n"
}

func clampLines(width int, lines []string) string {
	width = normalizeScreenWidth(width)
	for i, line := range lines {
		lines[i] = trimRunes(line, width)
	}
	return strings.Join(lines, "\r\n")
}

func padOrTrim(value string, width int, pad string) string {
	if width <= 0 {
		return ""
	}
	value = trimRunes(value, width)
	if runeLen(value) > width {
		return trimRunes(value, width)
	}
	return value + strings.Repeat(pad, width-runeLen(value))
}

func normalizeScreenWidth(width int) int {
	if width <= 0 {
		return DefaultWidth
	}
	if width < 40 {
		return 40
	}
	return width
}

func renderPanel(width int, title string, content []string, fg string) string {
	width = normalizeScreenWidth(width)
	if fg == "" {
		fg = FgCyan
	}
	innerWidth := width - 2
	lines := make([]string, 0, len(content))
	for _, line := range content {
		lines = append(lines, padOrTrim(line, innerWidth, " "))
	}
	return DrawBox(width, len(lines)+2, title, lines, CP437Box, fg, BgBlack)
}
