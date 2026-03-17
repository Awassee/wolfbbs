package loginserver

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/gateway"
	"wolfbbs/internal/repository"
	"wolfbbs/internal/ui"
)

type sessionServices struct {
	boards     repository.BoardRepository
	msgs       repository.MessageRepository
	mail       repository.PrivateMailRepository
	chat       *chat.Service
	offlineDir string
}

func defaultSessionServices() sessionServices {
	return sessionServices{
		offlineDir: ".wolfbbs/offline",
	}
}

func (s *sessionServices) set(boards repository.BoardRepository, msgs repository.MessageRepository, mail repository.PrivateMailRepository, chatSvc *chat.Service, offlineDir string) {
	s.boards = boards
	s.msgs = msgs
	s.mail = mail
	s.chat = chatSvc
	offlineDir = strings.TrimSpace(offlineDir)
	if offlineDir == "" {
		offlineDir = ".wolfbbs/offline"
	}
	s.offlineDir = offlineDir
}

func runBoardsCommandMode(peer textPeer, authSvc *auth.Service, user *authUser, services sessionServices) {
	if services.boards == nil || services.msgs == nil {
		_ = peer.WriteLine("Message boards are unavailable on this transport.")
		return
	}
	for {
		boards, err := services.boards.List()
		if err != nil {
			_ = peer.WriteLine("Could not list boards: " + err.Error())
			return
		}
		if len(boards) == 0 && user != nil && user.ID > 0 {
			_ = services.boards.Create(&domain.Board{Name: "General", Description: "General discussion", CreatedBy: user.ID})
			boards, _ = services.boards.List()
		}
		ensureStableBoardOrder(boards)
		_ = peer.WriteLine("Boards")
		if len(boards) == 0 {
			_ = peer.WriteLine("No boards available.")
		}
		for _, board := range boards {
			msgs, _ := services.msgs.ListByBoard(board.ID)
			_ = peer.WriteLine(fmt.Sprintf("%3d | %-22s | conf=%-12s | msgs=%d", board.ID, fallback(board.Name, "Untitled"), fallback(board.Conference, "General"), len(msgs)))
		}
		_ = peer.WriteLine("Commands: R <boardID> [msgID], P <boardID>, Q")
		raw, err := peer.ReadLine()
		if err != nil {
			return
		}
		fields := strings.Fields(strings.TrimSpace(raw))
		if len(fields) == 0 {
			continue
		}
		cmd := strings.ToUpper(fields[0])
		switch cmd {
		case "Q", "QUIT", "/QUIT":
			return
		case "R":
			if len(fields) < 2 {
				_ = peer.WriteLine("Usage: R <boardID> [msgID]")
				continue
			}
			boardID, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil || boardID <= 0 {
				_ = peer.WriteLine("Invalid board ID.")
				continue
			}
			if len(fields) >= 3 {
				msgID, msgErr := strconv.ParseInt(fields[2], 10, 64)
				if msgErr != nil || msgID <= 0 {
					_ = peer.WriteLine("Invalid message ID.")
					continue
				}
				showBoardMessage(peer, services, boardID, msgID)
				continue
			}
			listBoardMessages(peer, services, boardID)
		case "P":
			if len(fields) < 2 {
				_ = peer.WriteLine("Usage: P <boardID>")
				continue
			}
			if user == nil || user.ID <= 0 {
				_ = peer.WriteLine("Login required for posting.")
				continue
			}
			boardID, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil || boardID <= 0 {
				_ = peer.WriteLine("Invalid board ID.")
				continue
			}
			if _, err := services.boards.Get(boardID); err != nil {
				_ = peer.WriteLine("Unknown board.")
				continue
			}
			_ = peer.WriteLine("Subject:")
			subject, err := peer.ReadLine()
			if err != nil {
				return
			}
			subject = strings.TrimSpace(subject)
			if subject == "" {
				_ = peer.WriteLine("Subject is required.")
				continue
			}
			body, ok := readBodyFromPeer(peer, 4096)
			if !ok {
				return
			}
			if body == "" {
				_ = peer.WriteLine("Body is required.")
				continue
			}
			if err := services.msgs.CreateMessage(&domain.Message{
				BoardID:  boardID,
				AuthorID: user.ID,
				Subject:  subject,
				Body:     body,
			}); err != nil {
				_ = peer.WriteLine("Post failed: " + err.Error())
				continue
			}
			_ = peer.WriteLine("Post saved.")
		default:
			_ = peer.WriteLine("Unknown boards command.")
		}
	}
}

func listBoardMessages(peer textPeer, services sessionServices, boardID int64) {
	board, err := services.boards.Get(boardID)
	if err != nil {
		_ = peer.WriteLine("Unknown board.")
		return
	}
	msgs, err := services.msgs.ListByBoard(boardID)
	if err != nil {
		_ = peer.WriteLine("Could not read board messages.")
		return
	}
	_ = peer.WriteLine("Board: " + board.Name)
	if len(msgs) == 0 {
		_ = peer.WriteLine("No messages in this board.")
		return
	}
	for _, msg := range msgs {
		stamp := msg.CreatedAt.Local().Format("01-02 15:04")
		_ = peer.WriteLine(fmt.Sprintf("%4d | %-34s | %s", msg.ID, clampText(msg.Subject, 34), stamp))
	}
	_ = peer.WriteLine("Use R <boardID> <msgID> to open a full post.")
}

func showBoardMessage(peer textPeer, services sessionServices, boardID, msgID int64) {
	board, err := services.boards.Get(boardID)
	if err != nil {
		_ = peer.WriteLine("Unknown board.")
		return
	}
	msg, err := services.msgs.GetMessage(msgID)
	if err != nil || msg == nil || msg.BoardID != boardID {
		_ = peer.WriteLine("Message not found in that board.")
		return
	}
	_ = peer.WriteLine("Board: " + board.Name)
	_ = peer.WriteLine(fmt.Sprintf("Message #%d", msg.ID))
	_ = peer.WriteLine("Subject: " + msg.Subject)
	_ = peer.WriteLine("Posted: " + msg.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	_ = peer.WriteLine(strings.Repeat("-", 60))
	for _, line := range strings.Split(strings.ReplaceAll(msg.Body, "\r\n", "\n"), "\n") {
		_ = peer.WriteLine(clampText(line, 200))
	}
	_ = peer.WriteLine(strings.Repeat("-", 60))
}

func runMailCommandMode(peer textPeer, authSvc *auth.Service, user *authUser, services sessionServices) {
	if services.mail == nil {
		_ = peer.WriteLine("Private mail is unavailable on this transport.")
		return
	}
	if user == nil || user.ID <= 0 {
		_ = peer.WriteLine("Login required for mail.")
		return
	}
	for {
		inbox, _ := services.mail.ListInbox(user.ID, 20)
		outbox, _ := services.mail.ListOutbox(user.ID, 20)
		_ = peer.WriteLine("Private Mail")
		_ = peer.WriteLine("Inbox:")
		if len(inbox) == 0 {
			_ = peer.WriteLine("  (empty)")
		}
		for _, row := range inbox {
			status := "new"
			if row.ReadAt != nil {
				status = "read"
			}
			_ = peer.WriteLine(fmt.Sprintf("  %4d | %-28s | %s | %s", row.ID, clampText(row.Subject, 28), row.CreatedAt.Local().Format("01-02 15:04"), status))
		}
		_ = peer.WriteLine("Outbox:")
		if len(outbox) == 0 {
			_ = peer.WriteLine("  (empty)")
		}
		for _, row := range outbox {
			target := fmt.Sprintf("uid:%d", row.ToUserID)
			if row.ExternalTo != nil {
				target = strings.TrimSpace(*row.ExternalTo)
			}
			_ = peer.WriteLine(fmt.Sprintf("  %4d | %-22s | %s", row.ID, clampText(row.Subject, 22), clampText(target, 24)))
		}
		_ = peer.WriteLine("Commands: C compose, R <id> read, P <id> reply, D <id> delete, Q")
		raw, err := peer.ReadLine()
		if err != nil {
			return
		}
		fields := strings.Fields(strings.TrimSpace(raw))
		if len(fields) == 0 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "Q", "QUIT", "/QUIT":
			return
		case "C":
			composeMail(peer, authSvc, user, services)
		case "R":
			if len(fields) < 2 {
				_ = peer.WriteLine("Usage: R <id>")
				continue
			}
			id, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil || id <= 0 {
				_ = peer.WriteLine("Invalid mail ID.")
				continue
			}
			readMail(peer, user, services, id)
		case "P":
			if len(fields) < 2 {
				_ = peer.WriteLine("Usage: P <id>")
				continue
			}
			id, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil || id <= 0 {
				_ = peer.WriteLine("Invalid mail ID.")
				continue
			}
			replyMail(peer, user, services, id)
		case "D":
			if len(fields) < 2 {
				_ = peer.WriteLine("Usage: D <id>")
				continue
			}
			id, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil || id <= 0 {
				_ = peer.WriteLine("Invalid mail ID.")
				continue
			}
			deleteMail(peer, user, services, id)
		default:
			_ = peer.WriteLine("Unknown mail command.")
		}
	}
}

func composeMail(peer textPeer, authSvc *auth.Service, user *authUser, services sessionServices) {
	_ = peer.WriteLine("To handle or external email:")
	to, err := peer.ReadLine()
	if err != nil {
		return
	}
	to = strings.TrimSpace(to)
	if to == "" {
		_ = peer.WriteLine("Recipient is required.")
		return
	}
	_ = peer.WriteLine("Subject:")
	subject, err := peer.ReadLine()
	if err != nil {
		return
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		_ = peer.WriteLine("Subject is required.")
		return
	}
	body, ok := readBodyFromPeer(peer, 4096)
	if !ok {
		return
	}
	if body == "" {
		_ = peer.WriteLine("Body is required.")
		return
	}
	msg := &domain.PrivateMail{
		FromUserID: user.ID,
		Subject:    subject,
		Body:       body,
	}
	if strings.Contains(to, "@") {
		msg.ExternalTo = &to
	} else {
		targetUser, err := authSvc.GetUser(to)
		if err != nil || targetUser == nil {
			_ = peer.WriteLine("Unknown recipient handle.")
			return
		}
		msg.ToUserID = targetUser.ID
	}
	if err := services.mail.CreateMail(msg); err != nil {
		_ = peer.WriteLine("Compose failed: " + err.Error())
		return
	}
	_ = peer.WriteLine("Mail sent.")
}

func readMail(peer textPeer, user *authUser, services sessionServices, mailID int64) {
	row, err := services.mail.GetMail(mailID)
	if err != nil || row == nil {
		_ = peer.WriteLine("Mail not found.")
		return
	}
	if row.ToUserID != user.ID && row.FromUserID != user.ID {
		_ = peer.WriteLine("Not authorized to read that mail.")
		return
	}
	if row.ToUserID == user.ID {
		_ = services.mail.MarkRead(row.ID, time.Now().UTC())
	}
	_ = peer.WriteLine(fmt.Sprintf("Mail #%d", row.ID))
	_ = peer.WriteLine("Subject: " + row.Subject)
	_ = peer.WriteLine("Sent: " + row.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	if row.ExternalTo != nil {
		_ = peer.WriteLine("External To: " + strings.TrimSpace(*row.ExternalTo))
	}
	_ = peer.WriteLine(strings.Repeat("-", 60))
	for _, line := range strings.Split(strings.ReplaceAll(row.Body, "\r\n", "\n"), "\n") {
		_ = peer.WriteLine(clampText(line, 200))
	}
	_ = peer.WriteLine(strings.Repeat("-", 60))
}

func replyMail(peer textPeer, user *authUser, services sessionServices, mailID int64) {
	row, err := services.mail.GetMail(mailID)
	if err != nil || row == nil {
		_ = peer.WriteLine("Mail not found.")
		return
	}
	if row.ToUserID != user.ID && row.FromUserID != user.ID {
		_ = peer.WriteLine("Not authorized to reply to that mail.")
		return
	}
	reply := &domain.PrivateMail{
		FromUserID: user.ID,
		Subject:    buildReplySubject(row.Subject),
	}
	if row.ToUserID == user.ID {
		reply.ToUserID = row.FromUserID
	} else {
		reply.ToUserID = row.ToUserID
		if row.ExternalTo != nil {
			target := strings.TrimSpace(*row.ExternalTo)
			if target != "" {
				reply.ExternalTo = &target
			}
		}
	}
	if reply.ToUserID <= 0 && reply.ExternalTo == nil {
		_ = peer.WriteLine("Could not determine reply target.")
		return
	}
	_ = peer.WriteLine("Subject [" + reply.Subject + "]:")
	override, err := peer.ReadLine()
	if err != nil {
		return
	}
	override = strings.TrimSpace(override)
	if override != "" {
		reply.Subject = override
	}
	body, ok := readBodyFromPeer(peer, 4096)
	if !ok {
		return
	}
	if body == "" {
		_ = peer.WriteLine("Body is required.")
		return
	}
	reply.Body = strings.TrimSpace(body + "\n\n" + quoteForReply(row.Body))
	if err := services.mail.CreateMail(reply); err != nil {
		_ = peer.WriteLine("Reply failed: " + err.Error())
		return
	}
	_ = peer.WriteLine("Reply sent.")
}

func deleteMail(peer textPeer, user *authUser, services sessionServices, mailID int64) {
	row, err := services.mail.GetMail(mailID)
	if err != nil || row == nil {
		_ = peer.WriteLine("Mail not found.")
		return
	}
	if row.ToUserID != user.ID && row.FromUserID != user.ID {
		_ = peer.WriteLine("Not authorized to delete that mail.")
		return
	}
	if err := services.mail.DeleteMail(mailID); err != nil {
		_ = peer.WriteLine("Delete failed: " + err.Error())
		return
	}
	_ = peer.WriteLine("Mail deleted.")
}

func runChatCommandMode(peer textPeer, handle string, services sessionServices) {
	if services.chat == nil {
		_ = peer.WriteLine("Chat service is unavailable on this transport.")
		return
	}
	channel := "#lobby"
	services.chat.JoinChannel(handle, channel)
	defer services.chat.LeaveChannel(handle, channel)

	for {
		history := services.chat.History(channel, 12)
		online := services.chat.OnlineInChannel(channel)
		_ = peer.WriteLine(fmt.Sprintf("Chat %s (online=%d)", channel, len(online)))
		if len(history) == 0 {
			_ = peer.WriteLine("No messages yet.")
		}
		for _, msg := range history {
			_ = peer.WriteLine(fmt.Sprintf("[%s] %-12s %s", msg.CreatedAt.Local().Format("15:04"), clampText(msg.From, 12), clampText(msg.Body, 140)))
		}
		_ = peer.WriteLine("Commands: S <msg>, J <#channel>, O online, Q")
		raw, err := peer.ReadLine()
		if err != nil {
			return
		}
		fields := strings.Fields(strings.TrimSpace(raw))
		if len(fields) == 0 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "Q", "QUIT", "/QUIT":
			return
		case "O":
			if len(online) == 0 {
				_ = peer.WriteLine("No users online in channel.")
				continue
			}
			for _, row := range online {
				_ = peer.WriteLine(fmt.Sprintf("%-14s idle=%ds area=%s", clampText(row.Nick, 14), row.IdleSec, clampText(row.Area, 24)))
			}
		case "J":
			if len(fields) < 2 {
				_ = peer.WriteLine("Usage: J <#channel>")
				continue
			}
			next := chat.NormalizeChannel(fields[1])
			if next == "" {
				next = "#lobby"
			}
			services.chat.LeaveChannel(handle, channel)
			channel = next
			services.chat.JoinChannel(handle, channel)
		case "S":
			msg := strings.TrimSpace(strings.TrimPrefix(raw, fields[0]))
			if msg == "" {
				_ = peer.WriteLine("Message:")
				line, err := peer.ReadLine()
				if err != nil {
					return
				}
				msg = strings.TrimSpace(line)
			}
			if msg == "" {
				_ = peer.WriteLine("Empty message.")
				continue
			}
			if _, err := services.chat.Post(handle, channel, msg); err != nil {
				_ = peer.WriteLine("Send blocked: " + err.Error())
				continue
			}
			_ = peer.WriteLine("Message sent.")
		default:
			_ = peer.WriteLine("Unknown chat command.")
		}
	}
}

func runGatewayCommandMode(peer textPeer, handle string, services sessionServices) {
	emailGateway := gateway.NewEmailGateway(gateway.LoadEmailConfigFromEnv())
	for {
		_ = peer.WriteLine("Gateway commands: W <url> fetch, O <url> save-offline, E <email> send, Q")
		raw, err := peer.ReadLine()
		if err != nil {
			return
		}
		fields := strings.Fields(strings.TrimSpace(raw))
		if len(fields) == 0 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "Q", "QUIT", "/QUIT":
			return
		case "W":
			if len(fields) < 2 {
				_ = peer.WriteLine("Usage: W <url>")
				continue
			}
			url := strings.TrimSpace(fields[1])
			if url == "" {
				_ = peer.WriteLine("URL required.")
				continue
			}
			text, err := gateway.FetchText(context.Background(), url, gateway.DefaultFetchConfig)
			if err != nil {
				_ = peer.WriteLine("Gateway blocked: " + err.Error())
				continue
			}
			_ = peer.WriteLine("Fetched. Showing first 120 lines:")
			writeMultiline(peer, clampLines(text, 120))
		case "O":
			if len(fields) < 2 {
				_ = peer.WriteLine("Usage: O <url>")
				continue
			}
			url := strings.TrimSpace(fields[1])
			if url == "" {
				_ = peer.WriteLine("URL required.")
				continue
			}
			text, err := gateway.FetchText(context.Background(), url, gateway.DefaultFetchConfig)
			if err != nil {
				_ = peer.WriteLine("Gateway blocked: " + err.Error())
				continue
			}
			path, err := gateway.SaveOffline(services.offlineDir, handle, url, text)
			if err != nil {
				_ = peer.WriteLine("Save failed: " + err.Error())
				continue
			}
			_ = peer.WriteLine("Saved: " + filepath.Clean(path))
		case "E":
			if len(fields) < 2 {
				_ = peer.WriteLine("Usage: E <email>")
				continue
			}
			if !emailGateway.Enabled() {
				_ = peer.WriteLine("SMTP relay is not configured.")
				continue
			}
			to := strings.TrimSpace(fields[1])
			if to == "" {
				_ = peer.WriteLine("Recipient email required.")
				continue
			}
			_ = peer.WriteLine("Subject:")
			subject, err := peer.ReadLine()
			if err != nil {
				return
			}
			subject = strings.TrimSpace(subject)
			if subject == "" {
				_ = peer.WriteLine("Subject is required.")
				continue
			}
			body, ok := readBodyFromPeer(peer, 16384)
			if !ok {
				return
			}
			if body == "" {
				_ = peer.WriteLine("Body is required.")
				continue
			}
			if err := emailGateway.SendOutbound(handle, []string{to}, subject, body); err != nil {
				_ = peer.WriteLine("Email send failed: " + err.Error())
				continue
			}
			_ = peer.WriteLine("Email queued.")
		default:
			_ = peer.WriteLine("Unknown gateway command.")
		}
	}
}

func runSettingsCommandMode(peer textPeer, authSvc *auth.Service, handle string) {
	if authSvc == nil {
		_ = peer.WriteLine("Settings unavailable.")
		return
	}
	user, err := authSvc.GetUser(handle)
	if err != nil || user == nil {
		_ = peer.WriteLine("Could not load account settings.")
		return
	}
	theme := strings.TrimSpace(user.Theme)
	if theme == "" {
		theme = ui.ThemeNames()[0]
	}
	ansiEnabled := user.ANSIEnabled
	pagingEnabled := user.PagingEnabled
	time24h := user.TimeFormat24h
	themeNames := ui.ThemeNames()

	for {
		_ = peer.WriteLine("Settings")
		_ = peer.WriteLine("Theme: " + theme)
		_ = peer.WriteLine("ANSI enabled: " + boolText(ansiEnabled))
		_ = peer.WriteLine("Paging enabled: " + boolText(pagingEnabled))
		_ = peer.WriteLine("24-hour clock: " + boolText(time24h))
		_ = peer.WriteLine("Commands: T cycle theme, A ansi, P paging, C clock, S save, Q")
		raw, err := peer.ReadLine()
		if err != nil {
			return
		}
		switch strings.ToUpper(strings.TrimSpace(raw)) {
		case "Q", "QUIT", "/QUIT":
			return
		case "T":
			theme = nextTheme(themeNames, theme)
		case "A":
			ansiEnabled = !ansiEnabled
		case "P":
			pagingEnabled = !pagingEnabled
		case "C":
			time24h = !time24h
		case "S":
			if err := authSvc.SetPreferences(handle, theme, ansiEnabled, pagingEnabled, time24h); err != nil {
				_ = peer.WriteLine("Save failed: " + err.Error())
			} else {
				_ = peer.WriteLine("Settings saved.")
			}
		default:
			_ = peer.WriteLine("Unknown settings command.")
		}
	}
}

func readBodyFromPeer(peer textPeer, maxChars int) (string, bool) {
	if maxChars <= 0 {
		maxChars = 4096
	}
	_ = peer.WriteLine("Body: end with '.' on a line by itself")
	lines := make([]string, 0, 16)
	size := 0
	for {
		line, err := peer.ReadLine()
		if err != nil {
			return "", false
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "." {
			break
		}
		if size+len(line) > maxChars {
			continue
		}
		size += len(line)
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), true
}

func writeMultiline(peer textPeer, text string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for _, line := range strings.Split(text, "\n") {
		_ = peer.WriteLine(clampText(line, 220))
	}
}

func clampLines(text string, maxLines int) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if maxLines <= 0 || len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:maxLines], "\n") + "\n...[truncated]"
}

func nextTheme(themes []string, current string) string {
	if len(themes) == 0 {
		return "retro-amber"
	}
	idx := 0
	for i, row := range themes {
		if strings.EqualFold(strings.TrimSpace(row), strings.TrimSpace(current)) {
			idx = i
			break
		}
	}
	return themes[(idx+1)%len(themes)]
}

func quoteForReply(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, "> "+line)
	}
	return strings.Join(out, "\n")
}

func buildReplySubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "Re: (no subject)"
	}
	if strings.HasPrefix(strings.ToLower(subject), "re:") {
		return subject
	}
	return "Re: " + subject
}

func boolText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func clampText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func ensureStableBoardOrder(rows []domain.Board) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ID == rows[j].ID {
			return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
		}
		return rows[i].ID < rows[j].ID
	})
}
