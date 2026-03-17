package logging

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	FormatBunyan = "bunyan"
	FormatJSON   = "json"
	FormatText   = "text"
)

func NewLogger(service string) *slog.Logger {
	service = strings.TrimSpace(service)
	if service == "" {
		service = "wolfbbs"
	}
	format := strings.ToLower(strings.TrimSpace(os.Getenv("WOLFBBS_LOG_FORMAT")))
	if format == "" {
		format = FormatBunyan
	}
	level := parseLevel(strings.TrimSpace(os.Getenv("WOLFBBS_LOG_LEVEL")))
	switch format {
	case FormatText:
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})).With("service", service)
	case FormatJSON:
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})).With("service", service)
	default:
		h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
			ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				if a.Key == slog.LevelKey {
					if lv, ok := a.Value.Any().(slog.Level); ok {
						return slog.Int("level", bunyanLevel(lv))
					}
				}
				return a
			},
		})
		return slog.New(h).With("name", service, "v", 0)
	}
}

func ConfigureStdLogger(service string) {
	format := strings.ToLower(strings.TrimSpace(os.Getenv("WOLFBBS_LOG_FORMAT")))
	if format == "" {
		format = FormatBunyan
	}
	log.SetFlags(0)
	if format == FormatBunyan {
		log.SetOutput(&bunyanStdWriter{service: strings.TrimSpace(service), out: os.Stdout})
		return
	}
	log.SetOutput(os.Stdout)
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func bunyanLevel(level slog.Level) int {
	switch {
	case level <= slog.LevelDebug:
		return 20
	case level >= slog.LevelError:
		return 50
	case level >= slog.LevelWarn:
		return 40
	default:
		return 30
	}
}

type bunyanStdWriter struct {
	mu      sync.Mutex
	service string
	out     io.Writer
}

func (w *bunyanStdWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	lines := bytes.Split(p, []byte("\n"))
	for _, line := range lines {
		msg := strings.TrimSpace(string(line))
		if msg == "" {
			continue
		}
		row := map[string]interface{}{
			"time":  time.Now().UTC().Format(time.RFC3339Nano),
			"level": 30,
			"msg":   msg,
			"name":  fallback(w.service, "wolfbbs"),
			"v":     0,
		}
		raw, _ := json.Marshal(row)
		_, _ = w.out.Write(append(raw, '\n'))
	}
	return len(p), nil
}

func fallback(value, alt string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return alt
	}
	return value
}
