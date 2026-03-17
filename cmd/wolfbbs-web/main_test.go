package main

import (
	"bytes"
	"encoding/json"
	"errors"
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
	"wolfbbs/internal/domain"
	"wolfbbs/internal/doors"
	"wolfbbs/internal/gateway"
	"wolfbbs/internal/mods"
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
	if rr.Code != http.StatusOK {
		t.Fatalf("valid create status = %d body=%q", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "created user alphauser") {
		t.Fatalf("expected create success message, got %q", rr.Body.String())
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

	create := url.Values{}
	create.Set("action", "create")
	create.Set("handle", "qauser")
	create.Set("password", "qa123456")
	create.Set("role", "user")
	rr := postUsers(create)
	if rr.Code != http.StatusOK {
		t.Fatalf("create user status = %d body=%q", rr.Code, rr.Body.String())
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
	if rr.Code != http.StatusOK {
		t.Fatalf("reset password status = %d body=%q", rr.Code, rr.Body.String())
	}
	resetMsg := strings.TrimSpace(rr.Body.String())
	const resetPrefix = "reset password for qauser to "
	if !strings.HasPrefix(resetMsg, resetPrefix) {
		t.Fatalf("unexpected reset response: %q", resetMsg)
	}
	newPassword := strings.TrimSpace(strings.TrimPrefix(resetMsg, resetPrefix))
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
	authSvc := auth.NewService(userRepo)
	_, _ = authSvc.Register("prefs", "password123")

	app := &webApp{
		authSvc:  authSvc,
		userRepo: userRepo,
		sessions: map[string]sessionState{},
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
	app.handleGuestTour(rr, httptest.NewRequest(http.MethodGet, "/tour", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("tour status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Guided Tour") {
		t.Fatalf("tour page missing heading: %s", rr.Body.String())
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
	if got := webQuickJumpPath("status"); got != "/status" {
		t.Fatalf("expected /status, got %q", got)
	}
	if got := webQuickJumpPath("doors"); got != "/doors" {
		t.Fatalf("expected /doors, got %q", got)
	}
	if got := webQuickJumpPath("radar"); got != "/radar" {
		t.Fatalf("expected /radar, got %q", got)
	}
	if got := webQuickJumpPath("clubhouse"); got != "/clubhouse" {
		t.Fatalf("expected /clubhouse, got %q", got)
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
