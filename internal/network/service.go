package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

const (
	FormatFTN     = "ftn"
	FormatBSO     = "bso"
	FormatQWK     = "qwk"
	FormatNetmail = "netmail"
)

type PacketMessage struct {
	BoardID    int64  `json:"board_id,omitempty"`
	Board      string `json:"board,omitempty"`
	Conference string `json:"conference,omitempty"`
	FromUserID int64  `json:"from_user_id,omitempty"`
	ToUserID   int64  `json:"to_user_id,omitempty"`
	ToHandle   string `json:"to_handle,omitempty"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	ParentID   int64  `json:"parent_id,omitempty"`
	ThreadID   int64  `json:"thread_id,omitempty"`
}

type Packet struct {
	Version   int             `json:"version"`
	Format    string          `json:"format"`
	Exported  time.Time       `json:"exported_at"`
	SourceBBS string          `json:"source_bbs,omitempty"`
	Messages  []PacketMessage `json:"messages"`
}

type Status struct {
	SpoolDir        string `json:"spool_dir"`
	InboundPackets  int    `json:"inbound_packets"`
	OutboundPackets int    `json:"outbound_packets"`
}

type Service struct {
	spoolDir  string
	boards    repository.BoardRepository
	messages  repository.MessageRepository
	users     repository.UserRepository
	mail      repository.PrivateMailRepository
	source    string
	importCmd string
	exportCmd string
	boardMap  map[string]int64
	handleMap map[string]string
}

func NewService(spoolDir string, boards repository.BoardRepository, messages repository.MessageRepository, users repository.UserRepository, mail repository.PrivateMailRepository) *Service {
	spoolDir = strings.TrimSpace(spoolDir)
	if spoolDir == "" {
		spoolDir = ".wolfbbs/network"
	}
	return &Service{
		spoolDir:  spoolDir,
		boards:    boards,
		messages:  messages,
		users:     users,
		mail:      mail,
		source:    "WolfBBS",
		importCmd: strings.TrimSpace(os.Getenv("WOLFBBS_NET_IMPORT_CMD")),
		exportCmd: strings.TrimSpace(os.Getenv("WOLFBBS_NET_EXPORT_CMD")),
		boardMap:  parseBoardRoutes(strings.TrimSpace(os.Getenv("WOLFBBS_NET_BOARD_ROUTES"))),
		handleMap: parseHandleRoutes(strings.TrimSpace(os.Getenv("WOLFBBS_NET_HANDLE_ROUTES"))),
	}
}

func (s *Service) SetExternalCommands(importCmd, exportCmd string) {
	s.importCmd = strings.TrimSpace(importCmd)
	s.exportCmd = strings.TrimSpace(exportCmd)
}

func (s *Service) SetBoardRoutes(routes map[string]int64) {
	s.boardMap = cloneBoardRoutes(routes)
}

func (s *Service) SetHandleRoutes(routes map[string]string) {
	s.handleMap = cloneHandleRoutes(routes)
}

func normalizeFormat(format string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case FormatFTN, FormatBSO, FormatQWK, FormatNetmail:
		return format, nil
	default:
		return "", fmt.Errorf("unsupported format %q", format)
	}
}

func (s *Service) ensureDir(parts ...string) (string, error) {
	base := s.spoolDir
	path := filepath.Join(append([]string{base}, parts...)...)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Service) ExportBoard(format string, boardID int64) (string, error) {
	if s.messages == nil {
		return "", errors.New("message repository is required")
	}
	if boardID <= 0 {
		return "", errors.New("board id is required")
	}
	format, err := normalizeFormat(format)
	if err != nil {
		return "", err
	}
	if format == FormatNetmail {
		return "", errors.New("netmail export uses QueueNetmail")
	}
	if s.boards != nil {
		if _, err := s.boards.Get(boardID); err != nil {
			return "", err
		}
	}
	boardName := ""
	conference := ""
	if s.boards != nil {
		if board, err := s.boards.Get(boardID); err == nil && board != nil {
			boardName = strings.TrimSpace(board.Name)
			conference = strings.TrimSpace(board.Conference)
		}
	}
	msgs, err := s.messages.ListByBoard(boardID)
	if err != nil {
		return "", err
	}
	p := Packet{
		Version:   1,
		Format:    format,
		Exported:  time.Now().UTC(),
		SourceBBS: s.source,
		Messages:  make([]PacketMessage, 0, len(msgs)),
	}
	for _, msg := range msgs {
		p.Messages = append(p.Messages, PacketMessage{
			BoardID:    msg.BoardID,
			Board:      boardName,
			Conference: conference,
			FromUserID: msg.AuthorID,
			Subject:    strings.TrimSpace(msg.Subject),
			Body:       strings.TrimSpace(msg.Body),
			ParentID:   msg.ParentID,
			ThreadID:   msg.ThreadID,
		})
	}
	return s.writePacket("outbound", format, p)
}

func (s *Service) QueueNetmail(fromUserID int64, toHandle, subject, body string) (string, error) {
	if s.users == nil || s.mail == nil {
		return "", errors.New("user and mail repositories are required")
	}
	if fromUserID <= 0 {
		return "", errors.New("from user id is required")
	}
	toHandle = strings.TrimSpace(toHandle)
	if toHandle == "" {
		return "", errors.New("to handle is required")
	}
	subject = strings.TrimSpace(subject)
	body = strings.TrimSpace(body)
	if subject == "" || body == "" {
		return "", errors.New("subject and body are required")
	}
	p := Packet{
		Version:   1,
		Format:    FormatNetmail,
		Exported:  time.Now().UTC(),
		SourceBBS: s.source,
		Messages: []PacketMessage{
			{
				FromUserID: fromUserID,
				ToHandle:   toHandle,
				Subject:    subject,
				Body:       body,
			},
		},
	}
	return s.writePacket("outbound", FormatNetmail, p)
}

func (s *Service) ImportPacket(path string, defaultBoardID, defaultAuthorID int64) (int, error) {
	var p Packet
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, errors.New("packet path is required")
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".qwk" || ext == ".rep" || ext == ".zip" {
		packet, err := readQWKBundle(path)
		if err != nil {
			return 0, err
		}
		p = packet
	} else {
		raw, err := os.ReadFile(path)
		if err != nil {
			return 0, err
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			// Allow QWK bundle parsing even with non-standard extension.
			packet, qwkErr := readQWKBundle(path)
			if qwkErr != nil {
				return 0, err
			}
			p = packet
		}
	}
	format, err := normalizeFormat(p.Format)
	if err != nil {
		return 0, err
	}
	switch format {
	case FormatNetmail:
		return s.importNetmail(p)
	default:
		return s.importBoardPacket(p, defaultBoardID, defaultAuthorID)
	}
}

func (s *Service) ImportInboundQueue(defaultBoardID, defaultAuthorID int64) (int, error) {
	inboundRoot, err := s.ensureDir("inbound")
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(inboundRoot)
	if err != nil {
		return 0, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	total := 0
	for _, formatDir := range entries {
		if !formatDir.IsDir() {
			continue
		}
		format := formatDir.Name()
		files, err := os.ReadDir(filepath.Join(inboundRoot, format))
		if err != nil {
			return total, err
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
		for _, file := range files {
			if file.IsDir() || !isPacketFile(format, file.Name()) {
				continue
			}
			path := filepath.Join(inboundRoot, format, file.Name())
			count, err := s.ImportPacket(path, defaultBoardID, defaultAuthorID)
			if err != nil {
				return total, err
			}
			total += count
			processedDir, err := s.ensureDir("processed", format)
			if err != nil {
				return total, err
			}
			_ = os.Rename(path, filepath.Join(processedDir, file.Name()))
		}
	}
	return total, nil
}

func (s *Service) importBoardPacket(p Packet, defaultBoardID, defaultAuthorID int64) (int, error) {
	if s.messages == nil {
		return 0, errors.New("message repository is required")
	}
	imported := 0
	for _, row := range p.Messages {
		boardID := s.resolveBoardID(row)
		if boardID <= 0 {
			boardID = defaultBoardID
		}
		if boardID <= 0 {
			return imported, errors.New("board id is required for imported message")
		}
		authorID := row.FromUserID
		if authorID <= 0 {
			authorID = defaultAuthorID
		}
		if authorID <= 0 {
			return imported, errors.New("author id is required for imported message")
		}
		subject := strings.TrimSpace(row.Subject)
		body := strings.TrimSpace(row.Body)
		if subject == "" || body == "" {
			continue
		}
		if err := s.messages.CreateMessage(&domain.Message{
			BoardID:   boardID,
			AuthorID:  authorID,
			ParentID:  row.ParentID,
			ThreadID:  row.ThreadID,
			Subject:   subject,
			Body:      body,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			return imported, err
		}
		imported++
	}
	return imported, nil
}

func (s *Service) importNetmail(p Packet) (int, error) {
	if s.mail == nil || s.users == nil {
		return 0, errors.New("mail and user repositories are required")
	}
	imported := 0
	for _, row := range p.Messages {
		subject := strings.TrimSpace(row.Subject)
		body := strings.TrimSpace(row.Body)
		if subject == "" || body == "" {
			continue
		}
		fromID := row.FromUserID
		if fromID <= 0 {
			continue
		}
		toID := row.ToUserID
		if toID <= 0 {
			for _, handle := range s.resolveNetmailHandles(row.ToHandle) {
				user, err := s.users.GetByHandle(handle)
				if err == nil && user != nil {
					toID = user.ID
					break
				}
			}
		}
		if toID <= 0 {
			continue
		}
		if err := s.mail.CreateMail(&domain.PrivateMail{
			FromUserID: fromID,
			ToUserID:   toID,
			Subject:    subject,
			Body:       body,
			CreatedAt:  time.Now().UTC(),
		}); err != nil {
			return imported, err
		}
		imported++
	}
	return imported, nil
}

func (s *Service) writePacket(direction, format string, p Packet) (string, error) {
	format, err := normalizeFormat(format)
	if err != nil {
		return "", err
	}
	dir, err := s.ensureDir(direction, format)
	if err != nil {
		return "", err
	}
	ext := ".json"
	if format == FormatQWK {
		ext = ".qwk"
	}
	filename := fmt.Sprintf("%s_%s_%d%s", format, time.Now().UTC().Format("20060102T150405Z"), time.Now().UTC().UnixNano(), ext)
	path := filepath.Join(dir, filename)
	if format == FormatQWK {
		if err := writeQWKBundle(path, p); err != nil {
			return "", err
		}
		return path, nil
	}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Service) Status() (Status, error) {
	status := Status{SpoolDir: s.spoolDir}
	inbound, err := s.countPackets("inbound")
	if err != nil {
		return status, err
	}
	outbound, err := s.countPackets("outbound")
	if err != nil {
		return status, err
	}
	status.InboundPackets = inbound
	status.OutboundPackets = outbound
	return status, nil
}

func (s *Service) RunExternalImport(ctx context.Context) error {
	if strings.TrimSpace(s.importCmd) == "" {
		return errors.New("import command is not configured")
	}
	return s.runExternal(ctx, s.importCmd, "import")
}

func (s *Service) RunExternalExport(ctx context.Context) error {
	if strings.TrimSpace(s.exportCmd) == "" {
		return errors.New("export command is not configured")
	}
	return s.runExternal(ctx, s.exportCmd, "export")
}

func (s *Service) runExternal(ctx context.Context, command, mode string) error {
	if _, err := s.ensureDir(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.Dir = s.spoolDir
	cmd.Env = append(os.Environ(),
		"WOLFBBS_NET_SPOOL_DIR="+s.spoolDir,
		"WOLFBBS_NET_MODE="+mode,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}

func (s *Service) countPackets(root string) (int, error) {
	base, err := s.ensureDir(root)
	if err != nil {
		return 0, err
	}
	count := 0
	err = filepath.WalkDir(base, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		name := strings.ToLower(strings.TrimSpace(d.Name()))
		if strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".qwk") || strings.HasSuffix(name, ".rep") || strings.HasSuffix(name, ".zip") {
			count++
		}
		return nil
	})
	return count, err
}

func isPacketFile(format, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if strings.HasSuffix(name, ".json") {
		return true
	}
	if format == FormatQWK {
		return strings.HasSuffix(name, ".qwk") || strings.HasSuffix(name, ".rep") || strings.HasSuffix(name, ".zip")
	}
	return false
}

func (s *Service) resolveBoardID(row PacketMessage) int64 {
	if row.BoardID > 0 {
		return row.BoardID
	}
	keys := []string{
		normalizeRouteKey(row.Board),
		normalizeRouteKey(row.Conference),
		normalizeRouteKey(row.Conference + "/" + row.Board),
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if id, ok := s.boardMap[key]; ok && id > 0 {
			return id
		}
	}
	if s.boards != nil {
		boards, err := s.boards.List()
		if err == nil {
			boardKey := normalizeRouteKey(row.Board)
			confKey := normalizeRouteKey(row.Conference)
			fullKey := normalizeRouteKey(row.Conference + "/" + row.Board)
			for _, board := range boards {
				if board.ID <= 0 {
					continue
				}
				nameKey := normalizeRouteKey(board.Name)
				confName := strings.TrimSpace(board.Conference)
				confNameKey := normalizeRouteKey(confName)
				boardFull := normalizeRouteKey(confName + "/" + board.Name)
				if boardKey != "" && nameKey == boardKey {
					return board.ID
				}
				if confKey != "" && confNameKey == confKey {
					return board.ID
				}
				if fullKey != "" && boardFull == fullKey {
					return board.ID
				}
			}
		}
	}
	return 0
}

func (s *Service) resolveNetmailHandles(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	candidates := []string{raw}
	if mapped, ok := s.handleMap[normalizeRouteKey(raw)]; ok {
		candidates = append(candidates, mapped)
	}
	if at := strings.Index(raw, "@"); at > 0 {
		local := strings.TrimSpace(raw[:at])
		if local != "" {
			candidates = append(candidates, local)
			if mapped, ok := s.handleMap[normalizeRouteKey(local)]; ok {
				candidates = append(candidates, mapped)
			}
		}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(candidates))
	for _, row := range candidates {
		row = strings.TrimSpace(row)
		if row == "" {
			continue
		}
		key := normalizeRouteKey(row)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, row)
	}
	return out
}

func normalizeRouteKey(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func parseBoardRoutes(raw string) map[string]int64 {
	out := map[string]int64{}
	for _, row := range strings.Split(raw, ",") {
		row = strings.TrimSpace(row)
		if row == "" {
			continue
		}
		parts := strings.SplitN(row, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := normalizeRouteKey(parts[0])
		id := int64(0)
		fmt.Sscan(strings.TrimSpace(parts[1]), &id)
		if key == "" || id <= 0 {
			continue
		}
		out[key] = id
	}
	return out
}

func parseHandleRoutes(raw string) map[string]string {
	out := map[string]string{}
	for _, row := range strings.Split(raw, ",") {
		row = strings.TrimSpace(row)
		if row == "" {
			continue
		}
		parts := strings.SplitN(row, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := normalizeRouteKey(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func cloneBoardRoutes(routes map[string]int64) map[string]int64 {
	out := map[string]int64{}
	for key, value := range routes {
		key = normalizeRouteKey(key)
		if key == "" || value <= 0 {
			continue
		}
		out[key] = value
	}
	return out
}

func cloneHandleRoutes(routes map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range routes {
		key = normalizeRouteKey(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}
