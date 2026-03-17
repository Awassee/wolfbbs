package mods

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

type OneLiner struct {
	Handle string
	Text   string
	At     time.Time
}

type BBSListing struct {
	Name string
	Host string
	Port int
}

type OneLinerzMod struct {
	mu      sync.RWMutex
	maxRows int
	rows    []OneLiner
}

func NewOneLinerzMod(maxRows int) *OneLinerzMod {
	if maxRows <= 0 {
		maxRows = 40
	}
	return &OneLinerzMod{maxRows: maxRows, rows: []OneLiner{}}
}

func (m *OneLinerzMod) ID() string          { return "onelinerz" }
func (m *OneLinerzMod) Description() string { return "Classic one-line caller quotes." }
func (m *OneLinerzMod) Init(context.Context) error {
	return nil
}
func (m *OneLinerzMod) Tick(context.Context, time.Time) error {
	return nil
}
func (m *OneLinerzMod) Shutdown(context.Context) error {
	return nil
}
func (m *OneLinerzMod) Snapshot() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]string{
		"entries": strconv.Itoa(len(m.rows)),
	}
}
func (m *OneLinerzMod) Add(handle, text string) {
	handle = strings.TrimSpace(handle)
	text = strings.TrimSpace(text)
	if handle == "" || text == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows = append(m.rows, OneLiner{Handle: handle, Text: text, At: time.Now().UTC()})
	if len(m.rows) > m.maxRows {
		m.rows = m.rows[len(m.rows)-m.maxRows:]
	}
}
func (m *OneLinerzMod) List(limit int) []OneLiner {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > len(m.rows) {
		limit = len(m.rows)
	}
	start := len(m.rows) - limit
	out := make([]OneLiner, 0, limit)
	for i := len(m.rows) - 1; i >= start; i-- {
		out = append(out, m.rows[i])
	}
	return out
}

type RumorzMod struct {
	mu           sync.RWMutex
	rumors       []string
	currentIndex int
	lastRotate   time.Time
}

func NewRumorzMod(seed []string) *RumorzMod {
	filtered := make([]string, 0, len(seed))
	for _, row := range seed {
		row = strings.TrimSpace(row)
		if row != "" {
			filtered = append(filtered, row)
		}
	}
	if len(filtered) == 0 {
		filtered = []string{
			"Rumor has it the sysop patched the matrix again.",
			"Rumor has it #lobby coffee is now twice as strong.",
			"Rumor has it the door night bracket is rigged by luck.",
		}
	}
	return &RumorzMod{rumors: filtered}
}

func (m *RumorzMod) ID() string          { return "rumorz" }
func (m *RumorzMod) Description() string { return "Rotating rumor line for status/news blocks." }
func (m *RumorzMod) Init(ctx context.Context) error {
	return m.Tick(ctx, time.Now().UTC())
}
func (m *RumorzMod) Tick(context.Context, time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentIndex = (m.currentIndex + 1) % len(m.rumors)
	m.lastRotate = time.Now().UTC()
	return nil
}
func (m *RumorzMod) Shutdown(context.Context) error {
	return nil
}
func (m *RumorzMod) Snapshot() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]string{
		"active": m.Current(),
	}
}
func (m *RumorzMod) Current() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.rumors) == 0 {
		return ""
	}
	return m.rumors[m.currentIndex%len(m.rumors)]
}

type BBSListMod struct {
	mu   sync.RWMutex
	rows []BBSListing
	max  int
}

func NewBBSListMod(max int) *BBSListMod {
	if max <= 0 {
		max = 100
	}
	return &BBSListMod{max: max, rows: []BBSListing{}}
}

func (m *BBSListMod) ID() string          { return "bbslist" }
func (m *BBSListMod) Description() string { return "Curated neighboring BBS list." }
func (m *BBSListMod) Init(context.Context) error {
	return nil
}
func (m *BBSListMod) Tick(context.Context, time.Time) error {
	return nil
}
func (m *BBSListMod) Shutdown(context.Context) error {
	return nil
}
func (m *BBSListMod) Snapshot() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]string{"entries": strconv.Itoa(len(m.rows))}
}
func (m *BBSListMod) Add(name, host string, port int) {
	name = strings.TrimSpace(name)
	host = strings.TrimSpace(host)
	if name == "" || host == "" || port <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows = append(m.rows, BBSListing{Name: name, Host: host, Port: port})
	if len(m.rows) > m.max {
		m.rows = m.rows[len(m.rows)-m.max:]
	}
}
func (m *BBSListMod) List(limit int) []BBSListing {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > len(m.rows) {
		limit = len(m.rows)
	}
	out := make([]BBSListing, 0, limit)
	for i := len(m.rows) - 1; i >= len(m.rows)-limit; i-- {
		out = append(out, m.rows[i])
	}
	return out
}

type WhoOnlineMod struct {
	provider func() int
}

func NewWhoOnlineMod(provider func() int) *WhoOnlineMod {
	if provider == nil {
		provider = func() int { return 0 }
	}
	return &WhoOnlineMod{provider: provider}
}

func (m *WhoOnlineMod) ID() string { return "whos_online" }
func (m *WhoOnlineMod) Description() string {
	return "Tracks current online count from runtime providers."
}
func (m *WhoOnlineMod) Init(context.Context) error {
	return nil
}
func (m *WhoOnlineMod) Tick(context.Context, time.Time) error {
	return nil
}
func (m *WhoOnlineMod) Shutdown(context.Context) error {
	return nil
}
func (m *WhoOnlineMod) Snapshot() map[string]string {
	return map[string]string{
		"online": strconv.Itoa(m.provider()),
	}
}
