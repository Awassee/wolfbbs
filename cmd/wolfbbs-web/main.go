package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"os"
	"strconv"
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
	ID      int
	From    string
	To      string
	Subject string
	SentAt  string
	Read    bool
}

type chatHistoryResponse struct {
	Channel  string          `json:"channel"`
	Messages []chat.Message  `json:"messages"`
	Online   []chat.Presence `json:"online,omitempty"`
	LastID   int64           `json:"last_id"`
}

type chatModerationPayload struct {
	Channel  string `json:"channel"`
	Action   string `json:"action"`
	Target   string `json:"target"`
	Reason   string `json:"reason"`
	Duration string `json:"duration"`
}

type sessionState struct {
	handle string
	expire time.Time
	csrf   string
}

const (
	roleUser      = "user"
	roleModerator = "moderator"
	roleAdmin     = "admin"
)

var roleWeight = map[string]int{
	roleUser:      1,
	roleModerator: 2,
	roleAdmin:     3,
}

type adminLogEntry struct {
	Time    time.Time
	Actor   string
	Target  string
	Action  string
	Details string
}

type webApp struct {
	authSvc   *auth.Service
	userRepo  repository.UserRepository
	boardRepo repository.BoardRepository
	msgRepo   repository.MessageRepository
	mailRepo  repository.PrivateMailRepository
	adminRepo repository.AdminRepository
	email     *gateway.EmailGateway
	sessions  map[string]sessionState
	sync.Mutex
	chatSvc      *chat.Service
	offlineDir   string
	readOnly     bool
	inboundToken string
	inboundAllow map[string]struct{}
}

var seedUsers = []struct {
	handle string
	pass   string
	role   string
}{
	{"admin", "wolfbbs-admin", "admin"},
	{"guest", "wolfbbs", "user"},
	{"mailbot", "wolfbbs-mailbot", "user"},
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
	authSvc := auth.NewService(storage.Users)
	seedWebUsers(authSvc)
	seedDefaultBoards(storage.Boards)

	app := &webApp{
		authSvc:   authSvc,
		userRepo:  storage.Users,
		boardRepo: storage.Boards,
		msgRepo:   storage.Messages,
		mailRepo:  storage.Mail,
		adminRepo: storage.Admin,
		email:     gateway.NewEmailGateway(gateway.LoadEmailConfigFromEnv()),
		sessions:  map[string]sessionState{},
		chatSvc:   chat.NewService(),
		offlineDir: func() string {
			dir := strings.TrimSpace(os.Getenv("WOLFBBS_OFFLINE_DIR"))
			if dir == "" {
				dir = ".wolfbbs/offline"
			}
			return dir
		}(),
		readOnly:     strings.EqualFold(strings.TrimSpace(os.Getenv("WOLFBBS_READ_ONLY")), "1") || strings.EqualFold(strings.TrimSpace(os.Getenv("WOLFBBS_READ_ONLY")), "true"),
		inboundToken: strings.TrimSpace(os.Getenv("WOLFBBS_INBOUND_TOKEN")),
		inboundAllow: parseAllowDomains(strings.TrimSpace(os.Getenv("WOLFBBS_MAILIN_ALLOW_DOMAINS"))),
	}

	http.HandleFunc("/", app.handleRoot)
	http.HandleFunc("/login", app.handleLogin)
	http.HandleFunc("/admin/login", app.handleLogin)
	http.HandleFunc("/logout", app.handleLogout)
	http.Handle("/boards", app.authRequired(http.HandlerFunc(app.handleBoards)))
	http.Handle("/mail", app.authRequired(http.HandlerFunc(app.handleMail)))
	http.Handle("/settings", app.authRequired(http.HandlerFunc(app.handleSettings)))
	http.Handle("/admin", app.mustBeRole(roleAdmin, app.handleAdmin))
	http.Handle("/admin/users", app.mustBeRole(roleAdmin, app.handleAdminUsers))
	http.Handle("/admin/boards", app.mustBeRole(roleAdmin, app.handleAdminBoards))
	http.Handle("/admin/mail", app.mustBeRole(roleAdmin, app.handleAdminMail))
	http.Handle("/admin/files", app.mustBeRole(roleAdmin, app.handleAdminFiles))
	http.Handle("/admin/gateways", app.mustBeRole(roleAdmin, app.handleAdminGateways))
	http.Handle("/admin/chat", app.mustBeRole(roleAdmin, app.handleAdminChat))
	http.Handle("/admin/system", app.mustBeRole(roleAdmin, app.handleAdminSystem))
	http.Handle("/admin/audit", app.mustBeRole(roleAdmin, app.handleAdminAudit))
	http.Handle("/chat", app.authRequired(http.HandlerFunc(app.handleChat)))
	http.Handle("/chat/send", app.authRequired(http.HandlerFunc(app.handleChatSend)))
	http.Handle("/chat/stream", app.authRequired(http.HandlerFunc(app.handleChatStream)))
	http.Handle("/chat/channels", app.authRequired(http.HandlerFunc(app.handleChatChannels)))
	http.Handle("/chat/join", app.authRequired(http.HandlerFunc(app.handleChatJoin)))
	http.Handle("/chat/leave", app.authRequired(http.HandlerFunc(app.handleChatLeave)))
	http.Handle("/chat/history", app.authRequired(http.HandlerFunc(app.handleChatHistory)))
	http.Handle("/chat/online", app.authRequired(http.HandlerFunc(app.handleChatOnline)))
	http.Handle("/chat/moderation", app.mustBeRole(roleModerator, http.HandlerFunc(app.handleChatModeration)))
	http.Handle("/gateway", app.authRequired(http.HandlerFunc(app.handleGateway)))
	http.HandleFunc("/mail/inbound", app.handleMailInbound)
	http.HandleFunc("/healthz", app.handleHealthz)

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

func (a *webApp) handleHealthz(w http.ResponseWriter, r *http.Request) {
	_ = r
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (a *webApp) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if user, ok := a.currentUser(r); ok {
			if user != nil && a.hasRole(user, roleAdmin) && strings.HasPrefix(r.URL.Path, "/admin") {
				http.Redirect(w, r, "/admin", http.StatusFound)
			} else {
				http.Redirect(w, r, "/boards", http.StatusFound)
			}
			return
		}
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
			Secure:   strings.EqualFold(strings.TrimSpace(os.Getenv("WOLFBBS_SECURE_COOKIE")), "true"),
			SameSite: http.SameSiteStrictMode,
			Expires:  time.Now().Add(2 * time.Hour),
		})
	}
	redirectTo := "/boards"
	if strings.HasPrefix(r.URL.Path, "/admin") && a.hasRole(user, roleAdmin) {
		redirectTo = "/admin"
	}
	http.Redirect(w, r, redirectTo, http.StatusFound)
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
	if r.Method == http.MethodPost {
		if !a.requireCSRF(w, r) {
			return
		}
		boardID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("board_id")), 10, 64)
		parentID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("parent_id")), 10, 64)
		subject := strings.TrimSpace(r.FormValue("subject"))
		body := strings.TrimSpace(r.FormValue("body"))
		if boardID <= 0 || subject == "" || body == "" {
			http.Error(w, "board/subject/body required", http.StatusBadRequest)
			return
		}
		if err := a.msgRepo.CreateMessage(&domain.Message{
			BoardID:  boardID,
			AuthorID: user.ID,
			ParentID: parentID,
			Subject:  subject,
			Body:     body,
		}); err != nil {
			http.Error(w, "could not create message", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/boards?board=%d", boardID), http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	boardID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("board")), 10, 64)
	if boardID <= 0 {
		boards, err := a.boardRepo.List()
		if err != nil {
			http.Error(w, "failed to load boards", http.StatusInternalServerError)
			return
		}
		rows := strings.Builder{}
		for _, board := range boards {
			msgs, _ := a.msgRepo.ListByBoard(board.ID)
			lastAt := ""
			lastSub := ""
			if len(msgs) > 0 {
				last := msgs[len(msgs)-1]
				lastAt = last.CreatedAt.Format("2006-01-02 15:04")
				lastSub = last.Subject
			}
			rows.WriteString(fmt.Sprintf(`<tr><td>%d</td><td><a href="/boards?board=%d">%s</a></td><td>%d</td><td>%s</td><td>%s</td></tr>`,
				board.ID, board.ID, board.Name, len(msgs), lastAt, htmlEscape(lastSub)))
		}
		page := fmt.Sprintf(`<html><body>
<p>Signed in as %s</p>
<p><a href="/mail">mail</a> | <a href="/settings">settings</a> | <a href="/chat">chat</a> | <a href="/gateway">gateway</a> | <a href="/logout">logout</a></p>
<h1>Message Boards</h1>
<table border="1">
<tr><th>ID</th><th>Board</th><th>Topics</th><th>Last</th><th>Last subject</th></tr>%s</table>
</body></html>`, user.Handle, rows.String())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page))
		return
	}

	board, err := a.boardRepo.Get(boardID)
	if err != nil {
		http.Error(w, "board not found", http.StatusNotFound)
		return
	}
	msgs, err := a.msgRepo.ListByBoard(boardID)
	if err != nil {
		http.Error(w, "failed to load messages", http.StatusInternalServerError)
		return
	}
	handleByID := a.userHandleLookup()
	csrf := a.csrfHiddenInput(r)
	messageID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
	view := strings.Builder{}
	if messageID > 0 {
		msg, err := a.msgRepo.GetMessage(messageID)
		if err == nil && msg.BoardID == boardID {
			view.WriteString(`<h2>Reader</h2>`)
			view.WriteString(`<p><strong>Subject:</strong> ` + htmlEscape(msg.Subject) + `<br>`)
			view.WriteString(`<strong>From:</strong> ` + htmlEscape(handleByID[msg.AuthorID]) + `<br>`)
			if msg.ParentID > 0 {
				view.WriteString(`<strong>Reply-To:</strong> #` + strconv.FormatInt(msg.ParentID, 10) + `<br>`)
			}
			view.WriteString(`<strong>When:</strong> ` + msg.CreatedAt.Format("2006-01-02 15:04:05") + `</p>`)
			view.WriteString(`<pre>` + htmlEscape(msg.Body) + `</pre>`)
			view.WriteString(`<h3>Reply</h3>`)
			view.WriteString(`<form method="POST" action="/boards"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `"><input type="hidden" name="parent_id" value="` + strconv.FormatInt(msg.ID, 10) + `">` + csrf)
			view.WriteString(`<label>Subject: <input name="subject" value="Re: ` + htmlEscape(msg.Subject) + `" size="60"></label><br>`)
			view.WriteString(`<label>Body:<br><textarea name="body" rows="10" cols="80">` + htmlEscape(quoteBody(msg.Body)) + `</textarea></label><br>`)
			view.WriteString(`<button type="submit">Post Reply</button></form>`)
		}
	}

	rows := strings.Builder{}
	for _, msg := range msgs {
		subject := msg.Subject
		if msg.ParentID > 0 {
			subject = "> " + subject
		}
		rows.WriteString(fmt.Sprintf(`<tr><td>%d</td><td><a href="/boards?board=%d&id=%d">%s</a></td><td>%s</td><td>%s</td></tr>`,
			msg.ID, boardID, msg.ID, htmlEscape(subject), htmlEscape(handleByID[msg.AuthorID]), msg.CreatedAt.Format("2006-01-02 15:04")))
	}
	page := `<html><body><h1>Board: ` + htmlEscape(board.Name) + `</h1>` +
		`<p><a href="/boards">all boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/logout">logout</a></p>` +
		`<table border="1"><tr><th>ID</th><th>Subject</th><th>Author</th><th>When</th></tr>` + rows.String() + `</table>` +
		`<h2>New Post</h2><form method="POST" action="/boards"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `">` + csrf +
		`<label>Subject: <input name="subject" size="60"></label><br><label>Body:<br><textarea name="body" rows="10" cols="80"></textarea></label><br><button type="submit">Post</button></form>` +
		view.String() + `</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleMail(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method == http.MethodPost {
		if !a.requireCSRF(w, r) {
			return
		}
		toRaw := strings.TrimSpace(r.FormValue("to"))
		subject := strings.TrimSpace(r.FormValue("subject"))
		body := strings.TrimSpace(r.FormValue("body"))
		if toRaw == "" || subject == "" || body == "" {
			http.Error(w, "to/subject/body required", http.StatusBadRequest)
			return
		}

		msg := &domain.PrivateMail{
			FromUserID: user.ID,
			Subject:    subject,
			Body:       body,
		}
		if strings.Contains(toRaw, "@") {
			if !user.Verified {
				http.Error(w, "verified account required for external email", http.StatusForbidden)
				return
			}
			if a.adminRepo != nil {
				policy, err := a.adminRepo.GetMailOutboundPolicy(user.Handle)
				if err == nil && policy != nil && policy.OutboundDisabled {
					http.Error(w, "outbound email disabled for this account", http.StatusForbidden)
					return
				}
			}
			recipient := toRaw
			msg.ExternalTo = &recipient
			if err := a.email.SendOutbound(user.Handle, []string{recipient}, subject, body); err != nil {
				http.Error(w, "email relay error: "+err.Error(), http.StatusBadRequest)
				return
			}
		} else {
			target, err := a.authSvc.GetUser(toRaw)
			if err != nil || target == nil {
				http.Error(w, "unknown recipient handle", http.StatusBadRequest)
				return
			}
			msg.ToUserID = target.ID
		}
		if err := a.mailRepo.CreateMail(msg); err != nil {
			http.Error(w, "could not save mail", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/mail", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if id := strings.TrimSpace(r.URL.Query().Get("id")); id != "" {
		mailID, _ := strconv.ParseInt(id, 10, 64)
		item, err := a.mailRepo.GetMail(mailID)
		if err != nil || item == nil {
			http.Error(w, "mail not found", http.StatusNotFound)
			return
		}
		if item.ToUserID != user.ID && item.FromUserID != user.ID {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if item.ToUserID == user.ID && item.ReadAt == nil {
			_ = a.mailRepo.MarkRead(item.ID, time.Now().UTC())
		}
		handleByID := a.userHandleLookup()
		to := ""
		if item.ExternalTo != nil {
			to = *item.ExternalTo
		} else {
			to = handleByID[item.ToUserID]
		}
		page := `<html><body><h1>Mail #` + strconv.FormatInt(item.ID, 10) + `</h1><p><a href="/mail">back</a></p>` +
			`<p><strong>From:</strong> ` + htmlEscape(handleByID[item.FromUserID]) + `<br>` +
			`<strong>To:</strong> ` + htmlEscape(to) + `<br>` +
			`<strong>Subject:</strong> ` + htmlEscape(item.Subject) + `<br>` +
			`<strong>Sent:</strong> ` + item.CreatedAt.Format("2006-01-02 15:04:05") + `</p>` +
			`<pre>` + htmlEscape(item.Body) + `</pre></body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page))
		return
	}

	inbox, _ := a.mailRepo.ListInbox(user.ID, 100)
	outbox, _ := a.mailRepo.ListOutbox(user.ID, 100)
	handleByID := a.userHandleLookup()
	inRows := strings.Builder{}
	for _, row := range inbox {
		status := "unread"
		if row.ReadAt != nil {
			status = "read"
		}
		inRows.WriteString(fmt.Sprintf(`<tr><td><a href="/mail?id=%d">%d</a></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			row.ID, row.ID, htmlEscape(handleByID[row.FromUserID]), htmlEscape(row.Subject), row.CreatedAt.Format("2006-01-02 15:04"), status))
	}
	outRows := strings.Builder{}
	for _, row := range outbox {
		target := handleByID[row.ToUserID]
		if row.ExternalTo != nil {
			target = *row.ExternalTo
		}
		outRows.WriteString(fmt.Sprintf(`<tr><td><a href="/mail?id=%d">%d</a></td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			row.ID, row.ID, htmlEscape(target), htmlEscape(row.Subject), row.CreatedAt.Format("2006-01-02 15:04")))
	}
	csrf := a.csrfHiddenInput(r)
	page := `<html><body><h1>Private Mail</h1><p><a href="/boards">boards</a> | <a href="/chat">chat</a> | <a href="/logout">logout</a></p>` +
		`<h2>Compose</h2><form method="POST" action="/mail">` + csrf +
		`<label>To (handle or email): <input name="to" size="40"></label><br>` +
		`<label>Subject: <input name="subject" size="60"></label><br>` +
		`<label>Body:<br><textarea name="body" rows="10" cols="80"></textarea></label><br><button type="submit">Send</button></form>` +
		`<h2>Inbox</h2><table border="1"><tr><th>ID</th><th>From</th><th>Subject</th><th>Sent</th><th>Status</th></tr>` + inRows.String() + `</table>` +
		`<h2>Outbox</h2><table border="1"><tr><th>ID</th><th>To</th><th>Subject</th><th>Sent</th></tr>` + outRows.String() + `</table>` +
		`</body></html>`
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

	if r.Method == http.MethodPost {
		if !a.requireAdminWrite(w, r) {
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
		case "verify":
			_ = a.authSvc.SetVerified(target, true)
			a.recordAdminAction(user.Handle, target, "verify_user", "")
		case "unverify":
			_ = a.authSvc.SetVerified(target, false)
			a.recordAdminAction(user.Handle, target, "unverify_user", "")
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
	csrf := a.csrfHiddenInput(r)
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
		verified := "no"
		if u.Verified {
			verified = "yes"
		}
		rows.WriteString(`<tr><td>` + u.Handle + `</td><td>` + status + `</td><td>` + u.Role + `</td><td>` + verified + `</td><td>`)
		rows.WriteString(fmt.Sprintf(`<form method="POST" action="/admin/users">
			<input type="hidden" name="handle" value="%s">
			%s
			<input type="hidden" name="action" value="enable"><button type="submit">Enable</button></form>`, u.Handle, csrf))
		rows.WriteString(fmt.Sprintf(`<form method="POST" action="/admin/users">
			<input type="hidden" name="handle" value="%s">
			%s
			<input type="hidden" name="action" value="disable"><button type="submit">Disable</button></form>`, u.Handle, csrf))
		rows.WriteString(fmt.Sprintf(`<form method="POST" action="/admin/users">
			<input type="hidden" name="handle" value="%s">
			%s
			<input type="hidden" name="action" value="ban"><button type="submit">Ban</button></form>`, u.Handle, csrf))
		rows.WriteString(fmt.Sprintf(`<form method="POST" action="/admin/users">
			<input type="hidden" name="handle" value="%s">
			%s
			<input type="hidden" name="action" value="unban"><button type="submit">Unban</button></form>`, u.Handle, csrf))
		rows.WriteString(fmt.Sprintf(`<form method="POST" action="/admin/users">
			<input type="hidden" name="handle" value="%s">
			%s
			<input type="hidden" name="action" value="reset"><button type="submit">Reset Password</button></form>`, u.Handle, csrf))
		rows.WriteString(fmt.Sprintf(`<form method="POST" action="/admin/users">
			<input type="hidden" name="handle" value="%s">
			%s
			<input type="hidden" name="action" value="verify"><button type="submit">Verify</button></form>`, u.Handle, csrf))
		rows.WriteString(fmt.Sprintf(`<form method="POST" action="/admin/users">
			<input type="hidden" name="handle" value="%s">
			%s
			<input type="hidden" name="action" value="unverify"><button type="submit">Unverify</button></form>`, u.Handle, csrf))
		rows.WriteString(fmt.Sprintf(`<form method="POST" action="/admin/users">
			<input type="hidden" name="handle" value="%s">
			%s
			<input type="hidden" name="action" value="set_role">
			<select name="role">
				<option value="user"%s>User</option>
				<option value="moderator"%s>Moderator</option>
				<option value="admin"%s>Admin</option>
			</select><button type="submit">Set role</button></form>`, u.Handle, csrf, selectedIf(u.Role == "user"), selectedIf(u.Role == "moderator"), selectedIf(u.Role == "admin")))
		rows.WriteString(`</td></tr>`)
	}

	page := `<html><body><h1>Users</h1><p><a href="/admin">back</a></p>` +
		`<form method="GET"><label>Search: <input name="q" value="` + filter + `"></label><button type="submit">filter</button></form>` +
		`<table border="1"><tr><th>Handle</th><th>Status</th><th>Role</th><th>Verified</th><th>Actions</th></tr>` + rows.String() + `</table>` +
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
	if a.boardRepo == nil {
		http.Error(w, "board repository unavailable", http.StatusInternalServerError)
		return
	}
	if r.Method == http.MethodPost {
		if !a.requireAdminWrite(w, r) {
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
			desc := strings.TrimSpace(r.FormValue("description"))
			_ = a.boardRepo.Create(&domain.Board{Name: title, Description: desc, CreatedBy: user.ID})
			a.recordAdminAction(user.Handle, title, "create_board", "")
		case "delete":
			id := strings.TrimSpace(r.FormValue("id"))
			if boardID, err := strconv.ParseInt(id, 10, 64); err == nil {
				board, _ := a.boardRepo.Get(boardID)
				if err := a.boardRepo.Delete(boardID); err == nil {
					target := id
					if board != nil {
						target = board.Name
					}
					a.recordAdminAction(user.Handle, target, "delete_board", "")
				}
			}
		}
		http.Redirect(w, r, "/admin/boards", http.StatusFound)
		return
	}

	boards, err := a.boardRepo.List()
	if err != nil {
		http.Error(w, "failed to load boards", http.StatusInternalServerError)
		return
	}
	rows := strings.Builder{}
	csrf := a.csrfHiddenInput(r)
	for _, b := range boards {
		msgs, _ := a.msgRepo.ListByBoard(b.ID)
		last := ""
		if len(msgs) > 0 {
			last = msgs[len(msgs)-1].CreatedAt.Format("2006-01-02 15:04")
		}
		rows.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td>%d</td><td>%s</td>`, b.ID, htmlEscape(b.Name), len(msgs), last))
		rows.WriteString(fmt.Sprintf(`<td><form method="POST" action="/admin/boards"><input type="hidden" name="action" value="delete"><input type="hidden" name="id" value="%d">`+csrf+`<button type="submit">delete</button></form></td>`, b.ID))
		rows.WriteString(`</tr>`)
	}
	page := `<html><body><h1>Boards</h1><p><a href="/admin">back</a></p>` +
		`<form method="POST"><label>Title <input name="title"></label> <label>Description <input name="description" size="50"></label>` + csrf + `<input type="hidden" name="action" value="create"><button type="submit">add</button></form>` +
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
		if !a.requireAdminWrite(w, r) {
			return
		}
		target := strings.TrimSpace(r.FormValue("handle"))
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		if target != "" {
			disabled := action == "disable_outbound"
			if a.adminRepo != nil {
				_ = a.adminRepo.SetMailOutboundPolicy(target, disabled)
			}
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
	csrf := a.csrfHiddenInput(r)
	policies := []domain.MailOutboundPolicy{}
	if a.adminRepo != nil {
		policies, _ = a.adminRepo.ListMailOutboundPolicies()
	}
	for _, row := range policies {
		rows.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%t</td>`, htmlEscape(row.Handle), row.OutboundDisabled))
		rows.WriteString(`<td><form method="POST" action="/admin/mail">`)
		rows.WriteString(`<input type="hidden" name="handle" value="` + row.Handle + `">`)
		rows.WriteString(csrf)
		if row.OutboundDisabled {
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
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method == http.MethodPost {
		if !a.requireAdminWrite(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		switch action {
		case "create":
			area := &domain.FileArea{
				Name:        strings.TrimSpace(r.FormValue("name")),
				Path:        strings.TrimSpace(r.FormValue("path")),
				Description: strings.TrimSpace(r.FormValue("description")),
			}
			if a.adminRepo != nil {
				if err := a.adminRepo.CreateFileArea(area); err == nil {
					a.recordAdminAction(user.Handle, area.Name, "create_file_area", area.Path)
				}
			}
		case "delete":
			id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
			if a.adminRepo != nil && id > 0 {
				_ = a.adminRepo.DeleteFileArea(id)
				a.recordAdminAction(user.Handle, strconv.FormatInt(id, 10), "delete_file_area", "")
			}
		}
		http.Redirect(w, r, "/admin/files", http.StatusFound)
		return
	}

	areas := []domain.FileArea{}
	if a.adminRepo != nil {
		areas, _ = a.adminRepo.ListFileAreas()
	}
	csrf := a.csrfHiddenInput(r)
	rows := strings.Builder{}
	for _, area := range areas {
		rows.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td>%s</td><td>%s</td>`, area.ID, htmlEscape(area.Name), htmlEscape(area.Path), htmlEscape(area.Description)))
		rows.WriteString(`<td><form method="POST" action="/admin/files">` + csrf + `<input type="hidden" name="action" value="delete"><input type="hidden" name="id" value="` + strconv.FormatInt(area.ID, 10) + `"><button type="submit">delete</button></form></td></tr>`)
	}
	if rows.Len() == 0 {
		rows.WriteString(`<tr><td colspan="5">No file areas configured</td></tr>`)
	}
	page := `<html><body><h1>Files</h1><p><a href="/admin">back</a></p>` +
		`<form method="POST"><input type="hidden" name="action" value="create">` + csrf +
		`<label>Name <input name="name"></label> <label>Path <input name="path" size="30"></label> <label>Description <input name="description" size="40"></label> <button type="submit">add</button></form>` +
		`<table border="1"><tr><th>ID</th><th>Name</th><th>Path</th><th>Description</th><th>Action</th></tr>` + rows.String() + `</table></body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminGateways(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method == http.MethodPost {
		if !a.requireAdminWrite(w, r) {
			return
		}
		cfg := &domain.GatewaySettings{
			SMTPHost:        strings.TrimSpace(r.FormValue("smtp_host")),
			SMTPPort:        parseInt(r.FormValue("smtp_port"), 587),
			SMTPUser:        strings.TrimSpace(r.FormValue("smtp_user")),
			SMTPPass:        strings.TrimSpace(r.FormValue("smtp_pass")),
			FromDomain:      strings.TrimSpace(r.FormValue("from_domain")),
			MaxRecipients:   parseInt(r.FormValue("max_recipients"), 3),
			MaxMessageBytes: parseInt(r.FormValue("max_message_bytes"), 65536),
			WebTimeoutSec:   parseInt(r.FormValue("web_timeout_sec"), 10),
			WebMaxBytes:     parseInt(r.FormValue("web_max_bytes"), 2*1024*1024),
		}
		if a.adminRepo != nil {
			if err := a.adminRepo.UpsertGatewaySettings(cfg); err == nil {
				a.recordAdminAction(user.Handle, "gateway_settings", "update_gateway_settings", "saved")
			}
		}
		http.Redirect(w, r, "/admin/gateways", http.StatusFound)
		return
	}
	cfg := &domain.GatewaySettings{
		SMTPHost:        strings.TrimSpace(os.Getenv("SMTP_HOST")),
		SMTPPort:        parseInt(os.Getenv("SMTP_PORT"), 587),
		SMTPUser:        strings.TrimSpace(os.Getenv("SMTP_USER")),
		SMTPPass:        strings.TrimSpace(os.Getenv("SMTP_PASS")),
		FromDomain:      strings.TrimSpace(os.Getenv("FROM_DOMAIN")),
		MaxRecipients:   parseInt(os.Getenv("WOLFBBS_MAIL_MAX_RECIPIENTS"), 3),
		MaxMessageBytes: parseInt(os.Getenv("WOLFBBS_MAIL_MAX_BYTES"), 65536),
		WebTimeoutSec:   10,
		WebMaxBytes:     2 * 1024 * 1024,
	}
	if a.adminRepo != nil {
		if dbCfg, err := a.adminRepo.GetGatewaySettings(); err == nil && dbCfg != nil {
			cfg = dbCfg
		}
	}
	csrf := a.csrfHiddenInput(r)
	page := `<html><body><h1>Gateway Controls</h1><p><a href="/admin">back</a></p>` +
		`<form method="POST">` + csrf +
		`<label>SMTP Host <input name="smtp_host" value="` + htmlEscape(cfg.SMTPHost) + `"></label><br>` +
		`<label>SMTP Port <input name="smtp_port" value="` + strconv.Itoa(cfg.SMTPPort) + `"></label><br>` +
		`<label>SMTP User <input name="smtp_user" value="` + htmlEscape(cfg.SMTPUser) + `"></label><br>` +
		`<label>SMTP Pass <input type="password" name="smtp_pass" value="` + htmlEscape(cfg.SMTPPass) + `"></label><br>` +
		`<label>From Domain <input name="from_domain" value="` + htmlEscape(cfg.FromDomain) + `"></label><br>` +
		`<label>Max Recipients <input name="max_recipients" value="` + strconv.Itoa(cfg.MaxRecipients) + `"></label><br>` +
		`<label>Max Message Bytes <input name="max_message_bytes" value="` + strconv.Itoa(cfg.MaxMessageBytes) + `"></label><br>` +
		`<label>Web Timeout Sec <input name="web_timeout_sec" value="` + strconv.Itoa(cfg.WebTimeoutSec) + `"></label><br>` +
		`<label>Web Max Bytes <input name="web_max_bytes" value="` + strconv.Itoa(cfg.WebMaxBytes) + `"></label><br>` +
		`<button type="submit">Save</button></form>` +
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
	entries := []domain.AdminAudit{}
	if a.adminRepo != nil {
		entries, _ = a.adminRepo.ListAudit(500)
	}
	for _, entry := range entries {
		rows.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			entry.CreatedAt.Format("2006-01-02 15:04:05"), htmlEscape(entry.Actor), htmlEscape(entry.Target), htmlEscape(entry.Action), htmlEscape(entry.Details)))
	}
	if rows.Len() == 0 {
		rows.WriteString(`<tr><td colspan="5">No admin actions yet</td></tr>`)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<html><body><h1>Admin Audit Log</h1><p><a href="/admin">back</a></p><table border="1"><tr><th>Time</th><th>Actor</th><th>Target</th><th>Action</th><th>Details</th></tr>` + rows.String() + `</table></body></html>`))
}

func (a *webApp) roleForUser(u *domain.User) int {
	if u == nil {
		return 0
	}
	return roleWeight[strings.ToLower(strings.TrimSpace(u.Role))]
}

func (a *webApp) hasRole(u *domain.User, minimum string) bool {
	return a.roleForUser(u) >= roleWeight[strings.ToLower(strings.TrimSpace(minimum))]
}

func (a *webApp) authRequired(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.currentUser(r); !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	})
}

func (a *webApp) mustBeRole(minRole string, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.currentUser(r)
		if !ok {
			loginPath := "/login"
			if strings.HasPrefix(r.URL.Path, "/admin") {
				loginPath = "/admin/login"
			}
			http.Redirect(w, r, loginPath, http.StatusFound)
			return
		}
		if !a.hasRole(u, minRole) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

func (a *webApp) requireCSRF(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}

	s, ok := currentSessionState(r, a)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	requestToken := r.FormValue("csrf_token")
	if requestToken == "" {
		requestToken = r.Header.Get("X-CSRF-Token")
	}
	if requestToken == "" {
		http.Error(w, "csrf token missing", http.StatusForbidden)
		return false
	}
	if !secureEquals(requestToken, s.csrf) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return false
	}
	return true
}

func (a *webApp) requireAdminWrite(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	if a.readOnly {
		http.Error(w, "read-only mode", http.StatusForbidden)
		return false
	}
	return a.requireCSRF(w, r)
}

func (a *webApp) csrfHiddenInput(r *http.Request) string {
	session, ok := currentSessionState(r, a)
	if !ok {
		return ""
	}
	return fmt.Sprintf(`<input type="hidden" name="csrf_token" value="%s">`, session.csrf)
}

func secureEquals(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	diff := 0
	for i := 0; i < len(a); i++ {
		diff |= int(a[i] ^ b[i])
	}
	return diff == 0
}

func currentSessionState(r *http.Request, a *webApp) (sessionState, bool) {
	c, err := r.Cookie("wolfbbs_session")
	if err != nil {
		return sessionState{}, false
	}
	a.Lock()
	defer a.Unlock()
	state, ok := a.sessions[c.Value]
	if !ok {
		return sessionState{}, false
	}
	if time.Now().After(state.expire) {
		delete(a.sessions, c.Value)
		return sessionState{}, false
	}
	return state, true
}

func (a *webApp) currentUser(r *http.Request) (*domain.User, bool) {
	session, ok := currentSessionState(r, a)
	if !ok {
		return nil, false
	}
	u, err := a.authSvc.GetUser(session.handle)
	if err != nil {
		return nil, false
	}
	return u, true
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
	if a.adminRepo != nil {
		_ = a.adminRepo.AddAudit(&domain.AdminAudit{
			Actor:   actor,
			Target:  target,
			Action:  action,
			Details: details,
		})
	}
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

func randomToken(length int) string {
	return randomPassword(length)
}

func (a *webApp) handleChat(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	csrf := ""
	if state, ok := currentSessionState(r, a); ok {
		csrf = state.csrf
	}
	csrfJSON := fmt.Sprintf("%q", csrf)

	modActions := ""
	csrfInput := a.csrfHiddenInput(r)
	if a.hasRole(user, roleModerator) {
		modActions = `
			<section id="mod">
				<h3>Moderation</h3>
				<form id="modForm" action="/chat/moderation" method="POST">
					` + csrfInput + `
					<label>Channel:
						<input type="text" name="channel" value="#lobby">
					</label>
					<label>Target: <input type="text" name="target" placeholder="target" required></label>
					<label>Reason: <input type="text" name="reason"></label>
					<label>Duration (optional): <input type="text" name="duration" value="" placeholder="5m"></label>
					<input type="hidden" name="action">
					<select name="action">
						<option value="kick">Kick</option>
						<option value="mute">Mute</option>
						<option value="unmute">Unmute</option>
						<option value="ban">Ban</option>
						<option value="unban">Unban</option>
					</select>
					<button type="submit">Apply</button>
				</form>
			</section>`
	}

	chatPage := `<!doctype html>
	<html>
	<body>
		<h1>WolfBBS Chat</h1>
		<p>Logged in as ` + user.Handle + `</p>
		<p><label>Channel:
			<select id="channelSelect"></select>
		</label></p>
		<div id="chat" style="height:300px; width: 800px; border:1px solid #333; overflow:auto; font-family: monospace; white-space: pre;"></div>
		<form id="sendForm">
			<input type="text" id="message" style="width: 600px;" autocomplete="off">
			<button type="submit">Send</button>
		</form>
		<p>Online:
			<span id="online"></span>
		</p>
		` + modActions + `
		<script>
			const csrf = ` + csrfJSON + `;
			const streamState = {es: null, channel: '#lobby'};

			function formatLine(m) {
				return '[' + m.created_at + '] ' + m.from + ': ' + m.body;
			}

			async function loadChannels() {
				const res = await fetch('/chat/channels', {credentials: 'same-origin'});
				if (!res.ok) return;
				const payload = await res.json();
				const select = document.getElementById('channelSelect');
				select.innerHTML = '';
				const channels = payload.channels || ['#lobby'];
				channels.forEach((name) => {
					const option = document.createElement('option');
					option.value = name;
					option.textContent = name;
					select.appendChild(option);
				});
				if (!channels.includes(streamState.channel)) {
					streamState.channel = channels[0] || '#lobby';
				}
				select.value = streamState.channel;
			}

			async function loadHistory() {
				const ch = streamState.channel;
				const res = await fetch('/chat/history?channel=' + encodeURIComponent(ch) + '&limit=100', {credentials: 'same-origin'});
				if (!res.ok) return;
				const payload = await res.json();
				const box = document.getElementById('chat');
				box.textContent = '';
				for (const m of payload.messages || []) {
					const line = document.createElement('div');
					line.textContent = formatLine(m);
					box.appendChild(line);
				}
			}

			async function loadOnline() {
				const ch = streamState.channel;
				const res = await fetch('/chat/online?channel=' + encodeURIComponent(ch), {credentials: 'same-origin'});
				if (!res.ok) return;
				const payload = await res.json();
				const online = document.getElementById('online');
				const names = (payload.presence || []).map((p) => p.nick).join(', ');
				online.textContent = names || 'none';
			}

			async function join() {
				const ch = streamState.channel;
				await fetch('/chat/join', {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json',
						'X-CSRF-Token': csrf,
					},
					body: JSON.stringify({ channel: ch }),
				});
			}

			function stopStream() {
				if (streamState.es) {
					streamState.es.close();
					streamState.es = null;
				}
			}

			function watch() {
				stopStream();
				const es = new EventSource('/chat/stream?channel=' + encodeURIComponent(streamState.channel) + '&after_id=0');
				streamState.es = es;
				es.onmessage = function(evt){
					const msg = JSON.parse(evt.data);
					const line = document.createElement('div');
					line.textContent = formatLine(msg);
					const box = document.getElementById('chat');
					box.appendChild(line);
					box.scrollTop = box.scrollHeight;
				};
				es.onerror = function() {
					es.close();
					setTimeout(watch, 1200);
				};
			}

			async function switchChannel(next) {
				streamState.channel = next;
				await join();
				await loadHistory();
				await loadOnline();
				watch();
			}

			async function leave(channel) {
				await fetch('/chat/leave', {
					method: 'POST',
					headers: {'Content-Type':'application/json','X-CSRF-Token': csrf},
					body: JSON.stringify({channel: channel}),
				});
			}

			document.getElementById('channelSelect').addEventListener('change', async function(evt){
				await switchChannel(evt.target.value);
			});

			document.getElementById('sendForm').addEventListener('submit', async function(evt){
				evt.preventDefault();
				const message = document.getElementById('message').value;
				if (!message) return;
				await fetch('/chat/send', {
					method:'POST',
					headers:{'Content-Type':'application/json','X-CSRF-Token': csrf},
					body: JSON.stringify({channel: streamState.channel, message: message}),
				});
				document.getElementById('message').value = '';
				await loadHistory();
			});

			window.addEventListener('load', async () => {
				await loadChannels();
				streamState.channel = document.getElementById('channelSelect').value || '#lobby';
				await join();
				await loadHistory();
				await loadOnline();
				watch();
				setInterval(loadOnline, 5000);
			});

			window.addEventListener('beforeunload', async () => {
				await leave(streamState.channel);
				stopStream();
			});
		</script>
	</body>
	</html>`
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
	if !a.requireCSRF(w, r) {
		return
	}
	body, err := chatRequestBody(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid request"))
		return
	}
	channel := chat.NormalizeChannel(body["channel"])
	if channel == "" {
		channel = "#lobby"
	}
	message := body["message"]
	if message == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("message required"))
		return
	}
	msg, err := a.chatSvc.Post(user.Handle, channel, message)
	if err != nil {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	_ = writeJSON(w, http.StatusCreated, msg)
}

func (a *webApp) handleChatStream(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	channel := chat.NormalizeChannel(strings.TrimSpace(r.URL.Query().Get("channel")))
	if channel == "" {
		channel = "#lobby"
	}
	a.chatSvc.JoinChannel(user.Handle, channel)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("stream unsupported"))
		return
	}
	after := parseChatSince(r)
	for _, msg := range a.chatSvc.HistorySince(channel, after, 50) {
		_ = writeMessageEvent(w, msg)
	}
	sub, closeSub := a.chatSvc.Subscribe(channel, user.Handle)
	defer closeSub()
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-sub:
			if !ok {
				return
			}
			if msg.Channel != channel {
				continue
			}
			if msg.ID <= after {
				continue
			}
			if err := writeMessageEvent(w, msg); err != nil {
				return
			}
			flusher.Flush()
			after = msg.ID
		}
	}
}

func writeMessageEvent(w http.ResponseWriter, msg chat.Message) error {
	payload, err := json.Marshal(map[string]interface{}{
		"id":         msg.ID,
		"from":       msg.From,
		"body":       msg.Body,
		"created_at": msg.CreatedAt.Format("15:04:05"),
		"channel":    msg.Channel,
		"to":         msg.To,
	})
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	_, err = w.Write([]byte("\n\n"))
	return err
}

func (a *webApp) handleChatChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]interface{}{"channels": a.chatSvc.ListChannels()})
}

func (a *webApp) handleChatJoin(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !a.requireCSRF(w, r) {
		return
	}
	body, err := chatRequestBody(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid request"))
		return
	}
	channel := chat.NormalizeChannel(body["channel"])
	if channel == "" {
		channel = "#lobby"
	}
	a.chatSvc.JoinChannel(user.Handle, channel)
	_ = writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "channel": channel})
}

func (a *webApp) handleChatLeave(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !a.requireCSRF(w, r) {
		return
	}
	body, err := chatRequestBody(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid request"))
		return
	}
	channel := chat.NormalizeChannel(body["channel"])
	if channel == "" {
		channel = "#lobby"
	}
	a.chatSvc.LeaveChannel(user.Handle, channel)
	_ = writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "channel": channel})
}

func (a *webApp) handleChatHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	channel := chat.NormalizeChannel(strings.TrimSpace(r.URL.Query().Get("channel")))
	if channel == "" {
		channel = "#lobby"
	}
	limit := parseLimit(r.URL.Query().Get("limit"), 50)
	after := parseChatSince(r)
	var msgs []chat.Message
	if after > 0 {
		msgs = a.chatSvc.HistorySince(channel, after, limit)
	} else {
		msgs = a.chatSvc.History(channel, limit)
	}
	var last int64
	if len(msgs) > 0 {
		last = msgs[len(msgs)-1].ID
	}
	presence := a.chatSvc.OnlineInChannel(channel)
	_ = writeJSON(w, http.StatusOK, chatHistoryResponse{
		Channel:  channel,
		Messages: msgs,
		Online:   presence,
		LastID:   last,
	})
}

func (a *webApp) handleChatOnline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	channel := chat.NormalizeChannel(strings.TrimSpace(r.URL.Query().Get("channel")))
	presence := a.chatSvc.OnlineInChannel(channel)
	_ = writeJSON(w, http.StatusOK, map[string]interface{}{
		"channel":  channel,
		"presence": presence,
		"count":    len(presence),
	})
}

func (a *webApp) handleChatModeration(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if !a.hasRole(user, roleModerator) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !a.requireCSRF(w, r) {
		return
	}
	payload := chatModerationPayload{}
	if err := chatRequestBodyStruct(r, &payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid request"))
		return
	}
	channel := chat.NormalizeChannel(payload.Channel)
	if channel == "" {
		channel = "#lobby"
	}
	target := strings.TrimSpace(payload.Target)
	action := strings.ToLower(strings.TrimSpace(payload.Action))
	switch action {
	case "ban":
		a.chatSvc.Ban(channel, target, user.Handle, payload.Reason, payload.Duration)
	case "unban":
		a.chatSvc.Unban(channel, target)
	case "mute":
		a.chatSvc.Mute(channel, target, user.Handle, payload.Reason, payload.Duration)
	case "unmute":
		a.chatSvc.Unmute(channel, target)
	case "kick":
		a.chatSvc.Kick(channel, user.Handle, target, payload.Reason)
	default:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("unsupported action"))
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *webApp) createSession(handle string) (string, bool) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", false
	}
	sid := hex.EncodeToString(b)
	csrf := randomToken(32)
	a.Lock()
	a.sessions[sid] = sessionState{handle: handle, expire: time.Now().Add(2 * time.Hour), csrf: csrf}
	a.Unlock()
	return sid, true
}

func parseLimit(raw string, defaultVal int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return defaultVal
	}
	return v
}

func parseChatSince(r *http.Request) int64 {
	var afterID int64
	raw := strings.TrimSpace(r.URL.Query().Get("after_id"))
	if raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v >= 0 {
			afterID = v
		}
	}
	return afterID
}

func chatRequestBody(r *http.Request) (map[string]string, error) {
	payload := map[string]string{}
	if err := parseJSONBody(r, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func chatRequestBodyStruct(r *http.Request, out interface{}) error {
	return parseJSONBody(r, out)
}

func parseJSONBody(r *http.Request, out interface{}) error {
	if out == nil {
		return nil
	}
	if !strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return parseBodyFromForm(r, out)
	}
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()
	return dec.Decode(out)
}

func parseBodyFromForm(r *http.Request, out interface{}) error {
	_ = r.ParseForm()
	if payload, ok := out.(*chatModerationPayload); ok {
		payload.Channel = strings.TrimSpace(r.FormValue("channel"))
		payload.Action = strings.TrimSpace(r.FormValue("action"))
		payload.Target = strings.TrimSpace(r.FormValue("target"))
		payload.Reason = strings.TrimSpace(r.FormValue("reason"))
		payload.Duration = strings.TrimSpace(r.FormValue("duration"))
		return nil
	}
	if payload, ok := out.(*map[string]string); ok {
		if *payload == nil {
			*payload = map[string]string{}
		}
		(*payload)["channel"] = strings.TrimSpace(r.FormValue("channel"))
		(*payload)["message"] = strings.TrimSpace(r.FormValue("message"))
		return nil
	}
	return nil
}

func (a *webApp) handleMailInbound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if strings.TrimSpace(a.inboundToken) == "" {
		http.Error(w, "inbound disabled", http.StatusNotFound)
		return
	}
	if !secureEquals(strings.TrimSpace(r.Header.Get("X-Inbound-Token")), a.inboundToken) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var payload struct {
		From       string `json:"from"`
		To         string `json:"to"`
		Subject    string `json:"subject"`
		Body       string `json:"body"`
		RawHeaders string `json:"raw_headers"`
	}
	if err := parseJSONBody(r, &payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if len(a.inboundAllow) > 0 {
		domain := senderDomain(payload.From)
		if domain == "" {
			http.Error(w, "invalid sender", http.StatusBadRequest)
			return
		}
		if _, ok := a.inboundAllow[domain]; !ok {
			http.Error(w, "sender domain blocked", http.StatusForbidden)
			return
		}
	}
	targetHandle := gateway.ParseInboundRecipient(payload.To)
	if targetHandle == "" {
		http.Error(w, "target recipient missing", http.StatusBadRequest)
		return
	}
	target, err := a.authSvc.GetUser(targetHandle)
	if err != nil || target == nil {
		http.Error(w, "target recipient not found", http.StatusNotFound)
		return
	}
	fromUser, err := a.authSvc.GetUser("mailbot")
	if err != nil || fromUser == nil {
		http.Error(w, "mailbot account unavailable", http.StatusInternalServerError)
		return
	}
	body := strings.TrimSpace(payload.Body)
	if body == "" {
		http.Error(w, "body required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.RawHeaders) != "" {
		body += "\n\n--- RAW HEADERS ---\n" + payload.RawHeaders
	}
	if err := a.mailRepo.CreateMail(&domain.PrivateMail{
		FromUserID: fromUser.ID,
		ToUserID:   target.ID,
		Subject:    strings.TrimSpace(payload.Subject),
		Body:       body,
	}); err != nil {
		http.Error(w, "could not store inbound mail", http.StatusInternalServerError)
		return
	}
	_ = writeJSON(w, http.StatusCreated, map[string]string{"status": "ok", "recipient": target.Handle})
}

func (a *webApp) userHandleLookup() map[int64]string {
	users, _ := a.authSvc.ListUsers()
	out := make(map[int64]string, len(users))
	for _, user := range users {
		out[user.ID] = user.Handle
	}
	return out
}

func htmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, `"`, "&quot;")
	return value
}

func quoteBody(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, "> "+line)
	}
	return strings.Join(out, "\n")
}

func seedDefaultBoards(repo repository.BoardRepository) {
	if repo == nil {
		return
	}
	boards, err := repo.List()
	if err != nil || len(boards) > 0 {
		return
	}
	seed := []domain.Board{
		{Name: "General", Description: "General system discussion", CreatedBy: 1},
		{Name: "Node Talk", Description: "Node status and operator chat", CreatedBy: 1},
		{Name: "Tooling", Description: "Build scripts and deployment", CreatedBy: 1},
	}
	for i := range seed {
		_ = repo.Create(&seed[i])
	}
}

func parseInt(raw string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseAllowDomains(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		out[part] = struct{}{}
	}
	return out
}

func senderDomain(from string) string {
	from = strings.TrimSpace(from)
	if from == "" {
		return ""
	}
	if strings.Contains(from, "<") {
		if addr, err := mail.ParseAddress(from); err == nil {
			from = addr.Address
		}
	}
	at := strings.LastIndex(from, "@")
	if at <= 0 || at+1 >= len(from) {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(from[at+1:]))
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) error {
	raw, err := json.Marshal(body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("json encode failure"))
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(raw)
	return err
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
