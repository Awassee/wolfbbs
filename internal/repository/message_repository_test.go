package repository

import (
	"testing"

	"wolfbbs/internal/domain"
)

func TestInMemoryMessageRepository(t *testing.T) {
	repo := NewInMemoryMessageRepository()
	msg := &domain.Message{BoardID: 1, AuthorID: 1, Subject: "Welcome", Body: "hello"}
	if err := repo.CreateMessage(msg); err != nil {
		t.Fatalf("create message: %v", err)
	}
	if msg.ID == 0 {
		t.Fatalf("expected id")
	}
	reply := &domain.Message{BoardID: 1, AuthorID: 2, ParentID: msg.ID, Subject: "Re: Welcome", Body: "reply"}
	if err := repo.CreateMessage(reply); err != nil {
		t.Fatalf("create reply: %v", err)
	}
	if reply.ThreadID != msg.ID {
		t.Fatalf("reply thread id = %d, want %d", reply.ThreadID, msg.ID)
	}
	out, err := repo.ListByBoard(1)
	if err != nil {
		t.Fatalf("list board: %v", err)
	}
	if len(out) != 2 || out[0].ID != msg.ID || out[1].ID != reply.ID {
		t.Fatalf("unexpected list output: %#v", out)
	}
	_, err = repo.GetMessage(msg.ID)
	if err != nil {
		t.Fatalf("get message: %v", err)
	}
	if _, err := repo.GetPointer(1, 1); err == nil {
		t.Fatalf("expected pointer not found before set")
	}
	if err := repo.SetPointer(1, 1, reply.ID, reply.CreatedAt); err != nil {
		t.Fatalf("set pointer: %v", err)
	}
	ptr, err := repo.GetPointer(1, 1)
	if err != nil {
		t.Fatalf("get pointer: %v", err)
	}
	if ptr.LastReadID != reply.ID {
		t.Fatalf("pointer last read id = %d, want %d", ptr.LastReadID, reply.ID)
	}
}
