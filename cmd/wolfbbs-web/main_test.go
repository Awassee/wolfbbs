package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"wolfbbs/internal/auth"
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
