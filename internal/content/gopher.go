package content

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"

	"wolfbbs/internal/repository"
)

type GopherServer struct {
	addr       string
	publicHost string
	boards     repository.BoardRepository
	messages   repository.MessageRepository

	mu sync.Mutex
	ln net.Listener
}

func NewGopherServer(addr, publicHost string, boards repository.BoardRepository, messages repository.MessageRepository) *GopherServer {
	return &GopherServer{
		addr:       strings.TrimSpace(addr),
		publicHost: strings.TrimSpace(publicHost),
		boards:     boards,
		messages:   messages,
	}
}

func (s *GopherServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln != nil {
		return nil
	}
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln
	go s.acceptLoop()
	return nil
}

func (s *GopherServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return nil
	}
	err := s.ln.Close()
	s.ln = nil
	return err
}

func (s *GopherServer) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

func (s *GopherServer) acceptLoop() {
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

func (s *GopherServer) handleConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return
	}
	selector := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
	selector = strings.TrimSuffix(selector, "\r")
	if selector == "" {
		selector = "/"
	}
	host, port := s.endpointHints()
	switch {
	case selector == "/" || selector == "":
		_ = s.renderRoot(conn, host, port)
	case strings.HasPrefix(selector, "/board/"):
		boardID, parseErr := strconv.ParseInt(strings.TrimPrefix(selector, "/board/"), 10, 64)
		if parseErr != nil {
			writeGopherInfo(conn, "invalid board selector")
			return
		}
		_ = s.renderBoard(conn, host, port, boardID)
	case strings.HasPrefix(selector, "/msg/"):
		msgID, parseErr := strconv.ParseInt(strings.TrimPrefix(selector, "/msg/"), 10, 64)
		if parseErr != nil {
			writeGopherInfo(conn, "invalid message selector")
			return
		}
		_ = s.renderMessage(conn, msgID)
	default:
		writeGopherInfo(conn, "unknown selector")
	}
}

func (s *GopherServer) endpointHints() (string, string) {
	host := s.publicHost
	port := "70"
	addr := s.Addr()
	if addr != "" {
		if h, p, err := net.SplitHostPort(addr); err == nil {
			if host == "" {
				if h == "" || h == "0.0.0.0" || h == "::" {
					host = "localhost"
				} else {
					host = h
				}
			}
			if p != "" {
				port = p
			}
		}
	}
	if host == "" {
		host = "localhost"
	}
	return host, port
}

func (s *GopherServer) renderRoot(w io.Writer, host, port string) error {
	boards, err := s.boards.List()
	if err != nil {
		writeGopherInfo(w, "failed to load boards")
		return nil
	}
	_, _ = fmt.Fprintf(w, "iWolfBBS Gopher Gateway\tfake\t%s\t%s\r\n", host, port)
	if len(boards) == 0 {
		_, _ = fmt.Fprintf(w, "iNo boards available\tfake\t%s\t%s\r\n", host, port)
		_, _ = io.WriteString(w, ".\r\n")
		return nil
	}
	for _, board := range boards {
		label := sanitizeGopherText(fmt.Sprintf("%d) %s", board.ID, board.Name))
		_, _ = fmt.Fprintf(w, "1%s\t/board/%d\t%s\t%s\r\n", label, board.ID, host, port)
	}
	_, _ = io.WriteString(w, ".\r\n")
	return nil
}

func (s *GopherServer) renderBoard(w io.Writer, host, port string, boardID int64) error {
	board, err := s.boards.Get(boardID)
	if err != nil {
		writeGopherInfo(w, "board not found")
		return nil
	}
	msgs, err := s.messages.ListByBoard(boardID)
	if err != nil {
		writeGopherInfo(w, "failed to load messages")
		return nil
	}
	_, _ = fmt.Fprintf(w, "iBoard: %s\tfake\t%s\t%s\r\n", sanitizeGopherText(board.Name), host, port)
	for _, msg := range msgs {
		subject := sanitizeGopherText(msg.Subject)
		_, _ = fmt.Fprintf(w, "0#%d %s\t/msg/%d\t%s\t%s\r\n", msg.ID, subject, msg.ID, host, port)
	}
	_, _ = fmt.Fprintf(w, "1Back to boards\t/\t%s\t%s\r\n", host, port)
	_, _ = io.WriteString(w, ".\r\n")
	return nil
}

func (s *GopherServer) renderMessage(w io.Writer, messageID int64) error {
	msg, err := s.messages.GetMessage(messageID)
	if err != nil {
		writeGopherInfo(w, "message not found")
		return nil
	}
	_, _ = io.WriteString(w, "WolfBBS Message\r\n")
	_, _ = fmt.Fprintf(w, "Message-ID: %d\r\n", msg.ID)
	_, _ = fmt.Fprintf(w, "Board-ID: %d\r\n", msg.BoardID)
	_, _ = fmt.Fprintf(w, "Author-ID: %d\r\n", msg.AuthorID)
	_, _ = fmt.Fprintf(w, "Subject: %s\r\n", sanitizeGopherText(msg.Subject))
	_, _ = fmt.Fprintf(w, "Date: %s\r\n", msg.CreatedAt.UTC().Format("2006-01-02 15:04:05Z"))
	_, _ = io.WriteString(w, "\r\n")
	for _, line := range strings.Split(msg.Body, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, ".") {
			line = "." + line
		}
		_, _ = io.WriteString(w, line+"\r\n")
	}
	_, _ = io.WriteString(w, ".\r\n")
	return nil
}

func writeGopherInfo(w io.Writer, text string) {
	_, _ = io.WriteString(w, "i"+sanitizeGopherText(text)+"\tfake\terror\t1\r\n.\r\n")
}

func sanitizeGopherText(value string) string {
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
