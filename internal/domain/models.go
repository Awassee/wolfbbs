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
