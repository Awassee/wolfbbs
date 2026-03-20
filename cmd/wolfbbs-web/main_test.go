package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/config"
	"wolfbbs/internal/discovery"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/doors"
	"wolfbbs/internal/gateway"
	"wolfbbs/internal/mods"
	"wolfbbs/internal/repository"
	"wolfbbs/internal/ui"
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

func TestMustBeRoleAdminAppliesACSRule(t *testing.T) {
	t.Setenv("WOLFBBS_ACS_ADMIN", "handle=allowed")
	app := &webApp{
		authSvc:  auth.NewService(repository.NewInMemoryUserRepository()),
		sessions: map[string]sessionState{},
	}
	_, _ = app.authSvc.Register("allowed", "password123")
	_, _ = app.authSvc.Register("blocked", "password123")
	if err := app.authSvc.SetRole("allowed", roleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}
	if err := app.authSvc.SetRole("blocked", roleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}
	allowedSession, ok := app.createSession("allowed")
	if !ok {
		t.Fatal("allowed session creation failed")
	}
	blockedSession, ok := app.createSession("blocked")
	if !ok {
		t.Fatal("blocked session creation failed")
	}
	protected := app.mustBeRole(roleAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/system", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: blockedSession})
	rr := httptest.NewRecorder()
	protected.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected blocked sysop to fail ACS, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/system", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: allowedSession})
	rr = httptest.NewRecorder()
	protected.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected allowed sysop to pass ACS, got %d", rr.Code)
	}
}

func TestWFCDashboardStrictPolicy(t *testing.T) {
	t.Setenv("WOLFBBS_WFC_STRICT", "true")
	app := &webApp{
		authSvc:  auth.NewService(repository.NewInMemoryUserRepository()),
		sessions: map[string]sessionState{},
	}
	_, _ = app.authSvc.Register("sysop", "password123")
	if err := app.authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}
	insecureSID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	secureSID, ok := app.createSessionWithContext("sysop", 2, "https", true)
	if !ok {
		t.Fatal("secure session creation failed")
	}

	protected := app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminSystem))

	req := httptest.NewRequest(http.MethodGet, "/admin/system", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: insecureSID})
	rr := httptest.NewRecorder()
	protected.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected strict wfc policy to deny insecure/1fa session, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/system", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: secureSID})
	rr = httptest.NewRecorder()
	protected.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected strict wfc policy to allow secure 2fa session, got %d", rr.Code)
	}
}

func TestMailAndFileGatewayACS(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	if _, err := authSvc.Register("reader", "password123"); err != nil {
		t.Fatalf("register user: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		mailRepo:  mailRepo,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("reader")
	if !ok {
		t.Fatal("session creation failed")
	}

	t.Setenv("WOLFBBS_ACS_MAIL_READ", "role=moderator")
	req := httptest.NewRequest(http.MethodGet, "/mail", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected /mail to be forbidden by ACS, got %d", rr.Code)
	}

	t.Setenv("WOLFBBS_ACS_MAIL_READ", "")
	t.Setenv("WOLFBBS_ACS_MAIL_SEND", "role=moderator")
	form := url.Values{}
	form.Set("to", "reader")
	form.Set("subject", "hello")
	form.Set("body", "world")
	form.Set("csrf_token", "token")
	req = httptest.NewRequest(http.MethodPost, "/mail", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	app.sessions[sid] = sessionState{handle: "reader", expire: time.Now().Add(time.Hour), csrf: "token"}
	rr = httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected /mail post to be forbidden by ACS send rule, got %d", rr.Code)
	}

	t.Setenv("WOLFBBS_ACS_FILES_READ", "role=moderator")
	req = httptest.NewRequest(http.MethodGet, "/gateway?view=files", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected file gateway to be forbidden by ACS, got %d", rr.Code)
	}
}

func TestAdminMailDisableOutboundBlocksExternalSend(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	if err := authSvc.SetVerified("caller", true); err != nil {
		t.Fatalf("set caller verified: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		adminRepo: adminRepo,
		mailRepo:  mailRepo,
		sessions:  map[string]sessionState{},
		email:     gateway.NewEmailGateway(gateway.EmailConfig{}),
	}
	sysopSID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("sysop session creation failed")
	}
	callerSID, ok := app.createSession("caller")
	if !ok {
		t.Fatal("caller session creation failed")
	}
	sysopCSRF := app.sessions[sysopSID].csrf
	callerCSRF := app.sessions[callerSID].csrf

	form := url.Values{}
	form.Set("handle", "caller")
	form.Set("action", "disable_outbound")
	form.Set("csrf_token", sysopCSRF)
	req := httptest.NewRequest(http.MethodPost, "/admin/mail", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminMail)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected admin mail update redirect, got %d", rr.Code)
	}

	policy, err := adminRepo.GetMailOutboundPolicy("caller")
	if err != nil || policy == nil || !policy.OutboundDisabled {
		t.Fatalf("expected outbound policy disabled, got policy=%+v err=%v", policy, err)
	}

	send := url.Values{}
	send.Set("to", "target@example.net")
	send.Set("subject", "blocked")
	send.Set("body", "blocked test")
	send.Set("csrf_token", callerCSRF)
	req = httptest.NewRequest(http.MethodPost, "/mail", strings.NewReader(send.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected external send blocked with 403, got %d", rr.Code)
	}
}

func TestAdminSystemDashboardRendersWFCMetrics(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}

	chatSvc := chat.NewServiceForTest()
	chatSvc.JoinChannel("sysop", "#lobby")

	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		chatSvc:   chatSvc,
		sessions:  map[string]sessionState{},
		startedAt: time.Now().Add(-2 * time.Minute),
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}

	protected := app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminSystem))
	req := httptest.NewRequest(http.MethodGet, "/admin/system", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	protected.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "System / WFC Dashboard") {
		t.Fatalf("expected dashboard heading in response: %s", body)
	}
	if !strings.Contains(body, "Online / Node State") {
		t.Fatalf("expected node state section in response: %s", body)
	}
	if !strings.Contains(body, "sysop") {
		t.Fatalf("expected online user row in response: %s", body)
	}
}

func TestAdminSystemDashboardShowsCallerOriginAndAddress(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}
	now := time.Now().UTC()
	if err := adminRepo.UpsertNodeSession(&domain.NodeSession{
		SessionID:    "sess-origin",
		NodeID:       7,
		Username:     "sysop",
		Area:         "Main Menu",
		RemoteAddr:   "192.168.1.44:2200",
		LoginAt:      now.Add(-10 * time.Minute),
		LastActivity: now.Add(-10 * time.Second),
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("upsert node session: %v", err)
	}
	if err := adminRepo.AddCallerHistory(&domain.CallerHistory{
		SessionID:       "sess-old",
		NodeID:          5,
		Username:        "alpha",
		Area:            "Boards",
		RemoteAddr:      "203.0.113.99:2323",
		LoginAt:         now.Add(-40 * time.Minute),
		LogoutAt:        now.Add(-30 * time.Minute),
		DurationSeconds: 600,
		CreatedAt:       now.Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("add caller history: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
		chatSvc:   chat.NewServiceForTest(),
		startedAt: time.Now().Add(-2 * time.Minute),
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}

	protected := app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminSystem))
	req := httptest.NewRequest(http.MethodGet, "/admin/system", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	protected.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		"<th>Origin</th>",
		"<th>From</th>",
		"Origin loopback/lan/wan",
		"192.168.1.44",
		"203.0.113.99",
		"LAN",
		"WAN",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in dashboard body: %s", want, body)
		}
	}
}

func TestAdminNodeStateReturnsPersistedSessions(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}
	now := time.Now().UTC()
	if err := adminRepo.UpsertNodeSession(&domain.NodeSession{
		SessionID:    "sess-1",
		NodeID:       2,
		Username:     "sysop",
		Area:         "Main Menu",
		RemoteAddr:   "127.0.0.1:2222",
		LoginAt:      now.Add(-5 * time.Minute),
		LastActivity: now.Add(-20 * time.Second),
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("upsert node session: %v", err)
	}
	if err := adminRepo.AddCallerHistory(&domain.CallerHistory{
		SessionID:       "sess-old",
		NodeID:          1,
		Username:        "alpha",
		Area:            "Boards",
		RemoteAddr:      "127.0.0.1:1234",
		LoginAt:         now.Add(-30 * time.Minute),
		LogoutAt:        now.Add(-20 * time.Minute),
		DurationSeconds: 600,
		CreatedAt:       now.Add(-20 * time.Minute),
	}); err != nil {
		t.Fatalf("add caller history: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
		chatSvc:   chat.NewServiceForTest(),
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/node-state", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminNodeState)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"session_count":1`) {
		t.Fatalf("expected node session count in payload: %s", body)
	}
	if !strings.Contains(body, `"caller_count":1`) {
		t.Fatalf("expected caller count in payload: %s", body)
	}
	if !strings.Contains(body, `"node_id":2`) {
		t.Fatalf("expected node id in payload: %s", body)
	}
	if !strings.Contains(body, `"remote_host":"127.0.0.1"`) {
		t.Fatalf("expected remote host in payload: %s", body)
	}
	if !strings.Contains(body, `"remote_origin":"loopback"`) {
		t.Fatalf("expected remote origin in payload: %s", body)
	}
}

func TestAdminSetupConfigAndErrorScreens(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}

	app := &webApp{
		authSvc:       authSvc,
		userRepo:      userRepo,
		boardRepo:     boardRepo,
		msgRepo:       msgRepo,
		adminRepo:     adminRepo,
		chatSvc:       chat.NewServiceForTest(),
		sessions:      map[string]sessionState{},
		savedSearches: map[string][]string{},
		lockedChat:    map[string]bool{},
	}
	menuRoot := t.TempDir()
	menuFile := filepath.Join(menuRoot, "main.hjson")
	if err := os.WriteFile(menuFile, []byte(`{
  title: "Main Menu"
  entries: [
    { hotkey: "M", label: "Messages", action: "boards.open" }
    { hotkey: "Q", label: "Quit", action: "session.quit" }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write menu fixture: %v", err)
	}
	app.menuRoot = menuRoot
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	app.Lock()
	csrf := app.sessions[sid].csrf
	app.Unlock()

	protectedSetup := app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminSetup))
	setupNoticeFromRedirect := func(rr *httptest.ResponseRecorder) string {
		t.Helper()
		location := strings.TrimSpace(rr.Header().Get("Location"))
		if location == "" {
			t.Fatal("expected redirect location")
		}
		redirectURL, err := url.Parse(location)
		if err != nil {
			t.Fatalf("parse redirect location %q: %v", location, err)
		}
		return strings.TrimSpace(redirectURL.Query().Get("notice"))
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/setup", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	protectedSetup.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("setup status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Setup & Install") {
		t.Fatalf("missing setup heading: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Guided Setup Profile") {
		t.Fatalf("missing guided setup profile section: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Basic") || !strings.Contains(rr.Body.String(), "Critical") || !strings.Contains(rr.Body.String(), "Expert") {
		t.Fatalf("missing setup sections: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Launch Checklist") {
		t.Fatalf("missing launch checklist: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Launch Readiness") {
		t.Fatalf("missing launch readiness section: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Go-live verdict") {
		t.Fatalf("missing go-live verdict: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Real caller account exists") {
		t.Fatalf("missing readiness caller account row: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Common Gotchas") {
		t.Fatalf("missing common gotchas: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "After Bootstrap") {
		t.Fatalf("missing after bootstrap section: %s", rr.Body.String())
	}
	for _, needle := range []string{"Work top to bottom", "Copy</button></li>"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("setup page missing %q: %s", needle, rr.Body.String())
		}
	}

	form := url.Values{}
	form.Set("action", "seed_default_boards")
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	protectedSetup.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("setup seed boards status = %d", rr.Code)
	}
	if got := setupNoticeFromRedirect(rr); got != "Seeded 3 default board(s)." {
		t.Fatalf("expected seed boards notice, got %q", got)
	}
	boards, err := boardRepo.List()
	if err != nil {
		t.Fatalf("list boards after seed: %v", err)
	}
	if len(boards) != 3 {
		t.Fatalf("expected 3 boards after seed, got %d (%+v)", len(boards), boards)
	}

	form = url.Values{}
	form.Set("action", "seed_default_boards")
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	protectedSetup.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("setup reseed boards status = %d", rr.Code)
	}
	if got := setupNoticeFromRedirect(rr); got != "Default boards already present." {
		t.Fatalf("expected reseed boards notice, got %q", got)
	}
	boards, err = boardRepo.List()
	if err != nil {
		t.Fatalf("list boards after reseed: %v", err)
	}
	if len(boards) != 3 {
		t.Fatalf("expected 3 boards after reseed, got %d (%+v)", len(boards), boards)
	}

	form = url.Values{}
	form.Set("action", "ensure_mailbot")
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	protectedSetup.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("setup ensure mailbot status = %d", rr.Code)
	}
	if got := setupNoticeFromRedirect(rr); got != "Mailbot service account checked." {
		t.Fatalf("expected ensure mailbot notice, got %q", got)
	}
	mailbot, err := authSvc.GetUser("mailbot")
	if err != nil {
		t.Fatalf("mailbot should exist after ensure action: %v", err)
	}
	if mailbot.Enabled {
		t.Fatalf("mailbot account should be disabled after ensure action: %+v", *mailbot)
	}
	if !mailbot.Verified {
		t.Fatalf("mailbot account should be verified after ensure action: %+v", *mailbot)
	}
	if mailbot.Role != roleUser {
		t.Fatalf("mailbot role should be %q, got %q", roleUser, mailbot.Role)
	}

	form = url.Values{}
	form.Set("action", "save_setup_profile")
	form.Set("site_name", "WolfTest")
	form.Set("site_hostname", "bbs.test")
	form.Set("motd", "Welcome aboard")
	form.Set("announcement", "Maintenance tonight")
	form.Set("secure_cookie", "1")
	form.Set("require_verified_email", "1")
	form.Set("guest_tour", "1")
	form.Set("discover", "1")
	form.Set("quick_jump", "1")
	form.Set("classic_search", "1")
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	protectedSetup.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("setup profile save status = %d", rr.Code)
	}
	if app.siteName != "WolfTest" || app.siteHostname != "bbs.test" {
		t.Fatalf("site identity not updated from setup: name=%q host=%q", app.siteName, app.siteHostname)
	}
	if !app.secureCookie || !app.requireVerifiedEmail || !app.guestTour || !app.discover || !app.quickJump || !app.classicSearch {
		t.Fatalf("setup profile flags not applied: secure=%t requireVerified=%t guest=%t discover=%t quick=%t classic=%t", app.secureCookie, app.requireVerifiedEmail, app.guestTour, app.discover, app.quickJump, app.classicSearch)
	}
	if v, err := adminRepo.GetSystemSetting(sysSettingSiteName); err != nil || strings.TrimSpace(v) != "WolfTest" {
		t.Fatalf("expected persisted site.name=WolfTest, got value=%q err=%v", v, err)
	}
	if v, err := adminRepo.GetSystemSetting(sysSettingSiteHostname); err != nil || strings.TrimSpace(v) != "bbs.test" {
		t.Fatalf("expected persisted site.hostname=bbs.test, got value=%q err=%v", v, err)
	}

	form = url.Values{}
	form.Set("action", "save_flags")
	form.Set("web_onramp", "1")
	form.Set("guest_tour", "1")
	form.Set("discover", "1")
	form.Set("quick_jump", "1")
	form.Set("classic_search", "1")
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminConfig)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("runtime flags post status = %d", rr.Code)
	}
	if !app.modernOnRamp || !app.guestTour || !app.discover || !app.quickJump || !app.classicSearch {
		t.Fatalf("runtime flags not applied: onRamp=%t tour=%t discover=%t quick=%t classic=%t", app.modernOnRamp, app.guestTour, app.discover, app.quickJump, app.classicSearch)
	}

	form = url.Values{}
	form.Set("action", "save_security")
	form.Set("read_only", "1")
	form.Set("secure_cookie", "1")
	form.Set("require_verified_email", "1")
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminConfig)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("security config post status = %d", rr.Code)
	}
	if !app.readOnly || !app.secureCookie || !app.requireVerifiedEmail {
		t.Fatalf("security flags not applied: readOnly=%t secureCookie=%t requireVerified=%t", app.readOnly, app.secureCookie, app.requireVerifiedEmail)
	}
	if v, err := adminRepo.GetSystemSetting(sysSettingSecureCookie); err != nil || strings.TrimSpace(v) != "true" {
		t.Fatalf("expected persisted secure_cookie=true, got value=%q err=%v", v, err)
	}
	if v, err := adminRepo.GetSystemSetting(sysSettingRequireVerifiedEmail); err != nil || strings.TrimSpace(v) != "true" {
		t.Fatalf("expected persisted require_verified=true, got value=%q err=%v", v, err)
	}
	if v, err := adminRepo.GetSystemSetting("site.read_only"); err != nil || strings.TrimSpace(v) != "true" {
		t.Fatalf("expected persisted read_only=true, got value=%q err=%v", v, err)
	}
	app.readOnly = false

	req = httptest.NewRequest(http.MethodGet, "/admin/config?menu_file="+url.QueryEscape(menuFile), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminConfig)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("config get for menu editor status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "ANSI Menu Runtime") {
		t.Fatalf("expected menu editor section, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Basic: Identity") {
		t.Fatalf("expected identity config section, got %s", rr.Body.String())
	}

	form = url.Values{}
	form.Set("action", "validate_menu")
	form.Set("menu_file", menuFile)
	form.Set("menu_body", `{title:"Broken", entries:[{hotkey:"M",label:"Messages",action:"boards.open"} {hotkey:"M",label:"Dup",action:"mail.open"}]}`)
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminConfig)).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid menu validate status = %d", rr.Code)
	}
	if !strings.Contains(strings.ToLower(rr.Body.String()), "menu error") {
		t.Fatalf("expected menu error output, got %s", rr.Body.String())
	}

	validMenu := `{
  id: "main"
  title: "Menu X"
  entries: [
    { hotkey: "M", label: "Messages", action: "boards.open" }
    { hotkey: "Q", label: "Quit", action: "session.quit" }
  ]
}`
	form = url.Values{}
	form.Set("action", "save_menu")
	form.Set("menu_file", menuFile)
	form.Set("menu_body", validMenu)
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminConfig)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save menu status = %d", rr.Code)
	}
	savedMenu, err := os.ReadFile(menuFile)
	if err != nil {
		t.Fatalf("read saved menu: %v", err)
	}
	if !strings.Contains(string(savedMenu), "Menu X") {
		t.Fatalf("expected saved menu content, got %s", string(savedMenu))
	}
	if v, err := adminRepo.GetSystemSetting(sysSettingMenuFile); err != nil || strings.TrimSpace(v) != menuFile {
		t.Fatalf("expected persisted menu file setting, got value=%q err=%v", v, err)
	}

	app.addAppError("test", errors.New("sample runtime failure"))
	req = httptest.NewRequest(http.MethodGet, "/admin/errors", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminErrors)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("errors page status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "sample runtime failure") {
		t.Fatalf("missing runtime error row: %s", rr.Body.String())
	}
}

func TestHelpPageIncludesRoleGuidanceAndReferences(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}
	app := &webApp{
		authSvc:  authSvc,
		userRepo: userRepo,
		sessions: map[string]sessionState{},
	}

	guestReq := httptest.NewRequest(http.MethodGet, "/help", nil)
	guestRR := httptest.NewRecorder()
	app.handleHelp(guestRR, guestReq)
	if guestRR.Code != http.StatusOK {
		t.Fatalf("guest help status = %d", guestRR.Code)
	}
	guestBody := guestRR.Body.String()
	for _, needle := range []string{
		"If you're visiting for the first time",
		"10-minute launch plan",
		"If something feels broken",
		"Use the right surface",
		"First-run verification path",
		"docs/START_HERE.md",
		"docs/LAUNCH_CHECKLIST.md",
		"docs/TROUBLESHOOTING.md",
		"docs/OPERATIONS.md",
	} {
		if !strings.Contains(guestBody, needle) {
			t.Fatalf("guest help missing %q in body: %s", needle, guestBody)
		}
	}

	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	adminReq := httptest.NewRequest(http.MethodGet, "/help", nil)
	adminReq.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	adminRR := httptest.NewRecorder()
	app.handleHelp(adminRR, adminReq)
	if adminRR.Code != http.StatusOK {
		t.Fatalf("admin help status = %d", adminRR.Code)
	}
	adminBody := adminRR.Body.String()
	for _, needle := range []string{
		"If you're the sysop",
		"Common sysop jobs",
		"10-minute launch plan",
		"If something feels broken",
		"/admin/setup",
		"/admin/system",
	} {
		if !strings.Contains(adminBody, needle) {
			t.Fatalf("admin help missing %q in body: %s", needle, adminBody)
		}
	}
}

func TestAdminUsersCreateValidationAndDuplicateErrors(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		boardRepo: repository.NewInMemoryBoardRepository(),
		msgRepo:   repository.NewInMemoryMessageRepository(),
		adminRepo: repository.NewInMemoryAdminRepository(),
		chatSvc:   chat.NewServiceForTest(),
		sessions:  map[string]sessionState{},
	}
	sessionID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	app.Lock()
	csrf := app.sessions[sessionID].csrf
	app.Unlock()

	protectedUsers := app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminUsers))
	postCreate := func(handle, password, role string) *httptest.ResponseRecorder {
		t.Helper()
		form := url.Values{}
		form.Set("action", "create")
		form.Set("handle", handle)
		form.Set("password", password)
		form.Set("role", role)
		form.Set("csrf_token", csrf)
		req := httptest.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sessionID})
		rr := httptest.NewRecorder()
		protectedUsers.ServeHTTP(rr, req)
		return rr
	}
	getUsersPage := func() *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
		req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sessionID})
		rr := httptest.NewRecorder()
		protectedUsers.ServeHTTP(rr, req)
		return rr
	}

	rr := postCreate("", "password123", "user")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty handle create status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "handle is required") {
		t.Fatalf("expected handle validation error, got %q", rr.Body.String())
	}

	rr = postCreate("alphauser", "short", "user")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("short password create status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "password must be at least 8 characters") {
		t.Fatalf("expected password validation error, got %q", rr.Body.String())
	}

	rr = postCreate("alphauser", "password123", "moderator")
	if rr.Code != http.StatusFound {
		t.Fatalf("valid create status = %d body=%q", rr.Code, rr.Body.String())
	}
	if location := rr.Result().Header.Get("Location"); !strings.Contains(location, "/admin/users?notice=") {
		t.Fatalf("expected create redirect, got %q", location)
	}
	rr = getUsersPage()
	if rr.Code != http.StatusOK {
		t.Fatalf("admin users page status = %d", rr.Code)
	}
	for _, needle := range []string{"One-time credential receipts", "alphauser", "password123"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("expected receipt %q on users page, got %q", needle, rr.Body.String())
		}
	}
	created, err := authSvc.GetUser("alphauser")
	if err != nil {
		t.Fatalf("get created user: %v", err)
	}
	if created.Role != roleModerator {
		t.Fatalf("expected created user role %q, got %q", roleModerator, created.Role)
	}

	rr = postCreate("alphauser", "password123", "user")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("duplicate create status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "handle already exists") {
		t.Fatalf("expected duplicate handle error, got %q", rr.Body.String())
	}
}

func TestAdminUsersLifecycleActionsAndAuthEffects(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		adminRepo: adminRepo,
		boardRepo: repository.NewInMemoryBoardRepository(),
		msgRepo:   repository.NewInMemoryMessageRepository(),
		chatSvc:   chat.NewServiceForTest(),
		sessions:  map[string]sessionState{},
	}
	sessionID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	app.Lock()
	csrf := app.sessions[sessionID].csrf
	app.Unlock()

	protectedUsers := app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminUsers))
	postUsers := func(form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		cloned := url.Values{}
		for key, values := range form {
			next := make([]string, len(values))
			copy(next, values)
			cloned[key] = next
		}
		form = cloned
		form.Set("csrf_token", csrf)
		req := httptest.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sessionID})
		rr := httptest.NewRecorder()
		protectedUsers.ServeHTTP(rr, req)
		return rr
	}
	getUsersPage := func() *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
		req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sessionID})
		rr := httptest.NewRecorder()
		protectedUsers.ServeHTTP(rr, req)
		return rr
	}
	extractReceiptPassword := func(body, handle string) string {
		t.Helper()
		prefix := `<strong>Temporary password:</strong> <code>`
		handleNeedle := `<strong>Handle:</strong> <code>` + handle + `</code>`
		idx := strings.Index(body, handleNeedle)
		if idx < 0 {
			t.Fatalf("missing receipt for %s in body %q", handle, body)
		}
		body = body[idx:]
		start := strings.Index(body, prefix)
		if start < 0 {
			t.Fatalf("missing password prefix in body %q", body)
		}
		body = body[start+len(prefix):]
		end := strings.Index(body, "</code>")
		if end < 0 {
			t.Fatalf("missing password suffix in body %q", body)
		}
		return strings.TrimSpace(body[:end])
	}

	create := url.Values{}
	create.Set("action", "create")
	create.Set("handle", "qauser")
	create.Set("password", "qa123456")
	create.Set("role", "user")
	rr := postUsers(create)
	if rr.Code != http.StatusFound {
		t.Fatalf("create user status = %d body=%q", rr.Code, rr.Body.String())
	}
	rr = getUsersPage()
	if !strings.Contains(rr.Body.String(), "qa123456") {
		t.Fatalf("expected create credential receipt on page, got %q", rr.Body.String())
	}

	disable := url.Values{}
	disable.Set("action", "disable")
	disable.Set("handle", "qauser")
	rr = postUsers(disable)
	if rr.Code != http.StatusFound {
		t.Fatalf("disable user status = %d", rr.Code)
	}
	if _, err := authSvc.Login("qauser", "qa123456"); err == nil {
		t.Fatal("disabled user login should fail")
	}

	enable := url.Values{}
	enable.Set("action", "enable")
	enable.Set("handle", "qauser")
	rr = postUsers(enable)
	if rr.Code != http.StatusFound {
		t.Fatalf("enable user status = %d", rr.Code)
	}
	if _, err := authSvc.Login("qauser", "qa123456"); err != nil {
		t.Fatalf("enabled user login should succeed: %v", err)
	}

	ban := url.Values{}
	ban.Set("action", "ban")
	ban.Set("handle", "qauser")
	rr = postUsers(ban)
	if rr.Code != http.StatusFound {
		t.Fatalf("ban user status = %d", rr.Code)
	}
	if _, err := authSvc.Login("qauser", "qa123456"); err == nil {
		t.Fatal("banned user login should fail")
	}

	unban := url.Values{}
	unban.Set("action", "unban")
	unban.Set("handle", "qauser")
	rr = postUsers(unban)
	if rr.Code != http.StatusFound {
		t.Fatalf("unban user status = %d", rr.Code)
	}
	if _, err := authSvc.Login("qauser", "qa123456"); err != nil {
		t.Fatalf("unbanned user login should succeed: %v", err)
	}

	setRole := url.Values{}
	setRole.Set("action", "set_role")
	setRole.Set("handle", "qauser")
	setRole.Set("role", "moderator")
	rr = postUsers(setRole)
	if rr.Code != http.StatusFound {
		t.Fatalf("set role status = %d", rr.Code)
	}
	qaUser, err := authSvc.GetUser("qauser")
	if err != nil {
		t.Fatalf("get qauser after set role: %v", err)
	}
	if qaUser.Role != roleModerator {
		t.Fatalf("qauser role = %q, want %q", qaUser.Role, roleModerator)
	}

	verify := url.Values{}
	verify.Set("action", "verify")
	verify.Set("handle", "qauser")
	rr = postUsers(verify)
	if rr.Code != http.StatusFound {
		t.Fatalf("verify user status = %d", rr.Code)
	}
	qaUser, err = authSvc.GetUser("qauser")
	if err != nil {
		t.Fatalf("get qauser after verify: %v", err)
	}
	if !qaUser.Verified {
		t.Fatalf("qauser should be verified after verify action: %+v", *qaUser)
	}

	unverify := url.Values{}
	unverify.Set("action", "unverify")
	unverify.Set("handle", "qauser")
	rr = postUsers(unverify)
	if rr.Code != http.StatusFound {
		t.Fatalf("unverify user status = %d", rr.Code)
	}
	qaUser, err = authSvc.GetUser("qauser")
	if err != nil {
		t.Fatalf("get qauser after unverify: %v", err)
	}
	if qaUser.Verified {
		t.Fatalf("qauser should not be verified after unverify action: %+v", *qaUser)
	}

	reset := url.Values{}
	reset.Set("action", "reset")
	reset.Set("handle", "qauser")
	rr = postUsers(reset)
	if rr.Code != http.StatusFound {
		t.Fatalf("reset password status = %d body=%q", rr.Code, rr.Body.String())
	}
	rr = getUsersPage()
	if rr.Code != http.StatusOK {
		t.Fatalf("admin users page status = %d", rr.Code)
	}
	newPassword := extractReceiptPassword(rr.Body.String(), "qauser")
	if len(newPassword) < 8 {
		t.Fatalf("expected generated password length >= 8, got %q", newPassword)
	}
	if _, err := authSvc.Login("qauser", "qa123456"); err == nil {
		t.Fatal("old password should fail after reset")
	}
	if _, err := authSvc.Login("qauser", newPassword); err != nil {
		t.Fatalf("new password should work after reset: %v", err)
	}

	auditRows, err := adminRepo.ListAudit(50)
	if err != nil {
		t.Fatalf("list audit rows: %v", err)
	}
	expectActions := []string{
		"create_user",
		"disable_user",
		"enable_user",
		"ban_user",
		"unban_user",
		"set_role",
		"verify_user",
		"unverify_user",
		"reset_password",
	}
	for _, want := range expectActions {
		found := false
		for _, row := range auditRows {
			if row.Action == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected audit action %q in %+v", want, auditRows)
		}
	}
}

func TestGatewayFilebaseQueueAndTicket(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	user, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register user: %v", err)
	}

	dir := t.TempDir()
	filePath := filepath.Join(dir, "wolfbbs-guide.txt")
	if err := os.WriteFile(filePath, []byte("wolfbbs-file-content"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	if err := adminRepo.CreateFileArea(&domain.FileArea{Name: "Uploads", Path: dir, Description: "test"}); err != nil {
		t.Fatalf("create area: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	if _, _, err := app.indexAreaFiles(domain.FileArea{ID: 1, Name: "Uploads", Path: dir}, user.ID); err != nil {
		t.Fatalf("index area files: %v", err)
	}
	files, err := adminRepo.ListFileEntries(0, "wolfbbs-guide", nil, 10)
	if err != nil {
		t.Fatalf("list indexed files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one indexed file, got %d", len(files))
	}
	fileID := files[0].ID

	sid, ok := app.createSession(user.Handle)
	if !ok {
		t.Fatal("create session failed")
	}
	app.Lock()
	csrf := app.sessions[sid].csrf
	app.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/gateway?view=files", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("gateway files GET status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "wolfbbs-guide.txt") {
		t.Fatalf("expected indexed file in gateway files view: %s", rr.Body.String())
	}

	form := url.Values{}
	form.Set("action", "queue_add")
	form.Set("file_id", strconv.FormatInt(fileID, 10))
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/gateway", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("queue add status = %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/gateway?view=files&batch=1", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("batch zip status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.HasPrefix(rr.Body.String(), "PK") {
		sample := rr.Body.String()
		if len(sample) > 8 {
			sample = sample[:8]
		}
		t.Fatalf("expected zip magic prefix, got %q", sample)
	}

	form = url.Values{}
	form.Set("action", "ticket")
	form.Set("file_id", strconv.FormatInt(fileID, 10))
	form.Set("ttl_minutes", "10")
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/gateway", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("ticket issue status = %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	parsed, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse redirect url %q: %v", loc, err)
	}
	token := strings.TrimSpace(parsed.Query().Get("issued_token"))
	if token == "" {
		t.Fatalf("expected issued token in redirect location: %s", loc)
	}
	expiresRaw := strings.TrimSpace(parsed.Query().Get("issued_expires"))
	if expiresRaw == "" {
		t.Fatalf("expected issued_expires in redirect location: %s", loc)
	}
	if _, err := time.Parse(time.RFC3339, expiresRaw); err != nil {
		t.Fatalf("invalid issued_expires in redirect location: %s", loc)
	}

	req = httptest.NewRequest(http.MethodGet, parsed.Path+"?"+parsed.RawQuery, nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ticket status card page = %d body=%s", rr.Code, rr.Body.String())
	}
	for _, needle := range []string{"Download Ticket Issued", "Expires:"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("expected %q in ticket status card: %s", needle, rr.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/gateway?download="+url.QueryEscape(token), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("download status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "wolfbbs-file-content") {
		t.Fatalf("expected file body in download response, got %q", rr.Body.String())
	}
}

func TestGatewayHubRendersModernDoors(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register user: %v", err)
	}
	app := &webApp{
		authSvc:  authSvc,
		userRepo: userRepo,
		sessions: map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("create session failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/gateway", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("gateway hub status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{
		"Text Web Browser",
		"Email Gateway",
		"Generative AI",
		"Feed Reader",
		"Article Summarizer",
		"JSON Explorer",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in gateway hub body: %s", needle, body)
		}
	}
}

func TestAdminGatewaysPersistsAISettingsAndGatewayView(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	csrf := app.sessions[sid].csrf

	form := url.Values{}
	form.Set("smtp_host", "smtp.example.com")
	form.Set("smtp_port", "2525")
	form.Set("smtp_user", "mailer")
	form.Set("smtp_pass", "secret")
	form.Set("from_domain", "bbs.example.com")
	form.Set("max_recipients", "4")
	form.Set("max_message_bytes", "4096")
	form.Set("web_timeout_sec", "12")
	form.Set("web_max_bytes", "123456")
	form.Set("ai_enabled", "1")
	form.Set("ai_base_url", "https://api.example.ai")
	form.Set("ai_model", "gpt-unit")
	form.Set("ai_api_key", "unit-secret")
	form.Set("ai_system_prompt", "Be concise.")
	form.Set("ai_timeout_sec", "22")
	form.Set("ai_max_tokens", "333")
	form.Set("csrf_token", csrf)
	req := httptest.NewRequest(http.MethodPost, "/admin/gateways", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, app.handleAdminGateways).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("admin gateways post status = %d", rr.Code)
	}
	if got := rr.Header().Get("Location"); !strings.Contains(got, "/admin/gateways?notice=") {
		t.Fatalf("expected notice redirect, got %q", got)
	}

	if cfg, err := adminRepo.GetGatewaySettings(); err != nil || cfg == nil {
		t.Fatalf("expected gateway settings saved, cfg=%+v err=%v", cfg, err)
	} else if cfg.SMTPHost != "smtp.example.com" || cfg.SMTPPort != 2525 || cfg.FromDomain != "bbs.example.com" {
		t.Fatalf("unexpected smtp cfg: %+v", *cfg)
	}
	for key, want := range map[string]string{
		sysSettingGatewayAIBaseURL:      "https://api.example.ai",
		sysSettingGatewayAIModel:        "gpt-unit",
		sysSettingGatewayAIAPIKey:       "unit-secret",
		sysSettingGatewayAISystemPrompt: "Be concise.",
		sysSettingGatewayAITimeoutSec:   "22",
		sysSettingGatewayAIMaxTokens:    "333",
	} {
		got, err := adminRepo.GetSystemSetting(key)
		if err != nil {
			t.Fatalf("missing system setting %s: %v", key, err)
		}
		if strings.TrimSpace(got) != want {
			t.Fatalf("system setting %s mismatch: got %q want %q", key, got, want)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/gateway?view=ai", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("gateway ai view status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"Generative AI Door", "gpt-unit", "https://api.example.ai"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in ai view body: %s", needle, body)
		}
	}
}

func TestMailExternalSendUsesAdminGatewayLimits(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if err := authSvc.SetVerified("caller", true); err != nil {
		t.Fatalf("set verified: %v", err)
	}
	if err := adminRepo.UpsertGatewaySettings(&domain.GatewaySettings{
		MaxRecipients:   3,
		MaxMessageBytes: 32,
		WebTimeoutSec:   10,
		WebMaxBytes:     2 * 1024 * 1024,
	}); err != nil {
		t.Fatalf("seed gateway settings: %v", err)
	}
	app := &webApp{
		authSvc:              authSvc,
		userRepo:             userRepo,
		adminRepo:            adminRepo,
		mailRepo:             mailRepo,
		sessions:             map[string]sessionState{},
		requireVerifiedEmail: false,
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	csrf := app.sessions[sid].csrf

	form := url.Values{}
	form.Set("to", "outside@example.net")
	form.Set("subject", "hello")
	form.Set("body", strings.Repeat("x", 80))
	form.Set("csrf_token", csrf)
	req := httptest.NewRequest(http.MethodPost, "/mail", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect on relay error, got %d", rr.Code)
	}
	location := rr.Header().Get("Location")
	decoded, _ := url.QueryUnescape(location)
	if !strings.Contains(strings.ToLower(decoded), "message exceeds 32 bytes") {
		t.Fatalf("expected max message bytes error from admin gateway config, location=%q decoded=%q", location, decoded)
	}
}

func TestAdminGatewaysRejectsUnsafeAIBaseURLByDefault(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}

	app := &webApp{
		authSvc:    authSvc,
		adminRepo:  adminRepo,
		sessions:   map[string]sessionState{},
		rateLimits: map[string][]time.Time{},
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}

	form := url.Values{}
	form.Set("smtp_host", "smtp.example.com")
	form.Set("smtp_port", "587")
	form.Set("from_domain", "bbs.example.com")
	form.Set("ai_enabled", "1")
	form.Set("ai_base_url", "http://127.0.0.1:11434")
	form.Set("ai_model", "gpt-unit")
	form.Set("ai_api_key", "unit-secret")
	form.Set("csrf_token", app.sessions[sid].csrf)

	req := httptest.NewRequest(http.MethodPost, "/admin/gateways", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, app.handleAdminGateways).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("admin gateways post status = %d", rr.Code)
	}
	if got := rr.Header().Get("Location"); !strings.Contains(got, "/admin/gateways?error=") {
		t.Fatalf("expected error redirect, got %q", got)
	}
	if got, err := adminRepo.GetSystemSetting(sysSettingGatewayAIBaseURL); err == nil && strings.TrimSpace(got) != "" {
		t.Fatalf("expected unsafe ai base url to be rejected, got persisted value %q", got)
	}
}

func TestChatChannelLockEnforcedForNonModerators(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if _, err := authSvc.Register("reader", "password123"); err != nil {
		t.Fatalf("register reader: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}

	app := &webApp{
		authSvc:       authSvc,
		userRepo:      userRepo,
		sessions:      map[string]sessionState{},
		chatSvc:       chat.NewServiceForTest(),
		lockedChat:    map[string]bool{},
		savedSearches: map[string][]string{},
	}
	sysopSID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("sysop session failed")
	}
	readerSID, ok := app.createSession("reader")
	if !ok {
		t.Fatal("reader session failed")
	}
	app.Lock()
	sysopCSRF := app.sessions[sysopSID].csrf
	readerCSRF := app.sessions[readerSID].csrf
	app.Unlock()

	lockForm := url.Values{}
	lockForm.Set("action", "lock_channel")
	lockForm.Set("channel", "#lobby")
	lockForm.Set("csrf_token", sysopCSRF)
	req := httptest.NewRequest(http.MethodPost, "/admin/chat", strings.NewReader(lockForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminChat)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("admin lock status = %d", rr.Code)
	}

	sendBody := map[string]string{"channel": "#lobby", "message": "hello"}
	raw, _ := json.Marshal(sendBody)
	req = httptest.NewRequest(http.MethodPost, "/chat/send", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", readerCSRF)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: readerSID})
	rr = httptest.NewRecorder()
	app.handleChatSend(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected locked channel send to be forbidden, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/chat/send", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", sysopCSRF)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.handleChatSend(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected sysop send to pass on locked channel, got %d", rr.Code)
	}
}

func TestBoardsACSAndPointers(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	authSvc := auth.NewService(userRepo)
	user, err := authSvc.Register("reader", "password123")
	if err != nil {
		t.Fatalf("register reader: %v", err)
	}
	if _, err := authSvc.Register("mod", "password123"); err != nil {
		t.Fatalf("register mod: %v", err)
	}
	if err := authSvc.SetRole("mod", roleModerator); err != nil {
		t.Fatalf("set moderator role: %v", err)
	}
	moderator, err := authSvc.GetUser("mod")
	if err != nil {
		t.Fatalf("get moderator: %v", err)
	}

	restricted := &domain.Board{Name: "Staff", Conference: "Ops", ReadACS: "role=moderator", WriteACS: "role=moderator", CreatedBy: moderator.ID}
	if err := boardRepo.Create(restricted); err != nil {
		t.Fatalf("create restricted board: %v", err)
	}
	general := &domain.Board{Name: "General", Conference: "Public", ReadACS: "role=user", WriteACS: "verified", CreatedBy: moderator.ID}
	if err := boardRepo.Create(general); err != nil {
		t.Fatalf("create general board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:  general.ID,
		AuthorID: moderator.ID,
		Subject:  "Welcome",
		Body:     "hello callers",
	}); err != nil {
		t.Fatalf("create seed message: %v", err)
	}
	seedMsgs, err := msgRepo.ListByBoard(general.ID)
	if err != nil || len(seedMsgs) != 1 {
		t.Fatalf("seed messages: msgs=%#v err=%v", seedMsgs, err)
	}

	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		sessions:  map[string]sessionState{},
		chatSvc:   chat.NewServiceForTest(),
	}
	sid, ok := app.createSession("reader")
	if !ok {
		t.Fatal("session creation failed")
	}
	app.Lock()
	csrf := app.sessions[sid].csrf
	app.Unlock()

	// User cannot read moderator-only board.
	req := httptest.NewRequest(http.MethodGet, "/boards?board="+strconv.FormatInt(restricted.ID, 10), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for restricted board read, got %d", rr.Code)
	}

	// User can read general board but cannot post until verified.
	req = httptest.NewRequest(http.MethodGet, "/boards?board="+strconv.FormatInt(general.ID, 10), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for readable board, got %d", rr.Code)
	}
	for _, want := range []string{"Board scan", "Posting flow"} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("board view missing %q: %s", want, rr.Body.String())
		}
	}

	form := url.Values{}
	form.Set("board_id", strconv.FormatInt(general.ID, 10))
	form.Set("subject", "post without verify")
	form.Set("body", "not allowed")
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for write ACS denial, got %d", rr.Code)
	}

	if err := authSvc.SetVerified(user.Handle, true); err != nil {
		t.Fatalf("set verified: %v", err)
	}
	form.Set("subject", "verified post")
	form.Set("body", "allowed now")
	req = httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect after allowed post, got %d", rr.Code)
	}

	// Opening a message view updates per-board pointer.
	req = httptest.NewRequest(http.MethodGet, "/boards?board="+strconv.FormatInt(general.ID, 10)+"&id="+strconv.FormatInt(seedMsgs[0].ID, 10), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for reader view, got %d", rr.Code)
	}
	for _, want := range []string{"Reading thread #", "Moderation path", `data-draft-key="board-`} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("board reader missing %q: %s", want, rr.Body.String())
		}
	}
	ptr, err := msgRepo.GetPointer(user.ID, general.ID)
	if err != nil {
		t.Fatalf("expected pointer after reader view: %v", err)
	}
	if ptr.LastReadID != seedMsgs[0].ID {
		t.Fatalf("pointer last read id = %d, want %d", ptr.LastReadID, seedMsgs[0].ID)
	}
}

func TestBoardsCreateReportFromReader(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	authSvc := auth.NewService(userRepo)
	author, err := authSvc.Register("author", "password123")
	if err != nil {
		t.Fatalf("register author: %v", err)
	}
	reporter, err := authSvc.Register("reporter", "password123")
	if err != nil {
		t.Fatalf("register reporter: %v", err)
	}
	board := &domain.Board{Name: "General", Conference: "Public", ReadACS: "role=user", WriteACS: "role=user", CreatedBy: author.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	msg := &domain.Message{BoardID: board.ID, AuthorID: author.ID, Subject: "Welcome", Body: "hello"}
	if err := msgRepo.CreateMessage(msg); err != nil {
		t.Fatalf("create message: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession(reporter.Handle)
	if !ok {
		t.Fatal("session creation failed")
	}
	app.Lock()
	csrf := app.sessions[sid].csrf
	app.Unlock()

	form := url.Values{}
	form.Set("action", "report")
	form.Set("board_id", strconv.FormatInt(board.ID, 10))
	form.Set("message_id", strconv.FormatInt(msg.ID, 10))
	form.Set("reason", "spam")
	form.Set("csrf_token", csrf)
	req := httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect after report, got %d", rr.Code)
	}
	reports, err := msgRepo.ListReports(10, "open")
	if err != nil {
		t.Fatalf("list reports: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected one report, got %d", len(reports))
	}
	if reports[0].MessageID != msg.ID || reports[0].ReporterID != reporter.ID || reports[0].Reason != "spam" {
		t.Fatalf("unexpected report row: %+v", reports[0])
	}
}

func TestAdminBoardsModerationActions(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	authSvc := auth.NewService(userRepo)
	sysop, err := authSvc.Register("sysop", "password123")
	if err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	poster, err := authSvc.Register("poster", "password123")
	if err != nil {
		t.Fatalf("register poster: %v", err)
	}
	reporter, err := authSvc.Register("reporter", "password123")
	if err != nil {
		t.Fatalf("register reporter: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	source := &domain.Board{Name: "General", Conference: "Public", ReadACS: "role=user", WriteACS: "role=user", CreatedBy: sysop.ID}
	if err := boardRepo.Create(source); err != nil {
		t.Fatalf("create source board: %v", err)
	}
	target := &domain.Board{Name: "Ops", Conference: "Ops", ReadACS: "role=user", WriteACS: "role=user", CreatedBy: sysop.ID}
	if err := boardRepo.Create(target); err != nil {
		t.Fatalf("create target board: %v", err)
	}
	msg := &domain.Message{BoardID: source.ID, AuthorID: poster.ID, Subject: "Thread", Body: "body"}
	if err := msgRepo.CreateMessage(msg); err != nil {
		t.Fatalf("create message: %v", err)
	}
	if err := msgRepo.CreateReport(&domain.MessageReport{
		MessageID:  msg.ID,
		ReporterID: reporter.ID,
		Reason:     "abuse",
		Status:     "open",
	}); err != nil {
		t.Fatalf("create report: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	app.Lock()
	csrf := app.sessions[sid].csrf
	app.Unlock()

	getReq := httptest.NewRequest(http.MethodGet, "/admin/boards?manage_board="+strconv.FormatInt(source.ID, 10), nil)
	getReq.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	getRR := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminBoards)).ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("admin boards get status = %d", getRR.Code)
	}
	body := getRR.Body.String()
	if !strings.Contains(body, "Moderation Queue") || !strings.Contains(body, "abuse") {
		t.Fatalf("expected moderation queue with report details, body=%s", body)
	}

	lockForm := url.Values{}
	lockForm.Set("action", "lock_thread")
	lockForm.Set("thread_id", strconv.FormatInt(msg.ThreadID, 10))
	lockForm.Set("redirect_to", "/admin/boards?manage_board="+strconv.FormatInt(source.ID, 10))
	lockForm.Set("csrf_token", csrf)
	lockReq := httptest.NewRequest(http.MethodPost, "/admin/boards", strings.NewReader(lockForm.Encode()))
	lockReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	lockReq.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	lockRR := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminBoards)).ServeHTTP(lockRR, lockReq)
	if lockRR.Code != http.StatusFound {
		t.Fatalf("lock thread status = %d", lockRR.Code)
	}
	locked, err := msgRepo.IsThreadLocked(msg.ThreadID)
	if err != nil {
		t.Fatalf("is thread locked: %v", err)
	}
	if !locked {
		t.Fatal("expected thread to be locked")
	}

	reports, err := msgRepo.ListReports(10, "open")
	if err != nil || len(reports) != 1 {
		t.Fatalf("list open reports: len=%d err=%v", len(reports), err)
	}
	resolveForm := url.Values{}
	resolveForm.Set("action", "resolve_report")
	resolveForm.Set("report_id", strconv.FormatInt(reports[0].ID, 10))
	resolveForm.Set("redirect_to", "/admin/boards?manage_board="+strconv.FormatInt(source.ID, 10))
	resolveForm.Set("csrf_token", csrf)
	resolveReq := httptest.NewRequest(http.MethodPost, "/admin/boards", strings.NewReader(resolveForm.Encode()))
	resolveReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resolveReq.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	resolveRR := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminBoards)).ServeHTTP(resolveRR, resolveReq)
	if resolveRR.Code != http.StatusFound {
		t.Fatalf("resolve report status = %d", resolveRR.Code)
	}
	resolved, err := msgRepo.ListReports(10, "resolved")
	if err != nil || len(resolved) != 1 {
		t.Fatalf("resolved reports: len=%d err=%v", len(resolved), err)
	}

	moveForm := url.Values{}
	moveForm.Set("action", "move_thread")
	moveForm.Set("thread_id", strconv.FormatInt(msg.ThreadID, 10))
	moveForm.Set("to_board_id", strconv.FormatInt(target.ID, 10))
	moveForm.Set("redirect_to", "/admin/boards?manage_board="+strconv.FormatInt(target.ID, 10))
	moveForm.Set("csrf_token", csrf)
	moveReq := httptest.NewRequest(http.MethodPost, "/admin/boards", strings.NewReader(moveForm.Encode()))
	moveReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	moveReq.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	moveRR := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminBoards)).ServeHTTP(moveRR, moveReq)
	if moveRR.Code != http.StatusFound {
		t.Fatalf("move thread status = %d", moveRR.Code)
	}
	movedMsg, err := msgRepo.GetMessage(msg.ID)
	if err != nil {
		t.Fatalf("get moved message: %v", err)
	}
	if movedMsg.BoardID != target.ID {
		t.Fatalf("message board id = %d, want %d", movedMsg.BoardID, target.ID)
	}

	deleteForm := url.Values{}
	deleteForm.Set("action", "delete_message")
	deleteForm.Set("message_id", strconv.FormatInt(msg.ID, 10))
	deleteForm.Set("reason", "rule violation")
	deleteForm.Set("redirect_to", "/admin/boards?manage_board="+strconv.FormatInt(target.ID, 10))
	deleteForm.Set("csrf_token", csrf)
	deleteReq := httptest.NewRequest(http.MethodPost, "/admin/boards", strings.NewReader(deleteForm.Encode()))
	deleteReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	deleteReq.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	deleteRR := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminBoards)).ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusFound {
		t.Fatalf("delete message status = %d", deleteRR.Code)
	}
	if _, err := msgRepo.GetMessage(msg.ID); err == nil {
		t.Fatal("expected deleted message to be missing")
	}
}

func TestAdminBoardsMacrosAndThreadLifecycle(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	sysop, _ := authSvc.Register("sysop", "password123")
	poster, _ := authSvc.Register("poster", "password123")
	reporter, _ := authSvc.Register("reporter", "password123")
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	source := &domain.Board{Name: "General", Conference: "Public", ReadACS: "role=user", WriteACS: "role=user", CreatedBy: sysop.ID}
	target := &domain.Board{Name: "Ops", Conference: "Ops", ReadACS: "role=user", WriteACS: "role=user", CreatedBy: sysop.ID}
	if err := boardRepo.Create(source); err != nil {
		t.Fatalf("create source board: %v", err)
	}
	if err := boardRepo.Create(target); err != nil {
		t.Fatalf("create target board: %v", err)
	}
	spamMsg := &domain.Message{BoardID: source.ID, AuthorID: poster.ID, Subject: "Spam Thread", Body: "junk"}
	if err := msgRepo.CreateMessage(spamMsg); err != nil {
		t.Fatalf("create spam msg: %v", err)
	}
	if err := msgRepo.CreateReport(&domain.MessageReport{MessageID: spamMsg.ID, ReporterID: reporter.ID, Reason: "spam"}); err != nil {
		t.Fatalf("create spam report: %v", err)
	}
	frozenMsg := &domain.Message{BoardID: source.ID, AuthorID: poster.ID, Subject: "Heated Thread", Body: "body"}
	if err := msgRepo.CreateMessage(frozenMsg); err != nil {
		t.Fatalf("create frozen msg: %v", err)
	}
	if err := msgRepo.CreateReport(&domain.MessageReport{MessageID: frozenMsg.ID, ReporterID: reporter.ID, Reason: "abuse"}); err != nil {
		t.Fatalf("create abuse report: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	adminSID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("admin session creation failed")
	}
	adminCSRF := app.sessions[adminSID].csrf

	reports, err := msgRepo.ListReports(20, "open")
	if err != nil || len(reports) != 2 {
		t.Fatalf("list reports: len=%d err=%v", len(reports), err)
	}
	spamReportID := reports[0].ID
	abuseReportID := reports[1].ID
	if reports[0].MessageID != spamMsg.ID {
		spamReportID, abuseReportID = abuseReportID, spamReportID
	}

	form := url.Values{
		"csrf_token":  {adminCSRF},
		"action":      {"resolve_report_macro"},
		"report_id":   {strconv.FormatInt(spamReportID, 10)},
		"macro":       {"spam_delete"},
		"redirect_to": {"/admin/boards?manage_board=" + strconv.FormatInt(source.ID, 10)},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: adminSID})
	rr := httptest.NewRecorder()
	app.handleAdminBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("spam macro status = %d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := msgRepo.GetMessage(spamMsg.ID); err == nil {
		t.Fatal("expected spam macro to delete message")
	}

	form = url.Values{
		"csrf_token":  {adminCSRF},
		"action":      {"resolve_report_macro"},
		"report_id":   {strconv.FormatInt(abuseReportID, 10)},
		"macro":       {"abuse_lock"},
		"redirect_to": {"/admin/boards?manage_board=" + strconv.FormatInt(source.ID, 10)},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: adminSID})
	rr = httptest.NewRecorder()
	app.handleAdminBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("abuse macro status = %d body=%s", rr.Code, rr.Body.String())
	}
	if state := app.threadLifecycleStateFor(frozenMsg.ThreadID); state != threadLifecycleFrozen {
		t.Fatalf("thread state = %s, want frozen", state)
	}

	posterSID, ok := app.createSession("poster")
	if !ok {
		t.Fatal("poster session creation failed")
	}
	req = httptest.NewRequest(http.MethodGet, "/boards?board="+strconv.FormatInt(source.ID, 10)+"&id="+strconv.FormatInt(frozenMsg.ID, 10), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: posterSID})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("frozen reader status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"Frozen thread: read-only", "Editing is disabled because this thread is frozen.", "Replying is disabled because this thread is frozen."} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in frozen reader: %s", needle, body)
		}
	}

	form = url.Values{
		"csrf_token":   {adminCSRF},
		"action":       {"set_thread_state"},
		"thread_id":    {strconv.FormatInt(frozenMsg.ThreadID, 10)},
		"thread_state": {"archived"},
		"redirect_to":  {"/admin/boards?manage_board=" + strconv.FormatInt(source.ID, 10)},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: adminSID})
	rr = httptest.NewRecorder()
	app.handleAdminBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("set archived state status = %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/boards?board="+strconv.FormatInt(source.ID, 10), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: posterSID})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if strings.Contains(rr.Body.String(), "Heated Thread") {
		t.Fatalf("expected archived thread hidden from default board view: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/boards?board="+strconv.FormatInt(source.ID, 10)+"&show_archived=1", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: posterSID})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if !strings.Contains(rr.Body.String(), "Heated Thread") {
		t.Fatalf("expected archived thread visible when requested: %s", rr.Body.String())
	}
}

func TestBoardsWelcomeKitStewardsStaffNotesAndEscalation(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	sysop, _ := authSvc.Register("sysop", "password123")
	moderator, _ := authSvc.Register("mod", "password123")
	caller, _ := authSvc.Register("caller", "password123")
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	if err := authSvc.SetRole("mod", roleModerator); err != nil {
		t.Fatalf("set mod role: %v", err)
	}
	board := &domain.Board{Name: "LaunchPad", Conference: "Public", CreatedBy: sysop.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sysopSID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("sysop session creation failed")
	}
	sysopCSRF := app.sessions[sysopSID].csrf

	form := url.Values{
		"csrf_token":      {sysopCSRF},
		"action":          {"save_board_welcome"},
		"board_id":        {strconv.FormatInt(board.ID, 10)},
		"redirect_to":     {"/admin/boards?manage_board=" + strconv.FormatInt(board.ID, 10)},
		"intro":           {"Start here if you are new to this conference."},
		"seed_prompts":    {"Introduce yourself\nWhat should this board become?"},
		"starter_threads": {"Classic thread kickoff\nLaunch checklist"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr := httptest.NewRecorder()
	app.handleAdminBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save welcome kit status = %d", rr.Code)
	}

	form = url.Values{
		"csrf_token":  {sysopCSRF},
		"action":      {"save_board_stewards"},
		"board_id":    {strconv.FormatInt(board.ID, 10)},
		"redirect_to": {"/admin/boards?manage_board=" + strconv.FormatInt(board.ID, 10)},
		"stewards":    {"sysop | onboarding | first-callers\nmod | archives | keeps classics tidy"},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.handleAdminBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save stewards status = %d", rr.Code)
	}

	form = url.Values{
		"csrf_token":  {sysopCSRF},
		"action":      {"save_board_staff_note"},
		"board_id":    {strconv.FormatInt(board.ID, 10)},
		"redirect_to": {"/admin/boards?manage_board=" + strconv.FormatInt(board.ID, 10)},
		"staff_note":  {"Needs a steady moderator eye during launch."},
		"escalate":    {"1"},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.handleAdminBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save staff note status = %d", rr.Code)
	}

	callerSID, ok := app.createSession(caller.Handle)
	if !ok {
		t.Fatal("caller session creation failed")
	}
	req = httptest.NewRequest(http.MethodGet, "/boards?board="+strconv.FormatInt(board.ID, 10), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	body := rr.Body.String()
	for _, needle := range []string{"Board Welcome Kit", "Start here if you are new to this conference.", "Introduce yourself", "Topic Stewards", "first-callers"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in caller board view: %s", needle, body)
		}
	}
	if strings.Contains(body, "Needs a steady moderator eye during launch.") {
		t.Fatalf("caller should not see staff note: %s", body)
	}

	modSID, ok := app.createSession(moderator.Handle)
	if !ok {
		t.Fatal("mod session creation failed")
	}
	req = httptest.NewRequest(http.MethodGet, "/boards?board="+strconv.FormatInt(board.ID, 10), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: modSID})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if !strings.Contains(rr.Body.String(), "Needs a steady moderator eye during launch.") {
		t.Fatalf("moderator should see staff note: %s", rr.Body.String())
	}
	escalations := app.loadStaffEscalations()
	if len(escalations) != 1 || !strings.Contains(escalations[0].Handle, "board:LaunchPad") {
		t.Fatalf("expected board escalation, got %#v", escalations)
	}
}

func TestBoardsPollVotingAndRevisionHistory(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	caller, _ := authSvc.Register("caller", "password123")
	board := &domain.Board{Name: "Votes", Conference: "Public", ReadACS: "role=user", WriteACS: "role=user", CreatedBy: caller.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession(caller.Handle)
	if !ok {
		t.Fatal("session creation failed")
	}
	csrf := app.sessions[sid].csrf

	form := url.Values{
		"csrf_token":    {csrf},
		"action":        {"create_poll"},
		"board_id":      {strconv.FormatInt(board.ID, 10)},
		"subject":       {"Tournament Vote"},
		"poll_question": {"What should we run on Friday night?"},
		"poll_context":  {"Pick the strongest return hook."},
		"poll_options":  {"Trivia\nTradeWars\nLORD"},
	}
	req := httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("create poll status = %d body=%s", rr.Code, rr.Body.String())
	}
	msgs, err := msgRepo.ListByBoard(board.ID)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("list board messages: len=%d err=%v", len(msgs), err)
	}
	root := msgs[0]

	req = httptest.NewRequest(http.MethodGet, "/boards?board="+strconv.FormatInt(board.ID, 10)+"&id="+strconv.FormatInt(root.ID, 10), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if !strings.Contains(rr.Body.String(), "What should we run on Friday night?") {
		t.Fatalf("reader missing poll question: %s", rr.Body.String())
	}

	form = url.Values{
		"csrf_token":   {csrf},
		"action":       {"vote_poll"},
		"board_id":     {strconv.FormatInt(board.ID, 10)},
		"thread_id":    {strconv.FormatInt(root.ThreadID, 10)},
		"option_index": {"1"},
	}
	req = httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("vote poll status = %d", rr.Code)
	}

	form = url.Values{
		"csrf_token": {csrf},
		"action":     {"edit_post"},
		"board_id":   {strconv.FormatInt(board.ID, 10)},
		"message_id": {strconv.FormatInt(root.ID, 10)},
		"subject":    {"Tournament Vote Updated"},
		"body":       {"Updated context for the vote."},
	}
	req = httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("edit post status = %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/boards?board="+strconv.FormatInt(board.ID, 10)+"&id="+strconv.FormatInt(root.ID, 10), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	body := rr.Body.String()
	for _, needle := range []string{"your vote", "Revision History", "Tournament Vote", "Tournament Vote Updated"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in poll reader: %s", needle, body)
		}
	}
}

func TestBulletinsBestOfWeekAndDigestConferencePacks(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	authSvc := auth.NewService(userRepo)
	caller, _ := authSvc.Register("caller", "password123")
	friend, _ := authSvc.Register("friend", "password123")
	boardA := &domain.Board{Name: "General Chat", Conference: "General", ReadACS: "role=user", WriteACS: "role=user", CreatedBy: caller.ID}
	boardB := &domain.Board{Name: "Ops Notes", Conference: "Ops", ReadACS: "role=user", WriteACS: "role=user", CreatedBy: caller.ID}
	if err := boardRepo.Create(boardA); err != nil {
		t.Fatalf("create boardA: %v", err)
	}
	if err := boardRepo.Create(boardB); err != nil {
		t.Fatalf("create boardB: %v", err)
	}
	msgA := &domain.Message{BoardID: boardA.ID, AuthorID: friend.ID, Subject: "Best Thread", Body: "kickoff"}
	if err := msgRepo.CreateMessage(msgA); err != nil {
		t.Fatalf("create msgA: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{BoardID: boardA.ID, AuthorID: caller.ID, ParentID: msgA.ID, Subject: "Re: Best Thread", Body: "reply"}); err != nil {
		t.Fatalf("create reply: %v", err)
	}
	msgB := &domain.Message{BoardID: boardB.ID, AuthorID: friend.ID, Subject: "Ops Digest", Body: "ops body"}
	if err := msgRepo.CreateMessage(msgB); err != nil {
		t.Fatalf("create msgB: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		mailRepo:  mailRepo,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	app.setBoardSubscription(caller.Handle, boardA.ID, boardSubscriptionDigest)
	app.setBoardSubscription(caller.Handle, boardB.ID, boardSubscriptionDigest)
	app.persistBestOfWeekEntries([]bestOfWeekEntry{{MessageID: msgA.ID, Note: "editor's pick", AddedBy: "sysop", AddedAt: time.Now().UTC()}})

	sid, ok := app.createSession(caller.Handle)
	if !ok {
		t.Fatal("session creation failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/bulletins", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleBulletins(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("bulletins status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"Best of Week", "Best Thread", "editor's pick"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in bulletins: %s", needle, body)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/digest", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleDigest(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("digest status = %d", rr.Code)
	}
	body = rr.Body.String()
	for _, needle := range []string{"Conference Packs", "General", "Ops", "General Chat", "Ops Notes"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in digest: %s", needle, body)
		}
	}
}

func TestAdminBoardsImportLegacyArchive(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	sysop, _ := authSvc.Register("sysop", "password123")
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	board := &domain.Board{Name: "Imported", Conference: "General", ReadACS: "role=user", WriteACS: "role=user", CreatedBy: sysop.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	csrf := app.sessions[sid].csrf
	archive := "Subject: Welcome Back\nFrom: oldsysop\nDate: 2025-04-01 20:15\n\nFirst imported body.\n---\nSubject: Another Thread\n\nSecond imported body."
	form := url.Values{
		"csrf_token":      {csrf},
		"action":          {"import_legacy_archive"},
		"import_board_id": {strconv.FormatInt(board.ID, 10)},
		"redirect_to":     {"/admin/boards?manage_board=" + strconv.FormatInt(board.ID, 10)},
		"archive_blob":    {archive},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleAdminBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("import archive status = %d body=%s", rr.Code, rr.Body.String())
	}
	msgs, err := msgRepo.ListByBoard(board.ID)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("imported messages: len=%d err=%v", len(msgs), err)
	}
	if !strings.Contains(msgs[0].Body, "Imported From: oldsysop") {
		t.Fatalf("expected imported metadata in first message body: %s", msgs[0].Body)
	}
	if msgs[1].Subject != "Another Thread" {
		t.Fatalf("unexpected second imported subject: %+v", msgs[1])
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
	var rawPayload map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &rawPayload); err != nil {
		t.Fatalf("history raw decode: %v", err)
	}
	rawMsgs, ok := rawPayload["messages"].([]interface{})
	if !ok || len(rawMsgs) == 0 {
		t.Fatalf("expected messages array in raw payload, got %#v", rawPayload["messages"])
	}
	lastRaw, ok := rawMsgs[len(rawMsgs)-1].(map[string]interface{})
	if !ok {
		t.Fatalf("expected message object, got %#v", rawMsgs[len(rawMsgs)-1])
	}
	for _, key := range []string{"from", "body", "created_at", "channel"} {
		if _, ok := lastRaw[key]; !ok {
			t.Fatalf("missing wire key %q in message payload: %#v", key, lastRaw)
		}
	}
	for _, key := range []string{"From", "Body", "CreatedAt", "Channel"} {
		if _, ok := lastRaw[key]; ok {
			t.Fatalf("unexpected Go struct field key %q leaked into payload: %#v", key, lastRaw)
		}
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

func TestWriteMessageEventWireSchema(t *testing.T) {
	rr := httptest.NewRecorder()
	msg := chat.Message{
		ID:        11,
		Channel:   "#lobby",
		From:      "alice",
		Body:      "wire schema check",
		CreatedAt: time.Date(2026, 2, 28, 13, 21, 22, 0, time.UTC),
	}
	if err := writeMessageEvent(rr, msg); err != nil {
		t.Fatalf("writeMessageEvent: %v", err)
	}
	body := rr.Body.String()
	if !strings.HasPrefix(body, "data: ") {
		t.Fatalf("expected SSE data prefix, got %q", body)
	}
	raw := strings.TrimSpace(strings.TrimPrefix(body, "data: "))
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode SSE payload: %v", err)
	}
	if payload["from"] != "alice" {
		t.Fatalf("expected from=alice, got %#v", payload["from"])
	}
	if payload["body"] != "wire schema check" {
		t.Fatalf("expected body value, got %#v", payload["body"])
	}
	if payload["created_at"] != "13:21:22" {
		t.Fatalf("expected created_at=13:21:22, got %#v", payload["created_at"])
	}
	if _, ok := payload["From"]; ok {
		t.Fatalf("unexpected Go struct field key leaked into SSE payload: %#v", payload)
	}
}

func TestWithModernUIInjectsStylesIntoHTML(t *testing.T) {
	app := &webApp{}
	handler := app.withModernUI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<!doctype html><html><body><h1>Hello</h1></body></html>`))
	}))
	req := httptest.NewRequest(http.MethodGet, "/boards", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `id="wolfbbs-modern-ui"`) {
		t.Fatalf("expected modern UI styles to be injected, got %q", body)
	}
	if !strings.Contains(body, `<h1>Hello</h1>`) {
		t.Fatalf("expected original content to remain, got %q", body)
	}
}

func TestWithModernUIDoesNotTouchJSON(t *testing.T) {
	app := &webApp{}
	handler := app.withModernUI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	req := httptest.NewRequest(http.MethodGet, "/statusz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
	if body := strings.TrimSpace(rr.Body.String()); body != `{"ok":true}` {
		t.Fatalf("expected JSON body to be unchanged, got %q", body)
	}
}

func TestWithModernUISkipsChatStreamPath(t *testing.T) {
	app := &webApp{}
	handler := app.withModernUI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if _, ok := w.(http.Flusher); !ok {
			t.Fatal("expected flusher for stream response")
		}
		_, _ = w.Write([]byte("data: {\"from\":\"alice\",\"body\":\"hello\"}\n\n"))
		w.(http.Flusher).Flush()
	}))
	req := httptest.NewRequest(http.MethodGet, "/chat/stream?channel=%23lobby", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, "wolfbbs-modern-ui") {
		t.Fatalf("expected no style injection for stream endpoint, got %q", body)
	}
	if !strings.Contains(body, `"from":"alice"`) {
		t.Fatalf("expected stream payload to pass through, got %q", body)
	}
}

func TestModernUIBootstrapIncludesAdvancedUXPassFeatures(t *testing.T) {
	for _, needle := range []string{
		"wolfbbs-breadcrumbs",
		"wolfbbs-pref-controls",
		"wolfbbs-table-toolbar",
		"wolfbbs-form-actions-sticky",
		"wolfbbsBackToTop",
		"wolfbbsScrollProgress",
		"toggleFavoriteCurrentRoute",
		"downloadTableCSV",
		"mountSectionToggles",
		"enhanceFieldValidation",
	} {
		if !strings.Contains(modernUIBootstrap, needle) {
			t.Fatalf("expected modern UI bootstrap to include %q", needle)
		}
	}
}

func TestModernUIBootstrapIncludesNextTenFeatureHooks(t *testing.T) {
	for _, needle := range []string{
		"goalCoachRegistry",
		"mountGoalCoach",
		"tableViewPrefix",
		"saveTableView",
		"wolfbbsNotesButton",
		"mountQuickNotesWorkspace",
		"mountRouteScorecard",
		"enhanceEmptyStates",
		"mountMacroHelp",
		"attachFocusTimerUI",
		"enhanceComposeTemplates",
		"mountReplayBanner",
		"telemetryKey",
		"trackTelemetry",
		"downloadJSONFile(\"wolfbbs-ux-telemetry.json\"",
	} {
		if !strings.Contains(modernUIBootstrap, needle) {
			t.Fatalf("expected modern UI bootstrap to include %q", needle)
		}
	}
}

func TestModernUIBootstrapIncludesRoundTwoTwentyUXHooks(t *testing.T) {
	for _, needle := range []string{
		"ensureLiveRegion",
		"announceLive",
		"paletteHistoryKey",
		"savePaletteHistoryEntry",
		"mountNetworkStatusChip",
		"mountLatencyChip",
		"Auto refresh",
		"wolfbbs-column-toggle-panel",
		"Copy JSON",
		"mountGuideMinimizeControl",
		"wolfbbs-section-progress",
		"mountShortcutLegendOverlay",
		"wolfbbs-text-counter",
		"form:required-missing",
		"wolfbbsUXDiagButton",
		"applyFocusMode",
		"focusModeKey",
		"trackTelemetry(\"route:view\")",
		"downloadJSONFile(\"wolfbbs-ui-diagnostics.json\"",
		"mountTelemetryChip",
	} {
		if !strings.Contains(modernUIBootstrap, needle) {
			t.Fatalf("expected modern UI bootstrap to include %q", needle)
		}
	}
}

func TestModernUIBootstrapIncludesRoundThreeNextTenUXHooks(t *testing.T) {
	for _, needle := range []string{
		"toastHistoryKey",
		"undoStackKey",
		"workspaceKey",
		"revisitKeyPrefix",
		"kpiSnapshotKeyPrefix",
		"spotlightKey",
		"checkpointKey",
		"applyGlossaryEnhancer",
		"mountRevisitBanner",
		"mountKPIDeltas",
		"mountToastCenter",
		"mountWorkspaceHub",
		"buildHandoffMarkdown",
		"mountSpotlightSearch",
		"mountCheckpointHub",
		"pushUndoAction",
		"popUndoAction",
		"wolfbbs-selected-row",
		"Export selected",
		"Copy JSON",
		"wolfbbsOpenWorkspaceHub",
		"wolfbbsOpenToastCenter",
		"wolfbbsOpenCheckpointHub",
		"wolfbbs-spotlight-hit",
		"Last visited this route",
		"Handoff markdown copied",
		"Sections 0/",
	} {
		if !strings.Contains(modernUIBootstrap, needle) {
			t.Fatalf("expected modern UI bootstrap to include %q", needle)
		}
	}
}

func TestModernUIBootstrapIncludesRoundFourNextTenUXHooks(t *testing.T) {
	for _, needle := range []string{
		"draftKeyPrefix",
		"incidentKey",
		"playbookKeyPrefix",
		"reminderKey",
		"releaseGateKey",
		"feedbackKey",
		"kpiWatchKeyPrefix",
		"syncChannelName",
		"wolfbbsSyncChannel",
		"mountCrossTabSync",
		"mountDraftCenter",
		"mountKPIWatchCenter",
		"mountIncidentConsole",
		"mountPlaybookRunner",
		"mountReminderScheduler",
		"mountReleaseGate",
		"mountFeedbackPulse",
		"Draft autosave is available.",
		"Select visible",
		"Invert selected",
		"Copy selected JSON",
		"KPI Watch Center",
		"Incident Console",
		"Playbook Runner",
		"Reminder Scheduler",
		"Release Gate",
		"Feedback Pulse",
		"Ctrl/Cmd+Shift+D",
	} {
		if !strings.Contains(modernUIBootstrap, needle) {
			t.Fatalf("expected modern UI bootstrap to include %q", needle)
		}
	}
}

func TestHandleChatPageSupportsLegacyAndCurrentMessageKeys(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register user: %v", err)
	}
	app := &webApp{
		authSvc:  authSvc,
		chatSvc:  chat.NewServiceForTest(),
		sessions: map[string]sessionState{},
	}
	sessionID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sessionID})
	rr := httptest.NewRecorder()
	app.handleChat(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("chat page status = %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "msgValue(m, 'created_at', 'CreatedAt'") {
		t.Fatalf("expected created_at fallback logic in chat page script, got %q", body)
	}
	if !strings.Contains(body, "msgValue(m, 'from', 'From'") {
		t.Fatalf("expected from fallback logic in chat page script")
	}
	if !strings.Contains(body, "msgValue(m, 'body', 'Body'") {
		t.Fatalf("expected body fallback logic in chat page script")
	}
	for _, needle := range []string{
		"Open Channel",
		"Reconnect Stream",
		"streamState.lastSeenId",
		"const canModerate =",
		"This room is locked for non-moderators.",
		"normalizeIncomingMessage",
		"presenceValue(row, 'nick', 'Nick'",
		"chatStatePill",
		"updateMessageCount()",
		"currentHandle",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected chat page to include %q, got %q", needle, body)
		}
	}
}

func TestHandleChatStreamMembershipLifecycle(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	app := &webApp{
		authSvc:  authSvc,
		chatSvc:  chat.NewServiceForTest(),
		sessions: map[string]sessionState{},
	}
	sessionID, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}

	waitPresenceCount := func(channel string, want int) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if got := len(app.chatSvc.OnlineInChannel(channel)); got == want {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("presence count for %s did not reach %d; got %+v", channel, want, app.chatSvc.OnlineInChannel(channel))
	}

	t.Run("ephemeral stream join leaves on disconnect", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/chat/stream?channel=%23lobby", nil)
		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()
		req = req.WithContext(ctx)
		req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sessionID})
		rr := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			app.handleChatStream(rr, req)
			close(done)
		}()
		waitPresenceCount("#lobby", 1)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("stream handler did not exit after context cancel")
		}
		waitPresenceCount("#lobby", 0)
	})

	t.Run("prejoined membership survives stream disconnect", func(t *testing.T) {
		app.chatSvc.JoinChannel("caller", "#lobby")
		req := httptest.NewRequest(http.MethodGet, "/chat/stream?channel=%23lobby", nil)
		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()
		req = req.WithContext(ctx)
		req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sessionID})
		rr := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			app.handleChatStream(rr, req)
			close(done)
		}()
		waitPresenceCount("#lobby", 1)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("stream handler did not exit after context cancel")
		}
		waitPresenceCount("#lobby", 1)
		app.chatSvc.LeaveChannel("caller", "#lobby")
		waitPresenceCount("#lobby", 0)
	})
}

func TestHandleChatOnlineDefaultsToLobby(t *testing.T) {
	app := &webApp{chatSvc: chat.NewServiceForTest()}
	app.chatSvc.JoinChannel("caller", "#lobby")

	req := httptest.NewRequest(http.MethodGet, "/chat/online", nil)
	rr := httptest.NewRecorder()
	app.handleChatOnline(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("chat online status = %d", rr.Code)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode chat online payload: %v", err)
	}
	if payload["channel"] != "#lobby" {
		t.Fatalf("expected default channel #lobby, got %#v", payload["channel"])
	}
	if int(payload["count"].(float64)) != 1 {
		t.Fatalf("expected default lobby presence count 1, got %#v", payload["count"])
	}
}

func TestHandleChatOnlineUsesLowercasePresenceKeys(t *testing.T) {
	app := &webApp{chatSvc: chat.NewServiceForTest()}
	app.chatSvc.JoinChannel("caller", "#lobby")

	req := httptest.NewRequest(http.MethodGet, "/chat/online?channel=%23lobby", nil)
	rr := httptest.NewRecorder()
	app.handleChatOnline(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("chat online status = %d", rr.Code)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode chat online payload: %v", err)
	}
	rows, ok := payload["presence"].([]interface{})
	if !ok || len(rows) == 0 {
		t.Fatalf("expected presence rows, got %#v", payload["presence"])
	}
	first, ok := rows[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected object row, got %#v", rows[0])
	}
	for _, key := range []string{"nick", "node", "online", "idle_sec", "area", "login_at", "last_seen"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("expected key %q in presence row, got %#v", key, first)
		}
	}
	for _, key := range []string{"Nick", "Node", "Online", "IdleSec", "Area", "LoginAt", "LastSeen"} {
		if _, ok := first[key]; ok {
			t.Fatalf("unexpected uppercase key %q leaked into presence row: %#v", key, first)
		}
	}
}

func TestHealthReadyMetricsHandlers(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	app := &webApp{
		authSvc:   auth.NewService(userRepo),
		boardRepo: repository.NewInMemoryBoardRepository(),
		chatSvc:   chat.NewServiceForTest(),
		sessions:  map[string]sessionState{},
	}

	rr := httptest.NewRecorder()
	app.handleHealthz(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz status = %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	app.handleReadyz(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("readyz status = %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	app.handleMetrics(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", rr.Code)
	}
	if rr.Body.Len() == 0 {
		t.Fatal("metrics body should not be empty")
	}
}

func TestSettingsRequiresCSRFAndUpdatesPreferences(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	_, _ = authSvc.Register("prefs", "password123")

	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("prefs")
	if !ok {
		t.Fatal("session creation failed")
	}
	app.Lock()
	csrf := app.sessions[sid].csrf
	app.Unlock()

	form := url.Values{}
	form.Set("action", "update_prefs")
	form.Set("theme", "teal")
	form.Set("ansi_enabled", "0")
	form.Set("paging_enabled", "0")
	form.Set("time_format_24h", "1")
	form.Set("home_route", "/today")

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleSettings(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected csrf failure, got %d", rr.Code)
	}

	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleSettings(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect after pref save, got %d", rr.Code)
	}
	if location := rr.Result().Header.Get("Location"); !strings.Contains(location, "/settings?notice=") {
		t.Fatalf("expected settings notice redirect, got %q", location)
	}
	updated, err := authSvc.GetUser("prefs")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if updated.Theme != "teal" || updated.ANSIEnabled || updated.PagingEnabled || !updated.TimeFormat24h {
		t.Fatalf("unexpected preferences after save: %+v", updated)
	}
	if route := app.loadHomeRoute("prefs"); route != "/today" {
		t.Fatalf("expected persisted home route /today, got %q", route)
	}
}

func TestSettingsValidationShowsFriendlyError(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	_, _ = authSvc.Register("prefs2", "password123")

	app := &webApp{
		authSvc:  authSvc,
		userRepo: userRepo,
		sessions: map[string]sessionState{},
	}
	sid, ok := app.createSession("prefs2")
	if !ok {
		t.Fatal("session creation failed")
	}
	app.Lock()
	csrf := app.sessions[sid].csrf
	app.Unlock()

	form := url.Values{}
	form.Set("action", "change_password")
	form.Set("password", "newpass123")
	form.Set("confirm", "different")
	form.Set("csrf_token", csrf)

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleSettings(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect on validation error, got %d", rr.Code)
	}
	if location := rr.Result().Header.Get("Location"); !strings.Contains(location, "/settings?error=") {
		t.Fatalf("expected settings error redirect, got %q", location)
	}
}

func TestMailValidationShowsFriendlyError(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	authSvc := auth.NewService(userRepo)
	_, _ = authSvc.Register("mailer", "password123")

	app := &webApp{
		authSvc:  authSvc,
		userRepo: userRepo,
		mailRepo: mailRepo,
		sessions: map[string]sessionState{},
	}
	sid, ok := app.createSession("mailer")
	if !ok {
		t.Fatal("session creation failed")
	}
	app.Lock()
	csrf := app.sessions[sid].csrf
	app.Unlock()

	form := url.Values{}
	form.Set("to", "nobody")
	form.Set("subject", "missing body")
	form.Set("body", "")
	form.Set("csrf_token", csrf)
	req := httptest.NewRequest(http.MethodPost, "/mail", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect on mail validation error, got %d", rr.Code)
	}
	if location := rr.Result().Header.Get("Location"); !strings.Contains(location, "/mail?error=") {
		t.Fatalf("expected mail error redirect, got %q", location)
	}
}

func TestMailSavedReplyKitsPersistPrefillAndDelete(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	friend, err := authSvc.Register("friend", "password123")
	if err != nil {
		t.Fatalf("register friend: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		mailRepo:  mailRepo,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	csrf := app.sessions[sid].csrf

	form := url.Values{}
	form.Set("action", "save_template")
	form.Set("template_name", "Door Invite")
	form.Set("template_subject", "Meet me in the Door Hub")
	form.Set("template_body", "Meet me in /doors after tonight's scan.")
	form.Set("template_urgency", "fyi")
	form.Set("csrf_token", csrf)
	req := httptest.NewRequest(http.MethodPost, "/mail", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save template status = %d body=%s", rr.Code, rr.Body.String())
	}
	templates := app.loadMailTemplates("caller")
	if len(templates) != 1 || templates[0].Name != "Door Invite" {
		t.Fatalf("unexpected templates after save: %+v", templates)
	}

	req = httptest.NewRequest(http.MethodGet, "/mail?saved_template="+templates[0].ID+"&to=friend", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("mail with saved template status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"Saved Reply Kits", `value="friend"`, "Meet me in the Door Hub", "Meet me in /doors after tonight's scan."} {
		if !strings.Contains(body, want) {
			t.Fatalf("mail with saved template missing %q: %s", want, body)
		}
	}

	send := url.Values{}
	send.Set("action", "send")
	send.Set("template_id", templates[0].ID)
	send.Set("to", "friend")
	send.Set("urgency", "fyi")
	send.Set("subject", "Meet me in the Door Hub")
	send.Set("body", "Meet me in /doors after tonight's scan.")
	send.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/mail", strings.NewReader(send.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("send from template status = %d body=%s", rr.Code, rr.Body.String())
	}
	inbox, err := mailRepo.ListInbox(friend.ID, 5)
	if err != nil || len(inbox) != 1 {
		t.Fatalf("expected one inbox item, got err=%v inbox=%+v", err, inbox)
	}
	if !strings.Contains(inbox[0].Subject, "[FYI]") {
		t.Fatalf("expected urgency-tagged subject, got %+v", inbox[0])
	}
	templates = app.loadMailTemplates("caller")
	if templates[0].UseCount != 1 {
		t.Fatalf("expected template use count 1, got %+v", templates[0])
	}

	form = url.Values{}
	form.Set("action", "delete_template")
	form.Set("template_id", templates[0].ID)
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/mail", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("delete template status = %d body=%s", rr.Code, rr.Body.String())
	}
	if got := app.loadMailTemplates(caller.Handle); len(got) != 0 {
		t.Fatalf("expected templates deleted, got %+v", got)
	}
}

func TestGatewayRequiresCSRFForPost(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	_, _ = authSvc.Register("gate", "password123")

	app := &webApp{
		authSvc:      authSvc,
		userRepo:     userRepo,
		sessions:     map[string]sessionState{},
		offlineDir:   t.TempDir(),
		inboundAllow: map[string]struct{}{},
	}
	sid, ok := app.createSession("gate")
	if !ok {
		t.Fatal("session creation failed")
	}

	form := url.Values{}
	form.Set("url", "https://example.com")
	req := httptest.NewRequest(http.MethodPost, "/gateway", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected csrf failure, got %d", rr.Code)
	}
}

func TestGatewayFetchValidationShowsFriendlyError(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	_, _ = authSvc.Register("gate2", "password123")

	app := &webApp{
		authSvc:      authSvc,
		userRepo:     userRepo,
		sessions:     map[string]sessionState{},
		offlineDir:   t.TempDir(),
		inboundAllow: map[string]struct{}{},
	}
	sid, ok := app.createSession("gate2")
	if !ok {
		t.Fatal("session creation failed")
	}
	app.Lock()
	csrf := app.sessions[sid].csrf
	app.Unlock()

	form := url.Values{}
	form.Set("action", "fetch")
	form.Set("csrf_token", csrf)
	req := httptest.NewRequest(http.MethodPost, "/gateway", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect for missing url, got %d", rr.Code)
	}
	location := rr.Result().Header.Get("Location")
	if !strings.Contains(location, "/gateway?error=") {
		t.Fatalf("expected error redirect on gateway validation, got %q", location)
	}

	req = httptest.NewRequest(http.MethodGet, location, nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected gateway screen with error banner, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Error:") {
		t.Fatalf("expected gateway error banner, got %s", rr.Body.String())
	}
}

func TestPasswordResetRequestAndComplete(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	resetRepo := repository.NewInMemoryPasswordResetRepository()
	authSvc := auth.NewServiceWithPolicy(userRepo, auth.HashPolicy{
		Algorithm:        auth.HashPBKDF2SHA256,
		PBKDF2Iterations: 1000,
		PBKDF2SaltBytes:  16,
		UpgradeOnLogin:   true,
	})
	authSvc.SetPasswordResetRepository(resetRepo)
	if _, err := authSvc.Register("resetweb", "password123"); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	app := &webApp{
		authSvc:      authSvc,
		userRepo:     userRepo,
		sessions:     map[string]sessionState{},
		resetTTL:     10 * time.Minute,
		showResetDev: true,
	}

	form := url.Values{}
	form.Set("handle", "resetweb")
	req := httptest.NewRequest(http.MethodPost, "/reset/request", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	app.handlePasswordResetRequest(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reset request status = %d", rr.Code)
	}
	body := rr.Body.String()
	idx := strings.Index(body, "Dev token: ")
	if idx < 0 {
		t.Fatalf("expected dev token in response, got: %s", body)
	}
	token := strings.TrimSpace(body[idx+len("Dev token: "):])
	if end := strings.Index(token, "<"); end >= 0 {
		token = strings.TrimSpace(token[:end])
	}
	if token == "" {
		t.Fatal("expected reset token")
	}

	form = url.Values{}
	form.Set("token", token)
	form.Set("password", "newpassword123")
	req = httptest.NewRequest(http.MethodPost, "/reset/complete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	app.handlePasswordResetComplete(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("reset complete status = %d", rr.Code)
	}
	if _, err := authSvc.Login("resetweb", "newpassword123"); err != nil {
		t.Fatalf("new password login failed: %v", err)
	}
}

func TestConnectAndTourPages(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	doorRepo := repository.NewInMemoryDoorRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("touruser", "password123"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := boardRepo.Create(&domain.Board{Name: "General", Description: "Main", CreatedBy: 1}); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{BoardID: 1, AuthorID: 1, Subject: "Featured", Body: "hello"}); err != nil {
		t.Fatalf("create message: %v", err)
	}

	app := &webApp{
		authSvc:       authSvc,
		userRepo:      userRepo,
		boardRepo:     boardRepo,
		msgRepo:       msgRepo,
		adminRepo:     adminRepo,
		doorRepo:      doorRepo,
		chatSvc:       chat.NewServiceForTest(),
		sessions:      map[string]sessionState{},
		modernOnRamp:  true,
		guestTour:     true,
		discover:      true,
		wsTerminalURL: "ws://localhost:6080/ws-login",
		savedSearches: map[string][]string{},
	}
	if _, err := app.chatSvc.Post("sysop", "#lobby", "welcome aboard"); err != nil {
		t.Fatalf("seed chat: %v", err)
	}

	rr := httptest.NewRecorder()
	app.handleConnect(rr, httptest.NewRequest(http.MethodGet, "/connect", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("connect status = %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "WolfBBS Connect") || !strings.Contains(body, "ws://localhost:6080/ws-login") {
		t.Fatalf("connect page missing expected content: %s", body)
	}
	for _, needle := range []string{"Choose your client", "First call checklist", "Clipboard-Friendly Commands", "Connection Sanity", "Terminal Profile Presets", "Compatibility Notes", "Mobile-first connection picks", "iPhone/iPad", "Android"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("connect page missing %q: %s", needle, body)
		}
	}
	for _, want := range []string{
		"xterm.min.js",
		"xterm-addon-fit",
		"function createFallbackTerminal(container)",
		"const term = window.Terminal ? new window.Terminal(",
		"JSON.stringify({t: type, d: data || \"\"})",
		"pendingFrames.push(frame)",
		"sendFrame(\"key\", \"\\n\")",
		"scheduleReconnect(\"socket closed\")",
		"document.visibilityState === \"visible\"",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("connect page terminal script missing %q", want)
		}
	}
	if strings.Contains(body, "if (!v) return;") {
		t.Fatalf("connect page still blocks empty line submit")
	}
	rr = httptest.NewRecorder()
	app.handleConnect(rr, httptest.NewRequest(http.MethodGet, "/connect?preset=syncterm", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("connect preset status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `const preset = "syncterm";`) {
		t.Fatalf("connect page missing syncterm preset wiring: %s", rr.Body.String())
	}

	sid, ok := app.createSession("touruser")
	if !ok {
		t.Fatal("session creation failed")
	}
	app.persistHomeRoute("touruser", "/today")
	req := httptest.NewRequest(http.MethodGet, "/connect", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleConnect(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("connect redirect status = %d", rr.Code)
	}
	if got := rr.Result().Header.Get("Location"); got != "/today" {
		t.Fatalf("expected connect redirect to preferred home route, got %q", got)
	}

	rr = httptest.NewRecorder()
	app.handleGuestTour(rr, httptest.NewRequest(http.MethodGet, "/tour", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("tour status = %d", rr.Code)
	}
	for _, needle := range []string{"Guided Tour", "How to become a caller", "Why people come back"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("tour page missing %q: %s", needle, rr.Body.String())
		}
	}
}

func TestStatusAndConfigCenters(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("viewer", "password123"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := boardRepo.Create(&domain.Board{Name: "General", Description: "Main", CreatedBy: 1}); err != nil {
		t.Fatalf("create board: %v", err)
	}

	app := &webApp{
		authSvc:       authSvc,
		userRepo:      userRepo,
		boardRepo:     boardRepo,
		msgRepo:       msgRepo,
		adminRepo:     adminRepo,
		chatSvc:       chat.NewServiceForTest(),
		sessions:      map[string]sessionState{},
		runtimeCfg:    config.DefaultRuntime(),
		quickJump:     true,
		classicSearch: true,
		discover:      true,
		guestTour:     true,
		modernOnRamp:  true,
	}
	sid, ok := app.createSession("viewer")
	if !ok {
		t.Fatal("session creation failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleStatusCenter(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status center status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Status Center") {
		t.Fatalf("missing status center heading: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Quick jump") {
		t.Fatalf("missing quick jump state row: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Guest tour") {
		t.Fatalf("missing guest tour state row: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Telnet login server") {
		t.Fatalf("missing login transport row: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Machine-readable status JSON (/statusz)") {
		t.Fatalf("missing statusz link: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/statusz", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleStatusJSON(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("statusz status = %d", rr.Code)
	}
	var snapshot statusSnapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode statusz payload: %v body=%s", err, rr.Body.String())
	}
	if snapshot.Summary.Total == 0 || len(snapshot.Checks) == 0 {
		t.Fatalf("expected non-empty status checks in statusz payload: %+v", snapshot)
	}
	if snapshot.Role != roleUser {
		t.Fatalf("expected normalized role user in statusz payload, got %q", snapshot.Role)
	}
	if len(snapshot.Recommendations) == 0 {
		t.Fatalf("expected recommendations in statusz payload: %+v", snapshot)
	}

	req = httptest.NewRequest(http.MethodGet, "/config", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleConfigCenter(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("config center status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Config Center") {
		t.Fatalf("missing config center heading: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Transport and Service Config") {
		t.Fatalf("missing transport config section: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Runtime Feature Flags") {
		t.Fatalf("missing runtime feature flags section: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Easy Setup Path") {
		t.Fatalf("missing setup path section: %s", rr.Body.String())
	}
}

func TestAdminLaunchDashboardAndBoardsEmptyState(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	doorRepo := repository.NewInMemoryDoorRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	doorRegistry := doors.NewRegistry()
	doorRegistry.SetRepository(doorRepo)
	app := &webApp{
		authSvc:       authSvc,
		userRepo:      userRepo,
		boardRepo:     boardRepo,
		msgRepo:       msgRepo,
		adminRepo:     adminRepo,
		doorRepo:      doorRepo,
		doorRegistry:  doorRegistry,
		chatSvc:       chat.NewServiceForTest(),
		sessions:      map[string]sessionState{},
		savedSearches: map[string][]string{},
		runtimeCfg:    config.DefaultRuntime(),
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleAdmin(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin panel status = %d", rr.Code)
	}
	for _, needle := range []string{"Sysop Control Panel", "Launch Digest", "Launch Center", "User Ops"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("admin panel missing %q: %s", needle, rr.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/launch", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleAdminLaunch(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin launch status = %d", rr.Code)
	}
	for _, needle := range []string{"Launch Center", "Operator Commands", "docs/OPERATOR_PLAYBOOK.md", "Use Launch Center as home base"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("admin launch missing %q: %s", needle, rr.Body.String())
		}
	}
	for _, needle := range []string{"Go-Live Checkpoints", "Rollback Steps"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("admin launch missing %q: %s", needle, rr.Body.String())
		}
	}
	form := url.Values{}
	form.Set("action", "toggle_checkpoint")
	form.Set("checkpoint", "rollback_ready")
	form.Set("done", "1")
	form.Set("csrf_token", app.sessions[sid].csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/launch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleAdminLaunch(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("admin launch checkpoint status = %d", rr.Code)
	}
	if raw, err := adminRepo.GetSystemSetting(launchChecklistSettingKey("sysop")); err != nil {
		t.Fatalf("load launch checklist: %v", err)
	} else if !strings.Contains(raw, "rollback_ready") {
		t.Fatalf("expected persisted launch checkpoint, got %q", raw)
	}

	req = httptest.NewRequest(http.MethodGet, "/status", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleStatusCenter(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin status center status = %d", rr.Code)
	}
	for _, needle := range []string{"Launch Readiness", "/admin/launch", "checks passing"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("admin status center missing %q: %s", needle, rr.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/config", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleConfigCenter(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin config center status = %d", rr.Code)
	}
	for _, needle := range []string{"Launch Change Order", "/admin/launch"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("admin config center missing %q: %s", needle, rr.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/boards", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("boards status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Board Launch Tip") {
		t.Fatalf("boards empty state missing launch tip: %s", rr.Body.String())
	}
}

func TestRoleAwareEmptyStatesSurfaceAcrossCallerPages(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	doorRepo := repository.NewInMemoryDoorRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	doorRegistry := doors.NewRegistry()
	doorRegistry.SetRepository(doorRepo)
	app := &webApp{
		authSvc:      authSvc,
		userRepo:     userRepo,
		boardRepo:    boardRepo,
		msgRepo:      msgRepo,
		adminRepo:    adminRepo,
		doorRepo:     doorRepo,
		doorRegistry: doorRegistry,
		chatSvc:      chat.NewServiceForTest(),
		sessions:     map[string]sessionState{},
		runtimeCfg:   config.DefaultRuntime(),
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/boards", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("boards status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Boards need a first conversation") {
		t.Fatalf("caller boards empty state missing: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/chat", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleChat(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("chat status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Chat needs a first line") {
		t.Fatalf("chat empty state missing: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/newfiles", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleNewFiles(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("newfiles status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "FileBase needs a seed upload") {
		t.Fatalf("newfiles empty state missing: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/doors", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleDoors(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("doors status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Doors are loaded but not lived in yet") {
		t.Fatalf("doors empty state missing: %s", rr.Body.String())
	}
}

func TestStartAttentionOpsAndRichComposeSurfaces(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)

	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	friend, err := authSvc.Register("friend", "password123")
	if err != nil {
		t.Fatalf("register friend: %v", err)
	}
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}

	last := time.Now().UTC().Add(-4 * time.Hour)
	caller.LastLoginAt = &last
	if err := userRepo.Update(caller); err != nil {
		t.Fatalf("update caller last login: %v", err)
	}

	if err := boardRepo.Create(&domain.Board{Name: "General", Description: "Main board", Conference: "Public", CreatedBy: caller.ID}); err != nil {
		t.Fatalf("create board: %v", err)
	}
	root := &domain.Message{
		BoardID:   1,
		AuthorID:  friend.ID,
		Subject:   "Attention needed",
		Body:      "caller should look here",
		CreatedAt: time.Now().UTC().Add(-2 * time.Hour),
	}
	if err := msgRepo.CreateMessage(root); err != nil {
		t.Fatalf("create root message: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   1,
		AuthorID:  friend.ID,
		ParentID:  root.ID,
		Subject:   "Re: Attention needed",
		Body:      "caller this is a reply for you",
		CreatedAt: time.Now().UTC().Add(-90 * time.Minute),
	}); err != nil {
		t.Fatalf("create reply message: %v", err)
	}
	if err := mailRepo.CreateMail(&domain.PrivateMail{
		FromUserID: friend.ID,
		ToUserID:   caller.ID,
		Subject:    "Unread attention mail",
		Body:       "check this now",
		CreatedAt:  time.Now().UTC().Add(-45 * time.Minute),
	}); err != nil {
		t.Fatalf("create unread mail: %v", err)
	}
	if err := adminRepo.UpsertNodeSession(&domain.NodeSession{
		SessionID:    "live-sysop",
		NodeID:       1,
		Username:     "sysop",
		Area:         "WFC",
		RemoteAddr:   "203.0.113.20:2222",
		LoginAt:      time.Now().UTC().Add(-20 * time.Minute),
		LastActivity: time.Now().UTC().Add(-3 * time.Minute),
	}); err != nil {
		t.Fatalf("seed node session: %v", err)
	}
	if err := adminRepo.UpsertNodeSession(&domain.NodeSession{
		SessionID:    "stale-node",
		NodeID:       7,
		Username:     "stale",
		Area:         "Idle",
		RemoteAddr:   "198.51.100.77:2222",
		LoginAt:      time.Now().UTC().Add(-2 * time.Hour),
		LastActivity: time.Now().UTC().Add(-95 * time.Minute),
	}); err != nil {
		t.Fatalf("seed stale node session: %v", err)
	}
	if err := adminRepo.AddCallerHistory(&domain.CallerHistory{
		SessionID:       "recent-caller",
		NodeID:          2,
		Username:        "caller",
		Area:            "Boards",
		RemoteAddr:      "198.51.100.7:2222",
		LoginAt:         time.Now().UTC().Add(-70 * time.Minute),
		LogoutAt:        time.Now().UTC().Add(-10 * time.Minute),
		DurationSeconds: 3600,
	}); err != nil {
		t.Fatalf("seed caller history: %v", err)
	}
	if err := adminRepo.AddAudit(&domain.AdminAudit{
		Actor:     "sysop",
		Target:    "boards",
		Action:    "seed_default_boards",
		Details:   "seeded=3",
		CreatedAt: time.Now().UTC().Add(-5 * time.Minute),
	}); err != nil {
		t.Fatalf("seed audit: %v", err)
	}
	appErr := errors.New("qa runtime issue")

	app := &webApp{
		authSvc:       authSvc,
		userRepo:      userRepo,
		boardRepo:     boardRepo,
		msgRepo:       msgRepo,
		mailRepo:      mailRepo,
		adminRepo:     adminRepo,
		chatSvc:       chat.NewServiceForTest(),
		sessions:      map[string]sessionState{},
		rateLimits:    map[string][]time.Time{},
		runtimeCfg:    config.DefaultRuntime(),
		discover:      true,
		quickJump:     true,
		savedSearches: map[string][]string{},
	}
	app.addAppError("qa.ops", appErr)

	callerSID, ok := app.createSession("caller")
	if !ok {
		t.Fatal("caller session creation failed")
	}
	sysopSID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("sysop session creation failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/start", nil)
	rr := httptest.NewRecorder()
	app.handleStartCenter(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("guest start status = %d", rr.Code)
	}
	for _, needle := range []string{"Start Center", "Caller path", "Sysop path", "Guest Quick-Start Checklist"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("guest start missing %q: %s", needle, rr.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/start", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleStartCenter(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("caller start status = %d", rr.Code)
	}
	for _, needle := range []string{"Attention Center", "everything that needs follow-up", "Use Today first", "First Caller Session", "Create your first board post"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("caller start missing %q: %s", needle, rr.Body.String())
		}
	}
	req = httptest.NewRequest(http.MethodGet, "/today", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleToday(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("today preflight status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Quick-Start Checklist") {
		t.Fatalf("expected today to surface onboarding checklist: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/first-call", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleFirstCallSession(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first-call status = %d", rr.Code)
	}
	for _, needle := range []string{"First Caller Session", "Starter Board Post", "Lobby Hello", "Private Mail Check", "Choose Home Route"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("first-call page missing %q: %s", needle, rr.Body.String())
		}
	}
	form := url.Values{}
	form.Set("action", "starter_post")
	form.Set("board_id", "1")
	form.Set("subject", "Starter subject")
	form.Set("body", "Starter body")
	form.Set("csrf_token", app.sessions[callerSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/first-call", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleFirstCallSession(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("first-call starter post status = %d", rr.Code)
	}
	form = url.Values{}
	form.Set("action", "starter_chat")
	form.Set("channel", "#lobby")
	form.Set("body", "Starter lobby line")
	form.Set("csrf_token", app.sessions[callerSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/first-call", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleFirstCallSession(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("first-call starter chat status = %d", rr.Code)
	}
	form = url.Values{}
	form.Set("action", "starter_mail")
	form.Set("to", "friend")
	form.Set("subject", "Starter mail")
	form.Set("body", "Starter private mail body")
	form.Set("csrf_token", app.sessions[callerSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/first-call", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleFirstCallSession(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("first-call starter mail status = %d", rr.Code)
	}
	form = url.Values{}
	form.Set("action", "save_home_route")
	form.Set("home_route", "/today")
	form.Set("csrf_token", app.sessions[callerSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/first-call", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleFirstCallSession(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("first-call home route status = %d", rr.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleRoot(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("root redirect status = %d", rr.Code)
	}
	if got := rr.Result().Header.Get("Location"); got != "/today" {
		t.Fatalf("expected root redirect to /today, got %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/attention", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleAttentionCenter(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("attention status = %d", rr.Code)
	}
	for _, needle := range []string{"Attention Center", "Direct Follow-Up", "Inbox Needs Action", "Unread attention mail", "Attention needed"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("attention page missing %q: %s", needle, rr.Body.String())
		}
	}
	digest, err := discovery.BuildSinceLastCall(app.boardRepo, app.msgRepo, app.mailRepo, caller, 18)
	if err != nil {
		t.Fatalf("build digest: %v", err)
	}
	var dismissItem discovery.Item
	for _, item := range digest.Items {
		if item.Kind == "reply" || item.Kind == "mention" {
			dismissItem = item
			break
		}
	}
	if dismissItem.Line == "" {
		t.Fatalf("expected dismissible attention item in digest: %+v", digest.Items)
	}
	form = url.Values{}
	form.Set("action", "mark_read")
	form.Set("item_key", attentionItemKey(dismissItem))
	form.Set("csrf_token", app.sessions[callerSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/attention", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleAttentionCenter(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("mark attention read status = %d", rr.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/attention", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleAttentionCenter(rr, req)
	if !strings.Contains(rr.Body.String(), "Seen This Cycle") || !strings.Contains(rr.Body.String(), dismissItem.Line) {
		t.Fatalf("expected read attention item to stay visible in seen state: %s", rr.Body.String())
	}
	rawRead, err := adminRepo.GetSystemSetting(attentionReadSettingKey("caller"))
	if err != nil {
		t.Fatalf("persisted attention reads missing: %v", err)
	}
	if !strings.Contains(rawRead, attentionItemKey(dismissItem)) {
		t.Fatalf("persisted attention reads missing item key: %s", rawRead)
	}
	form = url.Values{}
	form.Set("action", "mark_unread")
	form.Set("item_key", attentionItemKey(dismissItem))
	form.Set("csrf_token", app.sessions[callerSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/attention", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleAttentionCenter(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("mark attention unread status = %d", rr.Code)
	}
	if clearedRead, err := adminRepo.GetSystemSetting(attentionReadSettingKey("caller")); err != nil {
		t.Fatalf("load cleared attention reads: %v", err)
	} else if strings.TrimSpace(clearedRead) != "" {
		t.Fatalf("expected persisted attention reads to clear, got %q", clearedRead)
	}

	form = url.Values{}
	form.Set("action", "dismiss")
	form.Set("item_key", attentionItemKey(dismissItem))
	form.Set("csrf_token", app.sessions[callerSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/attention", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleAttentionCenter(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("dismiss attention status = %d", rr.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/attention", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleAttentionCenter(rr, req)
	if strings.Contains(rr.Body.String(), dismissItem.Line) {
		t.Fatalf("expected dismissed attention item to be removed: %s", rr.Body.String())
	}
	rawDismissed, err := adminRepo.GetSystemSetting(attentionDismissedSettingKey("caller"))
	if err != nil {
		t.Fatalf("persisted attention dismissals missing: %v", err)
	}
	if !strings.Contains(rawDismissed, attentionItemKey(dismissItem)) {
		t.Fatalf("persisted attention dismissals missing item key: %s", rawDismissed)
	}
	reloadedApp := &webApp{
		authSvc:       authSvc,
		userRepo:      userRepo,
		boardRepo:     boardRepo,
		msgRepo:       msgRepo,
		mailRepo:      mailRepo,
		adminRepo:     adminRepo,
		chatSvc:       chat.NewServiceForTest(),
		sessions:      map[string]sessionState{},
		rateLimits:    map[string][]time.Time{},
		runtimeCfg:    config.DefaultRuntime(),
		discover:      true,
		quickJump:     true,
		savedSearches: map[string][]string{},
	}
	reloadedSID, ok := reloadedApp.createSession("caller")
	if !ok {
		t.Fatal("reloaded caller session creation failed")
	}
	req = httptest.NewRequest(http.MethodGet, "/attention", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: reloadedSID})
	rr = httptest.NewRecorder()
	reloadedApp.handleAttentionCenter(rr, req)
	if strings.Contains(rr.Body.String(), dismissItem.Line) {
		t.Fatalf("expected dismissed attention item to persist across app reload: %s", rr.Body.String())
	}

	form = url.Values{}
	form.Set("action", "clear_dismissed")
	form.Set("csrf_token", app.sessions[callerSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/attention", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleAttentionCenter(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("clear dismissed attention status = %d", rr.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/attention", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleAttentionCenter(rr, req)
	if !strings.Contains(rr.Body.String(), dismissItem.Line) {
		t.Fatalf("expected restored attention item to reappear: %s", rr.Body.String())
	}
	if cleared, err := adminRepo.GetSystemSetting(attentionDismissedSettingKey("caller")); err != nil {
		t.Fatalf("load cleared attention dismissals: %v", err)
	} else if strings.TrimSpace(cleared) != "" {
		t.Fatalf("expected persisted attention dismissals to clear, got %q", cleared)
	}

	form = url.Values{}
	form.Set("action", "dismiss_board")
	form.Set("board_id", "1")
	form.Set("csrf_token", app.sessions[callerSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/attention", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleAttentionCenter(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("dismiss board status = %d", rr.Code)
	}
	rawDismissed, err = adminRepo.GetSystemSetting(attentionDismissedSettingKey("caller"))
	if err != nil {
		t.Fatalf("persisted board attention dismissals missing: %v", err)
	}
	if !strings.Contains(rawDismissed, boardAttentionKey(1)) {
		t.Fatalf("persisted attention dismissals missing board key: %s", rawDismissed)
	}

	form = url.Values{}
	form.Set("action", "mark_all_mail_read")
	form.Set("csrf_token", app.sessions[callerSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/attention", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleAttentionCenter(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("mark all mail read status = %d", rr.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/attention", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleAttentionCenter(rr, req)
	if !strings.Contains(rr.Body.String(), "No unread mail is waiting.") {
		t.Fatalf("expected unread mail queue to clear: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/mail", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("mail status = %d", rr.Code)
	}
	for _, needle := range []string{`data-rich-compose="mail-compose"`, `data-compose-signature="caller"`} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("mail compose missing %q: %s", needle, rr.Body.String())
		}
	}
	if !strings.Contains(rr.Body.String(), `Compose`) {
		t.Fatalf("mail compose missing compose heading: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/mail?id=1", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("mail reader status = %d", rr.Code)
	}
	for _, needle := range []string{`data-rich-compose="mail-reply"`, `data-compose-signature="caller"`, `data-compose-quote="&gt; check this now`} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("mail reply missing %q: %s", needle, rr.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/mail", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("mail status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `data-rich-compose="mail-compose"`) {
		t.Fatalf("mail compose missing rich compose marker: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/boards?board=1&id=1", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("board reader status = %d", rr.Code)
	}
	for _, needle := range []string{`data-rich-compose="board-reply"`, `data-compose-signature="caller"`, `data-compose-quote="&gt; caller should look here`} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("board reader missing %q: %s", needle, rr.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/feedback", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleFeedback(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("feedback status = %d", rr.Code)
	}
	for _, needle := range []string{`data-rich-compose="feedback-compose"`, `data-compose-signature="caller"`} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("feedback missing %q: %s", needle, rr.Body.String())
		}
	}
	app.sessions["expired-web"] = sessionState{handle: "caller", expire: time.Now().Add(-time.Minute), csrf: "expired"}
	app.rateLimits["login:198.51.100.9"] = []time.Time{time.Now().UTC()}
	app.rateLimits["reset:198.51.100.9"] = []time.Time{time.Now().UTC()}

	req = httptest.NewRequest(http.MethodGet, "/admin/ops", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.handleAdminOps(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ops status = %d", rr.Code)
	}
	for _, needle := range []string{"Ops Center", "Operator Confidence", "Proof Pack", "Artifact Health", "Current Operator Focus", "Recent Runtime Errors", "Recent Audit Trail", "seed_default_boards", "Live Sessions", "Clear Runtime Errors", "Prune Sessions", "Purge Expired Web Sessions", "Clear Rate Limits", "qa runtime issue"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("ops page missing %q: %s", needle, rr.Body.String())
		}
	}
	form = url.Values{}
	form.Set("action", "clear_errors")
	form.Set("csrf_token", app.sessions[sysopSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/ops", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.handleAdminOps(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("clear errors status = %d", rr.Code)
	}
	if len(app.latestErrors(10)) != 0 {
		t.Fatalf("expected error log to be cleared, got %+v", app.latestErrors(10))
	}

	form = url.Values{}
	form.Set("action", "prune_idle_sessions")
	form.Set("idle_minutes", "30")
	form.Set("csrf_token", app.sessions[sysopSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/ops", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.handleAdminOps(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("prune sessions status = %d", rr.Code)
	}
	sessions, err := adminRepo.ListNodeSessions(10)
	if err != nil {
		t.Fatalf("list node sessions: %v", err)
	}
	for _, row := range sessions {
		if row.SessionID == "stale-node" {
			t.Fatalf("expected stale session to be pruned: %+v", sessions)
		}
	}

	form = url.Values{}
	form.Set("action", "purge_web_sessions")
	form.Set("csrf_token", app.sessions[sysopSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/ops", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.handleAdminOps(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("purge web sessions status = %d", rr.Code)
	}
	if _, ok := app.sessions["expired-web"]; ok {
		t.Fatalf("expected expired web session to be purged")
	}

	form = url.Values{}
	form.Set("action", "clear_rate_limits")
	form.Set("csrf_token", app.sessions[sysopSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/ops", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.handleAdminOps(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("clear rate limits status = %d", rr.Code)
	}
	if got := app.countRateLimits(); got != 0 {
		t.Fatalf("expected rate limits to clear, got %d", got)
	}
}

func TestShowcaseGuestAndCallerViews(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		boardRepo: repository.NewInMemoryBoardRepository(),
		adminRepo: repository.NewInMemoryAdminRepository(),
		chatSvc:   chat.NewServiceForTest(),
		sessions:  map[string]sessionState{},
	}

	req := httptest.NewRequest(http.MethodGet, "/showcase", nil)
	rr := httptest.NewRecorder()
	app.handleShowcase(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("guest showcase status = %d", rr.Code)
	}
	for _, needle := range []string{"Product Showcase", "10-minute smoke flow", "Experience Lanes", "docs/EXTENSION_SDK.md"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("guest showcase missing %q: %s", needle, rr.Body.String())
		}
	}

	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("caller session creation failed")
	}
	req = httptest.NewRequest(http.MethodGet, "/showcase", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleShowcase(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("caller showcase status = %d", rr.Code)
	}
	for _, needle := range []string{"First-Run Progress", "/first-call"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("caller showcase missing %q: %s", needle, rr.Body.String())
		}
	}
}

func TestDiscoverDigestAndSavedSearch(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	authSvc := auth.NewService(userRepo)
	user, err := authSvc.Register("discoverer", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	last := time.Now().UTC().Add(-3 * time.Hour)
	user.LastLoginAt = &last
	if err := userRepo.Update(user); err != nil {
		t.Fatalf("update user: %v", err)
	}
	if err := boardRepo.Create(&domain.Board{Name: "General", Description: "Main", CreatedBy: user.ID}); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   1,
		AuthorID:  99,
		Subject:   "Ping discoverer",
		Body:      "discoverer check this thread",
		CreatedAt: time.Now().UTC().Add(-1 * time.Hour),
	}); err != nil {
		t.Fatalf("create msg: %v", err)
	}
	if err := mailRepo.CreateMail(&domain.PrivateMail{
		FromUserID: 100,
		ToUserID:   user.ID,
		Subject:    "New mail",
		Body:       "mail body",
		CreatedAt:  time.Now().UTC().Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("create mail: %v", err)
	}

	app := &webApp{
		authSvc:       authSvc,
		userRepo:      userRepo,
		boardRepo:     boardRepo,
		msgRepo:       msgRepo,
		mailRepo:      mailRepo,
		chatSvc:       chat.NewServiceForTest(),
		sessions:      map[string]sessionState{},
		discover:      true,
		classicSearch: true,
		savedSearches: map[string][]string{},
	}
	sid, ok := app.createSession("discoverer")
	if !ok {
		t.Fatal("session creation failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/discover?q=discoverer&save=1", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleDiscover(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("discover status = %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Since Your Last Call") {
		t.Fatalf("discover page missing heading: %s", body)
	}
	if !strings.Contains(body, "Saved Searches") {
		t.Fatalf("discover page missing saved searches section: %s", body)
	}
	if len(app.savedSearchList("discoverer")) == 0 {
		t.Fatal("expected saved search entry")
	}
}

func TestBoardSubscriptionsEventsAndTodayBrief(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)

	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	friend, err := authSvc.Register("friend", "password123")
	if err != nil {
		t.Fatalf("register friend: %v", err)
	}
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}

	if err := boardRepo.Create(&domain.Board{Name: "General Alpha", Description: "watched board", Conference: "Public", CreatedBy: caller.ID}); err != nil {
		t.Fatalf("create board 1: %v", err)
	}
	if err := boardRepo.Create(&domain.Board{Name: "Tooling Beta", Description: "unwatched board", Conference: "Ops", CreatedBy: caller.ID}); err != nil {
		t.Fatalf("create board 2: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   1,
		AuthorID:  friend.ID,
		Subject:   "Tracked update",
		Body:      "watch this board",
		CreatedAt: time.Now().UTC().Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("create tracked message: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   2,
		AuthorID:  friend.ID,
		Subject:   "Other update",
		Body:      "do not watch this board",
		CreatedAt: time.Now().UTC().Add(-90 * time.Minute),
	}); err != nil {
		t.Fatalf("create untracked message: %v", err)
	}

	app := &webApp{
		authSvc:       authSvc,
		userRepo:      userRepo,
		boardRepo:     boardRepo,
		msgRepo:       msgRepo,
		mailRepo:      mailRepo,
		adminRepo:     adminRepo,
		sessions:      map[string]sessionState{},
		rateLimits:    map[string][]time.Time{},
		runtimeCfg:    config.DefaultRuntime(),
		discover:      true,
		quickJump:     true,
		savedSearches: map[string][]string{},
	}

	callerSID, ok := app.createSession("caller")
	if !ok {
		t.Fatal("caller session creation failed")
	}
	sysopSID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("sysop session creation failed")
	}

	form := url.Values{}
	form.Set("action", "watch")
	form.Set("board_id", "1")
	form.Set("csrf_token", app.sessions[callerSID].csrf)
	req := httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr := httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("watch board status = %d", rr.Code)
	}
	rawSubs, err := adminRepo.GetSystemSetting(boardSubscriptionSettingKey("caller"))
	if err != nil {
		t.Fatalf("load board subscriptions: %v", err)
	}
	if !strings.Contains(rawSubs, `"1":"watch"`) {
		t.Fatalf("expected watch tier persistence, got %q", rawSubs)
	}

	form = url.Values{}
	form.Set("action", "digest")
	form.Set("board_id", "2")
	form.Set("csrf_token", app.sessions[callerSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("digest board status = %d", rr.Code)
	}
	rawSubs, err = adminRepo.GetSystemSetting(boardSubscriptionSettingKey("caller"))
	if err != nil {
		t.Fatalf("reload board subscriptions: %v", err)
	}
	for _, needle := range []string{`"1":"watch"`, `"2":"digest"`} {
		if !strings.Contains(rawSubs, needle) {
			t.Fatalf("expected persisted board subscriptions to contain %q in %q", needle, rawSubs)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/boards?mode=watched", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("watched boards status = %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "General Alpha") {
		t.Fatalf("expected watched board in watched mode: %s", body)
	}
	if strings.Contains(body, `<td><a href="/boards?board=2">Tooling Beta</a></td>`) {
		t.Fatalf("expected unwatched board row to be absent in watched mode: %s", body)
	}
	if !strings.Contains(body, `name="subscription_mode"`) || !strings.Contains(body, `option value="watch" selected`) {
		t.Fatalf("expected watch tier selector in watched mode: %s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/boards?mode=digest", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("digest boards status = %d", rr.Code)
	}
	body = rr.Body.String()
	if !strings.Contains(body, "Tooling Beta") || strings.Contains(body, `<td><a href="/boards?board=1">General Alpha</a></td>`) {
		t.Fatalf("expected digest mode to isolate digest board rows: %s", body)
	}

	startsAt := time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04")
	endsAt := time.Now().Add(50 * time.Hour).Format("2006-01-02T15:04")
	repeatUntil := time.Now().Add(28 * 24 * time.Hour).Format("2006-01-02T15:04")
	form = url.Values{}
	form.Set("action", "create")
	form.Set("title", "Friday Tournament Night")
	form.Set("category", "tournament")
	form.Set("starts_at", startsAt)
	form.Set("ends_at", endsAt)
	form.Set("recurrence", "weekly")
	form.Set("repeat_until", repeatUntil)
	form.Set("location", "#lobby")
	form.Set("host", "sysop")
	form.Set("audience", "all callers")
	form.Set("description", "Door bracket and score showdown.")
	form.Set("link", "/doors")
	form.Set("csrf_token", app.sessions[sysopSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/events", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminEvents)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("create event status = %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/events", nil)
	rr = httptest.NewRecorder()
	app.handleEventsCalendar(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("events status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Friday Tournament Night") || !strings.Contains(rr.Body.String(), "Weekly until") {
		t.Fatalf("expected public events page to show created event: %s", rr.Body.String())
	}
	if strings.Count(rr.Body.String(), "Friday Tournament Night") < 2 {
		t.Fatalf("expected recurring event expansion on public calendar: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/today", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleToday(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("today status = %d", rr.Code)
	}
	for _, needle := range []string{"Today Brief", "Friday Tournament Night", "General Alpha", "Tooling Beta", "Digest Tier Boards", "watch tier"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("today brief missing %q: %s", needle, rr.Body.String())
		}
	}

	reloaded := &webApp{
		authSvc:       authSvc,
		userRepo:      userRepo,
		boardRepo:     boardRepo,
		msgRepo:       msgRepo,
		mailRepo:      mailRepo,
		adminRepo:     adminRepo,
		sessions:      map[string]sessionState{},
		rateLimits:    map[string][]time.Time{},
		runtimeCfg:    config.DefaultRuntime(),
		discover:      true,
		quickJump:     true,
		savedSearches: map[string][]string{},
	}
	reloadedSID, ok := reloaded.createSession("caller")
	if !ok {
		t.Fatal("reloaded caller session creation failed")
	}
	req = httptest.NewRequest(http.MethodGet, "/today", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: reloadedSID})
	rr = httptest.NewRecorder()
	reloaded.handleToday(rr, req)
	if !strings.Contains(rr.Body.String(), "Friday Tournament Night") || !strings.Contains(rr.Body.String(), "General Alpha") || !strings.Contains(rr.Body.String(), "Tooling Beta") {
		t.Fatalf("expected today brief data to persist after reload: %s", rr.Body.String())
	}

	events := app.loadCommunityEvents()
	if len(events) != 1 {
		t.Fatalf("expected one event, got %+v", events)
	}
	form = url.Values{}
	form.Set("action", "delete")
	form.Set("id", events[0].ID)
	form.Set("csrf_token", app.sessions[sysopSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/events", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminEvents)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("delete event status = %d", rr.Code)
	}

	form = url.Values{}
	form.Set("action", "unwatch")
	form.Set("board_id", "1")
	form.Set("csrf_token", app.sessions[callerSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("unwatch board status = %d", rr.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/boards?mode=watched", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if strings.Contains(rr.Body.String(), `<td><a href="/boards?board=1">General Alpha</a></td>`) {
		t.Fatalf("expected cleared watch tier board to leave watched mode: %s", rr.Body.String())
	}
}

func TestDigestTournamentAndAdminUploadReviewFlow(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)

	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	sysop, err := authSvc.Register("sysop", "password123")
	if err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole(sysop.Handle, roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	if err := boardRepo.Create(&domain.Board{Name: "General Alpha", Description: "Main", Conference: "Public", CreatedBy: sysop.ID}); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := boardRepo.Create(&domain.Board{Name: "Tooling Beta", Description: "Digest lane", Conference: "Ops", CreatedBy: sysop.ID}); err != nil {
		t.Fatalf("create second board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   1,
		AuthorID:  sysop.ID,
		Subject:   "Reply needed",
		Body:      "caller please review this thread",
		CreatedAt: time.Now().UTC().Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("create general message: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   2,
		AuthorID:  sysop.ID,
		Subject:   "Digest board update",
		Body:      "low-noise ops update",
		CreatedAt: time.Now().UTC().Add(-90 * time.Minute),
	}); err != nil {
		t.Fatalf("create digest message: %v", err)
	}
	if err := mailRepo.CreateMail(&domain.PrivateMail{
		FromUserID: sysop.ID,
		ToUserID:   caller.ID,
		Subject:    "Private follow-up",
		Body:       "caller inbox item",
		CreatedAt:  time.Now().UTC().Add(-75 * time.Minute),
	}); err != nil {
		t.Fatalf("create mail: %v", err)
	}

	app := &webApp{
		authSvc:    authSvc,
		userRepo:   userRepo,
		boardRepo:  boardRepo,
		msgRepo:    msgRepo,
		mailRepo:   mailRepo,
		adminRepo:  adminRepo,
		sessions:   map[string]sessionState{},
		discover:   true,
		quickJump:  true,
		chatSvc:    chat.NewServiceForTest(),
		rateLimits: map[string][]time.Time{},
	}
	callerSID, ok := app.createSession(caller.Handle)
	if !ok {
		t.Fatal("caller session creation failed")
	}
	sysopSID, ok := app.createSession(sysop.Handle)
	if !ok {
		t.Fatal("sysop session creation failed")
	}
	app.setBoardSubscription(caller.Handle, 2, boardSubscriptionDigest)

	startsAt := time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04")
	endsAt := time.Now().Add(50 * time.Hour).Format("2006-01-02T15:04")
	repeatUntil := time.Now().Add(21 * 24 * time.Hour).Format("2006-01-02T15:04")
	form := url.Values{}
	form.Set("action", "create")
	form.Set("title", "Friday Tournament Night")
	form.Set("category", "tournament")
	form.Set("starts_at", startsAt)
	form.Set("ends_at", endsAt)
	form.Set("recurrence", "weekly")
	form.Set("repeat_until", repeatUntil)
	form.Set("location", "#lobby")
	form.Set("host", sysop.Handle)
	form.Set("audience", "all callers")
	form.Set("description", "Bracket-ready door ladder.")
	form.Set("link", "/doors")
	form.Set("csrf_token", app.sessions[sysopSID].csrf)
	req := httptest.NewRequest(http.MethodPost, "/admin/events", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminEvents)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("create tournament event status = %d", rr.Code)
	}

	form = url.Values{}
	form.Set("action", "update_digest")
	form.Set("digest_enabled", "1")
	form.Set("digest_max_items", "10")
	form.Set("digest_include_events", "1")
	form.Set("digest_include_boards", "1")
	form.Set("csrf_token", app.sessions[callerSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleSettings(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("update digest prefs status = %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/digest", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleDigest(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("digest status = %d", rr.Code)
	}
	for _, needle := range []string{"Daily Digest", "Digest Tier Boards", "Friday Tournament Night", "active"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("digest page missing %q: %s", needle, rr.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/tournaments", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleTournaments(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("tournaments status = %d", rr.Code)
	}
	for _, needle := range []string{"Tournament Center", "Upcoming Tournaments", "Friday Tournament Night"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("tournaments page missing %q: %s", needle, rr.Body.String())
		}
	}

	events := app.loadCommunityEvents()
	if len(events) != 1 {
		t.Fatalf("expected one event after create, got %+v", events)
	}
	req = httptest.NewRequest(http.MethodGet, "/admin/events?edit="+url.QueryEscape(events[0].ID), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminEvents)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("edit event page status = %d", rr.Code)
	}
	for _, needle := range []string{"Edit Event", "Save Changes", `value="Friday Tournament Night"`} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("edit event page missing %q: %s", needle, rr.Body.String())
		}
	}
	req = httptest.NewRequest(http.MethodGet, "/admin/events?template=tournament", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminEvents)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("template event page status = %d", rr.Code)
	}
	for _, needle := range []string{"Quick Templates", "Calendar Preview", "Tournament template loaded.", `data-draft-key="admin-event-editor"`} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("template event page missing %q: %s", needle, rr.Body.String())
		}
	}

	form = url.Values{}
	form.Set("action", "update")
	form.Set("id", events[0].ID)
	form.Set("title", "Playoff Ladder Finals")
	form.Set("category", "tournament")
	form.Set("starts_at", startsAt)
	form.Set("ends_at", endsAt)
	form.Set("recurrence", "weekly")
	form.Set("repeat_until", repeatUntil)
	form.Set("location", "/doors")
	form.Set("host", sysop.Handle)
	form.Set("audience", "ranked callers")
	form.Set("description", "Updated playoff ladder.")
	form.Set("link", "/scores")
	form.Set("csrf_token", app.sessions[sysopSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/events", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminEvents)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("update event status = %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/events", nil)
	rr = httptest.NewRecorder()
	app.handleEventsCalendar(rr, req)
	calendarBody := rr.Body.String()
	if !strings.Contains(calendarBody, "Playoff Ladder Finals") || strings.Contains(calendarBody, "Friday Tournament Night") {
		t.Fatalf("expected updated event title on calendar: %s", calendarBody)
	}

	tmpDir := t.TempDir()
	if err := adminRepo.CreateFileArea(&domain.FileArea{Name: "Uploads", Path: tmpDir, Description: "qa uploads"}); err != nil {
		t.Fatalf("create file area: %v", err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("action", "upload")
	_ = writer.WriteField("area_id", "1")
	_ = writer.WriteField("description", "Classic ANSI pack")
	_ = writer.WriteField("tags", "retro,ansi")
	_ = writer.WriteField("review_hold", "1")
	_ = writer.WriteField("review_notes", "scan pending")
	_ = writer.WriteField("csrf_token", app.sessions[sysopSID].csrf)
	fileWriter, err := writer.CreateFormFile("upload_file", "wolfpack.ans")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fileWriter.Write([]byte("ANSI FILE CONTENT")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/files", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminFiles)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("upload file status = %d", rr.Code)
	}
	files, err := adminRepo.ListFileEntries(0, "", nil, 10)
	if err != nil || len(files) != 1 {
		t.Fatalf("expected uploaded file entry, got %v %+v", err, files)
	}
	review, ok := app.fileReviewItem(files[0].ID)
	if !ok || review.Status != fileReviewHold {
		t.Fatalf("expected file to enter hold review, got %+v ok=%t", review, ok)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/files", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminFiles)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin files status = %d", rr.Code)
	}
	for _, needle := range []string{"Upload Intake", "Review Queue", "wolfpack.ans", "hold"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("admin files page missing %q: %s", needle, rr.Body.String())
		}
	}
	form = url.Values{}
	form.Set("action", "save_filter")
	form.Set("filter_user_id", strconv.FormatInt(sysop.ID, 10))
	form.Set("name", "retro pack")
	form.Set("query", "wolfpack")
	form.Set("tags", "retro,ansi")
	form.Set("csrf_token", app.sessions[sysopSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/files", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminFiles)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save file filter status = %d", rr.Code)
	}
	filters, err := adminRepo.ListFileFilters(sysop.ID)
	if err != nil || len(filters) == 0 {
		t.Fatalf("expected saved file filter, got %v %+v", err, filters)
	}
	req = httptest.NewRequest(http.MethodGet, "/admin/files?saved_filter="+strconv.FormatInt(filters[0].ID, 10)+"&filter_user="+strconv.FormatInt(sysop.ID, 10), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminFiles)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("apply file filter status = %d", rr.Code)
	}
	for _, needle := range []string{"Active File Filters", "saved filter: retro pack", `value="wolfpack"`} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("applied file filter page missing %q: %s", needle, rr.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/newfiles", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleNewFiles(rr, req)
	if strings.Contains(rr.Body.String(), "wolfpack.ans") {
		t.Fatalf("held upload should not be visible to callers: %s", rr.Body.String())
	}

	form = url.Values{}
	form.Set("action", "review_file")
	form.Set("file_id", strconv.FormatInt(files[0].ID, 10))
	form.Set("status", fileReviewApproved)
	form.Set("review_notes", "looks clean")
	form.Set("csrf_token", app.sessions[sysopSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/files", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminFiles)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("approve upload status = %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/newfiles", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleNewFiles(rr, req)
	if !strings.Contains(rr.Body.String(), "wolfpack.ans") {
		t.Fatalf("approved upload should be visible to callers: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/gateway?view=files", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleGateway(rr, req)
	if !strings.Contains(rr.Body.String(), "wolfpack.ans") {
		t.Fatalf("approved upload should be visible in gateway browser: %s", rr.Body.String())
	}

	filePath := files[0].Path
	form = url.Values{}
	form.Set("action", "delete_file")
	form.Set("file_id", strconv.FormatInt(files[0].ID, 10))
	form.Set("csrf_token", app.sessions[sysopSID].csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/files", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminFiles)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("delete file status = %d", rr.Code)
	}
	if _, err := adminRepo.GetFileEntry(files[0].ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected deleted file entry to be gone, got err=%v", err)
	}
	if _, ok := app.fileReviewItem(files[0].ID); ok {
		t.Fatalf("expected deleted file to leave review queue")
	}
	if _, err := os.Stat(filePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected uploaded file to be removed from disk, got err=%v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/newfiles", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleNewFiles(rr, req)
	if strings.Contains(rr.Body.String(), "wolfpack.ans") {
		t.Fatalf("deleted upload should not be visible to callers: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/gateway?view=files", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleGateway(rr, req)
	if strings.Contains(rr.Body.String(), "wolfpack.ans") {
		t.Fatalf("deleted upload should not be visible in gateway browser: %s", rr.Body.String())
	}
}

func TestAdminFilesRejectsOversizedUpload(t *testing.T) {
	t.Setenv("WOLFBBS_UPLOAD_MAX_BYTES", "1048576")

	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	sysop, err := authSvc.Register("sysop", "password123")
	if err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole(sysop.Handle, roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}

	app := &webApp{
		authSvc:    authSvc,
		adminRepo:  adminRepo,
		sessions:   map[string]sessionState{},
		rateLimits: map[string][]time.Time{},
	}
	sid, ok := app.createSession(sysop.Handle)
	if !ok {
		t.Fatal("session creation failed")
	}

	tmpDir := t.TempDir()
	if err := adminRepo.CreateFileArea(&domain.FileArea{Name: "Uploads", Path: tmpDir, Description: "qa uploads"}); err != nil {
		t.Fatalf("create file area: %v", err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("action", "upload")
	_ = writer.WriteField("area_id", "1")
	_ = writer.WriteField("description", "Too big")
	_ = writer.WriteField("tags", "retro")
	_ = writer.WriteField("csrf_token", app.sessions[sid].csrf)
	fileWriter, err := writer.CreateFormFile("upload_file", "toolarge.ans")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fileWriter.Write(bytes.Repeat([]byte("A"), 2*1024*1024)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/files", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminFiles)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("upload file status = %d", rr.Code)
	}
	if got := rr.Header().Get("Location"); !strings.Contains(got, "error=") {
		t.Fatalf("expected error redirect, got %q", got)
	}
	files, err := adminRepo.ListFileEntries(0, "", nil, 10)
	if err != nil {
		t.Fatalf("list file entries: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected oversized upload to be rejected, got %+v", files)
	}
}

func TestAdminFilesUploadPolicyHookRejectsUpload(t *testing.T) {
	script := filepath.Join(t.TempDir(), "reject-upload.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"scan policy blocked $WOLFBBS_UPLOAD_NAME\"\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write hook script: %v", err)
	}
	t.Setenv("WOLFBBS_UPLOAD_POLICY_HOOK", script)

	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	sysop, err := authSvc.Register("sysop", "password123")
	if err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole(sysop.Handle, roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}

	app := &webApp{
		authSvc:    authSvc,
		adminRepo:  adminRepo,
		sessions:   map[string]sessionState{},
		rateLimits: map[string][]time.Time{},
	}
	sid, ok := app.createSession(sysop.Handle)
	if !ok {
		t.Fatal("session creation failed")
	}

	tmpDir := t.TempDir()
	if err := adminRepo.CreateFileArea(&domain.FileArea{Name: "Uploads", Path: tmpDir, Description: "qa uploads"}); err != nil {
		t.Fatalf("create file area: %v", err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("action", "upload")
	_ = writer.WriteField("area_id", "1")
	_ = writer.WriteField("description", "Policy blocked")
	_ = writer.WriteField("tags", "retro")
	_ = writer.WriteField("csrf_token", app.sessions[sid].csrf)
	fileWriter, err := writer.CreateFormFile("upload_file", "blocked.ans")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fileWriter.Write([]byte("ANSI FILE CONTENT")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/files", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminFiles)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("upload file status = %d", rr.Code)
	}
	if got := rr.Header().Get("Location"); !strings.Contains(got, "error=") {
		t.Fatalf("expected error redirect, got %q", got)
	}
	files, err := adminRepo.ListFileEntries(0, "", nil, 10)
	if err != nil {
		t.Fatalf("list file entries: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected hook-rejected upload to be absent, got %+v", files)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "blocked.ans")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected rejected upload file to be removed, stat err=%v", err)
	}
}

func TestBoardsDashboardAndDoorCockpit(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	doorRepo := repository.NewInMemoryDoorRepository()
	authSvc := auth.NewService(userRepo)

	user, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if err := boardRepo.Create(&domain.Board{Name: "General", Description: "Main", CreatedBy: user.ID}); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   1,
		AuthorID:  user.ID,
		Subject:   "Unread topic",
		Body:      "hello world",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	if err := mailRepo.CreateMail(&domain.PrivateMail{
		FromUserID: 999,
		ToUserID:   user.ID,
		Subject:    "Unread mail",
		Body:       "hello caller",
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create mail: %v", err)
	}
	if err := adminRepo.AddCallerHistory(&domain.CallerHistory{
		SessionID:       "s1",
		NodeID:          1,
		Username:        "sysop",
		Area:            "Doors",
		RemoteAddr:      "203.0.113.10:2222",
		LoginAt:         time.Now().UTC().Add(-10 * time.Minute),
		LogoutAt:        time.Now().UTC().Add(-5 * time.Minute),
		DurationSeconds: 300,
	}); err != nil {
		t.Fatalf("add caller history: %v", err)
	}

	doorRegistry := doors.NewRegistry()
	doorRegistry.SetRepository(doorRepo)
	if err := doorRegistry.LoadManifestDir(filepath.Join("..", "..", "doors")); err != nil {
		t.Fatalf("load door manifests: %v", err)
	}
	lastPlayed := time.Now().UTC().Add(-30 * time.Minute)
	if err := doorRepo.UpsertUserMeta(&domain.DoorUserMeta{
		UserID:       user.ID,
		DoorID:       "dragon-tavern-legends",
		Favorite:     true,
		LastPlayedAt: &lastPlayed,
		PlayCount:    3,
	}); err != nil {
		t.Fatalf("seed user meta: %v", err)
	}
	if err := doorRepo.AddAchievement(&domain.DoorAchievement{
		DoorID:          "dragon-tavern-legends",
		UserID:          user.ID,
		AchievementCode: "first_quest",
	}); err != nil {
		t.Fatalf("seed achievement: %v", err)
	}
	if err := doorRepo.SubmitScore(&domain.DoorScore{
		DoorID:    "dragon-tavern-legends",
		UserID:    user.ID,
		ScoreType: "points",
		Value:     42,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed score: %v", err)
	}
	if err := doorRepo.AddEvent(&domain.DoorEvent{
		DoorID:    "dragon-tavern-legends",
		UserID:    user.ID,
		EventType: "door_launch",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	chatSvc := chat.NewServiceForTest()
	chatSvc.JoinChannel("caller", "#lobby")
	chatSvc.JoinChannel("sysop", "#lobby")
	oneLinerz := mods.NewOneLinerzMod(80)
	oneLinerz.Add("sysop", "Tonight is door night.")
	rumorz := mods.NewRumorzMod([]string{"Dragon Tavern is hot tonight."})
	bbsList := mods.NewBBSListMod(20)
	bbsList.Add("Night Owl BBS", "bbs.example.com", 23)

	app := &webApp{
		authSvc:       authSvc,
		userRepo:      userRepo,
		boardRepo:     boardRepo,
		msgRepo:       msgRepo,
		mailRepo:      mailRepo,
		adminRepo:     adminRepo,
		doorRepo:      doorRepo,
		doorRegistry:  doorRegistry,
		chatSvc:       chatSvc,
		sessions:      map[string]sessionState{},
		discover:      true,
		quickJump:     true,
		savedSearches: map[string][]string{},
		oneLinerzMod:  oneLinerz,
		rumorzMod:     rumorz,
		bbsListMod:    bbsList,
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/boards", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("boards status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"Caller Cockpit", "Door Cockpit", "Last Callers", "Tonight is door night", "Recommended door"} {
		if !strings.Contains(body, want) {
			t.Fatalf("boards dashboard missing %q: %s", want, body)
		}
	}
	for _, want := range []string{"Caller Radar", "Clubhouse"} {
		if !strings.Contains(body, want) {
			t.Fatalf("boards dashboard missing new action %q: %s", want, body)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/doors?mode=favorites", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleDoors(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("doors status = %d", rr.Code)
	}
	body = rr.Body.String()
	for _, want := range []string{"Door Cockpit", "Recommended For This Caller", "Dragon Tavern Legends", "favorite", "door launch"} {
		if !strings.Contains(strings.ToLower(body), strings.ToLower(want)) {
			t.Fatalf("door cockpit missing %q: %s", want, body)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/radar", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleRadar(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("radar status = %d", rr.Code)
	}
	body = rr.Body.String()
	for _, want := range []string{"Caller Radar", "Board Pulse", "Live Caller Radar", "Arcade Heat", "Dragon Tavern is hot tonight."} {
		if !strings.Contains(body, want) {
			t.Fatalf("radar missing %q: %s", want, body)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/clubhouse", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleClubhouse(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("clubhouse status = %d", rr.Code)
	}
	body = rr.Body.String()
	for _, want := range []string{"Clubhouse", "OneLinerz Wall", "Night Owl BBS", "Rumorz", "Tonight is door night"} {
		if !strings.Contains(body, want) {
			t.Fatalf("clubhouse missing %q: %s", want, body)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/scores", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleScores(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("scores status = %d", rr.Code)
	}
	body = rr.Body.String()
	for _, want := range []string{"Door Scores & Trophies", "Current Champions", "Your Scorecard", "Dragon Tavern Legends"} {
		if !strings.Contains(body, want) {
			t.Fatalf("scores missing %q: %s", want, body)
		}
	}

	form := url.Values{}
	form.Set("action", "toggle_favorite")
	form.Set("door_id", "dragon-tavern-legends")
	form.Set("csrf_token", app.sessions[sid].csrf)
	req = httptest.NewRequest(http.MethodPost, "/doors", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleDoors(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("door toggle status = %d", rr.Code)
	}
	meta, err := doorRepo.GetUserMeta(user.ID, "dragon-tavern-legends")
	if err != nil {
		t.Fatalf("load updated user meta: %v", err)
	}
	if meta.Favorite {
		t.Fatalf("expected favorite to be toggled off, got %+v", meta)
	}

	form = url.Values{}
	form.Set("action", "add_oneliner")
	form.Set("text", "Clubhouse test line")
	form.Set("csrf_token", app.sessions[sid].csrf)
	req = httptest.NewRequest(http.MethodPost, "/clubhouse", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleClubhouse(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("clubhouse oneliner post status = %d", rr.Code)
	}
	if got := app.oneLinerzMod.List(1); len(got) == 0 || !strings.Contains(got[0].Text, "Clubhouse test line") {
		t.Fatalf("expected clubhouse one-liner to be posted, got %+v", got)
	}

	form = url.Values{}
	form.Set("action", "add_bbs")
	form.Set("name", "Skyline BBS")
	form.Set("host", "skyline.example.com")
	form.Set("port", "2323")
	form.Set("csrf_token", app.sessions[sid].csrf)
	req = httptest.NewRequest(http.MethodPost, "/clubhouse", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleClubhouse(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("clubhouse bbs post status = %d", rr.Code)
	}
	if got := app.bbsListMod.List(1); len(got) == 0 || got[0].Name != "Skyline BBS" {
		t.Fatalf("expected clubhouse bbs listing, got %+v", got)
	}
}

func TestLegacyModernClassicRoutes(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	doorRepo := repository.NewInMemoryDoorRepository()
	authSvc := auth.NewService(userRepo)

	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	friend, err := authSvc.Register("friend", "password123")
	if err != nil {
		t.Fatalf("register friend: %v", err)
	}
	sysop, err := authSvc.Register("sysop", "password123")
	if err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole(sysop.Handle, roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	if err := authSvc.SetVerified(friend.Handle, true); err != nil {
		t.Fatalf("verify friend: %v", err)
	}
	if err := boardRepo.Create(&domain.Board{Name: "General", Description: "Main", Conference: "Public", CreatedBy: sysop.ID}); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := boardRepo.Create(&domain.Board{Name: "Ops", Description: "Staff notes", Conference: "Ops", CreatedBy: sysop.ID}); err != nil {
		t.Fatalf("create second board: %v", err)
	}
	post := &domain.Message{
		BoardID:   1,
		AuthorID:  caller.ID,
		Subject:   "Classic thread kickoff",
		Body:      "Hello from caller.",
		CreatedAt: time.Now().UTC().Add(-2 * time.Hour),
	}
	if err := msgRepo.CreateMessage(post); err != nil {
		t.Fatalf("create post: %v", err)
	}
	reply := &domain.Message{
		BoardID:   1,
		AuthorID:  friend.ID,
		ParentID:  post.ID,
		Subject:   "Re: Classic thread kickoff",
		Body:      "caller, this is a tracked reply.",
		CreatedAt: time.Now().UTC().Add(-time.Hour),
	}
	if err := msgRepo.CreateMessage(reply); err != nil {
		t.Fatalf("create reply: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   2,
		AuthorID:  sysop.ID,
		Subject:   "Sysop note",
		Body:      "Operational chatter only.",
		CreatedAt: time.Now().UTC().Add(-45 * time.Minute),
	}); err != nil {
		t.Fatalf("create second board message: %v", err)
	}
	if err := mailRepo.CreateMail(&domain.PrivateMail{
		FromUserID: friend.ID,
		ToUserID:   caller.ID,
		Subject:    "Ping caller",
		Body:       "mail body",
		CreatedAt:  time.Now().UTC().Add(-90 * time.Minute),
	}); err != nil {
		t.Fatalf("seed inbox mail: %v", err)
	}
	if err := mailRepo.CreateMail(&domain.PrivateMail{
		FromUserID: caller.ID,
		ToUserID:   friend.ID,
		Subject:    "Re: Ping caller",
		Body:       "thanks",
		CreatedAt:  time.Now().UTC().Add(-80 * time.Minute),
	}); err != nil {
		t.Fatalf("seed outbox mail: %v", err)
	}
	if err := adminRepo.UpsertNodeSession(&domain.NodeSession{
		SessionID:    "friend-live",
		NodeID:       2,
		Username:     friend.Handle,
		Area:         "Chat",
		RemoteAddr:   "198.51.100.9:2222",
		LoginAt:      time.Now().UTC().Add(-15 * time.Minute),
		LastActivity: time.Now().UTC().Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed node session: %v", err)
	}
	if err := adminRepo.AddCallerHistory(&domain.CallerHistory{
		SessionID:       "friend-old",
		NodeID:          2,
		Username:        friend.Handle,
		Area:            "Boards",
		RemoteAddr:      "198.51.100.9:2222",
		LoginAt:         time.Now().UTC().Add(-40 * time.Minute),
		LogoutAt:        time.Now().UTC().Add(-20 * time.Minute),
		DurationSeconds: 1200,
	}); err != nil {
		t.Fatalf("seed caller history: %v", err)
	}
	if err := adminRepo.CreateFileArea(&domain.FileArea{Name: "Uploads", Path: "/tmp/uploads"}); err != nil {
		t.Fatalf("create file area: %v", err)
	}
	if err := adminRepo.UpsertFileEntry(&domain.FileEntry{
		AreaID:      1,
		Name:        "retro-pack.zip",
		Path:        "/tmp/uploads/retro-pack.zip",
		Description: "Classic file archive",
		Tags:        []string{"zip", "retro"},
		SizeBytes:   1024,
		UploaderID:  friend.ID,
		UploadedAt:  time.Now().UTC().Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("seed file entry: %v", err)
	}
	if err := adminRepo.SetFileRating(caller.ID, 1, 5); err != nil {
		t.Fatalf("rate file: %v", err)
	}
	if err := adminRepo.SaveFileFilter(&domain.FileFilter{
		UserID: caller.ID,
		Name:   "retro",
		Query:  "retro",
		Tags:   []string{"zip"},
	}); err != nil {
		t.Fatalf("save file filter: %v", err)
	}
	if err := adminRepo.EnqueueDownload(caller.ID, 1); err != nil {
		t.Fatalf("enqueue file: %v", err)
	}

	oneLinerz := mods.NewOneLinerzMod(20)
	oneLinerz.Add(friend.Handle, "Still dialing in from the frontier.")
	rumorz := mods.NewRumorzMod([]string{"Rumor has it the sysop reads feedback before coffee."})

	app := &webApp{
		authSvc:       authSvc,
		userRepo:      userRepo,
		boardRepo:     boardRepo,
		msgRepo:       msgRepo,
		mailRepo:      mailRepo,
		adminRepo:     adminRepo,
		doorRepo:      doorRepo,
		sessions:      map[string]sessionState{},
		discover:      true,
		classicSearch: true,
		savedSearches: map[string][]string{},
		oneLinerzMod:  oneLinerz,
		rumorzMod:     rumorz,
	}
	sid, ok := app.createSession(caller.Handle)
	if !ok {
		t.Fatal("session creation failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/bulletins", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleBulletins(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("bulletins status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"Bulletin Center", "Spotlight", "Classic thread kickoff", "retro-pack.zip", "Still dialing in from the frontier."} {
		if !strings.Contains(body, want) {
			t.Fatalf("bulletins missing %q: %s", want, body)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/directory?handle=friend", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("directory status = %d", rr.Code)
	}
	body = rr.Body.String()
	for _, want := range []string{"Caller Directory", "Caller Card: friend", "Send mail", "Caller Stats"} {
		if !strings.Contains(body, want) {
			t.Fatalf("directory missing %q: %s", want, body)
		}
	}
	req = httptest.NewRequest(http.MethodGet, "/directory?role=user&verified=verified&online=1", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("directory filtered status = %d", rr.Code)
	}
	body = rr.Body.String()
	for _, want := range []string{"visible callers", "Origin", "WAN"} {
		if !strings.Contains(body, want) {
			t.Fatalf("directory filtered missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "/directory?handle=sysop") {
		t.Fatalf("directory filtered view should not include sysop row: %s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/boards?mode=mentions&q=General", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("boards status = %d", rr.Code)
	}
	body = rr.Body.String()
	for _, want := range []string{"Personal Board Queue", "Mentions", "General"} {
		if !strings.Contains(body, want) {
			t.Fatalf("boards missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `<tr><td>2</td><td><a href="/boards?board=2">Ops</a></td>`) {
		t.Fatalf("boards mentions filter should not include Ops table row: %s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/finder?q=caller&save=1&board=1&author=friend&tracker=reply", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleFinder(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("finder status = %d", rr.Code)
	}
	body = rr.Body.String()
	for _, want := range []string{"Message Finder", "Thread Tracker", "Classic thread kickoff", "Saved Queries", "all tracker items", "Conf"} {
		if !strings.Contains(body, want) {
			t.Fatalf("finder missing %q: %s", want, body)
		}
	}
	if len(app.savedSearchList(caller.Handle)) == 0 {
		t.Fatal("expected saved finder query")
	}

	req = httptest.NewRequest(http.MethodGet, "/newfiles?tag=zip&sort=rating&since=30d", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleNewFiles(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("newfiles status = %d", rr.Code)
	}
	body = rr.Body.String()
	for _, want := range []string{"New Files Desk", "retro-pack.zip", "Download Desk", "Top Rated Picks", "visible uploads", "saved filters"} {
		if !strings.Contains(body, want) {
			t.Fatalf("newfiles missing %q: %s", want, body)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/mail?to=friend&box=unread&q=Ping&template=door_invite", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("mail status = %d", rr.Code)
	}
	body = rr.Body.String()
	for _, want := range []string{"Recent Correspondents", "Address Book", "Saved Reply Kits", `value="friend"`, "Meet me in the Door Hub", "all mail", "Active Mail Filters", `data-draft-key="mail-compose"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("mail missing %q: %s", want, body)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/mail?id=1", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("mail reader status = %d", rr.Code)
	}
	body = rr.Body.String()
	for _, want := range []string{"Quick Reply", "Send Reply", "Ping caller", "Reply in context", `data-draft-key="mail-reply-1"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("mail reader missing %q: %s", want, body)
		}
	}

	form := url.Values{}
	form.Set("category", "bug")
	form.Set("subject", "Screen issue")
	form.Set("body", "A classic bug report for the sysop.")
	form.Set("csrf_token", app.sessions[sid].csrf)
	req = httptest.NewRequest(http.MethodPost, "/feedback", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleFeedback(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("feedback post status = %d", rr.Code)
	}
	sysopInbox, err := mailRepo.ListInbox(sysop.ID, 20)
	if err != nil {
		t.Fatalf("load sysop inbox: %v", err)
	}
	found := false
	for _, row := range sysopInbox {
		if strings.Contains(row.Subject, "Screen issue") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected feedback mail in sysop inbox, got %+v", sysopInbox)
	}
}

func TestHandleRootRedirectTargets(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("rootuser", "password123"); err != nil {
		t.Fatalf("register: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		sessions:  map[string]sessionState{},
		chatSvc:   chat.NewServiceForTest(),
		doorRepo:  repository.NewInMemoryDoorRepository(),
		boardRepo: repository.NewInMemoryBoardRepository(),
		msgRepo:   repository.NewInMemoryMessageRepository(),
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	app.handleRoot(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	if location := rr.Result().Header.Get("Location"); location != "/login" {
		t.Fatalf("expected /login redirect by default, got %q", location)
	}

	app.modernOnRamp = true
	rr = httptest.NewRecorder()
	app.handleRoot(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	if location := rr.Result().Header.Get("Location"); location != "/connect" {
		t.Fatalf("expected /connect redirect when on-ramp enabled, got %q", location)
	}

	sid, ok := app.createSession("rootuser")
	if !ok {
		t.Fatal("session creation failed")
	}
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleRoot(rr, req)
	if location := rr.Result().Header.Get("Location"); location != "/boards" {
		t.Fatalf("expected authenticated redirect to /boards, got %q", location)
	}
}

func TestLoginPageFlags(t *testing.T) {
	page := loginPage("WolfBBS", "/login", false, false)
	if strings.Contains(page, "/connect") || strings.Contains(page, "/tour") {
		t.Fatalf("unexpected on-ramp links when flags disabled: %s", page)
	}
	page = loginPage("WolfBBS", "/login", true, true)
	if !strings.Contains(page, "/connect") || !strings.Contains(page, "/tour") {
		t.Fatalf("expected on-ramp links when flags enabled: %s", page)
	}
}

func TestPasswordResetRequestInvokesNotifierForEmailHandle(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	resetRepo := repository.NewInMemoryPasswordResetRepository()
	authSvc := auth.NewService(userRepo)
	authSvc.SetPasswordResetRepository(resetRepo)
	if _, err := authSvc.Register("reset@example.com", "password123"); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	notified := false
	notifiedHandle := ""
	notifiedToken := ""
	notifiedPath := ""
	app := &webApp{
		authSvc:  authSvc,
		userRepo: userRepo,
		sessions: map[string]sessionState{},
		resetTTL: 5 * time.Minute,
		resetNotifier: func(handle, token string, r *http.Request) error {
			notified = true
			notifiedHandle = handle
			notifiedToken = token
			if r != nil && r.URL != nil {
				notifiedPath = r.URL.Path
			}
			return nil
		},
	}

	form := url.Values{}
	form.Set("handle", "reset@example.com")
	req := httptest.NewRequest(http.MethodPost, "/reset/request", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	app.handlePasswordResetRequest(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reset request status = %d", rr.Code)
	}
	if !notified {
		t.Fatal("expected reset notifier to be called")
	}
	if notifiedHandle != "reset@example.com" {
		t.Fatalf("unexpected notified handle %q", notifiedHandle)
	}
	if len(strings.TrimSpace(notifiedToken)) < 16 {
		t.Fatalf("unexpected token value %q", notifiedToken)
	}
	if notifiedPath != "/reset/request" {
		t.Fatalf("unexpected request path %q", notifiedPath)
	}
}

func TestHandleLoginRateLimitsFailedAttempts(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("reader", "password123"); err != nil {
		t.Fatalf("register: %v", err)
	}
	app := &webApp{
		authSvc:         authSvc,
		userRepo:        userRepo,
		sessions:        map[string]sessionState{},
		rateLimits:      map[string][]time.Time{},
		loginRateLimit:  2,
		loginRateWindow: time.Hour,
	}

	post := func(password string) *httptest.ResponseRecorder {
		form := url.Values{}
		form.Set("handle", "reader")
		form.Set("password", password)
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.RemoteAddr = "198.51.100.50:4444"
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		app.handleLogin(rr, req)
		return rr
	}

	if rr := post("wrongpass"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("first failed login status = %d", rr.Code)
	}
	if rr := post("wrongpass"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("second failed login status = %d", rr.Code)
	}
	if rr := post("wrongpass"); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("third failed login status = %d", rr.Code)
	}
}

func TestPasswordResetRequestRateLimit(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	resetRepo := repository.NewInMemoryPasswordResetRepository()
	authSvc := auth.NewService(userRepo)
	authSvc.SetPasswordResetRepository(resetRepo)
	if _, err := authSvc.Register("resetme", "password123"); err != nil {
		t.Fatalf("register: %v", err)
	}
	app := &webApp{
		authSvc:         authSvc,
		userRepo:        userRepo,
		sessions:        map[string]sessionState{},
		rateLimits:      map[string][]time.Time{},
		resetRateLimit:  2,
		resetRateWindow: time.Hour,
	}

	form := url.Values{}
	form.Set("handle", "resetme")
	for attempt := 1; attempt <= 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/reset/request", strings.NewReader(form.Encode()))
		req.RemoteAddr = "198.51.100.60:5555"
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		app.handlePasswordResetRequest(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("reset request %d status = %d", attempt, rr.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/reset/request", strings.NewReader(form.Encode()))
	req.RemoteAddr = "198.51.100.60:5555"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	app.handlePasswordResetRequest(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for rate-limited reset request, got %d", rr.Code)
	}
}

func TestActivityPubBaseIgnoresHostHeaderPoisoning(t *testing.T) {
	app := &webApp{
		siteHostname: "bbs.example",
		secureCookie: true,
	}
	req := httptest.NewRequest(http.MethodGet, "/ap/users/alice", nil)
	req.Host = "attacker.example"
	req.Header.Set("X-Forwarded-Proto", "https")
	if got := app.activityPubBase(req); got != "https://bbs.example" {
		t.Fatalf("activityPubBase() = %q", got)
	}
}

func TestParseJSONBodyRejectsOversizePayload(t *testing.T) {
	payload := `{"message":"` + strings.Repeat("a", maxJSONRequestBytes) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/chat/send", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	var body map[string]string
	if err := parseJSONBody(req, &body); err == nil {
		t.Fatal("expected oversized json payload to be rejected")
	}
}

func TestHandleMailInboundBlocksPublicDefaultToken(t *testing.T) {
	app := &webApp{
		inboundToken: defaultInboundToken,
	}
	body := strings.NewReader(`{"from":"sender@example.com","to":"reader@example.com","subject":"hello","body":"world"}`)
	req := httptest.NewRequest(http.MethodPost, "/mail/inbound", body)
	req.RemoteAddr = "198.51.100.70:6666"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Inbound-Token", defaultInboundToken)
	rr := httptest.NewRecorder()
	app.handleMailInbound(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for public default inbound token, got %d", rr.Code)
	}
}

func TestHandleSuggestionsReturnsMatches(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if _, err := authSvc.Register("systembot", "password123"); err != nil {
		t.Fatalf("register systembot: %v", err)
	}
	app := &webApp{
		authSvc:  authSvc,
		sessions: map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/handles/suggest?q=sys", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleHandleSuggestions(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var payload map[string][]string
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	items := payload["items"]
	if len(items) == 0 {
		t.Fatalf("expected suggestions, got none")
	}
	if items[0] != "sysop" && items[0] != "systembot" {
		t.Fatalf("unexpected first suggestion: %+v", items)
	}
	for _, item := range items {
		if strings.EqualFold(item, "caller") {
			t.Fatalf("expected current caller to be excluded, got %+v", items)
		}
	}
}

func TestBookmarksCanSaveBoardAndMail(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)

	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	friend, err := authSvc.Register("friend", "password123")
	if err != nil {
		t.Fatalf("register friend: %v", err)
	}
	board := &domain.Board{Name: "General", Conference: "General", CreatedBy: friend.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	msg := &domain.Message{BoardID: board.ID, AuthorID: friend.ID, Subject: "Read Later Thread", Body: "Save this one."}
	if err := msgRepo.CreateMessage(msg); err != nil {
		t.Fatalf("create message: %v", err)
	}
	mail := &domain.PrivateMail{FromUserID: friend.ID, ToUserID: caller.ID, Subject: "Saved Mail", Body: "Mail body"}
	if err := mailRepo.CreateMail(mail); err != nil {
		t.Fatalf("create mail: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		mailRepo:  mailRepo,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	csrf := app.sessions[sid].csrf

	postForm := func(values url.Values) {
		req := httptest.NewRequest(http.MethodPost, "/bookmarks", strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
		rr := httptest.NewRecorder()
		app.handleBookmarks(rr, req)
		if rr.Code != http.StatusFound {
			t.Fatalf("expected redirect, got %d body=%s", rr.Code, rr.Body.String())
		}
	}

	values := url.Values{}
	values.Set("action", "add_board_message")
	values.Set("message_id", strconv.FormatInt(msg.ID, 10))
	values.Set("return_to", "/bookmarks")
	values.Set("csrf_token", csrf)
	postForm(values)

	values = url.Values{}
	values.Set("action", "add_mail")
	values.Set("mail_id", strconv.FormatInt(mail.ID, 10))
	values.Set("return_to", "/bookmarks")
	values.Set("csrf_token", csrf)
	postForm(values)

	req := httptest.NewRequest(http.MethodGet, "/bookmarks", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleBookmarks(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"Read-Later Queue", "Read Later Thread", "Saved Mail"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in bookmarks page: %s", needle, body)
		}
	}
}

func TestBookmarksRejectExternalReturnTo(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)

	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	friend, err := authSvc.Register("friend", "password123")
	if err != nil {
		t.Fatalf("register friend: %v", err)
	}
	board := &domain.Board{Name: "General", Conference: "General", CreatedBy: friend.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	msg := &domain.Message{BoardID: board.ID, AuthorID: friend.ID, Subject: "Read Later Thread", Body: "Save this one."}
	if err := msgRepo.CreateMessage(msg); err != nil {
		t.Fatalf("create message: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession(caller.Handle)
	if !ok {
		t.Fatal("session creation failed")
	}
	values := url.Values{}
	values.Set("action", "add_board_message")
	values.Set("message_id", strconv.FormatInt(msg.ID, 10))
	values.Set("return_to", "//evil.example/phish")
	values.Set("csrf_token", app.sessions[sid].csrf)
	req := httptest.NewRequest(http.MethodPost, "/bookmarks", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleBookmarks(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d body=%s", rr.Code, rr.Body.String())
	}
	location := rr.Header().Get("Location")
	if strings.HasPrefix(location, "//") || strings.Contains(location, "evil.example") {
		t.Fatalf("expected local redirect, got %q", location)
	}
	if !strings.HasPrefix(location, "/bookmarks?") {
		t.Fatalf("expected bookmarks redirect, got %q", location)
	}
}

func TestDirectoryShowsPresenceDetails(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	user, err := authSvc.GetUser("caller")
	if err != nil || user == nil {
		t.Fatalf("get caller: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	now := time.Now().UTC()
	if err := adminRepo.UpsertNodeSession(&domain.NodeSession{
		SessionID:    "n1",
		NodeID:       1,
		Username:     user.Handle,
		Area:         "chat",
		RemoteAddr:   "198.51.100.10:2323",
		LoginAt:      now.Add(-15 * time.Minute),
		LastActivity: now.Add(-2 * time.Minute),
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("upsert node session: %v", err)
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/directory?handle=caller", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"Node 1", "Idle", "198.51.100.10"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in directory page: %s", needle, body)
		}
	}
	if strings.Count(body, "Node 1") < 2 {
		t.Fatalf("expected live node details in both caller card and table row: %s", body)
	}
	if strings.Count(body, "198.51.100.10") < 2 {
		t.Fatalf("expected live origin details in both caller card and table row: %s", body)
	}
}

func TestDirectoryRejectsExternalReturnTo(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if _, err := authSvc.Register("friend", "password123"); err != nil {
		t.Fatalf("register friend: %v", err)
	}
	app := &webApp{
		authSvc:  authSvc,
		sessions: map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	form := url.Values{
		"csrf_token": {app.sessions[sid].csrf},
		"action":     {"toggle_favorite"},
		"target":     {"friend"},
		"return_to":  {"//evil.example/phish"},
	}
	req := httptest.NewRequest(http.MethodPost, "/directory", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d body=%s", rr.Code, rr.Body.String())
	}
	location := rr.Header().Get("Location")
	if strings.HasPrefix(location, "//") || strings.Contains(location, "evil.example") {
		t.Fatalf("expected local redirect, got %q", location)
	}
	if !strings.HasPrefix(location, "/directory?") {
		t.Fatalf("expected directory redirect, got %q", location)
	}
}

func TestDirectoryHidesDisabledAndBannedUsers(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if _, err := authSvc.Register("disabled", "password123"); err != nil {
		t.Fatalf("register disabled: %v", err)
	}
	if _, err := authSvc.Register("banned", "password123"); err != nil {
		t.Fatalf("register banned: %v", err)
	}
	if err := authSvc.SetEnabled("disabled", false); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if err := authSvc.SetBanned("banned", true); err != nil {
		t.Fatalf("ban user: %v", err)
	}
	app := &webApp{
		authSvc:  authSvc,
		sessions: map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/directory", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, hidden := range []string{"disabled", "banned"} {
		if strings.Contains(body, `>`+hidden+`</a>`) {
			t.Fatalf("expected hidden account %q to be absent from directory: %s", hidden, body)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/directory?handle=banned", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for banned profile request, got %d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "Caller Card: banned") {
		t.Fatalf("expected banned caller profile to stay hidden: %s", rr.Body.String())
	}
}

func TestGatewayTicketRejectsExternalReturnTo(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	user, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register user: %v", err)
	}

	dir := t.TempDir()
	filePath := filepath.Join(dir, "wolfbbs-guide.txt")
	if err := os.WriteFile(filePath, []byte("wolfbbs-file-content"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	if err := adminRepo.CreateFileArea(&domain.FileArea{Name: "Uploads", Path: dir, Description: "test"}); err != nil {
		t.Fatalf("create area: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	if _, _, err := app.indexAreaFiles(domain.FileArea{ID: 1, Name: "Uploads", Path: dir}, user.ID); err != nil {
		t.Fatalf("index area files: %v", err)
	}
	files, err := adminRepo.ListFileEntries(0, "wolfbbs-guide", nil, 10)
	if err != nil {
		t.Fatalf("list indexed files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one indexed file, got %d", len(files))
	}
	sid, ok := app.createSession(user.Handle)
	if !ok {
		t.Fatal("create session failed")
	}
	form := url.Values{}
	form.Set("action", "ticket")
	form.Set("file_id", strconv.FormatInt(files[0].ID, 10))
	form.Set("ttl_minutes", "10")
	form.Set("return_to", "//evil.example/phish")
	form.Set("csrf_token", app.sessions[sid].csrf)
	req := httptest.NewRequest(http.MethodPost, "/gateway", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d body=%s", rr.Code, rr.Body.String())
	}
	location := rr.Header().Get("Location")
	if strings.HasPrefix(location, "//") || strings.Contains(location, "evil.example") {
		t.Fatalf("expected local redirect, got %q", location)
	}
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse redirect %q: %v", location, err)
	}
	if parsed.Path != "/gateway" {
		t.Fatalf("expected /gateway redirect path, got %q", parsed.Path)
	}
	if parsed.Query().Get("view") != "files" {
		t.Fatalf("expected fallback file view redirect, got %q", location)
	}
	if strings.TrimSpace(parsed.Query().Get("issued_token")) == "" {
		t.Fatalf("expected issued_token on redirect, got %q", location)
	}
	if !strings.Contains(parsed.Query().Get("notice"), "Download ticket issued") {
		t.Fatalf("expected notice on redirect, got %q", location)
	}
}

func TestGatewayFilesShowsPreviewAndRelatedUploads(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	area := &domain.FileArea{Name: "Uploads", Path: t.TempDir(), Description: "test area"}
	if err := adminRepo.CreateFileArea(area); err != nil {
		t.Fatalf("create area: %v", err)
	}
	entry := &domain.FileEntry{AreaID: area.ID, Name: "ansi-pack.zip", Path: filepath.Join(area.Path, "ansi-pack.zip"), Description: "ANSI collection", Tags: []string{"ansi", "pack"}, SHA256: "abc", SizeBytes: 42, UploadedAt: time.Now().UTC()}
	if err := adminRepo.UpsertFileEntry(entry); err != nil {
		t.Fatalf("upsert entry: %v", err)
	}
	related := &domain.FileEntry{AreaID: area.ID, Name: "ansi-lab.zip", Path: filepath.Join(area.Path, "ansi-lab.zip"), Description: "Related ANSI upload", Tags: []string{"ansi", "lab"}, SHA256: "def", SizeBytes: 24, UploadedAt: time.Now().UTC()}
	if err := adminRepo.UpsertFileEntry(related); err != nil {
		t.Fatalf("upsert related entry: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/gateway?view=files&id="+strconv.FormatInt(entry.ID, 10), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"File Preview", "Related Uploads", "ansi-lab.zip"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in file preview page: %s", needle, body)
		}
	}
}

func TestAdminFilesTicketShowsExpiryCard(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	sysop, err := authSvc.Register("sysop", "password123")
	if err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}

	dir := t.TempDir()
	filePath := filepath.Join(dir, "ops-guide.txt")
	if err := os.WriteFile(filePath, []byte("ops"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	if err := adminRepo.CreateFileArea(&domain.FileArea{Name: "Uploads", Path: dir, Description: "test"}); err != nil {
		t.Fatalf("create area: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	if _, _, err := app.indexAreaFiles(domain.FileArea{ID: 1, Name: "Uploads", Path: dir}, sysop.ID); err != nil {
		t.Fatalf("index area files: %v", err)
	}
	files, err := adminRepo.ListFileEntries(0, "ops-guide", nil, 10)
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one indexed file, got %d", len(files))
	}
	if err := adminRepo.EnqueueDownload(sysop.ID, files[0].ID); err != nil {
		t.Fatalf("enqueue file: %v", err)
	}

	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	form := url.Values{}
	form.Set("action", "ticket")
	form.Set("queue_user_id", strconv.FormatInt(sysop.ID, 10))
	form.Set("file_id", strconv.FormatInt(files[0].ID, 10))
	form.Set("ttl_minutes", "30")
	form.Set("csrf_token", app.sessions[sid].csrf)
	req := httptest.NewRequest(http.MethodPost, "/admin/files", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminFiles)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("issue ticket status = %d body=%s", rr.Code, rr.Body.String())
	}
	location := rr.Header().Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse redirect %q: %v", location, err)
	}
	if strings.TrimSpace(parsed.Query().Get("issued_token")) == "" {
		t.Fatalf("expected issued token, got %q", location)
	}
	if strings.TrimSpace(parsed.Query().Get("issued_expires")) == "" {
		t.Fatalf("expected issued expires, got %q", location)
	}

	req = httptest.NewRequest(http.MethodGet, parsed.Path+"?"+parsed.RawQuery, nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminFiles)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin files view status = %d body=%s", rr.Code, rr.Body.String())
	}
	for _, needle := range []string{"Issued Ticket", "Expires:", "ops-guide.txt"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("expected %q in admin files ticket card: %s", needle, rr.Body.String())
		}
	}
}

func TestGatewayFileRequestCollectionsAndOfflineWorkflow(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()

	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	sysop, err := authSvc.Register("sysop", "password123")
	if err != nil {
		t.Fatalf("register sysop: %v", err)
	}

	area := &domain.FileArea{Name: "Uploads", Path: t.TempDir(), Description: "test area"}
	if err := adminRepo.CreateFileArea(area); err != nil {
		t.Fatalf("create area: %v", err)
	}
	fileA := filepath.Join(area.Path, "ansi-pack.zip")
	if err := os.WriteFile(fileA, []byte("ansi-pack"), 0o644); err != nil {
		t.Fatalf("write fileA: %v", err)
	}
	fileB := filepath.Join(area.Path, "ansi-pack-v2.zip")
	if err := os.WriteFile(fileB, []byte("ansi-pack-v2"), 0o644); err != nil {
		t.Fatalf("write fileB: %v", err)
	}
	entry := &domain.FileEntry{AreaID: area.ID, Name: "ansi-pack.zip", Path: fileA, Description: "ANSI collection", Tags: []string{"ansi", "pack"}, SHA256: "abc", SizeBytes: 9, UploaderID: caller.ID, UploadedAt: time.Now().UTC()}
	if err := adminRepo.UpsertFileEntry(entry); err != nil {
		t.Fatalf("upsert entry: %v", err)
	}
	related := &domain.FileEntry{AreaID: area.ID, Name: "ansi-pack-v2.zip", Path: fileB, Description: "Duplicate candidate", Tags: []string{"ansi", "pack"}, SHA256: "abc", SizeBytes: 12, UploaderID: caller.ID, UploadedAt: time.Now().UTC()}
	if err := adminRepo.UpsertFileEntry(related); err != nil {
		t.Fatalf("upsert related: %v", err)
	}

	board := &domain.Board{Name: "General", Conference: "Local", CreatedBy: caller.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	msg := &domain.Message{BoardID: board.ID, AuthorID: sysop.ID, Subject: "Packet subject", Body: "Offline packet body"}
	if err := msgRepo.CreateMessage(msg); err != nil {
		t.Fatalf("create message: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		mailRepo:  mailRepo,
		sessions:  map[string]sessionState{},
	}
	app.setCuratorNote(entry.ID, fileCuratorNote{Curator: "sysop", Note: "Start here for curated ANSI picks."})
	app.upsertFeaturedCollection(featuredFileCollection{
		ID:          "featured-ansi",
		Title:       "ANSI Starter Pack",
		Description: "A guided bundle of classic ANSI files.",
		FileIDs:     []int64{entry.ID, related.ID},
		Tags:        []string{"ansi", "pack"},
		Curator:     "sysop",
	})
	app.setBoardSubscription(caller.Handle, board.ID, boardSubscriptionWatch)

	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	csrf := app.sessions[sid].csrf

	form := url.Values{}
	form.Set("action", "request_file")
	form.Set("title", "Need ANSI docs")
	form.Set("description", "Looking for a curated guide to ANSI workflows.")
	form.Set("desired_area", strconv.FormatInt(area.ID, 10))
	form.Set("return_to", "/gateway?view=files")
	form.Set("csrf_token", csrf)
	req := httptest.NewRequest(http.MethodPost, "/gateway", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("request file status = %d", rr.Code)
	}
	requests := app.loadFileRequests()
	if len(requests) != 1 || requests[0].Title != "Need ANSI docs" {
		t.Fatalf("expected saved file request, got %+v", requests)
	}

	req = httptest.NewRequest(http.MethodGet, "/gateway?view=files&id="+strconv.FormatInt(entry.ID, 10), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("gateway preview status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"Curator Note", "Duplicate Check", "ANSI Starter Pack", "Need ANSI docs"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in gateway preview body: %s", needle, body)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/collections", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleCollections(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "ANSI Starter Pack") {
		t.Fatalf("collections page missing curated bundle: status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/offline?download=watched&format=json", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleOffline(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("offline export status = %d", rr.Code)
	}
	var packet offlinePacket
	if err := json.Unmarshal(rr.Body.Bytes(), &packet); err != nil {
		t.Fatalf("decode packet: %v", err)
	}
	if len(packet.Boards) == 0 || packet.Boards[0].Name != "General" {
		t.Fatalf("expected watched board in packet, got %+v", packet)
	}

	replyPayload := `{"replies":[{"to":"sysop","subject":"Re: packet catch-up","body":"Imported offline reply","urgency":"normal"}]}`
	form = url.Values{}
	form.Set("action", "import_mail_replies")
	form.Set("payload", replyPayload)
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/offline", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleOffline(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("offline import status = %d", rr.Code)
	}
	inbox, err := mailRepo.ListInbox(sysop.ID, 10)
	if err != nil || len(inbox) != 1 || !strings.Contains(inbox[0].Body, "Imported offline reply") {
		t.Fatalf("expected imported offline mail, got err=%v inbox=%+v", err, inbox)
	}
}

func TestAdminFilesWorkflowSectionsAndRepairTools(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()

	sysop, err := authSvc.Register("sysop", "password123")
	if err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}
	area := &domain.FileArea{Name: "Uploads", Path: t.TempDir(), Description: "test area"}
	if err := adminRepo.CreateFileArea(area); err != nil {
		t.Fatalf("create area: %v", err)
	}
	brokenPath := filepath.Join(area.Path, "broken.zip")
	if err := os.WriteFile(brokenPath, []byte("broken"), 0o644); err != nil {
		t.Fatalf("write broken file: %v", err)
	}
	entry := &domain.FileEntry{
		AreaID:      area.ID,
		Name:        "broken.zip",
		Path:        brokenPath,
		Description: "Needs hash repair",
		Tags:        []string{"repair"},
		UploaderID:  sysop.ID,
		UploadedAt:  time.Now().UTC(),
	}
	if err := adminRepo.UpsertFileEntry(entry); err != nil {
		t.Fatalf("upsert entry: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	app.upsertFileRequest(fileRequestItem{
		ID:          "req-1",
		Requester:   "caller",
		Title:       "Upload docs",
		Description: "Please add docs to the filebase.",
		DesiredArea: area.ID,
		Status:      "open",
		CreatedAt:   time.Now().UTC(),
	})
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	csrf := app.sessions[sid].csrf

	form := url.Values{}
	form.Set("action", "save_upload_draft")
	form.Set("draft_name", "door-docs.zip")
	form.Set("draft_area_id", strconv.FormatInt(area.ID, 10))
	form.Set("draft_status", "review")
	form.Set("draft_reviewer", "sysop")
	form.Set("draft_tags", "docs,door")
	form.Set("draft_source", "request")
	form.Set("draft_notes", "waiting on packaging")
	form.Set("csrf_token", csrf)
	req := httptest.NewRequest(http.MethodPost, "/admin/files", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminFiles)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save draft status = %d", rr.Code)
	}

	form = url.Values{}
	form.Set("action", "save_collection")
	form.Set("collection_title", "Docs Bundle")
	form.Set("collection_tags", "docs")
	form.Set("collection_file_ids", strconv.FormatInt(entry.ID, 10))
	form.Set("collection_description", "Documentation picks.")
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/files", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminFiles)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save collection status = %d", rr.Code)
	}

	form = url.Values{}
	form.Set("action", "repair_hash")
	form.Set("file_id", strconv.FormatInt(entry.ID, 10))
	form.Set("csrf_token", csrf)
	req = httptest.NewRequest(http.MethodPost, "/admin/files", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminFiles)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("repair hash status = %d", rr.Code)
	}
	updated, err := adminRepo.GetFileEntry(entry.ID)
	if err != nil || strings.TrimSpace(updated.SHA256) == "" {
		t.Fatalf("expected repaired hash, got err=%v entry=%+v", err, updated)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/files", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminFiles)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin files status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"Curator Lane", "Next Curator Actions", "File Request Queue", "Upload Drafts", "Featured Collections", "Recovery Assistant", "Upload docs", "door-docs.zip", "Docs Bundle"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q on admin files page: %s", needle, body)
		}
	}
}

func TestAdminBoardsShowsModerationHints(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	authSvc := auth.NewService(userRepo)
	sysop, err := authSvc.Register("sysop", "password123")
	if err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}
	poster, err := authSvc.Register("poster", "password123")
	if err != nil {
		t.Fatalf("register poster: %v", err)
	}
	reporter, err := authSvc.Register("reporter", "password123")
	if err != nil {
		t.Fatalf("register reporter: %v", err)
	}
	board := &domain.Board{Name: "General", Conference: "General", CreatedBy: sysop.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	msg1 := &domain.Message{BoardID: board.ID, AuthorID: poster.ID, Subject: "Duplicate Post", Body: "Same body"}
	msg2 := &domain.Message{BoardID: board.ID, AuthorID: poster.ID, Subject: "Duplicate Post", Body: "Same body"}
	if err := msgRepo.CreateMessage(msg1); err != nil {
		t.Fatalf("create msg1: %v", err)
	}
	if err := msgRepo.CreateMessage(msg2); err != nil {
		t.Fatalf("create msg2: %v", err)
	}
	if err := msgRepo.CreateReport(&domain.MessageReport{MessageID: msg1.ID, ReporterID: reporter.ID, Reason: "spam"}); err != nil {
		t.Fatalf("create report: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/boards?manage_board="+strconv.FormatInt(board.ID, 10), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleAdminBoards(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"Subject + Hints", "possible duplicate x2", "reports 1"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in moderation page: %s", needle, body)
		}
	}
}

func TestBuildModerationHintsIgnoresZeroThreadID(t *testing.T) {
	now := time.Now().UTC()
	msgs := []domain.Message{
		{ID: 1, ThreadID: 0, Subject: "First root", Body: "One", CreatedAt: now.Add(-5 * time.Minute)},
		{ID: 2, ThreadID: 0, Subject: "Second root", Body: "Two", CreatedAt: now.Add(-4 * time.Minute)},
	}
	hints := buildModerationHints(msgs, nil)
	for _, id := range []int64{1, 2} {
		if strings.Contains(hints[id], "thread ") {
			t.Fatalf("expected zero-thread root post %d to avoid false thread hint, got %q", id, hints[id])
		}
	}
}

func TestAdminBulletinsCreateAndPublicBulletinsShowTimedAnnouncement(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	adminSID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("admin session creation failed")
	}
	adminCSRF := app.sessions[adminSID].csrf
	form := url.Values{
		"csrf_token": {adminCSRF},
		"action":     {"create"},
		"title":      {"Tonight's Wire"},
		"body":       {"Tournament starts in the Door Cockpit."},
		"starts_at":  {time.Now().Add(-5 * time.Minute).Format("2006-01-02T15:04")},
		"ends_at":    {time.Now().Add(55 * time.Minute).Format("2006-01-02T15:04")},
		"link":       {"/tournaments"},
		"audience":   {"all callers"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/bulletins", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: adminSID})
	rr := httptest.NewRecorder()
	app.handleAdminBulletins(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d body=%s", rr.Code, rr.Body.String())
	}

	callerSID, ok := app.createSession("caller")
	if !ok {
		t.Fatal("caller session creation failed")
	}
	req = httptest.NewRequest(http.MethodGet, "/bulletins", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleBulletins(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"Timed Announcements", "Tonight's Wire", "Tournament starts in the Door Cockpit.", "/tournaments"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in bulletins page: %s", needle, body)
		}
	}
}

func TestSettingsDigestPreferencesPersistCadence(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	csrf := app.sessions[sid].csrf
	form := url.Values{
		"csrf_token":               {csrf},
		"action":                   {"update_digest"},
		"digest_enabled":           {"1"},
		"digest_max_items":         {"16"},
		"digest_include_events":    {"1"},
		"digest_include_boards":    {"1"},
		"digest_weekly_mail":       {"1"},
		"digest_attention_cadence": {"weekly"},
		"digest_bulletin_cadence":  {"off"},
		"digest_event_cadence":     {"always"},
	}
	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleSettings(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("settings digest redirect status = %d body=%s", rr.Code, rr.Body.String())
	}

	pref := app.loadDigestPreferences("caller")
	if !pref.Enabled || !pref.WeeklyMail || pref.MaxItems != 16 {
		t.Fatalf("unexpected persisted digest prefs: %+v", pref)
	}
	if pref.AttentionCadence != "weekly" || pref.BulletinCadence != "off" || pref.EventCadence != "always" {
		t.Fatalf("unexpected cadence prefs: %+v", pref)
	}
}

func TestBulletinAckAppearsInPublicAndAdminViews(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	app.persistScheduledBulletins([]scheduledBulletin{{
		ID:        "wire-1",
		Title:     "Tonight's Wire",
		Body:      "Tournament starts in the Door Cockpit.",
		StartsAt:  time.Now().UTC().Add(-5 * time.Minute),
		EndsAt:    time.Now().UTC().Add(time.Hour),
		Link:      "/tournaments",
		Audience:  "all callers",
		CreatedBy: "sysop",
		CreatedAt: time.Now().UTC(),
	}})

	callerSID, ok := app.createSession("caller")
	if !ok {
		t.Fatal("caller session creation failed")
	}
	callerCSRF := app.sessions[callerSID].csrf
	form := url.Values{
		"csrf_token": {callerCSRF},
		"action":     {"ack_bulletin"},
		"id":         {"wire-1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/bulletins", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr := httptest.NewRecorder()
	app.handleBulletins(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("bulletin ack redirect status = %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/bulletins", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleBulletins(rr, req)
	if !strings.Contains(rr.Body.String(), "acked") {
		t.Fatalf("expected ack marker in public bulletins page: %s", rr.Body.String())
	}

	adminSID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("admin session creation failed")
	}
	req = httptest.NewRequest(http.MethodGet, "/admin/bulletins", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: adminSID})
	rr = httptest.NewRecorder()
	app.handleAdminBulletins(rr, req)
	if !strings.Contains(rr.Body.String(), "1 /") {
		t.Fatalf("expected ack summary in admin bulletins page: %s", rr.Body.String())
	}
}

func TestBuildBoardPulseSkipsBoardsInQuietHours(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	user, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	board := &domain.Board{Name: "Quiet Board", Conference: "General", CreatedBy: user.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{BoardID: board.ID, AuthorID: user.ID, Subject: "Hello", Body: "world"}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		adminRepo: adminRepo,
	}
	now := time.Now().UTC().In(time.Local)
	app.setBoardQuietHours(user.Handle, board.ID, quietHoursWindow{
		Enabled:   true,
		StartHour: now.Hour(),
		EndHour:   (now.Hour() + 1) % 24,
	})
	storedBoard, _ := boardRepo.Get(board.ID)
	if rows := app.buildBoardPulse(user, []domain.Board{*storedBoard}, 8); len(rows) != 0 {
		t.Fatalf("expected quiet board to be hidden from pulse, got %+v", rows)
	}
	app.setBoardQuietHours(user.Handle, board.ID, quietHoursWindow{})
	if rows := app.buildBoardPulse(user, []domain.Board{*storedBoard}, 8); len(rows) != 1 {
		t.Fatalf("expected board pulse after clearing quiet hours, got %+v", rows)
	}
}

func TestMailUrgencyPersistsAndRenders(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if _, err := authSvc.Register("friend", "password123"); err != nil {
		t.Fatalf("register friend: %v", err)
	}
	app := &webApp{
		authSvc:  authSvc,
		mailRepo: mailRepo,
		sessions: map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	csrf := app.sessions[sid].csrf
	form := url.Values{
		"csrf_token": {csrf},
		"to":         {"friend"},
		"urgency":    {"urgent"},
		"subject":    {"Need reply"},
		"body":       {"Please check this first."},
	}
	req := httptest.NewRequest(http.MethodPost, "/mail", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("mail send redirect status = %d body=%s", rr.Code, rr.Body.String())
	}
	caller, _ := authSvc.GetUser("caller")
	friend, _ := authSvc.GetUser("friend")
	outbox, _ := mailRepo.ListOutbox(caller.ID, 10)
	if len(outbox) != 1 || outbox[0].Subject != "[URGENT] Need reply" || outbox[0].ToUserID != friend.ID {
		t.Fatalf("unexpected outbox rows: %+v", outbox)
	}

	friendSID, ok := app.createSession("friend")
	if !ok {
		t.Fatal("friend session creation failed")
	}
	req = httptest.NewRequest(http.MethodGet, "/mail", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: friendSID})
	rr = httptest.NewRecorder()
	app.handleMail(rr, req)
	if !strings.Contains(rr.Body.String(), "urgent") {
		t.Fatalf("expected urgency badge in inbox: %s", rr.Body.String())
	}
}

func TestAttentionSnoozeMovesItemOutOfUnreadQueue(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	other, err := authSvc.Register("other", "password123")
	if err != nil {
		t.Fatalf("register other: %v", err)
	}
	board := &domain.Board{Name: "General", Conference: "Local", CreatedBy: caller.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   board.ID,
		AuthorID:  other.ID,
		Subject:   "Ping caller",
		Body:      "caller, please review this thread.",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		mailRepo:  mailRepo,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	csrf := app.sessions[sid].csrf
	digest, err := discovery.BuildSinceLastCall(boardRepo, msgRepo, mailRepo, caller, 12)
	if err != nil || len(digest.Items) == 0 {
		t.Fatalf("build digest: %+v err=%v", digest, err)
	}
	itemKey := attentionItemKey(digest.Items[0])
	form := url.Values{
		"csrf_token":   {csrf},
		"action":       {"snooze"},
		"item_key":     {itemKey},
		"snooze_hours": {"4"},
	}
	req := httptest.NewRequest(http.MethodPost, "/attention", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleAttentionCenter(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("attention snooze redirect status = %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/attention", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleAttentionCenter(rr, req)
	body := rr.Body.String()
	if !strings.Contains(body, "Snoozed") || !strings.Contains(body, "Restore") {
		t.Fatalf("expected snoozed section in attention page: %s", body)
	}
}

func TestWeeklyDigestMailSendsOncePerWeek(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	other, err := authSvc.Register("other", "password123")
	if err != nil {
		t.Fatalf("register other: %v", err)
	}
	seedServiceUsers(authSvc)
	board := &domain.Board{Name: "General", Conference: "Local", CreatedBy: caller.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   board.ID,
		AuthorID:  other.ID,
		Subject:   "Weekly check-in",
		Body:      "There is new movement here.",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		mailRepo:  mailRepo,
		adminRepo: adminRepo,
	}
	pref := defaultDigestPreferences()
	pref.WeeklyMail = true
	now := time.Now().UTC()
	if err := app.sendWeeklyDigestMail(caller, pref, now); err != nil {
		t.Fatalf("send weekly digest: %v", err)
	}
	if err := app.sendWeeklyDigestMail(caller, pref, now.Add(time.Hour)); err != nil {
		t.Fatalf("second weekly digest send: %v", err)
	}
	inbox, err := mailRepo.ListInbox(caller.ID, 10)
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if len(inbox) != 1 || inbox[0].Subject != "Weekly Digest" || !strings.Contains(inbox[0].Body, "Weekly WolfBBS digest") {
		t.Fatalf("unexpected inbox rows: %+v", inbox)
	}
}

func TestSettingsApplyAttentionPreset(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	form := url.Values{
		"csrf_token": {app.sessions[sid].csrf},
		"action":     {"apply_attention_preset"},
		"preset":     {"sysop"},
	}
	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleSettings(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("apply attention preset status = %d body=%s", rr.Code, rr.Body.String())
	}
	preset, ok := attentionPresetByName("sysop")
	if !ok {
		t.Fatal("expected sysop preset")
	}
	if got := app.loadDigestPreferences("caller"); got != normalizeDigestPreferences(preset.Pref) {
		t.Fatalf("expected persisted preset %+v, got %+v", normalizeDigestPreferences(preset.Pref), got)
	}

	req = httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleSettings(rr, req)
	body := rr.Body.String()
	for _, needle := range []string{"Attention Rule Presets", "Current rules match the Sysop preset.", "Export notification state JSON", "recommended for your role"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("settings page missing %q: %s", needle, body)
		}
	}
}

func TestSettingsEnableDisableTOTP(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}

	post := func(action string) {
		form := url.Values{
			"csrf_token": {app.sessions[sid].csrf},
			"action":     {action},
		}
		req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
		rr := httptest.NewRecorder()
		app.handleSettings(rr, req)
		if rr.Code != http.StatusFound {
			t.Fatalf("%s status = %d body=%s", action, rr.Code, rr.Body.String())
		}
	}

	post("enable_2fa")
	user, err := authSvc.GetUser("caller")
	if err != nil {
		t.Fatalf("get user after enable: %v", err)
	}
	if strings.TrimSpace(user.TOTPSecret) == "" {
		t.Fatal("expected TOTP secret to be persisted")
	}
	if len(user.RecoveryCodes) != 8 {
		t.Fatalf("expected 8 recovery codes, got %d", len(user.RecoveryCodes))
	}

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleSettings(rr, req)
	body := rr.Body.String()
	for _, needle := range []string{"2FA is enabled.", "Disable TOTP"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("settings page missing %q after enable: %s", needle, body)
		}
	}

	post("disable_2fa")
	user, err = authSvc.GetUser("caller")
	if err != nil {
		t.Fatalf("get user after disable: %v", err)
	}
	if strings.TrimSpace(user.TOTPSecret) != "" {
		t.Fatal("expected TOTP secret to be cleared")
	}
	if len(user.RecoveryCodes) != 0 {
		t.Fatalf("expected recovery codes cleared, got %d", len(user.RecoveryCodes))
	}
}

func TestAttentionExportReturnsCallerNotificationState(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	pref := defaultDigestPreferences()
	pref.Enabled = true
	pref.WeeklyMail = true
	pref.AttentionCadence = "daily"
	app.persistDigestPreferences("caller", pref)
	app.setBoardSubscription("caller", 42, boardSubscriptionDigest)
	app.setBoardQuietHours("caller", 42, quietHoursWindow{Enabled: true, StartHour: 22, EndHour: 8})
	app.markRouteSeen("caller", "/attention")
	app.acknowledgeBulletin("caller", "wire-1")
	app.dismissAttentionItem("caller", "reply|42|10|Needs follow-up")
	app.markAttentionItemRead("caller", "board-pulse|42")
	app.snoozeAttentionItem("caller", "page|friend", time.Now().UTC().Add(3*time.Hour))

	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/attention/export", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleAttentionExport(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("attention export status = %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Result().Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected json content type, got %q", got)
	}
	if got := rr.Result().Header.Get("Content-Disposition"); !strings.Contains(got, "wolfbbs-notifications-caller.json") {
		t.Fatalf("unexpected content disposition: %q", got)
	}
	var payload attentionExportPayload
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode attention export: %v body=%s", err, rr.Body.String())
	}
	if payload.Handle != "caller" || payload.PresetRecommendation != "caller" {
		t.Fatalf("unexpected export identity: %+v", payload)
	}
	if payload.DigestPreferences != normalizeDigestPreferences(pref) {
		t.Fatalf("unexpected digest preferences in export: %+v", payload.DigestPreferences)
	}
	if got := payload.BoardSubscriptions["42"]; got != "digest" {
		t.Fatalf("expected board subscription export, got %q", got)
	}
	if got := payload.BoardQuietHours["42"]; got != "22:00-08:00" {
		t.Fatalf("expected quiet hours export, got %q", got)
	}
	if payload.RouteSeen["/attention"] == "" || payload.BulletinAcknowledgements["wire-1"] == "" {
		t.Fatalf("expected route seen and bulletin ack exports, got %+v", payload)
	}
	if payload.Attention.Dismissed["reply|42|10|Needs follow-up"] == "" || payload.Attention.Read["board-pulse|42"] == "" || payload.Attention.Snoozed["page|friend"] == "" {
		t.Fatalf("expected attention state export, got %+v", payload.Attention)
	}
}

func TestTodayShowsCallerActivityHeatmap(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	friend, err := authSvc.Register("friend", "password123")
	if err != nil {
		t.Fatalf("register friend: %v", err)
	}
	if err := boardRepo.Create(&domain.Board{Name: "General", Conference: "Public", CreatedBy: friend.ID}); err != nil {
		t.Fatalf("create board: %v", err)
	}
	activityAt := time.Now().UTC().Add(-26 * time.Hour)
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   1,
		AuthorID:  caller.ID,
		Subject:   "Heatmap check",
		Body:      "Testing activity rollup.",
		CreatedAt: activityAt,
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	if err := mailRepo.CreateMail(&domain.PrivateMail{
		FromUserID: friend.ID,
		ToUserID:   caller.ID,
		Subject:    "Mail heat",
		Body:       "Private follow-up",
		CreatedAt:  activityAt.Add(10 * time.Minute),
	}); err != nil {
		t.Fatalf("create mail: %v", err)
	}
	if err := adminRepo.AddCallerHistory(&domain.CallerHistory{
		SessionID:       "caller-history",
		NodeID:          1,
		Username:        caller.Handle,
		Area:            "Boards",
		RemoteAddr:      "198.51.100.4:2222",
		LoginAt:         activityAt.Add(15 * time.Minute),
		LogoutAt:        activityAt.Add(45 * time.Minute),
		DurationSeconds: 1800,
	}); err != nil {
		t.Fatalf("add caller history: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		mailRepo:  mailRepo,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/today", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleToday(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("today status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, needle := range []string{"Activity Heatmap", "Posts, private mail, and completed calls over the last 14 days.", "touch(es)", "hot"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("today page missing %q: %s", needle, body)
		}
	}
}

func TestAdminOpsShowsAndResolvesPagesAndEscalations(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if _, err := authSvc.Register("mod", "password123"); err != nil {
		t.Fatalf("register mod: %v", err)
	}
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	if err := authSvc.SetRole("mod", roleModerator); err != nil {
		t.Fatalf("set mod role: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	app.queuePageRequest("caller", "sysop", "Need help with login", "directory / Node 1")
	app.queueStaffEscalation("mod", "caller", "Needs follow-up before tournament night.")
	adminSID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("admin session creation failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/ops", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: adminSID})
	rr := httptest.NewRecorder()
	app.handleAdminOps(rr, req)
	body := rr.Body.String()
	for _, needle := range []string{"Need help with login", "Needs follow-up before tournament night."} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in ops page: %s", needle, body)
		}
	}

	adminCSRF := app.sessions[adminSID].csrf
	pageID := app.loadPageRequests()[0].ID
	escalationID := app.loadStaffEscalations()[0].ID
	for _, form := range []url.Values{
		{"csrf_token": {adminCSRF}, "action": {"resolve_page"}, "id": {pageID}},
		{"csrf_token": {adminCSRF}, "action": {"resolve_escalation"}, "id": {escalationID}},
	} {
		req = httptest.NewRequest(http.MethodPost, "/admin/ops", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: adminSID})
		rr = httptest.NewRecorder()
		app.handleAdminOps(rr, req)
		if rr.Code != http.StatusFound {
			t.Fatalf("ops action redirect status = %d body=%s", rr.Code, rr.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/ops", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: adminSID})
	rr = httptest.NewRecorder()
	app.handleAdminOps(rr, req)
	body = rr.Body.String()
	if !strings.Contains(body, "No unresolved pages.") || !strings.Contains(body, "resolved") {
		t.Fatalf("expected resolved ops state, got: %s", body)
	}
}

func TestDirectoryFavoritesStaffNotesAndCorrespondents(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	friend, err := authSvc.Register("friend", "password123")
	if err != nil {
		t.Fatalf("register friend: %v", err)
	}
	if _, err := authSvc.Register("mod", "password123"); err != nil {
		t.Fatalf("register mod: %v", err)
	}
	if err := authSvc.SetRole("mod", roleModerator); err != nil {
		t.Fatalf("set moderator role: %v", err)
	}
	if err := mailRepo.CreateMail(&domain.PrivateMail{FromUserID: caller.ID, ToUserID: friend.ID, Subject: "Hi", Body: "First"}); err != nil {
		t.Fatalf("create mail 1: %v", err)
	}
	if err := mailRepo.CreateMail(&domain.PrivateMail{FromUserID: friend.ID, ToUserID: caller.ID, Subject: "Re: Hi", Body: "Second"}); err != nil {
		t.Fatalf("create mail 2: %v", err)
	}
	now := time.Now().UTC()
	if err := adminRepo.UpsertNodeSession(&domain.NodeSession{
		SessionID:    "friend-online",
		NodeID:       4,
		Username:     friend.Handle,
		Area:         "chat",
		RemoteAddr:   "203.0.113.4:2323",
		LoginAt:      now.Add(-20 * time.Minute),
		LastActivity: now.Add(-3 * time.Minute),
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("upsert node session: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		mailRepo:  mailRepo,
		sessions:  map[string]sessionState{},
	}
	callerSID, ok := app.createSession("caller")
	if !ok {
		t.Fatal("caller session creation failed")
	}
	callerCSRF := app.sessions[callerSID].csrf
	form := url.Values{
		"csrf_token": {callerCSRF},
		"action":     {"toggle_favorite"},
		"target":     {"friend"},
		"return_to":  {"/directory?handle=friend"},
	}
	req := httptest.NewRequest(http.MethodPost, "/directory", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr := httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("favorite redirect status = %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/directory?favorites=1&handle=friend", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("favorites view status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"Favorite Callers", "friend", "Recurring Correspondents", "2 exchanges", "Send Page"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in directory page: %s", needle, body)
		}
	}

	modSID, ok := app.createSession("mod")
	if !ok {
		t.Fatal("moderator session creation failed")
	}
	modCSRF := app.sessions[modSID].csrf
	form = url.Values{
		"csrf_token": {modCSRF},
		"action":     {"save_staff_note"},
		"target":     {"friend"},
		"staff_note": {"Prefers evening pages; helpful in tournament support."},
		"return_to":  {"/directory?handle=friend"},
	}
	req = httptest.NewRequest(http.MethodPost, "/directory", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: modSID})
	rr = httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("staff note redirect status = %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/directory?handle=friend", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: modSID})
	rr = httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if !strings.Contains(rr.Body.String(), "Prefers evening pages; helpful in tournament support.") {
		t.Fatalf("expected staff note in moderator directory view: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/directory?handle=friend", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if strings.Contains(rr.Body.String(), "Prefers evening pages; helpful in tournament support.") {
		t.Fatalf("expected staff note to stay hidden from caller: %s", rr.Body.String())
	}
}

func TestDirectoryPagingFeedsAttentionCenter(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if _, err := authSvc.Register("friend", "password123"); err != nil {
		t.Fatalf("register friend: %v", err)
	}
	now := time.Now().UTC()
	if err := adminRepo.UpsertNodeSession(&domain.NodeSession{
		SessionID:    "friend-live",
		NodeID:       2,
		Username:     "friend",
		Area:         "bbs",
		RemoteAddr:   "198.51.100.20:2323",
		LoginAt:      now.Add(-30 * time.Minute),
		LastActivity: now.Add(-1 * time.Minute),
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("upsert node session: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		mailRepo:  mailRepo,
		sessions:  map[string]sessionState{},
	}
	callerSID, ok := app.createSession("caller")
	if !ok {
		t.Fatal("caller session creation failed")
	}
	form := url.Values{
		"csrf_token": {app.sessions[callerSID].csrf},
		"action":     {"send_page"},
		"target":     {"friend"},
		"message":    {"Check the tournament thread."},
		"source":     {"directory"},
		"return_to":  {"/directory?handle=friend"},
	}
	req := httptest.NewRequest(http.MethodPost, "/directory", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr := httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("send page redirect status = %d body=%s", rr.Code, rr.Body.String())
	}

	friendSID, ok := app.createSession("friend")
	if !ok {
		t.Fatal("friend session creation failed")
	}
	req = httptest.NewRequest(http.MethodGet, "/attention", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: friendSID})
	rr = httptest.NewRecorder()
	app.handleAttentionCenter(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("attention status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"Live Pages", "Page from caller", "Check the tournament thread.", "/mail?to=caller", "Reply by mail"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in attention center: %s", needle, body)
		}
	}
}

func TestMailShowsPresenceAwareHandoffForOnlineRecipient(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if _, err := authSvc.Register("friend", "password123"); err != nil {
		t.Fatalf("register friend: %v", err)
	}
	now := time.Now().UTC()
	if err := adminRepo.UpsertNodeSession(&domain.NodeSession{
		SessionID:    "friend-online",
		NodeID:       9,
		Username:     "friend",
		Area:         "chat",
		RemoteAddr:   "198.51.100.99:23",
		LoginAt:      now.Add(-10 * time.Minute),
		LastActivity: now.Add(-90 * time.Second),
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("upsert node session: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		mailRepo:  mailRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/mail?to=friend", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleMail(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("mail status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"friend is online now", "Node 9", "send a page"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in mail page: %s", needle, body)
		}
	}
}

func TestTournamentsShowsStandingsAndBracket(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	repo := repository.NewInMemoryDoorRepository()
	reg := doors.NewRegistry()
	reg.SetRepository(repo)
	customDoors := []doors.Door{
		{ID: "alpha", Hotkey: "A", Name: "Alpha Arena", Command: "/bin/true"},
		{ID: "beta", Hotkey: "B", Name: "Beta Battle", Command: "/bin/true"},
		{ID: "gamma", Hotkey: "G", Name: "Gamma Gauntlet", Command: "/bin/true"},
		{ID: "delta", Hotkey: "D", Name: "Delta Duel", Command: "/bin/true"},
	}
	for _, door := range customDoors {
		reg.Register(door)
	}
	names := []string{"caller", "rival1", "rival2", "rival3", "rival4"}
	ids := make(map[string]int64, len(names))
	for _, name := range names {
		user, err := authSvc.Register(name, "password123")
		if err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
		ids[name] = user.ID
	}
	scores := []struct {
		doorID string
		handle string
		value  int64
	}{
		{doorID: "alpha", handle: "caller", value: 900},
		{doorID: "beta", handle: "rival1", value: 840},
		{doorID: "gamma", handle: "rival2", value: 780},
		{doorID: "delta", handle: "rival3", value: 720},
	}
	for _, row := range scores {
		if err := repo.SubmitScore(&domain.DoorScore{DoorID: row.doorID, UserID: ids[row.handle], ScoreType: "points", Value: row.value, CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("submit score: %v", err)
		}
	}
	app := &webApp{
		authSvc:      authSvc,
		doorRegistry: reg,
		sessions:     map[string]sessionState{},
	}
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session creation failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/tournaments", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleTournaments(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"Standings Table", "Bracket Preview", "Alpha Arena", "Semifinal 1"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in tournament page: %s", needle, body)
		}
	}
}

func TestSeedDefaultBoardsAddsMissingDefaultsOnly(t *testing.T) {
	repo := repository.NewInMemoryBoardRepository()
	if err := repo.Create(&domain.Board{Name: "General", Description: "already there", CreatedBy: 99}); err != nil {
		t.Fatalf("create pre-existing board: %v", err)
	}

	created, err := seedDefaultBoards(repo)
	if err != nil {
		t.Fatalf("seed default boards: %v", err)
	}
	if created != 2 {
		t.Fatalf("expected 2 boards created when General exists, got %d", created)
	}

	boards, err := repo.List()
	if err != nil {
		t.Fatalf("list boards: %v", err)
	}
	if len(boards) != 3 {
		t.Fatalf("expected 3 total boards after first seed, got %d", len(boards))
	}

	created, err = seedDefaultBoards(repo)
	if err != nil {
		t.Fatalf("seed default boards rerun: %v", err)
	}
	if created != 0 {
		t.Fatalf("expected idempotent rerun to create 0 boards, got %d", created)
	}

	boards, err = repo.List()
	if err != nil {
		t.Fatalf("list boards after rerun: %v", err)
	}
	if len(boards) != 3 {
		t.Fatalf("expected 3 total boards after rerun, got %d", len(boards))
	}
}

func TestSeedDefaultBoardsFailsWithNilRepo(t *testing.T) {
	created, err := seedDefaultBoards(nil)
	if err == nil {
		t.Fatal("expected nil repository to return an error")
	}
	if created != 0 {
		t.Fatalf("expected 0 created boards on nil repository, got %d", created)
	}
}

func TestActivityPubDisabledReturnsNotFound(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		boardRepo: repository.NewInMemoryBoardRepository(),
		msgRepo:   repository.NewInMemoryMessageRepository(),
		apEnabled: false,
	}

	req := httptest.NewRequest(http.MethodGet, "/ap/users/alice", nil)
	rr := httptest.NewRecorder()
	app.handleActivityPubUsers(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when ap disabled, got %d", rr.Code)
	}
}

func TestPasswordResetRecipient(t *testing.T) {
	if got := passwordResetRecipient("reader@example.com"); got != "reader@example.com" {
		t.Fatalf("expected valid address, got %q", got)
	}
	if got := passwordResetRecipient("not-an-email"); got != "" {
		t.Fatalf("expected invalid address to be rejected, got %q", got)
	}
}

func TestWebQuickJumpPath(t *testing.T) {
	if got := webQuickJumpPath("mail"); got != "/mail" {
		t.Fatalf("expected /mail, got %q", got)
	}
	if got := webQuickJumpPath("showcase"); got != "/showcase" {
		t.Fatalf("expected /showcase, got %q", got)
	}
	if got := webQuickJumpPath("status"); got != "/status" {
		t.Fatalf("expected /status, got %q", got)
	}
	if got := webQuickJumpPath("doors"); got != "/doors" {
		t.Fatalf("expected /doors, got %q", got)
	}
	if got := webQuickJumpPath("bulletins"); got != "/bulletins" {
		t.Fatalf("expected /bulletins, got %q", got)
	}
	if got := webQuickJumpPath("directory"); got != "/directory" {
		t.Fatalf("expected /directory, got %q", got)
	}
	if got := webQuickJumpPath("finder"); got != "/finder" {
		t.Fatalf("expected /finder, got %q", got)
	}
	if got := webQuickJumpPath("newfiles"); got != "/newfiles" {
		t.Fatalf("expected /newfiles, got %q", got)
	}
	if got := webQuickJumpPath("feedback"); got != "/feedback" {
		t.Fatalf("expected /feedback, got %q", got)
	}
	if got := webQuickJumpPath("radar"); got != "/radar" {
		t.Fatalf("expected /radar, got %q", got)
	}
	if got := webQuickJumpPath("clubhouse"); got != "/clubhouse" {
		t.Fatalf("expected /clubhouse, got %q", got)
	}
	if got := webQuickJumpPath("streaks"); got != "/streaks" {
		t.Fatalf("expected /streaks, got %q", got)
	}
	if got := webQuickJumpPath("next"); got != "/next" {
		t.Fatalf("expected /next, got %q", got)
	}
	if got := webQuickJumpPath("spotlights"); got != "/spotlights" {
		t.Fatalf("expected /spotlights, got %q", got)
	}
	if got := webQuickJumpPath("challenges"); got != "/challenges" {
		t.Fatalf("expected /challenges, got %q", got)
	}
	if got := webQuickJumpPath("missions"); got != "/missions" {
		t.Fatalf("expected /missions, got %q", got)
	}
	if got := webQuickJumpPath("resume"); got != "/resume" {
		t.Fatalf("expected /resume, got %q", got)
	}
	if got := webQuickJumpPath("comeback"); got != "/doors/comeback" {
		t.Fatalf("expected /doors/comeback, got %q", got)
	}
	if got := webQuickJumpPath("mentorship"); got != "/mentorship" {
		t.Fatalf("expected /mentorship, got %q", got)
	}
	if got := webQuickJumpPath("milestones"); got != "/milestones" {
		t.Fatalf("expected /milestones, got %q", got)
	}
	if got := webQuickJumpPath("time-lane"); got != "/time-lane" {
		t.Fatalf("expected /time-lane, got %q", got)
	}
	if got := webQuickJumpPath("digest-prefs"); got != "/digest/preferences" {
		t.Fatalf("expected /digest/preferences, got %q", got)
	}
	if got := webQuickJumpPath("admin-missions"); got != "/admin/missions" {
		t.Fatalf("expected /admin/missions, got %q", got)
	}
	if got := webQuickJumpPath("admin-mentorship"); got != "/admin/mentorship" {
		t.Fatalf("expected /admin/mentorship, got %q", got)
	}
	if got := webQuickJumpPath("mod-center"); got != "/admin/mod-center" {
		t.Fatalf("expected /admin/mod-center, got %q", got)
	}
	if got := webQuickJumpPath("plugins"); got != "/admin/plugins" {
		t.Fatalf("expected /admin/plugins, got %q", got)
	}
	if got := webQuickJumpPath("themes"); got != "/admin/themes" {
		t.Fatalf("expected /admin/themes, got %q", got)
	}
	if got := webQuickJumpPath("webhooks"); got != "/admin/webhooks" {
		t.Fatalf("expected /admin/webhooks, got %q", got)
	}
	if got := webQuickJumpPath("analytics"); got != "/admin/analytics" {
		t.Fatalf("expected /admin/analytics, got %q", got)
	}
	if got := webQuickJumpPath("upgrade-safety"); got != "/admin/upgrade-safety" {
		t.Fatalf("expected /admin/upgrade-safety, got %q", got)
	}
	if got := webQuickJumpPath("backups"); got != "/admin/backups" {
		t.Fatalf("expected /admin/backups, got %q", got)
	}
	if got := webQuickJumpPath("release"); got != "/admin/release" {
		t.Fatalf("expected /admin/release, got %q", got)
	}
	if got := webQuickJumpPath("unknown"); got != "" {
		t.Fatalf("expected empty path for unknown target, got %q", got)
	}
}

func TestActivityPubWebFingerActorOutbox(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	authSvc := auth.NewService(userRepo)
	user, err := authSvc.Register("alice", "password123")
	if err != nil {
		t.Fatalf("register alice: %v", err)
	}
	board := &domain.Board{Name: "General", CreatedBy: user.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   board.ID,
		AuthorID:  user.ID,
		Subject:   "Hello AP",
		Body:      "ActivityPub body",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		apEnabled: true,
		apBaseURL: "https://bbs.example",
	}

	req := httptest.NewRequest(http.MethodGet, "/.well-known/webfinger?resource=acct:alice@bbs.example", nil)
	rr := httptest.NewRecorder()
	app.handleActivityPubWebFinger(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("webfinger status = %d", rr.Code)
	}
	var webfinger map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &webfinger); err != nil {
		t.Fatalf("decode webfinger: %v", err)
	}
	if webfinger["subject"] != "acct:alice@bbs.example" {
		t.Fatalf("unexpected webfinger subject: %#v", webfinger["subject"])
	}

	req = httptest.NewRequest(http.MethodGet, "/ap/users/alice", nil)
	rr = httptest.NewRecorder()
	app.handleActivityPubUsers(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("actor status = %d", rr.Code)
	}
	var actor map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &actor); err != nil {
		t.Fatalf("decode actor: %v", err)
	}
	if actor["type"] != "Person" {
		t.Fatalf("expected Person actor, got %#v", actor["type"])
	}

	req = httptest.NewRequest(http.MethodGet, "/ap/users/alice/outbox", nil)
	rr = httptest.NewRecorder()
	app.handleActivityPubUsers(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("outbox status = %d", rr.Code)
	}
	var outbox map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &outbox); err != nil {
		t.Fatalf("decode outbox: %v", err)
	}
	if outbox["type"] != "OrderedCollection" {
		t.Fatalf("expected OrderedCollection outbox, got %#v", outbox["type"])
	}
	items, ok := outbox["orderedItems"].([]interface{})
	if !ok || len(items) == 0 {
		t.Fatalf("expected outbox items, got %#v", outbox["orderedItems"])
	}
}

func TestActivityPubInboxAcceptsFollowAndAudits(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	user, err := authSvc.Register("alice", "password123")
	if err != nil {
		t.Fatalf("register alice: %v", err)
	}
	adminRepo := repository.NewInMemoryAdminRepository()
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		adminRepo: adminRepo,
		apEnabled: true,
		apBaseURL: "https://bbs.example",
	}

	body := strings.NewReader(`{
		"id":"https://remote.example/activities/follow-1",
		"type":"Follow",
		"actor":"https://remote.example/users/bob",
		"object":"https://bbs.example/ap/users/alice"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/ap/users/alice/inbox", body)
	req.Header.Set("Content-Type", "application/activity+json")
	rr := httptest.NewRecorder()

	app.handleActivityPubUsers(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("inbox status = %d body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode inbox response: %v", err)
	}
	if resp["status"] != "accepted" || resp["type"] != "Follow" || resp["recipient"] != user.Handle {
		t.Fatalf("unexpected inbox response: %#v", resp)
	}

	audit, err := adminRepo.ListAudit(10)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(audit))
	}
	if audit[0].Action != "activitypub_inbox_follow" {
		t.Fatalf("unexpected audit action: %#v", audit[0].Action)
	}
	if !strings.Contains(audit[0].Details, "actor=https://remote.example/users/bob") {
		t.Fatalf("unexpected audit details: %#v", audit[0].Details)
	}
}

func TestActivityPubInboxRejectsUnsupportedActivity(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("alice", "password123"); err != nil {
		t.Fatalf("register alice: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		apEnabled: true,
		apBaseURL: "https://bbs.example",
	}

	body := strings.NewReader(`{"type":"Block","actor":"https://remote.example/users/bob"}`)
	req := httptest.NewRequest(http.MethodPost, "/ap/users/alice/inbox", body)
	req.Header.Set("Content-Type", "application/activity+json")
	rr := httptest.NewRecorder()

	app.handleActivityPubUsers(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported activity, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "unsupported activity type") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestFrozenPollThreadBlocksVotes(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	board := &domain.Board{Name: "General", Conference: "Public", CreatedBy: caller.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	root := &domain.Message{BoardID: board.ID, AuthorID: caller.ID, Subject: "Poll", Body: "poll"}
	if err := msgRepo.CreateMessage(root); err != nil {
		t.Fatalf("create root: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		boardRepo: boardRepo,
		msgRepo:   msgRepo,
		adminRepo: repository.NewInMemoryAdminRepository(),
		sessions:  map[string]sessionState{},
	}
	app.setThreadPoll(root.ThreadID, threadPoll{
		Question: "Ship it?",
		Options:  []string{"yes", "no"},
	})
	app.setThreadLifecycleState(root.ThreadID, threadLifecycleFrozen)
	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("create session failed")
	}
	form := url.Values{
		"csrf_token":   {app.sessions[sid].csrf},
		"action":       {"vote_poll"},
		"board_id":     {strconv.FormatInt(board.ID, 10)},
		"thread_id":    {strconv.FormatInt(root.ThreadID, 10)},
		"option_index": {"0"},
	}
	req := httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleBoards(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Location"); !strings.Contains(got, "Poll+voting+is+disabled") {
		t.Fatalf("expected frozen poll error redirect, got %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/boards?board="+strconv.FormatInt(board.ID, 10)+"&id="+strconv.FormatInt(root.ID, 10), nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleBoards(rr, req)
	if strings.Contains(rr.Body.String(), "Save Vote") {
		t.Fatalf("expected vote form hidden for frozen poll: %s", rr.Body.String())
	}
}

func TestDirectoryProfileCirclesAliasesAndIncidents(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	friend, err := authSvc.Register("friend", "password123")
	if err != nil {
		t.Fatalf("register friend: %v", err)
	}
	if _, err := authSvc.Register("mod", "password123"); err != nil {
		t.Fatalf("register mod: %v", err)
	}
	if err := authSvc.SetRole("mod", roleModerator); err != nil {
		t.Fatalf("set mod role: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		mailRepo:  mailRepo,
		sessions:  map[string]sessionState{},
	}
	app.persistPublicProfileSettings("friend", publicProfileSettings{
		StatusLine:     "door runner after midnight",
		Bio:            "Likes tournaments and ANSI packs.",
		ContactPrefs:   []string{"mail", "chat"},
		ShowStatusLine: true,
		ShowBio:        true,
		ShowContact:    true,
	})
	if err := mailRepo.CreateMail(&domain.PrivateMail{FromUserID: caller.ID, ToUserID: friend.ID, Subject: "hello", Body: "one"}); err != nil {
		t.Fatalf("create outbound mail: %v", err)
	}
	if err := mailRepo.CreateMail(&domain.PrivateMail{FromUserID: friend.ID, ToUserID: caller.ID, Subject: "reply", Body: "two"}); err != nil {
		t.Fatalf("create inbound mail: %v", err)
	}
	app.queueStaffEscalation("mod", "friend", "Needs host follow-up.")

	callerSID, ok := app.createSession("caller")
	if !ok {
		t.Fatal("caller session failed")
	}
	form := url.Values{
		"csrf_token": {app.sessions[callerSID].csrf},
		"action":     {"save_alias"},
		"target":     {"friend"},
		"alias":      {"Wingman"},
		"return_to":  {"/directory?handle=friend"},
	}
	req := httptest.NewRequest(http.MethodPost, "/directory", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr := httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save alias status = %d body=%s", rr.Code, rr.Body.String())
	}

	form = url.Values{
		"csrf_token": {app.sessions[callerSID].csrf},
		"action":     {"save_circle"},
		"name":       {"Door Crew"},
		"members":    {"friend"},
		"note":       {"night tournament regulars"},
	}
	req = httptest.NewRequest(http.MethodPost, "/circles", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleCircles(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save circle status = %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/directory?handle=friend", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleDirectory(rr, req)
	body := rr.Body.String()
	for _, needle := range []string{"door runner after midnight", "Likes tournaments and ANSI packs.", "Prefers chat, mail", "Your alias:", "Wingman", "Door Crew", "Relationship Timeline", "mail to friend", "mail from friend"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in directory profile: %s", needle, body)
		}
	}

	modSID, ok := app.createSession("mod")
	if !ok {
		t.Fatal("mod session failed")
	}
	req = httptest.NewRequest(http.MethodGet, "/directory?handle=friend", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: modSID})
	rr = httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if !strings.Contains(rr.Body.String(), "Incident Timeline") || !strings.Contains(rr.Body.String(), "Needs host follow-up.") {
		t.Fatalf("expected incident timeline in moderator view: %s", rr.Body.String())
	}
}

func TestProfileExportIncludesPrivacyAliasesAndCircles(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	app.persistPublicProfileSettings("caller", publicProfileSettings{
		StatusLine:     "status line",
		Bio:            "bio text",
		ContactPrefs:   []string{"mail"},
		ShowStatusLine: true,
		ShowBio:        false,
		ShowContact:    true,
	})
	app.persistContactAliases("caller", map[string]string{"friend": "Wingman"})
	app.persistCallerCircles("caller", []callerCircle{{ID: "c1", Name: "Door Crew", Members: []string{"friend"}}})

	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("session failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/profile/export", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleProfileExport(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("profile export status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode profile export: %v body=%s", err, rr.Body.String())
	}
	if payload["handle"] != "caller" {
		t.Fatalf("unexpected export handle: %+v", payload)
	}
	card := payload["profile_card"].(map[string]interface{})
	if card["status_line"] != "status line" {
		t.Fatalf("unexpected profile card export: %+v", card)
	}
	aliases := payload["contact_aliases"].(map[string]interface{})
	if aliases["friend"] != "Wingman" {
		t.Fatalf("unexpected alias export: %+v", aliases)
	}
	circles := payload["caller_circles"].([]interface{})
	if len(circles) != 1 {
		t.Fatalf("expected one circle export, got %+v", circles)
	}
}

func TestAdminEventsShowsCampaignHealth(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set role: %v", err)
	}
	now := time.Now().UTC()
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}
	app.persistCommunityEvents([]communityEvent{
		{
			ID:          "evt-series-1",
			SeriesID:    "evt-series",
			Title:       "Tournament Night",
			Category:    "tournament",
			StartsAt:    now.Add(2 * time.Hour),
			Recurrence:  "weekly",
			Location:    "Door Cockpit",
			Host:        "sysop",
			Audience:    "all callers",
			Description: "Bracket ladder night",
			CreatedAt:   now.Add(-time.Hour),
		},
		{
			ID:          "evt-missed-recap",
			SeriesID:    "evt-missed-recap",
			Title:       "Retro Net",
			Category:    "social",
			StartsAt:    now.Add(-3 * time.Hour),
			EndsAt:      now.Add(-2 * time.Hour),
			Location:    "#lobby",
			Host:        "sysop",
			Audience:    "all callers",
			Description: "Missed recap test",
			CreatedAt:   now.Add(-6 * time.Hour),
		},
	})
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/events", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminEvents)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin events status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"Campaign Health", "Campaign Recommendations", "missing join paths", "missing recaps"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("admin events missing %q: %s", needle, body)
		}
	}
}

func TestEventsRSVPAndDirectoryInviteFlow(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	friend, err := authSvc.Register("friend", "password123")
	if err != nil {
		t.Fatalf("register friend: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		mailRepo:  mailRepo,
		sessions:  map[string]sessionState{},
	}
	app.persistCommunityEvents([]communityEvent{{
		ID:        "evt-1",
		SeriesID:  "evt-1",
		Title:     "Tournament Night",
		Category:  "tournament",
		StartsAt:  time.Now().UTC().Add(2 * time.Hour),
		Location:  "Door Cockpit",
		Host:      "sysop",
		Audience:  "all callers",
		Link:      "/tournaments",
		CreatedAt: time.Now().UTC(),
	}})
	now := time.Now().UTC()
	if err := adminRepo.UpsertNodeSession(&domain.NodeSession{
		SessionID:    "friend-live",
		NodeID:       3,
		Username:     "friend",
		Area:         "chat",
		RemoteAddr:   "198.51.100.20:2323",
		LoginAt:      now.Add(-10 * time.Minute),
		LastActivity: now.Add(-1 * time.Minute),
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	occurrence := app.upcomingCommunityEvents(5, time.Now().UTC())[0]

	sid, ok := app.createSession("caller")
	if !ok {
		t.Fatal("create session failed")
	}
	form := url.Values{
		"csrf_token": {app.sessions[sid].csrf},
		"action":     {"rsvp"},
		"event_id":   {occurrence.ID},
		"status":     {"going"},
	}
	req := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleEventsCalendar(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("events rsvp status = %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/events", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleEventsCalendar(rr, req)
	if !strings.Contains(rr.Body.String(), "RSVPs: going 1") {
		t.Fatalf("expected rsvp summary in events page: %s", rr.Body.String())
	}

	form = url.Values{
		"csrf_token":  {app.sessions[sid].csrf},
		"action":      {"send_event_invite"},
		"target":      {"friend"},
		"event_id":    {occurrence.ID},
		"notify_live": {"1"},
		"return_to":   {"/directory?handle=friend"},
	}
	req = httptest.NewRequest(http.MethodPost, "/directory", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleDirectory(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("directory invite status = %d body=%s", rr.Code, rr.Body.String())
	}

	inbox, err := mailRepo.ListInbox(friend.ID, 10)
	if err != nil || len(inbox) != 1 {
		t.Fatalf("expected invite mail, got inbox=%+v err=%v", inbox, err)
	}
	if !strings.Contains(inbox[0].Subject, "Invitation: Tournament Night") {
		t.Fatalf("unexpected invite subject: %+v", inbox[0])
	}
	if len(app.loadPageRequests()) != 1 {
		t.Fatalf("expected live page notification after invite")
	}
	if got := app.loadEventRSVPs("friend")[occurrence.ID].Status; got != "maybe" {
		t.Fatalf("expected invite to seed maybe RSVP, got %q", got)
	}
	_ = caller
}

func TestAdminMailSharedInboxAndMerge(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	sysop, err := authSvc.Register("sysop", "password123")
	if err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	mod, err := authSvc.Register("mod", "password123")
	if err != nil {
		t.Fatalf("register mod: %v", err)
	}
	caller1, err := authSvc.Register("caller1", "password123")
	if err != nil {
		t.Fatalf("register caller1: %v", err)
	}
	caller2, err := authSvc.Register("caller2", "password123")
	if err != nil {
		t.Fatalf("register caller2: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	if err := authSvc.SetRole("mod", roleModerator); err != nil {
		t.Fatalf("set mod role: %v", err)
	}
	if err := authSvc.SetVerified("caller1", true); err != nil {
		t.Fatalf("verify caller1: %v", err)
	}
	if err := mailRepo.CreateMail(&domain.PrivateMail{
		FromUserID: caller1.ID,
		ToUserID:   mod.ID,
		Subject:    "[feedback/bug] Missing screen",
		Body:       "Need help",
	}); err != nil {
		t.Fatalf("create shared mail: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		adminRepo: adminRepo,
		mailRepo:  mailRepo,
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("create sysop session failed")
	}
	form := url.Values{
		"csrf_token": {app.sessions[sid].csrf},
		"action":     {"assign_shared"},
		"mail_id":    {"1"},
		"assignee":   {"mod"},
		"status":     {"assigned"},
		"note":       {"triage tonight"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/mail", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleAdminMail(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("assign shared mail status = %d body=%s", rr.Code, rr.Body.String())
	}
	if got := app.loadModeratorInboxAssignments()[1].Assignee; got != "mod" {
		t.Fatalf("expected assignment persisted, got %+v", app.loadModeratorInboxAssignments())
	}

	form = url.Values{
		"csrf_token":    {app.sessions[sid].csrf},
		"action":        {"merge_send"},
		"role":          {"user"},
		"verified_only": {"1"},
		"subject":       {"Launch note"},
		"body":          {"Hello {{handle}}, welcome back."},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/mail", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleAdminMail(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("merge send status = %d body=%s", rr.Code, rr.Body.String())
	}
	inbox1, _ := mailRepo.ListInbox(caller1.ID, 20)
	inbox2, _ := mailRepo.ListInbox(caller2.ID, 20)
	if len(inbox1) != 1 || !strings.Contains(inbox1[0].Body, "Hello caller1") {
		t.Fatalf("expected merge mail for caller1, got %+v", inbox1)
	}
	if len(inbox2) != 0 {
		t.Fatalf("expected caller2 excluded by verified filter, got %+v", inbox2)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/mail", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleAdminMail(rr, req)
	body := rr.Body.String()
	for _, needle := range []string{"Shared Moderator Inbox", "triage tonight", "Mail Merge", "Missing screen"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in admin mail page: %s", needle, body)
		}
	}
	_ = sysop
}

func TestAdminReleaseDashboardRendersArtifacts(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		adminRepo: repository.NewInMemoryAdminRepository(),
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("create sysop session failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/release", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleAdminReleaseDashboard(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("release dashboard status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, needle := range []string{
		"Release Dashboard",
		"Roadmap Tranche 146-150",
		"docs/manual-acceptance-latest.md",
		"docs/function-registry.json",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in release dashboard: %s", needle, body)
		}
	}

	form := url.Values{
		"csrf_token": {app.sessions[sid].csrf},
		"action":     {"toggle_check"},
		"key":        {"go_test_all"},
		"done":       {"1"},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/release", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleAdminReleaseDashboard(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect on release checklist update, got %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/release", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleAdminReleaseDashboard(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("release dashboard status after update = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "go test ./...") {
		t.Fatalf("expected release checklist row after update: %s", rr.Body.String())
	}
}

func TestEventAttendanceAndRecapPublishFlow(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	adminRepo := repository.NewInMemoryAdminRepository()

	if _, err := authSvc.Register("caller", "password123"); err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}

	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		adminRepo: adminRepo,
		sessions:  map[string]sessionState{},
	}

	now := time.Now().UTC()
	event := communityEvent{
		ID:          "evt-recap-1",
		SeriesID:    "evt-recap-1",
		Title:       "Retro Net Night",
		Category:    "social",
		StartsAt:    now.Add(-90 * time.Minute),
		EndsAt:      now.Add(-30 * time.Minute),
		Location:    "#lobby",
		Host:        "sysop",
		Audience:    "all callers",
		Description: "Callers sync up after door runs.",
		Link:        "/chat",
		CreatedAt:   now.Add(-3 * time.Hour),
	}
	app.persistCommunityEvents([]communityEvent{event})
	occurrences := app.recentCommunityEvents(5, now)
	if len(occurrences) == 0 {
		t.Fatal("expected at least one recent event occurrence")
	}
	eventID := occurrences[0].ID

	callerSID, ok := app.createSession("caller")
	if !ok {
		t.Fatal("caller session creation failed")
	}
	sysopSID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("sysop session creation failed")
	}

	form := url.Values{
		"csrf_token": {app.sessions[callerSID].csrf},
		"action":     {"check_in"},
		"event_id":   {eventID},
	}
	req := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr := httptest.NewRecorder()
	app.handleEventsCalendar(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("check-in redirect status = %d body=%s", rr.Code, rr.Body.String())
	}
	if got := app.attendanceCounts()[eventID]; got != 1 {
		t.Fatalf("expected attendance count 1, got %d", got)
	}

	form = url.Values{
		"csrf_token":       {app.sessions[sysopSID].csrf},
		"action":           {"save_recap"},
		"event_id":         {eventID},
		"attendance_count": {"0"},
		"summary":          {"Strong turnout and great post-event follow-up."},
		"highlights":       {"Door champion rematch\nNext week schedule locked"},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/events", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminEvents)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save recap redirect status = %d body=%s", rr.Code, rr.Body.String())
	}

	recaps := app.loadEventRecaps()
	recap, ok := recaps[eventID]
	if !ok {
		t.Fatalf("expected recap for event %s, got %+v", eventID, recaps)
	}
	if recap.AttendanceCount != 1 {
		t.Fatalf("expected recap attendance fallback from check-ins, got %d", recap.AttendanceCount)
	}

	req = httptest.NewRequest(http.MethodGet, "/events", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleEventsCalendar(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("events status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{
		"Checked in: 1 caller(s)",
		"Recap:",
		"Strong turnout and great post-event follow-up.",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q on events page: %s", needle, body)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/events/recaps", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleEventRecaps(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("event recaps status = %d", rr.Code)
	}
	recapBody := rr.Body.String()
	for _, needle := range []string{
		"Event Recaps",
		"Strong turnout and great post-event follow-up.",
		"Door champion rematch",
	} {
		if !strings.Contains(recapBody, needle) {
			t.Fatalf("expected %q on event recaps page: %s", needle, recapBody)
		}
	}
}

func TestSeasonalChallengesAndClubhouseGoalsFlow(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	doorRepo := repository.NewInMemoryDoorRepository()
	authSvc := auth.NewService(userRepo)
	chatSvc := chat.NewServiceForTest()

	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}

	board := &domain.Board{Name: "General", Description: "Main board", Conference: "Public", CreatedBy: caller.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:   board.ID,
		AuthorID:  caller.ID,
		Subject:   "Challenge run log",
		Body:      "Keeping the season active.",
		CreatedAt: time.Now().UTC().Add(-5 * time.Minute),
	}); err != nil {
		t.Fatalf("create challenge message: %v", err)
	}

	chatSvc.JoinChannel("caller", "#lobby")
	if _, err := chatSvc.Post("caller", "#lobby", "Challenge lobby check-in"); err != nil {
		t.Fatalf("chat post: %v", err)
	}

	doorRegistry := doors.NewRegistry()
	doorRegistry.SetRepository(doorRepo)
	doorRegistry.Register(doors.Door{ID: "retro-door", Hotkey: "R", Name: "Retro Door", Command: "/bin/true"})
	if err := doorRepo.AddEvent(&domain.DoorEvent{
		DoorID:    "retro-door",
		UserID:    caller.ID,
		EventType: "start",
		CreatedAt: time.Now().UTC().Add(-5 * time.Minute),
	}); err != nil {
		t.Fatalf("seed door event: %v", err)
	}

	app := &webApp{
		authSvc:      authSvc,
		userRepo:     userRepo,
		boardRepo:    boardRepo,
		msgRepo:      msgRepo,
		adminRepo:    adminRepo,
		doorRepo:     doorRepo,
		doorRegistry: doorRegistry,
		chatSvc:      chatSvc,
		sessions:     map[string]sessionState{},
	}

	callerSID, ok := app.createSession("caller")
	if !ok {
		t.Fatal("caller session creation failed")
	}
	sysopSID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("sysop session creation failed")
	}

	startsAt := time.Now().Add(-2 * time.Hour).Format("2006-01-02T15:04")
	endsAt := time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04")
	form := url.Values{
		"csrf_token":   {app.sessions[sysopSID].csrf},
		"action":       {"save_challenge"},
		"name":         {"Spring Return Ladder"},
		"theme":        {"Retro Sprint"},
		"description":  {"Cross-surface points challenge."},
		"starts_at":    {startsAt},
		"ends_at":      {endsAt},
		"board_weight": {"2"},
		"chat_weight":  {"1"},
		"door_weight":  {"3"},
		"active":       {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/challenges", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr := httptest.NewRecorder()
	app.handleAdminChallenges(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save challenge redirect status = %d body=%s", rr.Code, rr.Body.String())
	}
	challenges := app.loadSeasonChallenges()
	if len(challenges) != 1 {
		t.Fatalf("expected one season challenge, got %+v", challenges)
	}

	form = url.Values{
		"csrf_token":  {app.sessions[sysopSID].csrf},
		"action":      {"save_goal"},
		"title":       {"Reach 10 Retro Runs"},
		"board_id":    {strconv.FormatInt(board.ID, 10)},
		"door_id":     {"retro-door"},
		"target":      {"10"},
		"progress":    {"2"},
		"description": {"Combine board posts and door launches."},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/challenges", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.handleAdminChallenges(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save goal redirect status = %d body=%s", rr.Code, rr.Body.String())
	}
	goals := app.loadClubhouseGoals()
	if len(goals) != 1 {
		t.Fatalf("expected one clubhouse goal, got %+v", goals)
	}
	goalID := goals[0].ID

	req = httptest.NewRequest(http.MethodGet, "/challenges", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleChallenges(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("challenges status = %d", rr.Code)
	}
	challengeBody := rr.Body.String()
	for _, needle := range []string{
		"Spring Return Ladder",
		"caller",
		">6</strong><span>your challenge points</span>",
	} {
		if !strings.Contains(challengeBody, needle) {
			t.Fatalf("expected %q on challenges page: %s", needle, challengeBody)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/clubhouse", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleClubhouse(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("clubhouse status = %d", rr.Code)
	}
	clubhouseBody := rr.Body.String()
	for _, needle := range []string{
		"Shared Goals",
		"Reach 10 Retro Runs",
		"/scores?door=retro-door",
	} {
		if !strings.Contains(clubhouseBody, needle) {
			t.Fatalf("expected %q on clubhouse page: %s", needle, clubhouseBody)
		}
	}

	form = url.Values{
		"csrf_token": {app.sessions[callerSID].csrf},
		"action":     {"goal_progress"},
		"goal_id":    {goalID},
		"delta":      {"3"},
	}
	req = httptest.NewRequest(http.MethodPost, "/clubhouse", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleClubhouse(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("goal progress redirect status = %d body=%s", rr.Code, rr.Body.String())
	}
	updatedGoals := app.loadClubhouseGoals()
	if len(updatedGoals) != 1 || updatedGoals[0].Progress != 5 {
		t.Fatalf("expected goal progress to advance to 5, got %+v", updatedGoals)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/challenges", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.handleAdminChallenges(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin challenges status = %d", rr.Code)
	}
	adminBody := rr.Body.String()
	for _, needle := range []string{"Season Challenges", "Spring Return Ladder", "Reach 10 Retro Runs"} {
		if !strings.Contains(adminBody, needle) {
			t.Fatalf("expected %q in admin challenges page: %s", needle, adminBody)
		}
	}
}

func TestAdminUpgradeSafetyAndBackupBrowser(t *testing.T) {
	prefix := t.TempDir()
	menuRoot := filepath.Join(prefix, "menus")
	offlineRoot := filepath.Join(prefix, "offline")
	if err := os.MkdirAll(menuRoot, 0o755); err != nil {
		t.Fatalf("mkdir menu root: %v", err)
	}
	if err := os.MkdirAll(offlineRoot, 0o755); err != nil {
		t.Fatalf("mkdir offline root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(prefix, ".env"), []byte("WOLFBBS_SITE_NAME=WolfBBS\n"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(prefix, "FIRST_STEPS.txt"), []byte("Step 1: verify setup\n"), 0o644); err != nil {
		t.Fatalf("write first steps: %v", err)
	}
	if err := os.WriteFile(filepath.Join(prefix, "SERVICE_STATUS.txt"), []byte("WolfBBS Service Status\n"), 0o644); err != nil {
		t.Fatalf("write service status: %v", err)
	}
	if err := os.WriteFile(filepath.Join(prefix, "install.log"), []byte("installer run\n"), 0o644); err != nil {
		t.Fatalf("write install log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(menuRoot, "main.hjson.20260318-120000.bak"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write menu backup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(offlineRoot, "caller.json"), []byte(`{"handle":"caller","boards":[]}`), 0o644); err != nil {
		t.Fatalf("write offline packet: %v", err)
	}
	if err := os.WriteFile(filepath.Join(offlineRoot, "caller.txt"), []byte("offline text packet"), 0o644); err != nil {
		t.Fatalf("write offline text: %v", err)
	}
	t.Setenv("WOLFBBS_INSTALL_PREFIX", prefix)

	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	app := &webApp{
		authSvc:    authSvc,
		userRepo:   userRepo,
		adminRepo:  repository.NewInMemoryAdminRepository(),
		sessions:   map[string]sessionState{},
		runtimeCfg: config.DefaultRuntime(),
		menuRoot:   menuRoot,
		offlineDir: offlineRoot,
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("sysop session creation failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/upgrade-safety", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleAdminUpgradeSafety(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upgrade safety status = %d", rr.Code)
	}
	safetyBody := rr.Body.String()
	for _, needle := range []string{"Upgrade Safety Dashboard", "Menu backup history", "Offline archive path"} {
		if !strings.Contains(safetyBody, needle) {
			t.Fatalf("expected %q on upgrade safety page: %s", needle, safetyBody)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/backups", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleAdminBackups(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("backup browser status = %d", rr.Code)
	}
	backupBody := rr.Body.String()
	for _, needle := range []string{"Backup Browser", "service_snapshot", "menu_backup", "offline packet for caller"} {
		if !strings.Contains(backupBody, needle) {
			t.Fatalf("expected %q on backup browser page: %s", needle, backupBody)
		}
	}

	artifacts := app.listBackupArtifacts(0)
	if len(artifacts) == 0 {
		t.Fatal("expected backup artifacts to be discovered")
	}
	kinds := map[string]bool{}
	for _, row := range artifacts {
		kinds[row.Kind] = true
	}
	for _, kind := range []string{"service_snapshot", "menu_backup", "offline_packet"} {
		if !kinds[kind] {
			t.Fatalf("expected artifact kind %q in %+v", kind, artifacts)
		}
	}
}

func TestRoadmap146PluginManifestCapabilityMatrix(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		adminRepo: repository.NewInMemoryAdminRepository(),
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}

	form := url.Values{
		"csrf_token":      {app.sessions[sid].csrf},
		"action":          {"save_plugin"},
		"id":              {"mod_sync"},
		"entrypoint":      {"/app/plugins/mod-sync"},
		"capabilities":    {"board.read,board.write,http.outbound"},
		"sandbox_profile": {"strict"},
		"retention_days":  {"45"},
		"enabled":         {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/plugins", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleAdminPlugins(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save plugin status = %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/plugins", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleAdminPlugins(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("plugins page status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, needle := range []string{"Plugin Manifest Contracts", "mod_sync", "board.read", "Capability Matrix", "Starter SDK Pack"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in plugins page: %s", needle, body)
		}
	}
}

func TestRoadmap146PluginStarterPackDownload(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		adminRepo: repository.NewInMemoryAdminRepository(),
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/plugins/starter?id=mod_sync", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleAdminPluginStarter(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("starter pack status = %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/zip") {
		t.Fatalf("expected zip content-type, got %q", got)
	}
	zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	names := map[string]bool{}
	manifestBody := ""
	for _, file := range zr.File {
		names[file.Name] = true
		if file.Name == "mod_sync/manifest.json" {
			rc, err := file.Open()
			if err != nil {
				t.Fatalf("open manifest file: %v", err)
			}
			raw, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				t.Fatalf("read manifest file: %v", err)
			}
			manifestBody = string(raw)
		}
	}
	for _, want := range []string{
		"mod_sync/manifest.json",
		"mod_sync/entrypoint.sh",
		"mod_sync/event.sample.json",
		"mod_sync/README.md",
	} {
		if !names[want] {
			t.Fatalf("missing expected starter file %q in %+v", want, names)
		}
	}
	if !strings.Contains(manifestBody, `"id": "mod_sync"`) {
		t.Fatalf("expected plugin id in manifest: %s", manifestBody)
	}
}

func TestRoadmap147ThemeMarketplaceImportAndApply(t *testing.T) {
	t.Setenv("WOLFBBS_THEME_BUNDLE_DIR", t.TempDir())
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	app := &webApp{
		authSvc:   authSvc,
		userRepo:  userRepo,
		adminRepo: repository.NewInMemoryAdminRepository(),
		sessions:  map[string]sessionState{},
	}
	sid, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("session creation failed")
	}

	bundle := `{"manifest":{"id":"retro_night","name":"Retro Night","version":"1.0.0","author":"sysop"},"themes":[{"name":"retro-night","status_fg":"fg-black","status_bg":"bg-gray","body_fg":"fg-yellow","accent_fg":"fg-orange","warn_fg":"fg-red","error_fg":"fg-magenta","muted_fg":"fg-green"}]}`
	form := url.Values{
		"csrf_token":  {app.sessions[sid].csrf},
		"action":      {"import_bundle"},
		"bundle_json": {bundle},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/themes", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.handleAdminThemes(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("import bundle status = %d body=%s", rr.Code, rr.Body.String())
	}

	form = url.Values{
		"csrf_token": {app.sessions[sid].csrf},
		"action":     {"apply_bundle"},
		"id":         {"retro_night"},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/themes", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr = httptest.NewRecorder()
	app.handleAdminThemes(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("apply bundle status = %d body=%s", rr.Code, rr.Body.String())
	}
	if got := app.activeThemeMarketplaceBundleID(); got != "retro_night" {
		t.Fatalf("expected active bundle retro_night, got %q", got)
	}

	themeFound := false
	for _, name := range ui.ThemeNames() {
		if name == "retro-night" {
			themeFound = true
			break
		}
	}
	if !themeFound {
		t.Fatalf("expected imported theme retro-night in %+v", ui.ThemeNames())
	}
}

func TestRoadmap148WebhookBridgeRetrySemantics(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode webhook payload: %v", err)
		}
		if payload["event"] != "board.created" {
			t.Fatalf("expected event board.created, got %+v", payload)
		}
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	app := &webApp{adminRepo: repository.NewInMemoryAdminRepository()}
	app.persistWebhookBridgeConfig(webhookBridgeConfig{
		Enabled:        true,
		Endpoint:       server.URL,
		Events:         []string{"board.created"},
		RetryAttempts:  2,
		BackoffMS:      1,
		TimeoutSeconds: 2,
	})
	app.dispatchWebhookEvent("board.created", map[string]string{"board_id": "42", "board": "General"})
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	logs := app.loadWebhookDeliveryLogs()
	if len(logs) == 0 {
		t.Fatal("expected webhook delivery log rows")
	}
	if !logs[0].Success || logs[0].Attempts != 2 {
		t.Fatalf("unexpected webhook log %+v", logs[0])
	}
}

func TestRoadmap149AnalyticsAndRetentionRoutes(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	doorRepo := repository.NewInMemoryDoorRepository()
	authSvc := auth.NewService(userRepo)
	chatSvc := chat.NewServiceForTest()

	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if _, err := authSvc.Register("sysop", "password123"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	board := &domain.Board{Name: "General", Conference: "Public", CreatedBy: caller.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{BoardID: board.ID, AuthorID: caller.ID, Subject: "weekly update", Body: "msg", CreatedAt: time.Now().UTC().Add(-30 * time.Minute)}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	chatSvc.JoinChannel("caller", "#lobby")
	if _, err := chatSvc.Post("caller", "#lobby", "chat check-in"); err != nil {
		t.Fatalf("chat post: %v", err)
	}
	doorRegistry := doors.NewRegistry()
	doorRegistry.SetRepository(doorRepo)
	doorRegistry.Register(doors.Door{ID: "retro-door", Hotkey: "R", Name: "Retro Door", Command: "/bin/true"})
	if err := doorRepo.AddEvent(&domain.DoorEvent{DoorID: "retro-door", UserID: caller.ID, EventType: "start", CreatedAt: time.Now().UTC().Add(-20 * time.Minute)}); err != nil {
		t.Fatalf("seed door event: %v", err)
	}

	app := &webApp{
		authSvc:      authSvc,
		userRepo:     userRepo,
		boardRepo:    boardRepo,
		msgRepo:      msgRepo,
		adminRepo:    adminRepo,
		doorRepo:     doorRepo,
		doorRegistry: doorRegistry,
		chatSvc:      chatSvc,
		sessions:     map[string]sessionState{},
	}
	app.setEventAttendance("evt-1", "caller", true)

	callerSID, ok := app.createSession("caller")
	if !ok {
		t.Fatal("caller session creation failed")
	}
	sysopSID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("sysop session creation failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/analytics", nil)
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr := httptest.NewRecorder()
	app.handleAdminAnalytics(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin analytics status = %d body=%s", rr.Code, rr.Body.String())
	}
	for _, needle := range []string{"Embedded Product Analytics", "Daily", "Weekly", "Monthly"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("expected %q on analytics page: %s", needle, rr.Body.String())
		}
	}

	form := url.Values{
		"csrf_token": {app.sessions[callerSID].csrf},
		"sunday":     {"10"},
		"monday":     {"20"},
		"tuesday":    {"12"},
		"wednesday":  {"12"},
		"thursday":   {"12"},
		"friday":     {"12"},
		"saturday":   {"12"},
	}
	req = httptest.NewRequest(http.MethodPost, "/digest/preferences", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleDigestPreferences(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("digest preferences save status = %d body=%s", rr.Code, rr.Body.String())
	}
	if got := app.loadWeekdayDigestPrefs("caller", 12).Monday; got != 20 {
		t.Fatalf("expected monday digest max 20, got %d", got)
	}

	starts := time.Now().Add(-2 * time.Hour).Format("2006-01-02T15:04")
	ends := time.Now().Add(24 * time.Hour).Format("2006-01-02T15:04")
	form = url.Values{
		"csrf_token":         {app.sessions[sysopSID].csrf},
		"action":             {"save_mission"},
		"id":                 {"spring-return-1"},
		"season":             {"Spring 2026"},
		"title":              {"Post Chat Play"},
		"description":        {"Complete all three lanes."},
		"starts_at":          {starts},
		"ends_at":            {ends},
		"target_board_posts": {"1"},
		"target_chat_posts":  {"1"},
		"target_door_runs":   {"1"},
		"active":             {"1"},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/missions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sysopSID})
	rr = httptest.NewRecorder()
	app.handleAdminMissions(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save mission status = %d body=%s", rr.Code, rr.Body.String())
	}

	form = url.Values{
		"csrf_token": {app.sessions[callerSID].csrf},
		"action":     {"claim"},
		"mission_id": {"spring-return-1"},
	}
	req = httptest.NewRequest(http.MethodPost, "/missions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
	rr = httptest.NewRecorder()
	app.handleMissions(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("claim mission status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !app.userMissionClaimed("spring-return-1", "caller") {
		t.Fatal("expected mission to be claimed for caller")
	}

	for _, tc := range []struct {
		path string
		fn   func(http.ResponseWriter, *http.Request)
	}{
		{path: "/next", fn: app.handleNextActions},
		{path: "/streaks", fn: app.handleStreaks},
		{path: "/spotlights", fn: app.handleSpotlights},
	} {
		req = httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: callerSID})
		rr = httptest.NewRecorder()
		tc.fn(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", tc.path, rr.Code, rr.Body.String())
		}
	}
}

func TestRoadmap156To165Features(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	doorRepo := repository.NewInMemoryDoorRepository()
	doorRegistry := doors.NewRegistry()
	doorRegistry.SetRepository(doorRepo)
	doorRegistry.Register(doors.Door{ID: "retro-door", Hotkey: "R", Name: "Retro Door", Command: "/bin/true"})
	chatSvc := chat.NewServiceForTest()

	sysop, err := authSvc.Register("sysop", "password123")
	if err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	mentor, err := authSvc.Register("mentor", "password123")
	if err != nil {
		t.Fatalf("register mentor: %v", err)
	}
	caller, err := authSvc.Register("caller", "password123")
	if err != nil {
		t.Fatalf("register caller: %v", err)
	}
	if err := authSvc.SetRole("sysop", roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	if err := authSvc.SetRole("mentor", roleModerator); err != nil {
		t.Fatalf("set mentor role: %v", err)
	}
	if err := authSvc.SetVerified("sysop", true); err != nil {
		t.Fatalf("verify sysop: %v", err)
	}
	if err := authSvc.SetVerified("mentor", true); err != nil {
		t.Fatalf("verify mentor: %v", err)
	}
	if err := authSvc.SetVerified("caller", true); err != nil {
		t.Fatalf("verify caller: %v", err)
	}

	board := &domain.Board{Name: "General", Conference: "Public", CreatedBy: sysop.ID}
	if err := boardRepo.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	msg := &domain.Message{
		BoardID:   board.ID,
		AuthorID:  caller.ID,
		Subject:   "Need help with setup",
		Body:      "starter question",
		CreatedAt: time.Now().UTC().Add(-45 * time.Minute),
	}
	if err := msgRepo.CreateMessage(msg); err != nil {
		t.Fatalf("create message: %v", err)
	}
	if err := msgRepo.CreateReport(&domain.MessageReport{
		MessageID:  msg.ID,
		ReporterID: mentor.ID,
		Reason:     "qa moderation report",
		Status:     "open",
		CreatedAt:  time.Now().UTC().Add(-20 * time.Minute),
	}); err != nil {
		t.Fatalf("create report: %v", err)
	}
	chatSvc.JoinChannel("caller", "#lobby")
	if _, err := chatSvc.Post("caller", "#lobby", "first chat line"); err != nil {
		t.Fatalf("chat post: %v", err)
	}
	if err := doorRepo.AddEvent(&domain.DoorEvent{
		DoorID:    "retro-door",
		UserID:    caller.ID,
		EventType: "start",
		CreatedAt: time.Now().UTC().Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("seed door event: %v", err)
	}

	app := &webApp{
		authSvc:      authSvc,
		userRepo:     userRepo,
		boardRepo:    boardRepo,
		msgRepo:      msgRepo,
		mailRepo:     mailRepo,
		adminRepo:    adminRepo,
		doorRepo:     doorRepo,
		doorRegistry: doorRegistry,
		chatSvc:      chatSvc,
		sessions:     map[string]sessionState{},
	}

	sysopSID, ok := app.createSession("sysop")
	if !ok {
		t.Fatal("sysop session creation failed")
	}
	callerSID, ok := app.createSession("caller")
	if !ok {
		t.Fatal("caller session creation failed")
	}
	sysopCSRF := app.sessions[sysopSID].csrf
	callerCSRF := app.sessions[callerSID].csrf

	mentorshipAdmin := app.mustBeRole(roleModerator, http.HandlerFunc(app.handleAdminMentorship))
	modCenter := app.mustBeRole(roleModerator, http.HandlerFunc(app.handleAdminModCenter))

	getWithSession := func(path, sid string, fn func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
		rr := httptest.NewRecorder()
		fn(rr, req)
		return rr
	}
	postWithSession := func(path, sid string, form url.Values, fn func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
		rr := httptest.NewRecorder()
		fn(rr, req)
		return rr
	}

	for _, tc := range []struct {
		path   string
		expect string
		fn     func(http.ResponseWriter, *http.Request)
	}{
		{path: "/resume", expect: "Smart Re-entry", fn: app.handleResumeCenter},
		{path: "/doors/comeback", expect: "Door Comeback Prompts", fn: app.handleDoorComeback},
		{path: "/time-lane", expect: "Time-of-Day Landing States", fn: app.handleTimeLane},
		{path: "/milestones", expect: "Milestone Celebrations", fn: app.handleMilestones},
	} {
		rr := getWithSession(tc.path, callerSID, tc.fn)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", tc.path, rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), tc.expect) {
			t.Fatalf("expected %q in %s body: %s", tc.expect, tc.path, rr.Body.String())
		}
	}

	form := url.Values{
		"csrf_token": {sysopCSRF},
		"action":     {"save_pair"},
		"mentee":     {"caller"},
		"mentor":     {"mentor"},
		"note":       {"welcome lane"},
		"active":     {"1"},
	}
	rr := postWithSession("/admin/mentorship", sysopSID, form, mentorshipAdmin.ServeHTTP)
	if rr.Code != http.StatusFound {
		t.Fatalf("save mentorship pair status = %d body=%s", rr.Code, rr.Body.String())
	}
	if pair, ok := app.mentorshipForMentee("caller"); !ok || pair.Mentor != "mentor" {
		t.Fatalf("expected active mentorship pair caller->mentor, got pair=%+v ok=%v", pair, ok)
	}

	rr = getWithSession("/mentorship", callerSID, app.handleMentorship)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Your Mentor") {
		t.Fatalf("mentorship page status=%d body=%s", rr.Code, rr.Body.String())
	}
	form = url.Values{
		"csrf_token": {callerCSRF},
		"action":     {"ping_mentor"},
		"message":    {"I am blocked on boards"},
	}
	rr = postWithSession("/mentorship", callerSID, form, app.handleMentorship)
	if rr.Code != http.StatusFound {
		t.Fatalf("ping mentor status = %d body=%s", rr.Code, rr.Body.String())
	}
	mentorInbox, err := mailRepo.ListInbox(mentor.ID, 20)
	if err != nil {
		t.Fatalf("list mentor inbox: %v", err)
	}
	if len(mentorInbox) == 0 || !strings.Contains(strings.ToLower(mentorInbox[0].Subject), "mentorship") {
		t.Fatalf("expected mentorship mail in mentor inbox, got %+v", mentorInbox)
	}

	form = url.Values{
		"csrf_token": {callerCSRF},
		"action":     {"celebrate"},
		"id":         {"first_post"},
	}
	rr = postWithSession("/milestones", callerSID, form, app.handleMilestones)
	if rr.Code != http.StatusFound {
		t.Fatalf("milestone celebrate status = %d body=%s", rr.Code, rr.Body.String())
	}
	if _, ok := app.loadMilestoneCelebrations("caller")["first_post"]; !ok {
		t.Fatal("expected first_post celebration to persist")
	}

	form = url.Values{
		"csrf_token": {callerCSRF},
		"action":     {"set_home"},
		"route":      {"/doors"},
	}
	rr = postWithSession("/time-lane", callerSID, form, app.handleTimeLane)
	if rr.Code != http.StatusFound {
		t.Fatalf("time-lane set home status = %d body=%s", rr.Code, rr.Body.String())
	}
	if got := app.loadHomeRoute("caller"); got != "/doors" {
		t.Fatalf("expected home route /doors, got %q", got)
	}

	rr = getWithSession("/directory?handle=caller", sysopSID, app.handleDirectory)
	if rr.Code != http.StatusOK {
		t.Fatalf("directory status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Caller Risk Summary") {
		t.Fatalf("expected caller risk summary card on directory profile: %s", rr.Body.String())
	}

	reports, err := msgRepo.ListReports(10, "")
	if err != nil || len(reports) == 0 {
		t.Fatalf("list reports for assignment: reports=%+v err=%v", reports, err)
	}
	reportID := reports[0].ID
	form = url.Values{
		"csrf_token": {sysopCSRF},
		"action":     {"save_report_assignment"},
		"report_id":  {strconv.FormatInt(reportID, 10)},
		"assignee":   {"mentor"},
		"status":     {"investigating"},
		"priority":   {"high"},
		"due_hours":  {"2"},
		"note":       {"triage this first"},
	}
	rr = postWithSession("/admin/mod-center", sysopSID, form, modCenter.ServeHTTP)
	if rr.Code != http.StatusFound {
		t.Fatalf("save report assignment status = %d body=%s", rr.Code, rr.Body.String())
	}
	assignments := app.loadReportAssignments()
	assignment, ok := assignments[reportID]
	if !ok {
		t.Fatalf("expected assignment for report %d, got %+v", reportID, assignments)
	}
	if assignment.Assignee != "mentor" || assignment.Priority != "high" || assignment.Status != "investigating" {
		t.Fatalf("unexpected assignment state: %+v", assignment)
	}
	if assignment.DueAt.IsZero() {
		t.Fatalf("expected assignment due time to be set: %+v", assignment)
	}

	form = url.Values{
		"csrf_token": {sysopCSRF},
		"action":     {"save_canned"},
		"id":         {"warn_spam_v1"},
		"category":   {"conduct"},
		"title":      {"Friendly Reminder"},
		"body":       {"Please keep posts constructive."},
	}
	rr = postWithSession("/admin/mod-center", sysopSID, form, modCenter.ServeHTTP)
	if rr.Code != http.StatusFound {
		t.Fatalf("save canned response status = %d body=%s", rr.Code, rr.Body.String())
	}
	templates := app.loadModeratorTemplates()
	if len(templates) == 0 || templates[0].ID != "warn_spam_v1" {
		t.Fatalf("expected canned template to persist, got %+v", templates)
	}

	form = url.Values{
		"csrf_token":  {sysopCSRF},
		"action":      {"send_canned"},
		"template_id": {"warn_spam_v1"},
		"target":      {"caller"},
		"extra_note":  {"next violation escalates"},
	}
	rr = postWithSession("/admin/mod-center", sysopSID, form, modCenter.ServeHTTP)
	if rr.Code != http.StatusFound {
		t.Fatalf("send canned response status = %d body=%s", rr.Code, rr.Body.String())
	}
	callerInbox, err := mailRepo.ListInbox(caller.ID, 20)
	if err != nil {
		t.Fatalf("list caller inbox: %v", err)
	}
	foundCanned := false
	for _, row := range callerInbox {
		if strings.Contains(strings.ToLower(row.Subject), "friendly reminder") {
			foundCanned = true
			break
		}
	}
	if !foundCanned {
		t.Fatalf("expected canned response mail in caller inbox, got %+v", callerInbox)
	}

	form = url.Values{
		"csrf_token":    {sysopCSRF},
		"action":        {"save_case"},
		"title":         {"Incident follow-up thread"},
		"status":        {"open"},
		"priority":      {"high"},
		"owner":         {"mentor"},
		"target_handle": {"caller"},
		"report_id":     {strconv.FormatInt(reportID, 10)},
		"summary":       {"track remediation"},
	}
	rr = postWithSession("/admin/mod-center", sysopSID, form, modCenter.ServeHTTP)
	if rr.Code != http.StatusFound {
		t.Fatalf("save case status = %d body=%s", rr.Code, rr.Body.String())
	}
	cases := app.loadCaseThreads()
	if len(cases) == 0 {
		t.Fatal("expected at least one case thread")
	}
	caseID := cases[0].ID
	form = url.Values{
		"csrf_token": {sysopCSRF},
		"action":     {"add_case_note"},
		"case_id":    {caseID},
		"note":       {"first handoff note"},
	}
	rr = postWithSession("/admin/mod-center", sysopSID, form, modCenter.ServeHTTP)
	if rr.Code != http.StatusFound {
		t.Fatalf("add case note status = %d body=%s", rr.Code, rr.Body.String())
	}
	form = url.Values{
		"csrf_token": {sysopCSRF},
		"action":     {"close_case"},
		"case_id":    {caseID},
	}
	rr = postWithSession("/admin/mod-center", sysopSID, form, modCenter.ServeHTTP)
	if rr.Code != http.StatusFound {
		t.Fatalf("close case status = %d body=%s", rr.Code, rr.Body.String())
	}
	cases = app.loadCaseThreads()
	if len(cases) == 0 || cases[0].Status != "resolved" || len(cases[0].Updates) == 0 {
		t.Fatalf("expected resolved case with updates, got %+v", cases)
	}

	rr = getWithSession("/admin/mod-center", sysopSID, modCenter.ServeHTTP)
	if rr.Code != http.StatusOK {
		t.Fatalf("mod center status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Moderation Center") {
		t.Fatalf("expected moderation center heading, got %s", rr.Body.String())
	}
}
