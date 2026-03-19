package network

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if !strings.HasSuffix(strings.ToLower(path), ".qwk") {
		t.Fatalf("expected qwk extension, got %s", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected exported packet at %s: %v", path, err)
	}
	bundle, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open qwk bundle: %v", err)
	}
	defer bundle.Close()
	seen := map[string]bool{}
	for _, file := range bundle.File {
		seen[strings.ToUpper(filepath.Base(file.Name))] = true
	}
	for _, required := range []string{"CONTROL.DAT", "HEADERS.DAT", "MESSAGES.DAT", "MESSAGES.JSON"} {
		if !seen[required] {
			t.Fatalf("missing %s in qwk bundle", required)
		}
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

func TestImportBoardPacketRouteByConferenceAndBoardName(t *testing.T) {
	users := repository.NewInMemoryUserRepository()
	boards := repository.NewInMemoryBoardRepository()
	msgs := repository.NewInMemoryMessageRepository()
	mail := repository.NewInMemoryPrivateMailRepository()
	authSvc := auth.NewService(users)
	alice := seedUser(t, authSvc, "alice")

	publicBoard := &domain.Board{Name: "General", Conference: "Public", Description: "General board", CreatedBy: alice.ID}
	retroBoard := &domain.Board{Name: "Retro", Conference: "Retro", Description: "Retro board", CreatedBy: alice.ID}
	if err := boards.Create(publicBoard); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := boards.Create(retroBoard); err != nil {
		t.Fatalf("create board: %v", err)
	}

	svc := NewService(t.TempDir(), boards, msgs, users, mail)
	svc.SetBoardRoutes(map[string]int64{
		"retro/retro": retroBoard.ID,
	})
	packet := Packet{
		Version:  1,
		Format:   FormatQWK,
		Exported: time.Now().UTC(),
		Messages: []PacketMessage{
			{Conference: "Retro", Board: "Retro", FromUserID: alice.ID, Subject: "Route test", Body: "hello"},
		},
	}
	path, err := svc.writePacket("inbound", FormatQWK, packet)
	if err != nil {
		t.Fatalf("write packet: %v", err)
	}
	imported, err := svc.ImportPacket(path, 0, 0)
	if err != nil {
		t.Fatalf("import packet: %v", err)
	}
	if imported != 1 {
		t.Fatalf("expected 1 imported message, got %d", imported)
	}
	rows, err := msgs.ListByBoard(retroBoard.ID)
	if err != nil {
		t.Fatalf("list board messages: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 routed message in retro board, got %d", len(rows))
	}
}

func TestImportNetmailHandleRouting(t *testing.T) {
	users := repository.NewInMemoryUserRepository()
	boards := repository.NewInMemoryBoardRepository()
	msgs := repository.NewInMemoryMessageRepository()
	mail := repository.NewInMemoryPrivateMailRepository()
	authSvc := auth.NewService(users)
	alice := seedUser(t, authSvc, "alice")
	bob := seedUser(t, authSvc, "bob")

	svc := NewService(t.TempDir(), boards, msgs, users, mail)
	svc.SetHandleRoutes(map[string]string{
		"ops@example.net": "bob",
	})
	packet := Packet{
		Version:  1,
		Format:   FormatNetmail,
		Exported: time.Now().UTC(),
		Messages: []PacketMessage{
			{FromUserID: alice.ID, ToHandle: "ops@example.net", Subject: "Alias route", Body: "route through alias"},
			{FromUserID: alice.ID, ToHandle: "bob@example.org", Subject: "Domain strip route", Body: "route by local handle"},
		},
	}
	path, err := svc.writePacket("inbound", FormatNetmail, packet)
	if err != nil {
		t.Fatalf("write netmail packet: %v", err)
	}
	imported, err := svc.ImportPacket(path, 0, 0)
	if err != nil {
		t.Fatalf("import netmail packet: %v", err)
	}
	if imported != 2 {
		t.Fatalf("expected 2 imported netmail rows, got %d", imported)
	}
	inbox, err := mail.ListInbox(bob.ID, 10)
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if len(inbox) != 2 {
		t.Fatalf("expected 2 routed netmail messages, got %d", len(inbox))
	}
}
