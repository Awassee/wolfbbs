package sshserver_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"wolfbbs/internal/auth"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
	"wolfbbs/internal/sshserver"
)

type safeBuffer struct {
	mu sync.RWMutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.b.String()
}

func TestSSHLoginFlow(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)
	_, err := authSvc.Register("tester", "password123")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := sshserver.New("127.0.0.1:0", logger, authSvc)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
	defer srv.Shutdown(context.Background())

	cfg := &ssh.ClientConfig{
		User:            "ignored",
		Auth:            []ssh.AuthMethod{ssh.Password("ignored")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	client, err := ssh.Dial("tcp", ln.Addr().String(), cfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	defer session.Close()

	if err := session.RequestPty("xterm", 25, 80, ssh.TerminalModes{ssh.ECHO: 1}); err != nil {
		t.Fatalf("request pty: %v", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := session.Shell(); err != nil {
		t.Fatalf("shell: %v", err)
	}

	var out safeBuffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&out, stdout)
	}()

	waitFor := func(substr string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(25 * time.Millisecond)
			curr := out.String()
			if strings.Contains(curr, substr) {
				return
			}
		}
		curr := out.String()
		t.Fatalf("timed out waiting for %q in output: %q", substr, curr)
	}

	waitFor("Press any key to continue")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Handle:")
	_, _ = stdin.Write([]byte("tester\n"))
	waitFor("Password:")
	_, _ = stdin.Write([]byte("password123\n"))
	waitFor("Any key to return.")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Enter selection:")
	_, _ = stdin.Write([]byte("Q\n"))

	_ = session.Wait()
	<-done
}

func TestSSHBoardPostFlow(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	doorRepo := repository.NewInMemoryDoorRepository()

	authSvc := auth.NewService(userRepo)
	user, err := authSvc.Register("poster", "password123")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := boardRepo.Create(&domain.Board{Name: "General", Description: "General board", CreatedBy: user.ID}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := sshserver.New("127.0.0.1:0", logger, authSvc)
	srv.SetRepositories(userRepo, boardRepo, msgRepo, mailRepo, adminRepo, doorRepo)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
	defer srv.Shutdown(context.Background())

	cfg := &ssh.ClientConfig{
		User:            "ignored",
		Auth:            []ssh.AuthMethod{ssh.Password("ignored")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	client, err := ssh.Dial("tcp", ln.Addr().String(), cfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	defer session.Close()
	if err := session.RequestPty("xterm", 25, 80, ssh.TerminalModes{ssh.ECHO: 1}); err != nil {
		t.Fatalf("request pty: %v", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := session.Shell(); err != nil {
		t.Fatalf("shell: %v", err)
	}

	var out safeBuffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&out, stdout)
	}()

	lastIdx := 0
	waitFor := func(substr string) {
		t.Helper()
		deadline := time.Now().Add(6 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
			curr := out.String()
			if lastIdx > len(curr) {
				lastIdx = len(curr)
			}
			next := curr[lastIdx:]
			if idx := strings.Index(next, substr); idx >= 0 {
				lastIdx += idx + len(substr)
				return
			}
		}
		t.Fatalf("timed out waiting for %q", substr)
	}

	waitFor("Press any key to continue")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Handle:")
	_, _ = stdin.Write([]byte("poster\n"))
	waitFor("Password:")
	_, _ = stdin.Write([]byte("password123\n"))
	waitFor("Any key to return.")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Enter selection:")
	_, _ = stdin.Write([]byte("M"))
	waitFor("Select board ID")
	_, _ = stdin.Write([]byte("1\n"))
	waitFor("Commands: (N)ew")
	_, _ = stdin.Write([]byte("N\n"))
	waitFor("Subject:")
	_, _ = stdin.Write([]byte("First threaded post\n"))
	_, _ = stdin.Write([]byte("hello world\n.\n"))
	waitFor("Commands: (N)ew")
	_, _ = stdin.Write([]byte("Q\n"))
	waitFor("Select board ID")
	_, _ = stdin.Write([]byte("Q\n"))
	waitFor("Enter selection:")
	_, _ = stdin.Write([]byte("Q"))

	_ = session.Wait()
	<-done

	msgs, err := msgRepo.ListByBoard(1)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Subject != "First threaded post" {
		t.Fatalf("unexpected subject: %q", msgs[0].Subject)
	}
}

func TestSSHMailReplyDeleteFlow(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	doorRepo := repository.NewInMemoryDoorRepository()

	authSvc := auth.NewService(userRepo)
	alice, err := authSvc.Register("alice", "password123")
	if err != nil {
		t.Fatalf("register alice: %v", err)
	}
	bob, err := authSvc.Register("bob", "password123")
	if err != nil {
		t.Fatalf("register bob: %v", err)
	}
	if err := mailRepo.CreateMail(&domain.PrivateMail{
		FromUserID: bob.ID,
		ToUserID:   alice.ID,
		Subject:    "Seed Mail",
		Body:       "seed body",
	}); err != nil {
		t.Fatalf("seed mail: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := sshserver.New("127.0.0.1:0", logger, authSvc)
	srv.SetRepositories(userRepo, boardRepo, msgRepo, mailRepo, adminRepo, doorRepo)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
	defer srv.Shutdown(context.Background())

	cfg := &ssh.ClientConfig{
		User:            "ignored",
		Auth:            []ssh.AuthMethod{ssh.Password("ignored")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	client, err := ssh.Dial("tcp", ln.Addr().String(), cfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	defer session.Close()
	if err := session.RequestPty("xterm", 25, 80, ssh.TerminalModes{ssh.ECHO: 1}); err != nil {
		t.Fatalf("request pty: %v", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := session.Shell(); err != nil {
		t.Fatalf("shell: %v", err)
	}

	var out safeBuffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&out, stdout)
	}()

	lastIdx := 0
	waitFor := func(substr string) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
			curr := out.String()
			if lastIdx > len(curr) {
				lastIdx = len(curr)
			}
			next := curr[lastIdx:]
			if idx := strings.Index(next, substr); idx >= 0 {
				lastIdx += idx + len(substr)
				return
			}
		}
		t.Fatalf("timed out waiting for %q", substr)
	}

	waitFor("Press any key to continue")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Handle:")
	_, _ = stdin.Write([]byte("alice\n"))
	waitFor("Password:")
	_, _ = stdin.Write([]byte("password123\n"))
	waitFor("Any key to return.")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Enter selection:")
	_, _ = stdin.Write([]byte("P"))
	waitFor("Commands: (C)ompose")

	_, _ = stdin.Write([]byte("C\n"))
	waitFor("To handle or external email:")
	_, _ = stdin.Write([]byte("bob\n"))
	waitFor("Subject:")
	_, _ = stdin.Write([]byte("hello bob\n"))
	waitFor("Urgency [normal]:")
	_, _ = stdin.Write([]byte("\n"))
	_, _ = stdin.Write([]byte("local mail from alice\n.\n"))
	waitFor("Commands: (C)ompose")

	_, _ = stdin.Write([]byte("R\n"))
	waitFor("Mail ID:")
	_, _ = stdin.Write([]byte("1\n"))
	waitFor("Reader commands: (P) reply  (D) delete  (Q) back")
	_, _ = stdin.Write([]byte("P"))
	waitFor("Subject [Re:")
	_, _ = stdin.Write([]byte("\n"))
	waitFor("Enter reply body")
	_, _ = stdin.Write([]byte("acknowledged\n.\n"))
	waitFor("Reply sent. Press any key.")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Commands: (C)ompose")

	_, _ = stdin.Write([]byte("R\n"))
	waitFor("Mail ID:")
	_, _ = stdin.Write([]byte("1\n"))
	waitFor("Reader commands: (P) reply  (D) delete  (Q) back")
	_, _ = stdin.Write([]byte("D"))
	waitFor("Mail deleted. Press any key.")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Commands: (C)ompose")

	_, _ = stdin.Write([]byte("Q\n"))
	waitFor("Enter selection:")
	_, _ = stdin.Write([]byte("Q"))

	_ = session.Wait()
	<-done

	aliceInbox, err := mailRepo.ListInbox(alice.ID, 50)
	if err != nil {
		t.Fatalf("alice inbox: %v", err)
	}
	for _, row := range aliceInbox {
		if row.Subject == "Seed Mail" {
			t.Fatalf("expected seed mail to be deleted; inbox=%#v", aliceInbox)
		}
	}
	bobInbox, err := mailRepo.ListInbox(bob.ID, 50)
	if err != nil {
		t.Fatalf("bob inbox: %v", err)
	}
	if len(bobInbox) < 2 {
		t.Fatalf("expected bob to receive compose + reply mail, got %d", len(bobInbox))
	}
}

func TestSSHNewscanDigestShowsRecentTraffic(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	doorRepo := repository.NewInMemoryDoorRepository()

	authSvc := auth.NewService(userRepo)
	scanner, err := authSvc.Register("scanner", "password123")
	if err != nil {
		t.Fatalf("register scanner: %v", err)
	}
	writer, err := authSvc.Register("writer", "password123")
	if err != nil {
		t.Fatalf("register writer: %v", err)
	}
	last := time.Now().UTC().Add(-2 * time.Hour)
	scanner.LastLoginAt = &last
	if err := userRepo.Update(scanner); err != nil {
		t.Fatalf("seed scanner last login: %v", err)
	}
	if err := boardRepo.Create(&domain.Board{Name: "General", Conference: "Public", Description: "General", CreatedBy: writer.ID}); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := msgRepo.CreateMessage(&domain.Message{
		BoardID:  1,
		AuthorID: writer.ID,
		Subject:  "Digest Check Subject",
		Body:     "new traffic",
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := sshserver.New("127.0.0.1:0", logger, authSvc)
	srv.SetRepositories(userRepo, boardRepo, msgRepo, mailRepo, adminRepo, doorRepo)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
	defer srv.Shutdown(context.Background())

	cfg := &ssh.ClientConfig{
		User:            "ignored",
		Auth:            []ssh.AuthMethod{ssh.Password("ignored")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	client, err := ssh.Dial("tcp", ln.Addr().String(), cfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	defer session.Close()
	if err := session.RequestPty("xterm", 25, 80, ssh.TerminalModes{ssh.ECHO: 1}); err != nil {
		t.Fatalf("request pty: %v", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := session.Shell(); err != nil {
		t.Fatalf("shell: %v", err)
	}

	var out safeBuffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&out, stdout)
	}()

	lastIdx := 0
	waitFor := func(substr string) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
			curr := out.String()
			if lastIdx > len(curr) {
				lastIdx = len(curr)
			}
			next := curr[lastIdx:]
			if idx := strings.Index(next, substr); idx >= 0 {
				lastIdx += idx + len(substr)
				return
			}
		}
		t.Fatalf("timed out waiting for %q", substr)
	}

	waitFor("Press any key to continue")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Handle:")
	_, _ = stdin.Write([]byte("scanner\n"))
	waitFor("Password:")
	_, _ = stdin.Write([]byte("password123\n"))
	waitFor("Since your last call:")
	waitFor("Digest Check Subject")
	waitFor("Any key to return.")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Enter selection:")
	_, _ = stdin.Write([]byte("Q"))

	_ = session.Wait()
	<-done
}

func TestSSHGatewayModernDoorsFlow(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	boardRepo := repository.NewInMemoryBoardRepository()
	msgRepo := repository.NewInMemoryMessageRepository()
	mailRepo := repository.NewInMemoryPrivateMailRepository()
	adminRepo := repository.NewInMemoryAdminRepository()
	doorRepo := repository.NewInMemoryDoorRepository()

	authSvc := auth.NewService(userRepo)
	if _, err := authSvc.Register("gatewayuser", "password123"); err != nil {
		t.Fatalf("register user: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := sshserver.New("127.0.0.1:0", logger, authSvc)
	srv.SetRepositories(userRepo, boardRepo, msgRepo, mailRepo, adminRepo, doorRepo)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = srv.Serve(ln) }()
	defer srv.Shutdown(context.Background())

	cfg := &ssh.ClientConfig{
		User:            "ignored",
		Auth:            []ssh.AuthMethod{ssh.Password("ignored")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	client, err := ssh.Dial("tcp", ln.Addr().String(), cfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	defer session.Close()
	if err := session.RequestPty("xterm", 25, 80, ssh.TerminalModes{ssh.ECHO: 1}); err != nil {
		t.Fatalf("request pty: %v", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := session.Shell(); err != nil {
		t.Fatalf("shell: %v", err)
	}

	var out safeBuffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&out, stdout)
	}()

	lastIdx := 0
	waitFor := func(substr string) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
			curr := out.String()
			if lastIdx > len(curr) {
				lastIdx = len(curr)
			}
			next := curr[lastIdx:]
			if idx := strings.Index(next, substr); idx >= 0 {
				lastIdx += idx + len(substr)
				return
			}
		}
		t.Fatalf("timed out waiting for %q", substr)
	}

	waitFor("Press any key to continue")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Handle:")
	_, _ = stdin.Write([]byte("gatewayuser\n"))
	waitFor("Password:")
	_, _ = stdin.Write([]byte("password123\n"))
	waitFor("Any key to return.")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Enter selection:")

	// Email gateway validation path.
	_, _ = stdin.Write([]byte("G"))
	waitFor("Internet Tools")
	_, _ = stdin.Write([]byte("E"))
	waitFor("To external email:")
	_, _ = stdin.Write([]byte("\n"))
	waitFor("Subject:")
	_, _ = stdin.Write([]byte("gateway test\n"))
	waitFor("Body")
	_, _ = stdin.Write([]byte("body\n.\n"))
	waitFor("To/subject/body required. Press any key.")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Enter selection:")

	// Text web browser validation path.
	_, _ = stdin.Write([]byte("G"))
	waitFor("Internet Tools")
	_, _ = stdin.Write([]byte("W"))
	waitFor("URL:")
	_, _ = stdin.Write([]byte("ftp://example.org\n"))
	waitFor("Gateway blocked: url must use http or https")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Enter selection:")

	// Feed reader validation path.
	_, _ = stdin.Write([]byte("G"))
	waitFor("Internet Tools")
	_, _ = stdin.Write([]byte("F"))
	waitFor("Feed URL:")
	_, _ = stdin.Write([]byte("\n"))
	waitFor("Feed URL required. Press any key.")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Enter selection:")

	// Summarizer validation path.
	_, _ = stdin.Write([]byte("G"))
	waitFor("Internet Tools")
	_, _ = stdin.Write([]byte("S"))
	waitFor("Article URL:")
	_, _ = stdin.Write([]byte("\n"))
	waitFor("Article URL required. Press any key.")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Enter selection:")

	// JSON explorer validation path.
	_, _ = stdin.Write([]byte("G"))
	waitFor("Internet Tools")
	_, _ = stdin.Write([]byte("J"))
	waitFor("JSON URL:")
	_, _ = stdin.Write([]byte("\n"))
	waitFor("JSON URL required. Press any key.")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Enter selection:")

	// AI door default-not-configured path.
	_, _ = stdin.Write([]byte("G"))
	waitFor("Internet Tools")
	_, _ = stdin.Write([]byte("A"))
	waitFor("AI gateway is disabled. Configure /admin/gateways and set API key. Press any key.")
	_, _ = stdin.Write([]byte("x"))
	waitFor("Enter selection:")

	_, _ = stdin.Write([]byte("Q"))
	_ = session.Wait()
	<-done
}
