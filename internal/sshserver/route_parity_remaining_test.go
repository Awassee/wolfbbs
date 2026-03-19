package sshserver

import (
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

func TestNormalizeSSHFeaturedCollections(t *testing.T) {
	now := time.Now().UTC()
	rows := normalizeSSHFeaturedCollections([]sshFeaturedFileCollection{
		{
			ID:          " newer ",
			Title:       "  Newer Picks ",
			Description: " recent bundle ",
			FileIDs:     []int64{9, 9, 4, -1},
			Tags:        []string{" Retro ", "retro", " ANSI "},
			Curator:     " SysOp ",
			UpdatedAt:   now,
		},
		{
			ID:        "older",
			Title:     "Older Picks",
			FileIDs:   []int64{1, 2},
			Tags:      []string{"files"},
			UpdatedAt: now.Add(-time.Hour),
		},
		{
			ID:    "missing-title",
			Title: "   ",
		},
	})

	if len(rows) != 2 {
		t.Fatalf("expected 2 normalized rows, got %d", len(rows))
	}
	if rows[0].ID != "newer" || rows[0].Title != "Newer Picks" {
		t.Fatalf("expected trimmed first collection, got %+v", rows[0])
	}
	if len(rows[0].FileIDs) != 2 || rows[0].FileIDs[0] != 9 || rows[0].FileIDs[1] != 4 {
		t.Fatalf("expected deduped file IDs, got %#v", rows[0].FileIDs)
	}
	if got := strings.Join(rows[0].Tags, ","); got != "retro,ansi" {
		t.Fatalf("expected normalized tags, got %q", got)
	}
	if rows[0].Curator != "sysop" {
		t.Fatalf("expected normalized curator, got %q", rows[0].Curator)
	}
}

func TestSuggestHandlesFiltersAndSorts(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	srv := &Server{auth: authSvc}

	alex, err := authSvc.Register("alex", "password123")
	if err != nil {
		t.Fatalf("register alex: %v", err)
	}
	if _, err := authSvc.Register("alice", "password123"); err != nil {
		t.Fatalf("register alice: %v", err)
	}
	bob, err := authSvc.Register("bob", "password123")
	if err != nil {
		t.Fatalf("register bob: %v", err)
	}
	bob.Banned = true
	if err := userRepo.Update(bob); err != nil {
		t.Fatalf("update bob: %v", err)
	}
	alex.Enabled = false
	if err := userRepo.Update(alex); err != nil {
		t.Fatalf("disable alex: %v", err)
	}

	rows := srv.suggestHandles("al", "alice", 8)
	if len(rows) != 0 {
		t.Fatalf("expected no visible handles after filtering, got %#v", rows)
	}

	if _, err := authSvc.Register("ally", "password123"); err != nil {
		t.Fatalf("register ally: %v", err)
	}
	if _, err := authSvc.Register("caleb", "password123"); err != nil {
		t.Fatalf("register caleb: %v", err)
	}
	rows = srv.suggestHandles("al", "alice", 8)
	if got := strings.Join(rows, ","); got != "ally,caleb" {
		t.Fatalf("expected prefix then contains matches, got %q", got)
	}
}

func TestParseSSHOfflineReplyPayloadSupportsObjectAndArray(t *testing.T) {
	objectRows, err := parseSSHOfflineReplyPayload(`{"replies":[{"to":"bob","subject":"hello","body":"world"}]}`)
	if err != nil {
		t.Fatalf("parse object payload: %v", err)
	}
	if len(objectRows) != 1 || objectRows[0].To != "bob" {
		t.Fatalf("unexpected object payload rows: %#v", objectRows)
	}

	arrayRows, err := parseSSHOfflineReplyPayload(`[{"to":"alice","subject":"hi","body":"there"}]`)
	if err != nil {
		t.Fatalf("parse array payload: %v", err)
	}
	if len(arrayRows) != 1 || arrayRows[0].To != "alice" {
		t.Fatalf("unexpected array payload rows: %#v", arrayRows)
	}

	if _, err := parseSSHOfflineReplyPayload(`{"replies":[]}`); err == nil {
		t.Fatal("expected empty payload to fail")
	}
}

func TestImportOfflineMailRepliesImportsValidLocalReplies(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	resetRepo := repository.NewInMemoryPasswordResetRepository()
	authSvc := auth.NewService(userRepo)
	authSvc.SetPasswordResetRepository(resetRepo)
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	srv := &Server{auth: authSvc, mail: mailRepo}

	alice, err := authSvc.Register("alice", "password123")
	if err != nil {
		t.Fatalf("register alice: %v", err)
	}
	bob, err := authSvc.Register("bob", "password123")
	if err != nil {
		t.Fatalf("register bob: %v", err)
	}
	payload := `{"replies":[{"to":"bob","subject":"offline hello","body":"queued from packet"},{"to":"unknown","subject":"skip","body":"skip"}]}`

	count, err := srv.importOfflineMailReplies(alice, payload)
	if err != nil {
		t.Fatalf("import replies: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 imported reply, got %d", count)
	}

	inbox, err := mailRepo.ListInbox(bob.ID, 10)
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("expected 1 inbox row, got %d", len(inbox))
	}
	if inbox[0].Subject != "offline hello" || inbox[0].Body != "queued from packet" {
		t.Fatalf("unexpected imported mail: %+v", inbox[0])
	}
}

func TestBuildOfflineBoardPacketUsesBoardSelection(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	srv := &Server{
		auth:   authSvc,
		users:  userRepo,
		admin:  adminRepo,
		boards: boardRepo,
		msgs:   msgRepo,
	}

	alice, err := authSvc.Register("alice", "password123")
	if err != nil {
		t.Fatalf("register alice: %v", err)
	}
	if _, err := authSvc.Register("bob", "password123"); err != nil {
		t.Fatalf("register bob: %v", err)
	}

	general := &domain.Board{Name: "General", Conference: "Local"}
	if err := boardRepo.Create(general); err != nil {
		t.Fatalf("create general board: %v", err)
	}
	dev := &domain.Board{Name: "Dev", Conference: "Ops"}
	if err := boardRepo.Create(dev); err != nil {
		t.Fatalf("create dev board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{BoardID: general.ID, AuthorID: alice.ID, Subject: "General note", Body: "General body"}); err != nil {
		t.Fatalf("create general message: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{BoardID: dev.ID, AuthorID: alice.ID, Subject: "Dev note", Body: "Dev body"}); err != nil {
		t.Fatalf("create dev message: %v", err)
	}
	if err := adminRepo.UpsertSystemSetting(sshHandleSettingKey(sshSettingBoardSubscriptions, alice.Handle), `{"2":"watch"}`); err != nil {
		t.Fatalf("seed board subscription: %v", err)
	}

	packet := srv.buildOfflineBoardPacket(alice, 8, 10)
	if packet.Handle != "alice" {
		t.Fatalf("expected packet handle alice, got %q", packet.Handle)
	}
	if len(packet.Boards) != 1 {
		t.Fatalf("expected 1 selected board, got %d", len(packet.Boards))
	}
	if packet.Boards[0].BoardID != dev.ID || packet.Boards[0].Name != "Dev" {
		t.Fatalf("expected dev board in packet, got %+v", packet.Boards[0])
	}
	if len(packet.Boards[0].MessageRows) != 1 || packet.Boards[0].MessageRows[0].Subject != "Dev note" {
		t.Fatalf("unexpected packet message rows: %#v", packet.Boards[0].MessageRows)
	}
}
