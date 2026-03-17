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
			wants:    []string{"Help: Private Mail", "Private Mail Commands", "Compose details:", "Press any key to return."},
		},
		{
			name:     "chat",
			rendered: RenderChatHelp(80),
			wants:    []string{"Help: Live Chat", "Live Chat Commands", "Default channel is #lobby", "Press any key to return."},
		},
		{
			name:     "gateway",
			rendered: RenderGatewayHelp(80),
			wants:    []string{"Help: Gateways", "Gateway Commands", "Web gateway", "Press any key to return."},
		},
		{
			name:     "files",
			rendered: RenderFilesHelp(80),
			wants:    []string{"Help: Files", "Files Commands", "newer than your last login", "Press any key to return."},
		},
		{
			name:     "login",
			rendered: RenderLoginHelp(80, true),
			wants:    []string{"Help: Login", "Login Screen Commands", "Type GUEST", "Press any key to return."},
		},
		{
			name:     "doors",
			rendered: RenderDoorsHelp(80),
			wants:    []string{"Help: Doors", "Door Hub Commands", "toggle favorite", "Press any key to return."},
		},
		{
			name:     "settings",
			rendered: RenderSettingsHelp(80),
			wants:    []string{"Help: Settings", "Settings Screen Commands", "save preferences", "Press any key to return."},
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
	for _, want := range []string{"Private Mail", "Inbox:", "Outbox:", "Commands: (C)ompose, (R)ead, Re(P)ly, (D)elete, (Q)uit, (?)help"} {
		if !strings.Contains(mail, want) {
			t.Fatalf("mail menu missing %q", want)
		}
	}

	files := RenderFilesMenu(80, []string{"  1 Uploads           /bbs/files                    Default area"})
	for _, want := range []string{"Files", "[R]ecent files", "[N]ew since last call", "Uploads"} {
		if !strings.Contains(files, want) {
			t.Fatalf("files menu missing %q", want)
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
	for _, want := range []string{"Favorites only", "Category=RPG", "Spotlight:", "Dragon Tavern Legends", "Favorite Toggle"} {
		if !strings.Contains(doorMenu, want) {
			t.Fatalf("door menu missing %q", want)
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
	withoutGuest := RenderLoginPromptWithGuest(80, false)
	if strings.Contains(withoutGuest, "Type GUEST for a read-only guided tour.") {
		t.Fatalf("expected no guest tour hint when disabled")
	}
}

func TestRenderWelcomeShowsWolfAndCopyright(t *testing.T) {
	rendered := RenderWelcome(80)
	for _, want := range []string{
		"WolfBBS Welcome",
		"/\\_/\\",
		"wolfbbs (c) 2026",
		"Press ESC to quit, any other key to continue.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("welcome screen missing %q", want)
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
