package repository

import (
	"path/filepath"
	"testing"
	"time"

	"wolfbbs/internal/domain"
)

func TestResolveDatabaseURLUsesSQLitePath(t *testing.T) {
	withEnv(t, "WOLFBBS_DATABASE_URL", "")
	withEnv(t, "DATABASE_URL", "")
	withEnv(t, "WOLFBBS_SQLITE_PATH", "/tmp/wolfbbs-test.db")
	withEnv(t, "PGHOST", "")
	withEnv(t, "PGUSER", "")
	withEnv(t, "PGPASSWORD", "")
	withEnv(t, "PGDATABASE", "")
	withEnv(t, "PGPORT", "")

	got := ResolveDatabaseURL()
	if got != "sqlite:///tmp/wolfbbs-test.db" {
		t.Fatalf("expected sqlite dsn from WOLFBBS_SQLITE_PATH, got %q", got)
	}
}

func TestOpenStorageFromEnvSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wolfbbs.sqlite")
	storage, err := OpenStorageFromEnv("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("open sqlite storage: %v", err)
	}
	defer storage.Close()

	user := &domain.User{
		Handle:        "sqliteuser",
		PasswordHash:  "hashed",
		Enabled:       true,
		ANSIEnabled:   true,
		PagingEnabled: true,
		Role:          "user",
		Theme:         "retro-amber",
	}
	if err := storage.Users.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	board := &domain.Board{Name: "General", Description: "General board", CreatedBy: user.ID}
	if err := storage.Boards.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	board.Description = "Updated description"
	board.Conference = "Retro"
	board.ReadACS = "role=user"
	board.WriteACS = "verified"
	if err := storage.Boards.Update(board); err != nil {
		t.Fatalf("update board: %v", err)
	}
	loadedBoard, err := storage.Boards.Get(board.ID)
	if err != nil {
		t.Fatalf("load updated board: %v", err)
	}
	if loadedBoard.Description != "Updated description" {
		t.Fatalf("expected updated board description, got %q", loadedBoard.Description)
	}
	if loadedBoard.Conference != "Retro" || loadedBoard.ReadACS != "role=user" || loadedBoard.WriteACS != "verified" {
		t.Fatalf("expected board conference/acs updates, got %#v", loadedBoard)
	}
	msg := &domain.Message{
		BoardID:  board.ID,
		AuthorID: user.ID,
		Subject:  "SQLite thread",
		Body:     "hello from sqlite backend",
	}
	if err := storage.Messages.CreateMessage(msg); err != nil {
		t.Fatalf("create message: %v", err)
	}
	msgs, err := storage.Messages.ListByBoard(board.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Subject != "SQLite thread" {
		t.Fatalf("unexpected messages result: %#v", msgs)
	}
	if err := storage.Messages.SetPointer(user.ID, board.ID, msgs[0].ID, msgs[0].CreatedAt); err != nil {
		t.Fatalf("set pointer: %v", err)
	}
	ptr, err := storage.Messages.GetPointer(user.ID, board.ID)
	if err != nil {
		t.Fatalf("get pointer: %v", err)
	}
	if ptr.LastReadID != msgs[0].ID {
		t.Fatalf("expected pointer last_read_id=%d, got %#v", msgs[0].ID, ptr)
	}

	area := &domain.FileArea{Name: "Uploads", Path: "/bbs/uploads", Description: "test area"}
	if err := storage.Admin.CreateFileArea(area); err != nil {
		t.Fatalf("create file area: %v", err)
	}
	areas, err := storage.Admin.ListFileAreas()
	if err != nil {
		t.Fatalf("list file areas: %v", err)
	}
	if len(areas) != 1 || areas[0].Name != "Uploads" {
		t.Fatalf("unexpected file areas: %#v", areas)
	}
	entry := &domain.FileEntry{
		AreaID:      area.ID,
		Name:        "wolfbbs.txt",
		Path:        "/bbs/uploads/wolfbbs.txt",
		Description: "wolfbbs release notes",
		Tags:        []string{"text", "wolfbbs"},
		SHA256:      "deadbeef",
		SizeBytes:   512,
		UploaderID:  user.ID,
		UploadedAt:  time.Now().UTC().Add(-time.Minute),
	}
	if err := storage.Admin.UpsertFileEntry(entry); err != nil {
		t.Fatalf("upsert file entry: %v", err)
	}
	dup := &domain.FileEntry{
		AreaID:      area.ID,
		Name:        "wolfbbs-copy.txt",
		Path:        "/bbs/uploads/wolfbbs-copy.txt",
		Description: "duplicate hash",
		Tags:        []string{"text"},
		SHA256:      "deadbeef",
		SizeBytes:   1024,
		UploaderID:  user.ID,
		UploadedAt:  time.Now().UTC(),
	}
	if err := storage.Admin.UpsertFileEntry(dup); err != nil {
		t.Fatalf("upsert duplicate file entry: %v", err)
	}
	if dup.ID != entry.ID {
		t.Fatalf("expected sqlite dedupe on sha256, got id=%d want=%d", dup.ID, entry.ID)
	}
	if err := storage.Admin.SetFileRating(user.ID, entry.ID, 5); err != nil {
		t.Fatalf("set file rating: %v", err)
	}
	rows, err := storage.Admin.ListFileEntries(area.ID, "wolfbbs", []string{"text"}, 20)
	if err != nil {
		t.Fatalf("list file entries: %v", err)
	}
	if len(rows) != 1 || rows[0].RatingCount != 1 {
		t.Fatalf("unexpected file rows: %#v", rows)
	}
	filterRow := &domain.FileFilter{
		UserID: user.ID,
		Name:   "wolfbbs-filter",
		Query:  "wolfbbs",
		Tags:   []string{"text"},
	}
	if err := storage.Admin.SaveFileFilter(filterRow); err != nil {
		t.Fatalf("save file filter: %v", err)
	}
	filterRows, err := storage.Admin.ListFileFilters(user.ID)
	if err != nil {
		t.Fatalf("list file filters: %v", err)
	}
	if len(filterRows) != 1 || filterRows[0].Name != "wolfbbs-filter" {
		t.Fatalf("unexpected file filters: %#v", filterRows)
	}
	if err := storage.Admin.EnqueueDownload(user.ID, entry.ID); err != nil {
		t.Fatalf("enqueue download: %v", err)
	}
	queueRows, err := storage.Admin.ListDownloadQueue(user.ID, 10)
	if err != nil {
		t.Fatalf("list download queue: %v", err)
	}
	if len(queueRows) != 1 {
		t.Fatalf("expected one queue row, got %#v", queueRows)
	}
	ticket := &domain.DownloadTicket{
		Token:     "sqlite-ticket",
		UserID:    user.ID,
		FileID:    entry.ID,
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}
	if err := storage.Admin.CreateDownloadTicket(ticket); err != nil {
		t.Fatalf("create download ticket: %v", err)
	}
	if _, err := storage.Admin.GetDownloadTicket(ticket.Token, time.Now().UTC()); err != nil {
		t.Fatalf("get download ticket: %v", err)
	}
	if err := storage.Admin.MarkDownloadTicketUsed(ticket.Token, time.Now().UTC()); err != nil {
		t.Fatalf("mark download ticket used: %v", err)
	}
	if _, err := storage.Admin.GetDownloadTicket(ticket.Token, time.Now().UTC()); err != ErrNotFound {
		t.Fatalf("expected used ticket to be hidden, err=%v", err)
	}

	token := &domain.PasswordResetToken{
		TokenHash: "tokhash",
		Handle:    user.Handle,
		ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
	}
	if err := storage.Resets.Create(token); err != nil {
		t.Fatalf("create reset token: %v", err)
	}
	loadedToken, err := storage.Resets.Get("tokhash")
	if err != nil {
		t.Fatalf("get reset token: %v", err)
	}
	if loadedToken.Handle != user.Handle {
		t.Fatalf("unexpected reset token handle: %q", loadedToken.Handle)
	}

	if err := storage.Admin.UpsertSystemSetting("site.motd", "Welcome callers"); err != nil {
		t.Fatalf("upsert system setting: %v", err)
	}
	value, err := storage.Admin.GetSystemSetting("site.motd")
	if err != nil {
		t.Fatalf("get system setting: %v", err)
	}
	if value != "Welcome callers" {
		t.Fatalf("unexpected system setting value: %q", value)
	}
}

func TestSQLiteSchemaCreatesFTSTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wolfbbs.sqlite")
	db, err := OpenSQLite("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	assertTable := func(name string) {
		t.Helper()
		var got string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type IN ('table','view') AND name = ?`, name).Scan(&got)
		if err != nil {
			t.Fatalf("expected sqlite object %q: %v", name, err)
		}
	}
	assertTable("message_search")
	assertTable("file_area_search")
}

func TestSQLiteNodeSessionAndCallerHistory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wolfbbs.sqlite")
	storage, err := OpenStorageFromEnv("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("open sqlite storage: %v", err)
	}
	defer storage.Close()

	now := time.Now().UTC()
	if err := storage.Admin.UpsertNodeSession(&domain.NodeSession{
		SessionID:    "sess-1",
		NodeID:       7,
		Username:     "alpha",
		Area:         "Main Menu",
		RemoteAddr:   "127.0.0.1:2222",
		LoginAt:      now.Add(-2 * time.Minute),
		LastActivity: now.Add(-10 * time.Second),
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("upsert node session: %v", err)
	}
	sessions, err := storage.Admin.ListNodeSessions(10)
	if err != nil {
		t.Fatalf("list node sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].NodeID != 7 || sessions[0].Username != "alpha" {
		t.Fatalf("unexpected node sessions: %#v", sessions)
	}
	if err := storage.Admin.DeleteNodeSession("sess-1"); err != nil {
		t.Fatalf("delete node session: %v", err)
	}
	sessions, err = storage.Admin.ListNodeSessions(10)
	if err != nil {
		t.Fatalf("list node sessions after delete: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no node sessions after delete, got %#v", sessions)
	}

	if err := storage.Admin.AddCallerHistory(&domain.CallerHistory{
		SessionID:       "sess-old",
		NodeID:          2,
		Username:        "beta",
		Area:            "Boards",
		RemoteAddr:      "127.0.0.1:3333",
		LoginAt:         now.Add(-20 * time.Minute),
		LogoutAt:        now.Add(-10 * time.Minute),
		DurationSeconds: 600,
		CreatedAt:       now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("add caller history: %v", err)
	}
	callers, err := storage.Admin.ListCallerHistory(10)
	if err != nil {
		t.Fatalf("list caller history: %v", err)
	}
	if len(callers) != 1 || callers[0].Username != "beta" || callers[0].NodeID != 2 {
		t.Fatalf("unexpected caller history: %#v", callers)
	}
}
