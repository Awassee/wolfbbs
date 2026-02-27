package doors

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/rbac"
	"wolfbbs/internal/repository"
	"wolfbbs/internal/ui"
)

type Manifest struct {
	ID              string `json:"id"`
	DisplayName     string `json:"display_name"`
	ShortDesc       string `json:"short_description"`
	Category        string `json:"category"`
	Version         string `json:"version"`
	EntryType       string `json:"entry_type"` // native|external
	Entrypoint      string `json:"entrypoint"`
	Hotkey          string `json:"hotkey"`
	ConcurrencyMode string `json:"concurrency_mode"` // exclusive|per_user|shared
	Requires        struct {
		ANSI    bool `json:"ansi"`
		MinCols int  `json:"min_cols"`
		MinRows int  `json:"min_rows"`
	} `json:"requires"`
	Capabilities struct {
		NeedsNetwork         bool `json:"needs_network"`
		NeedsFilesystemWrite bool `json:"needs_filesystem_write"`
		NeedsSound           bool `json:"needs_sound"`
	} `json:"capabilities"`
	DataRetention struct {
		MessagesDays int `json:"messages_days"`
		LogsDays     int `json:"logs_days"`
	} `json:"data_retention"`
	AdminDefaults struct {
		Enabled       bool `json:"enabled"`
		DailyTurns    int  `json:"daily_turns"`
		TimeBankMax   int  `json:"time_bank_max"`
		ResetHour     int  `json:"reset_hour"`
		MaxRunSec     int  `json:"max_run_seconds"`
		MaxOutputRate int  `json:"max_output_rate"`
	} `json:"admin_defaults"`
	RequiredRole string   `json:"required_role"`
	HelpText     string   `json:"help_text"`
	RulesText    string   `json:"rules_text"`
	Args         []string `json:"args"`
}

type Door struct {
	ID              string
	Name            string
	Description     string
	Category        string
	Version         string
	Type            string
	Command         string
	Args            []string
	Hotkey          string
	ConcurrencyMode string
	RequiresANSI    bool
	MinCols         int
	MinRows         int
	NeedsNetwork    bool
	NeedsFSWrite    bool
	MessagesDays    int
	LogsDays        int
	EnabledDefault  bool
	DailyTurns      int
	TimeBankMax     int
	ResetHour       int
	MaxRunSec       int
	MaxOutputRate   int
	RequiredRole    string
	HelpText        string
	RulesText       string
}

type DoorContext struct {
	UserID        int64
	Username      string
	Role          string
	UserCreatedAt time.Time
	NodeID        string
	SessionID     string
	Cols          int
	Rows          int
	ANSI          bool
	Encoding      string
	ColorDepth    int
	Timezone      string
	Now           time.Time
	StorageNS     string
}

type Registry struct {
	mu             sync.Mutex
	doorsByHotkey  map[string]Door
	doorsByID      map[string]Door
	repo           repository.DoorRepository
	exclusiveLock  sync.Mutex
	perUserLocksMu sync.Mutex
	perUserLocks   map[string]*sync.Mutex
}

func NewRegistry() *Registry {
	r := &Registry{
		doorsByHotkey: map[string]Door{},
		doorsByID:     map[string]Door{},
		repo:          repository.NewInMemoryDoorRepository(),
		perUserLocks:  map[string]*sync.Mutex{},
	}
	_ = r.LoadManifestDir("doors")
	return r
}

func (r *Registry) SetRepository(repo repository.DoorRepository) {
	if repo == nil {
		return
	}
	r.mu.Lock()
	r.repo = repo
	doors := make([]Door, 0, len(r.doorsByID))
	for _, door := range r.doorsByID {
		doors = append(doors, door)
	}
	r.mu.Unlock()
	for _, door := range doors {
		_ = r.ensureConfigDefaults(door)
	}
}

func (r *Registry) LoadManifestDir(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "doors"
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	loaded := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(root, entry.Name(), "door.json")
		body, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var mf Manifest
		if err := json.Unmarshal(body, &mf); err != nil {
			continue
		}
		if r.RegisterManifest(mf) {
			loaded++
		}
	}
	if loaded == 0 {
		// Keep at least one door available for development environments.
		SeedTrivia(r, strings.TrimSpace(os.Getenv("WOLFBBS_TRIVIA_BINARY")))
	}
	return nil
}

func (r *Registry) RegisterManifest(mf Manifest) bool {
	door := doorFromManifest(mf)
	if door.ID == "" || door.Hotkey == "" || door.Command == "" {
		return false
	}
	r.Register(door)
	return true
}

func (r *Registry) Register(door Door) {
	door.Hotkey = normalizeHotkey(door.Hotkey)
	door.ID = normalizeID(door.ID)
	door.Command = strings.TrimSpace(door.Command)
	if door.ID == "" || door.Hotkey == "" || door.Command == "" {
		return
	}
	if strings.TrimSpace(door.Name) == "" {
		door.Name = door.ID
	}
	if strings.TrimSpace(door.Category) == "" {
		door.Category = "utility"
	}
	if strings.TrimSpace(door.Type) == "" {
		door.Type = "native"
	}
	if strings.TrimSpace(door.ConcurrencyMode) == "" {
		door.ConcurrencyMode = "per_user"
	}
	if door.MinCols <= 0 {
		door.MinCols = 80
	}
	if door.MinRows <= 0 {
		door.MinRows = 25
	}
	if door.MaxRunSec <= 0 {
		door.MaxRunSec = 180
	}
	if door.MaxOutputRate <= 0 {
		door.MaxOutputRate = 4096
	}
	if door.DailyTurns <= 0 {
		door.DailyTurns = 40
	}
	if door.TimeBankMax <= 0 {
		door.TimeBankMax = 200
	}
	r.mu.Lock()
	r.doorsByHotkey[door.Hotkey] = door
	r.doorsByID[door.ID] = door
	r.mu.Unlock()
	_ = r.ensureConfigDefaults(door)
}

func (r *Registry) Doors() []Door {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Door, 0, len(r.doorsByHotkey))
	for _, d := range r.doorsByHotkey {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category == out[j].Category {
			if out[i].Hotkey == out[j].Hotkey {
				return out[i].Name < out[j].Name
			}
			return out[i].Hotkey < out[j].Hotkey
		}
		return out[i].Category < out[j].Category
	})
	return out
}

func (r *Registry) GetByHotkey(hotkey string) (Door, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.doorsByHotkey[normalizeHotkey(hotkey)]
	return row, ok
}

func (r *Registry) GetByID(doorID string) (Door, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.doorsByID[normalizeID(doorID)]
	return row, ok
}

func (r *Registry) Launch(ctx context.Context, hotkey string, stdin io.Reader, stdout, stderr io.Writer, env map[string]string) error {
	door, ok := r.GetByHotkey(hotkey)
	if !ok {
		return fmt.Errorf("unknown door")
	}
	doorCtx := parseDoorContext(door.ID, env)
	cfg := r.effectiveConfig(door)
	if !cfg.Enabled {
		return fmt.Errorf("door disabled by sysop")
	}
	requiredRole := strings.TrimSpace(door.RequiredRole)
	if strings.TrimSpace(cfg.RequiredRoleOverride) != "" {
		requiredRole = strings.TrimSpace(cfg.RequiredRoleOverride)
	}
	if !roleAllowed(strings.ToLower(strings.TrimSpace(doorCtx.Role)), requiredRole) {
		return fmt.Errorf("door requires role: %s", requiredRole)
	}
	if doorCtx.Cols > 0 && doorCtx.Rows > 0 {
		if doorCtx.Cols < door.MinCols || doorCtx.Rows < door.MinRows {
			return fmt.Errorf("door requires terminal size %dx%d", door.MinCols, door.MinRows)
		}
	}
	unlock := r.acquireConcurrencyLock(door, doorCtx.UserID)
	defer unlock()

	if err := r.consumeTurn(doorCtx.UserID, door, cfg, doorCtx.Now); err != nil {
		return err
	}

	_ = r.repo.AddEvent(&domain.DoorEvent{
		DoorID:      door.ID,
		UserID:      doorCtx.UserID,
		EventType:   "start",
		PayloadJSON: fmt.Sprintf(`{"node":"%s","session":"%s"}`, doorCtx.NodeID, doorCtx.SessionID),
	})

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.MaxRunSeconds)*time.Second)
	defer cancel()
	var runErr error
	switch strings.ToLower(door.Type) {
	case "external":
		runErr = r.runExternalDoor(runCtx, door, cfg, doorCtx, stdin, stdout, stderr, env)
	default:
		runErr = r.runNativeDoor(runCtx, door, cfg, doorCtx, stdin, stdout)
	}

	_ = r.touchUserMeta(doorCtx.UserID, door.ID)
	if runErr != nil {
		_ = r.repo.AddEvent(&domain.DoorEvent{
			DoorID:      door.ID,
			UserID:      doorCtx.UserID,
			EventType:   "error",
			PayloadJSON: fmt.Sprintf(`{"error":%q}`, truncateJSONValue(runErr.Error(), 280)),
		})
		return runErr
	}
	_ = r.repo.AddEvent(&domain.DoorEvent{
		DoorID:      door.ID,
		UserID:      doorCtx.UserID,
		EventType:   "stop",
		PayloadJSON: "{}",
	})
	return nil
}

func (r *Registry) ToggleFavorite(userID int64, doorID string) (bool, error) {
	doorID = normalizeID(doorID)
	row, err := r.repo.GetUserMeta(userID, doorID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return false, err
	}
	if err != nil || row == nil {
		row = &domain.DoorUserMeta{UserID: userID, DoorID: doorID}
	}
	row.Favorite = !row.Favorite
	row.UpdatedAt = time.Now().UTC()
	if err := r.repo.UpsertUserMeta(row); err != nil {
		return false, err
	}
	return row.Favorite, nil
}

func (r *Registry) AwardAchievement(doorID string, userID int64, code string) error {
	doorID = normalizeID(doorID)
	code = strings.ToLower(strings.TrimSpace(code))
	if doorID == "" || userID <= 0 || code == "" {
		return fmt.Errorf("door_id, user_id, and code are required")
	}
	return r.repo.AddAchievement(&domain.DoorAchievement{
		DoorID:          doorID,
		UserID:          userID,
		AchievementCode: code,
		CreatedAt:       time.Now().UTC(),
	})
}

func (r *Registry) SubmitScore(doorID string, userID int64, scoreType string, value int64, metadataJSON string) error {
	doorID = normalizeID(doorID)
	scoreType = strings.ToLower(strings.TrimSpace(scoreType))
	if doorID == "" || userID <= 0 || scoreType == "" {
		return fmt.Errorf("door_id, user_id, and score_type are required")
	}
	return r.repo.SubmitScore(&domain.DoorScore{
		DoorID:       doorID,
		UserID:       userID,
		ScoreType:    scoreType,
		Value:        value,
		MetadataJSON: strings.TrimSpace(metadataJSON),
		CreatedAt:    time.Now().UTC(),
	})
}

func (r *Registry) TurnsRemaining(userID int64, doorID string, now time.Time) (int, error) {
	door, ok := r.GetByID(doorID)
	if !ok {
		return 0, repository.ErrNotFound
	}
	cfg := r.effectiveConfig(door)
	if cfg.DailyTurns <= 0 {
		return 0, nil
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	ledger, err := r.repo.GetTurn(userID, door.ID, today)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return 0, err
	}
	if errors.Is(err, repository.ErrNotFound) || ledger == nil {
		latest, latestErr := r.repo.GetLatestTurn(userID, door.ID)
		if latestErr == nil && latest != nil {
			carry := latest.TimeBank
			leftover := cfg.DailyTurns - latest.TurnsUsed
			if leftover > 0 {
				carry += leftover
			}
			if carry > cfg.TimeBankMax {
				carry = cfg.TimeBankMax
			}
			return cfg.DailyTurns + carry, nil
		}
		return cfg.DailyTurns, nil
	}
	return maxInt(0, cfg.DailyTurns+ledger.TimeBank-ledger.TurnsUsed), nil
}

func (r *Registry) ListScores(doorID string, limit int) ([]domain.DoorScore, error) {
	return r.repo.ListScores(doorID, "points", limit)
}

func (r *Registry) ListAchievements(userID int64, doorID string, limit int) ([]domain.DoorAchievement, error) {
	return r.repo.ListAchievements(userID, doorID, limit)
}

func (r *Registry) ListFavorites(userID int64, limit int) ([]domain.DoorUserMeta, error) {
	return r.repo.ListFavorites(userID, limit)
}

func (r *Registry) ListRecent(userID int64, limit int) ([]domain.DoorUserMeta, error) {
	return r.repo.ListRecent(userID, limit)
}

func (r *Registry) ListEvents(doorID string, userID int64, limit int) ([]domain.DoorEvent, error) {
	return r.repo.ListEvents(doorID, userID, limit)
}

func (r *Registry) GetUsageStats(doorID string) (*domain.DoorUsageStats, error) {
	return r.repo.GetUsageStats(doorID)
}

func (r *Registry) SetDoorConfig(cfg *domain.DoorConfig) error {
	return r.repo.UpsertConfig(cfg)
}

func (r *Registry) GetDoorConfig(doorID string) (*domain.DoorConfig, error) {
	return r.repo.GetConfig(doorID)
}

func (r *Registry) ResetDoorScores(doorID string) error {
	return r.repo.ResetScores(doorID)
}

func (r *Registry) ensureConfigDefaults(door Door) error {
	if door.ID == "" {
		return nil
	}
	_, err := r.repo.GetConfig(door.ID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	return r.repo.UpsertConfig(&domain.DoorConfig{
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
	})
}

func (r *Registry) effectiveConfig(door Door) domain.DoorConfig {
	cfg := domain.DoorConfig{
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
	row, err := r.repo.GetConfig(door.ID)
	if err == nil && row != nil {
		cfg = *row
	}
	if cfg.MaxRunSeconds <= 0 {
		cfg.MaxRunSeconds = 180
	}
	if cfg.MaxOutputRate <= 0 {
		cfg.MaxOutputRate = 4096
	}
	return cfg
}

func (r *Registry) consumeTurn(userID int64, door Door, cfg domain.DoorConfig, now time.Time) error {
	if userID <= 0 || cfg.DailyTurns <= 0 {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	ledger, err := r.repo.GetTurn(userID, door.ID, day)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	if errors.Is(err, repository.ErrNotFound) || ledger == nil {
		carry := 0
		latest, latestErr := r.repo.GetLatestTurn(userID, door.ID)
		if latestErr == nil && latest != nil {
			carry = latest.TimeBank
			leftover := cfg.DailyTurns - latest.TurnsUsed
			if leftover > 0 {
				carry += leftover
			}
			if carry > cfg.TimeBankMax {
				carry = cfg.TimeBankMax
			}
		}
		ledger = &domain.DoorTurnLedger{
			UserID:    userID,
			DoorID:    door.ID,
			DayKey:    day,
			TurnsUsed: 0,
			TimeBank:  carry,
		}
	}
	remaining := cfg.DailyTurns + ledger.TimeBank - ledger.TurnsUsed
	if remaining <= 0 {
		return fmt.Errorf("no turns remaining today")
	}
	ledger.TurnsUsed++
	if ledger.TurnsUsed > cfg.DailyTurns && ledger.TimeBank > 0 {
		ledger.TimeBank--
	}
	ledger.UpdatedAt = time.Now().UTC()
	return r.repo.UpsertTurn(ledger)
}

func (r *Registry) touchUserMeta(userID int64, doorID string) error {
	if userID <= 0 || doorID == "" {
		return nil
	}
	row, err := r.repo.GetUserMeta(userID, doorID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	now := time.Now().UTC()
	if errors.Is(err, repository.ErrNotFound) || row == nil {
		row = &domain.DoorUserMeta{
			UserID:       userID,
			DoorID:       doorID,
			LastPlayedAt: &now,
			PlayCount:    1,
			UpdatedAt:    now,
		}
		return r.repo.UpsertUserMeta(row)
	}
	row.LastPlayedAt = &now
	row.PlayCount++
	row.UpdatedAt = now
	return r.repo.UpsertUserMeta(row)
}

func (r *Registry) acquireConcurrencyLock(door Door, userID int64) func() {
	mode := strings.ToLower(strings.TrimSpace(door.ConcurrencyMode))
	switch mode {
	case "exclusive":
		r.exclusiveLock.Lock()
		return func() { r.exclusiveLock.Unlock() }
	case "shared":
		return func() {}
	default:
		key := fmt.Sprintf("%s:%d", door.ID, userID)
		r.perUserLocksMu.Lock()
		lock := r.perUserLocks[key]
		if lock == nil {
			lock = &sync.Mutex{}
			r.perUserLocks[key] = lock
		}
		r.perUserLocksMu.Unlock()
		lock.Lock()
		return func() { lock.Unlock() }
	}
}

func (r *Registry) runNativeDoor(ctx context.Context, door Door, cfg domain.DoorConfig, dctx DoorContext, stdin io.Reader, stdout io.Writer) error {
	reader := bufio.NewReader(stdin)
	if handled, err := r.runAdvancedNativeDoor(ctx, door, cfg, dctx, reader, stdout); handled {
		return err
	}
	for {
		io.WriteString(stdout, ui.ClearScreen())
		panel := ui.DrawBox(80, 12, door.Name, []string{
			padRight("Door ID: "+door.ID, 78),
			padRight("Category: "+strings.ToUpper(door.Category)+"  Version: "+door.Version, 78),
			padRight(truncate(door.Description, 78), 78),
			padRight("", 78),
			padRight("(P)lay turn  (H)elp  (R)ules  (S)cores  (A)chievements  (Q)uit", 78),
			padRight("Classic flow: single-letter commands and immediate prompts.", 78),
			padRight("Daily turns and time bank rules apply when configured.", 78),
			padRight("", 78),
			padRight("Enter selection:", 78),
			padRight("", 78),
		}, ui.CP437Box, ui.FgCyan, ui.BgBlack)
		io.WriteString(stdout, panel)
		key, err := readDoorKey(reader)
		if err != nil {
			return err
		}
		switch key {
		case "Q", "ESC":
			return nil
		case "H", "?":
			io.WriteString(stdout, "\r\n"+doorHelp(door)+"\r\nPress any key.")
			_, _ = readDoorKey(reader)
		case "R":
			io.WriteString(stdout, "\r\n"+doorRules(door)+"\r\nPress any key.")
			_, _ = readDoorKey(reader)
		case "S":
			rows, _ := r.repo.ListScores(door.ID, "points", 10)
			io.WriteString(stdout, "\r\nTop Scores\r\n")
			if len(rows) == 0 {
				io.WriteString(stdout, "  no scores yet\r\n")
			}
			for idx, row := range rows {
				io.WriteString(stdout, fmt.Sprintf("  %2d) uid:%d  %d\r\n", idx+1, row.UserID, row.Value))
			}
			io.WriteString(stdout, "Press any key.")
			_, _ = readDoorKey(reader)
		case "A":
			rows, _ := r.repo.ListAchievements(dctx.UserID, door.ID, 20)
			io.WriteString(stdout, "\r\nAchievements\r\n")
			if len(rows) == 0 {
				io.WriteString(stdout, "  none yet\r\n")
			}
			for _, row := range rows {
				io.WriteString(stdout, "  - "+row.AchievementCode+"\r\n")
			}
			io.WriteString(stdout, "Press any key.")
			_, _ = readDoorKey(reader)
		case "P":
			if err := r.playNativeTurn(door, dctx, cfg, stdout); err != nil {
				return err
			}
			io.WriteString(stdout, "\r\nPress any key.")
			_, _ = readDoorKey(reader)
		default:
			io.WriteString(stdout, "\r\nUnknown key. Press any key.")
			_, _ = readDoorKey(reader)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (r *Registry) playNativeTurn(door Door, dctx DoorContext, cfg domain.DoorConfig, stdout io.Writer) error {
	row, err := r.repo.GetUserState(dctx.UserID, door.ID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	state := map[string]int64{
		"plays":        0,
		"total_points": 0,
	}
	if err == nil && row != nil && strings.TrimSpace(row.StateJSON) != "" {
		_ = json.Unmarshal([]byte(row.StateJSON), &state)
	}
	roll := (dctx.Now.UnixNano() + dctx.UserID + int64(len(door.ID))*17) % 91
	if roll < 0 {
		roll = -roll
	}
	points := roll + 10
	state["plays"]++
	state["total_points"] += points
	encoded, _ := json.Marshal(state)
	if err := r.repo.UpsertUserState(&domain.DoorUserState{
		UserID:    dctx.UserID,
		DoorID:    door.ID,
		StateJSON: string(encoded),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		return err
	}
	if err := r.repo.SubmitScore(&domain.DoorScore{
		DoorID:       door.ID,
		UserID:       dctx.UserID,
		ScoreType:    "points",
		Value:        state["total_points"],
		MetadataJSON: fmt.Sprintf(`{"turn_points":%d}`, points),
	}); err != nil {
		return err
	}
	if state["plays"] == 1 {
		_ = r.repo.AddAchievement(&domain.DoorAchievement{
			DoorID:          door.ID,
			UserID:          dctx.UserID,
			AchievementCode: "first_turn",
			CreatedAt:       time.Now().UTC(),
		})
	}
	if state["total_points"] >= 500 {
		_ = r.repo.AddAchievement(&domain.DoorAchievement{
			DoorID:          door.ID,
			UserID:          dctx.UserID,
			AchievementCode: "veteran_500",
			CreatedAt:       time.Now().UTC(),
		})
	}
	io.WriteString(stdout, fmt.Sprintf("\r\nTurn complete in %s.\r\n+%d points (total %d)\r\n", door.Name, points, state["total_points"]))
	return nil
}

func (r *Registry) runExternalDoor(ctx context.Context, door Door, cfg domain.DoorConfig, dctx DoorContext, stdin io.Reader, stdout, stderr io.Writer, env map[string]string) error {
	commandPath, err := resolveDoorCommand(door.Command)
	if err != nil {
		return err
	}
	if err := validateDoorPath(commandPath); err != nil {
		return err
	}
	dataDir := perDoorDataDir(door.ID, dctx.UserID)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	dropfiles, err := writeConfiguredDropfiles(dataDir, dctx)
	if err != nil {
		return err
	}
	cmdName := commandPath
	cmdArgs := append([]string{}, door.Args...)
	if !cfg.AllowNetwork && runtime.GOOS == "linux" {
		if unsharePath, unshareErr := exec.LookPath("unshare"); unshareErr == nil {
			cmdArgs = append([]string{"-n", "--", cmdName}, cmdArgs...)
			cmdName = unsharePath
		}
	}
	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	cmd.Stdin = stdin
	cmd.Stdout = newRateWriter(stdout, cfg.MaxOutputRate)
	cmd.Stderr = newRateWriter(stderr, cfg.MaxOutputRate)
	cmd.Dir = dataDir
	cmd.Env = append(os.Environ(),
		"WOLFBBS_MODE=door",
		"WOLFBBS_DOOR_ID="+door.ID,
		"WOLFBBS_DOOR_DATA_DIR="+dataDir,
		"WOLFBBS_DOOR_NETWORK="+boolText(cfg.AllowNetwork),
	)
	if dropfiles.Door32Path != "" {
		cmd.Env = append(cmd.Env, "WOLFBBS_DOOR32_SYS="+dropfiles.Door32Path)
	}
	if dropfiles.DoorSysPath != "" {
		cmd.Env = append(cmd.Env, "WOLFBBS_DOOR_SYS="+dropfiles.DoorSysPath)
	}
	if dropfiles.DorInfoPath != "" {
		cmd.Env = append(cmd.Env, "WOLFBBS_DORINFO_DEF="+dropfiles.DorInfoPath)
	}
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if !cfg.AllowFSWrite {
		// Best-effort write reduction by making the working dir read-only.
		_ = os.Chmod(dataDir, 0o500)
		defer os.Chmod(dataDir, 0o700)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("door %s failed: %w", door.Name, err)
	}
	return nil
}

func (r *Registry) StartDoorByID(ctx context.Context, doorID string, stdin io.Reader, stdout, stderr io.Writer, env map[string]string) error {
	door, ok := r.GetByID(doorID)
	if !ok {
		return repository.ErrNotFound
	}
	return r.Launch(ctx, door.Hotkey, stdin, stdout, stderr, env)
}

func (r *Registry) DoorByID(doorID string) (Door, bool) {
	return r.GetByID(doorID)
}

func TriviaDoor(binary string) Door {
	if strings.TrimSpace(binary) == "" {
		binary = "wolfbbs-trivia"
	}
	return Door{
		ID:              "trivia-sample",
		Name:            "Trivia",
		Description:     "Simple terminal trivia game",
		Category:        "utility",
		Type:            "external",
		Command:         binary,
		Hotkey:          "T",
		ConcurrencyMode: "shared",
		EnabledDefault:  true,
		DailyTurns:      0,
		TimeBankMax:     0,
		MaxRunSec:       120,
		MaxOutputRate:   4096,
	}
}

func SeedTrivia(r *Registry, binary string) {
	r.Register(TriviaDoor(binary))
}

func SeedFromEnv(r *Registry) {
	raw := strings.TrimSpace(os.Getenv("WOLFBBS_DOORS"))
	if raw == "" {
		return
	}
	entries := strings.Split(raw, ";")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "|")
		if len(parts) < 3 {
			continue
		}
		door := Door{
			ID:              normalizeID(strings.TrimSpace(parts[1])),
			Hotkey:          strings.TrimSpace(parts[0]),
			Name:            strings.TrimSpace(parts[1]),
			Description:     "External door registered via WOLFBBS_DOORS",
			Category:        "utility",
			Type:            "external",
			Command:         strings.TrimSpace(parts[2]),
			ConcurrencyMode: "shared",
			EnabledDefault:  true,
			DailyTurns:      0,
			MaxRunSec:       180,
			MaxOutputRate:   4096,
		}
		if len(parts) >= 4 {
			args := []string{}
			for _, arg := range strings.Split(parts[3], ",") {
				arg = strings.TrimSpace(arg)
				if arg != "" {
					args = append(args, arg)
				}
			}
			door.Args = args
		}
		if door.ID == "" {
			door.ID = normalizeID(door.Name)
		}
		r.Register(door)
	}
}

func DoorTimeout(ctx context.Context, seconds time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, seconds)
}

func resolveDoorCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("door command is required")
	}
	if strings.Contains(command, "/") {
		abs, err := filepath.Abs(command)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	found, err := exec.LookPath(command)
	if err != nil {
		return "", err
	}
	return found, nil
}

func validateDoorPath(commandPath string) error {
	allowed := strings.TrimSpace(os.Getenv("WOLFBBS_DOOR_ALLOW_DIR"))
	if allowed == "" {
		return nil
	}
	allowAbs, err := filepath.Abs(allowed)
	if err != nil {
		return err
	}
	cmdAbs, err := filepath.Abs(commandPath)
	if err != nil {
		return err
	}
	allowAbs = filepath.Clean(allowAbs)
	cmdAbs = filepath.Clean(cmdAbs)
	if cmdAbs == allowAbs {
		return nil
	}
	prefix := allowAbs + string(filepath.Separator)
	if !strings.HasPrefix(cmdAbs, prefix) {
		return fmt.Errorf("door command %q is outside allow directory %q", cmdAbs, allowAbs)
	}
	return nil
}

func parseDoorContext(doorID string, env map[string]string) DoorContext {
	now := time.Now().UTC()
	userID, _ := strconv.ParseInt(strings.TrimSpace(env["WOLFBBS_USER_ID"]), 10, 64)
	cols, _ := strconv.Atoi(strings.TrimSpace(env["WOLFBBS_TERM_COLS"]))
	rows, _ := strconv.Atoi(strings.TrimSpace(env["WOLFBBS_TERM_ROWS"]))
	ansi := strings.EqualFold(strings.TrimSpace(env["WOLFBBS_ANSI"]), "true") || strings.TrimSpace(env["WOLFBBS_ANSI"]) == "1"
	colorDepth, _ := strconv.Atoi(strings.TrimSpace(env["WOLFBBS_COLOR_DEPTH"]))
	createdAt, _ := time.Parse(time.RFC3339, strings.TrimSpace(env["WOLFBBS_USER_CREATED_AT"]))
	return DoorContext{
		UserID:        userID,
		Username:      strings.TrimSpace(env["WOLFBBS_HANDLE"]),
		Role:          strings.ToLower(strings.TrimSpace(env["WOLFBBS_ROLE"])),
		UserCreatedAt: createdAt,
		NodeID:        strings.TrimSpace(env["WOLFBBS_NODE"]),
		SessionID:     strings.TrimSpace(env["WOLFBBS_SESSION_ID"]),
		Cols:          cols,
		Rows:          rows,
		ANSI:          ansi,
		Encoding:      strings.ToLower(strings.TrimSpace(env["WOLFBBS_ENCODING"])),
		ColorDepth:    colorDepth,
		Timezone:      strings.TrimSpace(env["WOLFBBS_TZ"]),
		Now:           now,
		StorageNS:     fmt.Sprintf("doors/%s/user/%d", normalizeID(doorID), userID),
	}
}

func doorFromManifest(mf Manifest) Door {
	doorID := normalizeID(mf.ID)
	name := strings.TrimSpace(mf.DisplayName)
	if name == "" {
		name = doorID
	}
	category := strings.ToLower(strings.TrimSpace(mf.Category))
	if category == "" {
		category = "utility"
	}
	entryType := strings.ToLower(strings.TrimSpace(mf.EntryType))
	if entryType == "" {
		entryType = "native"
	}
	requiredRole := strings.ToLower(strings.TrimSpace(mf.RequiredRole))
	if requiredRole == "" {
		requiredRole = "user"
	}
	return Door{
		ID:              doorID,
		Name:            name,
		Description:     strings.TrimSpace(mf.ShortDesc),
		Category:        category,
		Version:         strings.TrimSpace(mf.Version),
		Type:            entryType,
		Command:         strings.TrimSpace(mf.Entrypoint),
		Args:            append([]string{}, mf.Args...),
		Hotkey:          normalizeHotkey(mf.Hotkey),
		ConcurrencyMode: strings.ToLower(strings.TrimSpace(mf.ConcurrencyMode)),
		RequiresANSI:    mf.Requires.ANSI,
		MinCols:         mf.Requires.MinCols,
		MinRows:         mf.Requires.MinRows,
		NeedsNetwork:    mf.Capabilities.NeedsNetwork,
		NeedsFSWrite:    mf.Capabilities.NeedsFilesystemWrite,
		MessagesDays:    mf.DataRetention.MessagesDays,
		LogsDays:        mf.DataRetention.LogsDays,
		EnabledDefault:  mf.AdminDefaults.Enabled,
		DailyTurns:      mf.AdminDefaults.DailyTurns,
		TimeBankMax:     mf.AdminDefaults.TimeBankMax,
		ResetHour:       mf.AdminDefaults.ResetHour,
		MaxRunSec:       mf.AdminDefaults.MaxRunSec,
		MaxOutputRate:   mf.AdminDefaults.MaxOutputRate,
		RequiredRole:    requiredRole,
		HelpText:        strings.TrimSpace(mf.HelpText),
		RulesText:       strings.TrimSpace(mf.RulesText),
	}
}

func normalizeHotkey(h string) string {
	h = strings.ToUpper(strings.TrimSpace(h))
	if len(h) > 1 {
		return strings.ToUpper(string([]rune(h)[0]))
	}
	return h
}

func normalizeID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return ""
	}
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "_", "-")
	return id
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func roleAllowed(userRole, required string) bool {
	required = strings.ToLower(strings.TrimSpace(required))
	if required == "" || required == "any" || required == "user" {
		return true
	}
	if required == "verified_user" {
		// Verification enforcement can be layered at caller level.
		return rbac.AtLeast(userRole, rbac.RoleUser)
	}
	return rbac.AtLeast(userRole, required)
}

func truncate(value string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func padRight(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(value) >= width {
		return value[:width]
	}
	return value + strings.Repeat(" ", width-len(value))
}

func doorHelp(door Door) string {
	text := strings.TrimSpace(door.HelpText)
	if text != "" {
		return text
	}
	return fmt.Sprintf("%s help:\n- Use single-letter commands.\n- Take turns to gain score.\n- Visit scores and achievements from the menu.", door.Name)
}

func doorRules(door Door) string {
	text := strings.TrimSpace(door.RulesText)
	if text != "" {
		return text
	}
	return fmt.Sprintf("%s rules:\n- Be respectful in PvP-facing doors.\n- No automation or flooding.\n- Daily turn limits may apply.", door.Name)
}

func boolText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func truncateJSONValue(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func perDoorDataDir(doorID string, userID int64) string {
	root := strings.TrimSpace(os.Getenv("WOLFBBS_DOOR_DATA_ROOT"))
	if root == "" {
		root = ".wolfbbs/doors"
	}
	return filepath.Join(root, normalizeID(doorID), strconv.FormatInt(userID, 10))
}

type rateWriter struct {
	inner     io.Writer
	maxPerSec int
	written   int
	window    time.Time
}

func newRateWriter(inner io.Writer, maxPerSec int) io.Writer {
	if inner == nil {
		inner = io.Discard
	}
	if maxPerSec <= 0 {
		maxPerSec = 4096
	}
	return &rateWriter{inner: inner, maxPerSec: maxPerSec, window: time.Now()}
}

func (w *rateWriter) Write(p []byte) (int, error) {
	now := time.Now()
	if now.Sub(w.window) >= time.Second {
		w.window = now
		w.written = 0
	}
	remain := w.maxPerSec - w.written
	if remain <= 0 {
		time.Sleep(time.Until(w.window.Add(time.Second)))
		w.window = time.Now()
		w.written = 0
		remain = w.maxPerSec
	}
	if len(p) > remain {
		p = p[:remain]
	}
	n, err := w.inner.Write(p)
	w.written += n
	return n, err
}

func readDoorKey(reader *bufio.Reader) (string, error) {
	ch, err := reader.ReadByte()
	if err != nil {
		return "", err
	}
	switch ch {
	case '\r', '\n':
		return "ENTER", nil
	case 0x1b:
		return "ESC", nil
	default:
		if ch >= 32 && ch <= 126 {
			return strings.ToUpper(string(ch)), nil
		}
		return "", nil
	}
}
