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
	"wolfbbs/internal/doors"
	"wolfbbs/internal/repository"
)

func TestHandleTopX(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	chatSvc := chat.NewServiceForTest()
	doorRepo := repository.NewInMemoryDoorRepository()
	reg := doors.NewRegistry()
	reg.SetRepository(doorRepo)
	reg.Register(doors.Door{ID: "retro-door", Hotkey: "R", Name: "Retro Door", Command: "/bin/true"})

	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("viewer", "password123"); err != nil {
		t.Fatalf("register viewer: %v", err)
	}
	if _, err := authSvc.Register("alice", "password123"); err != nil {
		t.Fatalf("register alice: %v", err)
	}
	if _, err := authSvc.Register("bob", "password123"); err != nil {
		t.Fatalf("register bob: %v", err)
	}
	viewer, _ := authSvc.GetUser("viewer")
	alice, _ := authSvc.GetUser("alice")
	bob, _ := authSvc.GetUser("bob")

	board := &domain.Board{Name: "General", Description: "General", CreatedBy: alice.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{BoardID: board.ID, AuthorID: alice.ID, Subject: "A", Body: "body", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{BoardID: board.ID, AuthorID: bob.ID, Subject: "B", Body: "body", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	chatSvc.JoinChannel("bob", "#lobby")
	if _, err := chatSvc.Post("bob", "#lobby", "hello topx"); err != nil {
		t.Fatalf("chat post: %v", err)
	}
	if err := reg.SubmitScore("retro-door", bob.ID, "points", 120, ""); err != nil {
		t.Fatalf("submit score: %v", err)
	}

	app := &webApp{
		authSvc:      authSvc,
		boardRepo:    boardRepo,
		msgRepo:      msgRepo,
		chatSvc:      chatSvc,
		doorRegistry: reg,
		sessions:     map[string]sessionState{},
	}
	sid, ok := app.createSession(viewer.Handle)
	if !ok {
		t.Fatal("session creation failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/topx?range=week", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.authRequired(http.HandlerFunc(app.handleTopX)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("topx status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, needle := range []string{"TopX Leaderboards", "Weekly TopX", "alice", "bob", "Momentum"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in topx page: %s", needle, body)
		}
	}
}
