package sshserver

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/ui"
)

const (
	pulseSettingSeasonMissions           = "community.season_missions"
	pulseSettingSeasonMissionCompletions = "community.season_mission_completions"
	pulseSettingMentorshipPairs          = "community.mentorship.pairs"
	pulseSettingCommunityEvents          = "community.calendar.events"
	pulseSettingEventRecaps              = "events.recaps"
	pulseSettingSeasonChallenges         = "community.season_challenges"
	pulseSettingDigestPrefsRoot          = "web.digest_prefs."
	pulseSettingDigestWeekdayPrefsRoot   = "web.digest_weekday."
)

type pulseSeasonMission struct {
	ID               string    `json:"id"`
	Season           string    `json:"season"`
	Title            string    `json:"title"`
	Description      string    `json:"description,omitempty"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
	TargetBoardPosts int       `json:"target_board_posts"`
	TargetChatPosts  int       `json:"target_chat_posts"`
	TargetDoorRuns   int       `json:"target_door_runs"`
	Active           bool      `json:"active"`
}

type pulseMissionProgress struct {
	BoardPosts int
	ChatPosts  int
	DoorRuns   int
}

type pulseMissionCompletion struct {
	MissionID string    `json:"mission_id"`
	Handle    string    `json:"handle"`
	ClaimedAt time.Time `json:"claimed_at"`
}

type pulseMentorshipPair struct {
	Mentee string    `json:"mentee"`
	Mentor string    `json:"mentor"`
	Note   string    `json:"note,omitempty"`
	Active bool      `json:"active"`
	At     time.Time `json:"updated_at,omitempty"`
}

type pulseCommunityEvent struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Category string    `json:"category"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Location string    `json:"location"`
	Host     string    `json:"host"`
	Link     string    `json:"link"`
}

type pulseEventRecap struct {
	EventID         string    `json:"event_id"`
	Title           string    `json:"title"`
	StartsAt        time.Time `json:"starts_at"`
	EndsAt          time.Time `json:"ends_at,omitempty"`
	AttendanceCount int       `json:"attendance_count"`
	Summary         string    `json:"summary,omitempty"`
	UpdatedBy       string    `json:"updated_by,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

type pulseSeasonChallenge struct {
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
}

type pulseSeasonChallengeScore struct {
	Handle     string
	BoardPosts int
	ChatPosts  int
	DoorRuns   int
	Points     int
}

type pulseDigestPreferences struct {
	MaxItems int `json:"max_items"`
}

type pulseWeekdayDigestPrefs struct {
	Sunday    int `json:"sunday"`
	Monday    int `json:"monday"`
	Tuesday   int `json:"tuesday"`
	Wednesday int `json:"wednesday"`
	Thursday  int `json:"thursday"`
	Friday    int `json:"friday"`
	Saturday  int `json:"saturday"`
}

type pulseSpotlightRow struct {
	Handle        string
	CurrentStreak int
	ActiveDays14  int
	Board7        int
	Chat7         int
	Door7         int
	Score         int
}

type pulseRankRow struct {
	Handle string
	Count  int
}

func (s *Server) runCallerPulse(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, account *domain.User, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if strings.TrimSpace(handle) == "" || strings.EqualFold(handle, "guest") {
		adminPause(sess, reader, touch, "Caller Pulse requires a signed-in account.")
		return
	}
	user := account
	if (user == nil || user.ID <= 0) && s.auth != nil {
		loaded, err := s.auth.GetUser(handle)
		if err == nil && loaded != nil {
			user = loaded
		}
	}
	if user == nil || user.ID <= 0 {
		adminPause(sess, reader, touch, "Could not load current account for Caller Pulse.")
		return
	}

	now := time.Now().UTC()
	window7 := now.Add(-7 * 24 * time.Hour)
	window30 := now.Add(-30 * 24 * time.Hour)

	streakCurrent, streakLongest, active14 := s.callerStreakMetrics(user.Handle, now)
	board7 := s.countBoardPostsForUserInRange(user.ID, window7, now)
	board30 := s.countBoardPostsForUserInRange(user.ID, window30, now)
	chat7 := s.countChatPostsForHandleInRange(user.Handle, window7, now)
	chat30 := s.countChatPostsForHandleInRange(user.Handle, window30, now)
	door7 := s.countDoorLaunchesForHandleInRange(user.Handle, window7, now)
	door30 := s.countDoorLaunchesForHandleInRange(user.Handle, window30, now)
	unreadMail := s.unreadMailCountForUser(user.ID)
	unreadBoards := s.unreadBoardCountForUser(user.ID)

	actions := make([]string, 0, 8)
	if unreadMail > 0 {
		actions = append(actions, fmt.Sprintf("Open mail first: %d unread private messages.", unreadMail))
	}
	if unreadBoards > 0 {
		actions = append(actions, fmt.Sprintf("Run board catch-up: %d board(s) have unread traffic.", unreadBoards))
	}
	if streakCurrent == 0 {
		actions = append(actions, "Restart streak today with one post, one chat line, and one door run.")
	} else if streakCurrent < 3 {
		actions = append(actions, "Protect momentum: keep the streak alive today.")
	}

	missions := s.loadPulseSeasonMissions()
	completions := s.loadPulseMissionCompletionSet(user.Handle)
	activeMissions := make([]pulseSeasonMission, 0, len(missions))
	pendingMissionCount := 0
	for _, mission := range missions {
		if !mission.Active {
			continue
		}
		if now.Before(mission.StartsAt.UTC()) || now.After(mission.EndsAt.UTC()) {
			continue
		}
		activeMissions = append(activeMissions, mission)
		progress := s.missionProgressForUser(user, mission)
		done := missionProgressSatisfied(progress, mission)
		if !done && !completions[mission.ID] {
			pendingMissionCount++
		}
	}
	if pendingMissionCount > 0 {
		actions = append(actions, fmt.Sprintf("Mission lane has %d pending objective(s).", pendingMissionCount))
	}

	upcomingTournaments := s.upcomingTournamentEvents(now, 5)
	if len(upcomingTournaments) > 0 {
		actions = append(actions, fmt.Sprintf("Tournament cadence active: %d upcoming slot(s).", len(upcomingTournaments)))
	} else {
		actions = append(actions, "No tournament slots queued; schedule one to strengthen return loops.")
	}
	if len(actions) == 0 {
		actions = append(actions, "All loops are healthy. Keep daily cadence steady.")
	}

	topPosts := s.topPosterRows(5)
	topChat := s.topChatRows(5)
	topDoors := s.topDoorRows(5)
	mentor, hasMentor := s.pulseMentorshipForMentee(user.Handle)
	upcomingEvents := s.upcomingCommunityEvents(now, 5)
	recentRecaps := s.recentPulseEventRecaps(5)
	spotlights := s.pulseSpotlights(8, now)
	challenge, hasChallenge := s.activePulseSeasonChallenge(now)
	challengeScores := []pulseSeasonChallengeScore{}
	myChallengeScore := pulseSeasonChallengeScore{Handle: user.Handle}
	if hasChallenge {
		challengeScores = s.pulseSeasonChallengeScores(challenge, 5)
		myChallengeScore = s.pulseSeasonChallengeScoreForUser(challenge, user)
	}
	digestBaseMax := s.loadPulseDigestMaxItems(user.Handle)
	digestWeekday := s.loadPulseWeekdayDigestPrefs(user.Handle, digestBaseMax)

	var report strings.Builder
	report.WriteString("Caller Pulse Center\n")
	report.WriteString("Terminal parity for: /next /streaks /topx /missions /tournaments /events /events/recaps /challenges /spotlights /digest/preferences /mentorship /milestones /time-lane /resume /doors/comeback\n")
	report.WriteString(strings.Repeat("=", 96) + "\n\n")

	report.WriteString("Next Actions (/next + /resume + /doors/comeback)\n")
	for idx, row := range actions {
		report.WriteString(fmt.Sprintf("%d. %s\n", idx+1, row))
	}
	report.WriteString("\n")

	report.WriteString("Streak Snapshot (/streaks)\n")
	report.WriteString(fmt.Sprintf("- Current streak: %d day(s)\n", streakCurrent))
	report.WriteString(fmt.Sprintf("- Longest streak: %d day(s)\n", streakLongest))
	report.WriteString(fmt.Sprintf("- Active days (14d): %d / 14\n", active14))
	report.WriteString(fmt.Sprintf("- 7d activity: boards=%d chat=%d doors=%d\n", board7, chat7, door7))
	report.WriteString(fmt.Sprintf("- 30d activity: boards=%d chat=%d doors=%d\n", board30, chat30, door30))
	report.WriteString("\n")

	report.WriteString("TopX Snapshot (/topx)\n")
	writePulseRankSection(&report, "Board posts leaderboard", topPosts)
	writePulseRankSection(&report, "Chat leaderboard", topChat)
	writePulseRankSection(&report, "Door-run leaderboard", topDoors)
	report.WriteString("\n")

	report.WriteString("Season Missions (/missions)\n")
	if len(activeMissions) == 0 {
		report.WriteString("- No active missions right now.\n")
	} else {
		for _, mission := range activeMissions {
			progress := s.missionProgressForUser(user, mission)
			status := "in progress"
			if completions[mission.ID] {
				status = "claimed"
			} else if missionProgressSatisfied(progress, mission) {
				status = "ready to claim"
			}
			report.WriteString(fmt.Sprintf("- %s [%s] board %d/%d chat %d/%d doors %d/%d\n",
				clampForTTY(mission.Title, 36),
				status,
				progress.BoardPosts, mission.TargetBoardPosts,
				progress.ChatPosts, mission.TargetChatPosts,
				progress.DoorRuns, mission.TargetDoorRuns,
			))
		}
	}
	report.WriteString("\n")

	report.WriteString("Tournament Lane (/tournaments)\n")
	if len(upcomingTournaments) == 0 {
		report.WriteString("- No upcoming tournaments scheduled.\n")
	} else {
		for _, row := range upcomingTournaments {
			report.WriteString(fmt.Sprintf("- %s  %s @ %s\n",
				formatClock(row.StartsAt.UTC(), time24h),
				clampForTTY(row.Title, 34),
				clampForTTY(defaultIfBlank(row.Location, "(location tbd)"), 20),
			))
		}
	}
	report.WriteString("\n")

	report.WriteString("Event Loop (/events + /events/recaps)\n")
	if len(upcomingEvents) == 0 {
		report.WriteString("- No upcoming events scheduled.\n")
	} else {
		report.WriteString("- Upcoming:\n")
		for _, row := range upcomingEvents {
			category := defaultIfBlank(strings.TrimSpace(row.Category), "event")
			report.WriteString(fmt.Sprintf("  - %s  [%s] %s @ %s\n",
				formatClock(row.StartsAt.UTC(), time24h),
				clampForTTY(strings.ToLower(category), 10),
				clampForTTY(row.Title, 34),
				clampForTTY(defaultIfBlank(row.Location, "(location tbd)"), 20),
			))
		}
	}
	if len(recentRecaps) == 0 {
		report.WriteString("- Recaps: no published recaps yet.\n")
	} else {
		report.WriteString("- Recaps:\n")
		for _, row := range recentRecaps {
			report.WriteString(fmt.Sprintf("  - %s  attendance=%d  %s\n",
				formatClock(row.StartsAt.UTC(), time24h),
				row.AttendanceCount,
				clampForTTY(defaultIfBlank(row.Title, row.EventID), 52),
			))
		}
	}
	report.WriteString("\n")

	report.WriteString("Season Challenge (/challenges)\n")
	if !hasChallenge {
		report.WriteString("- No active seasonal challenge configured.\n")
	} else {
		report.WriteString(fmt.Sprintf("- Active challenge: %s (%s to %s)\n",
			clampForTTY(challenge.Name, 48),
			challenge.StartsAt.Local().Format("2006-01-02"),
			challenge.EndsAt.Local().Format("2006-01-02"),
		))
		report.WriteString(fmt.Sprintf("- Scoring weights: boards x%d chat x%d doors x%d\n",
			challenge.BoardWeight,
			challenge.ChatWeight,
			challenge.DoorWeight,
		))
		report.WriteString(fmt.Sprintf("- Your score: %d points (boards=%d chat=%d doors=%d)\n",
			myChallengeScore.Points,
			myChallengeScore.BoardPosts,
			myChallengeScore.ChatPosts,
			myChallengeScore.DoorRuns,
		))
		if len(challengeScores) > 0 {
			report.WriteString("- Top callers:\n")
			for idx, row := range challengeScores {
				report.WriteString(fmt.Sprintf("  %d) %-14s %d pts\n",
					idx+1,
					clampForTTY(row.Handle, 14),
					row.Points,
				))
			}
		}
	}
	report.WriteString("\n")

	report.WriteString("Returning Caller Spotlights (/spotlights)\n")
	if len(spotlights) == 0 {
		report.WriteString("- No spotlight candidates yet.\n")
	} else {
		for idx, row := range spotlights {
			report.WriteString(fmt.Sprintf("  %d) %-14s streak=%d active14=%d boards=%d chat=%d doors=%d\n",
				idx+1,
				clampForTTY(row.Handle, 14),
				row.CurrentStreak,
				row.ActiveDays14,
				row.Board7,
				row.Chat7,
				row.Door7,
			))
		}
	}
	report.WriteString("\n")

	report.WriteString("Digest Weekday Preferences (/digest/preferences)\n")
	report.WriteString(fmt.Sprintf("- Base max items: %d\n", digestBaseMax))
	report.WriteString(fmt.Sprintf("- Sun %d  Mon %d  Tue %d  Wed %d  Thu %d  Fri %d  Sat %d\n",
		digestWeekday.Sunday,
		digestWeekday.Monday,
		digestWeekday.Tuesday,
		digestWeekday.Wednesday,
		digestWeekday.Thursday,
		digestWeekday.Friday,
		digestWeekday.Saturday,
	))
	report.WriteString("- Tip: press D after this report to edit weekday caps in terminal.\n\n")

	report.WriteString("Mentorship (/mentorship)\n")
	if hasMentor {
		report.WriteString(fmt.Sprintf("- Mentor: %s\n", mentor.Mentor))
		report.WriteString(fmt.Sprintf("- Note: %s\n", defaultIfBlank(clampForTTY(mentor.Note, 90), "(none)")))
	} else {
		report.WriteString("- No active mentor pairing assigned.\n")
	}
	report.WriteString("\n")

	report.WriteString("Milestones + Time Lane (/milestones + /time-lane)\n")
	report.WriteString(fmt.Sprintf("- Streak x3: %s\n", reached(streakCurrent >= 3)))
	report.WriteString(fmt.Sprintf("- Streak x14: %s\n", reached(streakCurrent >= 14)))
	report.WriteString(fmt.Sprintf("- 30-day engagement >= 25 actions: %s\n", reached(board30+chat30+door30 >= 25)))
	report.WriteString(fmt.Sprintf("- Time-lane recommendation: %s\n", timeLaneGuidance(time.Now(), streakCurrent, board7+chat7+door7)))

	writeClear(sess, ansiEnabled)
	renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Caller Pulse", user.Handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
	pagerWrite(sess, reader, strings.TrimSpace(report.String()))
	io.WriteString(sess, "\r\nCaller Pulse command [D=Digest prefs, Enter=Return]: ")
	cmd, err := readLine(reader, 80)
	if err != nil {
		return
	}
	touch()
	if strings.EqualFold(strings.TrimSpace(cmd), "d") {
		s.runPulseDigestPreferencesEditor(sess, reader, termWidth, renderWidth, user.Handle, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		return
	}
	adminPause(sess, reader, touch, "")
}

func writePulseRankSection(out *strings.Builder, title string, rows []pulseRankRow) {
	out.WriteString("- " + title + ":\n")
	if len(rows) == 0 {
		out.WriteString("  (no data)\n")
		return
	}
	for idx, row := range rows {
		out.WriteString(fmt.Sprintf("  %d) %-14s %d\n", idx+1, clampForTTY(row.Handle, 14), row.Count))
	}
}

func (s *Server) unreadMailCountForUser(userID int64) int {
	if s.mail == nil || userID <= 0 {
		return 0
	}
	inbox, err := s.mail.ListInbox(userID, 400)
	if err != nil {
		return 0
	}
	count := 0
	for _, row := range inbox {
		if row.ReadAt == nil {
			count++
		}
	}
	return count
}

func (s *Server) unreadBoardCountForUser(userID int64) int {
	if s.boards == nil || s.msgs == nil || userID <= 0 {
		return 0
	}
	boards, err := s.boards.List()
	if err != nil {
		return 0
	}
	count := 0
	for _, board := range boards {
		msgs, err := s.msgs.ListByBoard(board.ID)
		if err != nil || len(msgs) == 0 {
			continue
		}
		latestID := int64(0)
		for _, msg := range msgs {
			if msg.ID > latestID {
				latestID = msg.ID
			}
		}
		lastRead := int64(0)
		if ptr, err := s.msgs.GetPointer(userID, board.ID); err == nil && ptr != nil {
			lastRead = ptr.LastReadID
		}
		if latestID > lastRead {
			count++
		}
	}
	return count
}

func (s *Server) countBoardPostsForUserInRange(userID int64, start, end time.Time) int {
	if s.boards == nil || s.msgs == nil || userID <= 0 {
		return 0
	}
	boards, err := s.boards.List()
	if err != nil {
		return 0
	}
	total := 0
	for _, board := range boards {
		msgs, err := s.msgs.ListByBoard(board.ID)
		if err != nil {
			continue
		}
		for _, msg := range msgs {
			if msg.AuthorID != userID {
				continue
			}
			at := msg.CreatedAt.UTC()
			if !at.Before(start.UTC()) && !at.After(end.UTC()) {
				total++
			}
		}
	}
	return total
}

func (s *Server) countChatPostsForHandleInRange(handle string, start, end time.Time) int {
	if s.chatSvc == nil {
		return 0
	}
	total := 0
	for _, channel := range s.chatSvc.ListChannels() {
		for _, msg := range s.chatSvc.History(channel, 8000) {
			if !strings.EqualFold(msg.From, handle) {
				continue
			}
			at := msg.CreatedAt.UTC()
			if !at.Before(start.UTC()) && !at.After(end.UTC()) {
				total++
			}
		}
	}
	return total
}

func (s *Server) countDoorLaunchesForHandleInRange(handle string, start, end time.Time) int {
	if s.admin == nil || strings.TrimSpace(handle) == "" {
		return 0
	}
	rows, err := s.admin.ListAudit(4000)
	if err != nil {
		return 0
	}
	total := 0
	for _, row := range rows {
		if !strings.EqualFold(strings.TrimSpace(row.Actor), strings.TrimSpace(handle)) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(row.Action), "door_launch") {
			continue
		}
		at := row.CreatedAt.UTC()
		if !at.Before(start.UTC()) && !at.After(end.UTC()) {
			total++
		}
	}
	return total
}

func (s *Server) callerStreakMetrics(handle string, now time.Time) (current int, longest int, active14 int) {
	if s.admin == nil || strings.TrimSpace(handle) == "" {
		return 0, 0, 0
	}
	rows, err := s.admin.ListCallerHistory(2400)
	if err != nil {
		return 0, 0, 0
	}
	daySet := map[string]time.Time{}
	for _, row := range rows {
		if !strings.EqualFold(strings.TrimSpace(row.Username), strings.TrimSpace(handle)) {
			continue
		}
		day := row.LoginAt.UTC().Format("2006-01-02")
		if _, exists := daySet[day]; !exists {
			daySet[day] = row.LoginAt.UTC()
		}
	}
	if len(daySet) == 0 {
		return 0, 0, 0
	}
	days := make([]time.Time, 0, len(daySet))
	for _, day := range daySet {
		days = append(days, day)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })

	start14 := dateOnly(now.AddDate(0, 0, -13))
	for _, day := range days {
		if !dateOnly(day).Before(start14) {
			active14++
		}
	}
	longest = 1
	run := 1
	for i := 1; i < len(days); i++ {
		diff := int(dateOnly(days[i]).Sub(dateOnly(days[i-1])).Hours() / 24)
		if diff == 1 {
			run++
			if run > longest {
				longest = run
			}
		} else if diff > 1 {
			run = 1
		}
	}
	current = 0
	for i := 0; i < 366; i++ {
		key := dateOnly(now.AddDate(0, 0, -i)).Format("2006-01-02")
		if _, ok := daySet[key]; !ok {
			break
		}
		current++
	}
	return current, longest, active14
}

func dateOnly(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *Server) topPosterRows(limit int) []pulseRankRow {
	if limit <= 0 {
		limit = 5
	}
	if s.boards == nil || s.msgs == nil || s.users == nil {
		return nil
	}
	users, err := s.users.List()
	if err != nil {
		return nil
	}
	handles := make(map[int64]string, len(users))
	for _, row := range users {
		handles[row.ID] = row.Handle
	}
	boards, err := s.boards.List()
	if err != nil {
		return nil
	}
	counts := map[string]int{}
	for _, board := range boards {
		msgs, err := s.msgs.ListByBoard(board.ID)
		if err != nil {
			continue
		}
		for _, msg := range msgs {
			handle := strings.TrimSpace(handles[msg.AuthorID])
			if handle == "" {
				handle = "uid:" + strconv.FormatInt(msg.AuthorID, 10)
			}
			counts[handle]++
		}
	}
	return rankFromMap(counts, limit)
}

func (s *Server) topChatRows(limit int) []pulseRankRow {
	if limit <= 0 {
		limit = 5
	}
	if s.chatSvc == nil {
		return nil
	}
	counts := map[string]int{}
	for _, channel := range s.chatSvc.ListChannels() {
		for _, msg := range s.chatSvc.History(channel, 8000) {
			handle := strings.TrimSpace(msg.From)
			if handle == "" {
				continue
			}
			counts[handle]++
		}
	}
	return rankFromMap(counts, limit)
}

func (s *Server) topDoorRows(limit int) []pulseRankRow {
	if limit <= 0 {
		limit = 5
	}
	if s.admin == nil {
		return nil
	}
	rows, err := s.admin.ListAudit(4000)
	if err != nil {
		return nil
	}
	counts := map[string]int{}
	for _, row := range rows {
		if !strings.EqualFold(strings.TrimSpace(row.Action), "door_launch") {
			continue
		}
		handle := strings.TrimSpace(row.Actor)
		if handle == "" {
			continue
		}
		counts[handle]++
	}
	return rankFromMap(counts, limit)
}

func rankFromMap(counts map[string]int, limit int) []pulseRankRow {
	if len(counts) == 0 || limit <= 0 {
		return nil
	}
	rows := make([]pulseRankRow, 0, len(counts))
	for handle, count := range counts {
		if count <= 0 {
			continue
		}
		rows = append(rows, pulseRankRow{Handle: handle, Count: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count == rows[j].Count {
			return strings.ToLower(rows[i].Handle) < strings.ToLower(rows[j].Handle)
		}
		return rows[i].Count > rows[j].Count
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func (s *Server) loadPulseSeasonMissions() []pulseSeasonMission {
	if s.admin == nil {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(pulseSettingSeasonMissions)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := []pulseSeasonMission{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	out := make([]pulseSeasonMission, 0, len(decoded))
	for _, row := range decoded {
		row.ID = strings.TrimSpace(strings.ToLower(row.ID))
		row.Title = strings.TrimSpace(row.Title)
		row.Season = strings.TrimSpace(row.Season)
		if row.ID == "" || row.Title == "" || row.StartsAt.IsZero() || row.EndsAt.IsZero() || !row.EndsAt.After(row.StartsAt) {
			continue
		}
		if row.TargetBoardPosts < 0 {
			row.TargetBoardPosts = 0
		}
		if row.TargetChatPosts < 0 {
			row.TargetChatPosts = 0
		}
		if row.TargetDoorRuns < 0 {
			row.TargetDoorRuns = 0
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartsAt.Equal(out[j].StartsAt) {
			return out[i].Title < out[j].Title
		}
		return out[i].StartsAt.After(out[j].StartsAt)
	})
	return out
}

func (s *Server) loadPulseMissionCompletionSet(handle string) map[string]bool {
	out := map[string]bool{}
	if s.admin == nil {
		return out
	}
	raw, err := s.admin.GetSystemSetting(pulseSettingSeasonMissionCompletions)
	if err != nil || strings.TrimSpace(raw) == "" {
		return out
	}
	rows := []pulseMissionCompletion{}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return out
	}
	normHandle := strings.ToLower(strings.TrimSpace(handle))
	for _, row := range rows {
		if !strings.EqualFold(strings.TrimSpace(row.Handle), normHandle) {
			continue
		}
		missionID := strings.TrimSpace(strings.ToLower(row.MissionID))
		if missionID == "" {
			continue
		}
		out[missionID] = true
	}
	return out
}

func (s *Server) missionProgressForUser(user *domain.User, mission pulseSeasonMission) pulseMissionProgress {
	if user == nil || user.ID <= 0 {
		return pulseMissionProgress{}
	}
	start := mission.StartsAt.UTC()
	end := mission.EndsAt.UTC()
	return pulseMissionProgress{
		BoardPosts: s.countBoardPostsForUserInRange(user.ID, start, end),
		ChatPosts:  s.countChatPostsForHandleInRange(user.Handle, start, end),
		DoorRuns:   s.countDoorLaunchesForHandleInRange(user.Handle, start, end),
	}
}

func missionProgressSatisfied(progress pulseMissionProgress, mission pulseSeasonMission) bool {
	if mission.TargetBoardPosts > 0 && progress.BoardPosts < mission.TargetBoardPosts {
		return false
	}
	if mission.TargetChatPosts > 0 && progress.ChatPosts < mission.TargetChatPosts {
		return false
	}
	if mission.TargetDoorRuns > 0 && progress.DoorRuns < mission.TargetDoorRuns {
		return false
	}
	return true
}

func (s *Server) loadPulseMentorshipPairs() []pulseMentorshipPair {
	if s.admin == nil {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(pulseSettingMentorshipPairs)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	rows := []pulseMentorshipPair{}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	out := make([]pulseMentorshipPair, 0, len(rows))
	for _, row := range rows {
		row.Mentee = strings.ToLower(strings.TrimSpace(row.Mentee))
		row.Mentor = strings.ToLower(strings.TrimSpace(row.Mentor))
		row.Note = strings.TrimSpace(row.Note)
		if row.Mentee == "" || row.Mentor == "" || row.Mentee == row.Mentor {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mentee == out[j].Mentee {
			return out[i].At.After(out[j].At)
		}
		return out[i].Mentee < out[j].Mentee
	})
	return out
}

func (s *Server) pulseMentorshipForMentee(handle string) (pulseMentorshipPair, bool) {
	target := strings.ToLower(strings.TrimSpace(handle))
	if target == "" {
		return pulseMentorshipPair{}, false
	}
	for _, row := range s.loadPulseMentorshipPairs() {
		if row.Active && row.Mentee == target {
			return row, true
		}
	}
	return pulseMentorshipPair{}, false
}

func (s *Server) loadPulseCommunityEvents() []pulseCommunityEvent {
	if s.admin == nil {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(pulseSettingCommunityEvents)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	rows := []pulseCommunityEvent{}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	out := make([]pulseCommunityEvent, 0, len(rows))
	for _, row := range rows {
		row.ID = strings.TrimSpace(row.ID)
		row.Title = strings.TrimSpace(row.Title)
		row.Category = strings.ToLower(strings.TrimSpace(row.Category))
		if row.ID == "" || row.Title == "" || row.StartsAt.IsZero() {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartsAt.Before(out[j].StartsAt) })
	return out
}

func (s *Server) upcomingTournamentEvents(now time.Time, limit int) []pulseCommunityEvent {
	if limit <= 0 {
		limit = 5
	}
	events := s.loadPulseCommunityEvents()
	out := make([]pulseCommunityEvent, 0, limit)
	for _, row := range events {
		if row.Category != "tournament" {
			continue
		}
		if row.StartsAt.UTC().Before(now.UTC()) {
			continue
		}
		out = append(out, row)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Server) upcomingCommunityEvents(now time.Time, limit int) []pulseCommunityEvent {
	if limit <= 0 {
		limit = 5
	}
	events := s.loadPulseCommunityEvents()
	out := make([]pulseCommunityEvent, 0, limit)
	for _, row := range events {
		if row.StartsAt.UTC().Before(now.UTC()) {
			continue
		}
		out = append(out, row)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func normalizePulseHandleKey(handle string) string {
	return strings.ToLower(strings.TrimSpace(handle))
}

func pulseWeekdayDigestSettingKey(handle string) string {
	handle = normalizePulseHandleKey(handle)
	if handle == "" {
		return ""
	}
	return pulseSettingDigestWeekdayPrefsRoot + handle
}

func pulseDigestPrefsSettingKey(handle string) string {
	handle = normalizePulseHandleKey(handle)
	if handle == "" {
		return ""
	}
	return pulseSettingDigestPrefsRoot + handle
}

func pulseClampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func defaultPulseWeekdayDigestPrefs(base int) pulseWeekdayDigestPrefs {
	base = pulseClampInt(base, 4, 40)
	return pulseWeekdayDigestPrefs{
		Sunday:    base,
		Monday:    base,
		Tuesday:   base,
		Wednesday: base,
		Thursday:  base,
		Friday:    base,
		Saturday:  base,
	}
}

func normalizePulseWeekdayDigestPrefs(row pulseWeekdayDigestPrefs, fallback int) pulseWeekdayDigestPrefs {
	def := defaultPulseWeekdayDigestPrefs(fallback)
	if row.Sunday <= 0 {
		row.Sunday = def.Sunday
	}
	if row.Monday <= 0 {
		row.Monday = def.Monday
	}
	if row.Tuesday <= 0 {
		row.Tuesday = def.Tuesday
	}
	if row.Wednesday <= 0 {
		row.Wednesday = def.Wednesday
	}
	if row.Thursday <= 0 {
		row.Thursday = def.Thursday
	}
	if row.Friday <= 0 {
		row.Friday = def.Friday
	}
	if row.Saturday <= 0 {
		row.Saturday = def.Saturday
	}
	row.Sunday = pulseClampInt(row.Sunday, 4, 40)
	row.Monday = pulseClampInt(row.Monday, 4, 40)
	row.Tuesday = pulseClampInt(row.Tuesday, 4, 40)
	row.Wednesday = pulseClampInt(row.Wednesday, 4, 40)
	row.Thursday = pulseClampInt(row.Thursday, 4, 40)
	row.Friday = pulseClampInt(row.Friday, 4, 40)
	row.Saturday = pulseClampInt(row.Saturday, 4, 40)
	return row
}

func pulseIntFromAny(v interface{}) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	case int64:
		return int(x), true
	case json.Number:
		i, err := x.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(x))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func (s *Server) loadPulseDigestMaxItems(handle string) int {
	if s.admin == nil {
		return 12
	}
	key := pulseDigestPrefsSettingKey(handle)
	if key == "" {
		return 12
	}
	raw, err := s.admin.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return 12
	}
	payload := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return 12
	}
	value, ok := pulseIntFromAny(payload["max_items"])
	if !ok {
		return 12
	}
	return pulseClampInt(value, 6, 24)
}

func (s *Server) persistPulseDigestMaxItems(handle string, maxItems int) error {
	if s.admin == nil {
		return errors.New("admin repository unavailable")
	}
	key := pulseDigestPrefsSettingKey(handle)
	if key == "" {
		return errors.New("invalid handle")
	}
	maxItems = pulseClampInt(maxItems, 6, 24)
	payload := map[string]interface{}{}
	if raw, err := s.admin.GetSystemSetting(key); err == nil && strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &payload)
	}
	payload["max_items"] = maxItems
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.admin.UpsertSystemSetting(key, string(encoded))
}

func (s *Server) loadPulseWeekdayDigestPrefs(handle string, fallback int) pulseWeekdayDigestPrefs {
	if s.admin == nil {
		return defaultPulseWeekdayDigestPrefs(fallback)
	}
	key := pulseWeekdayDigestSettingKey(handle)
	if key == "" {
		return defaultPulseWeekdayDigestPrefs(fallback)
	}
	raw, err := s.admin.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return defaultPulseWeekdayDigestPrefs(fallback)
	}
	decoded := pulseWeekdayDigestPrefs{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return defaultPulseWeekdayDigestPrefs(fallback)
	}
	return normalizePulseWeekdayDigestPrefs(decoded, fallback)
}

func (s *Server) persistPulseWeekdayDigestPrefs(handle string, row pulseWeekdayDigestPrefs, fallback int) error {
	if s.admin == nil {
		return errors.New("admin repository unavailable")
	}
	key := pulseWeekdayDigestSettingKey(handle)
	if key == "" {
		return errors.New("invalid handle")
	}
	normalized := normalizePulseWeekdayDigestPrefs(row, fallback)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	return s.admin.UpsertSystemSetting(key, string(encoded))
}

func normalizePulseWeekdayToken(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sun", "sunday":
		return "sun"
	case "mon", "monday":
		return "mon"
	case "tue", "tues", "tuesday":
		return "tue"
	case "wed", "wednesday":
		return "wed"
	case "thu", "thur", "thurs", "thursday":
		return "thu"
	case "fri", "friday":
		return "fri"
	case "sat", "saturday":
		return "sat"
	default:
		return ""
	}
}

func setPulseWeekdayValue(p *pulseWeekdayDigestPrefs, day string, value int) {
	if p == nil {
		return
	}
	switch day {
	case "sun":
		p.Sunday = value
	case "mon":
		p.Monday = value
	case "tue":
		p.Tuesday = value
	case "wed":
		p.Wednesday = value
	case "thu":
		p.Thursday = value
	case "fri":
		p.Friday = value
	case "sat":
		p.Saturday = value
	}
}

func dayLabelFromToken(token string) string {
	switch token {
	case "sun":
		return "Sunday"
	case "mon":
		return "Monday"
	case "tue":
		return "Tuesday"
	case "wed":
		return "Wednesday"
	case "thu":
		return "Thursday"
	case "fri":
		return "Friday"
	case "sat":
		return "Saturday"
	default:
		return token
	}
}

func (s *Server) runPulseDigestPreferencesEditor(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s.admin == nil {
		adminPause(sess, reader, touch, "Digest preferences are unavailable (admin repository not configured).")
		return
	}
	for {
		base := s.loadPulseDigestMaxItems(handle)
		prefs := s.loadPulseWeekdayDigestPrefs(handle, base)
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Digest Weekday Preferences", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		lines := []string{
			"Update weekday digest caps directly in SSH.",
			"",
			fmt.Sprintf("Base max items: %d (range 6-24)", base),
			fmt.Sprintf("Sun %2d  Mon %2d  Tue %2d  Wed %2d  Thu %2d  Fri %2d  Sat %2d",
				prefs.Sunday, prefs.Monday, prefs.Tuesday, prefs.Wednesday, prefs.Thursday, prefs.Friday, prefs.Saturday),
			"",
			"Commands:",
			"  <day> <value>   e.g. mon 14 (range 4-40)",
			"  BASE <value>    update global digest max items",
			"  RESET           set every day to BASE",
			"  Q               return to caller pulse",
		}
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Digest Controls", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 120)
		if err != nil {
			return
		}
		touch()
		cmd := strings.TrimSpace(raw)
		if cmd == "" {
			continue
		}
		if isBackCommand(cmd) {
			return
		}
		fields := strings.Fields(cmd)
		switch strings.ToLower(fields[0]) {
		case "base":
			if len(fields) < 2 {
				adminPause(sess, reader, touch, "Usage: BASE <value>")
				continue
			}
			value, err := strconv.Atoi(fields[1])
			if err != nil {
				adminPause(sess, reader, touch, "BASE value must be numeric.")
				continue
			}
			value = pulseClampInt(value, 6, 24)
			if err := s.persistPulseDigestMaxItems(handle, value); err != nil {
				adminPause(sess, reader, touch, "Could not save base max: "+err.Error())
				continue
			}
			recordAudit(s.admin, handle, handle, "digest_base_update", fmt.Sprintf("max_items=%d", value))
			adminPause(sess, reader, touch, fmt.Sprintf("Saved base max items = %d.", value))
		case "reset":
			base = s.loadPulseDigestMaxItems(handle)
			reset := defaultPulseWeekdayDigestPrefs(base)
			if err := s.persistPulseWeekdayDigestPrefs(handle, reset, base); err != nil {
				adminPause(sess, reader, touch, "Could not reset weekday caps: "+err.Error())
				continue
			}
			recordAudit(s.admin, handle, handle, "digest_weekday_reset", fmt.Sprintf("base=%d", base))
			adminPause(sess, reader, touch, "Weekday caps reset to base value.")
		default:
			if len(fields) < 2 {
				adminPause(sess, reader, touch, "Usage: <day> <value>  (example: tue 16)")
				continue
			}
			day := normalizePulseWeekdayToken(fields[0])
			if day == "" {
				adminPause(sess, reader, touch, "Unknown day token. Use sun/mon/tue/wed/thu/fri/sat.")
				continue
			}
			value, err := strconv.Atoi(fields[1])
			if err != nil {
				adminPause(sess, reader, touch, "Weekday value must be numeric.")
				continue
			}
			value = pulseClampInt(value, 4, 40)
			base = s.loadPulseDigestMaxItems(handle)
			updated := s.loadPulseWeekdayDigestPrefs(handle, base)
			setPulseWeekdayValue(&updated, day, value)
			if err := s.persistPulseWeekdayDigestPrefs(handle, updated, base); err != nil {
				adminPause(sess, reader, touch, "Could not save weekday cap: "+err.Error())
				continue
			}
			recordAudit(s.admin, handle, handle, "digest_weekday_update", fmt.Sprintf("%s=%d", day, value))
			adminPause(sess, reader, touch, fmt.Sprintf("Saved %s = %d.", dayLabelFromToken(day), value))
		}
	}
}

func normalizePulseEventRecap(row pulseEventRecap) (pulseEventRecap, bool) {
	row.EventID = strings.TrimSpace(row.EventID)
	row.Title = strings.TrimSpace(row.Title)
	row.Summary = strings.TrimSpace(row.Summary)
	row.UpdatedBy = normalizePulseHandleKey(row.UpdatedBy)
	if row.EventID == "" || row.Title == "" || row.StartsAt.IsZero() {
		return pulseEventRecap{}, false
	}
	if row.AttendanceCount < 0 {
		row.AttendanceCount = 0
	}
	return row, true
}

func (s *Server) recentPulseEventRecaps(limit int) []pulseEventRecap {
	if limit <= 0 {
		limit = 5
	}
	if s.admin == nil {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(pulseSettingEventRecaps)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	rows := []pulseEventRecap{}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		fallback := map[string]pulseEventRecap{}
		if mapErr := json.Unmarshal([]byte(raw), &fallback); mapErr != nil {
			return nil
		}
		rows = make([]pulseEventRecap, 0, len(fallback))
		for _, row := range fallback {
			rows = append(rows, row)
		}
	}
	out := make([]pulseEventRecap, 0, len(rows))
	for _, row := range rows {
		normalized, ok := normalizePulseEventRecap(row)
		if !ok {
			continue
		}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartsAt.Equal(out[j].StartsAt) {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		}
		return out[i].StartsAt.After(out[j].StartsAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func normalizePulseSeasonChallenge(row pulseSeasonChallenge) (pulseSeasonChallenge, bool) {
	row.ID = strings.TrimSpace(row.ID)
	row.Name = strings.TrimSpace(row.Name)
	row.Theme = strings.TrimSpace(row.Theme)
	row.Description = strings.TrimSpace(row.Description)
	if row.Name == "" {
		return pulseSeasonChallenge{}, false
	}
	if row.ID == "" {
		row.ID = row.Name
	}
	if row.StartsAt.IsZero() {
		row.StartsAt = time.Now().UTC().Add(-24 * time.Hour)
	}
	if row.EndsAt.IsZero() || !row.EndsAt.After(row.StartsAt) {
		row.EndsAt = row.StartsAt.Add(30 * 24 * time.Hour)
	}
	row.BoardWeight = pulseClampInt(row.BoardWeight, 1, 20)
	row.ChatWeight = pulseClampInt(row.ChatWeight, 1, 20)
	row.DoorWeight = pulseClampInt(row.DoorWeight, 1, 20)
	return row, true
}

func (s *Server) loadPulseSeasonChallenges() []pulseSeasonChallenge {
	if s.admin == nil {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(pulseSettingSeasonChallenges)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := []pulseSeasonChallenge{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	out := make([]pulseSeasonChallenge, 0, len(decoded))
	for _, row := range decoded {
		normalized, ok := normalizePulseSeasonChallenge(row)
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

func (s *Server) activePulseSeasonChallenge(now time.Time) (pulseSeasonChallenge, bool) {
	rows := s.loadPulseSeasonChallenges()
	for _, row := range rows {
		if !row.Active {
			continue
		}
		if now.Before(row.StartsAt.UTC()) || now.After(row.EndsAt.UTC()) {
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
	return pulseSeasonChallenge{}, false
}

func (s *Server) pulseSeasonChallengeScoreForUser(challenge pulseSeasonChallenge, user *domain.User) pulseSeasonChallengeScore {
	row := pulseSeasonChallengeScore{}
	if user == nil {
		return row
	}
	row.Handle = user.Handle
	start := challenge.StartsAt.UTC()
	end := challenge.EndsAt.UTC()
	if user.ID > 0 {
		row.BoardPosts = s.countBoardPostsForUserInRange(user.ID, start, end)
	}
	row.ChatPosts = s.countChatPostsForHandleInRange(user.Handle, start, end)
	row.DoorRuns = s.countDoorLaunchesForHandleInRange(user.Handle, start, end)
	row.Points = row.BoardPosts*challenge.BoardWeight + row.ChatPosts*challenge.ChatWeight + row.DoorRuns*challenge.DoorWeight
	return row
}

func (s *Server) pulseSeasonChallengeScores(challenge pulseSeasonChallenge, limit int) []pulseSeasonChallengeScore {
	if limit <= 0 {
		limit = 5
	}
	if s.users == nil {
		return nil
	}
	users, err := s.users.List()
	if err != nil {
		return nil
	}
	rows := make([]pulseSeasonChallengeScore, 0, len(users))
	for _, user := range users {
		handle := strings.TrimSpace(user.Handle)
		if handle == "" || strings.EqualFold(handle, "guest") {
			continue
		}
		score := s.pulseSeasonChallengeScoreForUser(challenge, &user)
		if score.Points <= 0 && score.BoardPosts == 0 && score.ChatPosts == 0 && score.DoorRuns == 0 {
			continue
		}
		rows = append(rows, score)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Points != rows[j].Points {
			return rows[i].Points > rows[j].Points
		}
		if rows[i].DoorRuns != rows[j].DoorRuns {
			return rows[i].DoorRuns > rows[j].DoorRuns
		}
		if rows[i].BoardPosts != rows[j].BoardPosts {
			return rows[i].BoardPosts > rows[j].BoardPosts
		}
		return strings.ToLower(rows[i].Handle) < strings.ToLower(rows[j].Handle)
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func (s *Server) pulseSpotlights(limit int, now time.Time) []pulseSpotlightRow {
	if limit <= 0 {
		limit = 8
	}
	if s.users == nil {
		return nil
	}
	users, err := s.users.List()
	if err != nil {
		return nil
	}
	window7 := now.Add(-7 * 24 * time.Hour)
	rows := make([]pulseSpotlightRow, 0, len(users))
	for _, user := range users {
		handle := strings.TrimSpace(user.Handle)
		if handle == "" || strings.EqualFold(handle, "guest") {
			continue
		}
		current, _, active14 := s.callerStreakMetrics(handle, now)
		board7 := s.countBoardPostsForUserInRange(user.ID, window7, now)
		chat7 := s.countChatPostsForHandleInRange(handle, window7, now)
		door7 := s.countDoorLaunchesForHandleInRange(handle, window7, now)
		if current == 0 && active14 == 0 && board7 == 0 && chat7 == 0 && door7 == 0 {
			continue
		}
		score := current*20 + active14*3 + board7*2 + chat7*2 + door7*3
		rows = append(rows, pulseSpotlightRow{
			Handle:        handle,
			CurrentStreak: current,
			ActiveDays14:  active14,
			Board7:        board7,
			Chat7:         chat7,
			Door7:         door7,
			Score:         score,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		if rows[i].CurrentStreak != rows[j].CurrentStreak {
			return rows[i].CurrentStreak > rows[j].CurrentStreak
		}
		return strings.ToLower(rows[i].Handle) < strings.ToLower(rows[j].Handle)
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func reached(ok bool) string {
	if ok {
		return "reached"
	}
	return "not yet"
}

func timeLaneGuidance(now time.Time, streakCurrent int, activity7 int) string {
	hour := now.Local().Hour()
	switch {
	case hour < 10:
		if streakCurrent == 0 {
			return "Morning reset lane: start with /today then one post to restart streak."
		}
		return "Morning triage lane: /today then /next for quick catch-up."
	case hour < 17:
		if activity7 < 3 {
			return "Afternoon build lane: do one board reply + one chat line."
		}
		return "Afternoon sustain lane: check /missions progress and queue tonight's event."
	default:
		if activity7 < 5 {
			return "Evening comeback lane: run a door and join chat to restore rhythm."
		}
		return "Evening momentum lane: keep streak alive and prep tomorrow's /next queue."
	}
}
