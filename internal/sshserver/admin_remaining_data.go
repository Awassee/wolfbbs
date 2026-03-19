package sshserver

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"wolfbbs/internal/domain"
)

const (
	adminSettingScheduledBulletins  = "community.bulletins.schedule"
	adminSettingStaffEscalations    = "staff.escalations"
	adminSettingModInboxAssignments = "mail.shared_assignments"
	adminSettingFileReviewQueue     = "web.file_review.queue"
	adminSettingSharedRuntimeErrors = "runtime.errors.shared"
	adminSettingClubhouseGoals      = "community.clubhouse.goals"
	adminMaxSharedRuntimeErrors     = 200
	adminMaxSharedEscalations       = 200
)

const (
	adminFileReviewHold     = "hold"
	adminFileReviewApproved = "approved"
	adminFileReviewRejected = "rejected"
	adminMaxSidecarBytes    = 8192
)

type adminCommunityEvent struct {
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

type adminScheduledBulletin struct {
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

type adminSeasonChallenge struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Theme       string    `json:"theme,omitempty"`
	Description string    `json:"description,omitempty"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	BoardWeight int       `json:"board_weight"`
	ChatWeight  int       `json:"chat_weight"`
	DoorWeight  int       `json:"door_weight"`
	Active      bool      `json:"active"`
	UpdatedBy   string    `json:"updated_by,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type adminClubhouseGoal struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	BoardID     int64     `json:"board_id,omitempty"`
	DoorID      string    `json:"door_id,omitempty"`
	Target      int       `json:"target"`
	Progress    int       `json:"progress"`
	Description string    `json:"description,omitempty"`
	UpdatedBy   string    `json:"updated_by,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type adminModeratorInboxAssignment struct {
	MailID     int64     `json:"mail_id"`
	Assignee   string    `json:"assignee,omitempty"`
	Status     string    `json:"status"`
	Note       string    `json:"note,omitempty"`
	UpdatedBy  string    `json:"updated_by,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
}

type adminFileReviewItem struct {
	FileID     int64     `json:"file_id"`
	AreaID     int64     `json:"area_id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Notes      string    `json:"notes,omitempty"`
	Uploader   string    `json:"uploader,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ReviewedBy string    `json:"reviewed_by,omitempty"`
	ReviewedAt time.Time `json:"reviewed_at,omitempty"`
}

type adminSharedRuntimeError struct {
	Time    time.Time `json:"time"`
	Area    string    `json:"area"`
	Message string    `json:"message"`
}

type adminStaffEscalation struct {
	ID         string    `json:"id"`
	Handle     string    `json:"handle"`
	Actor      string    `json:"actor"`
	Note       string    `json:"note"`
	CreatedAt  time.Time `json:"created_at"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
}

func adminRandomID() string {
	token, err := randomTokenHex(6)
	if err != nil {
		return strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	}
	return token
}

func cleanOneLiner(value string, limit int) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.TrimSpace(value)
	if limit > 0 && len(value) > limit {
		return strings.TrimSpace(value[:limit])
	}
	return value
}

func parseAdminLocalDateTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("time is required")
	}
	location := time.Now().Location()
	layouts := []string{"2006-01-02 15:04", time.RFC3339, "2006-01-02T15:04"}
	for _, layout := range layouts {
		if ts, err := time.ParseInLocation(layout, raw, location); err == nil {
			return ts.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("expected YYYY-MM-DD HH:MM")
}

func formatAdminLocalDateTime(ts time.Time) string {
	if ts.IsZero() {
		return "-"
	}
	return ts.Local().Format("2006-01-02 15:04")
}

func parseAdminOptionalDateTime(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	return parseAdminLocalDateTime(raw)
}

func adminCSVToInt64(raw string, limit int) []int64 {
	parts := strings.Split(raw, ",")
	out := make([]int64, 0, len(parts))
	seen := map[int64]struct{}{}
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func adminBoolish(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on", "enabled", "active":
		return true
	default:
		return false
	}
}

func adminModeratorAssignmentStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "assigned", "resolved":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "open"
	}
}

func normalizeAdminFileReviewStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case adminFileReviewApproved:
		return adminFileReviewApproved
	case adminFileReviewRejected:
		return adminFileReviewRejected
	default:
		return adminFileReviewHold
	}
}

func normalizeAdminEventCategory(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "general"
	}
	return value
}

func normalizeAdminEventRecurrence(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "daily", "weekly", "monthly":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeAdminScheduledBulletin(row adminScheduledBulletin) (adminScheduledBulletin, bool) {
	row.ID = strings.TrimSpace(row.ID)
	row.Title = strings.TrimSpace(row.Title)
	row.Body = strings.TrimSpace(row.Body)
	row.Link = strings.TrimSpace(row.Link)
	row.Audience = strings.TrimSpace(row.Audience)
	row.CreatedBy = strings.TrimSpace(row.CreatedBy)
	if row.ID == "" {
		row.ID = adminRandomID()
	}
	if row.Title == "" || row.Body == "" || row.StartsAt.IsZero() {
		return adminScheduledBulletin{}, false
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

func (s *Server) loadAdminScheduledBulletins() []adminScheduledBulletin {
	if s == nil || s.admin == nil {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(adminSettingScheduledBulletins)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []adminScheduledBulletin
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	out := make([]adminScheduledBulletin, 0, len(rows))
	for _, row := range rows {
		if normalized, ok := normalizeAdminScheduledBulletin(row); ok {
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

func (s *Server) persistAdminScheduledBulletins(rows []adminScheduledBulletin) error {
	if s == nil || s.admin == nil {
		return fmt.Errorf("admin repository unavailable")
	}
	clean := make([]adminScheduledBulletin, 0, len(rows))
	for _, row := range rows {
		if normalized, ok := normalizeAdminScheduledBulletin(row); ok {
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
			return err
		}
		body = string(raw)
	}
	return s.admin.UpsertSystemSetting(adminSettingScheduledBulletins, body)
}

func findAdminScheduledBulletin(rows []adminScheduledBulletin, id string) (adminScheduledBulletin, int, bool) {
	id = strings.TrimSpace(id)
	for idx, row := range rows {
		if row.ID == id {
			return row, idx, true
		}
	}
	return adminScheduledBulletin{}, -1, false
}

func normalizeAdminCommunityEvent(row adminCommunityEvent) (adminCommunityEvent, bool) {
	row.ID = strings.TrimSpace(row.ID)
	row.SeriesID = strings.TrimSpace(row.SeriesID)
	row.Title = strings.TrimSpace(row.Title)
	row.Category = normalizeAdminEventCategory(row.Category)
	row.Recurrence = normalizeAdminEventRecurrence(row.Recurrence)
	row.Location = strings.TrimSpace(row.Location)
	row.Host = strings.TrimSpace(row.Host)
	row.Audience = strings.TrimSpace(row.Audience)
	row.Description = strings.TrimSpace(row.Description)
	row.Link = strings.TrimSpace(row.Link)
	if row.ID == "" {
		row.ID = adminRandomID()
	}
	if row.SeriesID == "" {
		row.SeriesID = row.ID
	}
	if row.Title == "" || row.StartsAt.IsZero() {
		return adminCommunityEvent{}, false
	}
	row.StartsAt = row.StartsAt.UTC()
	if !row.EndsAt.IsZero() {
		row.EndsAt = row.EndsAt.UTC()
		if row.EndsAt.Before(row.StartsAt) {
			row.EndsAt = time.Time{}
		}
	}
	if !row.RepeatUntil.IsZero() {
		row.RepeatUntil = row.RepeatUntil.UTC()
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = row.StartsAt
	}
	if row.Audience == "" {
		row.Audience = "all callers"
	}
	return row, true
}

func (s *Server) loadAdminCommunityEvents() []adminCommunityEvent {
	if s == nil || s.admin == nil {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(pulseSettingCommunityEvents)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []adminCommunityEvent
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	out := make([]adminCommunityEvent, 0, len(rows))
	for _, row := range rows {
		if normalized, ok := normalizeAdminCommunityEvent(row); ok {
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

func (s *Server) persistAdminCommunityEvents(rows []adminCommunityEvent) error {
	if s == nil || s.admin == nil {
		return fmt.Errorf("admin repository unavailable")
	}
	clean := make([]adminCommunityEvent, 0, len(rows))
	for _, row := range rows {
		if normalized, ok := normalizeAdminCommunityEvent(row); ok {
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
			return err
		}
		body = string(raw)
	}
	return s.admin.UpsertSystemSetting(pulseSettingCommunityEvents, body)
}

func findAdminCommunityEvent(rows []adminCommunityEvent, id string) (adminCommunityEvent, int, bool) {
	id = strings.TrimSpace(id)
	for idx, row := range rows {
		if row.ID == id {
			return row, idx, true
		}
	}
	return adminCommunityEvent{}, -1, false
}

func normalizeAdminSeasonChallenge(row adminSeasonChallenge) (adminSeasonChallenge, bool) {
	row.ID = strings.TrimSpace(row.ID)
	row.Name = strings.TrimSpace(row.Name)
	row.Theme = strings.TrimSpace(row.Theme)
	row.Description = strings.TrimSpace(row.Description)
	row.UpdatedBy = strings.ToLower(strings.TrimSpace(row.UpdatedBy))
	if row.Name == "" {
		return adminSeasonChallenge{}, false
	}
	if row.ID == "" {
		row.ID = adminRandomID()
	}
	if row.StartsAt.IsZero() {
		row.StartsAt = time.Now().UTC().Add(-24 * time.Hour)
	}
	if row.EndsAt.IsZero() || !row.EndsAt.After(row.StartsAt) {
		row.EndsAt = row.StartsAt.Add(30 * 24 * time.Hour)
	}
	row.BoardWeight = maxInt(1, minInt(20, row.BoardWeight))
	row.ChatWeight = maxInt(1, minInt(20, row.ChatWeight))
	row.DoorWeight = maxInt(1, minInt(20, row.DoorWeight))
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	return row, true
}

func (s *Server) loadAdminSeasonChallenges() []adminSeasonChallenge {
	if s == nil || s.admin == nil {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(pulseSettingSeasonChallenges)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []adminSeasonChallenge
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	out := make([]adminSeasonChallenge, 0, len(rows))
	for _, row := range rows {
		if normalized, ok := normalizeAdminSeasonChallenge(row); ok {
			out = append(out, normalized)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartsAt.Equal(out[j].StartsAt) {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		return out[i].StartsAt.After(out[j].StartsAt)
	})
	return out
}

func (s *Server) persistAdminSeasonChallenges(rows []adminSeasonChallenge) error {
	if s == nil || s.admin == nil {
		return fmt.Errorf("admin repository unavailable")
	}
	clean := make([]adminSeasonChallenge, 0, len(rows))
	for _, row := range rows {
		if normalized, ok := normalizeAdminSeasonChallenge(row); ok {
			clean = append(clean, normalized)
		}
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i].StartsAt.Equal(clean[j].StartsAt) {
			return strings.ToLower(clean[i].Name) < strings.ToLower(clean[j].Name)
		}
		return clean[i].StartsAt.After(clean[j].StartsAt)
	})
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			return err
		}
		body = string(raw)
	}
	return s.admin.UpsertSystemSetting(pulseSettingSeasonChallenges, body)
}

func findAdminSeasonChallenge(rows []adminSeasonChallenge, id string) (adminSeasonChallenge, int, bool) {
	id = strings.TrimSpace(id)
	for idx, row := range rows {
		if row.ID == id {
			return row, idx, true
		}
	}
	return adminSeasonChallenge{}, -1, false
}

func normalizeAdminClubhouseGoal(row adminClubhouseGoal) (adminClubhouseGoal, bool) {
	row.ID = strings.TrimSpace(row.ID)
	row.Title = cleanOneLiner(row.Title, 120)
	row.Description = cleanOneLiner(row.Description, 220)
	row.DoorID = strings.ToLower(strings.TrimSpace(row.DoorID))
	row.UpdatedBy = strings.ToLower(strings.TrimSpace(row.UpdatedBy))
	if row.Title == "" {
		return adminClubhouseGoal{}, false
	}
	if row.ID == "" {
		row.ID = adminRandomID()
	}
	row.Target = maxInt(1, minInt(200000, row.Target))
	if row.Progress < 0 {
		row.Progress = 0
	}
	if row.Progress > row.Target {
		row.Progress = row.Target
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	return row, true
}

func (s *Server) loadAdminClubhouseGoals() []adminClubhouseGoal {
	if s == nil || s.admin == nil {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(adminSettingClubhouseGoals)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []adminClubhouseGoal
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	out := make([]adminClubhouseGoal, 0, len(rows))
	for _, row := range rows {
		if normalized, ok := normalizeAdminClubhouseGoal(row); ok {
			out = append(out, normalized)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *Server) persistAdminClubhouseGoals(rows []adminClubhouseGoal) error {
	if s == nil || s.admin == nil {
		return fmt.Errorf("admin repository unavailable")
	}
	clean := make([]adminClubhouseGoal, 0, len(rows))
	for _, row := range rows {
		if normalized, ok := normalizeAdminClubhouseGoal(row); ok {
			clean = append(clean, normalized)
		}
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i].UpdatedAt.Equal(clean[j].UpdatedAt) {
			return strings.ToLower(clean[i].Title) < strings.ToLower(clean[j].Title)
		}
		return clean[i].UpdatedAt.After(clean[j].UpdatedAt)
	})
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			return err
		}
		body = string(raw)
	}
	return s.admin.UpsertSystemSetting(adminSettingClubhouseGoals, body)
}

func findAdminClubhouseGoal(rows []adminClubhouseGoal, id string) (adminClubhouseGoal, int, bool) {
	id = strings.TrimSpace(id)
	for idx, row := range rows {
		if row.ID == id {
			return row, idx, true
		}
	}
	return adminClubhouseGoal{}, -1, false
}

func (s *Server) loadModeratorInboxAssignmentsSSH() map[int64]adminModeratorInboxAssignment {
	out := map[int64]adminModeratorInboxAssignment{}
	if s == nil || s.admin == nil {
		return out
	}
	raw, err := s.admin.GetSystemSetting(adminSettingModInboxAssignments)
	if err != nil || strings.TrimSpace(raw) == "" {
		return out
	}
	decoded := map[string]adminModeratorInboxAssignment{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return out
	}
	for rawID, row := range decoded {
		id, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		row.MailID = id
		row.Assignee = cleanOneLiner(row.Assignee, 48)
		row.Note = cleanOneLiner(row.Note, 160)
		row.UpdatedBy = cleanOneLiner(row.UpdatedBy, 48)
		row.Status = adminModeratorAssignmentStatus(row.Status)
		out[id] = row
	}
	return out
}

func (s *Server) persistModeratorInboxAssignmentsSSH(rows map[int64]adminModeratorInboxAssignment) error {
	if s == nil || s.admin == nil {
		return fmt.Errorf("admin repository unavailable")
	}
	encoded := map[string]adminModeratorInboxAssignment{}
	for id, row := range rows {
		if id <= 0 {
			continue
		}
		row.MailID = id
		row.Assignee = cleanOneLiner(row.Assignee, 48)
		row.Note = cleanOneLiner(row.Note, 160)
		row.UpdatedBy = cleanOneLiner(row.UpdatedBy, 48)
		row.Status = adminModeratorAssignmentStatus(row.Status)
		encoded[strconv.FormatInt(id, 10)] = row
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			return err
		}
		body = string(raw)
	}
	return s.admin.UpsertSystemSetting(adminSettingModInboxAssignments, body)
}

func (s *Server) updateModeratorInboxAssignmentSSH(mailID int64, row adminModeratorInboxAssignment) error {
	if mailID <= 0 {
		return fmt.Errorf("mail id is required")
	}
	rows := s.loadModeratorInboxAssignmentsSSH()
	row.MailID = mailID
	row.Status = adminModeratorAssignmentStatus(row.Status)
	rows[mailID] = row
	return s.persistModeratorInboxAssignmentsSSH(rows)
}

func (s *Server) loadFileReviewQueueSSH() map[int64]adminFileReviewItem {
	out := map[int64]adminFileReviewItem{}
	if s == nil || s.admin == nil {
		return out
	}
	raw, err := s.admin.GetSystemSetting(adminSettingFileReviewQueue)
	if err != nil || strings.TrimSpace(raw) == "" {
		return out
	}
	decoded := map[string]adminFileReviewItem{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return out
	}
	for rawID, row := range decoded {
		fileID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || fileID <= 0 {
			continue
		}
		row.FileID = fileID
		row.Status = normalizeAdminFileReviewStatus(row.Status)
		out[fileID] = row
	}
	return out
}

func (s *Server) persistFileReviewQueueSSH(rows map[int64]adminFileReviewItem) error {
	if s == nil || s.admin == nil {
		return fmt.Errorf("admin repository unavailable")
	}
	encoded := map[string]adminFileReviewItem{}
	for fileID, row := range rows {
		if fileID <= 0 {
			continue
		}
		row.FileID = fileID
		row.Status = normalizeAdminFileReviewStatus(row.Status)
		encoded[strconv.FormatInt(fileID, 10)] = row
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			return err
		}
		body = string(raw)
	}
	return s.admin.UpsertSystemSetting(adminSettingFileReviewQueue, body)
}

func (s *Server) setFileReviewItemSSH(item adminFileReviewItem) error {
	if item.FileID <= 0 {
		return fmt.Errorf("file id is required")
	}
	rows := s.loadFileReviewQueueSSH()
	item.Status = normalizeAdminFileReviewStatus(item.Status)
	rows[item.FileID] = item
	return s.persistFileReviewQueueSSH(rows)
}

func (s *Server) removeFileReviewItemSSH(fileID int64) error {
	if fileID <= 0 {
		return nil
	}
	rows := s.loadFileReviewQueueSSH()
	delete(rows, fileID)
	return s.persistFileReviewQueueSSH(rows)
}

func normalizeAdminSharedRuntimeError(row adminSharedRuntimeError) (adminSharedRuntimeError, bool) {
	row.Time = row.Time.UTC()
	row.Area = cleanOneLiner(defaultIfBlank(row.Area, "runtime"), 64)
	row.Message = cleanOneLiner(row.Message, 240)
	if row.Time.IsZero() || row.Message == "" {
		return adminSharedRuntimeError{}, false
	}
	return row, true
}

func (s *Server) loadSharedRuntimeErrorsSSH() []adminSharedRuntimeError {
	if s == nil || s.admin == nil {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(adminSettingSharedRuntimeErrors)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []adminSharedRuntimeError
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	out := make([]adminSharedRuntimeError, 0, len(rows))
	for _, row := range rows {
		if normalized, ok := normalizeAdminSharedRuntimeError(row); ok {
			out = append(out, normalized)
		}
	}
	if len(out) > adminMaxSharedRuntimeErrors {
		out = out[len(out)-adminMaxSharedRuntimeErrors:]
	}
	return out
}

func (s *Server) persistSharedRuntimeErrorsSSH(rows []adminSharedRuntimeError) error {
	if s == nil || s.admin == nil {
		return fmt.Errorf("admin repository unavailable")
	}
	clean := make([]adminSharedRuntimeError, 0, len(rows))
	for _, row := range rows {
		if normalized, ok := normalizeAdminSharedRuntimeError(row); ok {
			clean = append(clean, normalized)
		}
	}
	if len(clean) > adminMaxSharedRuntimeErrors {
		clean = clean[len(clean)-adminMaxSharedRuntimeErrors:]
	}
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			return err
		}
		body = string(raw)
	}
	return s.admin.UpsertSystemSetting(adminSettingSharedRuntimeErrors, body)
}

func (s *Server) clearSharedRuntimeErrorsSSH() (int, error) {
	rows := s.loadSharedRuntimeErrorsSSH()
	if err := s.persistSharedRuntimeErrorsSSH(nil); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func normalizeAdminStaffEscalation(row adminStaffEscalation) (adminStaffEscalation, bool) {
	row.ID = strings.TrimSpace(row.ID)
	row.Handle = strings.TrimSpace(row.Handle)
	row.Actor = strings.TrimSpace(row.Actor)
	row.Note = cleanOneLiner(row.Note, 160)
	if row.ID == "" || row.Handle == "" || row.Note == "" || row.CreatedAt.IsZero() {
		return adminStaffEscalation{}, false
	}
	return row, true
}

func (s *Server) loadStaffEscalationsSSH() []adminStaffEscalation {
	if s == nil || s.admin == nil {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(adminSettingStaffEscalations)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []adminStaffEscalation
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	out := make([]adminStaffEscalation, 0, len(rows))
	for _, row := range rows {
		if normalized, ok := normalizeAdminStaffEscalation(row); ok {
			out = append(out, normalized)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ResolvedAt.IsZero() != out[j].ResolvedAt.IsZero() {
			return out[i].ResolvedAt.IsZero()
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > adminMaxSharedEscalations {
		out = out[:adminMaxSharedEscalations]
	}
	return out
}

func (s *Server) persistStaffEscalationsSSH(rows []adminStaffEscalation) error {
	if s == nil || s.admin == nil {
		return fmt.Errorf("admin repository unavailable")
	}
	clean := make([]adminStaffEscalation, 0, len(rows))
	for _, row := range rows {
		if normalized, ok := normalizeAdminStaffEscalation(row); ok {
			clean = append(clean, normalized)
		}
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i].ResolvedAt.IsZero() != clean[j].ResolvedAt.IsZero() {
			return clean[i].ResolvedAt.IsZero()
		}
		return clean[i].CreatedAt.After(clean[j].CreatedAt)
	})
	if len(clean) > adminMaxSharedEscalations {
		clean = clean[:adminMaxSharedEscalations]
	}
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			return err
		}
		body = string(raw)
	}
	return s.admin.UpsertSystemSetting(adminSettingStaffEscalations, body)
}

func (s *Server) resolveStaffEscalationSSH(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	rows := s.loadStaffEscalationsSSH()
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
		_ = s.persistStaffEscalationsSSH(rows)
	}
	return updated
}

func (s *Server) resolvePageRequestSSH(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	rows := s.loadPageRequests()
	next := make([]persistedPageRequest, 0, len(rows))
	removed := false
	for _, row := range rows {
		if row.ID == id {
			removed = true
			continue
		}
		next = append(next, row)
	}
	if removed {
		_ = s.persistPageRequests(next)
	}
	return removed
}

func (s *Server) persistFeaturedCollectionsSSH(rows []sshFeaturedFileCollection) error {
	if s == nil || s.admin == nil {
		return fmt.Errorf("admin repository unavailable")
	}
	clean := normalizeSSHFeaturedCollections(rows)
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			return err
		}
		body = string(raw)
	}
	return s.admin.UpsertSystemSetting(sshSettingFeaturedCollections, body)
}

func (s *Server) upsertFeaturedCollectionSSH(row sshFeaturedFileCollection) error {
	rows := s.loadFeaturedCollectionsSSH()
	row.ID = strings.TrimSpace(row.ID)
	if row.ID == "" {
		row.ID = adminRandomID()
	}
	row.Title = strings.TrimSpace(row.Title)
	row.Description = strings.TrimSpace(row.Description)
	row.Curator = strings.ToLower(strings.TrimSpace(row.Curator))
	row.FileIDs = normalizeCollectionFileIDs(row.FileIDs)
	row.Tags = normalizeCollectionTags(row.Tags)
	row.UpdatedAt = time.Now().UTC()
	updated := false
	for idx := range rows {
		if rows[idx].ID == row.ID {
			rows[idx] = row
			updated = true
			break
		}
	}
	if !updated {
		rows = append(rows, row)
	}
	return s.persistFeaturedCollectionsSSH(rows)
}

func (s *Server) deleteFeaturedCollectionSSH(id string) error {
	id = strings.TrimSpace(id)
	rows := s.loadFeaturedCollectionsSSH()
	next := make([]sshFeaturedFileCollection, 0, len(rows))
	for _, row := range rows {
		if row.ID == id {
			continue
		}
		next = append(next, row)
	}
	return s.persistFeaturedCollectionsSSH(next)
}

func adminFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func adminReadSidecarText(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(io.LimitReader(f, adminMaxSidecarBytes))
	lines := make([]string, 0, 3)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) >= 3 {
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines, " "))
}

func adminDeriveTags(filename, description string) []string {
	out := map[string]struct{}{}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	if ext != "" {
		out[ext] = struct{}{}
	}
	stem := strings.ToLower(strings.TrimSuffix(filename, filepath.Ext(filename)))
	for _, part := range strings.FieldsFunc(stem, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		part = strings.TrimSpace(part)
		if len(part) >= 3 {
			out[part] = struct{}{}
		}
	}
	for _, part := range strings.Fields(strings.ToLower(description)) {
		part = strings.TrimFunc(part, func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
		})
		if len(part) >= 4 {
			out[part] = struct{}{}
		}
	}
	tags := make([]string, 0, len(out))
	for tag := range out {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	if len(tags) > 8 {
		tags = tags[:8]
	}
	return tags
}

func adminFileMetadata(root, filename string) (string, []string) {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	for _, candidate := range []string{
		filepath.Join(root, base+".diz"),
		filepath.Join(root, base+".DIZ"),
		filepath.Join(root, base+".nfo"),
		filepath.Join(root, base+".NFO"),
	} {
		if text := adminReadSidecarText(candidate); text != "" {
			return text, adminDeriveTags(filename, text)
		}
	}
	return "", adminDeriveTags(filename, "")
}

func adminMergeFileTags(existing []string, raw string) []string {
	out := map[string]struct{}{}
	for _, tag := range existing {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag != "" {
			out[tag] = struct{}{}
		}
	}
	for _, tag := range strings.Split(raw, ",") {
		tag = strings.ToLower(strings.TrimSpace(tag))
		tag = strings.TrimFunc(tag, func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' && r != '_'
		})
		if len(tag) >= 2 {
			out[tag] = struct{}{}
		}
	}
	tags := make([]string, 0, len(out))
	for tag := range out {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	if len(tags) > 10 {
		tags = tags[:10]
	}
	return tags
}

func (s *Server) indexAreaFilesSSH(area domain.FileArea, uploaderID int64) (int, int, error) {
	if s == nil || s.admin == nil {
		return 0, 0, errors.New("admin repository unavailable")
	}
	root := filepath.Clean(strings.TrimSpace(area.Path))
	if root == "" || root == "." {
		return 0, 0, errors.New("invalid area path")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, 0, err
	}
	var indexed, failed int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".diz") || strings.HasSuffix(lower, ".nfo") {
			continue
		}
		fullPath := filepath.Join(root, name)
		info, err := entry.Info()
		if err != nil {
			failed++
			continue
		}
		sum, err := adminFileSHA256(fullPath)
		if err != nil {
			failed++
			continue
		}
		desc, tags := adminFileMetadata(root, name)
		row := &domain.FileEntry{
			AreaID:      area.ID,
			Name:        name,
			Path:        fullPath,
			Description: desc,
			Tags:        tags,
			SHA256:      sum,
			SizeBytes:   info.Size(),
			UploaderID:  uploaderID,
			UploadedAt:  info.ModTime().UTC(),
		}
		if err := s.admin.UpsertFileEntry(row); err != nil {
			failed++
			continue
		}
		indexed++
	}
	return indexed, failed, nil
}

func (s *Server) deleteIndexedFileSSH(entry *domain.FileEntry) error {
	if s == nil || s.admin == nil {
		return errors.New("admin repository unavailable")
	}
	if entry == nil {
		return errors.New("file entry is required")
	}
	if path := strings.TrimSpace(entry.Path); path != "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return s.admin.DeleteFileEntry(entry.ID)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func parseInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
}
