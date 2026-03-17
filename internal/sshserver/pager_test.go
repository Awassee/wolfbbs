package sshserver

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestPagerWriteShowsMorePrompt(t *testing.T) {
	var out bytes.Buffer
	input := bufio.NewReader(strings.NewReader(" \n"))
	lines := make([]string, 0, 32)
	for i := 0; i < 32; i++ {
		lines = append(lines, "line "+strings.Repeat("x", i%5))
	}
	pagerWrite(&out, input, strings.Join(lines, "\n"))
	rendered := out.String()
	if !strings.Contains(rendered, "-- More --") {
		t.Fatalf("expected pager output to include More prompt, got %q", rendered)
	}
}

func TestPagerWriteQuitStopsFurtherPages(t *testing.T) {
	var out bytes.Buffer
	input := bufio.NewReader(strings.NewReader("q"))
	lines := make([]string, 0, 42)
	for i := 0; i < 42; i++ {
		lines = append(lines, "row")
	}
	pagerWrite(&out, input, strings.Join(lines, "\n"))
	rendered := out.String()
	if count := strings.Count(rendered, "-- More --"); count != 1 {
		t.Fatalf("expected one More prompt when quitting early, got %d", count)
	}
}
