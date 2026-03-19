package sshserver

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/rbac"
	"wolfbbs/internal/ui"
)

func (s *Server) runAdminCenter(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, account *domain.User, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	for {
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin Control Deck", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		lines := []string{
			"Native SSH sysop controls (terminal-first parity with /admin).",
			"",
			"[C] Config/Setup identity, runtime flags, bootstrap",
			"[D] Doors        registry policy + score resets",
			"[E] Events       events + scheduled bulletins",
			"[F] Files        areas, review, queue, collections",
			"[G] Gateways     smtp/web/ai controls",
			"[H] Chat Ops     locks + moderation actions",
			"[I] Mail         shared inbox + outbound + merge",
			"[J] Challenges   seasons + clubhouse goals",
			"[L] Login/Auth   transport posture + admin auth notes",
			"[O] Ops/Errors   pages, escalations, runtime errors",
			"[R] Release      launch, upgrade, backups, release evidence",
			"[U] Users        create/disable/ban/role/password",
			"[B] Boards       create/update/delete boards",
			"[M] Moderation   reports + lock/unlock/move thread",
			"[A] Audit Log    recent admin actions",
			"[N] Node/Calls   active nodes + caller history",
			"[X] App Upgrade  run configured upgrade command",
			"[Q] Return",
		}
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Admin Center", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		io.WriteString(sess, "Selection: ")
		choice, err := readKey(reader)
		if err != nil {
			return
		}
		touch()
		switch choice {
		case "Q", "ESC":
			return
		case "C":
			s.runAdminConfigSetup(sess, reader, termWidth, renderWidth, actor, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "D":
			s.runAdminDoorsDesk(sess, reader, termWidth, renderWidth, actor, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "E":
			s.runAdminContentDesk(sess, reader, termWidth, renderWidth, actor, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "F":
			s.runAdminFilesDesk(sess, reader, termWidth, renderWidth, actor, account, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "G":
			s.runAdminGatewayDeck(sess, reader, termWidth, renderWidth, actor, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "H":
			s.runAdminChatOps(sess, reader, termWidth, renderWidth, actor, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "I":
			s.runAdminMailDesk(sess, reader, termWidth, renderWidth, actor, account, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "J":
			s.runAdminChallengesDesk(sess, reader, termWidth, renderWidth, actor, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "L":
			s.runAdminLoginDesk(sess, reader, termWidth, renderWidth, actor, account, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "O":
			s.runAdminOpsCenter(sess, reader, termWidth, renderWidth, actor, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "R":
			s.runAdminReleaseTooling(sess, reader, termWidth, renderWidth, actor, account, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "U":
			s.runAdminUsers(sess, reader, termWidth, renderWidth, actor, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "B":
			s.runAdminBoards(sess, reader, termWidth, renderWidth, actor, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "M":
			s.runAdminModeration(sess, reader, termWidth, renderWidth, actor, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "A":
			s.runAdminAuditView(sess, reader, termWidth, renderWidth, actor, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "N":
			s.runAdminNodeView(sess, reader, termWidth, renderWidth, actor, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "X":
			s.runAppUpgrade(sess, reader, actor, account, touch)
		default:
			adminPause(sess, reader, touch, "Unknown admin key.")
		}
	}
}

func (s *Server) runAdminUsers(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s.auth == nil || s.users == nil {
		adminPause(sess, reader, touch, "Auth/user repositories are not configured.")
		return
	}
	for {
		users, err := s.auth.ListUsers()
		if err != nil {
			adminPause(sess, reader, touch, "Could not load users: "+err.Error())
			return
		}
		sort.Slice(users, func(i, j int) bool {
			return strings.ToLower(users[i].Handle) < strings.ToLower(users[j].Handle)
		})
		lines := []string{
			"CREATE <handle> <password> [user|moderator|sysop]",
			"ROLE <handle> <user|moderator|sysop>",
			"ENABLE <handle> | DISABLE <handle> | BAN <handle> | UNBAN <handle>",
			"PASS <handle> <newPassword> | BACK",
			"",
			fmt.Sprintf("%-14s %-9s %-3s %-3s %-3s %s", "Handle", "Role", "On", "Ban", "2FA", "Last Login"),
		}
		for _, row := range users {
			lastLogin := "-"
			if row.LastLoginAt != nil {
				lastLogin = formatClock(row.LastLoginAt.UTC(), time24h)
			}
			twoFA := "N"
			if strings.TrimSpace(row.TOTPSecret) != "" {
				twoFA = "Y"
			}
			lines = append(lines, fmt.Sprintf("%-14s %-9s %-3s %-3s %-3s %s",
				clampForTTY(row.Handle, 14),
				rbac.NormalizeRole(row.Role),
				boolYN(row.Enabled),
				boolYN(row.Banned),
				twoFA,
				lastLogin,
			))
		}
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Users", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "User Operations", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 260)
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
		if len(fields) == 0 {
			continue
		}
		verb := strings.ToUpper(fields[0])
		switch verb {
		case "CREATE":
			if len(fields) < 3 {
				adminPause(sess, reader, touch, "Usage: CREATE <handle> <password> [role]")
				continue
			}
			user, regErr := s.auth.Register(fields[1], fields[2])
			if regErr != nil {
				adminPause(sess, reader, touch, "Create failed: "+regErr.Error())
				continue
			}
			role := rbac.RoleUser
			if len(fields) >= 4 {
				role = normalizeAdminRole(fields[3])
				if role == "" {
					adminPause(sess, reader, touch, "Role must be user/moderator/sysop.")
					continue
				}
			}
			if role != rbac.RoleUser {
				user.Role = role
				if err := s.users.Update(user); err != nil {
					adminPause(sess, reader, touch, "Role update failed: "+err.Error())
					continue
				}
			}
			recordAudit(s.admin, actor, user.Handle, "admin_user_create", "role="+role)
			adminPause(sess, reader, touch, "Created user "+user.Handle+" as "+role+".")
		case "ROLE":
			if len(fields) < 3 {
				adminPause(sess, reader, touch, "Usage: ROLE <handle> <user|moderator|sysop>")
				continue
			}
			role := normalizeAdminRole(fields[2])
			if role == "" {
				adminPause(sess, reader, touch, "Role must be user/moderator/sysop.")
				continue
			}
			user, err := s.auth.GetUser(fields[1])
			if err != nil || user == nil {
				adminPause(sess, reader, touch, "Unknown user.")
				continue
			}
			user.Role = role
			if err := s.users.Update(user); err != nil {
				adminPause(sess, reader, touch, "Role update failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, user.Handle, "admin_user_role", "role="+role)
			adminPause(sess, reader, touch, "Updated role for "+user.Handle+".")
		case "ENABLE", "DISABLE":
			if len(fields) < 2 {
				adminPause(sess, reader, touch, "Usage: ENABLE/DISABLE <handle>")
				continue
			}
			enabled := verb == "ENABLE"
			if err := s.auth.SetEnabled(fields[1], enabled); err != nil {
				adminPause(sess, reader, touch, "Update failed: "+err.Error())
				continue
			}
			action := "admin_user_disable"
			if enabled {
				action = "admin_user_enable"
			}
			recordAudit(s.admin, actor, fields[1], action, "")
			adminPause(sess, reader, touch, "Updated enabled state for "+fields[1]+".")
		case "BAN", "UNBAN":
			if len(fields) < 2 {
				adminPause(sess, reader, touch, "Usage: BAN/UNBAN <handle>")
				continue
			}
			banned := verb == "BAN"
			if err := s.auth.SetBanned(fields[1], banned); err != nil {
				adminPause(sess, reader, touch, "Update failed: "+err.Error())
				continue
			}
			action := "admin_user_unban"
			if banned {
				action = "admin_user_ban"
			}
			recordAudit(s.admin, actor, fields[1], action, "")
			adminPause(sess, reader, touch, "Updated ban state for "+fields[1]+".")
		case "PASS":
			if len(fields) < 3 {
				adminPause(sess, reader, touch, "Usage: PASS <handle> <newPassword>")
				continue
			}
			if err := s.auth.SetPassword(fields[1], fields[2]); err != nil {
				adminPause(sess, reader, touch, "Password update failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, fields[1], "admin_user_password_reset", "")
			adminPause(sess, reader, touch, "Password updated for "+fields[1]+".")
		default:
			adminPause(sess, reader, touch, "Unknown user command.")
		}
	}
}

func (s *Server) runAdminBoards(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s.boards == nil {
		adminPause(sess, reader, touch, "Board repository is not configured.")
		return
	}
	actorID := int64(0)
	if s.auth != nil {
		if account, err := s.auth.GetUser(actor); err == nil && account != nil {
			actorID = account.ID
		}
	}
	for {
		boards, err := s.boards.List()
		if err != nil {
			adminPause(sess, reader, touch, "Could not load boards: "+err.Error())
			return
		}
		lines := []string{
			"CREATE <name>|<description>|<conference>|<read_acs>|<write_acs>",
			"UPDATE <id>|<name>|<description>|<conference>|<read_acs>|<write_acs>",
			"DELETE <id> | BACK",
			"",
			fmt.Sprintf("%-4s %-24s %-14s %-10s %-10s", "ID", "Name", "Conference", "Read", "Write"),
		}
		for _, board := range boards {
			readRule := defaultIfBlank(strings.TrimSpace(board.ReadACS), "(default)")
			writeRule := defaultIfBlank(strings.TrimSpace(board.WriteACS), "(default)")
			lines = append(lines, fmt.Sprintf("%-4d %-24s %-14s %-10s %-10s",
				board.ID,
				clampForTTY(board.Name, 24),
				clampForTTY(boardConference(board), 14),
				clampForTTY(readRule, 10),
				clampForTTY(writeRule, 10),
			))
		}
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Boards", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Board Operations", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 360)
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
		case strings.HasPrefix(upper, "CREATE "):
			payload := strings.TrimSpace(cmd[len("CREATE "):])
			parts := splitFixedParts(payload, 5)
			if strings.TrimSpace(parts[0]) == "" {
				adminPause(sess, reader, touch, "Create requires at least a board name.")
				continue
			}
			row := &domain.Board{
				Name:        strings.TrimSpace(parts[0]),
				Description: strings.TrimSpace(parts[1]),
				Conference:  strings.TrimSpace(parts[2]),
				ReadACS:     strings.TrimSpace(parts[3]),
				WriteACS:    strings.TrimSpace(parts[4]),
				CreatedBy:   actorID,
				CreatedAt:   time.Now().UTC(),
			}
			if err := s.boards.Create(row); err != nil {
				adminPause(sess, reader, touch, "Create failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, row.Name, "admin_board_create", fmt.Sprintf("board_id=%d", row.ID))
			adminPause(sess, reader, touch, "Created board "+row.Name+".")
		case strings.HasPrefix(upper, "UPDATE "):
			payload := strings.TrimSpace(cmd[len("UPDATE "):])
			parts := splitFixedParts(payload, 6)
			id, convErr := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
			if convErr != nil || id <= 0 {
				adminPause(sess, reader, touch, "Usage: UPDATE <id>|<name>|<description>|<conference>|<read_acs>|<write_acs>")
				continue
			}
			row, err := s.boards.Get(id)
			if err != nil || row == nil {
				adminPause(sess, reader, touch, "Board not found.")
				continue
			}
			row.Name = strings.TrimSpace(parts[1])
			row.Description = strings.TrimSpace(parts[2])
			row.Conference = strings.TrimSpace(parts[3])
			row.ReadACS = strings.TrimSpace(parts[4])
			row.WriteACS = strings.TrimSpace(parts[5])
			if err := s.boards.Update(row); err != nil {
				adminPause(sess, reader, touch, "Update failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, row.Name, "admin_board_update", fmt.Sprintf("board_id=%d", row.ID))
			adminPause(sess, reader, touch, "Updated board "+row.Name+".")
		case strings.HasPrefix(upper, "DELETE "):
			fields := strings.Fields(cmd)
			if len(fields) < 2 {
				adminPause(sess, reader, touch, "Usage: DELETE <id>")
				continue
			}
			id, convErr := strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 64)
			if convErr != nil || id <= 0 {
				adminPause(sess, reader, touch, "Delete requires numeric board id.")
				continue
			}
			if err := s.boards.Delete(id); err != nil {
				adminPause(sess, reader, touch, "Delete failed: "+translateRepoError(err, "board delete"))
				continue
			}
			recordAudit(s.admin, actor, strconv.FormatInt(id, 10), "admin_board_delete", "")
			adminPause(sess, reader, touch, "Deleted board "+strconv.FormatInt(id, 10)+".")
		default:
			adminPause(sess, reader, touch, "Unknown board command.")
		}
	}
}

func (s *Server) runAdminModeration(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s.msgs == nil {
		adminPause(sess, reader, touch, "Message repository is not configured.")
		return
	}
	idToHandle := map[int64]string{}
	if s.users != nil {
		if users, err := s.users.List(); err == nil {
			for _, row := range users {
				idToHandle[row.ID] = row.Handle
			}
		}
	}
	for {
		reports, err := s.msgs.ListReports(20, "open")
		if err != nil {
			adminPause(sess, reader, touch, "Could not load reports: "+err.Error())
			return
		}
		lines := []string{
			"RESOLVE <report_id> | LOCK <thread_id> | UNLOCK <thread_id> | MOVE <thread_id> <board_id> | BACK",
			"",
			fmt.Sprintf("%-4s %-5s %-12s %-28s %s", "ID", "Msg", "Reporter", "Reason", "Created"),
		}
		if len(reports) == 0 {
			lines = append(lines, "(no open reports)")
		}
		for _, row := range reports {
			reporter := idToHandle[row.ReporterID]
			if strings.TrimSpace(reporter) == "" {
				reporter = strconv.FormatInt(row.ReporterID, 10)
			}
			lines = append(lines, fmt.Sprintf("%-4d %-5d %-12s %-28s %s",
				row.ID,
				row.MessageID,
				clampForTTY(reporter, 12),
				clampForTTY(strings.TrimSpace(row.Reason), 28),
				formatClock(row.CreatedAt.UTC(), time24h),
			))
		}
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Moderation", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Moderation Queue", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 220)
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
		if len(fields) == 0 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "RESOLVE":
			if len(fields) < 2 {
				adminPause(sess, reader, touch, "Usage: RESOLVE <report_id>")
				continue
			}
			id, convErr := strconv.ParseInt(fields[1], 10, 64)
			if convErr != nil || id <= 0 {
				adminPause(sess, reader, touch, "Report id must be numeric.")
				continue
			}
			if err := s.msgs.ResolveReport(id, actor, time.Now().UTC()); err != nil {
				adminPause(sess, reader, touch, "Resolve failed: "+translateRepoError(err, "resolve"))
				continue
			}
			recordAudit(s.admin, actor, strconv.FormatInt(id, 10), "admin_report_resolve", "")
			adminPause(sess, reader, touch, "Resolved report "+strconv.FormatInt(id, 10)+".")
		case "LOCK", "UNLOCK":
			if len(fields) < 2 {
				adminPause(sess, reader, touch, "Usage: LOCK/UNLOCK <thread_id>")
				continue
			}
			threadID, convErr := strconv.ParseInt(fields[1], 10, 64)
			if convErr != nil || threadID <= 0 {
				adminPause(sess, reader, touch, "Thread id must be numeric.")
				continue
			}
			locked := strings.EqualFold(fields[0], "LOCK")
			if err := s.msgs.SetThreadLocked(threadID, locked); err != nil {
				adminPause(sess, reader, touch, "Thread lock update failed: "+err.Error())
				continue
			}
			action := "admin_thread_unlock"
			if locked {
				action = "admin_thread_lock"
			}
			recordAudit(s.admin, actor, strconv.FormatInt(threadID, 10), action, "")
			adminPause(sess, reader, touch, "Updated thread lock for "+strconv.FormatInt(threadID, 10)+".")
		case "MOVE":
			if len(fields) < 3 {
				adminPause(sess, reader, touch, "Usage: MOVE <thread_id> <board_id>")
				continue
			}
			threadID, errA := strconv.ParseInt(fields[1], 10, 64)
			boardID, errB := strconv.ParseInt(fields[2], 10, 64)
			if errA != nil || errB != nil || threadID <= 0 || boardID <= 0 {
				adminPause(sess, reader, touch, "Thread/board ids must be numeric.")
				continue
			}
			if err := s.msgs.MoveThread(threadID, boardID); err != nil {
				adminPause(sess, reader, touch, "Move failed: "+translateRepoError(err, "move thread"))
				continue
			}
			recordAudit(s.admin, actor, strconv.FormatInt(threadID, 10), "admin_thread_move", "to_board="+strconv.FormatInt(boardID, 10))
			adminPause(sess, reader, touch, "Moved thread "+strconv.FormatInt(threadID, 10)+" to board "+strconv.FormatInt(boardID, 10)+".")
		default:
			adminPause(sess, reader, touch, "Unknown moderation command.")
		}
	}
}

func (s *Server) runAdminAuditView(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	lines := []string{"Recent admin actions"}
	if s.admin != nil {
		if entries, err := s.admin.ListAudit(60); err == nil {
			if len(entries) == 0 {
				lines = append(lines, "(no audit entries)")
			}
			for _, row := range entries {
				line := fmt.Sprintf("%s %-12s %-18s %-14s %s",
					formatClock(row.CreatedAt.UTC(), time24h),
					clampForTTY(row.Actor, 12),
					clampForTTY(row.Action, 18),
					clampForTTY(row.Target, 14),
					clampForTTY(row.Details, 44),
				)
				lines = append(lines, line)
			}
		} else {
			lines = append(lines, "Audit read failed: "+err.Error())
		}
	} else {
		lines = append(lines, "Admin repository unavailable.")
	}
	writeClear(sess, ansiEnabled)
	renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Audit", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
	renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Audit Trail", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
	adminPause(sess, reader, touch, "")
}

func (s *Server) runAdminNodeView(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	lines := []string{"Node Sessions"}
	if s.admin != nil {
		nodes, err := s.admin.ListNodeSessions(16)
		if err == nil {
			if len(nodes) == 0 {
				lines = append(lines, "(no active sessions)")
			}
			for _, row := range nodes {
				lines = append(lines, fmt.Sprintf("N%-2d %-12s %-4s %-15s %-14s %s",
					row.NodeID,
					clampForTTY(row.Username, 12),
					formatOriginTag(row.RemoteAddr),
					clampForTTY(normalizeRemoteHost(row.RemoteAddr), 15),
					clampForTTY(row.Area, 14),
					formatClock(row.LastActivity.UTC(), time24h),
				))
			}
		}
		lines = append(lines, "", "Caller History")
		history, err := s.admin.ListCallerHistory(16)
		if err == nil {
			if len(history) == 0 {
				lines = append(lines, "(no caller history)")
			}
			for _, row := range history {
				lines = append(lines, fmt.Sprintf("N%-2d %-12s %-4s %-15s %-14s %s",
					row.NodeID,
					clampForTTY(row.Username, 12),
					formatOriginTag(row.RemoteAddr),
					clampForTTY(normalizeRemoteHost(row.RemoteAddr), 15),
					clampForTTY(row.Area, 14),
					formatDuration(time.Duration(row.DurationSeconds)*time.Second),
				))
			}
		}
	} else {
		lines = append(lines, "Admin repository unavailable.")
	}
	writeClear(sess, ansiEnabled)
	renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Nodes", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
	renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Node + Caller View", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
	adminPause(sess, reader, touch, "")
}

func normalizeAdminRole(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "user":
		return rbac.RoleUser
	case "mod", "moderator":
		return rbac.RoleModerator
	case "admin", "sysop":
		return rbac.RoleSysop
	default:
		return ""
	}
}

func splitFixedParts(raw string, want int) []string {
	if want <= 0 {
		return nil
	}
	parts := strings.Split(raw, "|")
	out := make([]string, want)
	for i := 0; i < want; i++ {
		if i < len(parts) {
			out[i] = parts[i]
		} else {
			out[i] = ""
		}
	}
	return out
}

func isBackCommand(raw string) bool {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "Q", "BACK", "ESC", "QUIT", "RETURN":
		return true
	default:
		return false
	}
}

func boolYN(v bool) string {
	if v {
		return "Y"
	}
	return "N"
}

func adminPause(sess gssh.Session, reader *bufio.Reader, touch func(), message string) {
	if strings.TrimSpace(message) != "" {
		io.WriteString(sess, "\r\n"+strings.TrimSpace(message)+"\r\n")
	}
	io.WriteString(sess, "Press any key.\r\n")
	_, _ = readKey(reader)
	if touch != nil {
		touch()
	}
}
