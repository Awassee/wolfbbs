package mail_test

import (
	"testing"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/mail"
	"wolfbbs/internal/repository"
)

func TestSendAndInbox(t *testing.T) {
	users := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(users)
	sender, _ := authSvc.Register("alice", "password123")
	recipient, _ := authSvc.Register("bob", "password123")

	svc := mail.NewService(repository.NewInMemoryMailRepository(), users)
	if err := svc.Send(sender.ID, recipient.Handle, "Hi", "Welcome to WolfBBS"); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	inbox, err := svc.Inbox(recipient.ID)
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("expected 1 inbox mail, got %d", len(inbox))
	}
}
