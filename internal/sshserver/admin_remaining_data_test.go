package sshserver

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

func TestPersistAdminScheduledBulletinsRoundTrip(t *testing.T) {
	adminRepo := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: adminRepo}
	start := time.Date(2026, 3, 19, 20, 0, 0, 0, time.UTC)

	if err := srv.persistAdminScheduledBulletins([]adminScheduledBulletin{{
		Title:     "Launch Night",
		Body:      "Doors open at eight.",
		StartsAt:  start,
		Audience:  "all callers",
		CreatedBy: "sysop",
	}}); err != nil {
		t.Fatalf("persist bulletins: %v", err)
	}

	rows := srv.loadAdminScheduledBulletins()
	if len(rows) != 1 {
		t.Fatalf("expected 1 bulletin, got %d", len(rows))
	}
	if rows[0].ID == "" || rows[0].Audience != "all callers" {
		t.Fatalf("unexpected bulletin row: %+v", rows[0])
	}
}

func TestPersistAdminCommunityEventsRoundTrip(t *testing.T) {
	adminRepo := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: adminRepo}
	start := time.Date(2026, 3, 20, 1, 0, 0, 0, time.UTC)

	if err := srv.persistAdminCommunityEvents([]adminCommunityEvent{{
		Title:       "Tournament Night",
		Category:    "Tournament",
		StartsAt:    start,
		Location:    "#lobby",
		Host:        "sysop",
		Audience:    "all callers",
		Description: "Bracket play.",
	}}); err != nil {
		t.Fatalf("persist events: %v", err)
	}

	rows := srv.loadAdminCommunityEvents()
	if len(rows) != 1 {
		t.Fatalf("expected 1 event, got %d", len(rows))
	}
	if rows[0].ID == "" || rows[0].Category != "tournament" {
		t.Fatalf("unexpected event row: %+v", rows[0])
	}
}

func TestPersistAdminSeasonChallengesAndGoalsRoundTrip(t *testing.T) {
	adminRepo := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: adminRepo}
	start := time.Now().UTC().Add(-time.Hour)
	end := start.Add(24 * time.Hour)

	if err := srv.persistAdminSeasonChallenges([]adminSeasonChallenge{{
		Name:        "Spring Sprint",
		Theme:       "doors",
		Description: "Play and post.",
		StartsAt:    start,
		EndsAt:      end,
		BoardWeight: 3,
		ChatWeight:  1,
		DoorWeight:  2,
		Active:      true,
		UpdatedBy:   "sysop",
	}}); err != nil {
		t.Fatalf("persist challenges: %v", err)
	}
	if err := srv.persistAdminClubhouseGoals([]adminClubhouseGoal{{
		Title:       "Reach 50 runs",
		DoorID:      "retro-door",
		Target:      50,
		Progress:    12,
		Description: "Keep the board busy.",
		UpdatedBy:   "sysop",
	}}); err != nil {
		t.Fatalf("persist goals: %v", err)
	}

	challenges := srv.loadAdminSeasonChallenges()
	goals := srv.loadAdminClubhouseGoals()
	if len(challenges) != 1 || len(goals) != 1 {
		t.Fatalf("expected challenge+goal rows, got %d and %d", len(challenges), len(goals))
	}
	if challenges[0].ID == "" || goals[0].ID == "" {
		t.Fatalf("expected generated ids, got %+v %+v", challenges[0], goals[0])
	}
}

func TestModeratorInboxAssignmentsRoundTripSSH(t *testing.T) {
	adminRepo := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: adminRepo}

	if err := srv.updateModeratorInboxAssignmentSSH(42, adminModeratorInboxAssignment{Assignee: "mod", Status: "assigned", Note: "take first reply", UpdatedBy: "sysop"}); err != nil {
		t.Fatalf("update moderator assignment: %v", err)
	}
	rows := srv.loadModeratorInboxAssignmentsSSH()
	if rows[42].Assignee != "mod" || rows[42].Status != "assigned" {
		t.Fatalf("unexpected moderator assignment rows: %+v", rows)
	}
}

func TestFileReviewQueueRoundTripSSH(t *testing.T) {
	adminRepo := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: adminRepo}

	if err := srv.setFileReviewItemSSH(adminFileReviewItem{FileID: 7, AreaID: 3, Name: "demo.zip", Status: adminFileReviewApproved, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("set file review item: %v", err)
	}
	rows := srv.loadFileReviewQueueSSH()
	if rows[7].Status != adminFileReviewApproved {
		t.Fatalf("unexpected review queue row: %+v", rows[7])
	}
	if err := srv.removeFileReviewItemSSH(7); err != nil {
		t.Fatalf("remove file review item: %v", err)
	}
	if len(srv.loadFileReviewQueueSSH()) != 0 {
		t.Fatalf("expected empty review queue, got %+v", srv.loadFileReviewQueueSSH())
	}
}

func TestSharedRuntimeErrorsRoundTripSSH(t *testing.T) {
	adminRepo := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: adminRepo}
	now := time.Now().UTC()

	if err := srv.persistSharedRuntimeErrorsSSH([]adminSharedRuntimeError{{Time: now.Add(-time.Minute), Area: "mail", Message: "first"}, {Time: now, Area: "chat", Message: "second"}}); err != nil {
		t.Fatalf("persist shared runtime errors: %v", err)
	}
	rows := srv.loadSharedRuntimeErrorsSSH()
	if len(rows) != 2 || rows[1].Message != "second" {
		t.Fatalf("unexpected shared runtime errors: %+v", rows)
	}
	cleared, err := srv.clearSharedRuntimeErrorsSSH()
	if err != nil {
		t.Fatalf("clear shared runtime errors: %v", err)
	}
	if cleared != 2 || len(srv.loadSharedRuntimeErrorsSSH()) != 0 {
		t.Fatalf("expected clear to remove 2 rows, got cleared=%d rows=%+v", cleared, srv.loadSharedRuntimeErrorsSSH())
	}
}

func TestResolvePageRequestAndEscalationSSH(t *testing.T) {
	adminRepo := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: adminRepo}
	now := time.Now().UTC()

	if err := srv.persistPageRequests([]persistedPageRequest{{ID: "page-1", From: "alice", To: "sysop", Message: "ping", CreatedAt: now}}); err != nil {
		t.Fatalf("persist page requests: %v", err)
	}
	if !srv.resolvePageRequestSSH("page-1") {
		t.Fatal("expected page request to resolve")
	}
	if len(srv.loadPageRequests()) != 0 {
		t.Fatalf("expected page queue empty, got %+v", srv.loadPageRequests())
	}

	if err := srv.persistStaffEscalationsSSH([]adminStaffEscalation{{ID: "esc-1", Handle: "alice", Actor: "mod", Note: "needs follow-up", CreatedAt: now}}); err != nil {
		t.Fatalf("persist staff escalations: %v", err)
	}
	if !srv.resolveStaffEscalationSSH("esc-1") {
		t.Fatal("expected escalation to resolve")
	}
	rows := srv.loadStaffEscalationsSSH()
	if len(rows) != 1 || rows[0].ResolvedAt.IsZero() {
		t.Fatalf("expected resolved escalation row, got %+v", rows)
	}
}

func TestUpsertFeaturedCollectionSSH(t *testing.T) {
	adminRepo := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: adminRepo}

	if err := srv.upsertFeaturedCollectionSSH(sshFeaturedFileCollection{Title: "Top Picks", Description: "best files", FileIDs: []int64{9, 9, 4}, Tags: []string{"Retro", "retro"}, Curator: "SysOp"}); err != nil {
		t.Fatalf("upsert featured collection: %v", err)
	}
	rows := srv.loadFeaturedCollectionsSSH()
	if len(rows) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(rows))
	}
	if rows[0].ID == "" || len(rows[0].FileIDs) != 2 || rows[0].Curator != "sysop" {
		t.Fatalf("unexpected collection row: %+v", rows[0])
	}
}

func TestIndexAreaFilesSSH(t *testing.T) {
	adminRepo := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: adminRepo}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.zip"), []byte("payload"), 0o644); err != nil {
		t.Fatalf("write demo file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo.diz"), []byte("Ansi archive"), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	area := &domain.FileArea{Name: "Uploads", Path: dir, Description: "test area"}
	if err := adminRepo.CreateFileArea(area); err != nil {
		t.Fatalf("create file area: %v", err)
	}

	indexed, failed, err := srv.indexAreaFilesSSH(*area, 1)
	if err != nil {
		t.Fatalf("index area files: %v", err)
	}
	if indexed != 1 || failed != 0 {
		t.Fatalf("expected 1 indexed and 0 failed, got indexed=%d failed=%d", indexed, failed)
	}
	entries, err := adminRepo.ListFileEntries(area.ID, "", nil, 10)
	if err != nil {
		t.Fatalf("list file entries: %v", err)
	}
	if len(entries) != 1 || entries[0].SHA256 == "" || len(entries[0].Tags) == 0 {
		t.Fatalf("unexpected indexed entry: %+v", entries)
	}
}
