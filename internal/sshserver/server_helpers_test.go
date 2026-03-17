package sshserver

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" alpha, beta ,,gamma , ")
	if len(got) != 3 {
		t.Fatalf("expected 3 values, got %d", len(got))
	}
	if got[0] != "alpha" || got[1] != "beta" || got[2] != "gamma" {
		t.Fatalf("unexpected split output: %#v", got)
	}
}

func TestRandomTokenHex(t *testing.T) {
	token, err := randomTokenHex(24)
	if err != nil {
		t.Fatalf("random token: %v", err)
	}
	if len(token) != 48 {
		t.Fatalf("expected 48 chars, got %d", len(token))
	}
	if !regexp.MustCompile(`^[0-9a-f]+$`).MatchString(token) {
		t.Fatalf("token is not lowercase hex: %q", token)
	}
}

func TestIssueDownloadTicketValidation(t *testing.T) {
	s := &Server{}
	if _, err := s.issueDownloadTicket(1, 1, time.Minute); err == nil {
		t.Fatal("expected error when admin repository is unavailable")
	}
}

func TestResponsiveTTYFormatters(t *testing.T) {
	if got := formatCallerTTYRow(48, 1, "sysop", "02-28 19:30", "LAN", "192.168.1.20", "Main Menu", "00:00:08"); strings.Contains(got, "192.168.1.20") {
		t.Fatalf("expected compact caller row to omit long host, got %q", got)
	}
	for _, want := range []string{"sysop", "LAN", "Main Menu"} {
		if !strings.Contains(formatCallerTTYRow(48, 1, "sysop", "02-28 19:30", "LAN", "192.168.1.20", "Main Menu", "00:00:08"), want) {
			t.Fatalf("compact caller row missing %q", want)
		}
	}

	if got := formatBoardListRow(48, 12, "General Discussion", "General"); strings.Contains(got, "(General)") {
		t.Fatalf("expected compact board row to drop conference suffix, got %q", got)
	}

	if got := formatMailInboxRow(48, 7, "Long hello subject", "01-01 12:00", "new"); strings.Contains(got, "01-01 12:00") {
		t.Fatalf("expected compact mail row to drop timestamp, got %q", got)
	}

	if got := formatFileAreaRow(48, 1, "Uploads", "/bbs/files/uploads", "Default area"); strings.Contains(got, "Default area") {
		t.Fatalf("expected compact file area row to drop description, got %q", got)
	}
}
