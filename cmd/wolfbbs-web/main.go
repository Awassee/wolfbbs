package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
	"wolfbbs/internal/netutil"
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

type webDoorView struct {
	Door                 doors.Door
	TurnsRemaining       int
	Favorite             bool
	Recent               bool
	PlayCount            int
	LastPlayed           string
	PersonalAchievements int
	TopScore             int64
	TopScoreHandle       string
	DailyActive          int
	MonthlyActive        int
	TotalPlays           int64
	RecommendedScore     int
}

type boardsDashboardSnapshot struct {
	VisibleBoards     int
	UnreadPosts       int
	UnreadMail        int
	OnlineUsers       int
	FavoriteDoors     int
	RecommendedDoor   string
	RecommendedDoorID string
	RecentCallers     []string
	OneLiners         []string
}

type boardPulseRow struct {
	BoardID      int64
	BoardName    string
	Conference   string
	MessageCount int
	NewCount     int
	LastAt       string
	LastSubject  string
	Heat         int
}

type callerRadarRow struct {
	Handle   string
	Node     string
	Area     string
	Since    string
	Idle     string
	Origin   string
	From     string
	Duration string
}

type scoreChampion struct {
	DoorID    string
	DoorName  string
	Handle    string
	Score     int64
	ScoreType string
	CreatedAt string
}

type radarSnapshot struct {
	UnreadMail         int
	UnreadPosts        int
	OnlineUsers        int
	LiveNodes          int
	TrackedBoards      int
	ActivityItems      []string
	BoardPulse         []boardPulseRow
	LiveCallers        []callerRadarRow
	RecentCallers      []callerRadarRow
	RecommendedDoors   []webDoorView
	RecentAchievements []domain.DoorAchievement
	Rumor              string
}

type attentionActionRow struct {
	Label       string
	Href        string
	Meta        string
	ItemKey     string
	Read        bool
	Dismissible bool
}

type communityEvent struct {
	ID          string    `json:"id"`
	SeriesID    string    `json:"series_id,omitempty"`
	Title       string    `json:"title"`
	Category    string    `json:"category"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	Recurrence  string    `json:"recurrence,omitempty"`
	RepeatUntil time.Time `json:"repeat_until,omitempty"`
	Location    string    `json:"location"`
	Host        string    `json:"host"`
	Audience    string    `json:"audience"`
	Description string    `json:"description"`
	Link        string    `json:"link"`
	CreatedAt   time.Time `json:"created_at"`
}

type scoreboardSnapshot struct {
	FilterDoor           string
	DoorsWithScores      int
	VisibleScoreRows     int
	PersonalAchievements int
	ChampionRows         []scoreChampion
	RecentRows           []scoreChampion
	PersonalRows         []scoreChampion
}

type bulletinSnapshot struct {
	SystemWire     []string
	DigestItems    []string
	HotBoards      []string
	RecentCallers  []string
	OneLiners      []string
	RecentFiles    []string
	FeaturedThread string
	DownloadPick   string
}

type directoryRow struct {
	Handle    string
	Role      string
	Verified  bool
	Theme     string
	LastLogin string
	Online    bool
	Area      string
	Origin    string
}

type directoryProfile struct {
	Handle           string
	Role             string
	Theme            string
	Verified         bool
	LastLogin        string
	Online           bool
	OnlineArea       string
	OnlineOrigin     string
	Posts            int
	Mentions         int
	Replies          int
	MailSent         int
	MailReceived     int
	FavoriteDoor     string
	Achievements     int
	RecentCallerRows []string
}

type messageSearchHit struct {
	BoardID    int64
	BoardName  string
	Conference string
	MessageID  int64
	Subject    string
	Author     string
	AuthorID   int64
	CreatedAt  string
	Snippet    string
}

type newFilesSnapshot struct {
	RecentUploads []domain.FileEntry
	TopRated      []domain.FileEntry
	SavedFilters  []domain.FileFilter
	Queue         []domain.DownloadQueueItem
	QueueNames    map[int64]string
	AreaNames     map[int64]string
}

type threadTrackerItem struct {
	Kind      string
	BoardID   int64
	MessageID int64
	BoardName string
	Subject   string
	CreatedAt string
}

type boardMenuRow struct {
	Board        domain.Board
	MessageCount int
	NewCount     int
	MyPosts      int
	Mentions     int
	LastAt       string
	LastSubject  string
}

type boardQueueSnapshot struct {
	UnreadRows    []boardMenuRow
	MyRows        []boardMenuRow
	MentionRows   []boardMenuRow
	UnreadBoards  int
	MyBoards      int
	MentionBoards int
}

type boardSubscriptionMode string

const (
	boardSubscriptionNone   boardSubscriptionMode = ""
	boardSubscriptionWatch  boardSubscriptionMode = "watch"
	boardSubscriptionDigest boardSubscriptionMode = "digest"
	boardSubscriptionMute   boardSubscriptionMode = "mute"
)

type boardSubscriptionStat struct {
	Label string
	Mode  boardSubscriptionMode
	Count int
}

type onboardingTask struct {
	Key    string
	Title  string
	Detail string
	Href   string
	Done   bool
}

type firstCallSnapshot struct {
	TargetBoardID   int64
	TargetBoardName string
	MailTarget      string
	HomeRoute       string
	PostDone        bool
	ChatDone        bool
	MailDone        bool
	HomeDone        bool
	Tasks           []onboardingTask
}

type launchCheckpoint struct {
	Key    string
	Title  string
	Detail string
	Done   bool
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
	sysSettingCommunityEvents        = "community.calendar.events"
	sysSettingBoardSubscriptionsRoot = "web.board_subscriptions."
	sysSettingBoardWatchRoot         = "web.board_watch."
	sysSettingAttentionDismissedRoot = "web.attention.dismissed."
	sysSettingAttentionReadRoot      = "web.attention.read."
	sysSettingLaunchChecklistRoot    = "web.launch_checklist."
	sysSettingHomeRouteRoot          = "web.home_route."
	maxAdminErrorEntries             = 300
	maxActivityPubInboxBytes         = 1 << 20
	maxJSONRequestBytes              = 1 << 20
	defaultInboundToken              = "dev-inbound-token"
	defaultWebLoginRateLimit         = 10
	defaultWebLoginRateWindow        = 15 * time.Minute
	defaultResetRateLimit            = 5
	defaultResetRateWindow           = 30 * time.Minute
	maxAttentionDismissedItems       = 200
	maxAttentionReadItems            = 300
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
	proxyResolver        *netutil.ProxyResolver
	rateLimits           map[string][]time.Time
	loginRateLimit       int
	loginRateWindow      time.Duration
	resetRateLimit       int
	resetRateWindow      time.Duration
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
	attentionDismissed   map[string]map[string]time.Time
	attentionLoaded      map[string]bool
	attentionRead        map[string]map[string]time.Time
	attentionReadLoaded  map[string]bool
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
	if seeded, seedErr := seedDefaultBoards(storage.Boards); seedErr != nil {
		log.Printf("default board seed failed: %v", seedErr)
	} else if seeded > 0 {
		log.Printf("seeded %d default board(s)", seeded)
	}
	proxyResolver, err := netutil.NewProxyResolver(parseCSVStrings(runtimeCfg.Login.TrustedProxies))
	if err != nil {
		log.Printf("proxy resolver config invalid: %v", err)
		proxyResolver, _ = netutil.NewProxyResolver(nil)
	}

	app := &webApp{
		authSvc:         authSvc,
		userRepo:        storage.Users,
		boardRepo:       storage.Boards,
		msgRepo:         storage.Messages,
		mailRepo:        storage.Mail,
		adminRepo:       storage.Admin,
		doorRepo:        storage.Doors,
		email:           gateway.NewEmailGateway(gateway.LoadEmailConfigFromEnv()),
		sessions:        map[string]sessionState{},
		proxyResolver:   proxyResolver,
		rateLimits:      map[string][]time.Time{},
		loginRateLimit:  defaultWebLoginRateLimit,
		loginRateWindow: defaultWebLoginRateWindow,
		resetRateLimit:  defaultResetRateLimit,
		resetRateWindow: defaultResetRateWindow,
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
		apEnabled:           runtimeCfg.ActivityPub.Enabled,
		apBaseURL:           runtimeCfg.ActivityPub.BaseURL,
		publicBaseURL:       strings.TrimSpace(os.Getenv("WOLFBBS_PUBLIC_BASE_URL")),
		eventBus:            bus,
		startedAt:           time.Now().UTC(),
		runtimeCfg:          runtimeCfg,
		modernOnRamp:        envEnabledDefault("WOLFBBS_WEB_ONRAMP_ENABLE", false),
		guestTour:           envEnabledDefault("WOLFBBS_GUEST_TOUR_ENABLE", false),
		discover:            envEnabledDefault("WOLFBBS_DISCOVER_ENABLE", false),
		quickJump:           envEnabledDefault("WOLFBBS_QUICK_JUMP_ENABLE", false),
		classicSearch:       envEnabledDefault("WOLFBBS_CLASSIC_SEARCH_ENABLE", false),
		wsTerminalURL:       strings.TrimSpace(envFirst("WOLFBBS_WS_TERMINAL_URL", "WOLFBBS_WS_URL", "WOLFBBS_WSS_URL")),
		menuRoot:            strings.TrimSpace(os.Getenv("WOLFBBS_MENU_ROOT")),
		savedSearches:       map[string][]string{},
		attentionDismissed:  map[string]map[string]time.Time{},
		attentionLoaded:     map[string]bool{},
		attentionRead:       map[string]map[string]time.Time{},
		attentionReadLoaded: map[string]bool{},
		lockedChat:          map[string]bool{},
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
	http.HandleFunc("/start", app.handleStartCenter)
	http.Handle("/first-call", app.authRequired(http.HandlerFunc(app.handleFirstCallSession)))
	http.Handle("/today", app.authRequired(http.HandlerFunc(app.handleToday)))
	http.HandleFunc("/events", app.handleEventsCalendar)
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
	http.Handle("/attention", app.authRequired(http.HandlerFunc(app.handleAttentionCenter)))
	http.Handle("/discover", app.authRequired(http.HandlerFunc(app.handleDiscover)))
	http.Handle("/bulletins", app.authRequired(http.HandlerFunc(app.handleBulletins)))
	http.Handle("/directory", app.authRequired(http.HandlerFunc(app.handleDirectory)))
	http.Handle("/feedback", app.authRequired(http.HandlerFunc(app.handleFeedback)))
	http.Handle("/finder", app.authRequired(http.HandlerFunc(app.handleFinder)))
	http.Handle("/newfiles", app.authRequired(http.HandlerFunc(app.handleNewFiles)))
	http.Handle("/radar", app.authRequired(http.HandlerFunc(app.handleRadar)))
	http.Handle("/clubhouse", app.authRequired(http.HandlerFunc(app.handleClubhouse)))
	http.Handle("/doors", app.authRequired(http.HandlerFunc(app.handleDoors)))
	http.Handle("/admin", app.mustBeRole(roleAdmin, app.handleAdmin))
	http.Handle("/admin/users", app.mustBeRole(roleAdmin, app.handleAdminUsers))
	http.Handle("/admin/boards", app.mustBeRole(roleAdmin, app.handleAdminBoards))
	http.Handle("/admin/mail", app.mustBeRole(roleAdmin, app.handleAdminMail))
	http.Handle("/admin/files", app.mustBeRole(roleAdmin, app.handleAdminFiles))
	http.Handle("/admin/gateways", app.mustBeRole(roleAdmin, app.handleAdminGateways))
	http.Handle("/admin/chat", app.mustBeRole(roleAdmin, app.handleAdminChat))
	http.Handle("/admin/doors", app.mustBeRole(roleAdmin, app.handleAdminDoors))
	http.Handle("/admin/events", app.mustBeRole(roleAdmin, app.handleAdminEvents))
	http.Handle("/admin/launch", app.mustBeRole(roleAdmin, app.handleAdminLaunch))
	http.Handle("/admin/ops", app.mustBeRole(roleAdmin, app.handleAdminOps))
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
	server := &http.Server{
		Addr:              *listen,
		Handler:           app.withModernUI(http.DefaultServeMux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	log.Fatal(server.ListenAndServe())
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
	if user, ok := a.currentUser(r); ok {
		http.Redirect(w, r, a.preferredHomeRoute(user), http.StatusFound)
		return
	}
	if a.modernOnRamp {
		http.Redirect(w, r, "/connect", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func homeRouteSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingHomeRouteRoot + handle
}

func normalizeHomeRoute(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "/today":
		return "/today"
	case "/boards":
		return "/boards"
	case "/chat":
		return "/chat"
	case "/doors":
		return "/doors"
	default:
		return ""
	}
}

func homeRouteOptionRows(current string) string {
	options := []struct {
		Value string
		Label string
	}{
		{Value: "/today", Label: "/today"},
		{Value: "/boards", Label: "/boards"},
		{Value: "/chat", Label: "/chat"},
		{Value: "/doors", Label: "/doors"},
	}
	var out strings.Builder
	for _, option := range options {
		selected := ""
		if option.Value == current {
			selected = ` selected`
		}
		out.WriteString(`<option value="` + option.Value + `"` + selected + `>` + option.Label + `</option>`)
	}
	return out.String()
}

func (a *webApp) loadHomeRoute(handle string) string {
	if a.adminRepo == nil {
		return ""
	}
	key := homeRouteSettingKey(handle)
	if key == "" {
		return ""
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil {
		return ""
	}
	return normalizeHomeRoute(raw)
}

func (a *webApp) persistHomeRoute(handle, route string) {
	key := homeRouteSettingKey(handle)
	if key == "" {
		return
	}
	a.persistSystemSetting(key, normalizeHomeRoute(route))
}

func (a *webApp) preferredHomeRoute(user *domain.User) string {
	if user == nil {
		return "/boards"
	}
	if route := a.loadHomeRoute(user.Handle); route != "" {
		return route
	}
	return "/boards"
}

func launchChecklistSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingLaunchChecklistRoot + handle
}

func (a *webApp) launchChecklist(handle string) map[string]bool {
	out := map[string]bool{}
	if a.adminRepo == nil {
		return out
	}
	key := launchChecklistSettingKey(handle)
	if key == "" {
		return out
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		a.addAppError("launch.checklist", fmt.Errorf("decode launch checklist for %s: %w", handle, err))
		return map[string]bool{}
	}
	return out
}

func (a *webApp) persistLaunchChecklist(handle string, rows map[string]bool) {
	key := launchChecklistSettingKey(handle)
	if key == "" {
		return
	}
	filtered := map[string]bool{}
	for key, done := range rows {
		key = strings.TrimSpace(key)
		if key == "" || !done {
			continue
		}
		filtered[key] = true
	}
	body := ""
	if len(filtered) > 0 {
		raw, err := json.Marshal(filtered)
		if err != nil {
			a.addAppError("launch.checklist", fmt.Errorf("encode launch checklist for %s: %w", handle, err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(key, body)
}

func (a *webApp) setLaunchCheckpoint(handle, checkpoint string, done bool) {
	handle = normalizeHandleKey(handle)
	checkpoint = strings.TrimSpace(checkpoint)
	if handle == "" || checkpoint == "" {
		return
	}
	rows := a.launchChecklist(handle)
	if done {
		rows[checkpoint] = true
	} else {
		delete(rows, checkpoint)
	}
	a.persistLaunchChecklist(handle, rows)
}

func (a *webApp) firstWritableBoardFor(user *domain.User) *domain.Board {
	for _, board := range a.visibleBoardsFor(user) {
		board := board
		if a.canWriteBoard(user, &board) {
			return &board
		}
	}
	return nil
}

func (a *webApp) defaultFirstCallMailTarget(user *domain.User) string {
	if sysop := a.primarySysopUser(); sysop != nil && user != nil && !strings.EqualFold(sysop.Handle, user.Handle) {
		return sysop.Handle
	}
	users, err := a.authSvc.ListUsers()
	if err != nil {
		return "sysop"
	}
	for _, row := range users {
		if user != nil && strings.EqualFold(row.Handle, user.Handle) {
			continue
		}
		if rbac.NormalizeRole(row.Role) == roleAdmin {
			return row.Handle
		}
	}
	for _, row := range users {
		if user != nil && strings.EqualFold(row.Handle, user.Handle) {
			continue
		}
		return row.Handle
	}
	return "sysop"
}

func (a *webApp) hasUserBoardPost(user *domain.User) bool {
	if user == nil || a.msgRepo == nil {
		return false
	}
	for _, board := range a.visibleBoardsFor(user) {
		msgs, err := a.msgRepo.ListByBoard(board.ID)
		if err != nil {
			continue
		}
		for _, msg := range msgs {
			if msg.AuthorID == user.ID {
				return true
			}
		}
	}
	return false
}

func (a *webApp) hasUserChatPost(user *domain.User) bool {
	if user == nil || a.chatSvc == nil {
		return false
	}
	channels := a.chatSvc.ListChannels()
	if len(channels) == 0 {
		channels = []string{"#lobby"}
	}
	for _, channel := range channels {
		for _, msg := range a.chatSvc.History(channel, 200) {
			if strings.EqualFold(strings.TrimSpace(msg.From), user.Handle) {
				return true
			}
		}
	}
	return false
}

func (a *webApp) hasUserSentMail(user *domain.User) bool {
	if user == nil || a.mailRepo == nil {
		return false
	}
	rows, err := a.mailRepo.ListOutbox(user.ID, 50)
	if err != nil {
		return false
	}
	return len(rows) > 0
}

func (a *webApp) buildFirstCallSnapshot(user *domain.User) firstCallSnapshot {
	snapshot := firstCallSnapshot{
		MailTarget: a.defaultFirstCallMailTarget(user),
		HomeRoute:  a.preferredHomeRoute(user),
	}
	if board := a.firstWritableBoardFor(user); board != nil {
		snapshot.TargetBoardID = board.ID
		snapshot.TargetBoardName = board.Name
	}
	snapshot.PostDone = a.hasUserBoardPost(user)
	snapshot.ChatDone = a.hasUserChatPost(user)
	snapshot.MailDone = a.hasUserSentMail(user)
	snapshot.HomeDone = a.loadHomeRoute(user.Handle) != ""
	snapshot.Tasks = []onboardingTask{
		{
			Key:    "first_post",
			Title:  "Create your first board post",
			Detail: "Make one visible post so boards stop feeling theoretical.",
			Href:   "/first-call",
			Done:   snapshot.PostDone,
		},
		{
			Key:    "first_chat",
			Title:  "Send one lobby message",
			Detail: "Break the empty-room feeling and prove chat is live.",
			Href:   "/first-call",
			Done:   snapshot.ChatDone,
		},
		{
			Key:    "first_mail",
			Title:  "Send one private mail",
			Detail: "Confirm the board supports private follow-up, not just public posting.",
			Href:   "/first-call",
			Done:   snapshot.MailDone,
		},
		{
			Key:    "home_route",
			Title:  "Choose your home route",
			Detail: "Pick the page that should open first on future sign-ins.",
			Href:   "/settings",
			Done:   snapshot.HomeDone,
		},
	}
	return snapshot
}

func renderOnboardingChecklist(title string, tasks []onboardingTask) string {
	var items strings.Builder
	done := 0
	for _, task := range tasks {
		state := "open"
		action := ""
		if task.Done {
			state = "done"
			done++
		} else if strings.TrimSpace(task.Href) != "" {
			action = ` <a href="` + htmlEscape(task.Href) + `">Open</a>`
		}
		items.WriteString(`<li><strong>` + htmlEscape(task.Title) + `</strong> <span class="wolfbbs-muted">[` + state + `]</span><br>` + htmlEscape(task.Detail) + action + `</li>`)
	}
	return `<article class="wolfbbs-card"><h2>` + htmlEscape(title) + `</h2><p><strong>` + strconv.Itoa(done) + `/` + strconv.Itoa(len(tasks)) + `</strong> complete.</p><ul>` + items.String() + `</ul></article>`
}

func guestQuickStartChecklistHTML() string {
	return `<article class="wolfbbs-card"><h2>Guest Quick-Start Checklist</h2><p>This checklist persists in this browser so guests can leave and come back without losing their place.</p><ul class="wolfbbs-list-clean" id="guestQuickStartList"><li><label><input type="checkbox" data-guest-check="connect"> Pick a client from /connect.</label></li><li><label><input type="checkbox" data-guest-check="tour"> Open the guided tour or help hub.</label></li><li><label><input type="checkbox" data-guest-check="account"> Create or use an account and sign in.</label></li></ul><script>(function(){const key='wolfbbs:guest-quickstart';let state={};try{state=JSON.parse(localStorage.getItem(key)||'{}')||{};}catch(_err){state={};}document.querySelectorAll('[data-guest-check]').forEach(function(node){const id=node.getAttribute('data-guest-check');node.checked=Boolean(state[id]);node.addEventListener('change',function(){state[id]=node.checked;try{localStorage.setItem(key,JSON.stringify(state));}catch(_err){}});});})();</script></article>`
}

func (a *webApp) renderRoleAwareEmptyState(user *domain.User, surface string) string {
	role := roleUser
	if user != nil {
		role = rbac.NormalizeRole(user.Role)
	}
	title := "Nothing here yet"
	body := "This surface needs a first real interaction so it feels like a board instead of an empty shell."
	links := []string{`<a href="/start">Start Center</a>`, `<a href="/help">Help</a>`}
	switch surface {
	case "boards":
		title = "Boards need a first conversation"
		if role == roleAdmin {
			body = "Caller-visible boards are empty or filtered away. Seed starter boards, create one real caller, and make the first public post."
			links = []string{`<a href="/admin/setup?step=4">Bootstrap Step</a>`, `<a href="/admin/boards">Board Admin</a>`, `<a href="/first-call">First Caller Session</a>`}
		} else {
			body = "If the board list feels empty, use First Caller Session to make the first post and give the message area some shape."
			links = []string{`<a href="/first-call">First Caller Session</a>`, `<a href="/today">Today Brief</a>`, `<a href="/attention">Attention Center</a>`}
		}
	case "board_detail":
		title = "This board has no threads yet"
		if role == roleAdmin {
			body = "A starter topic here will make the board feel intentional immediately."
			links = []string{`<a href="/admin/boards">Board Admin</a>`, `<a href="/first-call">First Caller Session</a>`}
		} else {
			body = "Start the thread yourself and give the next caller something to answer."
			links = []string{`<a href="/first-call">First Caller Session</a>`, `<a href="/today">Today Brief</a>`}
		}
	case "chat":
		title = "Chat needs a first line"
		if role == roleAdmin || role == roleModerator {
			body = "A silent lobby reads like a broken feature. Post the opening line, then schedule or announce a concrete reason to be here."
			links = []string{`<a href="/first-call">First Caller Session</a>`, `<a href="/admin/events">Events Admin</a>`, `<a href="/admin/chat">Chat Admin</a>`}
		} else {
			body = "Be the first voice in the room. One short hello is enough to prove the lobby is alive."
			links = []string{`<a href="/first-call">First Caller Session</a>`, `<a href="/events">Community Calendar</a>`}
		}
	case "files":
		title = "FileBase needs a seed upload"
		if role == roleAdmin {
			body = "Recent uploads are empty. Index a starter area or upload one canonical pack so callers see a real file desk."
			links = []string{`<a href="/admin/files">Files Admin</a>`, `<a href="/gateway?view=files">FileBase Browser</a>`}
		} else {
			body = "The file desk is available, but it has not been seeded yet. Check back after the sysop indexes starter uploads."
			links = []string{`<a href="/gateway?view=files">FileBase Browser</a>`, `<a href="/bulletins">Bulletins</a>`}
		}
	case "doors":
		title = "Doors are loaded but not lived in yet"
		if role == roleAdmin {
			body = "The door catalog exists, but nobody has given it heat yet. Launch a first run, verify scores, and schedule a return event."
			links = []string{`<a href="/admin/doors">Doors Admin</a>`, `<a href="/admin/events">Events Admin</a>`, `<a href="/scores">Scores</a>`}
		} else {
			body = "Play the first door session and the cockpit will start to fill with recent activity, favorites, and trophies."
			links = []string{`<a href="/scores">Scores</a>`, `<a href="/events">Community Calendar</a>`}
		}
	}
	return `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>` + htmlEscape(title) + `</h2><p>` + htmlEscape(body) + `</p><p>` + strings.Join(links, ` | `) + `</p></article></section>`
}

func (a *webApp) handleStartCenter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, _ := a.currentUser(r)
	pageTitle := a.siteDisplayName() + " Start Center"
	nav := `<a href="/connect">connect</a> | <a href="/tour">tour</a> | <a href="/events">events</a> | <a href="/login">login</a> | <a href="/help">help</a>`
	intro := `<p>Start here when you want a clear next step instead of hunting through routes.</p>`
	kpis := `<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>Guest</strong><span>tour, connect, evaluate</span></article><article class="wolfbbs-kpi-card"><strong>Caller</strong><span>boards, attention, doors</span></article><article class="wolfbbs-kpi-card"><strong>Sysop</strong><span>setup, ops, launch</span></article></section>`
	laneGrid := `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Just exploring</h2><div class="wolfbbs-action-grid"><a class="wolfbbs-action-card" href="/connect"><strong>Connect</strong><span>SSH, web terminal, IRC, and clipboard-ready commands</span></a><a class="wolfbbs-action-card" href="/tour"><strong>Guided Tour</strong><span>Read-only walkthrough of the product shape</span></a><a class="wolfbbs-action-card" href="/help"><strong>Help</strong><span>Route map and surface guide</span></a></div></article><article class="wolfbbs-card"><h2>What success looks like</h2><ul class="wolfbbs-list-clean"><li>Guests should understand what the board does in under five minutes.</li><li>Callers should know where to go next after the first login.</li><li>Sysops should know whether the board is truly launch-ready.</li></ul></article></section>`
	if user == nil {
		page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>` + htmlEscape(pageTitle) + `</title></head><body><h1>` + htmlEscape(pageTitle) + `</h1><p>` + nav + `</p>` + intro + kpis + laneGrid + guestQuickStartChecklistHTML() + `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Caller path</h2><ol><li>Open <a href="/connect">/connect</a> or <a href="/tour">/tour</a>.</li><li>Create or use an account and sign in.</li><li>Start with boards, chat, doors, and mail.</li></ol></article><article class="wolfbbs-card"><h2>Sysop path</h2><ol><li>Sign in as sysop.</li><li>Finish <a href="/admin/setup">/admin/setup</a>.</li><li>Use <a href="/admin/launch">/admin/launch</a> and <a href="/status">/status</a> before inviting callers.</li></ol></article></section></body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page))
		return
	}

	role := rbac.NormalizeRole(user.Role)
	nav = `<a href="/boards">boards</a> | <a href="/attention">attention</a> | <a href="/mail">mail</a> | <a href="/doors">doors</a> | <a href="/radar">radar</a> | <a href="/help">help</a> | <a href="/logout">logout</a>`
	if a.hasRole(user, roleAdmin) {
		nav = `<a href="/admin">admin</a> | <a href="/admin/setup">setup</a> | <a href="/admin/ops">ops</a> | <a href="/admin/events">events</a> | <a href="/admin/launch">launch</a> | <a href="/status">status</a> | <a href="/boards">boards</a> | <a href="/help">help</a> | <a href="/logout">logout</a>`
	}
	hero := `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Your next best move</h2><div class="wolfbbs-action-grid"><a class="wolfbbs-action-card" href="/today"><strong>Today Brief</strong><span>daily loop: queue, watched boards, events</span></a><a class="wolfbbs-action-card" href="/attention"><strong>Attention Center</strong><span>everything that needs follow-up in one screen</span></a><a class="wolfbbs-action-card" href="/boards"><strong>Boards</strong><span>long-form discussion and unread scan</span></a><a class="wolfbbs-action-card" href="/mail"><strong>Mail</strong><span>private follow-up and direct replies</span></a><a class="wolfbbs-action-card" href="/doors"><strong>Doors</strong><span>retention loop, scores, and favorite games</span></a><a class="wolfbbs-action-card" href="/events"><strong>Events</strong><span>calendar and return hooks</span></a></div></article><article class="wolfbbs-card"><h2>Role</h2><p><strong>` + htmlEscape(role) + `</strong></p><p>` + htmlEscape(user.Handle) + ` should be able to answer "what do I do next?" from this page alone.</p></article></section>`
	if a.hasRole(user, roleAdmin) {
		readiness := a.buildSetupReadinessSnapshot(user)
		hero = `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Operator Lane</h2><div class="wolfbbs-action-grid"><a class="wolfbbs-action-card" href="/admin/setup"><strong>Setup Wizard</strong><span>identity, safety, bootstrap</span></a><a class="wolfbbs-action-card" href="/admin/ops"><strong>Ops Center</strong><span>alerts, audits, active sessions, next actions</span></a><a class="wolfbbs-action-card" href="/admin/events"><strong>Events Admin</strong><span>schedule reasons for callers to return</span></a><a class="wolfbbs-action-card" href="/admin/launch"><strong>Launch Center</strong><span>go-live verdict and launch checklist</span></a><a class="wolfbbs-action-card" href="/status"><strong>Status Center</strong><span>caller-facing health snapshot</span></a></div></article><article class="wolfbbs-card"><h2>Launch Verdict</h2><p><strong>` + htmlEscape(launchVerdictText(readiness)) + `</strong></p><p>` + strconv.Itoa(readiness.Summary.Pass) + `/` + strconv.Itoa(readiness.Summary.Total) + ` checks passing.</p></article></section>`
	}
	firstCallBlock := ``
	snapshot := a.buildFirstCallSnapshot(user)
	doneCount := 0
	for _, task := range snapshot.Tasks {
		if task.Done {
			doneCount++
		}
	}
	if doneCount < len(snapshot.Tasks) {
		firstCallBlock = renderOnboardingChecklist("First Caller Session", snapshot.Tasks) + `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Why this matters</h2><p>New caller friction is highest on the first login. Finish these four tasks once and the rest of the board starts to feel real instead of merely configured.</p><p><a href="/first-call">Open guided first caller session</a> | <a href="/settings">Choose home route</a></p></article></section>`
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>` + htmlEscape(pageTitle) + `</title></head><body><h1>` + htmlEscape(pageTitle) + `</h1><p>` + nav + `</p>` + intro + kpis + hero + firstCallBlock + `<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Use Today first</strong><p><a href="/today">/today</a> is the shortest daily caller loop once you are signed in.</p></article><article class="wolfbbs-helper-card"><strong>Use Discover for narrative catch-up</strong><p>When you want a broader digest instead of direct action items, open <a href="/discover">/discover</a>.</p></article><article class="wolfbbs-helper-card"><strong>Use SSH when you want the full board feel</strong><p><a href="/connect">/connect</a> remains the best starting point for the terminal-first experience.</p></article></section></body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleFirstCallSession(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if r.Method == http.MethodPost {
		if !a.requireCSRF(w, r) {
			return
		}
		snapshot := a.buildFirstCallSnapshot(user)
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		switch action {
		case "starter_post":
			if a.msgRepo == nil || a.boardRepo == nil {
				redirectWithError(w, r, "/first-call", "Board service is unavailable.")
				return
			}
			boardID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("board_id")), 10, 64)
			if boardID <= 0 {
				boardID = snapshot.TargetBoardID
			}
			if boardID <= 0 {
				redirectWithError(w, r, "/first-call", "No writable board is available yet.")
				return
			}
			board, err := a.boardRepo.Get(boardID)
			if err != nil || board == nil {
				redirectWithError(w, r, "/first-call", "Starter board not found.")
				return
			}
			if !a.canWriteBoard(user, board) {
				http.Error(w, "post denied by board ACS", http.StatusForbidden)
				return
			}
			subject := strings.TrimSpace(r.FormValue("subject"))
			body := strings.TrimSpace(r.FormValue("body"))
			if subject == "" {
				subject = "First call check-in"
			}
			if body == "" {
				body = "Running the guided first caller session. Boards are live."
			}
			if err := a.msgRepo.CreateMessage(&domain.Message{
				BoardID:   boardID,
				AuthorID:  user.ID,
				Subject:   subject,
				Body:      body,
				CreatedAt: time.Now().UTC(),
			}); err != nil {
				redirectWithError(w, r, "/first-call", "Could not create starter post.")
				return
			}
			redirectWithNotice(w, r, "/first-call", "Starter board post created.")
			return
		case "starter_chat":
			if a.chatSvc == nil {
				redirectWithError(w, r, "/first-call", "Chat service is unavailable.")
				return
			}
			channel := chat.NormalizeChannel(strings.TrimSpace(r.FormValue("channel")))
			if channel == "" {
				channel = "#lobby"
			}
			body := strings.TrimSpace(r.FormValue("body"))
			if body == "" {
				body = "Checking in from First Caller Session."
			}
			if _, err := a.chatSvc.Post(user.Handle, channel, body); err != nil {
				redirectWithError(w, r, "/first-call", "Could not send lobby message.")
				return
			}
			redirectWithNotice(w, r, "/first-call", "Starter lobby message sent.")
			return
		case "starter_mail":
			if a.mailRepo == nil || a.authSvc == nil {
				redirectWithError(w, r, "/first-call", "Mail service is unavailable.")
				return
			}
			targetHandle := strings.TrimSpace(r.FormValue("to"))
			if targetHandle == "" {
				targetHandle = snapshot.MailTarget
			}
			if targetHandle == "" {
				redirectWithError(w, r, "/first-call", "No mail target is available yet.")
				return
			}
			target, err := a.authSvc.GetUser(targetHandle)
			if err != nil || target == nil {
				redirectWithError(w, r, "/first-call", "Starter mail target not found.")
				return
			}
			subject := strings.TrimSpace(r.FormValue("subject"))
			body := strings.TrimSpace(r.FormValue("body"))
			if subject == "" {
				subject = "First-call hello"
			}
			if body == "" {
				body = "This is my first private mail from the guided caller session."
			}
			if err := a.mailRepo.CreateMail(&domain.PrivateMail{
				FromUserID: user.ID,
				ToUserID:   target.ID,
				Subject:    subject,
				Body:       body,
				CreatedAt:  time.Now().UTC(),
			}); err != nil {
				redirectWithError(w, r, "/first-call", "Could not send starter mail.")
				return
			}
			redirectWithNotice(w, r, "/first-call", "Starter private mail sent.")
			return
		case "save_home_route":
			route := normalizeHomeRoute(r.FormValue("home_route"))
			if route == "" {
				redirectWithError(w, r, "/first-call", "Choose a home route first.")
				return
			}
			a.persistHomeRoute(user.Handle, route)
			redirectWithNotice(w, r, "/first-call", "Home route saved.")
			return
		default:
			redirectWithError(w, r, "/first-call", "Unsupported first caller action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	snapshot := a.buildFirstCallSnapshot(user)
	doneCount := 0
	for _, task := range snapshot.Tasks {
		if task.Done {
			doneCount++
		}
	}
	progressBlock := renderOnboardingChecklist("First Caller Session Progress", snapshot.Tasks)
	successBlock := ``
	if doneCount == len(snapshot.Tasks) {
		successBlock = `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Caller baseline complete</h2><p>You have crossed the first-call threshold: boards, chat, private mail, and home-route preference are all proven.</p><p><a href="` + htmlEscape(a.preferredHomeRoute(user)) + `">Open your home route</a> | <a href="/today">Today Brief</a> | <a href="/attention">Attention Center</a></p></article></section>`
	}
	boardTarget := `No writable board available yet.`
	if snapshot.TargetBoardID > 0 {
		boardTarget = htmlEscape(snapshot.TargetBoardName) + ` (#` + strconv.FormatInt(snapshot.TargetBoardID, 10) + `)`
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>First Caller Session</title></head><body>
<p><a href="/start">start</a> | <a href="/today">today</a> | <a href="/attention">attention</a> | <a href="/boards">boards</a> | <a href="/chat">chat</a> | <a href="/mail">mail</a> | <a href="/settings">settings</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>First Caller Session</h1>
<p>Do the four things that prove the board is usable for a normal caller: make one post, send one chat line, send one private mail, and choose the page you want to land on after sign-in.</p>
` + progressBlock + successBlock + `
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Starter Board Post</h2><p><strong>Target board:</strong> ` + boardTarget + `</p><form method="POST" action="/first-call" data-draft-key="first-call-post" data-rich-compose="first-call-post" data-compose-signature="` + htmlEscape(user.Handle) + `"><input type="hidden" name="action" value="starter_post"><input type="hidden" name="board_id" value="` + strconv.FormatInt(snapshot.TargetBoardID, 10) + `">` + a.csrfHiddenInput(r) + `<label>Subject <input name="subject" value="First call check-in" size="60"></label><br><label>Body<br><textarea name="body" rows="8" cols="80">Running the guided first caller session. Boards are live.</textarea></label><br><button type="submit">Create starter post</button></form></article>
<article class="wolfbbs-card"><h2>Lobby Hello</h2><p><strong>Channel:</strong> #lobby</p><form method="POST" action="/first-call"><input type="hidden" name="action" value="starter_chat">` + a.csrfHiddenInput(r) + `<input type="hidden" name="channel" value="#lobby"><label>Message <input name="body" size="64" value="Checking in from First Caller Session."></label><button type="submit">Send lobby message</button></form><p class="wolfbbs-muted">One line is enough to prove the room is live.</p></article>
</section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Private Mail Check</h2><p><strong>Target:</strong> ` + htmlEscape(snapshot.MailTarget) + `</p><form method="POST" action="/first-call" data-draft-key="first-call-mail" data-rich-compose="first-call-mail" data-compose-signature="` + htmlEscape(user.Handle) + `"><input type="hidden" name="action" value="starter_mail">` + a.csrfHiddenInput(r) + `<label>To <input name="to" value="` + htmlEscape(snapshot.MailTarget) + `" size="32"></label><br><label>Subject <input name="subject" value="First-call hello" size="60"></label><br><label>Body<br><textarea name="body" rows="8" cols="80">This is my first private mail from the guided caller session.</textarea></label><br><button type="submit">Send private mail</button></form></article>
<article class="wolfbbs-card"><h2>Choose Home Route</h2><p>Pick the page that should open first after future sign-ins.</p><form method="POST" action="/first-call"><input type="hidden" name="action" value="save_home_route">` + a.csrfHiddenInput(r) + `<label>Home route <select name="home_route">` + homeRouteOptionRows(snapshot.HomeRoute) + `</select></label><button type="submit">Save home route</button></form><p class="wolfbbs-muted">You can still change this later in <a href="/settings">Settings</a>.</p></article>
</section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAttentionCenter(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	buildAttentionContext := func() (discovery.Result, []boardPulseRow, []communityEvent, string, map[int64]bool, map[int64]bool, error) {
		digest, err := discovery.BuildSinceLastCall(a.boardRepo, a.msgRepo, a.mailRepo, user, 18)
		if err != nil {
			return discovery.Result{}, nil, nil, "", nil, nil, err
		}
		visibleBoards := a.visibleBoardsFor(user)
		watchSubs := a.boardSubscriptionIDs(user.Handle, boardSubscriptionWatch)
		digestSubs := a.boardSubscriptionIDs(user.Handle, boardSubscriptionDigest)
		trackedBoards := filterBoardsByWatch(visibleBoards, watchSubs)
		trackedLabel := "watch tier"
		if len(trackedBoards) == 0 {
			trackedBoards = visibleBoards
			trackedLabel = "board pulse fallback"
		}
		return digest, a.buildBoardPulse(user, trackedBoards, 6), a.upcomingCommunityEvents(4, time.Now().UTC()), trackedLabel, watchSubs, digestSubs, nil
	}
	if r.Method == http.MethodPost {
		if !a.requireCSRF(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		switch action {
		case "dismiss":
			itemKey := strings.TrimSpace(r.FormValue("item_key"))
			if itemKey == "" {
				redirectWithError(w, r, "/attention", "Attention item key is required.")
				return
			}
			a.dismissAttentionItem(user.Handle, itemKey)
			redirectWithNotice(w, r, "/attention", "Attention item dismissed.")
			return
		case "mark_read":
			itemKey := strings.TrimSpace(r.FormValue("item_key"))
			if itemKey == "" {
				redirectWithError(w, r, "/attention", "Attention item key is required.")
				return
			}
			a.markAttentionItemRead(user.Handle, itemKey)
			redirectWithNotice(w, r, "/attention", "Attention item marked read.")
			return
		case "mark_unread":
			itemKey := strings.TrimSpace(r.FormValue("item_key"))
			if itemKey == "" {
				redirectWithError(w, r, "/attention", "Attention item key is required.")
				return
			}
			a.markAttentionItemUnread(user.Handle, itemKey)
			redirectWithNotice(w, r, "/attention", "Attention item marked unread.")
			return
		case "mark_all_read":
			digest, boardPulse, _, _, _, _, err := buildAttentionContext()
			if err != nil {
				redirectWithError(w, r, "/attention", "Attention feed unavailable.")
				return
			}
			keys := make([]string, 0, len(digest.Items)+len(boardPulse))
			for _, item := range digest.Items {
				key := attentionItemKey(item)
				if key != "" && !a.isAttentionKeyDismissed(user.Handle, key) {
					keys = append(keys, key)
				}
			}
			for _, row := range boardPulse {
				key := boardAttentionItemKey(row)
				if key != "" && !a.isAttentionKeyDismissed(user.Handle, key) {
					keys = append(keys, key)
				}
			}
			updated := a.markAttentionItemsRead(user.Handle, keys)
			redirectWithNotice(w, r, "/attention", fmt.Sprintf("Marked %d attention item(s) read.", updated))
			return
		case "clear_dismissed":
			a.clearAttentionDismissals(user.Handle)
			redirectWithNotice(w, r, "/attention", "Dismissed attention items restored.")
			return
		case "mark_mail_read":
			mailID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("mail_id")), 10, 64)
			if mailID <= 0 {
				redirectWithError(w, r, "/attention", "Mail ID is required.")
				return
			}
			item, err := a.mailRepo.GetMail(mailID)
			if err != nil || item == nil || item.ToUserID != user.ID {
				redirectWithError(w, r, "/attention", "Mail item not found.")
				return
			}
			_ = a.mailRepo.MarkRead(mailID, time.Now().UTC())
			redirectWithNotice(w, r, "/attention", "Mail marked read.")
			return
		case "mark_all_mail_read":
			updated := 0
			if a.mailRepo != nil {
				if inbox, err := a.mailRepo.ListInbox(user.ID, 200); err == nil {
					now := time.Now().UTC()
					for _, row := range inbox {
						if row.ReadAt != nil {
							continue
						}
						if err := a.mailRepo.MarkRead(row.ID, now); err == nil {
							updated++
						}
					}
				}
			}
			redirectWithNotice(w, r, "/attention", fmt.Sprintf("Marked %d mail item(s) as read.", updated))
			return
		case "dismiss_board":
			itemKey := strings.TrimSpace(r.FormValue("item_key"))
			if itemKey == "" {
				boardID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("board_id")), 10, 64)
				if boardID <= 0 {
					redirectWithError(w, r, "/attention", "Board ID is required.")
					return
				}
				itemKey = boardAttentionKey(boardID)
			}
			a.dismissAttentionItem(user.Handle, itemKey)
			redirectWithNotice(w, r, "/attention", "Board dismissed from attention queue.")
			return
		default:
			redirectWithError(w, r, "/attention", "Unsupported attention action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	digest, boardPulse, upcomingEvents, trackedLabel, watchSubs, digestSubs, err := buildAttentionContext()
	if err != nil {
		http.Error(w, "attention feed unavailable", http.StatusInternalServerError)
		return
	}

	handleByID := a.userHandleLookup()
	csrf := a.csrfHiddenInput(r)

	directUnread := make([]attentionActionRow, 0, 8)
	directSeen := make([]attentionActionRow, 0, 8)
	boardQueueUnread := make([]attentionActionRow, 0, 8)
	boardUnread := make([]attentionActionRow, 0, 8)
	boardSeen := make([]attentionActionRow, 0, 8)
	allUnread := make([]attentionActionRow, 0, len(digest.Items))
	replyMentions := 0
	boardUpdates := 0
	seenCount := 0
	for _, item := range digest.Items {
		itemKey := attentionItemKey(item)
		if a.isAttentionKeyDismissed(user.Handle, itemKey) {
			continue
		}
		href := "/discover"
		if item.BoardID > 0 {
			href = "/boards?board=" + strconv.FormatInt(item.BoardID, 10)
			if item.MessageID > 0 {
				href += "&id=" + strconv.FormatInt(item.MessageID, 10)
			}
		}
		meta := strings.ToUpper(item.Kind)
		if item.Conference != "" {
			meta += " | " + item.Conference
		}
		row := attentionActionRow{
			Label:       item.Line,
			Href:        href,
			Meta:        meta,
			ItemKey:     itemKey,
			Read:        a.isAttentionKeyRead(user.Handle, itemKey),
			Dismissible: item.Kind != "mail",
		}
		if row.Read {
			seenCount++
		} else {
			allUnread = append(allUnread, row)
		}
		switch item.Kind {
		case "mention", "reply":
			if row.Read {
				directSeen = append(directSeen, row)
			} else {
				replyMentions++
				directUnread = append(directUnread, row)
			}
		case "board":
			if row.Read {
				boardSeen = append(boardSeen, row)
			} else {
				boardQueueUnread = append(boardQueueUnread, row)
			}
		}
	}

	inboxRows := make([]domain.PrivateMail, 0, 12)
	if a.mailRepo != nil {
		if inbox, listErr := a.mailRepo.ListInbox(user.ID, 100); listErr == nil {
			for _, row := range inbox {
				if row.ReadAt == nil {
					inboxRows = append(inboxRows, row)
				}
			}
		}
	}
	sort.Slice(inboxRows, func(i, j int) bool {
		if inboxRows[i].CreatedAt.Equal(inboxRows[j].CreatedAt) {
			return inboxRows[i].ID > inboxRows[j].ID
		}
		return inboxRows[i].CreatedAt.After(inboxRows[j].CreatedAt)
	})
	if len(inboxRows) > 8 {
		inboxRows = inboxRows[:8]
	}

	renderAttentionList := func(rows []attentionActionRow, empty string) string {
		list := strings.Builder{}
		for _, row := range rows {
			list.WriteString(`<li><div class="wolfbbs-inline-actions"><a href="` + htmlEscape(row.Href) + `">` + htmlEscape(row.Label) + `</a><span class="wolfbbs-muted">` + htmlEscape(row.Meta) + `</span>`)
			if row.ItemKey != "" {
				action := "mark_read"
				label := "Mark Read"
				if row.Read {
					action = "mark_unread"
					label = "Mark Unread"
				}
				list.WriteString(`<form method="POST" action="/attention"><input type="hidden" name="action" value="` + action + `"><input type="hidden" name="item_key" value="` + htmlEscape(row.ItemKey) + `">` + csrf + `<button type="submit">` + label + `</button></form>`)
			}
			if row.Dismissible && row.ItemKey != "" {
				list.WriteString(`<form method="POST" action="/attention"><input type="hidden" name="action" value="dismiss"><input type="hidden" name="item_key" value="` + htmlEscape(row.ItemKey) + `">` + csrf + `<button type="submit">Dismiss</button></form>`)
			}
			list.WriteString(`</div></li>`)
		}
		if list.Len() == 0 {
			list.WriteString(`<li>` + htmlEscape(empty) + `</li>`)
		}
		return list.String()
	}

	for _, row := range boardPulse {
		itemKey := boardAttentionItemKey(row)
		if a.isAttentionKeyDismissed(user.Handle, itemKey) {
			continue
		}
		actionRow := attentionActionRow{
			Label:       row.BoardName,
			Href:        "/boards?board=" + strconv.FormatInt(row.BoardID, 10),
			Meta:        fmt.Sprintf("%d new | %d total | %s", row.NewCount, row.MessageCount, row.LastAt),
			ItemKey:     itemKey,
			Read:        a.isAttentionKeyRead(user.Handle, itemKey),
			Dismissible: true,
		}
		if actionRow.Read {
			seenCount++
			boardSeen = append(boardSeen, actionRow)
			continue
		}
		boardUpdates++
		allUnread = append(allUnread, actionRow)
		boardUnread = append(boardUnread, actionRow)
	}

	directList := renderAttentionList(directUnread, "No direct replies or mentions are waiting.")
	seenList := renderAttentionList(append(append([]attentionActionRow{}, directSeen...), boardSeen...), "No previously seen attention items are active.")

	inboxList := strings.Builder{}
	for _, row := range inboxRows {
		from := handleByID[row.FromUserID]
		if strings.TrimSpace(from) == "" {
			from = "unknown"
		}
		inboxList.WriteString(`<li><div class="wolfbbs-inline-actions"><a href="/mail?id=` + strconv.FormatInt(row.ID, 10) + `">` + htmlEscape(row.Subject) + `</a><span class="wolfbbs-muted">from ` + htmlEscape(from) + ` at ` + row.CreatedAt.Local().Format("2006-01-02 15:04") + `</span><form method="POST" action="/attention"><input type="hidden" name="action" value="mark_mail_read"><input type="hidden" name="mail_id" value="` + strconv.FormatInt(row.ID, 10) + `">` + csrf + `<button type="submit">Mark Read</button></form></div></li>`)
	}
	if inboxList.Len() == 0 {
		inboxList.WriteString(`<li>No unread mail is waiting.</li>`)
	}

	boardList := renderAttentionList(boardUnread, "No watch-tier board follow-up is waiting.")
	if len(boardUnread) == 0 && len(boardQueueUnread) > 0 {
		boardList = renderAttentionList(boardQueueUnread, "No watch-tier board follow-up is waiting.")
	}

	discoveryList := renderAttentionList(allUnread, "Nothing new since the last call.")
	eventList := strings.Builder{}
	for _, row := range upcomingEvents {
		meta := []string{formatCommunityEventWindow(row)}
		if recurrence := recurrenceSummary(row); recurrence != "" {
			meta = append(meta, recurrence)
		}
		if strings.TrimSpace(row.Location) != "" {
			meta = append(meta, row.Location)
		}
		eventList.WriteString(`<li><strong>` + htmlEscape(row.Title) + `</strong> <span class="wolfbbs-muted">` + htmlEscape(strings.Join(meta, " | ")) + `</span></li>`)
	}
	if eventList.Len() == 0 {
		eventList.WriteString(`<li>No upcoming events scheduled.</li>`)
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Attention Center</title></head><body>
<p><a href="/start">start</a> | <a href="/today">today</a> | <a href="/boards">boards</a> | <a href="/events">events</a> | <a href="/mail">mail</a> | <a href="/discover">discover</a> | <a href="/radar">radar</a> | <a href="/doors">doors</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Attention Center</h1>
<p>Single-screen triage for direct follow-up, unread mail, and boards that moved while you were away. Read state persists, so this screen stays short instead of resetting every refresh.</p>
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(allUnread)) + `</strong><span>unread attention</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(seenCount) + `</strong><span>read this cycle</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(replyMentions) + `</strong><span>mentions + replies</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(inboxRows)) + `</strong><span>unread mail</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(boardUpdates) + `</strong><span>board updates</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(watchSubs)) + ` / ` + strconv.Itoa(len(digestSubs)) + `</strong><span>watch / digest boards</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(upcomingEvents)) + `</strong><span>upcoming events</span></article>
</section>
<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Work top-down</strong><p>Direct replies and mentions should be handled first because they are the clearest pending obligation.</p></article><article class="wolfbbs-helper-card"><strong>Unread mail is private follow-up</strong><p>Use inbox for direct conversation, then return to boards when the topic belongs in public.</p></article><article class="wolfbbs-helper-card"><strong>Use tiers deliberately</strong><p>The board pulse is showing ` + htmlEscape(trackedLabel) + `. Watch boards surface here, digest boards stay in <a href="/today">/today</a>, and mute hides noise.</p></article></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Queue Actions</h2><div class="wolfbbs-inline-actions"><form method="POST" action="/attention"><input type="hidden" name="action" value="mark_all_read">` + csrf + `<button type="submit">Mark Current Attention Read</button></form><form method="POST" action="/attention"><input type="hidden" name="action" value="mark_all_mail_read">` + csrf + `<button type="submit">Mark All Mail Read</button></form><form method="POST" action="/attention"><input type="hidden" name="action" value="clear_dismissed">` + csrf + `<button type="submit">Restore Dismissed Items</button></form><a href="/discover">Open Full Discover Feed</a></div></article></section>
<section class="wolfbbs-grid">
<article><h2>Direct Follow-Up</h2><ul>` + directList + `</ul><p><a href="/boards?mode=mentions">Mentions queue</a> | <a href="/boards?mode=mine">Your threads</a></p></article>
<article><h2>Inbox Needs Action</h2><ul>` + inboxList.String() + `</ul><p><a href="/mail?box=unread">Open unread mail</a> | <a href="/mail">Compose</a></p></article>
</section>
<section class="wolfbbs-grid">
<article><h2>Watch Tier Board Pulse</h2><ul>` + boardList + `</ul><p><a href="/boards?mode=watched">Watch tier</a> | <a href="/boards?mode=digest">Digest tier</a> | <a href="/boards?mode=unread">Unread board scan</a></p></article>
<article><h2>Community Calendar</h2><ul>` + eventList.String() + `</ul><p><a href="/events">Open calendar</a> | <a href="/today">Open today brief</a></p></article>
</section>
<section class="wolfbbs-grid"><article><h2>Unread Notification Feed</h2><ul>` + discoveryList + `</ul><p><a href="/discover">Open full discover feed</a></p></article><article><h2>Seen This Cycle</h2><ul>` + seenList + `</ul><p>Read items stay out of your way until the board moves again.</p></article></section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminOps(w http.ResponseWriter, r *http.Request) {
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
		case "clear_errors":
			cleared := a.clearAppErrors()
			a.recordAdminAction(user.Handle, "runtime", "clear_errors", fmt.Sprintf("cleared=%d", cleared))
			redirectWithNotice(w, r, "/admin/ops", fmt.Sprintf("Cleared %d runtime error(s).", cleared))
			return
		case "prune_idle_sessions":
			minutes := parseIntWithFallback(r.FormValue("idle_minutes"), 30)
			pruned := 0
			if a.adminRepo != nil {
				if sessions, err := a.adminRepo.ListNodeSessions(500); err == nil {
					cutoff := time.Now().UTC().Add(-time.Duration(minutes) * time.Minute)
					for _, row := range sessions {
						if row.LastActivity.After(cutoff) {
							continue
						}
						if err := a.adminRepo.DeleteNodeSession(row.SessionID); err == nil {
							pruned++
						}
					}
				}
			}
			a.recordAdminAction(user.Handle, "node_sessions", "prune_idle_sessions", fmt.Sprintf("minutes=%d pruned=%d", minutes, pruned))
			redirectWithNotice(w, r, "/admin/ops", fmt.Sprintf("Pruned %d idle session(s).", pruned))
			return
		case "purge_web_sessions":
			pruned := a.pruneExpiredWebSessions()
			a.recordAdminAction(user.Handle, "web_sessions", "purge_expired_sessions", fmt.Sprintf("pruned=%d", pruned))
			redirectWithNotice(w, r, "/admin/ops", fmt.Sprintf("Purged %d expired web session(s).", pruned))
			return
		case "clear_rate_limits":
			cleared := a.clearAllRateLimits()
			a.recordAdminAction(user.Handle, "web_rate_limits", "clear_rate_limits", fmt.Sprintf("cleared=%d", cleared))
			redirectWithNotice(w, r, "/admin/ops", fmt.Sprintf("Cleared %d active rate-limit bucket(s).", cleared))
			return
		default:
			redirectWithError(w, r, "/admin/ops", "Unsupported ops action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	readiness := a.buildSetupReadinessSnapshot(user)
	runtime := a.buildStatusSnapshot(user)
	errors := a.latestErrors(10)
	csrf := a.csrfHiddenInput(r)
	auditRows := []domain.AdminAudit{}
	nodeSessions := []domain.NodeSession{}
	callerHistory := []domain.CallerHistory{}
	webSessionCount := a.countWebSessions()
	rateLimitCount := a.countRateLimits()
	if a.adminRepo != nil {
		auditRows, _ = a.adminRepo.ListAudit(12)
		nodeSessions, _ = a.adminRepo.ListNodeSessions(12)
		callerHistory, _ = a.adminRepo.ListCallerHistory(10)
	}

	nextActions := strings.Builder{}
	for _, row := range readiness.Recommendations {
		nextActions.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	for _, row := range runtime.Recommendations {
		nextActions.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	if len(errors) > 0 {
		nextActions.WriteString(`<li>Review runtime errors below before treating current health as trustworthy.</li>`)
	}
	if nextActions.Len() == 0 {
		nextActions.WriteString(`<li>No urgent blockers detected. Walk the caller path once, then review audits and logs for anything surprising.</li>`)
	}

	errorTable := strings.Builder{}
	for _, row := range errors {
		errorTable.WriteString(`<tr><td>` + row.Time.Local().Format("2006-01-02 15:04:05") + `</td><td>` + htmlEscape(row.Area) + `</td><td>` + htmlEscape(row.Message) + `</td></tr>`)
	}
	if errorTable.Len() == 0 {
		errorTable.WriteString(`<tr><td colspan="3">No runtime errors logged recently.</td></tr>`)
	}

	auditTable := strings.Builder{}
	for _, row := range auditRows {
		auditTable.WriteString(`<tr><td>` + row.CreatedAt.Local().Format("2006-01-02 15:04:05") + `</td><td>` + htmlEscape(row.Actor) + `</td><td>` + htmlEscape(row.Action) + `</td><td>` + htmlEscape(row.Target) + `</td><td>` + htmlEscape(row.Details) + `</td></tr>`)
	}
	if auditTable.Len() == 0 {
		auditTable.WriteString(`<tr><td colspan="5">No recent admin actions.</td></tr>`)
	}

	sessionTable := strings.Builder{}
	now := time.Now().UTC()
	for _, row := range nodeSessions {
		idle := now.Sub(row.LastActivity)
		if idle < 0 {
			idle = 0
		}
		sessionTable.WriteString(`<tr><td>` + htmlEscape(row.Username) + `</td><td>Node ` + strconv.Itoa(row.NodeID) + `</td><td>` + htmlEscape(row.Area) + `</td><td>` + row.LoginAt.Local().Format("2006-01-02 15:04") + `</td><td>` + formatDurationCompact(idle) + `</td><td>` + htmlEscape(strings.ToUpper(netutil.RemoteOrigin(row.RemoteAddr))) + `</td><td>` + htmlEscape(remoteHostDisplay(row.RemoteAddr)) + `</td></tr>`)
	}
	if sessionTable.Len() == 0 {
		sessionTable.WriteString(`<tr><td colspan="7">No active node sessions.</td></tr>`)
	}

	callerTable := strings.Builder{}
	for _, row := range callerHistory {
		callerTable.WriteString(`<tr><td>` + row.LogoutAt.Local().Format("2006-01-02 15:04") + `</td><td>` + htmlEscape(row.Username) + `</td><td>` + htmlEscape(row.Area) + `</td><td>` + formatDurationCompact(time.Duration(row.DurationSeconds)*time.Second) + `</td><td>` + htmlEscape(strings.ToUpper(netutil.RemoteOrigin(row.RemoteAddr))) + `</td><td>` + htmlEscape(remoteHostDisplay(row.RemoteAddr)) + `</td></tr>`)
	}
	if callerTable.Len() == 0 {
		callerTable.WriteString(`<tr><td colspan="6">No recent caller history.</td></tr>`)
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Ops Center</title></head><body>
<p><a href="/admin">admin</a> | <a href="/admin/launch">launch</a> | <a href="/admin/setup">setup</a> | <a href="/admin/system">system</a> | <a href="/admin/errors">errors</a> | <a href="/admin/audit">audit</a> | <a href="/status">status</a> | <a href="/help">help</a></p>
` + pageMessageBlock(r) + `
<h1>Ops Center</h1>
<p>Operator triage for launch readiness, runtime warnings, recent errors, audits, and active caller state.</p>
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(readiness.Summary.Pass) + `/` + strconv.Itoa(readiness.Summary.Total) + `</strong><span>launch checks passing</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(runtime.Summary.Warn) + `</strong><span>runtime warnings</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(errors)) + `</strong><span>recent runtime errors</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(nodeSessions)) + `</strong><span>active sessions</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(webSessionCount) + `</strong><span>web sessions</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(rateLimitCount) + `</strong><span>rate-limit buckets</span></article>
</section>
<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Readiness before polish</strong><p>Use launch and status signals to decide whether you have a real blocker or just a minor cleanup item.</p></article><article class="wolfbbs-helper-card"><strong>Errors change the meaning of green</strong><p>If runtime errors are piling up, treat every passing screen as provisional until you understand the failures.</p></article><article class="wolfbbs-helper-card"><strong>Audit + sessions explain surprises</strong><p>When callers report something odd, the fastest answers usually come from the recent audit trail and who is online right now.</p></article></section>
<section class="wolfbbs-grid">
<article><h2>Current Operator Focus</h2><ul>` + nextActions.String() + `</ul><p><a href="/admin/launch">Launch Center</a> | <a href="/admin/setup">Setup Wizard</a> | <a href="/status">Caller Status</a></p></article>
<article><h2>Operator Controls</h2><div class="wolfbbs-inline-actions"><form method="POST" action="/admin/ops"><input type="hidden" name="action" value="clear_errors">` + csrf + `<button type="submit">Clear Runtime Errors</button></form><form method="POST" action="/admin/ops" class="wolfbbs-inline-form"><input type="hidden" name="action" value="prune_idle_sessions">` + csrf + `<label>Prune idle sessions older than <input name="idle_minutes" value="30" inputmode="numeric"></label><button type="submit">Prune Sessions</button></form><form method="POST" action="/admin/ops"><input type="hidden" name="action" value="purge_web_sessions">` + csrf + `<button type="submit">Purge Expired Web Sessions</button></form><form method="POST" action="/admin/ops"><input type="hidden" name="action" value="clear_rate_limits">` + csrf + `<button type="submit">Clear Rate Limits</button></form></div><p><strong>Live counters:</strong> ` + strconv.Itoa(webSessionCount) + ` web sessions / ` + strconv.Itoa(rateLimitCount) + ` rate-limit buckets</p><h3>Operator Commands</h3><pre>bash install.sh --status
bash install.sh --doctor
bash install.sh --logs
bash install.sh --upgrade</pre></article>
</section>
<h2>Live Sessions</h2>
<table border="1"><tr><th>User</th><th>Node</th><th>Area</th><th>Login</th><th>Idle</th><th>Origin</th><th>From</th></tr>` + sessionTable.String() + `</table>
<h2>Recent Runtime Errors</h2>
<table border="1"><tr><th>When</th><th>Area</th><th>Error</th></tr>` + errorTable.String() + `</table>
<section class="wolfbbs-grid">
<article><h2>Recent Audit Trail</h2><table border="1"><tr><th>When</th><th>Actor</th><th>Action</th><th>Target</th><th>Details</th></tr>` + auditTable.String() + `</table></article>
<article><h2>Recent Callers</h2><table border="1"><tr><th>Logout</th><th>User</th><th>Area</th><th>Duration</th><th>Origin</th><th>From</th></tr>` + callerTable.String() + `</table></article>
</section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleToday(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	visibleBoards := a.visibleBoardsFor(user)
	watchSubs := a.boardSubscriptionIDs(user.Handle, boardSubscriptionWatch)
	digestSubs := a.boardSubscriptionIDs(user.Handle, boardSubscriptionDigest)
	muteSubs := a.boardSubscriptionIDs(user.Handle, boardSubscriptionMute)
	trackedBoards := filterBoardsByWatch(visibleBoards, watchSubs)
	trackedLabel := "watch tier"
	if len(trackedBoards) == 0 {
		trackedBoards = visibleBoards
		trackedLabel = "board pulse fallback"
	}
	boardPulse := a.buildBoardPulse(user, trackedBoards, 6)
	digestPulse := a.buildBoardPulse(user, filterBoardsByWatch(visibleBoards, digestSubs), 4)
	digest, _ := discovery.BuildSinceLastCall(a.boardRepo, a.msgRepo, a.mailRepo, user, 10)
	upcoming := a.upcomingCommunityEvents(5, time.Now().UTC())
	dashboard := a.buildBoardsDashboard(user, visibleBoards)
	recommendedDoor := "No recommendation yet."
	if dashboard.RecommendedDoor != "" {
		recommendedDoor = dashboard.RecommendedDoor
	}
	snapshot := a.buildFirstCallSnapshot(user)
	onboardingBlock := ``
	doneCount := 0
	for _, task := range snapshot.Tasks {
		if task.Done {
			doneCount++
		}
	}
	if doneCount < len(snapshot.Tasks) {
		onboardingBlock = renderOnboardingChecklist("Quick-Start Checklist", snapshot.Tasks)
	}

	nextRows := strings.Builder{}
	nextCount := 0
	readCount := 0
	for _, item := range digest.Items {
		itemKey := attentionItemKey(item)
		if a.isAttentionKeyDismissed(user.Handle, itemKey) || a.isAttentionKeyRead(user.Handle, itemKey) {
			if a.isAttentionKeyRead(user.Handle, itemKey) {
				readCount++
			}
			continue
		}
		href := "/discover"
		if item.BoardID > 0 {
			href = "/boards?board=" + strconv.FormatInt(item.BoardID, 10)
			if item.MessageID > 0 {
				href += "&id=" + strconv.FormatInt(item.MessageID, 10)
			}
		}
		nextRows.WriteString(`<li><a href="` + htmlEscape(href) + `">` + htmlEscape(item.Line) + `</a></li>`)
		nextCount++
		if nextCount >= 6 {
			break
		}
	}
	if nextRows.Len() == 0 {
		nextRows.WriteString(`<li>No unread direct follow-up items are waiting.</li>`)
	}

	boardRows := strings.Builder{}
	watchSignalCount := 0
	for _, row := range boardPulse {
		if itemKey := boardAttentionItemKey(row); a.isAttentionKeyDismissed(user.Handle, itemKey) || a.isAttentionKeyRead(user.Handle, itemKey) {
			if a.isAttentionKeyRead(user.Handle, itemKey) {
				readCount++
			}
			continue
		}
		watchSignalCount++
		boardRows.WriteString(`<tr><td><a href="/boards?board=` + strconv.FormatInt(row.BoardID, 10) + `">` + htmlEscape(row.BoardName) + `</a></td><td>` + htmlEscape(defaultConferenceValue(row.Conference)) + `</td><td>` + strconv.Itoa(row.NewCount) + `</td><td>` + strconv.Itoa(row.MessageCount) + `</td><td>` + htmlEscape(row.LastAt) + `</td><td>` + htmlEscape(row.LastSubject) + `</td></tr>`)
	}
	if boardRows.Len() == 0 {
		boardRows.WriteString(`<tr><td colspan="6">No unread watch-tier movement yet. Use board subscriptions inside <a href="/boards">/boards</a>.</td></tr>`)
	}

	digestRows := strings.Builder{}
	digestSignalCount := 0
	for _, row := range digestPulse {
		if itemKey := boardAttentionItemKey(row); a.isAttentionKeyDismissed(user.Handle, itemKey) || a.isAttentionKeyRead(user.Handle, itemKey) {
			if a.isAttentionKeyRead(user.Handle, itemKey) {
				readCount++
			}
			continue
		}
		digestSignalCount++
		digestRows.WriteString(`<tr><td><a href="/boards?board=` + strconv.FormatInt(row.BoardID, 10) + `">` + htmlEscape(row.BoardName) + `</a></td><td>` + strconv.Itoa(row.NewCount) + `</td><td>` + htmlEscape(row.LastAt) + `</td><td>` + htmlEscape(row.LastSubject) + `</td></tr>`)
	}
	if digestRows.Len() == 0 {
		digestRows.WriteString(`<tr><td colspan="4">No unread digest-tier movement yet. Digest boards appear here instead of Attention Center.</td></tr>`)
	}

	eventRows := strings.Builder{}
	for _, row := range upcoming {
		link := ""
		if strings.TrimSpace(row.Link) != "" {
			link = ` <a href="` + htmlEscape(strings.TrimSpace(row.Link)) + `">details</a>`
		}
		meta := []string{formatCommunityEventWindow(row)}
		if recurrence := recurrenceSummary(row); recurrence != "" {
			meta = append(meta, recurrence)
		}
		if strings.TrimSpace(row.Location) != "" {
			meta = append(meta, row.Location)
		}
		if strings.TrimSpace(row.Host) != "" {
			meta = append(meta, "hosted by "+row.Host)
		}
		eventRows.WriteString(`<li><strong>` + htmlEscape(row.Title) + `</strong> <span class="wolfbbs-muted">` + htmlEscape(strings.Join(meta, " | ")) + `</span><br>` + htmlEscape(cleanOneLiner(row.Description, 140)) + link + `</li>`)
	}
	if eventRows.Len() == 0 {
		eventRows.WriteString(`<li>No community events are scheduled yet.</li>`)
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Today Brief</title></head><body>
<p><a href="/start">start</a> | <a href="/attention">attention</a> | <a href="/boards">boards</a> | <a href="/events">events</a> | <a href="/mail">mail</a> | <a href="/radar">radar</a> | <a href="/doors">doors</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Today Brief</h1>
<p>One screen for the daily caller loop: immediate follow-up, subscription-tier board movement, upcoming events, and the best next route.</p>
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(nextCount+watchSignalCount+digestSignalCount) + `</strong><span>current loop signals</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(readCount) + `</strong><span>already read</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(dashboard.UnreadMail) + `</strong><span>unread mail</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(watchSubs)) + ` / ` + strconv.Itoa(len(digestSubs)) + ` / ` + strconv.Itoa(len(muteSubs)) + `</strong><span>watch / digest / mute</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(upcoming)) + `</strong><span>upcoming events</span></article>
<article class="wolfbbs-kpi-card"><strong>` + htmlEscape(recommendedDoor) + `</strong><span>recommended door</span></article>
</section>
` + onboardingBlock + `
<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Use Today first</strong><p>Start here when you want the shortest path through what changed.</p></article><article class="wolfbbs-helper-card"><strong>Subscription tiers matter</strong><p>Watch boards escalate into Attention Center, digest boards land here, and mute hides a board from routine loops without unsubscribing from it permanently.</p></article><article class="wolfbbs-helper-card"><strong>Events give callers a reason to return</strong><p>Use <a href="/events">/events</a> for the public calendar and <a href="/admin/events">/admin/events</a> to schedule recurring rhythms.</p></article></section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Needs Response</h2><ul>` + nextRows.String() + `</ul><p><a href="/attention">Open Attention Center</a> | <a href="/discover">Open Discover</a></p></article>
<article class="wolfbbs-card"><h2>Community Calendar</h2><ul>` + eventRows.String() + `</ul><p><a href="/events">Open full calendar</a></p></article>
</section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Watch Tier Boards</h2><p class="wolfbbs-muted">Currently showing ` + htmlEscape(trackedLabel) + `.</p><table border="1"><tr><th>Board</th><th>Conf</th><th>New</th><th>Total</th><th>Last</th><th>Last subject</th></tr>` + boardRows.String() + `</table></article>
<article class="wolfbbs-card"><h2>Digest Tier Boards</h2><p class="wolfbbs-muted">Lower urgency subscriptions stay here instead of interrupting Attention Center.</p><table border="1"><tr><th>Board</th><th>New</th><th>Last</th><th>Last subject</th></tr>` + digestRows.String() + `</table></article>
</section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Next Move</h2><ul><li><a href="/mail">Mail</a> if you need private follow-up.</li><li><a href="/boards?mode=watched">Watch tier</a> when you want threaded catch-up.</li><li><a href="/boards?mode=digest">Digest tier</a> when you want low-noise board scanning.</li><li><a href="/doors?mode=recommended">Doors</a> when you want a quick return loop.</li><li><a href="/chat">Chat</a> when the conversation should be live.</li></ul></article></section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleEventsCalendar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, _ := a.currentUser(r)
	nav := `<a href="/start">start</a> | <a href="/connect">connect</a> | <a href="/tour">tour</a> | <a href="/help">help</a>`
	if user != nil {
		nav = `<a href="/start">start</a> | <a href="/today">today</a> | <a href="/attention">attention</a> | <a href="/boards">boards</a> | <a href="/chat">chat</a> | <a href="/help">help</a> | <a href="/logout">logout</a>`
		if a.hasRole(user, roleAdmin) {
			nav = `<a href="/start">start</a> | <a href="/today">today</a> | <a href="/events">events</a> | <a href="/admin/events">admin events</a> | <a href="/admin/ops">ops</a> | <a href="/help">help</a> | <a href="/logout">logout</a>`
		}
	}
	now := time.Now().UTC()
	upcoming := a.upcomingCommunityEvents(24, now)
	recent := a.recentCommunityEvents(12, now)
	renderList := func(rows []communityEvent, empty string) string {
		out := strings.Builder{}
		for _, row := range rows {
			meta := []string{strings.ToUpper(row.Category), formatCommunityEventWindow(row)}
			if recurrence := recurrenceSummary(row); recurrence != "" {
				meta = append(meta, recurrence)
			}
			if strings.TrimSpace(row.Location) != "" {
				meta = append(meta, row.Location)
			}
			if strings.TrimSpace(row.Audience) != "" {
				meta = append(meta, row.Audience)
			}
			link := ""
			if strings.TrimSpace(row.Link) != "" {
				link = ` <a href="` + htmlEscape(strings.TrimSpace(row.Link)) + `">details</a>`
			}
			hostLine := ""
			if strings.TrimSpace(row.Host) != "" {
				hostLine = `<p class="wolfbbs-muted">Host: ` + htmlEscape(row.Host) + `</p>`
			}
			out.WriteString(`<article class="wolfbbs-card"><h3>` + htmlEscape(row.Title) + `</h3><p class="wolfbbs-muted">` + htmlEscape(strings.Join(meta, " | ")) + `</p>` + hostLine + `<p>` + htmlEscape(cleanOneLiner(row.Description, 220)) + link + `</p></article>`)
		}
		if out.Len() == 0 {
			out.WriteString(`<article class="wolfbbs-card"><p>` + htmlEscape(empty) + `</p></article>`)
		}
		return out.String()
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Community Calendar</title></head><body>
<p>` + nav + `</p>
` + pageMessageBlock(r) + `
<h1>Community Calendar</h1>
<p>Public schedule for nets, tournaments, social calls, and content drops. Use this page to give callers a concrete reason to return.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(upcoming)) + `</strong><span>upcoming events</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(recent)) + `</strong><span>recent events</span></article></section>
<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Plan against real dates</strong><p>Events work best when they are specific, visible, and easy to join from the board.</p></article><article class="wolfbbs-helper-card"><strong>Use Today for a caller brief</strong><p><a href="/today">/today</a> pulls upcoming events into the daily caller loop after login.</p></article><article class="wolfbbs-helper-card"><strong>Run this like a product</strong><p>Sysops should schedule recurring reasons to return instead of expecting callers to invent their own rhythm.</p></article></section>
<section><h2>Upcoming Events</h2><div class="wolfbbs-grid">` + renderList(upcoming, "No upcoming events scheduled yet.") + `</div></section>
<section><h2>Recent Events</h2><div class="wolfbbs-grid">` + renderList(recent, "No recent events yet.") + `</div></section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminEvents(w http.ResponseWriter, r *http.Request) {
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
			title := strings.TrimSpace(r.FormValue("title"))
			startsAt, err := parseLocalDateTime(r.FormValue("starts_at"))
			if title == "" || err != nil {
				redirectWithError(w, r, "/admin/events", "Title and a valid start time are required.")
				return
			}
			endsAt := time.Time{}
			if strings.TrimSpace(r.FormValue("ends_at")) != "" {
				endsAt, err = parseLocalDateTime(r.FormValue("ends_at"))
				if err != nil {
					redirectWithError(w, r, "/admin/events", "End time must be a valid local date/time.")
					return
				}
				if endsAt.Before(startsAt) {
					redirectWithError(w, r, "/admin/events", "End time must be after the start time.")
					return
				}
			}
			recurrence := normalizeEventRecurrence(r.FormValue("recurrence"))
			repeatUntil := time.Time{}
			if strings.TrimSpace(r.FormValue("repeat_until")) != "" {
				repeatUntil, err = parseLocalDateTime(r.FormValue("repeat_until"))
				if err != nil {
					redirectWithError(w, r, "/admin/events", "Repeat-until must be a valid local date/time.")
					return
				}
				if repeatUntil.Before(startsAt) {
					redirectWithError(w, r, "/admin/events", "Repeat-until must be after the start time.")
					return
				}
			}
			if recurrence != "" && repeatUntil.IsZero() {
				repeatUntil = startsAt.Add(90 * 24 * time.Hour)
			}
			rows := a.loadCommunityEvents()
			eventID := randomEventID()
			row := communityEvent{
				ID:          eventID,
				SeriesID:    eventID,
				Title:       title,
				Category:    normalizeEventCategory(r.FormValue("category")),
				StartsAt:    startsAt.UTC(),
				EndsAt:      endsAt.UTC(),
				Recurrence:  recurrence,
				RepeatUntil: repeatUntil.UTC(),
				Location:    strings.TrimSpace(r.FormValue("location")),
				Host:        strings.TrimSpace(r.FormValue("host")),
				Audience:    strings.TrimSpace(r.FormValue("audience")),
				Description: strings.TrimSpace(r.FormValue("description")),
				Link:        strings.TrimSpace(r.FormValue("link")),
				CreatedAt:   time.Now().UTC(),
			}
			if row.Audience == "" {
				row.Audience = "all callers"
			}
			rows = append(rows, row)
			a.persistCommunityEvents(rows)
			recurrenceLabel := row.Recurrence
			if strings.TrimSpace(recurrenceLabel) == "" {
				recurrenceLabel = "none"
			}
			a.recordAdminAction(user.Handle, "community_calendar", "create_event", fmt.Sprintf("id=%s title=%s recurrence=%s", row.ID, row.Title, recurrenceLabel))
			redirectWithNotice(w, r, "/admin/events", "Community event created.")
			return
		case "delete":
			id := strings.TrimSpace(r.FormValue("id"))
			if id == "" {
				redirectWithError(w, r, "/admin/events", "Event ID is required.")
				return
			}
			rows := a.loadCommunityEvents()
			next := make([]communityEvent, 0, len(rows))
			deletedTitle := ""
			for _, row := range rows {
				if row.ID == id {
					deletedTitle = row.Title
					continue
				}
				next = append(next, row)
			}
			if deletedTitle == "" {
				redirectWithError(w, r, "/admin/events", "Event not found.")
				return
			}
			a.persistCommunityEvents(next)
			a.recordAdminAction(user.Handle, "community_calendar", "delete_event", fmt.Sprintf("id=%s title=%s", id, deletedTitle))
			redirectWithNotice(w, r, "/admin/events", "Community event deleted.")
			return
		default:
			redirectWithError(w, r, "/admin/events", "Unsupported events action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rows := a.loadCommunityEvents()
	csrf := a.csrfHiddenInput(r)
	eventRows := strings.Builder{}
	for _, row := range rows {
		recurrenceLabel := recurrenceSummary(row)
		if strings.TrimSpace(recurrenceLabel) == "" {
			recurrenceLabel = "one-time"
		}
		eventRows.WriteString(`<tr><td>` + htmlEscape(row.Title) + `</td><td>` + htmlEscape(strings.ToUpper(row.Category)) + `</td><td>` + htmlEscape(formatCommunityEventWindow(row)) + `</td><td>` + htmlEscape(recurrenceLabel) + `</td><td>` + htmlEscape(row.Location) + `</td><td>` + htmlEscape(row.Audience) + `</td><td><form method="POST" action="/admin/events"><input type="hidden" name="action" value="delete"><input type="hidden" name="id" value="` + htmlEscape(row.ID) + `">` + csrf + `<button type="submit">Delete</button></form></td></tr>`)
	}
	if eventRows.Len() == 0 {
		eventRows.WriteString(`<tr><td colspan="7">No events scheduled yet.</td></tr>`)
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Events Admin</title></head><body>
<p><a href="/admin">admin</a> | <a href="/admin/setup">setup</a> | <a href="/admin/ops">ops</a> | <a href="/events">public calendar</a> | <a href="/today">today brief</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Events Admin</h1>
<p>Schedule public reasons for callers to return: nets, tournaments, featured content drops, and operator-run sessions.</p>
<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Be concrete</strong><p>Give callers a specific time, place, and audience. Vague events do not drive return behavior.</p></article><article class="wolfbbs-helper-card"><strong>Use categories deliberately</strong><p>System, social, door, tournament, content, and ops let the calendar read like a real board schedule.</p></article><article class="wolfbbs-helper-card"><strong>Build series, not chores</strong><p>Recurring events turn the board into a habit. Weekly or weekday series are better than retyping the same entry every day.</p></article></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Create Event</h2><form method="POST" action="/admin/events"><input type="hidden" name="action" value="create">` + csrf + `<label>Title <input name="title" size="48" placeholder="Friday Tournament Night"></label><br><label>Category <select name="category"><option value="social">social</option><option value="door">door</option><option value="tournament">tournament</option><option value="content">content</option><option value="system">system</option><option value="ops">ops</option></select></label><br><label>Starts <input type="datetime-local" name="starts_at"></label><br><label>Ends <input type="datetime-local" name="ends_at"></label><br><label>Recurrence <select name="recurrence"><option value="">one-time</option><option value="daily">daily</option><option value="weekdays">weekdays</option><option value="weekly">weekly</option><option value="monthly">monthly</option></select></label><br><label>Repeat Until <input type="datetime-local" name="repeat_until"></label><br><label>Location <input name="location" size="40" placeholder="#lobby, Door Cockpit, SSH"></label><br><label>Host <input name="host" size="40" placeholder="sysop"></label><br><label>Audience <input name="audience" size="40" placeholder="all callers"></label><br><label>Link <input name="link" size="60" placeholder="/doors or https://..."></label><br><label>Description<br><textarea name="description" rows="6" cols="72" placeholder="What happens, why it matters, and how to join."></textarea></label><br><button type="submit">Create Event</button></form></article><article class="wolfbbs-card"><h2>Event Design Notes</h2><ul><li>Use a real start time, not "later tonight".</li><li>Put join instructions in the description or link.</li><li>Use recurrence for weekly nets, weekday check-ins, or tournament ladders.</li><li>Delete or replace stale series instead of letting the calendar drift.</li></ul></article></section>
<section><h2>Scheduled Events</h2><table border="1"><tr><th>Title</th><th>Category</th><th>When</th><th>Series</th><th>Location</th><th>Audience</th><th>Action</th></tr>` + eventRows.String() + `</table></section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
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
		subjectHost = sanitizedConfiguredHost(a.siteHost())
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
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		a.handleActivityPubActor(w, r, user)
		return
	}
	switch strings.ToLower(strings.TrimSpace(parts[1])) {
	case "outbox":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		a.handleActivityPubOutbox(w, r, user)
	case "inbox":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		a.handleActivityPubInbox(w, r, user)
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

func (a *webApp) handleActivityPubInbox(w http.ResponseWriter, r *http.Request, user *domain.User) {
	r.Body = http.MaxBytesReader(w, r.Body, maxActivityPubInboxBytes)
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.UseNumber()

	var payload map[string]interface{}
	if err := dec.Decode(&payload); err != nil {
		http.Error(w, "invalid activity payload", http.StatusBadRequest)
		return
	}
	var extra interface{}
	if err := dec.Decode(&extra); err != io.EOF {
		http.Error(w, "invalid activity payload", http.StatusBadRequest)
		return
	}

	activityType := activityPubPrimaryType(payload["type"])
	if activityType == "" {
		http.Error(w, "activity type is required", http.StatusBadRequest)
		return
	}
	switch activityType {
	case "Accept", "Announce", "Create", "Delete", "Follow", "Like", "Undo", "Update":
	default:
		http.Error(w, "unsupported activity type", http.StatusBadRequest)
		return
	}

	actor := activityPubReference(payload["actor"])
	if actor == "" {
		http.Error(w, "actor is required", http.StatusBadRequest)
		return
	}
	objectRef := activityPubReference(payload["object"])
	activityID := activityPubReference(payload["id"])

	details := []string{
		"type=" + activityType,
		"actor=" + actor,
	}
	if objectRef != "" {
		details = append(details, "object="+objectRef)
	}
	if activityID != "" {
		details = append(details, "id="+activityID)
	}
	a.recordAdminAction("activitypub", user.Handle, "activitypub_inbox_"+strings.ToLower(activityType), strings.Join(details, " "))

	_ = writeJSON(w, http.StatusAccepted, map[string]string{
		"status":    "accepted",
		"type":      activityType,
		"recipient": user.Handle,
	})
}

func activityPubPrimaryType(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case []interface{}:
		for _, item := range value {
			if kind := activityPubPrimaryType(item); kind != "" {
				return kind
			}
		}
	case []string:
		for _, item := range value {
			if kind := strings.TrimSpace(item); kind != "" {
				return kind
			}
		}
	}
	return ""
}

func activityPubReference(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case []interface{}:
		for _, item := range value {
			if ref := activityPubReference(item); ref != "" {
				return ref
			}
		}
	case map[string]interface{}:
		if id := activityPubReference(value["id"]); id != "" {
			return id
		}
		if href := activityPubReference(value["url"]); href != "" {
			return href
		}
		if kind := activityPubPrimaryType(value["type"]); kind != "" {
			return kind
		}
	}
	return ""
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
  --surface-3:#eef5ff;
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
hr{
  border:0;
  border-top:1px solid var(--line);
  margin:16px 0;
}
.wolfbbs-muted{color:var(--muted)}
.wolfbbs-kpi-grid{
  display:grid;
  grid-template-columns:repeat(auto-fit,minmax(140px,1fr));
  gap:12px;
  margin:16px 0 18px;
}
.wolfbbs-kpi-card,.wolfbbs-card,.wolfbbs-action-card{
  background:var(--surface);
  border:1px solid var(--line);
  border-radius:16px;
  box-shadow:var(--shadow);
}
.wolfbbs-kpi-card{
  padding:16px 18px;
  display:flex;
  flex-direction:column;
  gap:4px;
}
.wolfbbs-kpi-card strong{
  font-size:1.65rem;
  line-height:1;
}
.wolfbbs-kpi-card span{
  color:var(--muted);
  font-weight:600;
}
.wolfbbs-grid{
  display:grid;
  grid-template-columns:repeat(auto-fit,minmax(260px,1fr));
  gap:14px;
  margin:14px 0 18px;
}
.wolfbbs-card-grid{
  display:grid;
  grid-template-columns:repeat(auto-fit,minmax(220px,1fr));
  gap:12px;
}
.wolfbbs-card{
  padding:16px 18px;
}
.wolfbbs-primer{
  position:relative;
  overflow:hidden;
  padding:18px 20px;
  margin:14px 0 18px;
  background:
    radial-gradient(circle at top right, rgba(15,79,168,.16), transparent 34%),
    linear-gradient(180deg,#fbfdff,#f2f7ff);
}
.wolfbbs-primer::after{
  content:"";
  position:absolute;
  inset:auto -30px -60px auto;
  width:180px;
  height:180px;
  background:radial-gradient(circle, rgba(15,79,168,.12), transparent 70%);
}
.wolfbbs-primer-eyebrow{
  display:inline-flex;
  margin-bottom:8px;
  color:#355f94;
  font-size:.78rem;
  font-weight:800;
  letter-spacing:.08em;
  text-transform:uppercase;
}
.wolfbbs-primer-title{
  display:block;
  margin-bottom:8px;
  color:#17385e;
  font-size:1.08rem;
}
.wolfbbs-primer p{
  margin:0 0 10px;
  color:#28415d;
}
.wolfbbs-primer ul{
  margin:10px 0 0 18px;
}
.wolfbbs-primer-actions{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  margin-top:12px;
}
.wolfbbs-primer-actions a{
  display:inline-flex;
  align-items:center;
  min-height:32px;
  padding:6px 12px;
  border-radius:999px;
  border:1px solid #c3d4ea;
  background:#f5faff;
  color:#0f3f83;
  font-size:.9rem;
  font-weight:700;
}
.wolfbbs-primer-actions a:hover{
  background:#eaf3ff;
  text-decoration:none;
}
.wolfbbs-action-grid{
  display:grid;
  grid-template-columns:repeat(auto-fit,minmax(170px,1fr));
  gap:10px;
  margin-top:12px;
}
.wolfbbs-action-card{
  display:flex;
  flex-direction:column;
  gap:6px;
  padding:14px 16px;
  color:var(--text);
  background:
    radial-gradient(circle at top right, rgba(15,79,168,.10), transparent 42%),
    linear-gradient(180deg,var(--surface),var(--surface-2));
}
.wolfbbs-action-card:hover{
  text-decoration:none;
  transform:translateY(-1px);
  transition:transform .16s ease;
}
.wolfbbs-action-card strong{
  color:#17385e;
  font-size:1rem;
}
.wolfbbs-action-card span{
  color:var(--muted);
  font-size:.92rem;
}
.wolfbbs-chip-row{
  display:flex;
  flex-wrap:wrap;
  gap:6px;
}
.wolfbbs-chip{
  display:inline-flex;
  align-items:center;
  min-height:24px;
  padding:2px 9px;
  border-radius:999px;
  background:#eef4ff;
  border:1px solid #c9daf3;
  color:#244c83;
  font-size:.8rem;
  font-weight:700;
}
.wolfbbs-inline-form{
  display:flex;
  flex-wrap:wrap;
  align-items:flex-end;
  gap:10px;
}
.wolfbbs-inline-form label{
  margin:0;
}
.wolfbbs-section-nav,.wolfbbs-recent-rail{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  margin:0 0 16px;
}
.wolfbbs-section-nav a,.wolfbbs-recent-rail a{
  display:inline-flex;
  align-items:center;
  min-height:30px;
  padding:6px 11px;
  border-radius:999px;
  background:rgba(255,255,255,.72);
  border:1px solid #cbdaee;
  color:#21467c;
  font-size:.85rem;
  font-weight:700;
}
.wolfbbs-section-nav a:hover,.wolfbbs-recent-rail a:hover{
  text-decoration:none;
  background:#eef5ff;
}
.wolfbbs-inline-filter{
  margin:12px 0 10px;
  padding:10px 12px;
  background:rgba(255,255,255,.78);
  border:1px solid var(--line);
  border-radius:12px;
  box-shadow:var(--shadow);
}
.wolfbbs-banner{
  display:flex;
  align-items:flex-start;
  justify-content:space-between;
  gap:12px;
  margin:14px 0;
  padding:14px 16px;
  border-radius:14px;
  border:1px solid var(--line);
  background:linear-gradient(180deg,#fbfdff,#f3f8ff);
  box-shadow:var(--shadow);
}
.wolfbbs-banner strong{
  display:block;
  margin-bottom:4px;
}
.wolfbbs-banner p{
  margin:0;
}
.wolfbbs-banner[data-kind="notice"]{
  border-color:#b9d5be;
  background:linear-gradient(180deg,#f7fff9,#edf9f0);
}
.wolfbbs-banner[data-kind="error"]{
  border-color:#efc1c6;
  background:linear-gradient(180deg,#fff9fa,#fff0f2);
}
.wolfbbs-banner-close{
  border:0;
  min-height:auto;
  padding:4px 8px;
  border-radius:999px;
  background:#e7eef8;
  color:#26486f;
  font-size:.8rem;
  box-shadow:none;
}
.wolfbbs-banner-close:hover{
  background:#d7e6fb;
}
.wolfbbs-filter-summary{
  margin:12px 0 16px;
}
.wolfbbs-filter-summary h2{
  margin-bottom:8px;
}
.wolfbbs-active-filters{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  align-items:center;
  margin:10px 0 0;
}
.wolfbbs-filter-chip{
  display:inline-flex;
  align-items:center;
  gap:6px;
  min-height:28px;
  padding:4px 10px;
  border-radius:999px;
  border:1px solid #c6d8ef;
  background:#f6fbff;
  color:#244c83;
  font-size:.82rem;
  font-weight:700;
}
.wolfbbs-filter-reset{
  display:inline-flex;
  align-items:center;
  min-height:28px;
  padding:4px 10px;
  border-radius:999px;
  border:1px solid #d3dce8;
  background:#fff;
  color:#37506c;
  font-size:.82rem;
  font-weight:700;
}
.wolfbbs-filter-reset:hover{
  text-decoration:none;
  background:#f5f9fe;
}
.wolfbbs-form-note{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  align-items:center;
  margin:0 0 10px;
  color:#35506b;
  font-size:.86rem;
}
.wolfbbs-form-note strong{
  color:#17385e;
}
.wolfbbs-form-status{
  display:inline-flex;
  align-items:center;
  min-height:26px;
  padding:3px 10px;
  border-radius:999px;
  background:#edf4ff;
  border:1px solid #c8daf1;
  color:#244c83;
  font-size:.78rem;
  font-weight:700;
}
.wolfbbs-form-status.dirty{
  background:#fff5df;
  border-color:#f0d7a0;
  color:#7b5400;
}
.wolfbbs-form-status.error{
  background:#fff0f2;
  border-color:#efc1c6;
  color:#8b2331;
}
.wolfbbs-form-secondary{
  border:1px solid #c8d7ea;
  background:#f6fbff;
  color:#23497d;
}
.wolfbbs-form-secondary:hover{
  background:#eaf3ff;
}
.wolfbbs-compose-shell{
  display:flex;
  flex-direction:column;
  gap:10px;
}
.wolfbbs-compose-toolbar{
  display:flex;
  flex-wrap:wrap;
  align-items:center;
  gap:8px;
  padding:10px 12px;
  border:1px solid #d2deef;
  border-radius:12px;
  background:#f6faff;
}
.wolfbbs-compose-toolbar button{
  min-height:30px;
  padding:6px 10px;
  border-radius:999px;
  border:1px solid #c8d7ea;
  background:#fff;
  color:#21467c;
  box-shadow:none;
  font-size:.82rem;
}
.wolfbbs-compose-toolbar button:hover{
  background:#eef5ff;
}
.wolfbbs-compose-meta{
  margin-left:auto;
  color:#4a5f76;
  font-size:.8rem;
  font-weight:700;
}
.wolfbbs-compose-preview{
  display:none;
  padding:12px 14px;
  border:1px dashed #bfd2eb;
  border-radius:12px;
  background:#fbfdff;
  color:#24384e;
}
.wolfbbs-compose-preview.active{
  display:block;
}
.wolfbbs-compose-preview p:last-child{
  margin-bottom:0;
}
.wolfbbs-compose-help{
  display:none;
  padding:12px 14px;
  border:1px solid #d6e1f0;
  border-radius:12px;
  background:#ffffff;
  color:#28415d;
}
.wolfbbs-compose-help.active{
  display:block;
}
.wolfbbs-compose-help strong{
  display:block;
  margin-bottom:8px;
}
.wolfbbs-compose-help ul{
  margin:0;
  padding-left:18px;
}
.wolfbbs-compose-focus{
  position:relative;
  z-index:3;
}
.wolfbbs-compose-focus textarea{
  min-height:320px;
}
.wolfbbs-compose-focus .wolfbbs-compose-shell{
  padding:12px;
  border:1px solid #c6d8ef;
  border-radius:14px;
  background:#ffffff;
  box-shadow:0 18px 30px rgba(15,27,42,.12);
}
body.wolfbbs-compose-fullscreen-open{
  overflow:hidden;
}
.wolfbbs-compose-fullscreen{
  position:fixed;
  inset:16px;
  z-index:82;
  overflow:auto;
  padding:18px;
  border:1px solid #c6d8ef;
  border-radius:18px;
  background:rgba(255,255,255,.98);
  box-shadow:0 24px 60px rgba(15,27,42,.22);
}
.wolfbbs-compose-fullscreen textarea{
  min-height:58vh;
}
.wolfbbs-compose-fullscreen .wolfbbs-compose-shell{
  padding:14px;
  border:1px solid #c6d8ef;
  border-radius:14px;
  background:#ffffff;
}
.wolfbbs-command-block{
  position:relative;
}
.wolfbbs-copy-button{
  position:absolute;
  top:10px;
  right:10px;
  border:1px solid #cad8eb;
  min-height:auto;
  padding:4px 9px;
  border-radius:999px;
  background:#fff;
  color:#26486f;
  box-shadow:none;
  font-size:.8rem;
}
.wolfbbs-copy-button:hover{
  background:#eef5ff;
}
.wolfbbs-inline-actions{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  align-items:center;
}
.wolfbbs-split{
  display:grid;
  grid-template-columns:repeat(auto-fit,minmax(260px,1fr));
  gap:14px;
  margin:14px 0 18px;
}
.wolfbbs-list-clean{
  list-style:none;
  padding:0;
  margin:0;
}
.wolfbbs-list-clean li{
  padding:8px 0;
  border-bottom:1px solid #e6eef8;
}
.wolfbbs-list-clean li:last-child{
  border-bottom:0;
}
.wolfbbs-helper-grid{
  display:grid;
  grid-template-columns:repeat(auto-fit,minmax(200px,1fr));
  gap:10px;
  margin:12px 0 16px;
}
.wolfbbs-helper-card{
  padding:14px 16px;
  border-radius:14px;
  border:1px solid var(--line);
  background:linear-gradient(180deg,#ffffff,#f5f9ff);
  box-shadow:var(--shadow);
}
.wolfbbs-helper-card strong{
  display:block;
  margin-bottom:6px;
  color:#17385e;
}
.wolfbbs-helper-card p{
  margin:0;
  color:#4b6178;
}
.wolfbbs-submit-busy{
  opacity:.72;
  pointer-events:none;
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
#wolfbbsCommandButton{
  position:fixed;
  right:20px;
  bottom:18px;
  z-index:40;
  min-height:42px;
  padding:10px 14px;
  border-radius:999px;
  border:1px solid rgba(255,255,255,.25);
  box-shadow:0 18px 34px rgba(9,57,122,.18);
}
#wolfbbsPaletteOverlay{
  position:fixed;
  inset:0;
  background:rgba(10,18,30,.42);
  display:none;
  z-index:60;
  padding:24px 16px;
}
#wolfbbsPaletteOverlay.active{display:block}
#wolfbbsPalette{
  max-width:760px;
  margin:0 auto;
  background:var(--surface);
  border:1px solid var(--line);
  border-radius:18px;
  box-shadow:0 22px 48px rgba(15,27,42,.22);
  overflow:hidden;
}
#wolfbbsPaletteHeader{
  padding:14px;
  background:linear-gradient(180deg,#f7fbff,#edf4fc);
  border-bottom:1px solid var(--line);
}
#wolfbbsPaletteList{
  max-height:min(60vh,520px);
  overflow:auto;
  padding:8px;
}
.wolfbbs-palette-item{
  display:flex;
  justify-content:space-between;
  gap:12px;
  padding:11px 12px;
  border-radius:12px;
  color:var(--text);
}
.wolfbbs-palette-item:hover{
  background:#eef5ff;
  text-decoration:none;
}
.wolfbbs-palette-meta{
  color:var(--muted);
  font-size:.82rem;
}
@media (max-width: 820px){
  body{padding:18px 14px 26px}
  body > p:has(> a){padding:9px 10px}
  input[type=text],input[type=password],input[type=email],input[type=number],input[type=url],input[type=search],select,textarea{
    width:100%;
  }
  #wolfbbsCommandButton{
    left:14px;
    right:14px;
    bottom:14px;
    justify-content:center;
  }
}
</style>
<script id="wolfbbs-modern-ui-js">
(() => {
  function initWolfbbsModernUI() {
    if (document.documentElement.dataset.wolfbbsModernUi === "1") return;
    document.documentElement.dataset.wolfbbsModernUi = "1";

  const navRows = Array.from(document.querySelectorAll("p")).filter((p) => p.querySelectorAll("a").length >= 3 && p.textContent.includes("|"));
  navRows.forEach((row) => row.classList.add("wolfbbs-nav-row"));

  const title = (document.querySelector("h1") && document.querySelector("h1").textContent.trim()) || document.title || "WolfBBS";
  const currentPath = location.pathname + location.search;
  const primerRegistry = {
    "/help": {
      eyebrow: "Start here",
      title: "Use WolfBBS by intent",
      body: "This page is the route map. If you run the board, finish /admin/setup before treating the product as ready. If you are a caller, start with boards, chat, and doors.",
      bullets: [
        "Use /admin/setup before /admin/config when launching a fresh board.",
        "Use /chat or IRC for the same live conversation layer.",
        "Use SSH when you want the full ANSI board feel."
      ],
      actions: [
        { label: "Admin setup", href: "/admin/setup" },
        { label: "Boards", href: "/boards" },
        { label: "Chat", href: "/chat" }
      ]
    },
    "/admin/setup": {
      eyebrow: "Launch path",
      title: "Finish setup in this order",
      body: "Identity and safety first, bootstrap actions second, then validate the real caller surfaces.",
      bullets: [
        "Save Step 1 and Step 2 before inviting users.",
        "Seed boards and create a real non-sysop account.",
        "Check /status and /admin/system after bootstrap."
      ],
      actions: [
        { label: "Admin config", href: "/admin/config" },
        { label: "Users", href: "/admin/users" },
        { label: "Status", href: "/status" }
      ]
    },
    "/admin/launch": {
      eyebrow: "Operator flow",
      title: "Run the board like a product, not a scavenger hunt",
      body: "This page consolidates launch readiness, runtime health, operator commands, and the direct links you need when the board is almost ready but not obviously done.",
      bullets: [
        "Fix launch blockers first, polish second.",
        "Walk the real caller journey before announcing anything.",
        "Use this route as the home base for first-run and recovery."
      ],
      actions: [
        { label: "Setup wizard", href: "/admin/setup" },
        { label: "System", href: "/admin/system" },
        { label: "Users", href: "/admin/users" }
      ]
    },
    "/admin/config": {
      eyebrow: "Runtime controls",
      title: "Use config after setup, not instead of it",
      body: "This page is for runtime flags, identity details, and service exposure after the baseline setup wizard is complete.",
      bullets: [
        "Keep public-facing changes deliberate.",
        "Verify status after changing ports, proxies, or optional services."
      ],
      actions: [
        { label: "Setup wizard", href: "/admin/setup" },
        { label: "System", href: "/admin/system" },
        { label: "Help", href: "/help" }
      ]
    },
    "/start": {
      eyebrow: "First stop",
      title: "Use Start Center to pick the right lane",
      body: "This page exists so guests, callers, and sysops can get a concrete next move without route hunting.",
      actions: [
        { label: "Connect", href: "/connect" },
        { label: "Today", href: "/today" },
        { label: "Help", href: "/help" },
        { label: "Boards", href: "/boards" }
      ]
    },
    "/today": {
      eyebrow: "Daily loop",
      title: "Today Brief compresses the caller day",
      body: "Use this page when you want watched boards, direct follow-up, and upcoming events in one place before you drift into route hunting.",
      actions: [
        { label: "Attention", href: "/attention" },
        { label: "Events", href: "/events" },
        { label: "Boards", href: "/boards?mode=watched" }
      ]
    },
    "/attention": {
      eyebrow: "Action queue",
      title: "Attention Center is the short list",
      body: "Use this page for the things that actually need response now: mentions, replies, unread mail, and board movement.",
      actions: [
        { label: "Boards", href: "/boards?mode=mentions" },
        { label: "Mail", href: "/mail?box=unread" },
        { label: "Discover", href: "/discover" }
      ]
    },
    "/boards": {
      eyebrow: "Caller home base",
      title: "Boards are the long-form center of gravity",
      body: "Use boards for persistent discussion, unread scanning, and threaded replies. Pair this page with mail for private follow-up and radar for what changed.",
      actions: [
        { label: "Today", href: "/today" },
        { label: "Mail", href: "/mail" },
        { label: "Radar", href: "/radar" },
        { label: "Bulletins", href: "/bulletins" }
      ]
    },
    "/events": {
      eyebrow: "Scheduled return hooks",
      title: "Events turn the board into a place with a rhythm",
      body: "Use the calendar to make activity concrete: tournaments, nets, content drops, and social sessions should all have a time and a path to join.",
      actions: [
        { label: "Today", href: "/today" },
        { label: "Clubhouse", href: "/clubhouse" },
        { label: "Boards", href: "/boards" }
      ]
    },
    "/chat": {
      eyebrow: "Shared live chat",
      title: "Web chat and IRC are the same conversation layer",
      body: "Use this page for quick live interaction. The default room is #lobby, and IRC users see the same channel state.",
      actions: [
        { label: "Clubhouse", href: "/clubhouse" },
        { label: "Help", href: "/help" },
        { label: "Status", href: "/status" }
      ]
    },
    "/doors": {
      eyebrow: "Games and stickiness",
      title: "Doors keep callers coming back",
      body: "Use favorites, recommendations, and score links to turn the door list into a daily destination instead of a dead catalog.",
      actions: [
        { label: "Scores", href: "/scores" },
        { label: "Clubhouse", href: "/clubhouse" },
        { label: "Boards", href: "/boards" }
      ]
    },
    "/status": {
      eyebrow: "Health snapshot",
      title: "Use status as the fast confidence check",
      body: "This is the quick answer to whether the board looks healthy. For sysop detail, follow through to the WFC dashboard and setup pages.",
      actions: [
        { label: "Admin system", href: "/admin/system" },
        { label: "Admin setup", href: "/admin/setup" },
        { label: "Launch center", href: "/admin/launch" },
        { label: "Help", href: "/help" }
      ]
    },
    "/mail": {
      eyebrow: "Private conversation",
      title: "Use mail for direct follow-up",
      body: "Boards are public, mail is direct. This is the right place for operator feedback, replies, and caller-to-caller private messages.",
      actions: [
        { label: "Boards", href: "/boards" },
        { label: "Directory", href: "/directory" },
        { label: "Help", href: "/help" }
      ]
    },
    "/radar": {
      eyebrow: "Mission control",
      title: "Radar shows what changed since the last call",
      body: "Use this view when you want a single-screen snapshot of pulse, callers, recommended doors, and recent activity.",
      actions: [
        { label: "Boards", href: "/boards" },
        { label: "Chat", href: "/chat" },
        { label: "Clubhouse", href: "/clubhouse" }
      ]
    },
    "/clubhouse": {
      eyebrow: "Social layer",
      title: "Clubhouse is where the board feels alive",
      body: "Use one-liners, BBS exchange, and social presence here to keep momentum between longer board posts.",
      actions: [
        { label: "Chat", href: "/chat" },
        { label: "Bulletins", href: "/bulletins" },
        { label: "Directory", href: "/directory" }
      ]
    },
    "/admin/ops": {
      eyebrow: "Operator triage",
      title: "Ops Center consolidates the decision surface",
      body: "Use this page when you need sessions, audits, errors, and launch health in one place before deciding what action is justified.",
      actions: [
        { label: "Launch center", href: "/admin/launch" },
        { label: "Events admin", href: "/admin/events" },
        { label: "System", href: "/admin/system" },
        { label: "Audit", href: "/admin/audit" }
      ]
    },
    "/admin/events": {
      eyebrow: "Retention operations",
      title: "Schedule the board like a real product",
      body: "This page exists so recurring reasons to return are managed deliberately instead of getting buried in one-off announcements.",
      actions: [
        { label: "Public calendar", href: "/events" },
        { label: "Today", href: "/today" },
        { label: "Ops center", href: "/admin/ops" }
      ]
    }
  };
  function primerForPath(pathname) {
    if (primerRegistry[pathname]) return primerRegistry[pathname];
    if (pathname.startsWith("/admin/launch")) return primerRegistry["/admin/launch"];
    if (pathname.startsWith("/admin/setup")) return primerRegistry["/admin/setup"];
    if (pathname.startsWith("/admin/config")) return primerRegistry["/admin/config"];
    if (pathname.startsWith("/admin/ops")) return primerRegistry["/admin/ops"];
    if (pathname.startsWith("/admin/events")) return primerRegistry["/admin/events"];
    return null;
  }
  const navLinks = [];
  const navSeen = new Set();
  Array.from(document.querySelectorAll("a[href]")).forEach((anchor) => {
    const href = anchor.getAttribute("href");
    if (!href || href[0] !== "/" || href.startsWith("/chat/") || href.startsWith("/mail/inbound")) return;
    const key = href + "|" + anchor.textContent.trim().toLowerCase();
    if (navSeen.has(key)) return;
    navSeen.add(key);
    navLinks.push({
      href: href,
      label: anchor.textContent.trim() || href,
      meta: (anchor.closest("p") ? "nav" : "page")
    });
  });

  try {
    const recentKey = "wolfbbsRecentPages";
    const recent = JSON.parse(localStorage.getItem(recentKey) || "[]").filter((item) => item && item.href);
    const next = [{href: currentPath, label: title}].concat(recent.filter((item) => item.href !== currentPath)).slice(0, 6);
    localStorage.setItem(recentKey, JSON.stringify(next));
    if (next.length > 1) {
      const rail = document.createElement("div");
      rail.className = "wolfbbs-recent-rail";
      next.slice(1).forEach((item) => {
        const a = document.createElement("a");
        a.href = item.href;
        a.textContent = item.label;
        rail.appendChild(a);
      });
      const firstHeading = document.querySelector("h1");
      if (firstHeading && firstHeading.parentNode) {
        firstHeading.parentNode.insertBefore(rail, firstHeading.nextSibling);
      }
    }
  } catch (_) {}

  document.querySelectorAll('[data-wolfbbs-flash]').forEach((banner) => {
    if (banner.querySelector('.wolfbbs-banner-close')) return;
    const close = document.createElement('button');
    close.type = 'button';
    close.className = 'wolfbbs-banner-close';
    close.textContent = 'Dismiss';
    close.addEventListener('click', () => banner.remove());
    banner.appendChild(close);
  });

  function copyText(text) {
    const value = String(text || "");
    if (!value) return Promise.reject(new Error("empty"));
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(value);
    }
    const area = document.createElement("textarea");
    area.value = value;
    area.setAttribute("readonly", "readonly");
    area.style.position = "absolute";
    area.style.left = "-9999px";
    document.body.appendChild(area);
    area.select();
    try {
      document.execCommand("copy");
      document.body.removeChild(area);
      return Promise.resolve();
    } catch (err) {
      document.body.removeChild(area);
      return Promise.reject(err);
    }
  }

  document.addEventListener('click', (event) => {
    const trigger = event.target.closest('[data-copy-text]');
    if (!trigger) return;
    event.preventDefault();
    const text = trigger.getAttribute('data-copy-text') || '';
    const original = trigger.textContent;
    copyText(text).then(() => {
      trigger.textContent = 'Copied';
      window.setTimeout(() => {
        trigger.textContent = original;
      }, 1200);
    }).catch(() => {
      trigger.textContent = 'Copy failed';
      window.setTimeout(() => {
        trigger.textContent = original;
      }, 1200);
    });
  });

  document.querySelectorAll('pre').forEach((block) => {
    if (block.closest('.wolfbbs-command-block')) return;
    const wrapper = document.createElement('div');
    wrapper.className = 'wolfbbs-command-block';
    block.parentNode.insertBefore(wrapper, block);
    wrapper.appendChild(block);
    const copy = document.createElement('button');
    copy.type = 'button';
    copy.className = 'wolfbbs-copy-button';
    copy.textContent = 'Copy';
    copy.setAttribute('data-copy-text', block.textContent || '');
    wrapper.appendChild(copy);
  });

  document.querySelectorAll('form[data-filter-form]').forEach((form) => {
    const controls = Array.from(form.querySelectorAll('input[name], select[name]')).filter((control) => {
      const type = (control.getAttribute('type') || '').toLowerCase();
      if (type === 'hidden' || type === 'submit' || type === 'button') return false;
      if (control.disabled) return false;
      return true;
    });
    const active = controls.map((control) => {
      const value = (control.value || '').trim();
      if (!value) return null;
      const label = control.getAttribute('data-filter-label') || control.name.replace(/_/g, ' ');
      return { label, value };
    }).filter(Boolean);
    if (!active.length) return;
    const summary = document.createElement('div');
    summary.className = 'wolfbbs-active-filters';
    active.forEach((item) => {
      const chip = document.createElement('span');
      chip.className = 'wolfbbs-filter-chip';
      chip.textContent = item.label + ': ' + item.value;
      summary.appendChild(chip);
    });
    const reset = document.createElement('a');
    reset.className = 'wolfbbs-filter-reset';
    reset.href = form.getAttribute('data-filter-reset') || form.getAttribute('action') || location.pathname;
    reset.textContent = 'Reset filters';
    summary.appendChild(reset);
    form.insertAdjacentElement('afterend', summary);
  });

  const dirtyForms = new Set();
  function draftFields(form) {
    return Array.from(form.querySelectorAll('textarea[name], input[name], select[name]')).filter((field) => {
      const type = (field.getAttribute('type') || '').toLowerCase();
      if (type === 'hidden' || type === 'submit' || type === 'button' || type === 'checkbox' || type === 'radio' || type === 'password') return false;
      return !field.disabled;
    });
  }

  document.querySelectorAll('form[data-draft-key]').forEach((form) => {
    const key = 'wolfbbs:draft:' + location.pathname + ':' + form.getAttribute('data-draft-key');
    const fields = draftFields(form);
    if (!fields.length) return;
    const note = document.createElement('div');
    note.className = 'wolfbbs-form-note';
    note.innerHTML = '<strong>Drafts:</strong> <span>Saved locally in this browser while you type.</span>';
    const status = document.createElement('span');
    status.className = 'wolfbbs-form-status';
    status.textContent = 'Draft idle';
    note.appendChild(status);
    const discard = document.createElement('button');
    discard.type = 'button';
    discard.className = 'wolfbbs-form-secondary';
    discard.textContent = 'Discard draft';
    note.appendChild(discard);
    form.insertBefore(note, form.firstChild);

    let dirty = false;
    let saveTimer = null;

    function formatSavedAt(raw) {
      try {
        const when = new Date(raw);
        if (Number.isNaN(when.getTime())) return '';
        return when.toLocaleString();
      } catch (_) {
        return '';
      }
    }

    function markClean(message) {
      dirty = false;
      dirtyForms.delete(form);
      status.classList.remove('dirty', 'error');
      status.textContent = message || 'Draft idle';
    }

    function markDirty(message) {
      dirty = true;
      dirtyForms.add(form);
      status.classList.remove('error');
      status.classList.add('dirty');
      status.textContent = message || 'Unsaved draft changes';
    }

    function serialize() {
      const payload = {};
      fields.forEach((field) => {
        payload[field.name] = field.value || '';
      });
      return payload;
    }

    function saveDraft() {
      try {
        const savedAt = new Date().toISOString();
        localStorage.setItem(key, JSON.stringify({
          savedAt: new Date().toISOString(),
          values: serialize()
        }));
        const label = formatSavedAt(savedAt);
        markClean(label ? 'Draft saved ' + label : 'Draft saved locally');
      } catch (_) {
        status.classList.remove('dirty');
        status.classList.add('error');
        status.textContent = 'Draft storage unavailable';
      }
    }

    try {
      const raw = localStorage.getItem(key);
      if (raw) {
        const saved = JSON.parse(raw);
        if (saved && saved.values) {
          fields.forEach((field) => {
            if (!field.value && saved.values[field.name]) {
              field.value = saved.values[field.name];
            }
          });
          const label = formatSavedAt(saved.savedAt);
          status.textContent = label ? 'Draft restored from ' + label : 'Draft restored';
        }
      }
    } catch (_) {}

    fields.forEach((field) => {
      field.addEventListener('input', () => {
        markDirty();
        if (saveTimer) window.clearTimeout(saveTimer);
        saveTimer = window.setTimeout(saveDraft, 400);
      });
      field.addEventListener('change', () => {
        markDirty();
        if (saveTimer) window.clearTimeout(saveTimer);
        saveTimer = window.setTimeout(saveDraft, 250);
      });
    });

    discard.addEventListener('click', () => {
      try {
        localStorage.removeItem(key);
      } catch (_) {}
      fields.forEach((field) => {
        field.value = '';
      });
      markClean('Draft discarded');
    });

    form.addEventListener('submit', (event) => {
      if (event.defaultPrevented) return;
      try {
        localStorage.removeItem(key);
      } catch (_) {}
      markClean('Submitting');
    });
  });

  function escapeHTML(value) {
    return String(value || '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function composePreviewHTML(value) {
    const text = String(value || '').trim();
    if (!text) {
      return '<p class="wolfbbs-muted">Nothing to preview yet.</p>';
    }
    return text.split(/\n{2,}/).map((block) => {
      return '<p>' + escapeHTML(block).replace(/\n/g, '<br>') + '</p>';
    }).join('');
  }

  document.querySelectorAll('form[data-rich-compose]').forEach((form) => {
    if (form.querySelector('.wolfbbs-compose-toolbar')) return;
    const textarea = form.querySelector('textarea[name="body"]');
    if (!textarea) return;
    const layoutKey = 'wolfbbs:compose:layout:' + location.pathname + ':' + (form.getAttribute('data-draft-key') || form.getAttribute('data-rich-compose') || textarea.name || 'body');

    const shell = document.createElement('div');
    shell.className = 'wolfbbs-compose-shell';
    const label = textarea.parentElement && textarea.parentElement.tagName.toLowerCase() === 'label' ? textarea.parentElement : null;
    if (label && label.parentNode) {
      if (!textarea.id) {
        textarea.id = 'wolfbbs-compose-' + Math.random().toString(36).slice(2, 10);
      }
      label.setAttribute('for', textarea.id);
      label.parentNode.insertBefore(shell, label.nextSibling);
    } else {
      textarea.parentNode.insertBefore(shell, textarea);
    }
    shell.appendChild(textarea);

    const toolbar = document.createElement('div');
    toolbar.className = 'wolfbbs-compose-toolbar';
    shell.insertBefore(toolbar, textarea);

    const previewButton = document.createElement('button');
    previewButton.type = 'button';
    previewButton.textContent = 'Preview';
    toolbar.appendChild(previewButton);

    const focusButton = document.createElement('button');
    focusButton.type = 'button';
    focusButton.textContent = 'Focus Mode';
    toolbar.appendChild(focusButton);

    const fullscreenButton = document.createElement('button');
    fullscreenButton.type = 'button';
    fullscreenButton.textContent = 'Fullscreen';
    toolbar.appendChild(fullscreenButton);

    const helpButton = document.createElement('button');
    helpButton.type = 'button';
    helpButton.textContent = 'Shortcuts';
    toolbar.appendChild(helpButton);

    const quoteSource = textarea.getAttribute('data-compose-quote') || form.getAttribute('data-compose-quote') || '';
    if (quoteSource.trim()) {
      const quoteButton = document.createElement('button');
      quoteButton.type = 'button';
      quoteButton.textContent = 'Quote Context';
      toolbar.appendChild(quoteButton);
      quoteButton.addEventListener('click', () => {
        const quoteText = quoteSource.trim();
        if (!quoteText) return;
        const separator = textarea.value.trim() ? '\n\n' : '';
        textarea.value = (textarea.value || '') + separator + quoteText;
        textarea.dispatchEvent(new Event('input', { bubbles: true }));
        textarea.focus();
      });
    }

    const signatureValue = (form.getAttribute('data-compose-signature') || '').trim();
    if (signatureValue) {
      const signatureButton = document.createElement('button');
      signatureButton.type = 'button';
      signatureButton.textContent = 'Insert Signature';
      toolbar.appendChild(signatureButton);
      signatureButton.addEventListener('click', () => {
        const signature = '\n\n-- \n' + signatureValue;
        if ((textarea.value || '').includes(signature.trim())) return;
        textarea.value = (textarea.value || '') + signature;
        textarea.dispatchEvent(new Event('input', { bubbles: true }));
        textarea.focus();
      });
    }

    const meta = document.createElement('span');
    meta.className = 'wolfbbs-compose-meta';
    toolbar.appendChild(meta);

    const preview = document.createElement('div');
    preview.className = 'wolfbbs-compose-preview';
    shell.appendChild(preview);

    const help = document.createElement('div');
    help.className = 'wolfbbs-compose-help';
    help.innerHTML = '<strong>Composer Shortcuts</strong><ul><li>Ctrl/Cmd+Enter: send</li><li>Ctrl/Cmd+Shift+P: preview</li><li>Ctrl/Cmd+Shift+F: fullscreen</li><li>Esc: exit focus or fullscreen</li></ul>';
    shell.appendChild(help);

    function persistLayout() {
      try {
        localStorage.setItem(layoutKey, JSON.stringify({
          preview: preview.classList.contains('active'),
          focus: form.classList.contains('wolfbbs-compose-focus'),
          fullscreen: form.classList.contains('wolfbbs-compose-fullscreen')
        }));
      } catch (_) {}
    }

    function syncMeta() {
      const text = textarea.value || '';
      const words = text.trim() ? text.trim().split(/\s+/).length : 0;
      meta.textContent = words + ' words / ' + text.length + ' chars / Ctrl+Enter sends';
    }

    function syncPreview() {
      preview.innerHTML = composePreviewHTML(textarea.value);
      syncMeta();
    }

    function togglePreview(force) {
      if (typeof force === 'boolean') {
        preview.classList.toggle('active', force);
      } else {
        preview.classList.toggle('active');
      }
      const open = preview.classList.contains('active');
      previewButton.textContent = open ? 'Hide Preview' : 'Preview';
      if (open) {
        syncPreview();
      }
      persistLayout();
    }

    function toggleFocus(force) {
      if (typeof force === 'boolean') {
        form.classList.toggle('wolfbbs-compose-focus', force);
      } else {
        form.classList.toggle('wolfbbs-compose-focus');
      }
      const open = form.classList.contains('wolfbbs-compose-focus');
      focusButton.textContent = open ? 'Exit Focus' : 'Focus Mode';
      if (open) {
        textarea.focus();
      }
      persistLayout();
    }

    function toggleFullscreen(force) {
      if (typeof force === 'boolean') {
        form.classList.toggle('wolfbbs-compose-fullscreen', force);
      } else {
        form.classList.toggle('wolfbbs-compose-fullscreen');
      }
      const open = form.classList.contains('wolfbbs-compose-fullscreen');
      document.body.classList.toggle('wolfbbs-compose-fullscreen-open', open);
      fullscreenButton.textContent = open ? 'Exit Fullscreen' : 'Fullscreen';
      if (open) {
        textarea.focus();
      }
      persistLayout();
    }

    function toggleHelp(force) {
      if (typeof force === 'boolean') {
        help.classList.toggle('active', force);
      } else {
        help.classList.toggle('active');
      }
      helpButton.textContent = help.classList.contains('active') ? 'Hide Shortcuts' : 'Shortcuts';
    }

    previewButton.addEventListener('click', () => {
      togglePreview();
    });

    focusButton.addEventListener('click', () => {
      toggleFocus();
    });

    fullscreenButton.addEventListener('click', () => {
      toggleFullscreen();
    });

    helpButton.addEventListener('click', () => {
      toggleHelp();
    });

    textarea.addEventListener('input', syncPreview);
    textarea.addEventListener('change', syncPreview);
    textarea.addEventListener('keydown', (event) => {
      const modifier = event.ctrlKey || event.metaKey;
      if (modifier && event.key === 'Enter') {
        event.preventDefault();
        if (typeof form.requestSubmit === 'function') {
          form.requestSubmit();
        } else {
          form.submit();
        }
        return;
      }
      if (modifier && event.shiftKey && String(event.key).toLowerCase() === 'p') {
        event.preventDefault();
        togglePreview();
        return;
      }
      if (modifier && event.shiftKey && String(event.key).toLowerCase() === 'f') {
        event.preventDefault();
        toggleFullscreen();
        return;
      }
      if (event.key === 'Escape' && help.classList.contains('active')) {
        event.preventDefault();
        toggleHelp(false);
        return;
      }
      if (event.key === 'Escape' && form.classList.contains('wolfbbs-compose-fullscreen')) {
        event.preventDefault();
        toggleFullscreen(false);
        return;
      }
      if (event.key === 'Escape' && form.classList.contains('wolfbbs-compose-focus')) {
        event.preventDefault();
        toggleFocus(false);
      }
    });
    try {
      const raw = localStorage.getItem(layoutKey);
      if (raw) {
        const saved = JSON.parse(raw);
        if (saved && saved.preview) togglePreview(true);
        if (saved && saved.focus) toggleFocus(true);
        if (saved && saved.fullscreen) toggleFullscreen(true);
      }
    } catch (_) {}
    syncPreview();
  });

  window.addEventListener('beforeunload', (event) => {
    if (!dirtyForms.size) return;
    event.preventDefault();
    event.returnValue = '';
  });

  function inferredConfirmMessage(form, submitter) {
    const actionValue = submitter && submitter.getAttribute('value') ? submitter.getAttribute('value') : '';
    const hiddenAction = form.querySelector('input[name="action"]');
    const hiddenActionValue = hiddenAction && hiddenAction.value ? hiddenAction.value.toLowerCase() : '';
    const formAction = (form.getAttribute('action') || location.pathname || '').toLowerCase();
    const adminScoped = formAction.indexOf('/admin') === 0 || formAction.includes('/admin/');
    const text = [
      submitter && submitter.textContent,
      actionValue,
      hiddenActionValue,
      form.getAttribute('data-confirm')
    ].join(' ').toLowerCase();
    if (!text.trim()) return '';
    if (submitter && submitter.getAttribute('data-confirm')) return submitter.getAttribute('data-confirm');
    if (text.includes('delete') && adminScoped) return 'Delete this item? This cannot be undone.';
    if (text.includes('reset password') && (adminScoped || hiddenActionValue === 'reset')) return 'Reset this password and replace the current one?';
    if (text.includes('ban') && adminScoped) return 'Ban this account now?';
    if (text.includes('disable') && adminScoped) return 'Disable this item now?';
    if (text.includes('remove') && adminScoped) return 'Remove this item now?';
    if (text.includes('lock') && adminScoped) return 'Apply this lock now?';
    return '';
  }

  document.addEventListener('submit', (event) => {
    const form = event.target;
    if (!form || form.tagName.toLowerCase() !== 'form') return;
    const submitter = event.submitter || form.querySelector('button[type="submit"], input[type="submit"]');
    const confirmMessage = inferredConfirmMessage(form, submitter);
    if (confirmMessage && !window.confirm(confirmMessage)) {
      event.preventDefault();
      return;
    }
    const method = (form.getAttribute('method') || 'get').toLowerCase();
    if (method !== 'post') return;
    const buttons = Array.from(form.querySelectorAll('button[type="submit"], input[type="submit"]'));
    buttons.forEach((button) => {
      if (button === submitter) {
        button.setAttribute('data-original-label', button.textContent || button.value || '');
        if (button.tagName.toLowerCase() === 'input') {
          button.value = 'Working...';
        } else {
          button.textContent = 'Working...';
        }
      }
      button.disabled = true;
      button.classList.add('wolfbbs-submit-busy');
    });
  }, true);

  const primer = primerForPath(location.pathname);
  if (primer) {
    const panel = document.createElement("section");
    panel.className = "wolfbbs-primer wolfbbs-card";
    const eyebrow = document.createElement("span");
    eyebrow.className = "wolfbbs-primer-eyebrow";
    eyebrow.textContent = primer.eyebrow || "Guide";
    panel.appendChild(eyebrow);
    const heading = document.createElement("strong");
    heading.className = "wolfbbs-primer-title";
    heading.textContent = primer.title || title;
    panel.appendChild(heading);
    if (primer.body) {
      const body = document.createElement("p");
      body.textContent = primer.body;
      panel.appendChild(body);
    }
    if (Array.isArray(primer.bullets) && primer.bullets.length) {
      const list = document.createElement("ul");
      primer.bullets.forEach((item) => {
        const li = document.createElement("li");
        li.textContent = item;
        list.appendChild(li);
      });
      panel.appendChild(list);
    }
    if (Array.isArray(primer.actions) && primer.actions.length) {
      const actions = document.createElement("div");
      actions.className = "wolfbbs-primer-actions";
      primer.actions.forEach((item) => {
        const link = document.createElement("a");
        link.href = item.href;
        link.textContent = item.label;
        actions.appendChild(link);
      });
      panel.appendChild(actions);
    }
    const h1 = document.querySelector("h1");
    const recentRail = document.querySelector(".wolfbbs-recent-rail");
    if (recentRail && recentRail.parentNode) {
      recentRail.parentNode.insertBefore(panel, recentRail.nextSibling);
    } else if (h1 && h1.parentNode) {
      h1.parentNode.insertBefore(panel, h1.nextSibling);
    }
  }

  const headings = Array.from(document.querySelectorAll("h2, h3"));
  if (headings.length >= 2) {
    const nav = document.createElement("div");
    nav.className = "wolfbbs-section-nav";
    headings.forEach((heading, idx) => {
      if (!heading.id) heading.id = "wolfbbs-section-" + idx;
      const link = document.createElement("a");
      link.href = "#" + heading.id;
      link.textContent = heading.textContent.trim();
      nav.appendChild(link);
    });
    const h1 = document.querySelector("h1");
    if (h1 && h1.parentNode) {
      h1.parentNode.insertBefore(nav, h1.nextSibling ? h1.nextSibling.nextSibling : null);
    }
  }

  const firstTable = document.querySelector("table");
  if (firstTable && firstTable.querySelectorAll("tr").length >= 6) {
    const box = document.createElement("div");
    box.className = "wolfbbs-inline-filter";
    const label = document.createElement("label");
    label.textContent = "Filter this page";
    const input = document.createElement("input");
    input.type = "search";
    input.placeholder = "type to filter visible rows";
    label.appendChild(input);
    box.appendChild(label);
    firstTable.parentNode.insertBefore(box, firstTable);
    input.addEventListener("input", () => {
      const q = input.value.trim().toLowerCase();
      Array.from(document.querySelectorAll("table")).forEach((table) => {
        const rows = Array.from(table.querySelectorAll("tr"));
        rows.forEach((row, index) => {
          if (index === 0) return;
          row.style.display = !q || row.textContent.toLowerCase().includes(q) ? "" : "none";
        });
      });
    });
  }

  const overlay = document.createElement("div");
  overlay.id = "wolfbbsPaletteOverlay";
  overlay.innerHTML = '<div id="wolfbbsPalette"><div id="wolfbbsPaletteHeader"><label style="display:block;margin:0"><span class="wolfbbs-muted">Command palette</span><input id="wolfbbsPaletteInput" type="search" placeholder="jump to boards, doors, status, admin..." style="width:100%;margin-top:8px"></label></div><div id="wolfbbsPaletteList"></div></div>';
  document.body.appendChild(overlay);

  const paletteButton = document.createElement("button");
  paletteButton.id = "wolfbbsCommandButton";
  paletteButton.type = "button";
  paletteButton.textContent = "Jump / Search";
  document.body.appendChild(paletteButton);

  const paletteInput = overlay.querySelector("#wolfbbsPaletteInput");
  const paletteList = overlay.querySelector("#wolfbbsPaletteList");
  const commands = navLinks.concat(headings.map((heading) => ({
    href: "#" + heading.id,
    label: heading.textContent.trim(),
    meta: "section"
  })));

  function renderPalette(query) {
    const q = (query || "").trim().toLowerCase();
    paletteList.innerHTML = "";
    commands
      .filter((item) => !q || item.label.toLowerCase().includes(q) || item.href.toLowerCase().includes(q))
      .slice(0, 16)
      .forEach((item) => {
        const link = document.createElement("a");
        link.className = "wolfbbs-palette-item";
        link.href = item.href;
        link.innerHTML = "<strong>" + item.label + "</strong><span class=\"wolfbbs-palette-meta\">" + item.meta + " • " + item.href + "</span>";
        paletteList.appendChild(link);
      });
    if (!paletteList.children.length) {
      const empty = document.createElement("div");
      empty.className = "wolfbbs-palette-item";
      empty.innerHTML = "<strong>No matches</strong><span class=\"wolfbbs-palette-meta\">Try boards, doors, chat, admin, status.</span>";
      paletteList.appendChild(empty);
    }
  }

  function openPalette() {
    overlay.classList.add("active");
    renderPalette("");
    paletteInput.value = "";
    window.setTimeout(() => paletteInput.focus(), 10);
  }

  function closePalette() {
    overlay.classList.remove("active");
  }

  paletteButton.addEventListener("click", openPalette);
  paletteInput.addEventListener("input", () => renderPalette(paletteInput.value));
  overlay.addEventListener("click", (event) => {
    if (event.target === overlay) closePalette();
  });
    document.addEventListener("keydown", (event) => {
    const tag = event.target && event.target.tagName ? event.target.tagName.toLowerCase() : "";
    const editing = tag === "input" || tag === "textarea" || tag === "select" || event.target.isContentEditable;
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
      event.preventDefault();
      openPalette();
      return;
    }
    if (event.key === "Escape" && overlay.classList.contains("active")) {
      event.preventDefault();
      closePalette();
      return;
    }
    if (!editing && event.key === "?") {
      event.preventDefault();
      openPalette();
    }
  });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initWolfbbsModernUI, { once: true });
    return;
  }
  initWolfbbsModernUI();
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
		applyCommonSecurityHeaders(w)
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

func applyCommonSecurityHeaders(w http.ResponseWriter) {
	headers := w.Header()
	if headers.Get("X-Content-Type-Options") == "" {
		headers.Set("X-Content-Type-Options", "nosniff")
	}
	if headers.Get("X-Frame-Options") == "" {
		headers.Set("X-Frame-Options", "DENY")
	}
	if headers.Get("Referrer-Policy") == "" {
		headers.Set("Referrer-Policy", "same-origin")
	}
	if headers.Get("Permissions-Policy") == "" {
		headers.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	}
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
	if base := normalizedPublicURL(a.apBaseURL); base != "" {
		return base
	}
	if base := normalizedPublicURL(a.publicBaseURL); base != "" {
		return base
	}
	scheme := "http"
	if a != nil && a.secureCookie {
		scheme = "https"
	}
	if r != nil && (r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")) {
		scheme = "https"
	}
	return scheme + "://" + sanitizedConfiguredHost(a.siteHost())
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
	base := normalizedPublicURL(a.publicBaseURL)
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
	if user, ok := a.currentUser(r); ok {
		http.Redirect(w, r, a.preferredHomeRoute(user), http.StatusFound)
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
	termBlock += `<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/xterm@5.5.0/css/xterm.min.css">
<div id="termHost" style="max-width:820px; margin-top:10px;">
<div id="xterm" style="height:360px; width:100%; border:1px solid #334; border-radius:8px; overflow:hidden;"></div>
<div id="termStatus" style="margin-top:8px; color:#666; font-size:12px;">connecting...</div>
</div>
<script src="https://cdn.jsdelivr.net/npm/xterm@5.5.0/lib/xterm.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/xterm-addon-fit@0.10.0/lib/xterm-addon-fit.min.js"></script>
<script>
(function(){
const host = document.getElementById('xterm');
const status = document.getElementById('termStatus');
const wsURL = ` + fmt.Sprintf("%q", wsURL) + `;
function createFallbackTerminal(container) {
  container.innerHTML = "";
  const view = document.createElement('pre');
  view.id = 'xterm-fallback';
  view.tabIndex = 0;
  view.setAttribute('aria-label', 'Web terminal');
  view.style.margin = '0';
  view.style.height = '100%';
  view.style.padding = '12px';
  view.style.overflowY = 'auto';
  view.style.whiteSpace = 'pre-wrap';
  view.style.outline = 'none';
  view.style.background = '#0b0f14';
  view.style.color = '#b7f7c1';
  view.style.font = "14px/1.45 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace";
  container.appendChild(view);
  let onData = function(){};
  view.addEventListener('keydown', function(evt) {
    if (evt.metaKey || evt.ctrlKey || evt.altKey) {
      return;
    }
    if (evt.key === 'Enter') {
      evt.preventDefault();
      onData('\r');
      return;
    }
    if (evt.key === 'Backspace') {
      evt.preventDefault();
      onData('\b');
      return;
    }
    if (evt.key === 'Tab') {
      evt.preventDefault();
      onData('\t');
      return;
    }
    if (evt.key.length === 1) {
      evt.preventDefault();
      onData(evt.key);
    }
  });
  return {
    loadAddon: function(){},
    open: function(){},
    focus: function(){ view.focus(); },
    write: function(text){
      view.textContent += String(text || '');
      view.scrollTop = view.scrollHeight;
    },
    writeln: function(text){
      view.textContent += String(text || '') + '\n';
      view.scrollTop = view.scrollHeight;
    },
    onData: function(handler){
      onData = handler || function(){};
    }
  };
}

const term = window.Terminal ? new window.Terminal({
  cursorBlink: true,
  convertEol: true,
  fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace",
  fontSize: 14,
  theme: {
    background: "#0b0f14",
    foreground: "#b7f7c1",
    cursor: "#f4f4f4",
    selectionBackground: "#334455"
  },
  scrollback: 3000
}) : createFallbackTerminal(host);
const fitAddon = window.Terminal && window.FitAddon && window.FitAddon.FitAddon ? new window.FitAddon.FitAddon() : null;
if (fitAddon) {
  term.loadAddon(fitAddon);
}
if (window.Terminal) {
  term.open(host);
  if (fitAddon) {
    fitAddon.fit();
  }
}
term.focus();
let ws = null;
let reconnectTimer = null;
let reconnectMs = 1000;
let connected = false;
let passwordMode = false;
let pendingFrames = [];

function setStatus(text){
  status.textContent = text;
}

function setOnlineState(isOnline){
  connected = isOnline;
  setStatus(isOnline ? "connected" : "disconnected");
}

function clearReconnect(){
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
}

function scheduleReconnect(reason){
  clearReconnect();
  setOnlineState(false);
  setStatus("disconnected: " + reason + " (retrying in " + Math.round(reconnectMs / 1000) + "s)");
  reconnectTimer = setTimeout(connect, reconnectMs);
  reconnectMs = Math.min(reconnectMs * 2, 10000);
}

function queueFrame(frame){
  pendingFrames.push(frame);
}

function sendFrame(type, data){
  const frame = JSON.stringify({t: type, d: data || ""});
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    queueFrame(frame);
    return false;
  }
  ws.send(frame);
  return true;
}

function flushPendingFrames(){
  if (!ws || ws.readyState !== WebSocket.OPEN || pendingFrames.length === 0) {
    return;
  }
  const frames = pendingFrames.slice();
  pendingFrames = [];
  for (let i = 0; i < frames.length; i++) {
    ws.send(frames[i]);
  }
}

function writeServer(chunk){
  const text = String(chunk || "").replace(/\r\n/g, "\n").replace(/\r/g, "\n");
  const lines = text.split("\n");
  for (let i = 0; i < lines.length; i++) {
    term.writeln(lines[i]);
  }
  const tail = lines.length > 0 ? lines[lines.length - 1].toLowerCase() : "";
  if (tail.includes("password:") || tail.includes("2fa")) {
    passwordMode = true;
  }
  if (tail.includes("enter selection:") || tail.includes("login successful") || tail.includes("login failed")) {
    if (!tail.includes("password")) {
      passwordMode = false;
    }
  }
}

function connect(){
  clearReconnect();
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
    return;
  }
  ws = new WebSocket(wsURL);
  ws.onopen = function(){
    reconnectMs = 1000;
    setOnlineState(true);
    term.writeln("[connected] " + wsURL);
    flushPendingFrames();
    term.focus();
  };
  ws.onmessage = function(evt){ writeServer(evt.data); };
  ws.onclose = function(){ scheduleReconnect("socket closed"); };
  ws.onerror = function(){ setStatus("socket error"); };
}

term.onData(function(data){
  if (!data) return;
  for (const ch of data) {
    const code = ch.charCodeAt(0);
    if (ch === "\r") {
      term.write("\r\n");
      if (!sendFrame("key", "\n") && !connected) connect();
      continue;
    }
    if (code === 127 || ch === "\b") {
      term.write("\b \b");
      if (!sendFrame("key", "\b") && !connected) connect();
      continue;
    }
    if (code < 32 && ch !== "\t") {
      continue;
    }
    if (passwordMode && code >= 32) {
      term.write("*");
    } else {
      term.write(ch);
    }
    if (!sendFrame("key", ch) && !connected) connect();
  }
});

host.addEventListener('click', function(){ term.focus(); });
document.addEventListener('visibilitychange', function(){
  if (document.visibilityState === "visible") {
    if (!ws || ws.readyState !== WebSocket.OPEN) connect();
    term.focus();
  }
});
window.addEventListener('focus', function(){
  if (!ws || ws.readyState !== WebSocket.OPEN) connect();
  term.focus();
});

window.addEventListener('resize', function(){
  if (fitAddon) {
    fitAddon.fit();
  }
});

setOnlineState(false);
connect();
setInterval(function(){
  if (ws && ws.readyState === WebSocket.OPEN) {
    sendFrame("ping", "");
  }
}, 25000);
setTimeout(function(){ term.focus(); }, 0);
})();
</script>`

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
	ircPort := parseInt(strings.TrimSpace(os.Getenv("WOLFBBS_IRC_PORT")), 6667)
	sshCommand := `ssh ` + connectHost + ` -p ` + strconv.Itoa(sshPort)
	telnetCommand := `telnet ` + connectHost + ` ` + strconv.Itoa(telnetPort)
	ircCommand := `irc://` + connectHost + `:` + strconv.Itoa(ircPort) + `/%23lobby`

	page := `<html><body>
<h1>` + htmlEscape(a.siteDisplayName()) + ` Connect</h1>
<p>Terminal-first remains the primary UX.</p>
` + motdBlock + `
` + announcementBlock + `
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>SSH</strong><span>primary caller path</span></article>
<article class="wolfbbs-kpi-card"><strong>Web</strong><span>browser terminal + setup</span></article>
<article class="wolfbbs-kpi-card"><strong>IRC</strong><span>live lobby access</span></article>
</section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Clipboard-Friendly Commands</h2><ul class="wolfbbs-list-clean">
<li><strong>SSH (recommended)</strong><br><code>` + htmlEscape(sshCommand) + `</code> <button type="button" class="wolfbbs-form-secondary" data-copy-text="` + htmlEscape(sshCommand) + `">Copy</button></li>
<li><strong>Telnet (optional)</strong><br><code>` + htmlEscape(telnetCommand) + `</code> <button type="button" class="wolfbbs-form-secondary" data-copy-text="` + htmlEscape(telnetCommand) + `">Copy</button></li>
<li><strong>IRC lobby</strong><br><code>` + htmlEscape(ircCommand) + `</code> <button type="button" class="wolfbbs-form-secondary" data-copy-text="` + htmlEscape(ircCommand) + `">Copy</button></li>
<li><strong>WebSocket login endpoint</strong><br><code>` + htmlEscape(wsURL) + `</code> <button type="button" class="wolfbbs-form-secondary" data-copy-text="` + htmlEscape(wsURL) + `">Copy</button></li>
</ul></article>
<article class="wolfbbs-card"><h2>Connection Sanity</h2><ul class="wolfbbs-list-clean">
<li><strong>Host:</strong> ` + htmlEscape(connectHost) + `</li>
<li><strong>SSH port:</strong> ` + strconv.Itoa(sshPort) + `</li>
<li><strong>Telnet port:</strong> ` + strconv.Itoa(telnetPort) + `</li>
<li><strong>Web terminal:</strong> reconnects automatically if the browser tab comes back into focus.</li>
</ul></article>
</section>
<h2>Choose your client</h2>
<table border="1">
<tr><th>Surface</th><th>Best for</th><th>Why pick it</th></tr>
<tr><td>SSH</td><td>real callers</td><td>The full ANSI board feel with menus, mail, files, and doors.</td></tr>
<tr><td>Web terminal</td><td>browser users</td><td>No terminal client required; good for quick access and testing.</td></tr>
<tr><td>IRC</td><td>chat regulars</td><td>Same live chat layer as the web UI, but in an IRC client.</td></tr>
</table>
<section class="wolfbbs-helper-grid">
<article class="wolfbbs-helper-card"><strong>Use SSH if you care about the full experience</strong><p>SSH is still the highest-fidelity path for ANSI menus, doors, and the classic board flow.</p></article>
<article class="wolfbbs-helper-card"><strong>Use the web terminal for zero-install access</strong><p>This is the fastest way to test logins, menus, and redraw behavior from a browser.</p></article>
<article class="wolfbbs-helper-card"><strong>Use IRC if chat is your entry point</strong><p>The lobby is the same live conversation layer seen in the web chat surface.</p></article>
</section>
<h2>First call checklist</h2>
<ol>
<li>Connect with SSH or the web terminal.</li>
<li>Read the MOTD and announcement.</li>
<li>Open boards, chat, and doors once so the main surfaces are familiar.</li>
<li>If you are just exploring, use the guided tour first.</li>
</ol>
<h2>If the web terminal looks stuck</h2>
<ul>
<li>Click inside the terminal to restore focus.</li>
<li>Switch tabs and come back; the browser terminal will reconnect automatically.</li>
<li>Use SSH if you want the most reliable full-screen ANSI behavior.</li>
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
<h2>How to become a caller</h2>
<ol>
<li>Create or use an account from <a href="/login">/login</a>.</li>
<li>Use <a href="/connect">/connect</a> for SSH, web terminal, or IRC details.</li>
<li>After login, start with boards, chat, and doors.</li>
</ol>
<h2>Why people come back</h2>
<ul>
<li>Boards keep the long-form community memory.</li>
<li>Chat and IRC provide the live social loop.</li>
<li>Doors and scores add the classic repeat-visit hook.</li>
</ul>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Cache-Control", "no-store")
		if user, ok := a.currentUser(r); ok {
			if user != nil && a.hasRole(user, roleAdmin) && strings.HasPrefix(r.URL.Path, "/admin") {
				http.Redirect(w, r, "/admin", http.StatusFound)
			} else {
				http.Redirect(w, r, a.preferredHomeRoute(user), http.StatusFound)
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
	loginKey := a.rateLimitKey("login", r)
	if !a.allowRateLimitedAction(loginKey, a.loginRateLimit, a.loginRateWindow, false) {
		http.Error(w, "too many login attempts", http.StatusTooManyRequests)
		return
	}
	handle := strings.TrimSpace(r.FormValue("handle"))
	password := strings.TrimSpace(r.FormValue("password"))
	totp := strings.TrimSpace(r.FormValue("totp"))

	user, err := a.authSvc.Authenticate(handle, password, totp)
	if err != nil {
		_ = a.allowRateLimitedAction(loginKey, a.loginRateLimit, a.loginRateWindow, true)
		if err == auth.ErrMissingSecondFactor || err == auth.ErrInvalidSecondFactor {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("invalid 2FA code"))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid credentials"))
		return
	}
	a.clearRateLimitedAction(loginKey)

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
	} else {
		redirectTo = a.preferredHomeRoute(user)
	}
	http.Redirect(w, r, redirectTo, http.StatusFound)
}

func (a *webApp) handlePasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resetRequestPage(a.siteDisplayName(), "")))
		return
	case http.MethodPost:
		resetKey := a.rateLimitKey("reset", r)
		if !a.allowRateLimitedAction(resetKey, a.resetRateLimit, a.resetRateWindow, true) {
			http.Error(w, "too many reset requests", http.StatusTooManyRequests)
			return
		}
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
		w.Header().Set("Cache-Control", "no-store")
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
	nav := `<a href="/start">start</a> | <a href="/login">login</a> | <a href="/connect">connect</a>`
	roleGuideTitle := "If you're visiting for the first time"
	roleGuide := `<ol>` +
		`<li>Start with <a href="/start">/start</a>, <a href="/connect">/connect</a>, or <a href="/tour">/tour</a> to understand the board before signing in.</li>` +
		`<li>Use <a href="/help">/help</a> to learn the route map and caller surface layout.</li>` +
		`<li>When you want the real experience, sign in and try SSH plus <a href="/boards">/boards</a> and <a href="/chat">/chat</a>.</li>` +
		`</ol>`
	if user != nil {
		roleLabel = rbac.NormalizeRole(user.Role)
		nav = `<a href="/start">start</a> | <a href="/today">today</a> | <a href="/attention">attention</a> | <a href="/events">events</a> | <a href="/boards">boards</a> | <a href="/bulletins">bulletins</a> | <a href="/directory">directory</a> | <a href="/finder">finder</a> | <a href="/newfiles">newfiles</a> | <a href="/feedback">feedback</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/radar">radar</a> | <a href="/clubhouse">clubhouse</a> | <a href="/doors">doors</a> | <a href="/settings">settings</a> | <a href="/status">status</a> | <a href="/config">config</a>`
		if a.discover {
			nav += ` | <a href="/discover">discover</a>`
		}
		if a.hasRole(user, roleAdmin) {
			nav += ` | <a href="/admin">admin</a>`
			roleGuideTitle = "If you're the sysop"
			roleGuide = `<ol>` +
				`<li>Start with <a href="/start">/start</a>, then finish <a href="/admin/setup">/admin/setup</a> before treating the board as ready for callers.</li>` +
				`<li>Use <a href="/admin/ops">/admin/ops</a> as the fast operator triage surface for errors, sessions, and audits.</li>` +
				`<li>Use <a href="/admin/events">/admin/events</a> to schedule concrete reasons for callers to return.</li>` +
				`<li>Review <a href="/admin/config">/admin/config</a> for runtime flags, identity, and exposed services.</li>` +
				`<li>Seed boards, create a non-sysop account in <a href="/admin/users">/admin/users</a>, then test <a href="/boards">/boards</a>, <a href="/chat">/chat</a>, <a href="/doors">/doors</a>, and SSH.</li>` +
				`<li>Use <a href="/status">/status</a> and <a href="/admin/system">/admin/system</a> as the daily health view.</li>` +
				`</ol>`
		} else {
			roleGuideTitle = "If you're a caller"
			roleGuide = `<ol>` +
				`<li>Start with <a href="/today">/today</a> and <a href="/attention">/attention</a> when you want a fast answer to what matters next.</li>` +
				`<li>Use <a href="/boards">/boards</a> for long-form discussion, <a href="/chat">/chat</a> for live conversation, and <a href="/doors">/doors</a> for game and score surfaces.</li>` +
				`<li>Use <a href="/events">/events</a> to see tournaments, social calls, and scheduled board activity.</li>` +
				`<li>Use <a href="/mail">/mail</a> for private conversation and <a href="/directory">/directory</a> to find other callers.</li>` +
				`<li>Try SSH when you want the full ANSI board experience.</li>` +
				`</ol>`
		}
		nav += ` | <a href="/logout">logout</a>`
	}
	discoverItem := ""
	if a.discover {
		discoverItem = `<li>/discover for since-your-last-call scanning and saved search flow</li>`
	}
	sysopSection := ""
	if user != nil && a.hasRole(user, roleAdmin) {
		sysopSection = `<h2>Common sysop jobs</h2>
<ul>
<li>Operator triage: <a href="/admin/ops">/admin/ops</a></li>
<li>First-run setup: <a href="/admin/setup">/admin/setup</a> then <a href="/admin/config">/admin/config</a></li>
<li>User and role management: <a href="/admin/users">/admin/users</a></li>
<li>Service and runtime health: <a href="/status">/status</a>, <a href="/admin/system">/admin/system</a>, <a href="/admin/errors">/admin/errors</a></li>
<li>Policy surfaces: <a href="/admin/chat">/admin/chat</a>, <a href="/admin/doors">/admin/doors</a>, <a href="/admin/files">/admin/files</a></li>
</ul>`
	}
	launchPlan := `<h2>10-minute launch plan</h2>
<ol>
<li>Start with <a href="/admin/setup">/admin/setup</a> if you run the board, or <a href="/connect">/connect</a> if you are just exploring.</li>
<li>Use <a href="/status">/status</a> to confirm the product surfaces you care about are actually present.</li>
<li>Walk one real caller path: <a href="/boards">/boards</a>, <a href="/chat">/chat</a>, <a href="/doors">/doors</a>, then SSH.</li>
<li>If anything feels off, stop guessing and use <code>bash install.sh --status</code>, <code>--doctor</code>, or <code>--repair</code>.</li>
</ol>`
	troubleMatrix := `<h2>If something feels broken</h2>
<table border="1">
<tr><th>Symptom</th><th>Where to look first</th><th>Practical next move</th></tr>
<tr><td>I cannot tell what to do after install</td><td><a href="/admin/launch">/admin/launch</a>, <code>docs/START_HERE.md</code></td><td>Use Launch Center first, then finish the setup steps in order.</td></tr>
<tr><td>The board feels empty</td><td><a href="/admin/setup?step=4">/admin/setup?step=4</a>, <a href="/boards">/boards</a></td><td>Seed default boards, post a starter message, and create a caller account.</td></tr>
<tr><td>Chat or IRC seems wrong</td><td><a href="/chat">/chat</a>, <a href="/admin/chat">/admin/chat</a>, <a href="/status">/status</a></td><td>Verify <code>#lobby</code>, moderation state, and bridge health before inviting users.</td></tr>
<tr><td>Browser routes work but launch still feels risky</td><td><a href="/status">/status</a>, <a href="/admin/system">/admin/system</a></td><td>Use the readiness views and fix warnings before you announce the board.</td></tr>
<tr><td>I need operator docs fast</td><td><code>docs/LAUNCH_CHECKLIST.md</code>, <code>docs/OPERATOR_PLAYBOOK.md</code></td><td>Use the checklist for go-live order, then the playbook when you need to know which screen or command to use next.</td></tr>
</table>`

	page := `<html><body>
<h1>` + htmlEscape(a.siteDisplayName()) + ` Help</h1>
<p>` + nav + `</p>
<p>Current role: ` + htmlEscape(roleLabel) + `</p>
<h2>` + roleGuideTitle + `</h2>
` + roleGuide + `
` + launchPlan + `
<h2>Use the right surface</h2>
<table border="1">
<tr><th>Surface</th><th>Best for</th><th>Why it exists</th></tr>
<tr><td>SSH / ANSI</td><td>callers and nostalgic operators</td><td>The full board feel: menus, boards, mail, files, doors, and classic flow.</td></tr>
<tr><td>Web companion</td><td>everyday users and browser-first callers</td><td>Boards, chat, directory, scores, and setup without a terminal client.</td></tr>
<tr><td>IRC</td><td>existing chat communities</td><td>Shares the same live chat layer as the web UI.</td></tr>
<tr><td>Admin web</td><td>sysops and moderators</td><td>Setup, config, users, health, runtime policy, and audit.</td></tr>
</table>
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
<li>/start, /today, /attention, /events, /boards, /bulletins, /directory, /finder, /newfiles, /feedback, /mail, /chat, /radar, /clubhouse, /doors, /settings, /gateway, /status, /config</li>
<li>/start for the fastest guest/caller/sysop handoff into the right lane</li>
<li>/today for the daily brief: watched boards, upcoming events, and the shortest responsible next step</li>
<li>/attention for direct follow-up, unread mail, and board movement that actually needs response</li>
<li>/events for the public community calendar and scheduled return hooks</li>
<li>/bulletins for system wire, hot boards, download pick, and classic bulletin-reading flow</li>
<li>/directory for caller lookup, caller cards, and direct compose links</li>
<li>/finder for cross-board search and thread tracker</li>
<li>/newfiles for recent uploads, queue desk, and top-rated file picks</li>
<li>/feedback for classic mail-to-sysop feedback flow</li>
<li>/radar for mission control: board pulse, live callers, discovery queue, and arcade heat</li>
<li>/clubhouse for one-liner posting, rumors, BBS exchange, and social presence</li>
<li>/doors for favorites, recommendations, recents, and policy-aware door directory</li>
<li>/scores for global door leaderboards</li>
` + discoverItem + `
<li>/healthz, /readyz, /metrics, /statusz for health/ops checks</li>
</ul>
<h2>First-run verification path</h2>
<ol>
<li>Open <a href="/admin/setup">/admin/setup</a> if you are the sysop.</li>
<li>Open <a href="/boards">/boards</a> and make sure seeded or starter content exists.</li>
<li>Open <a href="/chat">/chat</a> and send a message in <code>#lobby</code>.</li>
<li>Open <a href="/doors">/doors</a> and <a href="/scores">/scores</a> to verify game and score surfaces.</li>
<li>Check <a href="/status">/status</a> or <a href="/admin/system">/admin/system</a> before inviting users.</li>
</ol>
` + troubleMatrix + `
	<h2>Admin routes (sysop only)</h2>
	<ul>
	<li>/admin/ops, /admin/events, /admin/users, /admin/boards, /admin/mail, /admin/files, /admin/gateways</li>
	<li>/admin/chat, /admin/doors, /admin/setup, /admin/config, /admin/system, /admin/errors, /admin/audit</li>
	<li>Setup wizard path: /admin/setup?step=1 (Identity), step=2 (Safety), step=3 (Experience), step=4 (Bootstrap)</li>
	<li>Runtime service settings (telnet/ws/wss/content/connectors): /admin/config</li>
	</ul>
` + sysopSection + `
<h2>Reference docs</h2>
<ul>
<li><code>docs/START_HERE.md</code></li>
<li><code>docs/LAUNCH_CHECKLIST.md</code></li>
<li><code>docs/OPERATOR_PLAYBOOK.md</code></li>
<li><code>docs/TROUBLESHOOTING.md</code></li>
<li><code>docs/OPERATIONS.md</code></li>
<li><code>docs/help-guides.md</code></li>
<li><code>docs/INSTALL.md</code></li>
<li><code>docs/PRODUCT_GUIDE.md</code></li>
<li><code>docs/config-reference.md</code></li>
<li><code>docs/feature-reference.md</code></li>
</ul>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleBulletins(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	snapshot := a.buildBulletinSnapshot(user)
	systemRows := strings.Builder{}
	for _, row := range snapshot.SystemWire {
		systemRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	if systemRows.Len() == 0 {
		systemRows.WriteString(`<li>No system bulletins are active.</li>`)
	}
	digestRows := strings.Builder{}
	for _, row := range snapshot.DigestItems {
		digestRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	if digestRows.Len() == 0 {
		digestRows.WriteString(`<li>No newscan highlights right now.</li>`)
	}
	hotBoardRows := strings.Builder{}
	for _, row := range snapshot.HotBoards {
		hotBoardRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	if hotBoardRows.Len() == 0 {
		hotBoardRows.WriteString(`<li>No board pulse data yet.</li>`)
	}
	callerRows := strings.Builder{}
	for _, row := range snapshot.RecentCallers {
		callerRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	if callerRows.Len() == 0 {
		callerRows.WriteString(`<li>No recent callers yet.</li>`)
	}
	oneLinerRows := strings.Builder{}
	for _, row := range snapshot.OneLiners {
		oneLinerRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	if oneLinerRows.Len() == 0 {
		oneLinerRows.WriteString(`<li>No one-liners yet.</li>`)
	}
	fileRows := strings.Builder{}
	for _, row := range snapshot.RecentFiles {
		fileRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	if fileRows.Len() == 0 {
		fileRows.WriteString(`<li>No new files indexed yet.</li>`)
	}
	featuredThread := snapshot.FeaturedThread
	if strings.TrimSpace(featuredThread) == "" {
		featuredThread = "No featured thread yet."
	}
	downloadPick := snapshot.DownloadPick
	if strings.TrimSpace(downloadPick) == "" {
		downloadPick = "No download pick yet."
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Bulletin Center</title></head><body>
<p><a href="/boards">boards</a> | <a href="/directory">directory</a> | <a href="/finder">finder</a> | <a href="/newfiles">newfiles</a> | <a href="/feedback">feedback</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/doors">doors</a> | <a href="/status">status</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Bulletin Center</h1>
<p>Classic bulletin-reading, rebuilt as a live dashboard. Start here for system wire, hot boards, file picks, and tonight's pulse.</p>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>System Wire</h2><ul>` + systemRows.String() + `</ul></article>
<article class="wolfbbs-card"><h2>Spotlight</h2><p><strong>Featured thread:</strong> ` + htmlEscape(featuredThread) + `</p><p><strong>Download pick:</strong> ` + htmlEscape(downloadPick) + `</p></article>
</section>
<section class="wolfbbs-grid">
<article><h2>Hot Board Pulse</h2><ul>` + hotBoardRows.String() + `</ul><p><a href="/finder">Open finder</a></p></article>
<article><h2>Newscan Headlines</h2><ul>` + digestRows.String() + `</ul><p><a href="/discover">Open discover</a></p></article>
</section>
<section class="wolfbbs-grid">
<article><h2>Recent Callers</h2><ul>` + callerRows.String() + `</ul></article>
<article><h2>OneLinerz Wall</h2><ul>` + oneLinerRows.String() + `</ul></article>
<article><h2>New Files</h2><ul>` + fileRows.String() + `</ul><p><a href="/newfiles">Open new files desk</a></p></article>
</section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleDirectory(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	onlineOnly := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("online")), "1")
	verifiedFilter := normalizeDirectoryVerifiedFilter(r.URL.Query().Get("verified"))
	roleFilter := normalizeDirectoryRoleFilter(r.URL.Query().Get("role"))
	targetHandle := strings.TrimSpace(r.URL.Query().Get("handle"))
	if targetHandle == "" {
		targetHandle = user.Handle
	}
	rows := a.buildDirectoryRows(query, onlineOnly, verifiedFilter, roleFilter)
	onlineCount := 0
	verifiedCount := 0
	staffCount := 0
	for _, row := range rows {
		if row.Online {
			onlineCount++
		}
		if row.Verified {
			verifiedCount++
		}
		if roleWeight[rbac.NormalizeRole(row.Role)] >= roleWeight[roleModerator] {
			staffCount++
		}
	}
	var profile *directoryProfile
	if targetHandle != "" {
		if target, err := a.authSvc.GetUser(targetHandle); err == nil && target != nil {
			profile = a.buildDirectoryProfile(user, target)
		}
	}
	profileBlock := ``
	if profile != nil {
		recentRows := strings.Builder{}
		for _, row := range profile.RecentCallerRows {
			recentRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
		}
		if recentRows.Len() == 0 {
			recentRows.WriteString(`<li>No caller history rows for this handle yet.</li>`)
		}
		profileBlock = `<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Caller Card: ` + htmlEscape(profile.Handle) + `</h2><p><strong>Role:</strong> ` + htmlEscape(profile.Role) + ` | <strong>Theme:</strong> ` + htmlEscape(profile.Theme) + ` | <strong>Verified:</strong> ` + boolToText(profile.Verified) + `</p><p><strong>Last login:</strong> ` + htmlEscape(profile.LastLogin) + `</p><p><strong>Online now:</strong> ` + boolToText(profile.Online) + ``
		if profile.Online {
			profileBlock += ` in ` + htmlEscape(profile.OnlineArea) + ` from ` + htmlEscape(profile.OnlineOrigin)
		}
		profileBlock += `</p><p><a href="/mail?to=` + url.QueryEscape(profile.Handle) + `">Send mail</a> | <a href="/finder?q=` + url.QueryEscape(profile.Handle) + `">Search posts</a></p></article>
<article class="wolfbbs-card"><h2>Caller Stats</h2><ul><li>Posts: ` + strconv.Itoa(profile.Posts) + `</li><li>Mentions: ` + strconv.Itoa(profile.Mentions) + `</li><li>Replies: ` + strconv.Itoa(profile.Replies) + `</li><li>Mail sent: ` + strconv.Itoa(profile.MailSent) + `</li><li>Mail received: ` + strconv.Itoa(profile.MailReceived) + `</li><li>Favorite door: ` + htmlEscape(profile.FavoriteDoor) + `</li><li>Achievements: ` + strconv.Itoa(profile.Achievements) + `</li></ul></article>
<article class="wolfbbs-card"><h2>Recent Calls</h2><ul>` + recentRows.String() + `</ul></article>
</section>`
	}
	tableRows := strings.Builder{}
	for _, row := range rows {
		tableRows.WriteString(`<tr><td><a href="/directory?handle=` + url.QueryEscape(row.Handle) + `">` + htmlEscape(row.Handle) + `</a></td><td>` + htmlEscape(row.Role) + `</td><td>` + boolToText(row.Verified) + `</td><td>` + htmlEscape(row.Theme) + `</td><td>` + htmlEscape(row.LastLogin) + `</td><td>` + boolToText(row.Online) + `</td><td>` + htmlEscape(row.Area) + `</td><td>` + htmlEscape(row.Origin) + `</td><td><a href="/mail?to=` + url.QueryEscape(row.Handle) + `">mail</a></td></tr>`)
	}
	if tableRows.Len() == 0 {
		tableRows.WriteString(`<tr><td colspan="9">No callers matched the filter.</td></tr>`)
	}
	roleOptions := []string{"any", roleUser, roleModerator, roleAdmin}
	roleOptionRows := strings.Builder{}
	for _, row := range roleOptions {
		selected := ""
		if row == roleFilter {
			selected = ` selected`
		}
		label := row
		if row == "any" {
			label = "any role"
		}
		roleOptionRows.WriteString(`<option value="` + htmlEscape(row) + `"` + selected + `>` + htmlEscape(label) + `</option>`)
	}
	verifiedOptions := []string{"any", "verified", "unverified"}
	verifiedOptionRows := strings.Builder{}
	for _, row := range verifiedOptions {
		selected := ""
		if row == verifiedFilter {
			selected = ` selected`
		}
		label := row
		if row == "any" {
			label = "any verification"
		}
		verifiedOptionRows.WriteString(`<option value="` + htmlEscape(row) + `"` + selected + `>` + htmlEscape(label) + `</option>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Caller Directory</title></head><body>
<p><a href="/boards">boards</a> | <a href="/bulletins">bulletins</a> | <a href="/finder">finder</a> | <a href="/feedback">feedback</a> | <a href="/mail">mail</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Caller Directory</h1>
<p>Classic userlist, rebuilt with live presence, caller cards, and direct compose links.</p>
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(rows)) + `</strong><span>visible callers</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(onlineCount) + `</strong><span>online now</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(verifiedCount) + `</strong><span>verified</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(staffCount) + `</strong><span>staff in view</span></article>
</section>
<form method="GET" action="/directory" class="wolfbbs-inline-form"><label>Search <input name="q" value="` + htmlEscape(query) + `" placeholder="handle, role, theme"></label><label>Role <select name="role">` + roleOptionRows.String() + `</select></label><label>Verified <select name="verified">` + verifiedOptionRows.String() + `</select></label><label><input type="checkbox" name="online" value="1"`
	if onlineOnly {
		page += ` checked`
	}
	page += `> online only</label><button type="submit">Filter</button></form>
` + profileBlock + `
<table border="1"><tr><th>Handle</th><th>Role</th><th>Verified</th><th>Theme</th><th>Last Login</th><th>Online</th><th>Area</th><th>Origin</th><th>Action</th></tr>` + tableRows.String() + `</table>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleFeedback(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	sysop := a.primarySysopUser()
	if sysop == nil {
		http.Error(w, "sysop mailbox unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodPost {
		if !a.requireCSRF(w, r) {
			return
		}
		category := strings.TrimSpace(r.FormValue("category"))
		subject := strings.TrimSpace(r.FormValue("subject"))
		body := strings.TrimSpace(r.FormValue("body"))
		if subject == "" || body == "" {
			redirectWithError(w, r, "/feedback", "Subject and body are required.")
			return
		}
		if category == "" {
			category = "general"
		}
		fullSubject := "[feedback/" + cleanOneLiner(strings.ToLower(category), 16) + "] " + cleanOneLiner(subject, 72)
		if err := a.mailRepo.CreateMail(&domain.PrivateMail{
			FromUserID: user.ID,
			ToUserID:   sysop.ID,
			Subject:    fullSubject,
			Body:       body,
		}); err != nil {
			redirectWithError(w, r, "/feedback", "Could not send feedback mail.")
			return
		}
		if a.eventBus != nil {
			a.eventBus.Publish("feedback.sent", map[string]string{"from": user.Handle, "to": sysop.Handle, "category": category})
		}
		redirectWithNotice(w, r, "/feedback", "Feedback delivered to "+sysop.Handle+".")
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	csrf := a.csrfHiddenInput(r)
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Feedback to Sysop</title></head><body>
<p><a href="/boards">boards</a> | <a href="/bulletins">bulletins</a> | <a href="/directory">directory</a> | <a href="/finder">finder</a> | <a href="/mail">mail</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Feedback to Sysop</h1>
<p>Classic feedback module, rebuilt as direct internal mail to <strong>` + htmlEscape(sysop.Handle) + `</strong>.</p>
<form method="POST" action="/feedback" data-draft-key="feedback-compose" data-rich-compose="feedback-compose" data-compose-signature="` + htmlEscape(user.Handle) + `">
` + csrf + `
<label>Category <select name="category"><option value="bug">bug</option><option value="idea">idea</option><option value="abuse">abuse</option><option value="praise">praise</option><option value="general" selected>general</option></select></label><br>
<label>Subject <input name="subject" size="64" placeholder="What should the sysop know?"></label><br>
<label>Body<br><textarea name="body" rows="12" cols="80" placeholder="Describe the issue, request, or old-school rant."></textarea></label><br>
<button type="submit">Send Feedback</button>
</form>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleFinder(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	boardFilterID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("board")), 10, 64)
	authorFilter := strings.TrimSpace(r.URL.Query().Get("author"))
	trackerFilter := normalizeTrackerFilter(r.URL.Query().Get("tracker"))
	if strings.TrimSpace(r.URL.Query().Get("save")) == "1" && query != "" && a.classicSearch {
		a.addSavedSearch(user.Handle, query)
	}
	results := a.searchMessageHits(user, query, boardFilterID, authorFilter, 30)
	tracker := a.buildThreadTracker(user, trackerFilter, 12)
	saved := a.savedSearchList(user.Handle)
	boardOptions := a.visibleBoardsFor(user)
	matchedBoards := map[int64]struct{}{}
	resultRows := strings.Builder{}
	for _, row := range results {
		matchedBoards[row.BoardID] = struct{}{}
		resultRows.WriteString(`<tr><td><a href="/boards?board=` + strconv.FormatInt(row.BoardID, 10) + `&id=` + strconv.FormatInt(row.MessageID, 10) + `">` + htmlEscape(row.Subject) + `</a></td><td>` + htmlEscape(row.BoardName) + `</td><td>` + htmlEscape(row.Conference) + `</td><td>` + htmlEscape(row.Author) + `</td><td>` + htmlEscape(row.CreatedAt) + `</td><td>` + htmlEscape(row.Snippet) + `</td></tr>`)
	}
	if resultRows.Len() == 0 {
		resultRows.WriteString(`<tr><td colspan="6">No matches yet.</td></tr>`)
	}
	trackerRows := strings.Builder{}
	for _, row := range tracker {
		label := row.Kind
		switch row.Kind {
		case "post":
			label = "your post"
		case "mention":
			label = "mention"
		case "reply":
			label = "reply to you"
		}
		trackerRows.WriteString(`<li><strong>` + htmlEscape(label) + `:</strong> <a href="/boards?board=` + strconv.FormatInt(row.BoardID, 10) + `&id=` + strconv.FormatInt(row.MessageID, 10) + `">` + htmlEscape(row.BoardName) + ` / ` + htmlEscape(row.Subject) + `</a> <span class="wolfbbs-muted">` + htmlEscape(row.CreatedAt) + `</span></li>`)
	}
	if trackerRows.Len() == 0 {
		trackerRows.WriteString(`<li>No tracked thread activity yet.</li>`)
	}
	savedRows := strings.Builder{}
	for _, row := range saved {
		savedRows.WriteString(`<li><a href="/finder?q=` + url.QueryEscape(row) + `">` + htmlEscape(row) + `</a></li>`)
	}
	if savedRows.Len() == 0 {
		savedRows.WriteString(`<li>No saved finder queries yet.</li>`)
	}
	boardOptionRows := strings.Builder{}
	boardOptionRows.WriteString(`<option value="">All boards</option>`)
	for _, board := range boardOptions {
		selected := ""
		if board.ID == boardFilterID {
			selected = ` selected`
		}
		boardOptionRows.WriteString(`<option value="` + strconv.FormatInt(board.ID, 10) + `"` + selected + `>` + htmlEscape(board.Name) + `</option>`)
	}
	trackerOptions := []string{"all", "post", "mention", "reply"}
	trackerOptionRows := strings.Builder{}
	for _, row := range trackerOptions {
		selected := ""
		if row == trackerFilter {
			selected = ` selected`
		}
		label := row
		if row == "all" {
			label = "all tracker items"
		}
		if row == "post" {
			label = "your posts"
		}
		if row == "reply" {
			label = "replies to you"
		}
		trackerOptionRows.WriteString(`<option value="` + htmlEscape(row) + `"` + selected + `>` + htmlEscape(label) + `</option>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Message Finder</title></head><body>
<p><a href="/boards">boards</a> | <a href="/bulletins">bulletins</a> | <a href="/directory">directory</a> | <a href="/newfiles">newfiles</a> | <a href="/feedback">feedback</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Message Finder</h1>
<p>Cross-board search plus a personal thread tracker for replies, mentions, and your recent posts.</p>
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(results)) + `</strong><span>matches</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(matchedBoards)) + `</strong><span>boards touched</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(tracker)) + `</strong><span>tracker items</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(saved)) + `</strong><span>saved queries</span></article>
</section>
<form method="GET" action="/finder" class="wolfbbs-inline-form"><label>Query <input name="q" value="` + htmlEscape(query) + `" placeholder="subject or text"></label><label>Board <select name="board">` + boardOptionRows.String() + `</select></label><label>Author <input name="author" value="` + htmlEscape(authorFilter) + `" placeholder="handle"></label><label>Tracker <select name="tracker">` + trackerOptionRows.String() + `</select></label><button type="submit">Search</button><button type="submit" name="save" value="1">Save Query</button></form>
<section class="wolfbbs-grid">
<article><h2>Thread Tracker</h2><ul>` + trackerRows.String() + `</ul></article>
<article><h2>Saved Queries</h2><ul>` + savedRows.String() + `</ul></article>
</section>
<h2>Results</h2>
<table border="1"><tr><th>Subject</th><th>Board</th><th>Conf</th><th>Author</th><th>When</th><th>Snippet</th></tr>` + resultRows.String() + `</table>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleNewFiles(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if !a.canReadFiles(user, "browse") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	sinceFilter := normalizeFileSinceFilter(r.URL.Query().Get("since"))
	tagFilter := strings.TrimSpace(r.URL.Query().Get("tag"))
	sortMode := normalizeFileSortMode(r.URL.Query().Get("sort"))
	snapshot := a.buildNewFilesSnapshot(user, sinceFilter, tagFilter, sortMode)
	csrf := a.csrfHiddenInput(r)
	areaIDs := map[int64]struct{}{}
	recentRows := strings.Builder{}
	for _, row := range snapshot.RecentUploads {
		areaIDs[row.AreaID] = struct{}{}
		recentRows.WriteString(`<tr><td>` + htmlEscape(snapshot.AreaNames[row.AreaID]) + `</td><td>` + htmlEscape(row.Name) + `</td><td>` + htmlEscape(strings.Join(row.Tags, ",")) + `</td><td>` + row.UploadedAt.Local().Format("2006-01-02 15:04") + `</td><td>` + fmt.Sprintf("%.2f", row.RatingAvg) + ` (` + strconv.Itoa(row.RatingCount) + `)</td><td><form method="POST" action="/gateway">` + csrf + `<input type="hidden" name="return_to" value="/newfiles"><input type="hidden" name="action" value="queue_add"><input type="hidden" name="file_id" value="` + strconv.FormatInt(row.ID, 10) + `"><button type="submit">queue</button></form></td></tr>`)
	}
	if recentRows.Len() == 0 {
		recentRows.WriteString(`<tr><td colspan="6">No recent uploads yet.</td></tr>`)
	}
	topRows := strings.Builder{}
	for _, row := range snapshot.TopRated {
		topRows.WriteString(`<li><strong>` + htmlEscape(row.Name) + `</strong> in ` + htmlEscape(snapshot.AreaNames[row.AreaID]) + ` <span class="wolfbbs-muted">rating ` + fmt.Sprintf("%.2f", row.RatingAvg) + ` (` + strconv.Itoa(row.RatingCount) + `)</span></li>`)
	}
	if topRows.Len() == 0 {
		topRows.WriteString(`<li>No rated uploads yet.</li>`)
	}
	filterRows := strings.Builder{}
	for _, row := range snapshot.SavedFilters {
		target := `/newfiles`
		params := url.Values{}
		if len(row.Tags) > 0 {
			params.Set("tag", row.Tags[0])
		}
		if params.Encode() != "" {
			target += `?` + params.Encode()
		}
		filterRows.WriteString(`<li><a href="` + htmlEscape(target) + `">` + htmlEscape(row.Name) + `</a> - ` + htmlEscape(row.Query) + ` [` + htmlEscape(strings.Join(row.Tags, ",")) + `]</li>`)
	}
	if filterRows.Len() == 0 {
		filterRows.WriteString(`<li>No saved file filters yet.</li>`)
	}
	queueRows := strings.Builder{}
	for _, row := range snapshot.Queue {
		name := snapshot.QueueNames[row.FileID]
		if name == "" {
			name = "file #" + strconv.FormatInt(row.FileID, 10)
		}
		queueRows.WriteString(`<tr><td>` + htmlEscape(name) + `</td><td>` + row.CreatedAt.Local().Format("2006-01-02 15:04") + `</td><td><form method="POST" action="/gateway">` + csrf + `<input type="hidden" name="return_to" value="/newfiles"><input type="hidden" name="action" value="queue_remove"><input type="hidden" name="file_id" value="` + strconv.FormatInt(row.FileID, 10) + `"><button type="submit">remove</button></form></td></tr>`)
	}
	if queueRows.Len() == 0 {
		queueRows.WriteString(`<tr><td colspan="3">Queue is empty.</td></tr>`)
	}
	emptyFilesHelper := ``
	if len(snapshot.RecentUploads) == 0 && len(snapshot.TopRated) == 0 {
		emptyFilesHelper = a.renderRoleAwareEmptyState(user, "files")
	}
	sinceOptionRows := strings.Builder{}
	for _, row := range []string{"24h", "7d", "30d", "all"} {
		selected := ""
		if row == sinceFilter {
			selected = ` selected`
		}
		label := row
		if row == "24h" {
			label = "last 24 hours"
		}
		if row == "7d" {
			label = "last 7 days"
		}
		if row == "30d" {
			label = "last 30 days"
		}
		if row == "all" {
			label = "all uploads"
		}
		sinceOptionRows.WriteString(`<option value="` + htmlEscape(row) + `"` + selected + `>` + htmlEscape(label) + `</option>`)
	}
	sortOptionRows := strings.Builder{}
	for _, row := range []string{"latest", "rating", "name"} {
		selected := ""
		if row == sortMode {
			selected = ` selected`
		}
		label := row
		if row == "latest" {
			label = "latest first"
		}
		if row == "rating" {
			label = "top rated"
		}
		if row == "name" {
			label = "name"
		}
		sortOptionRows.WriteString(`<option value="` + htmlEscape(row) + `"` + selected + `>` + htmlEscape(label) + `</option>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>New Files Desk</title></head><body>
<p><a href="/boards">boards</a> | <a href="/bulletins">bulletins</a> | <a href="/finder">finder</a> | <a href="/directory">directory</a> | <a href="/gateway?view=files">full filebase</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>New Files Desk</h1>
<p>Modern new-files scan with queue management, saved filters, and top-rated picks.</p>
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(snapshot.RecentUploads)) + `</strong><span>visible uploads</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(areaIDs)) + `</strong><span>areas represented</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(snapshot.Queue)) + `</strong><span>queued downloads</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(snapshot.SavedFilters)) + `</strong><span>saved filters</span></article>
</section>
<form method="GET" action="/newfiles" class="wolfbbs-inline-form"><label>Window <select name="since">` + sinceOptionRows.String() + `</select></label><label>Tag <input name="tag" value="` + htmlEscape(tagFilter) + `" placeholder="zip, ansi, docs"></label><label>Sort <select name="sort">` + sortOptionRows.String() + `</select></label><button type="submit">Filter</button></form>
` + emptyFilesHelper + `
<section class="wolfbbs-grid">
<article><h2>Top Rated Picks</h2><ul>` + topRows.String() + `</ul></article>
<article><h2>Saved Filters</h2><ul>` + filterRows.String() + `</ul><p><a href="/gateway?view=files">Open full FileBase browser</a></p></article>
</section>
<h2>Recent Uploads</h2>
<table border="1"><tr><th>Area</th><th>Name</th><th>Tags</th><th>Uploaded</th><th>Rating</th><th>Action</th></tr>` + recentRows.String() + `</table>
<h2>Download Desk</h2>
<p><a href="/gateway?view=files&batch=1">Download queue as ZIP</a></p>
<table border="1"><tr><th>File</th><th>Queued</th><th>Action</th></tr>` + queueRows.String() + `</table>
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
		case "watch", "digest", "mute", "unwatch", "subscribe":
			if boardID <= 0 {
				redirectWithError(w, r, "/boards", "Board ID is required.")
				return
			}
			board, err := a.boardRepo.Get(boardID)
			if err != nil || board == nil {
				redirectWithError(w, r, "/boards", "Board not found.")
				return
			}
			if !a.canReadBoard(user, board) {
				http.Error(w, "watch denied by board ACS", http.StatusForbidden)
				return
			}
			mode := boardSubscriptionNone
			switch action {
			case "watch":
				mode = boardSubscriptionWatch
			case "digest":
				mode = boardSubscriptionDigest
			case "mute":
				mode = boardSubscriptionMute
			case "subscribe":
				mode = normalizeBoardSubscriptionMode(r.FormValue("subscription_mode"))
			}
			a.setBoardSubscription(user.Handle, boardID, mode)
			notice := "Board subscription cleared."
			if mode != boardSubscriptionNone {
				notice = "Board set to " + boardSubscriptionLabel(mode) + "."
			}
			redirectWithNotice(w, r, boardPath, notice)
			return
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
		boardQuery := strings.TrimSpace(r.URL.Query().Get("q"))
		boardMode := normalizeBoardMode(r.URL.Query().Get("mode"))
		subscriptions := a.boardSubscriptions(user.Handle)
		csrf := a.csrfHiddenInput(r)
		rows := strings.Builder{}
		messageBlock := pageMessageBlock(r)
		dashboard := a.buildBoardsDashboard(user, boards)
		boardRows, boardQueue := a.buildBoardMenuRows(user, boards, boardQuery, boardMode)
		if boardMode == "watched" || boardMode == "digest" || boardMode == "muted" {
			filteredRows := make([]boardMenuRow, 0, len(boardRows))
			for _, row := range boardRows {
				mode := subscriptions[row.Board.ID]
				if boardMode == "watched" && mode == boardSubscriptionWatch {
					filteredRows = append(filteredRows, row)
				}
				if boardMode == "digest" && mode == boardSubscriptionDigest {
					filteredRows = append(filteredRows, row)
				}
				if boardMode == "muted" && mode == boardSubscriptionMute {
					filteredRows = append(filteredRows, row)
				}
			}
			boardRows = filteredRows
		}
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
		recommendedDoorBlock := `<span class="wolfbbs-muted">No recommended door yet.</span>`
		if dashboard.RecommendedDoor != "" {
			recommendedDoorBlock = `<a href="/doors?mode=recommended">` + htmlEscape(dashboard.RecommendedDoor) + `</a>`
		}
		recentCallersBlock := strings.Builder{}
		for _, row := range dashboard.RecentCallers {
			recentCallersBlock.WriteString(`<li>` + htmlEscape(row) + `</li>`)
		}
		if recentCallersBlock.Len() == 0 {
			recentCallersBlock.WriteString(`<li>No recent callers yet.</li>`)
		}
		oneLinerBlock := strings.Builder{}
		for _, row := range dashboard.OneLiners {
			oneLinerBlock.WriteString(`<li>` + htmlEscape(row) + `</li>`)
		}
		if oneLinerBlock.Len() == 0 {
			oneLinerBlock.WriteString(`<li>No one-liners yet.</li>`)
		}
		subscriptionStats := a.boardSubscriptionStats(user.Handle)
		subscriptionSummary := strings.Builder{}
		for _, stat := range subscriptionStats {
			modePath := "all"
			switch stat.Mode {
			case boardSubscriptionWatch:
				modePath = "watched"
			case boardSubscriptionDigest:
				modePath = "digest"
			case boardSubscriptionMute:
				modePath = "muted"
			}
			subscriptionSummary.WriteString(`<li><a href="/boards?mode=` + htmlEscape(modePath) + `">` + htmlEscape(strings.Title(stat.Label)) + `</a>: ` + strconv.Itoa(stat.Count) + `</li>`)
		}
		for _, row := range boardRows {
			currentMode := normalizeBoardSubscriptionMode(string(subscriptions[row.Board.ID]))
			rows.WriteString(`<tr><td>` + strconv.FormatInt(row.Board.ID, 10) + `</td><td><a href="/boards?board=` + strconv.FormatInt(row.Board.ID, 10) + `">` + htmlEscape(row.Board.Name) + `</a></td><td>` + htmlEscape(defaultConferenceValue(row.Board.Conference)) + `</td><td>` + strconv.Itoa(row.MessageCount) + `</td><td>` + strconv.Itoa(row.NewCount) + `</td><td>` + strconv.Itoa(row.MyPosts) + `</td><td>` + strconv.Itoa(row.Mentions) + `</td><td>` + htmlEscape(row.LastAt) + `</td><td>` + htmlEscape(row.LastSubject) + `</td><td><form method="POST" action="/boards" class="wolfbbs-inline-actions"><input type="hidden" name="action" value="subscribe"><input type="hidden" name="board_id" value="` + strconv.FormatInt(row.Board.ID, 10) + `">` + csrf + `<label class="wolfbbs-muted">tier <select name="subscription_mode">` + boardSubscriptionOptionRows(currentMode) + `</select></label><button type="submit">Save</button></form></td></tr>`)
		}
		emptyBoardHelper := ""
		if rows.Len() == 0 {
			rows.WriteString(`<tr><td colspan="10">No boards matched the current filters.</td></tr>`)
			emptyBoardHelper = a.renderRoleAwareEmptyState(user, "boards")
			if a.hasRole(user, roleAdmin) {
				emptyBoardHelper = `<article class="wolfbbs-card"><h2>Board Launch Tip</h2><p>The board list is empty from the caller point of view. That usually means setup is not finished, content has not been seeded, or the current filters are too narrow.</p><p><a href="/admin/launch">Launch Center</a> | <a href="/admin/setup?step=4">Seed Default Boards</a> | <a href="/admin/boards">Board Admin</a></p></article>`
			}
		}
		quickJumpBlock := ""
		if a.quickJump {
			quickJumpBlock = `<form method="GET" action="/boards"><label>Quick Jump <input name="jump" size="24" placeholder="bulletins/directory/finder/newfiles/feedback"></label><button type="submit">Go</button></form>`
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
		modeOptions := []string{"all", "unread", "mine", "mentions", "watched", "digest", "muted"}
		modeOptionRows := strings.Builder{}
		for _, row := range modeOptions {
			selected := ""
			if row == boardMode {
				selected = ` selected`
			}
			label := row
			if row == "all" {
				label = "all boards"
			}
			modeOptionRows.WriteString(`<option value="` + htmlEscape(row) + `"` + selected + `>` + htmlEscape(label) + `</option>`)
		}
		filterItems := make([]string, 0, 3)
		if boardQuery != "" {
			filterItems = append(filterItems, `search "`+boardQuery+`"`)
		}
		if conferenceFilter != "" {
			filterItems = append(filterItems, "conference "+conferenceFilter)
		}
		if boardMode != "" && boardMode != "all" {
			filterItems = append(filterItems, "mode "+boardMode)
		}
		filterSummary := renderActiveFilterPanel("Active Board Filters", "/boards", filterItems)
		confFilterBlock := `<form method="GET" action="/boards" class="wolfbbs-inline-form" data-filter-form="boards" data-filter-reset="/boards"><label>Search <input name="q" value="` + htmlEscape(boardQuery) + `" placeholder="board, description, subject" data-filter-label="search"></label><label>Conference <select name="conference" data-filter-label="conference">` + confOptions.String() + `</select></label><label>Mode <select name="mode" data-filter-label="mode">` + modeOptionRows.String() + `</select></label><button type="submit">Filter</button></form>`
		scanHelperBlock := `<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Unread scan</strong><p>Use mode=unread to work through the boards that changed since your last call.</p></article><article class="wolfbbs-helper-card"><strong>Subscription tiers</strong><p>Watch escalates into Attention Center, digest stays in Today Brief, and mute removes a board from routine loops without deleting access.</p></article><article class="wolfbbs-helper-card"><strong>Conference narrowing</strong><p>Use the conference filter when the board list is broad and you need to triage a single area fast.</p></article></section>`
		discoverActionCard := `<a class="wolfbbs-action-card" href="/discover"><strong>Discover</strong><span>Catch up since last call</span></a>`
		if !a.discover {
			discoverActionCard = `<article class="wolfbbs-action-card"><strong>Discover</strong><span>Disabled by current feature flags</span></article>`
		}
		rumorBlock := ``
		if a.rumorzMod != nil {
			if rumor := strings.TrimSpace(a.rumorzMod.Current()); rumor != "" {
				rumorBlock = `<p><strong>Rumorz:</strong> ` + htmlEscape(rumor) + `</p>`
			}
		}
		dashboardBlock := `<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(dashboard.VisibleBoards) + `</strong><span>visible boards</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(dashboard.UnreadPosts) + `</strong><span>unread posts</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(dashboard.UnreadMail) + `</strong><span>unread mail</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(dashboard.OnlineUsers) + `</strong><span>chat online</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(dashboard.FavoriteDoors) + `</strong><span>favorite doors</span></article>
</section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Caller Cockpit</h2><p>Recommended door: ` + recommendedDoorBlock + `</p>` + rumorBlock + `<div class="wolfbbs-action-grid"><a class="wolfbbs-action-card" href="/today"><strong>Today Brief</strong><span>queue, watched boards, calendar</span></a><a class="wolfbbs-action-card" href="/mail"><strong>Inbox</strong><span>` + strconv.Itoa(dashboard.UnreadMail) + ` unread mail waiting</span></a>` + discoverActionCard + `<a class="wolfbbs-action-card" href="/doors"><strong>Door Cockpit</strong><span>Favorites, turns, trophies, policy</span></a><a class="wolfbbs-action-card" href="/events"><strong>Community Calendar</strong><span>scheduled return hooks and events</span></a><a class="wolfbbs-action-card" href="/radar"><strong>Caller Radar</strong><span>Board pulse, live callers, arcade heat</span></a><a class="wolfbbs-action-card" href="/clubhouse"><strong>Clubhouse</strong><span>One-liners, rumors, BBS exchange</span></a><a class="wolfbbs-action-card" href="/chat"><strong>Lobby Chat</strong><span>` + strconv.Itoa(dashboard.OnlineUsers) + ` callers online</span></a></div></article>
<article class="wolfbbs-card"><h2>Last Callers</h2><ul>` + recentCallersBlock.String() + `</ul></article>
<article class="wolfbbs-card"><h2>OneLinerz</h2><ul>` + oneLinerBlock.String() + `</ul></article>
</section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Legacy Classics</h2><div class="wolfbbs-action-grid"><a class="wolfbbs-action-card" href="/bulletins"><strong>Bulletin Center</strong><span>system wire, hot boards, download pick</span></a><a class="wolfbbs-action-card" href="/directory"><strong>Caller Directory</strong><span>user list, profile cards, direct mail links</span></a><a class="wolfbbs-action-card" href="/finder"><strong>Message Finder</strong><span>cross-board search and thread tracker</span></a><a class="wolfbbs-action-card" href="/newfiles"><strong>New Files Desk</strong><span>recent uploads, top-rated files, queue</span></a><a class="wolfbbs-action-card" href="/feedback"><strong>Feedback to Sysop</strong><span>classic feedback module, rebuilt</span></a></div></article>
<article class="wolfbbs-card"><h2>Personal Board Queue</h2><div class="wolfbbs-grid"><section><h3>Unread Scan</h3>` + boardQueueList(boardQueue.UnreadRows, "Unread queue is clear.") + `</section><section><h3>Your Threads</h3>` + boardQueueList(boardQueue.MyRows, "No personal threads tracked yet.") + `</section><section><h3>Mentions</h3>` + boardQueueList(boardQueue.MentionRows, "No mentions waiting.") + `</section><section><h3>Subscription Tiers</h3><ul>` + subscriptionSummary.String() + `</ul></section></div></article>
</section>`
		page := fmt.Sprintf(`<html><body>
<p>Signed in as %s</p>
<p><a href="/start">start</a> | <a href="/today">today</a> | <a href="/attention">attention</a> | <a href="/events">events</a> | <a href="/bulletins">bulletins</a> | <a href="/directory">directory</a> | <a href="/finder">finder</a> | <a href="/newfiles">newfiles</a> | <a href="/feedback">feedback</a> | <a href="/mail">mail</a> | <a href="/settings">settings</a> | <a href="/chat">chat</a> | <a href="/radar">radar</a> | <a href="/clubhouse">clubhouse</a> | <a href="/doors">doors</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/gateway">gateway</a>%s | <a href="/help">help</a> | <a href="/logout">logout</a></p>
%s
%s
%s
%s
%s
%s
%s
%s
%s
<p><strong>Tip:</strong> Select a board to read, then open a message ID to reply/report. Use search, conference, and mode filters to work your unread and mention queues.</p>
<h1>Message Boards</h1>
<table border="1">
<tr><th>ID</th><th>Board</th><th>Conf</th><th>Topics</th><th>New</th><th>Mine</th><th>Mentions</th><th>Last</th><th>Last subject</th><th>Subscription</th></tr>%s</table>
</body></html>`, user.Handle, discoverLink, messageBlock, motdBlock, announcementBlock, quickJumpBlock, confFilterBlock, filterSummary, dashboardBlock, scanHelperBlock, emptyBoardHelper, rows.String())
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
	subscriptions := a.boardSubscriptions(user.Handle)
	messageID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
	view := strings.Builder{}
	if messageID > 0 {
		msg, err := a.msgRepo.GetMessage(messageID)
		if err == nil && msg.BoardID == boardID {
			_ = a.msgRepo.SetPointer(user.ID, boardID, msg.ID, time.Now().UTC())
			view.WriteString(`<h2>Reader</h2>`)
			view.WriteString(`<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Reading thread #` + strconv.FormatInt(msg.ID, 10) + `</strong><p>Use reply to continue the thread or report to send it into the moderation queue.</p></article><article class="wolfbbs-helper-card"><strong>Reply target</strong><p>Replies will quote the current post so the thread keeps context.</p></article><article class="wolfbbs-helper-card"><strong>Moderation path</strong><p>Report is for abuse, spam, or content that needs sysop attention.</p></article></section>`)
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
				view.WriteString(`<form method="POST" action="/boards" data-draft-key="board-` + strconv.FormatInt(boardID, 10) + `-reply-` + strconv.FormatInt(msg.ID, 10) + `" data-rich-compose="board-reply" data-compose-signature="` + htmlEscape(user.Handle) + `" data-compose-quote="` + htmlEscape(quoteBody(msg.Body)) + `"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `"><input type="hidden" name="parent_id" value="` + strconv.FormatInt(msg.ID, 10) + `">` + csrf)
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
	boardHelperBlock := `<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Board scan</strong><p>Rows marked N are newer than your current read pointer for this board.</p></article><article class="wolfbbs-helper-card"><strong>Posting flow</strong><p>Drafts in the compose boxes are saved locally in this browser while you type.</p></article><article class="wolfbbs-helper-card"><strong>Subscription tier</strong><p>Watch sends this board into Attention Center, digest keeps it in Today Brief, and mute removes it from routine loops while keeping access intact.</p></article></section>`
	motdBlock := ""
	if strings.TrimSpace(a.motd) != "" {
		motdBlock = `<p><strong>MOTD:</strong> ` + htmlEscape(a.motd) + `</p>`
	}
	announcementBlock := ""
	if strings.TrimSpace(a.announcement) != "" {
		announcementBlock = `<p><strong>Announcement:</strong> ` + htmlEscape(a.announcement) + `</p>`
	}
	emptyBoardDetail := ``
	if len(msgs) == 0 {
		emptyBoardDetail = a.renderRoleAwareEmptyState(user, "board_detail")
	}
	page := `<html><body><h1>Board: ` + htmlEscape(board.Name) + `</h1>` +
		`<p><a href="/start">start</a> | <a href="/today">today</a> | <a href="/attention">attention</a> | <a href="/events">events</a> | <a href="/boards">all boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/doors">doors</a> | <a href="/status">status</a> | <a href="/config">config</a>` + discoverLink + ` | <a href="/help">help</a> | <a href="/logout">logout</a></p>` +
		messageBlock +
		`<p><strong>Reader keys:</strong> open subject to read, use Reply form, and Report for abuse/moderation queue.</p>` +
		`<p><strong>Conference:</strong> ` + htmlEscape(defaultConferenceValue(board.Conference)) + `</p>` +
		`<form method="POST" action="/boards" class="wolfbbs-inline-actions"><input type="hidden" name="action" value="subscribe"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `">` + csrf + `<label>Subscription <select name="subscription_mode">` + boardSubscriptionOptionRows(normalizeBoardSubscriptionMode(string(subscriptions[board.ID]))) + `</select></label><button type="submit">Save</button></form>` +
		motdBlock + announcementBlock + boardHelperBlock + emptyBoardDetail +
		`<table border="1"><tr><th>ID</th><th>New</th><th>Subject</th><th>Author</th><th>When</th></tr>` + rows.String() + `</table>`
	if a.canWriteBoard(user, board) {
		page += `<h3>New Post</h3><form method="POST" action="/boards" data-draft-key="board-` + strconv.FormatInt(boardID, 10) + `-post" data-rich-compose="board-post" data-compose-signature="` + htmlEscape(user.Handle) + `"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `">` + csrf +
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
	boxFilter := normalizeMailBox(r.URL.Query().Get("box"))
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	templateName := normalizeMailTemplate(r.URL.Query().Get("template"))

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
		replyTo := to
		if item.FromUserID != user.ID {
			replyTo = handleByID[item.FromUserID]
		}
		replySubject := item.Subject
		if !strings.HasPrefix(strings.ToLower(replySubject), "re:") {
			replySubject = "Re: " + replySubject
		}
		replyBody := quoteBody(item.Body)
		replyBlock := ``
		if a.canSendMail(user) && replyTo != "" {
			replyBlock = `<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Reply in context</strong><p>Quick Reply carries the quoted body forward so you can answer without losing the thread.</p></article><article class="wolfbbs-helper-card"><strong>Use mail for direct follow-up</strong><p>Keep public discussion on boards and use mail when the conversation should stay private.</p></article></section>` +
				`<h2>Quick Reply</h2><form method="POST" action="/mail" data-draft-key="mail-reply-` + strconv.FormatInt(item.ID, 10) + `" data-rich-compose="mail-reply" data-compose-signature="` + htmlEscape(user.Handle) + `" data-compose-quote="` + htmlEscape(replyBody) + `">` + a.csrfHiddenInput(r) +
				`<label>To <input name="to" size="40" value="` + htmlEscape(replyTo) + `"></label><br>` +
				`<label>Subject <input name="subject" size="60" value="` + htmlEscape(replySubject) + `"></label><br>` +
				`<label>Body<br><textarea name="body" rows="10" cols="80">` + htmlEscape(replyBody) + `</textarea></label><br><button type="submit">Send Reply</button></form>`
		}
		page := `<html><body><h1>Mail #` + strconv.FormatInt(item.ID, 10) + `</h1><p><a href="/mail">back</a> | <a href="/boards">boards</a> | <a href="/help">help</a></p>` +
			`<p><strong>From:</strong> ` + htmlEscape(handleByID[item.FromUserID]) + `<br>` +
			`<strong>To:</strong> ` + htmlEscape(to) + `<br>` +
			`<strong>Subject:</strong> ` + htmlEscape(item.Subject) + `<br>` +
			`<strong>Sent:</strong> ` + item.CreatedAt.Format("2006-01-02 15:04:05") + `</p>` +
			`<pre>` + htmlEscape(item.Body) + `</pre>` + replyBlock + `</body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page))
		return
	}

	inbox, _ := a.mailRepo.ListInbox(user.ID, 100)
	outbox, _ := a.mailRepo.ListOutbox(user.ID, 100)
	handleByID := a.userHandleLookup()
	prefillTo := strings.TrimSpace(r.URL.Query().Get("to"))
	prefillSubject := strings.TrimSpace(r.URL.Query().Get("subject"))
	prefillBody := strings.TrimSpace(r.URL.Query().Get("body"))
	if templateSubject, templateBody := mailTemplatePrefill(templateName); templateSubject != "" || templateBody != "" {
		if prefillSubject == "" {
			prefillSubject = templateSubject
		}
		if prefillBody == "" {
			prefillBody = templateBody
		}
	}
	unreadCount := 0
	for _, row := range inbox {
		if row.ReadAt == nil {
			unreadCount++
		}
	}
	visibleInbox, visibleOutbox := filterMailRows(inbox, outbox, handleByID, boxFilter, searchQuery)
	inRows := strings.Builder{}
	for _, row := range visibleInbox {
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
	for _, row := range visibleOutbox {
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
	addressRows := strings.Builder{}
	for _, row := range a.recentCorrespondents(user, inbox, outbox, 10) {
		addressRows.WriteString(`<li><a href="/mail?to=` + url.QueryEscape(row) + `">` + htmlEscape(row) + `</a></li>`)
	}
	if addressRows.Len() == 0 {
		addressRows.WriteString(`<li>No recent correspondents yet.</li>`)
	}
	localRows := strings.Builder{}
	for _, row := range a.localMailPicks(user.Handle, 10) {
		localRows.WriteString(`<li><a href="/mail?to=` + url.QueryEscape(row) + `">` + htmlEscape(row) + `</a> | <a href="/directory?handle=` + url.QueryEscape(row) + `">profile</a></li>`)
	}
	if localRows.Len() == 0 {
		localRows.WriteString(`<li>No local caller picks yet.</li>`)
	}
	boxOptionRows := strings.Builder{}
	for _, row := range []string{"all", "inbox", "unread", "outbox"} {
		selected := ""
		if row == boxFilter {
			selected = ` selected`
		}
		label := row
		if row == "all" {
			label = "all mail"
		}
		boxOptionRows.WriteString(`<option value="` + htmlEscape(row) + `"` + selected + `>` + htmlEscape(label) + `</option>`)
	}
	templateOptionRows := strings.Builder{}
	for _, row := range []string{"none", "short_note", "door_invite", "follow_up"} {
		selected := ""
		if row == templateName {
			selected = ` selected`
		}
		label := row
		switch row {
		case "none":
			label = "blank compose"
		case "short_note":
			label = "short note"
		case "door_invite":
			label = "door invite"
		case "follow_up":
			label = "follow-up"
		}
		templateOptionRows.WriteString(`<option value="` + htmlEscape(row) + `"` + selected + `>` + htmlEscape(label) + `</option>`)
	}
	filterItems := make([]string, 0, 3)
	if boxFilter != "" && boxFilter != "all" {
		filterItems = append(filterItems, "box "+boxFilter)
	}
	if searchQuery != "" {
		filterItems = append(filterItems, `search "`+searchQuery+`"`)
	}
	if templateName != "" && templateName != "none" {
		filterItems = append(filterItems, "template "+templateName)
	}
	filterSummary := renderActiveFilterPanel("Active Mail Filters", "/mail", filterItems)
	mailHelperBlock := `<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Triage inbox first</strong><p>Use unread to work through new mail before browsing your outbox history.</p></article><article class="wolfbbs-helper-card"><strong>Templates speed the first pass</strong><p>Use a template to load a starter subject/body, then edit it into a real message.</p></article><article class="wolfbbs-helper-card"><strong>Drafts are local</strong><p>Compose and reply boxes save locally in this browser while you type.</p></article></section>`
	page := `<html><body><h1>Private Mail</h1><p><a href="/start">start</a> | <a href="/attention">attention</a> | <a href="/boards">boards</a> | <a href="/bulletins">bulletins</a> | <a href="/directory">directory</a> | <a href="/finder">finder</a> | <a href="/feedback">feedback</a> | <a href="/chat">chat</a> | <a href="/status">status</a> | <a href="/config">config</a>` + discoverLink + ` | <a href="/help">help</a> | <a href="/logout">logout</a></p>` +
		messageBlock +
		`<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(inbox)) + `</strong><span>inbox</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(unreadCount) + `</strong><span>unread</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(outbox)) + `</strong><span>outbox</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(a.recentCorrespondents(user, inbox, outbox, 10))) + `</strong><span>recent correspondents</span></article></section>` +
		mailHelperBlock + filterSummary +
		`<form method="GET" action="/mail" class="wolfbbs-inline-form" data-filter-form="mail" data-filter-reset="/mail"><label>Box <select name="box" data-filter-label="box">` + boxOptionRows.String() + `</select></label><label>Search <input name="q" value="` + htmlEscape(searchQuery) + `" placeholder="subject, body, handle" data-filter-label="search"></label><button type="submit">Filter</button></form>` +
		`<form method="GET" action="/mail" class="wolfbbs-inline-form" data-filter-form="mail-template" data-filter-reset="/mail"><label>Template <select name="template" data-filter-label="template">` + templateOptionRows.String() + `</select></label><label>To <input name="to" value="` + htmlEscape(prefillTo) + `" placeholder="optional recipient" data-filter-label="to"></label><button type="submit">Load Template</button></form>` +
		`<p><strong>Tip:</strong> Use handle for local mail, email address for external relay (if enabled by policy).</p>` +
		`<h2>Compose</h2><form method="POST" action="/mail" data-draft-key="mail-compose" data-rich-compose="mail-compose" data-compose-signature="` + htmlEscape(user.Handle) + `">` + csrf +
		`<label>To (handle or email): <input name="to" size="40" value="` + htmlEscape(prefillTo) + `"></label><br>` +
		`<label>Subject: <input name="subject" size="60" value="` + htmlEscape(prefillSubject) + `"></label><br>` +
		`<label>Body:<br><textarea name="body" rows="10" cols="80">` + htmlEscape(prefillBody) + `</textarea></label><br><button type="submit">Send</button></form>` +
		`<section class="wolfbbs-grid"><article><h2>Recent Correspondents</h2><ul>` + addressRows.String() + `</ul></article><article><h2>Address Book</h2><ul>` + localRows.String() + `</ul></article></section>` +
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
			homeRoute := normalizeHomeRoute(r.FormValue("home_route"))
			if err := a.authSvc.SetPreferences(user.Handle, theme, ansiEnabled, pagingEnabled, timeFormat24h); err != nil {
				redirectWithError(w, r, "/settings", "Preference update failed.")
				return
			}
			if homeRoute != "" {
				a.persistHomeRoute(user.Handle, homeRoute)
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
	homeRoute := a.preferredHomeRoute(user)
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
		`<li>Home route: ` + htmlEscape(homeRoute) + `</li>` +
		`</ul>` +
		`<h2>Display Preferences</h2><form method="POST" action="/settings"><input type="hidden" name="action" value="update_prefs">` + csrf +
		`<label>Theme: <select name="theme">` + themeOptions + `</select></label><br>` +
		`<label><input type="checkbox" name="ansi_enabled" value="1" ` + checkedAttr(user.ANSIEnabled) + `> ANSI enabled</label><br>` +
		`<label><input type="checkbox" name="paging_enabled" value="1" ` + checkedAttr(user.PagingEnabled) + `> Paging enabled</label><br>` +
		`<label><input type="checkbox" name="time_format_24h" value="1" ` + checkedAttr(user.TimeFormat24h) + `> 24-hour time format</label><br>` +
		`<label>Home route <select name="home_route">` + homeRouteOptionRows(homeRoute) + `</select></label><br>` +
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
<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/radar">radar</a> | <a href="/clubhouse">clubhouse</a> | <a href="/doors">doors</a> | <a href="/settings">settings</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
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

func (a *webApp) handleRadar(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	snapshot := a.buildRadarSnapshot(user)
	boardRows := strings.Builder{}
	for _, row := range snapshot.BoardPulse {
		boardRows.WriteString(`<tr><td><a href="/boards?board=` + strconv.FormatInt(row.BoardID, 10) + `">` + htmlEscape(row.BoardName) + `</a></td><td>` + htmlEscape(defaultConferenceValue(row.Conference)) + `</td><td>` + strconv.Itoa(row.NewCount) + `</td><td>` + strconv.Itoa(row.MessageCount) + `</td><td>` + htmlEscape(row.LastAt) + `</td><td>` + htmlEscape(row.LastSubject) + `</td></tr>`)
	}
	if boardRows.Len() == 0 {
		boardRows.WriteString(`<tr><td colspan="6">No board pulse available yet.</td></tr>`)
	}

	activityRows := strings.Builder{}
	for _, row := range snapshot.ActivityItems {
		activityRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	if activityRows.Len() == 0 {
		activityRows.WriteString(`<li>No new activity since your last call.</li>`)
	}

	liveRows := strings.Builder{}
	for _, row := range snapshot.LiveCallers {
		liveRows.WriteString(`<tr><td>` + htmlEscape(row.Handle) + `</td><td>` + htmlEscape(row.Node) + `</td><td>` + htmlEscape(row.Area) + `</td><td>` + htmlEscape(row.Since) + `</td><td>` + htmlEscape(row.Idle) + `</td><td>` + htmlEscape(row.Origin) + `</td><td>` + htmlEscape(row.From) + `</td></tr>`)
	}
	if liveRows.Len() == 0 {
		liveRows.WriteString(`<tr><td colspan="7">No callers currently online.</td></tr>`)
	}

	recentRows := strings.Builder{}
	for _, row := range snapshot.RecentCallers {
		recentRows.WriteString(`<li><strong>` + htmlEscape(row.Handle) + `</strong> from ` + htmlEscape(row.From) + ` in ` + htmlEscape(row.Area) + ` <span class="wolfbbs-muted">` + htmlEscape(row.Duration) + `</span></li>`)
	}
	if recentRows.Len() == 0 {
		recentRows.WriteString(`<li>No caller history captured yet.</li>`)
	}

	doorCards := strings.Builder{}
	for _, row := range snapshot.RecommendedDoors {
		meta := []string{strings.ToUpper(row.Door.Category), "HK " + strings.ToUpper(row.Door.Hotkey)}
		if row.TurnsRemaining > 0 {
			meta = append(meta, strconv.Itoa(row.TurnsRemaining)+" turns")
		}
		if row.TopScoreHandle != "" {
			meta = append(meta, "champ "+row.TopScoreHandle)
		}
		doorCards.WriteString(`<article class="wolfbbs-card"><h3>` + htmlEscape(row.Door.Name) + `</h3><p>` + htmlEscape(row.Door.Description) + `</p><p class="wolfbbs-chip-row">`)
		for _, chip := range meta {
			doorCards.WriteString(`<span class="wolfbbs-chip">` + htmlEscape(chip) + `</span>`)
		}
		doorCards.WriteString(`</p><p><a href="/doors?mode=recommended">Open in Door Cockpit</a> | <a href="/scores?door=` + htmlEscape(row.Door.ID) + `">scores</a></p></article>`)
	}
	if doorCards.Len() == 0 {
		doorCards.WriteString(`<article class="wolfbbs-card"><h3>No door heat yet</h3><p>Once callers begin launching doors, Radar will show trending runs and score chases here.</p></article>`)
	}

	trophyRows := strings.Builder{}
	for _, row := range snapshot.RecentAchievements {
		trophyRows.WriteString(`<li><strong>` + htmlEscape(strings.ToUpper(row.DoorID)) + `</strong> ` + htmlEscape(row.AchievementCode) + ` <span class="wolfbbs-muted">` + row.CreatedAt.Local().Format("2006-01-02 15:04") + `</span></li>`)
	}
	if trophyRows.Len() == 0 {
		trophyRows.WriteString(`<li>No recent trophy activity.</li>`)
	}

	rumorLine := ""
	if strings.TrimSpace(snapshot.Rumor) != "" {
		rumorLine = `<p><strong>Rumorz:</strong> ` + htmlEscape(snapshot.Rumor) + `</p>`
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Caller Radar</title></head><body>
<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/clubhouse">clubhouse</a> | <a href="/doors">doors</a> | <a href="/scores">scores</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Caller Radar</h1>
<p>Mission control for unread activity, live callers, door heat, and tonight's board pulse.</p>
` + rumorLine + `
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(snapshot.UnreadPosts) + `</strong><span>unread posts</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(snapshot.UnreadMail) + `</strong><span>unread mail</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(snapshot.OnlineUsers) + `</strong><span>chat online</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(snapshot.LiveNodes) + `</strong><span>live nodes</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(snapshot.TrackedBoards) + `</strong><span>boards tracked</span></article>
</section>
<section class="wolfbbs-grid">
<article><h2>Discovery Queue</h2><ul>` + activityRows.String() + `</ul><p><a href="/discover">Open full discover feed</a></p></article>
<article><h2>Recent Trophy Activity</h2><ul>` + trophyRows.String() + `</ul><p><a href="/scores">Open scoreboards</a></p></article>
</section>
<h2>Board Pulse</h2>
<table border="1">
<tr><th>Board</th><th>Conf</th><th>New</th><th>Total</th><th>Last</th><th>Last subject</th></tr>` + boardRows.String() + `
</table>
<section class="wolfbbs-grid">
<article><h2>Live Caller Radar</h2><table border="1"><tr><th>User</th><th>Node</th><th>Area</th><th>Since</th><th>Idle</th><th>Origin</th><th>From</th></tr>` + liveRows.String() + `</table></article>
<article><h2>Recent Callers</h2><ul>` + recentRows.String() + `</ul><p><a href="/clubhouse">Open Clubhouse</a></p></article>
</section>
<h2>Arcade Heat</h2>
<div class="wolfbbs-card-grid">` + doorCards.String() + `</div>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleClubhouse(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	switch r.Method {
	case http.MethodPost:
		if !a.requireCSRF(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		switch action {
		case "add_oneliner":
			if a.oneLinerzMod == nil {
				redirectWithError(w, r, "/clubhouse", "OneLinerz mod is unavailable.")
				return
			}
			text := cleanOneLiner(r.FormValue("text"), 120)
			if strings.TrimSpace(text) == "" {
				redirectWithError(w, r, "/clubhouse", "One-liner text is required.")
				return
			}
			a.oneLinerzMod.Add(user.Handle, text)
			redirectWithNotice(w, r, "/clubhouse", "One-liner posted.")
			return
		case "add_bbs":
			if a.bbsListMod == nil {
				redirectWithError(w, r, "/clubhouse", "BBS list mod is unavailable.")
				return
			}
			name := cleanOneLiner(r.FormValue("name"), 72)
			host := cleanOneLiner(r.FormValue("host"), 120)
			port, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("port")))
			if strings.TrimSpace(name) == "" || strings.TrimSpace(host) == "" || port <= 0 || port > 65535 {
				redirectWithError(w, r, "/clubhouse", "Name, host, and a valid port are required.")
				return
			}
			a.bbsListMod.Add(name, host, port)
			redirectWithNotice(w, r, "/clubhouse", "BBS listing added to the exchange.")
			return
		default:
			redirectWithError(w, r, "/clubhouse", "Unsupported clubhouse action.")
			return
		}
	case http.MethodGet:
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	csrf := a.csrfHiddenInput(r)
	oneLinerRows := strings.Builder{}
	oneLiners := []mods.OneLiner{}
	if a.oneLinerzMod != nil {
		oneLiners = a.oneLinerzMod.List(12)
	}
	for _, row := range oneLiners {
		oneLinerRows.WriteString(`<li>[` + row.At.Local().Format("15:04") + `] <strong>` + htmlEscape(row.Handle) + `</strong>: ` + htmlEscape(row.Text) + `</li>`)
	}
	if oneLinerRows.Len() == 0 {
		oneLinerRows.WriteString(`<li>No one-liners posted yet.</li>`)
	}

	bbsRows := strings.Builder{}
	bbsList := []mods.BBSListing{}
	if a.bbsListMod != nil {
		bbsList = a.bbsListMod.List(20)
	}
	for _, row := range bbsList {
		bbsRows.WriteString(`<tr><td>` + htmlEscape(row.Name) + `</td><td>` + htmlEscape(row.Host) + `</td><td>` + strconv.Itoa(row.Port) + `</td></tr>`)
	}
	if bbsRows.Len() == 0 {
		bbsRows.WriteString(`<tr><td colspan="3">No BBS exchange listings yet.</td></tr>`)
	}

	onlineRows := strings.Builder{}
	onlineUsers := 0
	if a.chatSvc != nil {
		for _, row := range a.chatSvc.Online() {
			onlineUsers++
			onlineRows.WriteString(`<li><strong>` + htmlEscape(row.Nick) + `</strong> in ` + htmlEscape(row.Area) + ` <span class="wolfbbs-muted">idle ` + strconv.Itoa(row.IdleSec) + `s</span></li>`)
		}
	}
	if onlineRows.Len() == 0 {
		onlineRows.WriteString(`<li>No live presence reported by chat.</li>`)
	}

	rumorLine := "Rumor line unavailable."
	if a.rumorzMod != nil && strings.TrimSpace(a.rumorzMod.Current()) != "" {
		rumorLine = a.rumorzMod.Current()
	}
	recentCallers := []domain.CallerHistory{}
	if a.adminRepo != nil {
		recentCallers, _ = a.adminRepo.ListCallerHistory(6)
	}
	callerRows := strings.Builder{}
	for _, row := range recentCallers {
		callerRows.WriteString(`<li><strong>` + htmlEscape(row.Username) + `</strong> from ` + htmlEscape(remoteHostDisplay(row.RemoteAddr)) + ` <span class="wolfbbs-muted">` + row.LogoutAt.Local().Format("2006-01-02 15:04") + `</span></li>`)
	}
	if callerRows.Len() == 0 {
		callerRows.WriteString(`<li>No recent callers yet.</li>`)
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Clubhouse</title></head><body>
<p><a href="/boards">boards</a> | <a href="/radar">radar</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/doors">doors</a> | <a href="/scores">scores</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Clubhouse</h1>
<p>The social layer: post one-liners, browse the BBS exchange, check who's hanging around, and catch the latest rumor.</p>
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(oneLiners)) + `</strong><span>one-liners</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(bbsList)) + `</strong><span>bbs exchange links</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(onlineUsers) + `</strong><span>chat presences</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(recentCallers)) + `</strong><span>recent callers</span></article>
</section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Post a One-Liner</h2><form method="POST" action="/clubhouse"><input type="hidden" name="action" value="add_oneliner">` + csrf + `<label>Message <input name="text" maxlength="120" placeholder="keep it short, funny, or legendary"></label><button type="submit">Post</button></form><p class="wolfbbs-muted">Your handle is attached automatically.</p></article>
<article class="wolfbbs-card"><h2>Add a BBS Listing</h2><form method="POST" action="/clubhouse" class="wolfbbs-inline-form"><input type="hidden" name="action" value="add_bbs">` + csrf + `<label>Name <input name="name" maxlength="72" placeholder="Another Cool BBS"></label><label>Host <input name="host" maxlength="120" placeholder="bbs.example.com"></label><label>Port <input name="port" inputmode="numeric" value="23"></label><button type="submit">Add</button></form><p class="wolfbbs-muted">Use this for legit neighboring boards only.</p></article>
</section>
<section class="wolfbbs-grid">
<article><h2>OneLinerz Wall</h2><ul>` + oneLinerRows.String() + `</ul></article>
<article><h2>Rumorz</h2><p>` + htmlEscape(rumorLine) + `</p><h3>Who's Around</h3><ul>` + onlineRows.String() + `</ul></article>
</section>
<h2>BBS Exchange</h2>
<table border="1"><tr><th>Name</th><th>Host</th><th>Port</th></tr>` + bbsRows.String() + `</table>
<h2>Recent Callers</h2>
<ul>` + callerRows.String() + `</ul>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleDoors(w http.ResponseWriter, r *http.Request) {
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
		if !a.requireCSRF(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		doorID := strings.TrimSpace(r.FormValue("door_id"))
		switch action {
		case "toggle_favorite":
			if user.ID <= 0 || doorID == "" {
				http.Redirect(w, r, "/doors", http.StatusFound)
				return
			}
			_, _ = a.doorRegistry.ToggleFavorite(user.ID, doorID)
		}
		http.Redirect(w, r, "/doors", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = "all"
	}

	catalog := a.buildDoorCatalog(user)
	recommended := topRecommendedDoors(catalog, 3)
	filtered := filterDoorViews(catalog, q, category, mode)
	categories := doorCategories(catalog)
	totalAchievements := 0
	totalFavorites := 0
	totalRecent := 0
	totalTurns := 0
	for _, row := range catalog {
		totalAchievements += row.PersonalAchievements
		totalTurns += row.TurnsRemaining
		if row.Favorite {
			totalFavorites++
		}
		if row.Recent {
			totalRecent++
		}
	}

	csrf := a.csrfHiddenInput(r)
	messageBlock := pageMessageBlock(r)
	recommendedHTML := strings.Builder{}
	for _, row := range recommended {
		flags := []string{strings.ToUpper(row.Door.Category), "HK " + strings.ToUpper(row.Door.Hotkey)}
		if row.Favorite {
			flags = append(flags, "favorite")
		}
		if row.Recent {
			flags = append(flags, "recent")
		}
		if row.TurnsRemaining > 0 {
			flags = append(flags, strconv.Itoa(row.TurnsRemaining)+" turns")
		}
		recommendedHTML.WriteString(`<article class="wolfbbs-card">`)
		recommendedHTML.WriteString(`<h3>` + htmlEscape(row.Door.Name) + `</h3>`)
		recommendedHTML.WriteString(`<p>` + htmlEscape(row.Door.Description) + `</p>`)
		recommendedHTML.WriteString(`<p class="wolfbbs-chip-row">`)
		for _, flag := range flags {
			recommendedHTML.WriteString(`<span class="wolfbbs-chip">` + htmlEscape(flag) + `</span>`)
		}
		recommendedHTML.WriteString(`</p>`)
		recommendedHTML.WriteString(`</article>`)
	}
	if recommendedHTML.Len() == 0 {
		recommendedHTML.WriteString(`<article class="wolfbbs-card"><h3>No recommendations yet</h3><p>Play a door from the directory and favorites/recent picks will start to shape your cockpit.</p></article>`)
	}

	activityRows := strings.Builder{}
	for _, event := range a.mustDoorEvents("", user.ID, 8) {
		activityRows.WriteString(`<li><strong>` + htmlEscape(strings.ToUpper(event.DoorID)) + `</strong> ` + htmlEscape(strings.ReplaceAll(event.EventType, "_", " ")) + ` <span class="wolfbbs-muted">` + event.CreatedAt.Local().Format("2006-01-02 15:04") + `</span></li>`)
	}
	if activityRows.Len() == 0 {
		activityRows.WriteString(`<li>No personal door activity logged yet.</li>`)
	}

	achievementRows := strings.Builder{}
	for _, row := range a.mustDoorAchievements(user.ID, 8) {
		achievementRows.WriteString(`<li><strong>` + htmlEscape(strings.ToUpper(row.DoorID)) + `</strong> ` + htmlEscape(row.AchievementCode) + ` <span class="wolfbbs-muted">` + row.CreatedAt.Local().Format("2006-01-02 15:04") + `</span></li>`)
	}
	if achievementRows.Len() == 0 {
		achievementRows.WriteString(`<li>No trophies yet. Most doors award achievements after the first meaningful session.</li>`)
	}

	categoryOptions := strings.Builder{}
	selectedAll := ""
	if strings.TrimSpace(category) == "" {
		selectedAll = ` selected`
	}
	categoryOptions.WriteString(`<option value=""` + selectedAll + `>All categories</option>`)
	for _, row := range categories {
		selected := ""
		if strings.EqualFold(row, category) {
			selected = ` selected`
		}
		categoryOptions.WriteString(`<option value="` + htmlEscape(row) + `"` + selected + `>` + htmlEscape(strings.ToUpper(row)) + `</option>`)
	}

	modeOptions := []string{"all", "favorites", "recent", "recommended"}
	modeLabels := map[string]string{
		"all":         "All doors",
		"favorites":   "Favorites",
		"recent":      "Recent",
		"recommended": "Recommended",
	}
	modeSelect := strings.Builder{}
	for _, option := range modeOptions {
		selected := ""
		if option == mode {
			selected = ` selected`
		}
		modeSelect.WriteString(`<option value="` + option + `"` + selected + `>` + modeLabels[option] + `</option>`)
	}

	directoryRows := strings.Builder{}
	for _, row := range filtered {
		flags := []string{}
		if row.Favorite {
			flags = append(flags, "favorite")
		}
		if row.Recent {
			flags = append(flags, "recent")
		}
		if row.Door.NeedsNetwork {
			flags = append(flags, "network")
		}
		if row.Door.NeedsFSWrite {
			flags = append(flags, "fs-write")
		}
		if row.Door.RequiredRole != "" {
			flags = append(flags, "role="+row.Door.RequiredRole)
		}
		flagHTML := `-`
		if len(flags) > 0 {
			flagHTML = `<span class="wolfbbs-chip-row">`
			for _, flag := range flags {
				flagHTML += `<span class="wolfbbs-chip">` + htmlEscape(flag) + `</span>`
			}
			flagHTML += `</span>`
		}
		topLine := `No score yet`
		if row.TopScoreHandle != "" {
			topLine = htmlEscape(row.TopScoreHandle) + ` • ` + strconv.FormatInt(row.TopScore, 10)
		}
		lastPlayed := `never`
		if row.LastPlayed != "" {
			lastPlayed = row.LastPlayed
		}
		directoryRows.WriteString(`<tr><td>` + htmlEscape(strings.ToUpper(row.Door.Hotkey)) + `</td><td><strong>` + htmlEscape(row.Door.Name) + `</strong><br><span class="wolfbbs-muted">` + htmlEscape(row.Door.Description) + `</span></td><td>` + htmlEscape(strings.ToUpper(row.Door.Category)) + `</td><td>` + strconv.Itoa(row.TurnsRemaining) + `</td><td>` + lastPlayed + `<br><span class="wolfbbs-muted">plays ` + strconv.Itoa(row.PlayCount) + ` • achievements ` + strconv.Itoa(row.PersonalAchievements) + `</span></td><td>` + topLine + `<br><span class="wolfbbs-muted">daily ` + strconv.Itoa(row.DailyActive) + ` • total ` + strconv.FormatInt(row.TotalPlays, 10) + `</span></td><td>` + flagHTML + `</td><td><form method="POST" action="/doors">` + csrf + `<input type="hidden" name="action" value="toggle_favorite"><input type="hidden" name="door_id" value="` + htmlEscape(row.Door.ID) + `"><button type="submit">` + map[bool]string{true: "Unfavorite", false: "Favorite"}[row.Favorite] + `</button></form><p><a href="/scores?door=` + htmlEscape(row.Door.ID) + `">Scores</a></p></td></tr>`)
	}
	if directoryRows.Len() == 0 {
		directoryRows.WriteString(`<tr><td colspan="7">No doors matched the current filter.</td></tr>`)
	}
	emptyDoorsHelper := ``
	if len(catalog) == 0 || (len(filtered) == 0 && strings.TrimSpace(q) == "") || (totalFavorites == 0 && totalRecent == 0 && totalAchievements == 0) {
		emptyDoorsHelper = a.renderRoleAwareEmptyState(user, "doors")
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Door Cockpit</title></head><body>
<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/radar">radar</a> | <a href="/clubhouse">clubhouse</a> | <a href="/doors">doors</a> | <a href="/scores">scores</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + messageBlock + `
<h1>Door Cockpit</h1>
<p>One place for favorites, recommendations, trophies, turn budgets, and the full policy-aware door directory.</p>
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(catalog)) + `</strong><span>doors loaded</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(totalFavorites) + `</strong><span>favorites</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(totalRecent) + `</strong><span>recent plays</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(totalAchievements) + `</strong><span>achievements</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(totalTurns) + `</strong><span>turns available now</span></article>
</section>
` + emptyDoorsHelper + `
<section class="wolfbbs-grid">
<article><h2>Recommended For This Caller</h2><div class="wolfbbs-card-grid">` + recommendedHTML.String() + `</div></article>
<article><h2>Recent Activity</h2><ul>` + activityRows.String() + `</ul></article>
<article><h2>Trophy Progress</h2><ul>` + achievementRows.String() + `</ul></article>
</section>
<h2>Directory</h2>
<form method="GET" action="/doors" class="wolfbbs-inline-form">
<label>Search<input name="q" value="` + htmlEscape(q) + `" placeholder="name, id, category"></label>
<label>Category<select name="category">` + categoryOptions.String() + `</select></label>
<label>View<select name="mode">` + modeSelect.String() + `</select></label>
<button type="submit">Apply</button>
</form>
<table border="1">
<tr><th>HK</th><th>Door</th><th>Category</th><th>Turns</th><th>Your Runbook</th><th>Hall of Fame</th><th>Flags</th><th>Actions</th></tr>` + directoryRows.String() + `
</table>
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

func (a *webApp) buildSetupReadinessSnapshot(user *domain.User) statusSnapshot {
	boardsCount := 0
	if a.boardRepo != nil {
		if boards, err := a.boardRepo.List(); err == nil {
			boardsCount = len(boards)
		}
	}
	doorCount := 0
	if a.doorRegistry != nil {
		doorCount = len(a.doorRegistry.Doors())
	}
	usersCount := 0
	realCallerCount := 0
	moderatorCount := 0
	mailbotReady := false
	if a.authSvc != nil {
		if users, err := a.authSvc.ListUsers(); err == nil {
			usersCount = len(users)
			for _, row := range users {
				handle := strings.TrimSpace(strings.ToLower(row.Handle))
				role := rbac.NormalizeRole(row.Role)
				if handle == "mailbot" && !row.Enabled && row.Verified {
					mailbotReady = true
				}
				if role == roleModerator {
					moderatorCount++
				}
				if role != roleAdmin && handle != "mailbot" {
					realCallerCount++
				}
			}
		}
	}
	siteCustomized := strings.TrimSpace(a.siteDisplayName()) != "" &&
		strings.TrimSpace(a.siteHost()) != "" &&
		!(a.siteDisplayName() == "WolfBBS" && a.siteHost() == "localhost")
	safetyReady := a.requireVerifiedEmail && (a.secureCookie || strings.EqualFold(a.siteHost(), "localhost"))
	launchChecks := []statusCheck{
		{Name: "Site identity customized", OK: siteCustomized, Detail: a.siteDisplayName() + " @ " + a.siteHost()},
		{Name: "Safety baseline set", OK: safetyReady, Detail: fmt.Sprintf("secure_cookie=%s verified_email=%s", boolToText(a.secureCookie), boolToText(a.requireVerifiedEmail))},
		{Name: "Boards seeded", OK: boardsCount > 0, Detail: fmt.Sprintf("%d boards", boardsCount)},
		{Name: "Mailbot service account", OK: mailbotReady, Detail: boolToText(mailbotReady)},
		{Name: "Real caller account exists", OK: realCallerCount > 0, Detail: fmt.Sprintf("%d caller(s), %d moderator(s), %d total users", realCallerCount, moderatorCount, usersCount)},
		{Name: "Chat surface wired", OK: a.chatSvc != nil, Detail: boolToText(a.chatSvc != nil)},
		{Name: "Doors available", OK: doorCount > 0, Detail: fmt.Sprintf("%d doors", doorCount)},
	}
	pass := 0
	for _, row := range launchChecks {
		if row.OK {
			pass++
		}
	}
	recommendations := make([]string, 0, 6)
	if !siteCustomized {
		recommendations = append(recommendations, "Set a real board name and hostname in Step 1 before sharing the board publicly.")
	}
	if !safetyReady {
		recommendations = append(recommendations, "Finish Step 2 and enable verified-email protection; enable secure cookies when the board is behind HTTPS.")
	}
	if boardsCount == 0 {
		recommendations = append(recommendations, "Run the default board seeding action so callers do not land on an empty board.")
	}
	if !mailbotReady {
		recommendations = append(recommendations, "Run the mailbot bootstrap action so sysop and system flows have their service account.")
	}
	if realCallerCount == 0 {
		recommendations = append(recommendations, "Create at least one non-sysop account in /admin/users and test the real caller journey.")
	}
	if a.chatSvc == nil {
		recommendations = append(recommendations, "Investigate chat runtime wiring before launch; /chat should be usable on day one.")
	}
	if doorCount == 0 {
		recommendations = append(recommendations, "Load or register at least one door so the nostalgia loop is not empty.")
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Launch baseline looks good. Walk the real user path once more, then invite callers.")
	}
	role := roleAdmin
	handle := "sysop"
	if user != nil {
		role = rbac.NormalizeRole(user.Role)
		handle = user.Handle
	}
	return statusSnapshot{
		GeneratedAt: time.Now().UTC(),
		Site:        a.siteDisplayName(),
		Host:        a.siteHost(),
		User:        handle,
		Role:        role,
		Summary: statusSummary{
			Total: len(launchChecks),
			Pass:  pass,
			Warn:  len(launchChecks) - pass,
		},
		Checks:          launchChecks,
		Recommendations: recommendations,
	}
}

func launchVerdictText(snapshot statusSnapshot) string {
	if snapshot.Summary.Warn == 0 {
		return "Caller-ready baseline reached"
	}
	if snapshot.Summary.Pass >= snapshot.Summary.Total-2 {
		return "Close to launch"
	}
	return "Needs operator attention"
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
	launchBlock := ""
	if a.hasRole(user, roleAdmin) {
		adminLink = ` | <a href="/admin/ops">ops center</a> | <a href="/admin/system">sysop system</a>`
		readiness := a.buildSetupReadinessSnapshot(user)
		launchBlock = `<h2>Launch Readiness</h2>` +
			`<p><strong>Verdict:</strong> ` + htmlEscape(launchVerdictText(readiness)) + ` | ` + strconv.Itoa(readiness.Summary.Pass) + `/` + strconv.Itoa(readiness.Summary.Total) + ` launch checks PASS</p>` +
			`<p><a href="/admin/launch">Launch Center</a> | <a href="/admin/setup">Setup Wizard</a> | <a href="/admin/users">Create Caller</a></p>`
	}
	rows := strings.Builder{}
	for _, row := range snapshot.Checks {
		rows.WriteString(statusRow(row.Name, row.OK, row.Detail))
	}
	recoRows := strings.Builder{}
	for _, row := range snapshot.Recommendations {
		recoRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	statusHelperBlock := `<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(snapshot.Summary.Pass) + `</strong><span>checks passing</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(snapshot.Summary.Warn) + `</strong><span>warnings to review</span></article><article class="wolfbbs-kpi-card"><strong>` + snapshot.GeneratedAt.Local().Format("15:04") + `</strong><span>snapshot time</span></article></section>` +
		`<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Use this as the fast answer</strong><p>Status Center is the quickest way to verify whether the board looks healthy from the caller side.</p></article><article class="wolfbbs-helper-card"><strong>Warnings first, polish second</strong><p>Fix runtime and launch warnings here before spending time on lower-value visual polish.</p></article><article class="wolfbbs-helper-card"><strong>Escalate to sysop tools when needed</strong><p>Open Launch Center or System when you need operator detail behind a warning.</p></article></section>`
	page := `<html><body><h1>Status Center</h1>` +
		`<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/radar">radar</a> | <a href="/clubhouse">clubhouse</a> | <a href="/doors">doors</a> | <a href="/config">config</a> | <a href="/settings">settings</a> | <a href="/help">help</a> | <a href="/logout">logout</a>` + adminLink + `</p>` +
		`<p><strong>Summary:</strong> ` + strconv.Itoa(snapshot.Summary.Pass) + `/` + strconv.Itoa(snapshot.Summary.Total) + ` PASS, ` + strconv.Itoa(snapshot.Summary.Warn) + ` WARN | generated ` + snapshot.GeneratedAt.Local().Format("2006-01-02 15:04:05") + `</p>` +
		`<p><a href="/statusz">Machine-readable status JSON (/statusz)</a> | <a href="/radar">Caller Radar</a> | <a href="/clubhouse">Clubhouse</a></p>` +
		statusHelperBlock +
		launchBlock +
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
		adminLinks = `<h2>Launch Change Order</h2><ol>` +
			`<li><a href="/admin/launch">/admin/launch</a> for the operator home base and launch verdict.</li>` +
			`<li><a href="/admin/ops">/admin/ops</a> for errors, sessions, audits, and operator triage.</li>` +
			`<li><a href="/admin/setup">/admin/setup</a> for identity, safety, and bootstrap actions.</li>` +
			`<li><a href="/admin/config">/admin/config</a> for runtime services, flags, and exposure.</li>` +
			`<li><a href="/admin/users">/admin/users</a> to create the first real caller.</li>` +
			`<li><a href="/status">/status</a> and <a href="/admin/system">/admin/system</a> before launch.</li>` +
			`</ol>` +
			`<h2>Sysop Configuration Directory</h2><table border="1"><tr><th>Area</th><th>Configure</th><th>Status</th></tr>` +
			`<tr><td>Launch center</td><td><a href="/admin/launch">/admin/launch</a></td><td><a href="/status">/status</a></td></tr>` +
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
		`<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/radar">radar</a> | <a href="/clubhouse">clubhouse</a> | <a href="/doors">doors</a> | <a href="/status">status</a> | <a href="/settings">settings</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>` +
		`<p>Role: ` + htmlEscape(role) + `</p>` +
		`<p><strong>Site:</strong> ` + htmlEscape(a.siteDisplayName()) + ` (` + htmlEscape(a.siteHost()) + `)</p>` +
		`<h2>User Configuration</h2><ul>` +
		`<li>Display + ANSI + pager + 24h clock: <a href="/settings">/settings</a></li>` +
		`<li>Password + 2FA: <a href="/settings">/settings</a></li>` +
		`<li>Personal inbox/outbox and posting workflow: <a href="/mail">/mail</a> and <a href="/boards">/boards</a></li>` +
		`<li>Mission control + social layer: <a href="/radar">/radar</a> and <a href="/clubhouse">/clubhouse</a></li>` +
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
	readiness := a.buildSetupReadinessSnapshot(user)
	statusSnapshot := a.buildStatusSnapshot(user)
	adminActions := strings.Builder{}
	for _, item := range readiness.Recommendations {
		adminActions.WriteString(`<li>` + htmlEscape(item) + `</li>`)
	}
	if adminActions.Len() == 0 {
		adminActions.WriteString(`<li>No launch blockers detected. Walk the caller path once more, then announce the board.</li>`)
	}
	adminActionGrid := `<section class="wolfbbs-action-grid"><a class="wolfbbs-action-card" href="/admin/launch"><strong>Launch Center</strong><span>go-live verdict, next-best actions, operator commands</span></a><a class="wolfbbs-action-card" href="/admin/ops"><strong>Ops Center</strong><span>errors, sessions, audits, and triage</span></a><a class="wolfbbs-action-card" href="/admin/setup"><strong>Setup Wizard</strong><span>identity, safety, bootstrap, launch checklist</span></a><a class="wolfbbs-action-card" href="/admin/users"><strong>User Ops</strong><span>create callers, role changes, bans, resets</span></a><a class="wolfbbs-action-card" href="/admin/system"><strong>System</strong><span>runtime health, service state, deeper operator detail</span></a></section>`
	page := `<html><body><h1>Sysop Control Panel</h1><p>Logged in as ` + user.Handle + `</p>` +
		`<p><a href="/admin/users">Users</a> | <a href="/admin/boards">Boards</a> | <a href="/admin/mail">Mail</a> | ` +
		`<a href="/admin/files">Files</a> | <a href="/admin/gateways">Gateways</a> | <a href="/admin/chat">Chat</a> | ` +
		`<a href="/admin/doors">Doors</a> | <a href="/admin/launch">Launch Center</a> | <a href="/admin/ops">Ops Center</a> | <a href="/admin/setup">Setup</a> | <a href="/admin/config">Config</a> | ` +
		`<a href="/admin/system">System</a> | <a href="/admin/errors">Errors</a> | <a href="/admin/audit">Audit Log</a> | <a href="/help">Help</a></p>` +
		`<h2>Launch Digest</h2>` +
		`<p><strong>Verdict:</strong> ` + htmlEscape(launchVerdictText(readiness)) + ` | ` + strconv.Itoa(readiness.Summary.Pass) + `/` + strconv.Itoa(readiness.Summary.Total) + ` launch checks PASS | ` + strconv.Itoa(statusSnapshot.Summary.Warn) + ` runtime warnings | ` + strconv.Itoa(errorCount) + ` runtime errors logged</p>` +
		`<p><a href="/admin/launch">Open Launch Center</a> | <a href="/admin/setup">Finish setup</a> | <a href="/status">Caller status center</a></p>` +
		adminActionGrid +
		`<ul>` + adminActions.String() + `</ul>` +
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

func (a *webApp) handleAdminLaunch(w http.ResponseWriter, r *http.Request) {
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
		case "toggle_checkpoint":
			checkpoint := strings.TrimSpace(r.FormValue("checkpoint"))
			done := parseCheckbox(r.FormValue("done"))
			if checkpoint == "" {
				redirectWithError(w, r, "/admin/launch", "Checkpoint is required.")
				return
			}
			a.setLaunchCheckpoint(user.Handle, checkpoint, done)
			a.recordAdminAction(user.Handle, "launch", "toggle_checkpoint", checkpoint+"="+boolToText(done))
			redirectWithNotice(w, r, "/admin/launch", "Launch checkpoint updated.")
			return
		default:
			redirectWithError(w, r, "/admin/launch", "Unsupported launch action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	readiness := a.buildSetupReadinessSnapshot(user)
	runtime := a.buildStatusSnapshot(user)
	checkpointState := a.launchChecklist(user.Handle)
	checkpoints := []launchCheckpoint{
		{Key: "identity_reviewed", Title: "Identity reviewed", Detail: "Board name, host, MOTD, and announcement read like a public system, not a local test box.", Done: checkpointState["identity_reviewed"]},
		{Key: "safety_reviewed", Title: "Safety reviewed", Detail: "Secure-cookie, verified-email, and basic moderation posture were checked before inviting real callers.", Done: checkpointState["safety_reviewed"]},
		{Key: "bootstrap_reviewed", Title: "Bootstrap reviewed", Detail: "Boards, mailbot, doors, and starter content were seeded so first-time callers do not hit empty shells.", Done: checkpointState["bootstrap_reviewed"]},
		{Key: "caller_walk_reviewed", Title: "Caller path walked", Detail: "A non-sysop login was tested through boards, chat, mail, and at least one door.", Done: checkpointState["caller_walk_reviewed"]},
		{Key: "rollback_ready", Title: "Rollback ready", Detail: "You know which command restores service, where the logs live, and how to back out a bad upgrade quickly.", Done: checkpointState["rollback_ready"]},
	}
	readinessRows := strings.Builder{}
	for _, row := range readiness.Checks {
		readinessRows.WriteString(statusRow(row.Name, row.OK, row.Detail))
	}
	runtimeRows := strings.Builder{}
	for _, row := range runtime.Checks {
		runtimeRows.WriteString(statusRow(row.Name, row.OK, row.Detail))
	}
	launchActionRows := strings.Builder{}
	for _, row := range readiness.Recommendations {
		launchActionRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	for _, row := range runtime.Recommendations {
		launchActionRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	if launchActionRows.Len() == 0 {
		launchActionRows.WriteString(`<li>No immediate issues detected. Validate the real caller path and publish the board.</li>`)
	}
	checkpointRows := strings.Builder{}
	checkpointDone := 0
	csrf := a.csrfHiddenInput(r)
	for _, row := range checkpoints {
		if row.Done {
			checkpointDone++
		}
		buttonLabel := "Mark Complete"
		nextDone := "1"
		if row.Done {
			buttonLabel = "Mark Open"
			nextDone = "0"
		}
		checkpointRows.WriteString(`<tr><td><strong>` + htmlEscape(row.Title) + `</strong><br><span class="wolfbbs-muted">` + htmlEscape(row.Detail) + `</span></td><td>` + boolToText(row.Done) + `</td><td><form method="POST" action="/admin/launch" class="wolfbbs-inline-actions"><input type="hidden" name="action" value="toggle_checkpoint"><input type="hidden" name="checkpoint" value="` + htmlEscape(row.Key) + `"><input type="hidden" name="done" value="` + nextDone + `">` + csrf + `<button type="submit">` + buttonLabel + `</button></form></td></tr>`)
	}
	launchHelperBlock := `<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Use Launch Center as home base</strong><p>This page is the operator control room when the board is almost ready but not obviously done.</p></article><article class="wolfbbs-helper-card"><strong>Walk real caller paths</strong><p>Do not treat green config alone as done; validate boards, chat, doors, and mail like a normal user would.</p></article><article class="wolfbbs-helper-card"><strong>Keep commands close</strong><p>The operator commands below are copyable so recovery and upgrades do not require hunting through docs.</p></article></section>`
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Launch Center</title></head><body><h1>Launch Center</h1>` +
		`<p><a href="/admin">back</a> | <a href="/admin/ops">ops</a> | <a href="/admin/setup">setup</a> | <a href="/admin/config">config</a> | <a href="/admin/system">system</a> | <a href="/status">status</a> | <a href="/help">help</a></p>` +
		pageMessageBlock(r) +
		`<p>Use this page as the sysop home base for first-run, pre-launch review, and support triage.</p>` +
		launchHelperBlock +
		`<h2>Launch Summary</h2>` +
		`<p><strong>Verdict:</strong> ` + htmlEscape(launchVerdictText(readiness)) + ` | ` + strconv.Itoa(readiness.Summary.Pass) + `/` + strconv.Itoa(readiness.Summary.Total) + ` launch checks PASS | runtime ` + strconv.Itoa(runtime.Summary.Warn) + ` WARN</p>` +
		`<p><a href="/admin/setup">Setup Wizard</a> | <a href="/admin/users">Create Caller</a> | <a href="/boards">Walk Boards</a> | <a href="/chat">Walk Chat</a> | <a href="/doors">Walk Doors</a></p>` +
		`<h2>Go-Live Checkpoints</h2><p><strong>` + strconv.Itoa(checkpointDone) + `/` + strconv.Itoa(len(checkpoints)) + `</strong> operator checkpoints complete.</p><table border="1"><tr><th>Checkpoint</th><th>Done</th><th>Action</th></tr>` + checkpointRows.String() + `</table>` +
		`<h2>Launch Checks</h2><table border="1"><tr><th>Check</th><th>Status</th><th>Details</th></tr>` + readinessRows.String() + `</table>` +
		`<h2>Runtime Checks</h2><table border="1"><tr><th>Check</th><th>Status</th><th>Details</th></tr>` + runtimeRows.String() + `</table>` +
		`<h2>Next Best Actions</h2><ul>` + launchActionRows.String() + `</ul>` +
		`<h2>Run In This Order</h2><ol>` +
		`<li><a href="/admin/setup">/admin/setup</a> for identity, safety, and bootstrap actions.</li>` +
		`<li><a href="/admin/config">/admin/config</a> for runtime flags, services, and public-facing behavior.</li>` +
		`<li><a href="/admin/users">/admin/users</a> to create at least one non-sysop caller.</li>` +
		`<li><a href="/boards">/boards</a>, <a href="/chat">/chat</a>, <a href="/doors">/doors</a>, and <a href="/scores">/scores</a> as a real user.</li>` +
		`<li><a href="/status">/status</a> and <a href="/admin/system">/admin/system</a> before you announce the board.</li>` +
		`</ol>` +
		`<h2>Operator Commands</h2><pre>bash install.sh --status
bash install.sh --doctor
bash install.sh --repair
bash install.sh --logs
bash install.sh --upgrade</pre>` +
		`<h2>Rollback Steps</h2><ol><li>Run <code>bash install.sh --status</code> to confirm the active layout and service state.</li><li>Run <code>bash install.sh --logs</code> to capture the failing service before changing anything.</li><li>If the last upgrade caused the fault, use <code>bash install.sh --repair</code> or redeploy the previous tagged bundle.</li><li>Re-walk <a href="/start">/start</a>, <a href="/today">/today</a>, <a href="/boards">/boards</a>, <a href="/chat">/chat</a>, and <a href="/doors">/doors</a> after recovery.</li></ol>` +
		`<h2>Operator Docs</h2><ul>` +
		`<li><code>docs/START_HERE.md</code></li>` +
		`<li><code>docs/LAUNCH_CHECKLIST.md</code></li>` +
		`<li><code>docs/OPERATOR_PLAYBOOK.md</code></li>` +
		`<li><code>docs/TROUBLESHOOTING.md</code></li>` +
		`<li><code>docs/OPERATIONS.md</code></li>` +
		`</ul></body></html>`
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
			seeded, err := seedDefaultBoards(a.boardRepo)
			if err != nil {
				a.addAppError("admin.setup", fmt.Errorf("seed default boards: %w", err))
				a.recordAdminAction(user.Handle, "setup", "seed_default_boards", fmt.Sprintf("failed: %v", err))
				redirectWithNotice(w, r, "/admin/setup", "Default board seeding failed.")
				return
			}
			if seeded > 0 {
				a.recordAdminAction(user.Handle, "setup", "seed_default_boards", fmt.Sprintf("seeded=%d", seeded))
				redirectWithNotice(w, r, "/admin/setup", fmt.Sprintf("Seeded %d default board(s).", seeded))
				return
			}
			a.recordAdminAction(user.Handle, "setup", "seed_default_boards", "all defaults already present")
			redirectWithNotice(w, r, "/admin/setup", "Default boards already present.")
			return
		case "ensure_mailbot":
			seedServiceUsers(a.authSvc)
			a.recordAdminAction(user.Handle, "setup", "ensure_mailbot", "mailbot service account checked")
			redirectWithNotice(w, r, "/admin/setup", "Mailbot service account checked.")
			return
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
	readiness := a.buildSetupReadinessSnapshot(user)
	readinessRows := strings.Builder{}
	for _, row := range readiness.Checks {
		readinessRows.WriteString(statusRow(row.Name, row.OK, row.Detail))
	}
	readinessItems := strings.Builder{}
	for _, item := range readiness.Recommendations {
		readinessItems.WriteString(`<li>` + htmlEscape(item) + `</li>`)
	}
	readinessVerdict := launchVerdictText(readiness)

	csrf := a.csrfHiddenInput(r)
	noticeBlock := ""
	if setupNotice != "" {
		noticeBlock = renderPageBanner("notice", setupNotice)
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
	readinessBlock := `<h2>Launch Readiness</h2>` +
		`<p><strong>Go-live verdict:</strong> ` + htmlEscape(readinessVerdict) + ` | ` + strconv.Itoa(readiness.Summary.Pass) + `/` + strconv.Itoa(readiness.Summary.Total) + ` PASS, ` + strconv.Itoa(readiness.Summary.Warn) + ` remaining</p>` +
		`<table border="1"><tr><th>Readiness check</th><th>Status</th><th>Details</th></tr>` + readinessRows.String() + `</table>` +
		`<h3>Next best actions</h3><ul>` + readinessItems.String() + `</ul>`
	launchChecklist := `<h2>Launch Checklist</h2>` +
		`<ol>` +
		`<li>Save Step 1 and Step 2 before treating the board as caller-ready.</li>` +
		`<li>Run the bootstrap actions below to seed boards and verify service accounts.</li>` +
		`<li>Create a real caller or moderator in <a href="/admin/users">/admin/users</a>.</li>` +
		`<li>Walk <a href="/boards">/boards</a>, <a href="/chat">/chat</a>, <a href="/doors">/doors</a>, and <a href="/scores">/scores</a> as if you were a real user.</li>` +
		`<li>Check <a href="/status">/status</a> and <a href="/admin/system">/admin/system</a> before inviting callers.</li>` +
		`</ol>`
	commonGotchas := `<h2>Common Gotchas</h2>` +
		`<ul>` +
		`<li><strong>Secure cookie</strong> should only be enabled when the board is actually behind HTTPS.</li>` +
		`<li><strong>Read-only mode</strong> is for maintenance, not normal launch.</li>` +
		`<li><strong>Guest tour</strong>, discover, and quick jump are experience choices, not hard requirements.</li>` +
		`</ul>`
	setupHelperBlock := `<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Work top to bottom</strong><p>Identity and safety first, then experience flags, then bootstrap actions, then real-user validation.</p></article><article class="wolfbbs-helper-card"><strong>Create one real caller</strong><p>Do not stop at sysop-only setup. Use /admin/users to create a non-sysop account and test the normal path.</p></article><article class="wolfbbs-helper-card"><strong>Bootstrap is not launch</strong><p>Seeding boards and mailbot is necessary, but the board is only ready after the real surfaces behave correctly.</p></article></section>`
	page := `<html><body><h1>Setup & Install</h1><p><a href="/admin">back</a> | <a href="/admin/launch">launch</a> | <a href="/admin/system">system</a> | <a href="/help">help</a></p>` +
		`<p>Use this screen to verify base services and bootstrap sysop dependencies after install/upgrade.</p>` +
		`<p>UI-first setup: keep installer flags minimal; set board identity and runtime policy here.</p>` +
		`<h2>Setup Wizard</h2>` +
		`<p>` + htmlEscape(wizardHint) + `</p>` +
		progress +
		setupHelperBlock +
		`<p><strong>Tip:</strong> use the step links above, then save once after each section change.</p>` +
		readinessBlock +
		launchChecklist +
		commonGotchas +
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
		`<h2>After Bootstrap</h2><ul>` +
		`<li><a href="/admin/users">/admin/users</a> for caller and moderator creation</li>` +
		`<li><a href="/boards">/boards</a> and <a href="/chat">/chat</a> for real-user validation</li>` +
		`<li><a href="/doors">/doors</a> and <a href="/scores">/scores</a> for game surfaces</li>` +
		`<li><a href="/status">/status</a> and <a href="/admin/system">/admin/system</a> for post-launch verification</li>` +
		`</ul>` +
		`<h2>Install and Ops Shortcuts</h2>` +
		`<ul>` +
		`<li>Start here: <code>docs/START_HERE.md</code></li>` +
		`<li>Launch checklist: <code>docs/LAUNCH_CHECKLIST.md</code></li>` +
		`<li>Operator playbook: <code>docs/OPERATOR_PLAYBOOK.md</code></li>` +
		`<li>Troubleshooting: <code>docs/TROUBLESHOOTING.md</code></li>` +
		`<li>Operations guide: <code>docs/OPERATIONS.md</code></li>` +
		`<li>Installer docs: <code>docs/INSTALL.md</code></li>` +
		`<li>Health: <a href="/healthz">/healthz</a> and <a href="/readyz">/readyz</a></li>` +
		`<li>Metrics: <a href="/metrics">/metrics</a></li>` +
		`<li>Sysop CLI: <code>go run ./cmd/oputil status</code> <button type="button" class="wolfbbs-form-secondary" data-copy-text="go run ./cmd/oputil status">Copy</button></li>` +
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
			if target == "" {
				http.Error(w, "create user failed: handle is required", http.StatusBadRequest)
				return
			}
			role := rbac.NormalizeRole(strings.TrimSpace(r.FormValue("role")))
			if role == "" {
				role = roleUser
			}
			if password == "" {
				password = randomPassword(14)
			}
			created, err := a.authSvc.Register(target, password)
			if err != nil {
				a.addAppError("admin.users", fmt.Errorf("create user %s: %w", target, err))
				errText := strings.TrimSpace(err.Error())
				if errText == "" {
					errText = "unknown error"
				}
				http.Error(w, "create user failed: "+errText, http.StatusBadRequest)
				return
			}
			if err := a.authSvc.SetRole(created.Handle, role); err != nil {
				a.addAppError("admin.users", fmt.Errorf("set role for created user %s: %w", created.Handle, err))
				http.Error(w, "create user failed: role assignment failed", http.StatusBadRequest)
				return
			}
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
	filterSummary := renderActiveFilterPanel("Active User Filters", "/admin/users", []string{func() string {
		if filter == "" {
			return ""
		}
		return `search "` + filter + `"`
	}()})
	helperBlock := `<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Create callers fast</strong><p>If you leave password blank, WolfBBS generates one so you can hand off access quickly.</p></article><article class="wolfbbs-helper-card"><strong>Be deliberate with bans and resets</strong><p>User actions below now require confirmation to reduce accidental admin mistakes.</p></article><article class="wolfbbs-helper-card"><strong>Validate with a real account</strong><p>After creating a user, sign in with it and walk boards, chat, doors, and mail.</p></article></section>`

	page := `<html><body><h1>Sysop Users</h1><p><a href="/admin">back</a> | <a href="/admin/audit">audit</a> | <a href="/help">help</a></p>` +
		helperBlock + filterSummary +
		`<h2>Create User</h2><form method="POST">` + csrf +
		`<input type="hidden" name="action" value="create">` +
		`<label>Handle <input name="handle"></label> ` +
		`<label>Password <input name="password"></label> ` +
		`<label>Role <select name="role"><option value="user">User</option><option value="moderator">Moderator</option><option value="sysop">Sysop</option></select></label> ` +
		`<button type="submit">Create</button></form>` +
		`<form method="GET" data-filter-form="admin-users" data-filter-reset="/admin/users"><label>Search: <input name="q" value="` + filter + `" data-filter-label="search"></label><button type="submit">filter</button></form>` +
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
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if a.doorRegistry == nil {
		http.Error(w, "door registry unavailable", http.StatusInternalServerError)
		return
	}
	filterDoor := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("door")))
	snapshot := a.buildScoreboardSnapshot(user, filterDoor)
	doorOptions := strings.Builder{}
	doorOptions.WriteString(`<option value="">All doors</option>`)
	for _, door := range a.doorRegistry.Doors() {
		selected := ""
		if strings.EqualFold(door.ID, filterDoor) {
			selected = ` selected`
		}
		doorOptions.WriteString(`<option value="` + htmlEscape(door.ID) + `"` + selected + `>` + htmlEscape(door.Name) + `</option>`)
	}
	championRows := strings.Builder{}
	for _, row := range snapshot.ChampionRows {
		championRows.WriteString(`<tr><td>` + htmlEscape(row.DoorName) + `</td><td>` + htmlEscape(row.Handle) + `</td><td>` + strconv.FormatInt(row.Score, 10) + `</td><td>` + htmlEscape(row.ScoreType) + `</td><td>` + htmlEscape(row.CreatedAt) + `</td></tr>`)
	}
	if championRows.Len() == 0 {
		championRows.WriteString(`<tr><td colspan="5">No champion rows yet.</td></tr>`)
	}
	recentRows := strings.Builder{}
	for _, row := range snapshot.RecentRows {
		recentRows.WriteString(`<li><strong>` + htmlEscape(row.DoorName) + `:</strong> ` + htmlEscape(row.Handle) + ` posted ` + strconv.FormatInt(row.Score, 10) + ` ` + htmlEscape(row.ScoreType) + ` <span class="wolfbbs-muted">` + htmlEscape(row.CreatedAt) + `</span></li>`)
	}
	if recentRows.Len() == 0 {
		recentRows.WriteString(`<li>No recent score activity yet.</li>`)
	}
	personalRows := strings.Builder{}
	for _, row := range snapshot.PersonalRows {
		personalRows.WriteString(`<li><strong>` + htmlEscape(row.DoorName) + `:</strong> ` + strconv.FormatInt(row.Score, 10) + ` ` + htmlEscape(row.ScoreType) + ` <span class="wolfbbs-muted">` + htmlEscape(row.CreatedAt) + `</span></li>`)
	}
	if personalRows.Len() == 0 {
		personalRows.WriteString(`<li>No leaderboard entries for this caller yet.</li>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Door Scores & Trophies</title></head><body><h1>Door Scores & Trophies</h1>
<p><a href="/boards">boards</a> | <a href="/radar">radar</a> | <a href="/clubhouse">clubhouse</a> | <a href="/doors">doors</a> | <a href="/chat">chat</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/help">help</a></p>
<form method="GET" action="/scores" class="wolfbbs-inline-form"><label>Door <select name="door">` + doorOptions.String() + `</select></label><button type="submit">Filter</button></form>
<p><strong>Door filter:</strong> ` + htmlEscape(filterDoor) + `</p>
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(snapshot.DoorsWithScores) + `</strong><span>doors with scores</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(snapshot.VisibleScoreRows) + `</strong><span>leaderboard rows</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(snapshot.PersonalAchievements) + `</strong><span>your trophies</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(snapshot.PersonalRows)) + `</strong><span>your score entries</span></article>
</section>
<section class="wolfbbs-grid">
<article><h2>Current Champions</h2><table border="1"><tr><th>Door</th><th>User</th><th>Score</th><th>Type</th><th>When</th></tr>` + championRows.String() + `</table></article>
<article><h2>Your Scorecard</h2><ul>` + personalRows.String() + `</ul><p><a href="/doors">Open Door Cockpit</a></p></article>
</section>
<h2>Recent Score Activity</h2>
<ul>` + recentRows.String() + `</ul>
</body></html>`
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
	originCounts := map[string]int{
		"loopback": 0,
		"lan":      0,
		"wan":      0,
		"host":     0,
		"unknown":  0,
	}
	for _, row := range nodeSessions {
		originCounts[netutil.RemoteOrigin(row.RemoteAddr)]++
	}

	onlineRows := strings.Builder{}
	if len(nodeSessions) > 0 {
		now := time.Now().UTC()
		for _, row := range nodeSessions {
			idle := now.Sub(row.LastActivity)
			if idle < 0 {
				idle = 0
			}
			origin := strings.ToUpper(netutil.RemoteOrigin(row.RemoteAddr))
			from := remoteHostDisplay(row.RemoteAddr)
			onlineRows.WriteString(`<tr><td>` + htmlEscape(row.Username) + `</td><td>Node ` + strconv.Itoa(row.NodeID) + `</td><td>` + htmlEscape(row.Area) + `</td><td>` + row.LoginAt.Format("2006-01-02 15:04:05") + `</td><td>` + strconv.Itoa(int(idle.Seconds())) + `s</td><td>` + htmlEscape(origin) + `</td><td>` + htmlEscape(from) + `</td></tr>`)
		}
	} else {
		for _, row := range online {
			onlineRows.WriteString(`<tr><td>` + htmlEscape(row.Nick) + `</td><td>` + htmlEscape(row.Node) + `</td><td>` + htmlEscape(row.Area) + `</td><td>` + row.LoginAt.Format("2006-01-02 15:04:05") + `</td><td>` + strconv.Itoa(row.IdleSec) + `s</td><td>UNKNOWN</td><td>n/a</td></tr>`)
		}
	}
	if onlineRows.Len() == 0 {
		onlineRows.WriteString(`<tr><td colspan="7">No users currently online</td></tr>`)
	}

	callerRows := strings.Builder{}
	for _, caller := range callerHistory {
		origin := strings.ToUpper(netutil.RemoteOrigin(caller.RemoteAddr))
		from := remoteHostDisplay(caller.RemoteAddr)
		callerRows.WriteString(`<tr><td>` + caller.LogoutAt.Format("2006-01-02 15:04:05") + `</td><td>Node ` + strconv.Itoa(caller.NodeID) + `</td><td>` + htmlEscape(caller.Username) + `</td><td>` + htmlEscape(caller.Area) + `</td><td>` + strconv.FormatInt(caller.DurationSeconds, 10) + `s</td><td>` + htmlEscape(origin) + `</td><td>` + htmlEscape(from) + `</td></tr>`)
	}
	if callerRows.Len() == 0 {
		callerRows.WriteString(`<tr><td colspan="7">No caller history available</td></tr>`)
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
		`<tr><td>Origin loopback/lan/wan</td><td>` + strconv.Itoa(originCounts["loopback"]) + ` / ` + strconv.Itoa(originCounts["lan"]) + ` / ` + strconv.Itoa(originCounts["wan"]) + `</td></tr>` +
		`<tr><td>Origin host/unknown</td><td>` + strconv.Itoa(originCounts["host"]) + ` / ` + strconv.Itoa(originCounts["unknown"]) + `</td></tr>` +
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
		`<h2>Online / Node State</h2><table border="1"><tr><th>User</th><th>Node</th><th>Area</th><th>Login</th><th>Idle</th><th>Origin</th><th>From</th></tr>` + onlineRows.String() + `</table>` +
		`<h2>Last Callers</h2><table border="1"><tr><th>Logout</th><th>Node</th><th>User</th><th>Area</th><th>Duration</th><th>Origin</th><th>From</th></tr>` + callerRows.String() + `</table>` +
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
		RemoteHost   string `json:"remote_host"`
		RemoteOrigin string `json:"remote_origin"`
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
			RemoteHost:   remoteHostDisplay(row.RemoteAddr),
			RemoteOrigin: netutil.RemoteOrigin(row.RemoteAddr),
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

func (a *webApp) clearAppErrors() int {
	a.Lock()
	defer a.Unlock()
	cleared := len(a.errorLog)
	a.errorLog = nil
	return cleared
}

func normalizeHandleKey(handle string) string {
	return strings.ToLower(strings.TrimSpace(handle))
}

func attentionDismissedSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingAttentionDismissedRoot + handle
}

func attentionItemKey(item discovery.Item) string {
	return strings.TrimSpace(fmt.Sprintf("%s|%d|%d|%s", item.Kind, item.BoardID, item.MessageID, item.Line))
}

func boardAttentionKey(boardID int64) string {
	if boardID <= 0 {
		return ""
	}
	return fmt.Sprintf("board-pulse|%d", boardID)
}

func boardAttentionItemKey(row boardPulseRow) string {
	if row.BoardID <= 0 {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("board-pulse|%d|%d|%s|%s", row.BoardID, row.NewCount, row.LastAt, row.LastSubject))
}

func boardSubscriptionSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingBoardSubscriptionsRoot + handle
}

func boardWatchSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingBoardWatchRoot + handle
}

func normalizeBoardSubscriptionMode(value string) boardSubscriptionMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(boardSubscriptionWatch):
		return boardSubscriptionWatch
	case string(boardSubscriptionDigest):
		return boardSubscriptionDigest
	case string(boardSubscriptionMute):
		return boardSubscriptionMute
	default:
		return boardSubscriptionNone
	}
}

func boardSubscriptionLabel(mode boardSubscriptionMode) string {
	switch mode {
	case boardSubscriptionWatch:
		return "watch"
	case boardSubscriptionDigest:
		return "digest"
	case boardSubscriptionMute:
		return "mute"
	default:
		return "none"
	}
}

func boardSubscriptionOptionRows(current boardSubscriptionMode) string {
	options := []boardSubscriptionMode{
		boardSubscriptionNone,
		boardSubscriptionWatch,
		boardSubscriptionDigest,
		boardSubscriptionMute,
	}
	var out strings.Builder
	for _, option := range options {
		selected := ""
		if option == current {
			selected = ` selected`
		}
		out.WriteString(`<option value="` + htmlEscape(string(option)) + `"` + selected + `>` + htmlEscape(boardSubscriptionLabel(option)) + `</option>`)
	}
	return out.String()
}

func (a *webApp) boardSubscriptions(handle string) map[int64]boardSubscriptionMode {
	subs := map[int64]boardSubscriptionMode{}
	if a.adminRepo == nil {
		return subs
	}
	if key := boardSubscriptionSettingKey(handle); key != "" {
		raw, err := a.adminRepo.GetSystemSetting(key)
		if err == nil && strings.TrimSpace(raw) != "" {
			decoded := map[string]string{}
			if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
				a.addAppError("board_watch.persistence", fmt.Errorf("decode board subscriptions for %s: %w", handle, err))
			} else {
				for rawID, rawMode := range decoded {
					boardID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
					if err != nil || boardID <= 0 {
						continue
					}
					mode := normalizeBoardSubscriptionMode(rawMode)
					if mode != boardSubscriptionNone {
						subs[boardID] = mode
					}
				}
			}
		}
	}
	if len(subs) > 0 {
		return subs
	}
	key := boardWatchSettingKey(handle)
	if key == "" {
		return subs
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return subs
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		a.addAppError("board_watch.persistence", fmt.Errorf("decode watched boards for %s: %w", handle, err))
		return subs
	}
	for _, id := range ids {
		if id > 0 {
			subs[id] = boardSubscriptionWatch
		}
	}
	return subs
}

func (a *webApp) watchedBoardIDs(handle string) map[int64]bool {
	out := map[int64]bool{}
	for boardID, mode := range a.boardSubscriptions(handle) {
		if mode == boardSubscriptionWatch {
			out[boardID] = true
		}
	}
	return out
}

func (a *webApp) boardSubscriptionIDs(handle string, modes ...boardSubscriptionMode) map[int64]bool {
	if len(modes) == 0 {
		return map[int64]bool{}
	}
	allowed := map[boardSubscriptionMode]bool{}
	for _, mode := range modes {
		if mode != boardSubscriptionNone {
			allowed[mode] = true
		}
	}
	out := map[int64]bool{}
	for boardID, mode := range a.boardSubscriptions(handle) {
		if allowed[mode] {
			out[boardID] = true
		}
	}
	return out
}

func (a *webApp) boardSubscriptionStats(handle string) []boardSubscriptionStat {
	counts := map[boardSubscriptionMode]int{
		boardSubscriptionWatch:  0,
		boardSubscriptionDigest: 0,
		boardSubscriptionMute:   0,
	}
	for _, mode := range a.boardSubscriptions(handle) {
		if mode == boardSubscriptionNone {
			continue
		}
		counts[mode]++
	}
	return []boardSubscriptionStat{
		{Label: "watch", Mode: boardSubscriptionWatch, Count: counts[boardSubscriptionWatch]},
		{Label: "digest", Mode: boardSubscriptionDigest, Count: counts[boardSubscriptionDigest]},
		{Label: "mute", Mode: boardSubscriptionMute, Count: counts[boardSubscriptionMute]},
	}
}

func (a *webApp) persistWatchedBoardIDs(handle string, watched map[int64]bool) {
	subs := map[int64]boardSubscriptionMode{}
	for boardID, enabled := range watched {
		if enabled {
			subs[boardID] = boardSubscriptionWatch
		}
	}
	a.persistBoardSubscriptions(handle, subs)
}

func (a *webApp) persistBoardSubscriptions(handle string, subs map[int64]boardSubscriptionMode) {
	if a.adminRepo == nil {
		return
	}
	key := boardSubscriptionSettingKey(handle)
	if key == "" {
		return
	}
	encoded := map[string]string{}
	watchIDs := make([]int64, 0, len(subs))
	for boardID, mode := range subs {
		mode = normalizeBoardSubscriptionMode(string(mode))
		if boardID <= 0 || mode == boardSubscriptionNone {
			continue
		}
		encoded[strconv.FormatInt(boardID, 10)] = string(mode)
		if mode == boardSubscriptionWatch {
			watchIDs = append(watchIDs, boardID)
		}
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("board_watch.persistence", fmt.Errorf("encode board subscriptions for %s: %w", handle, err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(key, body)

	sort.Slice(watchIDs, func(i, j int) bool { return watchIDs[i] < watchIDs[j] })
	legacyBody := ""
	if len(watchIDs) > 0 {
		raw, err := json.Marshal(watchIDs)
		if err != nil {
			a.addAppError("board_watch.persistence", fmt.Errorf("encode legacy watch list for %s: %w", handle, err))
			return
		}
		legacyBody = string(raw)
	}
	a.persistSystemSetting(boardWatchSettingKey(handle), legacyBody)
}

func (a *webApp) setBoardSubscription(handle string, boardID int64, mode boardSubscriptionMode) {
	handle = normalizeHandleKey(handle)
	if handle == "" || boardID <= 0 {
		return
	}
	rows := a.boardSubscriptions(handle)
	mode = normalizeBoardSubscriptionMode(string(mode))
	if mode == boardSubscriptionNone {
		delete(rows, boardID)
	} else {
		rows[boardID] = mode
	}
	a.persistBoardSubscriptions(handle, rows)
}

func (a *webApp) setBoardWatched(handle string, boardID int64, watched bool) {
	if watched {
		a.setBoardSubscription(handle, boardID, boardSubscriptionWatch)
		return
	}
	a.setBoardSubscription(handle, boardID, boardSubscriptionNone)
}

func filterBoardsByWatch(boards []domain.Board, watched map[int64]bool) []domain.Board {
	if len(watched) == 0 {
		return nil
	}
	out := make([]domain.Board, 0, len(boards))
	for _, board := range boards {
		if watched[board.ID] {
			out = append(out, board)
		}
	}
	return out
}

func normalizeEventCategory(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "system", "social", "door", "tournament", "content", "ops":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "social"
	}
}

func normalizeEventRecurrence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "daily", "weekly", "monthly", "weekdays":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func parseLocalDateTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("datetime is required")
	}
	return time.ParseInLocation("2006-01-02T15:04", raw, time.Local)
}

func randomEventID() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	}
	return hex.EncodeToString(buf)
}

func (a *webApp) loadCommunityEvents() []communityEvent {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingCommunityEvents)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []communityEvent
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("community.events", fmt.Errorf("decode community events: %w", err))
		return nil
	}
	filtered := make([]communityEvent, 0, len(rows))
	for _, row := range rows {
		row.ID = strings.TrimSpace(row.ID)
		row.Title = strings.TrimSpace(row.Title)
		if row.ID == "" || row.Title == "" || row.StartsAt.IsZero() {
			continue
		}
		if strings.TrimSpace(row.SeriesID) == "" {
			row.SeriesID = row.ID
		}
		row.Category = normalizeEventCategory(row.Category)
		row.Recurrence = normalizeEventRecurrence(row.Recurrence)
		if !row.RepeatUntil.IsZero() {
			row.RepeatUntil = row.RepeatUntil.UTC()
		}
		if row.CreatedAt.IsZero() {
			row.CreatedAt = row.StartsAt
		}
		filtered = append(filtered, row)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].StartsAt.Equal(filtered[j].StartsAt) {
			return strings.ToLower(filtered[i].Title) < strings.ToLower(filtered[j].Title)
		}
		return filtered[i].StartsAt.Before(filtered[j].StartsAt)
	})
	return filtered
}

func (a *webApp) persistCommunityEvents(rows []communityEvent) {
	if a.adminRepo == nil {
		return
	}
	for i := range rows {
		rows[i].ID = strings.TrimSpace(rows[i].ID)
		rows[i].SeriesID = strings.TrimSpace(rows[i].SeriesID)
		if rows[i].SeriesID == "" {
			rows[i].SeriesID = rows[i].ID
		}
		rows[i].Title = strings.TrimSpace(rows[i].Title)
		rows[i].Category = normalizeEventCategory(rows[i].Category)
		rows[i].Recurrence = normalizeEventRecurrence(rows[i].Recurrence)
		if !rows[i].RepeatUntil.IsZero() {
			rows[i].RepeatUntil = rows[i].RepeatUntil.UTC()
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].StartsAt.Equal(rows[j].StartsAt) {
			return strings.ToLower(rows[i].Title) < strings.ToLower(rows[j].Title)
		}
		return rows[i].StartsAt.Before(rows[j].StartsAt)
	})
	body := ""
	if len(rows) > 0 {
		raw, err := json.Marshal(rows)
		if err != nil {
			a.addAppError("community.events", fmt.Errorf("encode community events: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingCommunityEvents, body)
}

func recurrenceSummary(row communityEvent) string {
	if normalizeEventRecurrence(row.Recurrence) == "" {
		return ""
	}
	label := strings.Title(row.Recurrence)
	if row.RepeatUntil.IsZero() {
		return label + " series"
	}
	return label + " until " + row.RepeatUntil.Local().Format("2006-01-02 15:04")
}

func nextCommunityEventStart(start time.Time, recurrence string) time.Time {
	switch normalizeEventRecurrence(recurrence) {
	case "daily":
		return start.AddDate(0, 0, 1)
	case "weekly":
		return start.AddDate(0, 0, 7)
	case "monthly":
		return start.AddDate(0, 1, 0)
	case "weekdays":
		next := start.AddDate(0, 0, 1)
		for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
			next = next.AddDate(0, 0, 1)
		}
		return next
	default:
		return time.Time{}
	}
}

func eventDuration(row communityEvent) time.Duration {
	if row.EndsAt.IsZero() || !row.EndsAt.After(row.StartsAt) {
		return 0
	}
	return row.EndsAt.Sub(row.StartsAt)
}

func expandCommunityEvent(row communityEvent, windowStart, windowEnd time.Time, limit int) []communityEvent {
	if row.StartsAt.IsZero() || limit == 0 {
		return nil
	}
	if windowEnd.Before(windowStart) {
		windowEnd = windowStart
	}
	duration := eventDuration(row)
	current := row.StartsAt.UTC()
	repeatUntil := row.RepeatUntil.UTC()
	if normalizeEventRecurrence(row.Recurrence) != "" && repeatUntil.IsZero() {
		repeatUntil = current.Add(90 * 24 * time.Hour)
	}
	out := make([]communityEvent, 0, 4)
	for {
		occurrence := row
		occurrence.StartsAt = current
		if duration > 0 {
			occurrence.EndsAt = current.Add(duration)
		} else {
			occurrence.EndsAt = time.Time{}
		}
		occurrence.ID = row.ID + "@" + current.Format("20060102150405")
		occurrence.SeriesID = row.SeriesID
		end := occurrence.EndsAt
		if end.IsZero() {
			end = occurrence.StartsAt
		}
		if !end.Before(windowStart) && !occurrence.StartsAt.After(windowEnd) {
			out = append(out, occurrence)
			if limit > 0 && len(out) >= limit {
				return out
			}
		}
		recurrence := normalizeEventRecurrence(row.Recurrence)
		if recurrence == "" {
			break
		}
		next := nextCommunityEventStart(current, recurrence)
		if next.IsZero() || !next.After(current) {
			break
		}
		if !repeatUntil.IsZero() && next.After(repeatUntil) {
			break
		}
		if next.After(windowEnd.Add(35 * 24 * time.Hour)) {
			break
		}
		current = next
	}
	return out
}

func (a *webApp) upcomingCommunityEvents(limit int, now time.Time) []communityEvent {
	rows := a.loadCommunityEvents()
	out := make([]communityEvent, 0, len(rows))
	windowEnd := now.Add(120 * 24 * time.Hour)
	for _, row := range rows {
		for _, occurrence := range expandCommunityEvent(row, now.Add(-2*time.Hour), windowEnd, limit) {
			end := occurrence.EndsAt
			if end.IsZero() {
				end = occurrence.StartsAt
			}
			if end.Before(now) {
				continue
			}
			out = append(out, occurrence)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartsAt.Equal(out[j].StartsAt) {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		}
		return out[i].StartsAt.Before(out[j].StartsAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (a *webApp) recentCommunityEvents(limit int, now time.Time) []communityEvent {
	rows := a.loadCommunityEvents()
	out := make([]communityEvent, 0, len(rows))
	windowStart := now.Add(-14 * 24 * time.Hour)
	for _, row := range rows {
		for _, occurrence := range expandCommunityEvent(row, windowStart, now, limit) {
			end := occurrence.EndsAt
			if end.IsZero() {
				end = occurrence.StartsAt
			}
			if end.After(now) || end.Before(windowStart) {
				continue
			}
			out = append(out, occurrence)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].EndsAt
		if left.IsZero() {
			left = out[i].StartsAt
		}
		right := out[j].EndsAt
		if right.IsZero() {
			right = out[j].StartsAt
		}
		if left.Equal(right) {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		}
		return left.After(right)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func formatCommunityEventWindow(row communityEvent) string {
	start := row.StartsAt.Local()
	if row.EndsAt.IsZero() || row.EndsAt.Equal(row.StartsAt) {
		return start.Format("Mon Jan 2, 2006 15:04")
	}
	end := row.EndsAt.Local()
	if start.Format("2006-01-02") == end.Format("2006-01-02") {
		return start.Format("Mon Jan 2, 2006 15:04") + " - " + end.Format("15:04")
	}
	return start.Format("Mon Jan 2, 2006 15:04") + " - " + end.Format("Mon Jan 2, 2006 15:04")
}

func (a *webApp) ensureAttentionDismissalsLoaded(handle string) {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return
	}

	a.Lock()
	if a.attentionLoaded == nil {
		a.attentionLoaded = map[string]bool{}
	}
	if a.attentionDismissed == nil {
		a.attentionDismissed = map[string]map[string]time.Time{}
	}
	if a.attentionLoaded[handle] {
		a.Unlock()
		return
	}
	a.attentionLoaded[handle] = true
	a.Unlock()

	rows := map[string]time.Time{}
	if a.adminRepo != nil {
		raw, err := a.adminRepo.GetSystemSetting(attentionDismissedSettingKey(handle))
		if err == nil && strings.TrimSpace(raw) != "" {
			decoded := map[string]string{}
			if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
				a.addAppError("attention.persistence", fmt.Errorf("decode dismissals for %s: %w", handle, err))
			} else {
				cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
				for key, value := range decoded {
					at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
					if err != nil || !at.After(cutoff) {
						continue
					}
					rows[strings.TrimSpace(key)] = at.UTC()
				}
			}
		}
	}

	a.Lock()
	if a.attentionDismissed == nil {
		a.attentionDismissed = map[string]map[string]time.Time{}
	}
	if len(rows) == 0 {
		delete(a.attentionDismissed, handle)
	} else {
		a.attentionDismissed[handle] = rows
	}
	a.Unlock()
}

func (a *webApp) persistAttentionDismissals(handle string) {
	handle = normalizeHandleKey(handle)
	if handle == "" || a.adminRepo == nil {
		return
	}
	a.ensureAttentionDismissalsLoaded(handle)

	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	type dismissalRow struct {
		Key string
		At  time.Time
	}

	a.Lock()
	source := a.attentionDismissed[handle]
	rows := make([]dismissalRow, 0, len(source))
	for key, at := range source {
		key = strings.TrimSpace(key)
		if key == "" || !at.After(cutoff) {
			continue
		}
		rows = append(rows, dismissalRow{Key: key, At: at.UTC()})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].At.Equal(rows[j].At) {
			return rows[i].Key < rows[j].Key
		}
		return rows[i].At.After(rows[j].At)
	})
	if len(rows) > maxAttentionDismissedItems {
		rows = rows[:maxAttentionDismissedItems]
	}
	if a.attentionDismissed == nil {
		a.attentionDismissed = map[string]map[string]time.Time{}
	}
	if len(rows) == 0 {
		delete(a.attentionDismissed, handle)
	} else {
		pruned := make(map[string]time.Time, len(rows))
		for _, row := range rows {
			pruned[row.Key] = row.At
		}
		a.attentionDismissed[handle] = pruned
	}
	a.Unlock()

	encoded := map[string]string{}
	for _, row := range rows {
		encoded[row.Key] = row.At.Format(time.RFC3339Nano)
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("attention.persistence", fmt.Errorf("encode dismissals for %s: %w", handle, err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(attentionDismissedSettingKey(handle), body)
}

func (a *webApp) isAttentionKeyDismissed(handle, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return false
	}
	a.ensureAttentionDismissalsLoaded(handle)
	persist := false
	a.Lock()
	if a.attentionDismissed == nil {
		a.attentionDismissed = map[string]map[string]time.Time{}
	}
	rows := a.attentionDismissed[handle]
	if len(rows) == 0 {
		a.Unlock()
		return false
	}
	at, ok := rows[key]
	if !ok {
		a.Unlock()
		return false
	}
	if time.Since(at) > 7*24*time.Hour {
		delete(rows, key)
		if len(rows) == 0 {
			delete(a.attentionDismissed, handle)
		}
		persist = true
	}
	a.Unlock()
	if persist {
		a.persistAttentionDismissals(handle)
		return false
	}
	return true
}

func (a *webApp) isAttentionDismissed(handle string, item discovery.Item) bool {
	return a.isAttentionKeyDismissed(handle, attentionItemKey(item))
}

func (a *webApp) dismissAttentionItem(handle, key string) {
	handle = normalizeHandleKey(handle)
	key = strings.TrimSpace(key)
	if handle == "" || key == "" {
		return
	}
	a.ensureAttentionDismissalsLoaded(handle)
	a.Lock()
	if a.attentionDismissed == nil {
		a.attentionDismissed = map[string]map[string]time.Time{}
	}
	if a.attentionDismissed[handle] == nil {
		a.attentionDismissed[handle] = map[string]time.Time{}
	}
	a.attentionDismissed[handle][key] = time.Now().UTC()
	a.Unlock()
	a.persistAttentionDismissals(handle)
}

func (a *webApp) clearAttentionDismissals(handle string) {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return
	}
	a.ensureAttentionDismissalsLoaded(handle)
	a.Lock()
	if a.attentionDismissed == nil {
		a.Unlock()
		return
	}
	delete(a.attentionDismissed, handle)
	a.Unlock()
	a.persistAttentionDismissals(handle)
}

func attentionReadSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingAttentionReadRoot + handle
}

func (a *webApp) ensureAttentionReadLoaded(handle string) {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return
	}

	a.Lock()
	if a.attentionReadLoaded == nil {
		a.attentionReadLoaded = map[string]bool{}
	}
	if a.attentionRead == nil {
		a.attentionRead = map[string]map[string]time.Time{}
	}
	if a.attentionReadLoaded[handle] {
		a.Unlock()
		return
	}
	a.attentionReadLoaded[handle] = true
	a.Unlock()

	rows := map[string]time.Time{}
	if a.adminRepo != nil {
		raw, err := a.adminRepo.GetSystemSetting(attentionReadSettingKey(handle))
		if err == nil && strings.TrimSpace(raw) != "" {
			decoded := map[string]string{}
			if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
				a.addAppError("attention.persistence", fmt.Errorf("decode reads for %s: %w", handle, err))
			} else {
				cutoff := time.Now().UTC().Add(-14 * 24 * time.Hour)
				for key, value := range decoded {
					at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
					if err != nil || !at.After(cutoff) {
						continue
					}
					rows[strings.TrimSpace(key)] = at.UTC()
				}
			}
		}
	}

	a.Lock()
	if a.attentionRead == nil {
		a.attentionRead = map[string]map[string]time.Time{}
	}
	if len(rows) == 0 {
		delete(a.attentionRead, handle)
	} else {
		a.attentionRead[handle] = rows
	}
	a.Unlock()
}

func (a *webApp) persistAttentionReads(handle string) {
	handle = normalizeHandleKey(handle)
	if handle == "" || a.adminRepo == nil {
		return
	}
	a.ensureAttentionReadLoaded(handle)

	cutoff := time.Now().UTC().Add(-14 * 24 * time.Hour)
	type readRow struct {
		Key string
		At  time.Time
	}

	a.Lock()
	source := a.attentionRead[handle]
	rows := make([]readRow, 0, len(source))
	for key, at := range source {
		key = strings.TrimSpace(key)
		if key == "" || !at.After(cutoff) {
			continue
		}
		rows = append(rows, readRow{Key: key, At: at.UTC()})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].At.Equal(rows[j].At) {
			return rows[i].Key < rows[j].Key
		}
		return rows[i].At.After(rows[j].At)
	})
	if len(rows) > maxAttentionReadItems {
		rows = rows[:maxAttentionReadItems]
	}
	if a.attentionRead == nil {
		a.attentionRead = map[string]map[string]time.Time{}
	}
	if len(rows) == 0 {
		delete(a.attentionRead, handle)
	} else {
		pruned := make(map[string]time.Time, len(rows))
		for _, row := range rows {
			pruned[row.Key] = row.At
		}
		a.attentionRead[handle] = pruned
	}
	a.Unlock()

	encoded := map[string]string{}
	for _, row := range rows {
		encoded[row.Key] = row.At.Format(time.RFC3339Nano)
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("attention.persistence", fmt.Errorf("encode reads for %s: %w", handle, err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(attentionReadSettingKey(handle), body)
}

func (a *webApp) isAttentionKeyRead(handle, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return false
	}
	a.ensureAttentionReadLoaded(handle)
	persist := false
	a.Lock()
	if a.attentionRead == nil {
		a.attentionRead = map[string]map[string]time.Time{}
	}
	rows := a.attentionRead[handle]
	if len(rows) == 0 {
		a.Unlock()
		return false
	}
	at, ok := rows[key]
	if !ok {
		a.Unlock()
		return false
	}
	if time.Since(at) > 14*24*time.Hour {
		delete(rows, key)
		if len(rows) == 0 {
			delete(a.attentionRead, handle)
		}
		persist = true
	}
	a.Unlock()
	if persist {
		a.persistAttentionReads(handle)
		return false
	}
	return true
}

func (a *webApp) markAttentionItemRead(handle, key string) {
	handle = normalizeHandleKey(handle)
	key = strings.TrimSpace(key)
	if handle == "" || key == "" {
		return
	}
	a.ensureAttentionReadLoaded(handle)
	a.Lock()
	if a.attentionRead == nil {
		a.attentionRead = map[string]map[string]time.Time{}
	}
	if a.attentionRead[handle] == nil {
		a.attentionRead[handle] = map[string]time.Time{}
	}
	a.attentionRead[handle][key] = time.Now().UTC()
	a.Unlock()
	a.persistAttentionReads(handle)
}

func (a *webApp) markAttentionItemUnread(handle, key string) {
	handle = normalizeHandleKey(handle)
	key = strings.TrimSpace(key)
	if handle == "" || key == "" {
		return
	}
	a.ensureAttentionReadLoaded(handle)
	a.Lock()
	if a.attentionRead == nil {
		a.Unlock()
		return
	}
	if rows := a.attentionRead[handle]; rows != nil {
		delete(rows, key)
		if len(rows) == 0 {
			delete(a.attentionRead, handle)
		}
	}
	a.Unlock()
	a.persistAttentionReads(handle)
}

func (a *webApp) markAttentionItemsRead(handle string, keys []string) int {
	count := 0
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		a.markAttentionItemRead(handle, key)
		count++
	}
	return count
}

func (a *webApp) pruneExpiredWebSessions() int {
	now := time.Now()
	a.Lock()
	defer a.Unlock()
	pruned := 0
	for sid, state := range a.sessions {
		if now.After(state.expire) {
			delete(a.sessions, sid)
			pruned++
		}
	}
	return pruned
}

func (a *webApp) countWebSessions() int {
	a.Lock()
	defer a.Unlock()
	return len(a.sessions)
}

func (a *webApp) clearAllRateLimits() int {
	a.Lock()
	defer a.Unlock()
	cleared := len(a.rateLimits)
	a.rateLimits = map[string][]time.Time{}
	return cleared
}

func (a *webApp) countRateLimits() int {
	a.Lock()
	defer a.Unlock()
	return len(a.rateLimits)
}

func boolToText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func remoteHostDisplay(remoteAddr string) string {
	host := strings.TrimSpace(netutil.RemoteHost(remoteAddr))
	if host == "" {
		return "unknown"
	}
	return host
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

func sanitizedConfiguredHost(raw string) string {
	raw = strings.TrimSpace(strings.TrimSuffix(raw, "/"))
	if raw == "" {
		return "localhost"
	}
	if strings.Contains(raw, "://") {
		if parsed, err := url.Parse(raw); err == nil && strings.TrimSpace(parsed.Host) != "" {
			return strings.TrimSpace(parsed.Host)
		}
	}
	if parsed, err := url.Parse("//" + raw); err == nil && strings.TrimSpace(parsed.Host) != "" {
		return strings.TrimSpace(parsed.Host)
	}
	return "localhost"
}

func normalizedPublicURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || strings.TrimSpace(parsed.Host) == "" {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return strings.TrimRight(parsed.String(), "/")
}

func parseCSVStrings(raw string) []string {
	out := make([]string, 0, 8)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func (a *webApp) clientAddress(r *http.Request) string {
	if r == nil {
		return ""
	}
	if a != nil && a.proxyResolver != nil {
		if resolved := strings.TrimSpace(a.proxyResolver.Resolve(r.RemoteAddr, r.Header)); resolved != "" {
			return resolved
		}
	}
	return strings.TrimSpace(netutil.RemoteHost(r.RemoteAddr))
}

func (a *webApp) rateLimitKey(prefix string, r *http.Request) string {
	client := strings.ToLower(strings.TrimSpace(a.clientAddress(r)))
	if client == "" {
		client = "unknown"
	}
	return prefix + ":" + client
}

func (a *webApp) allowRateLimitedAction(key string, limit int, window time.Duration, consume bool) bool {
	key = strings.TrimSpace(key)
	if key == "" || limit <= 0 || window <= 0 {
		return true
	}
	now := time.Now().UTC()
	cutoff := now.Add(-window)

	a.Lock()
	defer a.Unlock()
	if a.rateLimits == nil {
		a.rateLimits = map[string][]time.Time{}
	}
	rows := a.rateLimits[key]
	kept := make([]time.Time, 0, len(rows)+1)
	for _, row := range rows {
		if row.After(cutoff) {
			kept = append(kept, row)
		}
	}
	if len(kept) >= limit {
		a.rateLimits[key] = kept
		return false
	}
	if consume {
		kept = append(kept, now)
	}
	if len(kept) == 0 {
		delete(a.rateLimits, key)
	} else {
		a.rateLimits[key] = kept
	}
	return true
}

func (a *webApp) clearRateLimitedAction(key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	a.Lock()
	delete(a.rateLimits, key)
	a.Unlock()
}

func (a *webApp) isSafeDevInboundRemote(r *http.Request) bool {
	if a == nil || strings.TrimSpace(a.inboundToken) != defaultInboundToken {
		return true
	}
	origin := netutil.RemoteOrigin(a.clientAddress(r))
	return origin == "loopback" || origin == "lan"
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

	chatEmptyHelper := ``
	if len(a.chatSvc.History("#lobby", 1)) == 0 {
		chatEmptyHelper = a.renderRoleAwareEmptyState(user, "chat")
	}

	chatPage := `<!doctype html>
	<html>
	<body>
		<h1>` + htmlEscape(a.siteDisplayName()) + ` Chat</h1>
		<p>Logged in as ` + user.Handle + `</p>
		<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/settings">settings</a> | <a href="/doors">doors</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
		<p><strong>Quick keys:</strong> Enter sends message, Ctrl+L clears chat pane, channel selector switches rooms instantly.</p>
		` + chatEmptyHelper + `
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
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJSONRequestBytes+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > maxJSONRequestBytes {
		return fmt.Errorf("request body exceeds limit (%d bytes)", maxJSONRequestBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra interface{}
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("invalid json body")
	}
	return nil
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
	if !a.isSafeDevInboundRemote(r) {
		http.Error(w, "inbound token must be customized before public exposure", http.StatusForbidden)
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

func renderPageBanner(kind, text string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "error" {
		kind = "notice"
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	title := "Notice"
	if kind == "error" {
		title = "Error"
	}
	return `<section class="wolfbbs-banner" data-wolfbbs-flash="1" data-kind="` + htmlEscape(kind) + `"><div><strong>` + title + `:</strong><p>` + htmlEscape(text) + `</p></div></section>`
}

func renderActiveFilterPanel(title, resetPath string, items []string) string {
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	chips := strings.Builder{}
	for _, item := range filtered {
		chips.WriteString(`<span class="wolfbbs-chip">` + htmlEscape(item) + `</span>`)
	}
	resetLink := ``
	if strings.TrimSpace(resetPath) != "" {
		resetLink = ` <a href="` + htmlEscape(resetPath) + `">Clear filters</a>`
	}
	return `<section class="wolfbbs-card wolfbbs-filter-summary"><h2>` + htmlEscape(title) + `</h2><div class="wolfbbs-chip-row">` + chips.String() + `</div><p class="wolfbbs-muted">These filters are currently narrowing the page.` + resetLink + `</p></section>`
}

func pageMessageBlock(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	notice := strings.TrimSpace(r.URL.Query().Get("notice"))
	errText := strings.TrimSpace(r.URL.Query().Get("error"))
	out := strings.Builder{}
	if notice != "" {
		out.WriteString(renderPageBanner("notice", notice))
	}
	if errText != "" {
		out.WriteString(renderPageBanner("error", errText))
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

func seedDefaultBoards(repo repository.BoardRepository) (int, error) {
	if repo == nil {
		return 0, fmt.Errorf("board repository is required")
	}
	boards, err := repo.List()
	if err != nil {
		return 0, err
	}
	existing := make(map[string]struct{}, len(boards))
	for _, board := range boards {
		name := strings.ToLower(strings.TrimSpace(board.Name))
		if name == "" {
			continue
		}
		existing[name] = struct{}{}
	}
	seed := []domain.Board{
		{Name: "General", Description: "General system discussion", CreatedBy: 1},
		{Name: "Node Talk", Description: "Node status and operator chat", CreatedBy: 1},
		{Name: "Tooling", Description: "Build scripts and deployment", CreatedBy: 1},
	}
	created := 0
	for i := range seed {
		name := strings.ToLower(strings.TrimSpace(seed[i].Name))
		if _, ok := existing[name]; ok {
			continue
		}
		if err := repo.Create(&seed[i]); err != nil {
			return created, err
		}
		existing[name] = struct{}{}
		created++
	}
	return created, nil
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
	case "start", "home", "onramp":
		return "/start"
	case "attention", "attn", "queue":
		return "/attention"
	case "today", "brief", "daily", "t":
		return "/today"
	case "events", "calendar", "event", "e":
		return "/events"
	case "boards", "messages", "msg", "m":
		return "/boards"
	case "mail", "pm", "p":
		return "/mail"
	case "chat", "c":
		return "/chat"
	case "bulletins", "bulletin", "news", "b":
		return "/bulletins"
	case "directory", "users", "dir", "u":
		return "/directory"
	case "finder", "search", "find":
		return "/finder"
	case "newfiles", "files", "nf":
		return "/newfiles"
	case "feedback", "fb":
		return "/feedback"
	case "radar", "mission", "r":
		return "/radar"
	case "clubhouse", "community", "club":
		return "/clubhouse"
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
		return "/doors"
	case "admin", "a":
		return "/admin"
	case "ops":
		return "/admin/ops"
	case "admin-events", "sysop-events":
		return "/admin/events"
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

func (a *webApp) mustDoorAchievements(userID int64, limit int) []domain.DoorAchievement {
	if a.doorRegistry == nil {
		return nil
	}
	rows, err := a.doorRegistry.ListAchievements(userID, "", limit)
	if err != nil {
		return nil
	}
	return rows
}

func (a *webApp) buildDoorCatalog(user *domain.User) []webDoorView {
	if user == nil || a.doorRegistry == nil {
		return nil
	}
	favoriteRows, _ := a.doorRegistry.ListFavorites(user.ID, 256)
	recentRows, _ := a.doorRegistry.ListRecent(user.ID, 256)
	favoriteSet := map[string]bool{}
	recentMeta := map[string]domain.DoorUserMeta{}
	for _, row := range favoriteRows {
		favoriteSet[row.DoorID] = row.Favorite
	}
	for _, row := range recentRows {
		recentMeta[row.DoorID] = row
	}
	handleByID := a.userHandleLookup()
	now := time.Now()
	catalog := make([]webDoorView, 0, len(a.doorRegistry.Doors()))
	for _, door := range a.doorRegistry.Doors() {
		view := webDoorView{Door: door}
		if turns, err := a.doorRegistry.TurnsRemaining(user.ID, door.ID, now); err == nil && turns > 0 {
			view.TurnsRemaining = turns
		}
		view.Favorite = favoriteSet[door.ID]
		if meta, ok := recentMeta[door.ID]; ok {
			view.Recent = true
			view.PlayCount = meta.PlayCount
			if meta.LastPlayedAt != nil && !meta.LastPlayedAt.IsZero() {
				view.LastPlayed = meta.LastPlayedAt.Local().Format("2006-01-02 15:04")
			}
		}
		if rows, err := a.doorRegistry.ListAchievements(user.ID, door.ID, 50); err == nil {
			view.PersonalAchievements = len(rows)
		}
		if rows, err := a.doorRegistry.ListScores(door.ID, 1); err == nil && len(rows) > 0 {
			view.TopScore = rows[0].Value
			view.TopScoreHandle = handleByID[rows[0].UserID]
		}
		if stats, err := a.doorRegistry.GetUsageStats(door.ID); err == nil && stats != nil {
			view.DailyActive = stats.DailyActive
			view.MonthlyActive = stats.MonthlyActive
			view.TotalPlays = stats.TotalPlays
		}
		score := 0
		if view.Favorite {
			score += 100
		}
		if view.Recent {
			score += 65
		}
		score += minInt(view.PersonalAchievements, 5) * 7
		score += minInt(view.PlayCount, 10) * 3
		score += minInt(view.TurnsRemaining, 5) * 2
		score += minInt(int(view.TotalPlays), 18)
		score += minInt(view.DailyActive, 6) * 2
		if score == 0 {
			score = 5 + minInt(int(view.TotalPlays), 10) + minInt(view.DailyActive, 3)
		}
		view.RecommendedScore = score
		catalog = append(catalog, view)
	}
	sort.Slice(catalog, func(i, j int) bool {
		if catalog[i].Favorite != catalog[j].Favorite {
			return catalog[i].Favorite
		}
		if strings.ToLower(catalog[i].Door.Category) != strings.ToLower(catalog[j].Door.Category) {
			return strings.ToLower(catalog[i].Door.Category) < strings.ToLower(catalog[j].Door.Category)
		}
		return strings.ToLower(catalog[i].Door.Name) < strings.ToLower(catalog[j].Door.Name)
	})
	return catalog
}

func topRecommendedDoors(catalog []webDoorView, limit int) []webDoorView {
	if limit <= 0 || len(catalog) == 0 {
		return nil
	}
	rows := append([]webDoorView(nil), catalog...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].RecommendedScore != rows[j].RecommendedScore {
			return rows[i].RecommendedScore > rows[j].RecommendedScore
		}
		if rows[i].TurnsRemaining != rows[j].TurnsRemaining {
			return rows[i].TurnsRemaining > rows[j].TurnsRemaining
		}
		if rows[i].Favorite != rows[j].Favorite {
			return rows[i].Favorite
		}
		return strings.ToLower(rows[i].Door.Name) < strings.ToLower(rows[j].Door.Name)
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func filterDoorViews(catalog []webDoorView, q, category, mode string) []webDoorView {
	q = strings.ToLower(strings.TrimSpace(q))
	category = strings.ToLower(strings.TrimSpace(category))
	mode = strings.ToLower(strings.TrimSpace(mode))
	out := make([]webDoorView, 0, len(catalog))
	recommendedIDs := map[string]bool{}
	for _, row := range topRecommendedDoors(catalog, len(catalog)) {
		recommendedIDs[row.Door.ID] = true
	}
	for _, row := range catalog {
		if category != "" && strings.ToLower(strings.TrimSpace(row.Door.Category)) != category {
			continue
		}
		switch mode {
		case "favorites":
			if !row.Favorite {
				continue
			}
		case "recent":
			if !row.Recent {
				continue
			}
		case "recommended":
			if !recommendedIDs[row.Door.ID] {
				continue
			}
		}
		if q != "" {
			haystack := strings.ToLower(strings.Join([]string{
				row.Door.ID,
				row.Door.Name,
				row.Door.Category,
				row.Door.Description,
			}, " "))
			if !strings.Contains(haystack, q) {
				continue
			}
		}
		out = append(out, row)
	}
	return out
}

func doorCategories(catalog []webDoorView) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(catalog))
	for _, row := range catalog {
		category := strings.ToLower(strings.TrimSpace(row.Door.Category))
		if category == "" {
			continue
		}
		if _, ok := seen[category]; ok {
			continue
		}
		seen[category] = struct{}{}
		out = append(out, category)
	}
	sort.Strings(out)
	return out
}

func (a *webApp) buildBoardsDashboard(user *domain.User, boards []domain.Board) boardsDashboardSnapshot {
	snapshot := boardsDashboardSnapshot{VisibleBoards: len(boards)}
	if user == nil {
		return snapshot
	}
	if a.msgRepo != nil {
		for _, board := range boards {
			msgs, err := a.msgRepo.ListByBoard(board.ID)
			if err != nil {
				continue
			}
			pointerID := int64(0)
			if ptr, ptrErr := a.msgRepo.GetPointer(user.ID, board.ID); ptrErr == nil && ptr != nil {
				pointerID = ptr.LastReadID
			}
			for _, msg := range msgs {
				if msg.ID > pointerID {
					snapshot.UnreadPosts++
				}
			}
		}
	}
	if a.mailRepo != nil {
		if inbox, err := a.mailRepo.ListInbox(user.ID, 100); err == nil {
			for _, row := range inbox {
				if row.ReadAt == nil {
					snapshot.UnreadMail++
				}
			}
		}
	}
	if a.chatSvc != nil {
		snapshot.OnlineUsers = len(a.chatSvc.Online())
	}
	if a.doorRegistry != nil {
		if favorites, err := a.doorRegistry.ListFavorites(user.ID, 100); err == nil {
			snapshot.FavoriteDoors = len(favorites)
		}
		if picks := topRecommendedDoors(a.buildDoorCatalog(user), 1); len(picks) > 0 {
			snapshot.RecommendedDoor = picks[0].Door.Name
			snapshot.RecommendedDoorID = picks[0].Door.ID
		}
	}
	if a.adminRepo != nil {
		if callers, err := a.adminRepo.ListCallerHistory(4); err == nil {
			for _, row := range callers {
				snapshot.RecentCallers = append(snapshot.RecentCallers, row.Username+" from "+remoteHostDisplay(row.RemoteAddr))
			}
		}
	}
	if a.oneLinerzMod != nil {
		for _, row := range a.oneLinerzMod.List(4) {
			snapshot.OneLiners = append(snapshot.OneLiners, row.Handle+": "+cleanOneLiner(row.Text, 72))
		}
	}
	return snapshot
}

func (a *webApp) buildRadarSnapshot(user *domain.User) radarSnapshot {
	snapshot := radarSnapshot{}
	if user == nil {
		return snapshot
	}
	boards := []domain.Board{}
	if a.boardRepo != nil {
		if rows, err := a.boardRepo.List(); err == nil {
			for i := range rows {
				if a.canReadBoard(user, &rows[i]) {
					boards = append(boards, rows[i])
				}
			}
		}
	}
	dashboard := a.buildBoardsDashboard(user, boards)
	snapshot.UnreadMail = dashboard.UnreadMail
	snapshot.UnreadPosts = dashboard.UnreadPosts
	snapshot.OnlineUsers = dashboard.OnlineUsers
	snapshot.TrackedBoards = len(boards)
	snapshot.BoardPulse = a.buildBoardPulse(user, boards, 8)
	snapshot.RecommendedDoors = topRecommendedDoors(a.buildDoorCatalog(user), 4)
	snapshot.RecentAchievements = a.mustDoorAchievements(user.ID, 6)
	if a.rumorzMod != nil {
		snapshot.Rumor = strings.TrimSpace(a.rumorzMod.Current())
	}
	if digest, err := discovery.BuildSinceLastCall(a.boardRepo, a.msgRepo, a.mailRepo, user, 6); err == nil {
		for _, row := range digest.Items {
			snapshot.ActivityItems = append(snapshot.ActivityItems, row.Line)
		}
	}
	if a.adminRepo != nil {
		if rows, err := a.adminRepo.ListNodeSessions(12); err == nil {
			now := time.Now().UTC()
			for _, row := range rows {
				idle := now.Sub(row.LastActivity)
				if idle < 0 {
					idle = 0
				}
				snapshot.LiveCallers = append(snapshot.LiveCallers, callerRadarRow{
					Handle: row.Username,
					Node:   "Node " + strconv.Itoa(row.NodeID),
					Area:   row.Area,
					Since:  row.LoginAt.Local().Format("2006-01-02 15:04"),
					Idle:   formatDurationCompact(idle),
					Origin: strings.ToUpper(netutil.RemoteOrigin(row.RemoteAddr)),
					From:   remoteHostDisplay(row.RemoteAddr),
				})
			}
		}
		if rows, err := a.adminRepo.ListCallerHistory(8); err == nil {
			for _, row := range rows {
				snapshot.RecentCallers = append(snapshot.RecentCallers, callerRadarRow{
					Handle:   row.Username,
					Node:     "Node " + strconv.Itoa(row.NodeID),
					Area:     row.Area,
					Since:    row.LogoutAt.Local().Format("2006-01-02 15:04"),
					Origin:   strings.ToUpper(netutil.RemoteOrigin(row.RemoteAddr)),
					From:     remoteHostDisplay(row.RemoteAddr),
					Duration: formatDurationCompact(time.Duration(row.DurationSeconds) * time.Second),
				})
			}
		}
	}
	if len(snapshot.LiveCallers) == 0 && a.chatSvc != nil {
		for _, row := range a.chatSvc.Online() {
			snapshot.LiveCallers = append(snapshot.LiveCallers, callerRadarRow{
				Handle: row.Nick,
				Node:   row.Node,
				Area:   row.Area,
				Since:  row.LoginAt.Local().Format("2006-01-02 15:04"),
				Idle:   strconv.Itoa(row.IdleSec) + "s",
				Origin: "UNKNOWN",
				From:   "n/a",
			})
		}
	}
	snapshot.LiveNodes = len(snapshot.LiveCallers)
	return snapshot
}

func (a *webApp) buildBoardPulse(user *domain.User, boards []domain.Board, limit int) []boardPulseRow {
	if user == nil || len(boards) == 0 || a.msgRepo == nil {
		return nil
	}
	now := time.Now().UTC()
	out := make([]boardPulseRow, 0, len(boards))
	for _, board := range boards {
		msgs, err := a.msgRepo.ListByBoard(board.ID)
		if err != nil {
			continue
		}
		pointerID := int64(0)
		if ptr, ptrErr := a.msgRepo.GetPointer(user.ID, board.ID); ptrErr == nil && ptr != nil {
			pointerID = ptr.LastReadID
		}
		row := boardPulseRow{
			BoardID:      board.ID,
			BoardName:    board.Name,
			Conference:   board.Conference,
			MessageCount: len(msgs),
		}
		if len(msgs) > 0 {
			last := msgs[len(msgs)-1]
			row.LastAt = last.CreatedAt.Local().Format("2006-01-02 15:04")
			row.LastSubject = cleanOneLiner(last.Subject, 72)
			recencyBoost := 0
			if delta := now.Sub(last.CreatedAt); delta < 24*time.Hour {
				recencyBoost = 4
			} else if delta < 72*time.Hour {
				recencyBoost = 2
			}
			row.Heat += recencyBoost
		}
		for _, msg := range msgs {
			if msg.ID > pointerID {
				row.NewCount++
			}
		}
		row.Heat += row.NewCount*3 + minInt(row.MessageCount, 12)
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Heat != out[j].Heat {
			return out[i].Heat > out[j].Heat
		}
		if out[i].NewCount != out[j].NewCount {
			return out[i].NewCount > out[j].NewCount
		}
		return strings.ToLower(out[i].BoardName) < strings.ToLower(out[j].BoardName)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (a *webApp) buildScoreboardSnapshot(user *domain.User, filterDoor string) scoreboardSnapshot {
	snapshot := scoreboardSnapshot{FilterDoor: filterDoor}
	if a.doorRegistry == nil {
		return snapshot
	}
	lookup := a.userHandleLookup()
	personalBest := map[string]scoreChampion{}
	for _, door := range a.doorRegistry.Doors() {
		if filterDoor != "" && !strings.EqualFold(door.ID, filterDoor) {
			continue
		}
		scores, err := a.doorRegistry.ListScores(door.ID, 20)
		if err != nil || len(scores) == 0 {
			continue
		}
		snapshot.DoorsWithScores++
		snapshot.VisibleScoreRows += len(scores)
		top := scores[0]
		handle := lookup[top.UserID]
		if handle == "" {
			handle = "uid:" + strconv.FormatInt(top.UserID, 10)
		}
		snapshot.ChampionRows = append(snapshot.ChampionRows, scoreChampion{
			DoorID:    door.ID,
			DoorName:  door.Name,
			Handle:    handle,
			Score:     top.Value,
			ScoreType: top.ScoreType,
			CreatedAt: top.CreatedAt.Local().Format("2006-01-02 15:04"),
		})
		snapshot.RecentRows = append(snapshot.RecentRows, scoreChampion{
			DoorID:    door.ID,
			DoorName:  door.Name,
			Handle:    handle,
			Score:     top.Value,
			ScoreType: top.ScoreType,
			CreatedAt: top.CreatedAt.Local().Format("2006-01-02 15:04"),
		})
		if user == nil {
			continue
		}
		for _, row := range scores {
			if row.UserID != user.ID {
				continue
			}
			entry := scoreChampion{
				DoorID:    door.ID,
				DoorName:  door.Name,
				Handle:    user.Handle,
				Score:     row.Value,
				ScoreType: row.ScoreType,
				CreatedAt: row.CreatedAt.Local().Format("2006-01-02 15:04"),
			}
			if best, ok := personalBest[door.ID]; !ok || entry.Score > best.Score {
				personalBest[door.ID] = entry
			}
		}
	}
	for _, row := range personalBest {
		snapshot.PersonalRows = append(snapshot.PersonalRows, row)
	}
	if user != nil {
		snapshot.PersonalAchievements = len(a.mustDoorAchievements(user.ID, 100))
	}
	sort.Slice(snapshot.ChampionRows, func(i, j int) bool {
		if snapshot.ChampionRows[i].Score != snapshot.ChampionRows[j].Score {
			return snapshot.ChampionRows[i].Score > snapshot.ChampionRows[j].Score
		}
		return snapshot.ChampionRows[i].DoorName < snapshot.ChampionRows[j].DoorName
	})
	sort.Slice(snapshot.RecentRows, func(i, j int) bool {
		return snapshot.RecentRows[i].CreatedAt > snapshot.RecentRows[j].CreatedAt
	})
	sort.Slice(snapshot.PersonalRows, func(i, j int) bool {
		if snapshot.PersonalRows[i].Score != snapshot.PersonalRows[j].Score {
			return snapshot.PersonalRows[i].Score > snapshot.PersonalRows[j].Score
		}
		return snapshot.PersonalRows[i].DoorName < snapshot.PersonalRows[j].DoorName
	})
	if len(snapshot.ChampionRows) > 12 {
		snapshot.ChampionRows = snapshot.ChampionRows[:12]
	}
	if len(snapshot.RecentRows) > 10 {
		snapshot.RecentRows = snapshot.RecentRows[:10]
	}
	if len(snapshot.PersonalRows) > 10 {
		snapshot.PersonalRows = snapshot.PersonalRows[:10]
	}
	return snapshot
}

func (a *webApp) buildBulletinSnapshot(user *domain.User) bulletinSnapshot {
	snapshot := bulletinSnapshot{
		FeaturedThread: a.featuredThreadLine(),
		DownloadPick:   a.filebaseDownloadPick(),
		RecentCallers:  a.latestLogins(8),
	}
	if strings.TrimSpace(a.motd) != "" {
		snapshot.SystemWire = append(snapshot.SystemWire, "MOTD: "+cleanOneLiner(a.motd, 100))
	}
	if strings.TrimSpace(a.announcement) != "" {
		snapshot.SystemWire = append(snapshot.SystemWire, "Announcement: "+cleanOneLiner(a.announcement, 100))
	}
	if a.rumorzMod != nil {
		if rumor := strings.TrimSpace(a.rumorzMod.Current()); rumor != "" {
			snapshot.SystemWire = append(snapshot.SystemWire, "Rumorz: "+cleanOneLiner(rumor, 100))
		}
	}
	if user != nil {
		if digest, err := discovery.BuildSinceLastCall(a.boardRepo, a.msgRepo, a.mailRepo, user, 8); err == nil {
			for _, row := range digest.Items {
				snapshot.DigestItems = append(snapshot.DigestItems, row.Line)
			}
		}
		boards := a.visibleBoardsFor(user)
		for _, row := range a.buildBoardPulse(user, boards, 6) {
			snapshot.HotBoards = append(snapshot.HotBoards, fmt.Sprintf("%s (%d new, %d total)", row.BoardName, row.NewCount, row.MessageCount))
		}
	}
	if a.oneLinerzMod != nil {
		for _, row := range a.oneLinerzMod.List(6) {
			snapshot.OneLiners = append(snapshot.OneLiners, row.Handle+": "+cleanOneLiner(row.Text, 72))
		}
	}
	if a.adminRepo != nil {
		entries, _ := a.adminRepo.ListFileEntries(0, "", nil, 6)
		areaNames := map[int64]string{}
		if areas, err := a.adminRepo.ListFileAreas(); err == nil {
			for _, area := range areas {
				areaNames[area.ID] = area.Name
			}
		}
		for _, row := range entries {
			areaName := areaNames[row.AreaID]
			if areaName == "" {
				areaName = "Area " + strconv.FormatInt(row.AreaID, 10)
			}
			snapshot.RecentFiles = append(snapshot.RecentFiles, fmt.Sprintf("%s / %s", areaName, cleanOneLiner(row.Name, 56)))
		}
	}
	return snapshot
}

func (a *webApp) visibleBoardsFor(user *domain.User) []domain.Board {
	if user == nil || a.boardRepo == nil {
		return nil
	}
	boards, err := a.boardRepo.List()
	if err != nil {
		return nil
	}
	out := make([]domain.Board, 0, len(boards))
	for i := range boards {
		if a.canReadBoard(user, &boards[i]) {
			out = append(out, boards[i])
		}
	}
	return out
}

func (a *webApp) buildBoardMenuRows(user *domain.User, boards []domain.Board, query, mode string) ([]boardMenuRow, boardQueueSnapshot) {
	if user == nil || a.msgRepo == nil {
		return nil, boardQueueSnapshot{}
	}
	query = strings.ToLower(strings.TrimSpace(query))
	mode = normalizeBoardMode(mode)
	allRows := make([]boardMenuRow, 0, len(boards))
	for _, board := range boards {
		msgs, err := a.msgRepo.ListByBoard(board.ID)
		if err != nil {
			continue
		}
		pointerID := int64(0)
		if ptr, ptrErr := a.msgRepo.GetPointer(user.ID, board.ID); ptrErr == nil && ptr != nil {
			pointerID = ptr.LastReadID
		}
		row := boardMenuRow{Board: board, MessageCount: len(msgs)}
		for _, msg := range msgs {
			if msg.ID > pointerID {
				row.NewCount++
			}
			if msg.AuthorID == user.ID {
				row.MyPosts++
			}
			if strings.Contains(strings.ToLower(msg.Subject), strings.ToLower(user.Handle)) || strings.Contains(strings.ToLower(msg.Body), strings.ToLower(user.Handle)) {
				row.Mentions++
			}
		}
		if len(msgs) > 0 {
			last := msgs[len(msgs)-1]
			row.LastAt = last.CreatedAt.Local().Format("2006-01-02 15:04")
			row.LastSubject = cleanOneLiner(last.Subject, 72)
		}
		allRows = append(allRows, row)
	}
	sort.Slice(allRows, func(i, j int) bool {
		if allRows[i].NewCount != allRows[j].NewCount {
			return allRows[i].NewCount > allRows[j].NewCount
		}
		if allRows[i].Mentions != allRows[j].Mentions {
			return allRows[i].Mentions > allRows[j].Mentions
		}
		return strings.ToLower(allRows[i].Board.Name) < strings.ToLower(allRows[j].Board.Name)
	})
	queue := boardQueueSnapshot{}
	for _, row := range allRows {
		if row.NewCount > 0 {
			queue.UnreadBoards++
			if len(queue.UnreadRows) < 5 {
				queue.UnreadRows = append(queue.UnreadRows, row)
			}
		}
		if row.MyPosts > 0 {
			queue.MyBoards++
			if len(queue.MyRows) < 5 {
				queue.MyRows = append(queue.MyRows, row)
			}
		}
		if row.Mentions > 0 {
			queue.MentionBoards++
			if len(queue.MentionRows) < 5 {
				queue.MentionRows = append(queue.MentionRows, row)
			}
		}
	}
	filtered := make([]boardMenuRow, 0, len(allRows))
	for _, row := range allRows {
		switch mode {
		case "unread":
			if row.NewCount == 0 {
				continue
			}
		case "mine":
			if row.MyPosts == 0 {
				continue
			}
		case "mentions":
			if row.Mentions == 0 {
				continue
			}
		}
		if query != "" {
			hay := strings.ToLower(strings.Join([]string{
				row.Board.Name,
				row.Board.Description,
				defaultConferenceValue(row.Board.Conference),
				row.LastSubject,
			}, " "))
			if !strings.Contains(hay, query) {
				continue
			}
		}
		filtered = append(filtered, row)
	}
	return filtered, queue
}

func (a *webApp) buildDirectoryRows(query string, onlineOnly bool, verifiedFilter, roleFilter string) []directoryRow {
	users, err := a.authSvc.ListUsers()
	if err != nil {
		return nil
	}
	query = strings.ToLower(strings.TrimSpace(query))
	roleFilter = normalizeDirectoryRoleFilter(roleFilter)
	verifiedFilter = normalizeDirectoryVerifiedFilter(verifiedFilter)
	presence := map[string]directoryRow{}
	if a.adminRepo != nil {
		if sessions, err := a.adminRepo.ListNodeSessions(200); err == nil {
			for _, row := range sessions {
				presence[strings.ToLower(row.Username)] = directoryRow{
					Online: true,
					Area:   row.Area,
					Origin: strings.ToUpper(netutil.RemoteOrigin(row.RemoteAddr)),
				}
			}
		}
	}
	out := make([]directoryRow, 0, len(users))
	for _, row := range users {
		entry := directoryRow{
			Handle:   row.Handle,
			Role:     rbac.NormalizeRole(row.Role),
			Verified: row.Verified,
			Theme:    row.Theme,
			LastLogin: func() string {
				if row.LastLoginAt == nil || row.LastLoginAt.IsZero() {
					return "never"
				}
				return row.LastLoginAt.Local().Format("2006-01-02 15:04")
			}(),
		}
		if live, ok := presence[strings.ToLower(row.Handle)]; ok {
			entry.Online = live.Online
			entry.Area = live.Area
			entry.Origin = live.Origin
		}
		if onlineOnly && !entry.Online {
			continue
		}
		if roleFilter != "any" && entry.Role != roleFilter {
			continue
		}
		switch verifiedFilter {
		case "verified":
			if !entry.Verified {
				continue
			}
		case "unverified":
			if entry.Verified {
				continue
			}
		}
		if query != "" {
			hay := strings.ToLower(strings.Join([]string{entry.Handle, entry.Role, entry.Theme, entry.LastLogin, entry.Area, entry.Origin}, " "))
			if !strings.Contains(hay, query) {
				continue
			}
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Online != out[j].Online {
			return out[i].Online
		}
		if out[i].LastLogin != out[j].LastLogin {
			return out[i].LastLogin > out[j].LastLogin
		}
		return strings.ToLower(out[i].Handle) < strings.ToLower(out[j].Handle)
	})
	return out
}

func (a *webApp) buildDirectoryProfile(currentUser, target *domain.User) *directoryProfile {
	if currentUser == nil || target == nil {
		return nil
	}
	profile := &directoryProfile{
		Handle:   target.Handle,
		Role:     rbac.NormalizeRole(target.Role),
		Theme:    target.Theme,
		Verified: target.Verified,
	}
	if target.LastLoginAt != nil && !target.LastLoginAt.IsZero() {
		profile.LastLogin = target.LastLoginAt.Local().Format("2006-01-02 15:04")
	} else {
		profile.LastLogin = "never"
	}
	if a.adminRepo != nil {
		if sessions, err := a.adminRepo.ListNodeSessions(200); err == nil {
			for _, row := range sessions {
				if !strings.EqualFold(row.Username, target.Handle) {
					continue
				}
				profile.Online = true
				profile.OnlineArea = row.Area
				profile.OnlineOrigin = remoteHostDisplay(row.RemoteAddr)
				break
			}
		}
		if callers, err := a.adminRepo.ListCallerHistory(50); err == nil {
			for _, row := range callers {
				if !strings.EqualFold(row.Username, target.Handle) {
					continue
				}
				profile.RecentCallerRows = append(profile.RecentCallerRows, fmt.Sprintf("%s from %s in %s", row.LogoutAt.Local().Format("2006-01-02 15:04"), remoteHostDisplay(row.RemoteAddr), row.Area))
				if len(profile.RecentCallerRows) >= 6 {
					break
				}
			}
		}
	}
	if a.mailRepo != nil {
		if inbox, err := a.mailRepo.ListInbox(target.ID, 200); err == nil {
			profile.MailReceived = len(inbox)
		}
		if outbox, err := a.mailRepo.ListOutbox(target.ID, 200); err == nil {
			profile.MailSent = len(outbox)
		}
	}
	if a.msgRepo != nil {
		for _, board := range a.visibleBoardsFor(currentUser) {
			msgs, err := a.msgRepo.ListByBoard(board.ID)
			if err != nil {
				continue
			}
			byID := map[int64]domain.Message{}
			for _, msg := range msgs {
				byID[msg.ID] = msg
			}
			for _, msg := range msgs {
				if msg.AuthorID == target.ID {
					profile.Posts++
				}
				if strings.Contains(strings.ToLower(msg.Body), strings.ToLower(target.Handle)) || strings.Contains(strings.ToLower(msg.Subject), strings.ToLower(target.Handle)) {
					profile.Mentions++
				}
				if msg.ParentID > 0 {
					if parent, ok := byID[msg.ParentID]; ok && parent.AuthorID == target.ID {
						profile.Replies++
					}
				}
			}
		}
	}
	if a.doorRegistry != nil {
		if favorites, err := a.doorRegistry.ListFavorites(target.ID, 10); err == nil && len(favorites) > 0 {
			profile.FavoriteDoor = favorites[0].DoorID
		}
		profile.Achievements = len(a.mustDoorAchievements(target.ID, 100))
	}
	if profile.FavoriteDoor == "" {
		profile.FavoriteDoor = "none"
	}
	return profile
}

func (a *webApp) primarySysopUser() *domain.User {
	users, err := a.authSvc.ListUsers()
	if err != nil {
		return nil
	}
	candidates := make([]domain.User, 0, len(users))
	for _, row := range users {
		if rbac.NormalizeRole(row.Role) == roleAdmin {
			candidates = append(candidates, row)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		var li, lj time.Time
		if candidates[i].LastLoginAt != nil {
			li = candidates[i].LastLoginAt.UTC()
		}
		if candidates[j].LastLoginAt != nil {
			lj = candidates[j].LastLoginAt.UTC()
		}
		if li.Equal(lj) {
			return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
		}
		return li.After(lj)
	})
	pick := candidates[0]
	return &pick
}

func (a *webApp) searchMessageHits(user *domain.User, query string, boardID int64, authorFilter string, limit int) []messageSearchHit {
	query = strings.ToLower(strings.TrimSpace(query))
	authorFilter = strings.ToLower(strings.TrimSpace(authorFilter))
	if user == nil || limit <= 0 || a.msgRepo == nil {
		return nil
	}
	if query == "" && boardID <= 0 && authorFilter == "" {
		return nil
	}
	lookup := a.userHandleLookup()
	out := make([]messageSearchHit, 0, limit)
	for _, board := range a.visibleBoardsFor(user) {
		if boardID > 0 && board.ID != boardID {
			continue
		}
		msgs, err := a.msgRepo.ListByBoard(board.ID)
		if err != nil {
			continue
		}
		for _, msg := range msgs {
			author := lookup[msg.AuthorID]
			if authorFilter != "" && !strings.EqualFold(author, authorFilter) {
				continue
			}
			hay := strings.ToLower(msg.Subject + "\n" + msg.Body)
			if query != "" && !strings.Contains(hay, query) {
				continue
			}
			out = append(out, messageSearchHit{
				BoardID:    board.ID,
				BoardName:  board.Name,
				Conference: defaultConferenceValue(board.Conference),
				MessageID:  msg.ID,
				Subject:    cleanOneLiner(msg.Subject, 72),
				Author:     author,
				AuthorID:   msg.AuthorID,
				CreatedAt:  msg.CreatedAt.Local().Format("2006-01-02 15:04"),
				Snippet:    cleanOneLiner(msg.Body, 90),
			})
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func (a *webApp) buildThreadTracker(user *domain.User, trackerFilter string, limit int) []threadTrackerItem {
	if user == nil || limit <= 0 || a.msgRepo == nil {
		return nil
	}
	trackerFilter = normalizeTrackerFilter(trackerFilter)
	type trackerEntry struct {
		item threadTrackerItem
		when time.Time
	}
	rows := make([]trackerEntry, 0, limit)
	for _, board := range a.visibleBoardsFor(user) {
		msgs, err := a.msgRepo.ListByBoard(board.ID)
		if err != nil {
			continue
		}
		byID := map[int64]domain.Message{}
		for _, msg := range msgs {
			byID[msg.ID] = msg
		}
		for _, msg := range msgs {
			kind := ""
			switch {
			case msg.AuthorID == user.ID:
				kind = "post"
			case strings.Contains(strings.ToLower(msg.Body), strings.ToLower(user.Handle)) || strings.Contains(strings.ToLower(msg.Subject), strings.ToLower(user.Handle)):
				kind = "mention"
			case msg.ParentID > 0:
				if parent, ok := byID[msg.ParentID]; ok && parent.AuthorID == user.ID {
					kind = "reply"
				}
			}
			if kind == "" || (trackerFilter != "all" && kind != trackerFilter) {
				continue
			}
			rows = append(rows, trackerEntry{
				item: threadTrackerItem{
					Kind:      kind,
					BoardID:   board.ID,
					MessageID: msg.ID,
					BoardName: board.Name,
					Subject:   cleanOneLiner(msg.Subject, 56),
					CreatedAt: msg.CreatedAt.Local().Format("2006-01-02 15:04"),
				},
				when: msg.CreatedAt,
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].when.After(rows[j].when)
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]threadTrackerItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.item)
	}
	return out
}

func (a *webApp) buildNewFilesSnapshot(user *domain.User, sinceFilter, tagFilter, sortMode string) newFilesSnapshot {
	snapshot := newFilesSnapshot{QueueNames: map[int64]string{}, AreaNames: map[int64]string{}}
	if user == nil || a.adminRepo == nil {
		return snapshot
	}
	sinceFilter = normalizeFileSinceFilter(sinceFilter)
	sortMode = normalizeFileSortMode(sortMode)
	tagFilter = strings.ToLower(strings.TrimSpace(tagFilter))
	if areas, err := a.adminRepo.ListFileAreas(); err == nil {
		for _, row := range areas {
			snapshot.AreaNames[row.ID] = row.Name
		}
	}
	if rows, err := a.adminRepo.ListFileEntries(0, "", nil, 120); err == nil {
		filtered := make([]domain.FileEntry, 0, len(rows))
		cutoff := time.Time{}
		switch sinceFilter {
		case "24h":
			cutoff = time.Now().Add(-24 * time.Hour)
		case "7d":
			cutoff = time.Now().Add(-7 * 24 * time.Hour)
		case "30d":
			cutoff = time.Now().Add(-30 * 24 * time.Hour)
		}
		for _, row := range rows {
			if !cutoff.IsZero() && row.UploadedAt.Before(cutoff) {
				continue
			}
			if tagFilter != "" && !hasTagIgnoreCase(row.Tags, tagFilter) {
				continue
			}
			filtered = append(filtered, row)
		}
		switch sortMode {
		case "rating":
			sort.Slice(filtered, func(i, j int) bool {
				if filtered[i].RatingAvg == filtered[j].RatingAvg {
					if filtered[i].RatingCount == filtered[j].RatingCount {
						return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
					}
					return filtered[i].RatingCount > filtered[j].RatingCount
				}
				return filtered[i].RatingAvg > filtered[j].RatingAvg
			})
		case "name":
			sort.Slice(filtered, func(i, j int) bool {
				return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
			})
		default:
			sort.Slice(filtered, func(i, j int) bool {
				return filtered[i].UploadedAt.After(filtered[j].UploadedAt)
			})
		}
		snapshot.RecentUploads = filtered
		top := append([]domain.FileEntry(nil), filtered...)
		sort.Slice(top, func(i, j int) bool {
			if top[i].RatingAvg == top[j].RatingAvg {
				return top[i].RatingCount > top[j].RatingCount
			}
			return top[i].RatingAvg > top[j].RatingAvg
		})
		if len(top) > 8 {
			top = top[:8]
		}
		snapshot.TopRated = top
	}
	if rows, err := a.adminRepo.ListFileFilters(user.ID); err == nil {
		snapshot.SavedFilters = rows
	}
	if rows, err := a.adminRepo.ListDownloadQueue(user.ID, 100); err == nil {
		snapshot.Queue = rows
		for _, row := range rows {
			if entry, err := a.adminRepo.GetFileEntry(row.FileID); err == nil && entry != nil {
				snapshot.QueueNames[row.FileID] = entry.Name
			}
		}
	}
	if len(snapshot.RecentUploads) > 16 {
		snapshot.RecentUploads = snapshot.RecentUploads[:16]
	}
	return snapshot
}

func (a *webApp) recentCorrespondents(user *domain.User, inbox, outbox []domain.PrivateMail, limit int) []string {
	if user == nil || limit <= 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, limit)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	lookup := a.userHandleLookup()
	for _, row := range outbox {
		if row.ExternalTo != nil {
			add(*row.ExternalTo)
		} else {
			add(lookup[row.ToUserID])
		}
		if len(out) >= limit {
			return out
		}
	}
	for _, row := range inbox {
		add(lookup[row.FromUserID])
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func (a *webApp) localMailPicks(currentHandle string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	users, err := a.authSvc.ListUsers()
	if err != nil {
		return nil
	}
	currentHandle = strings.ToLower(strings.TrimSpace(currentHandle))
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
	out := make([]string, 0, limit)
	for _, row := range users {
		if strings.ToLower(strings.TrimSpace(row.Handle)) == currentHandle {
			continue
		}
		out = append(out, row.Handle)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeBoardMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "unread", "mine", "mentions", "watched", "digest", "muted":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "all"
	}
}

func normalizeMailBox(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "inbox", "unread", "outbox":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "all"
	}
}

func normalizeMailTemplate(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "short_note", "door_invite", "follow_up":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "none"
	}
}

func mailTemplatePrefill(template string) (string, string) {
	switch normalizeMailTemplate(template) {
	case "short_note":
		return "Quick note from WolfBBS", "Checking in from the board.\n\n"
	case "door_invite":
		return "Meet me in the Door Hub", "I found a good door run. Meet me in /doors and we can compare scores.\n\n"
	case "follow_up":
		return "Following up", "Following up on the last note so this does not fall through the cracks.\n\n"
	default:
		return "", ""
	}
}

func filterMailRows(inbox, outbox []domain.PrivateMail, handleByID map[int64]string, boxFilter, query string) ([]domain.PrivateMail, []domain.PrivateMail) {
	query = strings.ToLower(strings.TrimSpace(query))
	boxFilter = normalizeMailBox(boxFilter)
	visibleInbox := make([]domain.PrivateMail, 0, len(inbox))
	visibleOutbox := make([]domain.PrivateMail, 0, len(outbox))
	if boxFilter == "all" || boxFilter == "inbox" || boxFilter == "unread" {
		for _, row := range inbox {
			if boxFilter == "unread" && row.ReadAt != nil {
				continue
			}
			if !mailMatchesFilter(row, handleByID[row.FromUserID], query) {
				continue
			}
			visibleInbox = append(visibleInbox, row)
		}
	}
	if boxFilter == "all" || boxFilter == "outbox" {
		for _, row := range outbox {
			target := handleByID[row.ToUserID]
			if row.ExternalTo != nil {
				target = *row.ExternalTo
			}
			if !mailMatchesFilter(row, target, query) {
				continue
			}
			visibleOutbox = append(visibleOutbox, row)
		}
	}
	return visibleInbox, visibleOutbox
}

func mailMatchesFilter(row domain.PrivateMail, contact, query string) bool {
	if query == "" {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{contact, row.Subject, row.Body}, "\n"))
	return strings.Contains(hay, query)
}

func normalizeTrackerFilter(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "post", "mention", "reply":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "all"
	}
}

func normalizeFileSinceFilter(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "24h", "7d", "30d":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "all"
	}
}

func normalizeFileSortMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "rating", "name":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "latest"
	}
}

func normalizeDirectoryVerifiedFilter(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "verified", "unverified":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "any"
	}
}

func normalizeDirectoryRoleFilter(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case roleUser, roleModerator, roleAdmin:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "any"
	}
}

func boardQueueList(rows []boardMenuRow, empty string) string {
	if len(rows) == 0 {
		return `<p class="wolfbbs-muted">` + htmlEscape(empty) + `</p>`
	}
	list := strings.Builder{}
	list.WriteString(`<ul>`)
	for _, row := range rows {
		meta := []string{}
		if row.NewCount > 0 {
			meta = append(meta, strconv.Itoa(row.NewCount)+" new")
		}
		if row.MyPosts > 0 {
			meta = append(meta, strconv.Itoa(row.MyPosts)+" yours")
		}
		if row.Mentions > 0 {
			meta = append(meta, strconv.Itoa(row.Mentions)+" mentions")
		}
		list.WriteString(`<li><a href="/boards?board=` + strconv.FormatInt(row.Board.ID, 10) + `">` + htmlEscape(row.Board.Name) + `</a> <span class="wolfbbs-muted">` + htmlEscape(strings.Join(meta, " | ")) + `</span></li>`)
	}
	list.WriteString(`</ul>`)
	return list.String()
}

func hasTagIgnoreCase(tags []string, value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return true
	}
	for _, row := range tags {
		if strings.EqualFold(strings.TrimSpace(row), value) {
			return true
		}
	}
	return false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func formatDurationCompact(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return strconv.Itoa(int(d.Seconds())) + "s"
	}
	if d < time.Hour {
		return strconv.Itoa(int(d.Minutes())) + "m"
	}
	if d < 24*time.Hour {
		return strconv.Itoa(int(d.Hours())) + "h"
	}
	return strconv.Itoa(int(d.Hours()/24)) + "d"
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
