package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/repository"
)

func TestMustBeRoleAdminBlocksNonAdmin(t *testing.T) {
	app := &webApp{
		authSvc:  auth.NewService(repository.NewInMemoryUserRepository()),
		sessions: map[string]sessionState{},
	}
	_, _ = app.authSvc.Register("regular", "password123")
	_, _ = app.authSvc.Register("boss", "password123")
	if err := app.authSvc.SetRole("boss", roleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}

	userSession, ok := app.createSession("regular")
	if !ok {
		t.Fatal("session creation failed")
	}
	adminSession, ok := app.createSession("boss")
	if !ok {
		t.Fatal("session creation failed")
	}

	protected := app.mustBeRole(roleAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	userReq := httptest.NewRequest(http.MethodGet, "/admin", nil)
	userReq.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: userSession})
	rr := httptest.NewRecorder()
	protected.ServeHTTP(rr, userReq)
	if rr.Result().StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin expected 403, got %d", rr.Result().StatusCode)
	}
	userBoardsReq := httptest.NewRequest(http.MethodGet, "/admin/boards", nil)
	userBoardsReq.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: userSession})
	rr = httptest.NewRecorder()
	protected.ServeHTTP(rr, userBoardsReq)
	if rr.Result().StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin /admin/boards expected 403, got %d", rr.Result().StatusCode)
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/admin", nil)
	adminReq.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: adminSession})
	rr = httptest.NewRecorder()
	protected.ServeHTTP(rr, adminReq)
	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("admin expected 200, got %d", rr.Result().StatusCode)
	}

	adminBoardsReq := httptest.NewRequest(http.MethodGet, "/admin/boards", nil)
	adminBoardsReq.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: adminSession})
	rr = httptest.NewRecorder()
	protected.ServeHTTP(rr, adminBoardsReq)
	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("/admin/boards expected 200 for admin, got %d", rr.Result().StatusCode)
	}
}

func TestChatHistoryAcrossTwoSessions(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	_, _ = authSvc.Register("alice", "password123")
	_, _ = authSvc.Register("bob", "password123")

	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		mailRepo:  mailRepo,
		adminRepo: adminRepo,
		chatSvc:   chat.NewServiceForTest(),
		sessions:  map[string]sessionState{},
	}

	aliceSession, ok := app.createSession("alice")
	if !ok {
		t.Fatal("alice session failed")
	}
	bobSession, ok := app.createSession("bob")
	if !ok {
		t.Fatal("bob session failed")
	}

	getCSRF := func(sessionID string) string {
		app.Lock()
		defer app.Unlock()
		return app.sessions[sessionID].csrf
	}

	postJSON := func(path, sessionID, csrf string, body map[string]string, handler http.HandlerFunc) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", csrf)
		req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sessionID})
		rr := httptest.NewRecorder()
		handler(rr, req)
		return rr
	}

	if rr := postJSON("/chat/join", aliceSession, getCSRF(aliceSession), map[string]string{"channel": "#lobby"}, app.handleChatJoin); rr.Code != http.StatusOK {
		t.Fatalf("alice join status = %d", rr.Code)
	}
	if rr := postJSON("/chat/join", bobSession, getCSRF(bobSession), map[string]string{"channel": "#lobby"}, app.handleChatJoin); rr.Code != http.StatusOK {
		t.Fatalf("bob join status = %d", rr.Code)
	}
	if rr := postJSON("/chat/send", aliceSession, getCSRF(aliceSession), map[string]string{"channel": "#lobby", "message": "hello from alice"}, app.handleChatSend); rr.Code != http.StatusCreated {
		t.Fatalf("alice send status = %d", rr.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/chat/history?channel=%23lobby&limit=20", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: bobSession})
	rr := httptest.NewRecorder()
	app.handleChatHistory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("history status = %d", rr.Code)
	}
	var payload chatHistoryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("history decode: %v", err)
	}
	if len(payload.Messages) == 0 {
		t.Fatal("expected at least one message")
	}
	last := payload.Messages[len(payload.Messages)-1]
	if last.From != "alice" || last.Body != "hello from alice" {
		t.Fatalf("unexpected last message: %+v", last)
	}
}
