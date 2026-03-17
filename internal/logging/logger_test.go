package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestBunyanLevelMapping(t *testing.T) {
	if got := bunyanLevel(slog.LevelDebug); got != 20 {
		t.Fatalf("debug level = %d, want 20", got)
	}
	if got := bunyanLevel(slog.LevelInfo); got != 30 {
		t.Fatalf("info level = %d, want 30", got)
	}
	if got := bunyanLevel(slog.LevelWarn); got != 40 {
		t.Fatalf("warn level = %d, want 40", got)
	}
	if got := bunyanLevel(slog.LevelError); got != 50 {
		t.Fatalf("error level = %d, want 50", got)
	}
}

func TestBunyanStdWriter(t *testing.T) {
	var out bytes.Buffer
	w := &bunyanStdWriter{service: "wolfbbs-test", out: &out}
	_, err := w.Write([]byte("hello world\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	line := strings.TrimSpace(out.String())
	if line == "" {
		t.Fatal("expected output")
	}
	var row map[string]interface{}
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if row["name"] != "wolfbbs-test" {
		t.Fatalf("name=%v, want wolfbbs-test", row["name"])
	}
	if row["msg"] != "hello world" {
		t.Fatalf("msg=%v, want hello world", row["msg"])
	}
}
