package content

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

func TestNNTPListGroupAndArticle(t *testing.T) {
	boards := repository.NewInMemoryBoardRepository()
	messages := repository.NewInMemoryMessageRepository()
	board := &domain.Board{Name: "General"}
	if err := boards.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := messages.CreateMessage(&domain.Message{
		BoardID:   board.ID,
		AuthorID:  2,
		Subject:   "NNTP Subject",
		Body:      "Body line one\nBody line two",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}

	srv := NewNNTPServer("127.0.0.1:0", boards, messages)
	if err := srv.Start(); err != nil {
		t.Fatalf("start nntp server: %v", err)
	}
	defer srv.Close()

	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatalf("dial nntp: %v", err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)

	assertLinePrefix(t, reader, "200 ")
	sendLine(t, conn, "LIST")
	assertLinePrefix(t, reader, "215 ")
	lines := readMultiline(reader)
	if !strings.Contains(strings.Join(lines, "\n"), "wolfbbs.board.1") {
		t.Fatalf("expected board group in LIST response: %#v", lines)
	}

	sendLine(t, conn, "GROUP wolfbbs.board.1")
	assertLinePrefix(t, reader, "211 ")

	sendLine(t, conn, "XOVER 1-10")
	assertLinePrefix(t, reader, "224 ")
	lines = readMultiline(reader)
	if !strings.Contains(strings.Join(lines, "\n"), "NNTP Subject") {
		t.Fatalf("expected subject in XOVER response: %#v", lines)
	}

	sendLine(t, conn, "ARTICLE 1")
	assertLinePrefix(t, reader, "220 ")
	lines = readMultiline(reader)
	body := strings.Join(lines, "\n")
	if !strings.Contains(body, "Subject: NNTP Subject") || !strings.Contains(body, "Body line one") {
		t.Fatalf("unexpected article payload: %s", body)
	}

	sendLine(t, conn, "QUIT")
	assertLinePrefix(t, reader, "205 ")
}

func sendLine(t *testing.T, conn net.Conn, line string) {
	t.Helper()
	_, err := conn.Write([]byte(line + "\r\n"))
	if err != nil {
		t.Fatalf("send line %q: %v", line, err)
	}
}

func assertLinePrefix(t *testing.T, reader *bufio.Reader, prefix string) string {
	t.Helper()
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read line: %v", err)
	}
	line = strings.TrimSpace(strings.TrimSuffix(line, "\n"))
	line = strings.TrimSuffix(line, "\r")
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("expected line prefix %q, got %q", prefix, line)
	}
	return line
}

func readMultiline(reader *bufio.Reader) []string {
	out := []string{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if line == "." {
			break
		}
		out = append(out, line)
	}
	return out
}
