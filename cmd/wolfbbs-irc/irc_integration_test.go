package main

import (
	"bufio"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/repository"
)

func TestIRCGatewayFlow(t *testing.T) {
	resetIRCStateForTest()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	svc := chat.NewServiceForTest()
	repo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(repo)
	_, _ = authSvc.Register("ircuser", "ircpass1")
	_, _ = authSvc.Register("ircviewer", "ircpass2")

	go func() {
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
	history := svc.History("#lobby", 20)
	if len(history) == 0 {
		t.Fatal("chat history should include irc message")
	}
	if history[len(history)-1].Body != "integration test" {
		t.Fatalf("unexpected history last message: %+v", history[len(history)-1])
	}

	_, _ = sender.Write([]byte("QUIT :done\r\n"))
	_, _ = receiver.Write([]byte("QUIT :done\r\n"))
}

func TestIRCGatewayModerationEnforced(t *testing.T) {
	resetIRCStateForTest()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	svc := chat.NewServiceForTest()
	repo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(repo)
	_, _ = authSvc.Register("ircuser", "ircpass1")
	_, _ = authSvc.Register("sysop", "syspass1")

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		handleIRCConn(conn, svc, authSvc, "127.0.0.1")
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	reader := bufio.NewReader(client)

	if !waitForReaderLineContains(client, reader, "Welcome", 3*time.Second) {
		t.Fatal("missing initial welcome")
	}

	_, _ = client.Write([]byte("PASS ircpass1\r\n"))
	_, _ = client.Write([]byte("NICK ircuser\r\n"))
	_, _ = client.Write([]byte("USER ircuser 0 * :ircuser\r\n"))
	_, _ = client.Write([]byte("JOIN #lobby\r\n"))
	if !waitForLineContains(client, " 366 ", 3*time.Second) {
		t.Fatal("join did not complete")
	}

	svc.Mute("#lobby", "ircuser", "sysop", "integration mute", "2m")
	_, _ = client.Write([]byte("PRIVMSG #lobby :muted message\r\n"))
	if !waitForLineContains(client, " 437 ", 3*time.Second) {
		t.Fatal("expected moderated PRIVMSG to fail with numeric 437")
	}

	svc.Unmute("#lobby", "ircuser")
	_, _ = client.Write([]byte("PRIVMSG #lobby :after unmute\r\n"))
	if !waitForLineContains(client, "after unmute", 3*time.Second) {
		t.Fatal("expected unmuted user message to deliver")
	}
}

func TestIRCGatewayWhoisReportsLiveSessionDetails(t *testing.T) {
	resetIRCStateForTest()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	svc := chat.NewServiceForTest()
	repo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(repo)
	_, _ = authSvc.Register("ircuser", "ircpass1")

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		handleIRCConn(conn, svc, authSvc, "127.0.0.1")
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	reader := bufio.NewReader(client)

	if !waitForReaderLineContains(client, reader, "Welcome", 3*time.Second) {
		t.Fatal("missing initial welcome")
	}

	_, _ = client.Write([]byte("PASS ircpass1\r\n"))
	_, _ = client.Write([]byte("NICK ircuser\r\n"))
	_, _ = client.Write([]byte("USER ircuser 0 * :IRC Integration User\r\n"))
	_, _ = client.Write([]byte("JOIN #lobby\r\n"))
	joinLines := collectReaderLinesUntil(client, reader, " 366 ", 3*time.Second)
	if !lineSliceContains(joinLines, " 366 ") {
		t.Fatalf("join did not complete; lines=%#v", joinLines)
	}

	_, _ = client.Write([]byte("WHOIS ircuser\r\n"))
	lines := collectReaderLinesUntil(client, reader, " 318 ", 3*time.Second)
	assertContainsLine(t, lines, " 311 ")
	assertContainsLine(t, lines, " 319 ")
	assertContainsLine(t, lines, " 312 ")
	assertContainsLine(t, lines, " 317 ")

	var signonSeen bool
	for _, line := range lines {
		if !strings.Contains(line, " 317 ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 7 {
			t.Fatalf("unexpected 317 fields: %q", line)
		}
		idle, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			t.Fatalf("parse idle seconds: %v line=%q", err, line)
		}
		signon, err := strconv.ParseInt(fields[5], 10, 64)
		if err != nil {
			t.Fatalf("parse signon: %v line=%q", err, line)
		}
		if idle < 0 {
			t.Fatalf("expected non-negative idle seconds, got %d", idle)
		}
		if signon <= 0 {
			t.Fatalf("expected positive signon timestamp, got %d", signon)
		}
		signonSeen = true
	}
	if !signonSeen {
		t.Fatal("expected WHOIS 317 numeric")
	}

	_, _ = client.Write([]byte("WHOIS ghost\r\n"))
	missingLines := collectReaderLinesUntil(client, reader, " 318 ", 3*time.Second)
	assertContainsLine(t, missingLines, " 401 ")
}

func TestIRCGatewayNoticeAndCTCPRelay(t *testing.T) {
	resetIRCStateForTest()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	svc := chat.NewServiceForTest()
	repo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(repo)
	_, _ = authSvc.Register("ircuser", "ircpass1")
	_, _ = authSvc.Register("ircviewer", "ircpass2")

	go func() {
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

	senderReader := bufio.NewReader(sender)
	receiverReader := bufio.NewReader(receiver)

	if !waitForReaderLineContains(sender, senderReader, "Welcome", 3*time.Second) {
		t.Fatal("sender did not receive IRC welcome")
	}
	if !waitForReaderLineContains(receiver, receiverReader, "Welcome", 3*time.Second) {
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

	joinLines := collectReaderLinesUntil(receiver, receiverReader, " 366 ", 3*time.Second)
	if !lineSliceContains(joinLines, " 366 ") {
		t.Fatalf("receiver join did not complete; lines=%#v", joinLines)
	}
	joinLines = collectReaderLinesUntil(sender, senderReader, " 366 ", 3*time.Second)
	if !lineSliceContains(joinLines, " 366 ") {
		t.Fatalf("sender join did not complete; lines=%#v", joinLines)
	}

	_, _ = sender.Write([]byte("NOTICE ircviewer :direct notice\r\n"))
	directLines := collectReaderLinesUntil(receiver, receiverReader, "direct notice", 3*time.Second)
	assertContainsLine(t, directLines, " NOTICE ircviewer :direct notice")

	_, _ = sender.Write([]byte("NOTICE #lobby :channel notice\r\n"))
	channelNoticeLines := collectReaderLinesUntil(receiver, receiverReader, "channel notice", 3*time.Second)
	assertContainsLine(t, channelNoticeLines, " NOTICE #lobby :channel notice")

	_, _ = sender.Write([]byte("PRIVMSG #lobby :\u0001ACTION waves\u0001\r\n"))
	actionLines := collectReaderLinesUntil(receiver, receiverReader, "ACTION waves", 3*time.Second)
	assertContainsLine(t, actionLines, "\u0001ACTION waves\u0001")
}

func TestIRCGatewayPartBroadcastsToPeers(t *testing.T) {
	resetIRCStateForTest()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	svc := chat.NewServiceForTest()
	repo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(repo)
	_, _ = authSvc.Register("ircuser", "ircpass1")
	_, _ = authSvc.Register("ircviewer", "ircpass2")

	go func() {
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

	senderReader := bufio.NewReader(sender)
	receiverReader := bufio.NewReader(receiver)

	if !waitForReaderLineContains(sender, senderReader, "Welcome", 3*time.Second) {
		t.Fatal("sender did not receive IRC welcome")
	}
	if !waitForReaderLineContains(receiver, receiverReader, "Welcome", 3*time.Second) {
		t.Fatal("receiver did not receive IRC welcome")
	}

	_, _ = sender.Write([]byte("PASS ircpass1\r\n"))
	_, _ = sender.Write([]byte("NICK ircuser\r\n"))
	_, _ = sender.Write([]byte("USER ircuser 0 * :ircuser\r\n"))
	_, _ = sender.Write([]byte("JOIN #lobby\r\n"))
	joinLines := collectReaderLinesUntil(sender, senderReader, " 366 ", 3*time.Second)
	if !lineSliceContains(joinLines, " 366 ") {
		t.Fatalf("sender join did not complete; lines=%#v", joinLines)
	}

	_, _ = receiver.Write([]byte("PASS ircpass2\r\n"))
	_, _ = receiver.Write([]byte("NICK ircviewer\r\n"))
	_, _ = receiver.Write([]byte("USER ircviewer 0 * :ircviewer\r\n"))
	_, _ = receiver.Write([]byte("JOIN #lobby\r\n"))
	joinLines = collectReaderLinesUntil(receiver, receiverReader, " 366 ", 3*time.Second)
	if !lineSliceContains(joinLines, " 366 ") {
		t.Fatalf("receiver join did not complete; lines=%#v", joinLines)
	}

	_, _ = receiver.Write([]byte("PART #lobby\r\n"))
	partLines := collectReaderLinesUntil(sender, senderReader, " PART #lobby ", 3*time.Second)
	assertContainsLine(t, partLines, ":ircviewer PART #lobby :left")
}

func TestIRCGatewayKickBroadcastsAndAllowsRejoin(t *testing.T) {
	resetIRCStateForTest()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	svc := chat.NewServiceForTest()
	repo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(repo)
	if _, err := authSvc.Register("sysop", "syspass12"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", "sysop"); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	if _, err := authSvc.Register("reader", "readerpass1"); err != nil {
		t.Fatalf("register reader: %v", err)
	}

	go func() {
		for i := 0; i < 2; i++ {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleIRCConn(conn, svc, authSvc, "127.0.0.1")
		}
	}()

	sysopConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial sysop: %v", err)
	}
	defer sysopConn.Close()
	readerConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial reader: %v", err)
	}
	defer readerConn.Close()

	sysopReader := bufio.NewReader(sysopConn)
	readerReader := bufio.NewReader(readerConn)

	if !waitForReaderLineContains(sysopConn, sysopReader, "Welcome", 3*time.Second) {
		t.Fatal("sysop welcome missing")
	}
	if !waitForReaderLineContains(readerConn, readerReader, "Welcome", 3*time.Second) {
		t.Fatal("reader welcome missing")
	}

	_, _ = sysopConn.Write([]byte("PASS syspass12\r\n"))
	_, _ = sysopConn.Write([]byte("NICK sysop\r\n"))
	_, _ = sysopConn.Write([]byte("USER sysop 0 * :sysop\r\n"))
	_, _ = sysopConn.Write([]byte("JOIN #lobby\r\n"))
	joinLines := collectReaderLinesUntil(sysopConn, sysopReader, " 366 ", 3*time.Second)
	if !lineSliceContains(joinLines, " 366 ") {
		t.Fatalf("sysop join did not complete; lines=%#v", joinLines)
	}

	_, _ = readerConn.Write([]byte("PASS readerpass1\r\n"))
	_, _ = readerConn.Write([]byte("NICK reader\r\n"))
	_, _ = readerConn.Write([]byte("USER reader 0 * :reader\r\n"))
	_, _ = readerConn.Write([]byte("JOIN #lobby\r\n"))
	joinLines = collectReaderLinesUntil(readerConn, readerReader, " 366 ", 3*time.Second)
	if !lineSliceContains(joinLines, " 366 ") {
		t.Fatalf("reader join did not complete; lines=%#v", joinLines)
	}

	_, _ = sysopConn.Write([]byte("KICK #lobby reader\r\n"))
	kickLines := collectReaderLinesUntil(sysopConn, sysopReader, " KICK #lobby reader ", 3*time.Second)
	assertContainsLine(t, kickLines, ":sysop KICK #lobby reader :irc kick")
	kickLines = collectReaderLinesUntil(readerConn, readerReader, " KICK #lobby reader ", 3*time.Second)
	assertContainsLine(t, kickLines, ":sysop KICK #lobby reader :irc kick")

	_, _ = readerConn.Write([]byte("JOIN #lobby\r\n"))
	rejoinLines := collectReaderLinesUntil(readerConn, readerReader, " 366 ", 3*time.Second)
	if !lineSliceContains(rejoinLines, " 366 ") {
		t.Fatalf("reader rejoin did not complete; lines=%#v", rejoinLines)
	}
}

func TestIRCGatewayAwayReportsOnPrivmsgAndWhois(t *testing.T) {
	resetIRCStateForTest()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	svc := chat.NewServiceForTest()
	repo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(repo)
	_, _ = authSvc.Register("alice", "alicepass1")
	_, _ = authSvc.Register("bob", "bobpass12")

	go func() {
		for i := 0; i < 2; i++ {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleIRCConn(conn, svc, authSvc, "127.0.0.1")
		}
	}()

	alice, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial alice: %v", err)
	}
	defer alice.Close()
	bob, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial bob: %v", err)
	}
	defer bob.Close()

	aliceReader := bufio.NewReader(alice)
	bobReader := bufio.NewReader(bob)

	if !waitForReaderLineContains(alice, aliceReader, "Welcome", 3*time.Second) {
		t.Fatal("alice did not receive IRC welcome")
	}
	if !waitForReaderLineContains(bob, bobReader, "Welcome", 3*time.Second) {
		t.Fatal("bob did not receive IRC welcome")
	}

	_, _ = alice.Write([]byte("PASS alicepass1\r\n"))
	_, _ = alice.Write([]byte("NICK alice\r\n"))
	_, _ = alice.Write([]byte("USER alice 0 * :Alice\r\n"))

	_, _ = bob.Write([]byte("PASS bobpass12\r\n"))
	_, _ = bob.Write([]byte("NICK bob\r\n"))
	_, _ = bob.Write([]byte("USER bob 0 * :Bob\r\n"))

	_, _ = bob.Write([]byte("AWAY :back after lunch\r\n"))
	awaySetLines := collectReaderLinesUntil(bob, bobReader, " 306 ", 3*time.Second)
	assertContainsLine(t, awaySetLines, " 306 ")

	_, _ = alice.Write([]byte("PRIVMSG bob :ping while away\r\n"))
	awayNoticeLines := collectReaderLinesUntil(alice, aliceReader, " 301 ", 3*time.Second)
	assertContainsLine(t, awayNoticeLines, " 301 ")
	assertContainsLine(t, awayNoticeLines, "back after lunch")

	_, _ = alice.Write([]byte("WHOIS bob\r\n"))
	whoisLines := collectReaderLinesUntil(alice, aliceReader, " 318 ", 3*time.Second)
	assertContainsLine(t, whoisLines, " 301 ")

	_, _ = bob.Write([]byte("AWAY\r\n"))
	awayClearLines := collectReaderLinesUntil(bob, bobReader, " 305 ", 3*time.Second)
	assertContainsLine(t, awayClearLines, " 305 ")
}

func TestIRCGatewayNamesMarksModeratorsAsOperators(t *testing.T) {
	resetIRCStateForTest()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	svc := chat.NewServiceForTest()
	repo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(repo)
	if _, err := authSvc.Register("sysop", "syspass12"); err != nil {
		t.Fatalf("register sysop: %v", err)
	}
	if err := authSvc.SetRole("sysop", "sysop"); err != nil {
		t.Fatalf("set sysop role: %v", err)
	}
	if _, err := authSvc.Register("reader", "readerpass1"); err != nil {
		t.Fatalf("register reader: %v", err)
	}

	go func() {
		for i := 0; i < 2; i++ {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleIRCConn(conn, svc, authSvc, "127.0.0.1")
		}
	}()

	sysopConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial sysop: %v", err)
	}
	defer sysopConn.Close()
	readerConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial reader: %v", err)
	}
	defer readerConn.Close()

	sysopReader := bufio.NewReader(sysopConn)
	readerReader := bufio.NewReader(readerConn)

	if !waitForReaderLineContains(sysopConn, sysopReader, "Welcome", 3*time.Second) {
		t.Fatal("sysop welcome missing")
	}
	if !waitForReaderLineContains(readerConn, readerReader, "Welcome", 3*time.Second) {
		t.Fatal("reader welcome missing")
	}

	_, _ = sysopConn.Write([]byte("PASS syspass12\r\n"))
	_, _ = sysopConn.Write([]byte("NICK sysop\r\n"))
	_, _ = sysopConn.Write([]byte("USER sysop 0 * :sysop\r\n"))
	_, _ = sysopConn.Write([]byte("JOIN #lobby\r\n"))
	sysopJoinLines := collectReaderLinesUntil(sysopConn, sysopReader, " 366 ", 3*time.Second)
	if !lineSliceContains(sysopJoinLines, " 366 ") {
		t.Fatalf("sysop join did not complete; lines=%#v", sysopJoinLines)
	}

	_, _ = readerConn.Write([]byte("PASS readerpass1\r\n"))
	_, _ = readerConn.Write([]byte("NICK reader\r\n"))
	_, _ = readerConn.Write([]byte("USER reader 0 * :reader\r\n"))
	_, _ = readerConn.Write([]byte("JOIN #lobby\r\n"))

	joinLines := collectReaderLinesUntil(readerConn, readerReader, " 366 ", 3*time.Second)
	if !lineSliceContains(joinLines, " 366 ") {
		t.Fatalf("reader join did not complete; lines=%#v", joinLines)
	}
	_, _ = readerConn.Write([]byte("NAMES #lobby\r\n"))
	nameLines := collectReaderLinesUntil(readerConn, readerReader, " 366 ", 3*time.Second)
	assertContainsLine(t, nameLines, " 353 ")
	assertContainsLine(t, nameLines, "@sysop")
}

func waitForLineContains(conn net.Conn, needle string, timeout time.Duration) bool {
	r := bufio.NewReader(conn)
	return waitForReaderLineContains(conn, r, needle, timeout)
}

func waitForReaderLineContains(conn net.Conn, r *bufio.Reader, needle string, timeout time.Duration) bool {
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

func collectLinesUntil(conn net.Conn, needle string, timeout time.Duration) []string {
	r := bufio.NewReader(conn)
	return collectReaderLinesUntil(conn, r, needle, timeout)
}

func collectReaderLinesUntil(conn net.Conn, r *bufio.Reader, needle string, timeout time.Duration) []string {
	deadline := time.Now().Add(timeout)
	lines := make([]string, 0, 8)
	for {
		if time.Now().After(deadline) {
			return lines
		}
		_ = conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		line, err := r.ReadString('\n')
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return lines
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if strings.Contains(line, needle) {
			return lines
		}
	}
}

func assertContainsLine(t *testing.T, lines []string, needle string) {
	t.Helper()
	if lineSliceContains(lines, needle) {
		return
	}
	t.Fatalf("expected line containing %q in %#v", needle, lines)
}

func lineSliceContains(lines []string, needle string) bool {
	for _, line := range lines {
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}

func resetIRCStateForTest() {
	connectionMu.Lock()
	activeByIP = map[string]int{}
	activeTotal = 0
	connectionMu.Unlock()

	ipFloodMu.Lock()
	ipHits = map[string][]time.Time{}
	ipFloodMu.Unlock()

	clientsMu.Lock()
	clientsByNick = map[string]*ircClient{}
	channelPeers = map[string]map[string]*ircClient{}
	clientsMu.Unlock()
}
