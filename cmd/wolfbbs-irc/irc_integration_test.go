package main

import (
	"bufio"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/repository"
)

func TestIRCGatewayFlow(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		svc := chat.NewService()
		repo := repository.NewInMemoryUserRepository()
		authSvc := auth.NewService(repo)
		_, _ = authSvc.Register("ircuser", "ircpass1")
		_, _ = authSvc.Register("ircviewer", "ircpass2")
		for i := 0; i < 2; i++ {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleIRCConn(conn, svc, authSvc, "127.0.0.1")
		}
	}()

	sender, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial sender: %v", err)
	}
	defer sender.Close()
	receiver, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial receiver: %v", err)
	}
	defer receiver.Close()

	if !waitForLineContains(sender, "Welcome", 3*time.Second) {
		t.Fatal("sender did not receive IRC welcome")
	}
	if !waitForLineContains(receiver, "Welcome", 3*time.Second) {
		t.Fatal("receiver did not receive IRC welcome")
	}

	_, _ = sender.Write([]byte("PASS ircpass1\r\n"))
	_, _ = sender.Write([]byte("NICK ircuser\r\n"))
	_, _ = sender.Write([]byte("USER ircuser 0 * :ircuser\r\n"))
	_, _ = sender.Write([]byte("JOIN #lobby\r\n"))

	_, _ = receiver.Write([]byte("PASS ircpass2\r\n"))
	_, _ = receiver.Write([]byte("NICK ircviewer\r\n"))
	_, _ = receiver.Write([]byte("USER ircviewer 0 * :ircviewer\r\n"))
	_, _ = receiver.Write([]byte("JOIN #lobby\r\n"))

	if !waitForLineContains(sender, "353", 2*time.Second) {
		t.Fatal("sender did not finish join")
	}
	if !waitForLineContains(receiver, "353", 2*time.Second) {
		t.Fatal("receiver did not finish join")
	}

	_, _ = sender.Write([]byte("PRIVMSG #lobby :integration test\r\n"))
	if !waitForLineContains(receiver, "integration test", 3*time.Second) {
		t.Fatal("receiver did not receive integrated message")
	}

	_, _ = sender.Write([]byte("QUIT :done\r\n"))
	_, _ = receiver.Write([]byte("QUIT :done\r\n"))
}

func waitForLineContains(conn net.Conn, needle string, timeout time.Duration) bool {
	r := bufio.NewReader(conn)
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return false
		}
		_ = conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		line, err := r.ReadString('\n')
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if err == io.EOF {
				return false
			}
			return false
		}
		if strings.Contains(line, needle) {
			return true
		}
	}
}
