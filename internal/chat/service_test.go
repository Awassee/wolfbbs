package chat

import (
	"testing"
	"time"
)

func TestHistoryPersistence(t *testing.T) {
	svc := NewServiceForTest()

	first, err := svc.Post("alice", "#lobby", "first hello")
	if err != nil {
		t.Fatalf("post first: %v", err)
	}
	second, err := svc.Post("alice", "lobby", "second hello")
	if err != nil {
		t.Fatalf("post second: %v", err)
	}
	if second.ID <= first.ID {
		t.Fatalf("expected second message id to advance: got %d <= %d", second.ID, first.ID)
	}

	all := svc.History("#lobby", 10)
	if len(all) != 2 {
		t.Fatalf("history count = %d, want 2", len(all))
	}

	after := svc.HistorySince("#lobby", first.ID, 10)
	if len(after) != 1 || after[0].ID != second.ID {
		t.Fatalf("history since returned %+v, want one message with id %d", after, second.ID)
	}

	if messages := svc.History("#lobby", 1); len(messages) != 1 || messages[0].ID != second.ID {
		t.Fatalf("history limit mismatch: got %#v", messages)
	}
}

func TestRateLimit(t *testing.T) {
	svc := NewServiceForTest()

	for i := 0; i < defaultChatRateLimitBurst; i++ {
		if _, err := svc.Post("flooder", "#lobby", "msg"); err != nil {
			t.Fatalf("post %d should pass: %v", i, err)
		}
	}
	if _, err := svc.Post("flooder", "#lobby", "too fast"); err == nil {
		t.Fatalf("expected rate limit on message %d", defaultChatRateLimitBurst+1)
	}
}

func TestSubscribeAndPresence(t *testing.T) {
	svc := NewServiceForTest()
	if _, err := svc.Post("alice", "#lobby", "hello"); err != nil {
		t.Fatalf("post failed: %v", err)
	}
	pres := svc.OnlineInChannel("#lobby")
	if len(pres) != 1 {
		t.Fatalf("presence count = %d, want 1", len(pres))
	}
	if pres[0].Nick != "alice" {
		t.Fatalf("presence nick = %q, want alice", pres[0].Nick)
	}

	stream, closeStream := svc.Subscribe("#lobby", "alice")
	defer closeStream()
	msg, err := svc.Post("alice", "#lobby", "online")
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}

	select {
	case got := <-stream:
		if got.ID != msg.ID {
			t.Fatalf("subscription got id %d, want %d", got.ID, msg.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("subscription timed out")
	}
}

