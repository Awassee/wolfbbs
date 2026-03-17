package domain

import "time"

type DoorConfig struct {
	DoorID               string
	Enabled              bool
	DailyTurns           int
	TimeBankMax          int
	ResetHourLocal       int
	RequiredRoleOverride string
	MessagesDays         int
	LogsDays             int
	MaxRunSeconds        int
	MaxOutputRate        int
	AllowNetwork         bool
	AllowFSWrite         bool
	UpdatedAt            time.Time
}

type DoorUserState struct {
	UserID    int64
	DoorID    string
	StateJSON string
	UpdatedAt time.Time
}

type DoorGlobalState struct {
	DoorID    string
	StateJSON string
	UpdatedAt time.Time
}

type DoorEvent struct {
	ID          int64
	DoorID      string
	UserID      int64
	EventType   string
	PayloadJSON string
	CreatedAt   time.Time
}

type DoorTurnLedger struct {
	UserID    int64
	DoorID    string
	DayKey    time.Time
	TurnsUsed int
	TimeBank  int
	UpdatedAt time.Time
}

type DoorAchievement struct {
	DoorID          string
	UserID          int64
	AchievementCode string
	CreatedAt       time.Time
}

type DoorScore struct {
	ID           int64
	DoorID       string
	UserID       int64
	ScoreType    string
	Value        int64
	MetadataJSON string
	CreatedAt    time.Time
}

type DoorUserMeta struct {
	UserID       int64
	DoorID       string
	Favorite     bool
	LastPlayedAt *time.Time
	PlayCount    int
	UpdatedAt    time.Time
}

type DoorUsageStats struct {
	DoorID        string
	DailyActive   int
	MonthlyActive int
	TotalPlays    int64
}
