package sshserver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/rbac"
	"wolfbbs/internal/ui"
)

const (
	profileSettingBookmarksRoot          = "web.bookmarks."
	profileSettingCallerCirclesRoot      = "profile.circles."
	profileSettingPublicProfileRoot      = "profile.public."
	profileSettingContactAliasesRoot     = "profile.aliases."
	profileSettingHomeRouteRoot          = "web.home_route."
	profileSettingBoardSubscriptionsRoot = "web.board_subscriptions."
	profileSettingBoardQuietHoursRoot    = "web.board_quiet."
	profileSettingAttentionDismissedRoot = "web.attention.dismissed."
	profileSettingAttentionReadRoot      = "web.attention.read."
	profileSettingAttentionSnoozeRoot    = "web.attention.snooze."
	profileSettingRouteSeenRoot          = "web.route_seen."
	profileSettingBulletinAckRoot        = "community.bulletins.ack."
	profileMaxBookmarkItems              = 200
	profileMaxCircles                    = 24
	profileMaxCircleMembers              = 32
)

type profileBookmarkEntry struct {
	Key       string    `json:"key"`
	Kind      string    `json:"kind"`
	Label     string    `json:"label"`
	Href      string    `json:"href"`
	Meta      string    `json:"meta,omitempty"`
	BoardID   int64     `json:"board_id,omitempty"`
	MessageID int64     `json:"message_id,omitempty"`
	MailID    int64     `json:"mail_id,omitempty"`
	AddedAt   time.Time `json:"added_at"`
}

type profileCallerCircle struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Members []string `json:"members,omitempty"`
	Note    string   `json:"note,omitempty"`
}

func profileNormalizeHandleKey(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func profileSettingKey(root, handle string) string {
	key := profileNormalizeHandleKey(handle)
	if key == "" {
		return ""
	}
	return root + key
}

func normalizeProfileBookmarkEntry(entry profileBookmarkEntry) (profileBookmarkEntry, bool) {
	entry.Kind = strings.ToLower(strings.TrimSpace(entry.Kind))
	entry.Key = strings.TrimSpace(entry.Key)
	entry.Label = strings.TrimSpace(entry.Label)
	entry.Href = strings.TrimSpace(entry.Href)
	entry.Meta = strings.TrimSpace(entry.Meta)
	switch entry.Kind {
	case "board_message":
		if entry.MessageID <= 0 {
			return profileBookmarkEntry{}, false
		}
		entry.Key = "board:" + strconv.FormatInt(entry.MessageID, 10)
		if entry.Href == "" {
			if entry.BoardID > 0 {
				entry.Href = fmt.Sprintf("/boards?board=%d&id=%d", entry.BoardID, entry.MessageID)
			} else {
				entry.Href = "/boards?id=" + strconv.FormatInt(entry.MessageID, 10)
			}
		}
	case "mail":
		if entry.MailID <= 0 {
			return profileBookmarkEntry{}, false
		}
		entry.Key = "mail:" + strconv.FormatInt(entry.MailID, 10)
		if entry.Href == "" {
			entry.Href = "/mail?id=" + strconv.FormatInt(entry.MailID, 10)
		}
	default:
		return profileBookmarkEntry{}, false
	}
	if entry.Label == "" {
		entry.Label = entry.Key
	}
	if entry.AddedAt.IsZero() {
		entry.AddedAt = time.Now().UTC()
	}
	return entry, true
}

func (s *Server) profileBookmarks(handle string) []profileBookmarkEntry {
	if s == nil || s.admin == nil {
		return nil
	}
	key := profileSettingKey(profileSettingBookmarksRoot, handle)
	if key == "" {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var decoded []profileBookmarkEntry
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	out := make([]profileBookmarkEntry, 0, len(decoded))
	for _, row := range decoded {
		if normalized, ok := normalizeProfileBookmarkEntry(row); ok {
			out = append(out, normalized)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AddedAt.Equal(out[j].AddedAt) {
			return out[i].Key < out[j].Key
		}
		return out[i].AddedAt.After(out[j].AddedAt)
	})
	if len(out) > profileMaxBookmarkItems {
		out = out[:profileMaxBookmarkItems]
	}
	return out
}

func (s *Server) persistProfileBookmarks(handle string, rows []profileBookmarkEntry) error {
	if s == nil || s.admin == nil {
		return fmt.Errorf("bookmark storage unavailable")
	}
	key := profileSettingKey(profileSettingBookmarksRoot, handle)
	if key == "" {
		return fmt.Errorf("missing caller handle")
	}
	out := make([]profileBookmarkEntry, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		normalized, ok := normalizeProfileBookmarkEntry(row)
		if !ok {
			continue
		}
		if _, exists := seen[normalized.Key]; exists {
			continue
		}
		seen[normalized.Key] = struct{}{}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AddedAt.Equal(out[j].AddedAt) {
			return out[i].Key < out[j].Key
		}
		return out[i].AddedAt.After(out[j].AddedAt)
	})
	if len(out) > profileMaxBookmarkItems {
		out = out[:profileMaxBookmarkItems]
	}
	body := ""
	if len(out) > 0 {
		raw, err := json.Marshal(out)
		if err != nil {
			return err
		}
		body = string(raw)
	}
	return s.admin.UpsertSystemSetting(key, body)
}

func normalizeCircleID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now().UTC().Format("20060102T150405.000000000")
	}
	if len(raw) > 48 {
		raw = raw[:48]
	}
	return raw
}

func normalizeCircleMembers(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, row := range raw {
		member := strings.TrimSpace(row)
		if member == "" {
			continue
		}
		member = clampForTTY(member, 32)
		key := profileNormalizeHandleKey(member)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, member)
		if len(out) >= profileMaxCircleMembers {
			break
		}
	}
	return out
}

func normalizeProfileCircles(rows []profileCallerCircle) []profileCallerCircle {
	out := make([]profileCallerCircle, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		row.ID = normalizeCircleID(row.ID)
		row.Name = clampForTTY(strings.TrimSpace(row.Name), 48)
		row.Note = clampForTTY(strings.TrimSpace(row.Note), 120)
		if row.Name == "" {
			continue
		}
		if _, exists := seen[row.ID]; exists {
			continue
		}
		seen[row.ID] = struct{}{}
		row.Members = normalizeCircleMembers(row.Members)
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	if len(out) > profileMaxCircles {
		out = out[:profileMaxCircles]
	}
	return out
}

func (s *Server) profileCircles(handle string) []profileCallerCircle {
	if s == nil || s.admin == nil {
		return nil
	}
	key := profileSettingKey(profileSettingCallerCirclesRoot, handle)
	if key == "" {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var decoded []profileCallerCircle
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	return normalizeProfileCircles(decoded)
}

func (s *Server) persistProfileCircles(handle string, rows []profileCallerCircle) error {
	if s == nil || s.admin == nil {
		return fmt.Errorf("circle storage unavailable")
	}
	key := profileSettingKey(profileSettingCallerCirclesRoot, handle)
	if key == "" {
		return fmt.Errorf("missing caller handle")
	}
	clean := normalizeProfileCircles(rows)
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			return err
		}
		body = string(raw)
	}
	return s.admin.UpsertSystemSetting(key, body)
}

func (s *Server) upsertProfileCircle(handle, circleID, name, note string, members []string) error {
	rows := s.profileCircles(handle)
	circleID = strings.TrimSpace(circleID)
	updated := false
	for idx := range rows {
		if circleID == "" || rows[idx].ID != circleID {
			continue
		}
		rows[idx].Name = name
		rows[idx].Note = note
		rows[idx].Members = members
		updated = true
		break
	}
	if !updated {
		rows = append(rows, profileCallerCircle{
			ID:      normalizeCircleID(circleID),
			Name:    name,
			Note:    note,
			Members: members,
		})
	}
	return s.persistProfileCircles(handle, rows)
}

func (s *Server) deleteProfileCircle(handle, circleID string) error {
	circleID = strings.TrimSpace(circleID)
	if circleID == "" {
		return fmt.Errorf("circle id is required")
	}
	rows := s.profileCircles(handle)
	next := make([]profileCallerCircle, 0, len(rows))
	for _, row := range rows {
		if row.ID != circleID {
			next = append(next, row)
		}
	}
	return s.persistProfileCircles(handle, next)
}

func loadJSONSettingObject(adminKey string, admin interface {
	GetSystemSetting(key string) (string, error)
}) map[string]interface{} {
	if admin == nil || strings.TrimSpace(adminKey) == "" {
		return nil
	}
	raw, err := admin.GetSystemSetting(adminKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	obj := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil
	}
	if len(obj) == 0 {
		return nil
	}
	return obj
}

func decodeTimeMap(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	byTime := map[string]time.Time{}
	if err := json.Unmarshal([]byte(raw), &byTime); err == nil {
		out := map[string]string{}
		for key, at := range byTime {
			key = strings.TrimSpace(key)
			if key == "" || at.IsZero() {
				continue
			}
			out[key] = at.UTC().Format(time.RFC3339Nano)
		}
		if len(out) > 0 {
			return out
		}
	}
	byString := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &byString); err == nil && len(byString) > 0 {
		out := map[string]string{}
		for key, value := range byString {
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key == "" || value == "" {
				continue
			}
			out[key] = value
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func (s *Server) loadProfileTimeMap(root, handle string) map[string]string {
	if s == nil || s.admin == nil {
		return nil
	}
	key := profileSettingKey(root, handle)
	if key == "" {
		return nil
	}
	raw, err := s.admin.GetSystemSetting(key)
	if err != nil {
		return nil
	}
	return decodeTimeMap(raw)
}

func defaultDigestPreferencesMap() map[string]interface{} {
	return map[string]interface{}{
		"enabled":           false,
		"max_items":         12,
		"include_events":    true,
		"include_boards":    true,
		"weekly_mail":       false,
		"attention_cadence": "always",
		"bulletin_cadence":  "always",
		"event_cadence":     "always",
	}
}

func (s *Server) loadProfileDigestPreferences(handle string) map[string]interface{} {
	prefs := defaultDigestPreferencesMap()
	if s == nil || s.admin == nil {
		return prefs
	}
	key := pulseDigestPrefsSettingKey(handle)
	if key == "" {
		return prefs
	}
	raw, err := s.admin.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return prefs
	}
	obj := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return prefs
	}
	for key, value := range obj {
		prefs[key] = value
	}
	return prefs
}

func (s *Server) loadProfileHomeRoute(handle string) string {
	if s == nil || s.admin == nil {
		return "/start"
	}
	key := profileSettingKey(profileSettingHomeRouteRoot, handle)
	if key == "" {
		return "/start"
	}
	raw, err := s.admin.GetSystemSetting(key)
	if err != nil {
		return "/start"
	}
	route := strings.TrimSpace(raw)
	if route == "" {
		return "/start"
	}
	return route
}

func (s *Server) buildProfileExportPayload(user *domain.User) map[string]interface{} {
	payload := map[string]interface{}{
		"generated_at":       time.Now().UTC().Format(time.RFC3339Nano),
		"handle":             "",
		"role":               rbac.RoleUser,
		"profile_card":       map[string]interface{}{},
		"contact_aliases":    map[string]string{},
		"caller_circles":     []profileCallerCircle{},
		"digest_preferences": defaultDigestPreferencesMap(),
		"home_route":         "/start",
		"bookmarks":          []profileBookmarkEntry{},
	}
	if user == nil {
		return payload
	}
	handle := strings.TrimSpace(user.Handle)
	payload["handle"] = handle
	payload["role"] = defaultIfBlank(rbac.NormalizeRole(user.Role), rbac.RoleUser)
	payload["theme"] = strings.TrimSpace(user.Theme)
	payload["ansi_enabled"] = user.ANSIEnabled
	payload["paging_enabled"] = user.PagingEnabled
	payload["time_format_24h"] = user.TimeFormat24h
	if profileCard := loadJSONSettingObject(profileSettingKey(profileSettingPublicProfileRoot, handle), s.admin); profileCard != nil {
		payload["profile_card"] = profileCard
	}
	if aliases := loadJSONSettingObject(profileSettingKey(profileSettingContactAliasesRoot, handle), s.admin); aliases != nil {
		payload["contact_aliases"] = aliases
	}
	payload["caller_circles"] = s.profileCircles(handle)
	payload["digest_preferences"] = s.loadProfileDigestPreferences(handle)
	payload["home_route"] = s.loadProfileHomeRoute(handle)
	payload["bookmarks"] = s.profileBookmarks(handle)
	return payload
}

func attentionPresetForRole(role string) string {
	switch rbac.NormalizeRole(role) {
	case rbac.RoleSysop:
		return "staff"
	case rbac.RoleModerator:
		return "trusted"
	default:
		return "caller"
	}
}

func (s *Server) buildAttentionExportPayload(user *domain.User) map[string]interface{} {
	payload := map[string]interface{}{
		"version":                   "1",
		"generated_at":              time.Now().UTC().Format(time.RFC3339Nano),
		"handle":                    "",
		"role":                      rbac.RoleUser,
		"preset_recommendation":     "caller",
		"digest_preferences":        defaultDigestPreferencesMap(),
		"board_subscriptions":       map[string]interface{}{},
		"board_quiet_hours":         map[string]interface{}{},
		"route_seen":                map[string]string{},
		"bulletin_acknowledgements": map[string]string{},
		"attention": map[string]interface{}{
			"dismissed": map[string]string{},
			"read":      map[string]string{},
			"snoozed":   map[string]string{},
		},
	}
	if user == nil {
		return payload
	}
	handle := strings.TrimSpace(user.Handle)
	payload["handle"] = handle
	payload["role"] = defaultIfBlank(rbac.NormalizeRole(user.Role), rbac.RoleUser)
	payload["preset_recommendation"] = attentionPresetForRole(user.Role)
	payload["digest_preferences"] = s.loadProfileDigestPreferences(handle)
	if subscriptions := loadJSONSettingObject(profileSettingKey(profileSettingBoardSubscriptionsRoot, handle), s.admin); subscriptions != nil {
		payload["board_subscriptions"] = subscriptions
	}
	if quietHours := loadJSONSettingObject(profileSettingKey(profileSettingBoardQuietHoursRoot, handle), s.admin); quietHours != nil {
		payload["board_quiet_hours"] = quietHours
	}
	payload["route_seen"] = s.loadProfileTimeMap(profileSettingRouteSeenRoot, handle)
	payload["bulletin_acknowledgements"] = s.loadProfileTimeMap(profileSettingBulletinAckRoot, handle)
	attention := payload["attention"].(map[string]interface{})
	attention["dismissed"] = s.loadProfileTimeMap(profileSettingAttentionDismissedRoot, handle)
	attention["read"] = s.loadProfileTimeMap(profileSettingAttentionReadRoot, handle)
	attention["snoozed"] = s.loadProfileTimeMap(profileSettingAttentionSnoozeRoot, handle)
	return payload
}

func profileExportRootDir() string {
	root := strings.TrimSpace(os.Getenv("WOLFBBS_OFFLINE_DIR"))
	if root == "" {
		root = filepath.Join(installPrefixPathSSH(), "offline")
	}
	return filepath.Join(filepath.Clean(root), "caller")
}

func saveExportJSON(kind, handle string, body []byte) (string, error) {
	handleKey := profileNormalizeHandleKey(handle)
	if handleKey == "" {
		handleKey = "caller"
	}
	dir := filepath.Join(profileExportRootDir(), handleKey)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("wolfbbs-%s-%s.json", kind, time.Now().UTC().Format("20060102-150405"))
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func promptSaveExport(out io.Writer, reader *bufio.Reader, handle, kind string, body []byte) (string, error) {
	_, _ = io.WriteString(out, "\r\nSave JSON export to offline path? (Y/N): ")
	choice, err := readLine(reader, 8)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(strings.TrimSpace(choice), "y") && !strings.EqualFold(strings.TrimSpace(choice), "yes") {
		return "", nil
	}
	return saveExportJSON(kind, handle, body)
}

func (s *Server) runProfileExportViewer(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, user *domain.User, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if touch == nil {
		touch = func() {}
	}
	payload := s.buildProfileExportPayload(user)
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		adminPause(sess, reader, touch, "Could not render profile export JSON: "+err.Error())
		return
	}
	writeClear(sess, ansiEnabled)
	renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Profile Export JSON", defaultIfBlank(user.Handle, "caller"), time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
	pagerWrite(sess, reader, string(body))
	path, saveErr := promptSaveExport(sess, reader, defaultIfBlank(user.Handle, "caller"), "profile", body)
	if saveErr != nil {
		adminPause(sess, reader, touch, "Could not save profile export: "+saveErr.Error())
		return
	}
	if strings.TrimSpace(path) != "" {
		adminPause(sess, reader, touch, "Saved profile export: "+path)
		return
	}
	touch()
}

func (s *Server) runAttentionExportViewer(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, user *domain.User, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if touch == nil {
		touch = func() {}
	}
	payload := s.buildAttentionExportPayload(user)
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		adminPause(sess, reader, touch, "Could not render attention export JSON: "+err.Error())
		return
	}
	writeClear(sess, ansiEnabled)
	renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Attention Export JSON", defaultIfBlank(user.Handle, "caller"), time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
	pagerWrite(sess, reader, string(body))
	path, saveErr := promptSaveExport(sess, reader, defaultIfBlank(user.Handle, "caller"), "notifications", body)
	if saveErr != nil {
		adminPause(sess, reader, touch, "Could not save attention export: "+saveErr.Error())
		return
	}
	if strings.TrimSpace(path) != "" {
		adminPause(sess, reader, touch, "Saved attention export: "+path)
		return
	}
	touch()
}

func (s *Server) runBookmarkCenter(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if touch == nil {
		touch = func() {}
	}
	if s == nil || s.admin == nil {
		adminPause(sess, reader, touch, "Bookmark storage is unavailable.")
		return
	}
	for {
		rows := s.profileBookmarks(handle)
		lines := []string{
			"Read-later queue parity for /bookmarks",
			"",
			fmt.Sprintf("%-3s %-14s %-36s %-14s", "#", "Kind", "Label", "Added"),
			strings.Repeat("-", 74),
		}
		if len(rows) == 0 {
			lines = append(lines, "(no bookmark rows)")
		} else {
			for idx, row := range rows {
				lines = append(lines, fmt.Sprintf("%-3d %-14s %-36s %-14s",
					idx+1,
					clampForTTY(strings.ReplaceAll(row.Kind, "_", " "), 14),
					clampForTTY(defaultIfBlank(row.Label, row.Key), 36),
					formatClock(row.AddedAt.Local(), time24h),
				))
			}
		}
		lines = append(lines, "", "Commands: D <index|key> delete  C clear  Q return")
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Bookmarks", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Bookmarks", lines, ui.CP437Box, ui.FgCyan, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 80)
		if err != nil {
			return
		}
		touch()
		cmd := strings.TrimSpace(raw)
		if cmd == "" {
			continue
		}
		if strings.EqualFold(cmd, "q") || strings.EqualFold(cmd, "back") {
			return
		}
		if strings.EqualFold(cmd, "c") || strings.EqualFold(cmd, "clear") {
			if err := s.persistProfileBookmarks(handle, nil); err != nil {
				adminPause(sess, reader, touch, "Could not clear bookmarks: "+err.Error())
			} else {
				adminPause(sess, reader, touch, "Bookmarks cleared.")
			}
			continue
		}
		parts := strings.Fields(cmd)
		if len(parts) >= 2 && strings.EqualFold(parts[0], "d") {
			target := strings.TrimSpace(strings.Join(parts[1:], " "))
			if target == "" {
				adminPause(sess, reader, touch, "Usage: D <index|key>")
				continue
			}
			key := target
			if idx, parseErr := strconv.Atoi(target); parseErr == nil && idx >= 1 && idx <= len(rows) {
				key = rows[idx-1].Key
			}
			next := make([]profileBookmarkEntry, 0, len(rows))
			removed := false
			for _, row := range rows {
				if row.Key == key {
					removed = true
					continue
				}
				next = append(next, row)
			}
			if !removed {
				adminPause(sess, reader, touch, "Bookmark not found.")
				continue
			}
			if err := s.persistProfileBookmarks(handle, next); err != nil {
				adminPause(sess, reader, touch, "Could not delete bookmark: "+err.Error())
				continue
			}
			adminPause(sess, reader, touch, "Bookmark removed.")
			continue
		}
		adminPause(sess, reader, touch, "Use D <index|key>, C, or Q.")
	}
}

func (s *Server) runCircleCenter(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if touch == nil {
		touch = func() {}
	}
	if s == nil || s.admin == nil {
		adminPause(sess, reader, touch, "Circle storage is unavailable.")
		return
	}
	for {
		rows := s.profileCircles(handle)
		lines := []string{
			"Caller circles parity for /circles",
			"",
			fmt.Sprintf("%-3s %-24s %-8s %-28s", "#", "Name", "Members", "Note"),
			strings.Repeat("-", 78),
		}
		if len(rows) == 0 {
			lines = append(lines, "(no circles)")
		} else {
			for idx, row := range rows {
				lines = append(lines, fmt.Sprintf("%-3d %-24s %-8d %-28s",
					idx+1,
					clampForTTY(row.Name, 24),
					len(row.Members),
					clampForTTY(row.Note, 28),
				))
			}
		}
		lines = append(lines, "", "Commands: N new circle  D <index|id> delete  Q return")
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Caller Circles", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Circles", lines, ui.CP437Box, ui.FgCyan, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 120)
		if err != nil {
			return
		}
		touch()
		cmd := strings.TrimSpace(raw)
		if cmd == "" {
			continue
		}
		if strings.EqualFold(cmd, "q") || strings.EqualFold(cmd, "back") {
			return
		}
		if strings.EqualFold(cmd, "n") || strings.EqualFold(cmd, "new") {
			_, _ = io.WriteString(sess, "\r\nCircle name: ")
			name, err := readLine(reader, 64)
			if err != nil {
				return
			}
			touch()
			name = strings.TrimSpace(name)
			if name == "" {
				adminPause(sess, reader, touch, "Circle name is required.")
				continue
			}
			_, _ = io.WriteString(sess, "Members (comma-separated handles): ")
			memberText, err := readLine(reader, 240)
			if err != nil {
				return
			}
			touch()
			_, _ = io.WriteString(sess, "Note: ")
			note, err := readLine(reader, 180)
			if err != nil {
				return
			}
			touch()
			if err := s.upsertProfileCircle(handle, "", name, note, splitCSV(memberText)); err != nil {
				adminPause(sess, reader, touch, "Could not save circle: "+err.Error())
				continue
			}
			adminPause(sess, reader, touch, "Circle saved.")
			continue
		}
		parts := strings.Fields(cmd)
		if len(parts) >= 2 && strings.EqualFold(parts[0], "d") {
			target := strings.TrimSpace(strings.Join(parts[1:], " "))
			if target == "" {
				adminPause(sess, reader, touch, "Usage: D <index|id>")
				continue
			}
			circleID := target
			if idx, parseErr := strconv.Atoi(target); parseErr == nil && idx >= 1 && idx <= len(rows) {
				circleID = rows[idx-1].ID
			}
			if err := s.deleteProfileCircle(handle, circleID); err != nil {
				adminPause(sess, reader, touch, "Could not delete circle: "+err.Error())
				continue
			}
			adminPause(sess, reader, touch, "Circle removed.")
			continue
		}
		adminPause(sess, reader, touch, "Use N, D <index|id>, or Q.")
	}
}
