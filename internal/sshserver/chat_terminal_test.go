package sshserver

import (
	"testing"

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
