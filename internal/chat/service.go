package chat

import (
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wolfbbs/internal/repository"
)

const (
	defaultChatRateLimitWindow = time.Second
	defaultChatRateLimitBurst  = 8
	defaultChatHistoryLimit    = 200
	defaultChatRetentionHours  = 24
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

type ModerationAction struct {
	Type      string
	Channel   string
	Actor     string
	Target    string
	Reason    string
	CreatedAt time.Time
}

type moderationState struct {
	ExpiresAt time.Time
	Reason    string
	Actor     string
}

type Service struct {
	mu           sync.Mutex
	channels     map[string]map[string]bool
	history      map[string][]Message
	presence     map[string]*Presence
	rateWindow   map[string][]time.Time
	bans         map[string]map[string]moderationState
	mutes        map[string]map[string]moderationState
	subscribers  map[string]map[int64]*chatSubscription
	nextSubID    int64
	nextID       int64
	historyLimit int
	rateWindowSz time.Duration
	rateBurst    int
	retention    time.Duration
	node         string
	db           *sql.DB
	audit        []ModerationAction
	pollInterval time.Duration
}

type chatSubscription struct {
	channel  string
	messages chan Message
	stop     chan struct{}
	lastID   int64
}

func NewService() *Service {
	dsn := strings.TrimSpace(os.Getenv("WOLFBBS_DATABASE_URL"))
	if dsn == "" {
		dsn = strings.TrimSpace(repository.ResolveDatabaseURL())
	}
	return newServiceWithStorage(dsn)
}

func NewServiceForTest() *Service {
	return newServiceWithStorage("")
}

func NewServiceWithStorage(dsn string) *Service {
	return newServiceWithStorage(strings.TrimSpace(dsn))
}

func newServiceWithStorage(dsn string) *Service {
	s := &Service{
		channels:     map[string]map[string]bool{},
		history:      map[string][]Message{},
		presence:     map[string]*Presence{},
		rateWindow:   map[string][]time.Time{},
		bans:         map[string]map[string]moderationState{},
		mutes:        map[string]map[string]moderationState{},
		subscribers:  map[string]map[int64]*chatSubscription{},
		historyLimit: defaultChatHistoryLimit,
		rateWindowSz: defaultChatRateLimitWindow,
		rateBurst:    defaultChatRateLimitBurst,
		retention:    defaultChatRetentionHours * time.Hour,
		node:         "bbs-node",
		pollInterval: 500 * time.Millisecond,
	}

	if dsn == "" {
		return s
	}

	if hn := strings.TrimSpace(os.Getenv("HOSTNAME")); hn != "" {
		s.node = hn
	}
	if v := strings.TrimSpace(os.Getenv("WOLFBBS_CHAT_HISTORY_LIMIT")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			s.historyLimit = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("WOLFBBS_CHAT_RATE_BURST")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			s.rateBurst = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("WOLFBBS_CHAT_RATE_WINDOW_MS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			s.rateWindowSz = time.Duration(parsed) * time.Millisecond
		}
	}
	if v := strings.TrimSpace(os.Getenv("WOLFBBS_CHAT_RETENTION_HOURS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			s.retention = time.Duration(parsed) * time.Hour
		}
	}
	if v := strings.TrimSpace(os.Getenv("WOLFBBS_CHAT_POLL_INTERVAL_MS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			s.pollInterval = time.Duration(parsed) * time.Millisecond
		}
	}

	db, err := openChatDB(dsn)
	if err != nil {
		return s
	}
	s.db = db
	_ = s.ensureSchema()
	_ = s.cleanupOldMessages()
	return s
}

func normalizeChannel(channel string) string {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return "#lobby"
	}
	if !strings.HasPrefix(channel, "#") {
		return "#" + channel
	}
	return channel
}

func NormalizeChannel(channel string) string {
	return normalizeChannel(channel)
}

func normalizeNick(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func openChatDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)

	retries := 10
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_DB_CONNECT_RETRIES")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			retries = v
		}
	}
	delay := 500 * time.Millisecond
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_DB_CONNECT_DELAY_MS")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
			delay = time.Duration(v) * time.Millisecond
		}
	}

	var pingErr error
	for i := 0; i < retries; i++ {
		pingErr = db.Ping()
		if pingErr == nil {
			return db, nil
		}
		if i+1 < retries {
			time.Sleep(delay)
		}
	}
	_ = db.Close()
	return nil, pingErr
}

func (s *Service) ensureSchema() error {
	if s.db == nil {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS chat_channels (
  name TEXT PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`,
		`CREATE TABLE IF NOT EXISTS chat_messages (
  id BIGSERIAL PRIMARY KEY,
  channel TEXT NOT NULL,
  from_user TEXT NOT NULL,
  to_user TEXT,
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_channel_id ON chat_messages(channel, id DESC)`,
		`CREATE TABLE IF NOT EXISTS chat_presence (
  nick TEXT PRIMARY KEY,
  node TEXT NOT NULL,
  area TEXT NOT NULL,
  online BOOLEAN NOT NULL DEFAULT TRUE,
  login_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen TIMESTAMPTZ NOT NULL DEFAULT now()
)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	_, _ = s.db.Exec("INSERT INTO chat_channels(name) VALUES('#lobby') ON CONFLICT(name) DO NOTHING")
	return nil
}

func (s *Service) cleanupOldMessages() error {
	if s.db == nil || s.retention <= 0 {
		return nil
	}
	cutoff := time.Now().Add(-s.retention)
	_, err := s.db.Exec(`DELETE FROM chat_messages WHERE created_at < $1`, cutoff)
	return err
}

func (s *Service) cleanExpiredModeration() {
	now := time.Now()
	for c, users := range s.bans {
		for user, state := range users {
			if !state.ExpiresAt.IsZero() && now.After(state.ExpiresAt) {
				delete(users, user)
			}
		}
		if len(users) == 0 {
			delete(s.bans, c)
		}
	}
	for c, users := range s.mutes {
		for user, state := range users {
			if !state.ExpiresAt.IsZero() && now.After(state.ExpiresAt) {
				delete(users, user)
			}
		}
		if len(users) == 0 {
			delete(s.mutes, c)
		}
	}
}

func (s *Service) ensureChannel(channel string) {
	channel = normalizeChannel(channel)
	if _, ok := s.channels[channel]; !ok {
		s.channels[channel] = map[string]bool{}
	}
	if _, ok := s.subscribers[channel]; !ok {
		s.subscribers[channel] = map[int64]*chatSubscription{}
	}
	if s.db != nil {
		_, _ = s.db.Exec("INSERT INTO chat_channels(name) VALUES($1) ON CONFLICT(name) DO NOTHING", channel)
	}
}

func (s *Service) latestMessageID(channel string) int64 {
	channel = normalizeChannel(channel)
	if s.db == nil {
		msgs := s.history[channel]
		if len(msgs) == 0 {
			return 0
		}
		return msgs[len(msgs)-1].ID
	}
	var out int64
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM chat_messages WHERE channel = $1`, channel).Scan(&out); err != nil {
		return 0
	}
	return out
}

func (s *Service) setPresenceLocked(nick, channel string) {
	now := time.Now()
	channel = normalizeChannel(channel)
	if nick == "" {
		return
	}
	if s.db != nil {
		_, _ = s.db.Exec(`INSERT INTO chat_presence(nick,node,area,online,login_at,last_seen)
		  VALUES($1,$2,$3,TRUE,NOW(),NOW())
		  ON CONFLICT (nick) DO UPDATE
		  SET node = EXCLUDED.node, area = EXCLUDED.area, online = TRUE, last_seen = NOW()`, nick, s.node, channel)
	}
	p := s.presence[nick]
	if p == nil {
		p = &Presence{Nick: nick, Node: s.node, LoginAt: now, LastSeen: now, Online: true}
		s.presence[nick] = p
	}
	p.Area = channel
	p.Node = s.node
	p.Online = true
	p.LastSeen = now
	if p.LoginAt.IsZero() {
		p.LoginAt = now
	}
}

func (s *Service) JoinChannel(nick, channel string) {
	nick = normalizeNick(nick)
	channel = normalizeChannel(channel)
	if nick == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureChannel(channel)
	s.channels[channel][nick] = true
	s.setPresenceLocked(nick, channel)
}

func (s *Service) LeaveChannel(nick, channel string) {
	nick = normalizeNick(nick)
	channel = normalizeChannel(channel)
	s.mu.Lock()
	defer s.mu.Unlock()
	if members, ok := s.channels[channel]; ok {
		delete(members, nick)
	}
	if p, ok := s.presence[nick]; ok && p.Area == channel {
		p.Area = ""
	}
	if s.db != nil {
		_, _ = s.db.Exec(`UPDATE chat_presence SET online = FALSE, area = '' WHERE nick = $1`, nick)
	}
}

func (s *Service) Subscribe(channel, nick string) (chan Message, func()) {
	channel = normalizeChannel(channel)
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = nick

	s.ensureChannel(channel)
	s.nextSubID++
	id := s.nextSubID
	ch := make(chan Message, 32)
	sub := &chatSubscription{
		channel:  channel,
		messages: ch,
		stop:     make(chan struct{}),
		lastID:   s.latestMessageID(channel),
	}
	s.subscribers[channel][id] = sub
	if s.db != nil {
		go s.watchSubscription(sub)
	}

	closeFn := func() {
		s.unsubscribe(channel, id)
	}
	return ch, closeFn
}

func (s *Service) unsubscribe(channel string, id int64) {
	channel = normalizeChannel(channel)
	s.mu.Lock()
	defer s.mu.Unlock()
	if channelSubs, ok := s.subscribers[channel]; ok {
		if sub, found := channelSubs[id]; found {
			delete(channelSubs, id)
			select {
			case <-sub.stop:
			default:
				close(sub.stop)
			}
			close(sub.messages)
		}
	}
}

func (s *Service) watchSubscription(sub *chatSubscription) {
	if sub == nil || sub.messages == nil || sub.stop == nil {
		return
	}
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sub.stop:
			return
		case <-ticker.C:
			msgs := s.HistorySince(sub.channel, sub.lastID, 50)
			for _, msg := range msgs {
				if msg.ID <= sub.lastID {
					continue
				}
				sub.lastID = msg.ID
				func() {
					defer func() {
						_ = recover()
					}()
					select {
					case sub.messages <- msg:
					default:
					}
				}()
			}
		}
	}
}

func (s *Service) rateLimited(nick string) bool {
	window := s.rateWindow[nick]
	now := time.Now()
	cutoff := now.Add(-s.rateWindowSz)
	kept := window[:0]
	for _, t := range window {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= s.rateBurst {
		s.rateWindow[nick] = kept
		return true
	}
	kept = append(kept, now)
	s.rateWindow[nick] = kept
	return false
}

func parseModerationDuration(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 {
		return time.Time{}
	}
	return time.Now().Add(duration)
}

func (s *Service) Post(nick, channel, body string) (Message, error) {
	nick = normalizeNick(nick)
	channel = normalizeChannel(channel)
	body = strings.TrimSpace(body)
	if nick == "" {
		return Message{}, fmt.Errorf("empty nick")
	}
	if body == "" {
		return Message{}, fmt.Errorf("empty message")
	}

	s.mu.Lock()
	s.cleanExpiredModeration()
	if bans, ok := s.bans[channel]; ok {
		if state, exists := bans[nick]; exists {
			if state.ExpiresAt.IsZero() || state.ExpiresAt.After(time.Now()) {
				s.mu.Unlock()
				return Message{}, fmt.Errorf("banned")
			}
		}
	}
	if mutes, ok := s.mutes[channel]; ok {
		if state, exists := mutes[nick]; exists {
			if state.ExpiresAt.IsZero() || state.ExpiresAt.After(time.Now()) {
				s.mu.Unlock()
				return Message{}, fmt.Errorf("muted")
			}
		}
	}
	if s.rateLimited(nick) {
		s.mu.Unlock()
		return Message{}, fmt.Errorf("rate limit")
	}

	msg := Message{
		Channel:   channel,
		From:      nick,
		Body:      body,
		CreatedAt: time.Now(),
	}

	if s.db == nil {
		s.nextID++
		msg.ID = s.nextID
		s.history[channel] = append(s.history[channel], msg)
		if len(s.history[channel]) > s.historyLimit {
			remove := len(s.history[channel]) - s.historyLimit
			s.history[channel] = append([]Message{}, s.history[channel][remove:]...)
		}
	} else {
		if err := s.db.QueryRow(`INSERT INTO chat_messages(channel, from_user, body) VALUES($1,$2,$3)
		  RETURNING id, created_at`, channel, nick, body).Scan(&msg.ID, &msg.CreatedAt); err != nil {
			s.mu.Unlock()
			return Message{}, err
		}
		_ = s.cleanupOldMessages()
	}

	s.ensureChannel(channel)
	s.channels[channel][nick] = true
	s.setPresenceLocked(nick, channel)

	subs := make([]chan Message, 0, len(s.subscribers[channel]))
	for _, subscriber := range s.subscribers[channel] {
		subs = append(subs, subscriber.messages)
	}
	s.mu.Unlock()

	if s.db != nil {
		return msg, nil
	}

	for _, subscriber := range subs {
		func(ch chan Message) {
			defer func() {
				_ = recover()
			}()
			select {
			case ch <- msg:
			default:
			}
		}(subscriber)
	}
	return msg, nil
}

func (s *Service) History(channel string, limit int) []Message {
	channel = normalizeChannel(channel)
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		h := s.history[channel]
		if limit <= 0 || len(h) <= limit {
			return append([]Message{}, h...)
		}
		return append([]Message{}, h[len(h)-limit:]...)
	}
	if limit <= 0 {
		limit = s.historyLimit
	}
	rows, err := s.db.Query(`SELECT id, channel, from_user, COALESCE(to_user, ''), body, created_at
	  FROM chat_messages WHERE channel=$1 ORDER BY id DESC LIMIT $2`, channel, limit)
	if err != nil {
		return []Message{}
	}
	defer rows.Close()

	outRev := make([]Message, 0, limit)
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.Channel, &msg.From, &msg.To, &msg.Body, &msg.CreatedAt); err != nil {
			return []Message{}
		}
		outRev = append(outRev, msg)
	}
	// Reverse to return ascending order.
	out := make([]Message, len(outRev))
	for i := range outRev {
		out[len(outRev)-1-i] = outRev[i]
	}
	return out
}

func (s *Service) HistorySince(channel string, afterID int64, limit int) []Message {
	channel = normalizeChannel(channel)
	if limit <= 0 {
		limit = s.historyLimit
	}
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		h := s.history[channel]
		if afterID <= 0 {
			if len(h) <= limit {
				return append([]Message{}, h...)
			}
			return append([]Message{}, h[len(h)-limit:]...)
		}
		start := 0
		for start < len(h) && h[start].ID <= afterID {
			start++
		}
		out := append([]Message{}, h[start:]...)
		if len(out) <= limit {
			return out
		}
		return out[len(out)-limit:]
	}

	rows, err := s.db.Query(`SELECT id, channel, from_user, COALESCE(to_user, ''), body, created_at
	  FROM chat_messages WHERE channel=$1 AND id>$2 ORDER BY id ASC LIMIT $3`, channel, afterID, limit)
	if err != nil {
		return []Message{}
	}
	defer rows.Close()
	out := make([]Message, 0, limit)
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.Channel, &msg.From, &msg.To, &msg.Body, &msg.CreatedAt); err != nil {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func (s *Service) Online() []Presence {
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := make([]Presence, 0, len(s.presence))
		for _, p := range s.presence {
			if p.Online {
				cp := *p
				cp.IdleSec = int(time.Since(cp.LastSeen).Seconds())
				out = append(out, cp)
			}
		}
		sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Nick) < strings.ToLower(out[j].Nick) })
		return out
	}

	rows, err := s.db.Query(`SELECT nick, node, area, login_at, last_seen, online FROM chat_presence WHERE online = TRUE`)
	if err != nil {
		return []Presence{}
	}
	defer rows.Close()
	out := make([]Presence, 0)
	for rows.Next() {
		var p Presence
		if err := rows.Scan(&p.Nick, &p.Node, &p.Area, &p.LoginAt, &p.LastSeen, &p.Online); err != nil {
			continue
		}
		p.IdleSec = int(time.Since(p.LastSeen).Seconds())
		out = append(out, p)
	}
	return out
}

func (s *Service) OnlineInChannel(channel string) []Presence {
	channel = normalizeChannel(channel)
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := make([]Presence, 0)
		for _, p := range s.presence {
			if p.Online && p.Area == channel {
				cp := *p
				cp.IdleSec = int(time.Since(cp.LastSeen).Seconds())
				out = append(out, cp)
			}
		}
		sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Nick) < strings.ToLower(out[j].Nick) })
		return out
	}
	rows, err := s.db.Query(`SELECT nick, node, area, login_at, last_seen, online FROM chat_presence WHERE online = TRUE AND area = $1`, channel)
	if err != nil {
		return []Presence{}
	}
	defer rows.Close()
	out := make([]Presence, 0)
	for rows.Next() {
		var p Presence
		if err := rows.Scan(&p.Nick, &p.Node, &p.Area, &p.LoginAt, &p.LastSeen, &p.Online); err != nil {
			continue
		}
		p.IdleSec = int(time.Since(p.LastSeen).Seconds())
		out = append(out, p)
	}
	return out
}

func (s *Service) IsMember(nick, channel string) bool {
	nick = normalizeNick(nick)
	channel = normalizeChannel(channel)
	s.mu.Lock()
	defer s.mu.Unlock()
	members, ok := s.channels[channel]
	if !ok {
		return false
	}
	return members[nick]
}

func (s *Service) ListChannels() []string {
	channels := []string{"#lobby"}
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		for name := range s.channels {
			found := false
			for _, existing := range channels {
				if existing == name {
					found = true
					break
				}
			}
			if !found {
				channels = append(channels, name)
			}
		}
		sort.Slice(channels, func(i, j int) bool { return strings.ToLower(channels[i]) < strings.ToLower(channels[j]) })
		return channels
	}

	rows, err := s.db.Query(`SELECT name FROM chat_channels ORDER BY name`)
	if err != nil {
		return channels
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		channels = append(channels, name)
	}
	sort.Slice(channels, func(i, j int) bool { return strings.ToLower(channels[i]) < strings.ToLower(channels[j]) })
	return channels
}

func (s *Service) IsBanned(channel, nick string) bool {
	nick = normalizeNick(nick)
	channel = normalizeChannel(channel)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanExpiredModeration()
	if bans, ok := s.bans[channel]; ok {
		state, ok := bans[nick]
		return ok && (state.ExpiresAt.IsZero() || state.ExpiresAt.After(time.Now()))
	}
	return false
}

func (s *Service) IsMuted(channel, nick string) bool {
	nick = normalizeNick(nick)
	channel = normalizeChannel(channel)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanExpiredModeration()
	if mutes, ok := s.mutes[channel]; ok {
		state, ok := mutes[nick]
		return ok && (state.ExpiresAt.IsZero() || state.ExpiresAt.After(time.Now()))
	}
	return false
}

func (s *Service) Ban(channel, nick, actor, reason, duration string) {
	channel = normalizeChannel(channel)
	nick = normalizeNick(nick)
	state := moderationState{
		Reason:    strings.TrimSpace(reason),
		Actor:     actor,
		ExpiresAt: parseModerationDuration(duration),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bans[channel] == nil {
		s.bans[channel] = map[string]moderationState{}
	}
	s.bans[channel][nick] = state
	s.audit = append(s.audit, ModerationAction{
		Type:      "ban",
		Channel:   channel,
		Actor:     actor,
		Target:    nick,
		Reason:    reason,
		CreatedAt: time.Now(),
	})
	if members, ok := s.channels[channel]; ok {
		delete(members, nick)
	}
	if p, ok := s.presence[nick]; ok && p.Area == channel {
		p.Area = ""
	}
}

func (s *Service) Unban(channel, nick string) {
	channel = normalizeChannel(channel)
	nick = normalizeNick(nick)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.bans[channel], nick)
}

func (s *Service) Mute(channel, nick, actor, reason, duration string) {
	channel = normalizeChannel(channel)
	nick = normalizeNick(nick)
	state := moderationState{
		Reason:    strings.TrimSpace(reason),
		Actor:     actor,
		ExpiresAt: parseModerationDuration(duration),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mutes[channel] == nil {
		s.mutes[channel] = map[string]moderationState{}
	}
	s.mutes[channel][nick] = state
	s.audit = append(s.audit, ModerationAction{
		Type:      "mute",
		Channel:   channel,
		Actor:     actor,
		Target:    nick,
		Reason:    reason,
		CreatedAt: time.Now(),
	})
}

func (s *Service) Unmute(channel, nick string) {
	channel = normalizeChannel(channel)
	nick = normalizeNick(nick)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.mutes[channel], nick)
}

func (s *Service) Kick(channel, actor, target, reason string) {
	channel = normalizeChannel(channel)
	target = normalizeNick(target)
	s.mu.Lock()
	defer s.mu.Unlock()
	if members, ok := s.channels[channel]; ok {
		delete(members, target)
	}
	if p, ok := s.presence[target]; ok && p.Area == channel {
		p.Area = ""
	}
	s.audit = append(s.audit, ModerationAction{
		Type:      "kick",
		Channel:   channel,
		Actor:     actor,
		Target:    target,
		Reason:    reason,
		CreatedAt: time.Now(),
	})
}

func (s *Service) ModerationLog(limit int) []ModerationAction {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit >= len(s.audit) {
		out := make([]ModerationAction, len(s.audit))
		copy(out, s.audit)
		return out
	}
	out := make([]ModerationAction, limit)
	copy(out, s.audit[len(s.audit)-limit:])
	return out
}
