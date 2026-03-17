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

func TestGopherSelectors(t *testing.T) {
	boards := repository.NewInMemoryBoardRepository()
	messages := repository.NewInMemoryMessageRepository()
	board := &domain.Board{Name: "General"}
	if err := boards.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := messages.CreateMessage(&domain.Message{
		BoardID:   board.ID,
		AuthorID:  1,
		Subject:   "Welcome",
		Body:      "Hello from Gopher",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}

	srv := NewGopherServer("127.0.0.1:0", "localhost", boards, messages)
	if err := srv.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer srv.Close()

	root := requestGopher(t, srv.Addr(), "/")
	if !strings.Contains(root, "/board/1") {
		t.Fatalf("expected board selector in root menu: %s", root)
	}
	boardMenu := requestGopher(t, srv.Addr(), "/board/1")
	if !strings.Contains(boardMenu, "/msg/1") {
		t.Fatalf("expected message selector in board menu: %s", boardMenu)
	}
	msgText := requestGopher(t, srv.Addr(), "/msg/1")
	if !strings.Contains(msgText, "Hello from Gopher") {
		t.Fatalf("expected message body in article text: %s", msgText)
	}
}

func requestGopher(t *testing.T, addr, selector string) string {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial gopher: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(selector + "\r\n")); err != nil {
		t.Fatalf("write selector: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var out strings.Builder
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			out.WriteString(line)
		}
		if err != nil {
			break
		}
	}
	return out.String()
}
