package domain

import "time"

type User struct {
	ID            int64
	Handle        string
	PasswordHash  string
	Enabled       bool
	Banned        bool
	ForceReset    bool
	Theme         string
	TimeFormat24h bool
	ANSIEnabled   bool
	PagingEnabled bool
	Role          string
	TOTPSecret    string
	RecoveryCodes []string
	Verified      bool
	LastLoginAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Board struct {
	ID          int64
	Name        string
	Description string
	Conference  string
	ReadACS     string
	WriteACS    string
	CreatedBy   int64
	CreatedAt   time.Time
}

type Message struct {
	ID        int64
	BoardID   int64
	AuthorID  int64
	ParentID  int64
	ThreadID  int64
	Subject   string
	Body      string
	CreatedAt time.Time
}

type MessagePointer struct {
	UserID     int64
	BoardID    int64
	LastReadID int64
	LastReadAt time.Time
	UpdatedAt  time.Time
}

type MessageReport struct {
	ID         int64
	MessageID  int64
	ReporterID int64
	Reason     string
	Status     string
	CreatedAt  time.Time
	ResolvedAt *time.Time
	ResolvedBy string
}

type PrivateMail struct {
	ID         int64
	FromUserID int64
	ToUserID   int64
	Subject    string
	Body       string
	ExternalTo *string
	CreatedAt  time.Time
	ReadAt     *time.Time
}

type FileArea struct {
	ID          int64
	Name        string
	Path        string
	Description string
	CreatedAt   time.Time
}

type FileEntry struct {
	ID          int64
	AreaID       int64
	Name         string
	Path         string
	Description  string
	Tags         []string
	SHA256       string
	SizeBytes    int64
	UploaderID   int64
	UploadedAt   time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	RatingAvg    float64
	RatingCount  int
}

type FileFilter struct {
	ID        int64
	UserID    int64
	Name      string
	Query     string
	Tags      []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type DownloadQueueItem struct {
	ID        int64
	UserID    int64
	FileID    int64
	CreatedAt time.Time
}

type DownloadTicket struct {
	Token     string
	UserID    int64
	FileID    int64
	ExpiresAt time.Time
	CreatedAt time.Time
	UsedAt    *time.Time
}

type GatewaySettings struct {
	SMTPHost        string
	SMTPPort        int
	SMTPUser        string
	SMTPPass        string
	FromDomain      string
	MaxRecipients   int
	MaxMessageBytes int
	WebTimeoutSec   int
	WebMaxBytes     int
	UpdatedAt       time.Time
}

type MailOutboundPolicy struct {
	Handle           string
	OutboundDisabled bool
	UpdatedAt        time.Time
}

type AdminAudit struct {
	ID        int64
	Actor     string
	Target    string
	Action    string
	Details   string
	CreatedAt time.Time
}

type PasswordResetToken struct {
	TokenHash  string
	Handle     string
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

type NodeSession struct {
	SessionID    string
	NodeID       int
	Username     string
	Area         string
	RemoteAddr   string
	LoginAt      time.Time
	LastActivity time.Time
	UpdatedAt    time.Time
}

type CallerHistory struct {
	ID              int64
	SessionID       string
	NodeID          int
	Username        string
	Area            string
	RemoteAddr      string
	LoginAt         time.Time
	LogoutAt        time.Time
	DurationSeconds int64
	CreatedAt       time.Time
}
