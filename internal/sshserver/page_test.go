package sshserver

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
	"strings"
	"testing"

	"wolfbbs/internal/repository"
)

func TestQueuePageRequestPersistsSharedStore(t *testing.T) {
	adminRepo := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: adminRepo}

	if err := srv.queuePageRequest("alice", "bob", "Need help in doors", "ssh who-online / Node 1"); err != nil {
		t.Fatalf("queue page request: %v", err)
	}
	rows := srv.loadPageRequests()
	if len(rows) != 1 {
		t.Fatalf("expected one page request, got %d", len(rows))
	}
	if rows[0].From != "alice" || rows[0].To != "bob" {
		t.Fatalf("unexpected page routing row: %#v", rows[0])
	}
	if rows[0].Message == "" || rows[0].ID == "" {
		t.Fatalf("expected persisted id/message, got %#v", rows[0])
	}
}

func TestReadMessageBodyComposeHelpers(t *testing.T) {
	input := strings.Join([]string{
		"first line",
		"",
		"second line",
		"/preview",
		"/del",
		"/help",
		"third line",
		".",
	}, "\n") + "\n"
	reader := bufio.NewReader(strings.NewReader(input))
	var out bytes.Buffer

	body, err := readMessageBody(&out, reader, 20, 4096)
	if err != nil {
		t.Fatalf("read message body: %v", err)
	}
	if body != "first line\n\nthird line" {
		t.Fatalf("unexpected compose body: %q", body)
	}
	text := out.String()
	for _, needle := range []string{"Body> ", "draft preview", "Removed last line.", "Compose helpers"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected %q in compose output: %s", needle, text)
		}
	}
}

func TestReadLineHandlesBackspaceAndMaxLength(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("hellox\x7f\n"))
	got, err := readLine(reader, 16)
	if err != nil {
		t.Fatalf("read line with backspace: %v", err)
	}
	if got != "hello" {
		t.Fatalf("expected backspace-corrected input, got %q", got)
	}

	reader = bufio.NewReader(strings.NewReader("abcdef\n"))
	got, err = readLine(reader, 4)
	if err != nil {
		t.Fatalf("read line with limit: %v", err)
	}
	if got != "abcd" {
		t.Fatalf("expected max-length truncation, got %q", got)
	}

	reader = bufio.NewReader(strings.NewReader("quit\r\nx"))
	got, err = readLine(reader, 16)
	if err != nil {
		t.Fatalf("read line with CRLF: %v", err)
	}
	if got != "quit" {
		t.Fatalf("expected CRLF-terminated input to return quit, got %q", got)
	}
	next, err := reader.ReadByte()
	if err != nil {
		t.Fatalf("read trailing byte after CRLF line: %v", err)
	}
	if next != 'x' {
		t.Fatalf("expected CRLF pair to be consumed before next byte, got %q", string(next))
	}
}

func TestReadLineHandlesCursorMotionAndDelete(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("boars\x1b[Dd\n"))
	got, err := readLine(reader, 16)
	if err != nil {
		t.Fatalf("read line with cursor insert: %v", err)
	}
	if got != "boards" {
		t.Fatalf("expected cursor insertion to produce boards, got %q", got)
	}

	reader = bufio.NewReader(strings.NewReader("abcde\x1b[D\x1b[D\x1b[3~\n"))
	got, err = readLine(reader, 16)
	if err != nil {
		t.Fatalf("read line with delete: %v", err)
	}
	if got != "abce" {
		t.Fatalf("expected delete to remove current char, got %q", got)
	}

	reader = bufio.NewReader(strings.NewReader("tail\x1b[Hpre-\x1b[F!\n"))
	got, err = readLine(reader, 16)
	if err != nil {
		t.Fatalf("read line with home/end: %v", err)
	}
	if got != "pre-tail!" {
		t.Fatalf("expected home/end editing, got %q", got)
	}
}

func TestReadLineEchoesVisibleInputAndMasksSecrets(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("hello\n"))
	var out bytes.Buffer
	registerLineInput(reader, &out, true)
	defer unregisterLineInput(reader)

	got, err := readLine(reader, 16)
	if err != nil {
		t.Fatalf("read line with echo: %v", err)
	}
	if got != "hello" {
		t.Fatalf("expected echoed input to return hello, got %q", got)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "hello") {
		t.Fatalf("expected echo output to contain visible input, got %q", rendered)
	}
	if !strings.Contains(rendered, "\r\n") {
		t.Fatalf("expected echoed input to end with CRLF, got %q", rendered)
	}

	reader = bufio.NewReader(strings.NewReader("secret\n"))
	out.Reset()
	registerLineInput(reader, &out, true)
	restore := setLineInputMask(reader, true)
	defer unregisterLineInput(reader)
	got, err = readLine(reader, 16)
	restore()
	if err != nil {
		t.Fatalf("read masked line: %v", err)
	}
	if got != "secret" {
		t.Fatalf("expected masked input to preserve actual value, got %q", got)
	}
	rendered = out.String()
	if strings.Contains(rendered, "secret") {
		t.Fatalf("expected masked echo to hide raw value, got %q", rendered)
	}
	if !strings.Contains(rendered, "******") {
		t.Fatalf("expected masked echo to contain placeholder glyphs, got %q", rendered)
	}
}

func TestReadLineEchoesInPlainModeWhenPossible(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("mail\n"))
	var out bytes.Buffer
	registerLineInput(reader, &out, false)
	defer unregisterLineInput(reader)

	got, err := readLine(reader, 16)
	if err != nil {
		t.Fatalf("read line in plain mode: %v", err)
	}
	if got != "mail" {
		t.Fatalf("expected mail, got %q", got)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "mail") {
		t.Fatalf("expected plain mode echo to show typed text, got %q", rendered)
	}
}

func TestRenderFramePreservesRequestedLineBreaks(t *testing.T) {
	var out bytes.Buffer
	renderFrame(&out, 80, 80, "ONE\r\n", false, "")
	renderFrame(&out, 80, 80, "TWO\r\n", false, "")
	if got := out.String(); got != "ONE\r\nTWO\r\n" {
		t.Fatalf("renderFrame should preserve frame newlines, got %q", got)
	}

	out.Reset()
	renderFrame(io.Writer(&out), 100, 80, "BOX\r\n", false, "")
	if !strings.HasSuffix(out.String(), "\r\n") {
		t.Fatalf("expected centered frame to keep trailing newline, got %q", out.String())
	}
}

func TestPagerWriteHonorsContinueAndQuit(t *testing.T) {
	var lines []string
	for i := 1; i <= 34; i++ {
		lines = append(lines, "line "+strings.Repeat("0", 2-len(strconv.Itoa(i)))+strconv.Itoa(i))
	}
	text := strings.Join(lines, "\n")

	var out bytes.Buffer
	reader := bufio.NewReader(strings.NewReader(" q"))
	pagerWrite(&out, reader, text)
	rendered := out.String()
	if !strings.Contains(rendered, "line 01") || !strings.Contains(rendered, "line 32") {
		t.Fatalf("expected continued pager output, got %q", rendered)
	}
	if strings.Contains(rendered, "line 34") {
		t.Fatalf("expected pager quit to stop final page, got %q", rendered)
	}
	if !strings.Contains(rendered, "More") {
		t.Fatalf("expected pager footer prompt, got %q", rendered)
	}
}
