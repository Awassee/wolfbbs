package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wolfbbs/internal/domain"
)

type DoorRepository interface {
	UpsertConfig(cfg *domain.DoorConfig) error
	GetConfig(doorID string) (*domain.DoorConfig, error)
	ListConfigs() ([]domain.DoorConfig, error)

	UpsertUserState(state *domain.DoorUserState) error
	GetUserState(userID int64, doorID string) (*domain.DoorUserState, error)

	UpsertGlobalState(state *domain.DoorGlobalState) error
	GetGlobalState(doorID string) (*domain.DoorGlobalState, error)

	AddEvent(event *domain.DoorEvent) error
	ListEvents(doorID string, userID int64, limit int) ([]domain.DoorEvent, error)

	UpsertTurn(turn *domain.DoorTurnLedger) error
	GetTurn(userID int64, doorID string, dayKey time.Time) (*domain.DoorTurnLedger, error)
	GetLatestTurn(userID int64, doorID string) (*domain.DoorTurnLedger, error)

	AddAchievement(row *domain.DoorAchievement) error
	ListAchievements(userID int64, doorID string, limit int) ([]domain.DoorAchievement, error)

	SubmitScore(score *domain.DoorScore) error
	ListScores(doorID, scoreType string, limit int) ([]domain.DoorScore, error)
	ResetScores(doorID string) error

	UpsertUserMeta(meta *domain.DoorUserMeta) error
	GetUserMeta(userID int64, doorID string) (*domain.DoorUserMeta, error)
	ListFavorites(userID int64, limit int) ([]domain.DoorUserMeta, error)
	ListRecent(userID int64, limit int) ([]domain.DoorUserMeta, error)
	GetUsageStats(doorID string) (*domain.DoorUsageStats, error)
}

type InMemoryDoorRepository struct {
	mu           sync.Mutex
	nextEventID  int64
	nextScoreID  int64
	configs      map[string]domain.DoorConfig
	userStates   map[string]domain.DoorUserState
	globalStates map[string]domain.DoorGlobalState
	events       []domain.DoorEvent
	turns        map[string]domain.DoorTurnLedger
	achievements map[string]domain.DoorAchievement
	scores       []domain.DoorScore
	userMeta     map[string]domain.DoorUserMeta
}

func NewInMemoryDoorRepository() *InMemoryDoorRepository {
	return &InMemoryDoorRepository{
		nextEventID:  1,
		nextScoreID:  1,
		configs:      map[string]domain.DoorConfig{},
		userStates:   map[string]domain.DoorUserState{},
		globalStates: map[string]domain.DoorGlobalState{},
		events:       []domain.DoorEvent{},
		turns:        map[string]domain.DoorTurnLedger{},
		achievements: map[string]domain.DoorAchievement{},
		scores:       []domain.DoorScore{},
		userMeta:     map[string]domain.DoorUserMeta{},
	}
}

func normalizeDoorID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func dayKeyUTC(day time.Time) time.Time {
	if day.IsZero() {
		day = time.Now().UTC()
	}
	return time.Date(day.UTC().Year(), day.UTC().Month(), day.UTC().Day(), 0, 0, 0, 0, time.UTC)
}

func userDoorKey(userID int64, doorID string) string {
	return strconv.FormatInt(userID, 10) + "::" + normalizeDoorID(doorID)
}

func userDoorDayKey(userID int64, doorID string, day time.Time) string {
	return userDoorKey(userID, doorID) + "::" + dayKeyUTC(day).Format("2006-01-02")
}

func achKey(userID int64, doorID, code string) string {
	return userDoorKey(userID, doorID) + "::" + strings.ToLower(strings.TrimSpace(code))
}

func (r *InMemoryDoorRepository) UpsertConfig(cfg *domain.DoorConfig) error {
	if cfg == nil {
		return errors.New("door config is required")
	}
	doorID := normalizeDoorID(cfg.DoorID)
	if doorID == "" {
		return errors.New("door id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	copyCfg := *cfg
	copyCfg.DoorID = doorID
	if copyCfg.UpdatedAt.IsZero() {
		copyCfg.UpdatedAt = time.Now().UTC()
	}
	r.configs[doorID] = copyCfg
	return nil
}

func (r *InMemoryDoorRepository) GetConfig(doorID string) (*domain.DoorConfig, error) {
	doorID = normalizeDoorID(doorID)
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.configs[doorID]
	if !ok {
		return nil, ErrNotFound
	}
	out := row
	return &out, nil
}

func (r *InMemoryDoorRepository) ListConfigs() ([]domain.DoorConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.DoorConfig, 0, len(r.configs))
	for _, row := range r.configs {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DoorID < out[j].DoorID })
	return out, nil
}

func (r *InMemoryDoorRepository) UpsertUserState(state *domain.DoorUserState) error {
	if state == nil {
		return errors.New("door user state is required")
	}
	if state.UserID <= 0 {
		return errors.New("user id is required")
	}
	doorID := normalizeDoorID(state.DoorID)
	if doorID == "" {
		return errors.New("door id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	row := *state
	row.DoorID = doorID
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	r.userStates[userDoorKey(row.UserID, row.DoorID)] = row
	return nil
}

func (r *InMemoryDoorRepository) GetUserState(userID int64, doorID string) (*domain.DoorUserState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.userStates[userDoorKey(userID, doorID)]
	if !ok {
		return nil, ErrNotFound
	}
	out := row
	return &out, nil
}

func (r *InMemoryDoorRepository) UpsertGlobalState(state *domain.DoorGlobalState) error {
	if state == nil {
		return errors.New("door global state is required")
	}
	doorID := normalizeDoorID(state.DoorID)
	if doorID == "" {
		return errors.New("door id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	row := *state
	row.DoorID = doorID
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	r.globalStates[doorID] = row
	return nil
}

func (r *InMemoryDoorRepository) GetGlobalState(doorID string) (*domain.DoorGlobalState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.globalStates[normalizeDoorID(doorID)]
	if !ok {
		return nil, ErrNotFound
	}
	out := row
	return &out, nil
}

func (r *InMemoryDoorRepository) AddEvent(event *domain.DoorEvent) error {
	if event == nil {
		return errors.New("door event is required")
	}
	doorID := normalizeDoorID(event.DoorID)
	if doorID == "" {
		return errors.New("door id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	row := *event
	row.ID = r.nextEventID
	r.nextEventID++
	row.DoorID = doorID
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	}
	r.events = append(r.events, row)
	if len(r.events) > 10000 {
		r.events = r.events[len(r.events)-10000:]
	}
	event.ID = row.ID
	event.CreatedAt = row.CreatedAt
	return nil
}

func (r *InMemoryDoorRepository) ListEvents(doorID string, userID int64, limit int) ([]domain.DoorEvent, error) {
	doorID = normalizeDoorID(doorID)
	if limit <= 0 {
		limit = 200
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.DoorEvent, 0, limit)
	for i := len(r.events) - 1; i >= 0; i-- {
		row := r.events[i]
		if doorID != "" && row.DoorID != doorID {
			continue
		}
		if userID > 0 && row.UserID != userID {
			continue
		}
		out = append(out, row)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *InMemoryDoorRepository) UpsertTurn(turn *domain.DoorTurnLedger) error {
	if turn == nil {
		return errors.New("door turn ledger is required")
	}
	if turn.UserID <= 0 {
		return errors.New("user id is required")
	}
	doorID := normalizeDoorID(turn.DoorID)
	if doorID == "" {
		return errors.New("door id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	row := *turn
	row.DoorID = doorID
	row.DayKey = dayKeyUTC(row.DayKey)
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	r.turns[userDoorDayKey(row.UserID, row.DoorID, row.DayKey)] = row
	return nil
}

func (r *InMemoryDoorRepository) GetTurn(userID int64, doorID string, dayKey time.Time) (*domain.DoorTurnLedger, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.turns[userDoorDayKey(userID, doorID, dayKey)]
	if !ok {
		return nil, ErrNotFound
	}
	out := row
	return &out, nil
}

func (r *InMemoryDoorRepository) GetLatestTurn(userID int64, doorID string) (*domain.DoorTurnLedger, error) {
	doorID = normalizeDoorID(doorID)
	r.mu.Lock()
	defer r.mu.Unlock()
	var (
		found bool
		best  domain.DoorTurnLedger
	)
	for _, row := range r.turns {
		if row.UserID != userID || row.DoorID != doorID {
			continue
		}
		if !found || row.DayKey.After(best.DayKey) {
			best = row
			found = true
		}
	}
	if !found {
		return nil, ErrNotFound
	}
	out := best
	return &out, nil
}

func (r *InMemoryDoorRepository) AddAchievement(row *domain.DoorAchievement) error {
	if row == nil {
		return errors.New("door achievement is required")
	}
	if row.UserID <= 0 {
		return errors.New("user id is required")
	}
	doorID := normalizeDoorID(row.DoorID)
	code := strings.ToLower(strings.TrimSpace(row.AchievementCode))
	if doorID == "" || code == "" {
		return errors.New("door id and achievement code are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := achKey(row.UserID, doorID, code)
	if _, ok := r.achievements[key]; ok {
		return nil
	}
	copyRow := *row
	copyRow.DoorID = doorID
	copyRow.AchievementCode = code
	if copyRow.CreatedAt.IsZero() {
		copyRow.CreatedAt = time.Now().UTC()
	}
	r.achievements[key] = copyRow
	return nil
}

func (r *InMemoryDoorRepository) ListAchievements(userID int64, doorID string, limit int) ([]domain.DoorAchievement, error) {
	doorID = normalizeDoorID(doorID)
	if limit <= 0 {
		limit = 200
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.DoorAchievement, 0, limit)
	for _, row := range r.achievements {
		if userID > 0 && row.UserID != userID {
			continue
		}
		if doorID != "" && row.DoorID != doorID {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryDoorRepository) SubmitScore(score *domain.DoorScore) error {
	if score == nil {
		return errors.New("door score is required")
	}
	if score.UserID <= 0 {
		return errors.New("user id is required")
	}
	doorID := normalizeDoorID(score.DoorID)
	scoreType := strings.ToLower(strings.TrimSpace(score.ScoreType))
	if doorID == "" || scoreType == "" {
		return errors.New("door id and score type are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	row := *score
	row.ID = r.nextScoreID
	r.nextScoreID++
	row.DoorID = doorID
	row.ScoreType = scoreType
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	}
	r.scores = append(r.scores, row)
	score.ID = row.ID
	score.CreatedAt = row.CreatedAt
	return nil
}

func (r *InMemoryDoorRepository) ListScores(doorID, scoreType string, limit int) ([]domain.DoorScore, error) {
	doorID = normalizeDoorID(doorID)
	scoreType = strings.ToLower(strings.TrimSpace(scoreType))
	if limit <= 0 {
		limit = 20
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.DoorScore, 0, limit)
	for _, row := range r.scores {
		if doorID != "" && row.DoorID != doorID {
			continue
		}
		if scoreType != "" && row.ScoreType != scoreType {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value == out[j].Value {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Value > out[j].Value
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryDoorRepository) ResetScores(doorID string) error {
	doorID = normalizeDoorID(doorID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if doorID == "" {
		r.scores = r.scores[:0]
		return nil
	}
	out := make([]domain.DoorScore, 0, len(r.scores))
	for _, row := range r.scores {
		if row.DoorID == doorID {
			continue
		}
		out = append(out, row)
	}
	r.scores = out
	return nil
}

func (r *InMemoryDoorRepository) UpsertUserMeta(meta *domain.DoorUserMeta) error {
	if meta == nil {
		return errors.New("door user meta is required")
	}
	if meta.UserID <= 0 {
		return errors.New("user id is required")
	}
	doorID := normalizeDoorID(meta.DoorID)
	if doorID == "" {
		return errors.New("door id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	row := *meta
	row.DoorID = doorID
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	r.userMeta[userDoorKey(row.UserID, row.DoorID)] = row
	return nil
}

func (r *InMemoryDoorRepository) GetUserMeta(userID int64, doorID string) (*domain.DoorUserMeta, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.userMeta[userDoorKey(userID, doorID)]
	if !ok {
		return nil, ErrNotFound
	}
	out := row
	return &out, nil
}

func (r *InMemoryDoorRepository) ListFavorites(userID int64, limit int) ([]domain.DoorUserMeta, error) {
	return r.listMeta(userID, limit, func(row domain.DoorUserMeta) bool {
		return row.Favorite
	}, func(i, j domain.DoorUserMeta) bool {
		return i.DoorID < j.DoorID
	})
}

func (r *InMemoryDoorRepository) ListRecent(userID int64, limit int) ([]domain.DoorUserMeta, error) {
	return r.listMeta(userID, limit, func(row domain.DoorUserMeta) bool {
		return row.LastPlayedAt != nil
	}, func(i, j domain.DoorUserMeta) bool {
		return i.LastPlayedAt.After(*j.LastPlayedAt)
	})
}

func (r *InMemoryDoorRepository) listMeta(userID int64, limit int, keep func(domain.DoorUserMeta) bool, less func(domain.DoorUserMeta, domain.DoorUserMeta) bool) ([]domain.DoorUserMeta, error) {
	if limit <= 0 {
		limit = 20
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.DoorUserMeta, 0, limit)
	for _, row := range r.userMeta {
		if userID > 0 && row.UserID != userID {
			continue
		}
		if !keep(row) {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryDoorRepository) GetUsageStats(doorID string) (*domain.DoorUsageStats, error) {
	doorID = normalizeDoorID(doorID)
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	dayStart := now.Add(-24 * time.Hour)
	monthStart := now.AddDate(0, 0, -30)
	dayUsers := map[int64]struct{}{}
	monthUsers := map[int64]struct{}{}
	var plays int64
	for _, row := range r.userMeta {
		if doorID != "" && row.DoorID != doorID {
			continue
		}
		plays += int64(row.PlayCount)
		if row.LastPlayedAt != nil && row.LastPlayedAt.After(dayStart) {
			dayUsers[row.UserID] = struct{}{}
		}
		if row.LastPlayedAt != nil && row.LastPlayedAt.After(monthStart) {
			monthUsers[row.UserID] = struct{}{}
		}
	}
	return &domain.DoorUsageStats{
		DoorID:        doorID,
		DailyActive:   len(dayUsers),
		MonthlyActive: len(monthUsers),
		TotalPlays:    plays,
	}, nil
}

type PostgresDoorRepository struct {
	db *sql.DB
}

func NewPostgresDoorRepository(db *sql.DB) *PostgresDoorRepository {
	return &PostgresDoorRepository{db: db}
}

func (r *PostgresDoorRepository) UpsertConfig(cfg *domain.DoorConfig) error {
	if cfg == nil {
		return errors.New("door config is required")
	}
	doorID := normalizeDoorID(cfg.DoorID)
	if doorID == "" {
		return errors.New("door id is required")
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO door_configs (
  door_id, enabled, daily_turns, time_bank_max, reset_hour_local,
  required_role_override, messages_days, logs_days, max_run_seconds,
  max_output_rate, allow_network, allow_fs_write, updated_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now()
)
ON CONFLICT (door_id) DO UPDATE SET
  enabled = EXCLUDED.enabled,
  daily_turns = EXCLUDED.daily_turns,
  time_bank_max = EXCLUDED.time_bank_max,
  reset_hour_local = EXCLUDED.reset_hour_local,
  required_role_override = EXCLUDED.required_role_override,
  messages_days = EXCLUDED.messages_days,
  logs_days = EXCLUDED.logs_days,
  max_run_seconds = EXCLUDED.max_run_seconds,
  max_output_rate = EXCLUDED.max_output_rate,
  allow_network = EXCLUDED.allow_network,
  allow_fs_write = EXCLUDED.allow_fs_write,
  updated_at = now()`,
		doorID, cfg.Enabled, cfg.DailyTurns, cfg.TimeBankMax, cfg.ResetHourLocal,
		strings.TrimSpace(cfg.RequiredRoleOverride), cfg.MessagesDays, cfg.LogsDays,
		cfg.MaxRunSeconds, cfg.MaxOutputRate, cfg.AllowNetwork, cfg.AllowFSWrite,
	)
	return err
}

func (r *PostgresDoorRepository) GetConfig(doorID string) (*domain.DoorConfig, error) {
	doorID = normalizeDoorID(doorID)
	var row domain.DoorConfig
	err := r.db.QueryRowContext(context.Background(), `
SELECT door_id, enabled, daily_turns, time_bank_max, reset_hour_local,
       required_role_override, messages_days, logs_days, max_run_seconds,
       max_output_rate, allow_network, allow_fs_write, updated_at
FROM door_configs WHERE door_id = $1`, doorID).Scan(
		&row.DoorID, &row.Enabled, &row.DailyTurns, &row.TimeBankMax, &row.ResetHourLocal,
		&row.RequiredRoleOverride, &row.MessagesDays, &row.LogsDays, &row.MaxRunSeconds,
		&row.MaxOutputRate, &row.AllowNetwork, &row.AllowFSWrite, &row.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *PostgresDoorRepository) ListConfigs() ([]domain.DoorConfig, error) {
	rows, err := r.db.QueryContext(context.Background(), `
SELECT door_id, enabled, daily_turns, time_bank_max, reset_hour_local,
       required_role_override, messages_days, logs_days, max_run_seconds,
       max_output_rate, allow_network, allow_fs_write, updated_at
FROM door_configs
ORDER BY door_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.DoorConfig, 0)
	for rows.Next() {
		var row domain.DoorConfig
		if err := rows.Scan(
			&row.DoorID, &row.Enabled, &row.DailyTurns, &row.TimeBankMax, &row.ResetHourLocal,
			&row.RequiredRoleOverride, &row.MessagesDays, &row.LogsDays, &row.MaxRunSeconds,
			&row.MaxOutputRate, &row.AllowNetwork, &row.AllowFSWrite, &row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresDoorRepository) UpsertUserState(state *domain.DoorUserState) error {
	if state == nil {
		return errors.New("door user state is required")
	}
	if state.UserID <= 0 {
		return errors.New("user id is required")
	}
	doorID := normalizeDoorID(state.DoorID)
	if doorID == "" {
		return errors.New("door id is required")
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO door_user_state(user_id, door_id, state_json, updated_at)
VALUES ($1,$2,$3,now())
ON CONFLICT (user_id, door_id) DO UPDATE SET
  state_json = EXCLUDED.state_json,
  updated_at = now()`,
		state.UserID, doorID, strings.TrimSpace(state.StateJSON),
	)
	return err
}

func (r *PostgresDoorRepository) GetUserState(userID int64, doorID string) (*domain.DoorUserState, error) {
	doorID = normalizeDoorID(doorID)
	var row domain.DoorUserState
	err := r.db.QueryRowContext(context.Background(), `
SELECT user_id, door_id, state_json, updated_at
FROM door_user_state
WHERE user_id = $1 AND door_id = $2`,
		userID, doorID,
	).Scan(&row.UserID, &row.DoorID, &row.StateJSON, &row.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *PostgresDoorRepository) UpsertGlobalState(state *domain.DoorGlobalState) error {
	if state == nil {
		return errors.New("door global state is required")
	}
	doorID := normalizeDoorID(state.DoorID)
	if doorID == "" {
		return errors.New("door id is required")
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO door_global_state(door_id, state_json, updated_at)
VALUES ($1,$2,now())
ON CONFLICT (door_id) DO UPDATE SET
  state_json = EXCLUDED.state_json,
  updated_at = now()`,
		doorID, strings.TrimSpace(state.StateJSON),
	)
	return err
}

func (r *PostgresDoorRepository) GetGlobalState(doorID string) (*domain.DoorGlobalState, error) {
	doorID = normalizeDoorID(doorID)
	var row domain.DoorGlobalState
	err := r.db.QueryRowContext(context.Background(), `
SELECT door_id, state_json, updated_at
FROM door_global_state
WHERE door_id = $1`, doorID).Scan(&row.DoorID, &row.StateJSON, &row.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *PostgresDoorRepository) AddEvent(event *domain.DoorEvent) error {
	if event == nil {
		return errors.New("door event is required")
	}
	doorID := normalizeDoorID(event.DoorID)
	if doorID == "" {
		return errors.New("door id is required")
	}
	err := r.db.QueryRowContext(context.Background(), `
INSERT INTO door_event_log(door_id, user_id, event_type, payload_json, created_at)
VALUES ($1,$2,$3,$4,now())
RETURNING id, created_at`,
		doorID, event.UserID, strings.TrimSpace(event.EventType), strings.TrimSpace(event.PayloadJSON),
	).Scan(&event.ID, &event.CreatedAt)
	return err
}

func (r *PostgresDoorRepository) ListEvents(doorID string, userID int64, limit int) ([]domain.DoorEvent, error) {
	doorID = normalizeDoorID(doorID)
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, door_id, user_id, event_type, payload_json, created_at
FROM door_event_log
WHERE ($1 = '' OR door_id = $1)
  AND ($2 <= 0 OR user_id = $2)
ORDER BY id DESC
LIMIT $3`, doorID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.DoorEvent, 0, limit)
	for rows.Next() {
		var row domain.DoorEvent
		if err := rows.Scan(&row.ID, &row.DoorID, &row.UserID, &row.EventType, &row.PayloadJSON, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresDoorRepository) UpsertTurn(turn *domain.DoorTurnLedger) error {
	if turn == nil {
		return errors.New("door turn ledger is required")
	}
	if turn.UserID <= 0 {
		return errors.New("user id is required")
	}
	doorID := normalizeDoorID(turn.DoorID)
	if doorID == "" {
		return errors.New("door id is required")
	}
	day := dayKeyUTC(turn.DayKey)
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO door_turn_bank(user_id, door_id, day_key, turns_used, time_bank, updated_at)
VALUES ($1,$2,$3,$4,$5,now())
ON CONFLICT (user_id, door_id, day_key) DO UPDATE SET
  turns_used = EXCLUDED.turns_used,
  time_bank = EXCLUDED.time_bank,
  updated_at = now()`,
		turn.UserID, doorID, day, turn.TurnsUsed, turn.TimeBank,
	)
	return err
}

func (r *PostgresDoorRepository) GetTurn(userID int64, doorID string, dayKey time.Time) (*domain.DoorTurnLedger, error) {
	doorID = normalizeDoorID(doorID)
	var row domain.DoorTurnLedger
	err := r.db.QueryRowContext(context.Background(), `
SELECT user_id, door_id, day_key, turns_used, time_bank, updated_at
FROM door_turn_bank
WHERE user_id = $1 AND door_id = $2 AND day_key = $3`,
		userID, doorID, dayKeyUTC(dayKey),
	).Scan(&row.UserID, &row.DoorID, &row.DayKey, &row.TurnsUsed, &row.TimeBank, &row.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *PostgresDoorRepository) GetLatestTurn(userID int64, doorID string) (*domain.DoorTurnLedger, error) {
	doorID = normalizeDoorID(doorID)
	var row domain.DoorTurnLedger
	err := r.db.QueryRowContext(context.Background(), `
SELECT user_id, door_id, day_key, turns_used, time_bank, updated_at
FROM door_turn_bank
WHERE user_id = $1 AND door_id = $2
ORDER BY day_key DESC
LIMIT 1`, userID, doorID).Scan(&row.UserID, &row.DoorID, &row.DayKey, &row.TurnsUsed, &row.TimeBank, &row.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *PostgresDoorRepository) AddAchievement(row *domain.DoorAchievement) error {
	if row == nil {
		return errors.New("door achievement is required")
	}
	if row.UserID <= 0 {
		return errors.New("user id is required")
	}
	doorID := normalizeDoorID(row.DoorID)
	code := strings.ToLower(strings.TrimSpace(row.AchievementCode))
	if doorID == "" || code == "" {
		return errors.New("door id and achievement code are required")
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO door_achievements(door_id, user_id, achievement_code, created_at)
VALUES ($1,$2,$3,now())
ON CONFLICT (door_id, user_id, achievement_code) DO NOTHING`,
		doorID, row.UserID, code,
	)
	return err
}

func (r *PostgresDoorRepository) ListAchievements(userID int64, doorID string, limit int) ([]domain.DoorAchievement, error) {
	doorID = normalizeDoorID(doorID)
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT door_id, user_id, achievement_code, created_at
FROM door_achievements
WHERE ($1 <= 0 OR user_id = $1)
  AND ($2 = '' OR door_id = $2)
ORDER BY created_at DESC
LIMIT $3`, userID, doorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.DoorAchievement, 0, limit)
	for rows.Next() {
		var row domain.DoorAchievement
		if err := rows.Scan(&row.DoorID, &row.UserID, &row.AchievementCode, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresDoorRepository) SubmitScore(score *domain.DoorScore) error {
	if score == nil {
		return errors.New("door score is required")
	}
	if score.UserID <= 0 {
		return errors.New("user id is required")
	}
	doorID := normalizeDoorID(score.DoorID)
	scoreType := strings.ToLower(strings.TrimSpace(score.ScoreType))
	if doorID == "" || scoreType == "" {
		return errors.New("door id and score type are required")
	}
	return r.db.QueryRowContext(context.Background(), `
INSERT INTO door_scores(door_id, user_id, score_type, value, metadata_json, created_at)
VALUES ($1,$2,$3,$4,$5,now())
RETURNING id, created_at`,
		doorID, score.UserID, scoreType, score.Value, strings.TrimSpace(score.MetadataJSON),
	).Scan(&score.ID, &score.CreatedAt)
}

func (r *PostgresDoorRepository) ListScores(doorID, scoreType string, limit int) ([]domain.DoorScore, error) {
	doorID = normalizeDoorID(doorID)
	scoreType = strings.ToLower(strings.TrimSpace(scoreType))
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, door_id, user_id, score_type, value, metadata_json, created_at
FROM door_scores
WHERE ($1 = '' OR door_id = $1)
  AND ($2 = '' OR score_type = $2)
ORDER BY value DESC, created_at ASC
LIMIT $3`, doorID, scoreType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.DoorScore, 0, limit)
	for rows.Next() {
		var row domain.DoorScore
		if err := rows.Scan(&row.ID, &row.DoorID, &row.UserID, &row.ScoreType, &row.Value, &row.MetadataJSON, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresDoorRepository) ResetScores(doorID string) error {
	doorID = normalizeDoorID(doorID)
	if doorID == "" {
		_, err := r.db.ExecContext(context.Background(), `DELETE FROM door_scores`)
		return err
	}
	_, err := r.db.ExecContext(context.Background(), `DELETE FROM door_scores WHERE door_id = $1`, doorID)
	return err
}

func (r *PostgresDoorRepository) UpsertUserMeta(meta *domain.DoorUserMeta) error {
	if meta == nil {
		return errors.New("door user meta is required")
	}
	if meta.UserID <= 0 {
		return errors.New("user id is required")
	}
	doorID := normalizeDoorID(meta.DoorID)
	if doorID == "" {
		return errors.New("door id is required")
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO door_user_meta(user_id, door_id, favorite, last_played_at, play_count, updated_at)
VALUES ($1,$2,$3,$4,$5,now())
ON CONFLICT (user_id, door_id) DO UPDATE SET
  favorite = EXCLUDED.favorite,
  last_played_at = EXCLUDED.last_played_at,
  play_count = EXCLUDED.play_count,
  updated_at = now()`,
		meta.UserID, doorID, meta.Favorite, meta.LastPlayedAt, meta.PlayCount,
	)
	return err
}

func (r *PostgresDoorRepository) GetUserMeta(userID int64, doorID string) (*domain.DoorUserMeta, error) {
	doorID = normalizeDoorID(doorID)
	var row domain.DoorUserMeta
	var lastPlayed sql.NullTime
	err := r.db.QueryRowContext(context.Background(), `
SELECT user_id, door_id, favorite, last_played_at, play_count, updated_at
FROM door_user_meta
WHERE user_id = $1 AND door_id = $2`,
		userID, doorID,
	).Scan(&row.UserID, &row.DoorID, &row.Favorite, &lastPlayed, &row.PlayCount, &row.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if lastPlayed.Valid {
		row.LastPlayedAt = &lastPlayed.Time
	}
	return &row, nil
}

func (r *PostgresDoorRepository) ListFavorites(userID int64, limit int) ([]domain.DoorUserMeta, error) {
	if limit <= 0 {
		limit = 20
	}
	return r.listMeta(`
SELECT user_id, door_id, favorite, last_played_at, play_count, updated_at
FROM door_user_meta
WHERE user_id = $1 AND favorite = TRUE
ORDER BY door_id
LIMIT $2`, userID, limit)
}

func (r *PostgresDoorRepository) ListRecent(userID int64, limit int) ([]domain.DoorUserMeta, error) {
	if limit <= 0 {
		limit = 20
	}
	return r.listMeta(`
SELECT user_id, door_id, favorite, last_played_at, play_count, updated_at
FROM door_user_meta
WHERE user_id = $1 AND last_played_at IS NOT NULL
ORDER BY last_played_at DESC
LIMIT $2`, userID, limit)
}

func (r *PostgresDoorRepository) listMeta(query string, userID int64, limit int) ([]domain.DoorUserMeta, error) {
	rows, err := r.db.QueryContext(context.Background(), query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.DoorUserMeta, 0, limit)
	for rows.Next() {
		var row domain.DoorUserMeta
		var lastPlayed sql.NullTime
		if err := rows.Scan(&row.UserID, &row.DoorID, &row.Favorite, &lastPlayed, &row.PlayCount, &row.UpdatedAt); err != nil {
			return nil, err
		}
		if lastPlayed.Valid {
			row.LastPlayedAt = &lastPlayed.Time
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresDoorRepository) GetUsageStats(doorID string) (*domain.DoorUsageStats, error) {
	doorID = normalizeDoorID(doorID)
	var row domain.DoorUsageStats
	row.DoorID = doorID
	err := r.db.QueryRowContext(context.Background(), `
SELECT
  COUNT(DISTINCT CASE WHEN last_played_at >= now() - interval '1 day' THEN user_id END) AS dau,
  COUNT(DISTINCT CASE WHEN last_played_at >= now() - interval '30 day' THEN user_id END) AS mau,
  COALESCE(SUM(play_count), 0) AS total_plays
FROM door_user_meta
WHERE ($1 = '' OR door_id = $1)`, doorID).Scan(&row.DailyActive, &row.MonthlyActive, &row.TotalPlays)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

var _ DoorRepository = (*InMemoryDoorRepository)(nil)
var _ DoorRepository = (*PostgresDoorRepository)(nil)
