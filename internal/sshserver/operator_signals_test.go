package sshserver

import (
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/chat"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

func TestCollectAdminSignalsCountsRecentActivity(t *testing.T) {
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	chatSvc := chat.NewServiceForTest()

	board := &domain.Board{Name: "General", CreatedBy: 1}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   board.ID,
		AuthorID:  2,
		Subject:   "hello",
		Body:      "world",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	if _, err := chatSvc.Post("caller", "#lobby", "hello lobby"); err != nil {
		t.Fatalf("chat post: %v", err)
	}
	for _, row := range []domain.AdminAudit{
		{Actor: "sysop", Target: "setup", Action: "seed_default_boards", Details: "failed: duplicate data source", CreatedAt: time.Now().UTC()},
		{Actor: "sysop", Target: "caller", Action: "create_user", Details: "role=user", CreatedAt: time.Now().UTC()},
		{Actor: "sysop", Target: "app_upgrade", Action: "app_upgrade_success", Details: "ok", CreatedAt: time.Now().UTC()},
		{Actor: "sysop", Target: "app_upgrade", Action: "app_upgrade_failed", Details: "boom", CreatedAt: time.Now().UTC()},
	} {
		rowCopy := row
		if err := adminRepo.AddAudit(&rowCopy); err != nil {
			t.Fatalf("seed audit %s: %v", row.Action, err)
		}
	}

	srv := &Server{
		boards:  boardRepo,
		msgs:    msgRepo,
		admin:   adminRepo,
		chatSvc: chatSvc,
	}
	signals := srv.collectAdminSignals(7)
	if signals.ActiveBoards != 1 || signals.ActiveChannels != 1 {
		t.Fatalf("unexpected board/chat activity counts: %+v", signals)
	}
	if signals.SetupFailures != 1 || signals.UserCreates != 1 {
		t.Fatalf("unexpected setup/user counts: %+v", signals)
	}
	if signals.UpgradeSuccesses != 1 || signals.UpgradeFailures != 1 {
		t.Fatalf("unexpected upgrade counts: %+v", signals)
	}
	lines := strings.Join(srv.adminSignalLines(7), "\n")
	for _, needle := range []string{"boards active", "setup failures", "app upgrade success/fail"} {
		if !strings.Contains(lines, needle) {
			t.Fatalf("admin signal lines missing %q: %s", needle, lines)
		}
	}
}

func TestRecordAdminAuditPersistsEntry(t *testing.T) {
	adminRepo := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: adminRepo}
	srv.recordAdminAudit("sysop", "app_upgrade", "app_upgrade_success", "ok")

	rows, err := adminRepo.ListAudit(10)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one audit row, got %d", len(rows))
	}
	if rows[0].Action != "app_upgrade_success" {
		t.Fatalf("unexpected audit row: %+v", rows[0])
	}
}
