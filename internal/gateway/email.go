package gateway

import (
	"errors"
	"fmt"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type EmailConfig struct {
	Host             string
	Port             int
	User             string
	Pass             string
	FromDomain       string
	MaxRecipients    int
	MaxMessageBytes  int
	RateLimitPerHour int
}

type EmailGateway struct {
	cfg       EmailConfig
	mu        sync.Mutex
	sendTimes map[string][]time.Time
}

func LoadEmailConfigFromEnv() EmailConfig {
	cfg := EmailConfig{
		Host:             strings.TrimSpace(firstEnv("SMTP_HOST", "WOLFBBS_SMTP_HOST")),
		Port:             587,
		User:             strings.TrimSpace(firstEnv("SMTP_USER", "WOLFBBS_SMTP_USER")),
		Pass:             strings.TrimSpace(firstEnv("SMTP_PASS", "WOLFBBS_SMTP_PASS")),
		FromDomain:       strings.TrimSpace(firstEnv("FROM_DOMAIN", "WOLFBBS_FROM_DOMAIN")),
		MaxRecipients:    3,
		MaxMessageBytes:  64 * 1024,
		RateLimitPerHour: 20,
	}
	if value := strings.TrimSpace(firstEnv("SMTP_PORT", "WOLFBBS_SMTP_PORT")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			cfg.Port = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("WOLFBBS_MAIL_MAX_RECIPIENTS")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			cfg.MaxRecipients = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("WOLFBBS_MAIL_MAX_BYTES")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			cfg.MaxMessageBytes = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("WOLFBBS_MAIL_RATE_PER_HOUR")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			cfg.RateLimitPerHour = parsed
		}
	}
	return cfg
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func NewEmailGateway(cfg EmailConfig) *EmailGateway {
	return &EmailGateway{
		cfg:       cfg,
		sendTimes: map[string][]time.Time{},
	}
}

func (g *EmailGateway) Enabled() bool {
	return strings.TrimSpace(g.cfg.Host) != "" && g.cfg.Port > 0 && strings.TrimSpace(g.cfg.FromDomain) != ""
}

func (g *EmailGateway) ValidateOutbound(to []string, subject, body string) error {
	if len(to) == 0 {
		return errors.New("at least one recipient is required")
	}
	if len(to) > g.cfg.MaxRecipients {
		return fmt.Errorf("too many recipients (max %d)", g.cfg.MaxRecipients)
	}
	if strings.TrimSpace(subject) == "" {
		return errors.New("subject is required")
	}
	if strings.ContainsAny(subject, "\r\n") {
		return errors.New("subject must not contain CR/LF characters")
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("body is required")
	}
	if len(subject)+len(body) > g.cfg.MaxMessageBytes {
		return fmt.Errorf("message exceeds %d bytes", g.cfg.MaxMessageBytes)
	}
	for _, recipient := range to {
		if strings.ContainsAny(recipient, "\r\n") {
			return fmt.Errorf("invalid recipient %q", recipient)
		}
		if _, err := mail.ParseAddress(strings.TrimSpace(recipient)); err != nil {
			return fmt.Errorf("invalid recipient %q", recipient)
		}
	}
	return nil
}

func (g *EmailGateway) checkRate(handle string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now().UTC()
	cutoff := now.Add(-time.Hour)
	handle = strings.ToLower(strings.TrimSpace(handle))
	window := g.sendTimes[handle]
	kept := window[:0]
	for _, row := range window {
		if row.After(cutoff) {
			kept = append(kept, row)
		}
	}
	if len(kept) >= g.cfg.RateLimitPerHour {
		g.sendTimes[handle] = kept
		return fmt.Errorf("outbound rate limit exceeded (%d/hour)", g.cfg.RateLimitPerHour)
	}
	g.sendTimes[handle] = append(kept, now)
	return nil
}

func (g *EmailGateway) SendOutbound(fromHandle string, to []string, subject, body string) error {
	if err := g.ValidateOutbound(to, subject, body); err != nil {
		return err
	}
	if err := g.checkRate(fromHandle); err != nil {
		return err
	}
	if !g.Enabled() {
		return errors.New("smtp relay is not configured")
	}

	fromLocal := sanitizeLocalPart(fromHandle)
	from := fmt.Sprintf("%s@%s", fromLocal, g.cfg.FromDomain)
	addr := fmt.Sprintf("%s:%d", g.cfg.Host, g.cfg.Port)
	headers := []string{
		"From: " + from,
		"To: " + strings.Join(to, ", "),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"X-Mailer: WolfBBS",
	}
	msg := strings.Join(headers, "\r\n") + "\r\n\r\n" + body + "\r\n"

	var auth smtp.Auth
	if g.cfg.User != "" {
		auth = smtp.PlainAuth("", g.cfg.User, g.cfg.Pass, g.cfg.Host)
	}
	return smtp.SendMail(addr, auth, from, to, []byte(msg))
}

func sanitizeLocalPart(handle string) string {
	handle = strings.ToLower(strings.TrimSpace(handle))
	if handle == "" {
		return "wolfbbs"
	}
	var b strings.Builder
	for _, r := range handle {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('-')
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "wolfbbs"
	}
	return out
}

func ParseInboundRecipient(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	if strings.Contains(value, "<") && strings.Contains(value, ">") {
		if addr, err := mail.ParseAddress(value); err == nil {
			value = strings.ToLower(strings.TrimSpace(addr.Address))
		}
	}
	local := value
	if at := strings.Index(local, "@"); at > 0 {
		local = local[:at]
	}
	if plus := strings.Index(local, "+"); plus > 0 {
		local = local[:plus]
	}
	local = strings.TrimSpace(local)
	if local == "" {
		return ""
	}
	return local
}
