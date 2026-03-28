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
	return RenderWelcomeForProfile(width, false, nil)
}

func RenderWelcomeForProfile(width int, compact bool, hints []string) string {
	width = normalizeScreenWidth(width)
	if compact {
		lines := []string{
			"wolfbbs (c) 2026",
			"Compact session profile active.",
			"Using shorter, safer output for this terminal.",
		}
		for _, hint := range hints {
			trimmed := strings.TrimSpace(hint)
			if trimmed == "" {
				continue
			}
			lines = append(lines, "- "+trimmed)
		}
		lines = append(lines, "Press ESC to quit, any other key to continue.")
		panel := renderPanel(width, "WolfBBS Welcome", lines, FgYellow)
		var b strings.Builder
		for _, line := range strings.Split(strings.TrimSuffix(panel, "\r\n"), "\r\n") {
			b.WriteString(FgYellow)
			b.WriteString(line)
			b.WriteString(Reset)
			b.WriteString("\r\n")
		}
		return b.String()
	}
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
		"Pick the job you want to do. Help is always one key away.",
		"",
		sectionLabel("Start Here"),
	}
	lines = append(lines, commandStripLines(width, []string{
		"[N] What's New",
		"[M] Read Boards",
		"[C] Chat Rooms",
		"[P] Private Mail",
	})...)
	lines = append(lines, "",
		sectionLabel("Talk + Read"),
		"Public messages, personal mail, live rooms, downloads, games, and internet tools.",
	)
	lines = append(lines, renderMenuGrid(width, []menuEntry{
		{Key: "M", Label: "Read Boards"},
		{Key: "P", Label: "Private Mail"},
		{Key: "C", Label: "Chat Rooms"},
		{Key: "F", Label: "Files & Downloads"},
		{Key: "D", Label: "Games & Doors"},
		{Key: "G", Label: "Internet Tools"},
	}, 3)...)
	lines = append(lines, "", sectionLabel("Track + Return"), "Catch up, see people, save packets for later, and revisit highlights.")
	lines = append(lines, renderMenuGrid(width, []menuEntry{
		{Key: "N", Label: "What's New"},
		{Key: "R", Label: "My Activity"},
		{Key: "L", Label: "Recent Callers"},
		{Key: "W", Label: "Who's Here Now"},
		{Key: "O", Label: "Offline Packets"},
		{Key: "V", Label: "Showcase Tour"},
	}, 3)...)
	lines = append(lines, "", sectionLabel("Personal + System"), "Adjust your experience, inspect board info, find hidden features, or sign off.")
	lines = append(lines, renderMenuGrid(width, []menuEntry{
		{Key: "S", Label: "My Settings"},
		{Key: "X", Label: "Board Info"},
		{Key: "Y", Label: "System Status"},
		{Key: "/", Label: "Find a Feature"},
		{Key: "A", Label: "Sysop Center"},
		{Key: "Q", Label: "Sign Off"},
	}, 3)...)
	lines = append(lines, "", sectionLabel("Popular Places"))
	lines = append(lines, commandStripLines(width, []string{
		"collections",
		"bookmarks",
		"circles",
		"events",
		"challenges",
		"digest-prefs",
	})...)
	lines = append(lines, "", sectionLabel("Global Shortcuts"))
	lines = append(lines, commandStripLines(width, []string{
		"Single-letter hotkeys",
		"Esc = Back",
		"? = Help",
		"/ = Find a Feature",
	})...)
	return renderPanel(width, "Main Menu", lines, FgCyan)
}

func RenderConfiguredMainMenu(width int, title, help string, entries []ActionMenuEntry) string {
	menuTitle := strings.TrimSpace(title)
	if menuTitle == "" {
		menuTitle = "Main Menu"
	}
	talk, track, personal, extras := partitionConfiguredMenuEntries(entries)
	lines := []string{}
	if len(talk) > 0 {
		lines = append(lines, sectionLabel("Talk + Read"))
		lines = append(lines, "Public messages, personal mail, live rooms, downloads, games, and internet tools.")
		lines = append(lines, renderMenuGrid(width, talk, 3)...)
	}
	if len(track) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, sectionLabel("Track + Return"))
		lines = append(lines, "Catch up, see people, save packets, and revisit highlights.")
		lines = append(lines, renderMenuGrid(width, track, 3)...)
	}
	if len(personal) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, sectionLabel("Personal + System"))
		lines = append(lines, "Adjust your experience, inspect board info, or sign off.")
		lines = append(lines, renderMenuGrid(width, personal, 3)...)
	}
	if len(extras) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, sectionLabel("Custom Command Deck"))
		lines = append(lines, renderConfiguredMenuRows(width, extras)...)
	}
	if len(entries) == 0 {
		lines = append(lines, sectionLabel("Custom Command Deck"))
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

func partitionConfiguredMenuEntries(entries []ActionMenuEntry) ([]menuEntry, []menuEntry, []menuEntry, []ActionMenuEntry) {
	talk := make([]menuEntry, 0, len(entries))
	track := make([]menuEntry, 0, len(entries))
	personal := make([]menuEntry, 0, len(entries))
	extras := make([]ActionMenuEntry, 0, len(entries))
	for _, entry := range entries {
		menuRow := menuEntry{Key: entry.Key, Label: friendlyLabelForTarget(entry.Target, entry.Label)}
		switch configuredMenuLane(entry.Target) {
		case "talk":
			talk = append(talk, menuRow)
		case "track":
			track = append(track, menuRow)
		case "personal":
			personal = append(personal, menuRow)
		default:
			extras = append(extras, entry)
		}
	}
	return talk, track, personal, extras
}

func configuredMenuLane(target string) string {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "boards.open", "mail.open", "chat.open", "files.open", "doors.open", "gateway.open":
		return "talk"
	case "system.newscan", "pulse.open", "system.last_callers", "system.who_online", "files.offline", "system.showcase":
		return "track"
	case "settings.open", "system.config_center", "system.status_center", "system.quick_jump", "admin.open", "session.quit":
		return "personal"
	default:
		return ""
	}
}

func friendlyLabelForTarget(target, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "boards.open":
		return "Read Boards"
	case "mail.open":
		return "Private Mail"
	case "chat.open":
		return "Chat Rooms"
	case "files.open":
		return "Files & Downloads"
	case "doors.open":
		return "Games & Doors"
	case "gateway.open":
		return "Internet Tools"
	case "system.newscan":
		return "What's New"
	case "pulse.open":
		return "My Activity"
	case "system.last_callers":
		return "Recent Callers"
	case "system.who_online":
		return "Who's Here Now"
	case "files.offline":
		return "Offline Packets"
	case "system.showcase":
		return "Showcase Tour"
	case "settings.open":
		return "My Settings"
	case "system.config_center":
		return "Board Info"
	case "system.status_center":
		return "System Status"
	case "system.quick_jump":
		return "Find a Feature"
	case "admin.open":
		return "Sysop Center"
	case "session.quit":
		return "Sign Off"
	case "system.app_upgrade":
		return "Upgrade App"
	}
	if strings.TrimSpace(fallback) != "" {
		return strings.TrimSpace(fallback)
	}
	return strings.TrimSpace(target)
}

func RenderMainMenuHelp(width int, menuHint string) string {
	lines := []string{
		"Main Menu Key Guide",
		"",
		"If you are new here:",
		"N  Start with what's new since your last visit",
		"M  Read boards        C  Join chat rooms",
		"P  Check private mail Q  Sign off when done",
		"",
		"Talk + Read",
		"M  Read Boards           P  Private Mail",
		"C  Chat Rooms            F  Files & Downloads",
		"D  Games & Doors         G  Internet Tools",
		"",
		"Track + Return",
		"N  What's New            R  My Activity",
		"L  Recent Callers        W  Who's Here Now",
		"O  Offline Packets       V  Showcase Tour",
		"",
		"Personal + System",
		"S  My Settings           X  Board Info",
		"Y  System Status         A  Sysop/Admin",
		"/  Find a Feature prompt",
		"   - Includes collections, offline packets, bookmarks, circles",
		"   - Includes events, recaps, challenges, digest choices, showcase",
		"   - Includes app-upgrade (/app upgrade) for sysop",
		"Q  Quit to sign-off      Esc = Back",
		"?  Show this help panel",
		"",
		"Use single-letter keys. Menus are immediate and case-insensitive.",
		"Find a Feature is the fastest way to reach newer parity screens by name.",
	}
	if hint := strings.TrimSpace(menuHint); hint != "" {
		lines = append(lines, "")
		lines = append(lines, "Menu note:")
		lines = append(lines, hint)
	}
	return renderHelpPanel(width, "Help: Main Menu", lines)
}

func RenderQuickJumpGuide(width int, sysop bool) string {
	lines := []string{
		"Use Find a Feature when you know the job but not the menu key.",
		"",
		sectionLabel("Talk + Read"),
		"boards/messages   read public message boards",
		"mail/private      check private mail",
		"chat/rooms        jump into live chat rooms",
		"files/downloads   browse file areas and tickets",
		"doors/games       play games and utilities",
		"gateway/internet  web, email, feeds, JSON, AI",
		"",
		sectionLabel("Follow-up + Utility"),
		"collections       curated file bundles",
		"offline/packets   save packets or import replies",
		"bookmarks         personal quick links",
		"circles           caller groups and tags",
		"settings          theme, pager, clock, exports",
		"showcase/tour     guided feature tour",
		"statusz/config    board or runtime snapshots",
		"",
		sectionLabel("Pulse + Community"),
		"pulse/activity    streaks, missions, next actions",
		"events/recaps     scheduled community events",
		"challenges        seasonal challenge board",
		"spotlights        feature highlights",
		"digest-prefs      weekly digest choices",
	}
	if sysop {
		lines = append(lines, "", sectionLabel("Sysop"))
		lines = append(lines, "admin             sysop tools")
		lines = append(lines, "app-upgrade       upgrade this board")
	}
	lines = append(lines, "", "Prompt shown below: Feature or place", "Press Enter on blank input to cancel.")
	return renderPanel(width, "Quick Jump Deck", lines, FgYellow) + "\r\n"
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
	return RenderLoginPromptProfile(width, guestTour, false)
}

func RenderLoginPromptProfile(width int, guestTour, compact bool) string {
	lines := []string{
		"Enter handle and password to continue.",
		"Unknown handle may create a new account after login attempt.",
		"Passwords are never stored in plaintext.",
	}
	if compact {
		lines = []string{
			"Compact session mode keeps screens shorter on this terminal.",
			"Enter handle and password to continue.",
		}
	}
	if guestTour {
		lead := []string{
			"Enter handle and password to continue.",
			"Type GUEST for a read-only guided tour.",
		}
		if compact {
			lead = []string{
				"Compact session mode keeps screens shorter on this terminal.",
				"Type GUEST for a read-only guided tour.",
			}
		}
		lines = append(lead, lines[1:]...)
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

func RenderMailCompose(width int, title, recipient, subject, urgency, seedName, note string) string {
	recipient = strings.TrimSpace(recipient)
	if recipient == "" {
		recipient = "(choose a caller handle or external email)"
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "(write a subject)"
	}
	urgency = strings.TrimSpace(urgency)
	if urgency == "" {
		urgency = "normal"
	}
	lines := []string{
		sectionLabel("Compose Desk"),
		"Mail is a step-by-step desk here: recipient, subject, urgency, then body.",
	}
	if trimmed := strings.TrimSpace(seedName); trimmed != "" {
		lines = append(lines, "Loaded reply kit: "+trimmed)
	}
	if trimmed := strings.TrimSpace(note); trimmed != "" {
		lines = append(lines, trimmed)
	}
	lines = append(lines,
		"",
		"To: "+recipient,
		"Subject: "+subject,
		"Urgency: "+strings.ToUpper(urgency),
		"",
		"Recipient help: type ?name-fragment at the To prompt to look up handles.",
		"Body editor: each line opens with Body>. Use /preview, /del, /help, and . to send.",
	)
	return renderPanel(width, title, lines, FgGreen) + "\r\n"
}

func RenderMailReader(width int, subject string, meta []string, body string) string {
	lines := []string{
		sectionLabel("Mail Header"),
		"Subject: " + strings.TrimSpace(subject),
	}
	lines = append(lines, meta...)
	lines = append(lines, "", sectionLabel("Message"))
	bodyLines := strings.Split(strings.ReplaceAll(strings.TrimSpace(body), "\r\n", "\n"), "\n")
	if len(bodyLines) == 0 || (len(bodyLines) == 1 && strings.TrimSpace(bodyLines[0]) == "") {
		lines = append(lines, "(empty)")
	} else {
		lines = append(lines, bodyLines...)
	}
	lines = append(lines, "", "Reader hotkeys: [P] Reply  [D] Delete  [Q] Back")
	return renderPanel(width, "Private Mail Reader", lines, FgCyan) + "\r\n"
}

func RenderGatewayMenu(width int) string {
	lines := []string{
		sectionLabel("Internet Tools"),
		"[W]eb browser     Read a web page in the pager and save it for later",
		"[E]mail gateway   Send outside email through the configured relay",
		"[F]eed reader     Turn RSS/Atom feeds into short headlines",
		"[S]ummarizer      Pull quick bullets from an article URL",
		"[J]SON explorer   Inspect JSON API responses in a readable view",
		"[X] My activity   Open your activity, events, and challenge center",
		"[A]I assistant    Ask the configured AI helper a question",
	}
	lines = append(lines, commandStripLines(width, []string{"[R]eturn", "[Q]uit", "[?] Help"})...)
	return renderPanel(width, "Internet Tools", lines, FgCyan) + "\r\n"
}

func RenderMailOverview(width int, inboxRows []string, outboxRows []string) string {
	lines := []string{
		sectionLabel("Mail Command Bar"),
	}
	lines = append(lines, commandStripLines(width, []string{"[C] Write mail", "[T] Saved replies", "[R] Read", "Re[P]ly", "[D] Delete", "[H] Find people", "[Q] Return", "[?] Help"})...)
	lines = append(lines, "Write personal notes, reuse saved replies, or look up a caller by handle.", "Press one hotkey now. Read/Reply/Delete will ask for a message number next.", "")
	lines = append(lines, sectionLabel("Inbox:"))
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
	lines = append(lines, "", "Hotkeys: (C)ompose, (T)emplates, (R)ead, Re(P)ly, (D)elete, (H)andles, (Q)uit, (?)help", "Press a hotkey:")
	return renderPanel(width, "Private Mail", lines, FgCyan) + "\r\n"
}

func RenderGatewayHelp(width int) string {
	lines := []string{
		"Internet Tools Commands",
		"",
		"W  Web browser",
		"   - Fetches a web page with timeout, size caps, and SSRF blocks",
		"   - Displays text in the ANSI pager",
		"   - Optional offline save per user",
		"",
		"E  Email gateway",
		"   - Sends through the configured SMTP relay",
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
		"X  My activity",
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
	return renderHelpPanel(width, "Help: Internet Tools", lines)
}

func RenderFilesMenu(width int, areas []string) string {
	lines := []string{
		sectionLabel("Files & Downloads"),
	}
	lines = append(lines, commandStripLines(width, []string{"[ID] Open area", "[R]ecent files", "[N]ew since last call", "[S]earch", "[I]ndexed search", "[D]ownload queue", "[C]ollections", "[O]ffline packets", "[Q] Return", "[?] Help"})...)
	lines = append(lines, "Open a file area or jump straight into collections, tickets, indexed search, and offline packets.", "", filesHeader(width), filesDivider(width))
	if len(areas) == 0 {
		lines = append(lines, "No file areas configured yet.")
	} else {
		lines = append(lines, areas...)
	}
	lines = append(lines, "")
	lines = append(lines, "Classic file areas with modern search, tickets, and packet tools.")
	return renderPanel(width, "Files & Downloads", lines, FgCyan) + "\r\n"
}

func RenderFilesHelp(width int) string {
	lines := []string{
		"Files & Downloads Commands",
		"",
		"From the files menu:",
		"- Enter area ID to browse that area",
		"- R lists recent files across all areas",
		"- N lists files newer than your last login",
		"- S searches filenames across all areas",
		"- I uses indexed FileBase search and queue add by file ID",
		"- D manages your download queue and one-time tickets",
		"- C opens curated file bundles for /collections",
		"- O opens offline packet export/import for /offline",
		"- Q or Esc returns to Main Menu",
		"",
		"Inside an area:",
		"- S updates filename filter",
		"- R refreshes listing",
		"- Q exits to area list",
	}
	return renderHelpPanel(width, "Help: Files & Downloads", lines)
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
		sectionLabel("Games & Doors"),
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
		return renderPanel(width, "Games & Doors", lines, FgYellow) + "\r\n"
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
	return renderPanel(width, "Games & Doors", lines, FgYellow) + "\r\n"
}

func RenderDoorsHelp(width int) string {
	lines := []string{
		"Games & Doors Commands",
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
	return renderHelpPanel(width, "Help: Games & Doors", lines)
}

func RenderMailHelp(width int) string {
	lines := []string{
		"Private Mail Commands",
		"",
		"Hotkeys act immediately; Enter is only needed after numbered prompts.",
		"",
		"C  Compose message",
		"T  Saved replies",
		"R  Read message by ID",
		"P  Reply to message by ID",
		"D  Delete message by ID",
		"H  Find recipient handles",
		"Q  Return to Main Menu",
		"",
		"Compose details:",
		"- Recipient can be local handle or external email",
		"- Saved replies preload reusable subject/body patterns",
		"- Type ?prefix in recipient prompt to search handles",
		"- Body entry shows a Body> prompt for each line",
		"- Finish by typing a single period on its own line",
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
		"Type a message and press Enter to send it to the current room.",
		"",
		"/join #room     join or open a room and switch to it",
		"/switch 2       move to a visible room slot or /switch #room",
		"/list           show the visible room windows",
		"/names          show who is in the current room",
		"/whois nick     show node and idle details for one name",
		"/part           leave the current room",
		"/refresh        redraw the client",
		"/help           open this help screen",
		"/quit           return to Main Menu",
		"",
		"Legacy shortcuts still work: 1-9, J, L, N, O, R, Q, ?",
		"",
		"Notes:",
		"- Default room is #lobby and it stays available as a safe fallback",
		"- Joining another room does not drop your current set",
		"- Locked rooms show read-only state for non-moderators",
		"- Messages persist and are shared with web and IRC clients",
	}
	return renderHelpPanel(width, "Help: Live Chat", lines)
}

func RenderChatDesk(width int, nick, current, topic string, joinedCount, onlineCount int, locked bool, notice string, channelRows, transcriptRows, rosterRows []string) string {
	lockState := "open"
	if locked {
		lockState = "locked"
	}
	nick = strings.TrimSpace(nick)
	if nick == "" {
		nick = "caller"
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = "Shared live room for web, SSH, and IRC callers."
	}
	lines := []string{
		"IRC-style chat desk with room windows, live buffer, and user list.",
	}
	lines = append(lines,
		"",
		fmt.Sprintf("Nick: %s   Channel: %s   Joined: %d   Here: %d   Mode: %s", nick, current, joinedCount, onlineCount, lockState),
		"Topic: "+topic,
	)
	if trimmed := strings.TrimSpace(notice); trimmed != "" {
		lines = append(lines, "Status: "+trimmed)
	}
	if normalizeScreenWidth(width) >= 72 {
		lines = append(lines, "")
		lines = append(lines, renderChatColumns(width, channelRows, transcriptRows, rosterRows)...)
	} else {
		lines = append(lines, "", sectionLabel("Windows"))
		if len(channelRows) == 0 {
			lines = append(lines, "No rooms available yet.")
		} else {
			lines = append(lines, channelRows...)
		}
		lines = append(lines, "", sectionLabel("Buffer"))
		if len(transcriptRows) == 0 {
			lines = append(lines, "No messages yet.")
		} else {
			lines = append(lines, transcriptRows...)
		}
		lines = append(lines, "", sectionLabel("Users"))
		if len(rosterRows) == 0 {
			lines = append(lines, "(nobody listed yet)")
		} else {
			lines = append(lines, rosterRows...)
		}
	}
	prompt := nick + "@" + current + ">"
	if locked {
		prompt = nick + "@" + current + " (read-only)>"
	}
	lines = append(lines, "", "Prompt: "+prompt, "Commands: /join #room  /switch 2  /list  /names  /whois nick  /topic  /part  /refresh  /help  /quit")
	return renderPanel(width, "Live Chat Client", lines, FgCyan) + "\r\n"
}

func renderChatColumns(width int, channelRows, transcriptRows, rosterRows []string) []string {
	innerWidth := normalizeScreenWidth(width) - 2
	channelWidth := 22
	rosterWidth := 18
	if innerWidth < 66 {
		channelWidth = 18
		rosterWidth = 14
	}
	transcriptWidth := innerWidth - channelWidth - rosterWidth - 4
	if transcriptWidth < 18 {
		transcriptWidth = 18
		channelWidth = ircMax(16, innerWidth-transcriptWidth-rosterWidth-4)
	}
	lines := []string{
		padOrTrim("Windows", channelWidth, " ") + "  " + padOrTrim("Buffer", transcriptWidth, " ") + "  " + padOrTrim("Users", rosterWidth, " "),
		padOrTrim(strings.Repeat("-", channelWidth), channelWidth, " ") + "  " + padOrTrim(strings.Repeat("-", transcriptWidth), transcriptWidth, " ") + "  " + padOrTrim(strings.Repeat("-", rosterWidth), rosterWidth, " "),
	}
	maxRows := ircMax(len(channelRows), ircMax(len(transcriptRows), len(rosterRows)))
	if maxRows == 0 {
		maxRows = 1
	}
	for i := 0; i < maxRows; i++ {
		left := ""
		if i < len(channelRows) {
			left = trimANSIVisible(channelRows[i], channelWidth)
		}
		center := ""
		if i < len(transcriptRows) {
			center = trimANSIVisible(transcriptRows[i], transcriptWidth)
		}
		right := ""
		if i < len(rosterRows) {
			right = trimANSIVisible(rosterRows[i], rosterWidth)
		}
		lines = append(lines, padOrTrim(left, channelWidth, " ")+"  "+padOrTrim(center, transcriptWidth, " ")+"  "+padOrTrim(right, rosterWidth, " "))
	}
	return lines
}

func ircMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func RenderSettingsDesk(width int, handle, theme, outputMode string, ansiEnabled, pagingEnabled, timeFormat24h, hasTwoFactor bool) string {
	lines := []string{
		"Choose what this call should feel like, then save the parts you want to keep for next time.",
		"",
		sectionLabel("Quick Choices"),
	}
	lines = append(lines, commandStripLines(width, []string{"[W] Change password", "[S] Save my choices", "[Q] Back", "[?] Help"})...)
	lines = append(lines,
		"Handle: "+strings.TrimSpace(handle),
		"Two-step sign-in: "+settingsState(hasTwoFactor, "enabled", "not enabled"),
		"",
		sectionLabel("Look + Feel"),
	)
	lines = append(lines, commandStripLines(width, []string{"[T] Theme", "[A] Color + ANSI", "[U] Output mode"})...)
	lines = append(lines,
		"Theme right now: "+strings.TrimSpace(theme),
		"Color + ANSI: "+settingsState(ansiEnabled, "on", "off"),
		"Output mode for this call: "+strings.TrimSpace(outputMode),
		"",
		sectionLabel("Reading Comfort"),
	)
	lines = append(lines, commandStripLines(width, []string{"[P] Long-screen pause", "[C] 24-hour clock"})...)
	lines = append(lines,
		"Pause on long screens: "+settingsState(pagingEnabled, "on", "off"),
		"Clock style: "+settingsState(timeFormat24h, "24-hour", "12-hour"),
		"",
		sectionLabel("Account Safety"),
	)
	lines = append(lines, commandStripLines(width, []string{"[W] Change password"})...)
	lines = append(lines,
		"Password changes happen here in terminal.",
		"Use Plain text safe mode if the screen ever looks scrambled or frozen.",
		"",
		sectionLabel("Personal Tools"),
	)
	lines = append(lines, commandStripLines(width, []string{"[B] Bookmarks", "[O] Circles", "[X] Profile export", "[E] Attention export"})...)
	lines = append(lines,
		"Bookmarks keep favorite places handy. Circles keep favorite people grouped.",
		"Exports write a clean copy of your profile or attention data for offline use.",
		"",
		"Changes preview live on this screen. Save when you want to keep them for your next call.",
	)
	return renderPanel(width, "My Settings", lines, FgCyan) + "\r\n"
}

func settingsState(enabled bool, onLabel, offLabel string) string {
	if enabled {
		return onLabel
	}
	return offLabel
}

func RenderSettingsHelp(width int) string {
	lines := []string{
		"My Settings Commands",
		"",
		"T  cycle theme",
		"A  toggle ANSI on/off",
		"U  cycle output mode for this session",
		"P  toggle pager on/off",
		"C  toggle 24-hour clock",
		"W  change your password",
		"B  open bookmarks manager",
		"O  open caller circles manager",
		"X  profile export JSON (/profile/export parity)",
		"E  attention export JSON (/attention/export parity)",
		"S  save preferences",
		"Q/Esc return without saving changes",
		"",
		"Theme + ANSI settings apply on next redraw immediately.",
		"Use Plain text safe mode if your terminal feels glitchy or frozen.",
		"Output mode changes this call only until you save and reconnect.",
	}
	return renderHelpPanel(width, "Help: Settings", lines)
}

func RenderCallerPulseDesk(width int, handle string, nextRows, weekRows, seasonRows, supportRows []string) string {
	lines := []string{
		"Caller Pulse is your one-screen check-in: catch up, protect your streak, and see which community loops are active.",
		"",
		"Caller: " + strings.TrimSpace(handle),
		"",
		sectionLabel("Do This Next"),
	}
	if len(nextRows) == 0 {
		lines = append(lines, "Nothing urgent is waiting. Keep your rhythm going.")
	} else {
		lines = append(lines, nextRows...)
	}
	lines = append(lines, "", sectionLabel("This Week"))
	if len(weekRows) == 0 {
		lines = append(lines, "No weekly activity to show yet.")
	} else {
		lines = append(lines, weekRows...)
	}
	lines = append(lines, "", sectionLabel("Season + Events"))
	if len(seasonRows) == 0 {
		lines = append(lines, "No missions, challenges, or events are active right now.")
	} else {
		lines = append(lines, seasonRows...)
	}
	lines = append(lines, "", sectionLabel("Support + Delivery"))
	if len(supportRows) == 0 {
		lines = append(lines, "No extra support settings are configured yet.")
	} else {
		lines = append(lines, supportRows...)
	}
	lines = append(lines, "")
	lines = append(lines, commandStripLines(width, []string{"[R] Full report", "[D] Digest plan", "[Q] Back"})...)
	lines = append(lines, "Press Enter to return, or use one of the highlighted letters.")
	return renderPanel(width, "Caller Pulse", lines, FgCyan) + "\r\n"
}

func RenderOfflineCenterDesk(width int, owner string, boardCount int, boardRows []string) string {
	lines := []string{
		"Take a reading packet with you, work through it offline, then bring mail replies back on your next call.",
		"",
		fmt.Sprintf("Packet owner: %s", strings.TrimSpace(owner)),
		fmt.Sprintf("Boards ready: %d", boardCount),
		"",
		sectionLabel("Packet Preview"),
	}
	if len(boardRows) == 0 {
		lines = append(lines, "No watched or digest boards are ready yet. Follow more boards or build up new traffic first.")
	} else {
		lines = append(lines, boardRows...)
	}
	lines = append(lines, "", sectionLabel("What You Can Do"))
	lines = append(lines, commandStripLines(width, []string{"[J] Save full packet (JSON)", "[T] Save reading packet (text)", "[I] Import mail replies", "[Q] Back"})...)
	lines = append(lines,
		"JSON keeps the packet structured for tools. Text is the easier reading copy.",
		"Import mail replies after you write them offline and bring the JSON payload back.",
	)
	return renderPanel(width, "Offline Center", lines, FgCyan) + "\r\n"
}

func RenderReconnectNotice(width int, area string, disconnectedAt time.Time, duration time.Duration, time24h bool) string {
	area = strings.TrimSpace(area)
	if area == "" {
		area = "Main Menu"
	}
	when := disconnectedAt.Local().Format("2006-01-02 15:04")
	if !time24h {
		when = disconnectedAt.Local().Format("2006-01-02 03:04 PM")
	}
	lines := []string{
		"It looks like your last session ended before you signed off.",
		"",
		"Last place: " + area,
		"Disconnected: " + when,
		"Visit length: " + humanDuration(duration),
		"",
		"If the terminal looked strange before disconnecting, open My Settings",
		"and switch Output mode to Plain text safe mode for this session.",
	}
	return renderHelpPanel(width, "Welcome Back", lines)
}

func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d / time.Hour)
	minutes := int((d % time.Hour) / time.Minute)
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
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
