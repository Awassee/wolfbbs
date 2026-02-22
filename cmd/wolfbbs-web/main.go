package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/gateway"
	"wolfbbs/internal/repository"
)

type boardRow struct {
	ID      int
	Title   string
	Topics  int
	LastAt  string
	LastSub string
}

type mailRow struct {
	ID     int
	From   string
	To     string
	Subject string
	SentAt string
	Read   bool
}

type sessionState struct {
	handle string
	expire time.Time
}

type adminLogEntry struct {
	Time    time.Time
	Actor   string
	Target  string
	Action  string
	Details string
}

type webApp struct {
	authSvc  *auth.Service
	sessions map[string]sessionState
	sync.Mutex
	boards   []boardRow
	chatSvc  *chat.Service
	offlineDir string
	readOnly bool
	adminLog  []adminLogEntry
	mailLimits map[string]bool
}

var seedUsers = []struct {
	handle string
	pass   string
	role   string
}{
	{"admin", "wolfbbs-admin", "admin"},
	{"guest", "wolfbbs", "user"},
}

func seedWebUsers(authSvc *auth.Service) {
	for _, seed := range seedUsers {
		existing, err := authSvc.GetUser(seed.handle)
		if err == nil && existing != nil {
			if existing.Role != seed.role {
				_ = authSvc.SetRole(seed.handle, seed.role)
			}
			continue
		}
		u, err := authSvc.Register(seed.handle, seed.pass)
		if err != nil {
			continue
		}
		u.Role = seed.role
		_ = authSvc.SetRole(seed.handle, seed.role)
	}
}

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	dbURL := flag.String("db", "", "PostgreSQL DSN (defaults to WOLFBBS_DATABASE_URL / DATABASE_URL / PG* env)")
	flag.Parse()
	if *dbURL == "" {
		*dbURL = repository.ResolveDatabaseURL()
	}

	storage, err := repository.OpenStorageFromEnv(*dbURL)
	if err != nil {
		log.Fatalf("repository init: %v", err)
	}
	defer storage.Close()
	repo := storage.Users
	authSvc := auth.NewService(repo)
	seedWebUsers(authSvc)

	app := &webApp{
		authSvc: authSvc,
		sessions: map[string]sessionState{},
		chatSvc: chat.NewService(),
		offlineDir: func() string {
			dir := strings.TrimSpace(os.Getenv("WOLFBBS_OFFLINE_DIR"))
			if dir == "" {
				dir = ".wolfbbs/offline"
			}
			return dir
		}(),
		readOnly: strings.EqualFold(strings.TrimSpace(os.Getenv("WOLFBBS_READ_ONLY")), "1") || strings.EqualFold(strings.TrimSpace(os.Getenv("WOLFBBS_READ_ONLY")), "true"),
		boards: []boardRow{
			{ID: 1, Title: "General", Topics: 21, LastAt: time.Now().Add(-90 * time.Minute).Format("15:04"), LastSub: "Welcome to WolfBBS"},
			{ID: 2, Title: "Node Talk", Topics: 12, LastAt: time.Now().Add(-22 * time.Minute).Format("15:04"), LastSub: "ANSI renderer update"},
			{ID: 3, Title: "Tooling", Topics: 9, LastAt: time.Now().Add(-4 * time.Hour).Format("15:04"), LastSub: "Gateway limits"},
		},
		mailLimits: map[string]bool{},
	}
	_, _ = app.chatSvc.Post("system", "#lobby", "Welcome to #lobby")

	http.HandleFunc("/", app.handleRoot)
	http.HandleFunc("/login", app.handleLogin)
	http.HandleFunc("/logout", app.handleLogout)
	http.HandleFunc("/boards", app.authRequired(app.handleBoards))
	http.HandleFunc("/mail", app.authRequired(app.handleMail))
	http.HandleFunc("/settings", app.authRequired(app.handleSettings))
	http.HandleFunc("/admin", app.roleRequired("admin", app.handleAdmin))
	http.HandleFunc("/admin/users", app.roleRequired("admin", app.handleAdminUsers))
	http.HandleFunc("/admin/boards", app.roleRequired("admin", app.handleAdminBoards))
	http.HandleFunc("/admin/mail", app.roleRequired("admin", app.handleAdminMail))
	http.HandleFunc("/admin/files", app.roleRequired("admin", app.handleAdminFiles))
	http.HandleFunc("/admin/gateways", app.roleRequired("admin", app.handleAdminGateways))
	http.HandleFunc("/admin/chat", app.roleRequired("admin", app.handleAdminChat))
	http.HandleFunc("/admin/system", app.roleRequired("admin", app.handleAdminSystem))
	http.HandleFunc("/admin/audit", app.roleRequired("admin", app.handleAdminAudit))
	http.HandleFunc("/chat", app.authRequired(app.handleChat))
	http.HandleFunc("/chat/send", app.authRequired(app.handleChatSend))
	http.HandleFunc("/chat/stream", app.authRequired(app.handleChatStream))
	http.HandleFunc("/gateway", app.authRequired(app.handleGateway))

	fmt.Printf("WolfBBS web companion on %s\n", *listen)
	log.Fatal(http.ListenAndServe(*listen, nil))
}

func (a *webApp) handleRoot(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.currentUser(r); ok {
		http.Redirect(w, r, "/boards", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (a *webApp) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(loginPage()))
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	handle := strings.TrimSpace(r.FormValue("handle"))
	password := strings.TrimSpace(r.FormValue("password"))
	totp := strings.TrimSpace(r.FormValue("totp"))

	user, err := a.authSvc.Authenticate(handle, password, totp)
	if err != nil {
		if err == auth.ErrMissingSecondFactor || err == auth.ErrInvalidSecondFactor {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("invalid 2FA code"))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid credentials"))
		return
	}

	if sid, ok := a.createSession(user.Handle); ok {
		http.SetCookie(w, &http.Cookie{
			Name:     "wolfbbs_session",
			Value:    sid,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			Expires:  time.Now().Add(2 * time.Hour),
		})
	}
	http.Redirect(w, r, "/boards", http.StatusFound)
}

func (a *webApp) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("wolfbbs_session"); err == nil {
		a.Lock()
		delete(a.sessions, c.Value)
		a.Unlock()
	}
	deleteCookie(w, "wolfbbs_session")
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (a *webApp) handleBoards(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	rows := strings.Builder{}
	for _, b := range a.boards {
		rows.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td>%d</td><td>%s</td><td>%s</td></tr>`, b.ID, b.Title, b.Topics, b.LastAt, b.LastSub))
	}
	page := fmt.Sprintf(`<html><body>
	<p>Signed in as %s</p>
	<p><a href="/mail">mail</a> | <a href="/settings">settings</a> | <a href="/chat">chat</a> | <a href="/gateway">gateway</a> | <a href="/logout">logout</a></p>
	<h1>Message Boards (read-only)</h1>
	<table border="1">
	<tr><th>ID</th><th>Board</th><th>Topics</th><th>Last</th><th>Last subject</th></tr>%s</table>
	</body></html>`, user.Handle, rows.String())
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleMail(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	inbox := seedMailRows(user.Handle)
	rows := strings.Builder{}
	for _, m := range inbox {
		status := "unread"
		if m.Read {
			status = "read"
		}
		rows.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`, m.ID, m.From, m.To, m.Subject, m.SentAt, status))
	}
	page := `<html><body><h1>Private Mail (read-only)</h1><table border="1"><tr><th>ID</th><th>From</th><th>To</th><th>Subject</th><th>Sent</th><th>Status</th></tr>` + rows.String() + `</table></body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleGateway(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		page := `<html><body>
	<h1>Gateway</h1>
	<p>Fetch readable text via text gateway (safety limits and SSRF blocks apply).</p>
	<form method="POST" action="/gateway">
		<label>URL: <input name="url" size="60" value="https://"></label><br><br>
		<label><input type="checkbox" name="save" value="1"> Save for offline reading</label><br><br>
		<button type="submit">Fetch</button>
	</form>
	</body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page))
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	url := strings.TrimSpace(r.FormValue("url"))
	if url == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("missing url"))
		return
	}
	result, err := gateway.FetchText(r.Context(), url, gateway.DefaultFetchConfig)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("web gateway fetch failed: " + err.Error()))
		return
	}
	note := ""
	if strings.TrimSpace(r.FormValue("save")) == "1" {
		user, _ := a.currentUser(r)
		path, err := gateway.SaveOffline(a.offlineDir, user.Handle, url, result)
		if err != nil {
			note = "\n\nCould not save offline copy: " + err.Error()
		} else {
			note = "\n\nSaved to: " + path
		}
	}
	result = strings.TrimSpace(result + note)
	escaped := strings.ReplaceAll(result, "&", "&amp;")
	escaped = strings.ReplaceAll(escaped, "<", "&lt;")
	escaped = strings.ReplaceAll(escaped, ">", "&gt;")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<html><body><h1>Gateway Reader</h1><pre>` + escaped + `</pre></body></html>`))
}

func (a *webApp) handleSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

		if r.Method == http.MethodPost {
			action := strings.TrimSpace(strings.ToLower(r.FormValue("action")))
			switch action {
			case "change_password":
				next := strings.TrimSpace(r.FormValue("password"))
				confirm := strings.TrimSpace(r.FormValue("confirm"))
				if next == "" || confirm == "" {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte("missing password"))
					return
				}
				if next != confirm {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte("passwords do not match"))
					return
				}
				if err := a.authSvc.SetPassword(user.Handle, next); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte("password update failed"))
					return
				}
			case "enable_2fa":
				secret, err := auth.GenerateTOTPSecret()
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte("2FA setup failed"))
					return
				}
				codes, err := auth.GenerateRecoveryCodes(8)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte("2FA setup failed"))
					return
				}
				_ = a.authSvc.SetTOTPSecret(user.Handle, secret)
				_ = a.authSvc.SetRecoveryCodes(user.Handle, codes)
			case "disable_2fa":
				_ = a.authSvc.SetTOTPSecret(user.Handle, "")
				_ = a.authSvc.SetRecoveryCodes(user.Handle, nil)
			case "regen_codes":
				codes, err := auth.GenerateRecoveryCodes(8)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte("2FA setup failed"))
					return
				}
				_ = a.authSvc.SetRecoveryCodes(user.Handle, codes)
			default:
				http.Redirect(w, r, "/settings", http.StatusFound)
				return
			}
			http.Redirect(w, r, "/settings", http.StatusFound)
			return
		}

		var secondFactorBlock strings.Builder
		if user.TOTPSecret == "" {
			secondFactorBlock.WriteString(`<p>2FA is currently disabled.</p>`)
			secondFactorBlock.WriteString(`<form method="POST" action="/settings"><input type="hidden" name="action" value="enable_2fa"><button type="submit">Enable TOTP</button></form>`)
		} else {
			secondFactorBlock.WriteString(`<p>2FA is enabled.</p>`)
			secondFactorBlock.WriteString(`<form method="POST" action="/settings"><input type="hidden" name="action" value="disable_2fa"><button type="submit">Disable TOTP</button></form>`)
			secondFactorBlock.WriteString(`<form method="POST" action="/settings"><input type="hidden" name="action" value="regen_codes"><button type="submit">Regenerate recovery codes</button></form>`)
			secondFactorBlock.WriteString(`<p>Recovery Codes: ` + strings.Join(user.RecoveryCodes, ", ") + `</p>`)
		}
		page := `<html><body><h1>Settings</h1><p>User: ` + user.Handle + `</p><ul>` +
			`<li>ANSI: ` + boolToText(user.ANSIEnabled) + `</li>` +
			`<li>Paging: ` + boolToText(user.PagingEnabled) + `</li>` +
			`<li>Time format 24h: ` + boolToText(user.TimeFormat24h) + `</li>` +
			`</ul>` +
			`<h2>Password</h2><form method="POST" action="/settings"><input type="hidden" name="action" value="change_password">` +
			`<label>New password: <input name="password" type="password"></label><br>` +
			`<label>Confirm: <input name="confirm" type="password"></label><br><button type="submit">Change password</button></form>` +
			secondFactorBlock.String() +
			`</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdmin(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	page := `<html><body><h1>Admin</h1><p>Logged in as ` + user.Handle + `</p>` +
		`<p><a href="/admin/users">Users</a> | <a href="/admin/boards">Boards</a> | <a href="/admin/mail">Mail</a> | ` +
		`<a href="/admin/files">Files</a> | <a href="/admin/gateways">Gateways</a> | <a href="/admin/chat">Chat</a> | ` +
		`<a href="/admin/system">System</a> | <a href="/admin/audit">Audit Log</a></p>` +
		`<ul><li>Users: list/search, disable, ban, reset passwords</li>` +
		`<li>Boards: create, edit, delete, permissions</li>` +
		`<li>Mail: audit, limit controls</li>` +
		`<li>Chat: channel state, kicks, mutes</li>` +
		`<li>Gateway controls and server health</li></ul>` +
		`<p>Read-only mode: ` + boolToText(a.readOnly) + `</p></body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if user.Role != "admin" {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
		return
	}

	if r.Method == http.MethodPost {
		if a.readOnly {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("read-only mode"))
			return
		}
		target := strings.TrimSpace(r.FormValue("handle"))
		action := strings.TrimSpace(strings.ToLower(r.FormValue("action")))
		switch action {
		case "disable":
			_ = a.authSvc.SetEnabled(target, false)
			a.recordAdminAction(user.Handle, target, "disable_user", "disabled account")
		case "enable":
			_ = a.authSvc.SetEnabled(target, true)
			a.recordAdminAction(user.Handle, target, "enable_user", "enabled account")
		case "ban":
			_ = a.authSvc.SetBanned(target, true)
			a.recordAdminAction(user.Handle, target, "ban_user", "banned account")
		case "unban":
			_ = a.authSvc.SetBanned(target, false)
			a.recordAdminAction(user.Handle, target, "unban_user", "removed ban")
		case "set_role":
			role := strings.TrimSpace(r.FormValue("role"))
			if role == "" {
				role = "user"
			}
			_ = a.authSvc.SetRole(target, role)
			a.recordAdminAction(user.Handle, target, "set_role", role)
		case "reset":
			pw := randomPassword(10)
			_ = a.authSvc.SetPassword(target, pw)
			a.recordAdminAction(user.Handle, target, "reset_password", "")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("reset password for " + target + " to " + pw))
			return
		}
		http.Redirect(w, r, "/admin/users", http.StatusFound)
		return
	}

	users, err := a.authSvc.ListUsers()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to load users"))
		return
	}

	filter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	rows := strings.Builder{}
	for _, u := range users {
		if filter != "" && !strings.Contains(strings.ToLower(u.Handle), filter) {
			continue
		}
		status := "active"
		if !u.Enabled {
			status = "disabled"
		}
		if u.Banned {
			status = "banned"
		}
		rows.WriteString(`<tr><td>` + u.Handle + `</td><td>` + status + `</td><td>` + u.Role + `</td><td>`)
		rows.WriteString(fmt.Sprintf(`<form method="POST" action="/admin/users">
			<input type="hidden" name="handle" value="%s">
			<input type="hidden" name="action" value="enable"><button type="submit">Enable</button></form>`, u.Handle))
		rows.WriteString(fmt.Sprintf(`<form method="POST" action="/admin/users">
			<input type="hidden" name="handle" value="%s">
			<input type="hidden" name="action" value="disable"><button type="submit">Disable</button></form>`, u.Handle))
		rows.WriteString(fmt.Sprintf(`<form method="POST" action="/admin/users">
			<input type="hidden" name="handle" value="%s">
			<input type="hidden" name="action" value="ban"><button type="submit">Ban</button></form>`, u.Handle))
		rows.WriteString(fmt.Sprintf(`<form method="POST" action="/admin/users">
			<input type="hidden" name="handle" value="%s">
			<input type="hidden" name="action" value="unban"><button type="submit">Unban</button></form>`, u.Handle))
		rows.WriteString(fmt.Sprintf(`<form method="POST" action="/admin/users">
			<input type="hidden" name="handle" value="%s">
			<input type="hidden" name="action" value="reset"><button type="submit">Reset Password</button></form>`, u.Handle))
		rows.WriteString(fmt.Sprintf(`<form method="POST" action="/admin/users">
			<input type="hidden" name="handle" value="%s">
			<input type="hidden" name="action" value="set_role">
			<select name="role">
				<option value="user"%s>User</option>
				<option value="moderator"%s>Moderator</option>
				<option value="admin"%s>Admin</option>
			</select><button type="submit">Set role</button></form>`, u.Handle, selectedIf(u.Role == "user"), selectedIf(u.Role == "moderator"), selectedIf(u.Role == "admin")))
		rows.WriteString(`</td></tr>`)
	}

	page := `<html><body><h1>Users</h1><p><a href="/admin">back</a></p>` +
		`<form method="GET"><label>Search: <input name="q" value="` + filter + `"></label><button type="submit">filter</button></form>` +
		`<table border="1"><tr><th>Handle</th><th>Status</th><th>Role</th><th>Actions</th></tr>` + rows.String() + `</table>` +
		`</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminBoards(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method == http.MethodPost {
		if a.readOnly {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("read-only mode"))
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		switch action {
		case "create":
			title := strings.TrimSpace(r.FormValue("title"))
			if title == "" {
				http.Redirect(w, r, "/admin/boards", http.StatusFound)
				return
			}
			newID := 1
			if len(a.boards) > 0 {
				newID = a.boards[len(a.boards)-1].ID + 1
			}
			a.Lock()
			a.boards = append(a.boards, boardRow{ID: newID, Title: title})
			a.Unlock()
			a.recordAdminAction(user.Handle, title, "create_board", "")
		case "delete":
			id := strings.TrimSpace(r.FormValue("id"))
			var deleted *boardRow
			a.Lock()
			for i, b := range a.boards {
				if fmt.Sprintf("%d", b.ID) == id {
					a.boards = append(a.boards[:i], a.boards[i+1:]...)
					deleted = &boardRow{ID: b.ID, Title: b.Title, Topics: b.Topics, LastAt: b.LastAt, LastSub: b.LastSub}
					break
				}
			}
			a.Unlock()
			if deleted != nil {
				a.recordAdminAction(user.Handle, deleted.Title, "delete_board", "")
			}
		}
		http.Redirect(w, r, "/admin/boards", http.StatusFound)
		return
	}

	rows := strings.Builder{}
	for _, b := range a.boards {
		rows.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td>%d</td><td>%s</td>`, b.ID, b.Title, b.Topics, b.LastAt))
		rows.WriteString(fmt.Sprintf(`<td><form method="POST" action="/admin/boards"><input type="hidden" name="action" value="delete"><input type="hidden" name="id" value="%d"><button type="submit">delete</button></form></td>`, b.ID))
		rows.WriteString(`</tr>`)
	}
	page := `<html><body><h1>Boards</h1><p><a href="/admin">back</a></p>` +
		`<form method="POST"><label>Title <input name="title"></label><input type="hidden" name="action" value="create"><button type="submit">add</button></form>` +
		`<table border="1"><tr><th>ID</th><th>Title</th><th>Topics</th><th>Last</th><th>Actions</th></tr>` + rows.String() + `</table>` +
		`</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminMail(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method == http.MethodPost {
		if a.readOnly {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("read-only mode"))
			return
		}
		target := strings.TrimSpace(r.FormValue("handle"))
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		if target != "" {
			disabled := action == "disable_outbound"
			a.Lock()
			a.mailLimits[target] = disabled
			a.Unlock()
			detail := "enabled outbound"
			if disabled {
				detail = "disabled outbound"
			}
			a.recordAdminAction(user.Handle, target, action, detail)
		}
		http.Redirect(w, r, "/admin/mail", http.StatusFound)
		return
	}
	rows := strings.Builder{}
	for handle, blocked := range a.mailLimits {
		rows.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%t</td>`, handle, blocked))
		rows.WriteString(`<td><form method="POST" action="/admin/mail">`)
		rows.WriteString(`<input type="hidden" name="handle" value="` + handle + `">`)
		if blocked {
			rows.WriteString(`<input type="hidden" name="action" value="enable_outbound"><button type="submit">enable</button>`)
		} else {
			rows.WriteString(`<input type="hidden" name="action" value="disable_outbound"><button type="submit">disable</button>`)
		}
		rows.WriteString(`</form></td></tr>`)
	}
	if rows.Len() == 0 {
		rows.WriteString(`<tr><td colspan="3">No per-user outbound limits configured</td></tr>`)
	}
	page := `<html><body><h1>Mail Controls</h1><p><a href="/admin">back</a></p>` +
		`<table border="1"><tr><th>Handle</th><th>OutboundDisabled</th><th>Action</th></tr>` + rows.String() + `</table></body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminFiles(w http.ResponseWriter, r *http.Request) {
	page := `<html><body><h1>Files</h1><p><a href="/admin">back</a></p>` +
		`<ul><li>File area metadata UI placeholder</li><li>Integrate storage scanner</li><li>Delete and audit controls</li></ul></body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminGateways(w http.ResponseWriter, r *http.Request) {
	page := `<html><body><h1>Gateway Controls</h1><p><a href="/admin">back</a></p>` +
		`<ul><li>Email relay env: SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASS, FROM_DOMAIN</li>` +
		`<li>Web gateway timeout: 10s, max body: 2MiB, SSRF denylist</li></ul>` +
		`<p>Use docs/web-gateway.md and docs/email-gateway.md for full policy.</p></body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminChat(w http.ResponseWriter, r *http.Request) {
	channelRows := strings.Builder{}
	for _, c := range a.chatSvc.ListChannels() {
		channelRows.WriteString(`<tr><td>` + c + `</td></tr>`)
	}
	if channelRows.Len() == 0 {
		channelRows.WriteString(`<tr><td>#lobby</td></tr>`)
	}
	page := `<html><body><h1>Chat Admin</h1><p><a href="/admin">back</a></p>` +
		`<table border="1"><tr><th>Channel</th></tr>` + channelRows.String() + `</table></body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminSystem(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`<html><body><h1>System</h1><p><a href="/admin">back</a></p><ul>` +
		`<li>Read-only mode: ` + boolToText(a.readOnly) + `</li>` +
		`<li>Uptime: simulated in MVP</li>` +
		`<li>Health: service routes active</li>` +
		`</ul></body></html>`))
}

func (a *webApp) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	rows := strings.Builder{}
	for _, entry := range a.adminLog {
		rows.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			entry.Time.Format("2006-01-02 15:04:05"), entry.Actor, entry.Target, entry.Action, entry.Details))
	}
	if rows.Len() == 0 {
		rows.WriteString(`<tr><td colspan="5">No admin actions yet</td></tr>`)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<html><body><h1>Admin Audit Log</h1><p><a href="/admin">back</a></p><table border="1"><tr><th>Time</th><th>Actor</th><th>Target</th><th>Action</th><th>Details</th></tr>` + rows.String() + `</table></body></html>`))
}

func boolToText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func selectedIf(active bool) string {
	if active {
		return " selected"
	}
	return ""
}

func (a *webApp) recordAdminAction(actor, target, action, details string) {
	a.Lock()
	a.adminLog = append(a.adminLog, adminLogEntry{
		Time:    time.Now(),
		Actor:   actor,
		Target:  target,
		Action:  action,
		Details: details,
	})
	if len(a.adminLog) > 200 {
		a.adminLog = a.adminLog[len(a.adminLog)-200:]
	}
	a.Unlock()
}

func randomPassword(length int) string {
	const chars = "ABCDEFGHJKMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789!@#$%"
	if length <= 0 {
		length = 12
	}
	b := make([]byte, length)
	n, err := rand.Read(b)
	if err != nil || n != length {
		for i := range b {
			b[i] = chars[i%len(chars)]
		}
	} else {
		for i := range b {
			b[i] = chars[int(b[i])%len(chars)]
		}
	}
	return string(b)
}

func (a *webApp) handleChat(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	chatPage := `<html><body>
	<h1>WolfBBS Chat</h1>
	<p>Logged in as ` + user.Handle + `</p>
	<form method="POST" action="/chat/send">
		<select name="channel"><option value="#lobby">#lobby</option></select>
		<input name="message" size="50">
		<button type="submit">Send</button>
	</form>
	<pre id="chat"></pre>
	<script>
	fetch('/chat/stream?channel=%23lobby').then(r=>r.text()).then(t=>{document.getElementById('chat').textContent=t});
	</script>
	</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(chatPage))
}

func (a *webApp) handleChatSend(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	channel := strings.TrimSpace(r.FormValue("channel"))
	message := strings.TrimSpace(r.FormValue("message"))
	if channel == "" {
		channel = "#lobby"
	}
	if message != "" {
		if _, err := a.chatSvc.Post(user.Handle, channel, message); err != nil {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("rate limited"))
			return
		}
	}
	http.Redirect(w, r, "/chat", http.StatusFound)
}

func (a *webApp) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.currentUser(r); !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	channel := strings.TrimSpace(r.URL.Query().Get("channel"))
	if channel == "" {
		channel = "#lobby"
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("stream unsupported"))
		return
	}
	h := a.chatSvc.History(channel, 32)
	for _, msg := range h {
		t := fmt.Sprintf("%s %s: %s", msg.CreatedAt.Format("15:04:05"), msg.From, msg.Body)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", t)
	}
	flusher.Flush()
}

func (a *webApp) authRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.currentUser(r); !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

func (a *webApp) roleRequired(role string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.currentUser(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if u.Role != role {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("forbidden"))
			return
		}
		next(w, r)
	}
}

func (a *webApp) currentUser(r *http.Request) (*domain.User, bool) {
	c, err := r.Cookie("wolfbbs_session")
	if err != nil {
		return nil, false
	}
	a.Lock()
	state, ok := a.sessions[c.Value]
	a.Unlock()
	if !ok || time.Now().After(state.expire) {
		return nil, false
	}
	u, err := a.authSvc.GetUser(state.handle)
	if err != nil {
		return nil, false
	}
	return u, true
}

func (a *webApp) createSession(handle string) (string, bool) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", false
	}
	sid := hex.EncodeToString(b)
	a.Lock()
	a.sessions[sid] = sessionState{handle: handle, expire: time.Now().Add(2 * time.Hour)}
	a.Unlock()
	return sid, true
}

func loginPage() string {
	return `<html><body>
	<h1>WolfBBS Web Login</h1>
	<form method="POST" action="/login">
		<label>Handle: <input name="handle"></label><br>
		<label>Password: <input name="password" type="password"></label><br>
		<label>2FA code: <input name="totp"></label><br>
		<button type="submit">Sign In</button>
	</form>
	</body></html>`
}

func seedMailRows(handle string) []mailRow {
	return []mailRow{
		{ID: 1, From: "sysop", To: handle, Subject: "Welcome", SentAt: time.Now().Add(-time.Hour).Format("2006-01-02 15:04"), Read: true},
		{ID: 2, From: handle, To: "ops", Subject: "Board status update", SentAt: time.Now().Add(-10 * time.Minute).Format("2006-01-02 15:04")},
	}
}

func deleteCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
	})
}
