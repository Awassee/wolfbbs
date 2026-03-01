package loginserver_test

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/loginserver"
	"wolfbbs/internal/repository"
	"wolfbbs/internal/session"
)

func TestTelnetLoginFlow(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("tnuser", "password123"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	user, _ := authSvc.GetUser("tnuser")
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	chatSvc := chat.NewServiceForTest()
	if err := boardRepo.Create(&domain.Board{Name: "General", Description: "Main", CreatedBy: user.ID}); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{BoardID: 1, AuthorID: user.ID, Subject: "Hello", Body: "world"}); err != nil {
		t.Fatalf("seed message: %v", err)
	}
	if err := mailRepo.CreateMail(&domain.PrivateMail{FromUserID: user.ID, ToUserID: user.ID, Subject: "Seed Mail", Body: "body"}); err != nil {
		t.Fatalf("seed mail: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	nodes := session.NewManager(255, 256)
	srv := loginserver.NewTelnetServer("127.0.0.1:0", logger, authSvc, nodes)
	srv.SetServices(boardRepo, msgRepo, mailRepo, chatSvc, t.TempDir())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		_ = srv.Serve(ln)
	}()
	defer srv.Shutdown(context.Background())

	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 3*time.Second)
	if err != nil {
		t.Fatalf("dial telnet: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	readUntil := func(substr string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("read line: %v", err)
			}
			if strings.Contains(line, substr) {
				return
			}
		}
		t.Fatalf("timeout waiting for %q", substr)
	}

	writeLine := func(value string) {
		t.Helper()
		if _, err := conn.Write([]byte(value + "\n")); err != nil {
			t.Fatalf("write line: %v", err)
		}
	}

	readUntil("Handle:")
	writeLine("tnuser")
	readUntil("Password:")
	writeLine("password123")
	readUntil("2FA code")
	writeLine("")
	readUntil("Login successful")
	readUntil("Enter selection:")
	writeLine("B")
	readUntil("Commands: R <boardID> [msgID], P <boardID>, Q")
	writeLine("Q")
	readUntil("Enter selection:")
	writeLine("M")
	readUntil("Commands: C compose")
	writeLine("Q")
	readUntil("Enter selection:")
	writeLine("Q")
	readUntil("Goodbye.")
}
