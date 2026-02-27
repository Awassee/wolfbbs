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

func TestInMemoryAdminRepositoryFileWorkflows(t *testing.T) {
	repo := NewInMemoryAdminRepository()
	area := &domain.FileArea{Name: "Uploads", Path: "/tmp/uploads"}
	if err := repo.CreateFileArea(area); err != nil {
		t.Fatalf("create area: %v", err)
	}

	first := &domain.FileEntry{
		AreaID:      area.ID,
		Name:        "doom.zip",
		Path:        "/tmp/uploads/doom.zip",
		Description: "Classic game shareware",
		Tags:        []string{"retro", "games"},
		SHA256:      "abc123",
		SizeBytes:   1024,
		UploaderID:  7,
		UploadedAt:  time.Now().UTC().Add(-time.Hour),
	}
	if err := repo.UpsertFileEntry(first); err != nil {
		t.Fatalf("upsert first entry: %v", err)
	}
	if first.ID <= 0 {
		t.Fatalf("expected first id, got %d", first.ID)
	}

	dup := &domain.FileEntry{
		AreaID:      area.ID,
		Name:        "doom-copy.zip",
		Path:        "/tmp/uploads/doom-copy.zip",
		Description: "Duplicate by hash",
		Tags:        []string{"retro"},
		SHA256:      "abc123",
		SizeBytes:   2048,
		UploaderID:  8,
		UploadedAt:  time.Now().UTC(),
	}
	if err := repo.UpsertFileEntry(dup); err != nil {
		t.Fatalf("upsert dup entry: %v", err)
	}
	if dup.ID != first.ID {
		t.Fatalf("expected dedupe id %d, got %d", first.ID, dup.ID)
	}

	if err := repo.SetFileRating(10, first.ID, 5); err != nil {
		t.Fatalf("set rating: %v", err)
	}
	if err := repo.SetFileRating(11, first.ID, 4); err != nil {
		t.Fatalf("set rating 2: %v", err)
	}

	got, err := repo.GetFileEntry(first.ID)
	if err != nil {
		t.Fatalf("get file entry: %v", err)
	}
	if got.RatingCount != 2 {
		t.Fatalf("expected rating count 2, got %d", got.RatingCount)
	}

	list, err := repo.ListFileEntries(area.ID, "doom", []string{"retro"}, 20)
	if err != nil {
		t.Fatalf("list file entries: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 file row, got %d", len(list))
	}

	filter := &domain.FileFilter{
		UserID: 10,
		Name:   "retro-doom",
		Query:  "doom",
		Tags:   []string{"retro"},
	}
	if err := repo.SaveFileFilter(filter); err != nil {
		t.Fatalf("save filter: %v", err)
	}
	filters, err := repo.ListFileFilters(10)
	if err != nil {
		t.Fatalf("list filters: %v", err)
	}
	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}

	if err := repo.EnqueueDownload(10, first.ID); err != nil {
		t.Fatalf("enqueue download: %v", err)
	}
	queue, err := repo.ListDownloadQueue(10, 10)
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	if len(queue) != 1 {
		t.Fatalf("expected queue len 1, got %d", len(queue))
	}
	ticket := &domain.DownloadTicket{
		Token:     "token-1",
		UserID:    10,
		FileID:    first.ID,
		ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
	}
	if err := repo.CreateDownloadTicket(ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	loaded, err := repo.GetDownloadTicket(ticket.Token, time.Now().UTC())
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if loaded.UserID != 10 || loaded.FileID != first.ID {
		t.Fatalf("unexpected ticket: %#v", loaded)
	}
	if err := repo.MarkDownloadTicketUsed(ticket.Token, time.Now().UTC()); err != nil {
		t.Fatalf("mark ticket used: %v", err)
	}
	if _, err := repo.GetDownloadTicket(ticket.Token, time.Now().UTC()); err != ErrNotFound {
		t.Fatalf("expected used ticket to be unavailable, got err=%v", err)
	}
}
