package network

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

func seedUser(t *testing.T, svc *auth.Service, handle string) *domain.User {
	t.Helper()
	if _, err := svc.Register(handle, "password123"); err != nil {
		t.Fatalf("register %s: %v", handle, err)
	}
	user, err := svc.GetUser(handle)
	if err != nil {
		t.Fatalf("get user %s: %v", handle, err)
	}
	return user
}

func TestBoardExportImportQWK(t *testing.T) {
	users := repository.NewInMemoryUserRepository()
	boards := repository.NewInMemoryBoardRepository()
	msgs := repository.NewInMemoryMessageRepository()
	mail := repository.NewInMemoryPrivateMailRepository()
	authSvc := auth.NewService(users)

	alice := seedUser(t, authSvc, "alice")
	bob := seedUser(t, authSvc, "bob")
	_ = bob

	boardA := &domain.Board{Name: "General", Description: "General Board", CreatedBy: alice.ID}
	if err := boards.Create(boardA); err != nil {
		t.Fatalf("create board A: %v", err)
	}
	if err := msgs.CreateMessage(&domain.Message{
		BoardID:  boardA.ID,
		AuthorID: alice.ID,
		Subject:  "Original Subject",
		Body:     "Original Body",
	}); err != nil {
		t.Fatalf("create source message: %v", err)
	}

	spool := t.TempDir()
	svc := NewService(spool, boards, msgs, users, mail)
	path, err := svc.ExportBoard(FormatQWK, boardA.ID)
	if err != nil {
		t.Fatalf("export board: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected exported packet at %s: %v", path, err)
	}

	imported, err := svc.ImportPacket(path, 0, alice.ID)
	if err != nil {
		t.Fatalf("import packet: %v", err)
	}
	if imported != 1 {
		t.Fatalf("expected 1 imported message, got %d", imported)
	}
	rows, err := msgs.ListByBoard(boardA.ID)
	if err != nil {
		t.Fatalf("list imported board: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows in source board after import, got %d", len(rows))
	}
	if rows[1].Subject != "Original Subject" {
		t.Fatalf("expected imported subject to match, got %q", rows[1].Subject)
	}
}

func TestNetmailQueueAndImport(t *testing.T) {
	users := repository.NewInMemoryUserRepository()
	boards := repository.NewInMemoryBoardRepository()
	msgs := repository.NewInMemoryMessageRepository()
	mail := repository.NewInMemoryPrivateMailRepository()
	authSvc := auth.NewService(users)

	alice := seedUser(t, authSvc, "alice")
	bob := seedUser(t, authSvc, "bob")

	spool := t.TempDir()
	svc := NewService(spool, boards, msgs, users, mail)
	outboundPath, err := svc.QueueNetmail(alice.ID, bob.Handle, "Netmail Subject", "Netmail Body")
	if err != nil {
		t.Fatalf("queue netmail: %v", err)
	}
	packetName := filepath.Base(outboundPath)
	inboundDir := filepath.Join(spool, "inbound", FormatNetmail)
	if err := os.MkdirAll(inboundDir, 0o755); err != nil {
		t.Fatalf("mkdir inbound: %v", err)
	}
	inboundPath := filepath.Join(inboundDir, packetName)
	if err := os.Rename(outboundPath, inboundPath); err != nil {
		t.Fatalf("move packet inbound: %v", err)
	}

	imported, err := svc.ImportInboundQueue(0, 0)
	if err != nil {
		t.Fatalf("import inbound queue: %v", err)
	}
	if imported != 1 {
		t.Fatalf("expected 1 imported netmail message, got %d", imported)
	}
	inbox, err := mail.ListInbox(bob.ID, 50)
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("expected 1 inbox message, got %d", len(inbox))
	}
	if inbox[0].Subject != "Netmail Subject" {
		t.Fatalf("unexpected subject %q", inbox[0].Subject)
	}

	status, err := svc.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.OutboundPackets != 0 {
		t.Fatalf("expected 0 outbound after moving packet, got %d", status.OutboundPackets)
	}
	if status.InboundPackets != 0 {
		t.Fatalf("expected 0 inbound after import, got %d", status.InboundPackets)
	}
}

func TestRunExternalHooks(t *testing.T) {
	users := repository.NewInMemoryUserRepository()
	boards := repository.NewInMemoryBoardRepository()
	msgs := repository.NewInMemoryMessageRepository()
	mail := repository.NewInMemoryPrivateMailRepository()
	spool := t.TempDir()
	svc := NewService(spool, boards, msgs, users, mail)
	marker := filepath.Join(spool, "hook.txt")
	svc.SetExternalCommands("echo import > hook.txt", "echo export > hook.txt")

	if err := svc.RunExternalExport(context.Background()); err != nil {
		t.Fatalf("run export hook: %v", err)
	}
	raw, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker after export: %v", err)
	}
	if string(raw) != "export\n" {
		t.Fatalf("unexpected export marker content: %q", string(raw))
	}
	if err := svc.RunExternalImport(context.Background()); err != nil {
		t.Fatalf("run import hook: %v", err)
	}
	raw, err = os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker after import: %v", err)
	}
	if string(raw) != "import\n" {
		t.Fatalf("unexpected import marker content: %q", string(raw))
	}
}
