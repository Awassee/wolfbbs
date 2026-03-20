package ui

import (
	"strings"
	"testing"
	"time"
)

func TestRenderTopBarWithClock12Hour(t *testing.T) {
	now := time.Date(2026, 2, 24, 21, 5, 0, 0, time.UTC)
	line := RenderTopBarWithClock(80, "WolfBBS", "user", now, "Node 1", DefaultTheme(), false)
	if !strings.Contains(line, "09:05 PM") {
		t.Fatalf("expected 12h clock in top bar, got %q", line)
	}
}

func TestRenderTopBarCompressesForNarrowWidths(t *testing.T) {
	now := time.Date(2026, 3, 19, 21, 5, 0, 0, time.UTC)
	narrow := stripANSIEscapes(RenderTopBarWithClock(34, "WolfBBS Showcase", "retrocaller", now, "Node 12", DefaultTheme(), true))
	if !strings.Contains(narrow, "21:05") {
		t.Fatalf("expected short clock in narrow top bar, got %q", narrow)
	}
	if strings.Contains(narrow, "User:") {
		t.Fatalf("expected compressed narrow top bar without verbose label, got %q", narrow)
	}
	if got := runeLen(narrow); got != 34 {
		t.Fatalf("expected narrow top bar width 34, got %d (%q)", got, narrow)
	}

	medium := stripANSIEscapes(RenderTopBarWithClock(52, "WolfBBS Showcase", "retrocaller", now, "Node 12", DefaultTheme(), true))
	if !strings.Contains(medium, "Node") {
		t.Fatalf("expected node label in medium top bar, got %q", medium)
	}
	if strings.Contains(medium, "2006-03-19 21:05") {
		t.Fatalf("expected compressed medium clock, got %q", medium)
	}
	if got := runeLen(medium); got != 52 {
		t.Fatalf("expected medium top bar width 52, got %d (%q)", got, medium)
	}
}

func TestRenderHelpPanels(t *testing.T) {
	cases := []struct {
		name     string
		rendered string
		wants    []string
	}{
		{
			name:     "main",
			rendered: RenderMainMenuHelp(80, "Custom menu hint"),
			wants:    []string{"Help: Main Menu", "Main Menu Key Guide", "Menu note:", "Custom menu hint", "Press any key to return."},
		},
		{
			name:     "boards",
			rendered: RenderBoardsHelp(80),
			wants:    []string{"Help: Message Boards", "Boards Navigation", "Reader keys:", "Press any key to return."},
		},
		{
			name:     "mail",
			rendered: RenderMailHelp(80),
			wants:    []string{"Help: Private Mail", "Private Mail Commands", "Saved replies", "Compose details:", "Press any key to return."},
		},
		{
			name:     "chat",
			rendered: RenderChatHelp(80),
			wants:    []string{"Help: Live Chat", "Live Chat Commands", "Default room is #lobby", "Press any key to return."},
		},
		{
			name:     "gateway",
			rendered: RenderGatewayHelp(80),
			wants:    []string{"Help: Internet Tools", "Internet Tools Commands", "Web browser", "Feed reader", "JSON explorer", "My activity", "AI assistant", "Press any key to return."},
		},
		{
			name:     "files",
			rendered: RenderFilesHelp(80),
			wants:    []string{"Help: Files & Downloads", "Files & Downloads Commands", "newer than your last login", "/collections", "/offline", "Press any key to return."},
		},
		{
			name:     "login",
			rendered: RenderLoginHelp(80, true),
			wants:    []string{"Help: Login", "Login Screen Commands", "Type GUEST", "Type RESET", "Press any key to return."},
		},
		{
			name:     "doors",
			rendered: RenderDoorsHelp(80),
			wants:    []string{"Help: Games & Doors", "Games & Doors Commands", "toggle favorite", "Press any key to return."},
		},
		{
			name:     "settings",
			rendered: RenderSettingsHelp(80),
			wants:    []string{"Help: Settings", "My Settings Commands", "profile export JSON", "save preferences", "Press any key to return."},
		},
	}

	for _, tc := range cases {
		for _, want := range tc.wants {
			if !strings.Contains(tc.rendered, want) {
				t.Fatalf("%s help panel missing %q", tc.name, want)
			}
		}
	}
}

func TestRenderBoardAndMailMenus(t *testing.T) {
	board := RenderBoardMessageIndex(80, "General", []string{"   1    First message                    01-01 12:00"})
	for _, want := range []string{"General", "First message", "Commands: (N)ew, (R)ead, (S)earch, (Q)uit board, (?)help"} {
		if !strings.Contains(board, want) {
			t.Fatalf("board menu missing %q", want)
		}
	}

	mail := RenderMailOverview(80, []string{"   1  Hello                 01-01 12:00  new"}, []string{"   2  Re: Hello             uid:1"})
	for _, want := range []string{"Private Mail", "Inbox:", "Outbox:", "(T)emplates", "Write personal notes", "Find people"} {
		if !strings.Contains(mail, want) {
			t.Fatalf("mail menu missing %q", want)
		}
	}

	files := RenderFilesMenu(80, []string{"  1 Uploads           /bbs/files                    Default area"})
	for _, want := range []string{"Files & Downloads", "[R]ecent files", "[N]ew since last call", "[C]ollections", "[O]ffline packets", "Open a file area", "Uploads"} {
		if !strings.Contains(files, want) {
			t.Fatalf("files menu missing %q", want)
		}
	}

	gateway := RenderGatewayMenu(80)
	for _, want := range []string{"Internet Tools", "[W]eb browser", "[F]eed reader", "[S]ummarizer", "[J]SON explorer", "[X] My activity", "[A]I assistant"} {
		if !strings.Contains(gateway, want) {
			t.Fatalf("gateway menu missing %q", want)
		}
	}

	doorMenu := RenderDoorMenu(80, []DoorMenuItem{
		{Hotkey: "D", Name: "Dragon Tavern Legends", Category: "rpg", TurnsRemaining: 3, Favorite: true},
	}, []string{"DRAGON-TAVERN-LEGENDS"}, []string{"SPACE-TRADER-WARS"}, DoorMenuSummary{
		Total:         12,
		Visible:       1,
		Category:      "rpg",
		FavoritesOnly: true,
		RecentOnly:    false,
		Spotlight:     "Dragon Tavern Legends [D] • 3 turns",
	})
	for _, want := range []string{"Games & Doors", "Favorites only", "Category=RPG", "Spotlight:", "Dragon Tavern Legends", "Favorite Toggle"} {
		if !strings.Contains(doorMenu, want) {
			t.Fatalf("door menu missing %q", want)
		}
	}
}

func TestRenderMainMenuAndQuickJumpDeck(t *testing.T) {
	menu := stripANSIEscapes(RenderMainMenu(80))
	for _, want := range []string{
		"Main Menu",
		"Start Here",
		"Talk + Read",
		"Track + Return",
		"Personal + System",
		"[C] Chat Rooms",
		"[O] Offline Packets",
		"[V] Showcase Tour",
		"Popular Places",
		"collections",
		"challenges",
		"digest-prefs",
	} {
		if !strings.Contains(menu, want) {
			t.Fatalf("main menu missing %q", want)
		}
	}

	jump := RenderQuickJumpGuide(80, true)
	for _, want := range []string{
		"Quick Jump Deck",
		"Talk + Read",
		"chat/rooms",
		"collections",
		"showcase/tour",
		"app-upgrade",
		"Feature or place",
	} {
		if !strings.Contains(jump, want) {
			t.Fatalf("quick jump deck missing %q", want)
		}
	}
}

func TestRenderChatDesk(t *testing.T) {
	rendered := RenderChatDesk(80, "#lobby", "Main lobby for general chat, greetings, and quick social check-ins.", 3, 2, false, []string{
		"1) #lobby       current | 2 live      sysop: hello",
		"2) #ansi        joined | now         caller: working on ansi art",
	}, []string{
		"15:04   sysop      hello world",
		"15:05   caller     ansi forever",
	})
	for _, want := range []string{
		"Live Chat",
		"Chat Command Bar",
		"[1-9] Switch room",
		"Current room: #lobby",
		"Room guide:",
		"Open Rooms",
		"Transcript",
		"#ansi",
		"Use J to open another room",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("chat desk missing %q", want)
		}
	}
}

func TestRenderConfiguredMainMenuResponsive(t *testing.T) {
	menu := stripANSIEscapes(RenderConfiguredMainMenu(80, "Custom Ops", "Operator shortcuts from menu file", []ActionMenuEntry{
		{Key: "M", Label: "Message Boards", Target: "boards.open"},
		{Key: "X", Label: "Config Center", Target: "system.config_center"},
		{Key: "U", Label: "Upgrade App", Target: "system.app_upgrade"},
	}))
	for _, want := range []string{"Custom Ops", "Talk + Read", "Public messages, personal mail", "Personal + System", "Custom Command Deck", "Operator shortcuts", "[M] Read Boards", "system.app_upgrade"} {
		if !strings.Contains(menu, want) {
			t.Fatalf("configured menu missing %q", want)
		}
	}

	narrow := RenderConfiguredMainMenu(32, "Ops", "Narrow terminal should still wrap details cleanly", []ActionMenuEntry{
		{Key: "A", Label: "Admin", Target: "admin.open"},
		{Key: "W", Label: "Who Online", Target: "system.who_online"},
	})
	plain := stripANSIEscapes(narrow)
	lines := strings.Split(strings.TrimSuffix(strings.ReplaceAll(plain, "\r\n", "\n"), "\n"), "\n")
	for _, line := range lines {
		if got := runeLen(line); got > 32 {
			t.Fatalf("configured narrow line width %d exceeds 32: %q", got, line)
		}
	}
}

func TestRenderSinceLastCallAndGuestTour(t *testing.T) {
	since := time.Date(2026, 2, 27, 10, 30, 0, 0, time.UTC)
	digest := RenderSinceLastCall(80, []string{"new in General: hello world"}, since)
	for _, want := range []string{"Newscan Digest", "Since your last call:", "new in General"} {
		if !strings.Contains(digest, want) {
			t.Fatalf("since-last-call missing %q", want)
		}
	}

	tour := RenderGuestTour(80, []string{"Last callers: alpha, beta", "Featured thread: Build notes"})
	for _, want := range []string{"Guest Tour", "Read-only guided tour", "Featured thread"} {
		if !strings.Contains(tour, want) {
			t.Fatalf("guest tour missing %q", want)
		}
	}
}

func TestRenderLoginPromptWithGuestToggle(t *testing.T) {
	withGuest := RenderLoginPromptWithGuest(80, true)
	if !strings.Contains(withGuest, "Type GUEST for a read-only guided tour.") {
		t.Fatalf("expected guest tour hint when enabled")
	}
	if !strings.Contains(withGuest, "Type RESET for password reset.") {
		t.Fatalf("expected password reset hint when enabled")
	}
	withoutGuest := RenderLoginPromptWithGuest(80, false)
	if strings.Contains(withoutGuest, "Type GUEST for a read-only guided tour.") {
		t.Fatalf("expected no guest tour hint when disabled")
	}
	if !strings.Contains(withoutGuest, "Type RESET for password reset.") {
		t.Fatalf("expected password reset hint when guest tour disabled")
	}
}

func TestRenderCompactWelcomeAndLoginPrompt(t *testing.T) {
	welcome := RenderWelcomeForProfile(72, true, []string{"Plain terminal fallback active.", "Short-page mode enabled for low terminal height."})
	for _, want := range []string{"Compact session profile active.", "Plain terminal fallback active.", "Short-page mode enabled for low terminal height."} {
		if !strings.Contains(welcome, want) {
			t.Fatalf("compact welcome missing %q", want)
		}
	}

	login := RenderLoginPromptProfile(72, true, true)
	for _, want := range []string{"Compact session mode keeps screens shorter on this terminal.", "Type GUEST for a read-only guided tour.", "Type RESET for password reset."} {
		if !strings.Contains(login, want) {
			t.Fatalf("compact login missing %q", want)
		}
	}
}

func TestRenderWelcomeShowsWolfAndCopyright(t *testing.T) {
	rendered := RenderWelcome(80)
	for _, want := range []string{
		"WolfBBS Welcome",
		"wolfbbs (c) 2026",
		"Wildcat-era glow, modern rails, node-ready ANSI.",
		"Press ESC to quit, any other key to continue.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("welcome screen missing %q", want)
		}
	}
}

func TestResponsiveScreensFitCommonWidths(t *testing.T) {
	cases := []struct {
		name     string
		width    int
		rendered string
	}{
		{name: "main-32", width: 32, rendered: RenderMainMenu(32)},
		{name: "welcome-40", width: 40, rendered: RenderWelcome(40)},
		{name: "welcome-72", width: 72, rendered: RenderWelcome(72)},
		{name: "main-40", width: 40, rendered: RenderMainMenu(40)},
		{name: "chat-54", width: 54, rendered: RenderChatDesk(54, "#lobby", "Main lobby for general chat.", 2, 1, false, []string{"1) #lobby current | 1 live sysop: hello", "2) #ansi joined | now caller: art drop"}, []string{"15:04 sysop hello there", "15:05 caller ansi forever"})},
		{name: "jump-40", width: 40, rendered: RenderQuickJumpGuide(40, false)},
		{name: "files-32", width: 32, rendered: RenderFilesMenu(32, []string{"  1 Uploads   /bbs/files"})},
		{name: "mail-54", width: 54, rendered: RenderMailOverview(54, []string{"  1  Hello there         01-01 12:00  new"}, []string{"  2  Re: Hello           uid:1"})},
		{name: "files-54", width: 54, rendered: RenderFilesMenu(54, []string{"  1 Uploads           /bbs/files            Default area"})},
		{name: "doors-54", width: 54, rendered: RenderDoorMenu(54, []DoorMenuItem{{Hotkey: "D", Name: "Dragon Tavern Legends", Category: "rpg", TurnsRemaining: 3, Favorite: true}}, []string{"DRAGON"}, []string{"SPACE"}, DoorMenuSummary{Total: 12, Visible: 1, Category: "rpg", FavoritesOnly: true, Spotlight: "Dragon Tavern Legends [D]"})},
		{name: "who-40", width: 40, rendered: RenderWhoOnline(40, []string{"01  sysop        LAN   Main Menu    00:00:08"})},
		{name: "last-40", width: 40, rendered: RenderLastCallers(40, []string{"01  sysop        WAN   Boards       00:12:11"})},
	}
	for _, tc := range cases {
		plain := stripANSIEscapes(tc.rendered)
		lines := strings.Split(strings.TrimSuffix(strings.ReplaceAll(plain, "\r\n", "\n"), "\n"), "\n")
		for _, line := range lines {
			if got := runeLen(line); got > tc.width {
				t.Fatalf("%s line width %d exceeds %d: %q", tc.name, got, tc.width, line)
			}
		}
	}
}

func TestResponsiveScreensMapCleanlyToASCII(t *testing.T) {
	rendered := RenderMainMenu(32) + RenderFilesMenu(32, []string{"  1 Uploads           /bbs/files            Default area"})
	plain := ApplyOutputProfile(rendered, false, "ascii")
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("expected ascii output without ANSI escapes")
	}
	for _, bad := range []string{"╔", "╗", "╚", "╝", "═", "║"} {
		if strings.Contains(plain, bad) {
			t.Fatalf("expected no unicode box drawing in ascii output: %q", plain)
		}
	}
	for _, want := range []string{"Main Menu", "Files & Downloads", "Find a Feature", "Uploads", "[ID] Open area", "[Q] Return"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("ascii output missing %q", want)
		}
	}
}

func TestRenderCallerPanelsIncludeOriginColumns(t *testing.T) {
	online := RenderWhoOnline(80, []string{"01  sysop        02-28 19:30      LAN   192.168.1.20    Main Menu    00:00:08"})
	for _, want := range []string{"Who's Online", "Orig", "From", "192.168.1.20"} {
		if !strings.Contains(online, want) {
			t.Fatalf("who online missing %q", want)
		}
	}

	last := RenderLastCallers(80, []string{"01  sysop        02-28 19:28      WAN   203.0.113.5     Boards       00:12:11"})
	for _, want := range []string{"Last Callers", "Orig", "From", "203.0.113.5"} {
		if !strings.Contains(last, want) {
			t.Fatalf("last callers missing %q", want)
		}
	}
}
