package gateway

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	defaultTimeoutSeconds = 10
	defaultMaxBytes      = 2 * 1024 * 1024
	defaultRedirectLimit = 4
	defaultUserAgent     = "WolfBBS text gateway"
	defaultLineWidth     = 78
)

var (
	// Remove comments, scripts and styles before extracting text.
	reComment       = regexp.MustCompile(`(?s)<!--.*?-->`)
	reScript        = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reStyle         = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	reLink          = regexp.MustCompile(`(?is)<a[^>]*href\s*=\s*["']([^"']+)["'][^>]*>(.*?)</a>`)
	reBlockSpacer   = regexp.MustCompile(`(?i)<\s*(br|/p|/h[1-6]|/div|/li|/tr|/table|hr)\s*>`)
	reTag           = regexp.MustCompile(`(?is)<[^>]+>`)
	reWhitespace    = regexp.MustCompile(`[\t\r ]+`)
	reMultiNewlines = regexp.MustCompile(`\n{3,}`)
)

type FetchConfig struct {
	Timeout           time.Duration
	MaxBodyBytes      int64
	MaxRedirects      int
	UserAgent         string
	AllowedTypes      []string
}

// DefaultFetchConfig mirrors the initial MVP web gateway safety defaults.
var DefaultFetchConfig = FetchConfig{
	Timeout:      defaultTimeoutSeconds * time.Second,
	MaxBodyBytes: defaultMaxBytes,
	MaxRedirects: defaultRedirectLimit,
	UserAgent:    defaultUserAgent,
	AllowedTypes: []string{
		"text/html",
		"text/plain",
		"application/xhtml+xml",
	},
}

// FetchText downloads a URL and returns wrapped, sanitized text.
func FetchText(ctx context.Context, rawURL string, cfg FetchConfig) (string, error) {
	cfg = sanitizeFetchConfig(cfg)
	if err := validateGatewayURL(rawURL); err != nil {
		return "", err
	}

	client := &http.Client{
		Timeout: cfg.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > cfg.MaxRedirects {
				return fmt.Errorf("too many redirects (max=%d)", cfg.MaxRedirects)
			}
			return validateGatewayURL(req.URL.String())
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", cfg.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http status %s", resp.Status)
	}

	if !isAllowedContentType(resp.Header.Get("Content-Type"), cfg.AllowedTypes) {
		return "", fmt.Errorf("content type blocked: %s", resp.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, cfg.MaxBodyBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(body)) > cfg.MaxBodyBytes {
		return "", fmt.Errorf("response body exceeds limit (%d bytes)", cfg.MaxBodyBytes)
	}

	render := sanitizeAndWrap(string(body), defaultLineWidth)
	return render, nil
}

func sanitizeAndWrap(body string, width int) string {
	text := strings.TrimSpace(body)
	text = reComment.ReplaceAllString(text, "")
	text = reScript.ReplaceAllString(text, "")
	text = reStyle.ReplaceAllString(text, "")
	text = reLink.ReplaceAllString(text, "$2 [$1]")
	text = reBlockSpacer.ReplaceAllString(text, "\n")
	text = reTag.ReplaceAllString(text, "")
	text = html.UnescapeString(text)
	text = reWhitespace.ReplaceAllString(text, " ")
	text = strings.ReplaceAll(text, "\r", "")
	text = reMultiNewlines.ReplaceAllString(text, "\n\n")
	text = strings.TrimSpace(text)
	return wrapLines(text, width)
}

func isAllowedContentType(header string, allowed []string) bool {
	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil {
		mediaType = strings.ToLower(strings.TrimSpace(strings.SplitN(header, ";", 2)[0]))
	}
	mediaType = strings.ToLower(mediaType)
	for _, a := range allowed {
		if strings.EqualFold(mediaType, strings.TrimSpace(a)) {
			return true
		}
	}
	return false
}

func wrapLines(input string, width int) string {
	if width <= 0 {
		width = defaultLineWidth
	}
	lines := strings.Split(input, "\n")
	out := make([]string, 0, len(lines)*2)
	for _, para := range lines {
		para = strings.TrimSpace(para)
		if para == "" {
			out = append(out, "")
			continue
		}

		words := strings.Fields(para)
		line := ""
		for _, word := range words {
			if len(line) == 0 {
				line = word
				continue
			}
			if len(line)+1+len(word) > width {
				out = append(out, line)
				line = word
				continue
			}
			line = line + " " + word
		}
		if len(line) > 0 {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\r\n")
}

func sanitizeFetchConfig(cfg FetchConfig) FetchConfig {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultFetchConfig.Timeout
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = DefaultFetchConfig.MaxBodyBytes
	}
	if cfg.MaxRedirects <= 0 {
		cfg.MaxRedirects = DefaultFetchConfig.MaxRedirects
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultFetchConfig.UserAgent
	}
	if len(cfg.AllowedTypes) == 0 {
		cfg.AllowedTypes = DefaultFetchConfig.AllowedTypes
	}
	return cfg
}

func validateGatewayURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("url must use http or https")
	}
	if parsed.Hostname() == "" {
		return errors.New("url must include a host")
	}
	if isBlockedHostname(parsed.Hostname()) {
		return errors.New("hostname blocked for SSRF safety")
	}

	ip := net.ParseIP(parsed.Hostname())
	if ip != nil {
		if isUnsafeIP(ip) {
			return errors.New("target ip blocked")
		}
		return nil
	}

	addrs, err := net.LookupIP(parsed.Hostname())
	if err != nil {
		return fmt.Errorf("dns resolution failed: %w", err)
	}
	for _, addr := range addrs {
		if isUnsafeIP(addr) {
			return errors.New("target ip blocked")
		}
	}
	return nil
}

func isBlockedHostname(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	switch host {
	case "localhost", "localhost.localdomain", "local", "ip6-localhost":
		return true
	}
	if strings.HasSuffix(host, ".local") {
		return true
	}
	return false
}

func isUnsafeIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		a, b := int(ip4[0]), int(ip4[1])
		switch {
		case a == 10:
		case a == 127:
		case a == 0:
		case a == 172 && b >= 16 && b <= 31:
		case a == 192 && b == 168:
		case a == 169 && b == 254:
		case a == 100 && b >= 64 && b <= 127:
		case a == 198 && (b == 18 || b == 19):
		case a == 240:
		default:
			return false
		}
		return true
	}
	return ip.IsPrivate()
}

// SaveOffline writes gateway output into a per-user folder for later reading.
func SaveOffline(baseDir, handle, sourceURL, text string) (string, error) {
	if baseDir == "" {
		baseDir = ".wolfbbs/offline"
	}
	if handle == "" {
		handle = "guest"
	}
	handle = sanitizePathComponent(handle)
	dir := filepath.Join(baseDir, handle)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	name := fmt.Sprintf("%s-%s.txt", time.Now().Format("20060102-150405"), sanitizePathComponent(sourceURL))
	if len(name) < 5 {
		name = "offline.txt"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func sanitizePathComponent(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "item"
	}
	return value
}
