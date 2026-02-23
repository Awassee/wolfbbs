package sshserver_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"wolfbbs/internal/auth"
	"wolfbbs/internal/repository"
	"wolfbbs/internal/sshserver"
)

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

	var out bytes.Buffer
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
	waitFor("Enter selection:")
	_, _ = stdin.Write([]byte("Q\n"))

	_ = session.Wait()
	<-done
}
