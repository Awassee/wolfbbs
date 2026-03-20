package sshserver

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"wolfbbs/internal/chat"
)

type ttyChatChannelSummary struct {
	Name            string
	Topic           string
	Joined          bool
	Current         bool
	Locked          bool
	OnlineCount     int
	LastMessageID   int64
	LastMessageAt   time.Time
	LastMessageFrom string
	LastPreview     string
}

func ttyChatActivityLabel(ts time.Time, time24h bool) string {
	if ts.IsZero() {
		return "quiet"
	}
	now := time.Now()
	delta := now.Sub(ts)
	switch {
	case delta < time.Minute:
		return "now"
	case delta < time.Hour:
		return fmt.Sprintf("%dm", int(delta.Minutes()))
	case delta < 24*time.Hour:
		return formatClock(ts.Local(), time24h)
	default:
		return ts.Local().Format("01-02")
	}
}

func ttyChatTopic(channel string, locked bool) string {
	channel = chat.NormalizeChannel(strings.TrimSpace(channel))
	base := "Shared live room for web, SSH, and IRC callers."
	switch strings.ToLower(channel) {
	case "#lobby":
		base = "Main lobby for general chat, greetings, and quick social check-ins."
	case "#help":
		base = "Help room for onboarding and operator nudges."
	default:
		name := strings.TrimSpace(strings.ReplaceAll(strings.TrimPrefix(channel, "#"), "-", " "))
		if name != "" {
			base = "Live room for " + name + " conversation across the board."
		}
	}
	if locked {
		return base + " Read-only for non-moderators while locked."
	}
	return base
}

func (s *Server) chatTTYSummaries(handle, current string) []ttyChatChannelSummary {
	current = chat.NormalizeChannel(strings.TrimSpace(current))
	if current == "" {
		current = "#lobby"
	}
	if s.chatSvc == nil {
		return []ttyChatChannelSummary{{
			Name:    current,
			Joined:  true,
			Current: true,
			Locked:  s.isChannelLockedSSH(current),
		}}
	}
	channels := s.chatSvc.ListChannels()
	if len(channels) == 0 {
		channels = []string{"#lobby"}
	}
	seen := map[string]struct{}{}
	ordered := make([]string, 0, len(channels)+1)
	for _, channel := range append(channels, current) {
		channel = chat.NormalizeChannel(strings.TrimSpace(channel))
		if channel == "" {
			continue
		}
		if _, ok := seen[channel]; ok {
			continue
		}
		seen[channel] = struct{}{}
		ordered = append(ordered, channel)
	}
	out := make([]ttyChatChannelSummary, 0, len(ordered))
	for _, channel := range ordered {
		row := ttyChatChannelSummary{
			Name:        channel,
			Joined:      channel == "#lobby" || s.chatSvc.IsInChannel(handle, channel),
			Current:     channel == current,
			Locked:      s.isChannelLockedSSH(channel),
			OnlineCount: len(s.chatSvc.OnlineInChannel(channel)),
		}
		row.Topic = ttyChatTopic(channel, row.Locked)
		history := s.chatSvc.History(channel, 1)
		if len(history) > 0 {
			last := history[len(history)-1]
			row.LastMessageID = last.ID
			row.LastMessageAt = last.CreatedAt
			row.LastMessageFrom = defaultIfBlank(strings.TrimSpace(last.From), "system")
			row.LastPreview = clampForTTY(strings.TrimSpace(last.Body), 56)
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Current != out[j].Current {
			return out[i].Current
		}
		if out[i].Joined != out[j].Joined {
			return out[i].Joined
		}
		if out[i].OnlineCount != out[j].OnlineCount {
			return out[i].OnlineCount > out[j].OnlineCount
		}
		if out[i].LastMessageID != out[j].LastMessageID {
			return out[i].LastMessageID > out[j].LastMessageID
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func chatTTYSlotRows(width int, summaries []ttyChatChannelSummary, time24h bool) ([]string, []ttyChatChannelSummary) {
	if len(summaries) == 0 {
		return nil, nil
	}
	limit := len(summaries)
	if limit > 9 {
		limit = 9
	}
	visible := make([]ttyChatChannelSummary, 0, limit)
	rows := make([]string, 0, limit)
	for idx, row := range summaries[:limit] {
		stateParts := make([]string, 0, 4)
		if row.Current {
			stateParts = append(stateParts, "current")
		} else if row.Joined {
			stateParts = append(stateParts, "joined")
		} else {
			stateParts = append(stateParts, "watch")
		}
		if row.Locked {
			stateParts = append(stateParts, "locked")
		}
		if row.OnlineCount > 0 {
			stateParts = append(stateParts, fmt.Sprintf("%d live", row.OnlineCount))
		} else {
			stateParts = append(stateParts, ttyChatActivityLabel(row.LastMessageAt, time24h))
		}
		preview := defaultIfBlank(row.LastPreview, "no recent traffic")
		if row.LastMessageFrom != "" {
			preview = row.LastMessageFrom + ": " + preview
		}
		rows = append(rows, fmt.Sprintf("%d) %-12s %-20s %s",
			idx+1,
			clampForTTY(row.Name, 12),
			clampForTTY(strings.Join(stateParts, " | "), 20),
			clampForTTY(preview, maxTTYInt(16, width-38)),
		))
		visible = append(visible, row)
	}
	return rows, visible
}

func chatTTYTranscriptRows(width int, history []chat.Message, time24h bool) []string {
	if len(history) == 0 {
		return nil
	}
	rows := make([]string, 0, len(history))
	bodyWidth := maxInt(12, width-24)
	for _, msg := range history {
		rows = append(rows, fmt.Sprintf("%-7s %-10s %s",
			formatClock(msg.CreatedAt.Local(), time24h),
			clampForTTY(defaultIfBlank(msg.From, "system"), 10),
			clampForTTY(strings.TrimSpace(msg.Body), bodyWidth),
		))
	}
	return rows
}

func firstJoinedChatFallback(summaries []ttyChatChannelSummary, current string) string {
	current = chat.NormalizeChannel(strings.TrimSpace(current))
	for _, row := range summaries {
		if row.Name == current {
			continue
		}
		if row.Joined {
			return row.Name
		}
	}
	return "#lobby"
}

func maxTTYInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
