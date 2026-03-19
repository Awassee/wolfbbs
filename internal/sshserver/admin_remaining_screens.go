package sshserver

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/doors"
	"wolfbbs/internal/rbac"
	"wolfbbs/internal/ui"
)

type adminSharedMailRow struct {
	Mail       domain.PrivateMail
	FromHandle string
	ToHandle   string
	Assignment adminModeratorInboxAssignment
}

func sshVisibleDirectoryUser(user *domain.User) bool {
	if user == nil {
		return false
	}
	return strings.TrimSpace(user.Handle) != "" && !user.Banned
}

func (s *Server) adminUserByHandle(handle string) (*domain.User, error) {
	if s == nil || s.auth == nil {
		return nil, fmt.Errorf("auth service unavailable")
	}
	user, err := s.auth.GetUser(strings.TrimSpace(handle))
	if err != nil || user == nil {
		return nil, fmt.Errorf("unknown user")
	}
	return user, nil
}

func (s *Server) buildAdminSharedMailRows() []adminSharedMailRow {
	if s == nil || s.auth == nil || s.mail == nil {
		return nil
	}
	users, err := s.auth.ListUsers()
	if err != nil {
		return nil
	}
	userByID := map[int64]domain.User{}
	moderatorHandles := make([]string, 0, len(users))
	for _, row := range users {
		userByID[row.ID] = row
		role := rbac.NormalizeRole(row.Role)
		if sshVisibleDirectoryUser(&row) && (role == rbac.RoleModerator || role == rbac.RoleSysop) {
			moderatorHandles = append(moderatorHandles, row.Handle)
		}
	}
	sort.Strings(moderatorHandles)
	assignments := s.loadModeratorInboxAssignmentsSSH()
	seen := map[int64]struct{}{}
	out := make([]adminSharedMailRow, 0, 32)
	for _, handle := range moderatorHandles {
		target, err := s.auth.GetUser(handle)
		if err != nil || target == nil {
			continue
		}
		inbox, err := s.mail.ListInbox(target.ID, 200)
		if err != nil {
			continue
		}
		for _, mail := range inbox {
			if _, ok := seen[mail.ID]; ok {
				continue
			}
			seen[mail.ID] = struct{}{}
			assignment := assignments[mail.ID]
			if assignment.Status == "" {
				assignment.Status = "open"
			}
			out = append(out, adminSharedMailRow{
				Mail:       mail,
				FromHandle: userByID[mail.FromUserID].Handle,
				ToHandle:   target.Handle,
				Assignment: assignment,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Mail.CreatedAt.After(out[j].Mail.CreatedAt)
	})
	return out
}

func (s *Server) newDoorRegistrySSH() *doors.Registry {
	registry := doors.NewRegistry()
	registry.SetRepository(s.doors)
	if triviaBinary := strings.TrimSpace(os.Getenv("WOLFBBS_TRIVIA_BINARY")); triviaBinary != "" {
		doors.SeedTrivia(registry, triviaBinary)
	}
	doors.SeedFromEnv(registry)
	return registry
}

func (s *Server) runAdminLoginDesk(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, account *domain.User, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	role := "user"
	verified := false
	if account != nil {
		role = rbac.NormalizeRole(account.Role)
		verified = account.Verified
	}
	lines := []string{
		"Native SSH login is the terminal-first equivalent of /admin/login.",
		"Use this desk to verify transport posture and auth expectations.",
		"",
		fmt.Sprintf("Current actor: %s", actor),
		fmt.Sprintf("Role: %s   Verified: %s", role, boolText(verified)),
		fmt.Sprintf("Telnet login: %s (%s)", boolText(s.flagFromConfig("runtime.login.telnet.enabled", "WOLFBBS_TELNET_ENABLE", false)), s.textFromConfig("runtime.login.telnet.listen", "WOLFBBS_TELNET_LISTEN", ":2323")),
		fmt.Sprintf("WebSocket login: %s (%s%s)", boolText(s.flagFromConfig("runtime.login.ws.enabled", "WOLFBBS_WS_ENABLE", false)), s.textFromConfig("runtime.login.ws.listen", "WOLFBBS_WS_LISTEN", ":6080"), s.textFromConfig("runtime.login.ws.path", "WOLFBBS_WS_PATH", "/ws-login")),
		fmt.Sprintf("WebSocket TLS: %s (%s%s)", boolText(s.flagFromConfig("runtime.login.wss.enabled", "WOLFBBS_WSS_ENABLE", false)), s.textFromConfig("runtime.login.wss.listen", "WOLFBBS_WSS_LISTEN", ":6443"), s.textFromConfig("runtime.login.wss.path", "WOLFBBS_WS_PATH", "/ws-login")),
		fmt.Sprintf("Guest tour enabled: %s", boolText(s.guestTourEnabled())),
		fmt.Sprintf("Secure cookies: %s", boolText(s.secureCookieEnabled())),
		fmt.Sprintf("Verified email gate: %s", boolText(s.requireVerifiedEmail())),
		"",
		"Use the web login screen when you need browser session cookies.",
		"Use SSH/Telnet/WebSocket login when you need terminal-native admin flow.",
	}
	writeClear(sess, ansiEnabled)
	renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Login + Auth", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
	renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Login + Auth", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
	adminPause(sess, reader, touch, "")
}

func (s *Server) runAdminContentDesk(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s == nil || s.admin == nil {
		adminPause(sess, reader, touch, "Admin repository unavailable.")
		return
	}
	if touch == nil {
		touch = func() {}
	}
	for {
		bulletins := s.loadAdminScheduledBulletins()
		events := s.loadAdminCommunityEvents()
		lines := []string{
			"BNEW/BEDIT/BDEL manage scheduled bulletins.",
			"ENEW/EEDIT/EDEL manage community calendar entries.",
			"Time format: YYYY-MM-DD HH:MM (local time).",
			"",
			fmt.Sprintf("Bulletins: %d", len(bulletins)),
		}
		if len(bulletins) == 0 {
			lines = append(lines, "  (none queued)")
		} else {
			for _, row := range bulletins[:minInt(len(bulletins), 5)] {
				lines = append(lines, fmt.Sprintf("  %s  %s  %s", clampForTTY(row.ID, 12), formatAdminLocalDateTime(row.StartsAt), clampForTTY(row.Title, renderWidth-34)))
			}
		}
		lines = append(lines, "", fmt.Sprintf("Events: %d", len(events)))
		if len(events) == 0 {
			lines = append(lines, "  (none scheduled)")
		} else {
			for _, row := range events[:minInt(len(events), 5)] {
				lines = append(lines, fmt.Sprintf("  %s  %s  %-10s %s", clampForTTY(row.ID, 12), formatAdminLocalDateTime(row.StartsAt), clampForTTY(row.Category, 10), clampForTTY(row.Title, renderWidth-46)))
			}
		}
		lines = append(lines,
			"",
			"BNEW title|body|start|end|audience|link",
			"BEDIT id|title|body|start|end|audience|link",
			"BDEL id",
			"ENEW title|category|start|end|location|host|audience|link|description",
			"EEDIT id|title|category|start|end|location|host|audience|link|description",
			"EDEL id | Q",
		)
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Events + Bulletins", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Content Desk", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 1024)
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
		upper := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(upper, "BNEW "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("BNEW "):]), 6)
			startsAt, err := parseAdminLocalDateTime(parts[2])
			if err != nil {
				adminPause(sess, reader, touch, "Bulletin start time is invalid.")
				continue
			}
			endsAt, err := parseAdminOptionalDateTime(parts[3])
			if err != nil {
				adminPause(sess, reader, touch, "Bulletin end time is invalid.")
				continue
			}
			row := adminScheduledBulletin{Title: parts[0], Body: parts[1], StartsAt: startsAt, EndsAt: endsAt, Audience: parts[4], Link: parts[5], CreatedBy: actor, CreatedAt: time.Now().UTC()}
			bulletins = append(bulletins, row)
			if err := s.persistAdminScheduledBulletins(bulletins); err != nil {
				adminPause(sess, reader, touch, "Save bulletin failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, row.Title, "create_bulletin", "")
			adminPause(sess, reader, touch, "Scheduled bulletin created.")
		case strings.HasPrefix(upper, "BEDIT "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("BEDIT "):]), 7)
			row, idx, found := findAdminScheduledBulletin(bulletins, parts[0])
			if !found {
				adminPause(sess, reader, touch, "Bulletin not found.")
				continue
			}
			startsAt, err := parseAdminLocalDateTime(parts[3])
			if err != nil {
				adminPause(sess, reader, touch, "Bulletin start time is invalid.")
				continue
			}
			endsAt, err := parseAdminOptionalDateTime(parts[4])
			if err != nil {
				adminPause(sess, reader, touch, "Bulletin end time is invalid.")
				continue
			}
			row.Title, row.Body, row.StartsAt, row.EndsAt, row.Audience, row.Link = parts[1], parts[2], startsAt, endsAt, parts[5], parts[6]
			bulletins[idx] = row
			if err := s.persistAdminScheduledBulletins(bulletins); err != nil {
				adminPause(sess, reader, touch, "Update bulletin failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, row.ID, "update_bulletin", row.Title)
			adminPause(sess, reader, touch, "Scheduled bulletin updated.")
		case strings.HasPrefix(upper, "BDEL "):
			id := strings.TrimSpace(cmd[len("BDEL "):])
			next := make([]adminScheduledBulletin, 0, len(bulletins))
			removed := false
			for _, row := range bulletins {
				if row.ID == id {
					removed = true
					continue
				}
				next = append(next, row)
			}
			if !removed {
				adminPause(sess, reader, touch, "Bulletin not found.")
				continue
			}
			if err := s.persistAdminScheduledBulletins(next); err != nil {
				adminPause(sess, reader, touch, "Delete bulletin failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, id, "delete_bulletin", "")
			adminPause(sess, reader, touch, "Scheduled bulletin deleted.")
		case strings.HasPrefix(upper, "ENEW "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("ENEW "):]), 9)
			startsAt, err := parseAdminLocalDateTime(parts[2])
			if err != nil {
				adminPause(sess, reader, touch, "Event start time is invalid.")
				continue
			}
			endsAt, err := parseAdminOptionalDateTime(parts[3])
			if err != nil {
				adminPause(sess, reader, touch, "Event end time is invalid.")
				continue
			}
			row := adminCommunityEvent{Title: parts[0], Category: parts[1], StartsAt: startsAt, EndsAt: endsAt, Location: parts[4], Host: parts[5], Audience: parts[6], Link: parts[7], Description: parts[8], CreatedAt: time.Now().UTC()}
			events = append(events, row)
			if err := s.persistAdminCommunityEvents(events); err != nil {
				adminPause(sess, reader, touch, "Save event failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, row.Title, "create_event", row.Category)
			adminPause(sess, reader, touch, "Community event created.")
		case strings.HasPrefix(upper, "EEDIT "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("EEDIT "):]), 10)
			row, idx, found := findAdminCommunityEvent(events, parts[0])
			if !found {
				adminPause(sess, reader, touch, "Event not found.")
				continue
			}
			startsAt, err := parseAdminLocalDateTime(parts[3])
			if err != nil {
				adminPause(sess, reader, touch, "Event start time is invalid.")
				continue
			}
			endsAt, err := parseAdminOptionalDateTime(parts[4])
			if err != nil {
				adminPause(sess, reader, touch, "Event end time is invalid.")
				continue
			}
			row.Title, row.Category, row.StartsAt, row.EndsAt = parts[1], parts[2], startsAt, endsAt
			row.Location, row.Host, row.Audience, row.Link, row.Description = parts[5], parts[6], parts[7], parts[8], parts[9]
			events[idx] = row
			if err := s.persistAdminCommunityEvents(events); err != nil {
				adminPause(sess, reader, touch, "Update event failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, row.ID, "update_event", row.Title)
			adminPause(sess, reader, touch, "Community event updated.")
		case strings.HasPrefix(upper, "EDEL "):
			id := strings.TrimSpace(cmd[len("EDEL "):])
			next := make([]adminCommunityEvent, 0, len(events))
			removed := false
			for _, row := range events {
				if row.ID == id {
					removed = true
					continue
				}
				next = append(next, row)
			}
			if !removed {
				adminPause(sess, reader, touch, "Event not found.")
				continue
			}
			if err := s.persistAdminCommunityEvents(next); err != nil {
				adminPause(sess, reader, touch, "Delete event failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, id, "delete_event", "")
			adminPause(sess, reader, touch, "Community event deleted.")
		default:
			adminPause(sess, reader, touch, "Unknown content command.")
		}
	}
}

func (s *Server) runAdminChallengesDesk(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s == nil || s.admin == nil {
		adminPause(sess, reader, touch, "Admin repository unavailable.")
		return
	}
	if touch == nil {
		touch = func() {}
	}
	for {
		challenges := s.loadAdminSeasonChallenges()
		goals := s.loadAdminClubhouseGoals()
		lines := []string{
			"CNEW/CEDIT/CDEL manage seasonal challenge windows.",
			"GNEW/GEDIT/GDEL manage clubhouse goals.",
			"Time format: YYYY-MM-DD HH:MM (local time).",
			"",
			fmt.Sprintf("Challenges: %d", len(challenges)),
		}
		if len(challenges) == 0 {
			lines = append(lines, "  (none configured)")
		} else {
			for _, row := range challenges[:minInt(len(challenges), 5)] {
				lines = append(lines, fmt.Sprintf("  %s  %s..%s  %s", clampForTTY(row.ID, 12), formatAdminLocalDateTime(row.StartsAt), formatAdminLocalDateTime(row.EndsAt), clampForTTY(row.Name, renderWidth-44)))
			}
		}
		lines = append(lines, "", fmt.Sprintf("Goals: %d", len(goals)))
		if len(goals) == 0 {
			lines = append(lines, "  (none configured)")
		} else {
			for _, row := range goals[:minInt(len(goals), 5)] {
				lines = append(lines, fmt.Sprintf("  %s  %3d/%-3d  %s", clampForTTY(row.ID, 12), row.Progress, row.Target, clampForTTY(row.Title, renderWidth-30)))
			}
		}
		lines = append(lines,
			"",
			"CNEW name|theme|start|end|boardWeight|chatWeight|doorWeight|active|description",
			"CEDIT id|name|theme|start|end|boardWeight|chatWeight|doorWeight|active|description",
			"CDEL id",
			"GNEW title|boardID|doorID|target|progress|description",
			"GEDIT id|title|boardID|doorID|target|progress|description",
			"GDEL id | Q",
		)
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Challenges", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Challenges + Goals", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 1024)
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
		upper := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(upper, "CNEW "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("CNEW "):]), 9)
			startsAt, err := parseAdminLocalDateTime(parts[2])
			if err != nil {
				adminPause(sess, reader, touch, "Challenge start time is invalid.")
				continue
			}
			endsAt, err := parseAdminLocalDateTime(parts[3])
			if err != nil {
				adminPause(sess, reader, touch, "Challenge end time is invalid.")
				continue
			}
			row := adminSeasonChallenge{Name: parts[0], Theme: parts[1], StartsAt: startsAt, EndsAt: endsAt, BoardWeight: parseInt(parts[4], 3), ChatWeight: parseInt(parts[5], 1), DoorWeight: parseInt(parts[6], 2), Active: adminBoolish(parts[7]), Description: parts[8]}
			row.UpdatedBy = actor
			row.UpdatedAt = time.Now().UTC()
			challenges = append(challenges, row)
			if err := s.persistAdminSeasonChallenges(challenges); err != nil {
				adminPause(sess, reader, touch, "Save challenge failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, row.Name, "save_challenge", "create")
			adminPause(sess, reader, touch, "Seasonal challenge saved.")
		case strings.HasPrefix(upper, "CEDIT "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("CEDIT "):]), 10)
			row, idx, found := findAdminSeasonChallenge(challenges, parts[0])
			if !found {
				adminPause(sess, reader, touch, "Challenge not found.")
				continue
			}
			startsAt, err := parseAdminLocalDateTime(parts[3])
			if err != nil {
				adminPause(sess, reader, touch, "Challenge start time is invalid.")
				continue
			}
			endsAt, err := parseAdminLocalDateTime(parts[4])
			if err != nil {
				adminPause(sess, reader, touch, "Challenge end time is invalid.")
				continue
			}
			row.Name, row.Theme, row.StartsAt, row.EndsAt = parts[1], parts[2], startsAt, endsAt
			row.BoardWeight, row.ChatWeight, row.DoorWeight = parseInt(parts[5], row.BoardWeight), parseInt(parts[6], row.ChatWeight), parseInt(parts[7], row.DoorWeight)
			row.Active = adminBoolish(parts[8])
			row.Description = parts[9]
			row.UpdatedBy = actor
			row.UpdatedAt = time.Now().UTC()
			challenges[idx] = row
			if err := s.persistAdminSeasonChallenges(challenges); err != nil {
				adminPause(sess, reader, touch, "Update challenge failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, row.ID, "save_challenge", "update")
			adminPause(sess, reader, touch, "Seasonal challenge updated.")
		case strings.HasPrefix(upper, "CDEL "):
			id := strings.TrimSpace(cmd[len("CDEL "):])
			next := make([]adminSeasonChallenge, 0, len(challenges))
			removed := false
			for _, row := range challenges {
				if row.ID == id {
					removed = true
					continue
				}
				next = append(next, row)
			}
			if !removed {
				adminPause(sess, reader, touch, "Challenge not found.")
				continue
			}
			if err := s.persistAdminSeasonChallenges(next); err != nil {
				adminPause(sess, reader, touch, "Delete challenge failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, id, "delete_challenge", "")
			adminPause(sess, reader, touch, "Seasonal challenge deleted.")
		case strings.HasPrefix(upper, "GNEW "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("GNEW "):]), 6)
			row := adminClubhouseGoal{Title: parts[0], BoardID: int64(parseInt(parts[1], 0)), DoorID: parts[2], Target: parseInt(parts[3], 50), Progress: parseInt(parts[4], 0), Description: parts[5], UpdatedBy: actor, UpdatedAt: time.Now().UTC()}
			goals = append(goals, row)
			if err := s.persistAdminClubhouseGoals(goals); err != nil {
				adminPause(sess, reader, touch, "Save goal failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, row.Title, "save_goal", "create")
			adminPause(sess, reader, touch, "Clubhouse goal saved.")
		case strings.HasPrefix(upper, "GEDIT "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("GEDIT "):]), 7)
			row, idx, found := findAdminClubhouseGoal(goals, parts[0])
			if !found {
				adminPause(sess, reader, touch, "Goal not found.")
				continue
			}
			row.Title, row.BoardID, row.DoorID = parts[1], int64(parseInt(parts[2], int(row.BoardID))), parts[3]
			row.Target, row.Progress, row.Description = parseInt(parts[4], row.Target), parseInt(parts[5], row.Progress), parts[6]
			row.UpdatedBy = actor
			row.UpdatedAt = time.Now().UTC()
			goals[idx] = row
			if err := s.persistAdminClubhouseGoals(goals); err != nil {
				adminPause(sess, reader, touch, "Update goal failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, row.ID, "save_goal", "update")
			adminPause(sess, reader, touch, "Clubhouse goal updated.")
		case strings.HasPrefix(upper, "GDEL "):
			id := strings.TrimSpace(cmd[len("GDEL "):])
			next := make([]adminClubhouseGoal, 0, len(goals))
			removed := false
			for _, row := range goals {
				if row.ID == id {
					removed = true
					continue
				}
				next = append(next, row)
			}
			if !removed {
				adminPause(sess, reader, touch, "Goal not found.")
				continue
			}
			if err := s.persistAdminClubhouseGoals(next); err != nil {
				adminPause(sess, reader, touch, "Delete goal failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, id, "delete_goal", "")
			adminPause(sess, reader, touch, "Clubhouse goal deleted.")
		default:
			adminPause(sess, reader, touch, "Unknown challenge command.")
		}
	}
}

func (s *Server) runAdminMailDesk(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, account *domain.User, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s == nil || s.admin == nil || s.auth == nil || s.mail == nil {
		adminPause(sess, reader, touch, "Mail controls are unavailable.")
		return
	}
	if touch == nil {
		touch = func() {}
	}
	for {
		rows := s.buildAdminSharedMailRows()
		policies, _ := s.admin.ListMailOutboundPolicies()
		sharedCount := 0
		resolvedCount := 0
		for _, row := range rows {
			if row.Assignment.Status == "resolved" {
				resolvedCount++
			} else {
				sharedCount++
			}
		}
		lines := []string{
			fmt.Sprintf("Shared queue: %d   Outbound overrides: %d", len(rows), len(policies)),
			"OUT <handle>|<on|off>",
			"ASSIGN <mailID>|<assignee>|<open|assigned|resolved>|<note>",
			"RESOLVE <mailID>",
			"MERGE <role>|<verified yes/no>|<handles csv>|<subject>|<body>",
			"",
			"Shared Moderator Inbox",
		}
		if len(rows) == 0 {
			lines = append(lines, "  (empty)")
		} else {
			for _, row := range rows[:minInt(len(rows), 6)] {
				lines = append(lines, fmt.Sprintf("  %d  %-10s -> %-10s %-8s %s", row.Mail.ID, clampForTTY(defaultIfBlank(row.FromHandle, "unknown"), 10), clampForTTY(defaultIfBlank(row.ToHandle, "unknown"), 10), clampForTTY(row.Assignment.Status, 8), clampForTTY(row.Mail.Subject, renderWidth-42)))
			}
		}
		lines = append(lines, "", fmt.Sprintf("Open shared items: %d   Resolved rows shown: %d", sharedCount, resolvedCount), "Outbound Policies")
		if len(policies) == 0 {
			lines = append(lines, "  (none)")
		} else {
			for _, row := range policies[:minInt(len(policies), 6)] {
				status := "on"
				if row.OutboundDisabled {
					status = "off"
				}
				lines = append(lines, fmt.Sprintf("  %-16s %s", clampForTTY(row.Handle, 16), status))
			}
		}
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Mail", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Mail Controls", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 1024)
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
		upper := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(upper, "OUT "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("OUT "):]), 2)
			handle := strings.TrimSpace(parts[0])
			disable := !adminBoolish(parts[1])
			if err := s.admin.SetMailOutboundPolicy(handle, disable); err != nil {
				adminPause(sess, reader, touch, "Outbound update failed: "+err.Error())
				continue
			}
			action := "enable_outbound"
			if disable {
				action = "disable_outbound"
			}
			recordAudit(s.admin, actor, handle, action, "")
			adminPause(sess, reader, touch, "Outbound policy updated.")
		case strings.HasPrefix(upper, "ASSIGN "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("ASSIGN "):]), 4)
			mailID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
			if err != nil || mailID <= 0 {
				adminPause(sess, reader, touch, "Use a valid mail ID.")
				continue
			}
			row := adminModeratorInboxAssignment{MailID: mailID, Assignee: parts[1], Status: parts[2], Note: parts[3], UpdatedBy: actor, UpdatedAt: time.Now().UTC()}
			if row.Status == "resolved" {
				row.ResolvedAt = row.UpdatedAt
			}
			if err := s.updateModeratorInboxAssignmentSSH(mailID, row); err != nil {
				adminPause(sess, reader, touch, "Assignment save failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, strconv.FormatInt(mailID, 10), "assign_shared_mail", row.Status)
			adminPause(sess, reader, touch, "Shared mail assignment updated.")
		case strings.HasPrefix(upper, "RESOLVE "):
			mailID, err := strconv.ParseInt(strings.TrimSpace(cmd[len("RESOLVE "):]), 10, 64)
			if err != nil || mailID <= 0 {
				adminPause(sess, reader, touch, "Use a valid mail ID.")
				continue
			}
			assignments := s.loadModeratorInboxAssignmentsSSH()
			row := assignments[mailID]
			row.MailID = mailID
			row.Status = "resolved"
			row.UpdatedBy = actor
			row.UpdatedAt = time.Now().UTC()
			row.ResolvedAt = row.UpdatedAt
			assignments[mailID] = row
			if err := s.persistModeratorInboxAssignmentsSSH(assignments); err != nil {
				adminPause(sess, reader, touch, "Resolve failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, strconv.FormatInt(mailID, 10), "resolve_shared_mail", "")
			adminPause(sess, reader, touch, "Shared mail resolved.")
		case strings.HasPrefix(upper, "MERGE "):
			if account == nil || account.ID <= 0 {
				adminPause(sess, reader, touch, "Signed-in account required for mail merge.")
				continue
			}
			parts := splitFixedParts(strings.TrimSpace(cmd[len("MERGE "):]), 5)
			if strings.TrimSpace(parts[3]) == "" || strings.TrimSpace(parts[4]) == "" {
				adminPause(sess, reader, touch, "Merge subject and body are required.")
				continue
			}
			roleFilter := normalizeAdminRole(parts[0])
			if strings.EqualFold(strings.TrimSpace(parts[0]), "any") {
				roleFilter = "any"
			}
			verifiedOnly := adminBoolish(parts[1])
			handleAllow := map[string]struct{}{}
			for _, value := range splitCSV(parts[2]) {
				handleAllow[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
			}
			users, err := s.auth.ListUsers()
			if err != nil {
				adminPause(sess, reader, touch, "List users failed: "+err.Error())
				continue
			}
			delivered := 0
			for _, row := range users {
				if !sshVisibleDirectoryUser(&row) || row.ID == account.ID {
					continue
				}
				if roleFilter != "any" && roleFilter != "" && rbac.NormalizeRole(row.Role) != roleFilter {
					continue
				}
				if verifiedOnly && !row.Verified {
					continue
				}
				if len(handleAllow) > 0 {
					if _, ok := handleAllow[strings.ToLower(strings.TrimSpace(row.Handle))]; !ok {
						continue
					}
				}
				body := strings.ReplaceAll(parts[4], "{{handle}}", row.Handle)
				if err := s.mail.CreateMail(&domain.PrivateMail{FromUserID: account.ID, ToUserID: row.ID, Subject: strings.TrimSpace(parts[3]), Body: strings.TrimSpace(body)}); err == nil {
					delivered++
				}
			}
			recordAudit(s.admin, actor, "mail.merge", "merge_send", fmt.Sprintf("delivered=%d", delivered))
			adminPause(sess, reader, touch, fmt.Sprintf("Mail merge delivered to %d caller(s).", delivered))
		default:
			adminPause(sess, reader, touch, "Unknown mail command.")
		}
	}
}

func (s *Server) runAdminFilesDesk(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, account *domain.User, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s == nil || s.admin == nil || s.auth == nil {
		adminPause(sess, reader, touch, "File controls are unavailable.")
		return
	}
	if touch == nil {
		touch = func() {}
	}
	for {
		areas, err := s.ensureFileAreasSeeded()
		if err != nil {
			adminPause(sess, reader, touch, "Load file areas failed: "+err.Error())
			return
		}
		entries, _ := s.admin.ListFileEntries(0, "", nil, 8)
		reviewQueue := s.loadFileReviewQueueSSH()
		collections := s.loadFeaturedCollectionsSSH()
		lines := []string{
			fmt.Sprintf("Areas: %d   Indexed files: %d   Review items: %d   Collections: %d", len(areas), len(entries), len(reviewQueue), len(collections)),
			"AREA <name>|<path>|<description>",
			"DROPAREA <id> | INDEX <areaID>|<uploaderHandle>",
			"REVIEW <fileID>|<hold|approved|rejected>|<note> | DELETE <fileID>",
			"RATE <handle>|<fileID>|<1-5> | QUEUE <handle>|<fileID> | DEQUEUE <handle>|<fileID>",
			"TICKET <handle>|<fileID>|<minutes> | FILTER <handle>|<name>|<query>|<tags>",
			"COLLECT <id>|<title>|<description>|<fileIDs csv>|<tags csv> | DELCOL <id>",
			"",
			"Areas",
		}
		for _, row := range areas[:minInt(len(areas), 4)] {
			lines = append(lines, fmt.Sprintf("  %-3d %-16s %s", row.ID, clampForTTY(row.Name, 16), clampForTTY(filepath.Clean(row.Path), renderWidth-26)))
		}
		lines = append(lines, "", "Indexed Files")
		if len(entries) == 0 {
			lines = append(lines, "  (none indexed)")
		} else {
			for _, row := range entries {
				lines = append(lines, fmt.Sprintf("  %-4d %-16s %s", row.ID, clampForTTY(defaultIfBlank(strings.Join(row.Tags, ","), "no-tags"), 16), clampForTTY(row.Name, renderWidth-28)))
			}
		}
		lines = append(lines, "", "Review Queue")
		if len(reviewQueue) == 0 {
			lines = append(lines, "  (empty)")
		} else {
			ids := make([]int64, 0, len(reviewQueue))
			for id := range reviewQueue {
				ids = append(ids, id)
			}
			sort.Slice(ids, func(i, j int) bool { return reviewQueue[ids[i]].CreatedAt.After(reviewQueue[ids[j]].CreatedAt) })
			for _, id := range ids[:minInt(len(ids), 4)] {
				row := reviewQueue[id]
				lines = append(lines, fmt.Sprintf("  %-4d %-9s %s", row.FileID, clampForTTY(row.Status, 9), clampForTTY(row.Name, renderWidth-22)))
			}
		}
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Files", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Files Desk", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 1024)
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
		upper := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(upper, "AREA "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("AREA "):]), 3)
			path := filepath.Clean(strings.TrimSpace(parts[1]))
			if path == "" || path == "." {
				adminPause(sess, reader, touch, "Area path is required.")
				continue
			}
			if err := os.MkdirAll(path, 0o755); err != nil {
				adminPause(sess, reader, touch, "Create area path failed: "+err.Error())
				continue
			}
			row := &domain.FileArea{Name: strings.TrimSpace(parts[0]), Path: path, Description: strings.TrimSpace(parts[2])}
			if err := s.admin.CreateFileArea(row); err != nil {
				adminPause(sess, reader, touch, "Create area failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, row.Name, "create_file_area", row.Path)
			adminPause(sess, reader, touch, "File area created.")
		case strings.HasPrefix(upper, "DROPAREA "):
			id, err := strconv.ParseInt(strings.TrimSpace(cmd[len("DROPAREA "):]), 10, 64)
			if err != nil || id <= 0 {
				adminPause(sess, reader, touch, "Use a valid area ID.")
				continue
			}
			if err := s.admin.DeleteFileArea(id); err != nil {
				adminPause(sess, reader, touch, "Delete area failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, strconv.FormatInt(id, 10), "delete_file_area", "")
			adminPause(sess, reader, touch, "File area deleted.")
		case strings.HasPrefix(upper, "INDEX "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("INDEX "):]), 2)
			areaID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
			if err != nil || areaID <= 0 {
				adminPause(sess, reader, touch, "Use a valid area ID.")
				continue
			}
			uploader, err := s.adminUserByHandle(defaultIfBlank(parts[1], actor))
			if err != nil {
				adminPause(sess, reader, touch, err.Error())
				continue
			}
			var selected *domain.FileArea
			for i := range areas {
				if areas[i].ID == areaID {
					selected = &areas[i]
					break
				}
			}
			if selected == nil {
				adminPause(sess, reader, touch, "Area not found.")
				continue
			}
			indexed, failed, err := s.indexAreaFilesSSH(*selected, uploader.ID)
			if err != nil {
				adminPause(sess, reader, touch, "Index area failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, selected.Name, "index_file_area", fmt.Sprintf("indexed=%d failed=%d", indexed, failed))
			adminPause(sess, reader, touch, fmt.Sprintf("Indexed %d file(s); %d failed.", indexed, failed))
		case strings.HasPrefix(upper, "REVIEW "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("REVIEW "):]), 3)
			fileID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
			if err != nil || fileID <= 0 {
				adminPause(sess, reader, touch, "Use a valid file ID.")
				continue
			}
			entry, err := s.admin.GetFileEntry(fileID)
			if err != nil || entry == nil {
				adminPause(sess, reader, touch, "File not found.")
				continue
			}
			row := adminFileReviewItem{FileID: entry.ID, AreaID: entry.AreaID, Name: entry.Name, Status: parts[1], Notes: parts[2], ReviewedBy: actor, ReviewedAt: time.Now().UTC()}
			if existing, ok := reviewQueue[fileID]; ok {
				row.Uploader = existing.Uploader
				row.CreatedAt = existing.CreatedAt
			} else {
				row.Uploader = actor
				row.CreatedAt = time.Now().UTC()
			}
			if err := s.setFileReviewItemSSH(row); err != nil {
				adminPause(sess, reader, touch, "Review save failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, strconv.FormatInt(fileID, 10), "review_file", row.Status)
			adminPause(sess, reader, touch, "File review updated.")
		case strings.HasPrefix(upper, "DELETE "):
			fileID, err := strconv.ParseInt(strings.TrimSpace(cmd[len("DELETE "):]), 10, 64)
			if err != nil || fileID <= 0 {
				adminPause(sess, reader, touch, "Use a valid file ID.")
				continue
			}
			entry, err := s.admin.GetFileEntry(fileID)
			if err != nil || entry == nil {
				adminPause(sess, reader, touch, "File not found.")
				continue
			}
			if err := s.deleteIndexedFileSSH(entry); err != nil {
				adminPause(sess, reader, touch, "Delete file failed: "+err.Error())
				continue
			}
			_ = s.removeFileReviewItemSSH(fileID)
			recordAudit(s.admin, actor, entry.Name, "delete_file_entry", fmt.Sprintf("file_id=%d", fileID))
			adminPause(sess, reader, touch, "Indexed file deleted.")
		case strings.HasPrefix(upper, "RATE "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("RATE "):]), 3)
			user, err := s.adminUserByHandle(parts[0])
			if err != nil {
				adminPause(sess, reader, touch, err.Error())
				continue
			}
			fileID, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
			if err != nil || fileID <= 0 {
				adminPause(sess, reader, touch, "Use a valid file ID.")
				continue
			}
			rating := parseInt(parts[2], 0)
			if rating <= 0 {
				adminPause(sess, reader, touch, "Use a rating between 1 and 5.")
				continue
			}
			if err := s.admin.SetFileRating(user.ID, fileID, rating); err != nil {
				adminPause(sess, reader, touch, "Set rating failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, strconv.FormatInt(fileID, 10), "rate_file", fmt.Sprintf("user=%d rating=%d", user.ID, rating))
			adminPause(sess, reader, touch, "File rating saved.")
		case strings.HasPrefix(upper, "QUEUE "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("QUEUE "):]), 2)
			user, err := s.adminUserByHandle(parts[0])
			if err != nil {
				adminPause(sess, reader, touch, err.Error())
				continue
			}
			fileID, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
			if err != nil || fileID <= 0 {
				adminPause(sess, reader, touch, "Use a valid file ID.")
				continue
			}
			if err := s.admin.EnqueueDownload(user.ID, fileID); err != nil {
				adminPause(sess, reader, touch, "Queue add failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, strconv.FormatInt(fileID, 10), "enqueue_download", fmt.Sprintf("user=%d", user.ID))
			adminPause(sess, reader, touch, "Download queued.")
		case strings.HasPrefix(upper, "DEQUEUE "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("DEQUEUE "):]), 2)
			user, err := s.adminUserByHandle(parts[0])
			if err != nil {
				adminPause(sess, reader, touch, err.Error())
				continue
			}
			fileID, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
			if err != nil || fileID <= 0 {
				adminPause(sess, reader, touch, "Use a valid file ID.")
				continue
			}
			if err := s.admin.DequeueDownload(user.ID, fileID); err != nil {
				adminPause(sess, reader, touch, "Queue remove failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, strconv.FormatInt(fileID, 10), "dequeue_download", fmt.Sprintf("user=%d", user.ID))
			adminPause(sess, reader, touch, "Download dequeued.")
		case strings.HasPrefix(upper, "TICKET "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("TICKET "):]), 3)
			user, err := s.adminUserByHandle(parts[0])
			if err != nil {
				adminPause(sess, reader, touch, err.Error())
				continue
			}
			fileID, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
			if err != nil || fileID <= 0 {
				adminPause(sess, reader, touch, "Use a valid file ID.")
				continue
			}
			ttlMinutes := parseInt(parts[2], 15)
			if ttlMinutes <= 0 {
				ttlMinutes = 15
			}
			ticket, err := s.issueDownloadTicket(user.ID, fileID, time.Duration(ttlMinutes)*time.Minute)
			if err != nil {
				adminPause(sess, reader, touch, "Issue ticket failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, strconv.FormatInt(fileID, 10), "issue_download_ticket", fmt.Sprintf("user=%d ttl=%d", user.ID, ttlMinutes))
			adminPause(sess, reader, touch, "Ticket: "+ticket.Token+" URL: /gateway?download="+ticket.Token)
		case strings.HasPrefix(upper, "FILTER "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("FILTER "):]), 4)
			user, err := s.adminUserByHandle(parts[0])
			if err != nil {
				adminPause(sess, reader, touch, err.Error())
				continue
			}
			row := &domain.FileFilter{UserID: user.ID, Name: strings.TrimSpace(parts[1]), Query: strings.TrimSpace(parts[2]), Tags: splitCSV(parts[3])}
			if err := s.admin.SaveFileFilter(row); err != nil {
				adminPause(sess, reader, touch, "Save filter failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, row.Name, "save_file_filter", fmt.Sprintf("user=%d", user.ID))
			adminPause(sess, reader, touch, "Saved file filter.")
		case strings.HasPrefix(upper, "COLLECT "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("COLLECT "):]), 5)
			row := sshFeaturedFileCollection{ID: parts[0], Title: parts[1], Description: parts[2], FileIDs: adminCSVToInt64(parts[3], sshMaxFeaturedCollectionFiles), Tags: splitCSV(parts[4]), Curator: actor}
			if err := s.upsertFeaturedCollectionSSH(row); err != nil {
				adminPause(sess, reader, touch, "Save collection failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, defaultIfBlank(row.ID, row.Title), "save_collection", row.Title)
			adminPause(sess, reader, touch, "Featured collection saved.")
		case strings.HasPrefix(upper, "DELCOL "):
			id := strings.TrimSpace(cmd[len("DELCOL "):])
			if err := s.deleteFeaturedCollectionSSH(id); err != nil {
				adminPause(sess, reader, touch, "Delete collection failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, id, "delete_collection", "")
			adminPause(sess, reader, touch, "Featured collection deleted.")
		default:
			adminPause(sess, reader, touch, "Unknown file command.")
		}
	}
}

func (s *Server) runAdminDoorsDesk(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s == nil || s.doors == nil {
		adminPause(sess, reader, touch, "Door repository unavailable.")
		return
	}
	if touch == nil {
		touch = func() {}
	}
	for {
		registry := s.newDoorRegistrySSH()
		doorRows := registry.Doors()
		sort.Slice(doorRows, func(i, j int) bool { return strings.ToLower(doorRows[i].Name) < strings.ToLower(doorRows[j].Name) })
		lines := []string{
			"SAVE <doorID>|<on/off>|turns|bank|reset|msgDays|logDays|runSec|outRate|net|fsw|role",
			"RESET <doorID>",
			"",
			fmt.Sprintf("Doors loaded: %d", len(doorRows)),
			fmt.Sprintf("%-3s %-18s %-5s %-5s %-5s %-8s %s", "HK", "Door", "On", "Turn", "Bank", "Role", "Category"),
		}
		for _, door := range doorRows[:minInt(len(doorRows), 10)] {
			cfg, _ := registry.GetDoorConfig(door.ID)
			if cfg == nil {
				cfg = &domain.DoorConfig{DoorID: door.ID, Enabled: door.EnabledDefault, DailyTurns: door.DailyTurns, TimeBankMax: door.TimeBankMax, ResetHourLocal: door.ResetHour, MessagesDays: maxInt(1, door.MessagesDays), LogsDays: maxInt(1, door.LogsDays), MaxRunSeconds: door.MaxRunSec, MaxOutputRate: door.MaxOutputRate, AllowNetwork: door.NeedsNetwork, AllowFSWrite: door.NeedsFSWrite}
			}
			role := defaultIfBlank(cfg.RequiredRoleOverride, "default")
			lines = append(lines, fmt.Sprintf("%-3s %-18s %-5s %-5d %-5d %-8s %s", strings.ToUpper(door.Hotkey), clampForTTY(door.Name, 18), clampForTTY(boolText(cfg.Enabled), 5), cfg.DailyTurns, cfg.TimeBankMax, clampForTTY(role, 8), clampForTTY(door.Category, renderWidth-50)))
		}
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Doors", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Door Controls", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 1024)
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
		upper := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(upper, "RESET "):
			doorID := strings.ToLower(strings.TrimSpace(cmd[len("RESET "):]))
			if err := registry.ResetDoorScores(doorID); err != nil {
				adminPause(sess, reader, touch, "Reset scores failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, doorID, "door_reset_scores", "")
			adminPause(sess, reader, touch, "Door scores reset.")
		case strings.HasPrefix(upper, "SAVE "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("SAVE "):]), 12)
			doorID := strings.ToLower(strings.TrimSpace(parts[0]))
			door, found := registry.DoorByID(doorID)
			if !found {
				adminPause(sess, reader, touch, "Door not found.")
				continue
			}
			cfg, _ := registry.GetDoorConfig(doorID)
			if cfg == nil {
				cfg = &domain.DoorConfig{DoorID: doorID, Enabled: door.EnabledDefault, DailyTurns: door.DailyTurns, TimeBankMax: door.TimeBankMax, ResetHourLocal: door.ResetHour, MessagesDays: maxInt(1, door.MessagesDays), LogsDays: maxInt(1, door.LogsDays), MaxRunSeconds: door.MaxRunSec, MaxOutputRate: door.MaxOutputRate, AllowNetwork: door.NeedsNetwork, AllowFSWrite: door.NeedsFSWrite}
			}
			cfg.Enabled = adminBoolish(parts[1])
			cfg.DailyTurns = parseInt(parts[2], cfg.DailyTurns)
			cfg.TimeBankMax = parseInt(parts[3], cfg.TimeBankMax)
			cfg.ResetHourLocal = parseInt(parts[4], cfg.ResetHourLocal)
			cfg.MessagesDays = parseInt(parts[5], cfg.MessagesDays)
			cfg.LogsDays = parseInt(parts[6], cfg.LogsDays)
			cfg.MaxRunSeconds = parseInt(parts[7], cfg.MaxRunSeconds)
			cfg.MaxOutputRate = parseInt(parts[8], cfg.MaxOutputRate)
			cfg.AllowNetwork = adminBoolish(parts[9])
			cfg.AllowFSWrite = adminBoolish(parts[10])
			cfg.RequiredRoleOverride = strings.TrimSpace(parts[11])
			if cfg.RequiredRoleOverride == "-" {
				cfg.RequiredRoleOverride = ""
			}
			if err := registry.SetDoorConfig(cfg); err != nil {
				adminPause(sess, reader, touch, "Save door config failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, doorID, "door_save_config", "updated policy")
			adminPause(sess, reader, touch, "Door configuration saved.")
		default:
			adminPause(sess, reader, touch, "Unknown door command.")
		}
	}
}

func (s *Server) runAdminOpsCenter(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s == nil || s.admin == nil {
		adminPause(sess, reader, touch, "Ops view unavailable.")
		return
	}
	if touch == nil {
		touch = func() {}
	}
	for {
		errors := s.loadSharedRuntimeErrorsSSH()
		pages := s.loadPageRequests()
		escalations := s.loadStaffEscalationsSSH()
		sessions, _ := s.admin.ListNodeSessions(8)
		auditRows, _ := s.admin.ListAudit(6)
		lines := []string{
			fmt.Sprintf("Shared runtime errors: %d   Open pages: %d   Escalations: %d   Active nodes: %d", len(errors), len(pages), countOpenEscalationsSSH(escalations), len(sessions)),
			"CLEAR ERRORS | PRUNE <minutes> | PAGE <id> | ESC <id>",
			"Web session and rate-limit counters remain web-process local.",
			"",
			"Recent Runtime Errors",
		}
		if len(errors) == 0 {
			lines = append(lines, "  (none)")
		} else {
			for _, row := range reverseSharedErrorsSSH(errors)[:minInt(len(errors), 5)] {
				lines = append(lines, fmt.Sprintf("  %s %-12s %s", formatClock(row.Time.Local(), time24h), clampForTTY(row.Area, 12), clampForTTY(row.Message, renderWidth-31)))
			}
		}
		lines = append(lines, "", "Open Pages")
		if len(pages) == 0 {
			lines = append(lines, "  (none)")
		} else {
			for _, row := range pages[:minInt(len(pages), 4)] {
				lines = append(lines, fmt.Sprintf("  %s %-10s -> %-10s %s", clampForTTY(row.ID, 8), clampForTTY(row.From, 10), clampForTTY(row.To, 10), clampForTTY(row.Message, renderWidth-35)))
			}
		}
		lines = append(lines, "", "Staff Escalations")
		if len(escalations) == 0 {
			lines = append(lines, "  (none)")
		} else {
			for _, row := range escalations[:minInt(len(escalations), 4)] {
				state := "open"
				if !row.ResolvedAt.IsZero() {
					state = "resolved"
				}
				lines = append(lines, fmt.Sprintf("  %s %-10s %-8s %s", clampForTTY(row.ID, 8), clampForTTY(row.Handle, 10), clampForTTY(state, 8), clampForTTY(row.Note, renderWidth-32)))
			}
		}
		lines = append(lines, "", "Recent Audit")
		if len(auditRows) == 0 {
			lines = append(lines, "  (none)")
		} else {
			for _, row := range auditRows {
				lines = append(lines, fmt.Sprintf("  %s %-10s %-18s %s", formatClock(row.CreatedAt.Local(), time24h), clampForTTY(row.Actor, 10), clampForTTY(row.Action, 18), clampForTTY(row.Target, renderWidth-36)))
			}
		}
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Ops + Errors", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Ops Center", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 256)
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
		upper := strings.ToUpper(cmd)
		switch {
		case upper == "CLEAR ERRORS":
			cleared, err := s.clearSharedRuntimeErrorsSSH()
			if err != nil {
				adminPause(sess, reader, touch, "Clear errors failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, "runtime", "clear_errors", fmt.Sprintf("cleared=%d", cleared))
			adminPause(sess, reader, touch, fmt.Sprintf("Cleared %d shared runtime error(s).", cleared))
		case strings.HasPrefix(upper, "PRUNE "):
			minutes := parseInt(strings.TrimSpace(cmd[len("PRUNE "):]), 30)
			cutoff := time.Now().UTC().Add(-time.Duration(minutes) * time.Minute)
			pruned := 0
			allSessions, _ := s.admin.ListNodeSessions(500)
			for _, row := range allSessions {
				if row.LastActivity.After(cutoff) {
					continue
				}
				if err := s.admin.DeleteNodeSession(row.SessionID); err == nil {
					pruned++
				}
			}
			recordAudit(s.admin, actor, "node_sessions", "prune_idle_sessions", fmt.Sprintf("minutes=%d pruned=%d", minutes, pruned))
			adminPause(sess, reader, touch, fmt.Sprintf("Pruned %d idle session(s).", pruned))
		case strings.HasPrefix(upper, "PAGE "):
			id := strings.TrimSpace(cmd[len("PAGE "):])
			if !s.resolvePageRequestSSH(id) {
				adminPause(sess, reader, touch, "Page request not found.")
				continue
			}
			recordAudit(s.admin, actor, "page_requests", "resolve_page", "id="+id)
			adminPause(sess, reader, touch, "Page request resolved.")
		case strings.HasPrefix(upper, "ESC "):
			id := strings.TrimSpace(cmd[len("ESC "):])
			if !s.resolveStaffEscalationSSH(id) {
				adminPause(sess, reader, touch, "Staff escalation not found.")
				continue
			}
			recordAudit(s.admin, actor, "staff_escalations", "resolve_escalation", "id="+id)
			adminPause(sess, reader, touch, "Staff escalation resolved.")
		default:
			adminPause(sess, reader, touch, "Unknown ops command.")
		}
	}
}

func reverseSharedErrorsSSH(rows []adminSharedRuntimeError) []adminSharedRuntimeError {
	out := make([]adminSharedRuntimeError, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		out = append(out, rows[i])
	}
	return out
}

func countOpenEscalationsSSH(rows []adminStaffEscalation) int {
	count := 0
	for _, row := range rows {
		if row.ResolvedAt.IsZero() {
			count++
		}
	}
	return count
}
