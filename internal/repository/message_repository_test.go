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
	out, err := repo.ListByBoard(1)
	if err != nil {
		t.Fatalf("list board: %v", err)
	}
	if len(out) != 1 || out[0].ID != msg.ID {
		t.Fatalf("unexpected list output: %#v", out)
	}
	_, err = repo.GetMessage(msg.ID)
	if err != nil {
		t.Fatalf("get message: %v", err)
	}
}
