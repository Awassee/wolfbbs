package sshserver

import (
	"bufio"
	"bytes"
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
	if body != "first line\nthird line" {
		t.Fatalf("unexpected compose body: %q", body)
	}
	text := out.String()
	for _, needle := range []string{"draft preview", "Removed last line.", "Compose helpers"} {
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
