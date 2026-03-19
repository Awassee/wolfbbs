package ui

import (
	"fmt"
	"strings"
	"time"
)

const (
	DefaultWidth  = 80
	DefaultHeight = 25
	MinWidth      = 32
)

type menuEntry struct {
	Key   string
	Label string
}

type ActionMenuEntry struct {
	Key    string
	Label  string
	Target string
}

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
	shortClock := now.Format("15:04")
	if !time24h {
		shortClock = now.Format("03:04PM")
	}
	line := ""
	switch {
	case width <= 40:
		userWidth := 6
		areaWidth := width - (1 + userWidth + len(shortClock) + 8)
		if areaWidth < 6 {
			areaWidth = 6
		}
		line = fmt.Sprintf(" %s | %s | %s ", compactTopBarValue(area, areaWidth), compactTopBarValue(user, userWidth), shortClock)
	case width <= 56:
		line = fmt.Sprintf(" %s | %s | %s | %s ", compactTopBarValue(area, 12), compactTopBarValue(user, 8), shortClock, compactTopBarValue(node, 8))
	default:
		line = fmt.Sprintf(" %s | User: %-12s | %s | %s ", area, user, clock, node)
	}
	line = padOrTrim(line, width, " ")
	return th.StatusBg + th.StatusFg + Bold + line + Reset
}

func RenderWelcome(width int) string {
	width = normalizeScreenWidth(width)
	lines := append([]string{}, welcomeWolfArt(width)...)
	lines = append(lines,
		"",
		"wolfbbs (c) 2026",
		"Wildcat-era glow, modern rails, node-ready ANSI.",
		"Press ESC to quit, any other key to continue.",
	)
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
		"Retro flow, modern rails. Choose a lane and jump fast.",
		"",
		sectionLabel("Comms + Content"),
	}
	lines = append(lines, renderMenuGrid(width, []menuEntry{
		{Key: "M", Label: "Message Boards"},
		{Key: "P", Label: "Private Mail"},
		{Key: "F", Label: "Files"},
		{Key: "C", Label: "Chat"},
		{Key: "G", Label: "Gateways"},
		{Key: "D", Label: "Doors"},
	}, 3)...)
	lines = append(lines, "", sectionLabel("Caller Intel + System"))
	lines = append(lines, renderMenuGrid(width, []menuEntry{
		{Key: "N", Label: "Newscan"},
		{Key: "R", Label: "Caller Pulse"},
		{Key: "L", Label: "Last Callers"},
		{Key: "W", Label: "Who's Online"},
		{Key: "S", Label: "Settings"},
		{Key: "X", Label: "Config Center"},
		{Key: "Y", Label: "Status Center"},
	}, 3)...)
	lines = append(lines, "", sectionLabel("Quick Ops"))
	lines = append(lines, renderMenuGrid(width, []menuEntry{
		{Key: "/", Label: "Quick Jump"},
		{Key: "A", Label: "Admin"},
		{Key: "Q", Label: "Quit to prompt"},
	}, 3)...)
	lines = append(lines, "", sectionLabel("Global Shortcuts"))
	lines = append(lines, commandStripLines(width, []string{
		"Single-letter hotkeys only",
		"Esc = Back",
		"? = Help",
		"/ = Quick Jump",
	})...)
	return renderPanel(width, "Main Menu", lines, FgCyan)
}

func RenderConfiguredMainMenu(width int, title, help string, entries []ActionMenuEntry) string {
	menuTitle := strings.TrimSpace(title)
	if menuTitle == "" {
		menuTitle = "Main Menu"
	}
	lines := []string{
		sectionLabel("Custom Command Deck"),
	}
	lines = append(lines, renderConfiguredMenuRows(width, entries)...)
	if len(entries) == 0 {
		lines = append(lines, "No menu options are currently available.")
	}
	if trimmedHelp := strings.TrimSpace(help); trimmedHelp != "" {
		lines = append(lines, "")
		lines = append(lines, sectionLabel("Menu Note"))
		lines = append(lines, trimmedHelp)
	}
	lines = append(lines, "")
	lines = append(lines, commandStripLines(width, []string{
		"Single-letter hotkeys only",
		"? = Help",
		"Q = Quit",
	})...)
	return renderPanel(width, menuTitle, lines, FgCyan)
}

func RenderMainMenuHelp(width int, menuHint string) string {
	lines := []string{
		"Main Menu Key Guide",
		"",
		"M  Message Boards        P  Private Mail",
		"F  Files                 C  Chat",
		"G  Gateways              D  Doors",
		"N  Newscan Digest        R  Caller Pulse",
		"S  Settings",
		"A  Sysop/Admin",
		"L  Last Callers          W  Who's Online",
		"X  Config Center         Y  Status Center",
		"/  Quick Jump prompt",
		"   - Includes bookmarks/circles/showcase/statusz aliases",
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
	lines = append(lines, "Type RESET for password reset.", "Type ? for login help.")
	return renderPanel(width, "Login", lines, FgGreen) + "\r\n"
}

func RenderLoginHelp(width int, guestTour bool) string {
	lines := []string{
		"Login Screen Commands",
		"",
		"Enter your handle, then your password.",
		"Unknown handles can be registered from the same flow.",
		"Type RESET to request or complete a password reset.",
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
		sectionLabel("Board Command Bar"),
	}
	lines = append(lines, commandStripLines(width, []string{"[ID] Open board", "[Q] Return", "[C] Conference", "[?] Help"})...)
	lines = append(lines, "")
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
		sectionLabel("Gateway Desk"),
		"[W]eb browser     Read URL in ANSI pager + offline save",
		"[E]mail gateway   Send external mail via SMTP relay",
		"[F]eed reader     Parse RSS/Atom into compact headlines",
		"[S]ummarizer      Build quick bullets from article URL",
		"[J]SON explorer   Pretty-print JSON API responses",
		"[X] Caller pulse  Terminal parity for next/streaks/events/challenges",
		"[A]I assistant    Prompt configured AI model",
	}
	lines = append(lines, commandStripLines(width, []string{"[R]eturn", "[Q]uit", "[?] Help"})...)
	return renderPanel(width, "Gateway Menu", lines, FgCyan) + "\r\n"
}

func RenderMailOverview(width int, inboxRows []string, outboxRows []string) string {
	lines := []string{
		sectionLabel("Mail Command Bar"),
	}
	lines = append(lines, commandStripLines(width, []string{"[C] Compose", "[R] Read", "Re[P]ly", "[D] Delete", "[H] Handles", "[Q] Quit", "[?] Help"})...)
	lines = append(lines, "", sectionLabel("Inbox:"))
	if len(inboxRows) == 0 {
		lines = append(lines, "  (empty)")
	} else {
		lines = append(lines, inboxRows...)
	}
	lines = append(lines, "")
	lines = append(lines, sectionLabel("Outbox:"))
	if len(outboxRows) == 0 {
		lines = append(lines, "  (empty)")
	} else {
		lines = append(lines, outboxRows...)
	}
	lines = append(lines, "", "Commands: (C)ompose, (R)ead, Re(P)ly, (D)elete, (H)andles, (Q)uit, (?)help", "Selection:")
	return renderPanel(width, "Private Mail", lines, FgCyan) + "\r\n"
}

func RenderGatewayHelp(width int) string {
	lines := []string{
		"Gateway Commands",
		"",
		"W  Web browser",
		"   - Fetches URL with timeout, size caps, SSRF blocks",
		"   - Displays text in ANSI pager",
		"   - Optional offline save per user",
		"",
		"E  Email gateway",
		"   - Sends through configured SMTP relay",
		"   - Verified accounts only (policy controlled)",
		"",
		"F  Feed reader",
		"   - Pull RSS/Atom feeds through gateway safety policy",
		"   - Renders newest items in compact terminal view",
		"",
		"S  Summarizer",
		"   - Builds headline + bullets + excerpt from URL",
		"",
		"J  JSON explorer",
		"   - Fetches JSON endpoints and pretty-prints output",
		"",
		"X  Caller pulse",
		"   - Terminal parity snapshot for /next, /streaks, /topx,",
		"     /missions, /tournaments, /events, /events/recaps,",
		"     /challenges, /spotlights, /digest/preferences,",
		"     /mentorship, /milestones, /time-lane, /resume,",
		"     /doors/comeback",
		"",
		"A  AI assistant",
		"   - Uses configured OpenAI-compatible gateway settings",
		"",
		"R/Q/Esc return to Main Menu",
		"",
		"Use complete URL input such as https://example.org",
	}
	return renderHelpPanel(width, "Help: Gateways", lines)
}

func RenderFilesMenu(width int, areas []string) string {
	lines := []string{
		sectionLabel("File Command Bar"),
	}
	lines = append(lines, commandStripLines(width, []string{"[ID] Open area", "[R]ecent files", "[N]ew since last call", "[S]earch", "[I]ndexed search", "[D]ownload queue", "[C]ollections", "[O]ffline center", "[Q] Return", "[?] Help"})...)
	lines = append(lines, "", filesHeader(width), filesDivider(width))
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
		"- C opens featured collections parity for /collections",
		"- O opens offline packet export/import parity for /offline",
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
		sectionLabel("Door Command Bar"),
	}
	lines = append(lines, commandStripLines(width, []string{"[R]eturn", "[Q]uit", "[!] Favorite Toggle", "[F]avorites", "[V]Recent", "[C]ategory", "[T] Trophies", "[?] Help"})...)
	lines = append(lines, "")
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
	lines = append(lines, doorHeader(width))
	lines = append(lines, doorDivider(width))
	for _, item := range items {
		turns := "-"
		if item.TurnsRemaining > 0 {
			turns = fmt.Sprintf("%d", item.TurnsRemaining)
		}
		flags := ""
		if item.Favorite {
			flags = "*"
		}
		lines = append(lines, formatDoorRow(width, item, turns, flags))
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
		"H  Search recipient handles",
		"Q  Return to Main Menu",
		"",
		"Compose details:",
		"- Recipient can be local handle or external email",
		"- Type ?prefix in recipient prompt to search handles",
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
		"B  open bookmarks manager",
		"O  open caller circles manager",
		"X  profile export JSON (/profile/export parity)",
		"E  attention export JSON (/attention/export parity)",
		"S  save preferences",
		"Q/Esc return without saving changes",
		"",
		"Theme + ANSI settings apply on next redraw immediately.",
	}
	return renderHelpPanel(width, "Help: Settings", lines)
}

func RenderLastCallers(width int, users []string) string {
	lines := []string{
		callerHeader(width, false),
		callerDivider(width, false),
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
		callerHeader(width, true),
		callerDivider(width, true),
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
		lines[i] = trimANSIVisible(line, width)
	}
	return strings.Join(lines, "\r\n")
}

func compactTopBarValue(value string, width int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if width <= 0 {
		return ""
	}
	r := []rune(value)
	if len(r) <= width {
		return value
	}
	if width <= 3 {
		return string(r[:width])
	}
	return string(r[:width-1]) + "+"
}

func padOrTrim(value string, width int, pad string) string {
	if width <= 0 {
		return ""
	}
	value = trimANSIVisible(value, width)
	if visibleRuneLen(value) > width {
		return trimANSIVisible(value, width)
	}
	return value + strings.Repeat(pad, width-visibleRuneLen(value))
}

func normalizeScreenWidth(width int) int {
	if width <= 0 {
		return DefaultWidth
	}
	if width < MinWidth {
		return MinWidth
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
		lines = append(lines, fitPanelLine(innerWidth, line)...)
	}
	return DrawBox(width, len(lines)+2, title, lines, CP437Box, fg, BgBlack)
}

func fitPanelLine(width int, line string) []string {
	if width <= 0 {
		return []string{""}
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return []string{strings.Repeat(" ", width)}
	}
	if strings.Contains(line, Esc) || strings.HasPrefix(line, " ") {
		return []string{padOrTrim(line, width, " ")}
	}
	if visibleRuneLen(line) <= width {
		return []string{padOrTrim(line, width, " ")}
	}
	raw := wrapWordsToWidth(line, width)
	if len(raw) == 0 {
		return []string{padOrTrim(line, width, " ")}
	}
	lines := make([]string, 0, len(raw))
	for _, row := range raw {
		lines = append(lines, padOrTrim(row, width, " "))
	}
	return lines
}

func wrapWordsToWidth(line string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	words := strings.Fields(line)
	if len(words) <= 1 {
		return []string{trimANSIVisible(line, width)}
	}
	lines := make([]string, 0, 4)
	current := words[0]
	for _, word := range words[1:] {
		if visibleRuneLen(current)+1+visibleRuneLen(word) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		current += " " + word
	}
	lines = append(lines, current)
	return lines
}

func renderMenuGrid(width int, entries []menuEntry, maxCols int) []string {
	innerWidth := normalizeScreenWidth(width) - 2
	cols := menuColumns(innerWidth, maxCols)
	colWidth := innerWidth
	if cols > 1 {
		colWidth = (innerWidth - ((cols - 1) * 2)) / cols
	}
	if colWidth < 14 {
		cols = 1
		colWidth = innerWidth
	}
	lines := make([]string, 0, (len(entries)+cols-1)/cols)
	for i := 0; i < len(entries); i += cols {
		row := make([]string, 0, cols)
		for j := 0; j < cols && i+j < len(entries); j++ {
			row = append(row, padOrTrim(renderMenuCell(entries[i+j]), colWidth, " "))
		}
		lines = append(lines, strings.Join(row, "  "))
	}
	return lines
}

func menuColumns(innerWidth, maxCols int) int {
	if maxCols < 1 {
		return 1
	}
	switch {
	case innerWidth >= 66:
		if maxCols > 3 {
			return 3
		}
		return maxCols
	case innerWidth >= 46:
		if maxCols > 2 {
			return 2
		}
		return maxCols
	default:
		return 1
	}
}

func renderMenuCell(entry menuEntry) string {
	key := strings.ToUpper(strings.TrimSpace(entry.Key))
	label := strings.TrimSpace(entry.Label)
	if key == "" {
		return label
	}
	return Bold + FgYellow + "[" + key + "]" + Reset + " " + label
}

func commandStripLines(width int, items []string) []string {
	innerWidth := normalizeScreenWidth(width) - 2
	if innerWidth <= 0 {
		return []string{}
	}
	filtered := make([]string, 0, len(items))
	for _, row := range items {
		trimmed := strings.TrimSpace(row)
		if trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	if len(filtered) == 0 {
		return []string{}
	}
	lines := make([]string, 0, 2)
	current := ""
	for _, item := range filtered {
		candidate := item
		if current != "" {
			candidate = current + "  " + item
		}
		if visibleRuneLen(candidate) <= innerWidth {
			current = candidate
			continue
		}
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
		if visibleRuneLen(item) <= innerWidth {
			current = item
			continue
		}
		lines = append(lines, wrapWordsToWidth(item, innerWidth)...)
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func renderConfiguredMenuRows(width int, entries []ActionMenuEntry) []string {
	innerWidth := normalizeScreenWidth(width) - 2
	if len(entries) == 0 {
		return []string{}
	}
	lines := make([]string, 0, len(entries)*2)
	if innerWidth >= 58 {
		commandWidth := 26
		if commandWidth > innerWidth-10 {
			commandWidth = innerWidth - 10
		}
		if commandWidth < 16 {
			commandWidth = 16
		}
		targetWidth := innerWidth - commandWidth - 2
		if targetWidth < 8 {
			targetWidth = 8
		}
		for _, entry := range entries {
			cell := renderMenuCell(menuEntry{Key: entry.Key, Label: entry.Label})
			target := strings.TrimSpace(entry.Target)
			if target == "" {
				target = "(internal)"
			}
			lines = append(lines, padOrTrim(cell, commandWidth, " ")+"  "+padOrTrim(trimANSIVisible(target, targetWidth), targetWidth, " "))
		}
		return lines
	}
	for _, entry := range entries {
		lines = append(lines, renderMenuCell(menuEntry{Key: entry.Key, Label: entry.Label}))
		target := strings.TrimSpace(entry.Target)
		if target != "" {
			for _, row := range wrapWordsToWidth("-> "+target, innerWidth-2) {
				lines = append(lines, "  "+row)
			}
		}
	}
	return lines
}

func sectionLabel(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	return "== " + title + " =="
}

func filesHeader(width int) string {
	if normalizeScreenWidth(width) >= 72 {
		return " ID  Area Name           Path                         Description"
	}
	if normalizeScreenWidth(width) >= 54 {
		return " ID  Area Name           Path                 Description"
	}
	return " ID  Area Name           Path"
}

func filesDivider(width int) string {
	switch {
	case normalizeScreenWidth(width) >= 72:
		return strings.Repeat("-", 68)
	case normalizeScreenWidth(width) >= 54:
		return strings.Repeat("-", 58)
	default:
		return strings.Repeat("-", 40)
	}
}

func callerHeader(width int, online bool) string {
	if normalizeScreenWidth(width) >= 74 {
		if online {
			return "Node User         Login Time        Orig  From            Area         Idle"
		}
		return "Node User         Login Time        Orig  From            Area         Duration"
	}
	if normalizeScreenWidth(width) >= 58 {
		if online {
			return "Node User         Orig  From            Area         Idle"
		}
		return "Node User         Orig  From            Area         Dur"
	}
	if online {
		return "Node User         Orig  Area         Idle"
	}
	return "Node User         Orig  Area         Dur"
}

func callerDivider(width int, online bool) string {
	switch {
	case normalizeScreenWidth(width) >= 74:
		if online {
			return strings.Repeat("-", 72)
		}
		return strings.Repeat("-", 76)
	case normalizeScreenWidth(width) >= 58:
		return strings.Repeat("-", 58)
	default:
		return strings.Repeat("-", 40)
	}
}

func doorHeader(width int) string {
	if normalizeScreenWidth(width) >= 72 {
		return "HK  Category   Door Name                          Turns  Flags"
	}
	if normalizeScreenWidth(width) >= 54 {
		return "HK  Category   Door Name                  Turns  Flags"
	}
	return "HK  Door Name                    Turns  Flags"
}

func doorDivider(width int) string {
	switch {
	case normalizeScreenWidth(width) >= 72:
		return strings.Repeat("-", 62)
	case normalizeScreenWidth(width) >= 54:
		return strings.Repeat("-", 54)
	default:
		return strings.Repeat("-", 40)
	}
}

func formatDoorRow(width int, item DoorMenuItem, turns, flags string) string {
	switch {
	case normalizeScreenWidth(width) >= 72:
		return fmt.Sprintf("%-3s %-10s %-33s %-6s %-3s",
			strings.ToUpper(item.Hotkey),
			trimRunes(strings.ToUpper(item.Category), 10),
			trimRunes(item.Name, 33),
			turns,
			flags,
		)
	case normalizeScreenWidth(width) >= 54:
		return fmt.Sprintf("%-3s %-10s %-25s %-6s %-3s",
			strings.ToUpper(item.Hotkey),
			trimRunes(strings.ToUpper(item.Category), 10),
			trimRunes(item.Name, 25),
			turns,
			flags,
		)
	default:
		return fmt.Sprintf("%-3s %-26s %-6s %-3s",
			strings.ToUpper(item.Hotkey),
			trimRunes(item.Name, 26),
			turns,
			flags,
		)
	}
}

func welcomeWolfArt(width int) []string {
	switch {
	case normalizeScreenWidth(width) >= 72:
		return []string{
			"                           .     .",
			"                          / \\.-./ \\",
			"                         / /\\_ _/\\\\ \\",
			"                         |/  o o  \\|",
			"                         ( == ^ == )",
			"                          )  ---  (",
			"                         /         \\",
		}
	case normalizeScreenWidth(width) >= 54:
		return []string{
			"                       /\\_/\\\\",
			"                      ( o.o )",
			"                       > ^ <",
			"                    WolfBBS Caller",
		}
	default:
		return []string{
			"                    /\\_/\\\\",
			"                   ( o.o )",
			"                    > ^ <",
		}
	}
}
