package chat

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type Message struct {
	ID        int64
	Channel   string
	From      string
	To        string
	Body      string
	CreatedAt time.Time
}

type Presence struct {
	Nick     string
	Node     string
	Online   bool
	IdleSec  int
	Area     string
	LoginAt  time.Time
	LastSeen time.Time
}

type Service struct {
	mu         sync.Mutex
	channels   map[string]map[string]bool
	history    map[string][]Message
	presence   map[string]*Presence
	nextID     int64
	rateWindow map[string][]time.Time
}

func NewService() *Service {
	return &Service{
		channels:   map[string]map[string]bool{},
		history:    map[string][]Message{},
		presence:   map[string]*Presence{},
		rateWindow: map[string][]time.Time{},
	}
}

func (s *Service) JoinChannel(nick, channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.channels[channel] == nil {
		s.channels[channel] = map[string]bool{}
	}
	s.channels[channel][strings.ToLower(nick)] = true
	s.presence[nick] = &Presence{Nick: nick, Online: true, Area: channel, LoginAt: time.Now(), LastSeen: time.Now()}
}

func (s *Service) LeaveChannel(nick, channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if members, ok := s.channels[channel]; ok {
		delete(members, strings.ToLower(nick))
	}
	if p, ok := s.presence[nick]; ok && p.Area == channel {
		p.Area = ""
	}
}

func (s *Service) Post(nick, channel, body string) (Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	recent := s.rateWindow[nick]
	now := time.Now()
	cutoff := now.Add(-time.Second)
	kept := recent[:0]
	for _, t := range recent {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	recent = kept
	if len(recent) > 8 {
		return Message{}, fmt.Errorf("flood protection")
	}
	recent = append(recent, now)
	s.rateWindow[nick] = recent

	s.nextID++
	msg := Message{ID: s.nextID, Channel: channel, From: nick, Body: body, CreatedAt: now}
	s.history[channel] = append(s.history[channel], msg)
	if p, ok := s.presence[nick]; ok {
		p.LastSeen = now
	}
	return msg, nil
}

func (s *Service) History(channel string, limit int) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.history[channel]
	if limit <= 0 || len(h) <= limit {
		return append([]Message{}, h...)
	}
	return append([]Message{}, h[len(h)-limit:]...)
}

func (s *Service) Online() []Presence {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Presence, 0, len(s.presence))
	for _, p := range s.presence {
		if p.Online {
			idle := int(time.Since(p.LastSeen).Seconds())
			p.IdleSec = idle
			out = append(out, *p)
		}
	}
	return out
}

func (s *Service) IsMember(nick, channel string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.channels[channel]
	if m == nil {
		return false
	}
	return m[strings.ToLower(nick)]
}

func (s *Service) ListChannels() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.channels))
	for c := range s.channels {
		out = append(out, c)
	}
	return out
}
