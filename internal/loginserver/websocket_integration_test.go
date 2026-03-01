package loginserver_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/loginserver"
	"wolfbbs/internal/repository"
	"wolfbbs/internal/session"
)

func TestWebSocketLoginFlow(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("wsuser", "password123"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	user, _ := authSvc.GetUser("wsuser")
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
	srv, err := loginserver.NewWebSocketServer("127.0.0.1:0", "/ws-login", logger, authSvc, nodes, nil)
	if err != nil {
		t.Fatalf("new websocket server: %v", err)
	}
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

	u := url.URL{Scheme: "ws", Host: ln.Addr().String(), Path: "/ws-login"}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	readUntil := func(substr string) string {
		t.Helper()
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("read message: %v", err)
			}
			text := string(msg)
			if strings.Contains(text, substr) {
				return text
			}
		}
		t.Fatalf("timeout waiting for %q", substr)
		return ""
	}

	send := func(line string) {
		t.Helper()
		if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
			t.Fatalf("write message: %v", err)
		}
	}

	readUntil("Handle:")
	send("wsuser")
	readUntil("Password:")
	send("password123")
	readUntil("2FA code")
	send("")
	readUntil("Login successful")
	readUntil("Enter selection:")
	send("B")
	readUntil("Commands: R <boardID> [msgID], P <boardID>, Q")
	send("Q")
	readUntil("Enter selection:")
	send("M")
	readUntil("Commands: C compose")
	send("Q")
	readUntil("Enter selection:")
	send("C")
	readUntil("Commands: S <msg>, J <#channel>, O online, Q")
	send("Q")
	readUntil("Enter selection:")
	send("Q")
	readUntil("Goodbye.")
}
