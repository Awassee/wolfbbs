package sshserver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"wolfbbs/internal/auth"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/ui"
)

const (
	sshSettingFeaturedCollections = "web.file_featured_collections"
	sshSettingBoardSubscriptions  = "web.board_subscriptions."
	sshSettingBoardWatch          = "web.board_watch."
	sshMaxHandleSuggestions       = 8
	sshMaxFeaturedCollectionFiles = 40
	sshMaxFeaturedCollectionTags  = 8
)

type sshFeaturedFileCollection struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	FileIDs     []int64   `json:"file_ids,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Curator     string    `json:"curator,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type sshOfflinePacket struct {
	GeneratedAt string                  `json:"generated_at"`
	Handle      string                  `json:"handle"`
	Boards      []sshOfflinePacketBoard `json:"boards"`
}

type sshOfflinePacketBoard struct {
	BoardID     int64                     `json:"board_id"`
	Name        string                    `json:"name"`
	Conference  string                    `json:"conference,omitempty"`
	MessageRows []sshOfflinePacketMessage `json:"messages"`
}

type sshOfflinePacketMessage struct {
	ID        int64  `json:"id"`
	Subject   string `json:"subject"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
	Body      string `json:"body"`
}

type sshOfflineMailReply struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Urgency string `json:"urgency,omitempty"`
}

type sshOfflineMailReplyPayload struct {
	Replies []sshOfflineMailReply `json:"replies"`
}

func sshHandleSettingKey(root, handle string) string {
	handle = strings.ToLower(strings.TrimSpace(handle))
	if handle == "" {
		return ""
	}
	return root + handle
}

func normalizeCollectionTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, raw := range tags {
		tag := strings.ToLower(strings.TrimSpace(raw))
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
		if len(out) >= sshMaxFeaturedCollectionTags {
			break
		}
	}
	return out
}

func normalizeCollectionFileIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := map[int64]struct{}{}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
		if len(out) >= sshMaxFeaturedCollectionFiles {
			break
		}
	}
	return out
}

func normalizeSSHFeaturedCollections(rows []sshFeaturedFileCollection) []sshFeaturedFileCollection {
	out := make([]sshFeaturedFileCollection, 0, len(rows))
	for _, row := range rows {
		row.ID = strings.TrimSpace(row.ID)
		row.Title = strings.TrimSpace(row.Title)
		row.Description = strings.TrimSpace(row.Description)
		row.Curator = strings.ToLower(strings.TrimSpace(row.Curator))
		row.Tags = normalizeCollectionTags(row.Tags)
		row.FileIDs = normalizeCollectionFileIDs(row.FileIDs)
		if row.ID == "" || row.Title == "" {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *Server) loadFeaturedCollectionsSSH() []sshFeaturedFileCollection {
	if s == nil || s.admin == nil {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(sshSettingFeaturedCollections)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []sshFeaturedFileCollection
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	return normalizeSSHFeaturedCollections(rows)
}

func collectionSummaryLine(collection sshFeaturedFileCollection, files map[int64]domain.FileEntry) string {
	names := make([]string, 0, 3)
	for _, fileID := range collection.FileIDs {
		entry, ok := files[fileID]
		if !ok {
			continue
		}
		names = append(names, entry.Name)
		if len(names) >= 3 {
			break
		}
	}
	if len(names) == 0 {
		return "No visible files matched yet."
	}
	return strings.Join(names, ", ")
}

func (s *Server) collectionFiles(collection sshFeaturedFileCollection) []domain.FileEntry {
	if s == nil || s.admin == nil {
		return nil
	}
	rows := make([]domain.FileEntry, 0, len(collection.FileIDs))
	for _, fileID := range collection.FileIDs {
		entry, err := s.admin.GetFileEntry(fileID)
		if err != nil || entry == nil {
			continue
		}
		rows = append(rows, *entry)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UploadedAt.Equal(rows[j].UploadedAt) {
			return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
		}
		return rows[i].UploadedAt.After(rows[j].UploadedAt)
	})
	return rows
}

func (s *Server) runFeaturedCollectionDetail(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, account *domain.User, collection sshFeaturedFileCollection, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if touch == nil {
		touch = func() {}
	}
	areas, _ := s.admin.ListFileAreas()
	areaNames := map[int64]string{}
	for _, row := range areas {
		areaNames[row.ID] = row.Name
	}
	for {
		files := s.collectionFiles(collection)
		lines := []string{
			defaultIfBlank(collection.Description, "Curated bundle for guided file browsing."),
			"Curator: " + defaultIfBlank(collection.Curator, "system"),
			"Tags: " + defaultIfBlank(strings.Join(collection.Tags, ", "), "auto"),
			"",
			indexedFilesHeader(renderWidth),
			indexedFilesDivider(renderWidth),
		}
		if len(files) == 0 {
			lines = append(lines, "No visible files are attached to this collection yet.")
		} else {
			for _, row := range files {
				lines = append(lines, formatIndexedFileRow(renderWidth, row.ID, areaNames[row.AreaID], row.Name, row.RatingAvg, strings.Join(row.Tags, ",")))
			}
		}
		if account != nil && account.ID > 0 {
			lines = append(lines, "", "Commands: A <fileID> queue add  Q return")
		} else {
			lines = append(lines, "", "Commands: Q return")
		}
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Collection: "+collection.Title, handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, collection.Title, lines, ui.CP437Box, ui.FgCyan, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 40)
		if err != nil {
			return
		}
		touch()
		cmd := strings.TrimSpace(raw)
		if cmd == "" {
			continue
		}
		if strings.EqualFold(cmd, "q") || strings.EqualFold(cmd, "back") {
			return
		}
		if account != nil && account.ID > 0 {
			parts := strings.Fields(cmd)
			if len(parts) == 2 && strings.EqualFold(parts[0], "a") {
				fileID, parseErr := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
				if parseErr != nil || fileID <= 0 {
					adminPause(sess, reader, touch, "Use A <fileID> to add a visible file to the queue.")
					continue
				}
				if err := s.admin.EnqueueDownload(account.ID, fileID); err != nil {
					adminPause(sess, reader, touch, "Queue add failed: "+err.Error())
					continue
				}
				adminPause(sess, reader, touch, "Queued file ID "+strconv.FormatInt(fileID, 10)+".")
				continue
			}
		}
		adminPause(sess, reader, touch, "Use Q to return.")
	}
}

func (s *Server) runFeaturedCollections(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, account *domain.User, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s == nil || s.admin == nil {
		adminPause(sess, reader, touch, "Collections are unavailable.")
		return
	}
	if touch == nil {
		touch = func() {}
	}
	for {
		collections := s.loadFeaturedCollectionsSSH()
		allFiles, _ := s.admin.ListFileEntries(0, "", nil, 400)
		fileByID := map[int64]domain.FileEntry{}
		for _, row := range allFiles {
			fileByID[row.ID] = row
		}
		lines := []string{
			"Curated file bundles parity for /collections",
			"",
		}
		if len(collections) == 0 {
			lines = append(lines, "No featured collections are configured yet.")
		} else {
			for idx, row := range collections {
				tagLine := defaultIfBlank(strings.Join(row.Tags, ", "), "auto")
				lines = append(lines, fmt.Sprintf("%d) %s", idx+1, clampForTTY(row.Title, renderWidth-6)))
				lines = append(lines, "   tags: "+clampForTTY(tagLine, renderWidth-10))
				lines = append(lines, "   files: "+clampForTTY(collectionSummaryLine(row, fileByID), renderWidth-11))
			}
		}
		lines = append(lines, "", "Commands: V <index> view collection  Q return")
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Featured Collections", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Collections", lines, ui.CP437Box, ui.FgCyan, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 32)
		if err != nil {
			return
		}
		touch()
		cmd := strings.TrimSpace(raw)
		if cmd == "" {
			continue
		}
		if strings.EqualFold(cmd, "q") || strings.EqualFold(cmd, "back") {
			return
		}
		parts := strings.Fields(cmd)
		if len(parts) == 2 && strings.EqualFold(parts[0], "v") {
			index, parseErr := strconv.Atoi(strings.TrimSpace(parts[1]))
			if parseErr != nil || index < 1 || index > len(collections) {
				adminPause(sess, reader, touch, "Use V <index> for a visible collection.")
				continue
			}
			s.runFeaturedCollectionDetail(sess, reader, termWidth, renderWidth, handle, account, collections[index-1], th, ansiEnabled, encoding, time24h, nodeLabel, touch)
			continue
		}
		adminPause(sess, reader, touch, "Use V <index> or Q.")
	}
}

func (s *Server) loadOfflineBoardSelection(handle string) map[int64]bool {
	selected := map[int64]bool{}
	if s == nil || s.admin == nil {
		return selected
	}
	if key := sshHandleSettingKey(sshSettingBoardSubscriptions, handle); key != "" {
		raw, err := s.admin.GetSystemSetting(key)
		if err == nil && strings.TrimSpace(raw) != "" {
			decoded := map[string]string{}
			if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
				for rawID, rawMode := range decoded {
					boardID, parseErr := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
					mode := strings.ToLower(strings.TrimSpace(rawMode))
					if parseErr != nil || boardID <= 0 {
						continue
					}
					if mode == "watch" || mode == "digest" {
						selected[boardID] = true
					}
				}
			}
		}
	}
	if len(selected) > 0 {
		return selected
	}
	if key := sshHandleSettingKey(sshSettingBoardWatch, handle); key != "" {
		raw, err := s.admin.GetSystemSetting(key)
		if err == nil && strings.TrimSpace(raw) != "" {
			var ids []int64
			if err := json.Unmarshal([]byte(raw), &ids); err == nil {
				for _, id := range ids {
					if id > 0 {
						selected[id] = true
					}
				}
			}
		}
	}
	return selected
}

func (s *Server) canReadBoardForUser(user *domain.User, board domain.Board) bool {
	if user == nil {
		return false
	}
	acsStrict := envBool(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_STRICT")))
	return evaluateAccess(strings.TrimSpace(boardReadRule(board)), user, user.Handle, map[string]string{
		"area":     "boards",
		"board_id": strconv.FormatInt(board.ID, 10),
	}, acsStrict, s.logger)
}

func (s *Server) offlineHandleLookup() map[int64]string {
	out := map[int64]string{}
	if s == nil || s.users == nil {
		return out
	}
	users, err := s.users.List()
	if err != nil {
		return out
	}
	for _, row := range users {
		out[row.ID] = row.Handle
	}
	return out
}

func (s *Server) buildOfflineBoardPacket(user *domain.User, limitBoards, limitMessages int) sshOfflinePacket {
	packet := sshOfflinePacket{GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if s == nil || user == nil || s.boards == nil || s.msgs == nil {
		return packet
	}
	packet.Handle = user.Handle
	if limitBoards <= 0 {
		limitBoards = 8
	}
	if limitMessages <= 0 {
		limitMessages = 10
	}
	boards, err := s.boards.List()
	if err != nil {
		return packet
	}
	sort.Slice(boards, func(i, j int) bool { return boards[i].ID < boards[j].ID })
	selected := s.loadOfflineBoardSelection(user.Handle)
	handles := s.offlineHandleLookup()
	selectedBoards := make([]domain.Board, 0, len(boards))
	for _, board := range boards {
		if !s.canReadBoardForUser(user, board) {
			continue
		}
		if len(selected) > 0 && !selected[board.ID] {
			continue
		}
		selectedBoards = append(selectedBoards, board)
	}
	if len(selectedBoards) == 0 {
		for _, board := range boards {
			if !s.canReadBoardForUser(user, board) {
				continue
			}
			selectedBoards = append(selectedBoards, board)
			if len(selectedBoards) >= limitBoards {
				break
			}
		}
	}
	for _, board := range selectedBoards {
		msgs, err := s.msgs.ListByBoard(board.ID)
		if err != nil || len(msgs) == 0 {
			continue
		}
		sort.Slice(msgs, func(i, j int) bool { return msgs[i].CreatedAt.After(msgs[j].CreatedAt) })
		rows := make([]sshOfflinePacketMessage, 0, limitMessages)
		for _, msg := range msgs {
			rows = append(rows, sshOfflinePacketMessage{
				ID:        msg.ID,
				Subject:   clampForTTY(strings.TrimSpace(msg.Subject), 96),
				Author:    defaultIfBlank(strings.TrimSpace(handles[msg.AuthorID]), "unknown"),
				CreatedAt: msg.CreatedAt.UTC().Format(time.RFC3339),
				Body:      msg.Body,
			})
			if len(rows) >= limitMessages {
				break
			}
		}
		if len(rows) == 0 {
			continue
		}
		packet.Boards = append(packet.Boards, sshOfflinePacketBoard{
			BoardID:     board.ID,
			Name:        board.Name,
			Conference:  boardConference(board),
			MessageRows: rows,
		})
		if len(packet.Boards) >= limitBoards {
			break
		}
	}
	return packet
}

func renderSSHOfflinePacketText(packet sshOfflinePacket) string {
	var out strings.Builder
	out.WriteString("WolfBBS Offline Packet\n")
	out.WriteString("Generated: " + packet.GeneratedAt + "\n")
	out.WriteString("Handle: " + packet.Handle + "\n\n")
	for _, board := range packet.Boards {
		out.WriteString("== [" + defaultIfBlank(strings.TrimSpace(board.Conference), "General") + "] " + board.Name + " ==\n")
		for _, msg := range board.MessageRows {
			out.WriteString("#" + strconv.FormatInt(msg.ID, 10) + " " + msg.Subject + "\n")
			out.WriteString("From: " + defaultIfBlank(msg.Author, "unknown") + " @ " + msg.CreatedAt + "\n")
			out.WriteString(msg.Body + "\n\n")
		}
	}
	if len(packet.Boards) == 0 {
		out.WriteString("No watched or digest boards have packet content yet.\n")
	}
	return out.String()
}

func parseSSHOfflineReplyPayload(raw string) ([]sshOfflineMailReply, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("payload is required")
	}
	var payload sshOfflineMailReplyPayload
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &payload.Replies); err != nil {
			return nil, err
		}
	} else {
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, err
		}
	}
	if len(payload.Replies) == 0 {
		return nil, fmt.Errorf("no replies found")
	}
	return payload.Replies, nil
}

func (s *Server) importOfflineMailReplies(user *domain.User, payload string) (int, error) {
	if s == nil || user == nil || s.mail == nil || s.auth == nil {
		return 0, fmt.Errorf("offline mail import is unavailable")
	}
	replies, err := parseSSHOfflineReplyPayload(payload)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range replies {
		to := strings.TrimSpace(row.To)
		subject := strings.TrimSpace(row.Subject)
		body := strings.TrimSpace(row.Body)
		if to == "" || subject == "" || body == "" {
			continue
		}
		target, err := s.auth.GetUser(to)
		if err != nil || target == nil || !target.Enabled || target.Banned {
			continue
		}
		if err := s.mail.CreateMail(&domain.PrivateMail{
			FromUserID: user.ID,
			ToUserID:   target.ID,
			Subject:    subject,
			Body:       body,
		}); err != nil {
			continue
		}
		count++
	}
	if count == 0 {
		return 0, fmt.Errorf("no valid local replies imported")
	}
	return count, nil
}

func readDotTerminatedBlock(reader *bufio.Reader, maxLines, maxChars int) (string, error) {
	if maxLines <= 0 {
		maxLines = 120
	}
	if maxChars <= 0 {
		maxChars = 16384
	}
	lines := make([]string, 0, maxLines)
	total := 0
	for i := 0; i < maxLines; i++ {
		line, err := readLine(reader, 512)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(line) == "." {
			break
		}
		total += len(line)
		if total > maxChars {
			break
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func saveOfflineArtifact(handle, kind, ext string, body []byte) (string, error) {
	handleKey := profileNormalizeHandleKey(handle)
	if handleKey == "" {
		handleKey = "caller"
	}
	dir := filepath.Join(profileExportRootDir(), handleKey)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("wolfbbs-%s-%s.%s", kind, time.Now().UTC().Format("20060102-150405"), ext)
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func promptSaveOfflineArtifact(out io.Writer, reader *bufio.Reader, handle, kind, ext string, body []byte) (string, error) {
	_, _ = io.WriteString(out, "\r\nSave artifact to offline path? (Y/N): ")
	choice, err := readLine(reader, 8)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(strings.TrimSpace(choice), "y") && !strings.EqualFold(strings.TrimSpace(choice), "yes") {
		return "", nil
	}
	return saveOfflineArtifact(handle, kind, ext, body)
}

func (s *Server) runOfflineCenter(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, account *domain.User, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s == nil || s.auth == nil {
		adminPause(sess, reader, touch, "Offline center is unavailable.")
		return
	}
	if touch == nil {
		touch = func() {}
	}
	user := account
	if user == nil || user.ID <= 0 {
		loaded, err := s.auth.GetUser(handle)
		if err == nil && loaded != nil {
			user = loaded
		}
	}
	if user == nil || user.ID <= 0 {
		adminPause(sess, reader, touch, "Offline center requires a signed-in account.")
		return
	}
	for {
		packet := s.buildOfflineBoardPacket(user, 8, 10)
		lines := []string{
			"Offline packet parity for /offline",
			fmt.Sprintf("Packet owner: %s", profileNormalizeHandleKey(user.Handle)),
			fmt.Sprintf("Boards in packet: %d", len(packet.Boards)),
			"",
		}
		if len(packet.Boards) == 0 {
			lines = append(lines, "No watched or digest boards are ready for an offline packet yet.")
		} else {
			for _, board := range packet.Boards {
				lines = append(lines, fmt.Sprintf("- %s (%d messages)", clampForTTY(board.Name, renderWidth-22), len(board.MessageRows)))
			}
		}
		lines = append(lines, "", "Commands: J export JSON  T export text  I import mail replies  Q return")
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Offline Center", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Offline", lines, ui.CP437Box, ui.FgCyan, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Selection: ")
		key, err := readKey(reader)
		if err != nil {
			return
		}
		touch()
		switch key {
		case "Q", "ESC":
			return
		case "J":
			body, err := json.MarshalIndent(packet, "", "  ")
			if err != nil {
				adminPause(sess, reader, touch, "Could not encode offline packet JSON: "+err.Error())
				continue
			}
			writeClear(sess, ansiEnabled)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Offline Packet JSON", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
			pagerWrite(sess, reader, string(body))
			path, saveErr := promptSaveOfflineArtifact(sess, reader, handle, "watched", "json", body)
			if saveErr != nil {
				adminPause(sess, reader, touch, "Could not save JSON packet: "+saveErr.Error())
				continue
			}
			if path != "" {
				adminPause(sess, reader, touch, "Saved JSON packet: "+path)
			}
		case "T":
			body := []byte(renderSSHOfflinePacketText(packet))
			writeClear(sess, ansiEnabled)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Offline Packet Text", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
			pagerWrite(sess, reader, string(body))
			path, saveErr := promptSaveOfflineArtifact(sess, reader, handle, "watched", "txt", body)
			if saveErr != nil {
				adminPause(sess, reader, touch, "Could not save text packet: "+saveErr.Error())
				continue
			}
			if path != "" {
				adminPause(sess, reader, touch, "Saved text packet: "+path)
			}
		case "I":
			writeClear(sess, ansiEnabled)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Offline Mail Import", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
			_, _ = io.WriteString(sess, "Paste JSON reply payload, end with '.' on its own line.\r\n")
			payload, err := readDotTerminatedBlock(reader, 120, 16384)
			if err != nil {
				return
			}
			touch()
			count, importErr := s.importOfflineMailReplies(user, payload)
			if importErr != nil {
				adminPause(sess, reader, touch, importErr.Error())
				continue
			}
			adminPause(sess, reader, touch, fmt.Sprintf("Imported %d offline mail replie(s).", count))
		default:
			adminPause(sess, reader, touch, "Use J, T, I, or Q.")
		}
	}
}

func (s *Server) suggestHandles(query, exclude string, limit int) []string {
	if s == nil || s.auth == nil {
		return nil
	}
	if limit <= 0 {
		limit = sshMaxHandleSuggestions
	}
	users, err := s.auth.ListUsers()
	if err != nil {
		return nil
	}
	query = strings.ToLower(strings.TrimSpace(query))
	exclude = strings.ToLower(strings.TrimSpace(exclude))
	prefix := make([]string, 0, limit)
	contains := make([]string, 0, limit)
	for _, row := range users {
		if !row.Enabled || row.Banned {
			continue
		}
		handle := strings.TrimSpace(row.Handle)
		if handle == "" {
			continue
		}
		lower := strings.ToLower(handle)
		if lower == exclude {
			continue
		}
		if query == "" {
			prefix = append(prefix, handle)
			continue
		}
		if strings.HasPrefix(lower, query) {
			prefix = append(prefix, handle)
			continue
		}
		if strings.Contains(lower, query) {
			contains = append(contains, handle)
		}
	}
	sort.Slice(prefix, func(i, j int) bool { return strings.ToLower(prefix[i]) < strings.ToLower(prefix[j]) })
	sort.Slice(contains, func(i, j int) bool { return strings.ToLower(contains[i]) < strings.ToLower(contains[j]) })
	out := append(prefix, contains...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *Server) runHandleSuggestions(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle, query string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if touch == nil {
		touch = func() {}
	}
	rows := s.suggestHandles(query, handle, sshMaxHandleSuggestions)
	lines := []string{
		"Handle assist parity for /handles/suggest",
		"Query: " + defaultIfBlank(strings.TrimSpace(query), "(all handles)"),
		"",
	}
	if len(rows) == 0 {
		lines = append(lines, "No matching handles.")
	} else {
		for idx, row := range rows {
			lines = append(lines, fmt.Sprintf("%d) %s", idx+1, row))
		}
	}
	lines = append(lines, "", "Press any key to return.")
	writeClear(sess, ansiEnabled)
	renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Handle Suggestions", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
	renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Handle Assist", lines, ui.CP437Box, ui.FgCyan, ui.BgBlack), ansiEnabled, encoding)
	_, _ = readKey(reader)
	touch()
}

func (s *Server) promptRecipient(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) (string, error) {
	if touch == nil {
		touch = func() {}
	}
	for {
		_, _ = io.WriteString(sess, "To handle or external email: ")
		to, err := readLine(reader, 128)
		if err != nil {
			return "", err
		}
		touch()
		to = strings.TrimSpace(to)
		if strings.HasPrefix(to, "?") {
			s.runHandleSuggestions(sess, reader, termWidth, renderWidth, handle, strings.TrimSpace(strings.TrimPrefix(to, "?")), th, ansiEnabled, encoding, time24h, nodeLabel, touch)
			continue
		}
		return to, nil
	}
}

func (s *Server) runPasswordResetFlow(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s == nil || s.auth == nil {
		adminPause(sess, reader, touch, "Password reset is unavailable.")
		return
	}
	if touch == nil {
		touch = func() {}
	}
	for {
		lines := []string{
			"Terminal parity for /reset/request and /reset/complete",
			"",
			"[R] Request reset token",
			"[C] Complete password reset",
			"[Q] Return to login",
			"",
			"Terminal reset prints the token directly so you can complete the flow here.",
		}
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Password Reset", "Guest", time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Password Reset", lines, ui.CP437Box, ui.FgGreen, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Selection: ")
		key, err := readKey(reader)
		if err != nil {
			return
		}
		touch()
		switch key {
		case "Q", "ESC":
			return
		case "R":
			_, _ = io.WriteString(sess, "\r\nHandle: ")
			handle, err := readLine(reader, 64)
			if err != nil {
				return
			}
			touch()
			handle = strings.TrimSpace(handle)
			token, issueErr := s.auth.IssuePasswordReset(handle, 30*time.Minute)
			if issueErr == auth.ErrResetNotEnabled {
				adminPause(sess, reader, touch, "Password reset is not enabled.")
				continue
			}
			if issueErr != nil && issueErr != auth.ErrInvalidCredentials {
				adminPause(sess, reader, touch, "Password reset is currently unavailable.")
				continue
			}
			message := "If the account exists, a password reset token has been issued."
			if token != "" {
				message = message + "\nToken: " + token + "\nUse Complete Reset to set the new password."
			}
			adminPause(sess, reader, touch, message)
		case "C":
			_, _ = io.WriteString(sess, "\r\nReset token: ")
			token, err := readLine(reader, 128)
			if err != nil {
				return
			}
			touch()
			_, _ = io.WriteString(sess, "New password: ")
			password, err := readLine(reader, 128)
			if err != nil {
				return
			}
			touch()
			if err := s.auth.ResetPasswordWithToken(strings.TrimSpace(token), strings.TrimSpace(password)); err != nil {
				adminPause(sess, reader, touch, "Reset token is invalid/expired or password is too short.")
				continue
			}
			adminPause(sess, reader, touch, "Password reset complete. Return to login with the new password.")
			return
		default:
			adminPause(sess, reader, touch, "Use R, C, or Q.")
		}
	}
}
