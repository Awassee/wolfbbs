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
	return ThemeByName("retro-amber")
}

func RenderTopBar(width int, boardName, user string, now time.Time, node string, th Theme) string {
	return RenderTopBarWithClock(width, boardName, user, now, node, th, true)
}

func RenderTopBarWithClock(width int, boardName, user string, now time.Time, node string, th Theme, time24h bool) string {
	width = normalizeScreenWidth(width)
	area := strings.TrimSpace(boardName)
	if area == "" {
		area = "WolfBBS"
	}
	if strings.TrimSpace(user) == "" {
		user = "Guest"
	}
	clock := now.Format("2006-01-02 15:04")
	if !time24h {
		clock = now.Format("2006-01-02 03:04 PM")
	}
	line := fmt.Sprintf(" %s | User: %-12s | %s | %s ", area, user, clock, node)
	line = padOrTrim(line, width, " ")
	return th.StatusBg + th.StatusFg + Bold + line + Reset
}

func RenderWelcome(width int) string {
	width = normalizeScreenWidth(width)
	lines := []string{
		"                           /\\_/\\",
		"                          / o o \\",
		"                         (   \"   )",
		"                          \\~(*)~/",
		"                           // \\\\",
		"",
		"wolfbbs (c) 2026",
		"Node-ready ANSI board with classic flow and modern plumbing.",
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
	b.WriteString(FgGreen + CenterText(width, "wolfbbs (c) 2026") + Reset + "\r\n")
	return b.String()
}

func RenderMainMenu(width int) string {
	lines := []string{
		"[M]essage Boards   [P]rivate Mail   [F]iles",
		"[C]hat             [G]ateways       [D]oors",
		"[N]ewscan          [S]ettings       [A]dmin",
		"[L]ast Callers     [W]ho's Online",
		"[X]Config Center   [Y]Status Ctr    [/]Quick Jump",
		"[Q]uit to caller prompt",
		"",
		"Single-letter hotkeys only. Esc = Back. ? = Help.",
	}
	return renderPanel(width, "Main Menu", lines, FgCyan)
}

func RenderMainMenuHelp(width int, menuHint string) string {
	lines := []string{
		"Main Menu Key Guide",
		"",
		"M  Message Boards        P  Private Mail",
		"F  Files                 C  Chat",
		"G  Gateways              D  Doors",
		"N  Newscan Digest        S  Settings",
		"A  Sysop/Admin",
		"L  Last Callers          W  Who's Online",
		"X  Config Center         Y  Status Center",
		"/  Quick Jump prompt",
		"   - Includes app-upgrade (/app upgrade) for sysop",
		"Q  Quit to sign-off      Esc = Back",
		"?  Show this help panel",
		"",
		"Use single-letter keys. Menus are immediate and case-insensitive.",
	}
	if hint := strings.TrimSpace(menuHint); hint != "" {
		lines = append(lines, "")
		lines = append(lines, "Menu note:")
		lines = append(lines, hint)
	}
	return renderHelpPanel(width, "Help: Main Menu", lines)
}

func RenderStatusCenter(width int, lines []string) string {
	out := []string{
		"Runtime status for this node/session.",
		"",
	}
	out = append(out, lines...)
	if len(lines) == 0 {
		out = append(out, "No status lines available.")
	}
	out = append(out, "", "Press any key to return.")
	return renderPanel(width, "Status Center", out, FgYellow)
}

func RenderConfigCenter(width int, lines []string) string {
	out := []string{
		"Configuration snapshot (user + sysop flags).",
		"",
	}
	out = append(out, lines...)
	if len(lines) == 0 {
		out = append(out, "No configuration lines available.")
	}
	out = append(out, "", "Press any key to return.")
	return renderPanel(width, "Config Center", out, FgGreen)
}

func RenderSearchResults(width int, title string, lines []string) string {
	out := append([]string{}, lines...)
	if len(out) == 0 {
		out = append(out, "No matches found.")
	}
	out = append(out, "", "Press any key to return.")
	return renderPanel(width, title, out, FgCyan)
}

func RenderLoginPrompt(width int) string {
	return RenderLoginPromptWithGuest(width, true)
}

func RenderLoginPromptWithGuest(width int, guestTour bool) string {
	lines := []string{
		"Enter handle and password to continue.",
		"Unknown handle may create a new account after login attempt.",
		"Passwords are never stored in plaintext.",
	}
	if guestTour {
		lines = append([]string{
			"Enter handle and password to continue.",
			"Type GUEST for a read-only guided tour.",
		}, lines[1:]...)
	}
	lines = append(lines, "Type ? for login help.")
	return renderPanel(width, "Login", lines, FgGreen) + "\r\n"
}

func RenderLoginHelp(width int, guestTour bool) string {
	lines := []string{
		"Login Screen Commands",
		"",
		"Enter your handle, then your password.",
		"Unknown handles can be registered from the same flow.",
		"ESC or Ctrl-C exits to sign-off.",
		"",
		"Security:",
		"- Passwords are hashed (bcrypt/pbkdf2 policy)",
		"- Optional TOTP 2FA is supported",
	}
	if guestTour {
		lines = append(lines, "- Type GUEST to enter read-only guided tour mode")
	}
	return renderHelpPanel(width, "Help: Login", lines)
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

func RenderSinceLastCall(width int, titles []string, since time.Time) string {
	lines := []string{
		fmt.Sprintf("Since your last call: %s", since.Local().Format("2006-01-02 15:04")),
		"",
	}
	if len(titles) == 0 {
		lines = append(lines, "No new traffic since your last call.")
	} else {
		for i, t := range titles {
			lines = append(lines, fmt.Sprintf("%2d) %s", i+1, t))
		}
	}
	lines = append(lines, "")
	lines = append(lines, "Any key to return.")
	return renderPanel(width, "Newscan Digest", lines, FgYellow) + "\r\n"
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
	lines = append(lines, "Inside board: (N)ew (R)ead (S)earch (Q)uit (?)Help")
	return renderPanel(width, "Message Boards", lines, FgCyan) + "\r\n"
}

func RenderBoardMessageIndex(width int, boardName string, rows []string) string {
	title := "Board Messages"
	if trimmed := strings.TrimSpace(boardName); trimmed != "" {
		title = trimmed
	}
	lines := []string{
		" ID  T  Subject                         Posted",
		strings.Repeat("-", 52),
	}
	if len(rows) == 0 {
		lines = append(lines, " No messages yet.")
	} else {
		lines = append(lines, rows...)
	}
	lines = append(lines, "")
	lines = append(lines, "Commands: (N)ew, (R)ead, (S)earch, (Q)uit board, (?)help")
	lines = append(lines, "Selection:")
	return renderPanel(width, title, lines, FgCyan) + "\r\n"
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

func RenderBoardsHelp(width int) string {
	lines := []string{
		"Boards Navigation",
		"",
		"Board list:",
		"- Enter board ID to open",
		"- Q or Esc returns to Main Menu",
		"",
		"Inside a board:",
		"N  New post",
		"R  Read messages",
		"S  Search messages in this board (classic list)",
		"Q  Return to board list",
		"",
		"Reader keys:",
		"R  Reply    N/Enter/Right/PgDn  Next message",
		"P/Left/PgUp Previous message",
		"Q/Esc       Exit reader",
		"",
		"Paging: Space or Enter continues, Q/Esc exits pager.",
	}
	return renderHelpPanel(width, "Help: Message Boards", lines)
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
		"[R]eturn / [Q]uit / [?]help",
	}
	return renderPanel(width, "Gateway Menu", lines, FgCyan) + "\r\n"
}

func RenderMailOverview(width int, inboxRows []string, outboxRows []string) string {
	lines := []string{"Inbox:"}
	if len(inboxRows) == 0 {
		lines = append(lines, "  (empty)")
	} else {
		lines = append(lines, inboxRows...)
	}
	lines = append(lines, "")
	lines = append(lines, "Outbox:")
	if len(outboxRows) == 0 {
		lines = append(lines, "  (empty)")
	} else {
		lines = append(lines, outboxRows...)
	}
	lines = append(lines, "")
	lines = append(lines, "Commands: (C)ompose, (R)ead, Re(P)ly, (D)elete, (Q)uit, (?)help")
	lines = append(lines, "Selection:")
	return renderPanel(width, "Private Mail", lines, FgCyan) + "\r\n"
}

func RenderGatewayHelp(width int) string {
	lines := []string{
		"Gateway Commands",
		"",
		"E  Email gateway",
		"   - Sends through configured SMTP relay",
		"   - Verified accounts only (policy controlled)",
		"",
		"W  Web gateway",
		"   - Fetches URL with timeout, size caps, SSRF blocks",
		"   - Displays text in ANSI pager",
		"   - Optional offline save per user",
		"",
		"R/Q/Esc return to Main Menu",
		"",
		"Use complete URL input such as https://example.org",
	}
	return renderHelpPanel(width, "Help: Gateways", lines)
}

func RenderFilesMenu(width int, areas []string) string {
	lines := []string{
		"[ID] Open area      [R]ecent files     [N]ew since last call",
		"[S]earch by name    [I]ndexed search   [D]ownload queue",
		"[Q] Return          [?] Help",
		"",
		" ID  Area Name           Path                         Description",
		strings.Repeat("-", 68),
	}
	if len(areas) == 0 {
		lines = append(lines, "No file areas configured yet.")
	} else {
		lines = append(lines, areas...)
	}
	lines = append(lines, "")
	lines = append(lines, "Classic file areas with modern metadata safety.")
	return renderPanel(width, "Files", lines, FgCyan) + "\r\n"
}

func RenderFilesHelp(width int) string {
	lines := []string{
		"Files Commands",
		"",
		"From files menu:",
		"- Enter area ID to browse that area",
		"- R lists recent files across all areas",
		"- N lists files newer than your last login",
		"- S searches filenames across all areas",
		"- I uses indexed FileBase search + queue add by file ID",
		"- D manages your download queue and one-time tickets",
		"- Q or Esc returns to Main Menu",
		"",
		"Inside an area:",
		"- S updates filename filter",
		"- R refreshes listing",
		"- Q exits to area list",
	}
	return renderHelpPanel(width, "Help: Files", lines)
}

type DoorMenuItem struct {
	Hotkey         string
	Name           string
	Category       string
	TurnsRemaining int
	Favorite       bool
}

type DoorMenuSummary struct {
	Total         int
	Visible       int
	Category      string
	FavoritesOnly bool
	RecentOnly    bool
	Spotlight     string
}

func RenderDoorMenu(width int, items []DoorMenuItem, favoriteIDs []string, recentIDs []string, summary DoorMenuSummary) string {
	lines := []string{
		"[R]eturn  [Q]uit  [!] Favorite Toggle  [F]avorites  [V]Recent  [C]ategory  [?] Help  [T] Trophies",
		"",
	}
	filterParts := []string{fmt.Sprintf("Showing %d of %d", summary.Visible, summary.Total)}
	if summary.Category != "" {
		filterParts = append(filterParts, "Category="+strings.ToUpper(summary.Category))
	}
	if summary.FavoritesOnly {
		filterParts = append(filterParts, "Favorites only")
	}
	if summary.RecentOnly {
		filterParts = append(filterParts, "Recent only")
	}
	lines = append(lines, strings.Join(filterParts, "  |  "))
	if strings.TrimSpace(summary.Spotlight) != "" {
		lines = append(lines, "Spotlight: "+summary.Spotlight)
	}
	lines = append(lines, "")
	if len(favoriteIDs) > 0 {
		lines = append(lines, "Favorites: "+strings.Join(favoriteIDs, ", "))
	}
	if len(recentIDs) > 0 {
		lines = append(lines, "Recent: "+strings.Join(recentIDs, ", "))
	}
	if len(favoriteIDs) > 0 || len(recentIDs) > 0 {
		lines = append(lines, "")
	}
	if len(items) == 0 {
		lines = append(lines, "No doors matched the active filter.")
		return renderPanel(width, "Door Hub", lines, FgYellow) + "\r\n"
	}
	lines = append(lines, "HK  Category   Door Name                          Turns  Flags")
	lines = append(lines, strings.Repeat("-", 62))
	for _, item := range items {
		turns := "-"
		if item.TurnsRemaining > 0 {
			turns = fmt.Sprintf("%d", item.TurnsRemaining)
		}
		flags := ""
		if item.Favorite {
			flags = "*"
		}
		lines = append(lines, fmt.Sprintf("%-3s %-10s %-33s %-6s %-3s",
			strings.ToUpper(item.Hotkey),
			trimRunes(strings.ToUpper(item.Category), 10),
			trimRunes(item.Name, 33),
			turns,
			flags,
		))
	}
	return renderPanel(width, "Door Hub", lines, FgYellow) + "\r\n"
}

func RenderDoorsHelp(width int) string {
	lines := []string{
		"Door Hub Commands",
		"",
		"[Door hotkey] launch selected door",
		"!            toggle favorite by hotkey",
		"F            toggle favorites-only filter",
		"V            toggle recent-only filter",
		"C            cycle category filter",
		"T            open scores and trophies",
		"Q/Esc/R      return to Main Menu",
		"",
		"Turns:",
		"- Each door can enforce daily turn limits",
		"- Some doors use time bank carryover",
		"",
		"External doors run with timeouts and sandbox limits.",
	}
	return renderHelpPanel(width, "Help: Doors", lines)
}

func RenderMailHelp(width int) string {
	lines := []string{
		"Private Mail Commands",
		"",
		"C  Compose message",
		"R  Read message by ID",
		"P  Reply to message by ID",
		"D  Delete message by ID",
		"Q  Return to Main Menu",
		"",
		"Compose details:",
		"- Recipient can be local handle or external email",
		"- Body entry ends with single period on its own line",
		"",
		"Reader details:",
		"- Inbox mail is marked read when opened",
		"- Outbox includes local and external destinations",
		"- Reader hotkeys: P reply, D delete, Q back",
	}
	return renderHelpPanel(width, "Help: Private Mail", lines)
}

func RenderChatHelp(width int) string {
	lines := []string{
		"Live Chat Commands",
		"",
		"S  Send message",
		"J  Join channel",
		"O  Show online users",
		"R/Enter refresh current channel",
		"Q/Esc return to Main Menu",
		"",
		"Notes:",
		"- Default channel is #lobby",
		"- Moderation and rate-limit rules are enforced server-side",
		"- Messages persist and are shared with web and IRC clients",
	}
	return renderHelpPanel(width, "Help: Live Chat", lines)
}

func RenderSettingsHelp(width int) string {
	lines := []string{
		"Settings Screen Commands",
		"",
		"T  cycle theme",
		"A  toggle ANSI on/off",
		"P  toggle pager on/off",
		"C  toggle 24-hour clock",
		"S  save preferences",
		"Q/Esc return without saving changes",
		"",
		"Theme + ANSI settings apply on next redraw immediately.",
	}
	return renderHelpPanel(width, "Help: Settings", lines)
}

func RenderLastCallers(width int, users []string) string {
	lines := []string{
		"Node User         Login Time        Orig  From            Area         Duration",
		strings.Repeat("-", 76),
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
		"Node User         Login Time        Orig  From            Area         Idle",
		strings.Repeat("-", 72),
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

func RenderGuestTour(width int, lines []string) string {
	out := []string{
		"Read-only guided tour. No posting in this mode.",
		"",
	}
	out = append(out, lines...)
	out = append(out, "")
	out = append(out, "Press any key to return to login.")
	return renderPanel(width, "Guest Tour", out, FgGreen) + "\r\n"
}

func renderHelpPanel(width int, title string, lines []string) string {
	helpLines := append([]string{}, lines...)
	helpLines = append(helpLines, "")
	helpLines = append(helpLines, "Press any key to return.")
	return renderPanel(width, title, helpLines, FgYellow) + "\r\n"
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
