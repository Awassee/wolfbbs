package mods

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type Snapshot struct {
	ID          string            `json:"id"`
	Description string            `json:"description"`
	Enabled     bool              `json:"enabled"`
	Running     bool              `json:"running"`
	LastError   string            `json:"last_error,omitempty"`
	LastTick    *time.Time        `json:"last_tick,omitempty"`
	Data        map[string]string `json:"data,omitempty"`
}

type Module interface {
	ID() string
	Description() string
	Init(context.Context) error
	Tick(context.Context, time.Time) error
	Shutdown(context.Context) error
	Snapshot() map[string]string
}

type moduleState struct {
	mod       Module
	enabled   bool
	running   bool
	lastErr   string
	lastTick  *time.Time
	createdAt time.Time
}

type Manager struct {
	mu       sync.RWMutex
	modules  map[string]*moduleState
	interval time.Duration
	cancel   context.CancelFunc
}

func NewManager(interval time.Duration) *Manager {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Manager{
		modules:  map[string]*moduleState{},
		interval: interval,
	}
}

func normalizeID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func (m *Manager) Register(mod Module, enabled bool) error {
	if m == nil || mod == nil {
		return errors.New("module is required")
	}
	id := normalizeID(mod.ID())
	if id == "" {
		return errors.New("module id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.modules[id]; exists {
		return errors.New("module already registered")
	}
	m.modules[id] = &moduleState{
		mod:       mod,
		enabled:   enabled,
		createdAt: time.Now().UTC(),
	}
	return nil
}

func (m *Manager) Start(ctx context.Context) error {
	if m == nil {
		return errors.New("manager is required")
	}
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()

	m.forEach(func(state *moduleState) {
		if !state.enabled {
			return
		}
		if err := state.mod.Init(runCtx); err != nil {
			state.lastErr = err.Error()
			return
		}
		state.running = true
	})

	go m.loop(runCtx)
	return nil
}

func (m *Manager) loop(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.forEach(func(state *moduleState) {
				if !state.enabled || !state.running {
					return
				}
				if err := state.mod.Tick(ctx, now.UTC()); err != nil {
					state.lastErr = err.Error()
					return
				}
				t := now.UTC()
				state.lastTick = &t
				state.lastErr = ""
			})
		}
	}
}

func (m *Manager) Stop(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.forEach(func(state *moduleState) {
		if !state.running {
			return
		}
		if err := state.mod.Shutdown(ctx); err != nil {
			state.lastErr = err.Error()
		}
		state.running = false
	})
	return nil
}

func (m *Manager) SetEnabled(id string, enabled bool) {
	id = normalizeID(id)
	m.mu.Lock()
	defer m.mu.Unlock()
	if row, ok := m.modules[id]; ok {
		row.enabled = enabled
	}
}

func (m *Manager) Snapshot() []Snapshot {
	m.mu.RLock()
	ids := make([]string, 0, len(m.modules))
	for id := range m.modules {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Snapshot, 0, len(ids))
	for _, id := range ids {
		row := m.modules[id]
		s := Snapshot{
			ID:          id,
			Description: row.mod.Description(),
			Enabled:     row.enabled,
			Running:     row.running,
			LastError:   row.lastErr,
			Data:        row.mod.Snapshot(),
		}
		if row.lastTick != nil {
			t := *row.lastTick
			s.LastTick = &t
		}
		out = append(out, s)
	}
	m.mu.RUnlock()
	return out
}

func (m *Manager) forEach(fn func(*moduleState)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.modules {
		fn(row)
	}
}
