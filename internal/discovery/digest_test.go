package discovery

import (
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

func TestBuildSinceLastCall(t *testing.T) {
	users := repository.NewInMemoryUserRepository()
	boards := repository.NewInMemoryBoardRepository()
	msgs := repository.NewInMemoryMessageRepository()
	mail := repository.NewInMemoryPrivateMailRepository()

	now := time.Now().UTC()
	user := &domain.User{
		Handle:       "captain",
		PasswordHash: "x",
		Enabled:      true,
	}
	if err := users.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	last := now.Add(-2 * time.Hour)
	user.LastLoginAt = &last
	if err := users.Update(user); err != nil {
		t.Fatalf("update user: %v", err)
	}

	if err := boards.Create(&domain.Board{Name: "General", Description: "Main", CreatedBy: user.ID}); err != nil {
		t.Fatalf("create board: %v", err)
	}

	parent := &domain.Message{
		BoardID:   1,
		AuthorID:  user.ID,
		Subject:   "Original post",
		Body:      "plain body",
		CreatedAt: now.Add(-3 * time.Hour),
	}
	if err := msgs.CreateMessage(parent); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	reply := &domain.Message{
		BoardID:   1,
		AuthorID:  999,
		ParentID:  parent.ID,
		Subject:   "Re: Original",
		Body:      "Hey captain check this",
		CreatedAt: now.Add(-1 * time.Hour),
	}
	if err := msgs.CreateMessage(reply); err != nil {
		t.Fatalf("create reply: %v", err)
	}
	mention := &domain.Message{
		BoardID:   1,
		AuthorID:  1000,
		Subject:   "Ping",
		Body:      "captain please review",
		CreatedAt: now.Add(-50 * time.Minute),
	}
	if err := msgs.CreateMessage(mention); err != nil {
		t.Fatalf("create mention: %v", err)
	}

	if err := mail.CreateMail(&domain.PrivateMail{
		FromUserID: 500,
		ToUserID:   user.ID,
		Subject:    "mail notice",
		Body:       "read this",
		CreatedAt:  now.Add(-40 * time.Minute),
	}); err != nil {
		t.Fatalf("create mail: %v", err)
	}

	result, err := BuildSinceLastCall(boards, msgs, mail, user, 12)
	if err != nil {
		t.Fatalf("build digest: %v", err)
	}
	if len(result.Items) == 0 {
		t.Fatal("expected digest items")
	}

	joined := strings.ToLower(result.Items[0].Line)
	foundReply := false
	foundMention := false
	foundMail := false
	_ = joined
	for _, item := range result.Items {
		switch item.Kind {
		case "reply":
			foundReply = true
		case "mention":
			foundMention = true
		case "mail":
			foundMail = true
		}
	}
	if !foundReply || !foundMention || !foundMail {
		t.Fatalf("expected reply+mention+mail kinds, got: %+v", result.Items)
	}

	ai := BuildAICatchUpLine(result.Items)
	if !strings.Contains(ai, "[AI-LABEL]") {
		t.Fatalf("expected ai label in summary, got %q", ai)
	}
}
