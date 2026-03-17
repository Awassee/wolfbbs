package session

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNoFreeNodes = errors.New("no free nodes")

type NodeState struct {
	SessionID    string
	NodeID       int
	Username     string
	Area         string
	RemoteAddr   string
	LoginAt      time.Time
	LastActivity time.Time
	IdleSeconds  int64
}

type CallerState struct {
	NodeID     int
	Username   string
	Area       string
	RemoteAddr string
	LoginAt    time.Time
	LogoutAt   time.Time
	Duration   time.Duration
}

type Manager struct {
	mu          sync.Mutex
	maxNodes    int
	maxCallers  int
	nodeToSID   map[int]string
	sessions    map[string]*NodeState
	lastCallers []CallerState
}

func NewManager(maxNodes, maxCallers int) *Manager {
	if maxNodes <= 0 {
		maxNodes = 255
	}
	if maxCallers <= 0 {
		maxCallers = 128
	}
	return &Manager{
		maxNodes:   maxNodes,
		maxCallers: maxCallers,
		nodeToSID:  map[int]string{},
		sessions:   map[string]*NodeState{},
	}
}

func (m *Manager) Start(sessionID, username, remoteAddr string) (NodeState, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return NodeState{}, ErrNoFreeNodes
	}
	username = fallbackUsername(username)

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing := m.sessions[sessionID]; existing != nil {
		copy := *existing
		copy.IdleSeconds = int64(time.Since(existing.LastActivity).Seconds())
		return copy, nil
	}

	nodeID := m.allocateNodeLocked()
	if nodeID <= 0 {
		return NodeState{}, ErrNoFreeNodes
	}
	now := time.Now().UTC()
	state := &NodeState{
		SessionID:    sessionID,
		NodeID:       nodeID,
		Username:     username,
		Area:         "Welcome",
		RemoteAddr:   strings.TrimSpace(remoteAddr),
		LoginAt:      now,
		LastActivity: now,
	}
	m.nodeToSID[nodeID] = sessionID
	m.sessions[sessionID] = state

	copy := *state
	copy.IdleSeconds = 0
	return copy, nil
}

func (m *Manager) SetUser(sessionID, username string) {
	username = fallbackUsername(username)
	m.mu.Lock()
	defer m.mu.Unlock()
	if state := m.sessions[strings.TrimSpace(sessionID)]; state != nil {
		state.Username = username
		state.LastActivity = time.Now().UTC()
	}
}

func (m *Manager) SetArea(sessionID, area string) {
	area = strings.TrimSpace(area)
	if area == "" {
		area = "Main"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if state := m.sessions[strings.TrimSpace(sessionID)]; state != nil {
		state.Area = area
		state.LastActivity = time.Now().UTC()
	}
}

func (m *Manager) Touch(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state := m.sessions[strings.TrimSpace(sessionID)]; state != nil {
		state.LastActivity = time.Now().UTC()
	}
}

func (m *Manager) Get(sessionID string) (NodeState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.sessions[strings.TrimSpace(sessionID)]
	if state == nil {
		return NodeState{}, false
	}
	copy := *state
	copy.IdleSeconds = int64(time.Since(state.LastActivity).Seconds())
	if copy.IdleSeconds < 0 {
		copy.IdleSeconds = 0
	}
	return copy, true
}

func (m *Manager) End(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.sessions[strings.TrimSpace(sessionID)]
	if state == nil {
		return
	}
	now := time.Now().UTC()
	duration := now.Sub(state.LoginAt)
	if duration < 0 {
		duration = 0
	}
	m.lastCallers = append([]CallerState{{
		NodeID:     state.NodeID,
		Username:   state.Username,
		Area:       state.Area,
		RemoteAddr: state.RemoteAddr,
		LoginAt:    state.LoginAt,
		LogoutAt:   now,
		Duration:   duration,
	}}, m.lastCallers...)
	if len(m.lastCallers) > m.maxCallers {
		m.lastCallers = m.lastCallers[:m.maxCallers]
	}
	delete(m.sessions, sessionID)
	delete(m.nodeToSID, state.NodeID)
}

func (m *Manager) Online() []NodeState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]NodeState, 0, len(m.sessions))
	now := time.Now().UTC()
	for _, state := range m.sessions {
		copy := *state
		copy.IdleSeconds = int64(now.Sub(state.LastActivity).Seconds())
		if copy.IdleSeconds < 0 {
			copy.IdleSeconds = 0
		}
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].NodeID < out[j].NodeID
	})
	return out
}

func (m *Manager) LastCallers(limit int) []CallerState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 || limit > len(m.lastCallers) {
		limit = len(m.lastCallers)
	}
	out := make([]CallerState, limit)
	copy(out, m.lastCallers[:limit])
	return out
}

func (m *Manager) allocateNodeLocked() int {
	for node := 1; node <= m.maxNodes; node++ {
		if m.nodeToSID[node] == "" {
			return node
		}
	}
	return 0
}

func fallbackUsername(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return "Guest"
	}
	return username
}
