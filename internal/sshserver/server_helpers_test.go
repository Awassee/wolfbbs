package sshserver

import (
	"regexp"
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
