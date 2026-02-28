package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wolfbbs/internal/acs"
	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/config"
	"wolfbbs/internal/content"
	"wolfbbs/internal/discovery"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/doors"
	"wolfbbs/internal/events"
	"wolfbbs/internal/gateway"
	"wolfbbs/internal/logging"
	"wolfbbs/internal/menu"
	"wolfbbs/internal/mods"
	"wolfbbs/internal/network"
	"wolfbbs/internal/rbac"
	"wolfbbs/internal/repository"
	"wolfbbs/internal/ui"
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
	Channel  string                `json:"channel"`
	Messages []chatMessageResponse `json:"messages"`
	Online   []chat.Presence       `json:"online,omitempty"`
	LastID   int64                 `json:"last_id"`
}

type chatMessageResponse struct {
	ID        int64  `json:"id"`
	From      string `json:"from"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	Channel   string `json:"channel"`
	To        string `json:"to,omitempty"`
}

type chatModerationPayload struct {
	Channel  string `json:"channel"`
	Action   string `json:"action"`
	Target   string `json:"target"`
	Reason   string `json:"reason"`
	Duration string `json:"duration"`
}

type statusCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

type statusSummary struct {
	Total int `json:"total"`
	Pass  int `json:"pass"`
	Warn  int `json:"warn"`
}

type statusSnapshot struct {
	GeneratedAt     time.Time     `json:"generated_at"`
	Site            string        `json:"site"`
	Host            string        `json:"host"`
	User            string        `json:"user"`
	Role            string        `json:"role"`
	Summary         statusSummary `json:"summary"`
	Checks          []statusCheck `json:"checks"`
	Recommendations []string      `json:"recommendations,omitempty"`
}

type sessionState struct {
	handle string
	expire time.Time
	csrf   string
}

const (
	roleUser      = rbac.RoleUser
	roleModerator = rbac.RoleModerator
	roleAdmin     = rbac.RoleSysop
)

var roleWeight = map[string]int{
	roleUser:      1,
	roleModerator: 2,
	roleAdmin:     3,
	"admin":       3,
}

type adminLogEntry struct {
	Time    time.Time
	Actor   string
	Target  string
	Action  string
	Details string
}

type appErrorEntry struct {
	Time    time.Time
	Area    string
	Message string
}

const (
	sysSettingSiteName               = "site.name"
	sysSettingSiteHostname           = "site.hostname"
	sysSettingMOTD                   = "site.motd"
	sysSettingAnnouncement           = "site.announcement"
	sysSettingReadOnly               = "site.read_only"
	sysSettingSecureCookie           = "site.secure_cookie"
	sysSettingWebOnRamp              = "site.web_onramp_enable"
	sysSettingGuestTour              = "site.guest_tour_enable"
	sysSettingDiscover               = "site.discover_enable"
	sysSettingQuickJump              = "site.quick_jump_enable"
	sysSettingClassicSearch          = "site.classic_search_enable"
	sysSettingRequireVerifiedEmail   = "mail.require_verified"
	sysSettingMenuEnabled            = "menu.enabled"
	sysSettingMenuFile               = "menu.file"
	sysSettingLockedChannels         = "chat.locked_channels"
	sysSettingACSStrict              = "runtime.acs.strict"
	sysSettingContentHost            = "runtime.content.host"
	sysSettingContentGopherListen    = "runtime.content.gopher_listen"
	sysSettingContentNNTPListen      = "runtime.content.nntp_listen"
	sysSettingContentNNTPSListen     = "runtime.content.nntps_listen"
	sysSettingContentNNTPSCert       = "runtime.content.nntps_cert"
	sysSettingContentNNTPSKey        = "runtime.content.nntps_key"
	sysSettingActivityPubEnabled     = "runtime.activitypub.enabled"
	sysSettingActivityPubBaseURL     = "runtime.activitypub.base_url"
	sysSettingLoginTelnetEnabled     = "runtime.login.telnet.enabled"
	sysSettingLoginTelnetListen      = "runtime.login.telnet.listen"
	sysSettingLoginWSEnabled         = "runtime.login.ws.enabled"
	sysSettingLoginWSListen          = "runtime.login.ws.listen"
	sysSettingLoginWSPath            = "runtime.login.ws.path"
	sysSettingLoginWSSEnabled        = "runtime.login.wss.enabled"
	sysSettingLoginWSSListen         = "runtime.login.wss.listen"
	sysSettingLoginWSSPath           = "runtime.login.wss.path"
	sysSettingLoginWSSCert           = "runtime.login.wss.cert"
	sysSettingLoginWSSKey            = "runtime.login.wss.key"
	sysSettingTrustedProxies         = "runtime.login.trusted_proxies"
	sysSettingConnectorDoorPartyOn   = "runtime.connector.doorparty.enabled"
	sysSettingConnectorDoorPartyCmd  = "runtime.connector.doorparty.command"
	sysSettingConnectorDoorPartyArgs = "runtime.connector.doorparty.args"
	sysSettingConnectorBBSLinkOn     = "runtime.connector.bbslink.enabled"
	sysSettingConnectorBBSLinkCmd    = "runtime.connector.bbslink.command"
	sysSettingConnectorBBSLinkArgs   = "runtime.connector.bbslink.args"
	sysSettingConnectorTelnetOn      = "runtime.connector.telnet_bridge.enabled"
	sysSettingConnectorTelnetCmd     = "runtime.connector.telnet_bridge.command"
	sysSettingConnectorTelnetArgs    = "runtime.connector.telnet_bridge.args"
	maxAdminErrorEntries             = 300
)

type webApp struct {
	authSvc   *auth.Service
	userRepo  repository.UserRepository
	boardRepo repository.BoardRepository
	msgRepo   repository.MessageRepository
	mailRepo  repository.PrivateMailRepository
	adminRepo repository.AdminRepository
	doorRepo  repository.DoorRepository
	email     *gateway.EmailGateway
	sessions  map[string]sessionState
	sync.Mutex
	chatSvc              *chat.Service
	doorRegistry         *doors.Registry
	eventBus             *events.Bus
	offlineDir           string
	siteName             string
	siteHostname         string
	readOnly             bool
	secureCookie         bool
	requireVerifiedEmail bool
	inboundToken         string
	inboundAllow         map[string]struct{}
	resetTTL             time.Duration
	showResetDev         bool
	apEnabled            bool
	apBaseURL            string
	publicBaseURL        string
	resetNotifier        func(handle, token string, r *http.Request) error
	startedAt            time.Time
	runtimeCfg           config.Runtime
	modernOnRamp         bool
	guestTour            bool
	discover             bool
	quickJump            bool
	classicSearch        bool
	wsTerminalURL        string
	menuRoot             string
	savedSearches        map[string][]string
	motd                 string
	announcement         string
	errorLog             []appErrorEntry
	lockedChat           map[string]bool
	networkSvc           *network.Service
	modsManager          *mods.Manager
	oneLinerzMod         *mods.OneLinerzMod
	rumorzMod            *mods.RumorzMod
	bbsListMod           *mods.BBSListMod
	whoOnlineMod         *mods.WhoOnlineMod
}

func seedWebUsers(authSvc *auth.Service) {
	entries := []struct {
		handle string
		pass   string
		role   string
	}{
		{
			handle: strings.TrimSpace(os.Getenv("WOLFBBS_BOOTSTRAP_ADMIN_HANDLE")),
			pass:   strings.TrimSpace(os.Getenv("WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD")),
			role:   roleAdmin,
		},
		{
			handle: strings.TrimSpace(os.Getenv("WOLFBBS_BOOTSTRAP_MODERATOR_HANDLE")),
			pass:   strings.TrimSpace(os.Getenv("WOLFBBS_BOOTSTRAP_MODERATOR_PASSWORD")),
			role:   roleModerator,
		},
		{
			handle: strings.TrimSpace(os.Getenv("WOLFBBS_BOOTSTRAP_USER_HANDLE")),
			pass:   strings.TrimSpace(os.Getenv("WOLFBBS_BOOTSTRAP_USER_PASSWORD")),
			role:   roleUser,
		},
	}
	for _, entry := range entries {
		if entry.handle == "" || entry.pass == "" {
			continue
		}
		existing, err := authSvc.GetUser(entry.handle)
		if err == nil && existing != nil {
			if existing.Role != entry.role {
				_ = authSvc.SetRole(entry.handle, entry.role)
			}
			continue
		}
		u, err := authSvc.Register(entry.handle, entry.pass)
		if err != nil {
			continue
		}
		u.Role = entry.role
		_ = authSvc.SetRole(entry.handle, entry.role)
	}
}

func seedServiceUsers(authSvc *auth.Service) {
	const serviceHandle = "mailbot"
	if authSvc == nil {
		return
	}
	existing, err := authSvc.GetUser(serviceHandle)
	if err == nil && existing != nil {
		_ = authSvc.SetEnabled(serviceHandle, false)
		_ = authSvc.SetVerified(serviceHandle, true)
		_ = authSvc.SetRole(serviceHandle, roleUser)
		return
	}
	if _, err := authSvc.Register(serviceHandle, randomPassword(28)); err != nil {
		return
	}
	_ = authSvc.SetEnabled(serviceHandle, false)
	_ = authSvc.SetVerified(serviceHandle, true)
	_ = authSvc.SetRole(serviceHandle, roleUser)
}

func main() {
	logging.ConfigureStdLogger("wolfbbs-web")
	listen := flag.String("listen", ":8080", "HTTP listen address")
	dbURL := flag.String("db", "", "PostgreSQL DSN (defaults to WOLFBBS_DATABASE_URL / DATABASE_URL / PG* env)")
	flag.Parse()
	if *dbURL == "" {
		*dbURL = repository.ResolveDatabaseURL()
	}
	runtimeCfg, cfgErr := config.LoadRuntimeFromEnv()
	if cfgErr != nil {
		log.Fatalf("config init: %v", cfgErr)
	}
	if err := ui.LoadThemesFromEnv(); err != nil {
		log.Printf("theme config load failed; using built-in themes: %v", err)
	}

	storage, err := repository.OpenStorageFromEnv(*dbURL)
	if err != nil {
		log.Fatalf("repository init: %v", err)
	}
	defer storage.Close()
	authSvc := auth.NewService(storage.Users)
	authSvc.SetPasswordResetRepository(storage.Resets)
	bus := events.NewBus()
	bus.Subscribe("*", func(ev events.Event) {
		log.Printf("event=%s fields=%v", ev.Name, ev.Fields)
	})
	authSvc.SetEventBus(bus)
	seedWebUsers(authSvc)
	seedServiceUsers(authSvc)
	seedDefaultBoards(storage.Boards)

	app := &webApp{
		authSvc:   authSvc,
		userRepo:  storage.Users,
		boardRepo: storage.Boards,
		msgRepo:   storage.Messages,
		mailRepo:  storage.Mail,
		adminRepo: storage.Admin,
		doorRepo:  storage.Doors,
		email:     gateway.NewEmailGateway(gateway.LoadEmailConfigFromEnv()),
		sessions:  map[string]sessionState{},
		chatSvc: func() *chat.Service {
			svc := chat.NewService()
			svc.SetEventBus(bus)
			return svc
		}(),
		doorRegistry: func() *doors.Registry {
			reg := doors.NewRegistry()
			reg.SetRepository(storage.Doors)
			if triviaBinary := strings.TrimSpace(os.Getenv("WOLFBBS_TRIVIA_BINARY")); triviaBinary != "" {
				doors.SeedTrivia(reg, triviaBinary)
			}
			doors.SeedFromEnv(reg)
			return reg
		}(),
		offlineDir: func() string {
			dir := strings.TrimSpace(os.Getenv("WOLFBBS_OFFLINE_DIR"))
			if dir == "" {
				dir = ".wolfbbs/offline"
			}
			return dir
		}(),
		siteName: func() string {
			name := strings.TrimSpace(os.Getenv("WOLFBBS_BBS_NAME"))
			if name == "" {
				name = "WolfBBS"
			}
			return name
		}(),
		siteHostname: func() string {
			host := strings.TrimSpace(os.Getenv("WOLFBBS_HOSTNAME"))
			if host == "" {
				host = "localhost"
			}
			return host
		}(),
		readOnly:             strings.EqualFold(strings.TrimSpace(os.Getenv("WOLFBBS_READ_ONLY")), "1") || strings.EqualFold(strings.TrimSpace(os.Getenv("WOLFBBS_READ_ONLY")), "true"),
		secureCookie:         envEnabledDefault("WOLFBBS_SECURE_COOKIE", false),
		requireVerifiedEmail: envEnabledDefault("WOLFBBS_REQUIRE_VERIFIED_EMAIL", true),
		inboundToken:         strings.TrimSpace(os.Getenv("WOLFBBS_INBOUND_TOKEN")),
		inboundAllow:         parseAllowDomains(strings.TrimSpace(os.Getenv("WOLFBBS_MAILIN_ALLOW_DOMAINS"))),
		resetTTL: func() time.Duration {
			raw := strings.TrimSpace(os.Getenv("WOLFBBS_RESET_TTL_MINUTES"))
			if raw == "" {
				return 30 * time.Minute
			}
			minutes, err := strconv.Atoi(raw)
			if err != nil || minutes <= 0 {
				return 30 * time.Minute
			}
			return time.Duration(minutes) * time.Minute
		}(),
		showResetDev: strings.EqualFold(strings.TrimSpace(os.Getenv("WOLFBBS_DEV_SHOW_RESET_TOKEN")), "1") ||
			strings.EqualFold(strings.TrimSpace(os.Getenv("WOLFBBS_DEV_SHOW_RESET_TOKEN")), "true"),
		apEnabled:     runtimeCfg.ActivityPub.Enabled,
		apBaseURL:     runtimeCfg.ActivityPub.BaseURL,
		publicBaseURL: strings.TrimSpace(os.Getenv("WOLFBBS_PUBLIC_BASE_URL")),
		eventBus:      bus,
		startedAt:     time.Now().UTC(),
		runtimeCfg:    runtimeCfg,
		modernOnRamp:  envEnabledDefault("WOLFBBS_WEB_ONRAMP_ENABLE", false),
		guestTour:     envEnabledDefault("WOLFBBS_GUEST_TOUR_ENABLE", false),
		discover:      envEnabledDefault("WOLFBBS_DISCOVER_ENABLE", false),
		quickJump:     envEnabledDefault("WOLFBBS_QUICK_JUMP_ENABLE", false),
		classicSearch: envEnabledDefault("WOLFBBS_CLASSIC_SEARCH_ENABLE", false),
		wsTerminalURL: strings.TrimSpace(envFirst("WOLFBBS_WS_TERMINAL_URL", "WOLFBBS_WS_URL", "WOLFBBS_WSS_URL")),
		menuRoot:      strings.TrimSpace(os.Getenv("WOLFBBS_MENU_ROOT")),
		savedSearches: map[string][]string{},
		lockedChat:    map[string]bool{},
		networkSvc: func() *network.Service {
			spoolDir := strings.TrimSpace(os.Getenv("WOLFBBS_NET_SPOOL_DIR"))
			if spoolDir == "" {
				spoolDir = ".wolfbbs/network"
			}
			return network.NewService(spoolDir, storage.Boards, storage.Messages, storage.Users, storage.Mail)
		}(),
	}
	app.loadPersistedAdminSettings()
	app.oneLinerzMod = mods.NewOneLinerzMod(80)
	app.rumorzMod = mods.NewRumorzMod(nil)
	app.bbsListMod = mods.NewBBSListMod(200)
	app.whoOnlineMod = mods.NewWhoOnlineMod(func() int {
		if app.chatSvc == nil {
			return 0
		}
		return len(app.chatSvc.Online())
	})
	app.modsManager = mods.NewManager(time.Minute)
	_ = app.modsManager.Register(app.oneLinerzMod, true)
	_ = app.modsManager.Register(app.rumorzMod, true)
	_ = app.modsManager.Register(app.bbsListMod, true)
	_ = app.modsManager.Register(app.whoOnlineMod, true)
	if err := app.modsManager.Start(context.Background()); err != nil {
		log.Printf("mods manager start failed: %v", err)
	}
	defer func() {
		if app.modsManager != nil {
			_ = app.modsManager.Stop(context.Background())
		}
	}()
	if app.eventBus != nil {
		app.eventBus.Subscribe("chat.post", func(ev events.Event) {
			handle := strings.TrimSpace(ev.Fields["nick"])
			body := strings.TrimSpace(ev.Fields["message"])
			if handle == "" || body == "" || app.oneLinerzMod == nil {
				return
			}
			app.oneLinerzMod.Add(handle, cleanOneLiner(body, 120))
		})
		app.eventBus.Subscribe("message.posted", func(ev events.Event) {
			handle := strings.TrimSpace(ev.Fields["author"])
			subject := strings.TrimSpace(ev.Fields["subject"])
			if handle == "" || subject == "" || app.oneLinerzMod == nil {
				return
			}
			app.oneLinerzMod.Add(handle, "posted: "+cleanOneLiner(subject, 96))
		})
	}

	http.HandleFunc("/", app.handleRoot)
	http.HandleFunc("/connect", app.handleConnect)
	http.HandleFunc("/tour", app.handleGuestTour)
	http.HandleFunc("/login", app.handleLogin)
	http.HandleFunc("/admin/login", app.handleLogin)
	http.HandleFunc("/help", app.handleHelp)
	http.HandleFunc("/reset/request", app.handlePasswordResetRequest)
	http.HandleFunc("/reset/complete", app.handlePasswordResetComplete)
	http.HandleFunc("/logout", app.handleLogout)
	http.HandleFunc("/.well-known/webfinger", app.handleActivityPubWebFinger)
	http.HandleFunc("/ap/users/", app.handleActivityPubUsers)
	http.Handle("/boards", app.authRequired(http.HandlerFunc(app.handleBoards)))
	http.Handle("/mail", app.authRequired(http.HandlerFunc(app.handleMail)))
	http.Handle("/settings", app.authRequired(http.HandlerFunc(app.handleSettings)))
	http.Handle("/status", app.authRequired(http.HandlerFunc(app.handleStatusCenter)))
	http.Handle("/statusz", app.authRequired(http.HandlerFunc(app.handleStatusJSON)))
	http.Handle("/config", app.authRequired(http.HandlerFunc(app.handleConfigCenter)))
	http.Handle("/discover", app.authRequired(http.HandlerFunc(app.handleDiscover)))
	http.Handle("/admin", app.mustBeRole(roleAdmin, app.handleAdmin))
	http.Handle("/admin/users", app.mustBeRole(roleAdmin, app.handleAdminUsers))
	http.Handle("/admin/boards", app.mustBeRole(roleAdmin, app.handleAdminBoards))
	http.Handle("/admin/mail", app.mustBeRole(roleAdmin, app.handleAdminMail))
	http.Handle("/admin/files", app.mustBeRole(roleAdmin, app.handleAdminFiles))
	http.Handle("/admin/gateways", app.mustBeRole(roleAdmin, app.handleAdminGateways))
	http.Handle("/admin/chat", app.mustBeRole(roleAdmin, app.handleAdminChat))
	http.Handle("/admin/doors", app.mustBeRole(roleAdmin, app.handleAdminDoors))
	http.Handle("/admin/setup", app.mustBeRole(roleAdmin, app.handleAdminSetup))
	http.Handle("/admin/config", app.mustBeRole(roleAdmin, app.handleAdminConfig))
	http.Handle("/admin/errors", app.mustBeRole(roleAdmin, app.handleAdminErrors))
	http.Handle("/admin/system", app.mustBeRole(roleAdmin, app.handleAdminSystem))
	http.Handle("/admin/node-state", app.mustBeRole(roleAdmin, app.handleAdminNodeState))
	http.Handle("/admin/audit", app.mustBeRole(roleAdmin, app.handleAdminAudit))
	http.Handle("/scores", app.authRequired(http.HandlerFunc(app.handleScores)))
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
	http.HandleFunc("/readyz", app.handleReadyz)
	http.HandleFunc("/metrics", app.handleMetrics)

	startOptionalContentServers(runtimeCfg, storage.Boards, storage.Messages)

	fmt.Printf("WolfBBS web companion on %s\n", *listen)
	log.Fatal(http.ListenAndServe(*listen, app.withModernUI(http.DefaultServeMux)))
}

func startOptionalContentServers(runtimeCfg config.Runtime, boardRepo repository.BoardRepository, msgRepo repository.MessageRepository) {
	publicHost := strings.TrimSpace(runtimeCfg.Content.Host)
	if publicHost == "" {
		publicHost = "localhost"
	}
	if listen := strings.TrimSpace(runtimeCfg.Content.GopherListen); listen != "" {
		gopherServer := content.NewGopherServer(listen, publicHost, boardRepo, msgRepo)
		if err := gopherServer.Start(); err != nil {
			log.Printf("gopher server failed to start on %s: %v", listen, err)
		} else {
			log.Printf("gopher server listening on %s", gopherServer.Addr())
		}
	}
	if listen := strings.TrimSpace(runtimeCfg.Content.NNTPListen); listen != "" {
		nntpServer := content.NewNNTPServer(listen, boardRepo, msgRepo)
		if err := nntpServer.Start(); err != nil {
			log.Printf("nntp server failed to start on %s: %v", listen, err)
		} else {
			log.Printf("nntp server listening on %s", nntpServer.Addr())
		}
	}
	if listen := strings.TrimSpace(runtimeCfg.Content.NNTPSListen); listen != "" {
		certPath := strings.TrimSpace(runtimeCfg.Content.NNTPSCert)
		keyPath := strings.TrimSpace(runtimeCfg.Content.NNTPSKey)
		nntpsServer := content.NewNNTPTLSServer(listen, certPath, keyPath, boardRepo, msgRepo)
		if err := nntpsServer.Start(); err != nil {
			log.Printf("nntps server failed to start on %s: %v", listen, err)
		} else {
			log.Printf("nntps server listening on %s", nntpsServer.Addr())
		}
	}
}

func (a *webApp) handleRoot(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.currentUser(r); ok {
		http.Redirect(w, r, "/boards", http.StatusFound)
		return
	}
	if a.modernOnRamp {
		http.Redirect(w, r, "/connect", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (a *webApp) handleHealthz(w http.ResponseWriter, r *http.Request) {
	_ = r
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (a *webApp) handleReadyz(w http.ResponseWriter, r *http.Request) {
	_ = r
	if a.boardRepo != nil {
		if _, err := a.boardRepo.List(); err != nil {
			http.Error(w, "db unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func (a *webApp) handleMetrics(w http.ResponseWriter, r *http.Request) {
	_ = r
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	a.Lock()
	sessionCount := len(a.sessions)
	a.Unlock()
	channelCount := 0
	if a.chatSvc != nil {
		channelCount = len(a.chatSvc.ListChannels())
	}
	_, _ = fmt.Fprintf(w, "wolfbbs_sessions %d\n", sessionCount)
	_, _ = fmt.Fprintf(w, "wolfbbs_chat_channels %d\n", channelCount)
	_, _ = fmt.Fprintln(w, "wolfbbs_build_info{version=\"dev\"} 1")
}

func (a *webApp) handleActivityPubWebFinger(w http.ResponseWriter, r *http.Request) {
	if !a.apEnabled {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	resource := strings.TrimSpace(r.URL.Query().Get("resource"))
	if resource == "" || !strings.HasPrefix(strings.ToLower(resource), "acct:") {
		http.Error(w, "resource query is required", http.StatusBadRequest)
		return
	}
	acct := strings.TrimPrefix(resource, "acct:")
	parts := strings.SplitN(acct, "@", 2)
	if len(parts) != 2 {
		http.Error(w, "invalid acct resource", http.StatusBadRequest)
		return
	}
	handle := strings.TrimSpace(parts[0])
	if handle == "" {
		http.Error(w, "invalid acct resource", http.StatusBadRequest)
		return
	}
	user, err := a.authSvc.GetUser(handle)
	if err != nil || user == nil {
		http.NotFound(w, r)
		return
	}
	base := a.activityPubBase(r)
	actorURL := base + "/ap/users/" + url.PathEscape(user.Handle)
	subjectHost := parts[1]
	if strings.TrimSpace(subjectHost) == "" {
		subjectHost = r.Host
	}
	_ = writeJSON(w, http.StatusOK, map[string]interface{}{
		"subject": fmt.Sprintf("acct:%s@%s", user.Handle, subjectHost),
		"links": []map[string]string{
			{
				"rel":  "self",
				"type": "application/activity+json",
				"href": actorURL,
			},
		},
	})
}

func (a *webApp) handleActivityPubUsers(w http.ResponseWriter, r *http.Request) {
	if !a.apEnabled {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/ap/users/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	handle, err := url.PathUnescape(strings.TrimSpace(parts[0]))
	if err != nil || handle == "" {
		http.NotFound(w, r)
		return
	}
	user, err := a.authSvc.GetUser(handle)
	if err != nil || user == nil {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 {
		a.handleActivityPubActor(w, r, user)
		return
	}
	switch strings.ToLower(strings.TrimSpace(parts[1])) {
	case "outbox":
		a.handleActivityPubOutbox(w, r, user)
	case "inbox":
		http.Error(w, "activitypub inbox is disabled", http.StatusNotImplemented)
	default:
		http.NotFound(w, r)
	}
}

func (a *webApp) handleActivityPubActor(w http.ResponseWriter, r *http.Request, user *domain.User) {
	base := a.activityPubBase(r)
	actorURL := base + "/ap/users/" + url.PathEscape(user.Handle)
	_ = writeJSON(w, http.StatusOK, map[string]interface{}{
		"@context":          "https://www.w3.org/ns/activitystreams",
		"id":                actorURL,
		"type":              "Person",
		"preferredUsername": user.Handle,
		"name":              user.Handle,
		"inbox":             actorURL + "/inbox",
		"outbox":            actorURL + "/outbox",
	})
}

func (a *webApp) handleActivityPubOutbox(w http.ResponseWriter, r *http.Request, user *domain.User) {
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			if parsed > 100 {
				parsed = 100
			}
			limit = parsed
		}
	}
	all := a.listMessagesByAuthor(user.ID)
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID > all[j].ID
		}
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})
	if len(all) > limit {
		all = all[:limit]
	}

	base := a.activityPubBase(r)
	actorURL := base + "/ap/users/" + url.PathEscape(user.Handle)
	items := make([]map[string]interface{}, 0, len(all))
	for _, msg := range all {
		noteID := fmt.Sprintf("%s/outbox/%d", actorURL, msg.ID)
		note := map[string]interface{}{
			"id":           noteID,
			"type":         "Note",
			"attributedTo": actorURL,
			"published":    msg.CreatedAt.UTC().Format(time.RFC3339),
			"summary":      msg.Subject,
			"content":      msg.Body,
			"to":           []string{"https://www.w3.org/ns/activitystreams#Public"},
		}
		if msg.ParentID > 0 {
			note["inReplyTo"] = fmt.Sprintf("%s/outbox/%d", actorURL, msg.ParentID)
		}
		items = append(items, map[string]interface{}{
			"id":        noteID + "#create",
			"type":      "Create",
			"actor":     actorURL,
			"object":    note,
			"to":        []string{"https://www.w3.org/ns/activitystreams#Public"},
			"published": msg.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	_ = writeJSON(w, http.StatusOK, map[string]interface{}{
		"@context":     "https://www.w3.org/ns/activitystreams",
		"id":           actorURL + "/outbox",
		"type":         "OrderedCollection",
		"totalItems":   len(items),
		"orderedItems": items,
	})
}

func (a *webApp) listMessagesByAuthor(userID int64) []domain.Message {
	if userID <= 0 || a.boardRepo == nil || a.msgRepo == nil {
		return nil
	}
	boards, err := a.boardRepo.List()
	if err != nil {
		return nil
	}
	out := make([]domain.Message, 0, 64)
	for _, board := range boards {
		msgs, listErr := a.msgRepo.ListByBoard(board.ID)
		if listErr != nil {
			continue
		}
		for _, msg := range msgs {
			if msg.AuthorID == userID {
				out = append(out, msg)
			}
		}
	}
	return out
}

const modernUIBootstrap = `<style id="wolfbbs-modern-ui">
:root{
  --bg:#f3f7fb;
  --bg-alt:#e8eef7;
  --surface:#ffffff;
  --surface-2:#f8fbff;
  --text:#0f1b2a;
  --muted:#516173;
  --line:#d9e3ef;
  --accent:#0f4fa8;
  --accent-strong:#09397a;
  --ok:#157347;
  --warn:#9a6700;
  --danger:#a81f2f;
  --shadow:0 12px 26px rgba(15,27,42,.10);
  --radius:14px;
}
*{box-sizing:border-box}
html,body{height:100%}
body{
  margin:0;
  padding:28px 24px 40px;
  color:var(--text);
  font:15px/1.45 "Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  background:
    radial-gradient(1200px 340px at 10% -18%, #d7e6fb 0%, transparent 62%),
    radial-gradient(1100px 260px at 88% -15%, #dbe8f8 0%, transparent 62%),
    linear-gradient(180deg,var(--bg),var(--bg-alt));
}
h1,h2,h3{margin:0 0 10px;font-weight:700;line-height:1.2}
h1{font-size:1.7rem;letter-spacing:.01em}
h2{font-size:1.2rem}
h3{font-size:1.02rem}
p,ul,ol,table,form,section,article,pre{margin:0 0 14px}
a{
  color:var(--accent);
  text-decoration:none;
  text-underline-offset:2px;
}
a:hover{color:var(--accent-strong);text-decoration:underline}
body > h1:first-of-type{
  margin-bottom:14px;
}
body > p:first-of-type{
  color:var(--muted);
}
body > p:has(> a){
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  align-items:center;
  padding:10px 12px;
  background:var(--surface);
  border:1px solid var(--line);
  border-radius:12px;
  box-shadow:var(--shadow);
}
body > p:has(> a) a{
  display:inline-flex;
  align-items:center;
  justify-content:center;
  min-height:32px;
  padding:6px 12px;
  border-radius:999px;
  border:1px solid #c3d4ea;
  background:#f5faff;
  color:#0f3f83;
  font-weight:600;
  font-size:.92rem;
  text-transform:lowercase;
}
body > p:has(> a) a:hover{
  background:#eaf3ff;
  border-color:#9cbce4;
  text-decoration:none;
}
table{
  width:100%;
  border-collapse:separate;
  border-spacing:0;
  background:var(--surface);
  border:1px solid var(--line);
  border-radius:12px;
  overflow:hidden;
  box-shadow:var(--shadow);
}
th,td{
  padding:10px 12px;
  text-align:left;
  border-bottom:1px solid #e7eef7;
  vertical-align:top;
}
th{
  background:#eef4fb;
  color:#1e3551;
  font-weight:700;
  font-size:.87rem;
  text-transform:uppercase;
  letter-spacing:.04em;
}
tr:nth-child(even) td{background:#fbfdff}
tr:last-child td{border-bottom:0}
form{
  background:var(--surface);
  border:1px solid var(--line);
  border-radius:12px;
  padding:14px;
  box-shadow:var(--shadow);
}
label{
  display:inline-flex;
  flex-direction:column;
  gap:6px;
  margin:0 10px 10px 0;
  font-weight:600;
  color:#2b4058;
}
input[type=text],input[type=password],input[type=email],input[type=number],input[type=url],input[type=search],select,textarea{
  width:min(100%,520px);
  min-height:38px;
  border-radius:10px;
  border:1px solid #bccde3;
  background:#fff;
  color:var(--text);
  padding:8px 10px;
  font:inherit;
  transition:border-color .16s ease, box-shadow .16s ease;
}
textarea{min-height:110px;resize:vertical}
input:focus,select:focus,textarea:focus{
  outline:0;
  border-color:#3c7fd5;
  box-shadow:0 0 0 3px rgba(60,127,213,.17);
}
button,input[type=submit],input[type=button]{
  border:0;
  border-radius:10px;
  min-height:36px;
  padding:8px 14px;
  cursor:pointer;
  font:600 .95rem/1 "Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  color:#fff;
  background:linear-gradient(180deg,#2565c0,#0f4fa8);
}
button:hover,input[type=submit]:hover,input[type=button]:hover{
  background:linear-gradient(180deg,#1a56aa,#093f88);
}
code,pre{
  font-family:"SFMono-Regular","Menlo","Consolas",monospace;
}
pre{
  padding:10px 12px;
  border:1px solid var(--line);
  border-radius:10px;
  background:#f7fbff;
  overflow:auto;
}
#chat{
  border:1px solid #173456 !important;
  border-radius:12px;
  background:#0d1726;
  color:#d9e6fb;
  box-shadow:0 8px 20px rgba(5,10,18,.32);
}
#chat > div{
  line-height:1.35;
  padding:2px 4px;
}
#chatStatus{
  color:#254f87 !important;
  font-weight:600;
}
#mod{
  margin-top:12px;
}
#mod form{
  background:#f7fbff;
  border-color:#bfd4ec;
}
hr{
  border:0;
  border-top:1px solid var(--line);
  margin:16px 0;
}
@media (max-width: 820px){
  body{padding:18px 14px 26px}
  body > p:has(> a){padding:9px 10px}
  input[type=text],input[type=password],input[type=email],input[type=number],input[type=url],input[type=search],select,textarea{
    width:100%;
  }
}
</style>
<script id="wolfbbs-modern-ui-js">
(() => {
  if (document.documentElement.dataset.wolfbbsModernUi === "1") return;
  document.documentElement.dataset.wolfbbsModernUi = "1";
  const navRows = [...document.querySelectorAll("p")].filter((p) => p.querySelectorAll("a").length >= 3 && p.textContent.includes("|"));
  navRows.forEach((row) => row.classList.add("wolfbbs-nav-row"));
})();
</script>`

type htmlStyleWriter struct {
	writer      http.ResponseWriter
	header      http.Header
	status      int
	sent        bool
	passthrough bool
	body        bytes.Buffer
}

func newHTMLStyleWriter(w http.ResponseWriter) *htmlStyleWriter {
	return &htmlStyleWriter{
		writer: w,
		header: make(http.Header),
	}
}

func (w *htmlStyleWriter) Header() http.Header {
	return w.header
}

func (w *htmlStyleWriter) WriteHeader(statusCode int) {
	if w.status == 0 {
		w.status = statusCode
	}
	if w.passthrough {
		w.sendHeaders()
	}
}

func (w *htmlStyleWriter) Write(data []byte) (int, error) {
	if w.passthrough {
		w.sendHeaders()
		return w.writer.Write(data)
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

func (w *htmlStyleWriter) Flush() {
	if !w.passthrough {
		w.passthrough = true
		w.sendHeaders()
		if w.body.Len() > 0 {
			_, _ = w.writer.Write(w.body.Bytes())
			w.body.Reset()
		}
	}
	if flusher, ok := w.writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *htmlStyleWriter) sendHeaders() {
	if w.sent {
		return
	}
	for key, values := range w.header {
		target := w.writer.Header()
		for _, value := range values {
			target.Add(key, value)
		}
	}
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	w.writer.WriteHeader(status)
	w.sent = true
}

func (a *webApp) withModernUI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/chat/stream") {
			next.ServeHTTP(w, r)
			return
		}
		writer := newHTMLStyleWriter(w)
		next.ServeHTTP(writer, r)
		if writer.passthrough {
			return
		}

		body := writer.body.Bytes()
		contentType := strings.ToLower(strings.TrimSpace(writer.header.Get("Content-Type")))
		if shouldInjectModernUI(contentType, body) {
			body = []byte(injectModernUI(string(body)))
			writer.header.Del("Content-Length")
		}

		writer.sendHeaders()
		if len(body) > 0 {
			_, _ = w.Write(body)
		}
	})
}

func shouldInjectModernUI(contentType string, body []byte) bool {
	if strings.Contains(contentType, "application/json") || strings.Contains(contentType, "text/event-stream") {
		return false
	}
	if strings.Contains(contentType, "text/html") {
		return true
	}
	trimmed := strings.ToLower(strings.TrimSpace(string(body)))
	return strings.HasPrefix(trimmed, "<!doctype html") || strings.HasPrefix(trimmed, "<html")
}

func injectModernUI(page string) string {
	if strings.Contains(page, `id="wolfbbs-modern-ui"`) {
		return page
	}
	lower := strings.ToLower(page)
	if idx := strings.Index(lower, "</head>"); idx >= 0 {
		return page[:idx] + modernUIBootstrap + page[idx:]
	}
	if bodyIdx := strings.Index(lower, "<body"); bodyIdx >= 0 {
		rest := lower[bodyIdx:]
		if end := strings.Index(rest, ">"); end >= 0 {
			insertAt := bodyIdx + end + 1
			return page[:insertAt] + modernUIBootstrap + page[insertAt:]
		}
	}
	return modernUIBootstrap + page
}

func (a *webApp) activityPubBase(r *http.Request) string {
	base := strings.TrimSpace(a.apBaseURL)
	if base != "" {
		return strings.TrimSuffix(base, "/")
	}
	scheme := "http"
	if r != nil {
		if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
			scheme = proto
		} else if r.TLS != nil {
			scheme = "https"
		}
		if host := strings.TrimSpace(r.Host); host != "" {
			return scheme + "://" + host
		}
	}
	return scheme + "://localhost"
}

func (a *webApp) deliverPasswordReset(r *http.Request, handle, token string) error {
	if a.resetNotifier != nil {
		return a.resetNotifier(handle, token, r)
	}
	if a.email == nil || !a.email.Enabled() {
		return nil
	}
	recipient := passwordResetRecipient(handle)
	if recipient == "" {
		return nil
	}
	base := strings.TrimSpace(a.publicBaseURL)
	if base == "" {
		base = a.activityPubBase(r)
	}
	base = strings.TrimRight(base, "/")
	resetURL := base + "/reset/complete?token=" + url.QueryEscape(strings.TrimSpace(token))
	subject := a.siteDisplayName() + " password reset"
	body := "A password reset was requested for your " + a.siteDisplayName() + " account.\n\n" +
		"If this was you, open this link to set a new password:\n" + resetURL + "\n\n" +
		"If you did not request this reset, you can ignore this message."
	return a.email.SendOutbound("wolfbbs-reset", []string{recipient}, subject, body)
}

func passwordResetRecipient(handle string) string {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return ""
	}
	addr, err := mail.ParseAddress(handle)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(addr.Address)
}

func (a *webApp) handleConnect(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.currentUser(r); ok {
		http.Redirect(w, r, "/boards", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	wsURL := strings.TrimSpace(a.wsTerminalURL)
	if wsURL == "" {
		wsURL = "ws://localhost:6080/ws-login"
	}
	termBlock := `<p>WebSocket terminal is configured for command-mode login server.</p>`
	if a.modernOnRamp {
		termBlock += `<pre id="term" style="height:220px; width:780px; border:1px solid #333; overflow:auto; background:#111; color:#9f9; padding:8px; font-family:monospace;"></pre>
<form id="termForm">
<label>Input: <input id="termInput" size="80" autocomplete="off"></label>
<button type="submit">Send</button>
</form>
<script>
(function(){
const out = document.getElementById('term');
const input = document.getElementById('termInput');
const form = document.getElementById('termForm');
let ws;
function append(line){
  out.textContent += line + "\n";
  out.scrollTop = out.scrollHeight;
}
function connect(){
  ws = new WebSocket(` + fmt.Sprintf("%q", wsURL) + `);
  ws.onopen = function(){ append("[connected] " + ` + fmt.Sprintf("%q", wsURL) + `); };
  ws.onmessage = function(evt){ append(evt.data); };
  ws.onclose = function(){ append("[disconnected]"); };
  ws.onerror = function(){ append("[error] websocket failure"); };
}
form.addEventListener('submit', function(evt){
  evt.preventDefault();
  if (!ws || ws.readyState !== 1) { append("[offline] reconnecting"); connect(); return; }
  const v = input.value;
  if (!v) return;
  ws.send(v);
  input.value = "";
});
connect();
})();
</script>`
	}

	tourLink := ""
	if a.guestTour {
		tourLink = `<p><a href="/tour">Enter guided guest tour (read-only)</a></p>`
	}
	motdBlock := ""
	if strings.TrimSpace(a.motd) != "" {
		motdBlock = `<p><strong>MOTD:</strong> ` + htmlEscape(a.motd) + `</p>`
	}
	announcementBlock := ""
	if strings.TrimSpace(a.announcement) != "" {
		announcementBlock = `<p><strong>Announcement:</strong> ` + htmlEscape(a.announcement) + `</p>`
	}
	connectHost := a.siteHost()
	sshPort := parseInt(strings.TrimSpace(os.Getenv("WOLFBBS_SSH_PORT")), 2222)
	telnetPort := parseInt(strings.TrimSpace(os.Getenv("WOLFBBS_TELNET_PORT")), 2323)

	page := `<html><body>
<h1>` + htmlEscape(a.siteDisplayName()) + ` Connect</h1>
<p>Terminal-first remains the primary UX.</p>
` + motdBlock + `
` + announcementBlock + `
<ul>
<li>SSH (recommended): <code>ssh ` + htmlEscape(connectHost) + ` -p ` + strconv.Itoa(sshPort) + `</code></li>
<li>Telnet (optional): <code>telnet ` + htmlEscape(connectHost) + ` ` + strconv.Itoa(telnetPort) + `</code></li>
<li>WebSocket login endpoint: <code>` + htmlEscape(wsURL) + `</code></li>
</ul>
<p><a href="/login">Sign in with account</a> | <a href="/help">help</a></p>
` + tourLink + `
` + termBlock + `
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleGuestTour(w http.ResponseWriter, r *http.Request) {
	if !a.guestTour {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	lastCallers := a.latestLogins(6)
	if len(lastCallers) == 0 {
		lastCallers = append(lastCallers, "No caller history yet.")
	}
	oneLiners := []string{}
	if a.chatSvc != nil {
		for _, row := range a.chatSvc.History("#lobby", 5) {
			oneLiners = append(oneLiners, fmt.Sprintf("[%s] %s: %s",
				row.CreatedAt.Local().Format("15:04"),
				row.From,
				cleanOneLiner(row.Body, 70)))
		}
	}
	if len(oneLiners) == 0 {
		oneLiners = append(oneLiners, "No one-liners yet.")
	}

	feature := "No featured thread yet."
	if headline := a.featuredThreadLine(); headline != "" {
		feature = headline
	}
	downloadPick := a.filebaseDownloadPick()

	rows := strings.Builder{}
	for _, line := range lastCallers {
		rows.WriteString(`<li>` + htmlEscape(line) + `</li>`)
	}
	chatRows := strings.Builder{}
	for _, line := range oneLiners {
		chatRows.WriteString(`<li>` + htmlEscape(line) + `</li>`)
	}

	page := `<html><body>
<h1>` + htmlEscape(a.siteDisplayName()) + ` Guided Tour (Read-Only)</h1>
<p><a href="/connect">connect</a> | <a href="/login">login</a> | <a href="/help">help</a></p>
<h2>Last Callers</h2><ul>` + rows.String() + `</ul>
<h2>One-Liners</h2><ul>` + chatRows.String() + `</ul>
<h2>Featured Thread</h2><p>` + htmlEscape(feature) + `</p>
<h2>Today's Download Pick</h2><p>` + htmlEscape(downloadPick) + `</p>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
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
		_, _ = w.Write([]byte(loginPage(a.siteDisplayName(), r.URL.Path, a.modernOnRamp, a.guestTour)))
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
			Secure:   a.secureCookie,
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

func (a *webApp) handlePasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resetRequestPage(a.siteDisplayName(), "")))
		return
	case http.MethodPost:
		handle := strings.TrimSpace(r.FormValue("handle"))
		token, err := a.authSvc.IssuePasswordReset(handle, a.resetTTL)
		message := "If the account exists, a password reset token has been issued."
		if err != nil && err != auth.ErrInvalidCredentials {
			message = "Password reset is currently unavailable."
		}
		if err == nil && token != "" {
			if notifyErr := a.deliverPasswordReset(r, handle, token); notifyErr != nil {
				log.Printf("password reset delivery failed for handle=%s: %v", handle, notifyErr)
			}
		}
		if a.showResetDev && token != "" {
			message = message + " Dev token: " + token
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resetRequestPage(a.siteDisplayName(), htmlEscape(message))))
		return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func (a *webApp) handlePasswordResetComplete(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		token := strings.TrimSpace(r.URL.Query().Get("token"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resetCompletePage(a.siteDisplayName(), token, "")))
		return
	case http.MethodPost:
		token := strings.TrimSpace(r.FormValue("token"))
		password := strings.TrimSpace(r.FormValue("password"))
		if err := a.authSvc.ResetPasswordWithToken(token, password); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(resetCompletePage(a.siteDisplayName(), token, "Reset token is invalid/expired or password is too short.")))
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
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

func (a *webApp) handleHelp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, _ := a.currentUser(r)
	roleLabel := "guest"
	nav := `<a href="/login">login</a> | <a href="/connect">connect</a>`
	if user != nil {
		roleLabel = rbac.NormalizeRole(user.Role)
		nav = `<a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/settings">settings</a> | <a href="/status">status</a> | <a href="/config">config</a>`
		if a.discover {
			nav += ` | <a href="/discover">discover</a>`
		}
		if a.hasRole(user, roleAdmin) {
			nav += ` | <a href="/admin">admin</a>`
		}
		nav += ` | <a href="/logout">logout</a>`
	}

	page := `<html><body>
<h1>` + htmlEscape(a.siteDisplayName()) + ` Help</h1>
<p>` + nav + `</p>
<p>Current role: ` + htmlEscape(roleLabel) + `</p>
<h2>Terminal (SSH) quick keys</h2>
<ul>
<li>Main menu: M/P/F/C/G/D/N/S/A/L/W, Q quits, ? opens contextual help.</li>
<li>Boards reader: R reply, N next, P previous, Q exit, Space/Enter for paging.</li>
<li>Mail: C compose, R read by ID, Q return.</li>
<li>Chat: S send, J join, O online, R refresh, Q return.</li>
<li>Gateway: E email relay, W text web fetch, Q return.</li>
</ul>
<h2>Web routes</h2>
<ul>
<li>/boards, /mail, /chat, /settings, /gateway, /status, /config</li>
<li>/scores for door leaderboards</li>
<li>/healthz, /readyz, /metrics, /statusz for health/ops checks</li>
</ul>
	<h2>Admin routes (sysop only)</h2>
	<ul>
	<li>/admin/users, /admin/boards, /admin/mail, /admin/files, /admin/gateways</li>
	<li>/admin/chat, /admin/doors, /admin/setup, /admin/config, /admin/system, /admin/errors, /admin/audit</li>
	<li>Setup wizard path: /admin/setup?step=1 (Identity), step=2 (Safety), step=3 (Experience), step=4 (Bootstrap)</li>
	<li>Runtime service settings (telnet/ws/wss/content/connectors): /admin/config</li>
	</ul>
<h2>Reference docs</h2>
<ul>
<li><code>docs/help-guides.md</code></li>
<li><code>docs/INSTALL.md</code></li>
<li><code>docs/config-reference.md</code></li>
<li><code>docs/feature-reference.md</code></li>
</ul>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
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
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		if action == "" {
			action = "post"
		}
		boardID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("board_id")), 10, 64)
		boardPath := "/boards"
		if boardID > 0 {
			boardPath = fmt.Sprintf("/boards?board=%d", boardID)
		}
		switch action {
		case "report":
			messageID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("message_id")), 10, 64)
			reason := strings.TrimSpace(r.FormValue("reason"))
			if messageID <= 0 {
				redirectWithError(w, r, boardPath, "Message ID is required for reporting.")
				return
			}
			msg, err := a.msgRepo.GetMessage(messageID)
			if err != nil || msg == nil {
				redirectWithError(w, r, boardPath, "Message not found.")
				return
			}
			board, err := a.boardRepo.Get(msg.BoardID)
			if err != nil || board == nil {
				redirectWithError(w, r, "/boards", "Board not found for that message.")
				return
			}
			if !a.canReadBoard(user, board) {
				http.Error(w, "report denied by board ACS", http.StatusForbidden)
				return
			}
			if reason == "" {
				reason = "reported from web reader"
			}
			if err := a.msgRepo.CreateReport(&domain.MessageReport{
				MessageID:  messageID,
				ReporterID: user.ID,
				Reason:     reason,
				Status:     "open",
			}); err != nil {
				http.Error(w, "could not create report", http.StatusInternalServerError)
				return
			}
			if a.eventBus != nil {
				a.eventBus.Publish("message.reported", map[string]string{
					"user":    user.Handle,
					"board":   strconv.FormatInt(msg.BoardID, 10),
					"message": strconv.FormatInt(messageID, 10),
				})
			}
			redirectWithNotice(w, r, fmt.Sprintf("/boards?board=%d&id=%d", msg.BoardID, messageID), "Report submitted to moderation queue.")
		case "post":
			parentID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("parent_id")), 10, 64)
			subject := strings.TrimSpace(r.FormValue("subject"))
			body := strings.TrimSpace(r.FormValue("body"))
			if boardID <= 0 || subject == "" || body == "" {
				redirectWithError(w, r, boardPath, "Board, subject, and body are required.")
				return
			}
			board, err := a.boardRepo.Get(boardID)
			if err != nil || board == nil {
				redirectWithError(w, r, "/boards", "Board not found.")
				return
			}
			if !a.canWriteBoard(user, board) {
				http.Error(w, "posting denied by board ACS", http.StatusForbidden)
				return
			}
			if err := a.msgRepo.CreateMessage(&domain.Message{
				BoardID:  boardID,
				AuthorID: user.ID,
				ParentID: parentID,
				Subject:  subject,
				Body:     body,
			}); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "locked") {
					redirectWithError(w, r, boardPath, "Thread is locked.")
					return
				}
				redirectWithError(w, r, boardPath, "Could not create message. Please retry.")
				return
			}
			if a.eventBus != nil {
				a.eventBus.Publish("message.posted", map[string]string{
					"user":    user.Handle,
					"board":   strconv.FormatInt(boardID, 10),
					"subject": subject,
				})
			}
			redirectWithNotice(w, r, fmt.Sprintf("/boards?board=%d", boardID), "Message posted.")
		default:
			redirectWithError(w, r, boardPath, "Unsupported board action.")
		}
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if a.quickJump {
		if jump := strings.TrimSpace(r.URL.Query().Get("jump")); jump != "" {
			if dest := webQuickJumpPath(jump); dest != "" {
				http.Redirect(w, r, dest, http.StatusFound)
				return
			}
		}
	}

	boardID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("board")), 10, 64)
	conferenceFilter := strings.TrimSpace(r.URL.Query().Get("conference"))
	if strings.EqualFold(conferenceFilter, "all") {
		conferenceFilter = ""
	}
	if boardID <= 0 {
		boards, err := a.boardRepo.List()
		if err != nil {
			http.Error(w, "failed to load boards", http.StatusInternalServerError)
			return
		}
		visibleBoards := make([]domain.Board, 0, len(boards))
		conferenceSet := map[string]struct{}{}
		for _, board := range boards {
			if a.canReadBoard(user, &board) {
				conferenceSet[defaultConferenceValue(board.Conference)] = struct{}{}
				visibleBoards = append(visibleBoards, board)
			}
		}
		boards = make([]domain.Board, 0, len(visibleBoards))
		for _, board := range visibleBoards {
			if conferenceFilter != "" && !strings.EqualFold(defaultConferenceValue(board.Conference), conferenceFilter) {
				continue
			}
			boards = append(boards, board)
		}
		conferences := make([]string, 0, len(conferenceSet))
		for row := range conferenceSet {
			conferences = append(conferences, row)
		}
		sort.Slice(conferences, func(i, j int) bool { return strings.ToLower(conferences[i]) < strings.ToLower(conferences[j]) })
		rows := strings.Builder{}
		messageBlock := pageMessageBlock(r)
		motdBlock := ""
		if strings.TrimSpace(a.motd) != "" {
			motdBlock = `<p><strong>MOTD:</strong> ` + htmlEscape(a.motd) + `</p>`
		}
		announcementBlock := ""
		if strings.TrimSpace(a.announcement) != "" {
			announcementBlock = `<p><strong>Announcement:</strong> ` + htmlEscape(a.announcement) + `</p>`
		}
		discoverLink := ""
		if a.discover {
			discoverLink = ` | <a href="/discover">discover</a>`
		}
		for _, board := range boards {
			msgs, _ := a.msgRepo.ListByBoard(board.ID)
			pointerID := int64(0)
			if ptr, ptrErr := a.msgRepo.GetPointer(user.ID, board.ID); ptrErr == nil && ptr != nil {
				pointerID = ptr.LastReadID
			}
			newCount := 0
			for _, msg := range msgs {
				if msg.ID > pointerID {
					newCount++
				}
			}
			lastAt := ""
			lastSub := ""
			if len(msgs) > 0 {
				last := msgs[len(msgs)-1]
				lastAt = last.CreatedAt.Format("2006-01-02 15:04")
				lastSub = last.Subject
			}
			rows.WriteString(fmt.Sprintf(`<tr><td>%d</td><td><a href="/boards?board=%d">%s</a></td><td>%s</td><td>%d</td><td>%d</td><td>%s</td><td>%s</td></tr>`,
				board.ID, board.ID, board.Name, htmlEscape(defaultConferenceValue(board.Conference)), len(msgs), newCount, lastAt, htmlEscape(lastSub)))
		}
		quickJumpBlock := ""
		if a.quickJump {
			quickJumpBlock = `<form method="GET" action="/boards"><label>Quick Jump <input name="jump" size="24" placeholder="mail/chat/gateway/status/config"></label><button type="submit">Go</button></form>`
		}
		confOptions := strings.Builder{}
		selectedAll := ` selected`
		if conferenceFilter != "" {
			selectedAll = ``
		}
		confOptions.WriteString(`<option value=""` + selectedAll + `>All conferences</option>`)
		for _, conf := range conferences {
			selected := ""
			if strings.EqualFold(conf, conferenceFilter) {
				selected = ` selected`
			}
			confOptions.WriteString(`<option value="` + htmlEscape(conf) + `"` + selected + `>` + htmlEscape(conf) + `</option>`)
		}
		confFilterBlock := `<form method="GET" action="/boards"><label>Conference <select name="conference">` + confOptions.String() + `</select></label><button type="submit">Filter</button></form>`
		page := fmt.Sprintf(`<html><body>
<p>Signed in as %s</p>
<p><a href="/mail">mail</a> | <a href="/settings">settings</a> | <a href="/chat">chat</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/gateway">gateway</a>%s | <a href="/help">help</a> | <a href="/logout">logout</a></p>
%s
%s
%s
%s
%s
<p><strong>Tip:</strong> Select a board to read, then open a message ID to reply/report. Use conference filter to keep scans short.</p>
<h1>Message Boards</h1>
<table border="1">
<tr><th>ID</th><th>Board</th><th>Conf</th><th>Topics</th><th>New</th><th>Last</th><th>Last subject</th></tr>%s</table>
</body></html>`, user.Handle, discoverLink, messageBlock, motdBlock, announcementBlock, quickJumpBlock, confFilterBlock, rows.String())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page))
		return
	}

	board, err := a.boardRepo.Get(boardID)
	if err != nil {
		http.Error(w, "board not found", http.StatusNotFound)
		return
	}
	if !a.canReadBoard(user, board) {
		http.Error(w, "board read denied by ACS", http.StatusForbidden)
		return
	}
	msgs, err := a.msgRepo.ListByBoard(boardID)
	if err != nil {
		http.Error(w, "failed to load messages", http.StatusInternalServerError)
		return
	}
	pointerID := int64(0)
	if ptr, ptrErr := a.msgRepo.GetPointer(user.ID, boardID); ptrErr == nil && ptr != nil {
		pointerID = ptr.LastReadID
	}
	handleByID := a.userHandleLookup()
	csrf := a.csrfHiddenInput(r)
	messageID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
	view := strings.Builder{}
	if messageID > 0 {
		msg, err := a.msgRepo.GetMessage(messageID)
		if err == nil && msg.BoardID == boardID {
			_ = a.msgRepo.SetPointer(user.ID, boardID, msg.ID, time.Now().UTC())
			view.WriteString(`<h2>Reader</h2>`)
			view.WriteString(`<p><strong>Subject:</strong> ` + htmlEscape(msg.Subject) + `<br>`)
			view.WriteString(`<strong>From:</strong> ` + htmlEscape(handleByID[msg.AuthorID]) + `<br>`)
			if msg.ParentID > 0 {
				view.WriteString(`<strong>Reply-To:</strong> #` + strconv.FormatInt(msg.ParentID, 10) + `<br>`)
			}
			view.WriteString(`<strong>When:</strong> ` + msg.CreatedAt.Format("2006-01-02 15:04:05") + `</p>`)
			view.WriteString(`<pre>` + htmlEscape(msg.Body) + `</pre>`)
			view.WriteString(`<h3>Report</h3>`)
			view.WriteString(`<form method="POST" action="/boards"><input type="hidden" name="action" value="report"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `"><input type="hidden" name="message_id" value="` + strconv.FormatInt(msg.ID, 10) + `">` + csrf)
			view.WriteString(`<label>Reason: <input name="reason" size="48" placeholder="spam, abuse, off-topic"></label> <button type="submit">Report Post</button></form>`)
			if a.canWriteBoard(user, board) {
				view.WriteString(`<h3>Reply</h3>`)
				view.WriteString(`<form method="POST" action="/boards"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `"><input type="hidden" name="parent_id" value="` + strconv.FormatInt(msg.ID, 10) + `">` + csrf)
				view.WriteString(`<label>Subject: <input name="subject" value="Re: ` + htmlEscape(msg.Subject) + `" size="60"></label><br>`)
				view.WriteString(`<label>Body:<br><textarea name="body" rows="10" cols="80">` + htmlEscape(quoteBody(msg.Body)) + `</textarea></label><br>`)
				view.WriteString(`<button type="submit">Post Reply</button></form>`)
			} else {
				view.WriteString(`<p><em>Replying is disabled by board ACS policy.</em></p>`)
			}
		}
	}

	rows := strings.Builder{}
	for _, msg := range msgs {
		subject := msg.Subject
		if msg.ParentID > 0 {
			subject = "> " + subject
		}
		newMark := ""
		if msg.ID > pointerID {
			newMark = "N"
		}
		rows.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td><a href="/boards?board=%d&id=%d">%s</a></td><td>%s</td><td>%s</td></tr>`,
			msg.ID, newMark, boardID, msg.ID, htmlEscape(subject), htmlEscape(handleByID[msg.AuthorID]), msg.CreatedAt.Format("2006-01-02 15:04")))
	}
	discoverLink := ""
	if a.discover {
		discoverLink = ` | <a href="/discover">discover</a>`
	}
	messageBlock := pageMessageBlock(r)
	motdBlock := ""
	if strings.TrimSpace(a.motd) != "" {
		motdBlock = `<p><strong>MOTD:</strong> ` + htmlEscape(a.motd) + `</p>`
	}
	announcementBlock := ""
	if strings.TrimSpace(a.announcement) != "" {
		announcementBlock = `<p><strong>Announcement:</strong> ` + htmlEscape(a.announcement) + `</p>`
	}
	page := `<html><body><h1>Board: ` + htmlEscape(board.Name) + `</h1>` +
		`<p><a href="/boards">all boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/status">status</a> | <a href="/config">config</a>` + discoverLink + ` | <a href="/help">help</a> | <a href="/logout">logout</a></p>` +
		messageBlock +
		`<p><strong>Reader keys:</strong> open subject to read, use Reply form, and Report for abuse/moderation queue.</p>` +
		`<p><strong>Conference:</strong> ` + htmlEscape(defaultConferenceValue(board.Conference)) + `</p>` +
		motdBlock + announcementBlock +
		`<table border="1"><tr><th>ID</th><th>New</th><th>Subject</th><th>Author</th><th>When</th></tr>` + rows.String() + `</table>`
	if a.canWriteBoard(user, board) {
		page += `<h3>New Post</h3><form method="POST" action="/boards"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `">` + csrf +
			`<label>Subject: <input name="subject" size="60"></label><br><label>Body:<br><textarea name="body" rows="10" cols="80"></textarea></label><br><button type="submit">Post</button></form>`
	} else {
		page += `<p><em>Posting is disabled by board ACS policy.</em></p>`
	}
	page +=
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
	if !a.canReadMail(user) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodPost {
		if !a.requireCSRF(w, r) {
			return
		}
		if !a.canSendMail(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		toRaw := strings.TrimSpace(r.FormValue("to"))
		subject := strings.TrimSpace(r.FormValue("subject"))
		body := strings.TrimSpace(r.FormValue("body"))
		if toRaw == "" || subject == "" || body == "" {
			redirectWithError(w, r, "/mail", "To, subject, and body are required.")
			return
		}

		msg := &domain.PrivateMail{
			FromUserID: user.ID,
			Subject:    subject,
			Body:       body,
		}
		if strings.Contains(toRaw, "@") {
			if a.requireVerifiedEmail && !user.Verified {
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
				redirectWithError(w, r, "/mail", "Email relay error: "+err.Error())
				return
			}
		} else {
			target, err := a.authSvc.GetUser(toRaw)
			if err != nil || target == nil {
				redirectWithError(w, r, "/mail", "Unknown recipient handle.")
				return
			}
			msg.ToUserID = target.ID
		}
		if err := a.mailRepo.CreateMail(msg); err != nil {
			redirectWithError(w, r, "/mail", "Could not save mail. Please retry.")
			return
		}
		if a.eventBus != nil {
			a.eventBus.Publish("mail.sent", map[string]string{
				"user": user.Handle,
				"to":   toRaw,
			})
		}
		redirectWithNotice(w, r, "/mail", "Mail sent to "+toRaw+".")
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
			redirectWithError(w, r, "/mail", "Mail not found.")
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
		page := `<html><body><h1>Mail #` + strconv.FormatInt(item.ID, 10) + `</h1><p><a href="/mail">back</a> | <a href="/boards">boards</a> | <a href="/help">help</a></p>` +
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
	if inRows.Len() == 0 {
		inRows.WriteString(`<tr><td colspan="5">Inbox is empty.</td></tr>`)
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
	if outRows.Len() == 0 {
		outRows.WriteString(`<tr><td colspan="4">Outbox is empty.</td></tr>`)
	}
	csrf := a.csrfHiddenInput(r)
	messageBlock := pageMessageBlock(r)
	discoverLink := ""
	if a.discover {
		discoverLink = ` | <a href="/discover">discover</a>`
	}
	page := `<html><body><h1>Private Mail</h1><p><a href="/boards">boards</a> | <a href="/chat">chat</a> | <a href="/status">status</a> | <a href="/config">config</a>` + discoverLink + ` | <a href="/help">help</a> | <a href="/logout">logout</a></p>` +
		messageBlock +
		`<p><strong>Tip:</strong> Use handle for local mail, email address for external relay (if enabled by policy).</p>` +
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
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	downloadToken := strings.TrimSpace(r.URL.Query().Get("download"))
	fileView := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("view")), "files")
	if r.Method == http.MethodGet {
		if downloadToken != "" && !a.canReadFiles(user, "download") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if a.serveGatewayDownload(w, r, user, downloadToken) {
			return
		}
		if fileView {
			if !a.canReadFiles(user, "browse") {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if a.serveGatewayBatchZip(w, r, user) {
				return
			}
			a.renderGatewayFiles(w, r, user)
			return
		}
		csrf := a.csrfHiddenInput(r)
		messageBlock := pageMessageBlock(r)
		page := `<html><body>
	<h1>Gateway</h1>
	<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
	` + messageBlock + `
	<p>Fetch readable text via text gateway (safety limits and SSRF blocks apply).</p>
	<form method="POST" action="/gateway">
		` + csrf + `
		<input type="hidden" name="action" value="fetch">
		<label>URL: <input name="url" size="60" value="https://"></label><br><br>
		<label><input type="checkbox" name="save" value="1"> Save for offline reading</label><br><br>
		<button type="submit">Fetch</button>
	</form>
	<p><a href="/gateway?view=files">FileBase browser + queue + temp links</a></p>
	</body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page))
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !a.requireCSRF(w, r) {
		return
	}
	if strings.TrimSpace(strings.ToLower(r.FormValue("action"))) != "fetch" && !a.canReadFiles(user, "manage") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if a.handleGatewayFileAction(w, r, user) {
		return
	}
	url := strings.TrimSpace(r.FormValue("url"))
	if url == "" {
		redirectWithError(w, r, "/gateway", "Enter a URL before fetching.")
		return
	}
	result, err := gateway.FetchText(r.Context(), url, gateway.DefaultFetchConfig)
	if err != nil {
		redirectWithError(w, r, "/gateway", "Web gateway fetch failed: "+err.Error())
		return
	}
	note := ""
	if strings.TrimSpace(r.FormValue("save")) == "1" {
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
	_, _ = w.Write([]byte(`<html><body><h1>Gateway Reader</h1><p><a href="/gateway">back</a> | <a href="/help">help</a></p><pre>` + escaped + `</pre></body></html>`))
}

func (a *webApp) handleSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if r.Method == http.MethodPost {
		if !a.requireCSRF(w, r) {
			return
		}
		action := strings.TrimSpace(strings.ToLower(r.FormValue("action")))
		notice := "Settings updated."
		switch action {
		case "change_password":
			next := strings.TrimSpace(r.FormValue("password"))
			confirm := strings.TrimSpace(r.FormValue("confirm"))
			if next == "" || confirm == "" {
				redirectWithError(w, r, "/settings", "Password and confirmation are required.")
				return
			}
			if next != confirm {
				redirectWithError(w, r, "/settings", "Passwords do not match.")
				return
			}
			if err := a.authSvc.SetPassword(user.Handle, next); err != nil {
				redirectWithError(w, r, "/settings", "Password update failed.")
				return
			}
			notice = "Password changed."
		case "update_prefs":
			theme := strings.TrimSpace(r.FormValue("theme"))
			if theme == "" {
				theme = user.Theme
			}
			ansiEnabled := parseCheckbox(r.FormValue("ansi_enabled"))
			pagingEnabled := parseCheckbox(r.FormValue("paging_enabled"))
			timeFormat24h := parseCheckbox(r.FormValue("time_format_24h"))
			if err := a.authSvc.SetPreferences(user.Handle, theme, ansiEnabled, pagingEnabled, timeFormat24h); err != nil {
				redirectWithError(w, r, "/settings", "Preference update failed.")
				return
			}
			notice = "Display preferences saved."
		case "enable_2fa":
			secret, err := auth.GenerateTOTPSecret()
			if err != nil {
				redirectWithError(w, r, "/settings", "2FA setup failed.")
				return
			}
			codes, err := auth.GenerateRecoveryCodes(8)
			if err != nil {
				redirectWithError(w, r, "/settings", "2FA setup failed.")
				return
			}
			_ = a.authSvc.SetTOTPSecret(user.Handle, secret)
			_ = a.authSvc.SetRecoveryCodes(user.Handle, codes)
			notice = "2FA enabled. Save your recovery codes."
		case "disable_2fa":
			_ = a.authSvc.SetTOTPSecret(user.Handle, "")
			_ = a.authSvc.SetRecoveryCodes(user.Handle, nil)
			notice = "2FA disabled."
		case "regen_codes":
			codes, err := auth.GenerateRecoveryCodes(8)
			if err != nil {
				redirectWithError(w, r, "/settings", "2FA setup failed.")
				return
			}
			_ = a.authSvc.SetRecoveryCodes(user.Handle, codes)
			notice = "Recovery codes regenerated."
		default:
			http.Redirect(w, r, "/settings", http.StatusFound)
			return
		}
		redirectWithNotice(w, r, "/settings", notice)
		return
	}

	csrf := a.csrfHiddenInput(r)
	messageBlock := pageMessageBlock(r)
	themeOptions := buildThemeOptionsHTML(user.Theme)
	var secondFactorBlock strings.Builder
	adminSettingsBlock := ""
	if a.hasRole(user, roleAdmin) {
		adminSettingsBlock = `<h2>Sysop Runtime Settings</h2><p><a href="/admin/setup">Setup Wizard</a> | <a href="/admin/config">Runtime Configuration</a> | <a href="/admin/system">WFC Dashboard</a></p>`
	}
	if user.TOTPSecret == "" {
		secondFactorBlock.WriteString(`<p>2FA is currently disabled.</p>`)
		secondFactorBlock.WriteString(`<form method="POST" action="/settings"><input type="hidden" name="action" value="enable_2fa">` + csrf + `<button type="submit">Enable TOTP</button></form>`)
	} else {
		secondFactorBlock.WriteString(`<p>2FA is enabled.</p>`)
		secondFactorBlock.WriteString(`<form method="POST" action="/settings"><input type="hidden" name="action" value="disable_2fa">` + csrf + `<button type="submit">Disable TOTP</button></form>`)
		secondFactorBlock.WriteString(`<form method="POST" action="/settings"><input type="hidden" name="action" value="regen_codes">` + csrf + `<button type="submit">Regenerate recovery codes</button></form>`)
		secondFactorBlock.WriteString(`<p>Recovery Codes: ` + strings.Join(user.RecoveryCodes, ", ") + `</p>`)
	}
	page := `<html><body><h1>Settings</h1><p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>` + messageBlock + `<p>User: ` + user.Handle + `</p><ul>` +
		`<li>ANSI: ` + boolToText(user.ANSIEnabled) + `</li>` +
		`<li>Paging: ` + boolToText(user.PagingEnabled) + `</li>` +
		`<li>Time format 24h: ` + boolToText(user.TimeFormat24h) + `</li>` +
		`</ul>` +
		`<h2>Display Preferences</h2><form method="POST" action="/settings"><input type="hidden" name="action" value="update_prefs">` + csrf +
		`<label>Theme: <select name="theme">` + themeOptions + `</select></label><br>` +
		`<label><input type="checkbox" name="ansi_enabled" value="1" ` + checkedAttr(user.ANSIEnabled) + `> ANSI enabled</label><br>` +
		`<label><input type="checkbox" name="paging_enabled" value="1" ` + checkedAttr(user.PagingEnabled) + `> Paging enabled</label><br>` +
		`<label><input type="checkbox" name="time_format_24h" value="1" ` + checkedAttr(user.TimeFormat24h) + `> 24-hour time format</label><br>` +
		`<button type="submit">Save Preferences</button></form>` +
		`<h2>Password</h2><form method="POST" action="/settings"><input type="hidden" name="action" value="change_password">` + csrf +
		`<label>New password: <input name="password" type="password"></label><br>` +
		`<label>Confirm: <input name="confirm" type="password"></label><br><button type="submit">Change password</button></form>` +
		adminSettingsBlock +
		secondFactorBlock.String() +
		`</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleDiscover(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if !a.discover {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	const maxItems = 15
	digest, err := discovery.BuildSinceLastCall(a.boardRepo, a.msgRepo, a.mailRepo, user, maxItems)
	if err != nil {
		http.Error(w, "discover feed unavailable", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(r.URL.Query().Get("save")) == "1" {
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if query != "" && a.classicSearch {
			a.addSavedSearch(user.Handle, query)
		}
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	conferenceSummary := discovery.BuildConferenceSummary(digest.Items, 6)
	searchRows := []string{}
	saved := []string{}
	if a.classicSearch {
		searchRows = a.searchRows(query, maxItems)
		saved = a.savedSearchList(user.Handle)
	}

	itemsRows := strings.Builder{}
	for _, row := range digest.Items {
		itemsRows.WriteString(`<li>` + htmlEscape(row.Line) + `</li>`)
	}
	if itemsRows.Len() == 0 {
		itemsRows.WriteString(`<li>No new items since your last call.</li>`)
	}
	conferenceRows := strings.Builder{}
	for _, row := range conferenceSummary {
		conferenceRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	if conferenceRows.Len() == 0 {
		conferenceRows.WriteString(`<li>No area-level updates since your last call.</li>`)
	}
	searchHTML := strings.Builder{}
	savedHTML := strings.Builder{}
	if a.classicSearch {
		for _, row := range searchRows {
			searchHTML.WriteString(`<li>` + htmlEscape(row) + `</li>`)
		}
		if searchHTML.Len() == 0 {
			searchHTML.WriteString(`<li>No matches for current search.</li>`)
		}
		for _, row := range saved {
			link := "/discover?q=" + url.QueryEscape(row)
			savedHTML.WriteString(`<li><a href="` + link + `">` + htmlEscape(row) + `</a></li>`)
		}
		if savedHTML.Len() == 0 {
			savedHTML.WriteString(`<li>No saved searches yet.</li>`)
		}
	} else {
		searchHTML.WriteString(`<li>Classic search is disabled by sysop policy.</li>`)
		savedHTML.WriteString(`<li>Classic search is disabled by sysop policy.</li>`)
	}

	aiLine := ""
	if envEnabledDefault("WOLFBBS_AI_ASSIST_ENABLE", false) {
		if summary := discovery.BuildAICatchUpLine(digest.Items); summary != "" {
			aiLine = `<p><strong>` + htmlEscape(summary) + `</strong></p>`
		}
	}
	rumorLine := ""
	if a.rumorzMod != nil {
		if rumor := strings.TrimSpace(a.rumorzMod.Current()); rumor != "" {
			rumorLine = `<p><strong>Rumorz:</strong> ` + htmlEscape(rumor) + `</p>`
		}
	}
	oneLiners := []mods.OneLiner{}
	if a.oneLinerzMod != nil {
		oneLiners = a.oneLinerzMod.List(8)
	}
	oneLinerHTML := strings.Builder{}
	for _, row := range oneLiners {
		oneLinerHTML.WriteString(`<li>[` + row.At.Local().Format("15:04") + `] <strong>` + htmlEscape(row.Handle) + `</strong>: ` + htmlEscape(row.Text) + `</li>`)
	}
	if oneLinerHTML.Len() == 0 {
		oneLinerHTML.WriteString(`<li>No one-liners yet.</li>`)
	}
	bbsRows := []mods.BBSListing{}
	if a.bbsListMod != nil {
		bbsRows = a.bbsListMod.List(6)
	}
	bbsHTML := strings.Builder{}
	for _, row := range bbsRows {
		bbsHTML.WriteString(`<li>` + htmlEscape(row.Name) + ` (` + htmlEscape(row.Host) + `:` + strconv.Itoa(row.Port) + `)</li>`)
	}
	if bbsHTML.Len() == 0 {
		bbsHTML.WriteString(`<li>No BBS links curated yet.</li>`)
	}

	page := `<html><body>
<h1>Since Your Last Call</h1>
<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/settings">settings</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
	<p>Transparent rules: replies-to-you, handle mentions, per-board new activity, and inbox mail. Max ` + strconv.Itoa(maxItems) + ` items.</p>
	<p>Last seen: ` + digest.Since.Local().Format("2006-01-02 15:04") + `</p>
	` + aiLine + `
	` + rumorLine + `
	<h2>Area Summary</h2>
	<ul>` + conferenceRows.String() + `</ul>
	<h2>Items</h2>
	<ul>` + itemsRows.String() + `</ul>
	<h2>Deep Search</h2>
<form method="GET" action="/discover">
<label>Query: <input name="q" value="` + htmlEscape(query) + `" size="42"></label>
<button type="submit">Search</button>
<button type="submit" name="save" value="1">Save Search</button>
</form>
<ul>` + searchHTML.String() + `</ul>
<h3>Saved Searches</h3>
<ul>` + savedHTML.String() + `</ul>
<h3>OneLinerz</h3>
<ul>` + oneLinerHTML.String() + `</ul>
<h3>BBS List</h3>
<ul>` + bbsHTML.String() + `</ul>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) buildStatusSnapshot(user *domain.User) statusSnapshot {
	boardsCount := 0
	if a.boardRepo != nil {
		if boards, err := a.boardRepo.List(); err == nil {
			boardsCount = len(boards)
		}
	}
	onlineCount := 0
	channelCount := 0
	if a.chatSvc != nil {
		onlineCount = len(a.chatSvc.Online())
		channelCount = len(a.chatSvc.ListChannels())
	}
	doorCount := 0
	if a.doorRegistry != nil {
		doorCount = len(a.doorRegistry.Doors())
	}
	gatewayConfigured := false
	if a.adminRepo != nil {
		if cfg, err := a.adminRepo.GetGatewaySettings(); err == nil && cfg != nil {
			gatewayConfigured = strings.TrimSpace(cfg.SMTPHost) != "" || strings.TrimSpace(cfg.FromDomain) != ""
		}
	}
	netState := "n/a"
	netEnabled := false
	if a.networkSvc != nil {
		if status, err := a.networkSvc.Status(); err == nil {
			netEnabled = true
			netState = fmt.Sprintf("spool=%s inbound=%d outbound=%d", status.SpoolDir, status.InboundPackets, status.OutboundPackets)
		} else {
			netState = err.Error()
		}
	}
	modCount := 0
	modRunning := 0
	if a.modsManager != nil {
		snap := a.modsManager.Snapshot()
		modCount = len(snap)
		for _, row := range snap {
			if row.Running {
				modRunning++
			}
		}
	}
	role := roleUser
	handle := "unknown"
	if user != nil {
		handle = user.Handle
		role = rbac.NormalizeRole(user.Role)
	}
	checks := []statusCheck{
		{Name: "Site identity", OK: strings.TrimSpace(a.siteDisplayName()) != "" && strings.TrimSpace(a.siteHost()) != "", Detail: a.siteDisplayName() + " @ " + a.siteHost()},
		{Name: "Account role", OK: true, Detail: role},
		{Name: "Boards service", OK: a.boardRepo != nil, Detail: strconv.Itoa(boardsCount) + " boards"},
		{Name: "Chat service", OK: a.chatSvc != nil, Detail: strconv.Itoa(channelCount) + " channels, " + strconv.Itoa(onlineCount) + " online"},
		{Name: "Doors registry", OK: a.doorRegistry != nil, Detail: strconv.Itoa(doorCount) + " doors loaded"},
		{Name: "Gateway config", OK: gatewayConfigured, Detail: boolToText(gatewayConfigured)},
		{Name: "Telnet login server", OK: a.runtimeCfg.Login.Telnet.Enabled, Detail: a.runtimeCfg.Login.Telnet.Listen},
		{Name: "WebSocket login server", OK: a.runtimeCfg.Login.WebSocket.Enabled, Detail: a.runtimeCfg.Login.WebSocket.Listen + a.runtimeCfg.Login.WebSocket.Path},
		{Name: "WebSocket TLS login server", OK: a.runtimeCfg.Login.WebSocketTLS.Enabled, Detail: a.runtimeCfg.Login.WebSocketTLS.Listen + a.runtimeCfg.Login.WebSocketTLS.Path},
		{Name: "Gopher content server", OK: strings.TrimSpace(a.runtimeCfg.Content.GopherListen) != "", Detail: a.runtimeCfg.Content.GopherListen},
		{Name: "NNTP content server", OK: strings.TrimSpace(a.runtimeCfg.Content.NNTPListen) != "", Detail: a.runtimeCfg.Content.NNTPListen},
		{Name: "NNTPS content server", OK: strings.TrimSpace(a.runtimeCfg.Content.NNTPSListen) != "", Detail: a.runtimeCfg.Content.NNTPSListen},
		{Name: "Message network spool", OK: netEnabled, Detail: netState},
		{Name: "DoorParty connector", OK: a.runtimeCfg.Connectors.DoorParty.Enabled, Detail: boolToText(a.runtimeCfg.Connectors.DoorParty.Enabled)},
		{Name: "BBSLink connector", OK: a.runtimeCfg.Connectors.BBSLink.Enabled, Detail: boolToText(a.runtimeCfg.Connectors.BBSLink.Enabled)},
		{Name: "Telnet bridge connector", OK: a.runtimeCfg.Connectors.Telnet.Enabled, Detail: boolToText(a.runtimeCfg.Connectors.Telnet.Enabled)},
		{Name: "ACS strict mode", OK: a.runtimeCfg.ACS.Strict, Detail: boolToText(a.runtimeCfg.ACS.Strict)},
		{Name: "ActivityPub bridge", OK: a.runtimeCfg.ActivityPub.Enabled, Detail: a.runtimeCfg.ActivityPub.BaseURL},
		{Name: "Trusted proxies configured", OK: strings.TrimSpace(a.runtimeCfg.Login.TrustedProxies) != "", Detail: a.runtimeCfg.Login.TrustedProxies},
		{Name: "HJSON menu runtime", OK: a.runtimeCfg.Menu.Enabled, Detail: a.runtimeCfg.Menu.File},
		{Name: "Built-in mods", OK: modCount > 0, Detail: fmt.Sprintf("%d total / %d running", modCount, modRunning)},
		{Name: "Discover feed", OK: a.discover, Detail: "flag: discover"},
		{Name: "Guest tour", OK: a.guestTour, Detail: "flag: guest tour"},
		{Name: "Web on-ramp", OK: a.modernOnRamp, Detail: "flag: connect/tour pages"},
		{Name: "Quick jump", OK: a.quickJump, Detail: "opt-in feature flag"},
		{Name: "Classic search", OK: a.classicSearch, Detail: "opt-in feature flag"},
		{Name: "Secure cookie", OK: a.secureCookie, Detail: "web session cookie security"},
		{Name: "Verified required for external email", OK: a.requireVerifiedEmail, Detail: "mail gateway protection"},
		{Name: "Read-only mode", OK: a.readOnly, Detail: "write operations blocked when true"},
	}
	pass := 0
	for _, row := range checks {
		if row.OK {
			pass++
		}
	}
	recommendations := make([]string, 0, 6)
	if !gatewayConfigured {
		recommendations = append(recommendations, "Configure SMTP relay and gateway limits in /admin/gateways.")
	}
	if !a.secureCookie {
		recommendations = append(recommendations, "Enable secure cookie mode in /admin/setup (Step 2) before public deployment.")
	}
	if !a.requireVerifiedEmail {
		recommendations = append(recommendations, "Require verified accounts for outbound external email in /admin/setup or /admin/config.")
	}
	if !a.runtimeCfg.ACS.Strict {
		recommendations = append(recommendations, "Enable ACS strict mode in /admin/config for tighter authorization defaults.")
	}
	if !a.runtimeCfg.Login.WebSocketTLS.Enabled {
		recommendations = append(recommendations, "Enable WSS login transport in /admin/config for browser terminal security.")
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "No immediate issues detected. Continue monitoring /metrics and /admin/system.")
	}
	return statusSnapshot{
		GeneratedAt: time.Now().UTC(),
		Site:        a.siteDisplayName(),
		Host:        a.siteHost(),
		User:        handle,
		Role:        role,
		Summary: statusSummary{
			Total: len(checks),
			Pass:  pass,
			Warn:  len(checks) - pass,
		},
		Checks:          checks,
		Recommendations: recommendations,
	}
}

func (a *webApp) handleStatusCenter(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	snapshot := a.buildStatusSnapshot(user)
	adminLink := ""
	if a.hasRole(user, roleAdmin) {
		adminLink = ` | <a href="/admin/system">sysop system</a>`
	}
	rows := strings.Builder{}
	for _, row := range snapshot.Checks {
		rows.WriteString(statusRow(row.Name, row.OK, row.Detail))
	}
	recoRows := strings.Builder{}
	for _, row := range snapshot.Recommendations {
		recoRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	page := `<html><body><h1>Status Center</h1>` +
		`<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/config">config</a> | <a href="/settings">settings</a> | <a href="/help">help</a> | <a href="/logout">logout</a>` + adminLink + `</p>` +
		`<p><strong>Summary:</strong> ` + strconv.Itoa(snapshot.Summary.Pass) + `/` + strconv.Itoa(snapshot.Summary.Total) + ` PASS, ` + strconv.Itoa(snapshot.Summary.Warn) + ` WARN | generated ` + snapshot.GeneratedAt.Local().Format("2006-01-02 15:04:05") + `</p>` +
		`<p><a href="/statusz">Machine-readable status JSON (/statusz)</a></p>` +
		`<h2>Checks</h2><table border="1"><tr><th>Function</th><th>State</th><th>Details</th></tr>` + rows.String() + `</table>` +
		`<h2>Recommendations</h2><ul>` + recoRows.String() + `</ul></body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleStatusJSON(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_ = writeJSON(w, http.StatusOK, a.buildStatusSnapshot(user))
}

func (a *webApp) handleConfigCenter(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	role := rbac.NormalizeRole(user.Role)
	adminLinks := ""
	if a.hasRole(user, roleAdmin) {
		adminLinks = `<h2>Sysop Configuration Directory</h2><table border="1"><tr><th>Area</th><th>Configure</th><th>Status</th></tr>` +
			`<tr><td>Identity + safety baseline</td><td><a href="/admin/setup">/admin/setup</a></td><td><a href="/admin/system">/admin/system</a></td></tr>` +
			`<tr><td>Runtime toggles + menu editor</td><td><a href="/admin/config">/admin/config</a></td><td><a href="/admin/system">/admin/system</a></td></tr>` +
			`<tr><td>Users + RBAC + verification</td><td><a href="/admin/users">/admin/users</a></td><td><a href="/admin/audit">/admin/audit</a></td></tr>` +
			`<tr><td>Boards + moderation + ACS</td><td><a href="/admin/boards">/admin/boards</a></td><td><a href="/admin/system">/admin/system</a></td></tr>` +
			`<tr><td>Mail policies</td><td><a href="/admin/mail">/admin/mail</a></td><td><a href="/admin/system">/admin/system</a></td></tr>` +
			`<tr><td>File areas + queue + tickets</td><td><a href="/admin/files">/admin/files</a></td><td><a href="/admin/system">/admin/system</a></td></tr>` +
			`<tr><td>Gateway safety limits</td><td><a href="/admin/gateways">/admin/gateways</a></td><td><a href="/admin/system">/admin/system</a></td></tr>` +
			`<tr><td>Chat channels + moderation</td><td><a href="/admin/chat">/admin/chat</a></td><td><a href="/admin/system">/admin/system</a></td></tr>` +
			`<tr><td>Doors + turns + scores</td><td><a href="/admin/doors">/admin/doors</a></td><td><a href="/admin/system">/admin/system</a></td></tr>` +
			`<tr><td>Errors + audit logs</td><td><a href="/admin/errors">/admin/errors</a> / <a href="/admin/audit">/admin/audit</a></td><td><a href="/admin/system">/admin/system</a></td></tr>` +
			`</table>`
	}
	runtimeConfigPath := strings.TrimSpace(config.ResolveConfigPath())
	if runtimeConfigPath == "" {
		runtimeConfigPath = "env-only defaults"
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Config Center</title></head><body><h1>Config Center</h1>` +
		`<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/status">status</a> | <a href="/settings">settings</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>` +
		`<p>Role: ` + htmlEscape(role) + `</p>` +
		`<p><strong>Site:</strong> ` + htmlEscape(a.siteDisplayName()) + ` (` + htmlEscape(a.siteHost()) + `)</p>` +
		`<h2>User Configuration</h2><ul>` +
		`<li>Display + ANSI + pager + 24h clock: <a href="/settings">/settings</a></li>` +
		`<li>Password + 2FA: <a href="/settings">/settings</a></li>` +
		`<li>Personal inbox/outbox and posting workflow: <a href="/mail">/mail</a> and <a href="/boards">/boards</a></li>` +
		`</ul>` +
		`<h2>Runtime Feature Flags (Current State)</h2><ul>` +
		`<li>Discover: ` + boolToText(a.discover) + `</li>` +
		`<li>Quick jump: ` + boolToText(a.quickJump) + `</li>` +
		`<li>Classic search: ` + boolToText(a.classicSearch) + `</li>` +
		`<li>Guest tour: ` + boolToText(a.guestTour) + `</li>` +
		`<li>Web on-ramp: ` + boolToText(a.modernOnRamp) + `</li>` +
		`<li>Read-only mode: ` + boolToText(a.readOnly) + `</li>` +
		`<li>Secure cookie mode: ` + boolToText(a.secureCookie) + `</li>` +
		`<li>External email requires verified account: ` + boolToText(a.requireVerifiedEmail) + `</li>` +
		`</ul>` +
		`<h2>Transport and Service Config</h2><ul>` +
		`<li>ACS strict mode: ` + boolToText(a.runtimeCfg.ACS.Strict) + `</li>` +
		`<li>Telnet login: ` + boolToText(a.runtimeCfg.Login.Telnet.Enabled) + ` (` + htmlEscape(a.runtimeCfg.Login.Telnet.Listen) + `)</li>` +
		`<li>WebSocket login: ` + boolToText(a.runtimeCfg.Login.WebSocket.Enabled) + ` (` + htmlEscape(a.runtimeCfg.Login.WebSocket.Listen+a.runtimeCfg.Login.WebSocket.Path) + `)</li>` +
		`<li>WebSocket TLS login: ` + boolToText(a.runtimeCfg.Login.WebSocketTLS.Enabled) + ` (` + htmlEscape(a.runtimeCfg.Login.WebSocketTLS.Listen+a.runtimeCfg.Login.WebSocketTLS.Path) + `)</li>` +
		`<li>Trusted proxy CIDRs: <code>` + htmlEscape(a.runtimeCfg.Login.TrustedProxies) + `</code></li>` +
		`<li>Gopher/NNTP/NNTPS: ` + htmlEscape(a.runtimeCfg.Content.GopherListen) + ` / ` + htmlEscape(a.runtimeCfg.Content.NNTPListen) + ` / ` + htmlEscape(a.runtimeCfg.Content.NNTPSListen) + `</li>` +
		`<li>Content host: ` + htmlEscape(a.runtimeCfg.Content.Host) + `</li>` +
		`<li>ActivityPub bridge: ` + boolToText(a.runtimeCfg.ActivityPub.Enabled) + ` (` + htmlEscape(a.runtimeCfg.ActivityPub.BaseURL) + `)</li>` +
		`<li>DoorParty/BBSLink/Telnet bridge: ` + boolToText(a.runtimeCfg.Connectors.DoorParty.Enabled) + ` / ` + boolToText(a.runtimeCfg.Connectors.BBSLink.Enabled) + ` / ` + boolToText(a.runtimeCfg.Connectors.Telnet.Enabled) + `</li>` +
		`<li>Connector commands: doorparty=<code>` + htmlEscape(a.runtimeCfg.Connectors.DoorParty.Command) + `</code> bbslink=<code>` + htmlEscape(a.runtimeCfg.Connectors.BBSLink.Command) + `</code> telnet=<code>` + htmlEscape(a.runtimeCfg.Connectors.Telnet.Command) + `</code></li>` +
		`<li>Network spool dir: ` + htmlEscape(func() string {
		if a.networkSvc == nil {
			return "disabled"
		}
		status, err := a.networkSvc.Status()
		if err != nil {
			return "error: " + err.Error()
		}
		return status.SpoolDir
	}()) + `</li>` +
		`<li>Built-in mods: onelinerz / rumorz / bbslist / whos_online</li>` +
		`<li>Runtime config source: <code>` + htmlEscape(runtimeConfigPath) + `</code></li>` +
		`</ul>` +
		`<h2>Easy Setup Path</h2><ol>` +
		`<li>Open <a href="/admin/setup">/admin/setup</a> to set site identity and safety baseline.</li>` +
		`<li>Use <a href="/admin/config">/admin/config</a> for runtime toggles and menu runtime.</li>` +
		`<li>Use <a href="/status">/status</a> and <a href="/admin/system">/admin/system</a> to verify health.</li>` +
		`</ol>` +
		adminLinks +
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
	errorCount := len(a.latestErrors(1000))
	page := `<html><body><h1>Sysop Control Panel</h1><p>Logged in as ` + user.Handle + `</p>` +
		`<p><a href="/admin/users">Users</a> | <a href="/admin/boards">Boards</a> | <a href="/admin/mail">Mail</a> | ` +
		`<a href="/admin/files">Files</a> | <a href="/admin/gateways">Gateways</a> | <a href="/admin/chat">Chat</a> | ` +
		`<a href="/admin/doors">Doors</a> | <a href="/admin/setup">Setup</a> | <a href="/admin/config">Config</a> | ` +
		`<a href="/admin/system">System</a> | <a href="/admin/errors">Errors</a> | <a href="/admin/audit">Audit Log</a> | <a href="/help">Help</a></p>` +
		`<ul><li>Users: list/search, disable, ban, reset passwords</li>` +
		`<li>Boards: create, edit, delete, permissions</li>` +
		`<li>Mail: audit, limit controls</li>` +
		`<li>Chat: channel state, kicks, mutes</li>` +
		`<li>Doors: per-door enable/disable, turn rules, logs, and score reset</li>` +
		`<li>Message networks: spool import/export via oputil + status in WFC</li>` +
		`<li>Built-in mods: onelinerz, rumorz, bbs list, who's online lifecycle</li>` +
		`<li>Gateway controls, setup checks, runtime config, and server health</li></ul>` +
		`<p><a href="/scores">Door Scores & Trophies</a></p>` +
		`<p>Read-only mode: ` + boolToText(a.readOnly) + ` | Runtime errors logged: ` + strconv.Itoa(errorCount) + `</p></body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminSetup(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	setupNotice := strings.TrimSpace(r.URL.Query().Get("notice"))
	setupStep := strings.TrimSpace(r.URL.Query().Get("step"))
	switch setupStep {
	case "1", "2", "3", "4":
	default:
		setupStep = "1"
	}
	if r.Method == http.MethodPost {
		if !a.requireAdminWrite(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		switch action {
		case "seed_default_boards":
			seedDefaultBoards(a.boardRepo)
			a.recordAdminAction(user.Handle, "setup", "seed_default_boards", "ran default board seeding")
		case "ensure_mailbot":
			seedServiceUsers(a.authSvc)
			a.recordAdminAction(user.Handle, "setup", "ensure_mailbot", "mailbot service account checked")
		case "save_setup_profile":
			siteName := strings.TrimSpace(r.FormValue("site_name"))
			siteHost := strings.TrimSpace(r.FormValue("site_hostname"))
			if siteName == "" {
				siteName = "WolfBBS"
			}
			if siteHost == "" {
				siteHost = "localhost"
			}
			a.siteName = siteName
			a.siteHostname = siteHost
			a.motd = strings.TrimSpace(r.FormValue("motd"))
			a.announcement = strings.TrimSpace(r.FormValue("announcement"))
			a.readOnly = formHasValue(r, "read_only")
			a.secureCookie = formHasValue(r, "secure_cookie")
			a.requireVerifiedEmail = formHasValue(r, "require_verified_email")
			a.modernOnRamp = formHasValue(r, "web_onramp")
			a.guestTour = formHasValue(r, "guest_tour")
			a.discover = formHasValue(r, "discover")
			a.quickJump = formHasValue(r, "quick_jump")
			a.classicSearch = formHasValue(r, "classic_search")
			a.persistSystemSetting(sysSettingSiteName, a.siteName)
			a.persistSystemSetting(sysSettingSiteHostname, a.siteHostname)
			a.persistSystemSetting(sysSettingMOTD, a.motd)
			a.persistSystemSetting(sysSettingAnnouncement, a.announcement)
			a.persistSystemSetting(sysSettingReadOnly, strconv.FormatBool(a.readOnly))
			a.persistSystemSetting(sysSettingSecureCookie, strconv.FormatBool(a.secureCookie))
			a.persistSystemSetting(sysSettingRequireVerifiedEmail, strconv.FormatBool(a.requireVerifiedEmail))
			a.persistSystemSetting(sysSettingWebOnRamp, strconv.FormatBool(a.modernOnRamp))
			a.persistSystemSetting(sysSettingGuestTour, strconv.FormatBool(a.guestTour))
			a.persistSystemSetting(sysSettingDiscover, strconv.FormatBool(a.discover))
			a.persistSystemSetting(sysSettingQuickJump, strconv.FormatBool(a.quickJump))
			a.persistSystemSetting(sysSettingClassicSearch, strconv.FormatBool(a.classicSearch))
			a.recordAdminAction(
				user.Handle,
				"setup",
				"save_setup_profile",
				fmt.Sprintf(
					"site=%s host=%s read_only=%t secure_cookie=%t require_verified=%t onramp=%t tour=%t discover=%t quick_jump=%t classic_search=%t",
					a.siteName,
					a.siteHostname,
					a.readOnly,
					a.secureCookie,
					a.requireVerifiedEmail,
					a.modernOnRamp,
					a.guestTour,
					a.discover,
					a.quickJump,
					a.classicSearch,
				),
			)
			http.Redirect(w, r, "/admin/setup?notice="+url.QueryEscape("Setup profile saved."), http.StatusFound)
			return
		}
		http.Redirect(w, r, "/admin/setup", http.StatusFound)
		return
	}

	users, usersErr := a.authSvc.ListUsers()
	if usersErr != nil {
		a.addAppError("admin.setup", fmt.Errorf("list users: %w", usersErr))
	}
	boards, boardsErr := a.boardRepo.List()
	if boardsErr != nil {
		a.addAppError("admin.setup", fmt.Errorf("list boards: %w", boardsErr))
	}
	gatewayConfigured := false
	if a.adminRepo != nil {
		if cfg, err := a.adminRepo.GetGatewaySettings(); err == nil && cfg != nil {
			gatewayConfigured = strings.TrimSpace(cfg.SMTPHost) != "" || strings.TrimSpace(cfg.FromDomain) != ""
		}
	}

	sysopCount := 0
	moderatorCount := 0
	serviceMailbot := false
	for _, row := range users {
		role := rbac.NormalizeRole(row.Role)
		if role == roleAdmin {
			sysopCount++
		}
		if role == roleModerator {
			moderatorCount++
		}
		if strings.EqualFold(strings.TrimSpace(row.Handle), "mailbot") {
			serviceMailbot = true
		}
	}

	healthRows := strings.Builder{}
	healthRows.WriteString(statusRow("DB users list", usersErr == nil, "auth repository reachable"))
	healthRows.WriteString(statusRow("DB boards list", boardsErr == nil, "board repository reachable"))
	healthRows.WriteString(statusRow("Sysop account", sysopCount > 0, strconv.Itoa(sysopCount)+" sysop account(s)"))
	healthRows.WriteString(statusRow("Moderator account", moderatorCount > 0, strconv.Itoa(moderatorCount)+" moderator account(s)"))
	healthRows.WriteString(statusRow("Mailbot service user", serviceMailbot, boolToText(serviceMailbot)))
	healthRows.WriteString(statusRow("Boards seeded", len(boards) > 0, strconv.Itoa(len(boards))+" board(s)"))
	healthRows.WriteString(statusRow("Gateway settings", gatewayConfigured, boolToText(gatewayConfigured)))
	healthRows.WriteString(statusRow("Inbound token configured", strings.TrimSpace(a.inboundToken) != "", boolToText(strings.TrimSpace(a.inboundToken) != "")))
	healthRows.WriteString(statusRow("Site identity configured", strings.TrimSpace(a.siteName) != "" && strings.TrimSpace(a.siteHostname) != "", a.siteDisplayName()+" @ "+a.siteHost()))
	healthRows.WriteString(statusRow("Secure cookie mode", a.secureCookie, boolToText(a.secureCookie)))
	healthRows.WriteString(statusRow("Verified required for external email", a.requireVerifiedEmail, boolToText(a.requireVerifiedEmail)))

	csrf := a.csrfHiddenInput(r)
	noticeBlock := ""
	if setupNotice != "" {
		noticeBlock = `<p><strong>` + htmlEscape(setupNotice) + `</strong></p>`
	}
	wizardHint := map[string]string{
		"1": "Step 1 of 4: set site identity, MOTD, and announcement text.",
		"2": "Step 2 of 4: apply critical safety controls (secure cookie, verified email, read-only switch).",
		"3": "Step 3 of 4: choose optional modern helpers while keeping ANSI-first defaults.",
		"4": "Step 4 of 4: run bootstrap actions and confirm health checks are green.",
	}[setupStep]
	progress := `<ol>` +
		`<li><a href="/admin/setup?step=1">Step 1: Identity</a></li>` +
		`<li><a href="/admin/setup?step=2">Step 2: Safety</a></li>` +
		`<li><a href="/admin/setup?step=3">Step 3: Experience</a></li>` +
		`<li><a href="/admin/setup?step=4">Step 4: Bootstrap</a></li>` +
		`</ol>`
	page := `<html><body><h1>Setup & Install</h1><p><a href="/admin">back</a> | <a href="/admin/system">system</a> | <a href="/help">help</a></p>` +
		`<p>Use this screen to verify base services and bootstrap sysop dependencies after install/upgrade.</p>` +
		`<p>UI-first setup: keep installer flags minimal; set board identity and runtime policy here.</p>` +
		`<h2>Setup Wizard</h2>` +
		`<p>` + htmlEscape(wizardHint) + `</p>` +
		progress +
		`<p><strong>Tip:</strong> use the step links above, then save once after each section change.</p>` +
		noticeBlock +
		`<h2>Guided Setup Profile</h2><form method="POST"><input type="hidden" name="action" value="save_setup_profile">` + csrf +
		`<fieldset><legend><strong>Step 1: Basic</strong></legend>` +
		`<label>Site name <input name="site_name" value="` + htmlEscape(a.siteDisplayName()) + `" size="32"></label><br>` +
		`<label>Hostname <input name="site_hostname" value="` + htmlEscape(a.siteHost()) + `" size="32"></label><br>` +
		`<label>MOTD<br><textarea name="motd" rows="3" cols="90">` + htmlEscape(a.motd) + `</textarea></label><br>` +
		`<label>Announcement<br><textarea name="announcement" rows="3" cols="90">` + htmlEscape(a.announcement) + `</textarea></label><br>` +
		`<small><a href="/admin/setup?step=2">Next: Safety &raquo;</a></small>` +
		`</fieldset>` +
		`<fieldset><legend><strong>Step 2: Critical</strong></legend>` +
		`<label><input type="checkbox" name="secure_cookie"` + checkedIf(a.secureCookie) + `> Secure cookie (enable behind HTTPS reverse proxy)</label><br>` +
		`<label><input type="checkbox" name="require_verified_email"` + checkedIf(a.requireVerifiedEmail) + `> Require verified account for external email gateway</label><br>` +
		`<label><input type="checkbox" name="read_only"` + checkedIf(a.readOnly) + `> Read-only maintenance mode</label><br>` +
		`<small><a href="/admin/setup?step=1">&laquo; Back</a> | <a href="/admin/setup?step=3">Next: Experience &raquo;</a></small>` +
		`</fieldset>` +
		`<fieldset><legend><strong>Step 3: Expert</strong></legend>` +
		`<label><input type="checkbox" name="web_onramp"` + checkedIf(a.modernOnRamp) + `> Enable web connect on-ramp</label><br>` +
		`<label><input type="checkbox" name="guest_tour"` + checkedIf(a.guestTour) + `> Enable guest tour</label><br>` +
		`<label><input type="checkbox" name="discover"` + checkedIf(a.discover) + `> Enable discover/newscan view</label><br>` +
		`<label><input type="checkbox" name="quick_jump"` + checkedIf(a.quickJump) + `> Enable quick jump</label><br>` +
		`<label><input type="checkbox" name="classic_search"` + checkedIf(a.classicSearch) + `> Enable classic search lists</label><br>` +
		`<small><a href="/admin/setup?step=2">&laquo; Back</a> | <a href="/admin/setup?step=4">Next: Bootstrap &raquo;</a></small>` +
		`</fieldset>` +
		`<button type="submit">Save Setup Profile</button></form>` +
		`<table border="1"><tr><th>Check</th><th>Status</th><th>Details</th></tr>` + healthRows.String() + `</table>` +
		`<h2>Step 4: Bootstrap Actions</h2>` +
		`<form method="POST"><input type="hidden" name="action" value="seed_default_boards">` + csrf + `<button type="submit">Seed Default Boards</button></form>` +
		`<form method="POST"><input type="hidden" name="action" value="ensure_mailbot">` + csrf + `<button type="submit">Ensure Mailbot Account</button></form>` +
		`<h2>Install and Ops Shortcuts</h2>` +
		`<ul>` +
		`<li>Installer docs: <code>docs/INSTALL.md</code></li>` +
		`<li>Health: <a href="/healthz">/healthz</a> and <a href="/readyz">/readyz</a></li>` +
		`<li>Metrics: <a href="/metrics">/metrics</a></li>` +
		`<li>Sysop CLI: <code>go run ./cmd/oputil status</code></li>` +
		`</ul></body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func statusRow(name string, ok bool, detail string) string {
	status := "FAIL"
	if ok {
		status = "PASS"
	}
	return `<tr><td>` + htmlEscape(name) + `</td><td>` + status + `</td><td>` + htmlEscape(detail) + `</td></tr>`
}

func (a *webApp) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	menuFile := strings.TrimSpace(r.URL.Query().Get("menu_file"))
	menuNotice := strings.TrimSpace(r.URL.Query().Get("menu_notice"))
	menuErr := ""
	menuBody := ""

	if r.Method == http.MethodPost {
		if !a.requireAdminWrite(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		switch action {
		case "save_identity":
			siteName := strings.TrimSpace(r.FormValue("site_name"))
			siteHost := strings.TrimSpace(r.FormValue("site_hostname"))
			if siteName == "" {
				siteName = "WolfBBS"
			}
			if siteHost == "" {
				siteHost = "localhost"
			}
			a.siteName = siteName
			a.siteHostname = siteHost
			a.persistSystemSetting(sysSettingSiteName, a.siteName)
			a.persistSystemSetting(sysSettingSiteHostname, a.siteHostname)
			a.recordAdminAction(user.Handle, "config", "save_identity", "site="+a.siteName+" host="+a.siteHostname)
			http.Redirect(w, r, "/admin/config", http.StatusFound)
			return
		case "save_text":
			a.motd = strings.TrimSpace(r.FormValue("motd"))
			a.announcement = strings.TrimSpace(r.FormValue("announcement"))
			a.persistSystemSetting(sysSettingMOTD, a.motd)
			a.persistSystemSetting(sysSettingAnnouncement, a.announcement)
			a.recordAdminAction(user.Handle, "config", "save_site_text", "updated motd/announcement")
			http.Redirect(w, r, "/admin/config", http.StatusFound)
			return
		case "save_security":
			a.readOnly = formHasValue(r, "read_only")
			a.secureCookie = formHasValue(r, "secure_cookie")
			a.requireVerifiedEmail = formHasValue(r, "require_verified_email")
			a.persistSystemSetting(sysSettingReadOnly, strconv.FormatBool(a.readOnly))
			a.persistSystemSetting(sysSettingSecureCookie, strconv.FormatBool(a.secureCookie))
			a.persistSystemSetting(sysSettingRequireVerifiedEmail, strconv.FormatBool(a.requireVerifiedEmail))
			a.recordAdminAction(
				user.Handle,
				"config",
				"save_security_flags",
				fmt.Sprintf("read_only=%t secure_cookie=%t require_verified=%t", a.readOnly, a.secureCookie, a.requireVerifiedEmail),
			)
			http.Redirect(w, r, "/admin/config", http.StatusFound)
			return
		case "save_flags":
			a.modernOnRamp = formHasValue(r, "web_onramp")
			a.guestTour = formHasValue(r, "guest_tour")
			a.discover = formHasValue(r, "discover")
			a.quickJump = formHasValue(r, "quick_jump")
			a.classicSearch = formHasValue(r, "classic_search")
			a.persistSystemSetting(sysSettingWebOnRamp, strconv.FormatBool(a.modernOnRamp))
			a.persistSystemSetting(sysSettingGuestTour, strconv.FormatBool(a.guestTour))
			a.persistSystemSetting(sysSettingDiscover, strconv.FormatBool(a.discover))
			a.persistSystemSetting(sysSettingQuickJump, strconv.FormatBool(a.quickJump))
			a.persistSystemSetting(sysSettingClassicSearch, strconv.FormatBool(a.classicSearch))
			a.recordAdminAction(user.Handle, "config", "save_runtime_flags", fmt.Sprintf("web_onramp=%t guest_tour=%t discover=%t quick_jump=%t classic_search=%t", a.modernOnRamp, a.guestTour, a.discover, a.quickJump, a.classicSearch))
			http.Redirect(w, r, "/admin/config", http.StatusFound)
			return
		case "save_runtime_services":
			a.runtimeCfg.ACS.Strict = formHasValue(r, "acs_strict")
			a.runtimeCfg.Content.Host = strings.TrimSpace(r.FormValue("content_host"))
			a.runtimeCfg.Content.GopherListen = strings.TrimSpace(r.FormValue("content_gopher_listen"))
			a.runtimeCfg.Content.NNTPListen = strings.TrimSpace(r.FormValue("content_nntp_listen"))
			a.runtimeCfg.Content.NNTPSListen = strings.TrimSpace(r.FormValue("content_nntps_listen"))
			a.runtimeCfg.Content.NNTPSCert = strings.TrimSpace(r.FormValue("content_nntps_cert"))
			a.runtimeCfg.Content.NNTPSKey = strings.TrimSpace(r.FormValue("content_nntps_key"))
			a.runtimeCfg.ActivityPub.Enabled = formHasValue(r, "activitypub_enabled")
			a.runtimeCfg.ActivityPub.BaseURL = strings.TrimSpace(r.FormValue("activitypub_base_url"))
			a.runtimeCfg.Login.Telnet.Enabled = formHasValue(r, "login_telnet_enabled")
			a.runtimeCfg.Login.Telnet.Listen = strings.TrimSpace(r.FormValue("login_telnet_listen"))
			a.runtimeCfg.Login.WebSocket.Enabled = formHasValue(r, "login_ws_enabled")
			a.runtimeCfg.Login.WebSocket.Listen = strings.TrimSpace(r.FormValue("login_ws_listen"))
			a.runtimeCfg.Login.WebSocket.Path = strings.TrimSpace(r.FormValue("login_ws_path"))
			a.runtimeCfg.Login.WebSocketTLS.Enabled = formHasValue(r, "login_wss_enabled")
			a.runtimeCfg.Login.WebSocketTLS.Listen = strings.TrimSpace(r.FormValue("login_wss_listen"))
			a.runtimeCfg.Login.WebSocketTLS.Path = strings.TrimSpace(r.FormValue("login_wss_path"))
			a.runtimeCfg.Login.WebSocketTLS.Cert = strings.TrimSpace(r.FormValue("login_wss_cert"))
			a.runtimeCfg.Login.WebSocketTLS.Key = strings.TrimSpace(r.FormValue("login_wss_key"))
			a.runtimeCfg.Login.TrustedProxies = strings.TrimSpace(r.FormValue("login_trusted_proxies"))
			a.runtimeCfg.Connectors.DoorParty.Enabled = formHasValue(r, "connector_doorparty_enabled")
			a.runtimeCfg.Connectors.DoorParty.Command = strings.TrimSpace(r.FormValue("connector_doorparty_command"))
			a.runtimeCfg.Connectors.DoorParty.Args = strings.TrimSpace(r.FormValue("connector_doorparty_args"))
			a.runtimeCfg.Connectors.BBSLink.Enabled = formHasValue(r, "connector_bbslink_enabled")
			a.runtimeCfg.Connectors.BBSLink.Command = strings.TrimSpace(r.FormValue("connector_bbslink_command"))
			a.runtimeCfg.Connectors.BBSLink.Args = strings.TrimSpace(r.FormValue("connector_bbslink_args"))
			a.runtimeCfg.Connectors.Telnet.Enabled = formHasValue(r, "connector_telnet_enabled")
			a.runtimeCfg.Connectors.Telnet.Command = strings.TrimSpace(r.FormValue("connector_telnet_command"))
			a.runtimeCfg.Connectors.Telnet.Args = strings.TrimSpace(r.FormValue("connector_telnet_args"))
			a.persistSystemSetting(sysSettingACSStrict, strconv.FormatBool(a.runtimeCfg.ACS.Strict))
			a.persistSystemSetting(sysSettingContentHost, a.runtimeCfg.Content.Host)
			a.persistSystemSetting(sysSettingContentGopherListen, a.runtimeCfg.Content.GopherListen)
			a.persistSystemSetting(sysSettingContentNNTPListen, a.runtimeCfg.Content.NNTPListen)
			a.persistSystemSetting(sysSettingContentNNTPSListen, a.runtimeCfg.Content.NNTPSListen)
			a.persistSystemSetting(sysSettingContentNNTPSCert, a.runtimeCfg.Content.NNTPSCert)
			a.persistSystemSetting(sysSettingContentNNTPSKey, a.runtimeCfg.Content.NNTPSKey)
			a.persistSystemSetting(sysSettingActivityPubEnabled, strconv.FormatBool(a.runtimeCfg.ActivityPub.Enabled))
			a.persistSystemSetting(sysSettingActivityPubBaseURL, a.runtimeCfg.ActivityPub.BaseURL)
			a.persistSystemSetting(sysSettingLoginTelnetEnabled, strconv.FormatBool(a.runtimeCfg.Login.Telnet.Enabled))
			a.persistSystemSetting(sysSettingLoginTelnetListen, a.runtimeCfg.Login.Telnet.Listen)
			a.persistSystemSetting(sysSettingLoginWSEnabled, strconv.FormatBool(a.runtimeCfg.Login.WebSocket.Enabled))
			a.persistSystemSetting(sysSettingLoginWSListen, a.runtimeCfg.Login.WebSocket.Listen)
			a.persistSystemSetting(sysSettingLoginWSPath, a.runtimeCfg.Login.WebSocket.Path)
			a.persistSystemSetting(sysSettingLoginWSSEnabled, strconv.FormatBool(a.runtimeCfg.Login.WebSocketTLS.Enabled))
			a.persistSystemSetting(sysSettingLoginWSSListen, a.runtimeCfg.Login.WebSocketTLS.Listen)
			a.persistSystemSetting(sysSettingLoginWSSPath, a.runtimeCfg.Login.WebSocketTLS.Path)
			a.persistSystemSetting(sysSettingLoginWSSCert, a.runtimeCfg.Login.WebSocketTLS.Cert)
			a.persistSystemSetting(sysSettingLoginWSSKey, a.runtimeCfg.Login.WebSocketTLS.Key)
			a.persistSystemSetting(sysSettingTrustedProxies, a.runtimeCfg.Login.TrustedProxies)
			a.persistSystemSetting(sysSettingConnectorDoorPartyOn, strconv.FormatBool(a.runtimeCfg.Connectors.DoorParty.Enabled))
			a.persistSystemSetting(sysSettingConnectorDoorPartyCmd, a.runtimeCfg.Connectors.DoorParty.Command)
			a.persistSystemSetting(sysSettingConnectorDoorPartyArgs, a.runtimeCfg.Connectors.DoorParty.Args)
			a.persistSystemSetting(sysSettingConnectorBBSLinkOn, strconv.FormatBool(a.runtimeCfg.Connectors.BBSLink.Enabled))
			a.persistSystemSetting(sysSettingConnectorBBSLinkCmd, a.runtimeCfg.Connectors.BBSLink.Command)
			a.persistSystemSetting(sysSettingConnectorBBSLinkArgs, a.runtimeCfg.Connectors.BBSLink.Args)
			a.persistSystemSetting(sysSettingConnectorTelnetOn, strconv.FormatBool(a.runtimeCfg.Connectors.Telnet.Enabled))
			a.persistSystemSetting(sysSettingConnectorTelnetCmd, a.runtimeCfg.Connectors.Telnet.Command)
			a.persistSystemSetting(sysSettingConnectorTelnetArgs, a.runtimeCfg.Connectors.Telnet.Args)
			a.recordAdminAction(
				user.Handle,
				"config",
				"save_runtime_services",
				fmt.Sprintf(
					"telnet=%t ws=%t wss=%t gopher=%s nntp=%s nntps=%s",
					a.runtimeCfg.Login.Telnet.Enabled,
					a.runtimeCfg.Login.WebSocket.Enabled,
					a.runtimeCfg.Login.WebSocketTLS.Enabled,
					a.runtimeCfg.Content.GopherListen,
					a.runtimeCfg.Content.NNTPListen,
					a.runtimeCfg.Content.NNTPSListen,
				),
			)
			http.Redirect(w, r, "/admin/config", http.StatusFound)
			return
		case "save_menu_settings":
			enabled := formHasValue(r, "menu_enabled")
			normalizedFile, err := a.normalizeMenuFilePath(r.FormValue("menu_file"))
			if err != nil {
				menuErr = err.Error()
				menuFile = strings.TrimSpace(r.FormValue("menu_file"))
				menuBody = r.FormValue("menu_body")
				break
			}
			a.runtimeCfg.Menu.Enabled = enabled
			a.runtimeCfg.Menu.File = normalizedFile
			a.persistSystemSetting(sysSettingMenuEnabled, strconv.FormatBool(enabled))
			a.persistSystemSetting(sysSettingMenuFile, normalizedFile)
			a.recordAdminAction(user.Handle, "config", "save_menu_runtime", fmt.Sprintf("enabled=%t file=%s", enabled, normalizedFile))
			http.Redirect(w, r, "/admin/config?menu_file="+url.QueryEscape(normalizedFile)+"&menu_notice="+url.QueryEscape("Menu runtime settings saved."), http.StatusFound)
			return
		case "validate_menu", "save_menu":
			menuBody = r.FormValue("menu_body")
			normalizedFile, err := a.normalizeMenuFilePath(r.FormValue("menu_file"))
			if err != nil {
				menuErr = err.Error()
				menuFile = strings.TrimSpace(r.FormValue("menu_file"))
				break
			}
			menuFile = normalizedFile
			screen, err := menu.ParseHJSON([]byte(menuBody))
			if err != nil {
				menuErr = err.Error()
				break
			}
			menuNotice = fmt.Sprintf("Menu parse OK: id=%s title=%s entries=%d", screen.ID, screen.Title, len(screen.Entries))
			if action == "save_menu" {
				if err := a.saveMenuFile(menuFile, menuBody); err != nil {
					menuErr = err.Error()
					break
				}
				a.runtimeCfg.Menu.File = menuFile
				a.persistSystemSetting(sysSettingMenuFile, menuFile)
				a.recordAdminAction(user.Handle, "config", "save_menu_file", fmt.Sprintf("file=%s entries=%d", menuFile, len(screen.Entries)))
				http.Redirect(w, r, "/admin/config?menu_file="+url.QueryEscape(menuFile)+"&menu_notice="+url.QueryEscape("Menu file saved. Reconnect SSH sessions to apply changes."), http.StatusFound)
				return
			}
		default:
			http.Redirect(w, r, "/admin/config", http.StatusFound)
			return
		}
	}

	if strings.TrimSpace(menuFile) == "" {
		menuFile = strings.TrimSpace(a.runtimeCfg.Menu.File)
	}
	if strings.TrimSpace(menuFile) == "" {
		menuFile = a.defaultMenuFile()
	}
	if normalized, err := a.normalizeMenuFilePath(menuFile); err == nil {
		menuFile = normalized
	}
	if strings.TrimSpace(menuBody) == "" {
		if loaded, err := a.loadMenuFile(menuFile); err == nil {
			menuBody = loaded
		} else if strings.TrimSpace(menuErr) == "" {
			menuErr = "menu load failed: " + err.Error()
		}
	}
	statusCode := http.StatusOK
	if r.Method == http.MethodPost && strings.TrimSpace(menuErr) != "" {
		statusCode = http.StatusBadRequest
	}
	a.renderAdminConfigPage(w, r, statusCode, menuFile, menuBody, menuNotice, menuErr)
}

func (a *webApp) handleAdminErrors(w http.ResponseWriter, r *http.Request) {
	rows := strings.Builder{}
	for _, entry := range a.latestErrors(250) {
		rows.WriteString(`<tr><td>` + entry.Time.Local().Format("2006-01-02 15:04:05") + `</td><td>` + htmlEscape(entry.Area) + `</td><td>` + htmlEscape(entry.Message) + `</td></tr>`)
	}
	if rows.Len() == 0 {
		rows.WriteString(`<tr><td colspan="3">No runtime errors logged in current process.</td></tr>`)
	}
	page := `<html><body><h1>Runtime Error Log</h1><p><a href="/admin">back</a> | <a href="/admin/system">system</a> | <a href="/help">help</a></p>` +
		`<p>Shows recent application/runtime errors captured by the web companion process.</p>` +
		`<table border="1"><tr><th>Time</th><th>Area</th><th>Message</th></tr>` + rows.String() + `</table></body></html>`
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
		case "create":
			password := strings.TrimSpace(r.FormValue("password"))
			role := strings.TrimSpace(r.FormValue("role"))
			if role == "" {
				role = roleUser
			}
			if password == "" {
				password = randomPassword(14)
			}
			created, err := a.authSvc.Register(target, password)
			if err != nil {
				a.addAppError("admin.users", fmt.Errorf("create user %s: %w", target, err))
				http.Error(w, "create user failed", http.StatusBadRequest)
				return
			}
			_ = a.authSvc.SetRole(created.Handle, role)
			a.recordAdminAction(user.Handle, created.Handle, "create_user", "role="+role)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("created user " + created.Handle + " with password " + password))
			return
		case "disable":
			if err := a.authSvc.SetEnabled(target, false); err != nil {
				a.addAppError("admin.users", fmt.Errorf("disable %s: %w", target, err))
			}
			a.recordAdminAction(user.Handle, target, "disable_user", "disabled account")
		case "enable":
			if err := a.authSvc.SetEnabled(target, true); err != nil {
				a.addAppError("admin.users", fmt.Errorf("enable %s: %w", target, err))
			}
			a.recordAdminAction(user.Handle, target, "enable_user", "enabled account")
		case "ban":
			if err := a.authSvc.SetBanned(target, true); err != nil {
				a.addAppError("admin.users", fmt.Errorf("ban %s: %w", target, err))
			}
			a.recordAdminAction(user.Handle, target, "ban_user", "banned account")
		case "unban":
			if err := a.authSvc.SetBanned(target, false); err != nil {
				a.addAppError("admin.users", fmt.Errorf("unban %s: %w", target, err))
			}
			a.recordAdminAction(user.Handle, target, "unban_user", "removed ban")
		case "set_role":
			role := strings.TrimSpace(r.FormValue("role"))
			if role == "" {
				role = "user"
			}
			if err := a.authSvc.SetRole(target, role); err != nil {
				a.addAppError("admin.users", fmt.Errorf("set role %s -> %s: %w", target, role, err))
			}
			a.recordAdminAction(user.Handle, target, "set_role", role)
		case "reset":
			pw := randomPassword(10)
			if err := a.authSvc.SetPassword(target, pw); err != nil {
				a.addAppError("admin.users", fmt.Errorf("reset password %s: %w", target, err))
				http.Error(w, "reset password failed", http.StatusBadRequest)
				return
			}
			a.recordAdminAction(user.Handle, target, "reset_password", "")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("reset password for " + target + " to " + pw))
			return
		case "verify":
			if err := a.authSvc.SetVerified(target, true); err != nil {
				a.addAppError("admin.users", fmt.Errorf("verify %s: %w", target, err))
			}
			a.recordAdminAction(user.Handle, target, "verify_user", "")
		case "unverify":
			if err := a.authSvc.SetVerified(target, false); err != nil {
				a.addAppError("admin.users", fmt.Errorf("unverify %s: %w", target, err))
			}
			a.recordAdminAction(user.Handle, target, "unverify_user", "")
		}
		http.Redirect(w, r, "/admin/users", http.StatusFound)
		return
	}

	users, err := a.authSvc.ListUsers()
	if err != nil {
		a.addAppError("admin.users", fmt.Errorf("list users: %w", err))
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
		lastLogin := "-"
		if u.LastLoginAt != nil && !u.LastLoginAt.IsZero() {
			lastLogin = u.LastLoginAt.Local().Format("2006-01-02 15:04")
		}
		rows.WriteString(`<tr><td>` + u.Handle + `</td><td>` + status + `</td><td>` + u.Role + `</td><td>` + verified + `</td><td>` + lastLogin + `</td><td>`)
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
				<option value="sysop"%s>Sysop</option>
			</select><button type="submit">Set role</button></form>`, u.Handle, csrf, selectedIf(rbac.NormalizeRole(u.Role) == rbac.RoleUser), selectedIf(rbac.NormalizeRole(u.Role) == rbac.RoleModerator), selectedIf(rbac.NormalizeRole(u.Role) == rbac.RoleSysop)))
		rows.WriteString(`</td></tr>`)
	}

	page := `<html><body><h1>Sysop Users</h1><p><a href="/admin">back</a> | <a href="/admin/audit">audit</a> | <a href="/help">help</a></p>` +
		`<h2>Create User</h2><form method="POST">` + csrf +
		`<input type="hidden" name="action" value="create">` +
		`<label>Handle <input name="handle"></label> ` +
		`<label>Password <input name="password"></label> ` +
		`<label>Role <select name="role"><option value="user">User</option><option value="moderator">Moderator</option><option value="sysop">Sysop</option></select></label> ` +
		`<button type="submit">Create</button></form>` +
		`<form method="GET"><label>Search: <input name="q" value="` + filter + `"></label><button type="submit">filter</button></form>` +
		`<table border="1"><tr><th>Handle</th><th>Status</th><th>Role</th><th>Verified</th><th>Last Login</th><th>Actions</th></tr>` + rows.String() + `</table>` +
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
		redirectTo := strings.TrimSpace(r.FormValue("redirect_to"))
		if !strings.HasPrefix(redirectTo, "/admin/boards") {
			redirectTo = "/admin/boards"
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
			conference := strings.TrimSpace(r.FormValue("conference"))
			readACS := strings.TrimSpace(r.FormValue("read_acs"))
			writeACS := strings.TrimSpace(r.FormValue("write_acs"))
			_ = a.boardRepo.Create(&domain.Board{
				Name:        title,
				Description: desc,
				Conference:  conference,
				ReadACS:     readACS,
				WriteACS:    writeACS,
				CreatedBy:   user.ID,
			})
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
				} else {
					a.addAppError("admin.boards", fmt.Errorf("delete board %s: %w", id, err))
				}
			}
		case "update":
			id := strings.TrimSpace(r.FormValue("id"))
			title := strings.TrimSpace(r.FormValue("title"))
			desc := strings.TrimSpace(r.FormValue("description"))
			conference := strings.TrimSpace(r.FormValue("conference"))
			readACS := strings.TrimSpace(r.FormValue("read_acs"))
			writeACS := strings.TrimSpace(r.FormValue("write_acs"))
			boardID, parseErr := strconv.ParseInt(id, 10, 64)
			if parseErr == nil && boardID > 0 {
				board, getErr := a.boardRepo.Get(boardID)
				if getErr != nil {
					a.addAppError("admin.boards", fmt.Errorf("load board %s: %w", id, getErr))
				} else if board != nil {
					if title != "" {
						board.Name = title
					}
					board.Description = desc
					board.Conference = conference
					board.ReadACS = readACS
					board.WriteACS = writeACS
					if err := a.boardRepo.Update(board); err != nil {
						a.addAppError("admin.boards", fmt.Errorf("update board %s: %w", id, err))
					} else {
						a.recordAdminAction(user.Handle, board.Name, "update_board", "description/title/acs updated")
					}
				}
			}
		case "delete_message":
			messageID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("message_id")), 10, 64)
			reason := strings.TrimSpace(r.FormValue("reason"))
			if messageID > 0 {
				target := "message #" + strconv.FormatInt(messageID, 10)
				if msg, err := a.msgRepo.GetMessage(messageID); err == nil && msg != nil {
					target = fmt.Sprintf("message #%d (board %d)", msg.ID, msg.BoardID)
				}
				if err := a.msgRepo.DeleteMessage(messageID); err != nil {
					a.addAppError("admin.boards", fmt.Errorf("delete message %d: %w", messageID, err))
				} else {
					if reason == "" {
						reason = "deleted from sysop board controls"
					}
					a.recordAdminAction(user.Handle, target, "delete_message", reason)
				}
			}
		case "lock_thread", "unlock_thread":
			threadID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("thread_id")), 10, 64)
			if threadID > 0 {
				locked := action == "lock_thread"
				if err := a.msgRepo.SetThreadLocked(threadID, locked); err != nil {
					a.addAppError("admin.boards", fmt.Errorf("set thread lock %d: %w", threadID, err))
				} else {
					detail := "locked"
					if !locked {
						detail = "unlocked"
					}
					a.recordAdminAction(user.Handle, "thread #"+strconv.FormatInt(threadID, 10), action, detail)
				}
			}
		case "move_thread":
			threadID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("thread_id")), 10, 64)
			toBoardID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("to_board_id")), 10, 64)
			if threadID > 0 && toBoardID > 0 {
				if err := a.msgRepo.MoveThread(threadID, toBoardID); err != nil {
					a.addAppError("admin.boards", fmt.Errorf("move thread %d -> %d: %w", threadID, toBoardID, err))
				} else {
					a.recordAdminAction(user.Handle, "thread #"+strconv.FormatInt(threadID, 10), "move_thread", "to board "+strconv.FormatInt(toBoardID, 10))
				}
			}
		case "resolve_report":
			reportID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("report_id")), 10, 64)
			if reportID > 0 {
				if err := a.msgRepo.ResolveReport(reportID, user.Handle, time.Now().UTC()); err != nil {
					a.addAppError("admin.boards", fmt.Errorf("resolve report %d: %w", reportID, err))
				} else {
					a.recordAdminAction(user.Handle, "report #"+strconv.FormatInt(reportID, 10), "resolve_report", "")
				}
			}
		}
		http.Redirect(w, r, redirectTo, http.StatusFound)
		return
	}

	boards, err := a.boardRepo.List()
	if err != nil {
		a.addAppError("admin.boards", fmt.Errorf("list boards: %w", err))
		http.Error(w, "failed to load boards", http.StatusInternalServerError)
		return
	}
	boardNameByID := make(map[int64]string, len(boards))
	for _, b := range boards {
		boardNameByID[b.ID] = b.Name
	}
	manageBoardID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("manage_board")), 10, 64)
	reportStatus := strings.TrimSpace(r.URL.Query().Get("report_status"))
	if strings.EqualFold(reportStatus, "all") {
		reportStatus = ""
	}
	redirectParams := url.Values{}
	if manageBoardID > 0 {
		redirectParams.Set("manage_board", strconv.FormatInt(manageBoardID, 10))
	}
	if reportStatus != "" {
		redirectParams.Set("report_status", reportStatus)
	}
	redirectTo := "/admin/boards"
	if encoded := redirectParams.Encode(); encoded != "" {
		redirectTo = redirectTo + "?" + encoded
	}

	rows := strings.Builder{}
	csrf := a.csrfHiddenInput(r)
	for _, b := range boards {
		msgs, _ := a.msgRepo.ListByBoard(b.ID)
		last := ""
		if len(msgs) > 0 {
			last = msgs[len(msgs)-1].CreatedAt.Format("2006-01-02 15:04")
		}
		rows.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td>%s</td><td>%d</td><td>%s</td>`,
			b.ID,
			htmlEscape(b.Name),
			htmlEscape(defaultConferenceValue(b.Conference)),
			len(msgs),
			last))
		rows.WriteString(`<td><form method="POST" action="/admin/boards">` + csrf +
			`<input type="hidden" name="action" value="update"><input type="hidden" name="id" value="` + strconv.FormatInt(b.ID, 10) + `">` +
			`<input name="title" value="` + htmlEscape(b.Name) + `" size="16"> ` +
			`<input name="conference" value="` + htmlEscape(defaultConferenceValue(b.Conference)) + `" size="12"> ` +
			`<input name="description" value="` + htmlEscape(b.Description) + `" size="22"> ` +
			`<input name="read_acs" value="` + htmlEscape(b.ReadACS) + `" size="18" placeholder="read ACS"> ` +
			`<input name="write_acs" value="` + htmlEscape(b.WriteACS) + `" size="18" placeholder="write ACS"> ` +
			`<button type="submit">save</button></form></td>`)
		rows.WriteString(fmt.Sprintf(`<td><form method="POST" action="/admin/boards"><input type="hidden" name="action" value="delete"><input type="hidden" name="id" value="%d">`+csrf+`<button type="submit">delete</button></form></td>`, b.ID))
		rows.WriteString(`<td><a href="/admin/boards?manage_board=` + strconv.FormatInt(b.ID, 10) + `">moderate</a></td>`)
		rows.WriteString(`</tr>`)
	}
	reports, reportErr := a.msgRepo.ListReports(250, reportStatus)
	if reportErr != nil {
		a.addAppError("admin.boards", fmt.Errorf("list reports: %w", reportErr))
	}
	handleByID := a.userHandleLookup()
	reportRows := strings.Builder{}
	for _, row := range reports {
		reportBoard := int64(0)
		reportSubject := "(message unavailable)"
		if msg, msgErr := a.msgRepo.GetMessage(row.MessageID); msgErr == nil && msg != nil {
			reportBoard = msg.BoardID
			reportSubject = msg.Subject
		}
		reporter := handleByID[row.ReporterID]
		if reporter == "" {
			reporter = "#" + strconv.FormatInt(row.ReporterID, 10)
		}
		boardName := "?"
		if reportBoard > 0 {
			if name := boardNameByID[reportBoard]; name != "" {
				boardName = name
			}
		}
		reportRows.WriteString(`<tr><td>` + strconv.FormatInt(row.ID, 10) + `</td><td>` + strconv.FormatInt(row.MessageID, 10) + `</td><td>` + htmlEscape(boardName) + `</td><td>` + htmlEscape(reportSubject) + `</td><td>` + htmlEscape(reporter) + `</td><td>` + htmlEscape(row.Reason) + `</td><td>` + htmlEscape(row.Status) + `</td><td>` + row.CreatedAt.Local().Format("2006-01-02 15:04") + `</td><td>`)
		if strings.EqualFold(row.Status, "resolved") {
			reportRows.WriteString(`resolved`)
		} else {
			reportRows.WriteString(`<form method="POST" action="/admin/boards">` + csrf +
				`<input type="hidden" name="action" value="resolve_report">` +
				`<input type="hidden" name="report_id" value="` + strconv.FormatInt(row.ID, 10) + `">` +
				`<input type="hidden" name="redirect_to" value="` + htmlEscape(redirectTo) + `">` +
				`<button type="submit">resolve</button></form>`)
		}
		reportRows.WriteString(`</td></tr>`)
	}
	if reportRows.Len() == 0 {
		reportRows.WriteString(`<tr><td colspan="9">No reports in this filter</td></tr>`)
	}

	manageRows := strings.Builder{}
	if manageBoardID > 0 {
		msgs, msgErr := a.msgRepo.ListByBoard(manageBoardID)
		if msgErr != nil {
			a.addAppError("admin.boards", fmt.Errorf("list board %d messages: %w", manageBoardID, msgErr))
		} else {
			moveOptions := strings.Builder{}
			for _, board := range boards {
				selected := ""
				if board.ID == manageBoardID {
					selected = ` selected`
				}
				moveOptions.WriteString(`<option value="` + strconv.FormatInt(board.ID, 10) + `"` + selected + `>` + htmlEscape(board.Name) + `</option>`)
			}
			for _, msg := range msgs {
				author := handleByID[msg.AuthorID]
				if author == "" {
					author = "#" + strconv.FormatInt(msg.AuthorID, 10)
				}
				threadLocked, lockErr := a.msgRepo.IsThreadLocked(msg.ThreadID)
				if lockErr != nil {
					a.addAppError("admin.boards", fmt.Errorf("thread lock check %d: %w", msg.ThreadID, lockErr))
				}
				lockAction := "lock_thread"
				lockLabel := "lock"
				if threadLocked {
					lockAction = "unlock_thread"
					lockLabel = "unlock"
				}
				manageRows.WriteString(`<tr><td>` + strconv.FormatInt(msg.ID, 10) + `</td><td>` + strconv.FormatInt(msg.ThreadID, 10) + `</td><td>` + htmlEscape(author) + `</td><td>` + htmlEscape(msg.Subject) + `</td><td>` + msg.CreatedAt.Local().Format("2006-01-02 15:04") + `</td><td>`)
				manageRows.WriteString(`<form method="POST" action="/admin/boards">` + csrf +
					`<input type="hidden" name="action" value="delete_message">` +
					`<input type="hidden" name="message_id" value="` + strconv.FormatInt(msg.ID, 10) + `">` +
					`<input type="hidden" name="redirect_to" value="` + htmlEscape(redirectTo) + `">` +
					`<input name="reason" size="16" placeholder="reason">` +
					`<button type="submit">delete</button></form>`)
				manageRows.WriteString(`<form method="POST" action="/admin/boards">` + csrf +
					`<input type="hidden" name="action" value="` + lockAction + `">` +
					`<input type="hidden" name="thread_id" value="` + strconv.FormatInt(msg.ThreadID, 10) + `">` +
					`<input type="hidden" name="redirect_to" value="` + htmlEscape(redirectTo) + `">` +
					`<button type="submit">` + lockLabel + `</button></form>`)
				manageRows.WriteString(`<form method="POST" action="/admin/boards">` + csrf +
					`<input type="hidden" name="action" value="move_thread">` +
					`<input type="hidden" name="thread_id" value="` + strconv.FormatInt(msg.ThreadID, 10) + `">` +
					`<input type="hidden" name="redirect_to" value="` + htmlEscape(redirectTo) + `">` +
					`<select name="to_board_id">` + moveOptions.String() + `</select>` +
					`<button type="submit">move</button></form>`)
				manageRows.WriteString(`</td></tr>`)
			}
		}
	}
	if manageRows.Len() == 0 {
		manageRows.WriteString(`<tr><td colspan="6">Select a board to moderate posts and thread state.</td></tr>`)
	}

	reportStatusSelect := map[string]string{"": "", "open": "", "resolved": ""}
	reportStatusSelect[reportStatus] = ` selected`
	page := `<html><body><h1>Sysop Boards</h1><p><a href="/admin">back</a> | <a href="/help">help</a></p>` +
		`<form method="POST"><label>Title <input name="title"></label> <label>Conference <input name="conference" value="General" size="14"></label> <label>Description <input name="description" size="28"></label> <label>Read ACS <input name="read_acs" size="16"></label> <label>Write ACS <input name="write_acs" size="16"></label>` + csrf + `<input type="hidden" name="action" value="create"><button type="submit">add</button></form>` +
		`<table border="1"><tr><th>ID</th><th>Title</th><th>Conf</th><th>Topics</th><th>Last</th><th>Edit</th><th>Actions</th><th>Moderation</th></tr>` + rows.String() + `</table>` +
		`<h2>Moderation Queue</h2>` +
		`<form method="GET" action="/admin/boards"><label>Status <select name="report_status"><option value=""` + reportStatusSelect[""] + `>all</option><option value="open"` + reportStatusSelect["open"] + `>open</option><option value="resolved"` + reportStatusSelect["resolved"] + `>resolved</option></select></label><label> Board <input name="manage_board" size="6" value="` + strconv.FormatInt(manageBoardID, 10) + `"></label><button type="submit">apply</button></form>` +
		`<table border="1"><tr><th>ID</th><th>Message</th><th>Board</th><th>Subject</th><th>Reporter</th><th>Reason</th><th>Status</th><th>Created</th><th>Action</th></tr>` + reportRows.String() + `</table>` +
		`<h2>Board Message Moderation</h2><p>Delete with reason, lock/unlock thread, and move thread to another board.</p>` +
		`<table border="1"><tr><th>ID</th><th>Thread</th><th>Author</th><th>Subject</th><th>When</th><th>Actions</th></tr>` + manageRows.String() + `</table>` +
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
	page := `<html><body><h1>Mail Controls</h1><p><a href="/admin">back</a> | <a href="/help">help</a></p>` +
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
		redirectURL := "/admin/files"
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
		case "index_area":
			id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
			uploaderID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("uploader_id")), 10, 64)
			if uploaderID <= 0 {
				uploaderID = user.ID
			}
			if a.adminRepo != nil && id > 0 {
				areas, _ := a.adminRepo.ListFileAreas()
				for _, area := range areas {
					if area.ID != id {
						continue
					}
					indexed, failed, err := a.indexAreaFiles(area, uploaderID)
					if err != nil {
						a.addAppError("admin.files", fmt.Errorf("index area %d: %w", id, err))
					} else {
						a.recordAdminAction(user.Handle, area.Name, "index_file_area", fmt.Sprintf("indexed=%d failed=%d", indexed, failed))
					}
					break
				}
			}
		case "rate":
			targetUserID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("user_id")), 10, 64)
			fileID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
			rating := parseInt(r.FormValue("rating"), 0)
			if targetUserID <= 0 {
				targetUserID = user.ID
			}
			if a.adminRepo != nil && targetUserID > 0 && fileID > 0 && rating > 0 {
				if err := a.adminRepo.SetFileRating(targetUserID, fileID, rating); err != nil {
					a.addAppError("admin.files", fmt.Errorf("set file rating: %w", err))
				} else {
					a.recordAdminAction(user.Handle, strconv.FormatInt(fileID, 10), "rate_file", fmt.Sprintf("user=%d rating=%d", targetUserID, rating))
				}
			}
		case "save_filter":
			targetUserID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("filter_user_id")), 10, 64)
			if targetUserID <= 0 {
				targetUserID = user.ID
			}
			filter := &domain.FileFilter{
				UserID: targetUserID,
				Name:   strings.TrimSpace(r.FormValue("name")),
				Query:  strings.TrimSpace(r.FormValue("query")),
			}
			rawTags := strings.TrimSpace(r.FormValue("tags"))
			for _, tag := range strings.Split(rawTags, ",") {
				tag = strings.TrimSpace(tag)
				if tag != "" {
					filter.Tags = append(filter.Tags, tag)
				}
			}
			if a.adminRepo != nil {
				if err := a.adminRepo.SaveFileFilter(filter); err != nil {
					a.addAppError("admin.files", fmt.Errorf("save file filter: %w", err))
				} else {
					a.recordAdminAction(user.Handle, filter.Name, "save_file_filter", fmt.Sprintf("user=%d", targetUserID))
					redirectURL = "/admin/files?filter_user=" + strconv.FormatInt(targetUserID, 10)
				}
			}
		case "queue_add":
			targetUserID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("queue_user_id")), 10, 64)
			fileID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
			if targetUserID <= 0 {
				targetUserID = user.ID
			}
			if a.adminRepo != nil && targetUserID > 0 && fileID > 0 {
				if err := a.adminRepo.EnqueueDownload(targetUserID, fileID); err != nil {
					a.addAppError("admin.files", fmt.Errorf("enqueue download: %w", err))
				} else {
					a.recordAdminAction(user.Handle, strconv.FormatInt(fileID, 10), "enqueue_download", fmt.Sprintf("user=%d", targetUserID))
					redirectURL = "/admin/files?queue_user=" + strconv.FormatInt(targetUserID, 10)
				}
			}
		case "queue_del":
			targetUserID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("queue_user_id")), 10, 64)
			fileID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
			if a.adminRepo != nil && targetUserID > 0 && fileID > 0 {
				if err := a.adminRepo.DequeueDownload(targetUserID, fileID); err != nil {
					a.addAppError("admin.files", fmt.Errorf("dequeue download: %w", err))
				} else {
					a.recordAdminAction(user.Handle, strconv.FormatInt(fileID, 10), "dequeue_download", fmt.Sprintf("user=%d", targetUserID))
					redirectURL = "/admin/files?queue_user=" + strconv.FormatInt(targetUserID, 10)
				}
			}
		case "ticket":
			targetUserID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("queue_user_id")), 10, 64)
			fileID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
			ttlMinutes := parseInt(r.FormValue("ttl_minutes"), 15)
			if ttlMinutes <= 0 {
				ttlMinutes = 15
			}
			if targetUserID <= 0 {
				targetUserID = user.ID
			}
			if targetUserID > 0 && fileID > 0 {
				ticket, err := a.createDownloadTicket(targetUserID, fileID, time.Duration(ttlMinutes)*time.Minute)
				if err != nil {
					a.addAppError("admin.files", fmt.Errorf("create download ticket: %w", err))
				} else {
					a.recordAdminAction(user.Handle, strconv.FormatInt(fileID, 10), "issue_download_ticket", fmt.Sprintf("user=%d ttl=%dm", targetUserID, ttlMinutes))
					redirectURL = "/admin/files?queue_user=" + strconv.FormatInt(targetUserID, 10) + "&issued_token=" + url.QueryEscape(ticket.Token)
				}
			}
		}
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	areas := []domain.FileArea{}
	files := []domain.FileEntry{}
	filters := []domain.FileFilter{}
	queue := []domain.DownloadQueueItem{}
	queueFiles := map[int64]string{}
	areaNames := map[int64]string{}
	fileAreaID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("area")), 10, 64)
	queueUserID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("queue_user")), 10, 64)
	filterUserID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("filter_user")), 10, 64)
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	searchTagsRaw := strings.TrimSpace(r.URL.Query().Get("tags"))
	searchTags := make([]string, 0)
	for _, tag := range strings.Split(searchTagsRaw, ",") {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			searchTags = append(searchTags, tag)
		}
	}
	issuedToken := strings.TrimSpace(r.URL.Query().Get("issued_token"))
	if a.adminRepo != nil {
		areas, _ = a.adminRepo.ListFileAreas()
		for _, area := range areas {
			areaNames[area.ID] = area.Name
		}
		files, _ = a.adminRepo.ListFileEntries(fileAreaID, searchQuery, searchTags, 300)
		if filterUserID > 0 {
			filters, _ = a.adminRepo.ListFileFilters(filterUserID)
		} else {
			filters, _ = a.adminRepo.ListFileFilters(0)
		}
		if queueUserID > 0 {
			queue, _ = a.adminRepo.ListDownloadQueue(queueUserID, 200)
			for _, item := range queue {
				if entry, err := a.adminRepo.GetFileEntry(item.FileID); err == nil && entry != nil {
					queueFiles[item.FileID] = entry.Name
				}
			}
		}
	}
	csrf := a.csrfHiddenInput(r)
	areaRows := strings.Builder{}
	for _, area := range areas {
		areaRows.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td>%s</td><td>%s</td>`, area.ID, htmlEscape(area.Name), htmlEscape(area.Path), htmlEscape(area.Description)))
		areaRows.WriteString(`<td><form method="POST" action="/admin/files">` + csrf + `<input type="hidden" name="action" value="index_area"><input type="hidden" name="id" value="` + strconv.FormatInt(area.ID, 10) + `"><input type="hidden" name="uploader_id" value="` + strconv.FormatInt(user.ID, 10) + `"><button type="submit">index</button></form></td>`)
		areaRows.WriteString(`<td><form method="POST" action="/admin/files">` + csrf + `<input type="hidden" name="action" value="delete"><input type="hidden" name="id" value="` + strconv.FormatInt(area.ID, 10) + `"><button type="submit">delete</button></form></td></tr>`)
	}
	if areaRows.Len() == 0 {
		areaRows.WriteString(`<tr><td colspan="6">No file areas configured</td></tr>`)
	}

	fileRows := strings.Builder{}
	for _, row := range files {
		tags := htmlEscape(strings.Join(row.Tags, ","))
		areaName := htmlEscape(areaNames[row.AreaID])
		fileRows.WriteString(`<tr><td>` + strconv.FormatInt(row.ID, 10) + `</td><td>` + areaName + `</td><td>` + htmlEscape(row.Name) + `</td><td>` + tags + `</td><td>` + fmt.Sprintf("%.2f", row.RatingAvg) + ` (` + strconv.Itoa(row.RatingCount) + `)</td><td>` + htmlEscape(row.SHA256) + `</td>`)
		fileRows.WriteString(`<td><form method="POST" action="/admin/files">` + csrf +
			`<input type="hidden" name="action" value="rate"><input type="hidden" name="file_id" value="` + strconv.FormatInt(row.ID, 10) + `"><input type="hidden" name="user_id" value="` + strconv.FormatInt(user.ID, 10) + `">` +
			`<input name="rating" size="2" value="5"><button type="submit">rate</button></form>`)
		fileRows.WriteString(`<form method="POST" action="/admin/files">` + csrf +
			`<input type="hidden" name="action" value="queue_add"><input type="hidden" name="file_id" value="` + strconv.FormatInt(row.ID, 10) + `"><input name="queue_user_id" size="6" value="` + strconv.FormatInt(user.ID, 10) + `"><button type="submit">queue</button></form></td></tr>`)
	}
	if fileRows.Len() == 0 {
		fileRows.WriteString(`<tr><td colspan="7">No indexed files matched</td></tr>`)
	}

	filterRows := strings.Builder{}
	for _, row := range filters {
		filterRows.WriteString(`<tr><td>` + strconv.FormatInt(row.ID, 10) + `</td><td>` + strconv.FormatInt(row.UserID, 10) + `</td><td>` + htmlEscape(row.Name) + `</td><td>` + htmlEscape(row.Query) + `</td><td>` + htmlEscape(strings.Join(row.Tags, ",")) + `</td></tr>`)
	}
	if filterRows.Len() == 0 {
		filterRows.WriteString(`<tr><td colspan="5">No saved filters</td></tr>`)
	}

	queueRows := strings.Builder{}
	for _, item := range queue {
		displayName := queueFiles[item.FileID]
		if displayName == "" {
			displayName = "file #" + strconv.FormatInt(item.FileID, 10)
		}
		queueRows.WriteString(`<tr><td>` + strconv.FormatInt(item.ID, 10) + `</td><td>` + strconv.FormatInt(item.UserID, 10) + `</td><td>` + htmlEscape(displayName) + `</td><td>` + item.CreatedAt.Local().Format(time.RFC3339) + `</td><td>`)
		queueRows.WriteString(`<form method="POST" action="/admin/files">` + csrf + `<input type="hidden" name="action" value="queue_del"><input type="hidden" name="queue_user_id" value="` + strconv.FormatInt(item.UserID, 10) + `"><input type="hidden" name="file_id" value="` + strconv.FormatInt(item.FileID, 10) + `"><button type="submit">remove</button></form>`)
		queueRows.WriteString(`<form method="POST" action="/admin/files">` + csrf + `<input type="hidden" name="action" value="ticket"><input type="hidden" name="queue_user_id" value="` + strconv.FormatInt(item.UserID, 10) + `"><input type="hidden" name="file_id" value="` + strconv.FormatInt(item.FileID, 10) + `"><input name="ttl_minutes" size="4" value="15"><button type="submit">ticket</button></form>`)
		queueRows.WriteString(`</td></tr>`)
	}
	if queueRows.Len() == 0 {
		queueRows.WriteString(`<tr><td colspan="5">No queue rows for selected user</td></tr>`)
	}

	var ticketNotice string
	if issuedToken != "" {
		ticketNotice = `<p><strong>Issued ticket:</strong> <code>` + htmlEscape(issuedToken) + `</code><br><a href="/gateway?download=` + url.QueryEscape(issuedToken) + `">/gateway?download=` + url.QueryEscape(issuedToken) + `</a></p>`
	}

	page := `<html><body><h1>Files</h1><p><a href="/admin">back</a> | <a href="/gateway">gateway</a> | <a href="/help">help</a></p>` + ticketNotice +
		`<form method="POST"><input type="hidden" name="action" value="create">` + csrf +
		`<label>Name <input name="name"></label> <label>Path <input name="path" size="30"></label> <label>Description <input name="description" size="40"></label> <button type="submit">add</button></form>` +
		`<table border="1"><tr><th>ID</th><th>Name</th><th>Path</th><th>Description</th><th>Index</th><th>Delete</th></tr>` + areaRows.String() + `</table>` +
		`<h2>Indexed Files</h2><form method="GET"><label>Area ID <input name="area" value="` + strconv.FormatInt(fileAreaID, 10) + `" size="6"></label> <label>Query <input name="q" value="` + htmlEscape(searchQuery) + `" size="24"></label> <label>Tags <input name="tags" value="` + htmlEscape(searchTagsRaw) + `" size="24"></label> <button type="submit">search</button></form>` +
		`<table border="1"><tr><th>ID</th><th>Area</th><th>Name</th><th>Tags</th><th>Rating</th><th>SHA-256</th><th>Actions</th></tr>` + fileRows.String() + `</table>` +
		`<h2>Saved Filters</h2><form method="POST">` + csrf + `<input type="hidden" name="action" value="save_filter"><label>User ID <input name="filter_user_id" value="` + strconv.FormatInt(maxInt64(filterUserID, user.ID), 10) + `" size="8"></label> <label>Name <input name="name" size="16"></label> <label>Query <input name="query" size="24"></label> <label>Tags <input name="tags" size="24" placeholder="tag1,tag2"></label> <button type="submit">save</button></form>` +
		`<table border="1"><tr><th>ID</th><th>User</th><th>Name</th><th>Query</th><th>Tags</th></tr>` + filterRows.String() + `</table>` +
		`<h2>Download Queue</h2><form method="GET"><label>User ID <input name="queue_user" value="` + strconv.FormatInt(maxInt64(queueUserID, user.ID), 10) + `" size="8"></label><button type="submit">view queue</button></form>` +
		`<table border="1"><tr><th>ID</th><th>User</th><th>File</th><th>Queued At</th><th>Actions</th></tr>` + queueRows.String() + `</table></body></html>`
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
			} else {
				a.addAppError("admin.gateways", fmt.Errorf("save gateway settings: %w", err))
			}
		}
		http.Redirect(w, r, "/admin/gateways", http.StatusFound)
		return
	}
	cfg := &domain.GatewaySettings{
		SMTPHost:        strings.TrimSpace(envFirst("SMTP_HOST", "WOLFBBS_SMTP_HOST")),
		SMTPPort:        parseInt(envFirst("SMTP_PORT", "WOLFBBS_SMTP_PORT"), 587),
		SMTPUser:        strings.TrimSpace(envFirst("SMTP_USER", "WOLFBBS_SMTP_USER")),
		SMTPPass:        strings.TrimSpace(envFirst("SMTP_PASS", "WOLFBBS_SMTP_PASS")),
		FromDomain:      strings.TrimSpace(envFirst("FROM_DOMAIN", "WOLFBBS_FROM_DOMAIN")),
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
	page := `<html><body><h1>Gateway Controls</h1><p><a href="/admin">back</a> | <a href="/help">help</a></p>` +
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
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if a.chatSvc == nil {
		a.addAppError("admin.chat", fmt.Errorf("chat service unavailable"))
		http.Error(w, "chat service unavailable", http.StatusInternalServerError)
		return
	}
	if r.Method == http.MethodPost {
		if !a.requireAdminWrite(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		channel := chat.NormalizeChannel(strings.TrimSpace(r.FormValue("channel")))
		if channel == "" {
			channel = "#lobby"
		}
		switch action {
		case "create_channel":
			a.chatSvc.JoinChannel(user.Handle, channel)
			a.chatSvc.LeaveChannel(user.Handle, channel)
			a.recordAdminAction(user.Handle, channel, "chat_create_channel", "")
		case "lock_channel":
			a.setChannelLock(channel, true)
			a.recordAdminAction(user.Handle, channel, "chat_lock_channel", "")
		case "unlock_channel":
			a.setChannelLock(channel, false)
			a.recordAdminAction(user.Handle, channel, "chat_unlock_channel", "")
		}
		http.Redirect(w, r, "/admin/chat", http.StatusFound)
		return
	}

	channels := a.chatSvc.ListChannels()
	if len(channels) == 0 {
		channels = []string{"#lobby"}
	}
	online := a.chatSvc.Online()
	onlineByChannel := map[string]int{}
	for _, row := range online {
		ch := chat.NormalizeChannel(row.Area)
		if ch == "" {
			ch = "#lobby"
		}
		onlineByChannel[ch]++
	}
	csrf := a.csrfHiddenInput(r)
	channelRows := strings.Builder{}
	for _, c := range channels {
		locked := a.isChannelLocked(c)
		channelRows.WriteString(`<tr><td>` + htmlEscape(c) + `</td><td>` + strconv.Itoa(onlineByChannel[c]) + `</td><td>` + boolToText(locked) + `</td><td>`)
		if locked {
			channelRows.WriteString(`<form method="POST"><input type="hidden" name="action" value="unlock_channel"><input type="hidden" name="channel" value="` + htmlEscape(c) + `">` + csrf + `<button type="submit">Unlock</button></form>`)
		} else {
			channelRows.WriteString(`<form method="POST"><input type="hidden" name="action" value="lock_channel"><input type="hidden" name="channel" value="` + htmlEscape(c) + `">` + csrf + `<button type="submit">Lock</button></form>`)
		}
		channelRows.WriteString(`</td></tr>`)
	}
	modRows := strings.Builder{}
	for _, action := range a.chatSvc.ModerationLog(200) {
		modRows.WriteString(`<tr><td>` + action.CreatedAt.Local().Format("2006-01-02 15:04:05") + `</td><td>` + htmlEscape(action.Type) + `</td><td>` + htmlEscape(action.Channel) + `</td><td>` + htmlEscape(action.Actor) + `</td><td>` + htmlEscape(action.Target) + `</td><td>` + htmlEscape(action.Reason) + `</td></tr>`)
	}
	if modRows.Len() == 0 {
		modRows.WriteString(`<tr><td colspan="6">No moderation events</td></tr>`)
	}
	page := `<html><body><h1>Chat Admin</h1><p><a href="/admin">back</a> | <a href="/help">help</a></p>` +
		`<h2>Channel Management</h2><form method="POST"><input type="hidden" name="action" value="create_channel">` + csrf + `<label>Channel <input name="channel" value="#new-channel"></label> <button type="submit">Create</button></form>` +
		`<table border="1"><tr><th>Channel</th><th>Online</th><th>Locked</th><th>Action</th></tr>` + channelRows.String() + `</table>` +
		`<p>Locked channels allow moderator/sysop posting only.</p>` +
		`<h2>Moderation Log</h2><table border="1"><tr><th>Time</th><th>Action</th><th>Channel</th><th>Actor</th><th>Target</th><th>Reason</th></tr>` + modRows.String() + `</table></body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminDoors(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if a.doorRegistry == nil {
		http.Error(w, "door registry unavailable", http.StatusInternalServerError)
		return
	}
	if r.Method == http.MethodPost {
		if !a.requireAdminWrite(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		doorID := strings.ToLower(strings.TrimSpace(r.FormValue("door_id")))
		if doorID == "" {
			http.Redirect(w, r, "/admin/doors", http.StatusFound)
			return
		}
		switch action {
		case "reset_scores":
			if err := a.doorRegistry.ResetDoorScores(doorID); err == nil {
				a.recordAdminAction(user.Handle, doorID, "door_reset_scores", "")
			}
		case "save_config":
			door, found := a.doorRegistry.DoorByID(doorID)
			if !found {
				http.Error(w, "unknown door", http.StatusBadRequest)
				return
			}
			cfg, err := a.doorRegistry.GetDoorConfig(doorID)
			if err != nil || cfg == nil {
				cfg = &domain.DoorConfig{
					DoorID:         doorID,
					Enabled:        door.EnabledDefault,
					DailyTurns:     door.DailyTurns,
					TimeBankMax:    door.TimeBankMax,
					ResetHourLocal: door.ResetHour,
					MessagesDays:   maxInt(1, door.MessagesDays),
					LogsDays:       maxInt(1, door.LogsDays),
					MaxRunSeconds:  door.MaxRunSec,
					MaxOutputRate:  door.MaxOutputRate,
					AllowNetwork:   door.NeedsNetwork,
					AllowFSWrite:   door.NeedsFSWrite,
				}
			}
			cfg.Enabled = formHasValue(r, "enabled")
			cfg.DailyTurns = parseIntWithFallback(r.FormValue("daily_turns"), cfg.DailyTurns)
			cfg.TimeBankMax = parseIntWithFallback(r.FormValue("time_bank_max"), cfg.TimeBankMax)
			cfg.ResetHourLocal = parseIntWithFallback(r.FormValue("reset_hour"), cfg.ResetHourLocal)
			cfg.MessagesDays = parseIntWithFallback(r.FormValue("messages_days"), cfg.MessagesDays)
			cfg.LogsDays = parseIntWithFallback(r.FormValue("logs_days"), cfg.LogsDays)
			cfg.MaxRunSeconds = parseIntWithFallback(r.FormValue("max_run_seconds"), cfg.MaxRunSeconds)
			cfg.MaxOutputRate = parseIntWithFallback(r.FormValue("max_output_rate"), cfg.MaxOutputRate)
			cfg.AllowNetwork = formHasValue(r, "allow_network")
			cfg.AllowFSWrite = formHasValue(r, "allow_fs_write")
			cfg.RequiredRoleOverride = strings.TrimSpace(r.FormValue("required_role_override"))
			if err := a.doorRegistry.SetDoorConfig(cfg); err == nil {
				a.recordAdminAction(user.Handle, doorID, "door_save_config", "updated policy")
			}
		}
		http.Redirect(w, r, "/admin/doors", http.StatusFound)
		return
	}

	filter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	csrf := a.csrfHiddenInput(r)
	rows := strings.Builder{}
	for _, door := range a.doorRegistry.Doors() {
		if filter != "" &&
			!strings.Contains(strings.ToLower(door.ID), filter) &&
			!strings.Contains(strings.ToLower(door.Name), filter) &&
			!strings.Contains(strings.ToLower(door.Category), filter) {
			continue
		}
		cfg, _ := a.doorRegistry.GetDoorConfig(door.ID)
		if cfg == nil {
			cfg = &domain.DoorConfig{
				DoorID:         door.ID,
				Enabled:        door.EnabledDefault,
				DailyTurns:     door.DailyTurns,
				TimeBankMax:    door.TimeBankMax,
				ResetHourLocal: door.ResetHour,
				MessagesDays:   maxInt(1, door.MessagesDays),
				LogsDays:       maxInt(1, door.LogsDays),
				MaxRunSeconds:  door.MaxRunSec,
				MaxOutputRate:  door.MaxOutputRate,
				AllowNetwork:   door.NeedsNetwork,
				AllowFSWrite:   door.NeedsFSWrite,
			}
		}
		stats, _ := a.doorRegistry.GetUsageStats(door.ID)
		if stats == nil {
			stats = &domain.DoorUsageStats{}
		}
		rows.WriteString(`<tr><td>` + strings.ToUpper(door.Hotkey) + `</td><td>` + htmlEscape(door.Name) + `</td><td>` + htmlEscape(door.Category) + `</td>`)
		rows.WriteString(`<td>` + strconv.Itoa(stats.DailyActive) + ` / ` + strconv.Itoa(stats.MonthlyActive) + `</td>`)
		rows.WriteString(`<td>` + strconv.FormatInt(stats.TotalPlays, 10) + `</td>`)
		rows.WriteString(`<td><form method="POST" action="/admin/doors">` + csrf +
			`<input type="hidden" name="action" value="save_config">` +
			`<input type="hidden" name="door_id" value="` + htmlEscape(door.ID) + `">` +
			`Enabled <input type="checkbox" name="enabled"` + checkedIf(cfg.Enabled) + `>` +
			` Turns <input size="4" name="daily_turns" value="` + strconv.Itoa(cfg.DailyTurns) + `">` +
			` Bank <input size="4" name="time_bank_max" value="` + strconv.Itoa(cfg.TimeBankMax) + `">` +
			` Reset <input size="2" name="reset_hour" value="` + strconv.Itoa(cfg.ResetHourLocal) + `">` +
			` Logs <input size="3" name="logs_days" value="` + strconv.Itoa(cfg.LogsDays) + `">` +
			` Msg <input size="3" name="messages_days" value="` + strconv.Itoa(cfg.MessagesDays) + `">` +
			` Run(s) <input size="4" name="max_run_seconds" value="` + strconv.Itoa(cfg.MaxRunSeconds) + `">` +
			` Out/s <input size="5" name="max_output_rate" value="` + strconv.Itoa(cfg.MaxOutputRate) + `">` +
			` Net <input type="checkbox" name="allow_network"` + checkedIf(cfg.AllowNetwork) + `>` +
			` FSW <input type="checkbox" name="allow_fs_write"` + checkedIf(cfg.AllowFSWrite) + `>` +
			` Role <input size="10" name="required_role_override" value="` + htmlEscape(cfg.RequiredRoleOverride) + `">` +
			` <button type="submit">Save</button></form>`)
		rows.WriteString(`<form method="POST" action="/admin/doors">` + csrf +
			`<input type="hidden" name="action" value="reset_scores">` +
			`<input type="hidden" name="door_id" value="` + htmlEscape(door.ID) + `">` +
			`<button type="submit">Reset Scores</button></form>`)
		rows.WriteString(`<a href="/scores?door=` + htmlEscape(door.ID) + `">Scores</a></td></tr>`)
	}
	if rows.Len() == 0 {
		rows.WriteString(`<tr><td colspan="6">No doors matched filter.</td></tr>`)
	}
	logRows := strings.Builder{}
	for _, event := range a.mustDoorEvents("", 0, 120) {
		logRows.WriteString(`<tr><td>` + event.CreatedAt.Format("2006-01-02 15:04:05") + `</td><td>` + htmlEscape(event.DoorID) + `</td><td>` + strconv.FormatInt(event.UserID, 10) + `</td><td>` + htmlEscape(event.EventType) + `</td><td>` + htmlEscape(event.PayloadJSON) + `</td></tr>`)
	}
	if logRows.Len() == 0 {
		logRows.WriteString(`<tr><td colspan="5">No door events logged yet.</td></tr>`)
	}
	page := `<html><body><h1>Doors Admin</h1><p><a href="/admin">back</a> | <a href="/scores">global scores</a> | <a href="/help">help</a></p>` +
		`<form method="GET"><label>Filter <input name="q" value="` + htmlEscape(filter) + `"></label><button type="submit">Apply</button></form>` +
		`<table border="1"><tr><th>HK</th><th>Name</th><th>Category</th><th>DAU/MAU</th><th>Total Plays</th><th>Config</th></tr>` + rows.String() + `</table>` +
		`<h2>Door Event Log</h2><table border="1"><tr><th>Time</th><th>Door</th><th>UserID</th><th>Event</th><th>Payload</th></tr>` + logRows.String() + `</table>` +
		`</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleScores(w http.ResponseWriter, r *http.Request) {
	_, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if a.doorRegistry == nil {
		http.Error(w, "door registry unavailable", http.StatusInternalServerError)
		return
	}
	filterDoor := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("door")))
	lookup := a.userHandleLookup()
	section := strings.Builder{}
	section.WriteString(`<p><a href="/boards">boards</a> | <a href="/chat">chat</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/admin">admin</a> | <a href="/help">help</a></p>`)
	section.WriteString(`<p><strong>Door filter:</strong> ` + htmlEscape(filterDoor) + `</p>`)
	section.WriteString(`<table border="1"><tr><th>Door</th><th>User</th><th>Score</th><th>Type</th><th>When</th></tr>`)
	for _, door := range a.doorRegistry.Doors() {
		if filterDoor != "" && door.ID != filterDoor {
			continue
		}
		scores, _ := a.doorRegistry.ListScores(door.ID, 10)
		if len(scores) == 0 {
			section.WriteString(`<tr><td>` + htmlEscape(door.Name) + `</td><td colspan="4">No scores yet</td></tr>`)
			continue
		}
		for _, score := range scores {
			handle := lookup[score.UserID]
			if handle == "" {
				handle = "uid:" + strconv.FormatInt(score.UserID, 10)
			}
			section.WriteString(`<tr><td>` + htmlEscape(door.Name) + `</td><td>` + htmlEscape(handle) + `</td><td>` + strconv.FormatInt(score.Value, 10) + `</td><td>` + htmlEscape(score.ScoreType) + `</td><td>` + score.CreatedAt.Format("2006-01-02 15:04:05") + `</td></tr>`)
		}
	}
	section.WriteString(`</table>`)
	page := `<html><body><h1>Door Scores & Trophies</h1>` + section.String() + `</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminSystem(w http.ResponseWriter, r *http.Request) {
	a.Lock()
	webSessions := len(a.sessions)
	a.Unlock()

	userCount := 0
	if a.authSvc != nil {
		if users, err := a.authSvc.ListUsers(); err == nil {
			userCount = len(users)
		}
	}

	boardCount := 0
	messageCount := 0
	if a.boardRepo != nil {
		if boards, err := a.boardRepo.List(); err == nil {
			boardCount = len(boards)
			if a.msgRepo != nil {
				for _, board := range boards {
					if msgs, listErr := a.msgRepo.ListByBoard(board.ID); listErr == nil {
						messageCount += len(msgs)
					}
				}
			}
		}
	}

	channels := []string{"#lobby"}
	online := []chat.Presence{}
	if a.chatSvc != nil {
		channels = a.chatSvc.ListChannels()
		online = a.chatSvc.Online()
	}
	nodeSessions := []domain.NodeSession{}
	callerHistory := []domain.CallerHistory{}
	if a.adminRepo != nil {
		nodeSessions, _ = a.adminRepo.ListNodeSessions(200)
		callerHistory, _ = a.adminRepo.ListCallerHistory(50)
	}
	lockedCount := 0
	for _, channel := range channels {
		if a.isChannelLocked(channel) {
			lockedCount++
		}
	}
	errorCount := len(a.latestErrors(1000))
	networkInbound := 0
	networkOutbound := 0
	if a.networkSvc != nil {
		if status, err := a.networkSvc.Status(); err == nil {
			networkInbound = status.InboundPackets
			networkOutbound = status.OutboundPackets
		}
	}
	modCount := 0
	modRunning := 0
	oneLinerCount := 0
	activeRumor := ""
	if a.modsManager != nil {
		snap := a.modsManager.Snapshot()
		modCount = len(snap)
		for _, row := range snap {
			if row.Running {
				modRunning++
			}
		}
	}
	if a.oneLinerzMod != nil {
		oneLinerCount = len(a.oneLinerzMod.List(1000))
	}
	if a.rumorzMod != nil {
		activeRumor = cleanOneLiner(a.rumorzMod.Current(), 80)
	}

	uptime := "unknown"
	if !a.startedAt.IsZero() {
		uptime = time.Since(a.startedAt).Round(time.Second).String()
	}

	onlineRows := strings.Builder{}
	if len(nodeSessions) > 0 {
		now := time.Now().UTC()
		for _, row := range nodeSessions {
			idle := now.Sub(row.LastActivity)
			if idle < 0 {
				idle = 0
			}
			onlineRows.WriteString(`<tr><td>` + htmlEscape(row.Username) + `</td><td>Node ` + strconv.Itoa(row.NodeID) + `</td><td>` + htmlEscape(row.Area) + `</td><td>` + row.LoginAt.Format("2006-01-02 15:04:05") + `</td><td>` + strconv.Itoa(int(idle.Seconds())) + `s</td></tr>`)
		}
	} else {
		for _, row := range online {
			onlineRows.WriteString(`<tr><td>` + htmlEscape(row.Nick) + `</td><td>` + htmlEscape(row.Node) + `</td><td>` + htmlEscape(row.Area) + `</td><td>` + row.LoginAt.Format("2006-01-02 15:04:05") + `</td><td>` + strconv.Itoa(row.IdleSec) + `s</td></tr>`)
		}
	}
	if onlineRows.Len() == 0 {
		onlineRows.WriteString(`<tr><td colspan="5">No users currently online</td></tr>`)
	}

	callerRows := strings.Builder{}
	for _, caller := range callerHistory {
		callerRows.WriteString(`<tr><td>` + caller.LogoutAt.Format("2006-01-02 15:04:05") + `</td><td>Node ` + strconv.Itoa(caller.NodeID) + `</td><td>` + htmlEscape(caller.Username) + `</td><td>` + htmlEscape(caller.Area) + `</td><td>` + strconv.FormatInt(caller.DurationSeconds, 10) + `s</td></tr>`)
	}
	if callerRows.Len() == 0 {
		callerRows.WriteString(`<tr><td colspan="5">No caller history available</td></tr>`)
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>System / WFC Dashboard</title></head><body><h1>System / WFC Dashboard</h1><p><a href="/admin">back</a> | <a href="/admin/setup">setup</a> | <a href="/admin/config">config</a> | <a href="/admin/errors">errors</a> | <a href="/admin/node-state">node-state json</a> | <a href="/help">help</a></p>` +
		`<table border="1"><tr><th>Metric</th><th>Value</th></tr>` +
		`<tr><td>Site name</td><td>` + htmlEscape(a.siteDisplayName()) + `</td></tr>` +
		`<tr><td>Site hostname</td><td>` + htmlEscape(a.siteHost()) + `</td></tr>` +
		`<tr><td>Read-only mode</td><td>` + boolToText(a.readOnly) + `</td></tr>` +
		`<tr><td>Secure cookie mode</td><td>` + boolToText(a.secureCookie) + `</td></tr>` +
		`<tr><td>Require verified external email</td><td>` + boolToText(a.requireVerifiedEmail) + `</td></tr>` +
		`<tr><td>Uptime</td><td>` + uptime + `</td></tr>` +
		`<tr><td>Web sessions</td><td>` + strconv.Itoa(webSessions) + `</td></tr>` +
		`<tr><td>Users</td><td>` + strconv.Itoa(userCount) + `</td></tr>` +
		`<tr><td>Boards</td><td>` + strconv.Itoa(boardCount) + `</td></tr>` +
		`<tr><td>Messages</td><td>` + strconv.Itoa(messageCount) + `</td></tr>` +
		`<tr><td>Chat channels</td><td>` + strconv.Itoa(len(channels)) + `</td></tr>` +
		`<tr><td>Locked channels</td><td>` + strconv.Itoa(lockedCount) + `</td></tr>` +
		`<tr><td>Online users</td><td>` + strconv.Itoa(len(online)) + `</td></tr>` +
		`<tr><td>Node sessions (persisted)</td><td>` + strconv.Itoa(len(nodeSessions)) + `</td></tr>` +
		`<tr><td>Caller history rows</td><td>` + strconv.Itoa(len(callerHistory)) + `</td></tr>` +
		`<tr><td>Runtime errors</td><td>` + strconv.Itoa(errorCount) + `</td></tr>` +
		`<tr><td>Mods (running/total)</td><td>` + strconv.Itoa(modRunning) + ` / ` + strconv.Itoa(modCount) + `</td></tr>` +
		`<tr><td>OneLinerz entries</td><td>` + strconv.Itoa(oneLinerCount) + `</td></tr>` +
		`<tr><td>Rumorz active line</td><td>` + htmlEscape(activeRumor) + `</td></tr>` +
		`<tr><td>Network inbound packets</td><td>` + strconv.Itoa(networkInbound) + `</td></tr>` +
		`<tr><td>Network outbound packets</td><td>` + strconv.Itoa(networkOutbound) + `</td></tr>` +
		`<tr><td>MOTD</td><td>` + htmlEscape(cleanOneLiner(a.motd, 80)) + `</td></tr>` +
		`<tr><td>Announcement</td><td>` + htmlEscape(cleanOneLiner(a.announcement, 80)) + `</td></tr>` +
		`</table>` +
		`<h2>Online / Node State</h2><table border="1"><tr><th>User</th><th>Node</th><th>Area</th><th>Login</th><th>Idle</th></tr>` + onlineRows.String() + `</table>` +
		`<h2>Last Callers</h2><table border="1"><tr><th>Logout</th><th>Node</th><th>User</th><th>Area</th><th>Duration</th></tr>` + callerRows.String() + `</table>` +
		`<p>Health endpoints: <a href="/healthz">/healthz</a> | <a href="/readyz">/readyz</a> | <a href="/metrics">/metrics</a></p>` +
		`</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminNodeState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	sessions := []domain.NodeSession{}
	callers := []domain.CallerHistory{}
	if a.adminRepo != nil {
		sessions, _ = a.adminRepo.ListNodeSessions(200)
		callers, _ = a.adminRepo.ListCallerHistory(200)
	}
	chatOnline := 0
	chatChannels := 0
	a.Lock()
	webSessionEstimate := len(a.sessions)
	a.Unlock()
	if a.chatSvc != nil {
		chatOnline = len(a.chatSvc.Online())
		chatChannels = len(a.chatSvc.ListChannels())
	}
	now := time.Now().UTC()
	type nodeRow struct {
		SessionID    string `json:"session_id"`
		NodeID       int    `json:"node_id"`
		Username     string `json:"username"`
		Area         string `json:"area"`
		RemoteAddr   string `json:"remote_addr"`
		LoginAt      string `json:"login_at"`
		LastActivity string `json:"last_activity"`
		IdleSeconds  int64  `json:"idle_seconds"`
	}
	nodes := make([]nodeRow, 0, len(sessions))
	for _, row := range sessions {
		idle := now.Sub(row.LastActivity)
		if idle < 0 {
			idle = 0
		}
		nodes = append(nodes, nodeRow{
			SessionID:    row.SessionID,
			NodeID:       row.NodeID,
			Username:     row.Username,
			Area:         row.Area,
			RemoteAddr:   row.RemoteAddr,
			LoginAt:      row.LoginAt.UTC().Format(time.RFC3339),
			LastActivity: row.LastActivity.UTC().Format(time.RFC3339),
			IdleSeconds:  int64(idle.Seconds()),
		})
	}
	_ = writeJSON(w, http.StatusOK, map[string]interface{}{
		"generated_at":    now.Format(time.RFC3339),
		"node_sessions":   nodes,
		"caller_history":  callers,
		"session_count":   len(nodes),
		"caller_count":    len(callers),
		"chat_online":     chatOnline,
		"chat_channels":   chatChannels,
		"web_session_est": webSessionEstimate,
	})
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
	_, _ = w.Write([]byte(`<html><body><h1>Admin Audit Log</h1><p><a href="/admin">back</a> | <a href="/help">help</a></p><table border="1"><tr><th>Time</th><th>Actor</th><th>Target</th><th>Action</th><th>Details</th></tr>` + rows.String() + `</table></body></html>`))
}

func (a *webApp) canReadBoard(user *domain.User, board *domain.Board) bool {
	if board == nil {
		return false
	}
	return a.evalACS(boardReadRuleForBoard(board), user, map[string]string{
		"area":       "boards",
		"mode":       "read",
		"board_id":   strconv.FormatInt(board.ID, 10),
		"board":      board.Name,
		"conference": defaultConferenceValue(board.Conference),
	})
}

func (a *webApp) canWriteBoard(user *domain.User, board *domain.Board) bool {
	if board == nil {
		return false
	}
	return a.evalACS(boardWriteRuleForBoard(board), user, map[string]string{
		"area":       "boards",
		"mode":       "post",
		"board_id":   strconv.FormatInt(board.ID, 10),
		"board":      board.Name,
		"conference": defaultConferenceValue(board.Conference),
	})
}

func (a *webApp) canReadMail(user *domain.User) bool {
	return a.evalACS(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_MAIL_READ")), user, map[string]string{
		"area": "mail",
		"mode": "read",
	})
}

func (a *webApp) canSendMail(user *domain.User) bool {
	return a.evalACS(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_MAIL_SEND")), user, map[string]string{
		"area": "mail",
		"mode": "compose",
	})
}

func (a *webApp) canReadFiles(user *domain.User, mode string) bool {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "read"
	}
	return a.evalACS(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_FILES_READ")), user, map[string]string{
		"area": "files",
		"mode": mode,
	})
}

func (a *webApp) canAccessAdminPath(user *domain.User, path string) bool {
	path = strings.TrimSpace(path)
	return a.evalACS(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_ADMIN")), user, map[string]string{
		"area": "admin",
		"path": path,
		"mode": "admin",
	})
}

func (a *webApp) evalACS(expr string, user *domain.User, attrs map[string]string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true
	}
	if attrs == nil {
		attrs = map[string]string{}
	}
	role := roleUser
	verified := false
	handle := ""
	if user != nil {
		role = rbac.NormalizeRole(user.Role)
		verified = user.Verified
		handle = strings.TrimSpace(user.Handle)
	}
	attrs["role"] = role
	attrs["verified"] = boolToText(verified)
	attrs["handle"] = handle
	allowed, err := acs.Evaluate(expr, acs.Context{
		Role:     role,
		Verified: verified,
		Attrs:    attrs,
	})
	if err != nil {
		return !envEnabledDefault("WOLFBBS_ACS_STRICT", false)
	}
	return allowed
}

func boardReadRuleForBoard(board *domain.Board) string {
	if board == nil {
		return strings.TrimSpace(os.Getenv("WOLFBBS_ACS_BOARDS_READ"))
	}
	if rule := strings.TrimSpace(board.ReadACS); rule != "" {
		return rule
	}
	return strings.TrimSpace(os.Getenv("WOLFBBS_ACS_BOARDS_READ"))
}

func boardWriteRuleForBoard(board *domain.Board) string {
	if board == nil {
		return strings.TrimSpace(os.Getenv("WOLFBBS_ACS_BOARDS_POST"))
	}
	if rule := strings.TrimSpace(board.WriteACS); rule != "" {
		return rule
	}
	return strings.TrimSpace(os.Getenv("WOLFBBS_ACS_BOARDS_POST"))
}

func defaultConferenceValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "General"
	}
	return value
}

func (a *webApp) roleForUser(u *domain.User) int {
	if u == nil {
		return 0
	}
	return roleWeight[rbac.NormalizeRole(u.Role)]
}

func (a *webApp) hasRole(u *domain.User, minimum string) bool {
	return a.roleForUser(u) >= roleWeight[rbac.NormalizeRole(minimum)]
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
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.URL.Path)), "/admin") && !a.canAccessAdminPath(u, r.URL.Path) {
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

func (a *webApp) loadPersistedAdminSettings() {
	if a.adminRepo == nil {
		return
	}
	settings, err := a.adminRepo.ListSystemSettings()
	if err != nil {
		a.addAppError("startup", fmt.Errorf("load system settings: %w", err))
		return
	}
	if len(settings) == 0 {
		return
	}
	applyText := func(key string, target *string) {
		if target == nil {
			return
		}
		value, ok := settings[key]
		if !ok {
			return
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		*target = value
	}
	applyBool := func(key string, target *bool) {
		if target == nil {
			return
		}
		value, ok := settings[key]
		if !ok {
			return
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		*target = parseCheckbox(value)
	}
	if value, ok := settings[sysSettingSiteName]; ok {
		value = strings.TrimSpace(value)
		if value != "" {
			a.siteName = value
		}
	}
	if value, ok := settings[sysSettingSiteHostname]; ok {
		value = strings.TrimSpace(value)
		if value != "" {
			a.siteHostname = value
		}
	}
	if value, ok := settings[sysSettingMOTD]; ok {
		a.motd = strings.TrimSpace(value)
	}
	if value, ok := settings[sysSettingAnnouncement]; ok {
		a.announcement = strings.TrimSpace(value)
	}
	if value, ok := settings[sysSettingReadOnly]; ok && strings.TrimSpace(value) != "" {
		a.readOnly = parseCheckbox(value)
	}
	if value, ok := settings[sysSettingSecureCookie]; ok && strings.TrimSpace(value) != "" {
		a.secureCookie = parseCheckbox(value)
	}
	if value, ok := settings[sysSettingRequireVerifiedEmail]; ok && strings.TrimSpace(value) != "" {
		a.requireVerifiedEmail = parseCheckbox(value)
	}
	if value, ok := settings[sysSettingWebOnRamp]; ok && strings.TrimSpace(value) != "" {
		a.modernOnRamp = parseCheckbox(value)
	}
	if value, ok := settings[sysSettingGuestTour]; ok && strings.TrimSpace(value) != "" {
		a.guestTour = parseCheckbox(value)
	}
	if value, ok := settings[sysSettingDiscover]; ok && strings.TrimSpace(value) != "" {
		a.discover = parseCheckbox(value)
	}
	if value, ok := settings[sysSettingQuickJump]; ok && strings.TrimSpace(value) != "" {
		a.quickJump = parseCheckbox(value)
	}
	if value, ok := settings[sysSettingClassicSearch]; ok && strings.TrimSpace(value) != "" {
		a.classicSearch = parseCheckbox(value)
	}
	if value, ok := settings[sysSettingMenuEnabled]; ok && strings.TrimSpace(value) != "" {
		a.runtimeCfg.Menu.Enabled = parseCheckbox(value)
	}
	if value, ok := settings[sysSettingMenuFile]; ok {
		value = strings.TrimSpace(value)
		if value != "" {
			a.runtimeCfg.Menu.File = value
		}
	}
	applyBool(sysSettingACSStrict, &a.runtimeCfg.ACS.Strict)
	applyText(sysSettingContentHost, &a.runtimeCfg.Content.Host)
	applyText(sysSettingContentGopherListen, &a.runtimeCfg.Content.GopherListen)
	applyText(sysSettingContentNNTPListen, &a.runtimeCfg.Content.NNTPListen)
	applyText(sysSettingContentNNTPSListen, &a.runtimeCfg.Content.NNTPSListen)
	applyText(sysSettingContentNNTPSCert, &a.runtimeCfg.Content.NNTPSCert)
	applyText(sysSettingContentNNTPSKey, &a.runtimeCfg.Content.NNTPSKey)
	applyBool(sysSettingActivityPubEnabled, &a.runtimeCfg.ActivityPub.Enabled)
	applyText(sysSettingActivityPubBaseURL, &a.runtimeCfg.ActivityPub.BaseURL)
	applyBool(sysSettingLoginTelnetEnabled, &a.runtimeCfg.Login.Telnet.Enabled)
	applyText(sysSettingLoginTelnetListen, &a.runtimeCfg.Login.Telnet.Listen)
	applyBool(sysSettingLoginWSEnabled, &a.runtimeCfg.Login.WebSocket.Enabled)
	applyText(sysSettingLoginWSListen, &a.runtimeCfg.Login.WebSocket.Listen)
	applyText(sysSettingLoginWSPath, &a.runtimeCfg.Login.WebSocket.Path)
	applyBool(sysSettingLoginWSSEnabled, &a.runtimeCfg.Login.WebSocketTLS.Enabled)
	applyText(sysSettingLoginWSSListen, &a.runtimeCfg.Login.WebSocketTLS.Listen)
	applyText(sysSettingLoginWSSPath, &a.runtimeCfg.Login.WebSocketTLS.Path)
	applyText(sysSettingLoginWSSCert, &a.runtimeCfg.Login.WebSocketTLS.Cert)
	applyText(sysSettingLoginWSSKey, &a.runtimeCfg.Login.WebSocketTLS.Key)
	applyText(sysSettingTrustedProxies, &a.runtimeCfg.Login.TrustedProxies)
	applyBool(sysSettingConnectorDoorPartyOn, &a.runtimeCfg.Connectors.DoorParty.Enabled)
	applyText(sysSettingConnectorDoorPartyCmd, &a.runtimeCfg.Connectors.DoorParty.Command)
	applyText(sysSettingConnectorDoorPartyArgs, &a.runtimeCfg.Connectors.DoorParty.Args)
	applyBool(sysSettingConnectorBBSLinkOn, &a.runtimeCfg.Connectors.BBSLink.Enabled)
	applyText(sysSettingConnectorBBSLinkCmd, &a.runtimeCfg.Connectors.BBSLink.Command)
	applyText(sysSettingConnectorBBSLinkArgs, &a.runtimeCfg.Connectors.BBSLink.Args)
	applyBool(sysSettingConnectorTelnetOn, &a.runtimeCfg.Connectors.Telnet.Enabled)
	applyText(sysSettingConnectorTelnetCmd, &a.runtimeCfg.Connectors.Telnet.Command)
	applyText(sysSettingConnectorTelnetArgs, &a.runtimeCfg.Connectors.Telnet.Args)
	a.loadLockedChannels(settings[sysSettingLockedChannels])
}

func (a *webApp) loadLockedChannels(raw string) {
	a.Lock()
	defer a.Unlock()
	if a.lockedChat == nil {
		a.lockedChat = map[string]bool{}
	}
	for _, part := range strings.Split(raw, ",") {
		channel := chat.NormalizeChannel(strings.TrimSpace(part))
		if strings.TrimSpace(channel) == "" {
			continue
		}
		a.lockedChat[channel] = true
	}
}

func (a *webApp) persistSystemSetting(key, value string) {
	if a.adminRepo == nil {
		return
	}
	if err := a.adminRepo.UpsertSystemSetting(key, value); err != nil {
		a.addAppError("admin.config", fmt.Errorf("save %s: %w", key, err))
	}
}

func (a *webApp) persistLockedChannels() {
	a.Lock()
	channels := make([]string, 0, len(a.lockedChat))
	for channel, locked := range a.lockedChat {
		if locked {
			channels = append(channels, channel)
		}
	}
	a.Unlock()
	sort.Slice(channels, func(i, j int) bool { return strings.ToLower(channels[i]) < strings.ToLower(channels[j]) })
	a.persistSystemSetting(sysSettingLockedChannels, strings.Join(channels, ","))
}

func (a *webApp) setChannelLock(channel string, locked bool) {
	channel = chat.NormalizeChannel(channel)
	if strings.TrimSpace(channel) == "" {
		return
	}
	a.Lock()
	if a.lockedChat == nil {
		a.lockedChat = map[string]bool{}
	}
	if locked {
		a.lockedChat[channel] = true
	} else {
		delete(a.lockedChat, channel)
	}
	a.Unlock()
	a.persistLockedChannels()
}

func (a *webApp) isChannelLocked(channel string) bool {
	channel = chat.NormalizeChannel(channel)
	a.Lock()
	defer a.Unlock()
	return a.lockedChat[channel]
}

func (a *webApp) addAppError(area string, err error) {
	if err == nil {
		return
	}
	entry := appErrorEntry{
		Time:    time.Now().UTC(),
		Area:    strings.TrimSpace(area),
		Message: strings.TrimSpace(err.Error()),
	}
	if entry.Area == "" {
		entry.Area = "runtime"
	}
	a.Lock()
	a.errorLog = append(a.errorLog, entry)
	if len(a.errorLog) > maxAdminErrorEntries {
		a.errorLog = append([]appErrorEntry{}, a.errorLog[len(a.errorLog)-maxAdminErrorEntries:]...)
	}
	a.Unlock()
	log.Printf("area=%s error=%s", entry.Area, entry.Message)
}

func (a *webApp) latestErrors(limit int) []appErrorEntry {
	if limit <= 0 {
		limit = 50
	}
	a.Lock()
	defer a.Unlock()
	total := len(a.errorLog)
	if total == 0 {
		return nil
	}
	if limit > total {
		limit = total
	}
	start := total - limit
	out := make([]appErrorEntry, 0, limit)
	for i := total - 1; i >= start; i-- {
		out = append(out, a.errorLog[i])
	}
	return out
}

func boolToText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func (a *webApp) siteDisplayName() string {
	if a == nil {
		return "WolfBBS"
	}
	name := strings.TrimSpace(a.siteName)
	if name == "" {
		name = "WolfBBS"
	}
	return name
}

func (a *webApp) siteHost() string {
	if a == nil {
		return "localhost"
	}
	host := strings.TrimSpace(a.siteHostname)
	if host == "" {
		host = "localhost"
	}
	return host
}

func checkedAttr(active bool) string {
	if active {
		return "checked"
	}
	return ""
}

func buildThemeOptionsHTML(current string) string {
	current = strings.TrimSpace(current)
	options := ui.ThemeNames()
	found := false
	var b strings.Builder
	for _, name := range options {
		selected := ""
		if strings.EqualFold(name, current) {
			selected = " selected"
			found = true
		}
		b.WriteString(`<option value="` + htmlEscape(name) + `"` + selected + `>` + htmlEscape(name) + `</option>`)
	}
	if !found && current != "" {
		b.WriteString(`<option value="` + htmlEscape(current) + `" selected>` + htmlEscape(current) + ` (custom)</option>`)
	}
	return b.String()
}

func parseCheckbox(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "1" || value == "true" || value == "on" || value == "yes"
}

func envEnabledDefault(name string, def bool) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	return parseCheckbox(raw)
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
		<h1>` + htmlEscape(a.siteDisplayName()) + ` Chat</h1>
		<p>Logged in as ` + user.Handle + `</p>
		<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/settings">settings</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
		<p><strong>Quick keys:</strong> Enter sends message, Ctrl+L clears chat pane, channel selector switches rooms instantly.</p>
		<p><label>Channel:
			<select id="channelSelect"></select>
		</label></p>
		<div id="chat" style="height:300px; width: 800px; border:1px solid #333; overflow:auto; font-family: monospace; white-space: pre;"></div>
		<p id="chatStatus" style="font-family: monospace;">Ready.</p>
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
			function setStatus(msg) {
				const el = document.getElementById('chatStatus');
				if (el) el.textContent = msg;
			}

			function msgValue(m, primary, legacy, fallback) {
				if (m && m[primary] !== undefined && m[primary] !== null && m[primary] !== '') return m[primary];
				if (m && legacy && m[legacy] !== undefined && m[legacy] !== null && m[legacy] !== '') return m[legacy];
				return fallback;
			}

			function formatLine(m) {
				const stamp = msgValue(m, 'created_at', 'CreatedAt', '--:--:--');
				const from = msgValue(m, 'from', 'From', 'system');
				const body = msgValue(m, 'body', 'Body', '');
				return '[' + stamp + '] ' + from + ': ' + body;
			}

			async function loadChannels() {
				const res = await fetch('/chat/channels', {credentials: 'same-origin'});
				if (!res.ok) {
					setStatus('Failed to load channels.');
					return;
				}
				const payload = await res.json();
				const select = document.getElementById('channelSelect');
				select.innerHTML = '';
				const channels = payload.channels || ['#lobby'];
				const lockedSet = new Set(payload.locked || []);
				channels.forEach((name) => {
					const option = document.createElement('option');
					option.value = name;
					option.textContent = lockedSet.has(name) ? (name + ' [locked]') : name;
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
				if (!res.ok) {
					setStatus('Could not load history for ' + ch);
					return;
				}
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
				if (!res.ok) {
					setStatus('Could not load online list.');
					return;
				}
				const payload = await res.json();
				const online = document.getElementById('online');
				const names = (payload.presence || []).map((p) => p.nick).join(', ');
				online.textContent = names || 'none';
			}

			async function join() {
				const ch = streamState.channel;
				const res = await fetch('/chat/join', {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json',
						'X-CSRF-Token': csrf,
					},
					body: JSON.stringify({ channel: ch }),
				});
				if (!res.ok) {
					setStatus('Join failed for ' + ch);
				}
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
					setStatus('Live on ' + streamState.channel);
				};
				es.onerror = function() {
					es.close();
					setStatus('Realtime disconnected; retrying...');
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
				const sendRes = await fetch('/chat/send', {
					method:'POST',
					headers:{'Content-Type':'application/json','X-CSRF-Token': csrf},
					body: JSON.stringify({channel: streamState.channel, message: message}),
				});
				if (!sendRes.ok) {
					const body = await sendRes.text();
					setStatus('Send failed: ' + body);
					return;
				}
				document.getElementById('message').value = '';
				setStatus('Sent to ' + streamState.channel);
				await loadHistory();
			});

			document.getElementById('message').addEventListener('keydown', function(evt){
				if (evt.key === 'l' && evt.ctrlKey) {
					evt.preventDefault();
					const box = document.getElementById('chat');
					box.textContent = '';
					setStatus('Chat pane cleared.');
				}
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
	if a.isChannelLocked(channel) && !a.hasRole(user, roleModerator) {
		http.Error(w, "channel is locked", http.StatusForbidden)
		return
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
	payload, err := json.Marshal(encodeChatMessage(msg))
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

func encodeChatMessage(msg chat.Message) chatMessageResponse {
	return chatMessageResponse{
		ID:        msg.ID,
		From:      msg.From,
		Body:      msg.Body,
		CreatedAt: msg.CreatedAt.Format("15:04:05"),
		Channel:   msg.Channel,
		To:        msg.To,
	}
}

func (a *webApp) handleChatChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	channels := a.chatSvc.ListChannels()
	locked := make([]string, 0)
	for _, channel := range channels {
		if a.isChannelLocked(channel) {
			locked = append(locked, channel)
		}
	}
	_ = writeJSON(w, http.StatusOK, map[string]interface{}{
		"channels": channels,
		"locked":   locked,
	})
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
	if a.isChannelLocked(channel) && !a.hasRole(user, roleModerator) {
		http.Error(w, "channel is locked", http.StatusForbidden)
		return
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
	encoded := make([]chatMessageResponse, 0, len(msgs))
	for _, msg := range msgs {
		encoded = append(encoded, encodeChatMessage(msg))
	}
	var last int64
	if len(msgs) > 0 {
		last = msgs[len(msgs)-1].ID
	}
	presence := a.chatSvc.OnlineInChannel(channel)
	_ = writeJSON(w, http.StatusOK, chatHistoryResponse{
		Channel:  channel,
		Messages: encoded,
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

func (a *webApp) latestLogins(limit int) []string {
	if limit <= 0 {
		limit = 6
	}
	users, err := a.authSvc.ListUsers()
	if err != nil {
		return nil
	}
	sort.Slice(users, func(i, j int) bool {
		var li, lj time.Time
		if users[i].LastLoginAt != nil {
			li = users[i].LastLoginAt.UTC()
		}
		if users[j].LastLoginAt != nil {
			lj = users[j].LastLoginAt.UTC()
		}
		if li.Equal(lj) {
			return strings.ToLower(users[i].Handle) < strings.ToLower(users[j].Handle)
		}
		return li.After(lj)
	})
	rows := make([]string, 0, limit)
	for _, user := range users {
		if user.LastLoginAt == nil || user.LastLoginAt.IsZero() {
			continue
		}
		rows = append(rows, fmt.Sprintf("%s @ %s", user.Handle, user.LastLoginAt.Local().Format("01-02 15:04")))
		if len(rows) >= limit {
			break
		}
	}
	return rows
}

func (a *webApp) featuredThreadLine() string {
	if a.boardRepo == nil || a.msgRepo == nil {
		return ""
	}
	boards, err := a.boardRepo.List()
	if err != nil {
		return ""
	}
	var selected *domain.Message
	boardName := ""
	for _, board := range boards {
		msgs, listErr := a.msgRepo.ListByBoard(board.ID)
		if listErr != nil || len(msgs) == 0 {
			continue
		}
		last := msgs[len(msgs)-1]
		if selected == nil || last.CreatedAt.After(selected.CreatedAt) {
			copy := last
			selected = &copy
			boardName = board.Name
		}
	}
	if selected == nil {
		return ""
	}
	return fmt.Sprintf("%s / %s", cleanOneLiner(boardName, 20), cleanOneLiner(selected.Subject, 64))
}

func (a *webApp) filebaseDownloadPick() string {
	if a.doorRepo == nil {
		return "FileBase Pro picks appear after uploads."
	}
	row, err := a.doorRepo.GetGlobalState("filebase-pro")
	if err != nil || row == nil || strings.TrimSpace(row.StateJSON) == "" {
		return "No file uploads yet. Check FileBase Pro later."
	}
	var state struct {
		Files []struct {
			Filename    string `json:"filename"`
			Description string `json:"description"`
			Downloads   int64  `json:"downloads"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(row.StateJSON), &state); err != nil || len(state.Files) == 0 {
		return "No file uploads yet. Check FileBase Pro later."
	}
	sort.Slice(state.Files, func(i, j int) bool {
		if state.Files[i].Downloads == state.Files[j].Downloads {
			return state.Files[i].Filename < state.Files[j].Filename
		}
		return state.Files[i].Downloads > state.Files[j].Downloads
	})
	pick := state.Files[0]
	desc := cleanOneLiner(pick.Description, 52)
	if desc == "" {
		desc = "classic upload"
	}
	return fmt.Sprintf("%s (%d dl) - %s", cleanOneLiner(pick.Filename, 24), pick.Downloads, desc)
}

func (a *webApp) addSavedSearch(handle, query string) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	query = strings.TrimSpace(query)
	if handle == "" || query == "" {
		return
	}
	a.Lock()
	defer a.Unlock()
	if a.savedSearches == nil {
		a.savedSearches = map[string][]string{}
	}
	current := a.savedSearches[handle]
	for _, row := range current {
		if strings.EqualFold(strings.TrimSpace(row), query) {
			return
		}
	}
	current = append([]string{query}, current...)
	if len(current) > 10 {
		current = current[:10]
	}
	a.savedSearches[handle] = current
}

func (a *webApp) savedSearchList(handle string) []string {
	handle = strings.ToLower(strings.TrimSpace(handle))
	if handle == "" {
		return nil
	}
	a.Lock()
	defer a.Unlock()
	rows := a.savedSearches[handle]
	out := make([]string, len(rows))
	copy(out, rows)
	return out
}

func (a *webApp) searchRows(query string, limit int) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || a.boardRepo == nil || a.msgRepo == nil {
		return nil
	}
	boards, err := a.boardRepo.List()
	if err != nil {
		return nil
	}
	rows := make([]string, 0, limit)
	for _, board := range boards {
		msgs, listErr := a.msgRepo.ListByBoard(board.ID)
		if listErr != nil {
			continue
		}
		for _, msg := range msgs {
			haystack := strings.ToLower(msg.Subject + "\n" + msg.Body)
			if !strings.Contains(haystack, query) {
				continue
			}
			rows = append(rows, fmt.Sprintf("%s #%d: %s", board.Name, msg.ID, cleanOneLiner(msg.Subject, 48)))
			if len(rows) >= limit {
				return rows
			}
		}
	}
	return rows
}

func cleanOneLiner(value string, limit int) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	r := []rune(value)
	if limit <= 1 {
		return string(r[:limit])
	}
	return string(r[:limit-1]) + "…"
}

func htmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, `"`, "&quot;")
	return value
}

func pageMessageBlock(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	notice := strings.TrimSpace(r.URL.Query().Get("notice"))
	errText := strings.TrimSpace(r.URL.Query().Get("error"))
	out := strings.Builder{}
	if notice != "" {
		out.WriteString(`<p><strong>Notice:</strong> ` + htmlEscape(notice) + `</p>`)
	}
	if errText != "" {
		out.WriteString(`<p><strong>Error:</strong> ` + htmlEscape(errText) + `</p>`)
	}
	return out.String()
}

func redirectWithNotice(w http.ResponseWriter, r *http.Request, path, notice string) {
	redirectWithQueryMessage(w, r, path, "notice", notice)
}

func redirectWithError(w http.ResponseWriter, r *http.Request, path, errText string) {
	redirectWithQueryMessage(w, r, path, "error", errText)
}

func redirectWithQueryMessage(w http.ResponseWriter, r *http.Request, path, key, value string) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		http.Redirect(w, r, path, http.StatusFound)
		return
	}
	u, err := url.Parse(path)
	if err != nil {
		http.Redirect(w, r, path, http.StatusFound)
		return
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
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

func parseIntWithFallback(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func maxInt64(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func envFirst(names ...string) string {
	for _, name := range names {
		value := strings.TrimSpace(os.Getenv(name))
		if value != "" {
			return value
		}
	}
	return ""
}

func formHasValue(r *http.Request, key string) bool {
	if r == nil {
		return false
	}
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return false
	}
	switch strings.ToLower(value) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

func webQuickJumpPath(raw string) string {
	target := strings.ToLower(strings.TrimSpace(raw))
	switch target {
	case "boards", "messages", "msg", "m":
		return "/boards"
	case "mail", "pm", "p":
		return "/mail"
	case "chat", "c":
		return "/chat"
	case "gateway", "g":
		return "/gateway"
	case "settings", "prefs", "s":
		return "/settings"
	case "status", "health", "y":
		return "/status"
	case "config", "cfg", "x":
		return "/config"
	case "discover", "newscan", "n":
		return "/discover"
	case "scores", "doors", "d":
		return "/scores"
	case "admin", "a":
		return "/admin"
	default:
		return ""
	}
}

func checkedIf(active bool) string {
	if active {
		return ` checked`
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (a *webApp) mustDoorEvents(doorID string, userID int64, limit int) []domain.DoorEvent {
	if a.doorRegistry == nil {
		return nil
	}
	rows, err := a.doorRegistry.ListEvents(doorID, userID, limit)
	if err != nil {
		return nil
	}
	return rows
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

func loginPage(siteName, path string, showConnect bool, showTour bool) string {
	siteName = strings.TrimSpace(siteName)
	if siteName == "" {
		siteName = "WolfBBS"
	}
	title := htmlEscape(siteName)
	extra := strings.Builder{}
	if showConnect {
		extra.WriteString(`<p><a href="/connect">Quick connect</a></p>`)
	}
	if showTour {
		extra.WriteString(`<p><a href="/tour">Guided guest tour</a></p>`)
	}
	postPath := "/login"
	if strings.HasPrefix(strings.TrimSpace(path), "/admin") {
		postPath = "/admin/login"
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>` + title + ` Login</title></head><body>
	<h1>` + title + ` Web Login</h1>
	<p><a href="/help">Help</a></p>
	<form method="POST" action="` + postPath + `">
		<label>Handle: <input name="handle"></label><br>
		<label>Password: <input name="password" type="password"></label><br>
		<label>2FA code: <input name="totp"></label><br>
		<button type="submit">Sign In</button>
	</form>
	<p><a href="/reset/request">Forgot password?</a></p>
	` + extra.String() + `
	</body></html>`
}

func resetRequestPage(siteName, message string) string {
	siteName = strings.TrimSpace(siteName)
	if siteName == "" {
		siteName = "WolfBBS"
	}
	title := htmlEscape(siteName)
	if strings.TrimSpace(message) != "" {
		message = `<p>` + message + `</p>`
	}
	return `<html><body>
	<h1>` + title + ` Password Reset</h1>
	<p><a href="/help">Help</a></p>
	<p>Enter your handle and we will issue a reset token.</p>
	<form method="POST" action="/reset/request">
		<label>Handle: <input name="handle"></label><br>
		<button type="submit">Issue Reset Token</button>
	</form>` + message + `
	<p><a href="/login">Back to login</a></p>
	</body></html>`
}

func resetCompletePage(siteName, token, message string) string {
	siteName = strings.TrimSpace(siteName)
	if siteName == "" {
		siteName = "WolfBBS"
	}
	title := htmlEscape(siteName)
	token = htmlEscape(strings.TrimSpace(token))
	message = strings.TrimSpace(message)
	if message != "" {
		message = `<p>` + htmlEscape(message) + `</p>`
	}
	return `<html><body>
	<h1>` + title + ` Set New Password</h1>
	<p><a href="/help">Help</a></p>
	<form method="POST" action="/reset/complete">
		<input type="hidden" name="token" value="` + token + `">
		<label>Reset token: <input name="token" value="` + token + `" size="70"></label><br>
		<label>New password: <input name="password" type="password"></label><br>
		<button type="submit">Reset Password</button>
	</form>` + message + `
	<p><a href="/login">Back to login</a></p>
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
