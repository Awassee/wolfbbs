package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

func TestAdminCredentialReceiptQueuePopsOnce(t *testing.T) {
	app := &webApp{
		authSvc:   auth.NewService(repository.NewInMemoryUserRepository()),
		adminRepo: repository.NewInMemoryAdminRepository(),
		sessions:  map[string]sessionState{},
	}
	if _, err := app.authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	sessionID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sessionID})
	if !app.queueAdminCredentialReceipt(req, adminCredentialReceipt{
		Action:   "reset",
		Handle:   "caller",
		Password: "newpass123",
	}) {
		t.Fatal("queueAdminCredentialReceipt returned false")
	}
	first := renderAdminCredentialReceiptBlock(app.popAdminCredentialReceipts(req))
	if !strings.Contains(first, "One-time credential receipts") || !strings.Contains(first, "newpass123") {
		t.Fatalf("unexpected first receipt block: %q", first)
	}
	if next := renderAdminCredentialReceiptBlock(app.popAdminCredentialReceipts(req)); next != "" {
		t.Fatalf("expected receipt block to clear after pop, got %q", next)
	}
}

func TestOperatorInsightsPersistAndCountIntoAnalytics(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	board := &domain.Board{Name: "General", CreatedBy: caller.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   board.ID,
		AuthorID:  caller.ID,
		Subject:   "hello",
		Body:      "world",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	chatSvc := chat.NewServiceForTest()
	if _, err := chatSvc.Post(caller.Handle, "#lobby", "hello lobby"); err != nil {
		t.Fatalf("chat post: %v", err)
	}
	if err := adminRepo.AddAudit(&domain.AdminAudit{
		Actor:     "sysop",
		Target:    "setup",
		Action:    "seed_default_boards",
		Details:   "failed: duplicate data source",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed audit failure: %v", err)
	}
	if err := adminRepo.AddAudit(&domain.AdminAudit{
		Actor:     "sysop",
		Target:    "app_upgrade",
		Action:    "app_upgrade_failed",
		Details:   "upgrade command failed",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed upgrade failure: %v", err)
	}
	if err := adminRepo.AddAudit(&domain.AdminAudit{
		Actor:     "sysop",
		Target:    "caller",
		Action:    "create_user",
		Details:   "role=user",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed create user audit: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		chatSvc:   chatSvc,
		sessions:  map[string]sessionState{},
	}
	app.recordOperatorInsight("feedback.sent", "caller", "/feedback")
	app.recordOperatorInsight("first_call.complete", "caller", "/today")

	weekly := app.collectWindowActivity(7)
	if weekly.FeedbackItems != 1 {
		t.Fatalf("expected 1 feedback item, got %d", weekly.FeedbackItems)
	}
	if weekly.FirstCallCompletions != 1 {
		t.Fatalf("expected 1 first-call completion, got %d", weekly.FirstCallCompletions)
	}
	signals := app.collectOperatorSignals(7)
	if signals.ActiveBoards != 1 || signals.ActiveChannels != 1 {
		t.Fatalf("unexpected board/channel activity snapshot: %+v", signals)
	}
	if signals.SetupFailures != 1 || signals.UpgradeFailures != 1 || signals.UserCreates != 1 {
		t.Fatalf("unexpected operator signal counts: %+v", signals)
	}

	sessionID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/analytics", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sessionID})
	rr := httptest.NewRecorder()
	app.handleAdminAnalytics(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("analytics status = %d body=%s", rr.Code, rr.Body.String())
	}
	for _, needle := range []string{"weekly first-call completes", "weekly feedback items", "First-call completes", "Feedback items", "active boards (7d)", "setup failures (7d)", "upgrade failures (7d)"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("analytics page missing %q: %s", needle, rr.Body.String())
		}
	}
}

func TestRenderSysopFirstRunBlockShowsOperatorSignals(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	sysop, err := authSvc.Register("sysop", "password123")
	if err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	app.recordOperatorInsight("first_call.complete", "caller", "/first-call")
	if err := adminRepo.AddAudit(&domain.AdminAudit{
		Actor:     "sysop",
		Target:    "app_upgrade",
		Action:    "app_upgrade_failed",
		Details:   "boom",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	body := app.renderSysopFirstRunBlock(sysop)
	for _, needle := range []string{"First 15 Minutes As Sysop", "7-Day Operator Signals", "setup failures", "upgrade failures"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("first-run block missing %q: %s", needle, body)
		}
	}
}

func TestRecordFirstCallTransitionOnlyOnCompletion(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	authSvc := auth.NewService(userRepo)
	user, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	sysop, err := authSvc.Register("sysop", "password123")
	if err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	board := &domain.Board{Name: "General", CreatedBy: sysop.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		mailRepo:  mailRepo,
		chatSvc:   chat.NewServiceForTest(),
		sessions:  map[string]sessionState{},
	}

	before := app.buildFirstCallSnapshot(user)
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   board.ID,
		AuthorID:  user.ID,
		Subject:   "hello",
		Body:      "world",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	if _, err := app.chatSvc.Post(user.Handle, "#lobby", "chat"); err != nil {
		t.Fatalf("chat post: %v", err)
	}
	if err := mailRepo.CreateMail(&domain.PrivateMail{
		FromUserID: user.ID,
		ToUserID:   sysop.ID,
		Subject:    "hello",
		Body:       "mail",
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create mail: %v", err)
	}
	app.persistHomeRoute(user.Handle, "/today")
	app.recordFirstCallTransition(user, before, "/today")

	rows := app.loadOperatorInsightEvents()
	if len(rows) != 1 || rows[0].Kind != "first_call.complete" {
		t.Fatalf("unexpected operator insight rows: %+v", rows)
	}

	app.recordFirstCallTransition(user, app.buildFirstCallSnapshot(user), "/today")
	if got := len(app.loadOperatorInsightEvents()); got != 1 {
		t.Fatalf("expected no duplicate completion insights, got %d", got)
	}
}
