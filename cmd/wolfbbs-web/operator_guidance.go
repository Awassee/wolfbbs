package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"wolfbbs/internal/domain"
)

type operatorSignalSnapshot struct {
	WindowDays          int
	FirstCallCompletes  int
	ActiveBoards        int
	ActiveChannels      int
	SetupActions        int
	SetupFailures       int
	UserCreates         int
	UpgradeSuccesses    int
	UpgradeFailures     int
	ReleaseChecklistOps int
}

func (a *webApp) collectOperatorSignals(windowDays int) operatorSignalSnapshot {
	if windowDays <= 0 {
		windowDays = 7
	}
	out := operatorSignalSnapshot{WindowDays: windowDays}
	since := time.Now().UTC().Add(-time.Duration(windowDays) * 24 * time.Hour)

	boardIDs := map[int64]struct{}{}
	if a.boardRepo != nil && a.msgRepo != nil {
		if boards, err := a.boardRepo.List(); err == nil {
			for _, board := range boards {
				rows, err := a.msgRepo.ListByBoard(board.ID)
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

	channels := map[string]struct{}{}
	if a.chatSvc != nil {
		for _, channel := range a.chatSvc.ListChannels() {
			for _, msg := range a.chatSvc.History(channel, 5000) {
				if msg.CreatedAt.Before(since) {
					continue
				}
				channels[strings.ToLower(strings.TrimSpace(channel))] = struct{}{}
			}
		}
	}
	out.ActiveChannels = len(channels)

	for _, row := range a.loadOperatorInsightEvents() {
		if row.At.Before(since) {
			continue
		}
		if row.Kind == "first_call.complete" {
			out.FirstCallCompletes++
		}
	}

	if a.adminRepo == nil {
		return out
	}
	auditRows, err := a.adminRepo.ListAudit(5000)
	if err != nil {
		return out
	}
	for _, row := range auditRows {
		if row.CreatedAt.Before(since) {
			continue
		}
		action := strings.ToLower(strings.TrimSpace(row.Action))
		details := strings.ToLower(strings.TrimSpace(row.Details))
		switch action {
		case "save_setup_profile", "save_identity", "save_site_text", "save_runtime_flags", "ensure_mailbot", "seed_default_boards":
			out.SetupActions++
			if strings.Contains(details, "failed") {
				out.SetupFailures++
			}
		case "create_user":
			out.UserCreates++
		case "app_upgrade_success":
			out.UpgradeSuccesses++
		case "app_upgrade_failed":
			out.UpgradeFailures++
		case "toggle", "clear":
			if strings.EqualFold(strings.TrimSpace(row.Target), "release.checklist") {
				out.ReleaseChecklistOps++
			}
		}
	}
	return out
}

func readinessCheckOK(snapshot statusSnapshot, name string) bool {
	for _, row := range snapshot.Checks {
		if strings.EqualFold(strings.TrimSpace(row.Name), strings.TrimSpace(name)) {
			return row.OK
		}
	}
	return false
}

func (a *webApp) renderSysopFirstRunBlock(user *domain.User) string {
	readiness := a.buildSetupReadinessSnapshot(user)
	signals := a.collectOperatorSignals(7)
	steps := []struct {
		Title  string
		Detail string
		Href   string
		OK     bool
	}{
		{
			Title:  "Identity is public-ready",
			Detail: "Board name and hostname are no longer test defaults.",
			Href:   "/admin/setup?step=1",
			OK:     readinessCheckOK(readiness, "Site identity customized"),
		},
		{
			Title:  "Safety baseline is set",
			Detail: "Verified-email and secure-cookie posture are configured for the real deployment shape.",
			Href:   "/admin/setup?step=2",
			OK:     readinessCheckOK(readiness, "Safety baseline set"),
		},
		{
			Title:  "Bootstrap surfaces are seeded",
			Detail: "Boards and the mailbot account exist so callers do not hit blank or broken paths.",
			Href:   "/admin/setup?step=4",
			OK:     readinessCheckOK(readiness, "Boards seeded") && readinessCheckOK(readiness, "Mailbot service account"),
		},
		{
			Title:  "A real caller path exists",
			Detail: "At least one non-sysop account has been created for realistic testing.",
			Href:   "/admin/users",
			OK:     readinessCheckOK(readiness, "Real caller account exists"),
		},
		{
			Title:  "A first-call loop has been proven",
			Detail: "A caller completed the guided first-call session and exercised the day-one surfaces.",
			Href:   "/first-call",
			OK:     signals.FirstCallCompletes > 0,
		},
	}

	stepRows := strings.Builder{}
	done := 0
	for _, row := range steps {
		if row.OK {
			done++
		}
		status := "Open"
		if row.OK {
			status = "Done"
		}
		stepRows.WriteString(`<tr><td><strong>` + htmlEscape(row.Title) + `</strong><br><span class="wolfbbs-muted">` + htmlEscape(row.Detail) + `</span></td><td>` + status + `</td><td><a href="` + htmlEscape(row.Href) + `">Open</a></td></tr>`)
	}

	return `<section class="wolfbbs-grid">` +
		`<article class="wolfbbs-card"><h2>First 15 Minutes As Sysop</h2><p><strong>` + strconv.Itoa(done) + `/` + strconv.Itoa(len(steps)) + `</strong> first-run operator checkpoints cleared.</p>` +
		`<table border="1"><tr><th>Checkpoint</th><th>Status</th><th>Action</th></tr>` + stepRows.String() + `</table>` +
		`<p><a href="/admin/launch">Launch Center</a> | <a href="/admin/analytics">Analytics</a> | <a href="/status">Status</a> | <code>docs/FIRST_30_MINUTES.md</code></p></article>` +
		`<article class="wolfbbs-card"><h2>7-Day Operator Signals</h2>` +
		`<div class="wolfbbs-kpi-grid">` +
		`<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.FirstCallCompletes) + `</strong><span>first-call completes</span></article>` +
		`<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.ActiveBoards) + `</strong><span>active boards</span></article>` +
		`<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.ActiveChannels) + `</strong><span>active chat channels</span></article>` +
		`<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.SetupFailures) + `</strong><span>setup failures</span></article>` +
		`<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.UpgradeFailures) + `</strong><span>upgrade failures</span></article>` +
		`<article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.UserCreates) + `</strong><span>caller accounts created</span></article>` +
		`</div>` +
		`<p class="wolfbbs-muted">Use this block as the fast answer to “what still feels unproven?” before you move on to polish.</p></article>` +
		`</section>`
}

func buildOperatorSignalRecommendations(signals operatorSignalSnapshot) []string {
	out := make([]string, 0, 6)
	if signals.SetupFailures > 0 {
		out = append(out, fmt.Sprintf("%d setup action(s) failed in the last %d days. Re-run /admin/setup and clear the failure mode before expanding scope.", signals.SetupFailures, signals.WindowDays))
	}
	if signals.ActiveBoards == 0 {
		out = append(out, "No boards showed real traffic in this window. Seed content or walk the board path as a real caller.")
	}
	if signals.ActiveChannels == 0 {
		out = append(out, "No chat channels showed traffic in this window. Send a live message in /chat or through IRC before announcing the board.")
	}
	if signals.FirstCallCompletes == 0 {
		out = append(out, "No guided first-call sessions completed recently. Run /first-call after setup or upgrade changes.")
	}
	if signals.UpgradeFailures > 0 {
		out = append(out, fmt.Sprintf("%d in-app upgrade attempt(s) failed recently. Review /admin/release and installer doctor output before the next rollout window.", signals.UpgradeFailures))
	}
	if len(out) == 0 {
		out = append(out, "Operator signals look healthy. Keep walking the real caller path after each meaningful config or release change.")
	}
	return out
}
