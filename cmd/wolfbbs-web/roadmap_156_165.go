package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/rbac"
)

const (
	sysSettingMentorshipPairs        = "community.mentorship.pairs"
	sysSettingMilestoneCelebrations  = "web.milestones.celebrations."
	sysSettingReportAssignments      = "moderation.report.assignments"
	sysSettingModeratorTemplates     = "moderation.canned.templates"
	sysSettingModeratorCaseThreads   = "moderation.case_threads"
	maxMentorshipPairs               = 256
	maxModeratorTemplates            = 128
	maxModeratorCaseThreads          = 256
	maxModeratorCaseUpdatesPerThread = 60
)

type resumeItem struct {
	Key       string
	Title     string
	Detail    string
	Href      string
	Priority  int
	HomeRoute string
}

type mentorshipPair struct {
	Mentee    string    `json:"mentee"`
	Mentor    string    `json:"mentor"`
	Note      string    `json:"note,omitempty"`
	Active    bool      `json:"active"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type milestoneStatus struct {
	ID        string
	Title     string
	Detail    string
	Current   int
	Target    int
	Reached   bool
	Celebrate bool
}

type reportAssignment struct {
	ReportID   int64     `json:"report_id"`
	Assignee   string    `json:"assignee,omitempty"`
	Status     string    `json:"status"`
	Priority   string    `json:"priority"`
	Note       string    `json:"note,omitempty"`
	DueAt      time.Time `json:"due_at"`
	UpdatedBy  string    `json:"updated_by,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
}

type moderationTemplate struct {
	ID         string    `json:"id"`
	Category   string    `json:"category"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	UsageCount int       `json:"usage_count"`
	UpdatedBy  string    `json:"updated_by,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

type caseUpdate struct {
	At    time.Time `json:"at"`
	Actor string    `json:"actor"`
	Note  string    `json:"note"`
}

type caseThread struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Status       string       `json:"status"`
	Priority     string       `json:"priority"`
	Owner        string       `json:"owner,omitempty"`
	TargetHandle string       `json:"target_handle,omitempty"`
	ReportID     int64        `json:"report_id,omitempty"`
	Summary      string       `json:"summary,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	Updates      []caseUpdate `json:"updates,omitempty"`
}

type reportQueueRow struct {
	Report        domain.MessageReport
	Reporter      string
	MessageLink   string
	BoardName     string
	Subject       string
	Assignment    reportAssignment
	SLAState      string
	SLADetail     string
	DefaultSLAHrs int
}

func normalizeMentorshipPair(row mentorshipPair) (mentorshipPair, bool) {
	row.Mentee = normalizeHandleKey(row.Mentee)
	row.Mentor = normalizeHandleKey(row.Mentor)
	row.Note = cleanOneLiner(row.Note, 220)
	row.UpdatedBy = normalizeHandleKey(row.UpdatedBy)
	if row.Mentee == "" || row.Mentor == "" || row.Mentee == row.Mentor {
		return mentorshipPair{}, false
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	return row, true
}

func (a *webApp) loadMentorshipPairs() []mentorshipPair {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingMentorshipPairs)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	rows := []mentorshipPair{}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("mentorship", fmt.Errorf("decode mentorship pairs: %w", err))
		return nil
	}
	out := make([]mentorshipPair, 0, len(rows))
	for _, row := range rows {
		normalized, ok := normalizeMentorshipPair(row)
		if ok {
			out = append(out, normalized)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mentee == out[j].Mentee {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].Mentee < out[j].Mentee
	})
	if len(out) > maxMentorshipPairs {
		out = out[:maxMentorshipPairs]
	}
	return out
}

func (a *webApp) persistMentorshipPairs(rows []mentorshipPair) {
	if a.adminRepo == nil {
		return
	}
	out := make([]mentorshipPair, 0, len(rows))
	for _, row := range rows {
		normalized, ok := normalizeMentorshipPair(row)
		if ok {
			out = append(out, normalized)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mentee == out[j].Mentee {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].Mentee < out[j].Mentee
	})
	if len(out) > maxMentorshipPairs {
		out = out[:maxMentorshipPairs]
	}
	body := ""
	if len(out) > 0 {
		raw, err := json.Marshal(out)
		if err != nil {
			a.addAppError("mentorship", fmt.Errorf("encode mentorship pairs: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingMentorshipPairs, body)
}

func (a *webApp) mentorshipForMentee(handle string) (mentorshipPair, bool) {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return mentorshipPair{}, false
	}
	for _, row := range a.loadMentorshipPairs() {
		if row.Active && row.Mentee == handle {
			return row, true
		}
	}
	return mentorshipPair{}, false
}

func listStaffHandles(users []domain.User) []string {
	out := make([]string, 0, len(users))
	for _, row := range users {
		role := rbac.NormalizeRole(row.Role)
		if role != roleModerator && role != roleAdmin {
			continue
		}
		out = append(out, row.Handle)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func (a *webApp) buildResumeItems(user *domain.User) []resumeItem {
	if user == nil {
		return nil
	}
	items := make([]resumeItem, 0, 12)
	if unread := a.unreadMailCountForUser(user); unread > 0 {
		items = append(items, resumeItem{
			Key:       "mail",
			Title:     "Clear unread mail",
			Detail:    strconv.Itoa(unread) + " unread messages are waiting.",
			Href:      "/mail?box=unread",
			Priority:  100,
			HomeRoute: "/today",
		})
	}
	snapshot := a.buildFirstCallSnapshot(user)
	for _, task := range snapshot.Tasks {
		if task.Done || strings.TrimSpace(task.Href) == "" {
			continue
		}
		homeRoute := "/today"
		switch task.Href {
		case "/settings":
			homeRoute = "/today"
		case "/first-call":
			homeRoute = "/boards"
		}
		items = append(items, resumeItem{
			Key:       "firstcall_" + task.Key,
			Title:     task.Title,
			Detail:    task.Detail,
			Href:      task.Href,
			Priority:  90,
			HomeRoute: homeRoute,
		})
	}
	activity := a.myCallerActivity(user.Handle)
	if activity.CurrentStreak == 0 {
		items = append(items, resumeItem{
			Key:       "streak",
			Title:     "Recover your daily streak",
			Detail:    "One meaningful action today restarts momentum.",
			Href:      "/streaks",
			Priority:  80,
			HomeRoute: "/today",
		})
	}
	if activity.Door7 == 0 {
		items = append(items, resumeItem{
			Key:       "doors",
			Title:     "Run one comeback door",
			Detail:    "No door activity in the last 7 days.",
			Href:      "/doors/comeback",
			Priority:  70,
			HomeRoute: "/doors",
		})
	}
	upcoming := a.upcomingCommunityEvents(4, time.Now().UTC())
	if len(upcoming) > 0 {
		items = append(items, resumeItem{
			Key:       "events",
			Title:     "Check upcoming events",
			Detail:    strconv.Itoa(len(upcoming)) + " event(s) are in the near queue.",
			Href:      "/events",
			Priority:  60,
			HomeRoute: "/today",
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority == items[j].Priority {
			return items[i].Title < items[j].Title
		}
		return items[i].Priority > items[j].Priority
	})
	seen := map[string]struct{}{}
	out := make([]resumeItem, 0, len(items))
	for _, row := range items {
		key := row.Key + "|" + row.Href
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, row)
	}
	if len(out) == 0 {
		out = append(out, resumeItem{
			Key:       "steady",
			Title:     "You're caught up",
			Detail:    "Open Today Brief and keep the loop moving.",
			Href:      "/today",
			Priority:  1,
			HomeRoute: "/today",
		})
	}
	return out
}

func milestoneCelebrationSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingMilestoneCelebrations + handle
}

func (a *webApp) loadMilestoneCelebrations(handle string) map[string]time.Time {
	if a.adminRepo == nil {
		return map[string]time.Time{}
	}
	key := milestoneCelebrationSettingKey(handle)
	if key == "" {
		return map[string]time.Time{}
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return map[string]time.Time{}
	}
	decoded := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("milestones", fmt.Errorf("decode milestone celebrations: %w", err))
		return map[string]time.Time{}
	}
	out := map[string]time.Time{}
	for id, value := range decoded {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			continue
		}
		out[id] = at
	}
	return out
}

func (a *webApp) persistMilestoneCelebrations(handle string, rows map[string]time.Time) {
	key := milestoneCelebrationSettingKey(handle)
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
			a.addAppError("milestones", fmt.Errorf("encode milestone celebrations: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(key, body)
}

func (a *webApp) milestoneCountsForUser(user *domain.User) map[string]int {
	out := map[string]int{
		"posts":        0,
		"chat":         0,
		"doors":        0,
		"achievements": 0,
		"streak":       0,
	}
	if user == nil {
		return out
	}
	if a.msgRepo != nil && a.boardRepo != nil {
		if boards, err := a.boardRepo.List(); err == nil {
			for _, board := range boards {
				rows, err := a.msgRepo.ListByBoard(board.ID)
				if err != nil {
					continue
				}
				for _, row := range rows {
					if row.AuthorID == user.ID {
						out["posts"]++
					}
				}
			}
		}
	}
	if a.chatSvc != nil {
		for _, channel := range a.chatSvc.ListChannels() {
			for _, row := range a.chatSvc.History(channel, 8000) {
				if strings.EqualFold(row.From, user.Handle) {
					out["chat"]++
				}
			}
		}
	}
	if a.doorRegistry != nil {
		for _, door := range a.doorRegistry.Doors() {
			rows, err := a.doorRegistry.ListEvents(door.ID, user.ID, 8000)
			if err != nil {
				continue
			}
			for _, row := range rows {
				switch strings.ToLower(strings.TrimSpace(row.EventType)) {
				case "start", "connector_launch", "poll_vote", "poll_create":
					out["doors"]++
				}
			}
		}
	}
	out["achievements"] = len(a.mustDoorAchievements(user.ID, 400))
	out["streak"] = a.myCallerActivity(user.Handle).CurrentStreak
	return out
}

func buildMilestones(counts map[string]int, celebrations map[string]time.Time) []milestoneStatus {
	rows := []milestoneStatus{
		{ID: "first_post", Title: "First Board Post", Detail: "Create your first public post.", Current: counts["posts"], Target: 1},
		{ID: "board_regular", Title: "Board Regular", Detail: "Post 25 board messages.", Current: counts["posts"], Target: 25},
		{ID: "chat_hello", Title: "Lobby Icebreaker", Detail: "Send your first chat line.", Current: counts["chat"], Target: 1},
		{ID: "chat_regular", Title: "Chat Regular", Detail: "Send 50 chat lines.", Current: counts["chat"], Target: 50},
		{ID: "door_first", Title: "First Door Run", Detail: "Run one door session.", Current: counts["doors"], Target: 1},
		{ID: "door_veteran", Title: "Door Veteran", Detail: "Complete 20 door sessions.", Current: counts["doors"], Target: 20},
		{ID: "streak_3", Title: "Streak x3", Detail: "Maintain a 3-day streak.", Current: counts["streak"], Target: 3},
		{ID: "streak_14", Title: "Streak x14", Detail: "Maintain a 14-day streak.", Current: counts["streak"], Target: 14},
		{ID: "achieve_5", Title: "Achievement Collector", Detail: "Earn 5 door achievements.", Current: counts["achievements"], Target: 5},
	}
	for i := range rows {
		rows[i].Reached = rows[i].Current >= rows[i].Target
		_, celebrated := celebrations[rows[i].ID]
		rows[i].Celebrate = rows[i].Reached && !celebrated
	}
	return rows
}

func moderationAssignmentStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "open", "assigned", "investigating", "resolved":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "open"
	}
}

func moderationPriority(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "normal", "high", "urgent":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "normal"
	}
}

func defaultSLAHours(reason string) int {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if strings.Contains(reason, "dox") || strings.Contains(reason, "threat") || strings.Contains(reason, "abuse") || strings.Contains(reason, "harass") {
		return 4
	}
	if strings.Contains(reason, "spam") || strings.Contains(reason, "bot") {
		return 8
	}
	return 24
}

func inferPriority(reason string) string {
	hours := defaultSLAHours(reason)
	switch {
	case hours <= 4:
		return "urgent"
	case hours <= 8:
		return "high"
	default:
		return "normal"
	}
}

func normalizeReportAssignment(row reportAssignment, fallbackReason string) reportAssignment {
	row.Assignee = normalizeHandleKey(row.Assignee)
	row.Status = moderationAssignmentStatus(row.Status)
	row.Priority = moderationPriority(row.Priority)
	row.Note = cleanOneLiner(row.Note, 220)
	row.UpdatedBy = normalizeHandleKey(row.UpdatedBy)
	if row.Priority == "normal" && strings.TrimSpace(fallbackReason) != "" {
		row.Priority = inferPriority(fallbackReason)
	}
	if row.DueAt.IsZero() {
		row.DueAt = time.Now().UTC().Add(time.Duration(defaultSLAHours(fallbackReason)) * time.Hour)
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	return row
}

func (a *webApp) loadReportAssignments() map[int64]reportAssignment {
	if a.adminRepo == nil {
		return map[int64]reportAssignment{}
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingReportAssignments)
	if err != nil || strings.TrimSpace(raw) == "" {
		return map[int64]reportAssignment{}
	}
	decoded := map[string]reportAssignment{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("moderation.assignments", fmt.Errorf("decode report assignments: %w", err))
		return map[int64]reportAssignment{}
	}
	out := map[int64]reportAssignment{}
	for key, row := range decoded {
		id, err := strconv.ParseInt(strings.TrimSpace(key), 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		row.ReportID = id
		out[id] = normalizeReportAssignment(row, "")
	}
	return out
}

func (a *webApp) persistReportAssignments(rows map[int64]reportAssignment) {
	if a.adminRepo == nil {
		return
	}
	encoded := map[string]reportAssignment{}
	for id, row := range rows {
		if id <= 0 {
			continue
		}
		row.ReportID = id
		encoded[strconv.FormatInt(id, 10)] = normalizeReportAssignment(row, "")
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("moderation.assignments", fmt.Errorf("encode report assignments: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingReportAssignments, body)
}

func reportSLAState(row reportAssignment, report domain.MessageReport, now time.Time) (string, string) {
	if strings.EqualFold(report.Status, "resolved") || strings.EqualFold(row.Status, "resolved") {
		return "resolved", "resolved"
	}
	due := row.DueAt
	if due.IsZero() {
		due = report.CreatedAt.UTC().Add(time.Duration(defaultSLAHours(report.Reason)) * time.Hour)
	}
	if now.After(due) {
		return "breach", "overdue by " + formatDurationCompact(now.Sub(due))
	}
	remaining := due.Sub(now)
	if remaining <= 2*time.Hour {
		return "warning", "due in " + formatDurationCompact(remaining)
	}
	return "on-track", "due in " + formatDurationCompact(remaining)
}

func normalizeModeratorTemplate(row moderationTemplate) (moderationTemplate, bool) {
	row.ID = strings.TrimSpace(row.ID)
	if row.ID == "" {
		row.ID = "tpl-" + randomEventID()
	}
	row.Category = cleanOneLiner(strings.ToLower(row.Category), 24)
	if row.Category == "" {
		row.Category = "general"
	}
	row.Title = cleanOneLiner(row.Title, 96)
	row.Body = strings.TrimSpace(row.Body)
	row.UpdatedBy = normalizeHandleKey(row.UpdatedBy)
	if row.Title == "" || row.Body == "" {
		return moderationTemplate{}, false
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	return row, true
}

func (a *webApp) loadModeratorTemplates() []moderationTemplate {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingModeratorTemplates)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	rows := []moderationTemplate{}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("moderation.templates", fmt.Errorf("decode moderator templates: %w", err))
		return nil
	}
	out := make([]moderationTemplate, 0, len(rows))
	for _, row := range rows {
		normalized, ok := normalizeModeratorTemplate(row)
		if ok {
			out = append(out, normalized)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Title < out[j].Title
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > maxModeratorTemplates {
		out = out[:maxModeratorTemplates]
	}
	return out
}

func (a *webApp) persistModeratorTemplates(rows []moderationTemplate) {
	if a.adminRepo == nil {
		return
	}
	out := make([]moderationTemplate, 0, len(rows))
	for _, row := range rows {
		normalized, ok := normalizeModeratorTemplate(row)
		if ok {
			out = append(out, normalized)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Title < out[j].Title
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > maxModeratorTemplates {
		out = out[:maxModeratorTemplates]
	}
	body := ""
	if len(out) > 0 {
		raw, err := json.Marshal(out)
		if err != nil {
			a.addAppError("moderation.templates", fmt.Errorf("encode moderator templates: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingModeratorTemplates, body)
}

func normalizeCaseStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "open", "in_progress", "waiting", "resolved":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "open"
	}
}

func normalizeCasePriority(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "normal", "high", "urgent":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "normal"
	}
}

func normalizeCaseThread(row caseThread) (caseThread, bool) {
	row.ID = strings.TrimSpace(row.ID)
	if row.ID == "" {
		row.ID = "case-" + randomEventID()
	}
	row.Title = cleanOneLiner(row.Title, 96)
	row.Status = normalizeCaseStatus(row.Status)
	row.Priority = normalizeCasePriority(row.Priority)
	row.Owner = normalizeHandleKey(row.Owner)
	row.TargetHandle = normalizeHandleKey(row.TargetHandle)
	row.Summary = cleanOneLiner(row.Summary, 320)
	if row.Title == "" {
		return caseThread{}, false
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	}
	row.UpdatedAt = time.Now().UTC()
	cleanUpdates := make([]caseUpdate, 0, len(row.Updates))
	for _, update := range row.Updates {
		update.Actor = normalizeHandleKey(update.Actor)
		update.Note = cleanOneLiner(update.Note, 320)
		if update.Actor == "" || update.Note == "" {
			continue
		}
		if update.At.IsZero() {
			update.At = time.Now().UTC()
		}
		cleanUpdates = append(cleanUpdates, update)
	}
	sort.Slice(cleanUpdates, func(i, j int) bool {
		return cleanUpdates[i].At.After(cleanUpdates[j].At)
	})
	if len(cleanUpdates) > maxModeratorCaseUpdatesPerThread {
		cleanUpdates = cleanUpdates[:maxModeratorCaseUpdatesPerThread]
	}
	row.Updates = cleanUpdates
	return row, true
}

func (a *webApp) loadCaseThreads() []caseThread {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingModeratorCaseThreads)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	rows := []caseThread{}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("moderation.cases", fmt.Errorf("decode case threads: %w", err))
		return nil
	}
	out := make([]caseThread, 0, len(rows))
	for _, row := range rows {
		normalized, ok := normalizeCaseThread(row)
		if ok {
			out = append(out, normalized)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Title < out[j].Title
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > maxModeratorCaseThreads {
		out = out[:maxModeratorCaseThreads]
	}
	return out
}

func (a *webApp) persistCaseThreads(rows []caseThread) {
	if a.adminRepo == nil {
		return
	}
	out := make([]caseThread, 0, len(rows))
	for _, row := range rows {
		normalized, ok := normalizeCaseThread(row)
		if ok {
			out = append(out, normalized)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Title < out[j].Title
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > maxModeratorCaseThreads {
		out = out[:maxModeratorCaseThreads]
	}
	body := ""
	if len(out) > 0 {
		raw, err := json.Marshal(out)
		if err != nil {
			a.addAppError("moderation.cases", fmt.Errorf("encode case threads: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingModeratorCaseThreads, body)
}

func (a *webApp) buildCallerRiskSummary(target *domain.User) (int, string, []string) {
	if target == nil {
		return 0, "low", []string{"no target"}
	}
	score := 0
	signals := make([]string, 0, 10)
	if target.Banned {
		score += 8
		signals = append(signals, "account is currently banned")
	}
	if !target.Enabled {
		score += 4
		signals = append(signals, "account is disabled")
	}
	if !target.Verified {
		score += 2
		signals = append(signals, "account is not verified")
	}
	authoredMessageIDs := map[int64]struct{}{}
	if a.boardRepo != nil && a.msgRepo != nil {
		if boards, err := a.boardRepo.List(); err == nil {
			for _, board := range boards {
				rows, err := a.msgRepo.ListByBoard(board.ID)
				if err != nil {
					continue
				}
				for _, row := range rows {
					if row.AuthorID == target.ID {
						authoredMessageIDs[row.ID] = struct{}{}
					}
				}
			}
		}
	}
	openReports := 0
	resolvedReports := 0
	if a.msgRepo != nil {
		if reports, err := a.msgRepo.ListReports(600, ""); err == nil {
			for _, report := range reports {
				if _, ok := authoredMessageIDs[report.MessageID]; !ok {
					continue
				}
				if strings.EqualFold(report.Status, "resolved") {
					resolvedReports++
				} else {
					openReports++
				}
			}
		}
	}
	if openReports > 0 {
		score += openReports * 3
		signals = append(signals, strconv.Itoa(openReports)+" open report(s) on authored posts")
	}
	if resolvedReports > 0 {
		score += minInt(resolvedReports, 6)
		signals = append(signals, strconv.Itoa(resolvedReports)+" resolved report(s) in history")
	}
	timeline := a.buildIncidentTimeline(target, 24)
	if len(timeline) > 0 {
		score += minInt(len(timeline), 5)
		signals = append(signals, strconv.Itoa(len(timeline))+" incident timeline item(s)")
	}
	level := "low"
	switch {
	case score >= 14:
		level = "high"
	case score >= 6:
		level = "medium"
	}
	if len(signals) == 0 {
		signals = append(signals, "no risk signals currently detected")
	}
	return score, level, signals
}

func (a *webApp) handleResumeCenter(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	items := a.buildResumeItems(user)
	if r.Method == http.MethodPost {
		if !a.requireCSRF(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		switch action {
		case "set_home":
			route := normalizeHomeRoute(r.FormValue("route"))
			if route == "" {
				redirectWithError(w, r, "/resume", "Choose a valid home route.")
				return
			}
			a.persistHomeRoute(user.Handle, route)
			redirectWithNotice(w, r, "/resume", "Home route updated to "+route+".")
			return
		default:
			redirectWithError(w, r, "/resume", "Unsupported resume action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	top := items[0]
	rowList := strings.Builder{}
	for _, row := range items {
		setHome := ""
		if route := normalizeHomeRoute(row.HomeRoute); route != "" {
			setHome = `<form method="POST" action="/resume" class="wolfbbs-inline-form"><input type="hidden" name="action" value="set_home"><input type="hidden" name="route" value="` + htmlEscape(route) + `">` + a.csrfHiddenInput(r) + `<button type="submit">Set ` + htmlEscape(route) + ` as home</button></form>`
		}
		rowList.WriteString(`<article class="wolfbbs-card"><h2>` + htmlEscape(row.Title) + `</h2><p>` + htmlEscape(row.Detail) + `</p><p><a href="` + htmlEscape(row.Href) + `">Open</a></p>` + setHome + `</article>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Smart Re-entry</title></head><body>
<p><a href="/start">start</a> | <a href="/today">today</a> | <a href="/resume">resume</a> | <a href="/attention">attention</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Smart Re-entry</h1>
<p>Roadmap 156: preserve momentum between sessions by returning you to the most unfinished, highest-value workflow first.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + htmlEscape(top.Title) + `</strong><span>top re-entry recommendation</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(items)) + `</strong><span>active resume tracks</span></article></section>
<section class="wolfbbs-grid">` + rowList.String() + `</section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleDoorComeback(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	activity := a.myCallerActivity(user.Handle)
	recommended := topRecommendedDoors(a.buildDoorCatalog(user), 5)
	recommendedRows := strings.Builder{}
	for _, row := range recommended {
		recommendedRows.WriteString(`<li><strong>` + htmlEscape(row.Door.Name) + `</strong> <span class="wolfbbs-muted">` + htmlEscape(defaultIfBlank(row.Door.Description, "classic door run")) + `</span> <a href="/doors?door=` + url.QueryEscape(row.Door.ID) + `">open</a></li>`)
	}
	if recommendedRows.Len() == 0 {
		recommendedRows.WriteString(`<li>No recommended doors yet. Open the Door Cockpit and run any available door.</li>`)
	}
	comebackState := "streak active"
	comebackDetail := "Keep the loop warm with one quick door run today."
	if activity.CurrentStreak == 0 {
		comebackState = "streak broken"
		comebackDetail = "One door run today restores your momentum lane."
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Door Comeback</title></head><body>
<p><a href="/doors">doors</a> | <a href="/scores">scores</a> | <a href="/streaks">streaks</a> | <a href="/challenges">challenges</a> | <a href="/missions">missions</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Door Comeback Prompts</h1>
<p>Roadmap 157: reconnect callers to sticky game loops after streak breaks with concrete next actions.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + htmlEscape(comebackState) + `</strong><span>current comeback state</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(activity.Door7) + `</strong><span>door-active days (7d)</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(activity.CurrentStreak) + `</strong><span>current streak</span></article></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Why now</h2><p>` + htmlEscape(comebackDetail) + `</p><ul><li><a href="/doors?mode=recommended">Open recommended doors</a></li><li><a href="/scores">Check ladder movement</a></li><li><a href="/challenges">Join seasonal challenge scoring</a></li><li><a href="/missions">Claim mission progress</a></li></ul></article><article class="wolfbbs-card"><h2>Recommended comeback doors</h2><ul class="wolfbbs-list-clean">` + recommendedRows.String() + `</ul></article></section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleMentorship(w http.ResponseWriter, r *http.Request) {
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
		case "ping_mentor":
			pair, ok := a.mentorshipForMentee(user.Handle)
			if !ok {
				redirectWithError(w, r, "/mentorship", "No active mentor is assigned.")
				return
			}
			mentor, err := a.authSvc.GetUser(pair.Mentor)
			if err != nil || mentor == nil {
				redirectWithError(w, r, "/mentorship", "Assigned mentor account was not found.")
				return
			}
			body := strings.TrimSpace(r.FormValue("message"))
			if body == "" {
				body = "Hi mentor, can you help me with my next steps on WolfBBS?"
			}
			if a.mailRepo != nil {
				if err := a.mailRepo.CreateMail(&domain.PrivateMail{
					FromUserID: user.ID,
					ToUserID:   mentor.ID,
					Subject:    "[mentorship] check-in from " + user.Handle,
					Body:       body,
				}); err != nil {
					redirectWithError(w, r, "/mentorship", "Could not send mentorship check-in.")
					return
				}
			}
			redirectWithNotice(w, r, "/mentorship", "Mentor check-in sent.")
			return
		default:
			redirectWithError(w, r, "/mentorship", "Unsupported mentorship action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	pair, assigned := a.mentorshipForMentee(user.Handle)
	pairBlock := `<article class="wolfbbs-card"><h2>No Mentor Assigned Yet</h2><p>Ask sysop to pair you in <a href="/admin/mentorship">/admin/mentorship</a> if you want guided onboarding.</p></article>`
	if assigned {
		pairBlock = `<article class="wolfbbs-card"><h2>Your Mentor</h2><p><strong>` + htmlEscape(pair.Mentor) + `</strong></p><p>` + htmlEscape(defaultIfBlank(pair.Note, "Mentor pairing is active.")) + `</p><form method="POST" action="/mentorship"><input type="hidden" name="action" value="ping_mentor">` + a.csrfHiddenInput(r) + `<label>Message<br><textarea name="message" rows="5" cols="68" placeholder="Share where you are blocked."></textarea></label><br><button type="submit">Send Check-In</button></form></article>`
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Mentorship Desk</title></head><body>
<p><a href="/start">start</a> | <a href="/first-call">first-call</a> | <a href="/mentorship">mentorship</a> | <a href="/directory">directory</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Mentorship Pairing</h1>
<p>Roadmap 158: improve onboarding quality with explicit mentor/mentee pairing and direct check-ins.</p>
<section class="wolfbbs-grid">` + pairBlock + `<article class="wolfbbs-card"><h2>Suggested mentor topics</h2><ul><li>Choosing a durable home route</li><li>How to use boards vs chat for the right conversation</li><li>How to earn quick wins in doors, missions, and challenges</li></ul></article></section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminMentorship(w http.ResponseWriter, r *http.Request) {
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
		case "save_pair":
			row := mentorshipPair{
				Mentee:    r.FormValue("mentee"),
				Mentor:    r.FormValue("mentor"),
				Note:      r.FormValue("note"),
				Active:    formHasValue(r, "active"),
				UpdatedBy: user.Handle,
				UpdatedAt: time.Now().UTC(),
			}
			normalized, valid := normalizeMentorshipPair(row)
			if !valid {
				redirectWithError(w, r, "/admin/mentorship", "Mentee and mentor must be valid, distinct handles.")
				return
			}
			rows := a.loadMentorshipPairs()
			updated := false
			for i := range rows {
				if rows[i].Mentee == normalized.Mentee {
					rows[i] = normalized
					updated = true
					break
				}
			}
			if !updated {
				rows = append(rows, normalized)
			}
			a.persistMentorshipPairs(rows)
			a.recordAdminAction(user.Handle, normalized.Mentee, "save_mentorship_pair", "mentor="+normalized.Mentor)
			redirectWithNotice(w, r, "/admin/mentorship", "Mentorship pair saved.")
			return
		case "delete_pair":
			mentee := normalizeHandleKey(r.FormValue("mentee"))
			if mentee == "" {
				redirectWithError(w, r, "/admin/mentorship", "Mentee handle is required.")
				return
			}
			rows := a.loadMentorshipPairs()
			next := make([]mentorshipPair, 0, len(rows))
			removed := false
			for _, row := range rows {
				if row.Mentee == mentee {
					removed = true
					continue
				}
				next = append(next, row)
			}
			if !removed {
				redirectWithError(w, r, "/admin/mentorship", "Mentorship pair not found.")
				return
			}
			a.persistMentorshipPairs(next)
			a.recordAdminAction(user.Handle, mentee, "delete_mentorship_pair", "")
			redirectWithNotice(w, r, "/admin/mentorship", "Mentorship pair removed.")
			return
		default:
			redirectWithError(w, r, "/admin/mentorship", "Unsupported mentorship action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	users, _ := a.authSvc.ListUsers()
	allHandles := make([]string, 0, len(users))
	for _, row := range users {
		allHandles = append(allHandles, row.Handle)
	}
	sort.Slice(allHandles, func(i, j int) bool { return strings.ToLower(allHandles[i]) < strings.ToLower(allHandles[j]) })
	staffHandles := listStaffHandles(users)
	handleOptions := strings.Builder{}
	for _, handle := range allHandles {
		handleOptions.WriteString(`<option value="` + htmlEscape(handle) + `">` + htmlEscape(handle) + `</option>`)
	}
	staffOptions := strings.Builder{}
	for _, handle := range staffHandles {
		staffOptions.WriteString(`<option value="` + htmlEscape(handle) + `">` + htmlEscape(handle) + `</option>`)
	}
	rows := a.loadMentorshipPairs()
	tableRows := strings.Builder{}
	for _, row := range rows {
		tableRows.WriteString(`<tr><td>` + htmlEscape(row.Mentee) + `</td><td>` + htmlEscape(row.Mentor) + `</td><td>` + boolToText(row.Active) + `</td><td>` + htmlEscape(defaultIfBlank(row.Note, "-")) + `</td><td>` + row.UpdatedAt.Local().Format("2006-01-02 15:04") + `</td><td><form method="POST" action="/admin/mentorship" style="display:inline"><input type="hidden" name="action" value="delete_pair"><input type="hidden" name="mentee" value="` + htmlEscape(row.Mentee) + `">` + a.csrfHiddenInput(r) + `<button type="submit">Delete</button></form></td></tr>`)
	}
	if tableRows.Len() == 0 {
		tableRows.WriteString(`<tr><td colspan="6">No mentorship pairs configured yet.</td></tr>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Mentorship Admin</title></head><body>
<p><a href="/admin">admin</a> | <a href="/admin/users">users</a> | <a href="/mentorship">public mentorship desk</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Mentorship Pairing Admin</h1>
<p>Roadmap 158 control plane for new-caller mentor assignment.</p>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Create / Update Pair</h2><form method="POST" action="/admin/mentorship"><input type="hidden" name="action" value="save_pair">` + a.csrfHiddenInput(r) + `<label>Mentee <input name="mentee" list="allHandles" placeholder="newcaller"></label><datalist id="allHandles">` + handleOptions.String() + `</datalist><br><label>Mentor <input name="mentor" list="staffHandles" placeholder="moderator/sysop"></label><datalist id="staffHandles">` + staffOptions.String() + `</datalist><br><label>Note <input name="note" size="72" placeholder="onboarding focus and expectations"></label><br><label><input type="checkbox" name="active" value="1" checked> Active</label><br><button type="submit">Save Pair</button></form></article><article class="wolfbbs-card"><h2>Guidance</h2><ul><li>Pair new callers with active moderators/sysops.</li><li>Use notes for scope and availability expectations.</li><li>Ask mentees to use /mentorship check-ins for concrete blockers.</li></ul></article></section>
<table border="1"><tr><th>Mentee</th><th>Mentor</th><th>Active</th><th>Note</th><th>Updated</th><th>Action</th></tr>` + tableRows.String() + `</table>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleMilestones(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	celebrations := a.loadMilestoneCelebrations(user.Handle)
	counts := a.milestoneCountsForUser(user)
	rows := buildMilestones(counts, celebrations)
	if r.Method == http.MethodPost {
		if !a.requireCSRF(w, r) {
			return
		}
		if strings.ToLower(strings.TrimSpace(r.FormValue("action"))) != "celebrate" {
			redirectWithError(w, r, "/milestones", "Unsupported milestone action.")
			return
		}
		id := strings.TrimSpace(r.FormValue("id"))
		if id == "" {
			redirectWithError(w, r, "/milestones", "Milestone id is required.")
			return
		}
		allowed := false
		title := id
		for _, row := range rows {
			if row.ID != id {
				continue
			}
			allowed = row.Reached
			title = row.Title
			break
		}
		if !allowed {
			redirectWithError(w, r, "/milestones", "Milestone is not complete yet.")
			return
		}
		celebrations[id] = time.Now().UTC()
		a.persistMilestoneCelebrations(user.Handle, celebrations)
		if a.oneLinerzMod != nil {
			a.oneLinerzMod.Add(user.Handle, "celebrated milestone: "+title)
		}
		redirectWithNotice(w, r, "/milestones", "Milestone celebration posted.")
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cardRows := strings.Builder{}
	reachedCount := 0
	for _, row := range rows {
		if row.Reached {
			reachedCount++
		}
		status := "in progress"
		action := ""
		if row.Reached {
			status = "reached"
			action = `<span class="wolfbbs-muted">Already celebrated</span>`
			if row.Celebrate {
				action = `<form method="POST" action="/milestones" class="wolfbbs-inline-form"><input type="hidden" name="action" value="celebrate"><input type="hidden" name="id" value="` + htmlEscape(row.ID) + `">` + a.csrfHiddenInput(r) + `<button type="submit">Celebrate</button></form>`
			}
		}
		cardRows.WriteString(`<article class="wolfbbs-card"><h2>` + htmlEscape(row.Title) + `</h2><p>` + htmlEscape(row.Detail) + `</p><p><strong>` + strconv.Itoa(row.Current) + ` / ` + strconv.Itoa(row.Target) + `</strong> <span class="wolfbbs-muted">` + status + `</span></p>` + action + `</article>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Milestone Celebrations</title></head><body>
<p><a href="/start">start</a> | <a href="/streaks">streaks</a> | <a href="/challenges">challenges</a> | <a href="/missions">missions</a> | <a href="/clubhouse">clubhouse</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Profile Milestone Celebrations</h1>
<p>Roadmap 159: make progress visible with milestone cards and optional celebration posts.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(reachedCount) + `/` + strconv.Itoa(len(rows)) + `</strong><span>milestones reached</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(counts["streak"]) + `</strong><span>current streak</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(counts["achievements"]) + `</strong><span>door achievements</span></article></section>
<section class="wolfbbs-grid">` + cardRows.String() + `</section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

type timeLane struct {
	Key       string
	Title     string
	Detail    string
	Primary   string
	Secondary string
	HomeRoute string
}

func laneForHour(hour int) timeLane {
	switch {
	case hour >= 5 && hour < 11:
		return timeLane{
			Key:       "morning",
			Title:     "Morning Catch-Up",
			Detail:    "Start with summary surfaces before diving into conversations.",
			Primary:   "/today",
			Secondary: "/digest",
			HomeRoute: "/today",
		}
	case hour >= 11 && hour < 17:
		return timeLane{
			Key:       "day",
			Title:     "Daytime Build Loop",
			Detail:    "Use board threads and direct follow-up while momentum is high.",
			Primary:   "/boards",
			Secondary: "/attention",
			HomeRoute: "/boards",
		}
	case hour >= 17 && hour < 23:
		return timeLane{
			Key:       "evening",
			Title:     "Evening Social Loop",
			Detail:    "Join live conversations and scheduled events.",
			Primary:   "/chat",
			Secondary: "/events",
			HomeRoute: "/chat",
		}
	default:
		return timeLane{
			Key:       "night",
			Title:     "Night Arcade Loop",
			Detail:    "Low-noise loop: doors, scores, and focused streak recovery.",
			Primary:   "/doors",
			Secondary: "/streaks",
			HomeRoute: "/doors",
		}
	}
}

func (a *webApp) handleTimeLane(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	now := time.Now().In(time.Local)
	lane := laneForHour(now.Hour())
	if a.unreadMailCountForUser(user) > 0 {
		lane.Primary = "/mail?box=unread"
		lane.Detail = "Unread private mail exists, so inbox takes priority first."
	}
	if r.Method == http.MethodPost {
		if !a.requireCSRF(w, r) {
			return
		}
		if strings.ToLower(strings.TrimSpace(r.FormValue("action"))) != "set_home" {
			redirectWithError(w, r, "/time-lane", "Unsupported time-lane action.")
			return
		}
		home := normalizeHomeRoute(r.FormValue("route"))
		if home == "" {
			redirectWithError(w, r, "/time-lane", "Choose a valid home route.")
			return
		}
		a.persistHomeRoute(user.Handle, home)
		redirectWithNotice(w, r, "/time-lane", "Home route updated to "+home+".")
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Time Lane</title></head><body>
<p><a href="/start">start</a> | <a href="/today">today</a> | <a href="/time-lane">time lane</a> | <a href="/settings">settings</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Time-of-Day Landing States</h1>
<p>Roadmap 160: tailor landing state to current local time and current caller queue pressure.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + htmlEscape(lane.Title) + `</strong><span>active lane</span></article><article class="wolfbbs-kpi-card"><strong>` + now.Format("15:04") + `</strong><span>local caller time</span></article><article class="wolfbbs-kpi-card"><strong>` + htmlEscape(a.preferredHomeRoute(user)) + `</strong><span>current home route</span></article></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Lane guidance</h2><p>` + htmlEscape(lane.Detail) + `</p><ul><li><a href="` + htmlEscape(lane.Primary) + `">Primary route</a></li><li><a href="` + htmlEscape(lane.Secondary) + `">Secondary route</a></li><li><a href="/resume">Smart re-entry</a></li></ul><form method="POST" action="/time-lane"><input type="hidden" name="action" value="set_home"><input type="hidden" name="route" value="` + htmlEscape(lane.HomeRoute) + `">` + a.csrfHiddenInput(r) + `<button type="submit">Set ` + htmlEscape(lane.HomeRoute) + ` as home route</button></form></article></section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) buildReportQueueRows() []reportQueueRow {
	reports, err := a.msgRepo.ListReports(300, "")
	if err != nil {
		a.addAppError("moderation.reports", fmt.Errorf("list reports: %w", err))
		return nil
	}
	assignments := a.loadReportAssignments()
	handleByID := map[int64]string{}
	if users, err := a.authSvc.ListUsers(); err == nil {
		for _, row := range users {
			handleByID[row.ID] = row.Handle
		}
	}
	boardNames := map[int64]string{}
	if boards, err := a.boardRepo.List(); err == nil {
		for _, row := range boards {
			boardNames[row.ID] = row.Name
		}
	}
	now := time.Now().UTC()
	rows := make([]reportQueueRow, 0, len(reports))
	for _, report := range reports {
		msg, _ := a.msgRepo.GetMessage(report.MessageID)
		boardID := int64(0)
		subject := "(message unavailable)"
		if msg != nil {
			boardID = msg.BoardID
			subject = cleanOneLiner(msg.Subject, 72)
		}
		boardName := boardNames[boardID]
		if boardName == "" {
			boardName = "(unknown board)"
		}
		link := "#"
		if boardID > 0 {
			link = "/boards?board=" + strconv.FormatInt(boardID, 10) + "&id=" + strconv.FormatInt(report.MessageID, 10)
		}
		assignment, ok := assignments[report.ID]
		if !ok {
			assignment = normalizeReportAssignment(reportAssignment{
				ReportID: report.ID,
				Status:   report.Status,
				Priority: inferPriority(report.Reason),
				DueAt:    report.CreatedAt.UTC().Add(time.Duration(defaultSLAHours(report.Reason)) * time.Hour),
			}, report.Reason)
		} else {
			assignment = normalizeReportAssignment(assignment, report.Reason)
		}
		slaState, slaDetail := reportSLAState(assignment, report, now)
		rows = append(rows, reportQueueRow{
			Report:        report,
			Reporter:      defaultIfBlank(handleByID[report.ReporterID], "#"+strconv.FormatInt(report.ReporterID, 10)),
			MessageLink:   link,
			BoardName:     boardName,
			Subject:       subject,
			Assignment:    assignment,
			SLAState:      slaState,
			SLADetail:     slaDetail,
			DefaultSLAHrs: defaultSLAHours(report.Reason),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].SLAState == rows[j].SLAState {
			return rows[i].Report.CreatedAt.After(rows[j].Report.CreatedAt)
		}
		weight := func(state string) int {
			switch state {
			case "breach":
				return 3
			case "warning":
				return 2
			case "on-track":
				return 1
			default:
				return 0
			}
		}
		return weight(rows[i].SLAState) > weight(rows[j].SLAState)
	})
	return rows
}

func (a *webApp) handleAdminModCenter(w http.ResponseWriter, r *http.Request) {
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
		case "save_report_assignment":
			reportID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("report_id")), 10, 64)
			if reportID <= 0 {
				redirectWithError(w, r, "/admin/mod-center", "Report ID is required.")
				return
			}
			dueHours := parseIntWithFallback(r.FormValue("due_hours"), 24)
			dueHours = clampInt(dueHours, 1, 168)
			assignments := a.loadReportAssignments()
			row := assignments[reportID]
			row.ReportID = reportID
			row.Assignee = r.FormValue("assignee")
			row.Status = r.FormValue("status")
			row.Priority = r.FormValue("priority")
			row.Note = r.FormValue("note")
			row.DueAt = time.Now().UTC().Add(time.Duration(dueHours) * time.Hour)
			row.UpdatedBy = user.Handle
			row.UpdatedAt = time.Now().UTC()
			assignments[reportID] = normalizeReportAssignment(row, "")
			a.persistReportAssignments(assignments)
			a.recordAdminAction(user.Handle, "report #"+strconv.FormatInt(reportID, 10), "save_report_assignment", "assignee="+normalizeHandleKey(row.Assignee)+" status="+moderationAssignmentStatus(row.Status))
			redirectWithNotice(w, r, "/admin/mod-center", "Report assignment saved.")
			return
		case "resolve_report_assignment":
			reportID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("report_id")), 10, 64)
			if reportID <= 0 {
				redirectWithError(w, r, "/admin/mod-center", "Report ID is required.")
				return
			}
			if err := a.msgRepo.ResolveReport(reportID, user.Handle, time.Now().UTC()); err != nil {
				redirectWithError(w, r, "/admin/mod-center", "Could not resolve report.")
				return
			}
			assignments := a.loadReportAssignments()
			row := assignments[reportID]
			row.ReportID = reportID
			row.Status = "resolved"
			row.ResolvedAt = time.Now().UTC()
			row.UpdatedBy = user.Handle
			row.UpdatedAt = time.Now().UTC()
			assignments[reportID] = normalizeReportAssignment(row, "")
			a.persistReportAssignments(assignments)
			a.recordAdminAction(user.Handle, "report #"+strconv.FormatInt(reportID, 10), "resolve_report_assignment", "")
			redirectWithNotice(w, r, "/admin/mod-center", "Report resolved.")
			return
		case "save_canned":
			row := moderationTemplate{
				ID:         r.FormValue("id"),
				Category:   r.FormValue("category"),
				Title:      r.FormValue("title"),
				Body:       r.FormValue("body"),
				UpdatedBy:  user.Handle,
				UpdatedAt:  time.Now().UTC(),
				UsageCount: 0,
			}
			rows := a.loadModeratorTemplates()
			normalized, valid := normalizeModeratorTemplate(row)
			if !valid {
				redirectWithError(w, r, "/admin/mod-center", "Canned response title and body are required.")
				return
			}
			updated := false
			for i := range rows {
				if rows[i].ID == normalized.ID {
					normalized.UsageCount = rows[i].UsageCount
					rows[i] = normalized
					updated = true
					break
				}
			}
			if !updated {
				rows = append(rows, normalized)
			}
			a.persistModeratorTemplates(rows)
			a.recordAdminAction(user.Handle, normalized.ID, "save_canned_response", normalized.Category)
			redirectWithNotice(w, r, "/admin/mod-center", "Canned response saved.")
			return
		case "delete_canned":
			id := strings.TrimSpace(r.FormValue("id"))
			if id == "" {
				redirectWithError(w, r, "/admin/mod-center", "Canned response id is required.")
				return
			}
			rows := a.loadModeratorTemplates()
			next := make([]moderationTemplate, 0, len(rows))
			removed := false
			for _, row := range rows {
				if row.ID == id {
					removed = true
					continue
				}
				next = append(next, row)
			}
			if !removed {
				redirectWithError(w, r, "/admin/mod-center", "Canned response not found.")
				return
			}
			a.persistModeratorTemplates(next)
			a.recordAdminAction(user.Handle, id, "delete_canned_response", "")
			redirectWithNotice(w, r, "/admin/mod-center", "Canned response deleted.")
			return
		case "send_canned":
			id := strings.TrimSpace(r.FormValue("template_id"))
			targetHandle := strings.TrimSpace(r.FormValue("target"))
			target, err := a.authSvc.GetUser(targetHandle)
			if id == "" || err != nil || target == nil {
				redirectWithError(w, r, "/admin/mod-center", "Template and valid target handle are required.")
				return
			}
			var picked *moderationTemplate
			rows := a.loadModeratorTemplates()
			for i := range rows {
				if rows[i].ID == id {
					picked = &rows[i]
					break
				}
			}
			if picked == nil {
				redirectWithError(w, r, "/admin/mod-center", "Canned response not found.")
				return
			}
			body := picked.Body
			if extra := cleanOneLiner(r.FormValue("extra_note"), 220); extra != "" {
				body += "\n\n" + extra
			}
			if err := a.mailRepo.CreateMail(&domain.PrivateMail{
				FromUserID: user.ID,
				ToUserID:   target.ID,
				Subject:    "[moderation] " + cleanOneLiner(picked.Title, 72),
				Body:       body,
			}); err != nil {
				redirectWithError(w, r, "/admin/mod-center", "Could not send canned response.")
				return
			}
			for i := range rows {
				if rows[i].ID == picked.ID {
					rows[i].UsageCount++
					rows[i].UpdatedAt = time.Now().UTC()
					rows[i].UpdatedBy = user.Handle
					break
				}
			}
			a.persistModeratorTemplates(rows)
			a.recordAdminAction(user.Handle, target.Handle, "send_canned_response", "template="+picked.ID)
			redirectWithNotice(w, r, "/admin/mod-center", "Canned response sent.")
			return
		case "save_case":
			row := caseThread{
				ID:           strings.TrimSpace(r.FormValue("id")),
				Title:        r.FormValue("title"),
				Status:       r.FormValue("status"),
				Priority:     r.FormValue("priority"),
				Owner:        r.FormValue("owner"),
				TargetHandle: r.FormValue("target_handle"),
				Summary:      r.FormValue("summary"),
				ReportID:     int64(parseInt(r.FormValue("report_id"), 0)),
			}
			normalized, valid := normalizeCaseThread(row)
			if !valid {
				redirectWithError(w, r, "/admin/mod-center", "Case title is required.")
				return
			}
			rows := a.loadCaseThreads()
			updated := false
			for i := range rows {
				if rows[i].ID == normalized.ID {
					normalized.CreatedAt = rows[i].CreatedAt
					normalized.Updates = rows[i].Updates
					rows[i] = normalized
					updated = true
					break
				}
			}
			if !updated {
				normalized.CreatedAt = time.Now().UTC()
				normalized.UpdatedAt = normalized.CreatedAt
				rows = append(rows, normalized)
			}
			a.persistCaseThreads(rows)
			a.recordAdminAction(user.Handle, normalized.ID, "save_case_thread", normalized.Status+"/"+normalized.Priority)
			redirectWithNotice(w, r, "/admin/mod-center", "Case thread saved.")
			return
		case "add_case_note":
			caseID := strings.TrimSpace(r.FormValue("case_id"))
			note := cleanOneLiner(r.FormValue("note"), 320)
			if caseID == "" || note == "" {
				redirectWithError(w, r, "/admin/mod-center", "Case id and note are required.")
				return
			}
			rows := a.loadCaseThreads()
			found := false
			for i := range rows {
				if rows[i].ID != caseID {
					continue
				}
				rows[i].Updates = append([]caseUpdate{{
					At:    time.Now().UTC(),
					Actor: user.Handle,
					Note:  note,
				}}, rows[i].Updates...)
				rows[i].UpdatedAt = time.Now().UTC()
				found = true
				break
			}
			if !found {
				redirectWithError(w, r, "/admin/mod-center", "Case thread not found.")
				return
			}
			a.persistCaseThreads(rows)
			a.recordAdminAction(user.Handle, caseID, "add_case_note", "")
			redirectWithNotice(w, r, "/admin/mod-center", "Case note added.")
			return
		case "close_case":
			caseID := strings.TrimSpace(r.FormValue("case_id"))
			if caseID == "" {
				redirectWithError(w, r, "/admin/mod-center", "Case id is required.")
				return
			}
			rows := a.loadCaseThreads()
			found := false
			for i := range rows {
				if rows[i].ID != caseID {
					continue
				}
				rows[i].Status = "resolved"
				rows[i].UpdatedAt = time.Now().UTC()
				rows[i].Updates = append([]caseUpdate{{
					At:    time.Now().UTC(),
					Actor: user.Handle,
					Note:  "Case closed",
				}}, rows[i].Updates...)
				found = true
				break
			}
			if !found {
				redirectWithError(w, r, "/admin/mod-center", "Case thread not found.")
				return
			}
			a.persistCaseThreads(rows)
			a.recordAdminAction(user.Handle, caseID, "close_case_thread", "")
			redirectWithNotice(w, r, "/admin/mod-center", "Case closed.")
			return
		default:
			redirectWithError(w, r, "/admin/mod-center", "Unsupported moderation action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	reportRows := a.buildReportQueueRows()
	staff := []string{}
	if users, err := a.authSvc.ListUsers(); err == nil {
		staff = listStaffHandles(users)
	}
	staffOptions := strings.Builder{}
	for _, handle := range staff {
		staffOptions.WriteString(`<option value="` + htmlEscape(handle) + `">` + htmlEscape(handle) + `</option>`)
	}

	reportTable := strings.Builder{}
	breachCount := 0
	warnCount := 0
	for _, row := range reportRows {
		if row.SLAState == "breach" {
			breachCount++
		} else if row.SLAState == "warning" {
			warnCount++
		}
		reportTable.WriteString(`<tr><td>` + strconv.FormatInt(row.Report.ID, 10) + `</td><td><a href="` + htmlEscape(row.MessageLink) + `">` + htmlEscape(row.Subject) + `</a></td><td>` + htmlEscape(row.BoardName) + `</td><td>` + htmlEscape(row.Reporter) + `</td><td>` + htmlEscape(row.Assignment.Priority) + `</td><td>` + htmlEscape(row.Assignment.Status) + `</td><td>` + htmlEscape(defaultIfBlank(row.Assignment.Assignee, "unassigned")) + `</td><td>` + htmlEscape(row.SLAState+" / "+row.SLADetail) + `</td><td>` + htmlEscape(cleanOneLiner(row.Report.Reason, 96)) + `</td><td><form method="POST" action="/admin/mod-center" class="wolfbbs-inline-form"><input type="hidden" name="action" value="save_report_assignment"><input type="hidden" name="report_id" value="` + strconv.FormatInt(row.Report.ID, 10) + `">` + a.csrfHiddenInput(r) + `<label>Assignee <input name="assignee" list="staffHandles" value="` + htmlEscape(row.Assignment.Assignee) + `" size="10"></label><label>Status <select name="status"><option value="open"` + selectedIf(row.Assignment.Status == "open") + `>open</option><option value="assigned"` + selectedIf(row.Assignment.Status == "assigned") + `>assigned</option><option value="investigating"` + selectedIf(row.Assignment.Status == "investigating") + `>investigating</option><option value="resolved"` + selectedIf(row.Assignment.Status == "resolved") + `>resolved</option></select></label><label>Priority <select name="priority"><option value="low"` + selectedIf(row.Assignment.Priority == "low") + `>low</option><option value="normal"` + selectedIf(row.Assignment.Priority == "normal") + `>normal</option><option value="high"` + selectedIf(row.Assignment.Priority == "high") + `>high</option><option value="urgent"` + selectedIf(row.Assignment.Priority == "urgent") + `>urgent</option></select></label><label>Due hrs <input name="due_hours" value="` + strconv.Itoa(row.DefaultSLAHrs) + `" size="4"></label><label>Note <input name="note" value="` + htmlEscape(row.Assignment.Note) + `" size="18"></label><button type="submit">Save</button></form><form method="POST" action="/admin/mod-center" class="wolfbbs-inline-form"><input type="hidden" name="action" value="resolve_report_assignment"><input type="hidden" name="report_id" value="` + strconv.FormatInt(row.Report.ID, 10) + `">` + a.csrfHiddenInput(r) + `<button type="submit">Resolve</button></form></td></tr>`)
	}
	if reportTable.Len() == 0 {
		reportTable.WriteString(`<tr><td colspan="10">No reports available.</td></tr>`)
	}

	templates := a.loadModeratorTemplates()
	templateTable := strings.Builder{}
	for _, row := range templates {
		templateTable.WriteString(`<tr><td><code>` + htmlEscape(row.ID) + `</code></td><td>` + htmlEscape(row.Category) + `</td><td>` + htmlEscape(row.Title) + `</td><td>` + htmlEscape(cleanOneLiner(row.Body, 96)) + `</td><td>` + strconv.Itoa(row.UsageCount) + `</td><td><form method="POST" action="/admin/mod-center" style="display:inline"><input type="hidden" name="action" value="delete_canned"><input type="hidden" name="id" value="` + htmlEscape(row.ID) + `">` + a.csrfHiddenInput(r) + `<button type="submit">Delete</button></form></td></tr>`)
	}
	if templateTable.Len() == 0 {
		templateTable.WriteString(`<tr><td colspan="6">No canned responses saved.</td></tr>`)
	}
	templateOptions := strings.Builder{}
	for _, row := range templates {
		templateOptions.WriteString(`<option value="` + htmlEscape(row.ID) + `">` + htmlEscape(row.Title) + ` (` + htmlEscape(row.Category) + `)</option>`)
	}
	if templateOptions.Len() == 0 {
		templateOptions.WriteString(`<option value="">No templates</option>`)
	}

	cases := a.loadCaseThreads()
	caseTable := strings.Builder{}
	for _, row := range cases {
		caseTable.WriteString(`<tr><td><code>` + htmlEscape(row.ID) + `</code></td><td>` + htmlEscape(row.Title) + `</td><td>` + htmlEscape(row.Status) + `</td><td>` + htmlEscape(row.Priority) + `</td><td>` + htmlEscape(defaultIfBlank(row.Owner, "unassigned")) + `</td><td>` + htmlEscape(defaultIfBlank(row.TargetHandle, "-")) + `</td><td>` + strconv.FormatInt(row.ReportID, 10) + `</td><td>` + strconv.Itoa(len(row.Updates)) + `</td><td>` + row.UpdatedAt.Local().Format("2006-01-02 15:04") + `</td><td><form method="POST" action="/admin/mod-center" class="wolfbbs-inline-form"><input type="hidden" name="action" value="add_case_note"><input type="hidden" name="case_id" value="` + htmlEscape(row.ID) + `">` + a.csrfHiddenInput(r) + `<input name="note" size="22" placeholder="handoff or follow-up"><button type="submit">Add Note</button></form><form method="POST" action="/admin/mod-center" class="wolfbbs-inline-form"><input type="hidden" name="action" value="close_case"><input type="hidden" name="case_id" value="` + htmlEscape(row.ID) + `">` + a.csrfHiddenInput(r) + `<button type="submit">Close</button></form></td></tr>`)
	}
	if caseTable.Len() == 0 {
		caseTable.WriteString(`<tr><td colspan="10">No case threads yet.</td></tr>`)
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Moderation Center</title></head><body>
<p><a href="/admin">admin</a> | <a href="/admin/boards">boards</a> | <a href="/admin/mail">mail</a> | <a href="/admin/mod-center">mod center</a> | <a href="/directory">caller profiles</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Moderation Center</h1>
<p>Roadmap 161-165: report assignment queue + SLA tracking, caller risk context, canned responses, and case threads.</p>
<datalist id="staffHandles">` + staffOptions.String() + `</datalist>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(reportRows)) + `</strong><span>reports tracked</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(breachCount) + `</strong><span>SLA breaches</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(warnCount) + `</strong><span>SLA warnings</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(cases)) + `</strong><span>case threads</span></article></section>
<h2>Report Assignment Queue (161 + 162)</h2>
<table border="1"><tr><th>ID</th><th>Message</th><th>Board</th><th>Reporter</th><th>Priority</th><th>Status</th><th>Assignee</th><th>SLA</th><th>Reason</th><th>Actions</th></tr>` + reportTable.String() + `</table>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Canned Responses (164)</h2><form method="POST" action="/admin/mod-center"><input type="hidden" name="action" value="save_canned">` + a.csrfHiddenInput(r) + `<label>ID <input name="id" size="24" placeholder="warn_spam_v1"></label><br><label>Category <input name="category" size="16" value="general"></label><br><label>Title <input name="title" size="64" placeholder="Reminder about board rules"></label><br><label>Body<br><textarea name="body" rows="8" cols="82" placeholder="Template body with concrete next steps."></textarea></label><br><button type="submit">Save Template</button></form><h3>Send Template</h3><form method="POST" action="/admin/mod-center"><input type="hidden" name="action" value="send_canned">` + a.csrfHiddenInput(r) + `<label>Template <select name="template_id">` + templateOptions.String() + `</select></label><br><label>Target handle <input name="target" size="18" placeholder="caller"></label><br><label>Extra note <input name="extra_note" size="56" placeholder="optional one-line context"></label><br><button type="submit">Send Canned Response</button></form></article><article class="wolfbbs-card"><h2>Saved Templates</h2><table border="1"><tr><th>ID</th><th>Category</th><th>Title</th><th>Preview</th><th>Usage</th><th>Action</th></tr>` + templateTable.String() + `</table></article></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Case Threads (165)</h2><form method="POST" action="/admin/mod-center"><input type="hidden" name="action" value="save_case">` + a.csrfHiddenInput(r) + `<label>Case ID <input name="id" size="24" placeholder="case-... (leave blank to create)"></label><br><label>Title <input name="title" size="70" placeholder="Multi-step harassment review"></label><br><label>Status <select name="status"><option value="open">open</option><option value="in_progress">in_progress</option><option value="waiting">waiting</option><option value="resolved">resolved</option></select></label><br><label>Priority <select name="priority"><option value="normal">normal</option><option value="high">high</option><option value="urgent">urgent</option><option value="low">low</option></select></label><br><label>Owner <input name="owner" list="staffHandles" size="16"></label><br><label>Target handle <input name="target_handle" size="16" placeholder="caller"></label><br><label>Linked report ID <input name="report_id" size="8" placeholder="0"></label><br><label>Summary <input name="summary" size="82" placeholder="what happened and current decision path"></label><br><button type="submit">Save Case Thread</button></form></article></section>
<table border="1"><tr><th>ID</th><th>Title</th><th>Status</th><th>Priority</th><th>Owner</th><th>Target</th><th>Report</th><th>Updates</th><th>Updated</th><th>Actions</th></tr>` + caseTable.String() + `</table>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}
