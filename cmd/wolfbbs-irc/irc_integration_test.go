package main

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/chat"
)

func TestIRCGatewayFlow(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		handleIRCConn(conn, chat.NewService())
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	if !waitForLineContains(r, "welcome", 3*time.Second) {
		t.Fatal("did not receive IRC welcome")
	}
	_, _ = conn.Write([]byte("NICK ircuser\r\n"))
	_, _ = conn.Write([]byte("USER ircuser 0 * :ircuser\r\n"))
	_, _ = conn.Write([]byte("JOIN #lobby\r\n"))
	_, _ = conn.Write([]byte("PRIVMSG #lobby :integration test\r\n"))
	_, _ = conn.Write([]byte("QUIT :done\r\n"))
	if !waitForLineContains(r, "integration test", 3*time.Second) {
		t.Fatal("did not receive echoed chat message")
	}
}

func waitForLineContains(r *bufio.Reader, needle string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		line, err := r.ReadString('\n')
		if err != nil {
			continue
		}
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}
