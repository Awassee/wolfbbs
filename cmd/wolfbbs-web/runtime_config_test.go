package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/config"
	"wolfbbs/internal/repository"
)

func TestAdminConfigSaveRuntimeServicesAndReload(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	authSvc := auth.NewService(userRepo)
	user, err := authSvc.Register("sysop", "password123")
	if err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole(user.Handle, roleAdmin); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}

	app := &webApp{
		authSvc:      authSvc,
		adminRepo:    adminRepo,
		sessions:     map[string]sessionState{},
		runtimeCfg:   config.DefaultRuntime(),
		siteName:     "WolfBBS",
		siteHostname: "localhost",
		menuRoot:     t.TempDir(),
	}
	sid, ok := app.createSession(user.Handle)
	if !ok {
		t.Fatal("create session failed")
	}
	state := app.sessions[sid]

	form := url.Values{}
	form.Set("action", "save_runtime_services")
	form.Set("acs_strict", "1")
	form.Set("content_host", "bbs.example.com")
	form.Set("content_gopher_listen", ":7070")
	form.Set("content_nntp_listen", ":1190")
	form.Set("content_nntps_listen", ":5630")
	form.Set("content_nntps_cert", "/tmp/nntps.crt")
	form.Set("content_nntps_key", "/tmp/nntps.key")
	form.Set("activitypub_enabled", "1")
	form.Set("activitypub_base_url", "https://bbs.example.com/ap")
	form.Set("login_telnet_enabled", "1")
	form.Set("login_telnet_listen", ":2323")
	form.Set("login_ws_enabled", "1")
	form.Set("login_ws_listen", ":6080")
	form.Set("login_ws_path", "/ws-login")
	form.Set("login_wss_enabled", "1")
	form.Set("login_wss_listen", ":6443")
	form.Set("login_wss_path", "/wss-login")
	form.Set("login_wss_cert", "/tmp/wss.crt")
	form.Set("login_wss_key", "/tmp/wss.key")
	form.Set("login_trusted_proxies", "127.0.0.1/32,10.0.0.0/8")
	form.Set("connector_doorparty_enabled", "1")
	form.Set("connector_doorparty_command", "doorparty")
	form.Set("connector_doorparty_args", "--token x")
	form.Set("connector_bbslink_enabled", "1")
	form.Set("connector_bbslink_command", "bbslink")
	form.Set("connector_bbslink_args", "--node 1")
	form.Set("connector_telnet_enabled", "1")
	form.Set("connector_telnet_command", "bridge")
	form.Set("connector_telnet_args", "--host remote")
	form.Set("csrf_token", state.csrf)

	req := httptest.NewRequest(http.MethodPost, "/admin/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "wolfbbs_session", Value: sid})
	rr := httptest.NewRecorder()
	app.mustBeRole(roleAdmin, http.HandlerFunc(app.handleAdminConfig)).ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("save runtime services status = %d body=%s", rr.Code, rr.Body.String())
	}

	if !app.runtimeCfg.Login.WebSocket.Enabled || app.runtimeCfg.Login.WebSocket.Path != "/ws-login" {
		t.Fatalf("runtime websocket settings not applied: enabled=%t path=%q", app.runtimeCfg.Login.WebSocket.Enabled, app.runtimeCfg.Login.WebSocket.Path)
	}
	if !app.runtimeCfg.Connectors.BBSLink.Enabled || app.runtimeCfg.Connectors.BBSLink.Command != "bbslink" {
		t.Fatalf("runtime connector settings not applied: enabled=%t command=%q", app.runtimeCfg.Connectors.BBSLink.Enabled, app.runtimeCfg.Connectors.BBSLink.Command)
	}
	if app.runtimeCfg.Content.Host != "bbs.example.com" {
		t.Fatalf("runtime content host not applied: %q", app.runtimeCfg.Content.Host)
	}
	if v, err := adminRepo.GetSystemSetting(sysSettingLoginWSEnabled); err != nil || strings.TrimSpace(v) != "true" {
		t.Fatalf("persisted ws enable mismatch value=%q err=%v", v, err)
	}
	if v, err := adminRepo.GetSystemSetting(sysSettingConnectorBBSLinkCmd); err != nil || strings.TrimSpace(v) != "bbslink" {
		t.Fatalf("persisted bbslink command mismatch value=%q err=%v", v, err)
	}

	appReload := &webApp{
		adminRepo:  adminRepo,
		runtimeCfg: config.DefaultRuntime(),
	}
	appReload.loadPersistedAdminSettings()
	if !appReload.runtimeCfg.Login.WebSocket.Enabled || appReload.runtimeCfg.Login.WebSocket.Listen != ":6080" {
		t.Fatalf("runtime ws settings did not reload from system settings: enabled=%t listen=%q", appReload.runtimeCfg.Login.WebSocket.Enabled, appReload.runtimeCfg.Login.WebSocket.Listen)
	}
	if !appReload.runtimeCfg.Connectors.Telnet.Enabled || appReload.runtimeCfg.Connectors.Telnet.Command != "bridge" {
		t.Fatalf("runtime telnet connector did not reload: enabled=%t command=%q", appReload.runtimeCfg.Connectors.Telnet.Enabled, appReload.runtimeCfg.Connectors.Telnet.Command)
	}
}
