package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
	spoolDir string
	boards   repository.BoardRepository
	messages repository.MessageRepository
	users    repository.UserRepository
	mail     repository.PrivateMailRepository
	source   string
}

func NewService(spoolDir string, boards repository.BoardRepository, messages repository.MessageRepository, users repository.UserRepository, mail repository.PrivateMailRepository) *Service {
	spoolDir = strings.TrimSpace(spoolDir)
	if spoolDir == "" {
		spoolDir = ".wolfbbs/network"
	}
	return &Service{
		spoolDir: spoolDir,
		boards:   boards,
		messages: messages,
		users:    users,
		mail:     mail,
		source:   "WolfBBS",
	}
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
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var p Packet
	if err := json.Unmarshal(raw, &p); err != nil {
		return 0, err
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
			if file.IsDir() || !strings.HasSuffix(strings.ToLower(file.Name()), ".json") {
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
		boardID := row.BoardID
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
			handle := strings.TrimSpace(row.ToHandle)
			if handle == "" {
				continue
			}
			user, err := s.users.GetByHandle(handle)
			if err != nil {
				continue
			}
			toID = user.ID
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
	filename := fmt.Sprintf("%s_%s_%d.json", format, time.Now().UTC().Format("20060102T150405Z"), time.Now().UTC().UnixNano())
	path := filepath.Join(dir, filename)
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
	inbound, err := s.countJSON("inbound")
	if err != nil {
		return status, err
	}
	outbound, err := s.countJSON("outbound")
	if err != nil {
		return status, err
	}
	status.InboundPackets = inbound
	status.OutboundPackets = outbound
	return status, nil
}

func (s *Service) countJSON(root string) (int, error) {
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
		if strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			count++
		}
		return nil
	})
	return count, err
}
