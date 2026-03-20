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
	"path/filepath"
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

// webBuildVersion is overridden in CI/release builds via -ldflags -X main.webBuildVersion=...
var webBuildVersion = "v1.0.0"

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

type activityHeatmapCell struct {
	ShortLabel string
	DateLabel  string
	Count      int
	Level      int
	Detail     string
}

type attentionPreset struct {
	Name   string
	Label  string
	Detail string
	Pref   digestPreferences
}

type attentionExportPayload struct {
	Version                  string            `json:"version"`
	GeneratedAt              time.Time         `json:"generated_at"`
	Handle                   string            `json:"handle"`
	Role                     string            `json:"role"`
	PresetRecommendation     string            `json:"preset_recommendation"`
	DigestPreferences        digestPreferences `json:"digest_preferences"`
	BoardSubscriptions       map[string]string `json:"board_subscriptions,omitempty"`
	BoardQuietHours          map[string]string `json:"board_quiet_hours,omitempty"`
	RouteSeen                map[string]string `json:"route_seen,omitempty"`
	BulletinAcknowledgements map[string]string `json:"bulletin_acknowledgements,omitempty"`
	Attention                struct {
		Dismissed map[string]string `json:"dismissed,omitempty"`
		Read      map[string]string `json:"read,omitempty"`
		Snoozed   map[string]string `json:"snoozed,omitempty"`
	} `json:"attention"`
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

type scheduledBulletin struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at,omitempty"`
	Link      string    `json:"link,omitempty"`
	Audience  string    `json:"audience,omitempty"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type pageRequest struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Message   string    `json:"message"`
	Source    string    `json:"source,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type quietHoursWindow struct {
	Enabled   bool `json:"enabled"`
	StartHour int  `json:"start_hour"`
	EndHour   int  `json:"end_hour"`
}

type threadLifecycleState string

const (
	threadLifecycleActive   threadLifecycleState = "active"
	threadLifecycleSlow     threadLifecycleState = "slow"
	threadLifecycleArchived threadLifecycleState = "archived"
	threadLifecycleFrozen   threadLifecycleState = "frozen"
)

type boardWelcomeKit struct {
	Intro          string   `json:"intro"`
	SeedPrompts    []string `json:"seed_prompts,omitempty"`
	StarterThreads []string `json:"starter_threads,omitempty"`
}

type boardStaffNote struct {
	Note      string    `json:"note"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type boardSteward struct {
	Handle string `json:"handle"`
	Topic  string `json:"topic,omitempty"`
	Note   string `json:"note,omitempty"`
}

type threadPoll struct {
	Question  string         `json:"question"`
	Options   []string       `json:"options"`
	Votes     map[string]int `json:"votes,omitempty"`
	CreatedBy string         `json:"created_by,omitempty"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
	Closed    bool           `json:"closed,omitempty"`
}

type messageRevision struct {
	Subject  string    `json:"subject"`
	Body     string    `json:"body"`
	EditedBy string    `json:"edited_by"`
	EditedAt time.Time `json:"edited_at"`
}

type bestOfWeekEntry struct {
	MessageID int64     `json:"message_id"`
	Note      string    `json:"note,omitempty"`
	AddedBy   string    `json:"added_by,omitempty"`
	AddedAt   time.Time `json:"added_at,omitempty"`
}

type bestOfWeekRow struct {
	BoardID    int64
	BoardName  string
	MessageID  int64
	Subject    string
	Author     string
	CreatedAt  string
	ReplyCount int
	Reason     string
	Note       string
	Href       string
}

type legacyArchiveMessage struct {
	Subject string
	From    string
	Date    time.Time
	Body    string
}

type bulletinAckSummary struct {
	Count  int
	LastAt time.Time
}

type staffEscalationEntry struct {
	ID         string    `json:"id"`
	Handle     string    `json:"handle"`
	Actor      string    `json:"actor"`
	Note       string    `json:"note"`
	CreatedAt  time.Time `json:"created_at"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
}

type publicProfileSettings struct {
	StatusLine     string   `json:"status_line,omitempty"`
	Bio            string   `json:"bio,omitempty"`
	ContactPrefs   []string `json:"contact_prefs,omitempty"`
	ShowStatusLine bool     `json:"show_status_line"`
	ShowBio        bool     `json:"show_bio"`
	ShowContact    bool     `json:"show_contact"`
}

type callerCircle struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Members []string `json:"members,omitempty"`
	Note    string   `json:"note,omitempty"`
}

type moderatorInboxAssignment struct {
	MailID     int64     `json:"mail_id"`
	Assignee   string    `json:"assignee,omitempty"`
	Status     string    `json:"status"`
	Note       string    `json:"note,omitempty"`
	UpdatedBy  string    `json:"updated_by,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
}

type moderatorInboxRow struct {
	Mail       domain.PrivateMail
	FromHandle string
	ToHandle   string
	Assignment moderatorInboxAssignment
}

type eventRSVP struct {
	EventID    string    `json:"event_id"`
	Status     string    `json:"status"`
	InvitedBy  string    `json:"invited_by,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
}

type bulletinSnapshot struct {
	SystemWire     []string
	DigestItems    []string
	HotBoards      []string
	RecentCallers  []string
	OneLiners      []string
	RecentFiles    []string
	Scheduled      []scheduledBulletin
	BestOfWeek     []bestOfWeekRow
	FeaturedThread string
	DownloadPick   string
}

type callerLinkStat struct {
	Handle string
	Count  int
	Online bool
	Area   string
}

type directoryRow struct {
	Handle    string
	Role      string
	Verified  bool
	Theme     string
	LastLogin string
	Online    bool
	Node      string
	Idle      string
	Area      string
	Origin    string
	From      string
}

type directoryProfile struct {
	Handle           string
	Role             string
	Theme            string
	Verified         bool
	LastLogin        string
	Online           bool
	OnlineNode       string
	OnlineIdle       string
	OnlineArea       string
	OnlineOrigin     string
	OnlineFrom       string
	Posts            int
	Mentions         int
	Replies          int
	MailSent         int
	MailReceived     int
	FavoriteDoor     string
	FavoriteCaller   bool
	Achievements     int
	RecentCallerRows []string
	Correspondents   []callerLinkStat
	StaffNote        string
	StatusLine       string
	Bio              string
	ContactPrefs     []string
	Alias            string
	CircleNames      []string
	Relationship     []string
	IncidentTimeline []string
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

type bookmarkEntry struct {
	Key       string    `json:"key"`
	Kind      string    `json:"kind"`
	Label     string    `json:"label"`
	Href      string    `json:"href"`
	Meta      string    `json:"meta,omitempty"`
	BoardID   int64     `json:"board_id,omitempty"`
	MessageID int64     `json:"message_id,omitempty"`
	MailID    int64     `json:"mail_id,omitempty"`
	AddedAt   time.Time `json:"added_at"`
}

type tournamentStandingRow struct {
	Rank     int
	DoorName string
	Handle   string
	Score    int64
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

type digestPreferences struct {
	Enabled          bool   `json:"enabled"`
	MaxItems         int    `json:"max_items"`
	IncludeEvents    bool   `json:"include_events"`
	IncludeBoards    bool   `json:"include_boards"`
	WeeklyMail       bool   `json:"weekly_mail"`
	AttentionCadence string `json:"attention_cadence"`
	BulletinCadence  string `json:"bulletin_cadence"`
	EventCadence     string `json:"event_cadence"`
}

type aiGatewaySettings struct {
	Enabled      bool
	BaseURL      string
	Model        string
	APIKey       string
	SystemPrompt string
	TimeoutSec   int
	MaxTokens    int
}

type fileReviewItem struct {
	FileID     int64     `json:"file_id"`
	AreaID     int64     `json:"area_id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Notes      string    `json:"notes,omitempty"`
	Uploader   string    `json:"uploader,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ReviewedAt time.Time `json:"reviewed_at,omitempty"`
	ReviewedBy string    `json:"reviewed_by,omitempty"`
}

type launchCheckpoint struct {
	Key    string
	Title  string
	Detail string
	Done   bool
}

type sessionState struct {
	handle             string
	expire             time.Time
	csrf               string
	transport          string
	secure             bool
	authFactor         int
	credentialReceipts []adminCredentialReceipt
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
	sysSettingGatewayAIEnabled       = "gateway.ai.enabled"
	sysSettingGatewayAIBaseURL       = "gateway.ai.base_url"
	sysSettingGatewayAIModel         = "gateway.ai.model"
	sysSettingGatewayAIAPIKey        = "gateway.ai.api_key"
	sysSettingGatewayAISystemPrompt  = "gateway.ai.system_prompt"
	sysSettingGatewayAITimeoutSec    = "gateway.ai.timeout_sec"
	sysSettingGatewayAIMaxTokens     = "gateway.ai.max_tokens"
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
	sysSettingScheduledBulletins     = "community.bulletins.schedule"
	sysSettingPageRequests           = "community.pages"
	sysSettingBulletinAckRoot        = "community.bulletins.ack."
	sysSettingStaffEscalations       = "staff.escalations"
	sysSettingBoardSubscriptionsRoot = "web.board_subscriptions."
	sysSettingBoardWatchRoot         = "web.board_watch."
	sysSettingBoardQuietHoursRoot    = "web.board_quiet."
	sysSettingThreadLifecycleStates  = "boards.thread_lifecycle"
	sysSettingBoardWelcomeKits       = "boards.welcome_kits"
	sysSettingBoardStaffNotes        = "boards.staff_notes"
	sysSettingBoardStewards          = "boards.stewards"
	sysSettingThreadPolls            = "boards.polls"
	sysSettingMessageRevisions       = "boards.revisions"
	sysSettingBestOfWeek             = "boards.best_of_week"
	sysSettingPublicProfileRoot      = "profile.public."
	sysSettingContactAliasesRoot     = "profile.aliases."
	sysSettingCallerCirclesRoot      = "profile.circles."
	sysSettingModInboxAssignments    = "mail.shared_assignments"
	sysSettingEventRSVPRoot          = "events.rsvp."
	sysSettingAttentionDismissedRoot = "web.attention.dismissed."
	sysSettingAttentionReadRoot      = "web.attention.read."
	sysSettingAttentionSnoozeRoot    = "web.attention.snooze."
	sysSettingLaunchChecklistRoot    = "web.launch_checklist."
	sysSettingHomeRouteRoot          = "web.home_route."
	sysSettingDigestPrefsRoot        = "web.digest_prefs."
	sysSettingBookmarksRoot          = "web.bookmarks."
	sysSettingFavoriteCallersRoot    = "web.favorite_callers."
	sysSettingRouteSeenRoot          = "web.route_seen."
	sysSettingWeeklyDigestSentRoot   = "web.digest_mail_sent."
	sysSettingStaffNotesRoot         = "staff.notes."
	sysSettingFileReviewQueue        = "web.file_review.queue"
	sysSettingFileRequests           = "web.file_requests"
	sysSettingUploadDrafts           = "web.file_upload_drafts"
	sysSettingCuratorNotes           = "web.file_curator_notes"
	sysSettingFeaturedCollections    = "web.file_featured_collections"
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
	maxAttentionSnoozedItems         = 200
	maxBookmarkItems                 = 200
	maxHandleSuggestions             = 8
	maxFavoriteCallers               = 32
	maxPageRequests                  = 200
	maxStaffEscalations              = 200
	maxAIGatewayPromptChars          = 4000
)

const (
	fileReviewHold     = "hold"
	fileReviewApproved = "approved"
	fileReviewRejected = "rejected"
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
	proxyResolver         *netutil.ProxyResolver
	rateLimits            map[string][]time.Time
	loginRateLimit        int
	loginRateWindow       time.Duration
	resetRateLimit        int
	resetRateWindow       time.Duration
	chatSvc               *chat.Service
	doorRegistry          *doors.Registry
	eventBus              *events.Bus
	offlineDir            string
	siteName              string
	siteHostname          string
	readOnly              bool
	secureCookie          bool
	requireVerifiedEmail  bool
	inboundToken          string
	inboundAllow          map[string]struct{}
	resetTTL              time.Duration
	showResetDev          bool
	apEnabled             bool
	apBaseURL             string
	publicBaseURL         string
	resetNotifier         func(handle, token string, r *http.Request) error
	startedAt             time.Time
	runtimeCfg            config.Runtime
	modernOnRamp          bool
	guestTour             bool
	discover              bool
	quickJump             bool
	classicSearch         bool
	wsTerminalURL         string
	menuRoot              string
	savedSearches         map[string][]string
	attentionDismissed    map[string]map[string]time.Time
	attentionLoaded       map[string]bool
	attentionRead         map[string]map[string]time.Time
	attentionReadLoaded   map[string]bool
	attentionSnoozed      map[string]map[string]time.Time
	attentionSnoozeLoaded map[string]bool
	motd                  string
	announcement          string
	errorLog              []appErrorEntry
	lockedChat            map[string]bool
	networkSvc            *network.Service
	modsManager           *mods.Manager
	oneLinerzMod          *mods.OneLinerzMod
	rumorzMod             *mods.RumorzMod
	bbsListMod            *mods.BBSListMod
	whoOnlineMod          *mods.WhoOnlineMod
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
				dir = filepath.Join(installPrefixPath(), "offline")
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
		for _, eventName := range []string{"board.created", "board.updated", "board.deleted", "message.posted"} {
			name := eventName
			app.eventBus.Subscribe(name, func(ev events.Event) {
				go app.dispatchWebhookEvent(name, ev.Fields)
			})
		}
	}

	http.HandleFunc("/", app.handleRoot)
	http.HandleFunc("/start", app.handleStartCenter)
	http.HandleFunc("/showcase", app.handleShowcase)
	http.Handle("/first-call", app.authRequired(http.HandlerFunc(app.handleFirstCallSession)))
	http.Handle("/today", app.authRequired(http.HandlerFunc(app.handleToday)))
	http.Handle("/digest", app.authRequired(http.HandlerFunc(app.handleDigest)))
	http.HandleFunc("/events", app.handleEventsCalendar)
	http.HandleFunc("/events/recaps", app.handleEventRecaps)
	http.HandleFunc("/tournaments", app.handleTournaments)
	http.Handle("/challenges", app.authRequired(http.HandlerFunc(app.handleChallenges)))
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
	http.Handle("/bookmarks", app.authRequired(http.HandlerFunc(app.handleBookmarks)))
	http.Handle("/settings", app.authRequired(http.HandlerFunc(app.handleSettings)))
	http.Handle("/digest/preferences", app.authRequired(http.HandlerFunc(app.handleDigestPreferences)))
	http.Handle("/streaks", app.authRequired(http.HandlerFunc(app.handleStreaks)))
	http.Handle("/next", app.authRequired(http.HandlerFunc(app.handleNextActions)))
	http.Handle("/spotlights", app.authRequired(http.HandlerFunc(app.handleSpotlights)))
	http.Handle("/missions", app.authRequired(http.HandlerFunc(app.handleMissions)))
	http.Handle("/resume", app.authRequired(http.HandlerFunc(app.handleResumeCenter)))
	http.Handle("/doors/comeback", app.authRequired(http.HandlerFunc(app.handleDoorComeback)))
	http.Handle("/mentorship", app.authRequired(http.HandlerFunc(app.handleMentorship)))
	http.Handle("/milestones", app.authRequired(http.HandlerFunc(app.handleMilestones)))
	http.Handle("/time-lane", app.authRequired(http.HandlerFunc(app.handleTimeLane)))
	http.Handle("/profile/export", app.authRequired(http.HandlerFunc(app.handleProfileExport)))
	http.Handle("/circles", app.authRequired(http.HandlerFunc(app.handleCircles)))
	http.Handle("/status", app.authRequired(http.HandlerFunc(app.handleStatusCenter)))
	http.Handle("/statusz", app.authRequired(http.HandlerFunc(app.handleStatusJSON)))
	http.Handle("/config", app.authRequired(http.HandlerFunc(app.handleConfigCenter)))
	http.Handle("/attention", app.authRequired(http.HandlerFunc(app.handleAttentionCenter)))
	http.Handle("/attention/export", app.authRequired(http.HandlerFunc(app.handleAttentionExport)))
	http.Handle("/discover", app.authRequired(http.HandlerFunc(app.handleDiscover)))
	http.Handle("/bulletins", app.authRequired(http.HandlerFunc(app.handleBulletins)))
	http.Handle("/directory", app.authRequired(http.HandlerFunc(app.handleDirectory)))
	http.Handle("/feedback", app.authRequired(http.HandlerFunc(app.handleFeedback)))
	http.Handle("/finder", app.authRequired(http.HandlerFunc(app.handleFinder)))
	http.Handle("/newfiles", app.authRequired(http.HandlerFunc(app.handleNewFiles)))
	http.Handle("/collections", app.authRequired(http.HandlerFunc(app.handleCollections)))
	http.Handle("/offline", app.authRequired(http.HandlerFunc(app.handleOffline)))
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
	http.Handle("/admin/challenges", app.mustBeRole(roleAdmin, app.handleAdminChallenges))
	http.Handle("/admin/missions", app.mustBeRole(roleAdmin, app.handleAdminMissions))
	http.Handle("/admin/mentorship", app.mustBeRole(roleModerator, app.handleAdminMentorship))
	http.Handle("/admin/mod-center", app.mustBeRole(roleModerator, app.handleAdminModCenter))
	http.Handle("/admin/plugins", app.mustBeRole(roleAdmin, app.handleAdminPlugins))
	http.Handle("/admin/plugins/starter", app.mustBeRole(roleAdmin, app.handleAdminPluginStarter))
	http.Handle("/admin/themes", app.mustBeRole(roleAdmin, app.handleAdminThemes))
	http.Handle("/admin/webhooks", app.mustBeRole(roleAdmin, app.handleAdminWebhooks))
	http.Handle("/admin/analytics", app.mustBeRole(roleAdmin, app.handleAdminAnalytics))
	http.Handle("/admin/release", app.mustBeRole(roleAdmin, app.handleAdminReleaseDashboard))
	http.Handle("/admin/bulletins", app.mustBeRole(roleAdmin, app.handleAdminBulletins))
	http.Handle("/admin/launch", app.mustBeRole(roleAdmin, app.handleAdminLaunch))
	http.Handle("/admin/ops", app.mustBeRole(roleAdmin, app.handleAdminOps))
	http.Handle("/admin/upgrade-safety", app.mustBeRole(roleAdmin, app.handleAdminUpgradeSafety))
	http.Handle("/admin/backups", app.mustBeRole(roleAdmin, app.handleAdminBackups))
	http.Handle("/admin/setup", app.mustBeRole(roleAdmin, app.handleAdminSetup))
	http.Handle("/admin/config", app.mustBeRole(roleAdmin, app.handleAdminConfig))
	http.Handle("/admin/errors", app.mustBeRole(roleAdmin, app.handleAdminErrors))
	http.Handle("/admin/system", app.mustBeRole(roleAdmin, app.handleAdminSystem))
	http.Handle("/admin/node-state", app.mustBeRole(roleAdmin, app.handleAdminNodeState))
	http.Handle("/admin/audit", app.mustBeRole(roleAdmin, app.handleAdminAudit))
	http.Handle("/scores", app.authRequired(http.HandlerFunc(app.handleScores)))
	http.Handle("/topx", app.authRequired(http.HandlerFunc(app.handleTopX)))
	http.Handle("/chat", app.authRequired(http.HandlerFunc(app.handleChat)))
	http.Handle("/chat/send", app.authRequired(http.HandlerFunc(app.handleChatSend)))
	http.Handle("/chat/stream", app.authRequired(http.HandlerFunc(app.handleChatStream)))
	http.Handle("/chat/channels", app.authRequired(http.HandlerFunc(app.handleChatChannels)))
	http.Handle("/chat/join", app.authRequired(http.HandlerFunc(app.handleChatJoin)))
	http.Handle("/chat/leave", app.authRequired(http.HandlerFunc(app.handleChatLeave)))
	http.Handle("/chat/history", app.authRequired(http.HandlerFunc(app.handleChatHistory)))
	http.Handle("/chat/online", app.authRequired(http.HandlerFunc(app.handleChatOnline)))
	http.Handle("/chat/moderation", app.mustBeRole(roleModerator, http.HandlerFunc(app.handleChatModeration)))
	http.Handle("/handles/suggest", app.authRequired(http.HandlerFunc(app.handleHandleSuggestions)))
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
	case "/digest":
		return "/digest"
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
		{Value: "/digest", Label: "/digest"},
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

func digestPreferencesSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingDigestPrefsRoot + handle
}

func defaultDigestPreferences() digestPreferences {
	return digestPreferences{
		Enabled:          false,
		MaxItems:         12,
		IncludeEvents:    true,
		IncludeBoards:    true,
		WeeklyMail:       false,
		AttentionCadence: "always",
		BulletinCadence:  "daily",
		EventCadence:     "daily",
	}
}

func normalizeDigestPreferences(pref digestPreferences) digestPreferences {
	if pref.MaxItems < 6 {
		pref.MaxItems = 6
	}
	if pref.MaxItems > 24 {
		pref.MaxItems = 24
	}
	pref.AttentionCadence = normalizeDigestCadence(pref.AttentionCadence)
	pref.BulletinCadence = normalizeDigestCadence(pref.BulletinCadence)
	pref.EventCadence = normalizeDigestCadence(pref.EventCadence)
	return pref
}

func normalizeDigestCadence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "always", "daily", "weekly", "off":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "daily"
	}
}

func digestCadenceOptionRows(current string) string {
	current = normalizeDigestCadence(current)
	options := []struct {
		Value string
		Label string
	}{
		{Value: "always", Label: "always"},
		{Value: "daily", Label: "daily"},
		{Value: "weekly", Label: "weekly"},
		{Value: "off", Label: "off"},
	}
	var out strings.Builder
	for _, option := range options {
		out.WriteString(`<option value="` + option.Value + `"` + selectedIf(option.Value == current) + `>` + option.Label + `</option>`)
	}
	return out.String()
}

func attentionPresetCatalog() []attentionPreset {
	return []attentionPreset{
		{
			Name:   "guest",
			Label:  "Guest",
			Detail: "Lowest-noise rules for tentative or infrequent callers.",
			Pref: digestPreferences{
				Enabled:          true,
				MaxItems:         6,
				IncludeEvents:    false,
				IncludeBoards:    false,
				WeeklyMail:       false,
				AttentionCadence: "off",
				BulletinCadence:  "weekly",
				EventCadence:     "weekly",
			},
		},
		{
			Name:   "caller",
			Label:  "Caller",
			Detail: "Balanced defaults for daily reading without constant interruption.",
			Pref: digestPreferences{
				Enabled:          true,
				MaxItems:         12,
				IncludeEvents:    true,
				IncludeBoards:    true,
				WeeklyMail:       false,
				AttentionCadence: "daily",
				BulletinCadence:  "daily",
				EventCadence:     "daily",
			},
		},
		{
			Name:   "moderator",
			Label:  "Moderator",
			Detail: "Fast follow-up for moderation work, pages, and recurring queues.",
			Pref: digestPreferences{
				Enabled:          true,
				MaxItems:         16,
				IncludeEvents:    true,
				IncludeBoards:    true,
				WeeklyMail:       true,
				AttentionCadence: "always",
				BulletinCadence:  "daily",
				EventCadence:     "daily",
			},
		},
		{
			Name:   "sysop",
			Label:  "Sysop",
			Detail: "High-visibility rules for launch, bulletin, and event ownership.",
			Pref: digestPreferences{
				Enabled:          true,
				MaxItems:         20,
				IncludeEvents:    true,
				IncludeBoards:    true,
				WeeklyMail:       true,
				AttentionCadence: "always",
				BulletinCadence:  "always",
				EventCadence:     "always",
			},
		},
	}
}

func attentionPresetByName(name string) (attentionPreset, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, preset := range attentionPresetCatalog() {
		if preset.Name == name {
			preset.Pref = normalizeDigestPreferences(preset.Pref)
			return preset, true
		}
	}
	return attentionPreset{}, false
}

func attentionPresetNameForRole(role string) string {
	switch rbac.NormalizeRole(role) {
	case roleAdmin:
		return "sysop"
	case roleModerator:
		return "moderator"
	case roleUser:
		return "caller"
	default:
		return "guest"
	}
}

func recommendedAttentionPreset(user *domain.User) attentionPreset {
	name := attentionPresetNameForRole("")
	if user != nil {
		name = attentionPresetNameForRole(user.Role)
	}
	preset, ok := attentionPresetByName(name)
	if !ok {
		fallback, _ := attentionPresetByName("caller")
		return fallback
	}
	return preset
}

func currentAttentionPresetName(pref digestPreferences) string {
	pref = normalizeDigestPreferences(pref)
	for _, preset := range attentionPresetCatalog() {
		if pref == normalizeDigestPreferences(preset.Pref) {
			return preset.Name
		}
	}
	return ""
}

func summarizeDigestPreferences(pref digestPreferences) string {
	pref = normalizeDigestPreferences(pref)
	return fmt.Sprintf("attention %s | bulletins %s | events %s | weekly mail %s", pref.AttentionCadence, pref.BulletinCadence, pref.EventCadence, boolToText(pref.WeeklyMail))
}

func (a *webApp) loadDigestPreferences(handle string) digestPreferences {
	pref := defaultDigestPreferences()
	if a.adminRepo == nil {
		return pref
	}
	key := digestPreferencesSettingKey(handle)
	if key == "" {
		return pref
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return pref
	}
	if err := json.Unmarshal([]byte(raw), &pref); err != nil {
		a.addAppError("digest.preferences", fmt.Errorf("decode digest preferences for %s: %w", handle, err))
		return defaultDigestPreferences()
	}
	return normalizeDigestPreferences(pref)
}

func (a *webApp) persistDigestPreferences(handle string, pref digestPreferences) {
	key := digestPreferencesSettingKey(handle)
	if key == "" {
		return
	}
	pref = normalizeDigestPreferences(pref)
	raw, err := json.Marshal(pref)
	if err != nil {
		a.addAppError("digest.preferences", fmt.Errorf("encode digest preferences for %s: %w", handle, err))
		return
	}
	a.persistSystemSetting(key, string(raw))
}

func routeSeenSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingRouteSeenRoot + handle
}

func (a *webApp) loadRouteSeen(handle string) map[string]time.Time {
	if a.adminRepo == nil {
		return nil
	}
	key := routeSeenSettingKey(handle)
	if key == "" {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("route.seen", fmt.Errorf("decode route seen for %s: %w", handle, err))
		return nil
	}
	out := map[string]time.Time{}
	for route, value := range decoded {
		route = strings.TrimSpace(route)
		at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
		if route == "" || err != nil {
			continue
		}
		out[route] = at.UTC()
	}
	return out
}

func (a *webApp) persistRouteSeen(handle string, rows map[string]time.Time) {
	key := routeSeenSettingKey(handle)
	if key == "" {
		return
	}
	encoded := map[string]string{}
	for route, at := range rows {
		route = strings.TrimSpace(route)
		if route == "" || at.IsZero() {
			continue
		}
		encoded[route] = at.UTC().Format(time.RFC3339Nano)
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("route.seen", fmt.Errorf("encode route seen for %s: %w", handle, err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(key, body)
}

func (a *webApp) markRouteSeen(handle, route string) {
	handle = normalizeHandleKey(handle)
	route = safeLocalRedirectPath(route, "")
	if handle == "" || route == "" {
		return
	}
	rows := a.loadRouteSeen(handle)
	if rows == nil {
		rows = map[string]time.Time{}
	}
	rows[route] = time.Now().UTC()
	a.persistRouteSeen(handle, rows)
}

func routeCadenceDue(cadence string, lastSeen, now time.Time) bool {
	switch normalizeDigestCadence(cadence) {
	case "off":
		return false
	case "always":
		return true
	case "daily":
		return lastSeen.IsZero() || now.Sub(lastSeen) >= 24*time.Hour
	case "weekly":
		return lastSeen.IsZero() || now.Sub(lastSeen) >= 7*24*time.Hour
	default:
		return true
	}
}

func (a *webApp) routeSummaryDue(handle, route, cadence string, now time.Time) bool {
	rows := a.loadRouteSeen(handle)
	if rows == nil {
		return routeCadenceDue(cadence, time.Time{}, now)
	}
	return routeCadenceDue(cadence, rows[safeLocalRedirectPath(route, "")], now)
}

func weeklyDigestSentSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingWeeklyDigestSentRoot + handle
}

func (a *webApp) loadWeeklyDigestSentAt(handle string) time.Time {
	if a.adminRepo == nil {
		return time.Time{}
	}
	key := weeklyDigestSentSettingKey(handle)
	if key == "" {
		return time.Time{}
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}
	}
	return at.UTC()
}

func (a *webApp) persistWeeklyDigestSentAt(handle string, at time.Time) {
	key := weeklyDigestSentSettingKey(handle)
	if key == "" {
		return
	}
	body := ""
	if !at.IsZero() {
		body = at.UTC().Format(time.RFC3339Nano)
	}
	a.persistSystemSetting(key, body)
}

func normalizeMailUrgency(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fyi", "urgent", "asap", "normal":
		return strings.ToLower(strings.TrimSpace(value))
	case "low":
		return "fyi"
	default:
		return "normal"
	}
}

func mailUrgencyOptionRows(current string) string {
	current = normalizeMailUrgency(current)
	options := []struct {
		Value string
		Label string
	}{
		{Value: "normal", Label: "normal"},
		{Value: "fyi", Label: "FYI"},
		{Value: "urgent", Label: "urgent"},
		{Value: "asap", Label: "ASAP"},
	}
	var out strings.Builder
	for _, option := range options {
		out.WriteString(`<option value="` + option.Value + `"` + selectedIf(option.Value == current) + `>` + option.Label + `</option>`)
	}
	return out.String()
}

func applyMailUrgency(subject, urgency string) string {
	subject = strings.TrimSpace(subject)
	urgency = normalizeMailUrgency(urgency)
	_, base := splitMailUrgency(subject)
	switch urgency {
	case "fyi":
		return "[FYI] " + base
	case "urgent":
		return "[URGENT] " + base
	case "asap":
		return "[ASAP] " + base
	default:
		return base
	}
}

func splitMailUrgency(subject string) (string, string) {
	subject = strings.TrimSpace(subject)
	upper := strings.ToUpper(subject)
	switch {
	case strings.HasPrefix(upper, "[FYI] "):
		return "fyi", strings.TrimSpace(subject[len("[FYI] "):])
	case strings.HasPrefix(upper, "[URGENT] "):
		return "urgent", strings.TrimSpace(subject[len("[URGENT] "):])
	case strings.HasPrefix(upper, "[ASAP] "):
		return "asap", strings.TrimSpace(subject[len("[ASAP] "):])
	case strings.HasPrefix(upper, "[LOW] "):
		return "fyi", strings.TrimSpace(subject[len("[LOW] "):])
	default:
		return "normal", subject
	}
}

func mailUrgencyBadgeHTML(subject string) string {
	urgency, _ := splitMailUrgency(subject)
	switch urgency {
	case "fyi":
		return `<span class="wolfbbs-status-pill">FYI</span> `
	case "urgent":
		return `<span class="wolfbbs-status-pill danger">urgent</span> `
	case "asap":
		return `<span class="wolfbbs-status-pill danger">ASAP</span> `
	default:
		return ``
	}
}

func (a *webApp) sendWeeklyDigestMail(user *domain.User, pref digestPreferences, now time.Time) error {
	if user == nil || a.mailRepo == nil {
		return nil
	}
	if !pref.WeeklyMail {
		return nil
	}
	if !routeCadenceDue("weekly", a.loadWeeklyDigestSentAt(user.Handle), now) {
		return nil
	}
	fromUser, err := a.authSvc.GetUser("mailbot")
	if err != nil || fromUser == nil {
		return fmt.Errorf("mailbot account unavailable")
	}
	maxItems := a.digestMaxItemsForUser(user.Handle, pref.MaxItems, now)
	digest, err := discovery.BuildSinceLastCall(a.boardRepo, a.msgRepo, a.mailRepo, user, maxItems)
	if err != nil {
		return err
	}
	visibleBoards := a.visibleBoardsFor(user)
	boardPulse := []boardPulseRow{}
	if pref.IncludeBoards {
		boardPulse = a.buildBoardPulse(user, filterBoardsByWatch(visibleBoards, a.boardSubscriptionIDs(user.Handle, boardSubscriptionDigest)), 6)
	}
	events := []communityEvent{}
	if pref.IncludeEvents {
		events = a.upcomingCommunityEvents(4, now)
	}
	lines := []string{
		"Weekly WolfBBS digest",
		"",
		"Direct follow-up:",
	}
	if len(digest.Items) == 0 {
		lines = append(lines, "- No new direct follow-up.")
	} else {
		for _, item := range digest.Items {
			lines = append(lines, "- "+item.Line)
		}
	}
	lines = append(lines, "", "Digest-tier boards:")
	if len(boardPulse) == 0 {
		lines = append(lines, "- No digest-tier board movement.")
	} else {
		for _, row := range boardPulse {
			lines = append(lines, fmt.Sprintf("- %s: %d new, last %s (%s)", row.BoardName, row.NewCount, row.LastSubject, row.LastAt))
		}
	}
	lines = append(lines, "", "Upcoming events:")
	if len(events) == 0 {
		lines = append(lines, "- No scheduled events.")
	} else {
		for _, row := range events {
			lines = append(lines, "- "+row.Title+" | "+formatCommunityEventWindow(row))
		}
	}
	lines = append(lines, "", "Open /digest, /today, or /attention for the live web views.")
	if err := a.mailRepo.CreateMail(&domain.PrivateMail{
		FromUserID: fromUser.ID,
		ToUserID:   user.ID,
		Subject:    "Weekly Digest",
		Body:       strings.Join(lines, "\n"),
	}); err != nil {
		return err
	}
	a.persistWeeklyDigestSentAt(user.Handle, now)
	return nil
}

func bookmarkSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingBookmarksRoot + handle
}

func normalizeBookmarkEntry(entry bookmarkEntry) (bookmarkEntry, bool) {
	entry.Kind = strings.ToLower(strings.TrimSpace(entry.Kind))
	entry.Label = strings.TrimSpace(entry.Label)
	entry.Href = strings.TrimSpace(entry.Href)
	entry.Meta = strings.TrimSpace(entry.Meta)
	switch entry.Kind {
	case "board_message":
		if entry.MessageID <= 0 {
			return bookmarkEntry{}, false
		}
		entry.Key = "board:" + strconv.FormatInt(entry.MessageID, 10)
		if entry.Href == "" {
			if entry.BoardID > 0 {
				entry.Href = fmt.Sprintf("/boards?board=%d&id=%d", entry.BoardID, entry.MessageID)
			} else {
				entry.Href = "/boards?id=" + strconv.FormatInt(entry.MessageID, 10)
			}
		}
	case "mail":
		if entry.MailID <= 0 {
			return bookmarkEntry{}, false
		}
		entry.Key = "mail:" + strconv.FormatInt(entry.MailID, 10)
		if entry.Href == "" {
			entry.Href = "/mail?id=" + strconv.FormatInt(entry.MailID, 10)
		}
	default:
		return bookmarkEntry{}, false
	}
	if entry.Label == "" {
		entry.Label = entry.Key
	}
	if entry.AddedAt.IsZero() {
		entry.AddedAt = time.Now().UTC()
	}
	return entry, true
}

func (a *webApp) loadBookmarks(handle string) []bookmarkEntry {
	if a.adminRepo == nil {
		return nil
	}
	key := bookmarkSettingKey(handle)
	if key == "" {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var decoded []bookmarkEntry
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("bookmarks", fmt.Errorf("decode bookmarks for %s: %w", handle, err))
		return nil
	}
	out := make([]bookmarkEntry, 0, len(decoded))
	for _, row := range decoded {
		if normalized, ok := normalizeBookmarkEntry(row); ok {
			out = append(out, normalized)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AddedAt.Equal(out[j].AddedAt) {
			return out[i].Key < out[j].Key
		}
		return out[i].AddedAt.After(out[j].AddedAt)
	})
	if len(out) > maxBookmarkItems {
		out = out[:maxBookmarkItems]
	}
	return out
}

func (a *webApp) persistBookmarks(handle string, rows []bookmarkEntry) {
	key := bookmarkSettingKey(handle)
	if key == "" {
		return
	}
	clean := make([]bookmarkEntry, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		normalized, ok := normalizeBookmarkEntry(row)
		if !ok {
			continue
		}
		if _, exists := seen[normalized.Key]; exists {
			continue
		}
		seen[normalized.Key] = struct{}{}
		clean = append(clean, normalized)
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i].AddedAt.Equal(clean[j].AddedAt) {
			return clean[i].Key < clean[j].Key
		}
		return clean[i].AddedAt.After(clean[j].AddedAt)
	})
	if len(clean) > maxBookmarkItems {
		clean = clean[:maxBookmarkItems]
	}
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			a.addAppError("bookmarks", fmt.Errorf("encode bookmarks for %s: %w", handle, err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(key, body)
}

func (a *webApp) hasBookmark(handle, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	for _, row := range a.loadBookmarks(handle) {
		if row.Key == key {
			return true
		}
	}
	return false
}

func (a *webApp) addBookmark(handle string, entry bookmarkEntry) {
	normalized, ok := normalizeBookmarkEntry(entry)
	if !ok {
		return
	}
	rows := a.loadBookmarks(handle)
	next := make([]bookmarkEntry, 0, len(rows)+1)
	next = append(next, normalized)
	for _, row := range rows {
		if row.Key == normalized.Key {
			continue
		}
		next = append(next, row)
	}
	a.persistBookmarks(handle, next)
}

func (a *webApp) removeBookmark(handle, key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	rows := a.loadBookmarks(handle)
	if len(rows) == 0 {
		return
	}
	next := make([]bookmarkEntry, 0, len(rows))
	for _, row := range rows {
		if row.Key != key {
			next = append(next, row)
		}
	}
	a.persistBookmarks(handle, next)
}

func favoriteCallerSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingFavoriteCallersRoot + handle
}

func normalizeFavoriteCallers(rows []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		handle := strings.TrimSpace(row)
		if handle == "" {
			continue
		}
		key := normalizeHandleKey(handle)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, handle)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	if len(out) > maxFavoriteCallers {
		out = out[:maxFavoriteCallers]
	}
	return out
}

func (a *webApp) loadFavoriteCallers(handle string) []string {
	if a.adminRepo == nil {
		return nil
	}
	key := favoriteCallerSettingKey(handle)
	if key == "" {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []string
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("favorite.callers", fmt.Errorf("decode favorites for %s: %w", handle, err))
		return nil
	}
	return normalizeFavoriteCallers(rows)
}

func (a *webApp) persistFavoriteCallers(handle string, rows []string) {
	key := favoriteCallerSettingKey(handle)
	if key == "" {
		return
	}
	clean := normalizeFavoriteCallers(rows)
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			a.addAppError("favorite.callers", fmt.Errorf("encode favorites for %s: %w", handle, err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(key, body)
}

func (a *webApp) isFavoriteCaller(handle, target string) bool {
	targetKey := normalizeHandleKey(target)
	if targetKey == "" {
		return false
	}
	for _, row := range a.loadFavoriteCallers(handle) {
		if normalizeHandleKey(row) == targetKey {
			return true
		}
	}
	return false
}

func (a *webApp) toggleFavoriteCaller(handle, target string) bool {
	target = strings.TrimSpace(target)
	targetKey := normalizeHandleKey(target)
	if targetKey == "" {
		return false
	}
	rows := a.loadFavoriteCallers(handle)
	next := make([]string, 0, len(rows))
	removed := false
	for _, row := range rows {
		if normalizeHandleKey(row) == targetKey {
			removed = true
			continue
		}
		next = append(next, row)
	}
	if removed {
		a.persistFavoriteCallers(handle, next)
		return false
	}
	next = append([]string{target}, next...)
	a.persistFavoriteCallers(handle, next)
	return true
}

func staffNoteSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingStaffNotesRoot + handle
}

func (a *webApp) loadStaffNote(handle string) string {
	if a.adminRepo == nil {
		return ""
	}
	key := staffNoteSettingKey(handle)
	if key == "" {
		return ""
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(raw)
}

func (a *webApp) persistStaffNote(handle, note string) {
	key := staffNoteSettingKey(handle)
	if key == "" {
		return
	}
	a.persistSystemSetting(key, strings.TrimSpace(note))
}

func publicProfileSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingPublicProfileRoot + handle
}

func normalizeContactPrefs(rows []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		value := strings.ToLower(strings.TrimSpace(row))
		switch value {
		case "mail", "page", "chat":
		default:
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizePublicProfileSettings(row publicProfileSettings) publicProfileSettings {
	row.StatusLine = cleanOneLiner(row.StatusLine, 96)
	row.Bio = strings.TrimSpace(row.Bio)
	if len([]rune(row.Bio)) > 800 {
		row.Bio = string([]rune(row.Bio)[:800])
	}
	row.ContactPrefs = normalizeContactPrefs(row.ContactPrefs)
	return row
}

func (a *webApp) loadPublicProfileSettings(handle string) publicProfileSettings {
	row := publicProfileSettings{
		ShowStatusLine: true,
		ShowBio:        true,
		ShowContact:    true,
	}
	if a.adminRepo == nil {
		return row
	}
	key := publicProfileSettingKey(handle)
	if key == "" {
		return row
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return row
	}
	if err := json.Unmarshal([]byte(raw), &row); err != nil {
		a.addAppError("profile.public", fmt.Errorf("decode public profile for %s: %w", handle, err))
		return publicProfileSettings{ShowStatusLine: true, ShowBio: true, ShowContact: true}
	}
	lowerRaw := strings.ToLower(raw)
	if !strings.Contains(lowerRaw, "show_status_line") && !strings.Contains(lowerRaw, "show_bio") && !strings.Contains(lowerRaw, "show_contact") {
		row.ShowStatusLine = true
		row.ShowBio = true
		row.ShowContact = true
	}
	return normalizePublicProfileSettings(row)
}

func (a *webApp) persistPublicProfileSettings(handle string, row publicProfileSettings) {
	key := publicProfileSettingKey(handle)
	if key == "" {
		return
	}
	row = normalizePublicProfileSettings(row)
	raw, err := json.Marshal(row)
	if err != nil {
		a.addAppError("profile.public", fmt.Errorf("encode public profile for %s: %w", handle, err))
		return
	}
	a.persistSystemSetting(key, string(raw))
}

func contactAliasSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingContactAliasesRoot + handle
}

func normalizeContactAliases(rows map[string]string) map[string]string {
	if len(rows) == 0 {
		return nil
	}
	out := map[string]string{}
	for handle, alias := range rows {
		key := normalizeHandleKey(handle)
		alias = cleanOneLiner(alias, 40)
		if key == "" || alias == "" {
			continue
		}
		out[key] = alias
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (a *webApp) loadContactAliases(handle string) map[string]string {
	if a.adminRepo == nil {
		return nil
	}
	key := contactAliasSettingKey(handle)
	if key == "" {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	rows := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("profile.aliases", fmt.Errorf("decode contact aliases for %s: %w", handle, err))
		return nil
	}
	return normalizeContactAliases(rows)
}

func (a *webApp) persistContactAliases(handle string, rows map[string]string) {
	key := contactAliasSettingKey(handle)
	if key == "" {
		return
	}
	rows = normalizeContactAliases(rows)
	body := ""
	if len(rows) > 0 {
		raw, err := json.Marshal(rows)
		if err != nil {
			a.addAppError("profile.aliases", fmt.Errorf("encode contact aliases for %s: %w", handle, err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(key, body)
}

func (a *webApp) setContactAlias(owner, target, alias string) {
	target = normalizeHandleKey(target)
	if target == "" {
		return
	}
	rows := a.loadContactAliases(owner)
	if rows == nil {
		rows = map[string]string{}
	}
	alias = cleanOneLiner(alias, 40)
	if alias == "" {
		delete(rows, target)
	} else {
		rows[target] = alias
	}
	a.persistContactAliases(owner, rows)
}

func (a *webApp) contactAlias(owner, target string) string {
	return a.loadContactAliases(owner)[normalizeHandleKey(target)]
}

func callerCircleSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingCallerCirclesRoot + handle
}

func normalizeCallerCircles(rows []callerCircle) []callerCircle {
	out := make([]callerCircle, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		row.ID = cleanOneLiner(defaultIfBlank(row.ID, randomEventID()), 48)
		row.Name = cleanOneLiner(row.Name, 48)
		row.Note = cleanOneLiner(row.Note, 120)
		if row.Name == "" {
			continue
		}
		if _, ok := seen[row.ID]; ok {
			continue
		}
		seen[row.ID] = struct{}{}
		row.Members = normalizeFavoriteCallers(row.Members)
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	if len(out) > 24 {
		out = out[:24]
	}
	return out
}

func (a *webApp) loadCallerCircles(handle string) []callerCircle {
	if a.adminRepo == nil {
		return nil
	}
	key := callerCircleSettingKey(handle)
	if key == "" {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []callerCircle
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("profile.circles", fmt.Errorf("decode circles for %s: %w", handle, err))
		return nil
	}
	return normalizeCallerCircles(rows)
}

func (a *webApp) persistCallerCircles(handle string, rows []callerCircle) {
	key := callerCircleSettingKey(handle)
	if key == "" {
		return
	}
	rows = normalizeCallerCircles(rows)
	body := ""
	if len(rows) > 0 {
		raw, err := json.Marshal(rows)
		if err != nil {
			a.addAppError("profile.circles", fmt.Errorf("encode circles for %s: %w", handle, err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(key, body)
}

func (a *webApp) upsertCallerCircle(handle, circleID, name, note string, members []string) {
	rows := a.loadCallerCircles(handle)
	circleID = strings.TrimSpace(circleID)
	updated := false
	for idx, row := range rows {
		if circleID == "" || row.ID != circleID {
			continue
		}
		rows[idx].Name = name
		rows[idx].Note = note
		rows[idx].Members = members
		updated = true
		break
	}
	if !updated {
		rows = append(rows, callerCircle{
			ID:      defaultIfBlank(circleID, randomEventID()),
			Name:    name,
			Note:    note,
			Members: members,
		})
	}
	a.persistCallerCircles(handle, rows)
}

func (a *webApp) deleteCallerCircle(handle, circleID string) {
	circleID = strings.TrimSpace(circleID)
	if circleID == "" {
		return
	}
	rows := a.loadCallerCircles(handle)
	next := make([]callerCircle, 0, len(rows))
	for _, row := range rows {
		if row.ID != circleID {
			next = append(next, row)
		}
	}
	a.persistCallerCircles(handle, next)
}

func circlesForHandle(rows []callerCircle, target string) []string {
	targetKey := normalizeHandleKey(target)
	if targetKey == "" {
		return nil
	}
	out := []string{}
	for _, row := range rows {
		for _, member := range row.Members {
			if normalizeHandleKey(member) == targetKey {
				out = append(out, row.Name)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func moderatorInboxAssignmentStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "open", "assigned", "resolved":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "open"
	}
}

func (a *webApp) loadModeratorInboxAssignments() map[int64]moderatorInboxAssignment {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingModInboxAssignments)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := map[string]moderatorInboxAssignment{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("mail.shared", fmt.Errorf("decode moderator inbox assignments: %w", err))
		return nil
	}
	out := map[int64]moderatorInboxAssignment{}
	for rawID, row := range decoded {
		id, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		row.MailID = id
		row.Assignee = cleanOneLiner(row.Assignee, 48)
		row.Note = cleanOneLiner(row.Note, 160)
		row.UpdatedBy = cleanOneLiner(row.UpdatedBy, 48)
		row.Status = moderatorInboxAssignmentStatus(row.Status)
		out[id] = row
	}
	return out
}

func (a *webApp) persistModeratorInboxAssignments(rows map[int64]moderatorInboxAssignment) {
	body := ""
	if len(rows) > 0 {
		encoded := map[string]moderatorInboxAssignment{}
		for id, row := range rows {
			if id <= 0 {
				continue
			}
			row.MailID = id
			row.Status = moderatorInboxAssignmentStatus(row.Status)
			row.Assignee = cleanOneLiner(row.Assignee, 48)
			row.Note = cleanOneLiner(row.Note, 160)
			row.UpdatedBy = cleanOneLiner(row.UpdatedBy, 48)
			encoded[strconv.FormatInt(id, 10)] = row
		}
		if len(encoded) > 0 {
			raw, err := json.Marshal(encoded)
			if err != nil {
				a.addAppError("mail.shared", fmt.Errorf("encode moderator inbox assignments: %w", err))
				return
			}
			body = string(raw)
		}
	}
	a.persistSystemSetting(sysSettingModInboxAssignments, body)
}

func (a *webApp) updateModeratorInboxAssignment(mailID int64, row moderatorInboxAssignment) {
	if mailID <= 0 {
		return
	}
	rows := a.loadModeratorInboxAssignments()
	if rows == nil {
		rows = map[int64]moderatorInboxAssignment{}
	}
	row.MailID = mailID
	row.Status = moderatorInboxAssignmentStatus(row.Status)
	rows[mailID] = row
	a.persistModeratorInboxAssignments(rows)
}

func eventRSVPSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingEventRSVPRoot + handle
}

func normalizeEventRSVPStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "going", "maybe", "declined":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func (a *webApp) loadEventRSVPs(handle string) map[string]eventRSVP {
	if a.adminRepo == nil {
		return nil
	}
	key := eventRSVPSettingKey(handle)
	if key == "" {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := map[string]eventRSVP{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("events.rsvp", fmt.Errorf("decode rsvps for %s: %w", handle, err))
		return nil
	}
	out := map[string]eventRSVP{}
	for eventID, row := range decoded {
		eventID = strings.TrimSpace(eventID)
		row.EventID = eventID
		row.Status = normalizeEventRSVPStatus(row.Status)
		row.InvitedBy = cleanOneLiner(row.InvitedBy, 48)
		if eventID == "" || row.Status == "" {
			continue
		}
		out[eventID] = row
	}
	return out
}

func (a *webApp) persistEventRSVPs(handle string, rows map[string]eventRSVP) {
	key := eventRSVPSettingKey(handle)
	if key == "" {
		return
	}
	body := ""
	if len(rows) > 0 {
		encoded := map[string]eventRSVP{}
		for eventID, row := range rows {
			eventID = strings.TrimSpace(eventID)
			row.EventID = eventID
			row.Status = normalizeEventRSVPStatus(row.Status)
			row.InvitedBy = cleanOneLiner(row.InvitedBy, 48)
			if eventID == "" || row.Status == "" {
				continue
			}
			encoded[eventID] = row
		}
		if len(encoded) > 0 {
			raw, err := json.Marshal(encoded)
			if err != nil {
				a.addAppError("events.rsvp", fmt.Errorf("encode rsvps for %s: %w", handle, err))
				return
			}
			body = string(raw)
		}
	}
	a.persistSystemSetting(key, body)
}

func (a *webApp) setEventRSVP(handle, eventID, status, invitedBy string) {
	eventID = strings.TrimSpace(eventID)
	status = normalizeEventRSVPStatus(status)
	if eventID == "" {
		return
	}
	rows := a.loadEventRSVPs(handle)
	if rows == nil {
		rows = map[string]eventRSVP{}
	}
	if status == "" {
		delete(rows, eventID)
		a.persistEventRSVPs(handle, rows)
		return
	}
	rows[eventID] = eventRSVP{
		EventID:   eventID,
		Status:    status,
		InvitedBy: invitedBy,
		UpdatedAt: time.Now().UTC(),
	}
	a.persistEventRSVPs(handle, rows)
}

func normalizePageRequest(row pageRequest) (pageRequest, bool) {
	row.ID = strings.TrimSpace(row.ID)
	row.From = strings.TrimSpace(row.From)
	row.To = strings.TrimSpace(row.To)
	row.Message = cleanOneLiner(strings.TrimSpace(row.Message), 160)
	row.Source = cleanOneLiner(strings.TrimSpace(row.Source), 80)
	if row.ID == "" || row.From == "" || row.To == "" || row.CreatedAt.IsZero() {
		return pageRequest{}, false
	}
	return row, true
}

func (a *webApp) loadPageRequests() []pageRequest {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingPageRequests)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []pageRequest
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("page.requests", fmt.Errorf("decode page requests: %w", err))
		return nil
	}
	out := make([]pageRequest, 0, len(rows))
	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	for _, row := range rows {
		normalized, ok := normalizePageRequest(row)
		if !ok || normalized.CreatedAt.Before(cutoff) {
			continue
		}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > maxPageRequests {
		out = out[:maxPageRequests]
	}
	return out
}

func (a *webApp) persistPageRequests(rows []pageRequest) {
	if a.adminRepo == nil {
		return
	}
	clean := make([]pageRequest, 0, len(rows))
	for _, row := range rows {
		normalized, ok := normalizePageRequest(row)
		if !ok {
			continue
		}
		clean = append(clean, normalized)
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i].CreatedAt.Equal(clean[j].CreatedAt) {
			return clean[i].ID > clean[j].ID
		}
		return clean[i].CreatedAt.After(clean[j].CreatedAt)
	})
	if len(clean) > maxPageRequests {
		clean = clean[:maxPageRequests]
	}
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			a.addAppError("page.requests", fmt.Errorf("encode page requests: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingPageRequests, body)
}

func (a *webApp) queuePageRequest(from, to, message, source string) {
	rows := a.loadPageRequests()
	next := append([]pageRequest{{
		ID:        randomEventID(),
		From:      strings.TrimSpace(from),
		To:        strings.TrimSpace(to),
		Message:   strings.TrimSpace(message),
		Source:    strings.TrimSpace(source),
		CreatedAt: time.Now().UTC(),
	}}, rows...)
	a.persistPageRequests(next)
}

func (a *webApp) activePageRequestsFor(handle string, limit int) []pageRequest {
	handleKey := normalizeHandleKey(handle)
	if handleKey == "" {
		return nil
	}
	rows := a.loadPageRequests()
	out := make([]pageRequest, 0, len(rows))
	for _, row := range rows {
		if normalizeHandleKey(row.To) != handleKey {
			continue
		}
		out = append(out, row)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (a *webApp) resolvePageRequest(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	rows := a.loadPageRequests()
	if len(rows) == 0 {
		return false
	}
	next := make([]pageRequest, 0, len(rows))
	removed := false
	for _, row := range rows {
		if row.ID == id {
			removed = true
			continue
		}
		next = append(next, row)
	}
	if removed {
		a.persistPageRequests(next)
	}
	return removed
}

func pageAttentionKey(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return "page:" + id
}

func (a *webApp) suggestHandles(query, exclude string, limit int) []string {
	if limit <= 0 {
		limit = maxHandleSuggestions
	}
	users, err := a.authSvc.ListUsers()
	if err != nil {
		return nil
	}
	query = strings.ToLower(strings.TrimSpace(query))
	exclude = strings.ToLower(strings.TrimSpace(exclude))
	prefix := make([]string, 0, limit)
	contains := make([]string, 0, limit)
	for _, row := range users {
		if !row.Enabled || row.Banned {
			continue
		}
		handle := strings.TrimSpace(row.Handle)
		if handle == "" {
			continue
		}
		lower := strings.ToLower(handle)
		if lower == exclude {
			continue
		}
		if query == "" {
			prefix = append(prefix, handle)
		} else if strings.HasPrefix(lower, query) {
			prefix = append(prefix, handle)
		} else if strings.Contains(lower, query) {
			contains = append(contains, handle)
		}
	}
	sort.Slice(prefix, func(i, j int) bool { return strings.ToLower(prefix[i]) < strings.ToLower(prefix[j]) })
	sort.Slice(contains, func(i, j int) bool { return strings.ToLower(contains[i]) < strings.ToLower(contains[j]) })
	out := append(prefix, contains...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (a *webApp) boardBookmarkEntry(msg *domain.Message, board *domain.Board) bookmarkEntry {
	if msg == nil {
		return bookmarkEntry{}
	}
	boardID := msg.BoardID
	boardName := "Board " + strconv.FormatInt(boardID, 10)
	if board != nil {
		boardID = board.ID
		if strings.TrimSpace(board.Name) != "" {
			boardName = board.Name
		}
	}
	handle := a.userHandleLookup()[msg.AuthorID]
	if handle == "" {
		handle = "#" + strconv.FormatInt(msg.AuthorID, 10)
	}
	return bookmarkEntry{
		Kind:      "board_message",
		BoardID:   boardID,
		MessageID: msg.ID,
		Label:     defaultIfBlank(strings.TrimSpace(msg.Subject), "Message #"+strconv.FormatInt(msg.ID, 10)),
		Meta:      boardName + " • " + handle + " • " + msg.CreatedAt.Local().Format("2006-01-02 15:04"),
		Href:      fmt.Sprintf("/boards?board=%d&id=%d", boardID, msg.ID),
		AddedAt:   time.Now().UTC(),
	}
}

func (a *webApp) mailBookmarkEntry(item *domain.PrivateMail, current *domain.User) bookmarkEntry {
	if item == nil {
		return bookmarkEntry{}
	}
	handleByID := a.userHandleLookup()
	other := handleByID[item.FromUserID]
	if current != nil && item.FromUserID == current.ID {
		if item.ExternalTo != nil && strings.TrimSpace(*item.ExternalTo) != "" {
			other = strings.TrimSpace(*item.ExternalTo)
		} else if handleByID[item.ToUserID] != "" {
			other = handleByID[item.ToUserID]
		}
	}
	if other == "" {
		other = "mail"
	}
	return bookmarkEntry{
		Kind:    "mail",
		MailID:  item.ID,
		Label:   defaultIfBlank(strings.TrimSpace(item.Subject), "Mail #"+strconv.FormatInt(item.ID, 10)),
		Meta:    other + " • " + item.CreatedAt.Local().Format("2006-01-02 15:04"),
		Href:    "/mail?id=" + strconv.FormatInt(item.ID, 10),
		AddedAt: time.Now().UTC(),
	}
}

func normalizeFileReviewStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case fileReviewApproved:
		return fileReviewApproved
	case fileReviewRejected:
		return fileReviewRejected
	default:
		return fileReviewHold
	}
}

func (a *webApp) loadFileReviewQueue() map[int64]fileReviewItem {
	out := map[int64]fileReviewItem{}
	if a.adminRepo == nil {
		return out
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingFileReviewQueue)
	if err != nil || strings.TrimSpace(raw) == "" {
		return out
	}
	decoded := map[string]fileReviewItem{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("file.review", fmt.Errorf("decode file review queue: %w", err))
		return out
	}
	for rawID, row := range decoded {
		fileID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || fileID <= 0 {
			continue
		}
		row.FileID = fileID
		row.Status = normalizeFileReviewStatus(row.Status)
		out[fileID] = row
	}
	return out
}

func (a *webApp) persistFileReviewQueue(rows map[int64]fileReviewItem) {
	encoded := map[string]fileReviewItem{}
	for fileID, row := range rows {
		if fileID <= 0 {
			continue
		}
		row.FileID = fileID
		row.Status = normalizeFileReviewStatus(row.Status)
		encoded[strconv.FormatInt(fileID, 10)] = row
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("file.review", fmt.Errorf("encode file review queue: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingFileReviewQueue, body)
}

func (a *webApp) setFileReviewItem(item fileReviewItem) {
	if item.FileID <= 0 {
		return
	}
	rows := a.loadFileReviewQueue()
	item.Status = normalizeFileReviewStatus(item.Status)
	rows[item.FileID] = item
	a.persistFileReviewQueue(rows)
}

func (a *webApp) removeFileReviewItem(fileID int64) {
	if fileID <= 0 {
		return
	}
	rows := a.loadFileReviewQueue()
	delete(rows, fileID)
	a.persistFileReviewQueue(rows)
}

func (a *webApp) fileReviewItem(fileID int64) (fileReviewItem, bool) {
	if fileID <= 0 {
		return fileReviewItem{}, false
	}
	row, ok := a.loadFileReviewQueue()[fileID]
	return row, ok
}

func (a *webApp) reviewStatusLabel(fileID int64) string {
	row, ok := a.fileReviewItem(fileID)
	if !ok {
		return "live"
	}
	return row.Status
}

func (a *webApp) fileVisibleToCallers(fileID int64) bool {
	row, ok := a.fileReviewItem(fileID)
	if !ok {
		return true
	}
	switch row.Status {
	case fileReviewHold, fileReviewRejected:
		return false
	default:
		return true
	}
}

func (a *webApp) filterVisibleFiles(rows []domain.FileEntry) []domain.FileEntry {
	if len(rows) == 0 {
		return nil
	}
	out := make([]domain.FileEntry, 0, len(rows))
	for _, row := range rows {
		if a.fileVisibleToCallers(row.ID) {
			out = append(out, row)
		}
	}
	return out
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
	nav := `<a href="/connect">connect</a> | <a href="/tour">tour</a> | <a href="/showcase">showcase</a> | <a href="/events">events</a> | <a href="/login">login</a> | <a href="/help">help</a>`
	intro := `<p>Start here when you want a clear next step instead of hunting through routes.</p>`
	kpis := `<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>Guest</strong><span>tour, connect, evaluate</span></article><article class="wolfbbs-kpi-card"><strong>Caller</strong><span>boards, attention, doors</span></article><article class="wolfbbs-kpi-card"><strong>Sysop</strong><span>setup, ops, launch</span></article></section>`
	laneGrid := `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Just exploring</h2><div class="wolfbbs-action-grid"><a class="wolfbbs-action-card" href="/connect"><strong>Connect</strong><span>SSH, web terminal, IRC, and clipboard-ready commands</span></a><a class="wolfbbs-action-card" href="/tour"><strong>Guided Tour</strong><span>Read-only walkthrough of the product shape</span></a><a class="wolfbbs-action-card" href="/showcase"><strong>Showcase</strong><span>Feature map, launch path, and capability highlights</span></a><a class="wolfbbs-action-card" href="/help"><strong>Help</strong><span>Route map and surface guide</span></a></div></article><article class="wolfbbs-card"><h2>What success looks like</h2><ul class="wolfbbs-list-clean"><li>Guests should understand what the board does in under five minutes.</li><li>Callers should know where to go next after the first login.</li><li>Sysops should know whether the board is truly launch-ready.</li></ul></article></section>`
	if user == nil {
		page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>` + htmlEscape(pageTitle) + `</title></head><body><h1>` + htmlEscape(pageTitle) + `</h1><p>` + nav + `</p>` + intro + kpis + laneGrid + guestQuickStartChecklistHTML() + `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Caller path</h2><ol><li>Open <a href="/connect">/connect</a> or <a href="/tour">/tour</a>.</li><li>Create or use an account and sign in.</li><li>Start with boards, chat, doors, and mail.</li></ol></article><article class="wolfbbs-card"><h2>Sysop path</h2><ol><li>Sign in as sysop.</li><li>Finish <a href="/admin/setup">/admin/setup</a>.</li><li>Use <a href="/admin/launch">/admin/launch</a> and <a href="/status">/status</a> before inviting callers.</li></ol></article></section></body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page))
		return
	}

	role := rbac.NormalizeRole(user.Role)
	nav = `<a href="/boards">boards</a> | <a href="/attention">attention</a> | <a href="/mail">mail</a> | <a href="/doors">doors</a> | <a href="/radar">radar</a> | <a href="/showcase">showcase</a> | <a href="/help">help</a> | <a href="/logout">logout</a>`
	if a.hasRole(user, roleAdmin) {
		nav = `<a href="/admin">admin</a> | <a href="/admin/setup">setup</a> | <a href="/admin/ops">ops</a> | <a href="/admin/events">events</a> | <a href="/admin/launch">launch</a> | <a href="/status">status</a> | <a href="/boards">boards</a> | <a href="/showcase">showcase</a> | <a href="/help">help</a> | <a href="/logout">logout</a>`
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
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>` + htmlEscape(pageTitle) + `</title></head><body><h1>` + htmlEscape(pageTitle) + `</h1><p>` + nav + `</p>` + intro + kpis + hero + firstCallBlock + `<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Use Today first</strong><p><a href="/today">/today</a> is the shortest daily caller loop once you are signed in.</p></article><article class="wolfbbs-helper-card"><strong>Use Discover for narrative catch-up</strong><p>When you want a broader digest instead of direct action items, open <a href="/discover">/discover</a>.</p></article><article class="wolfbbs-helper-card"><strong>Use Showcase to orient quickly</strong><p><a href="/showcase">/showcase</a> maps core features into a practical first-run path.</p></article><article class="wolfbbs-helper-card"><strong>Use SSH when you want the full board feel</strong><p><a href="/connect">/connect</a> remains the best starting point for the terminal-first experience.</p></article></section></body></html>`
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
			a.recordOperatorInsight("first_call.starter_post", user.Handle, "/first-call")
			a.recordFirstCallTransition(user, snapshot, "/first-call")
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
			a.recordOperatorInsight("first_call.starter_chat", user.Handle, "/first-call")
			a.recordFirstCallTransition(user, snapshot, "/first-call")
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
			a.recordOperatorInsight("first_call.starter_mail", user.Handle, "/first-call")
			a.recordFirstCallTransition(user, snapshot, "/first-call")
			redirectWithNotice(w, r, "/first-call", "Starter private mail sent.")
			return
		case "save_home_route":
			route := normalizeHomeRoute(r.FormValue("home_route"))
			if route == "" {
				redirectWithError(w, r, "/first-call", "Choose a home route first.")
				return
			}
			a.persistHomeRoute(user.Handle, route)
			a.recordOperatorInsight("first_call.home_route", user.Handle, route)
			a.recordFirstCallTransition(user, snapshot, route)
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
	loadPages := func() []pageRequest {
		return a.activePageRequestsFor(user.Handle, 8)
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
				if key != "" && !a.isAttentionKeyDismissed(user.Handle, key) && a.attentionSnoozedUntil(user.Handle, key).IsZero() {
					keys = append(keys, key)
				}
			}
			for _, row := range boardPulse {
				key := boardAttentionItemKey(row)
				if key != "" && !a.isAttentionKeyDismissed(user.Handle, key) && a.attentionSnoozedUntil(user.Handle, key).IsZero() {
					keys = append(keys, key)
				}
			}
			for _, row := range loadPages() {
				key := pageAttentionKey(row.ID)
				if key != "" && !a.isAttentionKeyDismissed(user.Handle, key) && a.attentionSnoozedUntil(user.Handle, key).IsZero() {
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
		case "snooze":
			itemKey := strings.TrimSpace(r.FormValue("item_key"))
			if itemKey == "" {
				redirectWithError(w, r, "/attention", "Attention item key is required.")
				return
			}
			hours := parseIntWithFallback(r.FormValue("snooze_hours"), 4)
			if hours < 1 {
				hours = 1
			}
			if hours > 168 {
				hours = 168
			}
			a.snoozeAttentionItem(user.Handle, itemKey, time.Now().UTC().Add(time.Duration(hours)*time.Hour))
			redirectWithNotice(w, r, "/attention", fmt.Sprintf("Attention item snoozed for %d hour(s).", hours))
			return
		case "unsnooze":
			itemKey := strings.TrimSpace(r.FormValue("item_key"))
			if itemKey == "" {
				redirectWithError(w, r, "/attention", "Attention item key is required.")
				return
			}
			a.clearAttentionSnooze(user.Handle, itemKey)
			redirectWithNotice(w, r, "/attention", "Attention item restored from snooze.")
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
	a.markRouteSeen(user.Handle, "/attention")

	digest, boardPulse, upcomingEvents, trackedLabel, watchSubs, digestSubs, err := buildAttentionContext()
	if err != nil {
		http.Error(w, "attention feed unavailable", http.StatusInternalServerError)
		return
	}

	handleByID := a.userHandleLookup()
	csrf := a.csrfHiddenInput(r)

	directUnread := make([]attentionActionRow, 0, 8)
	directSeen := make([]attentionActionRow, 0, 8)
	pageUnread := make([]attentionActionRow, 0, 8)
	pageSeen := make([]attentionActionRow, 0, 8)
	snoozedRows := make([]attentionActionRow, 0, 8)
	boardQueueUnread := make([]attentionActionRow, 0, 8)
	boardUnread := make([]attentionActionRow, 0, 8)
	boardSeen := make([]attentionActionRow, 0, 8)
	allUnread := make([]attentionActionRow, 0, len(digest.Items))
	replyMentions := 0
	livePages := 0
	boardUpdates := 0
	seenCount := 0
	for _, item := range digest.Items {
		itemKey := attentionItemKey(item)
		snoozedUntil := a.attentionSnoozedUntil(user.Handle, itemKey)
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
		if !snoozedUntil.IsZero() {
			row.Meta += " | snoozed until " + snoozedUntil.Local().Format("01-02 15:04")
			snoozedRows = append(snoozedRows, row)
			continue
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
	for _, item := range loadPages() {
		itemKey := pageAttentionKey(item.ID)
		snoozedUntil := a.attentionSnoozedUntil(user.Handle, itemKey)
		if a.isAttentionKeyDismissed(user.Handle, itemKey) {
			continue
		}
		label := "Page from " + item.From
		href := "/mail?to=" + url.QueryEscape(item.From) + "&subject=" + url.QueryEscape("Reply to your page") + "&body=" + url.QueryEscape("Saw your page from "+item.Source+".\n\n")
		meta := item.CreatedAt.Local().Format("2006-01-02 15:04")
		if item.Source != "" {
			meta += " | " + item.Source
		}
		if item.Message != "" {
			meta += " | " + item.Message
		}
		row := attentionActionRow{
			Label:       label,
			Href:        href,
			Meta:        meta,
			ItemKey:     itemKey,
			Read:        a.isAttentionKeyRead(user.Handle, itemKey),
			Dismissible: true,
		}
		if !snoozedUntil.IsZero() {
			row.Meta += " | snoozed until " + snoozedUntil.Local().Format("01-02 15:04")
			snoozedRows = append(snoozedRows, row)
			continue
		}
		if row.Read {
			seenCount++
			pageSeen = append(pageSeen, row)
			continue
		}
		livePages++
		allUnread = append(allUnread, row)
		pageUnread = append(pageUnread, row)
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
			if !row.Read && row.ItemKey != "" {
				list.WriteString(`<form method="POST" action="/attention"><input type="hidden" name="action" value="snooze"><input type="hidden" name="item_key" value="` + htmlEscape(row.ItemKey) + `"><input type="hidden" name="snooze_hours" value="4">` + csrf + `<button type="submit">Snooze 4h</button></form>`)
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
		snoozedUntil := a.attentionSnoozedUntil(user.Handle, itemKey)
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
		if !snoozedUntil.IsZero() {
			actionRow.Meta += " | snoozed until " + snoozedUntil.Local().Format("01-02 15:04")
			snoozedRows = append(snoozedRows, actionRow)
			continue
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
	pageList := renderAttentionList(pageUnread, "No live pages are waiting.")
	seenList := renderAttentionList(append(append([]attentionActionRow{}, directSeen...), boardSeen...), "No previously seen attention items are active.")
	if len(pageSeen) > 0 {
		seenList = renderAttentionList(append(append(append([]attentionActionRow{}, directSeen...), pageSeen...), boardSeen...), "No previously seen attention items are active.")
	}
	snoozedList := strings.Builder{}
	for _, row := range snoozedRows {
		snoozedList.WriteString(`<li><div class="wolfbbs-inline-actions"><a href="` + htmlEscape(row.Href) + `">` + htmlEscape(row.Label) + `</a><span class="wolfbbs-muted">` + htmlEscape(row.Meta) + `</span><form method="POST" action="/attention"><input type="hidden" name="action" value="unsnooze"><input type="hidden" name="item_key" value="` + htmlEscape(row.ItemKey) + `">` + csrf + `<button type="submit">Restore</button></form></div></li>`)
	}
	if snoozedList.Len() == 0 {
		snoozedList.WriteString(`<li>No snoozed items.</li>`)
	}

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
<p><a href="/attention/export">Export my notification rules</a> | <a href="/settings">Tune digest and attention rules</a></p>
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(allUnread)) + `</strong><span>unread attention</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(seenCount) + `</strong><span>read this cycle</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(replyMentions) + `</strong><span>mentions + replies</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(livePages) + `</strong><span>live pages</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(inboxRows)) + `</strong><span>unread mail</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(boardUpdates) + `</strong><span>board updates</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(watchSubs)) + ` / ` + strconv.Itoa(len(digestSubs)) + `</strong><span>watch / digest boards</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(upcomingEvents)) + `</strong><span>upcoming events</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(snoozedRows)) + `</strong><span>snoozed</span></article>
</section>
<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Work top-down</strong><p>Direct replies, live pages, and mentions should be handled first because they are the clearest pending obligations.</p></article><article class="wolfbbs-helper-card"><strong>Unread mail is private follow-up</strong><p>Use inbox for direct conversation, then return to boards when the topic belongs in public.</p></article><article class="wolfbbs-helper-card"><strong>Use tiers deliberately</strong><p>The board pulse is showing ` + htmlEscape(trackedLabel) + `. Watch boards surface here, digest boards stay in <a href="/today">/today</a>, and mute hides noise.</p></article></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Queue Actions</h2><div class="wolfbbs-inline-actions"><form method="POST" action="/attention"><input type="hidden" name="action" value="mark_all_read">` + csrf + `<button type="submit">Mark Current Attention Read</button></form><form method="POST" action="/attention"><input type="hidden" name="action" value="mark_all_mail_read">` + csrf + `<button type="submit">Mark All Mail Read</button></form><form method="POST" action="/attention"><input type="hidden" name="action" value="clear_dismissed">` + csrf + `<button type="submit">Restore Dismissed Items</button></form><a class="wolfbbs-filter-reset" href="/discover">Open Full Discover Feed</a></div></article></section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Live Pages</h2><ul class="wolfbbs-list-clean">` + pageList + `</ul><p><a href="/directory">Caller directory</a> | <a href="/mail">Reply by mail</a></p></article>
<article class="wolfbbs-card"><h2>Direct Follow-Up</h2><ul class="wolfbbs-list-clean">` + directList + `</ul><p><a href="/boards?mode=mentions">Mentions queue</a> | <a href="/boards?mode=mine">Your threads</a></p></article>
<article class="wolfbbs-card"><h2>Inbox Needs Action</h2><ul class="wolfbbs-list-clean">` + inboxList.String() + `</ul><p><a href="/mail?box=unread">Open unread mail</a> | <a href="/mail">Compose</a></p></article>
</section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Watch Tier Board Pulse</h2><ul class="wolfbbs-list-clean">` + boardList + `</ul><p><a href="/boards?mode=watched">Watch tier</a> | <a href="/boards?mode=digest">Digest tier</a> | <a href="/boards?mode=unread">Unread board scan</a></p></article>
<article class="wolfbbs-card"><h2>Community Calendar</h2><ul class="wolfbbs-list-clean">` + eventList.String() + `</ul><p><a href="/events">Open calendar</a> | <a href="/today">Open today brief</a></p></article>
</section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Unread Notification Feed</h2><ul class="wolfbbs-list-clean">` + discoveryList + `</ul><p><a href="/discover">Open full discover feed</a></p></article><article class="wolfbbs-card"><h2>Seen This Cycle</h2><ul class="wolfbbs-list-clean">` + seenList + `</ul><p>Read items stay out of your way until the board moves again.</p></article></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Snoozed</h2><ul class="wolfbbs-list-clean">` + snoozedList.String() + `</ul><p>Snooze is for reminders you intend to come back to later without dismissing them permanently.</p></article></section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAttentionExport(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	payload := a.buildAttentionExport(user)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"wolfbbs-notifications-%s.json\"", normalizeHandleKey(user.Handle)))
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		a.addAppError("attention.export", fmt.Errorf("encode attention export for %s: %w", user.Handle, err))
	}
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
		case "resolve_page":
			id := strings.TrimSpace(r.FormValue("id"))
			if !a.resolvePageRequest(id) {
				redirectWithError(w, r, "/admin/ops", "Page request not found.")
				return
			}
			a.recordAdminAction(user.Handle, "page_requests", "resolve_page", "id="+id)
			redirectWithNotice(w, r, "/admin/ops", "Page request resolved.")
			return
		case "resolve_escalation":
			id := strings.TrimSpace(r.FormValue("id"))
			if !a.resolveStaffEscalation(id) {
				redirectWithError(w, r, "/admin/ops", "Staff escalation not found.")
				return
			}
			a.recordAdminAction(user.Handle, "staff_escalations", "resolve_escalation", "id="+id)
			redirectWithNotice(w, r, "/admin/ops", "Staff escalation resolved.")
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
	pageRequests := a.loadPageRequests()
	escalations := a.loadStaffEscalations()
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
	if len(pageRequests) > 0 {
		nextActions.WriteString(`<li>` + strconv.Itoa(len(pageRequests)) + ` unresolved page request(s) are still live. Clear or respond before they expire silently.</li>`)
	}
	unresolvedEscalations := 0
	for _, row := range escalations {
		if row.ResolvedAt.IsZero() {
			unresolvedEscalations++
		}
	}
	if unresolvedEscalations > 0 {
		nextActions.WriteString(`<li>` + strconv.Itoa(unresolvedEscalations) + ` staff escalation(s) still need closure.</li>`)
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
	pageTable := strings.Builder{}
	for _, row := range pageRequests {
		pageTable.WriteString(`<tr><td>` + row.CreatedAt.Local().Format("2006-01-02 15:04") + `</td><td>` + htmlEscape(row.From) + `</td><td>` + htmlEscape(row.To) + `</td><td>` + htmlEscape(defaultIfBlank(row.Source, "page")) + `</td><td>` + htmlEscape(defaultIfBlank(row.Message, "(no message)")) + `</td><td><form method="POST" action="/admin/ops"><input type="hidden" name="action" value="resolve_page"><input type="hidden" name="id" value="` + htmlEscape(row.ID) + `">` + csrf + `<button type="submit">Resolve</button></form></td></tr>`)
	}
	if pageTable.Len() == 0 {
		pageTable.WriteString(`<tr><td colspan="6">No unresolved pages.</td></tr>`)
	}
	escalationTable := strings.Builder{}
	for _, row := range escalations {
		state := `resolved ` + row.ResolvedAt.Local().Format("2006-01-02 15:04")
		actionCell := `<span class="wolfbbs-muted">closed</span>`
		if row.ResolvedAt.IsZero() {
			state = "open"
			actionCell = `<form method="POST" action="/admin/ops"><input type="hidden" name="action" value="resolve_escalation"><input type="hidden" name="id" value="` + htmlEscape(row.ID) + `">` + csrf + `<button type="submit">Resolve</button></form>`
		}
		escalationTable.WriteString(`<tr><td>` + row.CreatedAt.Local().Format("2006-01-02 15:04") + `</td><td>` + htmlEscape(row.Handle) + `</td><td>` + htmlEscape(defaultIfBlank(row.Actor, "staff")) + `</td><td>` + htmlEscape(row.Note) + `</td><td>` + htmlEscape(state) + `</td><td>` + actionCell + `</td></tr>`)
	}
	if escalationTable.Len() == 0 {
		escalationTable.WriteString(`<tr><td colspan="6">No staff escalations.</td></tr>`)
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Ops Center</title></head><body>
<p><a href="/admin">admin</a> | <a href="/admin/launch">launch</a> | <a href="/admin/setup">setup</a> | <a href="/admin/challenges">challenges</a> | <a href="/admin/upgrade-safety">upgrade safety</a> | <a href="/admin/backups">backups</a> | <a href="/admin/release">release</a> | <a href="/admin/system">system</a> | <a href="/admin/errors">errors</a> | <a href="/admin/audit">audit</a> | <a href="/status">status</a> | <a href="/help">help</a></p>
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
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(pageRequests)) + `</strong><span>open pages</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(unresolvedEscalations) + `</strong><span>staff escalations</span></article>
</section>
<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Readiness before polish</strong><p>Use launch and status signals to decide whether you have a real blocker or just a minor cleanup item.</p></article><article class="wolfbbs-helper-card"><strong>Errors change the meaning of green</strong><p>If runtime errors are piling up, treat every passing screen as provisional until you understand the failures.</p></article><article class="wolfbbs-helper-card"><strong>Audit + sessions explain surprises</strong><p>When callers report something odd, the fastest answers usually come from the recent audit trail and who is online right now.</p></article></section>
` + renderOperatorConfidenceBlock(a, user, readiness, runtime, errors) + `
<section class="wolfbbs-grid">
<article><h2>Current Operator Focus</h2><ul>` + nextActions.String() + `</ul><p><a href="/admin/launch">Launch Center</a> | <a href="/admin/setup">Setup Wizard</a> | <a href="/admin/upgrade-safety">Upgrade Safety</a> | <a href="/admin/backups">Backup Browser</a> | <a href="/admin/release">Release Dashboard</a> | <a href="/status">Caller Status</a></p></article>
<article><h2>Operator Controls</h2><div class="wolfbbs-inline-actions"><form method="POST" action="/admin/ops"><input type="hidden" name="action" value="clear_errors">` + csrf + `<button type="submit">Clear Runtime Errors</button></form><form method="POST" action="/admin/ops" class="wolfbbs-inline-form"><input type="hidden" name="action" value="prune_idle_sessions">` + csrf + `<label>Prune idle sessions older than <input name="idle_minutes" value="30" inputmode="numeric"></label><button type="submit">Prune Sessions</button></form><form method="POST" action="/admin/ops"><input type="hidden" name="action" value="purge_web_sessions">` + csrf + `<button type="submit">Purge Expired Web Sessions</button></form><form method="POST" action="/admin/ops"><input type="hidden" name="action" value="clear_rate_limits">` + csrf + `<button type="submit">Clear Rate Limits</button></form></div><p><strong>Live counters:</strong> ` + strconv.Itoa(webSessionCount) + ` web sessions / ` + strconv.Itoa(rateLimitCount) + ` rate-limit buckets</p><h3>Operator Commands</h3><pre>bash install.sh --status
bash install.sh --doctor
bash install.sh --logs
bash install.sh --upgrade</pre></article>
</section>
<h2>Live Sessions</h2>
<table border="1"><tr><th>User</th><th>Node</th><th>Area</th><th>Login</th><th>Idle</th><th>Origin</th><th>From</th></tr>` + sessionTable.String() + `</table>
<h2>Recent Runtime Errors</h2>
<table border="1"><tr><th>When</th><th>Area</th><th>Error</th></tr>` + errorTable.String() + `</table>
<section class="wolfbbs-grid"><article><h2>Open Pages</h2><table border="1"><tr><th>When</th><th>From</th><th>To</th><th>Source</th><th>Message</th><th>Action</th></tr>` + pageTable.String() + `</table></article><article><h2>Staff Escalations</h2><table border="1"><tr><th>When</th><th>Target</th><th>Actor</th><th>Note</th><th>Status</th><th>Action</th></tr>` + escalationTable.String() + `</table></article></section>
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
	if err := a.sendWeeklyDigestMail(user, a.loadDigestPreferences(user.Handle), time.Now().UTC()); err != nil {
		a.addAppError("digest.weekly_mail", err)
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

	heatmap := a.buildCallerActivityHeatmap(user, 14)
	heatmapHeader := strings.Builder{}
	heatmapCounts := strings.Builder{}
	heatmapLevels := strings.Builder{}
	activeDays := 0
	hottestDay := "No activity yet"
	hottestCount := 0
	for _, cell := range heatmap {
		heatmapHeader.WriteString(`<th title="` + htmlEscape(cell.Detail) + `">` + htmlEscape(cell.ShortLabel) + `<br><span class="wolfbbs-muted">` + htmlEscape(cell.DateLabel) + `</span></th>`)
		countLabel := strconv.Itoa(cell.Count) + ` touch(es)`
		if cell.Count == 0 {
			countLabel = `-`
		} else {
			activeDays++
			if cell.Count > hottestCount {
				hottestCount = cell.Count
				hottestDay = cell.DateLabel + ` (` + strconv.Itoa(cell.Count) + ` touch(es))`
			}
		}
		heatmapCounts.WriteString(`<td title="` + htmlEscape(cell.Detail) + `">` + countLabel + `</td>`)
		heatmapLevels.WriteString(`<td>` + htmlEscape(activityHeatLevelLabel(cell.Level)) + `</td>`)
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Today Brief</title></head><body>
<p><a href="/start">start</a> | <a href="/attention">attention</a> | <a href="/digest">digest</a> | <a href="/boards">boards</a> | <a href="/events">events</a> | <a href="/tournaments">tournaments</a> | <a href="/mail">mail</a> | <a href="/radar">radar</a> | <a href="/doors">doors</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
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
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(activeDays) + ` / 14</strong><span>active days</span></article>
<article class="wolfbbs-kpi-card"><strong>` + htmlEscape(hottestDay) + `</strong><span>hottest activity day</span></article>
</section>
` + onboardingBlock + `
<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Use Today first</strong><p>Start here when you want the shortest path through what changed.</p></article><article class="wolfbbs-helper-card"><strong>Subscription tiers matter</strong><p>Watch boards escalate into Attention Center, digest boards land here, and mute hides a board from routine loops without unsubscribing from it permanently.</p></article><article class="wolfbbs-helper-card"><strong>Events give callers a reason to return</strong><p>Use <a href="/events">/events</a> for the public calendar, <a href="/tournaments">/tournaments</a> for competitive nights, and <a href="/admin/events">/admin/events</a> to schedule recurring rhythms.</p></article></section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Needs Response</h2><ul class="wolfbbs-list-clean">` + nextRows.String() + `</ul><p><a href="/attention">Open Attention Center</a> | <a href="/discover">Open Discover</a></p></article>
<article class="wolfbbs-card"><h2>Community Calendar</h2><ul class="wolfbbs-list-clean">` + eventRows.String() + `</ul><p><a href="/events">Open full calendar</a></p></article>
</section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Activity Heatmap</h2><p class="wolfbbs-muted">Posts, private mail, and completed calls over the last 14 days.</p><table border="1"><tr>` + heatmapHeader.String() + `</tr><tr>` + heatmapCounts.String() + `</tr><tr>` + heatmapLevels.String() + `</tr></table><p><a href="/attention/export">Export notification state</a> | <a href="/settings">Adjust attention rules</a></p></article></section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Watch Tier Boards</h2><p class="wolfbbs-muted">Currently showing ` + htmlEscape(trackedLabel) + `.</p><table border="1"><tr><th>Board</th><th>Conf</th><th>New</th><th>Total</th><th>Last</th><th>Last subject</th></tr>` + boardRows.String() + `</table></article>
<article class="wolfbbs-card"><h2>Digest Tier Boards</h2><p class="wolfbbs-muted">Lower urgency subscriptions stay here instead of interrupting Attention Center.</p><table border="1"><tr><th>Board</th><th>New</th><th>Last</th><th>Last subject</th></tr>` + digestRows.String() + `</table></article>
</section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Next Move</h2><ul><li><a href="/mail">Mail</a> if you need private follow-up.</li><li><a href="/boards?mode=watched">Watch tier</a> when you want threaded catch-up.</li><li><a href="/boards?mode=digest">Digest tier</a> when you want low-noise board scanning.</li><li><a href="/digest">Daily Digest</a> when you want the lower-noise web summary.</li><li><a href="/doors?mode=recommended">Doors</a> when you want a quick return loop.</li><li><a href="/chat">Chat</a> when the conversation should be live.</li></ul></article></section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleDigest(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	pref := a.loadDigestPreferences(user.Handle)
	now := time.Now().UTC()
	if err := a.sendWeeklyDigestMail(user, pref, now); err != nil {
		a.addAppError("digest.weekly_mail", err)
	}
	a.markRouteSeen(user.Handle, "/digest")
	maxItems := a.digestMaxItemsForUser(user.Handle, pref.MaxItems, now)
	digest, err := discovery.BuildSinceLastCall(a.boardRepo, a.msgRepo, a.mailRepo, user, maxItems)
	if err != nil {
		http.Error(w, "digest unavailable", http.StatusInternalServerError)
		return
	}
	visibleBoards := a.visibleBoardsFor(user)
	digestBoards := []boardPulseRow{}
	if pref.IncludeBoards {
		digestBoards = a.buildBoardPulse(user, filterBoardsByWatch(visibleBoards, a.boardSubscriptionIDs(user.Handle, boardSubscriptionDigest)), 8)
	}
	attentionDue := a.routeSummaryDue(user.Handle, "/attention", pref.AttentionCadence, now)
	bulletinDue := a.routeSummaryDue(user.Handle, "/bulletins", pref.BulletinCadence, now)
	eventDue := pref.IncludeEvents && a.routeSummaryDue(user.Handle, "/events", pref.EventCadence, now)
	upcoming := []communityEvent{}
	if eventDue {
		upcoming = a.upcomingCommunityEvents(8, now)
	}
	bulletinSnapshot := bulletinSnapshot{}
	if bulletinDue {
		bulletinSnapshot = a.buildBulletinSnapshot(user)
	}
	itemRows := strings.Builder{}
	if attentionDue {
		for _, row := range digest.Items {
			href := "/mail"
			if row.BoardID > 0 {
				href = "/boards?board=" + strconv.FormatInt(row.BoardID, 10)
				if row.MessageID > 0 {
					href += "&id=" + strconv.FormatInt(row.MessageID, 10)
				}
			}
			itemRows.WriteString(`<li><a href="` + htmlEscape(href) + `">` + htmlEscape(row.Line) + `</a></li>`)
		}
		if itemRows.Len() == 0 {
			itemRows.WriteString(`<li>No direct follow-up accumulated since your last call.</li>`)
		}
	} else {
		itemRows.WriteString(`<li>Attention summary is held by your ` + htmlEscape(pref.AttentionCadence) + ` cadence. Open <a href="/attention">/attention</a> for the live queue.</li>`)
	}
	boardRows := strings.Builder{}
	for _, row := range digestBoards {
		boardRows.WriteString(`<tr><td><a href="/boards?board=` + strconv.FormatInt(row.BoardID, 10) + `">` + htmlEscape(row.BoardName) + `</a></td><td>` + strconv.Itoa(row.NewCount) + `</td><td>` + htmlEscape(row.LastAt) + `</td><td>` + htmlEscape(row.LastSubject) + `</td></tr>`)
	}
	if boardRows.Len() == 0 {
		if pref.IncludeBoards {
			boardRows.WriteString(`<tr><td colspan="4">No digest-tier board movement right now.</td></tr>`)
		} else {
			boardRows.WriteString(`<tr><td colspan="4">Digest-tier boards are disabled in your preferences.</td></tr>`)
		}
	}
	conferencePacks := strings.Builder{}
	if len(digestBoards) > 0 {
		grouped := map[string][]boardPulseRow{}
		order := []string{}
		for _, row := range digestBoards {
			conf := defaultConferenceValue(row.Conference)
			if _, ok := grouped[conf]; !ok {
				order = append(order, conf)
			}
			grouped[conf] = append(grouped[conf], row)
		}
		sort.Slice(order, func(i, j int) bool { return strings.ToLower(order[i]) < strings.ToLower(order[j]) })
		for _, conf := range order {
			rows := grouped[conf]
			packRows := strings.Builder{}
			for _, row := range rows {
				packRows.WriteString(`<li><a href="/boards?board=` + strconv.FormatInt(row.BoardID, 10) + `">` + htmlEscape(row.BoardName) + `</a> <span class="wolfbbs-muted">` + strconv.Itoa(row.NewCount) + ` new • ` + htmlEscape(defaultIfBlank(row.LastSubject, "no recent subject")) + `</span></li>`)
			}
			conferencePacks.WriteString(`<article class="wolfbbs-card"><h3>` + htmlEscape(conf) + `</h3><ul class="wolfbbs-list-clean">` + packRows.String() + `</ul></article>`)
		}
	} else {
		conferencePacks.WriteString(`<article class="wolfbbs-card"><p>No conference digest packs right now.</p></article>`)
	}
	eventRows := strings.Builder{}
	if eventDue {
		for _, row := range upcoming {
			eventRows.WriteString(`<li><strong>` + htmlEscape(row.Title) + `</strong> <span class="wolfbbs-muted">` + htmlEscape(formatCommunityEventWindow(row)) + `</span></li>`)
		}
		if eventRows.Len() == 0 {
			if pref.IncludeEvents {
				eventRows.WriteString(`<li>No scheduled events in the current digest window.</li>`)
			} else {
				eventRows.WriteString(`<li>Event reminders are disabled in your digest preferences.</li>`)
			}
		}
	} else if pref.IncludeEvents {
		eventRows.WriteString(`<li>Event reminders are held by your ` + htmlEscape(pref.EventCadence) + ` cadence. Open <a href="/events">/events</a> for the full calendar.</li>`)
	} else {
		eventRows.WriteString(`<li>Event reminders are disabled in your digest preferences.</li>`)
	}
	bulletinRows := strings.Builder{}
	if bulletinDue {
		for _, row := range bulletinSnapshot.SystemWire {
			bulletinRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
		}
		if bulletinRows.Len() == 0 {
			bulletinRows.WriteString(`<li>No bulletin wire items are active.</li>`)
		}
	} else {
		bulletinRows.WriteString(`<li>Bulletin wire is held by your ` + htmlEscape(pref.BulletinCadence) + ` cadence. Open <a href="/bulletins">/bulletins</a> for the live wire.</li>`)
	}
	stateLabel := "preview only"
	if pref.Enabled {
		stateLabel = "active"
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Daily Digest</title></head><body>
<p><a href="/start">start</a> | <a href="/today">today</a> | <a href="/attention">attention</a> | <a href="/boards">boards</a> | <a href="/events">events</a> | <a href="/settings">settings</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Daily Digest</h1>
<p>Low-noise web summary for callers who want a deliberate once-per-day scan instead of route-hopping.</p>
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + htmlEscape(stateLabel) + `</strong><span>digest mode</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(digest.Items)) + `</strong><span>follow-up items</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(digestBoards)) + `</strong><span>digest boards</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(upcoming)) + `</strong><span>event reminders</span></article>
<article class="wolfbbs-kpi-card"><strong>` + htmlEscape(pref.AttentionCadence) + ` / ` + htmlEscape(pref.BulletinCadence) + ` / ` + htmlEscape(pref.EventCadence) + `</strong><span>attention / bulletins / events</span></article>
<article class="wolfbbs-kpi-card"><strong>` + boolToText(pref.WeeklyMail) + `</strong><span>weekly internal mail</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(maxItems) + `</strong><span>weekday max items</span></article>
</section>
<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Web digest is ` + htmlEscape(stateLabel) + `</strong><p>Use <a href="/settings">/settings</a> to choose whether this stays as an explicit opt-in surface.</p></article><article class="wolfbbs-helper-card"><strong>Use this for low-noise loops</strong><p>Attention Center is still the place for urgent replies and mentions. Digest is the calmer summary.</p></article><article class="wolfbbs-helper-card"><strong>Subscription tiers shape this page</strong><p>Digest-tier boards land here while watch-tier boards stay in <a href="/today">/today</a> and <a href="/attention">/attention</a>.</p></article><article class="wolfbbs-helper-card"><strong>Cadence is per route</strong><p>Attention, bulletins, and events can each be shown always, daily, weekly, or turned off from <a href="/settings">/settings</a>.</p></article></section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Direct Follow-Up</h2><ul class="wolfbbs-list-clean">` + itemRows.String() + `</ul></article>
<article class="wolfbbs-card"><h2>Event Reminders</h2><ul class="wolfbbs-list-clean">` + eventRows.String() + `</ul><p><a href="/events">Full calendar</a> | <a href="/tournaments">Tournament Center</a></p></article>
</section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Digest Tier Boards</h2><table border="1"><tr><th>Board</th><th>New</th><th>Last</th><th>Last subject</th></tr>` + boardRows.String() + `</table></article>
<article class="wolfbbs-card"><h2>Bulletin Wire</h2><ul class="wolfbbs-list-clean">` + bulletinRows.String() + `</ul><p><a href="/bulletins">Open Bulletin Center</a></p></article>
</section>
<section><h2>Conference Packs</h2><div class="wolfbbs-grid">` + conferencePacks.String() + `</div></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Next Move</h2><ul><li><a href="/attention">Attention Center</a> if something here needs direct response.</li><li><a href="/boards?mode=digest">Digest-tier boards</a> for longer reads.</li><li><a href="/today">Today Brief</a> if you want the full daily loop.</li><li><a href="/digest/preferences">Digest weekday preferences</a> to tune max items per day.</li><li><a href="/settings">Settings</a> for global digest behavior.</li></ul><p class="wolfbbs-muted">Current weekday item caps: ` + htmlEscape(a.digestWeeklyOverrideSummary(user.Handle, pref.MaxItems)) + `</p></article></section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleEventsCalendar(w http.ResponseWriter, r *http.Request) {
	user, _ := a.currentUser(r)
	if r.Method == http.MethodPost {
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if !a.requireCSRF(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		eventID := strings.TrimSpace(r.FormValue("event_id"))
		if _, found := a.findEventOccurrenceByID(eventID, time.Now().UTC()); !found {
			redirectWithError(w, r, "/events", "Event not found.")
			return
		}
		switch action {
		case "rsvp":
			a.setEventRSVP(user.Handle, eventID, r.FormValue("status"), "")
			redirectWithNotice(w, r, "/events", "RSVP saved.")
			return
		case "clear_rsvp":
			a.setEventRSVP(user.Handle, eventID, "", "")
			redirectWithNotice(w, r, "/events", "RSVP cleared.")
			return
		case "check_in":
			a.setEventAttendance(eventID, user.Handle, true)
			redirectWithNotice(w, r, "/events", "Event attendance saved.")
			return
		case "clear_check_in":
			a.setEventAttendance(eventID, user.Handle, false)
			redirectWithNotice(w, r, "/events", "Event attendance cleared.")
			return
		default:
			redirectWithError(w, r, "/events", "Unsupported event action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if user != nil {
		a.markRouteSeen(user.Handle, "/events")
	}
	nav := `<a href="/start">start</a> | <a href="/connect">connect</a> | <a href="/tour">tour</a> | <a href="/help">help</a>`
	if user != nil {
		nav = `<a href="/start">start</a> | <a href="/today">today</a> | <a href="/attention">attention</a> | <a href="/boards">boards</a> | <a href="/chat">chat</a> | <a href="/challenges">challenges</a> | <a href="/events/recaps">recaps</a> | <a href="/help">help</a> | <a href="/logout">logout</a>`
		if a.hasRole(user, roleAdmin) {
			nav = `<a href="/start">start</a> | <a href="/today">today</a> | <a href="/events">events</a> | <a href="/events/recaps">recaps</a> | <a href="/challenges">challenges</a> | <a href="/admin/events">admin events</a> | <a href="/admin/challenges">admin challenges</a> | <a href="/admin/ops">ops</a> | <a href="/help">help</a> | <a href="/logout">logout</a>`
		}
	}
	now := time.Now().UTC()
	upcoming := a.upcomingCommunityEvents(24, now)
	recent := a.recentCommunityEvents(12, now)
	attendanceSummary := a.attendanceCounts()
	recaps := a.loadEventRecaps()
	myRSVPs := map[string]eventRSVP{}
	if user != nil {
		myRSVPs = a.loadEventRSVPs(user.Handle)
	}
	rsvpSummary := map[string]map[string]int{}
	if users, err := a.authSvc.ListUsers(); err == nil {
		for _, row := range users {
			if !directoryVisibleUser(&row) {
				continue
			}
			for eventID, rsvp := range a.loadEventRSVPs(row.Handle) {
				if rsvpSummary[eventID] == nil {
					rsvpSummary[eventID] = map[string]int{}
				}
				rsvpSummary[eventID][rsvp.Status]++
			}
		}
	}
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
			rsvpBlock := ``
			if counts := rsvpSummary[row.ID]; len(counts) > 0 {
				rsvpBlock = `<p class="wolfbbs-muted">RSVPs: going ` + strconv.Itoa(counts["going"]) + ` | maybe ` + strconv.Itoa(counts["maybe"]) + ` | declined ` + strconv.Itoa(counts["declined"]) + `</p>`
			}
			if attendanceSummary[row.ID] > 0 {
				rsvpBlock += `<p class="wolfbbs-muted">Checked in: ` + strconv.Itoa(attendanceSummary[row.ID]) + ` caller(s)</p>`
			}
			if user != nil {
				current := myRSVPs[row.ID].Status
				rsvpBlock += `<form method="POST" action="/events" class="wolfbbs-inline-form">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="rsvp"><input type="hidden" name="event_id" value="` + htmlEscape(row.ID) + `"><label>RSVP <select name="status"><option value="going"` + selectedIf(current == "going") + `>going</option><option value="maybe"` + selectedIf(current == "maybe") + `>maybe</option><option value="declined"` + selectedIf(current == "declined") + `>declined</option></select></label><button type="submit">Save</button></form>`
				if current != "" {
					rsvpBlock += `<form method="POST" action="/events" class="wolfbbs-inline-form">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="clear_rsvp"><input type="hidden" name="event_id" value="` + htmlEscape(row.ID) + `"><button type="submit">Clear RSVP</button></form>`
				}
				if eventCheckInWindow(row, now) {
					if a.eventCheckedIn(row.ID, user.Handle) {
						rsvpBlock += `<form method="POST" action="/events" class="wolfbbs-inline-form">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="clear_check_in"><input type="hidden" name="event_id" value="` + htmlEscape(row.ID) + `"><button type="submit">Clear Check-In</button></form>`
					} else {
						rsvpBlock += `<form method="POST" action="/events" class="wolfbbs-inline-form">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="check_in"><input type="hidden" name="event_id" value="` + htmlEscape(row.ID) + `"><button type="submit">Check In</button></form>`
					}
				}
			}
			recapBlock := ``
			if recap, ok := recaps[row.ID]; ok {
				highlightRows := strings.Builder{}
				for _, item := range recap.Highlights {
					highlightRows.WriteString(`<li>` + htmlEscape(item) + `</li>`)
				}
				if highlightRows.Len() == 0 {
					highlightRows.WriteString(`<li>No recap highlights yet.</li>`)
				}
				recapBlock = `<div class="wolfbbs-card"><p><strong>Recap:</strong> ` + htmlEscape(defaultIfBlank(recap.Summary, "No summary yet.")) + `</p><ul>` + highlightRows.String() + `</ul></div>`
			}
			out.WriteString(`<article class="wolfbbs-card"><h3>` + htmlEscape(row.Title) + `</h3><p class="wolfbbs-muted">` + htmlEscape(strings.Join(meta, " | ")) + `</p>` + hostLine + `<p>` + htmlEscape(cleanOneLiner(row.Description, 220)) + link + `</p>` + rsvpBlock + recapBlock + `</article>`)
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
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(upcoming)) + `</strong><span>upcoming events</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(recent)) + `</strong><span>recent events</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(recaps)) + `</strong><span>published recaps</span></article></section>
<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Plan against real dates</strong><p>Events work best when they are specific, visible, and easy to join from the board.</p></article><article class="wolfbbs-helper-card"><strong>Use Today for a caller brief</strong><p><a href="/today">/today</a> pulls upcoming events into the daily caller loop after login.</p></article><article class="wolfbbs-helper-card"><strong>Tournaments deserve their own lane</strong><p>Use <a href="/tournaments">/tournaments</a> when you want the competitive schedule separated from general community events.</p></article></section>
<section><h2>Upcoming Events</h2><div class="wolfbbs-grid">` + renderList(upcoming, "No upcoming events scheduled yet.") + `</div></section>
<section><h2>Recent Events</h2><div class="wolfbbs-grid">` + renderList(recent, "No recent events yet.") + `</div></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Competitive Layer</h2><p>Want bracket nights, score ladders, and door-focused return hooks instead of the full mixed calendar?</p><p><a href="/tournaments">Open Tournament Center</a> | <a href="/scores">Open Door Scores</a> | <a href="/events/recaps">Open Event Recaps</a></p></article></section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleTournaments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, _ := a.currentUser(r)
	nav := `<a href="/connect">connect</a> | <a href="/tour">tour</a> | <a href="/events">events</a> | <a href="/login">login</a> | <a href="/help">help</a>`
	if user != nil {
		nav = `<a href="/start">start</a> | <a href="/today">today</a> | <a href="/events">events</a> | <a href="/doors">doors</a> | <a href="/scores">scores</a> | <a href="/help">help</a> | <a href="/logout">logout</a>`
		if a.hasRole(user, roleAdmin) {
			nav = `<a href="/start">start</a> | <a href="/events">events</a> | <a href="/tournaments">tournaments</a> | <a href="/admin/events">admin events</a> | <a href="/scores">scores</a> | <a href="/logout">logout</a>`
		}
	}
	now := time.Now().UTC()
	filterCategory := func(rows []communityEvent, category string) []communityEvent {
		out := make([]communityEvent, 0, len(rows))
		for _, row := range rows {
			if strings.EqualFold(strings.TrimSpace(row.Category), category) {
				out = append(out, row)
			}
		}
		return out
	}
	upcoming := filterCategory(a.upcomingCommunityEvents(24, now), "tournament")
	recent := filterCategory(a.recentCommunityEvents(12, now), "tournament")
	champions := a.buildScoreboardSnapshot(user, "").ChampionRows
	if len(champions) > 6 {
		champions = champions[:6]
	}
	standings := make([]tournamentStandingRow, 0, len(champions))
	for idx, row := range champions {
		standings = append(standings, tournamentStandingRow{
			Rank:     idx + 1,
			DoorName: row.DoorName,
			Handle:   row.Handle,
			Score:    row.Score,
		})
	}
	renderCards := func(rows []communityEvent, empty string) string {
		var out strings.Builder
		for _, row := range rows {
			link := `<a href="/doors">open doors</a>`
			if strings.TrimSpace(row.Link) != "" {
				link = `<a href="` + htmlEscape(strings.TrimSpace(row.Link)) + `">join now</a>`
			}
			out.WriteString(`<article class="wolfbbs-card"><h3>` + htmlEscape(row.Title) + `</h3><p class="wolfbbs-muted">` + htmlEscape(formatCommunityEventWindow(row)) + ` | ` + htmlEscape(recurrenceSummary(row)) + `</p><p>` + htmlEscape(cleanOneLiner(row.Description, 220)) + `</p><p><strong>Location:</strong> ` + htmlEscape(defaultIfBlank(row.Location, "Door Cockpit")) + ` | <strong>Host:</strong> ` + htmlEscape(defaultIfBlank(row.Host, "sysop")) + `</p><p>` + link + `</p></article>`)
		}
		if out.Len() == 0 {
			out.WriteString(`<article class="wolfbbs-card"><p>` + htmlEscape(empty) + `</p></article>`)
		}
		return out.String()
	}
	championRows := strings.Builder{}
	for _, row := range champions {
		championRows.WriteString(`<tr><td>` + htmlEscape(row.DoorName) + `</td><td>` + htmlEscape(row.Handle) + `</td><td>` + strconv.FormatInt(row.Score, 10) + `</td><td>` + htmlEscape(row.ScoreType) + `</td></tr>`)
	}
	if championRows.Len() == 0 {
		championRows.WriteString(`<tr><td colspan="4">No tournament-adjacent scores yet.</td></tr>`)
	}
	standingRows := strings.Builder{}
	for _, row := range standings {
		standingRows.WriteString(`<tr><td>` + strconv.Itoa(row.Rank) + `</td><td>` + htmlEscape(row.DoorName) + `</td><td>` + htmlEscape(row.Handle) + `</td><td>` + strconv.FormatInt(row.Score, 10) + `</td></tr>`)
	}
	if standingRows.Len() == 0 {
		standingRows.WriteString(`<tr><td colspan="4">No standings yet.</td></tr>`)
	}
	bracketRows := strings.Builder{}
	if len(standings) >= 4 {
		matchups := [][2]tournamentStandingRow{
			{standings[0], standings[3]},
			{standings[1], standings[2]},
		}
		for idx, row := range matchups {
			bracketRows.WriteString(`<li>Semifinal ` + strconv.Itoa(idx+1) + `: <strong>#` + strconv.Itoa(row[0].Rank) + ` ` + htmlEscape(row[0].Handle) + `</strong> (` + htmlEscape(row[0].DoorName) + `) vs <strong>#` + strconv.Itoa(row[1].Rank) + ` ` + htmlEscape(row[1].Handle) + `</strong> (` + htmlEscape(row[1].DoorName) + `)</li>`)
		}
	} else {
		bracketRows.WriteString(`<li>Need at least four champion rows before a bracket preview makes sense.</li>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Tournament Center</title></head><body>
<p>` + nav + `</p>
` + pageMessageBlock(r) + `
<h1>Tournament Center</h1>
<p>Bracket-ready schedule for recurring door nights, ladder events, and score challenges that make callers return on purpose.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(upcoming)) + `</strong><span>upcoming tournaments</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(recent)) + `</strong><span>recent tournament slots</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(champions)) + `</strong><span>current champions</span></article></section>
<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Run brackets, not vague hype</strong><p>Tournaments should have a real time, a clear host, and a direct join path to the door or chat room.</p></article><article class="wolfbbs-helper-card"><strong>Pair schedule with scoreboards</strong><p>Use <a href="/scores">/scores</a> to make the competitive layer visible before and after the event.</p></article><article class="wolfbbs-helper-card"><strong>Keep the ladder alive</strong><p>Recurring series drive retention better than one-off “come hang out later” announcements.</p></article></section>
<section><h2>Upcoming Tournaments</h2><div class="wolfbbs-grid">` + renderCards(upcoming, "No tournaments scheduled yet.") + `</div></section>
<section><h2>Recent Tournament Nights</h2><div class="wolfbbs-grid">` + renderCards(recent, "No recent tournament history yet.") + `</div></section>
<section><h2>Current Door Champions</h2><table border="1"><tr><th>Door</th><th>Caller</th><th>Score</th><th>Type</th></tr>` + championRows.String() + `</table></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Standings Table</h2><table border="1"><tr><th>Rank</th><th>Door</th><th>Caller</th><th>Score</th></tr>` + standingRows.String() + `</table></article><article class="wolfbbs-card"><h2>Bracket Preview</h2><ul>` + bracketRows.String() + `</ul><p class="wolfbbs-muted">This is a lightweight seeding view built from current champions so callers can see how a bracket night would shape up before the event starts.</p></article></section>
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
		rows := a.loadCommunityEvents()
		switch action {
		case "create", "update":
			editID := strings.TrimSpace(r.FormValue("id"))
			existing := communityEvent{}
			existingIdx := -1
			if action == "update" {
				var found bool
				existing, existingIdx, found = findCommunityEvent(rows, editID)
				if !found {
					redirectWithError(w, r, "/admin/events", "Event not found.")
					return
				}
			}
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
			row := communityEvent{
				ID:          existing.ID,
				SeriesID:    existing.SeriesID,
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
				CreatedAt:   existing.CreatedAt,
			}
			if strings.TrimSpace(row.ID) == "" {
				row.ID = randomEventID()
			}
			if strings.TrimSpace(row.SeriesID) == "" {
				row.SeriesID = row.ID
			}
			if row.CreatedAt.IsZero() {
				row.CreatedAt = time.Now().UTC()
			}
			if row.Audience == "" {
				row.Audience = "all callers"
			}
			if existingIdx >= 0 {
				rows[existingIdx] = row
			} else {
				rows = append(rows, row)
			}
			a.persistCommunityEvents(rows)
			recurrenceLabel := row.Recurrence
			if strings.TrimSpace(recurrenceLabel) == "" {
				recurrenceLabel = "none"
			}
			actionLabel := "create_event"
			notice := "Community event created."
			if action == "update" {
				actionLabel = "update_event"
				notice = "Community event updated."
			}
			a.recordAdminAction(user.Handle, "community_calendar", actionLabel, fmt.Sprintf("id=%s title=%s recurrence=%s", row.ID, row.Title, recurrenceLabel))
			redirectWithNotice(w, r, "/admin/events", notice)
			return
		case "delete":
			id := strings.TrimSpace(r.FormValue("id"))
			if id == "" {
				redirectWithError(w, r, "/admin/events", "Event ID is required.")
				return
			}
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
		case "save_recap":
			eventID := strings.TrimSpace(r.FormValue("event_id"))
			eventRow, found := a.findEventOccurrenceByID(eventID, time.Now().UTC())
			if eventID == "" || !found {
				redirectWithError(w, r, "/admin/events", "Event for recap not found.")
				return
			}
			attendanceCount := parseIntWithFallback(r.FormValue("attendance_count"), 0)
			if attendanceCount <= 0 {
				attendanceCount = a.attendanceCounts()[eventID]
			}
			recap := eventRecap{
				EventID:         eventID,
				SeriesID:        eventRow.SeriesID,
				Title:           eventRow.Title,
				StartsAt:        eventRow.StartsAt,
				EndsAt:          eventRow.EndsAt,
				AttendanceCount: attendanceCount,
				Summary:         strings.TrimSpace(r.FormValue("summary")),
				Highlights:      parseHighlightLines(r.FormValue("highlights")),
				UpdatedBy:       user.Handle,
				UpdatedAt:       time.Now().UTC(),
			}
			if strings.TrimSpace(recap.Summary) == "" && len(recap.Highlights) == 0 {
				redirectWithError(w, r, "/admin/events", "Recap summary or highlights are required.")
				return
			}
			a.upsertEventRecap(recap)
			a.recordAdminAction(user.Handle, "community_calendar", "save_recap", fmt.Sprintf("id=%s title=%s attendance=%d", recap.EventID, recap.Title, recap.AttendanceCount))
			redirectWithNotice(w, r, "/admin/events", "Event recap saved.")
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
	editID := strings.TrimSpace(r.URL.Query().Get("edit"))
	templateKey := strings.TrimSpace(r.URL.Query().Get("template"))
	formEvent := communityEvent{
		Category: "social",
		Host:     "sysop",
		Audience: "all callers",
	}
	formAction := "create"
	formTitle := "Create Event"
	submitLabel := "Create Event"
	cancelLink := ""
	templateNotice := ""
	if editRow, _, found := findCommunityEvent(rows, editID); found {
		formEvent = editRow
		if strings.TrimSpace(formEvent.Host) == "" {
			formEvent.Host = "sysop"
		}
		if strings.TrimSpace(formEvent.Audience) == "" {
			formEvent.Audience = "all callers"
		}
		formAction = "update"
		formTitle = "Edit Event"
		submitLabel = "Save Changes"
		cancelLink = `<p><a href="/admin/events">Cancel editing</a></p>`
	} else if templateRow, notice, ok := communityEventTemplate(templateKey, time.Now()); ok {
		formEvent = templateRow
		templateNotice = renderPageBanner("notice", notice)
	}
	csrf := a.csrfHiddenInput(r)
	eventRows := strings.Builder{}
	for _, row := range rows {
		recurrenceLabel := recurrenceSummary(row)
		if strings.TrimSpace(recurrenceLabel) == "" {
			recurrenceLabel = "one-time"
		}
		eventRows.WriteString(`<tr><td>` + htmlEscape(row.Title) + `</td><td>` + htmlEscape(strings.ToUpper(row.Category)) + `</td><td>` + htmlEscape(formatCommunityEventWindow(row)) + `</td><td>` + htmlEscape(recurrenceLabel) + `</td><td>` + htmlEscape(row.Location) + `</td><td>` + htmlEscape(row.Audience) + `</td><td><a href="/admin/events?edit=` + url.QueryEscape(row.ID) + `#event-editor">Edit</a> <form method="POST" action="/admin/events" style="display:inline"><input type="hidden" name="action" value="delete"><input type="hidden" name="id" value="` + htmlEscape(row.ID) + `">` + csrf + `<button type="submit">Delete</button></form></td></tr>`)
	}
	if eventRows.Len() == 0 {
		eventRows.WriteString(`<tr><td colspan="7">No events scheduled yet.</td></tr>`)
	}

	categoryOptions := []string{"social", "door", "tournament", "content", "system", "ops"}
	categoryRows := strings.Builder{}
	for _, category := range categoryOptions {
		categoryRows.WriteString(`<option value="` + category + `"` + selectedIf(formEvent.Category == category) + `>` + category + `</option>`)
	}
	recurrenceRows := strings.Builder{}
	recurrenceRows.WriteString(`<option value=""` + selectedIf(formEvent.Recurrence == "") + `>one-time</option>`)
	for _, recurrence := range []string{"daily", "weekdays", "weekly", "monthly"} {
		recurrenceRows.WriteString(`<option value="` + recurrence + `"` + selectedIf(formEvent.Recurrence == recurrence) + `>` + recurrence + `</option>`)
	}
	previewCard := renderCommunityEventPreview(formEvent, 5)
	recaps := a.loadEventRecaps()
	attendanceSummary := a.attendanceCounts()
	recentForRecap := a.recentCommunityEvents(10, time.Now().UTC())
	campaignSummary := buildEventCampaignSummary(rows, recaps, attendanceSummary, time.Now().UTC())
	recapRows := strings.Builder{}
	for _, row := range recentForRecap {
		recap := recaps[row.ID]
		recapRows.WriteString(`<tr><td><strong>` + htmlEscape(row.Title) + `</strong><br><span class="wolfbbs-muted">` + htmlEscape(formatCommunityEventWindow(row)) + `</span></td><td><form method="POST" action="/admin/events"><input type="hidden" name="action" value="save_recap"><input type="hidden" name="event_id" value="` + htmlEscape(row.ID) + `">` + csrf + `<label>Attendance <input name="attendance_count" value="` + strconv.Itoa(maxInt(attendanceSummary[row.ID], recap.AttendanceCount)) + `" inputmode="numeric" size="5"></label><br><label>Summary <input name="summary" value="` + htmlEscape(recap.Summary) + `" size="72" placeholder="What happened and why callers should care."></label><br><label>Highlights (one per line)<br><textarea name="highlights" rows="3" cols="72" placeholder="Top score was broken&#10;Great board thread follow-up">` + htmlEscape(renderHighlightLines(recap.Highlights)) + `</textarea></label><br><button type="submit">Save Recap</button></form></td></tr>`)
	}
	if recapRows.Len() == 0 {
		recapRows.WriteString(`<tr><td colspan="2">No recent events available for recap yet.</td></tr>`)
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Events Admin</title></head><body>
<p><a href="/admin">admin</a> | <a href="/admin/setup">setup</a> | <a href="/admin/ops">ops</a> | <a href="/admin/challenges">challenges</a> | <a href="/events">public calendar</a> | <a href="/events/recaps">recaps</a> | <a href="/today">today brief</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
` + templateNotice + `
<h1>Events Admin</h1>
<p>Schedule public reasons for callers to return: nets, tournaments, featured content drops, and operator-run sessions.</p>
<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Be concrete</strong><p>Give callers a specific time, place, and audience. Vague events do not drive return behavior.</p></article><article class="wolfbbs-helper-card"><strong>Use categories deliberately</strong><p>System, social, door, tournament, content, and ops let the calendar read like a real board schedule.</p></article><article class="wolfbbs-helper-card"><strong>Build series, not chores</strong><p>Recurring events turn the board into a habit. Weekly or weekday series are better than retyping the same entry every day.</p></article></section>
` + renderEventCampaignHealth(campaignSummary) + `
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Quick Templates</h2><div class="wolfbbs-action-grid"><a class="wolfbbs-action-card" href="/admin/events?template=tournament#event-editor"><strong>Tournament Night</strong><span>weekly bracket or score ladder</span></a><a class="wolfbbs-action-card" href="/admin/events?template=social#event-editor"><strong>Lobby Net</strong><span>weekday live social check-in</span></a><a class="wolfbbs-action-card" href="/admin/events?template=content#event-editor"><strong>Content Drop</strong><span>scheduled bulletin, file, or featured thread</span></a><a class="wolfbbs-action-card" href="/admin/events?template=ops#event-editor"><strong>Sysop Review</strong><span>operator-only runbook cadence</span></a></div><p class="wolfbbs-muted">Templates load concrete defaults so you start from a believable event instead of a blank form.</p></article><article class="wolfbbs-card"><h2>Tournament Playbook</h2><ul class="wolfbbs-list-clean"><li>Use category <strong>tournament</strong> for ladders, score nights, and door brackets.</li><li>Point the link at <code>/doors</code>, <code>/tournaments</code>, or a door runbook so callers can join without guessing.</li><li>Weekly recurrence is the default starting point for a sustainable tournament rhythm.</li><li>Mirror the event in <a href="/tournaments">Tournament Center</a> and <a href="/scores">Scores</a> so the competitive layer feels alive.</li></ul></article></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card" id="event-editor"><h2>` + formTitle + `</h2><form method="POST" action="/admin/events" data-draft-key="admin-event-editor"><input type="hidden" name="action" value="` + formAction + `"><input type="hidden" name="id" value="` + htmlEscape(formEvent.ID) + `">` + csrf + `<label>Title <input name="title" size="48" value="` + htmlEscape(formEvent.Title) + `" placeholder="Friday Tournament Night"></label><br><label>Category <select name="category">` + categoryRows.String() + `</select></label><br><label>Starts <input type="datetime-local" name="starts_at" value="` + htmlEscape(formatLocalDateTimeValue(formEvent.StartsAt)) + `"></label><br><label>Ends <input type="datetime-local" name="ends_at" value="` + htmlEscape(formatLocalDateTimeValue(formEvent.EndsAt)) + `"></label><br><label>Recurrence <select name="recurrence">` + recurrenceRows.String() + `</select></label><br><label>Repeat Until <input type="datetime-local" name="repeat_until" value="` + htmlEscape(formatLocalDateTimeValue(formEvent.RepeatUntil)) + `"></label><br><label>Location <input name="location" size="40" value="` + htmlEscape(formEvent.Location) + `" placeholder="#lobby, Door Cockpit, SSH"></label><br><label>Host <input name="host" size="40" value="` + htmlEscape(formEvent.Host) + `" placeholder="sysop"></label><br><label>Audience <input name="audience" size="40" value="` + htmlEscape(formEvent.Audience) + `" placeholder="all callers"></label><br><label>Link <input name="link" size="60" value="` + htmlEscape(formEvent.Link) + `" placeholder="/doors or https://..."></label><br><label>Description<br><textarea name="description" rows="6" cols="72" placeholder="What happens, why it matters, and how to join.">` + htmlEscape(formEvent.Description) + `</textarea></label><br><button type="submit">` + submitLabel + `</button></form>` + cancelLink + `</article><article class="wolfbbs-card"><h2>Calendar Preview</h2>` + previewCard + `<p><a href="/events">Open public calendar</a> | <a href="/today">Open today brief</a></p></article></section>
<section><h2>Scheduled Events</h2><table border="1"><tr><th>Title</th><th>Category</th><th>When</th><th>Series</th><th>Location</th><th>Audience</th><th>Action</th></tr>` + eventRows.String() + `</table></section>
<section><h2>Post-Event Recaps</h2><p>Close the loop after each event: capture attendance and what happened so future scheduling decisions are evidence-based.</p><table border="1"><tr><th>Event</th><th>Recap</th></tr>` + recapRows.String() + `</table><p><a href="/events/recaps">View public recap feed</a></p></section>
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
	_, _ = fmt.Fprintf(w, "wolfbbs_build_info{version=%q} 1\n", webBuildVersion)
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
	lower := strings.ToLower(page)
	if htmlIdx := strings.Index(lower, "<html"); htmlIdx >= 0 && !strings.Contains(lower, "<html lang=") {
		rest := page[htmlIdx:]
		if end := strings.Index(rest, ">"); end >= 0 {
			insertAt := htmlIdx + end
			page = page[:insertAt] + ` lang="en"` + page[insertAt:]
			lower = strings.ToLower(page)
		}
	}
	headInject := ""
	if !strings.Contains(lower, "<title") {
		headInject += `<title>WolfBBS</title>`
	}
	if !strings.Contains(lower, `name="viewport"`) {
		headInject += `<meta name="viewport" content="width=device-width, initial-scale=1">`
	}
	if !strings.Contains(page, `id="wolfbbs-modern-ui"`) {
		headInject += modernUIBootstrap
	}
	if headInject == "" {
		return page
	}
	if idx := strings.Index(lower, "</head>"); idx >= 0 {
		return page[:idx] + headInject + page[idx:]
	}
	if htmlIdx := strings.Index(lower, "<html"); htmlIdx >= 0 {
		rest := lower[htmlIdx:]
		if end := strings.Index(rest, ">"); end >= 0 {
			insertAt := htmlIdx + end + 1
			return page[:insertAt] + `<head>` + headInject + `</head>` + page[insertAt:]
		}
	}
	if bodyIdx := strings.Index(lower, "<body"); bodyIdx >= 0 {
		rest := lower[bodyIdx:]
		if end := strings.Index(rest, ">"); end >= 0 {
			insertAt := bodyIdx + end + 1
			return page[:insertAt] + headInject + page[insertAt:]
		}
	}
	return `<!doctype html><html lang="en"><head>` + headInject + `</head><body>` + page + `</body></html>`
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
	emailGateway := a.activeEmailGateway()
	if emailGateway == nil || !emailGateway.Enabled() {
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
	return emailGateway.SendOutbound("wolfbbs-reset", []string{recipient}, subject, body)
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
	preset := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("preset")))
	switch preset {
	case "syncterm", "ftelnet", "vtx", "web":
	default:
		preset = "web"
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
const preset = ` + fmt.Sprintf("%q", preset) + `;
const connectSearch = new URLSearchParams(location.search);
let terminalPrimed = location.hash === '#xterm' || connectSearch.get('autofocus') === '1';
const presetMap = {
  web: { fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace", fontSize: 14, background: "#0b0f14", foreground: "#b7f7c1", cursor: "#f4f4f4", selection: "#334455" },
  syncterm: { fontFamily: "'IBM Plex Mono', 'Cascadia Mono', 'Courier New', monospace", fontSize: 15, background: "#050709", foreground: "#ffd27a", cursor: "#fff2c4", selection: "#5a3f1d" },
  ftelnet: { fontFamily: "'Cascadia Mono', 'Consolas', 'Courier New', monospace", fontSize: 15, background: "#06101a", foreground: "#9bd3ff", cursor: "#e6f5ff", selection: "#284b63" },
  vtx: { fontFamily: "'IBM Plex Mono', 'Consolas', 'Courier New', monospace", fontSize: 15, background: "#05080f", foreground: "#9be7c1", cursor: "#d6ffeb", selection: "#21523f" }
};
const presetConfig = presetMap[preset] || presetMap.web;
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
  view.style.background = presetConfig.background;
  view.style.color = presetConfig.foreground;
  view.style.font = String(presetConfig.fontSize || 14) + "px/1.45 " + presetConfig.fontFamily;
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
  fontFamily: presetConfig.fontFamily,
  fontSize: presetConfig.fontSize,
  theme: {
    background: presetConfig.background,
    foreground: presetConfig.foreground,
    cursor: presetConfig.cursor,
    selectionBackground: presetConfig.selection
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
function focusTerminal() {
  if (!terminalPrimed) return;
  term.focus();
}
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
    focusTerminal();
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

host.addEventListener('click', function(){
  terminalPrimed = true;
  focusTerminal();
});
document.addEventListener('visibilitychange', function(){
  if (document.visibilityState === "visible") {
    if (!ws || ws.readyState !== WebSocket.OPEN) connect();
    focusTerminal();
  }
});
window.addEventListener('focus', function(){
  if (!ws || ws.readyState !== WebSocket.OPEN) connect();
  focusTerminal();
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
setTimeout(function(){ focusTerminal(); }, 0);
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
	presetLabel := map[string]string{
		"web":      "Browser",
		"syncterm": "SyncTERM",
		"ftelnet":  "fTelnet",
		"vtx":      "VTX",
	}[preset]
	if presetLabel == "" {
		presetLabel = "Browser"
	}

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
<li><strong>Terminal preset:</strong> ` + htmlEscape(presetLabel) + `</li>
<li><strong>Web terminal:</strong> reconnects automatically if the browser tab comes back into focus.</li>
</ul></article>
</section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Terminal Profile Presets</h2><div class="wolfbbs-action-grid"><a class="wolfbbs-action-card" href="/connect?preset=web#xterm"><strong>Browser</strong><span>balanced default for most callers</span></a><a class="wolfbbs-action-card" href="/connect?preset=syncterm#xterm"><strong>SyncTERM</strong><span>warm amber CP437-style profile</span></a><a class="wolfbbs-action-card" href="/connect?preset=ftelnet#xterm"><strong>fTelnet</strong><span>bright cyan legacy-web profile</span></a><a class="wolfbbs-action-card" href="/connect?preset=vtx#xterm"><strong>VTX</strong><span>high-contrast green terminal profile</span></a></div></article>
<article class="wolfbbs-card"><h2>Compatibility Notes</h2><ul class="wolfbbs-list-clean"><li><strong>SyncTERM style:</strong> best when you want classic CP437-era contrast and larger glyphs.</li><li><strong>fTelnet style:</strong> tuned for older browser terminal look-and-feel.</li><li><strong>VTX style:</strong> higher contrast for long sessions and smaller displays.</li><li><strong>Fallback:</strong> if JS terminal addons fail, keyboard input still works via a plain text fallback.</li></ul></article>
</section>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Mobile-first connection picks</h2><div class="wolfbbs-action-grid"><a class="wolfbbs-action-card" href="#xterm"><strong>Browser Terminal</strong><span>best zero-install path on phones and tablets</span></a><a class="wolfbbs-action-card" href="ssh://` + htmlEscape(connectHost) + `:` + strconv.Itoa(sshPort) + `"><strong>SSH Client</strong><span>works best when your mobile terminal app supports ANSI</span></a><a class="wolfbbs-action-card" href="` + htmlEscape(ircCommand) + `"><strong>IRC Client</strong><span>use this when you only need the live lobby</span></a></div></article>
<article class="wolfbbs-card"><h2>Touch-device guidance</h2><ul class="wolfbbs-list-clean"><li><strong>iPhone/iPad:</strong> browser terminal first, SSH second if you already have a terminal client.</li><li><strong>Android:</strong> browser terminal or an ANSI-capable SSH client both work well.</li><li><strong>Keyboard quirks:</strong> tap inside the terminal before typing so focus and backspace land correctly.</li><li><strong>Low-friction path:</strong> keep /connect bookmarked if this is how you onboard new callers.</li></ul></article>
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
<li>On phones or tablets, start with the browser terminal before reaching for a standalone client.</li>
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

	authFactor := 1
	if strings.TrimSpace(user.TOTPSecret) != "" {
		authFactor = 2
	}
	transport, secureConn := requestSecurityProfile(r)
	if sid, ok := a.createSessionWithContext(user.Handle, authFactor, transport, secureConn); ok {
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
	nav := `<a href="/start">start</a> | <a href="/showcase">showcase</a> | <a href="/login">login</a> | <a href="/connect">connect</a>`
	roleGuideTitle := "If you're visiting for the first time"
	roleGuide := `<ol>` +
		`<li>Start with <a href="/start">/start</a>, <a href="/showcase">/showcase</a>, <a href="/connect">/connect</a>, or <a href="/tour">/tour</a> to understand the board before signing in.</li>` +
		`<li>Use <a href="/help">/help</a> to learn the route map and caller surface layout.</li>` +
		`<li>When you want the real experience, sign in and try SSH plus <a href="/boards">/boards</a> and <a href="/chat">/chat</a>.</li>` +
		`</ol>`
	if user != nil {
		roleLabel = rbac.NormalizeRole(user.Role)
		nav = `<a href="/start">start</a> | <a href="/showcase">showcase</a> | <a href="/today">today</a> | <a href="/attention">attention</a> | <a href="/events">events</a> | <a href="/events/recaps">recaps</a> | <a href="/challenges">challenges</a> | <a href="/boards">boards</a> | <a href="/bulletins">bulletins</a> | <a href="/directory">directory</a> | <a href="/finder">finder</a> | <a href="/newfiles">newfiles</a> | <a href="/feedback">feedback</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/radar">radar</a> | <a href="/clubhouse">clubhouse</a> | <a href="/doors">doors</a> | <a href="/settings">settings</a> | <a href="/status">status</a> | <a href="/config">config</a>`
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
				`<li>Use <a href="/admin/challenges">/admin/challenges</a> for seasonal scoring and shared clubhouse goals.</li>` +
				`<li>Use <a href="/admin/upgrade-safety">/admin/upgrade-safety</a> and <a href="/admin/backups">/admin/backups</a> before change windows.</li>` +
				`<li>Review <a href="/admin/config">/admin/config</a> for runtime flags, identity, and exposed services.</li>` +
				`<li>Seed boards, create a non-sysop account in <a href="/admin/users">/admin/users</a>, then test <a href="/boards">/boards</a>, <a href="/chat">/chat</a>, <a href="/doors">/doors</a>, and SSH.</li>` +
				`<li>Use <a href="/status">/status</a> and <a href="/admin/system">/admin/system</a> as the daily health view.</li>` +
				`</ol>`
		} else {
			roleGuideTitle = "If you're a caller"
			roleGuide = `<ol>` +
				`<li>Start with <a href="/today">/today</a> and <a href="/attention">/attention</a> when you want a fast answer to what matters next.</li>` +
				`<li>Use <a href="/boards">/boards</a> for long-form discussion, <a href="/chat">/chat</a> for live conversation, and <a href="/doors">/doors</a> for game and score surfaces.</li>` +
				`<li>Use <a href="/events">/events</a>, <a href="/events/recaps">/events/recaps</a>, and <a href="/challenges">/challenges</a> to stay in the event loop.</li>` +
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
<li>Season loop + social goals: <a href="/admin/challenges">/admin/challenges</a></li>
<li>Change safety: <a href="/admin/upgrade-safety">/admin/upgrade-safety</a> and <a href="/admin/backups">/admin/backups</a></li>
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
<li>/start, /showcase, /today, /attention, /events, /events/recaps, /challenges, /boards, /bulletins, /directory, /finder, /newfiles, /feedback, /mail, /chat, /radar, /clubhouse, /doors, /settings, /gateway, /status, /config</li>
<li>/start for the fastest guest/caller/sysop handoff into the right lane</li>
<li>/showcase for a compact product walk-through and first-run checklist</li>
<li>/today for the daily brief: watched boards, upcoming events, and the shortest responsible next step</li>
<li>/attention for direct follow-up, unread mail, and board movement that actually needs response</li>
<li>/events for the public community calendar and scheduled return hooks</li>
<li>/bulletins for system wire, hot boards, download pick, and classic bulletin-reading flow</li>
<li>/directory for caller lookup, caller cards, and direct compose links</li>
<li>/finder for cross-board search and thread tracker</li>
<li>/newfiles for recent uploads, queue desk, and top-rated file picks</li>
<li>/feedback for classic mail-to-sysop feedback flow</li>
<li>/radar for mission control: board pulse, live callers, discovery queue, and arcade heat</li>
<li>/clubhouse for one-liner posting, rumors, BBS exchange, and shared goal progress</li>
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
<li><code>docs/EXTENSION_SDK.md</code></li>
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
	if r.Method == http.MethodPost {
		if !a.requireCSRF(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		switch action {
		case "ack_bulletin":
			id := strings.TrimSpace(r.FormValue("id"))
			if id == "" {
				redirectWithError(w, r, "/bulletins", "Bulletin ID is required.")
				return
			}
			a.acknowledgeBulletin(user.Handle, id)
			redirectWithNotice(w, r, "/bulletins", "Bulletin acknowledged.")
			return
		default:
			redirectWithError(w, r, "/bulletins", "Unsupported bulletin action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	a.markRouteSeen(user.Handle, "/bulletins")
	snapshot := a.buildBulletinSnapshot(user)
	csrf := a.csrfHiddenInput(r)
	scheduledRows := strings.Builder{}
	for _, row := range snapshot.Scheduled {
		meta := []string{row.StartsAt.Local().Format("2006-01-02 15:04")}
		if !row.EndsAt.IsZero() {
			meta = append(meta, "until "+row.EndsAt.Local().Format("2006-01-02 15:04"))
		}
		if strings.TrimSpace(row.Audience) != "" {
			meta = append(meta, row.Audience)
		}
		link := ""
		if strings.TrimSpace(row.Link) != "" {
			link = ` <a href="` + htmlEscape(row.Link) + `">open</a>`
		}
		ackAt := a.bulletinAckedAt(user.Handle, row.ID)
		ackBlock := `<form method="POST" action="/bulletins" class="wolfbbs-inline-actions"><input type="hidden" name="action" value="ack_bulletin"><input type="hidden" name="id" value="` + htmlEscape(row.ID) + `">` + csrf + `<button type="submit">Acknowledge</button></form>`
		if !ackAt.IsZero() {
			ackBlock = `<span class="wolfbbs-status-pill ok">acked ` + ackAt.Local().Format("01-02 15:04") + `</span>`
		}
		scheduledRows.WriteString(`<li><strong>` + htmlEscape(row.Title) + `</strong> <span class="wolfbbs-muted">` + htmlEscape(strings.Join(meta, " | ")) + `</span><br>` + htmlEscape(row.Body) + link + `<div class="wolfbbs-inline-actions">` + ackBlock + `</div></li>`)
	}
	if scheduledRows.Len() == 0 {
		scheduledRows.WriteString(`<li>No timed bulletins are live right now.</li>`)
	}
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
	bestOfRows := strings.Builder{}
	for _, row := range snapshot.BestOfWeek {
		meta := []string{row.BoardName, row.CreatedAt, row.Author, row.Reason}
		if row.Note != "" {
			meta = append(meta, row.Note)
		}
		bestOfRows.WriteString(`<li><a href="` + htmlEscape(row.Href) + `">` + htmlEscape(row.Subject) + `</a><br><span class="wolfbbs-muted">` + htmlEscape(strings.Join(meta, " | ")) + `</span></li>`)
	}
	if bestOfRows.Len() == 0 {
		bestOfRows.WriteString(`<li>No best-of-week picks yet.</li>`)
	}
	featuredThread := snapshot.FeaturedThread
	if strings.TrimSpace(featuredThread) == "" {
		featuredThread = "No featured thread yet."
	}
	downloadPick := snapshot.DownloadPick
	if strings.TrimSpace(downloadPick) == "" {
		downloadPick = "No download pick yet."
	}
	adminLink := ""
	if a.hasRole(user, roleAdmin) {
		adminLink = ` | <a href="/admin/bulletins">admin bulletins</a>`
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Bulletin Center</title></head><body>
<p><a href="/boards">boards</a> | <a href="/directory">directory</a> | <a href="/finder">finder</a> | <a href="/newfiles">newfiles</a> | <a href="/feedback">feedback</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/doors">doors</a> | <a href="/status">status</a>` + adminLink + ` | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Bulletin Center</h1>
<p>Classic bulletin-reading, rebuilt as a live dashboard. Start here for system wire, hot boards, file picks, and tonight's pulse.</p>
<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>System Wire</h2><ul>` + systemRows.String() + `</ul></article>
<article class="wolfbbs-card"><h2>Spotlight</h2><p><strong>Featured thread:</strong> ` + htmlEscape(featuredThread) + `</p><p><strong>Download pick:</strong> ` + htmlEscape(downloadPick) + `</p></article>
</section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Timed Announcements</h2><ul class="wolfbbs-list-clean">` + scheduledRows.String() + `</ul></article></section>
<section class="wolfbbs-grid">
<article><h2>Hot Board Pulse</h2><ul>` + hotBoardRows.String() + `</ul><p><a href="/finder">Open finder</a></p></article>
<article><h2>Newscan Headlines</h2><ul>` + digestRows.String() + `</ul><p><a href="/discover">Open discover</a></p></article>
</section>
<section class="wolfbbs-grid">
<article><h2>Best of Week</h2><ul class="wolfbbs-list-clean">` + bestOfRows.String() + `</ul></article>
<article><h2>Recent Callers</h2><ul>` + callerRows.String() + `</ul></article>
<article><h2>OneLinerz Wall</h2><ul>` + oneLinerRows.String() + `</ul></article>
<article><h2>New Files</h2><ul>` + fileRows.String() + `</ul><p><a href="/newfiles">Open new files desk</a></p></article>
</section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminBulletins(w http.ResponseWriter, r *http.Request) {
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
		rows := a.loadScheduledBulletins()
		switch action {
		case "create", "update":
			existing := scheduledBulletin{}
			existingIdx := -1
			if action == "update" {
				var found bool
				existing, existingIdx, found = findScheduledBulletin(rows, r.FormValue("id"))
				if !found {
					redirectWithError(w, r, "/admin/bulletins", "Scheduled bulletin not found.")
					return
				}
			}
			title := strings.TrimSpace(r.FormValue("title"))
			body := strings.TrimSpace(r.FormValue("body"))
			startsAt, err := parseLocalDateTime(r.FormValue("starts_at"))
			if title == "" || body == "" || err != nil {
				redirectWithError(w, r, "/admin/bulletins", "Title, body, and a valid start time are required.")
				return
			}
			endsAt := time.Time{}
			if strings.TrimSpace(r.FormValue("ends_at")) != "" {
				endsAt, err = parseLocalDateTime(r.FormValue("ends_at"))
				if err != nil {
					redirectWithError(w, r, "/admin/bulletins", "End time must be a valid local date/time.")
					return
				}
				if endsAt.Before(startsAt) {
					redirectWithError(w, r, "/admin/bulletins", "End time must be after the start time.")
					return
				}
			}
			row := scheduledBulletin{
				ID:        existing.ID,
				Title:     title,
				Body:      body,
				StartsAt:  startsAt.UTC(),
				EndsAt:    endsAt.UTC(),
				Link:      strings.TrimSpace(r.FormValue("link")),
				Audience:  strings.TrimSpace(r.FormValue("audience")),
				CreatedBy: user.Handle,
				CreatedAt: existing.CreatedAt,
			}
			if row.ID == "" {
				row.ID = randomEventID()
			}
			if row.CreatedAt.IsZero() {
				row.CreatedAt = time.Now().UTC()
			}
			if existingIdx >= 0 {
				rows[existingIdx] = row
			} else {
				rows = append(rows, row)
			}
			a.persistScheduledBulletins(rows)
			actionLabel := "create_bulletin"
			notice := "Scheduled bulletin created."
			if action == "update" {
				actionLabel = "update_bulletin"
				notice = "Scheduled bulletin updated."
			}
			a.recordAdminAction(user.Handle, "scheduled_bulletins", actionLabel, fmt.Sprintf("id=%s title=%s", row.ID, row.Title))
			redirectWithNotice(w, r, "/admin/bulletins", notice)
			return
		case "delete":
			id := strings.TrimSpace(r.FormValue("id"))
			if id == "" {
				redirectWithError(w, r, "/admin/bulletins", "Bulletin ID is required.")
				return
			}
			next := make([]scheduledBulletin, 0, len(rows))
			deleted := ""
			for _, row := range rows {
				if row.ID == id {
					deleted = row.Title
					continue
				}
				next = append(next, row)
			}
			if deleted == "" {
				redirectWithError(w, r, "/admin/bulletins", "Scheduled bulletin not found.")
				return
			}
			a.persistScheduledBulletins(next)
			a.recordAdminAction(user.Handle, "scheduled_bulletins", "delete_bulletin", fmt.Sprintf("id=%s title=%s", id, deleted))
			redirectWithNotice(w, r, "/admin/bulletins", "Scheduled bulletin deleted.")
			return
		default:
			redirectWithError(w, r, "/admin/bulletins", "Unsupported bulletin action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rows := a.loadScheduledBulletins()
	ackSummary := a.loadBulletinAckSummary()
	editID := strings.TrimSpace(r.URL.Query().Get("edit"))
	formBulletin := scheduledBulletin{Audience: "all callers"}
	formAction := "create"
	formTitle := "Create Scheduled Bulletin"
	submitLabel := "Create Bulletin"
	cancelLink := ""
	if editRow, _, found := findScheduledBulletin(rows, editID); found {
		formBulletin = editRow
		formAction = "update"
		formTitle = "Edit Scheduled Bulletin"
		submitLabel = "Save Changes"
		cancelLink = `<p><a href="/admin/bulletins">Cancel editing</a></p>`
	}
	csrf := a.csrfHiddenInput(r)
	activeRows := strings.Builder{}
	for _, row := range a.activeScheduledBulletins(time.Now().UTC(), 8) {
		meta := row.StartsAt.Local().Format("2006-01-02 15:04")
		if !row.EndsAt.IsZero() {
			meta += " until " + row.EndsAt.Local().Format("2006-01-02 15:04")
		}
		activeRows.WriteString(`<li><strong>` + htmlEscape(row.Title) + `</strong> <span class="wolfbbs-muted">` + htmlEscape(meta) + `</span><br>` + htmlEscape(row.Body) + `</li>`)
	}
	if activeRows.Len() == 0 {
		activeRows.WriteString(`<li>No live timed announcements right now.</li>`)
	}
	upcomingRows := strings.Builder{}
	for _, row := range a.upcomingScheduledBulletins(time.Now().UTC(), 8) {
		upcomingRows.WriteString(`<li><strong>` + htmlEscape(row.Title) + `</strong> <span class="wolfbbs-muted">` + row.StartsAt.Local().Format("2006-01-02 15:04") + `</span></li>`)
	}
	if upcomingRows.Len() == 0 {
		upcomingRows.WriteString(`<li>No future bulletins queued.</li>`)
	}
	tableRows := strings.Builder{}
	for _, row := range rows {
		window := row.StartsAt.Local().Format("2006-01-02 15:04")
		if !row.EndsAt.IsZero() {
			window += " to " + row.EndsAt.Local().Format("2006-01-02 15:04")
		}
		ackMeta := "0"
		if stat, ok := ackSummary[row.ID]; ok {
			ackMeta = strconv.Itoa(stat.Count)
			if !stat.LastAt.IsZero() {
				ackMeta += ` / ` + stat.LastAt.Local().Format("01-02 15:04")
			}
		}
		tableRows.WriteString(`<tr><td>` + htmlEscape(row.Title) + `</td><td>` + htmlEscape(window) + `</td><td>` + htmlEscape(defaultIfBlank(row.Audience, "all callers")) + `</td><td>` + htmlEscape(cleanOneLiner(row.Body, 96)) + `</td><td>` + htmlEscape(ackMeta) + `</td><td><a href="/admin/bulletins?edit=` + url.QueryEscape(row.ID) + `#bulletin-editor">Edit</a> <form method="POST" action="/admin/bulletins" style="display:inline"><input type="hidden" name="action" value="delete"><input type="hidden" name="id" value="` + htmlEscape(row.ID) + `">` + csrf + `<button type="submit">Delete</button></form></td></tr>`)
	}
	if tableRows.Len() == 0 {
		tableRows.WriteString(`<tr><td colspan="6">No scheduled bulletins yet.</td></tr>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Scheduled Bulletins</title></head><body>
<p><a href="/admin">admin</a> | <a href="/bulletins">public bulletins</a> | <a href="/admin/events">events</a> | <a href="/admin/ops">ops</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Scheduled Bulletins</h1>
<p>Timed announcements for content drops, go-live notices, live sessions, and other system wire items that should appear on a schedule instead of being hand-posted.</p>
<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Use the wire deliberately</strong><p>Keep timed announcements short, concrete, and linked to a real next action like <a href="/events">/events</a>, <a href="/boards">/boards</a>, or <a href="/doors">/doors</a>.</p></article><article class="wolfbbs-helper-card"><strong>Start and end matter</strong><p>Use short windows for urgent notices so the bulletin center does not become a stale wall of yesterday’s news.</p></article><article class="wolfbbs-helper-card"><strong>Pair bulletins with routes</strong><p>Announcements work best when the link drops callers directly into the route they should use next.</p></article></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card" id="bulletin-editor"><h2>` + formTitle + `</h2><form method="POST" action="/admin/bulletins" data-draft-key="admin-bulletin-editor"><input type="hidden" name="action" value="` + formAction + `"><input type="hidden" name="id" value="` + htmlEscape(formBulletin.ID) + `">` + csrf + `<label>Title <input name="title" size="48" value="` + htmlEscape(formBulletin.Title) + `" placeholder="Tonight: Tournament Night"></label><br><label>Starts <input type="datetime-local" name="starts_at" value="` + htmlEscape(formatLocalDateTimeValue(formBulletin.StartsAt)) + `"></label><br><label>Ends <input type="datetime-local" name="ends_at" value="` + htmlEscape(formatLocalDateTimeValue(formBulletin.EndsAt)) + `"></label><br><label>Audience <input name="audience" size="32" value="` + htmlEscape(formBulletin.Audience) + `" placeholder="all callers"></label><br><label>Link <input name="link" size="56" value="` + htmlEscape(formBulletin.Link) + `" placeholder="/events or /doors"></label><br><label>Body<br><textarea name="body" rows="5" cols="72" placeholder="What is happening, why it matters, and where to go next.">` + htmlEscape(formBulletin.Body) + `</textarea></label><br><button type="submit">` + submitLabel + `</button></form>` + cancelLink + `</article><article class="wolfbbs-card"><h2>Live Wire Preview</h2><ul class="wolfbbs-list-clean">` + activeRows.String() + `</ul><h3>Upcoming</h3><ul class="wolfbbs-list-clean">` + upcomingRows.String() + `</ul></article></section>
<section><h2>Queued Bulletins</h2><table border="1"><tr><th>Title</th><th>Window</th><th>Audience</th><th>Body</th><th>Acks / last</th><th>Action</th></tr>` + tableRows.String() + `</table></section>
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
	if r.Method == http.MethodPost {
		if !a.requireCSRF(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		targetHandle := strings.TrimSpace(r.FormValue("target"))
		returnTo := "/directory"
		if targetHandle != "" {
			returnTo = "/directory?handle=" + url.QueryEscape(targetHandle)
		}
		returnTo = safeLocalRedirectPath(r.FormValue("return_to"), returnTo)
		target, err := a.authSvc.GetUser(targetHandle)
		if action != "save_staff_note" && (err != nil || !directoryVisibleUser(target)) {
			redirectWithError(w, r, returnTo, "Caller not found.")
			return
		}
		switch action {
		case "toggle_favorite":
			if target == nil || strings.EqualFold(target.Handle, user.Handle) {
				redirectWithError(w, r, returnTo, "Choose another caller.")
				return
			}
			favorited := a.toggleFavoriteCaller(user.Handle, target.Handle)
			label := "Removed from favorite callers."
			if favorited {
				label = "Added to favorite callers."
			}
			redirectWithNotice(w, r, returnTo, label)
			return
		case "send_page":
			if target == nil || strings.EqualFold(target.Handle, user.Handle) {
				redirectWithError(w, r, returnTo, "Choose another caller.")
				return
			}
			presence, online := a.activePresenceForHandle(target.Handle)
			if !online {
				redirectWithError(w, r, returnTo, "Caller is not on a live node right now.")
				return
			}
			message := strings.TrimSpace(r.FormValue("message"))
			if message == "" {
				message = "Please check mail or chat when you have a moment."
			}
			source := strings.TrimSpace(r.FormValue("source"))
			if source == "" {
				source = "directory"
			}
			a.queuePageRequest(user.Handle, target.Handle, message, source+" / "+presence.Node)
			redirectWithNotice(w, r, returnTo, "Page sent to "+target.Handle+".")
			return
		case "save_alias":
			if target == nil || strings.EqualFold(target.Handle, user.Handle) {
				redirectWithError(w, r, returnTo, "Choose another caller.")
				return
			}
			a.setContactAlias(user.Handle, target.Handle, r.FormValue("alias"))
			redirectWithNotice(w, r, returnTo, "Alias updated.")
			return
		case "send_event_invite":
			if target == nil || strings.EqualFold(target.Handle, user.Handle) {
				redirectWithError(w, r, returnTo, "Choose another caller.")
				return
			}
			eventID := strings.TrimSpace(r.FormValue("event_id"))
			eventRow, found := a.findEventOccurrenceByID(eventID, time.Now().UTC())
			if !found {
				redirectWithError(w, r, returnTo, "Event not found.")
				return
			}
			subject := "Invitation: " + cleanOneLiner(eventRow.Title, 72)
			body := "Join me for " + eventRow.Title + ".\n\n" +
				"When: " + formatCommunityEventWindow(eventRow) + "\n" +
				"Where: " + defaultIfBlank(eventRow.Location, "see event page") + "\n"
			if strings.TrimSpace(eventRow.Description) != "" {
				body += "\n" + cleanOneLiner(eventRow.Description, 320) + "\n"
			}
			if strings.TrimSpace(eventRow.Link) != "" {
				body += "\nLink: " + strings.TrimSpace(eventRow.Link) + "\n"
			}
			if err := a.mailRepo.CreateMail(&domain.PrivateMail{
				FromUserID: user.ID,
				ToUserID:   target.ID,
				Subject:    subject,
				Body:       strings.TrimSpace(body),
			}); err != nil {
				redirectWithError(w, r, returnTo, "Could not send event invite.")
				return
			}
			if parseCheckbox(r.FormValue("notify_live")) {
				if presence, online := a.activePresenceForHandle(target.Handle); online {
					a.queuePageRequest(user.Handle, target.Handle, "Event invite sent: "+cleanOneLiner(eventRow.Title, 72), "directory invite / "+presence.Node)
				}
			}
			a.setEventRSVP(target.Handle, eventRow.ID, "maybe", user.Handle)
			redirectWithNotice(w, r, returnTo, "Event invite sent to "+target.Handle+".")
			return
		case "save_staff_note":
			if !a.hasRole(user, roleModerator) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			target, err = a.authSvc.GetUser(targetHandle)
			if err != nil || !directoryVisibleUser(target) {
				redirectWithError(w, r, returnTo, "Caller not found.")
				return
			}
			note := strings.TrimSpace(r.FormValue("staff_note"))
			a.persistStaffNote(target.Handle, note)
			if parseCheckbox(r.FormValue("escalate")) && note != "" {
				a.queueStaffEscalation(user.Handle, target.Handle, note)
			}
			a.recordAdminAction(user.Handle, "caller_profile", "save_staff_note", fmt.Sprintf("target=%s length=%d", target.Handle, len(note)))
			redirectWithNotice(w, r, returnTo, "Staff note saved.")
			return
		default:
			redirectWithError(w, r, returnTo, "Unsupported directory action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	onlineOnly := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("online")), "1")
	favoritesOnly := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("favorites")), "1")
	verifiedFilter := normalizeDirectoryVerifiedFilter(r.URL.Query().Get("verified"))
	roleFilter := normalizeDirectoryRoleFilter(r.URL.Query().Get("role"))
	targetHandle := strings.TrimSpace(r.URL.Query().Get("handle"))
	if targetHandle == "" {
		targetHandle = user.Handle
	}
	rows := a.buildDirectoryRows(query, onlineOnly, verifiedFilter, roleFilter)
	favoriteHandles := a.loadFavoriteCallers(user.Handle)
	favoriteSet := map[string]bool{}
	for _, row := range favoriteHandles {
		favoriteSet[normalizeHandleKey(row)] = true
	}
	if favoritesOnly {
		filtered := make([]directoryRow, 0, len(rows))
		for _, row := range rows {
			if favoriteSet[normalizeHandleKey(row.Handle)] {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
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
	var profileTarget *domain.User
	if targetHandle != "" {
		if target, err := a.authSvc.GetUser(targetHandle); err == nil && directoryVisibleUser(target) {
			profileTarget = target
			profile = a.buildDirectoryProfile(user, target)
		}
	}
	profileBlock := ``
	if profile != nil {
		profile.FavoriteCaller = favoriteSet[normalizeHandleKey(profile.Handle)]
		profile.Correspondents = a.recurringCorrespondentStats(profileTarget, 5)
		if a.hasRole(user, roleModerator) {
			profile.StaffNote = a.loadStaffNote(profile.Handle)
		}
		recentRows := strings.Builder{}
		for _, row := range profile.RecentCallerRows {
			recentRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
		}
		if recentRows.Len() == 0 {
			recentRows.WriteString(`<li>No caller history rows for this handle yet.</li>`)
		}
		correspondentRows := strings.Builder{}
		for _, row := range profile.Correspondents {
			line := strconv.Itoa(row.Count) + ` exchanges`
			if row.Online {
				line += ` • online in ` + row.Area
			}
			correspondentRows.WriteString(`<li><a href="/directory?handle=` + url.QueryEscape(row.Handle) + `">` + htmlEscape(row.Handle) + `</a> <span class="wolfbbs-muted">` + htmlEscape(line) + `</span></li>`)
		}
		if correspondentRows.Len() == 0 {
			correspondentRows.WriteString(`<li>No recurring correspondents yet.</li>`)
		}
		relationshipRows := strings.Builder{}
		for _, row := range profile.Relationship {
			relationshipRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
		}
		if relationshipRows.Len() == 0 {
			relationshipRows.WriteString(`<li>No direct relationship timeline yet.</li>`)
		}
		incidentRows := strings.Builder{}
		for _, row := range profile.IncidentTimeline {
			incidentRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
		}
		if incidentRows.Len() == 0 {
			incidentRows.WriteString(`<li>No staff incident history for this caller.</li>`)
		}
		circleRows := strings.Builder{}
		for _, row := range profile.CircleNames {
			circleRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
		}
		if circleRows.Len() == 0 {
			circleRows.WriteString(`<li>This caller is not in one of your circles yet.</li>`)
		}
		contactMeta := "No contact preferences published."
		if len(profile.ContactPrefs) > 0 {
			contactMeta = "Prefers " + strings.Join(profile.ContactPrefs, ", ")
		}
		favoriteAction := ``
		pageAction := ``
		aliasAction := ``
		eventInviteAction := ``
		if !strings.EqualFold(profile.Handle, user.Handle) {
			buttonLabel := "Add Favorite Caller"
			if profile.FavoriteCaller {
				buttonLabel = "Remove Favorite Caller"
			}
			favoriteAction = `<form method="POST" action="/directory" class="wolfbbs-inline-actions"><input type="hidden" name="action" value="toggle_favorite"><input type="hidden" name="target" value="` + htmlEscape(profile.Handle) + `"><input type="hidden" name="return_to" value="/directory?handle=` + url.QueryEscape(profile.Handle) + `">` + a.csrfHiddenInput(r) + `<button type="submit">` + buttonLabel + `</button></form>`
			aliasAction = `<form method="POST" action="/directory" class="wolfbbs-inline-form"><input type="hidden" name="action" value="save_alias"><input type="hidden" name="target" value="` + htmlEscape(profile.Handle) + `"><input type="hidden" name="return_to" value="/directory?handle=` + url.QueryEscape(profile.Handle) + `">` + a.csrfHiddenInput(r) + `<label>Alias <input name="alias" size="28" value="` + htmlEscape(profile.Alias) + `" placeholder="personal nickname"></label><button type="submit">Save Alias</button></form>`
			if profile.Online {
				pageAction = `<form method="POST" action="/directory" class="wolfbbs-inline-form" id="page-desk"><input type="hidden" name="action" value="send_page"><input type="hidden" name="target" value="` + htmlEscape(profile.Handle) + `"><input type="hidden" name="return_to" value="/directory?handle=` + url.QueryEscape(profile.Handle) + `#page-desk"><input type="hidden" name="source" value="directory">` + a.csrfHiddenInput(r) + `<label>Page <input name="message" size="36" value="Please check mail when free."></label><button type="submit">Send Page</button></form>`
			}
			upcoming := a.upcomingCommunityEvents(12, time.Now().UTC())
			if len(upcoming) > 0 {
				optionRows := strings.Builder{}
				for _, row := range upcoming {
					optionRows.WriteString(`<option value="` + htmlEscape(row.ID) + `">` + htmlEscape(row.Title+" / "+row.StartsAt.Local().Format("01-02 15:04")) + `</option>`)
				}
				eventInviteAction = `<form method="POST" action="/directory" class="wolfbbs-inline-form"><input type="hidden" name="action" value="send_event_invite"><input type="hidden" name="target" value="` + htmlEscape(profile.Handle) + `"><input type="hidden" name="return_to" value="/directory?handle=` + url.QueryEscape(profile.Handle) + `">` + a.csrfHiddenInput(r) + `<label>Invite to <select name="event_id">` + optionRows.String() + `</select></label><label><input type="checkbox" name="notify_live" value="1"` + checkedIf(profile.Online) + `> notify live if online</label><button type="submit">Send Invite</button></form>`
			}
		}
		staffNoteBlock := ``
		riskSummaryBlock := ``
		if a.hasRole(user, roleModerator) {
			riskScore, riskLevel, riskSignals := a.buildCallerRiskSummary(profileTarget)
			riskRows := strings.Builder{}
			for _, row := range riskSignals {
				riskRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
			}
			if riskRows.Len() == 0 {
				riskRows.WriteString(`<li>No risk signals currently detected.</li>`)
			}
			riskSummaryBlock = `<article class="wolfbbs-card"><h2>Caller Risk Summary</h2><p><strong>Score:</strong> ` + strconv.Itoa(riskScore) + ` | <strong>Level:</strong> ` + htmlEscape(strings.ToUpper(riskLevel)) + `</p><ul>` + riskRows.String() + `</ul><p><a href="/admin/mod-center">Open moderation queue</a></p></article>`
			staffNoteBlock = `<article class="wolfbbs-card"><h2>Staff Notes</h2><form method="POST" action="/directory"><input type="hidden" name="action" value="save_staff_note"><input type="hidden" name="target" value="` + htmlEscape(profile.Handle) + `"><input type="hidden" name="return_to" value="/directory?handle=` + url.QueryEscape(profile.Handle) + `">` + a.csrfHiddenInput(r) + `<textarea name="staff_note" rows="6" cols="48" placeholder="Internal moderation/support context only.">` + htmlEscape(profile.StaffNote) + `</textarea><br><label><input type="checkbox" name="escalate" value="1"> Queue for staff follow-through</label><br><button type="submit">Save Staff Note</button></form></article>`
		}
		statusLineBlock := ``
		if profile.StatusLine != "" {
			statusLineBlock = `<p><strong>Status:</strong> ` + htmlEscape(profile.StatusLine) + `</p>`
		}
		bioBlock := ``
		if profile.Bio != "" {
			bioBlock = `<p><strong>Bio:</strong><br>` + htmlEscape(profile.Bio) + `</p>`
		}
		aliasLine := ``
		if profile.Alias != "" {
			aliasLine = `<p><strong>Your alias:</strong> ` + htmlEscape(profile.Alias) + `</p>`
		}
		profileBlock = `<section class="wolfbbs-grid">
<article class="wolfbbs-card"><h2>Caller Card: ` + htmlEscape(profile.Handle) + `</h2><p><strong>Role:</strong> ` + htmlEscape(profile.Role) + ` | <strong>Theme:</strong> ` + htmlEscape(profile.Theme) + ` | <strong>Verified:</strong> ` + boolToText(profile.Verified) + `</p>` + statusLineBlock + bioBlock + aliasLine + `<p><strong>Last login:</strong> ` + htmlEscape(profile.LastLogin) + `</p><p><strong>Online now:</strong> ` + boolToText(profile.Online) + ``
		if profile.Online {
			profileBlock += ` in ` + htmlEscape(profile.OnlineArea) + ` on ` + htmlEscape(profile.OnlineNode) + ` idle ` + htmlEscape(profile.OnlineIdle) + ` from ` + htmlEscape(profile.OnlineFrom)
		}
		profileBlock += `</p><p><strong>Contact prefs:</strong> ` + htmlEscape(contactMeta) + `</p><p><a href="/mail?to=` + url.QueryEscape(profile.Handle) + `">Send mail</a> | <a href="/finder?q=` + url.QueryEscape(profile.Handle) + `">Search posts</a></p>` + favoriteAction + aliasAction + pageAction + eventInviteAction + `</article>
<article class="wolfbbs-card"><h2>Caller Stats</h2><ul><li>Posts: ` + strconv.Itoa(profile.Posts) + `</li><li>Mentions: ` + strconv.Itoa(profile.Mentions) + `</li><li>Replies: ` + strconv.Itoa(profile.Replies) + `</li><li>Mail sent: ` + strconv.Itoa(profile.MailSent) + `</li><li>Mail received: ` + strconv.Itoa(profile.MailReceived) + `</li><li>Favorite door: ` + htmlEscape(profile.FavoriteDoor) + `</li><li>Achievements: ` + strconv.Itoa(profile.Achievements) + `</li></ul></article>
<article class="wolfbbs-card"><h2>Recent Calls</h2><ul>` + recentRows.String() + `</ul></article>
<article class="wolfbbs-card"><h2>Recurring Correspondents</h2><ul>` + correspondentRows.String() + `</ul></article>
<article class="wolfbbs-card"><h2>Your Circles</h2><ul>` + circleRows.String() + `</ul><p><a href="/circles">Manage circles</a></p></article>
<article class="wolfbbs-card"><h2>Relationship Timeline</h2><ul>` + relationshipRows.String() + `</ul></article>`
		if a.hasRole(user, roleModerator) {
			profileBlock += `<article class="wolfbbs-card"><h2>Incident Timeline</h2><ul>` + incidentRows.String() + `</ul></article>` + riskSummaryBlock
		}
		profileBlock += staffNoteBlock + `
</section>`
	}
	favoriteRows := strings.Builder{}
	visibleFavorites := 0
	for _, handle := range favoriteHandles {
		target, err := a.authSvc.GetUser(handle)
		if err != nil || !directoryVisibleUser(target) {
			continue
		}
		if profileTarget != nil && normalizeHandleKey(profileTarget.Handle) == normalizeHandleKey(handle) {
			// keep current profile visible through the main card rather than duplicate it here
			continue
		}
		visibleFavorites++
		live, _ := a.activePresenceForHandle(handle)
		actions := `<a href="/mail?to=` + url.QueryEscape(handle) + `">mail</a> | <a href="/directory?handle=` + url.QueryEscape(handle) + `">profile</a>`
		if live.Online {
			actions += ` | <a href="/directory?handle=` + url.QueryEscape(handle) + `#page-desk">page</a>`
		}
		meta := `favorite caller`
		if live.Online {
			meta = `online in ` + live.Area + ` on ` + live.Node
		}
		label := handle
		if alias := a.contactAlias(user.Handle, handle); alias != "" {
			label = alias + ` (` + handle + `)`
		}
		favoriteRows.WriteString(`<li><strong>` + htmlEscape(label) + `</strong> <span class="wolfbbs-muted">` + htmlEscape(meta) + `</span><br>` + actions + `</li>`)
	}
	if favoriteRows.Len() == 0 {
		favoriteRows.WriteString(`<li>No favorite callers yet.</li>`)
	}
	tableRows := strings.Builder{}
	for _, row := range rows {
		tableRows.WriteString(`<tr><td><a href="/directory?handle=` + url.QueryEscape(row.Handle) + `">` + htmlEscape(row.Handle) + `</a></td><td>` + htmlEscape(row.Role) + `</td><td>` + boolToText(row.Verified) + `</td><td>` + htmlEscape(row.Theme) + `</td><td>` + htmlEscape(row.LastLogin) + `</td><td>` + boolToText(row.Online) + `</td><td>` + htmlEscape(row.Node) + `</td><td>` + htmlEscape(row.Idle) + `</td><td>` + htmlEscape(row.Area) + `</td><td>` + htmlEscape(row.Origin) + `</td><td>` + htmlEscape(row.From) + `</td><td><a href="/mail?to=` + url.QueryEscape(row.Handle) + `">mail</a></td></tr>`)
	}
	if tableRows.Len() == 0 {
		tableRows.WriteString(`<tr><td colspan="12">No callers matched the filter.</td></tr>`)
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
<p><a href="/boards">boards</a> | <a href="/bookmarks">bookmarks</a> | <a href="/bulletins">bulletins</a> | <a href="/finder">finder</a> | <a href="/feedback">feedback</a> | <a href="/mail">mail</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Caller Directory</h1>
<p>Classic userlist, rebuilt with live presence, caller cards, idle time, and direct compose links.</p>
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(rows)) + `</strong><span>visible callers</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(onlineCount) + `</strong><span>online now</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(verifiedCount) + `</strong><span>verified</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(staffCount) + `</strong><span>staff in view</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(visibleFavorites) + `</strong><span>favorite callers</span></article>
</section>
<form method="GET" action="/directory" class="wolfbbs-inline-form"><label>Search <input name="q" value="` + htmlEscape(query) + `" placeholder="handle, role, theme"></label><label>Role <select name="role">` + roleOptionRows.String() + `</select></label><label>Verified <select name="verified">` + verifiedOptionRows.String() + `</select></label><label><input type="checkbox" name="online" value="1"`
	if onlineOnly {
		page += ` checked`
	}
	page += `> online only</label><label><input type="checkbox" name="favorites" value="1"`
	if favoritesOnly {
		page += ` checked`
	}
	page += `> favorite callers only</label><button type="submit">Filter</button></form>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Favorite Callers</h2><ul class="wolfbbs-list-clean">` + favoriteRows.String() + `</ul></article><article class="wolfbbs-card"><h2>Direct Handoff</h2><ul class="wolfbbs-list-clean"><li>Use <a href="/mail">mail</a> when the conversation needs privacy.</li><li>Use paging when someone is on a live node and you need their attention fast.</li><li>Use <a href="/chat">chat</a> when the room is the right place for the conversation.</li></ul></article></section>
` + profileBlock + `
<table border="1"><tr><th>Handle</th><th>Role</th><th>Verified</th><th>Theme</th><th>Last Login</th><th>Online</th><th>Node</th><th>Idle</th><th>Area</th><th>Origin</th><th>From</th><th>Action</th></tr>` + tableRows.String() + `</table>
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
		a.recordOperatorInsight("feedback.sent", user.Handle, "/feedback")
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
		case "quiet_hours":
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
				http.Error(w, "quiet-hours denied by board ACS", http.StatusForbidden)
				return
			}
			window := quietHoursWindow{
				Enabled:   parseCheckbox(r.FormValue("quiet_enabled")),
				StartHour: parseIntWithFallback(r.FormValue("quiet_start_hour"), 22),
				EndHour:   parseIntWithFallback(r.FormValue("quiet_end_hour"), 8),
			}
			a.setBoardQuietHours(user.Handle, boardID, window)
			notice := "Quiet hours cleared."
			if normalizeQuietHoursWindow(window).Enabled {
				notice = "Quiet hours set to " + formatQuietHoursWindow(window) + "."
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
		case "create_poll":
			question := strings.TrimSpace(r.FormValue("poll_question"))
			subject := strings.TrimSpace(r.FormValue("subject"))
			body := strings.TrimSpace(r.FormValue("poll_context"))
			options := splitTrimmedLines(r.FormValue("poll_options"), 8, 100)
			if boardID <= 0 || question == "" || len(options) < 2 {
				redirectWithError(w, r, boardPath, "Board, poll question, and at least two options are required.")
				return
			}
			board, err := a.boardRepo.Get(boardID)
			if err != nil || board == nil {
				redirectWithError(w, r, "/boards", "Board not found.")
				return
			}
			if !a.canWriteBoard(user, board) {
				http.Error(w, "poll creation denied by board ACS", http.StatusForbidden)
				return
			}
			if subject == "" {
				subject = question
			}
			if body == "" {
				body = "Poll thread: " + question
			}
			msg := &domain.Message{
				BoardID:  boardID,
				AuthorID: user.ID,
				Subject:  subject,
				Body:     body,
			}
			if err := a.msgRepo.CreateMessage(msg); err != nil {
				redirectWithError(w, r, boardPath, "Could not create poll thread. Please retry.")
				return
			}
			a.setThreadPoll(messageThreadID(*msg), threadPoll{
				Question:  question,
				Options:   options,
				CreatedBy: user.Handle,
				CreatedAt: time.Now().UTC(),
			})
			redirectWithNotice(w, r, fmt.Sprintf("/boards?board=%d&id=%d", boardID, msg.ID), "Poll thread created.")
			return
		case "vote_poll":
			threadID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("thread_id")), 10, 64)
			optionIndex, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("option_index")))
			root, err := a.msgRepo.GetMessage(threadID)
			if err != nil || root == nil {
				redirectWithError(w, r, boardPath, "Poll thread not found.")
				return
			}
			board, err := a.boardRepo.Get(root.BoardID)
			if err != nil || board == nil {
				redirectWithError(w, r, "/boards", "Board not found.")
				return
			}
			if !a.canWriteBoard(user, board) {
				http.Error(w, "poll vote denied by board ACS", http.StatusForbidden)
				return
			}
			if threadLifecycleBlocksReplies(a.threadLifecycleStateFor(threadID)) {
				redirectWithError(w, r, fmt.Sprintf("/boards?board=%d&id=%d", root.BoardID, root.ID), "Poll voting is disabled because this thread is frozen.")
				return
			}
			if !a.voteThreadPoll(threadID, user.Handle, optionIndex) {
				redirectWithError(w, r, fmt.Sprintf("/boards?board=%d&id=%d", root.BoardID, root.ID), "Could not record poll vote.")
				return
			}
			redirectWithNotice(w, r, fmt.Sprintf("/boards?board=%d&id=%d", root.BoardID, root.ID), "Poll vote recorded.")
			return
		case "edit_post":
			messageID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("message_id")), 10, 64)
			subject := strings.TrimSpace(r.FormValue("subject"))
			body := strings.TrimSpace(r.FormValue("body"))
			if messageID <= 0 || subject == "" || body == "" {
				redirectWithError(w, r, boardPath, "Message, subject, and body are required.")
				return
			}
			msg, err := a.msgRepo.GetMessage(messageID)
			if err != nil || msg == nil {
				redirectWithError(w, r, boardPath, "Message not found.")
				return
			}
			board, err := a.boardRepo.Get(msg.BoardID)
			if err != nil || board == nil {
				redirectWithError(w, r, "/boards", "Board not found.")
				return
			}
			if !a.canReadBoard(user, board) {
				http.Error(w, "edit denied by board ACS", http.StatusForbidden)
				return
			}
			canEdit := msg.AuthorID == user.ID || a.hasRole(user, roleModerator)
			if !canEdit {
				http.Error(w, "edit denied", http.StatusForbidden)
				return
			}
			threadState := a.threadLifecycleStateFor(messageThreadID(*msg))
			if threadLifecycleBlocksReplies(threadState) {
				redirectWithError(w, r, fmt.Sprintf("/boards?board=%d&id=%d", msg.BoardID, msg.ID), "Thread is frozen.")
				return
			}
			if subject == msg.Subject && body == msg.Body {
				redirectWithNotice(w, r, fmt.Sprintf("/boards?board=%d&id=%d", msg.BoardID, msg.ID), "No changes to save.")
				return
			}
			a.appendMessageRevision(msg.ID, messageRevision{
				Subject:  msg.Subject,
				Body:     msg.Body,
				EditedBy: user.Handle,
				EditedAt: time.Now().UTC(),
			})
			msg.Subject = subject
			msg.Body = body
			if err := a.msgRepo.UpdateMessage(msg); err != nil {
				redirectWithError(w, r, fmt.Sprintf("/boards?board=%d&id=%d", msg.BoardID, msg.ID), "Could not save post edits.")
				return
			}
			if a.hasRole(user, roleModerator) && msg.AuthorID != user.ID {
				a.recordAdminAction(user.Handle, "message #"+strconv.FormatInt(msg.ID, 10), "edit_message", "board moderation edit")
			}
			redirectWithNotice(w, r, fmt.Sprintf("/boards?board=%d&id=%d", msg.BoardID, msg.ID), "Post updated.")
			return
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
			threadNotice := "Message posted."
			if parentID > 0 {
				parent, err := a.msgRepo.GetMessage(parentID)
				if err != nil || parent == nil || parent.BoardID != boardID {
					redirectWithError(w, r, boardPath, "Parent message not found.")
					return
				}
				threadState := a.threadLifecycleStateFor(messageThreadID(*parent))
				if threadLifecycleBlocksReplies(threadState) {
					redirectWithError(w, r, boardPath, "Thread is frozen.")
					return
				}
				if threadLifecycleWarnsReplies(threadState) {
					threadNotice = "Message posted. This thread is in slow mode."
				}
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
			redirectWithNotice(w, r, fmt.Sprintf("/boards?board=%d", boardID), threadNotice)
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
<p><a href="/start">start</a> | <a href="/today">today</a> | <a href="/attention">attention</a> | <a href="/bookmarks">bookmarks</a> | <a href="/events">events</a> | <a href="/bulletins">bulletins</a> | <a href="/directory">directory</a> | <a href="/finder">finder</a> | <a href="/newfiles">newfiles</a> | <a href="/feedback">feedback</a> | <a href="/mail">mail</a> | <a href="/settings">settings</a> | <a href="/chat">chat</a> | <a href="/radar">radar</a> | <a href="/clubhouse">clubhouse</a> | <a href="/doors">doors</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/gateway">gateway</a>%s | <a href="/help">help</a> | <a href="/logout">logout</a></p>
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
	welcomeKit := a.boardWelcomeKitFor(board.ID)
	stewards := a.boardStewardsFor(board.ID)
	staffNote := boardStaffNote{}
	if a.hasRole(user, roleModerator) {
		staffNote = a.boardStaffNoteFor(board.ID)
	}
	threadStates := a.loadThreadLifecycleStates()
	messageID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
	showArchived := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("show_archived")), "1")
	view := strings.Builder{}
	if messageID > 0 {
		msg, err := a.msgRepo.GetMessage(messageID)
		if err == nil && msg.BoardID == boardID {
			threadID := messageThreadID(*msg)
			threadState := a.threadLifecycleStateFromCache(threadID, threadStates)
			revisions := a.messageRevisionsFor(msg.ID)
			poll, hasPoll := a.threadPollFor(threadID)
			_ = a.msgRepo.SetPointer(user.ID, boardID, msg.ID, time.Now().UTC())
			view.WriteString(`<h2>Reader</h2>`)
			view.WriteString(`<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Reading thread #` + strconv.FormatInt(threadID, 10) + `</strong><p>Use reply to continue the thread or report to send it into the moderation queue.</p></article><article class="wolfbbs-helper-card"><strong>Reply target</strong><p>Replies will quote the current post so the thread keeps context.</p></article><article class="wolfbbs-helper-card"><strong>Moderation path</strong><p>Report is for abuse, spam, or content that needs sysop attention.</p></article></section>`)
			if badge := threadLifecycleStatusPill(threadState); badge != "" {
				view.WriteString(`<p>` + badge + `</p>`)
			}
			switch threadState {
			case threadLifecycleSlow:
				view.WriteString(`<p class="wolfbbs-muted">Slow mode: keep replies deliberate and high-signal.</p>`)
			case threadLifecycleArchived:
				view.WriteString(`<p class="wolfbbs-muted">Archived thread: still readable, but hidden from the default board scan.</p>`)
			case threadLifecycleFrozen:
				view.WriteString(`<p class="wolfbbs-muted">Frozen thread: read-only until staff reopens it.</p>`)
			}
			view.WriteString(`<p><strong>Subject:</strong> ` + htmlEscape(msg.Subject) + `<br>`)
			view.WriteString(`<strong>From:</strong> ` + htmlEscape(handleByID[msg.AuthorID]) + `<br>`)
			if msg.ParentID > 0 {
				view.WriteString(`<strong>Reply-To:</strong> #` + strconv.FormatInt(msg.ParentID, 10) + `<br>`)
			}
			view.WriteString(`<strong>When:</strong> ` + msg.CreatedAt.Format("2006-01-02 15:04:05") + `</p>`)
			view.WriteString(`<pre>` + htmlEscape(msg.Body) + `</pre>`)
			if len(revisions) > 0 {
				revisionRows := strings.Builder{}
				for _, row := range revisions {
					revisionRows.WriteString(`<li><strong>` + htmlEscape(row.EditedBy) + `</strong> ` + row.EditedAt.Local().Format("2006-01-02 15:04") + `<br><span class="wolfbbs-muted">` + htmlEscape(cleanOneLiner(row.Subject, 72)) + `</span></li>`)
				}
				view.WriteString(`<h3>Revision History</h3><ul class="wolfbbs-list-clean">` + revisionRows.String() + `</ul>`)
			}
			if hasPoll {
				totals := pollVoteTotals(poll)
				pollRows := strings.Builder{}
				myVote := -1
				if poll.Votes != nil {
					if vote, ok := poll.Votes[normalizeHandleKey(user.Handle)]; ok {
						myVote = vote
					}
				}
				for idx, option := range poll.Options {
					meta := strconv.Itoa(totals[idx]) + ` vote(s)`
					if idx == myVote {
						meta += ` • your vote`
					}
					pollRows.WriteString(`<li><strong>` + htmlEscape(option) + `</strong> <span class="wolfbbs-muted">` + htmlEscape(meta) + `</span></li>`)
				}
				voteForm := `<p class="wolfbbs-muted">Poll is closed.</p>`
				if !poll.Closed && a.canWriteBoard(user, board) && !threadLifecycleBlocksReplies(threadState) {
					optionRows := strings.Builder{}
					for idx, option := range poll.Options {
						optionRows.WriteString(`<option value="` + strconv.Itoa(idx) + `">` + htmlEscape(option) + `</option>`)
					}
					voteForm = `<form method="POST" action="/boards" class="wolfbbs-inline-form"><input type="hidden" name="action" value="vote_poll"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `"><input type="hidden" name="thread_id" value="` + strconv.FormatInt(threadID, 10) + `">` + csrf + `<label>Vote <select name="option_index">` + optionRows.String() + `</select></label><button type="submit">Save Vote</button></form>`
				}
				view.WriteString(`<h3>Poll</h3><p><strong>` + htmlEscape(poll.Question) + `</strong></p><ul class="wolfbbs-list-clean">` + pollRows.String() + `</ul>` + voteForm)
			}
			bookmarkKey := "board:" + strconv.FormatInt(msg.ID, 10)
			bookmarkAction := "add_board_message"
			bookmarkLabel := "Save to Read-Later"
			if a.hasBookmark(user.Handle, bookmarkKey) {
				bookmarkAction = "remove"
				bookmarkLabel = "Remove from Read-Later"
			}
			view.WriteString(`<h3>Read-Later Queue</h3><form method="POST" action="/bookmarks" class="wolfbbs-inline-actions">` + csrf + `<input type="hidden" name="action" value="` + bookmarkAction + `">`)
			if bookmarkAction == "add_board_message" {
				view.WriteString(`<input type="hidden" name="message_id" value="` + strconv.FormatInt(msg.ID, 10) + `">`)
			} else {
				view.WriteString(`<input type="hidden" name="key" value="` + bookmarkKey + `">`)
			}
			view.WriteString(`<input type="hidden" name="return_to" value="/boards?board=` + strconv.FormatInt(boardID, 10) + `&id=` + strconv.FormatInt(msg.ID, 10) + `"><button type="submit">` + bookmarkLabel + `</button></form>`)
			view.WriteString(`<h3>Report</h3>`)
			view.WriteString(`<form method="POST" action="/boards"><input type="hidden" name="action" value="report"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `"><input type="hidden" name="message_id" value="` + strconv.FormatInt(msg.ID, 10) + `">` + csrf)
			view.WriteString(`<label>Reason: <input name="reason" size="48" placeholder="spam, abuse, off-topic"></label> <button type="submit">Report Post</button></form>`)
			canEdit := msg.AuthorID == user.ID || a.hasRole(user, roleModerator)
			if canEdit {
				if threadLifecycleBlocksReplies(threadState) {
					view.WriteString(`<p><em>Editing is disabled because this thread is frozen.</em></p>`)
				} else {
					view.WriteString(`<h3>Edit Post</h3>`)
					view.WriteString(`<form method="POST" action="/boards" data-draft-key="board-` + strconv.FormatInt(boardID, 10) + `-edit-` + strconv.FormatInt(msg.ID, 10) + `" data-rich-compose="board-edit" data-compose-signature="` + htmlEscape(user.Handle) + `"><input type="hidden" name="action" value="edit_post"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `"><input type="hidden" name="message_id" value="` + strconv.FormatInt(msg.ID, 10) + `">` + csrf)
					view.WriteString(`<label>Subject: <input name="subject" value="` + htmlEscape(msg.Subject) + `" size="60"></label><br>`)
					view.WriteString(`<label>Body:<br><textarea name="body" rows="8" cols="80">` + htmlEscape(msg.Body) + `</textarea></label><br>`)
					view.WriteString(`<button type="submit">Save Edit</button></form>`)
				}
			}
			if a.canWriteBoard(user, board) {
				if threadLifecycleBlocksReplies(threadState) {
					view.WriteString(`<p><em>Replying is disabled because this thread is frozen.</em></p>`)
				} else {
					view.WriteString(`<h3>Reply</h3>`)
					if threadLifecycleWarnsReplies(threadState) {
						view.WriteString(`<p class="wolfbbs-muted">Slow mode is active for this thread. Keep the reply concise and worth the interrupt.</p>`)
					}
					view.WriteString(`<form method="POST" action="/boards" data-draft-key="board-` + strconv.FormatInt(boardID, 10) + `-reply-` + strconv.FormatInt(msg.ID, 10) + `" data-rich-compose="board-reply" data-compose-signature="` + htmlEscape(user.Handle) + `" data-compose-quote="` + htmlEscape(quoteBody(msg.Body)) + `"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `"><input type="hidden" name="parent_id" value="` + strconv.FormatInt(msg.ID, 10) + `">` + csrf)
					view.WriteString(`<label>Subject: <input name="subject" value="Re: ` + htmlEscape(msg.Subject) + `" size="60"></label><br>`)
					view.WriteString(`<label>Body:<br><textarea name="body" rows="10" cols="80">` + htmlEscape(quoteBody(msg.Body)) + `</textarea></label><br>`)
					view.WriteString(`<button type="submit">Post Reply</button></form>`)
				}
			} else {
				view.WriteString(`<p><em>Replying is disabled by board ACS policy.</em></p>`)
			}
		}
	}

	archivedHiddenCount := 0
	displayMsgs := make([]domain.Message, 0, len(msgs))
	for _, msg := range msgs {
		state := a.threadLifecycleStateFromCache(messageThreadID(msg), threadStates)
		if state == threadLifecycleArchived && !showArchived && messageThreadID(msg) != messageID {
			archivedHiddenCount++
			continue
		}
		displayMsgs = append(displayMsgs, msg)
	}
	rows := strings.Builder{}
	for _, msg := range displayMsgs {
		subject := msg.Subject
		if msg.ParentID > 0 {
			subject = "> " + subject
		}
		newMark := ""
		if msg.ID > pointerID {
			newMark = "N"
		}
		subjectHTML := htmlEscape(subject)
		if badge := threadLifecycleStatusPill(a.threadLifecycleStateFromCache(messageThreadID(msg), threadStates)); badge != "" {
			subjectHTML = badge + ` ` + subjectHTML
		}
		rows.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td><a href="/boards?board=%d&id=%d">%s</a></td><td>%s</td><td>%s</td></tr>`,
			msg.ID, newMark, boardID, msg.ID, subjectHTML, htmlEscape(handleByID[msg.AuthorID]), msg.CreatedAt.Format("2006-01-02 15:04")))
	}
	discoverLink := ""
	if a.discover {
		discoverLink = ` | <a href="/discover">discover</a>`
	}
	messageBlock := pageMessageBlock(r)
	boardHelperBlock := `<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Board scan</strong><p>Rows marked N are newer than your current read pointer for this board.</p></article><article class="wolfbbs-helper-card"><strong>Posting flow</strong><p>Drafts in the compose boxes are saved locally in this browser while you type.</p></article><article class="wolfbbs-helper-card"><strong>Subscription tier</strong><p>Watch sends this board into Attention Center, digest keeps it in Today Brief, and mute removes it from routine loops while keeping access intact.</p></article><article class="wolfbbs-helper-card"><strong>Lifecycle states</strong><p>Slow warns callers, archived hides threads from the default scan, and frozen makes a thread read-only.</p></article></section>`
	motdBlock := ""
	if strings.TrimSpace(a.motd) != "" {
		motdBlock = `<p><strong>MOTD:</strong> ` + htmlEscape(a.motd) + `</p>`
	}
	announcementBlock := ""
	if strings.TrimSpace(a.announcement) != "" {
		announcementBlock = `<p><strong>Announcement:</strong> ` + htmlEscape(a.announcement) + `</p>`
	}
	emptyBoardDetail := ``
	if len(displayMsgs) == 0 {
		emptyBoardDetail = a.renderRoleAwareEmptyState(user, "board_detail")
	}
	if archivedHiddenCount > 0 && !showArchived {
		emptyBoardDetail += `<article class="wolfbbs-card"><h2>Archived Threads Hidden</h2><p>` + strconv.Itoa(archivedHiddenCount) + ` archived row(s) are hidden from the default scan.</p><p><a href="/boards?board=` + strconv.FormatInt(board.ID, 10) + `&show_archived=1">Show archived threads</a></p></article>`
	}
	quietWindow, quietConfigured := a.boardQuietHoursWindow(user.Handle, board.ID)
	quietWindow = normalizeQuietHoursWindow(quietWindow)
	quietStartOptions := strings.Builder{}
	quietEndOptions := strings.Builder{}
	for hour := 0; hour < 24; hour++ {
		label := fmt.Sprintf("%02d:00", hour)
		quietStartOptions.WriteString(`<option value="` + strconv.Itoa(hour) + `"` + selectedIf(hour == quietWindow.StartHour) + `>` + label + `</option>`)
		quietEndOptions.WriteString(`<option value="` + strconv.Itoa(hour) + `"` + selectedIf(hour == quietWindow.EndHour) + `>` + label + `</option>`)
	}
	welcomeBlock := ``
	if welcomeKit.Intro != "" || len(welcomeKit.SeedPrompts) > 0 || len(welcomeKit.StarterThreads) > 0 {
		promptRows := strings.Builder{}
		for _, row := range welcomeKit.SeedPrompts {
			promptRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
		}
		if promptRows.Len() == 0 {
			promptRows.WriteString(`<li>No starter prompts configured.</li>`)
		}
		starterRows := strings.Builder{}
		for _, row := range welcomeKit.StarterThreads {
			starterRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
		}
		if starterRows.Len() == 0 {
			starterRows.WriteString(`<li>No starter threads configured.</li>`)
		}
		welcomeBlock = `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Board Welcome Kit</h2><p>` + htmlEscape(defaultIfBlank(welcomeKit.Intro, "This board has starter prompts and launch guidance ready.")) + `</p><h3>Seed Prompts</h3><ul class="wolfbbs-list-clean">` + promptRows.String() + `</ul></article><article class="wolfbbs-card"><h2>Starter Thread Ideas</h2><ul class="wolfbbs-list-clean">` + starterRows.String() + `</ul></article></section>`
	}
	stewardBlock := ``
	if len(stewards) > 0 {
		stewardRows := strings.Builder{}
		for _, row := range stewards {
			meta := []string{}
			if row.Topic != "" {
				meta = append(meta, row.Topic)
			}
			if row.Note != "" {
				meta = append(meta, row.Note)
			}
			stewardRows.WriteString(`<li><strong>` + htmlEscape(row.Handle) + `</strong>`)
			if len(meta) > 0 {
				stewardRows.WriteString(` <span class="wolfbbs-muted">` + htmlEscape(strings.Join(meta, " | ")) + `</span>`)
			}
			stewardRows.WriteString(`</li>`)
		}
		stewardBlock = `<article class="wolfbbs-card"><h2>Topic Stewards</h2><ul class="wolfbbs-list-clean">` + stewardRows.String() + `</ul></article>`
	}
	staffNoteBlock := ``
	if a.hasRole(user, roleModerator) && staffNote.Note != "" {
		staffNoteBlock = `<article class="wolfbbs-card"><h2>Staff Note</h2><p>` + htmlEscape(staffNote.Note) + `</p><p class="wolfbbs-muted">Updated by ` + htmlEscape(defaultIfBlank(staffNote.UpdatedBy, "staff")) + ` at ` + staffNote.UpdatedAt.Local().Format("2006-01-02 15:04") + `</p></article>`
	}
	showArchivedLink := `<a href="/boards?board=` + strconv.FormatInt(board.ID, 10) + `&show_archived=1">show archived</a>`
	if showArchived {
		showArchivedLink = `<a href="/boards?board=` + strconv.FormatInt(board.ID, 10) + `">hide archived</a>`
	}
	page := `<html><body><h1>Board: ` + htmlEscape(board.Name) + `</h1>` +
		`<p><a href="/start">start</a> | <a href="/today">today</a> | <a href="/attention">attention</a> | <a href="/bookmarks">bookmarks</a> | <a href="/events">events</a> | <a href="/boards">all boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/doors">doors</a> | <a href="/status">status</a> | <a href="/config">config</a>` + discoverLink + ` | <a href="/help">help</a> | <a href="/logout">logout</a></p>` +
		messageBlock +
		`<p><strong>Reader keys:</strong> open subject to read, use Reply form, and Report for abuse/moderation queue.</p>` +
		`<p><strong>Conference:</strong> ` + htmlEscape(defaultConferenceValue(board.Conference)) + `</p>` +
		`<p><strong>Thread view:</strong> ` + showArchivedLink + `</p>` +
		`<form method="POST" action="/boards" class="wolfbbs-inline-actions"><input type="hidden" name="action" value="subscribe"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `">` + csrf + `<label>Subscription <select name="subscription_mode">` + boardSubscriptionOptionRows(normalizeBoardSubscriptionMode(string(subscriptions[board.ID]))) + `</select></label><button type="submit">Save</button></form>` +
		`<form method="POST" action="/boards" class="wolfbbs-inline-actions"><input type="hidden" name="action" value="quiet_hours"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `">` + csrf + `<label><input type="checkbox" name="quiet_enabled" value="1" ` + checkedAttr(quietConfigured && quietWindow.Enabled) + `> Quiet hours</label><label>from <select name="quiet_start_hour">` + quietStartOptions.String() + `</select></label><label>to <select name="quiet_end_hour">` + quietEndOptions.String() + `</select></label><button type="submit">Save Quiet Hours</button></form><p class="wolfbbs-muted">Current quiet window: ` + htmlEscape(formatQuietHoursWindow(quietWindow)) + `</p>` +
		motdBlock + announcementBlock + boardHelperBlock + welcomeBlock + `<section class="wolfbbs-grid">` + stewardBlock + staffNoteBlock + `</section>` + emptyBoardDetail +
		`<table border="1"><tr><th>ID</th><th>New</th><th>Subject</th><th>Author</th><th>When</th></tr>` + rows.String() + `</table>`
	if a.canWriteBoard(user, board) {
		page += `<h3>New Post</h3><form method="POST" action="/boards" data-draft-key="board-` + strconv.FormatInt(boardID, 10) + `-post" data-rich-compose="board-post" data-compose-signature="` + htmlEscape(user.Handle) + `"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `">` + csrf +
			`<label>Subject: <input name="subject" size="60"></label><br><label>Body:<br><textarea name="body" rows="10" cols="80"></textarea></label><br><button type="submit">Post</button></form>`
		page += `<h3>Create Poll Thread</h3><form method="POST" action="/boards" data-draft-key="board-` + strconv.FormatInt(boardID, 10) + `-poll" data-rich-compose="board-poll" data-compose-signature="` + htmlEscape(user.Handle) + `"><input type="hidden" name="action" value="create_poll"><input type="hidden" name="board_id" value="` + strconv.FormatInt(boardID, 10) + `">` + csrf +
			`<label>Poll Subject: <input name="subject" size="60" placeholder="Should we run a tournament night?"></label><br><label>Question: <input name="poll_question" size="72" placeholder="What should happen next on this board?"></label><br><label>Intro / context<br><textarea name="poll_context" rows="5" cols="80" placeholder="Optional context for the vote."></textarea></label><br><label>Options (one per line)<br><textarea name="poll_options" rows="5" cols="60" placeholder="Yes&#10;No&#10;Need more info"></textarea></label><br><button type="submit">Create Poll</button></form>`
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
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		if action == "" {
			action = "send"
		}
		if !a.canSendMail(user) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		switch action {
		case "save_template":
			if err := a.saveMailTemplate(
				user.Handle,
				r.FormValue("template_id"),
				r.FormValue("template_name"),
				r.FormValue("template_subject"),
				r.FormValue("template_body"),
				r.FormValue("template_urgency"),
			); err != nil {
				redirectWithError(w, r, "/mail", "Reply kit error: "+err.Error())
				return
			}
			redirectWithNotice(w, r, "/mail", "Saved reply kit.")
			return
		case "delete_template":
			if err := a.deleteMailTemplate(user.Handle, r.FormValue("template_id")); err != nil {
				redirectWithError(w, r, "/mail", "Reply kit error: "+err.Error())
				return
			}
			redirectWithNotice(w, r, "/mail", "Reply kit deleted.")
			return
		}
		toRaw := strings.TrimSpace(r.FormValue("to"))
		subject := strings.TrimSpace(r.FormValue("subject"))
		body := strings.TrimSpace(r.FormValue("body"))
		urgency := normalizeMailUrgency(r.FormValue("urgency"))
		selectedTemplateID := strings.TrimSpace(r.FormValue("template_id"))
		if toRaw == "" || subject == "" || body == "" {
			redirectWithError(w, r, "/mail", "To, subject, and body are required.")
			return
		}
		subject = applyMailUrgency(subject, urgency)

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
			emailGateway := a.activeEmailGateway()
			if err := emailGateway.SendOutbound(user.Handle, []string{recipient}, subject, body); err != nil {
				redirectWithError(w, r, "/mail", "Email relay error: "+err.Error())
				return
			}
		} else {
			target, err := a.authSvc.GetUser(toRaw)
			if err != nil || target == nil {
				redirectWithError(w, r, "/mail", "Unknown recipient handle.")
				return
			}
			if !directoryVisibleUser(target) {
				redirectWithError(w, r, "/mail", "Recipient is not available for local mail.")
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
		if selectedTemplateID != "" {
			a.markMailTemplateUsed(user.Handle, selectedTemplateID)
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
	selectedTemplateID := strings.TrimSpace(r.URL.Query().Get("saved_template"))
	mailTemplates := a.loadMailTemplates(user.Handle)

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
		replyUrgency, cleanSubject := splitMailUrgency(item.Subject)
		replySubject := cleanSubject
		if !strings.HasPrefix(strings.ToLower(replySubject), "re:") {
			replySubject = "Re: " + replySubject
		}
		replyBody := quoteBody(item.Body)
		if selectedTemplateID != "" {
			if tpl, ok := a.mailTemplate(user.Handle, selectedTemplateID); ok {
				replyUrgency = tpl.Urgency
				if strings.TrimSpace(tpl.Subject) != "" {
					replySubject = tpl.Subject
				}
				if strings.TrimSpace(tpl.Body) != "" {
					replyBody = strings.TrimSpace(tpl.Body + "\n\n" + quoteBody(item.Body))
				}
			}
		}
		replyBlock := ``
		if a.canSendMail(user) && replyTo != "" {
			templatePicker := ``
			if len(mailTemplates) > 0 {
				templatePicker = `<form method="GET" action="/mail" class="wolfbbs-inline-form"><input type="hidden" name="id" value="` + strconv.FormatInt(item.ID, 10) + `"><label>Saved kit <select name="saved_template">` + renderMailSavedTemplateOptions(mailTemplates, selectedTemplateID) + `</select></label><button type="submit">Load kit</button></form>`
			}
			replyBlock = `<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Reply in context</strong><p>Quick Reply carries the quoted body forward so you can answer without losing the thread.</p></article><article class="wolfbbs-helper-card"><strong>Use mail for direct follow-up</strong><p>Keep public discussion on boards and use mail when the conversation should stay private.</p></article><article class="wolfbbs-helper-card"><strong>Saved kits stay reusable</strong><p>Load a saved reply kit when you want a consistent follow-up note without retyping it.</p></article></section>` +
				templatePicker +
				`<h2>Quick Reply</h2><form method="POST" action="/mail" data-draft-key="mail-reply-` + strconv.FormatInt(item.ID, 10) + `" data-rich-compose="mail-reply" data-compose-signature="` + htmlEscape(user.Handle) + `" data-compose-quote="` + htmlEscape(replyBody) + `">` + a.csrfHiddenInput(r) +
				`<input type="hidden" name="template_id" value="` + htmlEscape(selectedTemplateID) + `">` +
				`<label>To <input name="to" size="40" value="` + htmlEscape(replyTo) + `"></label><br>` +
				`<label>Urgency <select name="urgency">` + mailUrgencyOptionRows(replyUrgency) + `</select></label><br>` +
				`<label>Subject <input name="subject" size="60" value="` + htmlEscape(replySubject) + `"></label><br>` +
				`<label>Body<br><textarea name="body" rows="10" cols="80">` + htmlEscape(replyBody) + `</textarea></label><br><button type="submit">Send Reply</button></form>`
		}
		bookmarkKey := "mail:" + strconv.FormatInt(item.ID, 10)
		bookmarkAction := "add_mail"
		bookmarkLabel := "Save to Read-Later"
		if a.hasBookmark(user.Handle, bookmarkKey) {
			bookmarkAction = "remove"
			bookmarkLabel = "Remove from Read-Later"
		}
		bookmarkBlock := `<form method="POST" action="/bookmarks" class="wolfbbs-inline-actions">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="` + bookmarkAction + `">`
		if bookmarkAction == "add_mail" {
			bookmarkBlock += `<input type="hidden" name="mail_id" value="` + strconv.FormatInt(item.ID, 10) + `">`
		} else {
			bookmarkBlock += `<input type="hidden" name="key" value="` + bookmarkKey + `">`
		}
		bookmarkBlock += `<input type="hidden" name="return_to" value="/mail?id=` + strconv.FormatInt(item.ID, 10) + `"><button type="submit">` + bookmarkLabel + `</button></form>`
		page := `<html><body><h1>Mail #` + strconv.FormatInt(item.ID, 10) + `</h1><p><a href="/mail">back</a> | <a href="/bookmarks">bookmarks</a> | <a href="/boards">boards</a> | <a href="/help">help</a></p>` +
			`<p><strong>From:</strong> ` + htmlEscape(handleByID[item.FromUserID]) + `<br>` +
			`<strong>To:</strong> ` + htmlEscape(to) + `<br>` +
			`<strong>Subject:</strong> ` + mailUrgencyBadgeHTML(item.Subject) + htmlEscape(cleanSubject) + `<br>` +
			`<strong>Sent:</strong> ` + item.CreatedAt.Format("2006-01-02 15:04:05") + `</p>` +
			bookmarkBlock + `<pre>` + htmlEscape(item.Body) + `</pre>` + replyBlock + `</body></html>`
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
	prefillUrgency := normalizeMailUrgency(r.URL.Query().Get("urgency"))
	if templateSubject, templateBody := mailTemplatePrefill(templateName); templateSubject != "" || templateBody != "" {
		if prefillSubject == "" {
			prefillSubject = templateSubject
		}
		if prefillBody == "" {
			prefillBody = templateBody
		}
	}
	if selectedTemplateID != "" {
		if tpl, ok := a.mailTemplate(user.Handle, selectedTemplateID); ok {
			if prefillSubject == "" {
				prefillSubject = tpl.Subject
			}
			if prefillBody == "" {
				prefillBody = tpl.Body
			}
			if prefillUrgency == "normal" && strings.TrimSpace(tpl.Urgency) != "" {
				prefillUrgency = normalizeMailUrgency(tpl.Urgency)
			}
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
		_, cleanSubject := splitMailUrgency(row.Subject)
		inRows.WriteString(fmt.Sprintf(`<tr><td><a href="/mail?id=%d">%d</a></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			row.ID, row.ID, htmlEscape(handleByID[row.FromUserID]), mailUrgencyBadgeHTML(row.Subject)+htmlEscape(cleanSubject), row.CreatedAt.Format("2006-01-02 15:04"), status))
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
		_, cleanSubject := splitMailUrgency(row.Subject)
		outRows.WriteString(fmt.Sprintf(`<tr><td><a href="/mail?id=%d">%d</a></td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			row.ID, row.ID, htmlEscape(target), mailUrgencyBadgeHTML(row.Subject)+htmlEscape(cleanSubject), row.CreatedAt.Format("2006-01-02 15:04")))
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
		label := row
		if alias := a.contactAlias(user.Handle, row); alias != "" {
			label = alias + ` (` + row + `)`
		}
		addressRows.WriteString(`<li><a href="/mail?to=` + url.QueryEscape(row) + `">` + htmlEscape(label) + `</a></li>`)
	}
	if addressRows.Len() == 0 {
		addressRows.WriteString(`<li>No recent correspondents yet.</li>`)
	}
	localRows := strings.Builder{}
	for _, row := range a.localMailPicks(user.Handle, 10) {
		label := row
		if alias := a.contactAlias(user.Handle, row); alias != "" {
			label = alias + ` (` + row + `)`
		}
		localRows.WriteString(`<li><a href="/mail?to=` + url.QueryEscape(row) + `">` + htmlEscape(label) + `</a> | <a href="/directory?handle=` + url.QueryEscape(row) + `">profile</a></li>`)
	}
	if localRows.Len() == 0 {
		localRows.WriteString(`<li>No local caller picks yet.</li>`)
	}
	presenceHandoffBlock := ``
	if prefillTo != "" && !strings.Contains(prefillTo, "@") {
		if target, err := a.authSvc.GetUser(prefillTo); err == nil && directoryVisibleUser(target) {
			if live, ok := a.activePresenceForHandle(target.Handle); ok {
				presenceHandoffBlock = `<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>` + htmlEscape(target.Handle) + ` is online now</strong><p>Currently in ` + htmlEscape(defaultIfBlank(live.Area, "live session")) + ` on ` + htmlEscape(defaultIfBlank(live.Node, "live node")) + ` from ` + htmlEscape(defaultIfBlank(live.From, "unknown")) + `.</p></article><article class="wolfbbs-helper-card"><strong>Use the right handoff</strong><p>If it is urgent, <a href="/directory?handle=` + url.QueryEscape(target.Handle) + `#page-desk">send a page</a>. If it belongs in live conversation, switch to <a href="/chat">chat</a>. If it needs privacy or a paper trail, stay in mail.</p></article></section>`
			}
		}
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
	mailHelperBlock := `<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Triage inbox first</strong><p>Use unread to work through new mail before browsing your outbox history.</p></article><article class="wolfbbs-helper-card"><strong>Templates speed the first pass</strong><p>Use a template or saved reply kit to load a starter subject/body, then edit it into a real message.</p></article><article class="wolfbbs-helper-card"><strong>Drafts are local</strong><p>Compose and reply boxes save locally in this browser while you type.</p></article></section>`
	templateManagerBlock := renderMailTemplateManager(user.Handle, csrf, selectedTemplateID, mailTemplates)
	page := `<html><body><h1>Private Mail</h1><p><a href="/start">start</a> | <a href="/attention">attention</a> | <a href="/bookmarks">bookmarks</a> | <a href="/boards">boards</a> | <a href="/bulletins">bulletins</a> | <a href="/directory">directory</a> | <a href="/finder">finder</a> | <a href="/feedback">feedback</a> | <a href="/chat">chat</a> | <a href="/status">status</a> | <a href="/config">config</a>` + discoverLink + ` | <a href="/help">help</a> | <a href="/logout">logout</a></p>` +
		messageBlock +
		`<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(inbox)) + `</strong><span>inbox</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(unreadCount) + `</strong><span>unread</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(outbox)) + `</strong><span>outbox</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(a.recentCorrespondents(user, inbox, outbox, 10))) + `</strong><span>recent correspondents</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(mailTemplates)) + `</strong><span>saved reply kits</span></article></section>` +
		mailHelperBlock + presenceHandoffBlock + filterSummary +
		`<form method="GET" action="/mail" class="wolfbbs-inline-form" data-filter-form="mail" data-filter-reset="/mail"><label>Box <select name="box" data-filter-label="box">` + boxOptionRows.String() + `</select></label><label>Search <input name="q" value="` + htmlEscape(searchQuery) + `" placeholder="subject, body, handle" data-filter-label="search"></label><button type="submit">Filter</button></form>` +
		`<form method="GET" action="/mail" class="wolfbbs-inline-form" data-filter-form="mail-template" data-filter-reset="/mail"><label>Template <select name="template" data-filter-label="template">` + templateOptionRows.String() + `</select></label><label>To <input name="to" value="` + htmlEscape(prefillTo) + `" placeholder="optional recipient" data-filter-label="to"></label><button type="submit">Load Template</button></form>` +
		`<form method="GET" action="/mail" class="wolfbbs-inline-form"><label>Saved reply kit <select name="saved_template">` + renderMailSavedTemplateOptions(mailTemplates, selectedTemplateID) + `</select></label><label>To <input name="to" value="` + htmlEscape(prefillTo) + `" size="18" placeholder="optional recipient"></label><button type="submit">Load Kit</button></form>` +
		`<p><strong>Tip:</strong> Use handle for local mail, email address for external relay (if enabled by policy).</p>` +
		templateManagerBlock +
		`<h2>Compose</h2><form method="POST" action="/mail" data-draft-key="mail-compose" data-rich-compose="mail-compose" data-compose-signature="` + htmlEscape(user.Handle) + `">` + csrf +
		`<input type="hidden" name="action" value="send"><input type="hidden" name="template_id" value="` + htmlEscape(selectedTemplateID) + `">` +
		`<label>To (handle or email): <input name="to" size="40" value="` + htmlEscape(prefillTo) + `"></label><br>` +
		`<label>Urgency <select name="urgency">` + mailUrgencyOptionRows(prefillUrgency) + `</select></label><br>` +
		`<label>Subject: <input name="subject" size="60" value="` + htmlEscape(prefillSubject) + `"></label><br>` +
		`<label>Body:<br><textarea name="body" rows="10" cols="80">` + htmlEscape(prefillBody) + `</textarea></label><br><button type="submit">Send</button></form>` +
		`<section class="wolfbbs-grid"><article><h2>Recent Correspondents</h2><ul>` + addressRows.String() + `</ul></article><article><h2>Address Book</h2><ul>` + localRows.String() + `</ul></article></section>` +
		`<h2>Inbox</h2><table border="1"><tr><th>ID</th><th>From</th><th>Subject</th><th>Sent</th><th>Status</th></tr>` + inRows.String() + `</table>` +
		`<h2>Outbox</h2><table border="1"><tr><th>ID</th><th>To</th><th>Subject</th><th>Sent</th></tr>` + outRows.String() + `</table>` +
		`</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleBookmarks(w http.ResponseWriter, r *http.Request) {
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
		returnTo := safeLocalRedirectPath(r.FormValue("return_to"), "/bookmarks")
		switch action {
		case "add_board_message":
			messageID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("message_id")), 10, 64)
			msg, err := a.msgRepo.GetMessage(messageID)
			if err != nil || msg == nil {
				redirectWithError(w, r, returnTo, "Message not found.")
				return
			}
			board, err := a.boardRepo.Get(msg.BoardID)
			if err != nil || board == nil || !a.canReadBoard(user, board) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			a.addBookmark(user.Handle, a.boardBookmarkEntry(msg, board))
			redirectWithNotice(w, r, returnTo, "Saved to read-later queue.")
			return
		case "add_mail":
			mailID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("mail_id")), 10, 64)
			item, err := a.mailRepo.GetMail(mailID)
			if err != nil || item == nil {
				redirectWithError(w, r, returnTo, "Mail not found.")
				return
			}
			if item.ToUserID != user.ID && item.FromUserID != user.ID {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			a.addBookmark(user.Handle, a.mailBookmarkEntry(item, user))
			redirectWithNotice(w, r, returnTo, "Saved to read-later queue.")
			return
		case "remove", "clear":
			if action == "clear" {
				a.persistBookmarks(user.Handle, nil)
				redirectWithNotice(w, r, returnTo, "Read-later queue cleared.")
				return
			}
			key := strings.TrimSpace(r.FormValue("key"))
			if key == "" {
				redirectWithError(w, r, returnTo, "Bookmark key is required.")
				return
			}
			a.removeBookmark(user.Handle, key)
			redirectWithNotice(w, r, returnTo, "Removed from read-later queue.")
			return
		default:
			redirectWithError(w, r, returnTo, "Unsupported bookmark action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rows := a.loadBookmarks(user.Handle)
	csrf := a.csrfHiddenInput(r)
	listRows := strings.Builder{}
	boardCount := 0
	mailCount := 0
	for _, row := range rows {
		status := ``
		switch row.Kind {
		case "board_message":
			boardCount++
			msg, err := a.msgRepo.GetMessage(row.MessageID)
			if err != nil || msg == nil {
				status = `<span class="wolfbbs-muted">missing</span>`
			} else if board, boardErr := a.boardRepo.Get(msg.BoardID); boardErr != nil || board == nil || !a.canReadBoard(user, board) {
				status = `<span class="wolfbbs-muted">no longer visible</span>`
			}
		case "mail":
			mailCount++
			item, err := a.mailRepo.GetMail(row.MailID)
			if err != nil || item == nil {
				status = `<span class="wolfbbs-muted">missing</span>`
			} else if item.ToUserID != user.ID && item.FromUserID != user.ID {
				status = `<span class="wolfbbs-muted">forbidden</span>`
			}
		}
		listRows.WriteString(`<tr><td>` + htmlEscape(strings.ReplaceAll(row.Kind, "_", " ")) + `</td><td><a href="` + htmlEscape(row.Href) + `">` + htmlEscape(row.Label) + `</a><br><span class="wolfbbs-muted">` + htmlEscape(defaultIfBlank(row.Meta, "saved item")) + `</span></td><td>` + htmlEscape(row.AddedAt.Local().Format("2006-01-02 15:04")) + `</td><td>` + status + `</td><td><form method="POST" action="/bookmarks">` + csrf + `<input type="hidden" name="action" value="remove"><input type="hidden" name="key" value="` + htmlEscape(row.Key) + `"><input type="hidden" name="return_to" value="/bookmarks"><button type="submit">remove</button></form></td></tr>`)
	}
	if listRows.Len() == 0 {
		listRows.WriteString(`<tr><td colspan="5">No saved board posts or mail yet.</td></tr>`)
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Read-Later Queue</title></head><body>
<p><a href="/start">start</a> | <a href="/attention">attention</a> | <a href="/today">today</a> | <a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Read-Later Queue</h1>
<p>Use this as the modern version of “come back to this thread later.” Save board posts or private mail from the reader, then work through them from one place.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(rows)) + `</strong><span>saved items</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(boardCount) + `</strong><span>board posts</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(mailCount) + `</strong><span>private mail</span></article></section>
<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Save from the reader</strong><p>Use the save button on a board post or private mail item when you want to come back later without losing it.</p></article><article class="wolfbbs-helper-card"><strong>Use this as a triage lane</strong><p>Attention Center is for urgent obligations. This queue is for deliberate reading and follow-up you want to keep for the next session.</p></article><article class="wolfbbs-helper-card"><strong>Remove items when done</strong><p>Keep the list short so it stays useful instead of becoming another hidden backlog.</p></article></section>
<form method="POST" action="/bookmarks">` + csrf + `<input type="hidden" name="action" value="clear"><input type="hidden" name="return_to" value="/bookmarks"><button type="submit">Clear Queue</button></form>
<table border="1"><tr><th>Kind</th><th>Item</th><th>Saved</th><th>Status</th><th>Action</th></tr>` + listRows.String() + `</table>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleHandleSuggestions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, _ := a.currentUser(r)
	exclude := ""
	if user != nil {
		exclude = user.Handle
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	items := a.suggestHandles(query, exclude, maxHandleSuggestions)
	if mode == "recipient" && query != "" {
		exact := strings.TrimSpace(query)
		if exact != "" {
			for _, item := range items {
				if strings.EqualFold(item, exact) {
					_ = writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
					return
				}
			}
		}
	}
	_ = writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func defaultGatewaySettingsFromEnv() *domain.GatewaySettings {
	return sanitizeGatewaySettings(&domain.GatewaySettings{
		SMTPHost:        strings.TrimSpace(envFirst("SMTP_HOST", "WOLFBBS_SMTP_HOST")),
		SMTPPort:        parseInt(envFirst("SMTP_PORT", "WOLFBBS_SMTP_PORT"), 587),
		SMTPUser:        strings.TrimSpace(envFirst("SMTP_USER", "WOLFBBS_SMTP_USER")),
		SMTPPass:        strings.TrimSpace(envFirst("SMTP_PASS", "WOLFBBS_SMTP_PASS")),
		FromDomain:      strings.TrimSpace(envFirst("FROM_DOMAIN", "WOLFBBS_FROM_DOMAIN")),
		MaxRecipients:   parseInt(os.Getenv("WOLFBBS_MAIL_MAX_RECIPIENTS"), 3),
		MaxMessageBytes: parseInt(os.Getenv("WOLFBBS_MAIL_MAX_BYTES"), 65536),
		WebTimeoutSec:   10,
		WebMaxBytes:     2 * 1024 * 1024,
	})
}

func sanitizeGatewaySettings(cfg *domain.GatewaySettings) *domain.GatewaySettings {
	if cfg == nil {
		cfg = &domain.GatewaySettings{}
	}
	out := *cfg
	if out.SMTPPort <= 0 {
		out.SMTPPort = 587
	}
	if out.MaxRecipients <= 0 {
		out.MaxRecipients = 3
	}
	if out.MaxMessageBytes <= 0 {
		out.MaxMessageBytes = 65536
	}
	if out.WebTimeoutSec <= 0 {
		out.WebTimeoutSec = 10
	}
	if out.WebTimeoutSec > 120 {
		out.WebTimeoutSec = 120
	}
	if out.WebMaxBytes <= 0 {
		out.WebMaxBytes = 2 * 1024 * 1024
	}
	if out.WebMaxBytes > 8*1024*1024 {
		out.WebMaxBytes = 8 * 1024 * 1024
	}
	return &out
}

func (a *webApp) activeGatewaySettings() *domain.GatewaySettings {
	cfg := defaultGatewaySettingsFromEnv()
	if a.adminRepo != nil {
		if dbCfg, err := a.adminRepo.GetGatewaySettings(); err == nil && dbCfg != nil {
			cfg = sanitizeGatewaySettings(dbCfg)
		}
	}
	return cfg
}

func (a *webApp) activeWebFetchConfig() gateway.FetchConfig {
	cfg := a.activeGatewaySettings()
	out := gateway.DefaultFetchConfig
	out.Timeout = time.Duration(cfg.WebTimeoutSec) * time.Second
	out.MaxBodyBytes = int64(cfg.WebMaxBytes)
	return out
}

func (a *webApp) activeEmailGateway() *gateway.EmailGateway {
	cfg := a.activeGatewaySettings()
	rateLimit := parseInt(strings.TrimSpace(os.Getenv("WOLFBBS_MAIL_RATE_PER_HOUR")), 20)
	if rateLimit <= 0 {
		rateLimit = 20
	}
	return gateway.NewEmailGateway(gateway.EmailConfig{
		Host:             strings.TrimSpace(cfg.SMTPHost),
		Port:             cfg.SMTPPort,
		User:             strings.TrimSpace(cfg.SMTPUser),
		Pass:             strings.TrimSpace(cfg.SMTPPass),
		FromDomain:       strings.TrimSpace(cfg.FromDomain),
		MaxRecipients:    cfg.MaxRecipients,
		MaxMessageBytes:  cfg.MaxMessageBytes,
		RateLimitPerHour: rateLimit,
	})
}

func defaultAIGatewaySettingsFromEnv() aiGatewaySettings {
	cfg := aiGatewaySettings{
		BaseURL:      strings.TrimSpace(envFirst("WOLFBBS_GATEWAY_AI_BASE_URL", "WOLFBBS_AI_BASE_URL", "OPENAI_BASE_URL")),
		Model:        strings.TrimSpace(envFirst("WOLFBBS_GATEWAY_AI_MODEL", "WOLFBBS_AI_MODEL", "OPENAI_MODEL")),
		APIKey:       strings.TrimSpace(envFirst("WOLFBBS_GATEWAY_AI_API_KEY", "WOLFBBS_AI_API_KEY", "OPENAI_API_KEY")),
		SystemPrompt: strings.TrimSpace(envFirst("WOLFBBS_GATEWAY_AI_SYSTEM_PROMPT", "WOLFBBS_AI_SYSTEM_PROMPT")),
		TimeoutSec:   parseInt(envFirst("WOLFBBS_GATEWAY_AI_TIMEOUT_SEC", "WOLFBBS_AI_TIMEOUT_SEC"), 20),
		MaxTokens:    parseInt(envFirst("WOLFBBS_GATEWAY_AI_MAX_TOKENS", "WOLFBBS_AI_MAX_TOKENS"), 400),
		Enabled:      parseCheckbox(envFirst("WOLFBBS_GATEWAY_AI_ENABLED", "WOLFBBS_AI_ENABLED")),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4.1-mini"
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 20
	}
	if cfg.TimeoutSec > 120 {
		cfg.TimeoutSec = 120
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 400
	}
	if cfg.MaxTokens > 4000 {
		cfg.MaxTokens = 4000
	}
	if cfg.APIKey != "" && !cfg.Enabled {
		cfg.Enabled = true
	}
	return cfg
}

func allowPrivateAIGatewayBaseURLs() bool {
	return parseCheckbox(envFirst("WOLFBBS_GATEWAY_AI_ALLOW_PRIVATE", "WOLFBBS_AI_ALLOW_PRIVATE"))
}

func (a *webApp) loadAIGatewaySettings() aiGatewaySettings {
	cfg := defaultAIGatewaySettingsFromEnv()
	if a.adminRepo == nil {
		return cfg
	}
	applyString := func(key string, target *string) {
		if target == nil {
			return
		}
		value, err := a.adminRepo.GetSystemSetting(key)
		if err != nil {
			return
		}
		*target = strings.TrimSpace(value)
	}
	applyString(sysSettingGatewayAIBaseURL, &cfg.BaseURL)
	applyString(sysSettingGatewayAIModel, &cfg.Model)
	applyString(sysSettingGatewayAIAPIKey, &cfg.APIKey)
	applyString(sysSettingGatewayAISystemPrompt, &cfg.SystemPrompt)
	if value, err := a.adminRepo.GetSystemSetting(sysSettingGatewayAIEnabled); err == nil && strings.TrimSpace(value) != "" {
		cfg.Enabled = parseCheckbox(value)
	}
	if value, err := a.adminRepo.GetSystemSetting(sysSettingGatewayAITimeoutSec); err == nil && strings.TrimSpace(value) != "" {
		cfg.TimeoutSec = parseInt(value, cfg.TimeoutSec)
	}
	if value, err := a.adminRepo.GetSystemSetting(sysSettingGatewayAIMaxTokens); err == nil && strings.TrimSpace(value) != "" {
		cfg.MaxTokens = parseInt(value, cfg.MaxTokens)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4.1-mini"
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 20
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 400
	}
	if cfg.APIKey != "" && !cfg.Enabled {
		cfg.Enabled = true
	}
	return cfg
}

func (a *webApp) persistAIGatewaySettings(next aiGatewaySettings) {
	if a.adminRepo == nil {
		return
	}
	if strings.TrimSpace(next.APIKey) == "" {
		current := a.loadAIGatewaySettings()
		next.APIKey = strings.TrimSpace(current.APIKey)
	}
	a.persistSystemSetting(sysSettingGatewayAIEnabled, strconv.FormatBool(next.Enabled))
	a.persistSystemSetting(sysSettingGatewayAIBaseURL, strings.TrimSpace(next.BaseURL))
	a.persistSystemSetting(sysSettingGatewayAIModel, strings.TrimSpace(next.Model))
	a.persistSystemSetting(sysSettingGatewayAIAPIKey, strings.TrimSpace(next.APIKey))
	a.persistSystemSetting(sysSettingGatewayAISystemPrompt, strings.TrimSpace(next.SystemPrompt))
	a.persistSystemSetting(sysSettingGatewayAITimeoutSec, strconv.Itoa(next.TimeoutSec))
	a.persistSystemSetting(sysSettingGatewayAIMaxTokens, strconv.Itoa(next.MaxTokens))
}

func (a *webApp) gatewayConfigured() bool {
	cfg := a.activeGatewaySettings()
	return strings.TrimSpace(cfg.SMTPHost) != "" && cfg.SMTPPort > 0 && strings.TrimSpace(cfg.FromDomain) != ""
}

func normalizeGatewayView(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "home", "hub":
		return "home"
	case "browser", "files", "email", "ai", "rss", "summarize", "json":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "home"
	}
}

func isGatewayFileAction(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "rate_file", "save_filter", "queue_add", "queue_remove", "ticket", "request_file":
		return true
	default:
		return false
	}
}

func (a *webApp) handleGateway(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	downloadToken := strings.TrimSpace(r.URL.Query().Get("download"))
	fileView := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("view")), "files")
	view := normalizeGatewayView(r.URL.Query().Get("view"))
	if fileView {
		view = "files"
	}
	gatewayNav := `<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/doors">doors</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>`
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
		switch view {
		case "browser":
			startURL := strings.TrimSpace(r.URL.Query().Get("url"))
			if startURL == "" {
				startURL = "https://"
			}
			page := `<html><body><h1>Text Web Browser Door</h1>` + gatewayNav + messageBlock +
				`<p>Fetch readable text through the safe gateway engine. Local/private hosts are blocked and limits are enforced.</p>` +
				`<form method="POST" action="/gateway">` + csrf + `<input type="hidden" name="action" value="fetch">` +
				`<label>URL <input name="url" size="72" value="` + htmlEscape(startURL) + `"></label><br><label><input type="checkbox" name="save" value="1"> Save offline copy</label><br><button type="submit">Open in reader</button></form>` +
				`<p><a href="/gateway?view=summarize">Summarize article</a> | <a href="/gateway?view=rss">RSS feeds</a> | <a href="/gateway?view=json">JSON API explorer</a> | <a href="/gateway?view=ai">AI client</a> | <a href="/gateway?view=files">FileBase gateway</a></p>` +
				`</body></html>`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(page))
			return
		case "rss":
			page := `<html><body><h1>RSS / Atom Feed Reader Door</h1>` + gatewayNav + messageBlock +
				`<p>Load an RSS/Atom feed and read the newest items in a compact BBS-friendly format.</p>` +
				`<form method="POST" action="/gateway">` + csrf + `<input type="hidden" name="action" value="rss_fetch">` +
				`<label>Feed URL <input name="feed_url" size="72" value="https://hnrss.org/frontpage"></label><br><label>Items <input name="limit" size="4" value="10"></label><br><button type="submit">Load feed</button></form>` +
				`<p><a href="/gateway?view=browser">Text browser</a> | <a href="/gateway?view=summarize">Summarizer</a> | <a href="/gateway?view=json">JSON explorer</a></p>` +
				`</body></html>`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(page))
			return
		case "summarize":
			sourceURL := strings.TrimSpace(r.URL.Query().Get("url"))
			if sourceURL == "" {
				sourceURL = "https://"
			}
			page := `<html><body><h1>Article Summarizer Door</h1>` + gatewayNav + messageBlock +
				`<p>Grab a web article and return fast bullets for callers who want the gist before diving in.</p>` +
				`<form method="POST" action="/gateway">` + csrf + `<input type="hidden" name="action" value="summarize_fetch">` +
				`<label>Article URL <input name="article_url" size="72" value="` + htmlEscape(sourceURL) + `"></label><br><label>Bullets <input name="max_bullets" size="4" value="5"></label><br><button type="submit">Summarize</button></form>` +
				`<p><a href="/gateway?view=browser">Text browser</a> | <a href="/gateway?view=rss">Feed reader</a> | <a href="/gateway?view=json">JSON explorer</a></p>` +
				`</body></html>`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(page))
			return
		case "json":
			page := `<html><body><h1>JSON API Explorer Door</h1>` + gatewayNav + messageBlock +
				`<p>Fetch JSON endpoints through the gateway safety policy and render pretty output for quick operator checks.</p>` +
				`<form method="POST" action="/gateway">` + csrf + `<input type="hidden" name="action" value="json_fetch">` +
				`<label>JSON URL <input name="json_url" size="72" value="https://api.github.com/repos/golang/go"></label><br><button type="submit">Fetch JSON</button></form>` +
				`<p><a href="/gateway?view=browser">Text browser</a> | <a href="/gateway?view=rss">Feed reader</a> | <a href="/gateway?view=summarize">Summarizer</a></p>` +
				`</body></html>`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(page))
			return
		case "ai":
			aiCfg := a.loadAIGatewaySettings()
			aiStatus := "disabled"
			if aiCfg.Enabled && strings.TrimSpace(aiCfg.APIKey) != "" {
				aiStatus = "ready"
			} else if strings.TrimSpace(aiCfg.APIKey) != "" {
				aiStatus = "configured but disabled"
			}
			modelText := htmlEscape(defaultIfBlank(aiCfg.Model, "n/a"))
			baseText := htmlEscape(defaultIfBlank(aiCfg.BaseURL, "n/a"))
			page := `<html><body><h1>Generative AI Door</h1>` + gatewayNav + messageBlock +
				`<p>Use a provider-compatible chat completion endpoint from inside the BBS. Every response is clearly marked.</p>` +
				`<p><strong>Status:</strong> ` + htmlEscape(aiStatus) + ` | <strong>Model:</strong> ` + modelText + ` | <strong>Endpoint:</strong> ` + baseText + `</p>` +
				`<form method="POST" action="/gateway">` + csrf + `<input type="hidden" name="action" value="ai_prompt">` +
				`<label>Prompt<br><textarea name="prompt" rows="8" cols="88" placeholder="Ask for a quick summary, draft, or idea list."></textarea></label><br><button type="submit">Send prompt</button></form>` +
				`<p><a href="/admin/gateways">Configure AI gateway</a> | <a href="/gateway?view=browser">Text browser</a> | <a href="/gateway?view=summarize">Summarizer</a></p>` +
				`</body></html>`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(page))
			return
		case "email":
			cfg := a.activeGatewaySettings()
			emailGateway := a.activeEmailGateway()
			status := "not configured"
			if emailGateway.Enabled() {
				status = "configured"
			}
			page := `<html><body><h1>Email Gateway Door</h1>` + gatewayNav + messageBlock +
				`<p>Outbound relay sends from <code>handle@from-domain</code> through your configured SMTP host.</p>` +
				`<table border="1"><tr><th>Check</th><th>Value</th></tr>` +
				`<tr><td>Relay status</td><td>` + htmlEscape(status) + `</td></tr>` +
				`<tr><td>SMTP host</td><td>` + htmlEscape(defaultIfBlank(cfg.SMTPHost, "not set")) + `:` + strconv.Itoa(cfg.SMTPPort) + `</td></tr>` +
				`<tr><td>From domain</td><td>` + htmlEscape(defaultIfBlank(cfg.FromDomain, "not set")) + `</td></tr>` +
				`<tr><td>Max recipients</td><td>` + strconv.Itoa(cfg.MaxRecipients) + `</td></tr>` +
				`<tr><td>Max message bytes</td><td>` + strconv.Itoa(cfg.MaxMessageBytes) + `</td></tr>` +
				`<tr><td>Verified required</td><td>` + boolToText(a.requireVerifiedEmail) + `</td></tr></table>` +
				`<p><a href="/mail">Open private mail compose</a> | <a href="/admin/gateways">Configure gateway controls</a> | <a href="/admin/mail">Check outbound policy</a></p>` +
				`</body></html>`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(page))
			return
		default:
			page := `<html><body><h1>Gateway Hub</h1>` + gatewayNav + messageBlock +
				`<p>Modern internet utility doors with classic BBS flow.</p>` +
				`<section class="wolfbbs-grid">` +
				`<article class="wolfbbs-card"><h2>Text Web Browser</h2><p>Read web pages as clean text.</p><p><a href="/gateway?view=browser">Open browser door</a></p></article>` +
				`<article class="wolfbbs-card"><h2>Email Gateway</h2><p>Check SMTP relay status and mail policy.</p><p><a href="/gateway?view=email">Open email door</a></p></article>` +
				`<article class="wolfbbs-card"><h2>Generative AI</h2><p>Prompt an AI model from inside the board.</p><p><a href="/gateway?view=ai">Open AI door</a></p></article>` +
				`<article class="wolfbbs-card"><h2>Feed Reader</h2><p>Pull RSS/Atom feeds into compact lists.</p><p><a href="/gateway?view=rss">Open feed door</a></p></article>` +
				`<article class="wolfbbs-card"><h2>Article Summarizer</h2><p>Get quick bullets before deep reading.</p><p><a href="/gateway?view=summarize">Open summarizer door</a></p></article>` +
				`<article class="wolfbbs-card"><h2>JSON Explorer</h2><p>Inspect JSON APIs safely.</p><p><a href="/gateway?view=json">Open JSON door</a></p></article>` +
				`</section>` +
				`<h2>Quick Fetch</h2><form method="POST" action="/gateway">` + csrf + `<input type="hidden" name="action" value="fetch"><label>URL <input name="url" size="72" value="https://"></label><label><input type="checkbox" name="save" value="1"> Save offline copy</label><button type="submit">Fetch</button></form>` +
				`<p><a href="/gateway?view=files">FileBase browser + queue + temp links</a></p>` +
				`</body></html>`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(page))
			return
		}
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !a.requireCSRF(w, r) {
		return
	}
	action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
	if isGatewayFileAction(action) {
		if !a.canReadFiles(user, "manage") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if a.handleGatewayFileAction(w, r, user) {
			return
		}
	}
	fetchCfg := a.activeWebFetchConfig()
	switch action {
	case "", "fetch", "browser_fetch":
		targetURL := strings.TrimSpace(r.FormValue("url"))
		if targetURL == "" {
			targetURL = strings.TrimSpace(r.FormValue("browser_url"))
		}
		if targetURL == "" {
			redirectWithError(w, r, "/gateway?view=browser", "Enter a URL before fetching.")
			return
		}
		result, err := gateway.FetchText(r.Context(), targetURL, fetchCfg)
		if err != nil {
			redirectWithError(w, r, "/gateway?view=browser", "Web gateway fetch failed: "+err.Error())
			return
		}
		note := ""
		if strings.TrimSpace(r.FormValue("save")) == "1" {
			path, err := gateway.SaveOffline(a.offlineDir, user.Handle, targetURL, result)
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
		_, _ = w.Write([]byte(`<html><body><h1>Gateway Reader</h1>` + gatewayNav + `<p><a href="/gateway?view=browser">back to browser</a> | <a href="/gateway?view=summarize&url=` + url.QueryEscape(targetURL) + `">summarize</a> | <a href="/help">help</a></p><pre>` + escaped + `</pre></body></html>`))
		return
	case "rss_fetch":
		feedURL := strings.TrimSpace(r.FormValue("feed_url"))
		if feedURL == "" {
			redirectWithError(w, r, "/gateway?view=rss", "Feed URL is required.")
			return
		}
		limit := parseInt(r.FormValue("limit"), 10)
		if limit <= 0 {
			limit = 10
		}
		if limit > 25 {
			limit = 25
		}
		feed, err := gateway.FetchFeed(r.Context(), feedURL, fetchCfg, limit)
		if err != nil {
			redirectWithError(w, r, "/gateway?view=rss", "Feed fetch failed: "+err.Error())
			return
		}
		rows := strings.Builder{}
		for _, item := range feed.Items {
			title := defaultIfBlank(strings.TrimSpace(item.Title), "untitled")
			link := strings.TrimSpace(item.Link)
			published := strings.TrimSpace(item.Published)
			if published == "" {
				published = "n/a"
			}
			summary := strings.TrimSpace(item.Summary)
			if summary == "" {
				summary = "No summary."
			}
			rows.WriteString(`<li><strong>` + htmlEscape(title) + `</strong> <span class="wolfbbs-muted">` + htmlEscape(published) + `</span><br>`)
			if link != "" {
				rows.WriteString(`<a href="` + htmlEscape(link) + `">` + htmlEscape(link) + `</a><br>`)
			}
			rows.WriteString(htmlEscape(summary) + `</li>`)
		}
		if rows.Len() == 0 {
			rows.WriteString(`<li>No feed items returned.</li>`)
		}
		page := `<html><body><h1>Feed Reader Results</h1>` + gatewayNav +
			`<p><a href="/gateway?view=rss">back to feed door</a> | <a href="/gateway?view=browser">text browser</a></p>` +
			`<p><strong>Feed:</strong> ` + htmlEscape(defaultIfBlank(feed.Title, feedURL)) + `</p><ol>` + rows.String() + `</ol></body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page))
		return
	case "summarize_fetch":
		articleURL := strings.TrimSpace(r.FormValue("article_url"))
		if articleURL == "" {
			redirectWithError(w, r, "/gateway?view=summarize", "Article URL is required.")
			return
		}
		maxBullets := parseInt(r.FormValue("max_bullets"), 5)
		if maxBullets <= 0 {
			maxBullets = 5
		}
		if maxBullets > 12 {
			maxBullets = 12
		}
		summary, err := gateway.SummarizeURL(r.Context(), articleURL, fetchCfg, maxBullets)
		if err != nil {
			redirectWithError(w, r, "/gateway?view=summarize", "Summarizer failed: "+err.Error())
			return
		}
		bullets := strings.Builder{}
		for _, row := range summary.Bullets {
			bullets.WriteString(`<li>` + htmlEscape(row) + `</li>`)
		}
		page := `<html><body><h1>Summary</h1>` + gatewayNav +
			`<p><a href="/gateway?view=summarize">back to summarizer</a> | <a href="/gateway?view=browser">text browser</a></p>` +
			`<p><strong>Source:</strong> <a href="` + htmlEscape(articleURL) + `">` + htmlEscape(articleURL) + `</a><br><strong>Title guess:</strong> ` + htmlEscape(summary.Title) + `<br><strong>Word count:</strong> ` + strconv.Itoa(summary.WordCount) + `</p>` +
			`<h2>Key bullets</h2><ul>` + bullets.String() + `</ul><h3>Excerpt</h3><pre>` + htmlEscape(summary.Excerpt) + `</pre></body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page))
		return
	case "json_fetch":
		jsonURL := strings.TrimSpace(r.FormValue("json_url"))
		if jsonURL == "" {
			redirectWithError(w, r, "/gateway?view=json", "JSON URL is required.")
			return
		}
		pretty, err := gateway.FetchJSON(r.Context(), jsonURL, fetchCfg)
		if err != nil {
			redirectWithError(w, r, "/gateway?view=json", "JSON fetch failed: "+err.Error())
			return
		}
		page := `<html><body><h1>JSON Explorer Result</h1>` + gatewayNav +
			`<p><a href="/gateway?view=json">back to JSON door</a> | <a href="/gateway?view=browser">text browser</a></p>` +
			`<p><strong>Source:</strong> <a href="` + htmlEscape(jsonURL) + `">` + htmlEscape(jsonURL) + `</a></p><pre>` + htmlEscape(pretty) + `</pre></body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page))
		return
	case "ai_prompt":
		prompt := strings.TrimSpace(r.FormValue("prompt"))
		if prompt == "" {
			redirectWithError(w, r, "/gateway?view=ai", "Prompt is required.")
			return
		}
		if len([]rune(prompt)) > maxAIGatewayPromptChars {
			redirectWithError(w, r, "/gateway?view=ai", fmt.Sprintf("Prompt exceeds %d characters.", maxAIGatewayPromptChars))
			return
		}
		aiCfg := a.loadAIGatewaySettings()
		if !aiCfg.Enabled {
			redirectWithError(w, r, "/gateway?view=ai", "AI gateway is disabled. Enable it in /admin/gateways.")
			return
		}
		client := gateway.NewAIClient(gateway.AIConfig{
			BaseURL:      aiCfg.BaseURL,
			AllowPrivate: allowPrivateAIGatewayBaseURLs(),
			APIKey:       aiCfg.APIKey,
			Model:        aiCfg.Model,
			SystemPrompt: aiCfg.SystemPrompt,
			Timeout:      time.Duration(aiCfg.TimeoutSec) * time.Second,
			MaxTokens:    aiCfg.MaxTokens,
		})
		reply, err := client.Complete(r.Context(), prompt)
		if err != nil {
			redirectWithError(w, r, "/gateway?view=ai", "AI request failed: "+err.Error())
			return
		}
		page := `<html><body><h1>AI Door Reply</h1>` + gatewayNav +
			`<p><a href="/gateway?view=ai">back to AI door</a> | <a href="/gateway?view=summarize">summarizer</a></p>` +
			`<p><strong>Prompt</strong></p><pre>` + htmlEscape(prompt) + `</pre>` +
			`<p><strong>Reply [AI-LABEL]</strong></p><pre>` + htmlEscape(reply) + `</pre></body></html>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(page))
		return
	default:
		redirectWithError(w, r, "/gateway", "Unsupported gateway action.")
		return
	}
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
		case "update_digest":
			pref := digestPreferences{
				Enabled:          parseCheckbox(r.FormValue("digest_enabled")),
				MaxItems:         parseInt(r.FormValue("digest_max_items"), defaultDigestPreferences().MaxItems),
				IncludeEvents:    parseCheckbox(r.FormValue("digest_include_events")),
				IncludeBoards:    parseCheckbox(r.FormValue("digest_include_boards")),
				WeeklyMail:       parseCheckbox(r.FormValue("digest_weekly_mail")),
				AttentionCadence: r.FormValue("digest_attention_cadence"),
				BulletinCadence:  r.FormValue("digest_bulletin_cadence"),
				EventCadence:     r.FormValue("digest_event_cadence"),
			}
			a.persistDigestPreferences(user.Handle, pref)
			notice = "Digest preferences saved."
		case "update_profile":
			row := publicProfileSettings{
				StatusLine:     strings.TrimSpace(r.FormValue("status_line")),
				Bio:            strings.TrimSpace(r.FormValue("bio")),
				ShowStatusLine: parseCheckbox(r.FormValue("show_status_line")),
				ShowBio:        parseCheckbox(r.FormValue("show_bio")),
				ShowContact:    parseCheckbox(r.FormValue("show_contact")),
			}
			for _, key := range []string{"contact_mail", "contact_page", "contact_chat"} {
				if parseCheckbox(r.FormValue(key)) {
					row.ContactPrefs = append(row.ContactPrefs, strings.TrimPrefix(key, "contact_"))
				}
			}
			a.persistPublicProfileSettings(user.Handle, row)
			notice = "Profile card preferences saved."
		case "apply_attention_preset":
			preset, ok := attentionPresetByName(r.FormValue("preset"))
			if !ok {
				redirectWithError(w, r, "/settings", "Attention preset not found.")
				return
			}
			a.persistDigestPreferences(user.Handle, preset.Pref)
			notice = "Applied " + preset.Label + " attention preset."
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
	digestPref := a.loadDigestPreferences(user.Handle)
	profileSettings := a.loadPublicProfileSettings(user.Handle)
	recommendedPreset := recommendedAttentionPreset(user)
	currentPresetName := currentAttentionPresetName(digestPref)
	currentPresetText := "Current rules are custom."
	if currentPresetName != "" {
		if preset, ok := attentionPresetByName(currentPresetName); ok {
			currentPresetText = "Current rules match the " + preset.Label + " preset."
		}
	}
	digestMaxItems := normalizeDigestPreferences(digestPref).MaxItems
	var digestOptions strings.Builder
	for _, value := range []int{6, 10, 12, 16, 20, 24} {
		selected := ""
		if value == digestMaxItems {
			selected = ` selected`
		}
		digestOptions.WriteString(`<option value="` + strconv.Itoa(value) + `"` + selected + `>` + strconv.Itoa(value) + ` items</option>`)
	}
	var secondFactorBlock strings.Builder
	var presetCards strings.Builder
	for _, preset := range attentionPresetCatalog() {
		statusBits := []string{summarizeDigestPreferences(preset.Pref)}
		if preset.Name == recommendedPreset.Name {
			statusBits = append(statusBits, "recommended for your role")
		}
		if preset.Name == currentPresetName {
			statusBits = append(statusBits, "current match")
		}
		presetCards.WriteString(`<article class="wolfbbs-helper-card"><strong>` + htmlEscape(preset.Label) + `</strong><p>` + htmlEscape(preset.Detail) + `</p><p class="wolfbbs-muted">` + htmlEscape(strings.Join(statusBits, " | ")) + `</p><form method="POST" action="/settings"><input type="hidden" name="action" value="apply_attention_preset"><input type="hidden" name="preset" value="` + htmlEscape(preset.Name) + `">` + csrf + `<button type="submit">Apply ` + htmlEscape(preset.Label) + `</button></form></article>`)
	}
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
		`<li>Web digest: ` + boolToText(digestPref.Enabled) + `</li>` +
		`<li>Profile export: <a href="/profile/export">download JSON</a></li>` +
		`<li>Circles: <a href="/circles">manage caller circles</a></li>` +
		`</ul>` +
		`<h2>Display Preferences</h2><form method="POST" action="/settings"><input type="hidden" name="action" value="update_prefs">` + csrf +
		`<label>Theme: <select name="theme">` + themeOptions + `</select></label><br>` +
		`<label><input type="checkbox" name="ansi_enabled" value="1" ` + checkedAttr(user.ANSIEnabled) + `> ANSI enabled</label><br>` +
		`<label><input type="checkbox" name="paging_enabled" value="1" ` + checkedAttr(user.PagingEnabled) + `> Paging enabled</label><br>` +
		`<label><input type="checkbox" name="time_format_24h" value="1" ` + checkedAttr(user.TimeFormat24h) + `> 24-hour time format</label><br>` +
		`<label>Home route <select name="home_route">` + homeRouteOptionRows(homeRoute) + `</select></label><br>` +
		`<button type="submit">Save Preferences</button></form>` +
		`<h2>Daily Digest</h2><form method="POST" action="/settings"><input type="hidden" name="action" value="update_digest">` + csrf +
		`<label><input type="checkbox" name="digest_enabled" value="1" ` + checkedAttr(digestPref.Enabled) + `> Enable low-noise web digest</label><br>` +
		`<label>Digest size <select name="digest_max_items">` + digestOptions.String() + `</select></label><br>` +
		`<label><input type="checkbox" name="digest_include_events" value="1" ` + checkedAttr(digestPref.IncludeEvents) + `> Include scheduled events</label><br>` +
		`<label><input type="checkbox" name="digest_include_boards" value="1" ` + checkedAttr(digestPref.IncludeBoards) + `> Include digest-tier board pulse</label><br>` +
		`<label><input type="checkbox" name="digest_weekly_mail" value="1" ` + checkedAttr(digestPref.WeeklyMail) + `> Send weekly digest by internal mail</label><br>` +
		`<label>Attention cadence <select name="digest_attention_cadence">` + digestCadenceOptionRows(digestPref.AttentionCadence) + `</select></label><br>` +
		`<label>Bulletin cadence <select name="digest_bulletin_cadence">` + digestCadenceOptionRows(digestPref.BulletinCadence) + `</select></label><br>` +
		`<label>Event cadence <select name="digest_event_cadence">` + digestCadenceOptionRows(digestPref.EventCadence) + `</select></label><br>` +
		`<button type="submit">Save Digest Preferences</button></form><p><a href="/digest">Open Daily Digest</a> | <a href="/digest/preferences">Weekday digest tuning</a> | <a href="/attention/export">Export notification state JSON</a></p>` +
		`<h2>Public Profile Card</h2><form method="POST" action="/settings"><input type="hidden" name="action" value="update_profile">` + csrf +
		`<label>Status line <input name="status_line" size="72" value="` + htmlEscape(profileSettings.StatusLine) + `" placeholder="night owl, door fiend, building cool stuff"></label><br>` +
		`<label>Bio<br><textarea name="bio" rows="6" cols="80" placeholder="Short caller bio for the directory card.">` + htmlEscape(profileSettings.Bio) + `</textarea></label><br>` +
		`<label><input type="checkbox" name="show_status_line" value="1" ` + checkedAttr(profileSettings.ShowStatusLine) + `> show status line</label><br>` +
		`<label><input type="checkbox" name="show_bio" value="1" ` + checkedAttr(profileSettings.ShowBio) + `> show bio</label><br>` +
		`<label><input type="checkbox" name="show_contact" value="1" ` + checkedAttr(profileSettings.ShowContact) + `> show verified contact preferences</label><br>` +
		`<fieldset><legend>Verified contact preferences</legend>` +
		`<label><input type="checkbox" name="contact_mail" value="1" ` + checkedAttr(containsString(profileSettings.ContactPrefs, "mail")) + `> prefer mail</label><br>` +
		`<label><input type="checkbox" name="contact_page" value="1" ` + checkedAttr(containsString(profileSettings.ContactPrefs, "page")) + `> page only when urgent</label><br>` +
		`<label><input type="checkbox" name="contact_chat" value="1" ` + checkedAttr(containsString(profileSettings.ContactPrefs, "chat")) + `> chat when live is fine</label>` +
		`</fieldset><button type="submit">Save Profile Card</button></form><p><a href="/directory?handle=` + url.QueryEscape(user.Handle) + `">Preview my caller card</a> | <a href="/profile/export">Export profile JSON</a></p>` +
		`<h2>Attention Rule Presets</h2><p>` + htmlEscape(currentPresetText) + ` Recommended preset for your role: <strong>` + htmlEscape(recommendedPreset.Label) + `</strong>.</p><section class="wolfbbs-helper-grid">` + presetCards.String() + `</section>` +
		`<h2>Password</h2><form method="POST" action="/settings"><input type="hidden" name="action" value="change_password">` + csrf +
		`<label>New password: <input name="password" type="password"></label><br>` +
		`<label>Confirm: <input name="confirm" type="password"></label><br><button type="submit">Change password</button></form>` +
		adminSettingsBlock +
		secondFactorBlock.String() +
		`</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleProfileExport(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	payload := map[string]interface{}{
		"generated_at":       time.Now().UTC().Format(time.RFC3339Nano),
		"handle":             user.Handle,
		"role":               rbac.NormalizeRole(user.Role),
		"profile_card":       a.loadPublicProfileSettings(user.Handle),
		"contact_aliases":    a.loadContactAliases(user.Handle),
		"caller_circles":     a.loadCallerCircles(user.Handle),
		"digest_preferences": a.loadDigestPreferences(user.Handle),
		"home_route":         a.preferredHomeRoute(user),
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"wolfbbs-profile-%s.json\"", normalizeHandleKey(user.Handle)))
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		a.addAppError("profile.export", fmt.Errorf("encode profile export for %s: %w", user.Handle, err))
	}
}

func (a *webApp) handleCircles(w http.ResponseWriter, r *http.Request) {
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
		switch action {
		case "save_circle":
			name := strings.TrimSpace(r.FormValue("name"))
			if name == "" {
				redirectWithError(w, r, "/circles", "Circle name is required.")
				return
			}
			members := splitTrimmedList(r.FormValue("members"), ",", maxFavoriteCallers)
			a.upsertCallerCircle(user.Handle, strings.TrimSpace(r.FormValue("circle_id")), name, strings.TrimSpace(r.FormValue("note")), members)
			redirectWithNotice(w, r, "/circles", "Circle saved.")
			return
		case "delete_circle":
			a.deleteCallerCircle(user.Handle, r.FormValue("circle_id"))
			redirectWithNotice(w, r, "/circles", "Circle removed.")
			return
		default:
			redirectWithError(w, r, "/circles", "Unsupported circle action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rows := a.loadCallerCircles(user.Handle)
	editID := strings.TrimSpace(r.URL.Query().Get("edit"))
	form := callerCircle{}
	formAction := "save_circle"
	formTitle := "Create Circle"
	submitLabel := "Save Circle"
	if editID != "" {
		for _, row := range rows {
			if row.ID == editID {
				form = row
				formTitle = "Edit Circle"
				submitLabel = "Update Circle"
				break
			}
		}
	}
	listRows := strings.Builder{}
	for _, row := range rows {
		memberText := htmlEscape(strings.Join(row.Members, ", "))
		if strings.TrimSpace(memberText) == "" {
			memberText = `<span class="wolfbbs-muted">no members yet</span>`
		}
		listRows.WriteString(`<tr><td>` + htmlEscape(row.Name) + `</td><td>` + memberText + `</td><td>` + htmlEscape(defaultIfBlank(row.Note, "n/a")) + `</td><td><a href="/circles?edit=` + url.QueryEscape(row.ID) + `">edit</a> <form method="POST" action="/circles" style="display:inline">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="delete_circle"><input type="hidden" name="circle_id" value="` + htmlEscape(row.ID) + `"><button type="submit">delete</button></form></td></tr>`)
	}
	if listRows.Len() == 0 {
		listRows.WriteString(`<tr><td colspan="4">No circles yet. Use one for family, door rivals, or your favorite regulars.</td></tr>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Caller Circles</title></head><body>
<p><a href="/settings">settings</a> | <a href="/directory">directory</a> | <a href="/mail">mail</a> | <a href="/events">events</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Caller Circles</h1>
<p>Build small personal groups for the people you message, invite, and follow most often.</p>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>` + formTitle + `</h2><form method="POST" action="/circles">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="` + formAction + `"><input type="hidden" name="circle_id" value="` + htmlEscape(form.ID) + `"><label>Name <input name="name" size="32" value="` + htmlEscape(form.Name) + `" placeholder="Door Crew"></label><br><label>Members (comma-separated handles)<br><textarea name="members" rows="5" cols="64" placeholder="alice, bob, sysop">` + htmlEscape(strings.Join(form.Members, ", ")) + `</textarea></label><br><label>Note <input name="note" size="56" value="` + htmlEscape(form.Note) + `" placeholder="Who belongs here and why"></label><br><button type="submit">` + submitLabel + `</button></form></article><article class="wolfbbs-card"><h2>How to use circles</h2><ul class="wolfbbs-list-clean"><li>Keep a shortlist for invites before events.</li><li>Use circles to remember groups beyond one-off favorite callers.</li><li>Pair aliasing in the directory with circles here for better relationship memory.</li></ul></article></section>
<table border="1"><tr><th>Name</th><th>Members</th><th>Note</th><th>Action</th></tr>` + listRows.String() + `</table>
</body></html>`
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
		case "goal_progress":
			goalID := strings.TrimSpace(r.FormValue("goal_id"))
			delta := parseIntWithFallback(r.FormValue("delta"), 1)
			if !a.addGoalContribution(goalID, user.Handle, delta) {
				redirectWithError(w, r, "/clubhouse", "Goal contribution failed.")
				return
			}
			redirectWithNotice(w, r, "/clubhouse", "Goal contribution saved.")
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
	goals := a.loadClubhouseGoals()
	goalsCompleted := 0
	goalRows := strings.Builder{}
	for _, row := range goals {
		if row.Progress >= row.Target {
			goalsCompleted++
		}
		progressPercent := 0
		if row.Target > 0 {
			progressPercent = (row.Progress * 100) / row.Target
		}
		bindingParts := make([]string, 0, 2)
		if row.BoardID > 0 {
			if boardName := a.boardNameByID(row.BoardID); boardName != "" {
				bindingParts = append(bindingParts, `board <a href="/boards?board=`+strconv.FormatInt(row.BoardID, 10)+`">`+htmlEscape(boardName)+`</a>`)
			}
		}
		if row.DoorID != "" {
			if doorName := a.doorNameByID(row.DoorID); doorName != "" {
				bindingParts = append(bindingParts, `door <a href="/scores?door=`+htmlEscape(row.DoorID)+`">`+htmlEscape(doorName)+`</a>`)
			}
		}
		bindingLine := "no board/door binding"
		if len(bindingParts) > 0 {
			bindingLine = strings.Join(bindingParts, " | ")
		}
		goalRows.WriteString(`<li><strong>` + htmlEscape(row.Title) + `</strong> <span class="wolfbbs-muted">` + strconv.Itoa(row.Progress) + `/` + strconv.Itoa(row.Target) + ` (` + strconv.Itoa(progressPercent) + `%)</span><br><span class="wolfbbs-muted">` + bindingLine + ` | updated by ` + htmlEscape(defaultIfBlank(row.UpdatedBy, "staff")) + `</span><br><form method="POST" action="/clubhouse" class="wolfbbs-inline-form"><input type="hidden" name="action" value="goal_progress"><input type="hidden" name="goal_id" value="` + htmlEscape(row.ID) + `">` + csrf + `<label>Add progress <input name="delta" value="1" inputmode="numeric" size="4"></label><button type="submit">Contribute</button></form></li>`)
	}
	if goalRows.Len() == 0 {
		goalRows.WriteString(`<li>No shared goals yet. Ask staff to define a season in <a href="/admin/challenges">Challenges Admin</a>.</li>`)
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Clubhouse</title></head><body>
<p><a href="/boards">boards</a> | <a href="/radar">radar</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/doors">doors</a> | <a href="/scores">scores</a> | <a href="/challenges">challenges</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Clubhouse</h1>
<p>The social layer: post one-liners, browse the BBS exchange, check who's hanging around, and catch the latest rumor.</p>
<section class="wolfbbs-kpi-grid">
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(oneLiners)) + `</strong><span>one-liners</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(bbsList)) + `</strong><span>bbs exchange links</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(onlineUsers) + `</strong><span>chat presences</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(recentCallers)) + `</strong><span>recent callers</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(goals)) + `</strong><span>shared goals</span></article>
<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(goalsCompleted) + `</strong><span>goals completed</span></article>
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
<h2>Shared Goals</h2>
<ul>` + goalRows.String() + `</ul>
<p><a href="/challenges">Open seasonal challenge board</a></p>
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
<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/radar">radar</a> | <a href="/clubhouse">clubhouse</a> | <a href="/doors">doors</a> | <a href="/gateway">gateway doors</a> | <a href="/scores">scores</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + messageBlock + `
<h1>Door Cockpit</h1>
<p>One place for favorites, recommendations, trophies, turn budgets, policy-aware door directory, and modern internet gateway doors.</p>
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
	gatewayConfigured := a.gatewayConfigured()
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
		adminLink = ` | <a href="/admin/ops">ops center</a> | <a href="/admin/system">sysop system</a> | <a href="/admin/upgrade-safety">upgrade safety</a> | <a href="/admin/backups">backup browser</a> | <a href="/admin/release">release dashboard</a>`
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
			`<li><a href="/admin/upgrade-safety">/admin/upgrade-safety</a> for change-risk checks before upgrades.</li>` +
			`<li><a href="/admin/backups">/admin/backups</a> to validate backup artifacts and offline packets.</li>` +
			`<li><a href="/admin/release">/admin/release</a> for roadmap/QA/doc artifact release coordination.</li>` +
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
			`<tr><td>Challenges + clubhouse goals</td><td><a href="/admin/challenges">/admin/challenges</a></td><td><a href="/challenges">/challenges</a></td></tr>` +
			`<tr><td>Gateway safety limits</td><td><a href="/admin/gateways">/admin/gateways</a></td><td><a href="/admin/system">/admin/system</a></td></tr>` +
			`<tr><td>Chat channels + moderation</td><td><a href="/admin/chat">/admin/chat</a></td><td><a href="/admin/system">/admin/system</a></td></tr>` +
			`<tr><td>Doors + turns + scores</td><td><a href="/admin/doors">/admin/doors</a></td><td><a href="/admin/system">/admin/system</a></td></tr>` +
			`<tr><td>Upgrade + backup safety</td><td><a href="/admin/upgrade-safety">/admin/upgrade-safety</a> / <a href="/admin/backups">/admin/backups</a></td><td><a href="/status">/status</a></td></tr>` +
			`<tr><td>Release cockpit</td><td><a href="/admin/release">/admin/release</a></td><td><a href="/status">/status</a></td></tr>` +
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
		`<li>Seasonal loops: <a href="/challenges">/challenges</a> and <a href="/events/recaps">/events/recaps</a></li>` +
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
	adminActionGrid := `<section class="wolfbbs-action-grid"><a class="wolfbbs-action-card" href="/admin/launch"><strong>Launch Center</strong><span>go-live verdict, next-best actions, operator commands</span></a><a class="wolfbbs-action-card" href="/admin/ops"><strong>Ops Center</strong><span>errors, sessions, audits, and triage</span></a><a class="wolfbbs-action-card" href="/admin/setup"><strong>Setup Wizard</strong><span>identity, safety, bootstrap, launch checklist</span></a><a class="wolfbbs-action-card" href="/admin/users"><strong>User Ops</strong><span>create callers, role changes, bans, resets</span></a><a class="wolfbbs-action-card" href="/admin/challenges"><strong>Challenges</strong><span>season engine + clubhouse shared goals</span></a><a class="wolfbbs-action-card" href="/admin/missions"><strong>Missions</strong><span>seasonal mission templates + completion loops</span></a><a class="wolfbbs-action-card" href="/admin/plugins"><strong>Plugins</strong><span>manifest/capability/sandbox contracts</span></a><a class="wolfbbs-action-card" href="/admin/themes"><strong>Themes</strong><span>marketplace import + apply workflow</span></a><a class="wolfbbs-action-card" href="/admin/webhooks"><strong>Webhooks</strong><span>external board event bridge with retries</span></a><a class="wolfbbs-action-card" href="/admin/analytics"><strong>Analytics</strong><span>daily/weekly/monthly product KPIs</span></a><a class="wolfbbs-action-card" href="/admin/upgrade-safety"><strong>Upgrade Safety</strong><span>change-risk checks before rollout</span></a><a class="wolfbbs-action-card" href="/admin/backups"><strong>Backup Browser</strong><span>artifact validation and recovery confidence</span></a><a class="wolfbbs-action-card" href="/admin/release"><strong>Release Dashboard</strong><span>roadmap, QA, docs, and artifact cockpit</span></a><a class="wolfbbs-action-card" href="/admin/system"><strong>System</strong><span>runtime health, service state, deeper operator detail</span></a></section>`
	page := `<html><body><h1>Sysop Control Panel</h1><p>Logged in as ` + user.Handle + `</p>` +
		`<p><a href="/admin/users">Users</a> | <a href="/admin/boards">Boards</a> | <a href="/admin/mail">Mail</a> | ` +
		`<a href="/admin/files">Files</a> | <a href="/admin/gateways">Gateways</a> | <a href="/admin/chat">Chat</a> | ` +
		`<a href="/admin/doors">Doors</a> | <a href="/admin/bulletins">Bulletins</a> | <a href="/admin/challenges">Challenges</a> | <a href="/admin/missions">Missions</a> | <a href="/admin/plugins">Plugins</a> | <a href="/admin/themes">Themes</a> | <a href="/admin/webhooks">Webhooks</a> | <a href="/admin/analytics">Analytics</a> | <a href="/admin/launch">Launch Center</a> | <a href="/admin/ops">Ops Center</a> | <a href="/admin/upgrade-safety">Upgrade Safety</a> | <a href="/admin/backups">Backups</a> | <a href="/admin/release">Release</a> | <a href="/admin/setup">Setup</a> | <a href="/admin/config">Config</a> | ` +
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
		`<li>Challenges + missions: seasonal scoring, shared goals, and completion tracks</li>` +
		`<li>Extension plane: plugin manifests, theme marketplace, and webhook bridge</li>` +
		`<li>Product analytics: daily/weekly/monthly KPI snapshots in admin analytics</li>` +
		`<li>Message networks: spool import/export via oputil + status in WFC</li>` +
		`<li>Built-in mods: onelinerz, rumorz, bbs list, who's online lifecycle</li>` +
		`<li>Gateway controls, setup checks, runtime config, and server health</li>` +
		`<li>Upgrade safety dashboard, backup browser, and release dashboard for release confidence</li></ul>` +
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
	firstRunBlock := a.renderSysopFirstRunBlock(user)
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Launch Center</title></head><body><h1>Launch Center</h1>` +
		`<p><a href="/admin">back</a> | <a href="/admin/ops">ops</a> | <a href="/admin/setup">setup</a> | <a href="/admin/challenges">challenges</a> | <a href="/admin/upgrade-safety">upgrade safety</a> | <a href="/admin/backups">backups</a> | <a href="/admin/release">release</a> | <a href="/admin/config">config</a> | <a href="/admin/system">system</a> | <a href="/status">status</a> | <a href="/help">help</a></p>` +
		pageMessageBlock(r) +
		`<p>Use this page as the sysop home base for first-run, pre-launch review, and support triage.</p>` +
		launchHelperBlock +
		firstRunBlock +
		`<h2>Launch Summary</h2>` +
		`<p><strong>Verdict:</strong> ` + htmlEscape(launchVerdictText(readiness)) + ` | ` + strconv.Itoa(readiness.Summary.Pass) + `/` + strconv.Itoa(readiness.Summary.Total) + ` launch checks PASS | runtime ` + strconv.Itoa(runtime.Summary.Warn) + ` WARN</p>` +
		`<p><a href="/admin/setup">Setup Wizard</a> | <a href="/admin/users">Create Caller</a> | <a href="/admin/challenges">Challenges</a> | <a href="/boards">Walk Boards</a> | <a href="/chat">Walk Chat</a> | <a href="/doors">Walk Doors</a></p>` +
		`<h2>Go-Live Checkpoints</h2><p><strong>` + strconv.Itoa(checkpointDone) + `/` + strconv.Itoa(len(checkpoints)) + `</strong> operator checkpoints complete.</p><table border="1"><tr><th>Checkpoint</th><th>Done</th><th>Action</th></tr>` + checkpointRows.String() + `</table>` +
		`<h2>Launch Checks</h2><table border="1"><tr><th>Check</th><th>Status</th><th>Details</th></tr>` + readinessRows.String() + `</table>` +
		`<h2>Runtime Checks</h2><table border="1"><tr><th>Check</th><th>Status</th><th>Details</th></tr>` + runtimeRows.String() + `</table>` +
		`<h2>Next Best Actions</h2><ul>` + launchActionRows.String() + `</ul>` +
		`<h2>Run In This Order</h2><ol>` +
		`<li><a href="/admin/setup">/admin/setup</a> for identity, safety, and bootstrap actions.</li>` +
		`<li><a href="/admin/upgrade-safety">/admin/upgrade-safety</a>, <a href="/admin/backups">/admin/backups</a>, and <a href="/admin/release">/admin/release</a> before rollout windows.</li>` +
		`<li><a href="/admin/config">/admin/config</a> for runtime flags, services, and public-facing behavior.</li>` +
		`<li><a href="/admin/users">/admin/users</a> and <a href="/admin/challenges">/admin/challenges</a> to create real caller loops.</li>` +
		`<li><a href="/boards">/boards</a>, <a href="/chat">/chat</a>, <a href="/doors">/doors</a>, <a href="/scores">/scores</a>, and <a href="/challenges">/challenges</a> as a real user.</li>` +
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
	gatewayConfigured := a.gatewayConfigured()

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
	firstRunBlock := a.renderSysopFirstRunBlock(user)
	page := `<html><body><h1>Setup & Install</h1><p><a href="/admin">back</a> | <a href="/admin/launch">launch</a> | <a href="/admin/system">system</a> | <a href="/help">help</a></p>` +
		`<p>Use this screen to verify base services and bootstrap sysop dependencies after install/upgrade.</p>` +
		`<p>UI-first setup: keep installer flags minimal; set board identity and runtime policy here.</p>` +
		`<h2>Setup Wizard</h2>` +
		`<p>` + htmlEscape(wizardHint) + `</p>` +
		progress +
		setupHelperBlock +
		firstRunBlock +
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
			_ = a.queueAdminCredentialReceipt(r, adminCredentialReceipt{
				Action:   "create",
				Handle:   created.Handle,
				Password: password,
			})
			redirectWithNotice(w, r, "/admin/users", "Created user "+created.Handle+".")
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
			_ = a.queueAdminCredentialReceipt(r, adminCredentialReceipt{
				Action:   "reset",
				Handle:   target,
				Password: pw,
			})
			redirectWithNotice(w, r, "/admin/users", "Reset password for "+target+".")
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
	messageBlock := pageMessageBlock(r) + renderAdminCredentialReceiptBlock(a.popAdminCredentialReceipts(r))
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
		messageBlock +
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
			board := &domain.Board{
				Name:        title,
				Description: desc,
				Conference:  conference,
				ReadACS:     readACS,
				WriteACS:    writeACS,
				CreatedBy:   user.ID,
			}
			if err := a.boardRepo.Create(board); err != nil {
				a.addAppError("admin.boards", fmt.Errorf("create board %s: %w", title, err))
			} else {
				a.recordAdminAction(user.Handle, title, "create_board", "")
				if a.eventBus != nil {
					a.eventBus.Publish("board.created", map[string]string{
						"board_id":   strconv.FormatInt(board.ID, 10),
						"board":      board.Name,
						"conference": defaultConferenceValue(board.Conference),
						"actor":      user.Handle,
					})
				}
			}
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
					if a.eventBus != nil {
						a.eventBus.Publish("board.deleted", map[string]string{
							"board_id": strconv.FormatInt(boardID, 10),
							"board":    target,
							"actor":    user.Handle,
						})
					}
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
						if a.eventBus != nil {
							a.eventBus.Publish("board.updated", map[string]string{
								"board_id":   strconv.FormatInt(board.ID, 10),
								"board":      board.Name,
								"conference": defaultConferenceValue(board.Conference),
								"actor":      user.Handle,
							})
						}
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
		case "set_thread_state":
			threadID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("thread_id")), 10, 64)
			state := normalizeThreadLifecycleState(r.FormValue("thread_state"))
			if threadID > 0 {
				a.setThreadLifecycleState(threadID, state)
				a.recordAdminAction(user.Handle, "thread #"+strconv.FormatInt(threadID, 10), "set_thread_state", string(state))
			}
		case "lock_thread", "unlock_thread":
			threadID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("thread_id")), 10, 64)
			if threadID > 0 {
				locked := action == "lock_thread"
				if err := a.msgRepo.SetThreadLocked(threadID, locked); err != nil {
					a.addAppError("admin.boards", fmt.Errorf("set thread lock %d: %w", threadID, err))
				} else {
					if locked {
						a.setThreadLifecycleState(threadID, threadLifecycleFrozen)
					} else {
						a.setThreadLifecycleState(threadID, threadLifecycleActive)
					}
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
		case "resolve_report_macro":
			reportID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("report_id")), 10, 64)
			macro := strings.ToLower(strings.TrimSpace(r.FormValue("macro")))
			toBoardID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("to_board_id")), 10, 64)
			if reportID > 0 {
				allReports, _ := a.msgRepo.ListReports(500, "")
				var report *domain.MessageReport
				for idx := range allReports {
					if allReports[idx].ID == reportID {
						report = &allReports[idx]
						break
					}
				}
				if report == nil {
					a.addAppError("admin.boards", fmt.Errorf("resolve macro report %d: report not found", reportID))
					break
				}
				msg, err := a.msgRepo.GetMessage(report.MessageID)
				if err != nil || msg == nil {
					a.addAppError("admin.boards", fmt.Errorf("resolve macro report %d: message not found", reportID))
					break
				}
				threadID := messageThreadID(*msg)
				detail := ""
				switch macro {
				case "spam_delete":
					if err := a.msgRepo.DeleteMessage(msg.ID); err != nil {
						a.addAppError("admin.boards", fmt.Errorf("macro delete message %d: %w", msg.ID, err))
						break
					}
					detail = "deleted message as spam"
				case "abuse_lock":
					a.setThreadLifecycleState(threadID, threadLifecycleFrozen)
					detail = "froze thread for abuse review"
				case "offtopic_move":
					if toBoardID <= 0 {
						a.addAppError("admin.boards", fmt.Errorf("macro move thread %d: destination board missing", threadID))
						break
					}
					if err := a.msgRepo.MoveThread(threadID, toBoardID); err != nil {
						a.addAppError("admin.boards", fmt.Errorf("macro move thread %d -> %d: %w", threadID, toBoardID, err))
						break
					}
					detail = "moved thread to board " + strconv.FormatInt(toBoardID, 10)
				default:
					detail = "resolved with clear note"
				}
				if err := a.msgRepo.ResolveReport(reportID, user.Handle, time.Now().UTC()); err != nil {
					a.addAppError("admin.boards", fmt.Errorf("resolve report %d after macro: %w", reportID, err))
				} else {
					a.recordAdminAction(user.Handle, "report #"+strconv.FormatInt(reportID, 10), "resolve_report_macro", macro+" / "+detail)
				}
			}
		case "save_board_welcome":
			boardID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("board_id")), 10, 64)
			if boardID > 0 {
				a.setBoardWelcomeKit(boardID, boardWelcomeKit{
					Intro:          strings.TrimSpace(r.FormValue("intro")),
					SeedPrompts:    splitTrimmedLines(r.FormValue("seed_prompts"), 6, 140),
					StarterThreads: splitTrimmedLines(r.FormValue("starter_threads"), 6, 140),
				})
				a.recordAdminAction(user.Handle, "board #"+strconv.FormatInt(boardID, 10), "save_board_welcome", "")
			}
		case "save_board_staff_note":
			boardID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("board_id")), 10, 64)
			if boardID > 0 {
				note := strings.TrimSpace(r.FormValue("staff_note"))
				a.setBoardStaffNote(boardID, boardStaffNote{
					Note:      note,
					UpdatedBy: user.Handle,
					UpdatedAt: time.Now().UTC(),
				})
				if parseCheckbox(r.FormValue("escalate")) && note != "" {
					target := "board:" + strconv.FormatInt(boardID, 10)
					if board, err := a.boardRepo.Get(boardID); err == nil && board != nil {
						target = "board:" + board.Name
					}
					a.queueStaffEscalation(user.Handle, target, note)
				}
				a.recordAdminAction(user.Handle, "board #"+strconv.FormatInt(boardID, 10), "save_board_staff_note", "")
			}
		case "save_board_stewards":
			boardID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("board_id")), 10, 64)
			if boardID > 0 {
				a.setBoardStewards(boardID, parseBoardStewards(r.FormValue("stewards")))
				a.recordAdminAction(user.Handle, "board #"+strconv.FormatInt(boardID, 10), "save_board_stewards", "")
			}
		case "save_best_of_week":
			a.persistBestOfWeekEntries(parseBestOfWeekEntries(r.FormValue("best_of_week"), user.Handle))
			a.recordAdminAction(user.Handle, "boards", "save_best_of_week", "")
		case "import_legacy_archive":
			boardID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("import_board_id")), 10, 64)
			board, err := a.boardRepo.Get(boardID)
			if err != nil || board == nil {
				redirectWithError(w, r, redirectTo, "Target board not found.")
				return
			}
			entries := parseLegacyArchive(r.FormValue("archive_blob"))
			if len(entries) == 0 {
				redirectWithError(w, r, redirectTo, "No legacy archive entries parsed.")
				return
			}
			imported := 0
			for _, entry := range entries {
				authorID := user.ID
				if entry.From != "" {
					if author, err := a.authSvc.GetUser(entry.From); err == nil && author != nil {
						authorID = author.ID
					}
				}
				body := strings.TrimSpace(entry.Body)
				meta := []string{}
				if entry.From != "" && authorID == user.ID {
					meta = append(meta, "Imported From: "+entry.From)
				}
				if !entry.Date.IsZero() {
					meta = append(meta, "Legacy Date: "+entry.Date.Local().Format("2006-01-02 15:04"))
				}
				if len(meta) > 0 {
					body = strings.Join(meta, "\n") + "\n\n" + body
				}
				if err := a.msgRepo.CreateMessage(&domain.Message{
					BoardID:   boardID,
					AuthorID:  authorID,
					Subject:   entry.Subject,
					Body:      body,
					CreatedAt: entry.Date,
				}); err != nil {
					a.addAppError("admin.boards", fmt.Errorf("import legacy archive into board %d: %w", boardID, err))
					continue
				}
				imported++
			}
			a.recordAdminAction(user.Handle, board.Name, "import_legacy_archive", fmt.Sprintf("imported=%d", imported))
			redirectWithNotice(w, r, redirectTo, fmt.Sprintf("Imported %d legacy message(s).", imported))
			return
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
	allBoardOptions := strings.Builder{}
	for _, b := range boards {
		boardNameByID[b.ID] = b.Name
		allBoardOptions.WriteString(`<option value="` + strconv.FormatInt(b.ID, 10) + `">` + htmlEscape(b.Name) + `</option>`)
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
	allReports, _ := a.msgRepo.ListReports(500, "")
	handleByID := a.userHandleLookup()
	reportRows := strings.Builder{}
	for _, row := range reports {
		reportBoard := int64(0)
		reportSubject := "(message unavailable)"
		hintText := ""
		if msg, msgErr := a.msgRepo.GetMessage(row.MessageID); msgErr == nil && msg != nil {
			reportBoard = msg.BoardID
			reportSubject = msg.Subject
			hintText = buildModerationHints([]domain.Message{*msg}, allReports)[msg.ID]
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
		messageCell := strconv.FormatInt(row.MessageID, 10)
		if reportBoard > 0 {
			messageCell = `<a href="/boards?board=` + strconv.FormatInt(reportBoard, 10) + `&id=` + strconv.FormatInt(row.MessageID, 10) + `">` + messageCell + `</a>`
		}
		reportRows.WriteString(`<tr><td>` + strconv.FormatInt(row.ID, 10) + `</td><td>` + messageCell + `</td><td>` + htmlEscape(boardName) + `</td><td>` + htmlEscape(reportSubject) + `</td><td>` + htmlEscape(reporter) + `</td><td>` + htmlEscape(row.Reason))
		if hintText != "" {
			reportRows.WriteString(`<br><span class="wolfbbs-muted">` + htmlEscape(hintText) + `</span>`)
		}
		reportRows.WriteString(`</td><td>` + htmlEscape(row.Status) + `</td><td>` + row.CreatedAt.Local().Format("2006-01-02 15:04") + `</td><td>`)
		if strings.EqualFold(row.Status, "resolved") {
			reportRows.WriteString(`resolved`)
		} else {
			reportRows.WriteString(`<form method="POST" action="/admin/boards">` + csrf +
				`<input type="hidden" name="action" value="resolve_report">` +
				`<input type="hidden" name="report_id" value="` + strconv.FormatInt(row.ID, 10) + `">` +
				`<input type="hidden" name="redirect_to" value="` + htmlEscape(redirectTo) + `">` +
				`<button type="submit">resolve</button></form>`)
			reportRows.WriteString(`<form method="POST" action="/admin/boards" class="wolfbbs-inline-form">` + csrf +
				`<input type="hidden" name="action" value="resolve_report_macro">` +
				`<input type="hidden" name="report_id" value="` + strconv.FormatInt(row.ID, 10) + `">` +
				`<input type="hidden" name="redirect_to" value="` + htmlEscape(redirectTo) + `">` +
				`<label>macro <select name="macro"><option value="spam_delete">spam delete</option><option value="abuse_lock">abuse freeze</option><option value="offtopic_move">off-topic move</option><option value="clear_note">clear note</option></select></label>` +
				`<label>board <select name="to_board_id">` + allBoardOptions.String() + `</select></label>` +
				`<button type="submit">run</button></form>`)
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
			moderationHints := buildModerationHints(msgs, allReports)
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
				threadState := a.threadLifecycleStateFor(messageThreadID(msg))
				subjectCell := `<a href="/boards?board=` + strconv.FormatInt(manageBoardID, 10) + `&id=` + strconv.FormatInt(msg.ID, 10) + `">` + htmlEscape(msg.Subject) + `</a>`
				hintText := moderationHints[msg.ID]
				manageRows.WriteString(`<tr><td>` + strconv.FormatInt(msg.ID, 10) + `</td><td>` + strconv.FormatInt(msg.ThreadID, 10) + `</td><td>` + htmlEscape(author) + `</td><td>` + subjectCell)
				if hintText != "" {
					manageRows.WriteString(`<br><span class="wolfbbs-muted">` + htmlEscape(hintText) + `</span>`)
				}
				if badge := threadLifecycleStatusPill(threadState); badge != "" {
					manageRows.WriteString(`<br>` + badge)
				}
				manageRows.WriteString(`</td><td>` + msg.CreatedAt.Local().Format("2006-01-02 15:04") + `</td><td>`)
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
				manageRows.WriteString(`<form method="POST" action="/admin/boards" class="wolfbbs-inline-form">` + csrf +
					`<input type="hidden" name="action" value="set_thread_state">` +
					`<input type="hidden" name="thread_id" value="` + strconv.FormatInt(msg.ThreadID, 10) + `">` +
					`<input type="hidden" name="redirect_to" value="` + htmlEscape(redirectTo) + `">` +
					`<select name="thread_state">` + threadLifecycleOptionRows(threadState) + `</select>` +
					`<button type="submit">state</button></form>`)
				manageRows.WriteString(`</td></tr>`)
			}
		}
	}
	if manageRows.Len() == 0 {
		manageRows.WriteString(`<tr><td colspan="6">Select a board to moderate posts and thread state.</td></tr>`)
	}

	manageBoardBlock := ``
	if manageBoardID > 0 {
		board, _ := a.boardRepo.Get(manageBoardID)
		boardName := "Board"
		if board != nil && strings.TrimSpace(board.Name) != "" {
			boardName = board.Name
		}
		welcomeKit := a.boardWelcomeKitFor(manageBoardID)
		staffNote := a.boardStaffNoteFor(manageBoardID)
		stewards := a.boardStewardsFor(manageBoardID)
		manageBoardBlock = `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Board Welcome Kit</h2><form method="POST" action="/admin/boards"><input type="hidden" name="action" value="save_board_welcome"><input type="hidden" name="board_id" value="` + strconv.FormatInt(manageBoardID, 10) + `"><input type="hidden" name="redirect_to" value="` + htmlEscape(redirectTo) + `">` + csrf + `<p><strong>` + htmlEscape(boardName) + `</strong></p><label>Intro<br><textarea name="intro" rows="4" cols="72" placeholder="Explain what belongs here and how a new caller should start.">` + htmlEscape(welcomeKit.Intro) + `</textarea></label><br><label>Seed prompts (one per line)<br><textarea name="seed_prompts" rows="5" cols="72" placeholder="Introduce yourself&#10;What should we build next?">` + htmlEscape(strings.Join(welcomeKit.SeedPrompts, "\n")) + `</textarea></label><br><label>Starter thread ideas (one per line)<br><textarea name="starter_threads" rows="4" cols="72" placeholder="Classic thread kickoff&#10;Frequently asked questions">` + htmlEscape(strings.Join(welcomeKit.StarterThreads, "\n")) + `</textarea></label><br><button type="submit">Save Welcome Kit</button></form></article><article class="wolfbbs-card"><h2>Board Staff Notes</h2><form method="POST" action="/admin/boards"><input type="hidden" name="action" value="save_board_staff_note"><input type="hidden" name="board_id" value="` + strconv.FormatInt(manageBoardID, 10) + `"><input type="hidden" name="redirect_to" value="` + htmlEscape(redirectTo) + `">` + csrf + `<label>Internal note<br><textarea name="staff_note" rows="6" cols="72" placeholder="Internal moderation context, launch notes, or escalation handoff only.">` + htmlEscape(staffNote.Note) + `</textarea></label><br><label><input type="checkbox" name="escalate" value="1"> Queue in staff escalation lane</label><br><button type="submit">Save Staff Note</button></form><p class="wolfbbs-muted">Last updated by ` + htmlEscape(defaultIfBlank(staffNote.UpdatedBy, "n/a")) + `.</p></article><article class="wolfbbs-card"><h2>Topic Stewards</h2><form method="POST" action="/admin/boards"><input type="hidden" name="action" value="save_board_stewards"><input type="hidden" name="board_id" value="` + strconv.FormatInt(manageBoardID, 10) + `"><input type="hidden" name="redirect_to" value="` + htmlEscape(redirectTo) + `">` + csrf + `<label>Stewards (handle | topic | note)<br><textarea name="stewards" rows="6" cols="72" placeholder="sysop | welcome lane | first-time caller questions&#10;mod1 | archives | keep classics organized">` + htmlEscape(renderBoardStewardsText(stewards)) + `</textarea></label><br><button type="submit">Save Stewards</button></form></article></section>`
	}

	bestOfBlock := `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Best of Week Editorial Lane</h2><form method="POST" action="/admin/boards"><input type="hidden" name="action" value="save_best_of_week"><input type="hidden" name="redirect_to" value="/admin/boards">` + csrf + `<label>Curated picks (message_id | optional note)<br><textarea name="best_of_week" rows="6" cols="72" placeholder="42 | kickoff thread worth spotlighting&#10;77 | high-signal answer chain">` + htmlEscape(renderBestOfWeekText(a.loadBestOfWeekEntries())) + `</textarea></label><br><button type="submit">Save Editorial Picks</button></form></article><article class="wolfbbs-card"><h2>Legacy Archive Import</h2><form method="POST" action="/admin/boards"><input type="hidden" name="action" value="import_legacy_archive"><input type="hidden" name="redirect_to" value="/admin/boards">` + csrf + `<label>Target board <select name="import_board_id">` + allBoardOptions.String() + `</select></label><br><label>Archive blob<br><textarea name="archive_blob" rows="10" cols="72" placeholder="Subject: Welcome back&#10;From: oldsysop&#10;Date: 2025-04-01 20:15&#10;&#10;Imported body text here.&#10;---&#10;Subject: Another thread&#10;&#10;Body">` + `</textarea></label><br><button type="submit">Import Archive</button></form></article></section>`

	reportStatusSelect := map[string]string{"": "", "open": "", "resolved": ""}
	reportStatusSelect[reportStatus] = ` selected`
	page := `<html><body><h1>Sysop Boards</h1><p><a href="/admin">back</a> | <a href="/help">help</a></p>` +
		`<form method="POST"><label>Title <input name="title"></label> <label>Conference <input name="conference" value="General" size="14"></label> <label>Description <input name="description" size="28"></label> <label>Read ACS <input name="read_acs" size="16"></label> <label>Write ACS <input name="write_acs" size="16"></label>` + csrf + `<input type="hidden" name="action" value="create"><button type="submit">add</button></form>` +
		`<table border="1"><tr><th>ID</th><th>Title</th><th>Conf</th><th>Topics</th><th>Last</th><th>Edit</th><th>Actions</th><th>Moderation</th></tr>` + rows.String() + `</table>` +
		`<h2>Moderation Queue</h2>` +
		`<form method="GET" action="/admin/boards"><label>Status <select name="report_status"><option value=""` + reportStatusSelect[""] + `>all</option><option value="open"` + reportStatusSelect["open"] + `>open</option><option value="resolved"` + reportStatusSelect["resolved"] + `>resolved</option></select></label><label> Board <input name="manage_board" size="6" value="` + strconv.FormatInt(manageBoardID, 10) + `"></label><button type="submit">apply</button></form>` +
		`<table border="1"><tr><th>ID</th><th>Message</th><th>Board</th><th>Subject</th><th>Reporter</th><th>Reason + Hints</th><th>Status</th><th>Created</th><th>Action</th></tr>` + reportRows.String() + `</table>` +
		`<h2>Board Message Moderation</h2><p>Delete with reason, lock/unlock thread, and move thread to another board.</p>` +
		`<table border="1"><tr><th>ID</th><th>Thread</th><th>Author</th><th>Subject + Hints</th><th>When</th><th>Actions</th></tr>` + manageRows.String() + `</table>` + manageBoardBlock + bestOfBlock +
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
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		target := strings.TrimSpace(r.FormValue("handle"))
		switch action {
		case "disable_outbound", "enable_outbound":
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
		case "assign_shared":
			mailID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("mail_id")), 10, 64)
			if mailID > 0 {
				row := moderatorInboxAssignment{
					MailID:    mailID,
					Assignee:  strings.TrimSpace(r.FormValue("assignee")),
					Status:    defaultIfBlank(r.FormValue("status"), "assigned"),
					Note:      strings.TrimSpace(r.FormValue("note")),
					UpdatedBy: user.Handle,
					UpdatedAt: time.Now().UTC(),
				}
				a.updateModeratorInboxAssignment(mailID, row)
				a.recordAdminAction(user.Handle, "mail #"+strconv.FormatInt(mailID, 10), "assign_shared_mail", defaultIfBlank(row.Assignee, "unassigned"))
			}
		case "resolve_shared":
			mailID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("mail_id")), 10, 64)
			if mailID > 0 {
				assignments := a.loadModeratorInboxAssignments()
				if assignments == nil {
					assignments = map[int64]moderatorInboxAssignment{}
				}
				row := assignments[mailID]
				row.MailID = mailID
				row.Status = "resolved"
				row.UpdatedBy = user.Handle
				row.UpdatedAt = time.Now().UTC()
				row.ResolvedAt = row.UpdatedAt
				assignments[mailID] = row
				a.persistModeratorInboxAssignments(assignments)
				a.recordAdminAction(user.Handle, "mail #"+strconv.FormatInt(mailID, 10), "resolve_shared_mail", "")
			}
		case "merge_send":
			subject := strings.TrimSpace(r.FormValue("subject"))
			body := strings.TrimSpace(r.FormValue("body"))
			if subject == "" || body == "" {
				redirectWithError(w, r, "/admin/mail", "Merge subject and body are required.")
				return
			}
			roleFilter := normalizeDirectoryRoleFilter(r.FormValue("role"))
			verifiedOnly := parseCheckbox(r.FormValue("verified_only"))
			explicit := splitTrimmedList(r.FormValue("handles"), ",", 64)
			explicitSet := map[string]struct{}{}
			for _, row := range explicit {
				explicitSet[normalizeHandleKey(row)] = struct{}{}
			}
			users, _ := a.authSvc.ListUsers()
			delivered := 0
			for _, row := range users {
				if !directoryVisibleUser(&row) || row.ID == user.ID {
					continue
				}
				if roleFilter != "any" && rbac.NormalizeRole(row.Role) != roleFilter {
					continue
				}
				if verifiedOnly && !row.Verified {
					continue
				}
				if len(explicitSet) > 0 {
					if _, ok := explicitSet[normalizeHandleKey(row.Handle)]; !ok {
						continue
					}
				}
				personalized := strings.ReplaceAll(body, "{{handle}}", row.Handle)
				if err := a.mailRepo.CreateMail(&domain.PrivateMail{
					FromUserID: user.ID,
					ToUserID:   row.ID,
					Subject:    subject,
					Body:       personalized,
				}); err == nil {
					delivered++
				}
			}
			a.recordAdminAction(user.Handle, "mail.merge", "merge_send", fmt.Sprintf("delivered=%d role=%s verified_only=%t", delivered, roleFilter, verifiedOnly))
			redirectWithNotice(w, r, "/admin/mail", fmt.Sprintf("Mail merge delivered to %d caller(s).", delivered))
			return
		}
		http.Redirect(w, r, "/admin/mail", http.StatusFound)
		return
	}
	csrf := a.csrfHiddenInput(r)
	users, _ := a.authSvc.ListUsers()
	userByID := map[int64]domain.User{}
	moderatorHandles := []string{}
	for _, row := range users {
		userByID[row.ID] = row
		if directoryVisibleUser(&row) && a.hasRole(&row, roleModerator) {
			moderatorHandles = append(moderatorHandles, row.Handle)
		}
	}
	sort.Strings(moderatorHandles)
	rows := strings.Builder{}
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

	assignments := a.loadModeratorInboxAssignments()
	queueRows := strings.Builder{}
	seenMail := map[int64]struct{}{}
	sharedCount := 0
	resolvedCount := 0
	for _, modHandle := range moderatorHandles {
		target, err := a.authSvc.GetUser(modHandle)
		if err != nil || target == nil {
			continue
		}
		inbox, err := a.mailRepo.ListInbox(target.ID, 200)
		if err != nil {
			continue
		}
		for _, mail := range inbox {
			if _, ok := seenMail[mail.ID]; ok {
				continue
			}
			seenMail[mail.ID] = struct{}{}
			assignment := assignments[mail.ID]
			if assignment.Status == "" {
				assignment.Status = "open"
			}
			if assignment.Status == "resolved" {
				resolvedCount++
			} else {
				sharedCount++
			}
			fromHandle := userByID[mail.FromUserID].Handle
			cleanSubject := cleanOneLiner(mail.Subject, 72)
			queueRows.WriteString(`<tr><td><a href="/mail?id=` + strconv.FormatInt(mail.ID, 10) + `">` + strconv.FormatInt(mail.ID, 10) + `</a></td><td>` + htmlEscape(defaultIfBlank(fromHandle, "#"+strconv.FormatInt(mail.FromUserID, 10))) + `</td><td>` + htmlEscape(target.Handle) + `</td><td>` + htmlEscape(cleanSubject) + `</td><td>` + mail.CreatedAt.Local().Format("2006-01-02 15:04") + `</td><td>` + htmlEscape(assignment.Status) + `</td><td>` + htmlEscape(defaultIfBlank(assignment.Assignee, "unassigned")) + `</td><td><form method="POST" action="/admin/mail" class="wolfbbs-inline-form">` + csrf + `<input type="hidden" name="action" value="assign_shared"><input type="hidden" name="mail_id" value="` + strconv.FormatInt(mail.ID, 10) + `"><label>Assignee <input name="assignee" size="12" value="` + htmlEscape(assignment.Assignee) + `" placeholder="moderator"></label><label>Status <select name="status"><option value="open"` + selectedIf(assignment.Status == "open") + `>open</option><option value="assigned"` + selectedIf(assignment.Status == "assigned") + `>assigned</option><option value="resolved"` + selectedIf(assignment.Status == "resolved") + `>resolved</option></select></label><input name="note" size="18" value="` + htmlEscape(assignment.Note) + `" placeholder="handoff note"><button type="submit">save</button></form><form method="POST" action="/admin/mail" class="wolfbbs-inline-form">` + csrf + `<input type="hidden" name="action" value="resolve_shared"><input type="hidden" name="mail_id" value="` + strconv.FormatInt(mail.ID, 10) + `"><button type="submit">resolve</button></form></td></tr>`)
		}
	}
	if queueRows.Len() == 0 {
		queueRows.WriteString(`<tr><td colspan="8">No shared moderator inbox items yet.</td></tr>`)
	}

	roleOptions := []string{"any", roleUser, roleModerator, roleAdmin}
	roleOptionRows := strings.Builder{}
	for _, row := range roleOptions {
		label := row
		if row == "any" {
			label = "any role"
		}
		roleOptionRows.WriteString(`<option value="` + htmlEscape(row) + `">` + htmlEscape(label) + `</option>`)
	}
	messageBlock := pageMessageBlock(r)
	page := `<html><body><h1>Mail Controls</h1><p><a href="/admin">back</a> | <a href="/help">help</a></p>` + messageBlock +
		`<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(sharedCount) + `</strong><span>open shared items</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(resolvedCount) + `</strong><span>resolved assignments</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(policies)) + `</strong><span>outbound overrides</span></article></section>` +
		`<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Mail Merge</h2><form method="POST" action="/admin/mail">` + csrf + `<input type="hidden" name="action" value="merge_send"><label>Role <select name="role">` + roleOptionRows.String() + `</select></label><br><label><input type="checkbox" name="verified_only" value="1"> verified only</label><br><label>Handles (optional comma-separated allowlist)<br><textarea name="handles" rows="3" cols="64" placeholder="alice, bob, caller42"></textarea></label><br><label>Subject <input name="subject" size="64" placeholder="Launch reminder"></label><br><label>Body<br><textarea name="body" rows="8" cols="80" placeholder="Hello {{handle}},&#10;&#10;..."></textarea></label><br><button type="submit">Send Merge</button></form></article><article class="wolfbbs-card"><h2>Operator Notes</h2><ul class="wolfbbs-list-clean"><li>Use the shared queue for moderation/support mail that needs assignment, not just passive reading.</li><li>Mail merge is local internal mail only. It is for targeted outreach, not blind external blast.</li><li>Use <code>{{handle}}</code> in the body for lightweight personalization.</li></ul></article></section>` +
		`<h2>Shared Moderator Inbox</h2><table border="1"><tr><th>ID</th><th>From</th><th>Inbox</th><th>Subject</th><th>When</th><th>Status</th><th>Assignee</th><th>Action</th></tr>` + queueRows.String() + `</table>` +
		`<h2>Outbound Controls</h2><table border="1"><tr><th>Handle</th><th>OutboundDisabled</th><th>Action</th></tr>` + rows.String() + `</table></body></html>`
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
		if a.adminFileWorkflowAction(user, r, &redirectURL) {
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}
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
		case "upload":
			areaID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("area_id")), 10, 64)
			reviewHold := formHasValue(r, "review_hold")
			reviewNotes := strings.TrimSpace(r.FormValue("review_notes"))
			if a.adminRepo != nil && areaID > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, configuredUploadRequestMaxBytes())
				areas, _ := a.adminRepo.ListFileAreas()
				for _, area := range areas {
					if area.ID != areaID {
						continue
					}
					entry, err := a.importUploadedFile(r, area, user, strings.TrimSpace(r.FormValue("description")), strings.TrimSpace(r.FormValue("tags")))
					if err != nil {
						a.addAppError("admin.files", fmt.Errorf("upload file: %w", err))
						redirectWithError(w, r, redirectURL, uploadFailureMessage(err))
						return
					}
					if reviewHold {
						a.setFileReviewItem(fileReviewItem{
							FileID:    entry.ID,
							AreaID:    entry.AreaID,
							Name:      entry.Name,
							Status:    fileReviewHold,
							Notes:     reviewNotes,
							Uploader:  user.Handle,
							CreatedAt: time.Now().UTC(),
						})
					}
					a.recordAdminAction(user.Handle, entry.Name, "upload_file", fmt.Sprintf("area=%d hold=%t", areaID, reviewHold))
					redirectURL = "/admin/files?area=" + strconv.FormatInt(areaID, 10)
					break
				}
			}
		case "delete":
			id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
			if a.adminRepo != nil && id > 0 {
				_ = a.adminRepo.DeleteFileArea(id)
				a.recordAdminAction(user.Handle, strconv.FormatInt(id, 10), "delete_file_area", "")
			}
		case "delete_file":
			fileID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
			if a.adminRepo != nil && fileID > 0 {
				entry, err := a.adminRepo.GetFileEntry(fileID)
				if err != nil || entry == nil {
					if err != nil {
						a.addAppError("admin.files", fmt.Errorf("load file %d for delete: %w", fileID, err))
					}
					redirectWithError(w, r, redirectURL, "File delete failed.")
					return
				}
				if err := a.deleteIndexedFile(entry); err != nil {
					a.addAppError("admin.files", fmt.Errorf("delete file %d: %w", fileID, err))
					redirectWithError(w, r, redirectURL, "File delete failed.")
					return
				}
				a.removeFileReviewItem(fileID)
				a.recordAdminAction(user.Handle, entry.Name, "delete_file_entry", fmt.Sprintf("file_id=%d", fileID))
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
			if ttlMinutes > 1440 {
				ttlMinutes = 1440
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
					redirectURL = "/admin/files?queue_user=" + strconv.FormatInt(targetUserID, 10) + "&issued_token=" + url.QueryEscape(ticket.Token) + "&issued_file=" + strconv.FormatInt(fileID, 10) + "&issued_expires=" + url.QueryEscape(ticket.ExpiresAt.UTC().Format(time.RFC3339))
				}
			}
		case "review_file":
			fileID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
			if fileID > 0 {
				row, ok := a.fileReviewItem(fileID)
				if !ok && a.adminRepo != nil {
					if entry, err := a.adminRepo.GetFileEntry(fileID); err == nil && entry != nil {
						row = fileReviewItem{
							FileID:    entry.ID,
							AreaID:    entry.AreaID,
							Name:      entry.Name,
							Uploader:  user.Handle,
							CreatedAt: time.Now().UTC(),
						}
						ok = true
					}
				}
				if ok {
					row.Status = normalizeFileReviewStatus(r.FormValue("status"))
					row.Notes = strings.TrimSpace(r.FormValue("review_notes"))
					row.ReviewedAt = time.Now().UTC()
					row.ReviewedBy = user.Handle
					a.setFileReviewItem(row)
					a.recordAdminAction(user.Handle, strconv.FormatInt(fileID, 10), "review_file", row.Status)
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
	reviewQueue := a.loadFileReviewQueue()
	fileAreaID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("area")), 10, 64)
	queueUserID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("queue_user")), 10, 64)
	filterUserID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("filter_user")), 10, 64)
	savedFilterID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("saved_filter")), 10, 64)
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
	issuedFileID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("issued_file")), 10, 64)
	issuedExpiresRaw := strings.TrimSpace(r.URL.Query().Get("issued_expires"))
	appliedFilterName := ""
	if a.adminRepo != nil {
		areas, _ = a.adminRepo.ListFileAreas()
		for _, area := range areas {
			areaNames[area.ID] = area.Name
		}
		if filterUserID > 0 {
			filters, _ = a.adminRepo.ListFileFilters(filterUserID)
		} else {
			filters, _ = a.adminRepo.ListFileFilters(0)
		}
		if savedFilter, ok := findFileFilterByID(filters, savedFilterID); ok {
			appliedFilterName = savedFilter.Name
			searchQuery = strings.TrimSpace(savedFilter.Query)
			searchTags = append([]string(nil), savedFilter.Tags...)
			searchTagsRaw = strings.Join(savedFilter.Tags, ",")
			if filterUserID <= 0 && savedFilter.UserID > 0 {
				filterUserID = savedFilter.UserID
			}
		}
		files, _ = a.adminRepo.ListFileEntries(fileAreaID, searchQuery, searchTags, 300)
		if filterUserID > 0 {
			filters, _ = a.adminRepo.ListFileFilters(filterUserID)
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
	areaOptionRows := strings.Builder{}
	areaRows := strings.Builder{}
	for _, area := range areas {
		selected := ""
		if area.ID == fileAreaID {
			selected = ` selected`
		}
		areaOptionRows.WriteString(`<option value="` + strconv.FormatInt(area.ID, 10) + `"` + selected + `>` + htmlEscape(area.Name) + `</option>`)
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
		reviewStatus := htmlEscape(a.reviewStatusLabel(row.ID))
		fileRows.WriteString(`<tr><td>` + strconv.FormatInt(row.ID, 10) + `</td><td>` + areaName + `</td><td>` + htmlEscape(row.Name) + `</td><td>` + tags + `</td><td>` + reviewStatus + `</td><td>` + fmt.Sprintf("%.2f", row.RatingAvg) + ` (` + strconv.Itoa(row.RatingCount) + `)</td><td>` + htmlEscape(row.SHA256) + `</td>`)
		fileRows.WriteString(`<td><form method="POST" action="/admin/files">` + csrf +
			`<input type="hidden" name="action" value="rate"><input type="hidden" name="file_id" value="` + strconv.FormatInt(row.ID, 10) + `"><input type="hidden" name="user_id" value="` + strconv.FormatInt(user.ID, 10) + `">` +
			`<input name="rating" size="2" value="5"><button type="submit">rate</button></form>`)
		fileRows.WriteString(`<form method="POST" action="/admin/files">` + csrf +
			`<input type="hidden" name="action" value="queue_add"><input type="hidden" name="file_id" value="` + strconv.FormatInt(row.ID, 10) + `"><input name="queue_user_id" size="6" value="` + strconv.FormatInt(user.ID, 10) + `"><button type="submit">queue</button></form>`)
		fileRows.WriteString(`<form method="POST" action="/admin/files">` + csrf +
			`<input type="hidden" name="action" value="delete_file"><input type="hidden" name="file_id" value="` + strconv.FormatInt(row.ID, 10) + `"><button type="submit">delete file</button></form></td></tr>`)
	}
	if fileRows.Len() == 0 {
		fileRows.WriteString(`<tr><td colspan="8">No indexed files matched</td></tr>`)
	}

	filterRows := strings.Builder{}
	for _, row := range filters {
		applyURL := `/admin/files?saved_filter=` + strconv.FormatInt(row.ID, 10)
		if row.UserID > 0 {
			applyURL += `&filter_user=` + strconv.FormatInt(row.UserID, 10)
		}
		filterRows.WriteString(`<tr><td>` + strconv.FormatInt(row.ID, 10) + `</td><td>` + strconv.FormatInt(row.UserID, 10) + `</td><td>` + htmlEscape(row.Name) + `</td><td>` + htmlEscape(row.Query) + `</td><td>` + htmlEscape(strings.Join(row.Tags, ",")) + `</td><td><a href="` + applyURL + `">apply</a></td></tr>`)
	}
	if filterRows.Len() == 0 {
		filterRows.WriteString(`<tr><td colspan="6">No saved filters</td></tr>`)
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

	reviewRows := strings.Builder{}
	reviewList := make([]fileReviewItem, 0, len(reviewQueue))
	for _, row := range reviewQueue {
		reviewList = append(reviewList, row)
	}
	sort.Slice(reviewList, func(i, j int) bool {
		if reviewList[i].CreatedAt.Equal(reviewList[j].CreatedAt) {
			return reviewList[i].FileID > reviewList[j].FileID
		}
		return reviewList[i].CreatedAt.After(reviewList[j].CreatedAt)
	})
	for _, row := range reviewList {
		areaName := areaNames[row.AreaID]
		if areaName == "" {
			areaName = "Area " + strconv.FormatInt(row.AreaID, 10)
		}
		reviewRows.WriteString(`<tr><td>` + strconv.FormatInt(row.FileID, 10) + `</td><td>` + htmlEscape(areaName) + `</td><td>` + htmlEscape(row.Name) + `</td><td>` + htmlEscape(row.Status) + `</td><td>` + htmlEscape(defaultIfBlank(row.Uploader, "unknown")) + `</td><td>` + row.CreatedAt.Local().Format("2006-01-02 15:04") + `</td><td>` + htmlEscape(row.Notes) + `</td><td><div class="wolfbbs-inline-actions"><form method="POST" action="/admin/files">` + csrf + `<input type="hidden" name="action" value="review_file"><input type="hidden" name="file_id" value="` + strconv.FormatInt(row.FileID, 10) + `"><select name="status"><option value="hold"` + selectedIf(row.Status == fileReviewHold) + `>hold</option><option value="approved"` + selectedIf(row.Status == fileReviewApproved) + `>approved</option><option value="rejected"` + selectedIf(row.Status == fileReviewRejected) + `>rejected</option></select><input name="review_notes" size="20" value="` + htmlEscape(row.Notes) + `" placeholder="review notes"><button type="submit">save</button></form><form method="POST" action="/admin/files">` + csrf + `<input type="hidden" name="action" value="delete_file"><input type="hidden" name="file_id" value="` + strconv.FormatInt(row.FileID, 10) + `"><button type="submit">delete file</button></form></div></td></tr>`)
	}
	if reviewRows.Len() == 0 {
		reviewRows.WriteString(`<tr><td colspan="8">No uploads waiting for review.</td></tr>`)
	}

	var ticketNotice string
	if issuedToken != "" {
		fileLabel := "selected queue file"
		if a.adminRepo != nil && issuedFileID > 0 {
			if entry, err := a.adminRepo.GetFileEntry(issuedFileID); err == nil && entry != nil && strings.TrimSpace(entry.Name) != "" {
				fileLabel = entry.Name
			} else {
				fileLabel = "file #" + strconv.FormatInt(issuedFileID, 10)
			}
		}
		expiresLabel := "expiry not provided"
		if issuedExpiresRaw != "" {
			if expiresAt, err := time.Parse(time.RFC3339, issuedExpiresRaw); err == nil {
				remaining := time.Until(expiresAt)
				remainingLabel := "expired"
				if remaining > 0 {
					minutes := int((remaining + time.Minute - time.Second) / time.Minute)
					remainingLabel = strconv.Itoa(minutes) + "m remaining"
				}
				expiresLabel = expiresAt.Local().Format("2006-01-02 15:04 MST") + " (" + remainingLabel + ")"
			} else {
				expiresLabel = issuedExpiresRaw
			}
		}
		ticketNotice = `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Issued Ticket</h2><ul class="wolfbbs-list-clean"><li><strong>Token:</strong> <code>` + htmlEscape(issuedToken) + `</code></li><li><strong>File:</strong> ` + htmlEscape(fileLabel) + `</li><li><strong>Expires:</strong> ` + htmlEscape(expiresLabel) + `</li><li><strong>Link:</strong> <a href="/gateway?download=` + url.QueryEscape(issuedToken) + `">/gateway?download=` + url.QueryEscape(issuedToken) + `</a></li></ul></article></section>`
	}
	activeFilters := make([]string, 0, 4)
	if fileAreaID > 0 {
		activeFilters = append(activeFilters, "area #"+strconv.FormatInt(fileAreaID, 10))
	}
	if searchQuery != "" {
		activeFilters = append(activeFilters, "query: "+searchQuery)
	}
	if searchTagsRaw != "" {
		activeFilters = append(activeFilters, "tags: "+searchTagsRaw)
	}
	if appliedFilterName != "" {
		activeFilters = append(activeFilters, "saved filter: "+appliedFilterName)
	}
	filterSummary := renderActiveFilterPanel("Active File Filters", "/admin/files", activeFilters)
	workflowSections := a.renderAdminFileWorkflowSections(r, user, areaOptionRows.String(), areaNames)

	page := `<html><body><h1>Files</h1><p><a href="/admin">back</a> | <a href="/gateway">gateway</a> | <a href="/help">help</a></p>` + ticketNotice +
		`<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Upload intake is browser-native now</strong><p>Use the upload form to land a file directly into a file area, extract metadata, and optionally put it on hold for review.</p></article><article class="wolfbbs-helper-card"><strong>Review before public exposure</strong><p>Hold status keeps a file out of caller-facing file lists until a sysop approves it.</p></article><article class="wolfbbs-helper-card"><strong>Use saved filters deliberately</strong><p>Saved filters are now actionable. Apply one to reload the indexed table instead of manually retyping the search every time.</p></article></section>` + filterSummary +
		`<form method="POST"><input type="hidden" name="action" value="create">` + csrf +
		`<label>Name <input name="name"></label> <label>Path <input name="path" size="30"></label> <label>Description <input name="description" size="40"></label> <button type="submit">add</button></form>` +
		`<table border="1"><tr><th>ID</th><th>Name</th><th>Path</th><th>Description</th><th>Index</th><th>Delete</th></tr>` + areaRows.String() + `</table>` +
		`<h2>Upload Intake</h2><form method="POST" enctype="multipart/form-data">` + csrf + `<input type="hidden" name="action" value="upload"><label>Area <select name="area_id">` + areaOptionRows.String() + `</select></label> <label>File <input type="file" name="upload_file"></label> <label>Description <input name="description" size="28"></label> <label>Tags <input name="tags" size="24" placeholder="ansi,retro,zip"></label> <label><input type="checkbox" name="review_hold" value="1" checked> Hold for review</label> <label>Review notes <input name="review_notes" size="24" placeholder="scan pending"></label> <button type="submit">Upload file</button></form>` +
		`<h2>Review Queue</h2><table border="1"><tr><th>File ID</th><th>Area</th><th>Name</th><th>Status</th><th>Uploader</th><th>Uploaded</th><th>Notes</th><th>Action</th></tr>` + reviewRows.String() + `</table>` +
		`<h2>Indexed Files</h2><form method="GET" data-filter-form="1" data-filter-reset="/admin/files"><label>Area ID <input name="area" value="` + strconv.FormatInt(fileAreaID, 10) + `" size="6" data-filter-label="area"></label> <label>Query <input name="q" value="` + htmlEscape(searchQuery) + `" size="24" data-filter-label="query"></label> <label>Tags <input name="tags" value="` + htmlEscape(searchTagsRaw) + `" size="24" data-filter-label="tags"></label> <button type="submit">search</button></form>` +
		`<table border="1"><tr><th>ID</th><th>Area</th><th>Name</th><th>Tags</th><th>Review</th><th>Rating</th><th>SHA-256</th><th>Actions</th></tr>` + fileRows.String() + `</table>` +
		`<h2>Saved Filters</h2><form method="POST">` + csrf + `<input type="hidden" name="action" value="save_filter"><label>User ID <input name="filter_user_id" value="` + strconv.FormatInt(maxInt64(filterUserID, user.ID), 10) + `" size="8"></label> <label>Name <input name="name" size="16"></label> <label>Query <input name="query" size="24"></label> <label>Tags <input name="tags" size="24" placeholder="tag1,tag2"></label> <button type="submit">save</button></form>` +
		`<table border="1"><tr><th>ID</th><th>User</th><th>Name</th><th>Query</th><th>Tags</th><th>Action</th></tr>` + filterRows.String() + `</table>` +
		`<h2>Download Queue</h2><form method="GET"><label>User ID <input name="queue_user" value="` + strconv.FormatInt(maxInt64(queueUserID, user.ID), 10) + `" size="8"></label><button type="submit">view queue</button></form>` +
		`<table border="1"><tr><th>ID</th><th>User</th><th>File</th><th>Queued At</th><th>Actions</th></tr>` + queueRows.String() + `</table>` +
		workflowSections +
		`</body></html>`
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
		cfg := sanitizeGatewaySettings(&domain.GatewaySettings{
			SMTPHost:        strings.TrimSpace(r.FormValue("smtp_host")),
			SMTPPort:        parseInt(r.FormValue("smtp_port"), 587),
			SMTPUser:        strings.TrimSpace(r.FormValue("smtp_user")),
			SMTPPass:        strings.TrimSpace(r.FormValue("smtp_pass")),
			FromDomain:      strings.TrimSpace(r.FormValue("from_domain")),
			MaxRecipients:   parseInt(r.FormValue("max_recipients"), 3),
			MaxMessageBytes: parseInt(r.FormValue("max_message_bytes"), 65536),
			WebTimeoutSec:   parseInt(r.FormValue("web_timeout_sec"), 10),
			WebMaxBytes:     parseInt(r.FormValue("web_max_bytes"), 2*1024*1024),
		})
		if a.adminRepo != nil && strings.TrimSpace(cfg.SMTPPass) == "" {
			if existing, err := a.adminRepo.GetGatewaySettings(); err == nil && existing != nil {
				cfg.SMTPPass = strings.TrimSpace(existing.SMTPPass)
			}
		}
		aiCfg := aiGatewaySettings{
			Enabled:      parseCheckbox(r.FormValue("ai_enabled")),
			BaseURL:      strings.TrimSpace(r.FormValue("ai_base_url")),
			Model:        strings.TrimSpace(r.FormValue("ai_model")),
			APIKey:       strings.TrimSpace(r.FormValue("ai_api_key")),
			SystemPrompt: strings.TrimSpace(r.FormValue("ai_system_prompt")),
			TimeoutSec:   parseInt(r.FormValue("ai_timeout_sec"), 20),
			MaxTokens:    parseInt(r.FormValue("ai_max_tokens"), 400),
		}
		if aiCfg.BaseURL == "" {
			aiCfg.BaseURL = "https://api.openai.com"
		}
		if aiCfg.Model == "" {
			aiCfg.Model = "gpt-4.1-mini"
		}
		if aiCfg.TimeoutSec <= 0 {
			aiCfg.TimeoutSec = 20
		}
		if aiCfg.MaxTokens <= 0 {
			aiCfg.MaxTokens = 400
		}
		if err := gateway.ValidateSafeHTTPURLConfig(aiCfg.BaseURL, allowPrivateAIGatewayBaseURLs()); err != nil {
			redirectWithError(w, r, "/admin/gateways", "AI base URL rejected: "+err.Error())
			return
		}
		if a.adminRepo != nil {
			if err := a.adminRepo.UpsertGatewaySettings(cfg); err == nil {
				a.recordAdminAction(user.Handle, "gateway_settings", "update_gateway_settings", "saved")
			} else {
				a.addAppError("admin.gateways", fmt.Errorf("save gateway settings: %w", err))
			}
		}
		a.persistAIGatewaySettings(aiCfg)
		redirectWithNotice(w, r, "/admin/gateways", "Gateway settings saved.")
		return
	}
	cfg := a.activeGatewaySettings()
	aiCfg := a.loadAIGatewaySettings()
	aiAllowPrivate := allowPrivateAIGatewayBaseURLs()
	emailGateway := a.activeEmailGateway()
	csrf := a.csrfHiddenInput(r)
	messageBlock := pageMessageBlock(r)
	maskedAIKey := "missing"
	if strings.TrimSpace(aiCfg.APIKey) != "" {
		maskedAIKey = "configured"
	}
	page := `<html><body><h1>Gateway Controls</h1><p><a href="/admin">back</a> | <a href="/gateway">gateway hub</a> | <a href="/help">help</a></p>` +
		messageBlock +
		`<h2>Email + Web Safety</h2>` +
		`<form method="POST">` + csrf +
		`<input type="hidden" name="action" value="save_gateway">` +
		`<label>SMTP Host <input name="smtp_host" value="` + htmlEscape(cfg.SMTPHost) + `"></label><br>` +
		`<label>SMTP Port <input name="smtp_port" value="` + strconv.Itoa(cfg.SMTPPort) + `"></label><br>` +
		`<label>SMTP User <input name="smtp_user" value="` + htmlEscape(cfg.SMTPUser) + `"></label><br>` +
		`<label>SMTP Pass <input type="password" name="smtp_pass" value=""></label> <span class="wolfbbs-muted">leave blank to keep existing secret</span><br>` +
		`<label>From Domain <input name="from_domain" value="` + htmlEscape(cfg.FromDomain) + `"></label><br>` +
		`<label>Max Recipients <input name="max_recipients" value="` + strconv.Itoa(cfg.MaxRecipients) + `"></label><br>` +
		`<label>Max Message Bytes <input name="max_message_bytes" value="` + strconv.Itoa(cfg.MaxMessageBytes) + `"></label><br>` +
		`<label>Web Timeout Sec <input name="web_timeout_sec" value="` + strconv.Itoa(cfg.WebTimeoutSec) + `"></label><br>` +
		`<label>Web Max Bytes <input name="web_max_bytes" value="` + strconv.Itoa(cfg.WebMaxBytes) + `"></label><br>` +
		`<h2>AI Gateway</h2>` +
		`<label><input type="checkbox" name="ai_enabled"` + checkedIf(aiCfg.Enabled) + `> Enable AI gateway door</label><br>` +
		`<label>AI Base URL <input name="ai_base_url" size="50" value="` + htmlEscape(aiCfg.BaseURL) + `"></label><br>` +
		`<label>AI Model <input name="ai_model" value="` + htmlEscape(aiCfg.Model) + `"></label><br>` +
		`<label>AI API Key <input type="password" name="ai_api_key" value=""></label> <span class="wolfbbs-muted">status: ` + htmlEscape(maskedAIKey) + `</span><br>` +
		`<label>AI Timeout Sec <input name="ai_timeout_sec" value="` + strconv.Itoa(aiCfg.TimeoutSec) + `"></label><br>` +
		`<label>AI Max Tokens <input name="ai_max_tokens" value="` + strconv.Itoa(aiCfg.MaxTokens) + `"></label><br>` +
		`<label>AI System Prompt<br><textarea name="ai_system_prompt" rows="4" cols="84">` + htmlEscape(aiCfg.SystemPrompt) + `</textarea></label><br>` +
		`<p class="wolfbbs-muted">Private or loopback AI endpoints stay blocked by default. Set <code>WOLFBBS_GATEWAY_AI_ALLOW_PRIVATE=1</code> only when you intentionally run a local/private model endpoint.</p>` +
		`<button type="submit">Save</button></form>` +
		`<h2>Diagnostics</h2><table border="1"><tr><th>Check</th><th>Status</th></tr>` +
		`<tr><td>Email relay configured</td><td>` + boolToText(emailGateway.Enabled()) + `</td></tr>` +
		`<tr><td>External email verification gate</td><td>` + boolToText(a.requireVerifiedEmail) + `</td></tr>` +
		`<tr><td>AI gateway enabled</td><td>` + boolToText(aiCfg.Enabled) + `</td></tr>` +
		`<tr><td>AI API key configured</td><td>` + boolToText(strings.TrimSpace(aiCfg.APIKey) != "") + `</td></tr>` +
		`<tr><td>AI private/loopback override</td><td>` + boolToText(aiAllowPrivate) + `</td></tr>` +
		`</table><p><a href="/gateway?view=email">Email door</a> | <a href="/gateway?view=ai">AI door</a> | <a href="/gateway?view=browser">Web browser door</a></p>` +
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
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}
	if !a.enforceWFCAccess(w, r, user) {
		return
	}

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
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}
	if !a.enforceWFCAccess(w, r, user) {
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
		Role:       role,
		Verified:   verified,
		Secure:     false,
		Transport:  "http",
		AuthFactor: 1,
		Groups:     roleGroups(role),
		Attrs:      attrs,
	})
	if err != nil {
		return !envEnabledDefault("WOLFBBS_ACS_STRICT", false)
	}
	return allowed
}

func (a *webApp) evalACSWithSession(expr string, user *domain.User, session sessionState, attrs map[string]string) bool {
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
	attrs["transport"] = strings.TrimSpace(session.transport)
	attrs["secure"] = boolToText(session.secure)
	attrs["auth_factor"] = strconv.Itoa(maxInt(session.authFactor, 1))
	allowed, err := acs.Evaluate(expr, acs.Context{
		Role:       role,
		Verified:   verified,
		Secure:     session.secure,
		Transport:  strings.TrimSpace(session.transport),
		AuthFactor: maxInt(session.authFactor, 1),
		Groups:     roleGroups(role),
		Attrs:      attrs,
	})
	if err != nil {
		return !envEnabledDefault("WOLFBBS_ACS_STRICT", false)
	}
	return allowed
}

func roleGroups(role string) []string {
	role = rbac.NormalizeRole(role)
	switch role {
	case roleAdmin:
		return []string{"staff", "ops", "wfc", "sysop"}
	case roleModerator:
		return []string{"staff", "moderator"}
	default:
		return []string{"users"}
	}
}

func wfcRequiredACS() string {
	expr := strings.TrimSpace(os.Getenv("WOLFBBS_WFC_REQUIRED_ACS"))
	if expr != "" {
		return expr
	}
	if envEnabledDefault("WOLFBBS_WFC_STRICT", false) {
		return "role=sysop and secure and auth_factor>=2 and group=wfc"
	}
	return "role=sysop"
}

func (a *webApp) enforceWFCAccess(w http.ResponseWriter, r *http.Request, user *domain.User) bool {
	expr := strings.TrimSpace(wfcRequiredACS())
	if expr == "" {
		return true
	}
	session, ok := currentSessionState(r, a)
	if !ok {
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return false
	}
	if a.evalACSWithSession(expr, user, session, map[string]string{"area": "admin", "path": "/admin/system", "mode": "wfc"}) {
		return true
	}
	http.Error(w, "wfc access policy denied", http.StatusForbidden)
	return false
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

func defaultIfBlank(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
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
	a.applyPersistedThemeBundle(settings)
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
	a.appendSharedRuntimeError(entry)
	log.Printf("area=%s error=%s", entry.Area, entry.Message)
}

func (a *webApp) latestErrors(limit int) []appErrorEntry {
	if shared := a.loadSharedRuntimeErrors(); len(shared) > 0 {
		return reverseSharedRuntimeErrors(shared, limit)
	}
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
	cleared := len(a.errorLog)
	a.errorLog = nil
	a.Unlock()
	a.persistSharedRuntimeErrors(nil)
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

func boardQuietHoursSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingBoardQuietHoursRoot + handle
}

func normalizeQuietHoursWindow(window quietHoursWindow) quietHoursWindow {
	window.StartHour = ((window.StartHour % 24) + 24) % 24
	window.EndHour = ((window.EndHour % 24) + 24) % 24
	if window.StartHour == window.EndHour {
		window.Enabled = false
	}
	return window
}

func (a *webApp) loadBoardQuietHours(handle string) map[int64]quietHoursWindow {
	if a.adminRepo == nil {
		return nil
	}
	key := boardQuietHoursSettingKey(handle)
	if key == "" {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := map[string]quietHoursWindow{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("board.quiet_hours", fmt.Errorf("decode quiet hours for %s: %w", handle, err))
		return nil
	}
	out := map[int64]quietHoursWindow{}
	for rawID, window := range decoded {
		boardID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || boardID <= 0 {
			continue
		}
		window = normalizeQuietHoursWindow(window)
		if !window.Enabled {
			continue
		}
		out[boardID] = window
	}
	return out
}

func (a *webApp) persistBoardQuietHours(handle string, rows map[int64]quietHoursWindow) {
	key := boardQuietHoursSettingKey(handle)
	if key == "" {
		return
	}
	encoded := map[string]quietHoursWindow{}
	for boardID, window := range rows {
		if boardID <= 0 {
			continue
		}
		window = normalizeQuietHoursWindow(window)
		if !window.Enabled {
			continue
		}
		encoded[strconv.FormatInt(boardID, 10)] = window
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("board.quiet_hours", fmt.Errorf("encode quiet hours for %s: %w", handle, err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(key, body)
}

func (a *webApp) setBoardQuietHours(handle string, boardID int64, window quietHoursWindow) {
	handle = normalizeHandleKey(handle)
	if handle == "" || boardID <= 0 {
		return
	}
	rows := a.loadBoardQuietHours(handle)
	if rows == nil {
		rows = map[int64]quietHoursWindow{}
	}
	window = normalizeQuietHoursWindow(window)
	if !window.Enabled {
		delete(rows, boardID)
	} else {
		rows[boardID] = window
	}
	a.persistBoardQuietHours(handle, rows)
}

func (a *webApp) boardQuietHoursWindow(handle string, boardID int64) (quietHoursWindow, bool) {
	rows := a.loadBoardQuietHours(handle)
	if rows == nil {
		return quietHoursWindow{}, false
	}
	window, ok := rows[boardID]
	return window, ok
}

func boardQuietHoursActive(window quietHoursWindow, now time.Time) bool {
	window = normalizeQuietHoursWindow(window)
	if !window.Enabled {
		return false
	}
	hour := now.In(time.Local).Hour()
	if window.StartHour < window.EndHour {
		return hour >= window.StartHour && hour < window.EndHour
	}
	return hour >= window.StartHour || hour < window.EndHour
}

func (a *webApp) isBoardQuietActive(handle string, boardID int64, now time.Time) bool {
	window, ok := a.boardQuietHoursWindow(handle, boardID)
	if !ok {
		return false
	}
	return boardQuietHoursActive(window, now)
}

func formatQuietHoursWindow(window quietHoursWindow) string {
	window = normalizeQuietHoursWindow(window)
	if !window.Enabled {
		return "off"
	}
	return fmt.Sprintf("%02d:00-%02d:00", window.StartHour, window.EndHour)
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

func normalizeThreadLifecycleState(value string) threadLifecycleState {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(threadLifecycleSlow):
		return threadLifecycleSlow
	case string(threadLifecycleArchived):
		return threadLifecycleArchived
	case string(threadLifecycleFrozen):
		return threadLifecycleFrozen
	default:
		return threadLifecycleActive
	}
}

func threadLifecycleLabel(state threadLifecycleState) string {
	switch normalizeThreadLifecycleState(string(state)) {
	case threadLifecycleSlow:
		return "slow"
	case threadLifecycleArchived:
		return "archived"
	case threadLifecycleFrozen:
		return "frozen"
	default:
		return "active"
	}
}

func threadLifecycleOptionRows(current threadLifecycleState) string {
	options := []threadLifecycleState{
		threadLifecycleActive,
		threadLifecycleSlow,
		threadLifecycleArchived,
		threadLifecycleFrozen,
	}
	var out strings.Builder
	for _, option := range options {
		selected := ""
		if normalizeThreadLifecycleState(string(current)) == option {
			selected = ` selected`
		}
		out.WriteString(`<option value="` + htmlEscape(string(option)) + `"` + selected + `>` + htmlEscape(threadLifecycleLabel(option)) + `</option>`)
	}
	return out.String()
}

func threadLifecycleStatusPill(state threadLifecycleState) string {
	state = normalizeThreadLifecycleState(string(state))
	if state == threadLifecycleActive {
		return ""
	}
	className := "warn"
	if state == threadLifecycleArchived {
		className = "muted"
	}
	if state == threadLifecycleFrozen {
		className = "bad"
	}
	return `<span class="wolfbbs-status-pill ` + className + `">` + htmlEscape(threadLifecycleLabel(state)) + `</span>`
}

func threadLifecycleBlocksReplies(state threadLifecycleState) bool {
	return normalizeThreadLifecycleState(string(state)) == threadLifecycleFrozen
}

func threadLifecycleWarnsReplies(state threadLifecycleState) bool {
	return normalizeThreadLifecycleState(string(state)) == threadLifecycleSlow
}

func normalizeStringLines(lines []string, limit, maxLen int) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.ReplaceAll(line, "\r", ""))
		if line == "" {
			continue
		}
		if maxLen > 0 {
			line = cleanOneLiner(line, maxLen)
		}
		out = append(out, line)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func splitTrimmedLines(raw string, limit, maxLen int) []string {
	return normalizeStringLines(strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n"), limit, maxLen)
}

func messageThreadID(msg domain.Message) int64 {
	if msg.ThreadID > 0 {
		return msg.ThreadID
	}
	if msg.ID > 0 {
		return msg.ID
	}
	if msg.ParentID > 0 {
		return msg.ParentID
	}
	return 0
}

func normalizeBoardWelcomeKit(row boardWelcomeKit) boardWelcomeKit {
	row.Intro = strings.TrimSpace(row.Intro)
	row.SeedPrompts = normalizeStringLines(row.SeedPrompts, 6, 140)
	row.StarterThreads = normalizeStringLines(row.StarterThreads, 6, 140)
	return row
}

func normalizeBoardStaffNote(row boardStaffNote) boardStaffNote {
	row.Note = strings.TrimSpace(row.Note)
	row.UpdatedBy = strings.TrimSpace(row.UpdatedBy)
	if row.Note == "" {
		return boardStaffNote{}
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	return row
}

func normalizeBoardStewards(rows []boardSteward) []boardSteward {
	out := make([]boardSteward, 0, len(rows))
	for _, row := range rows {
		row.Handle = cleanOneLiner(row.Handle, 48)
		row.Topic = cleanOneLiner(row.Topic, 72)
		row.Note = cleanOneLiner(row.Note, 120)
		if row.Handle == "" {
			continue
		}
		out = append(out, row)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func parseBoardStewards(raw string) []boardSteward {
	lines := splitTrimmedLines(raw, 8, 160)
	out := make([]boardSteward, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "|")
		row := boardSteward{}
		if len(parts) > 0 {
			row.Handle = parts[0]
		}
		if len(parts) > 1 {
			row.Topic = parts[1]
		}
		if len(parts) > 2 {
			row.Note = strings.Join(parts[2:], "|")
		}
		out = append(out, row)
	}
	return normalizeBoardStewards(out)
}

func renderBoardStewardsText(rows []boardSteward) string {
	if len(rows) == 0 {
		return ""
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		parts := []string{row.Handle}
		if row.Topic != "" {
			parts = append(parts, row.Topic)
		}
		if row.Note != "" {
			parts = append(parts, row.Note)
		}
		lines = append(lines, strings.Join(parts, " | "))
	}
	return strings.Join(lines, "\n")
}

func normalizeThreadPoll(row threadPoll) threadPoll {
	row.Question = strings.TrimSpace(row.Question)
	row.Options = normalizeStringLines(row.Options, 8, 100)
	if row.Votes == nil {
		row.Votes = map[string]int{}
	}
	if row.CreatedBy == "" {
		row.CreatedBy = "caller"
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	}
	return row
}

func normalizeMessageRevisions(rows []messageRevision) []messageRevision {
	out := make([]messageRevision, 0, len(rows))
	for _, row := range rows {
		row.Subject = strings.TrimSpace(row.Subject)
		row.Body = strings.TrimSpace(row.Body)
		row.EditedBy = strings.TrimSpace(row.EditedBy)
		if row.Subject == "" && row.Body == "" {
			continue
		}
		if row.EditedBy == "" {
			row.EditedBy = "editor"
		}
		if row.EditedAt.IsZero() {
			row.EditedAt = time.Now().UTC()
		}
		out = append(out, row)
		if len(out) >= 12 {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EditedAt.After(out[j].EditedAt) })
	return out
}

func normalizeBestOfWeekEntries(rows []bestOfWeekEntry) []bestOfWeekEntry {
	out := make([]bestOfWeekEntry, 0, len(rows))
	seen := map[int64]bool{}
	for _, row := range rows {
		if row.MessageID <= 0 || seen[row.MessageID] {
			continue
		}
		seen[row.MessageID] = true
		row.Note = cleanOneLiner(row.Note, 120)
		row.AddedBy = cleanOneLiner(row.AddedBy, 48)
		if row.AddedAt.IsZero() {
			row.AddedAt = time.Now().UTC()
		}
		out = append(out, row)
		if len(out) >= 12 {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AddedAt.After(out[j].AddedAt) })
	return out
}

func (a *webApp) loadThreadLifecycleStates() map[int64]threadLifecycleState {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingThreadLifecycleStates)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("boards.lifecycle", fmt.Errorf("decode thread lifecycle states: %w", err))
		return nil
	}
	out := map[int64]threadLifecycleState{}
	for rawID, rawState := range decoded {
		threadID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || threadID <= 0 {
			continue
		}
		state := normalizeThreadLifecycleState(rawState)
		if state == threadLifecycleActive {
			continue
		}
		out[threadID] = state
	}
	return out
}

func (a *webApp) persistThreadLifecycleStates(rows map[int64]threadLifecycleState) {
	if a.adminRepo == nil {
		return
	}
	encoded := map[string]string{}
	for threadID, state := range rows {
		if threadID <= 0 {
			continue
		}
		state = normalizeThreadLifecycleState(string(state))
		if state == threadLifecycleActive {
			continue
		}
		encoded[strconv.FormatInt(threadID, 10)] = string(state)
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("boards.lifecycle", fmt.Errorf("encode thread lifecycle states: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingThreadLifecycleStates, body)
}

func (a *webApp) setThreadLifecycleState(threadID int64, state threadLifecycleState) {
	if threadID <= 0 {
		return
	}
	rows := a.loadThreadLifecycleStates()
	if rows == nil {
		rows = map[int64]threadLifecycleState{}
	}
	state = normalizeThreadLifecycleState(string(state))
	if state == threadLifecycleActive {
		delete(rows, threadID)
	} else {
		rows[threadID] = state
	}
	a.persistThreadLifecycleStates(rows)
	if a.msgRepo != nil {
		_ = a.msgRepo.SetThreadLocked(threadID, state == threadLifecycleFrozen)
	}
}

func (a *webApp) threadLifecycleStateFor(threadID int64) threadLifecycleState {
	return a.threadLifecycleStateFromCache(threadID, a.loadThreadLifecycleStates())
}

func (a *webApp) threadLifecycleStateFromCache(threadID int64, states map[int64]threadLifecycleState) threadLifecycleState {
	if threadID <= 0 {
		return threadLifecycleActive
	}
	if a.msgRepo != nil {
		if locked, err := a.msgRepo.IsThreadLocked(threadID); err == nil && locked {
			return threadLifecycleFrozen
		}
	}
	if states == nil {
		return threadLifecycleActive
	}
	if state, ok := states[threadID]; ok {
		return normalizeThreadLifecycleState(string(state))
	}
	return threadLifecycleActive
}

func (a *webApp) loadBoardWelcomeKits() map[int64]boardWelcomeKit {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingBoardWelcomeKits)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := map[string]boardWelcomeKit{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("boards.welcome", fmt.Errorf("decode welcome kits: %w", err))
		return nil
	}
	out := map[int64]boardWelcomeKit{}
	for rawID, row := range decoded {
		boardID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || boardID <= 0 {
			continue
		}
		row = normalizeBoardWelcomeKit(row)
		if row.Intro == "" && len(row.SeedPrompts) == 0 && len(row.StarterThreads) == 0 {
			continue
		}
		out[boardID] = row
	}
	return out
}

func (a *webApp) persistBoardWelcomeKits(rows map[int64]boardWelcomeKit) {
	if a.adminRepo == nil {
		return
	}
	encoded := map[string]boardWelcomeKit{}
	for boardID, row := range rows {
		if boardID <= 0 {
			continue
		}
		row = normalizeBoardWelcomeKit(row)
		if row.Intro == "" && len(row.SeedPrompts) == 0 && len(row.StarterThreads) == 0 {
			continue
		}
		encoded[strconv.FormatInt(boardID, 10)] = row
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("boards.welcome", fmt.Errorf("encode welcome kits: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingBoardWelcomeKits, body)
}

func (a *webApp) boardWelcomeKitFor(boardID int64) boardWelcomeKit {
	rows := a.loadBoardWelcomeKits()
	if rows == nil {
		return boardWelcomeKit{}
	}
	return rows[boardID]
}

func (a *webApp) setBoardWelcomeKit(boardID int64, row boardWelcomeKit) {
	if boardID <= 0 {
		return
	}
	rows := a.loadBoardWelcomeKits()
	if rows == nil {
		rows = map[int64]boardWelcomeKit{}
	}
	row = normalizeBoardWelcomeKit(row)
	if row.Intro == "" && len(row.SeedPrompts) == 0 && len(row.StarterThreads) == 0 {
		delete(rows, boardID)
	} else {
		rows[boardID] = row
	}
	a.persistBoardWelcomeKits(rows)
}

func (a *webApp) loadBoardStaffNotes() map[int64]boardStaffNote {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingBoardStaffNotes)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := map[string]boardStaffNote{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("boards.staff_notes", fmt.Errorf("decode board staff notes: %w", err))
		return nil
	}
	out := map[int64]boardStaffNote{}
	for rawID, row := range decoded {
		boardID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || boardID <= 0 {
			continue
		}
		row = normalizeBoardStaffNote(row)
		if row.Note == "" {
			continue
		}
		out[boardID] = row
	}
	return out
}

func (a *webApp) persistBoardStaffNotes(rows map[int64]boardStaffNote) {
	if a.adminRepo == nil {
		return
	}
	encoded := map[string]boardStaffNote{}
	for boardID, row := range rows {
		if boardID <= 0 {
			continue
		}
		row = normalizeBoardStaffNote(row)
		if row.Note == "" {
			continue
		}
		encoded[strconv.FormatInt(boardID, 10)] = row
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("boards.staff_notes", fmt.Errorf("encode board staff notes: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingBoardStaffNotes, body)
}

func (a *webApp) boardStaffNoteFor(boardID int64) boardStaffNote {
	rows := a.loadBoardStaffNotes()
	if rows == nil {
		return boardStaffNote{}
	}
	return rows[boardID]
}

func (a *webApp) setBoardStaffNote(boardID int64, row boardStaffNote) {
	if boardID <= 0 {
		return
	}
	rows := a.loadBoardStaffNotes()
	if rows == nil {
		rows = map[int64]boardStaffNote{}
	}
	row = normalizeBoardStaffNote(row)
	if row.Note == "" {
		delete(rows, boardID)
	} else {
		rows[boardID] = row
	}
	a.persistBoardStaffNotes(rows)
}

func (a *webApp) loadBoardStewards() map[int64][]boardSteward {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingBoardStewards)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := map[string][]boardSteward{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("boards.stewards", fmt.Errorf("decode board stewards: %w", err))
		return nil
	}
	out := map[int64][]boardSteward{}
	for rawID, rows := range decoded {
		boardID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || boardID <= 0 {
			continue
		}
		rows = normalizeBoardStewards(rows)
		if len(rows) == 0 {
			continue
		}
		out[boardID] = rows
	}
	return out
}

func (a *webApp) persistBoardStewards(rows map[int64][]boardSteward) {
	if a.adminRepo == nil {
		return
	}
	encoded := map[string][]boardSteward{}
	for boardID, stewards := range rows {
		if boardID <= 0 {
			continue
		}
		stewards = normalizeBoardStewards(stewards)
		if len(stewards) == 0 {
			continue
		}
		encoded[strconv.FormatInt(boardID, 10)] = stewards
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("boards.stewards", fmt.Errorf("encode board stewards: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingBoardStewards, body)
}

func (a *webApp) boardStewardsFor(boardID int64) []boardSteward {
	rows := a.loadBoardStewards()
	if rows == nil {
		return nil
	}
	return rows[boardID]
}

func (a *webApp) setBoardStewards(boardID int64, rows []boardSteward) {
	if boardID <= 0 {
		return
	}
	all := a.loadBoardStewards()
	if all == nil {
		all = map[int64][]boardSteward{}
	}
	rows = normalizeBoardStewards(rows)
	if len(rows) == 0 {
		delete(all, boardID)
	} else {
		all[boardID] = rows
	}
	a.persistBoardStewards(all)
}

func (a *webApp) loadThreadPolls() map[int64]threadPoll {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingThreadPolls)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := map[string]threadPoll{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("boards.polls", fmt.Errorf("decode thread polls: %w", err))
		return nil
	}
	out := map[int64]threadPoll{}
	for rawID, row := range decoded {
		threadID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || threadID <= 0 {
			continue
		}
		row = normalizeThreadPoll(row)
		if row.Question == "" || len(row.Options) < 2 {
			continue
		}
		out[threadID] = row
	}
	return out
}

func (a *webApp) persistThreadPolls(rows map[int64]threadPoll) {
	if a.adminRepo == nil {
		return
	}
	encoded := map[string]threadPoll{}
	for threadID, row := range rows {
		if threadID <= 0 {
			continue
		}
		row = normalizeThreadPoll(row)
		if row.Question == "" || len(row.Options) < 2 {
			continue
		}
		encoded[strconv.FormatInt(threadID, 10)] = row
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("boards.polls", fmt.Errorf("encode thread polls: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingThreadPolls, body)
}

func (a *webApp) threadPollFor(threadID int64) (threadPoll, bool) {
	rows := a.loadThreadPolls()
	if rows == nil {
		return threadPoll{}, false
	}
	row, ok := rows[threadID]
	return row, ok
}

func (a *webApp) setThreadPoll(threadID int64, row threadPoll) {
	if threadID <= 0 {
		return
	}
	rows := a.loadThreadPolls()
	if rows == nil {
		rows = map[int64]threadPoll{}
	}
	row = normalizeThreadPoll(row)
	if row.Question == "" || len(row.Options) < 2 {
		delete(rows, threadID)
	} else {
		rows[threadID] = row
	}
	a.persistThreadPolls(rows)
}

func (a *webApp) voteThreadPoll(threadID int64, handle string, option int) bool {
	handle = normalizeHandleKey(handle)
	if threadID <= 0 || handle == "" {
		return false
	}
	rows := a.loadThreadPolls()
	if rows == nil {
		return false
	}
	row, ok := rows[threadID]
	if !ok {
		return false
	}
	row = normalizeThreadPoll(row)
	if row.Closed || option < 0 || option >= len(row.Options) {
		return false
	}
	if row.Votes == nil {
		row.Votes = map[string]int{}
	}
	row.Votes[handle] = option
	rows[threadID] = row
	a.persistThreadPolls(rows)
	return true
}

func pollVoteTotals(row threadPoll) []int {
	totals := make([]int, len(row.Options))
	for _, option := range row.Votes {
		if option >= 0 && option < len(totals) {
			totals[option]++
		}
	}
	return totals
}

func (a *webApp) loadMessageRevisions() map[int64][]messageRevision {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingMessageRevisions)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := map[string][]messageRevision{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("boards.revisions", fmt.Errorf("decode message revisions: %w", err))
		return nil
	}
	out := map[int64][]messageRevision{}
	for rawID, rows := range decoded {
		messageID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || messageID <= 0 {
			continue
		}
		rows = normalizeMessageRevisions(rows)
		if len(rows) == 0 {
			continue
		}
		out[messageID] = rows
	}
	return out
}

func (a *webApp) persistMessageRevisions(rows map[int64][]messageRevision) {
	if a.adminRepo == nil {
		return
	}
	encoded := map[string][]messageRevision{}
	for messageID, revisions := range rows {
		if messageID <= 0 {
			continue
		}
		revisions = normalizeMessageRevisions(revisions)
		if len(revisions) == 0 {
			continue
		}
		encoded[strconv.FormatInt(messageID, 10)] = revisions
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("boards.revisions", fmt.Errorf("encode message revisions: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingMessageRevisions, body)
}

func (a *webApp) messageRevisionsFor(messageID int64) []messageRevision {
	rows := a.loadMessageRevisions()
	if rows == nil {
		return nil
	}
	return rows[messageID]
}

func (a *webApp) appendMessageRevision(messageID int64, row messageRevision) {
	if messageID <= 0 {
		return
	}
	rows := a.loadMessageRevisions()
	if rows == nil {
		rows = map[int64][]messageRevision{}
	}
	revisions := append([]messageRevision{row}, rows[messageID]...)
	rows[messageID] = normalizeMessageRevisions(revisions)
	a.persistMessageRevisions(rows)
}

func (a *webApp) loadBestOfWeekEntries() []bestOfWeekEntry {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingBestOfWeek)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []bestOfWeekEntry
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("boards.best_of_week", fmt.Errorf("decode best of week picks: %w", err))
		return nil
	}
	return normalizeBestOfWeekEntries(rows)
}

func (a *webApp) persistBestOfWeekEntries(rows []bestOfWeekEntry) {
	if a.adminRepo == nil {
		return
	}
	rows = normalizeBestOfWeekEntries(rows)
	body := ""
	if len(rows) > 0 {
		raw, err := json.Marshal(rows)
		if err != nil {
			a.addAppError("boards.best_of_week", fmt.Errorf("encode best of week picks: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingBestOfWeek, body)
}

func parseBestOfWeekEntries(raw, actor string) []bestOfWeekEntry {
	lines := splitTrimmedLines(raw, 12, 180)
	out := make([]bestOfWeekEntry, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "|")
		messageID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil || messageID <= 0 {
			continue
		}
		entry := bestOfWeekEntry{
			MessageID: messageID,
			AddedBy:   cleanOneLiner(actor, 48),
			AddedAt:   time.Now().UTC(),
		}
		if len(parts) > 1 {
			entry.Note = strings.Join(parts[1:], "|")
		}
		out = append(out, entry)
	}
	return normalizeBestOfWeekEntries(out)
}

func renderBestOfWeekText(rows []bestOfWeekEntry) string {
	if len(rows) == 0 {
		return ""
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		line := strconv.FormatInt(row.MessageID, 10)
		if row.Note != "" {
			line += " | " + row.Note
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func parseLegacyArchiveDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"01/02/2006 15:04",
	} {
		if at, err := time.Parse(layout, raw); err == nil {
			return at.UTC()
		}
	}
	return time.Time{}
}

func parseLegacyArchive(raw string) []legacyArchiveMessage {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "\n---\n")
	out := make([]legacyArchiveMessage, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lines := strings.Split(part, "\n")
		row := legacyArchiveMessage{}
		bodyStart := 0
		foundHeader := false
		for idx, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				bodyStart = idx + 1
				if foundHeader {
					break
				}
				continue
			}
			switch {
			case strings.HasPrefix(strings.ToLower(line), "subject:"):
				row.Subject = strings.TrimSpace(line[len("subject:"):])
				foundHeader = true
			case strings.HasPrefix(strings.ToLower(line), "from:"):
				row.From = strings.TrimSpace(line[len("from:"):])
				foundHeader = true
			case strings.HasPrefix(strings.ToLower(line), "date:"):
				row.Date = parseLegacyArchiveDate(strings.TrimSpace(line[len("date:"):]))
				foundHeader = true
			default:
				if foundHeader {
					bodyStart = idx
				} else {
					bodyStart = 0
				}
				goto body
			}
		}
	body:
		body := strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
		if body == "" && !foundHeader {
			body = strings.TrimSpace(part)
		}
		row.Subject = cleanOneLiner(defaultIfBlank(row.Subject, "Legacy Import"), 96)
		row.From = cleanOneLiner(row.From, 48)
		row.Body = body
		if row.Body == "" {
			continue
		}
		out = append(out, row)
		if len(out) >= 100 {
			break
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

func formatLocalDateTimeValue(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.In(time.Local).Format("2006-01-02T15:04")
}

func findCommunityEvent(rows []communityEvent, id string) (communityEvent, int, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return communityEvent{}, -1, false
	}
	for idx, row := range rows {
		if strings.TrimSpace(row.ID) == id {
			return row, idx, true
		}
	}
	return communityEvent{}, -1, false
}

func communityEventTemplate(key string, now time.Time) (communityEvent, string, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	if now.IsZero() {
		now = time.Now()
	}
	baseStart := now.In(time.Local).Add(2 * time.Hour).Truncate(30 * time.Minute)
	switch key {
	case "tournament":
		start := time.Date(baseStart.Year(), baseStart.Month(), baseStart.Day(), 20, 0, 0, 0, time.Local)
		for start.Weekday() != time.Friday {
			start = start.AddDate(0, 0, 1)
		}
		return communityEvent{
			Title:       "Tournament Night",
			Category:    "tournament",
			StartsAt:    start,
			EndsAt:      start.Add(2 * time.Hour),
			Recurrence:  "weekly",
			RepeatUntil: start.AddDate(0, 2, 0),
			Location:    "/doors",
			Host:        "sysop",
			Audience:    "ranked callers",
			Description: "Weekly ladder or bracket night with a clear join path, host, and scoreboard follow-up.",
			Link:        "/tournaments",
		}, "Tournament template loaded.", true
	case "social":
		start := time.Date(baseStart.Year(), baseStart.Month(), baseStart.Day(), 21, 0, 0, 0, time.Local)
		return communityEvent{
			Title:       "Lobby Net",
			Category:    "social",
			StartsAt:    start,
			EndsAt:      start.Add(time.Hour),
			Recurrence:  "weekdays",
			RepeatUntil: start.AddDate(0, 1, 0),
			Location:    "#lobby",
			Host:        "sysop",
			Audience:    "all callers",
			Description: "Short social net to keep the live layer warm and give callers a predictable check-in window.",
			Link:        "/chat",
		}, "Social net template loaded.", true
	case "content":
		start := time.Date(baseStart.Year(), baseStart.Month(), baseStart.Day(), 19, 30, 0, 0, time.Local).AddDate(0, 0, 1)
		return communityEvent{
			Title:       "Featured Content Drop",
			Category:    "content",
			StartsAt:    start,
			EndsAt:      start.Add(90 * time.Minute),
			Recurrence:  "weekly",
			RepeatUntil: start.AddDate(0, 2, 0),
			Location:    "/bulletins",
			Host:        "sysop",
			Audience:    "all callers",
			Description: "Publish a featured thread, file, or bulletin with a specific time so the board feels programmed.",
			Link:        "/bulletins",
		}, "Content-drop template loaded.", true
	case "ops":
		start := time.Date(baseStart.Year(), baseStart.Month(), baseStart.Day(), 18, 0, 0, 0, time.Local)
		if start.Weekday() == time.Saturday {
			start = start.AddDate(0, 0, 2)
		}
		if start.Weekday() == time.Sunday {
			start = start.AddDate(0, 0, 1)
		}
		return communityEvent{
			Title:       "Sysop Review Window",
			Category:    "ops",
			StartsAt:    start,
			EndsAt:      start.Add(45 * time.Minute),
			Recurrence:  "weekdays",
			RepeatUntil: start.AddDate(0, 1, 0),
			Location:    "/admin/ops",
			Host:        "sysop",
			Audience:    "sysops",
			Description: "Short operator review for launch checks, runtime issues, scheduled content, and caller escalations.",
			Link:        "/admin/ops",
		}, "Ops template loaded.", true
	default:
		return communityEvent{}, "", false
	}
}

func renderCommunityEventPreview(row communityEvent, count int) string {
	if row.StartsAt.IsZero() {
		return `<p class="wolfbbs-muted">Set a valid start time to preview how this event will land on the public calendar.</p>`
	}
	occurrences := expandCommunityEvent(row, row.StartsAt.UTC().Add(-time.Minute), row.StartsAt.UTC().AddDate(0, 3, 0), count)
	if len(occurrences) == 0 {
		return `<p class="wolfbbs-muted">No occurrences fall inside the current preview window.</p>`
	}
	var out strings.Builder
	out.WriteString(`<ul class="wolfbbs-list-clean">`)
	for _, occurrence := range occurrences {
		meta := []string{formatCommunityEventWindow(occurrence)}
		if recurrence := recurrenceSummary(occurrence); recurrence != "" {
			meta = append(meta, recurrence)
		}
		if strings.TrimSpace(occurrence.Location) != "" {
			meta = append(meta, occurrence.Location)
		}
		out.WriteString(`<li><strong>` + htmlEscape(occurrence.Title) + `</strong><br><span class="wolfbbs-muted">` + htmlEscape(strings.Join(meta, " | ")) + `</span></li>`)
	}
	out.WriteString(`</ul>`)
	return out.String()
}

func findFileFilterByID(rows []domain.FileFilter, id int64) (domain.FileFilter, bool) {
	if id <= 0 {
		return domain.FileFilter{}, false
	}
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return domain.FileFilter{}, false
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

func normalizeScheduledBulletin(row scheduledBulletin) (scheduledBulletin, bool) {
	row.ID = strings.TrimSpace(row.ID)
	row.Title = strings.TrimSpace(row.Title)
	row.Body = strings.TrimSpace(row.Body)
	row.Link = strings.TrimSpace(row.Link)
	row.Audience = strings.TrimSpace(row.Audience)
	row.CreatedBy = strings.TrimSpace(row.CreatedBy)
	if row.ID == "" || row.Title == "" || row.Body == "" || row.StartsAt.IsZero() {
		return scheduledBulletin{}, false
	}
	row.StartsAt = row.StartsAt.UTC()
	if !row.EndsAt.IsZero() {
		row.EndsAt = row.EndsAt.UTC()
		if row.EndsAt.Before(row.StartsAt) {
			row.EndsAt = time.Time{}
		}
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = row.StartsAt
	}
	if row.Audience == "" {
		row.Audience = "all callers"
	}
	return row, true
}

func (a *webApp) loadScheduledBulletins() []scheduledBulletin {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingScheduledBulletins)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []scheduledBulletin
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("scheduled.bulletins", fmt.Errorf("decode scheduled bulletins: %w", err))
		return nil
	}
	out := make([]scheduledBulletin, 0, len(rows))
	for _, row := range rows {
		normalized, ok := normalizeScheduledBulletin(row)
		if ok {
			out = append(out, normalized)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartsAt.Equal(out[j].StartsAt) {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		}
		return out[i].StartsAt.Before(out[j].StartsAt)
	})
	return out
}

func (a *webApp) persistScheduledBulletins(rows []scheduledBulletin) {
	if a.adminRepo == nil {
		return
	}
	clean := make([]scheduledBulletin, 0, len(rows))
	for _, row := range rows {
		normalized, ok := normalizeScheduledBulletin(row)
		if ok {
			clean = append(clean, normalized)
		}
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i].StartsAt.Equal(clean[j].StartsAt) {
			return strings.ToLower(clean[i].Title) < strings.ToLower(clean[j].Title)
		}
		return clean[i].StartsAt.Before(clean[j].StartsAt)
	})
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			a.addAppError("scheduled.bulletins", fmt.Errorf("encode scheduled bulletins: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingScheduledBulletins, body)
}

func findScheduledBulletin(rows []scheduledBulletin, id string) (scheduledBulletin, int, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return scheduledBulletin{}, -1, false
	}
	for idx, row := range rows {
		if row.ID == id {
			return row, idx, true
		}
	}
	return scheduledBulletin{}, -1, false
}

func (a *webApp) activeScheduledBulletins(now time.Time, limit int) []scheduledBulletin {
	rows := a.loadScheduledBulletins()
	out := make([]scheduledBulletin, 0, len(rows))
	for _, row := range rows {
		if row.StartsAt.After(now) {
			continue
		}
		if !row.EndsAt.IsZero() && row.EndsAt.Before(now) {
			continue
		}
		out = append(out, row)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (a *webApp) upcomingScheduledBulletins(now time.Time, limit int) []scheduledBulletin {
	rows := a.loadScheduledBulletins()
	out := make([]scheduledBulletin, 0, len(rows))
	for _, row := range rows {
		if !row.StartsAt.After(now) {
			continue
		}
		out = append(out, row)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func bulletinAckSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingBulletinAckRoot + handle
}

func (a *webApp) loadBulletinAcks(handle string) map[string]time.Time {
	if a.adminRepo == nil {
		return nil
	}
	key := bulletinAckSettingKey(handle)
	if key == "" {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("bulletin.acks", fmt.Errorf("decode bulletin acks for %s: %w", handle, err))
		return nil
	}
	out := map[string]time.Time{}
	for id, value := range decoded {
		id = strings.TrimSpace(id)
		at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
		if id == "" || err != nil {
			continue
		}
		out[id] = at.UTC()
	}
	return out
}

func (a *webApp) persistBulletinAcks(handle string, rows map[string]time.Time) {
	key := bulletinAckSettingKey(handle)
	if key == "" {
		return
	}
	encoded := map[string]string{}
	for id, at := range rows {
		id = strings.TrimSpace(id)
		if id == "" || at.IsZero() {
			continue
		}
		encoded[id] = at.UTC().Format(time.RFC3339Nano)
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("bulletin.acks", fmt.Errorf("encode bulletin acks for %s: %w", handle, err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(key, body)
}

func (a *webApp) acknowledgeBulletin(handle, id string) {
	handle = normalizeHandleKey(handle)
	id = strings.TrimSpace(id)
	if handle == "" || id == "" {
		return
	}
	rows := a.loadBulletinAcks(handle)
	if rows == nil {
		rows = map[string]time.Time{}
	}
	rows[id] = time.Now().UTC()
	a.persistBulletinAcks(handle, rows)
}

func (a *webApp) bulletinAckedAt(handle, id string) time.Time {
	rows := a.loadBulletinAcks(handle)
	if rows == nil {
		return time.Time{}
	}
	return rows[strings.TrimSpace(id)]
}

func (a *webApp) loadBulletinAckSummary() map[string]bulletinAckSummary {
	if a.adminRepo == nil {
		return nil
	}
	settings, err := a.adminRepo.ListSystemSettings()
	if err != nil {
		return nil
	}
	out := map[string]bulletinAckSummary{}
	for key, raw := range settings {
		if !strings.HasPrefix(key, sysSettingBulletinAckRoot) || strings.TrimSpace(raw) == "" {
			continue
		}
		decoded := map[string]string{}
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			continue
		}
		for id, value := range decoded {
			at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
			if strings.TrimSpace(id) == "" || err != nil {
				continue
			}
			row := out[id]
			row.Count++
			if at.After(row.LastAt) {
				row.LastAt = at.UTC()
			}
			out[id] = row
		}
	}
	return out
}

func loadStaffEscalationsFromSettings(raw string) []staffEscalationEntry {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var decoded []staffEscalationEntry
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	out := make([]staffEscalationEntry, 0, len(decoded))
	for _, row := range decoded {
		row.ID = strings.TrimSpace(row.ID)
		row.Handle = strings.TrimSpace(row.Handle)
		row.Actor = strings.TrimSpace(row.Actor)
		row.Note = cleanOneLiner(row.Note, 160)
		if row.ID == "" || row.Handle == "" || row.Note == "" || row.CreatedAt.IsZero() {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ResolvedAt.IsZero() != out[j].ResolvedAt.IsZero() {
			return out[i].ResolvedAt.IsZero()
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > maxStaffEscalations {
		out = out[:maxStaffEscalations]
	}
	return out
}

func (a *webApp) loadStaffEscalations() []staffEscalationEntry {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingStaffEscalations)
	if err != nil {
		return nil
	}
	return loadStaffEscalationsFromSettings(raw)
}

func (a *webApp) persistStaffEscalations(rows []staffEscalationEntry) {
	if a.adminRepo == nil {
		return
	}
	clean := make([]staffEscalationEntry, 0, len(rows))
	for _, row := range rows {
		row.ID = strings.TrimSpace(row.ID)
		row.Handle = strings.TrimSpace(row.Handle)
		row.Actor = strings.TrimSpace(row.Actor)
		row.Note = cleanOneLiner(row.Note, 160)
		if row.ID == "" || row.Handle == "" || row.Note == "" || row.CreatedAt.IsZero() {
			continue
		}
		clean = append(clean, row)
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i].ResolvedAt.IsZero() != clean[j].ResolvedAt.IsZero() {
			return clean[i].ResolvedAt.IsZero()
		}
		return clean[i].CreatedAt.After(clean[j].CreatedAt)
	})
	if len(clean) > maxStaffEscalations {
		clean = clean[:maxStaffEscalations]
	}
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			a.addAppError("staff.escalations", fmt.Errorf("encode escalations: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingStaffEscalations, body)
}

func (a *webApp) queueStaffEscalation(actor, handle, note string) {
	handle = strings.TrimSpace(handle)
	note = cleanOneLiner(note, 160)
	if handle == "" || note == "" {
		return
	}
	rows := a.loadStaffEscalations()
	resolvedAt := time.Time{}
	for idx, row := range rows {
		if strings.EqualFold(row.Handle, handle) && row.ResolvedAt.IsZero() {
			rows[idx].Actor = strings.TrimSpace(actor)
			rows[idx].Note = note
			rows[idx].CreatedAt = time.Now().UTC()
			rows[idx].ResolvedAt = resolvedAt
			a.persistStaffEscalations(rows)
			return
		}
	}
	rows = append([]staffEscalationEntry{{
		ID:        randomEventID(),
		Handle:    handle,
		Actor:     strings.TrimSpace(actor),
		Note:      note,
		CreatedAt: time.Now().UTC(),
	}}, rows...)
	a.persistStaffEscalations(rows)
}

func (a *webApp) resolveStaffEscalation(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	rows := a.loadStaffEscalations()
	updated := false
	now := time.Now().UTC()
	for idx, row := range rows {
		if row.ID != id || !row.ResolvedAt.IsZero() {
			continue
		}
		rows[idx].ResolvedAt = now
		updated = true
		break
	}
	if updated {
		a.persistStaffEscalations(rows)
	}
	return updated
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

func (a *webApp) findEventOccurrenceByID(id string, now time.Time) (communityEvent, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return communityEvent{}, false
	}
	windowStart := now.Add(-30 * 24 * time.Hour)
	windowEnd := now.Add(180 * 24 * time.Hour)
	for _, row := range a.loadCommunityEvents() {
		for _, occurrence := range expandCommunityEvent(row, windowStart, windowEnd, 256) {
			if occurrence.ID == id {
				return occurrence, true
			}
		}
	}
	return communityEvent{}, false
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

func attentionSnoozeSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingAttentionSnoozeRoot + handle
}

func (a *webApp) ensureAttentionSnoozeLoaded(handle string) {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return
	}

	a.Lock()
	if a.attentionSnoozeLoaded == nil {
		a.attentionSnoozeLoaded = map[string]bool{}
	}
	if a.attentionSnoozed == nil {
		a.attentionSnoozed = map[string]map[string]time.Time{}
	}
	if a.attentionSnoozeLoaded[handle] {
		a.Unlock()
		return
	}
	a.attentionSnoozeLoaded[handle] = true
	a.Unlock()

	rows := map[string]time.Time{}
	if a.adminRepo != nil {
		raw, err := a.adminRepo.GetSystemSetting(attentionSnoozeSettingKey(handle))
		if err == nil && strings.TrimSpace(raw) != "" {
			decoded := map[string]string{}
			if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
				a.addAppError("attention.snooze", fmt.Errorf("decode snoozes for %s: %w", handle, err))
			} else {
				now := time.Now().UTC()
				for key, value := range decoded {
					at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
					if err != nil || !at.After(now) {
						continue
					}
					rows[strings.TrimSpace(key)] = at.UTC()
				}
			}
		}
	}

	a.Lock()
	if a.attentionSnoozed == nil {
		a.attentionSnoozed = map[string]map[string]time.Time{}
	}
	if len(rows) == 0 {
		delete(a.attentionSnoozed, handle)
	} else {
		a.attentionSnoozed[handle] = rows
	}
	a.Unlock()
}

func (a *webApp) persistAttentionSnoozes(handle string) {
	handle = normalizeHandleKey(handle)
	if handle == "" || a.adminRepo == nil {
		return
	}
	a.ensureAttentionSnoozeLoaded(handle)

	now := time.Now().UTC()
	type row struct {
		Key string
		At  time.Time
	}

	a.Lock()
	source := a.attentionSnoozed[handle]
	rows := make([]row, 0, len(source))
	for key, at := range source {
		key = strings.TrimSpace(key)
		if key == "" || !at.After(now) {
			continue
		}
		rows = append(rows, row{Key: key, At: at.UTC()})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].At.Equal(rows[j].At) {
			return rows[i].Key < rows[j].Key
		}
		return rows[i].At.After(rows[j].At)
	})
	if len(rows) > maxAttentionSnoozedItems {
		rows = rows[:maxAttentionSnoozedItems]
	}
	if a.attentionSnoozed == nil {
		a.attentionSnoozed = map[string]map[string]time.Time{}
	}
	if len(rows) == 0 {
		delete(a.attentionSnoozed, handle)
	} else {
		pruned := make(map[string]time.Time, len(rows))
		for _, row := range rows {
			pruned[row.Key] = row.At
		}
		a.attentionSnoozed[handle] = pruned
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
			a.addAppError("attention.snooze", fmt.Errorf("encode snoozes for %s: %w", handle, err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(attentionSnoozeSettingKey(handle), body)
}

func (a *webApp) attentionSnoozedUntil(handle, key string) time.Time {
	key = strings.TrimSpace(key)
	if key == "" {
		return time.Time{}
	}
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return time.Time{}
	}
	a.ensureAttentionSnoozeLoaded(handle)
	persist := false
	a.Lock()
	rows := a.attentionSnoozed[handle]
	if len(rows) == 0 {
		a.Unlock()
		return time.Time{}
	}
	at, ok := rows[key]
	if !ok {
		a.Unlock()
		return time.Time{}
	}
	if !at.After(time.Now().UTC()) {
		delete(rows, key)
		if len(rows) == 0 {
			delete(a.attentionSnoozed, handle)
		}
		persist = true
	}
	a.Unlock()
	if persist {
		a.persistAttentionSnoozes(handle)
		return time.Time{}
	}
	return at
}

func (a *webApp) snoozeAttentionItem(handle, key string, until time.Time) {
	handle = normalizeHandleKey(handle)
	key = strings.TrimSpace(key)
	if handle == "" || key == "" || until.IsZero() {
		return
	}
	a.ensureAttentionSnoozeLoaded(handle)
	a.Lock()
	if a.attentionSnoozed == nil {
		a.attentionSnoozed = map[string]map[string]time.Time{}
	}
	if a.attentionSnoozed[handle] == nil {
		a.attentionSnoozed[handle] = map[string]time.Time{}
	}
	a.attentionSnoozed[handle][key] = until.UTC()
	a.Unlock()
	a.persistAttentionSnoozes(handle)
}

func (a *webApp) clearAttentionSnooze(handle, key string) {
	handle = normalizeHandleKey(handle)
	key = strings.TrimSpace(key)
	if handle == "" || key == "" {
		return
	}
	a.ensureAttentionSnoozeLoaded(handle)
	a.Lock()
	if rows := a.attentionSnoozed[handle]; rows != nil {
		delete(rows, key)
		if len(rows) == 0 {
			delete(a.attentionSnoozed, handle)
		}
	}
	a.Unlock()
	a.persistAttentionSnoozes(handle)
}

func (a *webApp) attentionDismissedRows(handle string) map[string]time.Time {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return nil
	}
	a.ensureAttentionDismissalsLoaded(handle)
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	out := map[string]time.Time{}
	a.Lock()
	for key, at := range a.attentionDismissed[handle] {
		if strings.TrimSpace(key) == "" || !at.After(cutoff) {
			continue
		}
		out[key] = at.UTC()
	}
	a.Unlock()
	return out
}

func (a *webApp) attentionReadRows(handle string) map[string]time.Time {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return nil
	}
	a.ensureAttentionReadLoaded(handle)
	cutoff := time.Now().UTC().Add(-14 * 24 * time.Hour)
	out := map[string]time.Time{}
	a.Lock()
	for key, at := range a.attentionRead[handle] {
		if strings.TrimSpace(key) == "" || !at.After(cutoff) {
			continue
		}
		out[key] = at.UTC()
	}
	a.Unlock()
	return out
}

func (a *webApp) attentionSnoozeRows(handle string) map[string]time.Time {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return nil
	}
	a.ensureAttentionSnoozeLoaded(handle)
	now := time.Now().UTC()
	out := map[string]time.Time{}
	a.Lock()
	for key, at := range a.attentionSnoozed[handle] {
		if strings.TrimSpace(key) == "" || !at.After(now) {
			continue
		}
		out[key] = at.UTC()
	}
	a.Unlock()
	return out
}

func encodeTimeMap(rows map[string]time.Time) map[string]string {
	if len(rows) == 0 {
		return nil
	}
	out := make(map[string]string, len(rows))
	for key, at := range rows {
		key = strings.TrimSpace(key)
		if key == "" || at.IsZero() {
			continue
		}
		out[key] = at.UTC().Format(time.RFC3339Nano)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (a *webApp) buildAttentionExport(user *domain.User) attentionExportPayload {
	payload := attentionExportPayload{
		Version:                  "1",
		GeneratedAt:              time.Now().UTC(),
		DigestPreferences:        defaultDigestPreferences(),
		BoardSubscriptions:       map[string]string{},
		BoardQuietHours:          map[string]string{},
		RouteSeen:                map[string]string{},
		BulletinAcknowledgements: map[string]string{},
	}
	if user == nil {
		return payload
	}
	payload.Handle = user.Handle
	payload.Role = defaultIfBlank(rbac.NormalizeRole(user.Role), roleUser)
	payload.PresetRecommendation = recommendedAttentionPreset(user).Name
	payload.DigestPreferences = a.loadDigestPreferences(user.Handle)
	for boardID, mode := range a.boardSubscriptions(user.Handle) {
		payload.BoardSubscriptions[strconv.FormatInt(boardID, 10)] = string(mode)
	}
	for boardID, window := range a.loadBoardQuietHours(user.Handle) {
		payload.BoardQuietHours[strconv.FormatInt(boardID, 10)] = formatQuietHoursWindow(window)
	}
	payload.RouteSeen = encodeTimeMap(a.loadRouteSeen(user.Handle))
	payload.BulletinAcknowledgements = encodeTimeMap(a.loadBulletinAcks(user.Handle))
	payload.Attention.Dismissed = encodeTimeMap(a.attentionDismissedRows(user.Handle))
	payload.Attention.Read = encodeTimeMap(a.attentionReadRows(user.Handle))
	payload.Attention.Snoozed = encodeTimeMap(a.attentionSnoozeRows(user.Handle))
	if len(payload.BoardSubscriptions) == 0 {
		payload.BoardSubscriptions = nil
	}
	if len(payload.BoardQuietHours) == 0 {
		payload.BoardQuietHours = nil
	}
	return payload
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
	canModerateJSON := "false"
	currentHandleJSON := fmt.Sprintf("%q", strings.ToLower(strings.TrimSpace(user.Handle)))
	if a.hasRole(user, roleModerator) {
		canModerateJSON = "true"
	}

	modActions := ""
	csrfInput := a.csrfHiddenInput(r)
	if a.hasRole(user, roleModerator) {
		modActions = `
			<section id="mod" class="wolfbbs-card">
				<h2>Moderation</h2>
				<form id="modForm" action="/chat/moderation" method="POST" class="wolfbbs-inline-form" data-no-auto-busy="1">
					` + csrfInput + `
					<label>Channel:
						<input type="text" name="channel" value="#lobby">
					</label>
					<label>Target: <input type="text" name="target" placeholder="target" required></label>
					<label>Reason: <input type="text" name="reason"></label>
					<label>Duration (optional): <input type="text" name="duration" value="" placeholder="5m"></label>
					<select name="action">
						<option value="kick">Kick</option>
						<option value="mute">Mute</option>
						<option value="unmute">Unmute</option>
						<option value="ban">Ban</option>
						<option value="unban">Unban</option>
					</select>
					<button type="submit">Apply</button>
				</form>
				<p class="wolfbbs-muted">Use moderation from the live room when a channel needs intervention. Locked rooms are read-only for non-moderators.</p>
			</section>`
	}

	chatEmptyHelper := ""
	if len(a.chatSvc.History("#lobby", 1)) == 0 {
		chatEmptyHelper = a.renderRoleAwareEmptyState(user, "chat")
	}

	chatPage := `<!doctype html>
	<html>
	<body>
		<h1>` + htmlEscape(a.siteDisplayName()) + ` Chat</h1>
		<p><a href="/boards">boards</a> | <a href="/today">today</a> | <a href="/attention">attention</a> | <a href="/bookmarks">bookmarks</a> | <a href="/mail">mail</a> | <a href="/doors">doors</a> | <a href="/clubhouse">clubhouse</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
		` + pageMessageBlock(r) + `
		<p>Logged in as ` + user.Handle + `. Web chat and IRC are the same live conversation layer, so this page has to behave like a real client instead of a raw debug pane.</p>
		<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>#lobby</strong><span>default room</span></article><article class="wolfbbs-kpi-card"><strong>live</strong><span>shared with IRC bridge</span></article><article class="wolfbbs-kpi-card"><strong>Ctrl+L</strong><span>clear transcript</span></article><article class="wolfbbs-kpi-card"><strong>Enter</strong><span>send line</span></article></section>
		<section class="wolfbbs-helper-grid"><article class="wolfbbs-helper-card"><strong>Switch rooms without losing context</strong><p>The selector and custom channel box both move the transcript, online list, and moderation target together.</p></article><article class="wolfbbs-helper-card"><strong>Locked rooms are explicit</strong><p>If a channel is locked, the composer disables itself for non-moderators instead of failing only after submit.</p></article><article class="wolfbbs-helper-card"><strong>Reconnect is stateful</strong><p>The stream resumes after the last seen line so reconnects do not duplicate the transcript.</p></article></section>
		` + chatEmptyHelper + `
		<section class="wolfbbs-chat-layout">
			<article class="wolfbbs-card">
				<h2>Transcript</h2>
				<div class="wolfbbs-control-strip">
					<label>Channel
						<select id="channelSelect"></select>
					</label>
					<form id="customChannelForm" class="wolfbbs-inline-form">
						<label>Join or create room
							<input type="text" id="customChannel" placeholder="#ansi-lab" autocomplete="off">
						</label>
						<button type="submit">Open Channel</button>
					</form>
					<button type="button" id="chatReconnect">Reconnect Stream</button>
					<div class="wolfbbs-chat-toolbar-meta">
						<span id="chatStatePill" class="wolfbbs-chat-status-pill" data-state="idle">idle</span>
						<span id="chatMessageCount">0 lines</span>
					</div>
				</div>
				<div id="chat" class="wolfbbs-chat-pane" aria-live="polite"></div>
				<p id="chatStatus" style="font-family: monospace;">Ready.</p>
				<form id="sendForm" class="wolfbbs-inline-form">
					<label style="flex:1 1 420px">Message
						<input type="text" id="message" autocomplete="off" placeholder="Say something in the room...">
					</label>
					<button type="submit" id="sendButton">Send</button>
				</form>
			</article>
			<section class="wolfbbs-stack wolfbbs-chat-sidebar">
				<article class="wolfbbs-card">
					<h2>Room State</h2>
					<dl class="wolfbbs-meta-list"><div><dt>Current room</dt><dd id="chatCurrentChannel">#lobby</dd></div><div><dt>Mode</dt><dd id="chatMode">open</dd></div><div><dt>Online now</dt><dd id="onlineCount">0</dd></div><div><dt>Last sync</dt><dd id="chatLastSync">waiting</dd></div></dl>
					<div class="wolfbbs-channel-badges" id="channelBadges"></div>
				</article>
				<article class="wolfbbs-card">
					<h2>Who Is Here</h2>
					<div id="online">loading...</div>
					<p class="wolfbbs-muted">The list reflects the room you are actively viewing, including IRC callers in the same channel.</p>
				</article>
				<article class="wolfbbs-card">
					<h2>Fast Moves</h2>
					<div class="wolfbbs-action-grid"><a class="wolfbbs-action-card" href="/clubhouse"><strong>Clubhouse</strong><span>one-liners, neighboring boards, social layer</span></a><a class="wolfbbs-action-card" href="/attention"><strong>Attention</strong><span>return to direct follow-up</span></a><a class="wolfbbs-action-card" href="/connect"><strong>Connect</strong><span>SSH, IRC, and terminal access help</span></a></div>
				</article>
				` + modActions + `
			</section>
		</section>
		<script>
			const csrf = ` + csrfJSON + `;
			const canModerate = ` + canModerateJSON + `;
			const currentHandle = ` + currentHandleJSON + `;
			const streamState = {
				es: null,
				channel: '#lobby',
				lastSeenId: 0,
				reconnectTimer: null,
				locked: new Set(),
				statusHoldUntil: 0,
				statusHoldTimer: null,
			};
			let onlineRefreshTimer = null;
			let onlineRefreshInFlight = false;
			let chatComposerPrimed = false;
			function liveStatusText() {
				const readOnly = streamState.locked.has(streamState.channel) && !canModerate;
				if (readOnly) {
					return {msg: 'Live on ' + streamState.channel + ' (read-only)', state: 'warn'};
				}
				return {msg: 'Live on ' + streamState.channel, state: 'live'};
			}
			function setStatus(msg, state, opts) {
				const options = opts || {};
				const now = Date.now();
				const holdActive = streamState.statusHoldUntil > now;
				if (holdActive && !options.force) {
					const resolved = state || (String(msg || '').toLowerCase().includes('failed') ? 'error' : 'ready');
					if (resolved === 'live' || resolved === 'ready') {
						return;
					}
				}
				const el = document.getElementById('chatStatus');
				if (el) el.textContent = msg;
				const pill = document.getElementById('chatStatePill');
				if (pill) {
					const resolved = state || (String(msg || '').toLowerCase().includes('failed') ? 'error' : 'ready');
					pill.dataset.state = resolved;
					pill.textContent = resolved;
				}
				if (streamState.statusHoldTimer && options.clearHold) {
					window.clearTimeout(streamState.statusHoldTimer);
					streamState.statusHoldTimer = null;
					streamState.statusHoldUntil = 0;
				}
				if (options.stickyMs) {
					streamState.statusHoldUntil = now + options.stickyMs;
					if (streamState.statusHoldTimer) {
						window.clearTimeout(streamState.statusHoldTimer);
					}
					streamState.statusHoldTimer = window.setTimeout(() => {
						streamState.statusHoldUntil = 0;
						streamState.statusHoldTimer = null;
						const live = liveStatusText();
						setStatus(live.msg, live.state, {force: true});
					}, options.stickyMs);
				}
			}

			function msgValue(m, primary, legacy, fallback) {
				if (m && m[primary] !== undefined && m[primary] !== null && m[primary] !== '') return m[primary];
				if (m && legacy && m[legacy] !== undefined && m[legacy] !== null && m[legacy] !== '') return m[legacy];
				return fallback;
			}

			function presenceValue(m, primary, legacy, fallback) {
				if (m && m[primary] !== undefined && m[primary] !== null && m[primary] !== '') return m[primary];
				if (m && legacy && m[legacy] !== undefined && m[legacy] !== null && m[legacy] !== '') return m[legacy];
				return fallback;
			}

			function normalizeIncomingMessage(raw) {
				let m = raw;
				if (!m) return null;
				if (typeof m === 'string') {
					return {id: 0, from: 'system', body: m, created_at: '--:--:--', channel: streamState.channel};
				}
				if (typeof m.message === 'object' && m.message) m = m.message;
				if (typeof m.data === 'object' && m.data) m = m.data;
				if (typeof m.payload === 'object' && m.payload) m = m.payload;
				const from = msgValue(m, 'from', 'From', 'system');
				const body = msgValue(m, 'body', 'Body', msgValue(m, 'message', 'Message', ''));
				const createdAt = msgValue(m, 'created_at', 'CreatedAt', '--:--:--');
				const channel = msgValue(m, 'channel', 'Channel', streamState.channel);
				const id = Number(msgValue(m, 'id', 'ID', 0)) || 0;
				if (!from && !body) {
					return null;
				}
				return {id: id, from: from, body: body, created_at: createdAt, channel: channel};
			}

			function updateMessageCount() {
				const box = document.getElementById('chat');
				const counter = document.getElementById('chatMessageCount');
				if (!box || !counter) return;
				const count = box.querySelectorAll('.wolfbbs-chat-line').length;
				counter.textContent = count + (count === 1 ? ' line' : ' lines');
			}

			function formatLine(m) {
				const stamp = msgValue(m, 'created_at', 'CreatedAt', '--:--:--');
				const from = msgValue(m, 'from', 'From', 'system');
				const body = msgValue(m, 'body', 'Body', '');
				return '[' + stamp + '] ' + from + ': ' + body;
			}

			function normalizeChannelName(value) {
				const raw = String(value || '').trim();
				if (!raw) return '';
				return raw[0] === '#' ? raw : ('#' + raw);
			}

			function isNearBottom(box) {
				return (box.scrollTop + box.clientHeight) >= (box.scrollHeight - 32);
			}

			function updateComposerState() {
				const message = document.getElementById('message');
				const sendButton = document.getElementById('sendButton');
				const locked = streamState.locked.has(streamState.channel);
				const readOnly = locked && !canModerate;
				message.disabled = readOnly;
				sendButton.disabled = readOnly;
				document.getElementById('chatCurrentChannel').textContent = streamState.channel;
				document.getElementById('chatMode').textContent = readOnly ? 'read-only' : (locked ? 'locked (mod write)' : 'open');
				if (readOnly) {
					message.placeholder = 'This room is locked for non-moderators.';
				} else {
					message.placeholder = 'Say something in the room...';
				}
				const modChannel = document.querySelector('#modForm input[name="channel"]');
				if (modChannel) {
					modChannel.value = streamState.channel;
				}
			}

			function noteSync() {
				document.getElementById('chatLastSync').textContent = new Date().toLocaleTimeString();
			}

			function appendMessage(m) {
				const row = normalizeIncomingMessage(m);
				if (!row) return;
				const box = document.getElementById('chat');
				const stick = isNearBottom(box);
				const line = document.createElement('div');
				line.className = 'wolfbbs-chat-line';
				const meta = document.createElement('div');
				meta.className = 'wolfbbs-chat-line-meta';
				meta.textContent = '[' + msgValue(row, 'created_at', 'CreatedAt', '--:--:--') + ']';
				const body = document.createElement('div');
				body.className = 'wolfbbs-chat-line-body';
				const from = document.createElement('strong');
				const fromValue = String(msgValue(row, 'from', 'From', 'system') || 'system');
				from.textContent = fromValue + ': ';
				body.appendChild(from);
				const bodyText = String(msgValue(row, 'body', 'Body', ''));
				body.appendChild(document.createTextNode(bodyText));
				line.appendChild(meta);
				line.appendChild(body);
				const fromLower = fromValue.toLowerCase();
				if (fromLower === currentHandle) {
					line.classList.add('wolfbbs-chat-line-self');
				}
				if (fromLower === 'system' || fromLower === 'server') {
					line.classList.add('wolfbbs-chat-line-system');
				}
				if (currentHandle && bodyText.toLowerCase().includes('@' + currentHandle)) {
					line.classList.add('wolfbbs-chat-line-mention');
				}
				box.appendChild(line);
				const id = Number(msgValue(row, 'id', 'ID', 0)) || 0;
				if (id > streamState.lastSeenId) {
					streamState.lastSeenId = id;
				}
				if (stick) {
					box.scrollTop = box.scrollHeight;
				}
				updateMessageCount();
			}

			function ensureChannelOption(channel) {
				if (!channel) return;
				const normalized = normalizeChannelName(channel);
				const select = document.getElementById('channelSelect');
				const existing = Array.from(select.options).find((option) => option.value === normalized);
				if (existing) return;
				const option = document.createElement('option');
				option.value = normalized;
				option.textContent = normalized;
				select.appendChild(option);
			}

			function renderChannelBadges(channels) {
				const wrap = document.getElementById('channelBadges');
				wrap.innerHTML = '';
				(channels || []).slice(0, 8).forEach((name) => {
					const button = document.createElement('button');
					button.type = 'button';
					button.textContent = name;
					button.addEventListener('click', () => {
						refreshChannel(name, {focusComposer: true});
					});
					wrap.appendChild(button);
				});
			}

			function renderPresenceList(rows) {
				const online = document.getElementById('online');
				const list = Array.isArray(rows) ? rows : [];
				if (!list.length) {
					online.textContent = 'none';
					document.getElementById('onlineCount').textContent = '0';
					return;
				}
				const wrap = document.createElement('div');
				wrap.className = 'wolfbbs-presence-list';
				list.forEach((row) => {
					const card = document.createElement('div');
					card.className = 'wolfbbs-presence-card';
					const nickValue = String(presenceValue(row, 'nick', 'Nick', 'caller'));
					const nick = document.createElement('strong');
					nick.textContent = nickValue;
					card.appendChild(nick);
					const meta1 = document.createElement('span');
					meta1.textContent = String(presenceValue(row, 'area', 'Area', streamState.channel)) + ' • ' + String(presenceValue(row, 'node', 'Node', 'web'));
					card.appendChild(meta1);
					const meta2 = document.createElement('span');
					meta2.textContent = 'idle ' + String(presenceValue(row, 'idle_sec', 'IdleSec', 0)) + 's';
					card.appendChild(meta2);
					const loginAt = presenceValue(row, 'login_at', 'LoginAt', '');
					if (loginAt) {
						const meta3 = document.createElement('span');
						meta3.textContent = 'since ' + loginAt;
						card.appendChild(meta3);
					}
					const actions = document.createElement('div');
					actions.className = 'wolfbbs-inline-actions';
					const mail = document.createElement('a');
					mail.href = '/mail?to=' + encodeURIComponent(nickValue) + '&subject=' + encodeURIComponent('Follow-up from ' + streamState.channel) + '&body=' + encodeURIComponent('Continuing from ' + streamState.channel + '.\n\n');
					mail.textContent = 'mail';
					actions.appendChild(mail);
					const page = document.createElement('a');
					page.href = '/directory?handle=' + encodeURIComponent(nickValue) + '#page-desk';
					page.textContent = 'page';
					actions.appendChild(page);
					card.appendChild(actions);
					wrap.appendChild(card);
				});
				online.innerHTML = '';
				online.appendChild(wrap);
				document.getElementById('onlineCount').textContent = String(list.length);
			}

			async function loadChannels() {
				const res = await fetch('/chat/channels', {credentials: 'same-origin'});
				if (!res.ok) {
					setStatus('Failed to load channels.', 'error');
					return;
				}
				const payload = await res.json();
				const select = document.getElementById('channelSelect');
				select.innerHTML = '';
				const channels = payload.channels || ['#lobby'];
				streamState.locked = new Set(payload.locked || []);
				channels.forEach((name) => {
					const option = document.createElement('option');
					option.value = name;
					option.textContent = streamState.locked.has(name) ? (name + ' [locked]') : name;
					select.appendChild(option);
				});
				ensureChannelOption(streamState.channel);
				select.value = streamState.channel;
				renderChannelBadges(channels);
				updateComposerState();
				setStatus('Channels loaded.', 'ready');
			}

			async function loadHistory() {
				const ch = streamState.channel;
				const res = await fetch('/chat/history?channel=' + encodeURIComponent(ch) + '&limit=100', {credentials: 'same-origin'});
				if (!res.ok) {
					setStatus('Could not load history for ' + ch, 'error');
					return;
				}
				const payload = await res.json();
				const box = document.getElementById('chat');
				box.textContent = '';
				streamState.lastSeenId = 0;
				for (const m of payload.messages || []) {
					appendMessage(m);
				}
				if (payload.last_id) {
					streamState.lastSeenId = Number(payload.last_id) || streamState.lastSeenId;
				}
				noteSync();
				renderPresenceList(payload.online || []);
				box.scrollTop = box.scrollHeight;
				updateMessageCount();
			}

			async function loadOnline() {
				const ch = streamState.channel;
				const res = await fetch('/chat/online?channel=' + encodeURIComponent(ch), {credentials: 'same-origin'});
				if (!res.ok) {
					setStatus('Could not load online list.', 'error');
					return;
				}
				const payload = await res.json();
				renderPresenceList(payload.presence || []);
				noteSync();
			}

			function scheduleOnlineRefresh(delay) {
				if (onlineRefreshTimer) {
					window.clearTimeout(onlineRefreshTimer);
				}
				onlineRefreshTimer = window.setTimeout(async function(){
					if (document.hidden) {
						scheduleOnlineRefresh(15000);
						return;
					}
					if (onlineRefreshInFlight) {
						scheduleOnlineRefresh(5000);
						return;
					}
					onlineRefreshInFlight = true;
					try {
						await loadOnline();
					} finally {
						onlineRefreshInFlight = false;
						scheduleOnlineRefresh(document.hidden ? 15000 : 5000);
					}
				}, Math.max(500, delay || 5000));
			}

			function stopStream() {
				if (streamState.reconnectTimer) {
					window.clearTimeout(streamState.reconnectTimer);
					streamState.reconnectTimer = null;
				}
				if (onlineRefreshTimer) {
					window.clearTimeout(onlineRefreshTimer);
					onlineRefreshTimer = null;
				}
				if (streamState.es) {
					streamState.es.close();
					streamState.es = null;
				}
			}

			function watch() {
				stopStream();
				const es = new EventSource('/chat/stream?channel=' + encodeURIComponent(streamState.channel) + '&after_id=' + encodeURIComponent(String(streamState.lastSeenId || 0)));
				streamState.es = es;
				es.onopen = function() {
					setStatus('Live on ' + streamState.channel, 'live');
				};
				es.onmessage = function(evt){
					let msg = null;
					try {
						msg = JSON.parse(evt.data);
					} catch (err) {
						setStatus('Malformed stream event ignored.', 'warn');
						return;
					}
					appendMessage(msg);
					setStatus('Live on ' + streamState.channel, 'live');
					noteSync();
				};
				es.onerror = function() {
					es.close();
					setStatus('Realtime disconnected; retrying...', 'warn');
					streamState.reconnectTimer = window.setTimeout(watch, 1200);
				};
			}

			function focusChatComposer() {
				if (!chatComposerPrimed) return;
				document.getElementById('message').focus();
			}

			async function refreshChannel(next, opts) {
				const options = opts || {};
				const channel = normalizeChannelName(next);
				if (!channel) return;
				streamState.channel = channel;
				ensureChannelOption(channel);
				document.getElementById('channelSelect').value = channel;
				updateComposerState();
				await loadHistory();
				await loadOnline();
				watch();
				updateComposerState();
				const live = liveStatusText();
				setStatus(live.msg, live.state);
				if (options.focusComposer) {
					chatComposerPrimed = true;
					focusChatComposer();
				}
			}

			document.getElementById('channelSelect').addEventListener('change', async function(evt){
				await refreshChannel(evt.target.value, {focusComposer: true});
			});

			document.getElementById('customChannelForm').addEventListener('submit', async function(evt){
				evt.preventDefault();
				const field = document.getElementById('customChannel');
				const channel = normalizeChannelName(field.value);
				if (!channel) return;
				field.value = '';
				await refreshChannel(channel, {focusComposer: true});
			});

			document.getElementById('sendForm').addEventListener('submit', async function(evt){
				evt.preventDefault();
				const message = document.getElementById('message').value.trim();
				if (!message) return;
				setStatus('Sending message...', 'working');
				const sendRes = await fetch('/chat/send', {
					method:'POST',
					headers:{'Content-Type':'application/json','X-CSRF-Token': csrf},
					body: JSON.stringify({channel: streamState.channel, message: message}),
				});
				if (!sendRes.ok) {
					const body = await sendRes.text();
					setStatus('Send failed: ' + body, 'error');
					return;
				}
				document.getElementById('message').value = '';
				chatComposerPrimed = true;
				focusChatComposer();
				setStatus('Sent to ' + streamState.channel, 'live');
			});

			document.getElementById('message').addEventListener('focus', function(){
				chatComposerPrimed = true;
			});
			document.getElementById('message').addEventListener('pointerdown', function(){
				chatComposerPrimed = true;
			});
			document.getElementById('message').addEventListener('keydown', function(evt){
				if (evt.key === 'l' && evt.ctrlKey) {
					evt.preventDefault();
					const box = document.getElementById('chat');
					box.textContent = '';
					setStatus('Chat pane cleared.');
				}
			});

			const modForm = document.getElementById('modForm');
			if (modForm) {
				modForm.addEventListener('submit', async function(evt){
					evt.preventDefault();
					const form = new FormData(modForm);
					const payload = {
						channel: normalizeChannelName(form.get('channel')),
						action: String(form.get('action') || '').trim(),
						target: String(form.get('target') || '').trim(),
						reason: String(form.get('reason') || '').trim(),
						duration: String(form.get('duration') || '').trim(),
					};
					const res = await fetch('/chat/moderation', {
						method: 'POST',
						headers: {'Content-Type':'application/json','X-CSRF-Token': csrf},
						body: JSON.stringify(payload),
					});
					if (!res.ok) {
						setStatus('Moderation failed.', 'error');
						return;
					}
					setStatus('Moderation applied to ' + payload.channel, 'ready', {stickyMs: 2500});
					if (payload.channel === streamState.channel) {
						await loadOnline();
					}
				});
			}

			document.getElementById('chatReconnect').addEventListener('click', async function(){
				await loadChannels();
				await refreshChannel(streamState.channel, {focusComposer: chatComposerPrimed});
			});

			window.addEventListener('load', async () => {
				await loadChannels();
				streamState.channel = document.getElementById('channelSelect').value || '#lobby';
				await refreshChannel(streamState.channel);
				scheduleOnlineRefresh(5000);
			});

			document.addEventListener('visibilitychange', function(){
				if (!document.hidden) {
					loadOnline().finally(function(){
						scheduleOnlineRefresh(5000);
					});
					return;
				}
				scheduleOnlineRefresh(15000);
			});

			window.addEventListener('beforeunload', () => {
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
	joinedForStream := !a.chatSvc.IsInChannel(user.Handle, channel)
	if joinedForStream {
		a.chatSvc.JoinChannel(user.Handle, channel)
		defer a.chatSvc.LeaveChannel(user.Handle, channel)
	}
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
	if channel == "" {
		channel = "#lobby"
	}
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
	return a.createSessionWithContext(handle, 1, "http", false)
}

func (a *webApp) createSessionWithContext(handle string, authFactor int, transport string, secure bool) (string, bool) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", false
	}
	sid := hex.EncodeToString(b)
	csrf := randomToken(32)
	if authFactor <= 0 {
		authFactor = 1
	}
	transport = strings.ToLower(strings.TrimSpace(transport))
	if transport == "" {
		transport = "http"
	}
	a.Lock()
	a.sessions[sid] = sessionState{
		handle:     handle,
		expire:     time.Now().Add(2 * time.Hour),
		csrf:       csrf,
		transport:  transport,
		secure:     secure,
		authFactor: authFactor,
	}
	a.Unlock()
	return sid, true
}

func requestSecurityProfile(r *http.Request) (string, bool) {
	if r == nil {
		return "http", false
	}
	transport := "http"
	secure := false
	if r.TLS != nil {
		transport = "https"
		secure = true
	}
	if forwarded := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))); forwarded != "" {
		transport = forwarded
		switch forwarded {
		case "https", "wss", "ssh":
			secure = true
		}
	}
	return transport, secure
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

func (a *webApp) bestOfWeekRows(user *domain.User, limit int) []bestOfWeekRow {
	if a.boardRepo == nil || a.msgRepo == nil {
		return nil
	}
	if limit <= 0 {
		limit = 6
	}
	visibleBoards := a.visibleBoardsFor(user)
	if len(visibleBoards) == 0 {
		return nil
	}
	boardByID := map[int64]domain.Board{}
	for _, board := range visibleBoards {
		boardByID[board.ID] = board
	}
	reportCounts := map[int64]int{}
	if reports, err := a.msgRepo.ListReports(500, "open"); err == nil {
		for _, row := range reports {
			reportCounts[row.MessageID]++
		}
	}
	type candidate struct {
		Msg       domain.Message
		Board     domain.Board
		Replys    int
		Reports   int
		LastAt    time.Time
		Score     int
		Curated   bool
		CuratedAt time.Time
		Note      string
	}
	curated := map[int64]bestOfWeekEntry{}
	for _, row := range a.loadBestOfWeekEntries() {
		curated[row.MessageID] = row
	}
	candidates := map[int64]*candidate{}
	weekAgo := time.Now().UTC().Add(-7 * 24 * time.Hour)
	threadStates := a.loadThreadLifecycleStates()
	for _, board := range visibleBoards {
		msgs, err := a.msgRepo.ListByBoard(board.ID)
		if err != nil {
			continue
		}
		for _, msg := range msgs {
			threadID := messageThreadID(msg)
			if threadID <= 0 || a.threadLifecycleStateFromCache(threadID, threadStates) == threadLifecycleArchived {
				continue
			}
			entry := candidates[threadID]
			if entry == nil {
				entry = &candidate{Board: board}
				candidates[threadID] = entry
			}
			if msg.ParentID == 0 || msg.ID == threadID || entry.Msg.ID == 0 {
				entry.Msg = msg
				entry.Board = board
			}
			if msg.ParentID > 0 {
				entry.Replys++
			}
			entry.Reports += reportCounts[msg.ID]
			if msg.CreatedAt.After(entry.LastAt) {
				entry.LastAt = msg.CreatedAt
			}
		}
	}
	rows := make([]bestOfWeekRow, 0, limit)
	used := map[int64]bool{}
	for _, pick := range a.loadBestOfWeekEntries() {
		msg, err := a.msgRepo.GetMessage(pick.MessageID)
		if err != nil || msg == nil {
			continue
		}
		board, ok := boardByID[msg.BoardID]
		if !ok {
			continue
		}
		used[msg.ID] = true
		author := a.userHandleLookup()[msg.AuthorID]
		rows = append(rows, bestOfWeekRow{
			BoardID:   board.ID,
			BoardName: board.Name,
			MessageID: msg.ID,
			Subject:   cleanOneLiner(msg.Subject, 72),
			Author:    defaultIfBlank(author, "#"+strconv.FormatInt(msg.AuthorID, 10)),
			CreatedAt: msg.CreatedAt.Local().Format("2006-01-02 15:04"),
			Reason:    "curated pick",
			Note:      pick.Note,
			Href:      fmt.Sprintf("/boards?board=%d&id=%d", board.ID, msg.ID),
		})
		if len(rows) >= limit {
			return rows
		}
	}
	auto := make([]candidate, 0, len(candidates))
	for _, row := range candidates {
		if row == nil || row.Msg.ID <= 0 || used[row.Msg.ID] {
			continue
		}
		if row.Reports > 0 {
			continue
		}
		if row.LastAt.Before(weekAgo) && row.Msg.CreatedAt.Before(weekAgo) {
			continue
		}
		row.Score = row.Replys*4 + minInt(int(time.Since(row.LastAt).Hours()/24), 0)
		auto = append(auto, *row)
	}
	sort.Slice(auto, func(i, j int) bool {
		if auto[i].Replys != auto[j].Replys {
			return auto[i].Replys > auto[j].Replys
		}
		if auto[i].LastAt.Equal(auto[j].LastAt) {
			return strings.ToLower(auto[i].Msg.Subject) < strings.ToLower(auto[j].Msg.Subject)
		}
		return auto[i].LastAt.After(auto[j].LastAt)
	})
	lookup := a.userHandleLookup()
	for _, row := range auto {
		author := lookup[row.Msg.AuthorID]
		reason := strconv.Itoa(row.Replys) + " replies this week"
		if row.Replys == 0 {
			reason = "recent standout thread"
		}
		rows = append(rows, bestOfWeekRow{
			BoardID:    row.Board.ID,
			BoardName:  row.Board.Name,
			MessageID:  row.Msg.ID,
			Subject:    cleanOneLiner(row.Msg.Subject, 72),
			Author:     defaultIfBlank(author, "#"+strconv.FormatInt(row.Msg.AuthorID, 10)),
			CreatedAt:  row.Msg.CreatedAt.Local().Format("2006-01-02 15:04"),
			ReplyCount: row.Replys,
			Reason:     reason,
			Href:       fmt.Sprintf("/boards?board=%d&id=%d", row.Board.ID, row.Msg.ID),
		})
		if len(rows) >= limit {
			break
		}
	}
	return rows
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

func safeLocalRedirectPath(path, fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		fallback = "/"
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fallback
	}
	if strings.ContainsAny(path, "\r\n") || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return fallback
	}
	u, err := url.Parse(path)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/") {
		return fallback
	}
	return u.String()
}

func appendRedirectQuery(path, key, value string) string {
	path = safeLocalRedirectPath(path, "/")
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return path
	}
	u, err := url.Parse(path)
	if err != nil {
		return path
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String()
}

func redirectWithNotice(w http.ResponseWriter, r *http.Request, path, notice string) {
	redirectWithQueryMessage(w, r, path, "notice", notice)
}

func redirectWithError(w http.ResponseWriter, r *http.Request, path, errText string) {
	redirectWithQueryMessage(w, r, path, "error", errText)
}

func redirectWithQueryMessage(w http.ResponseWriter, r *http.Request, path, key, value string) {
	path = safeLocalRedirectPath(path, "/")
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		http.Redirect(w, r, path, http.StatusFound)
		return
	}
	http.Redirect(w, r, appendRedirectQuery(path, key, value), http.StatusFound)
}

func quoteBody(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, "> "+line)
	}
	return strings.Join(out, "\n")
}

func containsString(rows []string, value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, row := range rows {
		if strings.ToLower(strings.TrimSpace(row)) == value {
			return true
		}
	}
	return false
}

func splitTrimmedList(raw, sep string, limit int) []string {
	if limit <= 0 {
		limit = 64
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, limit)
	for _, row := range strings.Split(raw, sep) {
		row = strings.TrimSpace(row)
		key := normalizeHandleKey(row)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, row)
		if len(out) >= limit {
			break
		}
	}
	return out
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
	case "showcase", "demo", "walkthrough":
		return "/showcase"
	case "attention", "attn", "queue":
		return "/attention"
	case "today", "brief", "daily", "t":
		return "/today"
	case "digest", "daily-digest":
		return "/digest"
	case "digest-prefs", "digest-weekday", "weekday-digest":
		return "/digest/preferences"
	case "events", "calendar", "event", "e":
		return "/events"
	case "tournaments", "tourney", "bracket":
		return "/tournaments"
	case "challenges", "challenge", "season", "ladder":
		return "/challenges"
	case "boards", "messages", "msg", "m":
		return "/boards"
	case "mail", "pm", "p":
		return "/mail"
	case "bookmarks", "saved", "later", "read-later":
		return "/bookmarks"
	case "circles", "groups", "circle":
		return "/circles"
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
	case "collections", "packs", "curated":
		return "/collections"
	case "offline", "packet", "packets":
		return "/offline"
	case "feedback", "fb":
		return "/feedback"
	case "radar", "mission", "r":
		return "/radar"
	case "streaks", "streak":
		return "/streaks"
	case "next", "next-best", "nba":
		return "/next"
	case "spotlights", "spotlight", "returners":
		return "/spotlights"
	case "missions", "season-missions":
		return "/missions"
	case "resume", "re-entry", "reentry":
		return "/resume"
	case "comeback", "door-comeback", "streak-comeback":
		return "/doors/comeback"
	case "mentorship", "mentor", "onboarding":
		return "/mentorship"
	case "milestones", "celebrate":
		return "/milestones"
	case "time-lane", "timelane", "lane":
		return "/time-lane"
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
	case "admin-challenges", "season-admin":
		return "/admin/challenges"
	case "admin-missions", "missions-admin":
		return "/admin/missions"
	case "admin-mentorship", "mentor-admin":
		return "/admin/mentorship"
	case "mod-center", "moderation", "reports-queue":
		return "/admin/mod-center"
	case "plugins", "admin-plugins":
		return "/admin/plugins"
	case "themes", "admin-themes":
		return "/admin/themes"
	case "webhooks", "hooks", "admin-webhooks":
		return "/admin/webhooks"
	case "analytics", "admin-analytics", "metrics":
		return "/admin/analytics"
	case "upgrade", "upgrade-safety", "rollout":
		return "/admin/upgrade-safety"
	case "backup", "backups":
		return "/admin/backups"
	case "release", "release-dashboard", "ship", "shipboard":
		return "/admin/release"
	case "admin-bulletins", "bulletin-admin", "wire":
		return "/admin/bulletins"
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

func activityHeatLevelLabel(level int) string {
	switch {
	case level >= 4:
		return "hot"
	case level == 3:
		return "busy"
	case level == 2:
		return "steady"
	case level == 1:
		return "light"
	default:
		return "idle"
	}
}

func (a *webApp) buildCallerActivityHeatmap(user *domain.User, days int) []activityHeatmapCell {
	if user == nil || days <= 0 {
		return nil
	}
	now := time.Now().In(time.Local)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -(days - 1))
	counts := map[string]int{}
	addAt := func(at time.Time) {
		if at.IsZero() {
			return
		}
		local := at.In(time.Local)
		if local.Before(start) {
			return
		}
		counts[local.Format("2006-01-02")]++
	}
	if a.msgRepo != nil {
		for _, board := range a.visibleBoardsFor(user) {
			msgs, err := a.msgRepo.ListByBoard(board.ID)
			if err != nil {
				continue
			}
			for _, msg := range msgs {
				if msg.AuthorID == user.ID {
					addAt(msg.CreatedAt)
				}
			}
		}
	}
	if a.mailRepo != nil {
		if inbox, err := a.mailRepo.ListInbox(user.ID, 250); err == nil {
			for _, row := range inbox {
				addAt(row.CreatedAt)
			}
		}
		if outbox, err := a.mailRepo.ListOutbox(user.ID, 250); err == nil {
			for _, row := range outbox {
				addAt(row.CreatedAt)
			}
		}
	}
	if a.adminRepo != nil {
		if rows, err := a.adminRepo.ListCallerHistory(days * 8); err == nil {
			handleKey := normalizeHandleKey(user.Handle)
			for _, row := range rows {
				if normalizeHandleKey(row.Username) != handleKey {
					continue
				}
				at := row.LogoutAt
				if at.IsZero() {
					at = row.LoginAt
				}
				addAt(at)
			}
		}
	}
	if user.LastLoginAt != nil {
		addAt(*user.LastLoginAt)
	}
	maxCount := 0
	out := make([]activityHeatmapCell, 0, days)
	for offset := 0; offset < days; offset++ {
		day := start.AddDate(0, 0, offset)
		key := day.Format("2006-01-02")
		count := counts[key]
		if count > maxCount {
			maxCount = count
		}
		out = append(out, activityHeatmapCell{
			ShortLabel: day.Format("Mon"),
			DateLabel:  day.Format("01/02"),
			Count:      count,
			Detail:     day.Format("Mon Jan 2, 2006"),
		})
	}
	for i := range out {
		if out[i].Count <= 0 || maxCount <= 0 {
			continue
		}
		level := (out[i].Count*4 + maxCount - 1) / maxCount
		if level < 1 {
			level = 1
		}
		if level > 4 {
			level = 4
		}
		out[i].Level = level
		out[i].Detail = fmt.Sprintf("%s · %d activity touch(es)", out[i].Detail, out[i].Count)
	}
	return out
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
	threadStates := a.loadThreadLifecycleStates()
	out := make([]boardPulseRow, 0, len(boards))
	for _, board := range boards {
		if a.isBoardQuietActive(user.Handle, board.ID, now) {
			continue
		}
		msgs, err := a.msgRepo.ListByBoard(board.ID)
		if err != nil {
			continue
		}
		pointerID := int64(0)
		if ptr, ptrErr := a.msgRepo.GetPointer(user.ID, board.ID); ptrErr == nil && ptr != nil {
			pointerID = ptr.LastReadID
		}
		row := boardPulseRow{
			BoardID:    board.ID,
			BoardName:  board.Name,
			Conference: board.Conference,
		}
		var lastVisible *domain.Message
		for _, msg := range msgs {
			if a.threadLifecycleStateFromCache(messageThreadID(msg), threadStates) == threadLifecycleArchived {
				continue
			}
			row.MessageCount++
			if msg.ID > pointerID {
				row.NewCount++
			}
			copy := msg
			lastVisible = &copy
		}
		if lastVisible != nil {
			last := *lastVisible
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
	for _, row := range a.activeScheduledBulletins(time.Now().UTC(), 6) {
		snapshot.Scheduled = append(snapshot.Scheduled, row)
		snapshot.SystemWire = append(snapshot.SystemWire, "Scheduled: "+cleanOneLiner(row.Title+" — "+row.Body, 120))
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
		snapshot.BestOfWeek = a.bestOfWeekRows(user, 6)
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
		entries = a.filterVisibleFiles(entries)
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
	threadStates := a.loadThreadLifecycleStates()
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
		row := boardMenuRow{Board: board}
		var lastVisible *domain.Message
		for _, msg := range msgs {
			if a.threadLifecycleStateFromCache(messageThreadID(msg), threadStates) == threadLifecycleArchived {
				continue
			}
			row.MessageCount++
			if msg.ID > pointerID {
				row.NewCount++
			}
			if msg.AuthorID == user.ID {
				row.MyPosts++
			}
			if strings.Contains(strings.ToLower(msg.Subject), strings.ToLower(user.Handle)) || strings.Contains(strings.ToLower(msg.Body), strings.ToLower(user.Handle)) {
				row.Mentions++
			}
			copy := msg
			lastVisible = &copy
		}
		if lastVisible != nil {
			last := *lastVisible
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

func moderationFingerprint(msg domain.Message) string {
	subject := strings.ToLower(strings.TrimSpace(msg.Subject))
	body := strings.ToLower(strings.TrimSpace(msg.Body))
	body = cleanOneLiner(body, 160)
	return subject + "|" + body
}

func buildModerationHints(msgs []domain.Message, reports []domain.MessageReport) map[int64]string {
	threadCounts := map[int64]int{}
	reportCounts := map[int64]int{}
	fingerprintCounts := map[string]int{}
	fingerprintByMessage := map[int64]string{}
	now := time.Now().UTC()
	for _, row := range msgs {
		if row.ThreadID > 0 {
			threadCounts[row.ThreadID]++
		}
		fp := moderationFingerprint(row)
		fingerprintByMessage[row.ID] = fp
		if strings.TrimSpace(fp) != "|" {
			fingerprintCounts[fp]++
		}
	}
	for _, row := range reports {
		reportCounts[row.MessageID]++
	}
	out := map[int64]string{}
	for _, row := range msgs {
		hints := make([]string, 0, 4)
		if row.ThreadID > 0 {
			if count := threadCounts[row.ThreadID]; count > 1 {
				hints = append(hints, "thread "+strconv.Itoa(count))
			}
		}
		if count := reportCounts[row.ID]; count > 0 {
			hints = append(hints, "reports "+strconv.Itoa(count))
		}
		if fp := fingerprintByMessage[row.ID]; fp != "" {
			if count := fingerprintCounts[fp]; count > 1 {
				hints = append(hints, "possible duplicate x"+strconv.Itoa(count))
			}
		}
		age := now.Sub(row.CreatedAt)
		if age > 0 {
			hints = append(hints, "age "+formatDurationCompact(age))
		}
		out[row.ID] = strings.Join(hints, " • ")
	}
	return out
}

func directoryVisibleUser(user *domain.User) bool {
	return user != nil && user.Enabled && !user.Banned
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
			now := time.Now().UTC()
			for _, row := range sessions {
				idle := time.Duration(0)
				if !row.LastActivity.IsZero() {
					idle = now.Sub(row.LastActivity)
					if idle < 0 {
						idle = 0
					}
				}
				presence[strings.ToLower(row.Username)] = directoryRow{
					Online: true,
					Node:   "Node " + strconv.Itoa(row.NodeID),
					Idle:   formatDurationCompact(idle),
					Area:   row.Area,
					Origin: strings.ToUpper(netutil.RemoteOrigin(row.RemoteAddr)),
					From:   remoteHostDisplay(row.RemoteAddr),
				}
			}
		}
	}
	out := make([]directoryRow, 0, len(users))
	for _, row := range users {
		if !directoryVisibleUser(&row) {
			continue
		}
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
			entry.Node = live.Node
			entry.Idle = live.Idle
			entry.Area = live.Area
			entry.Origin = live.Origin
			entry.From = live.From
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
			hay := strings.ToLower(strings.Join([]string{entry.Handle, entry.Role, entry.Theme, entry.LastLogin, entry.Node, entry.Idle, entry.Area, entry.Origin, entry.From}, " "))
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
	if currentUser == nil || !directoryVisibleUser(target) {
		return nil
	}
	public := a.loadPublicProfileSettings(target.Handle)
	profile := &directoryProfile{
		Handle:       target.Handle,
		Role:         rbac.NormalizeRole(target.Role),
		Theme:        target.Theme,
		Verified:     target.Verified,
		StatusLine:   "",
		Bio:          "",
		ContactPrefs: nil,
		Alias:        a.contactAlias(currentUser.Handle, target.Handle),
		CircleNames:  circlesForHandle(a.loadCallerCircles(currentUser.Handle), target.Handle),
	}
	if public.ShowStatusLine {
		profile.StatusLine = public.StatusLine
	}
	if public.ShowBio {
		profile.Bio = public.Bio
	}
	if public.ShowContact {
		profile.ContactPrefs = public.ContactPrefs
	}
	if target.LastLoginAt != nil && !target.LastLoginAt.IsZero() {
		profile.LastLogin = target.LastLoginAt.Local().Format("2006-01-02 15:04")
	} else {
		profile.LastLogin = "never"
	}
	if a.adminRepo != nil {
		if sessions, err := a.adminRepo.ListNodeSessions(200); err == nil {
			now := time.Now().UTC()
			for _, row := range sessions {
				if !strings.EqualFold(row.Username, target.Handle) {
					continue
				}
				idle := time.Duration(0)
				if !row.LastActivity.IsZero() {
					idle = now.Sub(row.LastActivity)
					if idle < 0 {
						idle = 0
					}
				}
				profile.Online = true
				profile.OnlineNode = "Node " + strconv.Itoa(row.NodeID)
				profile.OnlineIdle = formatDurationCompact(idle)
				profile.OnlineArea = row.Area
				profile.OnlineOrigin = strings.ToUpper(netutil.RemoteOrigin(row.RemoteAddr))
				profile.OnlineFrom = remoteHostDisplay(row.RemoteAddr)
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
	profile.Relationship = a.buildRelationshipTimeline(currentUser, target, 8)
	if a.hasRole(currentUser, roleModerator) {
		profile.IncidentTimeline = a.buildIncidentTimeline(target, 8)
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
		rows = a.filterVisibleFiles(rows)
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
		filteredQueue := make([]domain.DownloadQueueItem, 0, len(rows))
		for _, row := range rows {
			if entry, err := a.adminRepo.GetFileEntry(row.FileID); err == nil && entry != nil && a.fileVisibleToCallers(entry.ID) {
				filteredQueue = append(filteredQueue, row)
				snapshot.QueueNames[row.FileID] = entry.Name
			}
		}
		snapshot.Queue = filteredQueue
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

func (a *webApp) recurringCorrespondentStats(target *domain.User, limit int) []callerLinkStat {
	if target == nil || a.mailRepo == nil || limit <= 0 {
		return nil
	}
	inbox, _ := a.mailRepo.ListInbox(target.ID, 250)
	outbox, _ := a.mailRepo.ListOutbox(target.ID, 250)
	lookup := a.userHandleLookup()
	counts := map[string]int{}
	add := func(handle string) {
		handle = strings.TrimSpace(handle)
		if handle == "" {
			return
		}
		counts[handle]++
	}
	for _, row := range inbox {
		add(lookup[row.FromUserID])
	}
	for _, row := range outbox {
		if row.ExternalTo != nil {
			continue
		}
		add(lookup[row.ToUserID])
	}
	out := make([]callerLinkStat, 0, len(counts))
	for handle, count := range counts {
		target, err := a.authSvc.GetUser(handle)
		if err != nil || !directoryVisibleUser(target) {
			continue
		}
		live, _ := a.activePresenceForHandle(handle)
		out = append(out, callerLinkStat{
			Handle: handle,
			Count:  count,
			Online: live.Online,
			Area:   live.Area,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.ToLower(out[i].Handle) < strings.ToLower(out[j].Handle)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (a *webApp) buildRelationshipTimeline(currentUser, target *domain.User, limit int) []string {
	if currentUser == nil || target == nil || limit <= 0 {
		return nil
	}
	type timelineRow struct {
		When time.Time
		Text string
	}
	rows := make([]timelineRow, 0, limit)
	if a.mailRepo != nil {
		if inbox, err := a.mailRepo.ListInbox(currentUser.ID, 250); err == nil {
			for _, row := range inbox {
				if row.FromUserID != target.ID {
					continue
				}
				rows = append(rows, timelineRow{
					When: row.CreatedAt,
					Text: "mail from " + target.Handle + ": " + cleanOneLiner(row.Subject, 72),
				})
			}
		}
		if outbox, err := a.mailRepo.ListOutbox(currentUser.ID, 250); err == nil {
			for _, row := range outbox {
				if row.ToUserID != target.ID {
					continue
				}
				rows = append(rows, timelineRow{
					When: row.CreatedAt,
					Text: "mail to " + target.Handle + ": " + cleanOneLiner(row.Subject, 72),
				})
			}
		}
	}
	if a.isFavoriteCaller(currentUser.Handle, target.Handle) {
		rows = append(rows, timelineRow{
			When: time.Now().UTC(),
			Text: "currently in your favorite callers list",
		})
	}
	if a.adminRepo != nil {
		if callers, err := a.adminRepo.ListCallerHistory(200); err == nil {
			for _, row := range callers {
				if !strings.EqualFold(row.Username, target.Handle) {
					continue
				}
				at := row.LogoutAt
				if at.IsZero() {
					at = row.LoginAt
				}
				rows = append(rows, timelineRow{
					When: at,
					Text: "recent call from " + defaultIfBlank(remoteHostDisplay(row.RemoteAddr), "unknown host") + " in " + defaultIfBlank(row.Area, "unknown area"),
				})
				if len(rows) >= limit*3 {
					break
				}
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].When.After(rows[j].When) })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.When.Local().Format("2006-01-02 15:04")+" — "+row.Text)
	}
	return out
}

func (a *webApp) buildIncidentTimeline(target *domain.User, limit int) []string {
	if target == nil || limit <= 0 {
		return nil
	}
	type timelineRow struct {
		When time.Time
		Text string
	}
	rows := make([]timelineRow, 0, limit)
	for _, row := range a.loadStaffEscalations() {
		if !strings.EqualFold(row.Handle, target.Handle) {
			continue
		}
		status := "open"
		when := row.CreatedAt
		if !row.ResolvedAt.IsZero() {
			status = "resolved"
			when = row.ResolvedAt
		}
		rows = append(rows, timelineRow{
			When: when,
			Text: "staff escalation " + status + ": " + cleanOneLiner(row.Note, 88),
		})
	}
	if a.msgRepo != nil && a.boardRepo != nil {
		boards, _ := a.boardRepo.List()
		reports, _ := a.msgRepo.ListReports(300, "")
		for _, board := range boards {
			msgs, err := a.msgRepo.ListByBoard(board.ID)
			if err != nil {
				continue
			}
			byID := map[int64]domain.Message{}
			for _, msg := range msgs {
				byID[msg.ID] = msg
			}
			for _, report := range reports {
				msg, ok := byID[report.MessageID]
				if !ok || msg.AuthorID != target.ID {
					continue
				}
				rows = append(rows, timelineRow{
					When: report.CreatedAt,
					Text: "reported post in " + board.Name + ": " + cleanOneLiner(report.Reason, 72),
				})
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].When.After(rows[j].When) })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.When.Local().Format("2006-01-02 15:04")+" — "+row.Text)
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
		if !directoryVisibleUser(&row) {
			continue
		}
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

func (a *webApp) activePresenceForHandle(handle string) (directoryRow, bool) {
	if a.adminRepo == nil {
		return directoryRow{}, false
	}
	handleKey := normalizeHandleKey(handle)
	if handleKey == "" {
		return directoryRow{}, false
	}
	sessions, err := a.adminRepo.ListNodeSessions(200)
	if err != nil {
		return directoryRow{}, false
	}
	now := time.Now().UTC()
	for _, row := range sessions {
		if normalizeHandleKey(row.Username) != handleKey {
			continue
		}
		idle := time.Duration(0)
		if !row.LastActivity.IsZero() {
			idle = now.Sub(row.LastActivity)
			if idle < 0 {
				idle = 0
			}
		}
		return directoryRow{
			Online: true,
			Node:   "Node " + strconv.Itoa(row.NodeID),
			Idle:   formatDurationCompact(idle),
			Area:   row.Area,
			Origin: strings.ToUpper(netutil.RemoteOrigin(row.RemoteAddr)),
			From:   remoteHostDisplay(row.RemoteAddr),
		}, true
	}
	return directoryRow{}, false
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
