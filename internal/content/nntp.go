package content

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

type NNTPServer struct {
	addr        string
	tlsCertPath string
	tlsKeyPath  string
	boards      repository.BoardRepository
	messages    repository.MessageRepository

	mu sync.Mutex
	ln net.Listener
}

func NewNNTPServer(addr string, boards repository.BoardRepository, messages repository.MessageRepository) *NNTPServer {
	return NewNNTPTLSServer(addr, "", "", boards, messages)
}

func NewNNTPTLSServer(addr, certPath, keyPath string, boards repository.BoardRepository, messages repository.MessageRepository) *NNTPServer {
	return &NNTPServer{
		addr:        strings.TrimSpace(addr),
		tlsCertPath: strings.TrimSpace(certPath),
		tlsKeyPath:  strings.TrimSpace(keyPath),
		boards:      boards,
		messages:    messages,
	}
}

func (s *NNTPServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln != nil {
		return nil
	}
	ln, err := s.newListener()
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}
	s.ln = ln
	go s.acceptLoop()
	return nil
}

func (s *NNTPServer) newListener() (net.Listener, error) {
	if s.tlsCertPath == "" && s.tlsKeyPath == "" {
		return net.Listen("tcp", s.addr)
	}
	if s.tlsCertPath == "" || s.tlsKeyPath == "" {
		return nil, fmt.Errorf("both nntps cert and key are required")
	}
	cert, err := tls.LoadX509KeyPair(s.tlsCertPath, s.tlsKeyPath)
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	return tls.Listen("tcp", s.addr, cfg)
}

func (s *NNTPServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return nil
	}
	err := s.ln.Close()
	s.ln = nil
	return err
}

func (s *NNTPServer) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

func (s *NNTPServer) acceptLoop() {
	for {
		s.mu.Lock()
		ln := s.ln
		s.mu.Unlock()
		if ln == nil {
			return
		}
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

type groupState struct {
	name     string
	boardID  int64
	messages []domain.Message
}

func (s *NNTPServer) handleConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	_, _ = writer.WriteString("200 WolfBBS NNTP service ready\r\n")
	_ = writer.Flush()

	state := groupState{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		cmd, arg := splitCommand(line)
		switch cmd {
		case "CAPABILITIES":
			_, _ = writer.WriteString("101 Capability list:\r\n")
			_, _ = writer.WriteString("VERSION 2\r\n")
			_, _ = writer.WriteString("READER\r\n")
			_, _ = writer.WriteString("LIST ACTIVE\r\n")
			_, _ = writer.WriteString(".\r\n")
		case "MODE":
			if strings.EqualFold(strings.TrimSpace(arg), "READER") {
				_, _ = writer.WriteString("200 Reader mode accepted\r\n")
			} else {
				_, _ = writer.WriteString("501 unsupported mode\r\n")
			}
		case "LIST":
			_ = s.handleList(writer)
		case "GROUP":
			group, boardID, msgs, groupErr := s.resolveGroup(strings.TrimSpace(arg))
			if groupErr != nil {
				_, _ = writer.WriteString("411 no such newsgroup\r\n")
				break
			}
			state = groupState{name: group, boardID: boardID, messages: msgs}
			low, high := articleBounds(len(msgs))
			_, _ = fmt.Fprintf(writer, "211 %d %d %d %s\r\n", len(msgs), low, high, group)
		case "XOVER":
			_ = s.handleXOver(writer, state, strings.TrimSpace(arg))
		case "ARTICLE", "HEAD", "BODY":
			_ = s.handleArticle(writer, state, cmd, strings.TrimSpace(arg))
		case "HELP":
			_, _ = writer.WriteString("100 help text follows\r\n")
			_, _ = writer.WriteString("Supported: CAPABILITIES MODE READER LIST GROUP XOVER ARTICLE HEAD BODY QUIT HELP\r\n")
			_, _ = writer.WriteString(".\r\n")
		case "QUIT":
			_, _ = writer.WriteString("205 closing connection\r\n")
			_ = writer.Flush()
			return
		default:
			_, _ = writer.WriteString("500 command not recognized\r\n")
		}
		_ = writer.Flush()
	}
}

func splitCommand(line string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(line), " ", 2)
	cmd := strings.ToUpper(strings.TrimSpace(parts[0]))
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
	}
	return cmd, arg
}

func (s *NNTPServer) handleList(writer *bufio.Writer) error {
	boards, err := s.boards.List()
	if err != nil {
		_, _ = writer.WriteString("503 board list unavailable\r\n")
		return nil
	}
	_, _ = writer.WriteString("215 list of newsgroups follows\r\n")
	for _, board := range boards {
		msgs, _ := s.messages.ListByBoard(board.ID)
		low, high := articleBounds(len(msgs))
		_, _ = fmt.Fprintf(writer, "%s %d %d y\r\n", boardGroupName(board.ID), high, low)
	}
	_, _ = writer.WriteString(".\r\n")
	return nil
}

func (s *NNTPServer) resolveGroup(group string) (string, int64, []domain.Message, error) {
	group = strings.ToLower(strings.TrimSpace(group))
	if group == "" {
		return "", 0, nil, fmt.Errorf("missing group")
	}
	boardID, err := parseGroupID(group)
	if err != nil {
		return "", 0, nil, err
	}
	if _, err := s.boards.Get(boardID); err != nil {
		return "", 0, nil, err
	}
	msgs, err := s.messages.ListByBoard(boardID)
	if err != nil {
		return "", 0, nil, err
	}
	return group, boardID, msgs, nil
}

func (s *NNTPServer) handleXOver(writer *bufio.Writer, state groupState, arg string) error {
	if state.name == "" {
		_, _ = writer.WriteString("412 no newsgroup selected\r\n")
		return nil
	}
	start, end := parseRange(arg, len(state.messages))
	if start <= 0 || end <= 0 || start > end {
		_, _ = writer.WriteString("423 no articles in that range\r\n")
		return nil
	}
	_, _ = writer.WriteString("224 Overview information follows\r\n")
	for i := start; i <= end; i++ {
		msg := state.messages[i-1]
		msgID := articleMessageID(msg.ID)
		subject := sanitizeHeader(msg.Subject)
		from := articleFrom(msg.AuthorID)
		date := msg.CreatedAt.UTC().Format(time.RFC1123Z)
		lines := countLines(msg.Body)
		bytes := len(msg.Body)
		_, _ = fmt.Fprintf(writer, "%d\t%s\t%s\t%s\t%s\t\t%d\t%d\r\n", i, subject, from, date, msgID, bytes, lines)
	}
	_, _ = writer.WriteString(".\r\n")
	return nil
}

func (s *NNTPServer) handleArticle(writer *bufio.Writer, state groupState, command, arg string) error {
	if state.name == "" {
		_, _ = writer.WriteString("412 no newsgroup selected\r\n")
		return nil
	}
	index, msg, ok := resolveArticle(state.messages, arg)
	if !ok {
		_, _ = writer.WriteString("423 no such article number\r\n")
		return nil
	}
	msgID := articleMessageID(msg.ID)
	switch command {
	case "HEAD":
		_, _ = fmt.Fprintf(writer, "221 %d %s head follows\r\n", index, msgID)
		writeArticleHeaders(writer, state.name, msg)
		_, _ = writer.WriteString(".\r\n")
	case "BODY":
		_, _ = fmt.Fprintf(writer, "222 %d %s body follows\r\n", index, msgID)
		writeArticleBody(writer, msg.Body)
		_, _ = writer.WriteString(".\r\n")
	default:
		_, _ = fmt.Fprintf(writer, "220 %d %s article follows\r\n", index, msgID)
		writeArticleHeaders(writer, state.name, msg)
		_, _ = writer.WriteString("\r\n")
		writeArticleBody(writer, msg.Body)
		_, _ = writer.WriteString(".\r\n")
	}
	return nil
}

func boardGroupName(boardID int64) string {
	return fmt.Sprintf("wolfbbs.board.%d", boardID)
}

func parseGroupID(group string) (int64, error) {
	if !strings.HasPrefix(group, "wolfbbs.board.") {
		return 0, fmt.Errorf("unsupported group")
	}
	idRaw := strings.TrimPrefix(group, "wolfbbs.board.")
	return strconv.ParseInt(idRaw, 10, 64)
}

func articleBounds(count int) (int, int) {
	if count <= 0 {
		return 0, 0
	}
	return 1, count
}

func parseRange(arg string, total int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return 1, total
	}
	if !strings.Contains(arg, "-") {
		n, err := strconv.Atoi(arg)
		if err != nil || n <= 0 || n > total {
			return 0, 0
		}
		return n, n
	}
	parts := strings.SplitN(arg, "-", 2)
	start, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	end := total
	if strings.TrimSpace(parts[1]) != "" {
		end, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	}
	if start <= 0 {
		start = 1
	}
	if end <= 0 || end > total {
		end = total
	}
	if start > total {
		return 0, 0
	}
	if start > end {
		return 0, 0
	}
	return start, end
}

func resolveArticle(messages []domain.Message, arg string) (int, domain.Message, bool) {
	if len(messages) == 0 {
		return 0, domain.Message{}, false
	}
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return 1, messages[0], true
	}
	if strings.HasPrefix(arg, "<msg-") && strings.HasSuffix(arg, "@wolfbbs.local>") {
		raw := strings.TrimPrefix(arg, "<msg-")
		raw = strings.TrimSuffix(raw, "@wolfbbs.local>")
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, domain.Message{}, false
		}
		for i, msg := range messages {
			if msg.ID == id {
				return i + 1, msg, true
			}
		}
		return 0, domain.Message{}, false
	}
	n, err := strconv.Atoi(arg)
	if err != nil || n <= 0 || n > len(messages) {
		return 0, domain.Message{}, false
	}
	return n, messages[n-1], true
}

func articleMessageID(messageID int64) string {
	return fmt.Sprintf("<msg-%d@wolfbbs.local>", messageID)
}

func articleFrom(authorID int64) string {
	return fmt.Sprintf("user%d@wolfbbs.local", authorID)
}

func writeArticleHeaders(w *bufio.Writer, group string, msg domain.Message) {
	_, _ = fmt.Fprintf(w, "Message-ID: %s\r\n", articleMessageID(msg.ID))
	_, _ = fmt.Fprintf(w, "Newsgroups: %s\r\n", group)
	_, _ = fmt.Fprintf(w, "Subject: %s\r\n", sanitizeHeader(msg.Subject))
	_, _ = fmt.Fprintf(w, "From: %s\r\n", articleFrom(msg.AuthorID))
	_, _ = fmt.Fprintf(w, "Date: %s\r\n", msg.CreatedAt.UTC().Format(time.RFC1123Z))
	_, _ = fmt.Fprintf(w, "X-WolfBBS-Board-ID: %d\r\n", msg.BoardID)
}

func writeArticleBody(w *bufio.Writer, body string) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, ".") {
			line = "." + line
		}
		_, _ = w.WriteString(line + "\r\n")
	}
}

func countLines(body string) int {
	if strings.TrimSpace(body) == "" {
		return 0
	}
	return len(strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n"))
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
