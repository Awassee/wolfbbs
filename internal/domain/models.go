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
