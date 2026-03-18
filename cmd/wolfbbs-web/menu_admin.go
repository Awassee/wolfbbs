package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (a *webApp) menuRootPath() string {
	root := filepath.Clean(strings.TrimSpace(a.menuRoot))
	if root == "" || root == "." {
		root = "menus"
	}
	return root
}

func (a *webApp) defaultMenuFile() string {
	root := a.menuRootPath()
	path := filepath.Join(root, "main.hjson")
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.ToSlash(path)
}

func (a *webApp) normalizeMenuFilePath(raw string) (string, error) {
	root := a.menuRootPath()
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	clean := filepath.Clean(strings.TrimSpace(raw))
	if clean == "" || clean == "." {
		clean = a.defaultMenuFile()
	}
	if !filepath.IsAbs(clean) {
		if filepath.IsAbs(root) {
			clean = filepath.Join(root, clean)
		} else {
			rootPrefix := filepath.ToSlash(root) + "/"
			cleanSlash := filepath.ToSlash(clean)
			if !strings.HasPrefix(cleanSlash, rootPrefix) {
				clean = filepath.Join(root, clean)
			}
		}
	}
	if strings.ToLower(filepath.Ext(clean)) != ".hjson" {
		return "", fmt.Errorf("menu file must end with .hjson")
	}

	absFile, err := filepath.Abs(clean)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, absFile)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("menu file must be within %s", root)
	}
	if filepath.IsAbs(clean) {
		return clean, nil
	}
	return filepath.ToSlash(clean), nil
}

func (a *webApp) listMenuFiles(selected string) []string {
	root := a.menuRootPath()
	files := make([]string, 0)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) != ".hjson" {
			return nil
		}
		if filepath.IsAbs(path) {
			files = append(files, path)
		} else {
			files = append(files, filepath.ToSlash(path))
		}
		return nil
	})
	if normalized, err := a.normalizeMenuFilePath(selected); err == nil {
		selected = normalized
	}
	if strings.TrimSpace(selected) != "" {
		found := false
		for _, row := range files {
			if row == selected {
				found = true
				break
			}
		}
		if !found {
			files = append(files, selected)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i]) < strings.ToLower(files[j])
	})
	return files
}

func (a *webApp) loadMenuFile(path string) (string, error) {
	path, err := a.normalizeMenuFilePath(path)
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (a *webApp) saveMenuFile(path, body string) error {
	path, err := a.normalizeMenuFilePath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	existing, readErr := os.ReadFile(path)
	if readErr == nil {
		backup := fmt.Sprintf("%s.%s.bak", path, time.Now().UTC().Format("20060102-150405"))
		if writeErr := os.WriteFile(backup, existing, 0o644); writeErr != nil {
			return fmt.Errorf("backup existing menu: %w", writeErr)
		}
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func (a *webApp) renderAdminConfigPage(w http.ResponseWriter, r *http.Request, statusCode int, menuFile, menuBody, menuNotice, menuErr string) {
	if statusCode <= 0 {
		statusCode = http.StatusOK
	}
	if strings.TrimSpace(menuFile) == "" {
		menuFile = a.defaultMenuFile()
	}
	if strings.TrimSpace(menuBody) == "" {
		if loaded, err := a.loadMenuFile(menuFile); err == nil {
			menuBody = loaded
		} else if strings.TrimSpace(menuErr) == "" {
			menuErr = "menu load failed: " + err.Error()
		}
	}

	noticeBlock := ""
	if strings.TrimSpace(menuNotice) != "" {
		noticeBlock = `<p><strong>Menu:</strong> ` + htmlEscape(menuNotice) + `</p>`
	}
	errBlock := ""
	if strings.TrimSpace(menuErr) != "" {
		errBlock = `<p><strong>Menu error:</strong> ` + htmlEscape(menuErr) + `</p>`
	}

	menuRows := strings.Builder{}
	for _, path := range a.listMenuFiles(menuFile) {
		menuRows.WriteString(`<li><a href="/admin/config?menu_file=` + url.QueryEscape(path) + `">` + htmlEscape(path) + `</a></li>`)
	}
	if menuRows.Len() == 0 {
		menuRows.WriteString(`<li>No menu files found under ` + htmlEscape(a.menuRootPath()) + `</li>`)
	}

	csrf := a.csrfHiddenInput(r)
	configHelperBlock := `<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Use setup first</strong><p>Identity and baseline safety belong in /admin/setup before deeper runtime changes here.</p></article><article class="wolfbbs-helper-card"><strong>Treat transport changes carefully</strong><p>Listener, proxy, and exposure changes should be followed by a status check and a real caller walk-through.</p></article><article class="wolfbbs-helper-card"><strong>Menu editor is live config</strong><p>The ANSI menu file editor below saves runtime menu sources. Drafts are stored locally while you type.</p></article></section>`
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Runtime Configuration</title></head><body><h1>Runtime Configuration</h1><p><a href="/admin">back</a> | <a href="/admin/setup">setup</a> | <a href="/help">help</a></p>` +
		`<p>These values are persisted in system settings and applied on service startup. Environment values remain fallback defaults.</p>` +
		configHelperBlock +
		`<h2>Basic: Identity</h2><form method="POST">` + csrf +
		`<input type="hidden" name="action" value="save_identity">` +
		`<label>Site Name <input name="site_name" value="` + htmlEscape(a.siteDisplayName()) + `" size="32"></label><br>` +
		`<label>Hostname <input name="site_hostname" value="` + htmlEscape(a.siteHost()) + `" size="32"></label><br>` +
		`<button type="submit">Save Identity</button></form>` +
		`<h2>Basic: Site Text</h2><form method="POST">` + csrf +
		`<input type="hidden" name="action" value="save_text">` +
		`<label>MOTD<br><textarea name="motd" rows="4" cols="90">` + htmlEscape(a.motd) + `</textarea></label><br>` +
		`<label>Announcement<br><textarea name="announcement" rows="4" cols="90">` + htmlEscape(a.announcement) + `</textarea></label><br>` +
		`<button type="submit">Save Text</button></form>` +
		`<h2>Critical: Security + Safety</h2><form method="POST">` + csrf +
		`<input type="hidden" name="action" value="save_security">` +
		`<label><input type="checkbox" name="read_only"` + checkedIf(a.readOnly) + `> Read-only mode (block admin writes)</label><br>` +
		`<label><input type="checkbox" name="secure_cookie"` + checkedIf(a.secureCookie) + `> Secure cookie mode (enable behind HTTPS)</label><br>` +
		`<label><input type="checkbox" name="require_verified_email"` + checkedIf(a.requireVerifiedEmail) + `> Require verified users for external email</label><br>` +
		`<button type="submit">Save Security</button></form>` +
		`<h2>Feature Flags</h2><form method="POST">` + csrf +
		`<input type="hidden" name="action" value="save_flags">` +
		`<label><input type="checkbox" name="web_onramp"` + checkedIf(a.modernOnRamp) + `> Enable web connect on-ramp</label><br>` +
		`<label><input type="checkbox" name="guest_tour"` + checkedIf(a.guestTour) + `> Enable guided guest tour</label><br>` +
		`<label><input type="checkbox" name="discover"` + checkedIf(a.discover) + `> Enable discover/newscan web page</label><br>` +
		`<label><input type="checkbox" name="quick_jump"` + checkedIf(a.quickJump) + `> Enable quick jump commands (TUI/web)</label><br>` +
		`<label><input type="checkbox" name="classic_search"` + checkedIf(a.classicSearch) + `> Enable classic deep search presentation</label><br>` +
		`<button type="submit">Save Flags</button></form>` +
		`<h2>Advanced: Runtime Services</h2><form method="POST">` + csrf +
		`<input type="hidden" name="action" value="save_runtime_services">` +
		`<fieldset><legend><strong>Access + Security</strong></legend>` +
		`<label><input type="checkbox" name="acs_strict"` + checkedIf(a.runtimeCfg.ACS.Strict) + `> ACS strict mode (deny by default when rules fail)</label><br>` +
		`<label>Trusted proxies CIDR list <input name="login_trusted_proxies" value="` + htmlEscape(a.runtimeCfg.Login.TrustedProxies) + `" size="80"></label><br>` +
		`</fieldset>` +
		`<fieldset><legend><strong>Login Servers</strong></legend>` +
		`<label><input type="checkbox" name="login_telnet_enabled"` + checkedIf(a.runtimeCfg.Login.Telnet.Enabled) + `> Enable Telnet</label> ` +
		`<label>Listen <input name="login_telnet_listen" value="` + htmlEscape(a.runtimeCfg.Login.Telnet.Listen) + `" size="14"></label><br>` +
		`<label><input type="checkbox" name="login_ws_enabled"` + checkedIf(a.runtimeCfg.Login.WebSocket.Enabled) + `> Enable WebSocket</label> ` +
		`<label>Listen <input name="login_ws_listen" value="` + htmlEscape(a.runtimeCfg.Login.WebSocket.Listen) + `" size="14"></label> ` +
		`<label>Path <input name="login_ws_path" value="` + htmlEscape(a.runtimeCfg.Login.WebSocket.Path) + `" size="20"></label><br>` +
		`<label><input type="checkbox" name="login_wss_enabled"` + checkedIf(a.runtimeCfg.Login.WebSocketTLS.Enabled) + `> Enable WebSocket TLS</label> ` +
		`<label>Listen <input name="login_wss_listen" value="` + htmlEscape(a.runtimeCfg.Login.WebSocketTLS.Listen) + `" size="14"></label> ` +
		`<label>Path <input name="login_wss_path" value="` + htmlEscape(a.runtimeCfg.Login.WebSocketTLS.Path) + `" size="20"></label><br>` +
		`<label>TLS Cert <input name="login_wss_cert" value="` + htmlEscape(a.runtimeCfg.Login.WebSocketTLS.Cert) + `" size="52"></label><br>` +
		`<label>TLS Key <input name="login_wss_key" value="` + htmlEscape(a.runtimeCfg.Login.WebSocketTLS.Key) + `" size="52"></label><br>` +
		`</fieldset>` +
		`<fieldset><legend><strong>Content Servers</strong></legend>` +
		`<label>Public host <input name="content_host" value="` + htmlEscape(a.runtimeCfg.Content.Host) + `" size="28"></label><br>` +
		`<label>Gopher listen <input name="content_gopher_listen" value="` + htmlEscape(a.runtimeCfg.Content.GopherListen) + `" size="14"></label><br>` +
		`<label>NNTP listen <input name="content_nntp_listen" value="` + htmlEscape(a.runtimeCfg.Content.NNTPListen) + `" size="14"></label><br>` +
		`<label>NNTPS listen <input name="content_nntps_listen" value="` + htmlEscape(a.runtimeCfg.Content.NNTPSListen) + `" size="14"></label><br>` +
		`<label>NNTPS cert <input name="content_nntps_cert" value="` + htmlEscape(a.runtimeCfg.Content.NNTPSCert) + `" size="52"></label><br>` +
		`<label>NNTPS key <input name="content_nntps_key" value="` + htmlEscape(a.runtimeCfg.Content.NNTPSKey) + `" size="52"></label><br>` +
		`</fieldset>` +
		`<fieldset><legend><strong>Federation + Connectors</strong></legend>` +
		`<label><input type="checkbox" name="activitypub_enabled"` + checkedIf(a.runtimeCfg.ActivityPub.Enabled) + `> Enable ActivityPub bridge</label> ` +
		`<label>Base URL <input name="activitypub_base_url" value="` + htmlEscape(a.runtimeCfg.ActivityPub.BaseURL) + `" size="52"></label><br>` +
		`<label><input type="checkbox" name="connector_doorparty_enabled"` + checkedIf(a.runtimeCfg.Connectors.DoorParty.Enabled) + `> DoorParty connector</label> ` +
		`<label>Command <input name="connector_doorparty_command" value="` + htmlEscape(a.runtimeCfg.Connectors.DoorParty.Command) + `" size="34"></label> ` +
		`<label>Args <input name="connector_doorparty_args" value="` + htmlEscape(a.runtimeCfg.Connectors.DoorParty.Args) + `" size="34"></label><br>` +
		`<label><input type="checkbox" name="connector_bbslink_enabled"` + checkedIf(a.runtimeCfg.Connectors.BBSLink.Enabled) + `> BBSLink connector</label> ` +
		`<label>Command <input name="connector_bbslink_command" value="` + htmlEscape(a.runtimeCfg.Connectors.BBSLink.Command) + `" size="34"></label> ` +
		`<label>Args <input name="connector_bbslink_args" value="` + htmlEscape(a.runtimeCfg.Connectors.BBSLink.Args) + `" size="34"></label><br>` +
		`<label><input type="checkbox" name="connector_telnet_enabled"` + checkedIf(a.runtimeCfg.Connectors.Telnet.Enabled) + `> Telnet bridge connector</label> ` +
		`<label>Command <input name="connector_telnet_command" value="` + htmlEscape(a.runtimeCfg.Connectors.Telnet.Command) + `" size="34"></label> ` +
		`<label>Args <input name="connector_telnet_args" value="` + htmlEscape(a.runtimeCfg.Connectors.Telnet.Args) + `" size="34"></label><br>` +
		`</fieldset>` +
		`<p><small>Runtime service values are persisted in system settings and loaded on startup. Restart BBS/Web services after transport/listener changes.</small></p>` +
		`<button type="submit">Save Runtime Services</button></form>` +
		`<h2>Advanced: ANSI Menu Runtime</h2>` +
		noticeBlock +
		errBlock +
		`<form method="POST">` + csrf +
		`<input type="hidden" name="action" value="save_menu_settings">` +
		`<label><input type="checkbox" name="menu_enabled"` + checkedIf(a.runtimeCfg.Menu.Enabled) + `> Enable HJSON menu runtime in SSH sessions</label><br>` +
		`<label>Menu file <input name="menu_file" value="` + htmlEscape(menuFile) + `" size="64"></label> ` +
		`<button type="submit">Save Menu Runtime</button></form>` +
		`<p>Available menu files under <code>` + htmlEscape(a.menuRootPath()) + `</code>:</p><ul>` + menuRows.String() + `</ul>` +
		`<form method="POST" data-draft-key="menu-editor-` + htmlEscape(menuFile) + `">` + csrf +
		`<label>Editing file <input name="menu_file" value="` + htmlEscape(menuFile) + `" size="64"></label><br>` +
		`<textarea name="menu_body" rows="26" cols="120">` + htmlEscape(menuBody) + `</textarea><br>` +
		`<button type="submit" name="action" value="validate_menu">Validate Menu</button> ` +
		`<button type="submit" name="action" value="save_menu">Save Menu File</button></form>` +
		`<p>Menu edits are validated via HJSON schema checks before save. Reconnect SSH sessions after saving to load changes.</p>` +
		`<h2>Config Directory</h2><ul>` +
		`<li><a href="/admin/setup">Setup Wizard</a> (identity + first-run baseline)</li>` +
		`<li><a href="/admin/users">Users</a> (roles, verification, bans)</li>` +
		`<li><a href="/admin/boards">Boards</a> (permissions and moderation)</li>` +
		`<li><a href="/admin/mail">Mail</a> and <a href="/admin/gateways">Gateways</a> (email/web gateway policies)</li>` +
		`<li><a href="/admin/files">Files</a>, <a href="/admin/chat">Chat</a>, <a href="/admin/doors">Doors</a></li>` +
		`<li><a href="/admin/system">System Dashboard</a> and <a href="/admin/errors">Errors</a></li>` +
		`</ul></body></html>`
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(page))
}
