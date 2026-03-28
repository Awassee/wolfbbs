package sshserver

import (
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/chat"
)

func TestChatTTYSummariesPrioritizeCurrentAndJoined(t *testing.T) {
	srv := &Server{chatSvc: chat.NewServiceForTest()}
	srv.chatSvc.JoinChannel("caller", "#ansi")
	srv.chatSvc.JoinChannel("friend", "#retro")
	if _, err := srv.chatSvc.Post("friend", "#retro", "retro traffic"); err != nil {
		t.Fatalf("post retro: %v", err)
	}
	if _, err := srv.chatSvc.Post("caller", "#ansi", "ansi traffic"); err != nil {
		t.Fatalf("post ansi: %v", err)
	}

	rows := srv.chatTTYSummaries("caller", "#ansi")
	if len(rows) < 2 {
		t.Fatalf("expected multiple channel summaries, got %+v", rows)
	}
	if rows[0].Name != "#ansi" || !rows[0].Current || !rows[0].Joined {
		t.Fatalf("expected current joined channel first, got %+v", rows[0])
	}
	foundRetro := false
	for _, row := range rows {
		if row.Name == "#retro" {
			foundRetro = true
			if row.OnlineCount == 0 {
				t.Fatalf("expected retro online count, got %+v", row)
			}
		}
	}
	if !foundRetro {
		t.Fatalf("expected retro channel in summaries, got %+v", rows)
	}
}

func TestFirstJoinedChatFallbackPrefersAnotherJoinedChannel(t *testing.T) {
	rows := []ttyChatChannelSummary{
		{Name: "#ansi", Current: true, Joined: true},
		{Name: "#retro", Joined: true},
		{Name: "#lobby", Joined: true},
	}
	if got := firstJoinedChatFallback(rows, "#ansi"); got != "#retro" {
		t.Fatalf("expected #retro fallback, got %q", got)
	}
	if got := firstJoinedChatFallback(nil, "#ansi"); got != "#lobby" {
		t.Fatalf("expected #lobby fallback for empty summaries, got %q", got)
	}
}

func TestChatTTYResolveSwitchTarget(t *testing.T) {
	rows := []ttyChatChannelSummary{
		{Name: "#lobby"},
		{Name: "#ansi"},
	}
	if got := chatTTYResolveSwitchTarget("2", rows); got != "#ansi" {
		t.Fatalf("expected slot 2 to resolve to #ansi, got %q", got)
	}
	if got := chatTTYResolveSwitchTarget("retro", rows); got != "#retro" {
		t.Fatalf("expected channel normalization for room name, got %q", got)
	}
	if got := chatTTYResolveSwitchTarget("9", rows); got != "" {
		t.Fatalf("expected missing slot to return empty, got %q", got)
	}
}

func TestChatTTYRosterNoticeIncludesSelf(t *testing.T) {
	online := []chat.Presence{
		{Nick: "friend"},
		{Nick: "caller"},
	}
	got := chatTTYRosterNotice(online, "caller")
	if got != "Names: caller (you), friend" {
		t.Fatalf("unexpected roster notice %q", got)
	}
}

func TestChatTTYWindowAndWhoisNotices(t *testing.T) {
	windows := chatTTYWindowNotice([]ttyChatChannelSummary{
		{Name: "#lobby", Current: true, Joined: true},
		{Name: "#ansi", Joined: true},
		{Name: "#retro"},
	})
	for _, want := range []string{"Windows:", "1:#lobby*", "2:#ansi+", "3:#retro-"} {
		if !strings.Contains(windows, want) {
			t.Fatalf("window notice missing %q: %q", want, windows)
		}
	}

	whois := chatTTYWhoisNotice([]chat.Presence{{
		Nick:    "caller",
		Node:    "Node 1",
		Area:    "Live Chat",
		IdleSec: 75,
		LoginAt: time.Date(2026, 3, 28, 14, 5, 0, 0, time.UTC),
	}}, "caller")
	for _, want := range []string{"Whois caller:", "Live Chat", "Node 1", "idle 1m"} {
		if !strings.Contains(whois, want) {
			t.Fatalf("whois notice missing %q: %q", want, whois)
		}
	}
}
