package repository

import (
	"testing"
	"time"

	"wolfbbs/internal/domain"
)

func TestInMemoryBoardRepository(t *testing.T) {
	repo := NewInMemoryBoardRepository()
	board := &domain.Board{Name: "General", Description: "General discussion", CreatedBy: 1}
	if err := repo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if board.ID == 0 {
		t.Fatalf("expected board id")
	}
	got, err := repo.Get(board.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if got.Name != "General" {
		t.Fatalf("unexpected board name: %q", got.Name)
	}
	list, err := repo.List()
	if err != nil {
		t.Fatalf("list boards: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 board, got %d", len(list))
	}
}

func TestInMemoryPrivateMailRepository(t *testing.T) {
	repo := NewInMemoryPrivateMailRepository()
	row := &domain.PrivateMail{
		FromUserID: 1,
		ToUserID:   2,
		Subject:    "Hello",
		Body:       "World",
	}
	if err := repo.CreateMail(row); err != nil {
		t.Fatalf("create mail: %v", err)
	}
	if row.ID == 0 {
		t.Fatalf("expected mail id")
	}
	inbox, err := repo.ListInbox(2, 10)
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("expected 1 inbox row, got %d", len(inbox))
	}
	if err := repo.MarkRead(row.ID, time.Now().UTC()); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	got, err := repo.GetMail(row.ID)
	if err != nil {
		t.Fatalf("get mail: %v", err)
	}
	if got.ReadAt == nil {
		t.Fatalf("expected read_at to be set")
	}
}

func TestInMemoryAdminRepository(t *testing.T) {
	repo := NewInMemoryAdminRepository()
	if err := repo.AddAudit(&domain.AdminAudit{
		Actor:   "admin",
		Target:  "guest",
		Action:  "ban_user",
		Details: "spam",
	}); err != nil {
		t.Fatalf("add audit: %v", err)
	}
	audit, err := repo.ListAudit(10)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(audit))
	}
	if err := repo.SetMailOutboundPolicy("guest", true); err != nil {
		t.Fatalf("set policy: %v", err)
	}
	policy, err := repo.GetMailOutboundPolicy("guest")
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if !policy.OutboundDisabled {
		t.Fatalf("expected outbound disabled")
	}
}
