package sshserver

import (
	"fmt"
	"strings"
	"time"

	"wolfbbs/internal/domain"
)

type adminSignalSnapshot struct {
	WindowDays       int
	ActiveBoards     int
	ActiveChannels   int
	SetupFailures    int
	UpgradeSuccesses int
	UpgradeFailures  int
	UserCreates      int
}

func (s *Server) recordAdminAudit(actor, target, action, details string) {
	if s == nil || s.admin == nil {
		return
	}
	_ = s.admin.AddAudit(&domain.AdminAudit{
		Actor:   strings.TrimSpace(actor),
		Target:  strings.TrimSpace(target),
		Action:  strings.TrimSpace(action),
		Details: strings.TrimSpace(details),
	})
}

func (s *Server) collectAdminSignals(windowDays int) adminSignalSnapshot {
	if windowDays <= 0 {
		windowDays = 7
	}
	out := adminSignalSnapshot{WindowDays: windowDays}
	since := time.Now().UTC().Add(-time.Duration(windowDays) * 24 * time.Hour)

	boardIDs := map[int64]struct{}{}
	if s != nil && s.boards != nil && s.msgs != nil {
		if boards, err := s.boards.List(); err == nil {
			for _, board := range boards {
				rows, err := s.msgs.ListByBoard(board.ID)
				if err != nil {
					continue
				}
				for _, msg := range rows {
					if msg.CreatedAt.Before(since) {
						continue
					}
					boardIDs[board.ID] = struct{}{}
				}
			}
		}
	}
	out.ActiveBoards = len(boardIDs)

	channelNames := map[string]struct{}{}
	if s != nil && s.chatSvc != nil {
		for _, channel := range s.chatSvc.ListChannels() {
			for _, msg := range s.chatSvc.History(channel, 5000) {
				if msg.CreatedAt.Before(since) {
					continue
				}
				channelNames[strings.ToLower(strings.TrimSpace(channel))] = struct{}{}
			}
		}
	}
	out.ActiveChannels = len(channelNames)

	if s == nil || s.admin == nil {
		return out
	}
	rows, err := s.admin.ListAudit(5000)
	if err != nil {
		return out
	}
	for _, row := range rows {
		if row.CreatedAt.Before(since) {
			continue
		}
		action := strings.ToLower(strings.TrimSpace(row.Action))
		details := strings.ToLower(strings.TrimSpace(row.Details))
		switch action {
		case "save_setup_profile", "save_identity", "save_site_text", "save_runtime_flags", "ensure_mailbot", "seed_default_boards":
			if strings.Contains(details, "failed") {
				out.SetupFailures++
			}
		case "create_user":
			out.UserCreates++
		case "app_upgrade_success":
			out.UpgradeSuccesses++
		case "app_upgrade_failed":
			out.UpgradeFailures++
		}
	}
	return out
}

func (s *Server) adminSignalLines(windowDays int) []string {
	signals := s.collectAdminSignals(windowDays)
	return []string{
		fmt.Sprintf("%dd boards active: %d   chat channels active: %d", signals.WindowDays, signals.ActiveBoards, signals.ActiveChannels),
		fmt.Sprintf("%dd setup failures: %d   caller accounts created: %d", signals.WindowDays, signals.SetupFailures, signals.UserCreates),
		fmt.Sprintf("%dd app upgrade success/fail: %d/%d", signals.WindowDays, signals.UpgradeSuccesses, signals.UpgradeFailures),
	}
}
