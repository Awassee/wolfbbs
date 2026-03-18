package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/rbac"
)

const (
	sysSettingEventAttendance  = "events.attendance"
	sysSettingEventRecaps      = "events.recaps"
	sysSettingSeasonChallenges = "community.season_challenges"
	sysSettingClubhouseGoals   = "community.clubhouse.goals"
)

type eventRecap struct {
	EventID         string    `json:"event_id"`
	SeriesID        string    `json:"series_id,omitempty"`
	Title           string    `json:"title"`
	StartsAt        time.Time `json:"starts_at"`
	EndsAt          time.Time `json:"ends_at,omitempty"`
	AttendanceCount int       `json:"attendance_count"`
	Summary         string    `json:"summary"`
	Highlights      []string  `json:"highlights,omitempty"`
	UpdatedBy       string    `json:"updated_by,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type seasonChallenge struct {
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

type seasonalChallengeScore struct {
	Handle     string
	BoardPosts int
	ChatPosts  int
	DoorRuns   int
	Points     int
}

type clubhouseGoal struct {
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

type backupArtifactRow struct {
	Path      string
	Kind      string
	Status    string
	Detail    string
	SizeBytes int64
	UpdatedAt time.Time
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func parseHighlightLines(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		clean := cleanOneLiner(part, 140)
		if clean == "" {
			continue
		}
		if _, ok := seen[strings.ToLower(clean)]; ok {
			continue
		}
		seen[strings.ToLower(clean)] = struct{}{}
		out = append(out, clean)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func renderHighlightLines(rows []string) string {
	if len(rows) == 0 {
		return ""
	}
	return strings.Join(rows, "\n")
}

func (a *webApp) loadEventAttendance() map[string]map[string]time.Time {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingEventAttendance)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := map[string]map[string]string{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("events.attendance", fmt.Errorf("decode attendance: %w", err))
		return nil
	}
	out := map[string]map[string]time.Time{}
	for eventID, handles := range decoded {
		eventID = strings.TrimSpace(eventID)
		if eventID == "" || len(handles) == 0 {
			continue
		}
		attendance := map[string]time.Time{}
		for handle, atRaw := range handles {
			handle = normalizeHandleKey(handle)
			at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(atRaw))
			if handle == "" || err != nil {
				continue
			}
			attendance[handle] = at.UTC()
		}
		if len(attendance) > 0 {
			out[eventID] = attendance
		}
	}
	return out
}

func (a *webApp) persistEventAttendance(rows map[string]map[string]time.Time) {
	if a.adminRepo == nil {
		return
	}
	encoded := map[string]map[string]string{}
	for eventID, handles := range rows {
		eventID = strings.TrimSpace(eventID)
		if eventID == "" || len(handles) == 0 {
			continue
		}
		bucket := map[string]string{}
		for handle, at := range handles {
			handle = normalizeHandleKey(handle)
			if handle == "" || at.IsZero() {
				continue
			}
			bucket[handle] = at.UTC().Format(time.RFC3339Nano)
		}
		if len(bucket) > 0 {
			encoded[eventID] = bucket
		}
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("events.attendance", fmt.Errorf("encode attendance: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingEventAttendance, body)
}

func (a *webApp) setEventAttendance(eventID, handle string, attended bool) {
	eventID = strings.TrimSpace(eventID)
	handle = normalizeHandleKey(handle)
	if eventID == "" || handle == "" {
		return
	}
	rows := a.loadEventAttendance()
	if rows == nil {
		rows = map[string]map[string]time.Time{}
	}
	if rows[eventID] == nil {
		rows[eventID] = map[string]time.Time{}
	}
	if attended {
		rows[eventID][handle] = time.Now().UTC()
	} else {
		delete(rows[eventID], handle)
		if len(rows[eventID]) == 0 {
			delete(rows, eventID)
		}
	}
	a.persistEventAttendance(rows)
}

func (a *webApp) attendanceCounts() map[string]int {
	rows := a.loadEventAttendance()
	if len(rows) == 0 {
		return nil
	}
	out := map[string]int{}
	for eventID, bucket := range rows {
		eventID = strings.TrimSpace(eventID)
		if eventID == "" {
			continue
		}
		if len(bucket) > 0 {
			out[eventID] = len(bucket)
		}
	}
	return out
}

func (a *webApp) eventCheckedIn(eventID, handle string) bool {
	eventID = strings.TrimSpace(eventID)
	handle = normalizeHandleKey(handle)
	if eventID == "" || handle == "" {
		return false
	}
	rows := a.loadEventAttendance()
	if len(rows) == 0 {
		return false
	}
	bucket := rows[eventID]
	if len(bucket) == 0 {
		return false
	}
	_, ok := bucket[handle]
	return ok
}

func eventCheckInWindow(row communityEvent, now time.Time) bool {
	if row.StartsAt.IsZero() {
		return false
	}
	start := row.StartsAt.Add(-2 * time.Hour)
	end := row.EndsAt
	if end.IsZero() || !end.After(row.StartsAt) {
		end = row.StartsAt.Add(6 * time.Hour)
	}
	end = end.Add(24 * time.Hour)
	return !now.Before(start) && !now.After(end)
}

func normalizeEventRecap(row eventRecap) (eventRecap, bool) {
	row.EventID = strings.TrimSpace(row.EventID)
	row.SeriesID = strings.TrimSpace(row.SeriesID)
	row.Title = cleanOneLiner(row.Title, 120)
	row.Summary = cleanOneLiner(row.Summary, 320)
	row.UpdatedBy = normalizeHandleKey(row.UpdatedBy)
	row.Highlights = parseHighlightLines(strings.Join(row.Highlights, "\n"))
	if row.EventID == "" || row.Title == "" || row.StartsAt.IsZero() {
		return eventRecap{}, false
	}
	if row.AttendanceCount < 0 {
		row.AttendanceCount = 0
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	return row, true
}

func (a *webApp) loadEventRecaps() map[string]eventRecap {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingEventRecaps)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	rows := []eventRecap{}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		fallback := map[string]eventRecap{}
		if mapErr := json.Unmarshal([]byte(raw), &fallback); mapErr != nil {
			a.addAppError("events.recaps", fmt.Errorf("decode recaps: %w", err))
			return nil
		}
		rows = make([]eventRecap, 0, len(fallback))
		for _, row := range fallback {
			rows = append(rows, row)
		}
	}
	out := map[string]eventRecap{}
	for _, row := range rows {
		normalized, ok := normalizeEventRecap(row)
		if !ok {
			continue
		}
		out[normalized.EventID] = normalized
	}
	return out
}

func (a *webApp) persistEventRecaps(rows map[string]eventRecap) {
	if a.adminRepo == nil {
		return
	}
	clean := make([]eventRecap, 0, len(rows))
	for _, row := range rows {
		normalized, ok := normalizeEventRecap(row)
		if !ok {
			continue
		}
		clean = append(clean, normalized)
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i].StartsAt.Equal(clean[j].StartsAt) {
			return strings.ToLower(clean[i].Title) < strings.ToLower(clean[j].Title)
		}
		return clean[i].StartsAt.After(clean[j].StartsAt)
	})
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			a.addAppError("events.recaps", fmt.Errorf("encode recaps: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingEventRecaps, body)
}

func (a *webApp) upsertEventRecap(row eventRecap) {
	normalized, ok := normalizeEventRecap(row)
	if !ok {
		return
	}
	rows := a.loadEventRecaps()
	if rows == nil {
		rows = map[string]eventRecap{}
	}
	rows[normalized.EventID] = normalized
	a.persistEventRecaps(rows)
}

func (a *webApp) recapsAsList(limit int) []eventRecap {
	rows := a.loadEventRecaps()
	if len(rows) == 0 {
		return nil
	}
	out := make([]eventRecap, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartsAt.Equal(out[j].StartsAt) {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		}
		return out[i].StartsAt.After(out[j].StartsAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (a *webApp) handleEventRecaps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, _ := a.currentUser(r)
	nav := `<a href="/connect">connect</a> | <a href="/tour">tour</a> | <a href="/events">events</a> | <a href="/login">login</a> | <a href="/help">help</a>`
	if user != nil {
		nav = `<a href="/start">start</a> | <a href="/events">events</a> | <a href="/events/recaps">recaps</a> | <a href="/today">today</a> | <a href="/help">help</a> | <a href="/logout">logout</a>`
		if a.hasRole(user, roleAdmin) {
			nav = `<a href="/start">start</a> | <a href="/events">events</a> | <a href="/events/recaps">recaps</a> | <a href="/admin/events">admin events</a> | <a href="/logout">logout</a>`
		}
	}
	recaps := a.recapsAsList(36)
	rows := strings.Builder{}
	for _, row := range recaps {
		highlights := strings.Builder{}
		for _, item := range row.Highlights {
			highlights.WriteString(`<li>` + htmlEscape(item) + `</li>`)
		}
		if highlights.Len() == 0 {
			highlights.WriteString(`<li>No highlights published yet.</li>`)
		}
		rows.WriteString(`<article class="wolfbbs-card"><h2>` + htmlEscape(row.Title) + `</h2><p class="wolfbbs-muted">` + htmlEscape(formatCommunityEventWindow(communityEvent{StartsAt: row.StartsAt, EndsAt: row.EndsAt})) + ` | attendance ` + strconv.Itoa(row.AttendanceCount) + ` | recap by ` + htmlEscape(defaultIfBlank(row.UpdatedBy, "staff")) + `</p><p>` + htmlEscape(defaultIfBlank(row.Summary, "No recap summary yet.")) + `</p><ul>` + highlights.String() + `</ul></article>`)
	}
	if rows.Len() == 0 {
		rows.WriteString(`<article class="wolfbbs-card"><p>No event recaps published yet. Use <a href="/admin/events">Events Admin</a> after each event to close the loop.</p></article>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Event Recaps</title></head><body>
<p>` + nav + `</p>
` + pageMessageBlock(r) + `
<h1>Event Recaps</h1>
<p>Attendance + outcome history so callers can see what happened and sysops can identify which events actually worked.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(recaps)) + `</strong><span>published recaps</span></article></section>
<div class="wolfbbs-grid">` + rows.String() + `</div>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func normalizeSeasonChallenge(row seasonChallenge) (seasonChallenge, bool) {
	row.ID = strings.TrimSpace(row.ID)
	row.Name = cleanOneLiner(row.Name, 96)
	row.Theme = cleanOneLiner(row.Theme, 96)
	row.Description = cleanOneLiner(row.Description, 260)
	row.UpdatedBy = normalizeHandleKey(row.UpdatedBy)
	if row.Name == "" {
		return seasonChallenge{}, false
	}
	if row.ID == "" {
		row.ID = randomEventID()
	}
	if row.StartsAt.IsZero() {
		row.StartsAt = time.Now().UTC().Add(-24 * time.Hour)
	}
	if row.EndsAt.IsZero() || !row.EndsAt.After(row.StartsAt) {
		row.EndsAt = row.StartsAt.Add(30 * 24 * time.Hour)
	}
	row.BoardWeight = clampInt(row.BoardWeight, 1, 20)
	row.ChatWeight = clampInt(row.ChatWeight, 1, 20)
	row.DoorWeight = clampInt(row.DoorWeight, 1, 20)
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	return row, true
}

func (a *webApp) loadSeasonChallenges() []seasonChallenge {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingSeasonChallenges)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := []seasonChallenge{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("season.challenges", fmt.Errorf("decode seasonal challenges: %w", err))
		return nil
	}
	out := make([]seasonChallenge, 0, len(decoded))
	for _, row := range decoded {
		normalized, ok := normalizeSeasonChallenge(row)
		if ok {
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

func (a *webApp) persistSeasonChallenges(rows []seasonChallenge) {
	if a.adminRepo == nil {
		return
	}
	clean := make([]seasonChallenge, 0, len(rows))
	for _, row := range rows {
		normalized, ok := normalizeSeasonChallenge(row)
		if ok {
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
			a.addAppError("season.challenges", fmt.Errorf("encode seasonal challenges: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingSeasonChallenges, body)
}

func (a *webApp) activeSeasonChallenge(now time.Time) (seasonChallenge, bool) {
	rows := a.loadSeasonChallenges()
	for _, row := range rows {
		if !row.Active {
			continue
		}
		if now.Before(row.StartsAt) || now.After(row.EndsAt) {
			continue
		}
		return row, true
	}
	for _, row := range rows {
		if row.Active {
			return row, true
		}
	}
	if len(rows) > 0 {
		return rows[0], true
	}
	return seasonChallenge{}, false
}

func (a *webApp) buildSeasonChallengeScores(challenge seasonChallenge, limit int) []seasonalChallengeScore {
	byHandle := map[string]*seasonalChallengeScore{}
	handleOrder := map[string]string{}
	idToHandle := map[int64]string{}
	if a.authSvc != nil {
		if users, err := a.authSvc.ListUsers(); err == nil {
			for _, row := range users {
				handle := strings.TrimSpace(row.Handle)
				if handle == "" {
					continue
				}
				key := normalizeHandleKey(handle)
				handleOrder[key] = handle
				idToHandle[row.ID] = handle
			}
		}
	}
	ensureRow := func(handle string) *seasonalChallengeScore {
		key := normalizeHandleKey(handle)
		if key == "" {
			return nil
		}
		canonical := handleOrder[key]
		if canonical == "" {
			canonical = handle
			handleOrder[key] = canonical
		}
		if byHandle[key] == nil {
			byHandle[key] = &seasonalChallengeScore{Handle: canonical}
		}
		return byHandle[key]
	}
	inWindow := func(at time.Time) bool {
		if at.IsZero() {
			return false
		}
		at = at.UTC()
		if at.Before(challenge.StartsAt.UTC()) {
			return false
		}
		if at.After(challenge.EndsAt.UTC()) {
			return false
		}
		return true
	}

	if a.boardRepo != nil && a.msgRepo != nil {
		if boards, err := a.boardRepo.List(); err == nil {
			for _, board := range boards {
				rows, err := a.msgRepo.ListByBoard(board.ID)
				if err != nil {
					continue
				}
				for _, msg := range rows {
					if !inWindow(msg.CreatedAt) {
						continue
					}
					handle := idToHandle[msg.AuthorID]
					if handle == "" {
						continue
					}
					if row := ensureRow(handle); row != nil {
						row.BoardPosts++
					}
				}
			}
		}
	}

	if a.chatSvc != nil {
		channels := a.chatSvc.ListChannels()
		for _, channel := range channels {
			for _, msg := range a.chatSvc.History(channel, 5000) {
				if !inWindow(msg.CreatedAt) {
					continue
				}
				if row := ensureRow(msg.From); row != nil {
					row.ChatPosts++
				}
			}
		}
	}

	if a.doorRegistry != nil {
		for _, door := range a.doorRegistry.Doors() {
			events, err := a.doorRegistry.ListEvents(door.ID, 0, 5000)
			if err != nil {
				continue
			}
			for _, event := range events {
				if !inWindow(event.CreatedAt) {
					continue
				}
				eventType := strings.ToLower(strings.TrimSpace(event.EventType))
				switch eventType {
				case "start", "connector_launch", "poll_vote", "poll_create":
				default:
					continue
				}
				handle := idToHandle[event.UserID]
				if handle == "" {
					continue
				}
				if row := ensureRow(handle); row != nil {
					row.DoorRuns++
				}
			}
		}
	}

	out := make([]seasonalChallengeScore, 0, len(byHandle))
	for _, row := range byHandle {
		row.Points = row.BoardPosts*challenge.BoardWeight + row.ChatPosts*challenge.ChatWeight + row.DoorRuns*challenge.DoorWeight
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Points != out[j].Points {
			return out[i].Points > out[j].Points
		}
		if out[i].DoorRuns != out[j].DoorRuns {
			return out[i].DoorRuns > out[j].DoorRuns
		}
		if out[i].BoardPosts != out[j].BoardPosts {
			return out[i].BoardPosts > out[j].BoardPosts
		}
		return strings.ToLower(out[i].Handle) < strings.ToLower(out[j].Handle)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (a *webApp) handleChallenges(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	now := time.Now().UTC()
	challenge, exists := a.activeSeasonChallenge(now)
	scoreRows := []seasonalChallengeScore{}
	if exists {
		scoreRows = a.buildSeasonChallengeScores(challenge, 24)
	}
	myRow := seasonalChallengeScore{Handle: user.Handle}
	for _, row := range scoreRows {
		if strings.EqualFold(row.Handle, user.Handle) {
			myRow = row
			break
		}
	}
	leaderboardRows := strings.Builder{}
	for idx, row := range scoreRows {
		leaderboardRows.WriteString(`<tr><td>` + strconv.Itoa(idx+1) + `</td><td>` + htmlEscape(row.Handle) + `</td><td>` + strconv.Itoa(row.BoardPosts) + `</td><td>` + strconv.Itoa(row.ChatPosts) + `</td><td>` + strconv.Itoa(row.DoorRuns) + `</td><td>` + strconv.Itoa(row.Points) + `</td></tr>`)
	}
	if leaderboardRows.Len() == 0 {
		leaderboardRows.WriteString(`<tr><td colspan="6">No challenge activity yet.</td></tr>`)
	}
	challengeCard := `<article class="wolfbbs-card"><h2>No Active Challenge</h2><p>Seasonal challenge is not configured yet.</p><p><a href="/admin/challenges">Create one in Admin</a></p></article>`
	if exists {
		challengeCard = `<article class="wolfbbs-card"><h2>` + htmlEscape(challenge.Name) + `</h2><p class="wolfbbs-muted">` + htmlEscape(defaultIfBlank(challenge.Theme, "seasonal challenge")) + ` | ` + challenge.StartsAt.Local().Format("2006-01-02") + ` to ` + challenge.EndsAt.Local().Format("2006-01-02") + `</p><p>` + htmlEscape(defaultIfBlank(challenge.Description, "Compete across boards, chat, and doors.")) + `</p><p><strong>Scoring:</strong> boards x` + strconv.Itoa(challenge.BoardWeight) + `, chat x` + strconv.Itoa(challenge.ChatWeight) + `, doors x` + strconv.Itoa(challenge.DoorWeight) + `</p></article>`
	}
	adminLink := ""
	if a.hasRole(user, roleAdmin) {
		adminLink = ` | <a href="/admin/challenges">admin</a>`
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Seasonal Challenges</title></head><body>
<p><a href="/start">start</a> | <a href="/clubhouse">clubhouse</a> | <a href="/boards">boards</a> | <a href="/chat">chat</a> | <a href="/doors">doors</a> | <a href="/scores">scores</a> | <a href="/help">help</a> | <a href="/logout">logout</a>` + adminLink + `</p>
` + pageMessageBlock(r) + `
<h1>Seasonal Challenges</h1>
<p>Cross-surface return loop that rewards activity in boards, lobby chat, and doors during a defined season window.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(myRow.Points) + `</strong><span>your challenge points</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(myRow.BoardPosts) + `</strong><span>your board posts</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(myRow.ChatPosts) + `</strong><span>your chat posts</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(myRow.DoorRuns) + `</strong><span>your door runs</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(scoreRows)) + `</strong><span>scored callers</span></article></section>
<section class="wolfbbs-grid">` + challengeCard + `<article class="wolfbbs-card"><h2>How To Climb</h2><ul><li>Reply on boards with meaningful posts.</li><li>Participate in lobby chat conversations.</li><li>Run doors and complete in-door actions.</li><li>Coordinate through <a href="/clubhouse">Clubhouse goals</a> for team momentum.</li></ul></article></section>
<h2>Leaderboard</h2>
<table border="1"><tr><th>Rank</th><th>Caller</th><th>Boards</th><th>Chat</th><th>Doors</th><th>Points</th></tr>` + leaderboardRows.String() + `</table>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func normalizeClubhouseGoal(row clubhouseGoal) (clubhouseGoal, bool) {
	row.ID = strings.TrimSpace(row.ID)
	row.Title = cleanOneLiner(row.Title, 120)
	row.Description = cleanOneLiner(row.Description, 220)
	row.DoorID = strings.ToLower(strings.TrimSpace(row.DoorID))
	row.UpdatedBy = normalizeHandleKey(row.UpdatedBy)
	if row.Title == "" {
		return clubhouseGoal{}, false
	}
	if row.ID == "" {
		row.ID = randomEventID()
	}
	if row.Target <= 0 {
		row.Target = 50
	}
	row.Target = clampInt(row.Target, 1, 200000)
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

func (a *webApp) loadClubhouseGoals() []clubhouseGoal {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingClubhouseGoals)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := []clubhouseGoal{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("clubhouse.goals", fmt.Errorf("decode goals: %w", err))
		return nil
	}
	out := make([]clubhouseGoal, 0, len(decoded))
	for _, row := range decoded {
		normalized, ok := normalizeClubhouseGoal(row)
		if ok {
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

func (a *webApp) persistClubhouseGoals(rows []clubhouseGoal) {
	if a.adminRepo == nil {
		return
	}
	clean := make([]clubhouseGoal, 0, len(rows))
	for _, row := range rows {
		normalized, ok := normalizeClubhouseGoal(row)
		if ok {
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
			a.addAppError("clubhouse.goals", fmt.Errorf("encode goals: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingClubhouseGoals, body)
}

func (a *webApp) addGoalContribution(goalID, actor string, delta int) bool {
	goalID = strings.TrimSpace(goalID)
	actor = normalizeHandleKey(actor)
	delta = clampInt(delta, 1, 50)
	if goalID == "" {
		return false
	}
	rows := a.loadClubhouseGoals()
	updated := false
	for idx := range rows {
		if rows[idx].ID != goalID {
			continue
		}
		rows[idx].Progress += delta
		if rows[idx].Progress > rows[idx].Target {
			rows[idx].Progress = rows[idx].Target
		}
		rows[idx].UpdatedAt = time.Now().UTC()
		if actor != "" {
			rows[idx].UpdatedBy = actor
		}
		updated = true
		break
	}
	if updated {
		a.persistClubhouseGoals(rows)
	}
	return updated
}

func (a *webApp) boardNameByID(boardID int64) string {
	if boardID <= 0 || a.boardRepo == nil {
		return ""
	}
	row, err := a.boardRepo.Get(boardID)
	if err != nil || row == nil {
		return ""
	}
	return strings.TrimSpace(row.Name)
}

func (a *webApp) doorNameByID(doorID string) string {
	doorID = strings.TrimSpace(doorID)
	if doorID == "" || a.doorRegistry == nil {
		return ""
	}
	row, ok := a.doorRegistry.DoorByID(doorID)
	if !ok {
		return ""
	}
	return strings.TrimSpace(row.Name)
}

func (a *webApp) handleAdminChallenges(w http.ResponseWriter, r *http.Request) {
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
		case "save_challenge":
			challengeID := strings.TrimSpace(r.FormValue("id"))
			startsAt, err := parseLocalDateTime(r.FormValue("starts_at"))
			if err != nil {
				redirectWithError(w, r, "/admin/challenges", "Challenge start must be a valid local date/time.")
				return
			}
			endsAt, err := parseLocalDateTime(r.FormValue("ends_at"))
			if err != nil || !endsAt.After(startsAt) {
				redirectWithError(w, r, "/admin/challenges", "Challenge end must be after the start time.")
				return
			}
			challenge := seasonChallenge{
				ID:          challengeID,
				Name:        strings.TrimSpace(r.FormValue("name")),
				Theme:       strings.TrimSpace(r.FormValue("theme")),
				Description: strings.TrimSpace(r.FormValue("description")),
				StartsAt:    startsAt.UTC(),
				EndsAt:      endsAt.UTC(),
				BoardWeight: parseIntWithFallback(r.FormValue("board_weight"), 3),
				ChatWeight:  parseIntWithFallback(r.FormValue("chat_weight"), 1),
				DoorWeight:  parseIntWithFallback(r.FormValue("door_weight"), 2),
				Active:      formHasValue(r, "active"),
				UpdatedBy:   user.Handle,
				UpdatedAt:   time.Now().UTC(),
			}
			normalized, valid := normalizeSeasonChallenge(challenge)
			if !valid {
				redirectWithError(w, r, "/admin/challenges", "Challenge name is required.")
				return
			}
			rows := a.loadSeasonChallenges()
			updated := false
			for idx := range rows {
				if rows[idx].ID == normalized.ID {
					rows[idx] = normalized
					updated = true
					break
				}
			}
			if !updated {
				rows = append(rows, normalized)
			}
			a.persistSeasonChallenges(rows)
			a.recordAdminAction(user.Handle, "seasonal_challenges", "save_challenge", fmt.Sprintf("id=%s name=%s active=%t", normalized.ID, normalized.Name, normalized.Active))
			redirectWithNotice(w, r, "/admin/challenges", "Seasonal challenge saved.")
			return
		case "delete_challenge":
			challengeID := strings.TrimSpace(r.FormValue("id"))
			if challengeID == "" {
				redirectWithError(w, r, "/admin/challenges", "Challenge ID is required.")
				return
			}
			rows := a.loadSeasonChallenges()
			next := make([]seasonChallenge, 0, len(rows))
			deleted := ""
			for _, row := range rows {
				if row.ID == challengeID {
					deleted = row.Name
					continue
				}
				next = append(next, row)
			}
			if deleted == "" {
				redirectWithError(w, r, "/admin/challenges", "Challenge not found.")
				return
			}
			a.persistSeasonChallenges(next)
			a.recordAdminAction(user.Handle, "seasonal_challenges", "delete_challenge", fmt.Sprintf("id=%s name=%s", challengeID, deleted))
			redirectWithNotice(w, r, "/admin/challenges", "Seasonal challenge deleted.")
			return
		case "save_goal":
			goal := clubhouseGoal{
				ID:          strings.TrimSpace(r.FormValue("id")),
				Title:       strings.TrimSpace(r.FormValue("title")),
				BoardID:     int64(parseInt(r.FormValue("board_id"), 0)),
				DoorID:      strings.TrimSpace(r.FormValue("door_id")),
				Target:      parseIntWithFallback(r.FormValue("target"), 50),
				Progress:    parseIntWithFallback(r.FormValue("progress"), 0),
				Description: strings.TrimSpace(r.FormValue("description")),
				UpdatedBy:   user.Handle,
				UpdatedAt:   time.Now().UTC(),
			}
			normalized, valid := normalizeClubhouseGoal(goal)
			if !valid {
				redirectWithError(w, r, "/admin/challenges", "Goal title is required.")
				return
			}
			rows := a.loadClubhouseGoals()
			updated := false
			for idx := range rows {
				if rows[idx].ID == normalized.ID {
					rows[idx] = normalized
					updated = true
					break
				}
			}
			if !updated {
				rows = append(rows, normalized)
			}
			a.persistClubhouseGoals(rows)
			a.recordAdminAction(user.Handle, "clubhouse_goals", "save_goal", fmt.Sprintf("id=%s title=%s", normalized.ID, normalized.Title))
			redirectWithNotice(w, r, "/admin/challenges", "Clubhouse goal saved.")
			return
		case "delete_goal":
			goalID := strings.TrimSpace(r.FormValue("id"))
			if goalID == "" {
				redirectWithError(w, r, "/admin/challenges", "Goal ID is required.")
				return
			}
			rows := a.loadClubhouseGoals()
			next := make([]clubhouseGoal, 0, len(rows))
			deleted := ""
			for _, row := range rows {
				if row.ID == goalID {
					deleted = row.Title
					continue
				}
				next = append(next, row)
			}
			if deleted == "" {
				redirectWithError(w, r, "/admin/challenges", "Goal not found.")
				return
			}
			a.persistClubhouseGoals(next)
			a.recordAdminAction(user.Handle, "clubhouse_goals", "delete_goal", fmt.Sprintf("id=%s title=%s", goalID, deleted))
			redirectWithNotice(w, r, "/admin/challenges", "Clubhouse goal deleted.")
			return
		default:
			redirectWithError(w, r, "/admin/challenges", "Unsupported challenge action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	challengeRows := a.loadSeasonChallenges()
	goalRows := a.loadClubhouseGoals()
	editChallengeID := strings.TrimSpace(r.URL.Query().Get("edit_challenge"))
	editGoalID := strings.TrimSpace(r.URL.Query().Get("edit_goal"))
	challengeForm := seasonChallenge{
		StartsAt:    time.Now().UTC(),
		EndsAt:      time.Now().UTC().Add(30 * 24 * time.Hour),
		BoardWeight: 3,
		ChatWeight:  1,
		DoorWeight:  2,
		Active:      true,
	}
	goalForm := clubhouseGoal{Target: 50, Progress: 0}
	for _, row := range challengeRows {
		if row.ID == editChallengeID {
			challengeForm = row
			break
		}
	}
	for _, row := range goalRows {
		if row.ID == editGoalID {
			goalForm = row
			break
		}
	}
	challengeTable := strings.Builder{}
	csrf := a.csrfHiddenInput(r)
	for _, row := range challengeRows {
		challengeTable.WriteString(`<tr><td>` + htmlEscape(row.Name) + `</td><td>` + htmlEscape(defaultIfBlank(row.Theme, "n/a")) + `</td><td>` + htmlEscape(row.StartsAt.Local().Format("2006-01-02")) + `</td><td>` + htmlEscape(row.EndsAt.Local().Format("2006-01-02")) + `</td><td>` + strconv.Itoa(row.BoardWeight) + ` / ` + strconv.Itoa(row.ChatWeight) + ` / ` + strconv.Itoa(row.DoorWeight) + `</td><td>` + boolToText(row.Active) + `</td><td><a href="/admin/challenges?edit_challenge=` + url.QueryEscape(row.ID) + `">Edit</a> <form method="POST" action="/admin/challenges" style="display:inline"><input type="hidden" name="action" value="delete_challenge"><input type="hidden" name="id" value="` + htmlEscape(row.ID) + `">` + csrf + `<button type="submit">Delete</button></form></td></tr>`)
	}
	if challengeTable.Len() == 0 {
		challengeTable.WriteString(`<tr><td colspan="7">No seasonal challenges configured yet.</td></tr>`)
	}
	goalTable := strings.Builder{}
	for _, row := range goalRows {
		boardLabel := a.boardNameByID(row.BoardID)
		doorLabel := a.doorNameByID(row.DoorID)
		bindings := ""
		if boardLabel != "" {
			bindings += "board: " + boardLabel
		}
		if doorLabel != "" {
			if bindings != "" {
				bindings += " | "
			}
			bindings += "door: " + doorLabel
		}
		if bindings == "" {
			bindings = "unbound"
		}
		goalTable.WriteString(`<tr><td>` + htmlEscape(row.Title) + `</td><td>` + htmlEscape(bindings) + `</td><td>` + strconv.Itoa(row.Progress) + ` / ` + strconv.Itoa(row.Target) + `</td><td>` + htmlEscape(defaultIfBlank(row.UpdatedBy, "n/a")) + `</td><td><a href="/admin/challenges?edit_goal=` + url.QueryEscape(row.ID) + `">Edit</a> <form method="POST" action="/admin/challenges" style="display:inline"><input type="hidden" name="action" value="delete_goal"><input type="hidden" name="id" value="` + htmlEscape(row.ID) + `">` + csrf + `<button type="submit">Delete</button></form></td></tr>`)
	}
	if goalTable.Len() == 0 {
		goalTable.WriteString(`<tr><td colspan="5">No clubhouse goals yet.</td></tr>`)
	}
	boardOptions := strings.Builder{}
	boardOptions.WriteString(`<option value="0">(none)</option>`)
	if a.boardRepo != nil {
		if boards, err := a.boardRepo.List(); err == nil {
			sort.Slice(boards, func(i, j int) bool {
				return strings.ToLower(boards[i].Name) < strings.ToLower(boards[j].Name)
			})
			for _, board := range boards {
				selected := ""
				if board.ID == goalForm.BoardID {
					selected = ` selected`
				}
				boardOptions.WriteString(`<option value="` + strconv.FormatInt(board.ID, 10) + `"` + selected + `>` + htmlEscape(board.Name) + `</option>`)
			}
		}
	}
	doorOptions := strings.Builder{}
	doorOptions.WriteString(`<option value="">(none)</option>`)
	if a.doorRegistry != nil {
		rows := a.doorRegistry.Doors()
		sort.Slice(rows, func(i, j int) bool {
			return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
		})
		for _, row := range rows {
			selected := ""
			if strings.EqualFold(row.ID, goalForm.DoorID) {
				selected = ` selected`
			}
			doorOptions.WriteString(`<option value="` + htmlEscape(row.ID) + `"` + selected + `>` + htmlEscape(row.Name) + `</option>`)
		}
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Challenges Admin</title></head><body>
<p><a href="/admin">admin</a> | <a href="/challenges">public challenge board</a> | <a href="/clubhouse">clubhouse</a> | <a href="/admin/launch">launch</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Challenges & Clubhouse Goals</h1>
<p>Control seasonal cross-surface scoring and shared goals that tie boards and doors together.</p>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Season Challenge Editor</h2><form method="POST" action="/admin/challenges"><input type="hidden" name="action" value="save_challenge"><input type="hidden" name="id" value="` + htmlEscape(challengeForm.ID) + `">` + csrf + `<label>Name <input name="name" value="` + htmlEscape(challengeForm.Name) + `" size="42" placeholder="Spring Return Ladder"></label><br><label>Theme <input name="theme" value="` + htmlEscape(challengeForm.Theme) + `" size="42" placeholder="Retro Sprint"></label><br><label>Starts <input type="datetime-local" name="starts_at" value="` + htmlEscape(formatLocalDateTimeValue(challengeForm.StartsAt)) + `"></label><br><label>Ends <input type="datetime-local" name="ends_at" value="` + htmlEscape(formatLocalDateTimeValue(challengeForm.EndsAt)) + `"></label><br><label>Boards weight <input name="board_weight" value="` + strconv.Itoa(challengeForm.BoardWeight) + `" inputmode="numeric"></label><br><label>Chat weight <input name="chat_weight" value="` + strconv.Itoa(challengeForm.ChatWeight) + `" inputmode="numeric"></label><br><label>Doors weight <input name="door_weight" value="` + strconv.Itoa(challengeForm.DoorWeight) + `" inputmode="numeric"></label><br><label>Description <input name="description" value="` + htmlEscape(challengeForm.Description) + `" size="72" placeholder="How callers score and why it matters"></label><br><label><input type="checkbox" name="active" value="1"` + checkedIf(challengeForm.Active) + `> Active</label><br><button type="submit">Save Challenge</button></form></article><article class="wolfbbs-card"><h2>Shared Goal Editor</h2><form method="POST" action="/admin/challenges"><input type="hidden" name="action" value="save_goal"><input type="hidden" name="id" value="` + htmlEscape(goalForm.ID) + `">` + csrf + `<label>Title <input name="title" value="` + htmlEscape(goalForm.Title) + `" size="42" placeholder="100 replies on Tournament board"></label><br><label>Board <select name="board_id">` + boardOptions.String() + `</select></label><br><label>Door <select name="door_id">` + doorOptions.String() + `</select></label><br><label>Target <input name="target" value="` + strconv.Itoa(goalForm.Target) + `" inputmode="numeric"></label><br><label>Progress <input name="progress" value="` + strconv.Itoa(goalForm.Progress) + `" inputmode="numeric"></label><br><label>Description <input name="description" value="` + htmlEscape(goalForm.Description) + `" size="72" placeholder="What the community should do"></label><br><button type="submit">Save Goal</button></form></article></section>
<h2>Season Challenges</h2>
<table border="1"><tr><th>Name</th><th>Theme</th><th>Start</th><th>End</th><th>Weights (B/C/D)</th><th>Active</th><th>Action</th></tr>` + challengeTable.String() + `</table>
<h2>Shared Goals</h2>
<table border="1"><tr><th>Goal</th><th>Bindings</th><th>Progress</th><th>Updated By</th><th>Action</th></tr>` + goalTable.String() + `</table>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func installPrefixPath() string {
	prefix := strings.TrimSpace(os.Getenv("WOLFBBS_INSTALL_PREFIX"))
	if prefix != "" {
		return filepath.Clean(prefix)
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Clean(".wolfbbs")
	}
	return filepath.Join(home, ".local", "share", "wolfbbs")
}

func pathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func (a *webApp) countMenuBackups(limit int) int {
	root := a.menuRootPath()
	count := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".bak") {
			count++
			if limit > 0 && count >= limit {
				return filepath.SkipDir
			}
		}
		return nil
	})
	return count
}

func loadFileSnippet(path string, maxBytes int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if maxBytes <= 0 {
		maxBytes = 8192
	}
	buf := make([]byte, maxBytes)
	n, _ := f.Read(buf)
	if n <= 0 {
		return ""
	}
	return string(buf[:n])
}

func (a *webApp) buildUpgradeSafetySnapshot(user *domain.User) statusSnapshot {
	prefix := installPrefixPath()
	snapshotPath := filepath.Join(prefix, "SERVICE_STATUS.txt")
	firstStepsPath := filepath.Join(prefix, "FIRST_STEPS.txt")
	envPath := filepath.Join(prefix, ".env")
	installLogPath := filepath.Join(prefix, "install.log")
	composePath := filepath.Join("docker-compose.yml")
	if !pathExists(composePath) {
		composePath = filepath.Join(prefix, "app", "docker-compose.yml")
	}
	snapshotRecent := false
	if info, err := os.Stat(snapshotPath); err == nil {
		snapshotRecent = time.Since(info.ModTime()) <= 72*time.Hour
	}
	menuBackupCount := a.countMenuBackups(1000)
	offlineDirExists := pathExists(a.offlineDir)
	runtimeWarnings := 0
	if user != nil {
		runtimeWarnings = a.buildStatusSnapshot(user).Summary.Warn
	}
	checks := []statusCheck{
		{Name: "Install prefix", OK: pathExists(prefix), Detail: prefix},
		{Name: "Installer env file", OK: pathExists(envPath), Detail: envPath},
		{Name: "Compose manifest", OK: pathExists(composePath), Detail: composePath},
		{Name: "First-steps brief", OK: pathExists(firstStepsPath), Detail: firstStepsPath},
		{Name: "Service snapshot", OK: pathExists(snapshotPath), Detail: snapshotPath},
		{Name: "Recent service snapshot", OK: snapshotRecent, Detail: boolToText(snapshotRecent)},
		{Name: "Installer log", OK: pathExists(installLogPath), Detail: installLogPath},
		{Name: "Menu backup history", OK: menuBackupCount > 0, Detail: strconv.Itoa(menuBackupCount) + " backup file(s)"},
		{Name: "Offline archive path", OK: offlineDirExists, Detail: a.offlineDir},
		{Name: "Runtime warning budget", OK: runtimeWarnings <= 8, Detail: strconv.Itoa(runtimeWarnings) + " runtime warnings"},
	}
	pass := 0
	for _, row := range checks {
		if row.OK {
			pass++
		}
	}
	recommendations := make([]string, 0, 8)
	if !pathExists(snapshotPath) || !snapshotRecent {
		recommendations = append(recommendations, "Run bash install.sh --status before upgrades so the latest service snapshot is captured.")
	}
	if !pathExists(envPath) {
		recommendations = append(recommendations, "Repair installer layout with bash install.sh --repair before doing version upgrades.")
	}
	if menuBackupCount == 0 {
		recommendations = append(recommendations, "Save at least one menu edit in /admin/config so rollback-ready .bak history exists.")
	}
	if runtimeWarnings > 8 {
		recommendations = append(recommendations, "Reduce runtime warnings in /status and /admin/ops before taking an upgrade window.")
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Upgrade safety baseline looks healthy. Proceed with normal upgrade cadence.")
	}
	handle := "sysop"
	role := roleAdmin
	if user != nil {
		handle = user.Handle
		role = rbac.NormalizeRole(user.Role)
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

func (a *webApp) handleAdminUpgradeSafety(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	snapshot := a.buildUpgradeSafetySnapshot(user)
	rows := strings.Builder{}
	for _, row := range snapshot.Checks {
		rows.WriteString(statusRow(row.Name, row.OK, row.Detail))
	}
	recoRows := strings.Builder{}
	for _, row := range snapshot.Recommendations {
		recoRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Upgrade Safety Dashboard</title></head><body>
<p><a href="/admin">admin</a> | <a href="/admin/launch">launch</a> | <a href="/admin/ops">ops</a> | <a href="/admin/backups">backup browser</a> | <a href="/status">status</a> | <a href="/help">help</a></p>
` + pageMessageBlock(r) + `
<h1>Upgrade Safety Dashboard</h1>
<p>Pre-upgrade trust surface shared by web admin and installer status output.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(snapshot.Summary.Pass) + `/` + strconv.Itoa(snapshot.Summary.Total) + `</strong><span>checks passing</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(snapshot.Summary.Warn) + `</strong><span>warnings</span></article><article class="wolfbbs-kpi-card"><strong>` + snapshot.GeneratedAt.Local().Format("2006-01-02 15:04") + `</strong><span>generated</span></article></section>
<h2>Safety Checks</h2>
<table border="1"><tr><th>Check</th><th>Status</th><th>Details</th></tr>` + rows.String() + `</table>
<h2>Operator Recommendations</h2><ul>` + recoRows.String() + `</ul>
<h2>Upgrade Commands</h2><pre>bash install.sh --status
bash install.sh --doctor
bash install.sh --upgrade
bash install.sh --rapid-upgrade
bash install.sh --logs</pre>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func backupKind(path, prefix, menuRoot, offlineRoot string) string {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "service_status.txt":
		return "service_snapshot"
	case "first_steps.txt":
		return "first_steps"
	case "install.log":
		return "installer_log"
	}
	lowerPath := strings.ToLower(path)
	if strings.HasSuffix(base, ".bak") {
		return "menu_backup"
	}
	if strings.HasPrefix(lowerPath, strings.ToLower(filepath.Clean(offlineRoot))+string(os.PathSeparator)) {
		if strings.HasSuffix(base, ".json") {
			return "offline_packet"
		}
		if strings.HasSuffix(base, ".txt") {
			return "offline_text"
		}
	}
	if strings.HasPrefix(lowerPath, strings.ToLower(filepath.Clean(menuRoot))+string(os.PathSeparator)) {
		return "menu_file"
	}
	if strings.HasPrefix(lowerPath, strings.ToLower(filepath.Clean(prefix))+string(os.PathSeparator)) {
		return "install_artifact"
	}
	return "artifact"
}

func validateBackupArtifact(path, kind string, info os.FileInfo) (string, string) {
	if info == nil {
		return "error", "missing metadata"
	}
	if info.Size() <= 0 {
		return "warn", "empty file"
	}
	switch kind {
	case "service_snapshot":
		snippet := loadFileSnippet(path, 8192)
		if !strings.Contains(snippet, "WolfBBS Service Status") {
			return "warn", "missing service status header"
		}
		return "ok", "service snapshot looks valid"
	case "offline_packet":
		body := loadFileSnippet(path, 1<<20)
		packet := offlinePacket{}
		if err := json.Unmarshal([]byte(body), &packet); err != nil {
			return "warn", "offline packet JSON failed validation"
		}
		if strings.TrimSpace(packet.Handle) == "" {
			return "warn", "offline packet missing handle"
		}
		return "ok", fmt.Sprintf("offline packet for %s (%d boards)", packet.Handle, len(packet.Boards))
	case "menu_backup":
		return "ok", "menu backup file"
	default:
		return "ok", "artifact readable"
	}
}

func walkArtifacts(root string, limit int, include func(path string, d os.DirEntry) bool) []string {
	if strings.TrimSpace(root) == "" || !pathExists(root) {
		return nil
	}
	out := make([]string, 0, 32)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() {
			if strings.Count(filepath.Clean(path), string(os.PathSeparator)) > strings.Count(filepath.Clean(root), string(os.PathSeparator))+6 {
				return filepath.SkipDir
			}
			return nil
		}
		if include != nil && !include(path, d) {
			return nil
		}
		out = append(out, path)
		if limit > 0 && len(out) >= limit {
			return filepath.SkipDir
		}
		return nil
	})
	return out
}

func (a *webApp) listBackupArtifacts(limit int) []backupArtifactRow {
	prefix := installPrefixPath()
	menuRoot := a.menuRootPath()
	offlineRoot := a.offlineDir
	candidates := make([]string, 0, 80)
	for _, path := range []string{
		filepath.Join(prefix, "SERVICE_STATUS.txt"),
		filepath.Join(prefix, "FIRST_STEPS.txt"),
		filepath.Join(prefix, "install.log"),
	} {
		if pathExists(path) {
			candidates = append(candidates, path)
		}
	}
	candidates = append(candidates, walkArtifacts(menuRoot, 120, func(path string, d os.DirEntry) bool {
		return strings.HasSuffix(strings.ToLower(d.Name()), ".bak")
	})...)
	candidates = append(candidates, walkArtifacts(offlineRoot, 120, func(path string, d os.DirEntry) bool {
		name := strings.ToLower(d.Name())
		return strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".txt")
	})...)
	seen := map[string]struct{}{}
	out := make([]backupArtifactRow, 0, len(candidates))
	for _, path := range candidates {
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		kind := backupKind(path, prefix, menuRoot, offlineRoot)
		status, detail := validateBackupArtifact(path, kind, info)
		out = append(out, backupArtifactRow{
			Path:      path,
			Kind:      kind,
			Status:    status,
			Detail:    detail,
			SizeBytes: info.Size(),
			UpdatedAt: info.ModTime().UTC(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Path < out[j].Path
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func backupStatusClass(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ok", "pass":
		return "ok"
	case "error", "fail", "danger":
		return "danger"
	default:
		return "warn"
	}
}

func (a *webApp) handleAdminBackups(w http.ResponseWriter, r *http.Request) {
	_, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rows := a.listBackupArtifacts(240)
	summary := map[string]int{"ok": 0, "warn": 0, "error": 0}
	tableRows := strings.Builder{}
	for _, row := range rows {
		statusKey := strings.ToLower(strings.TrimSpace(row.Status))
		switch statusKey {
		case "ok":
			summary["ok"]++
		case "error":
			summary["error"]++
		default:
			summary["warn"]++
		}
		tableRows.WriteString(`<tr><td><span class="wolfbbs-status-pill ` + backupStatusClass(row.Status) + `">` + htmlEscape(strings.ToUpper(row.Status)) + `</span></td><td>` + htmlEscape(row.Kind) + `</td><td><code>` + htmlEscape(row.Path) + `</code></td><td>` + strconv.FormatInt(row.SizeBytes, 10) + `</td><td>` + row.UpdatedAt.Local().Format("2006-01-02 15:04") + `</td><td>` + htmlEscape(row.Detail) + `</td></tr>`)
	}
	if tableRows.Len() == 0 {
		tableRows.WriteString(`<tr><td colspan="6">No backup artifacts discovered yet.</td></tr>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Backup Browser</title></head><body>
<p><a href="/admin">admin</a> | <a href="/admin/upgrade-safety">upgrade safety</a> | <a href="/admin/launch">launch</a> | <a href="/help">help</a></p>
` + pageMessageBlock(r) + `
<h1>Backup Browser</h1>
<p>Validation-focused browser for install artifacts, menu backups, and offline packet exports.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(rows)) + `</strong><span>artifacts scanned</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(summary["ok"]) + `</strong><span>valid</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(summary["warn"]+summary["error"]) + `</strong><span>needs review</span></article></section>
<table border="1"><tr><th>Status</th><th>Kind</th><th>Path</th><th>Bytes</th><th>Updated</th><th>Validation</th></tr>` + tableRows.String() + `</table>
<p><strong>Tip:</strong> run <code>bash install.sh --status</code> before and after upgrades so SERVICE_STATUS snapshots stay current.</p>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}
