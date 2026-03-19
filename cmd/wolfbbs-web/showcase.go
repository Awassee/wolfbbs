package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"wolfbbs/internal/rbac"
)

func (a *webApp) handleShowcase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, _ := a.currentUser(r)
	if user != nil {
		a.markRouteSeen(user.Handle, "/showcase")
	}

	boardCount := 0
	if a.boardRepo != nil {
		if rows, err := a.boardRepo.List(); err == nil {
			boardCount = len(rows)
		}
	}
	doorCount := 0
	if a.doorRegistry != nil {
		doorCount = len(a.doorRegistry.Doors())
	}
	onlineCount := 0
	if a.chatSvc != nil {
		onlineCount = len(a.chatSvc.Online())
	}
	now := time.Now()
	eventCount := 0
	for _, row := range a.loadCommunityEvents() {
		if row.StartsAt.After(now.Add(-30*time.Minute)) && row.StartsAt.Before(now.Add(14*24*time.Hour)) {
			eventCount++
		}
	}

	nav := `<a href="/start">start</a> | <a href="/connect">connect</a> | <a href="/tour">tour</a> | <a href="/help">help</a> | <a href="/login">login</a>`
	roleLabel := "guest"
	if user != nil {
		roleLabel = rbac.NormalizeRole(user.Role)
		nav = `<a href="/start">start</a> | <a href="/today">today</a> | <a href="/attention">attention</a> | <a href="/boards">boards</a> | <a href="/chat">chat</a> | <a href="/doors">doors</a> | <a href="/gateway">gateway</a> | <a href="/offline">offline</a> | <a href="/help">help</a> | <a href="/logout">logout</a>`
		if a.hasRole(user, roleAdmin) {
			nav = `<a href="/admin">admin</a> | <a href="/admin/setup">setup</a> | <a href="/admin/ops">ops</a> | <a href="/admin/release">release</a> | <a href="/status">status</a> | <a href="/start">start</a> | <a href="/help">help</a> | <a href="/logout">logout</a>`
		}
	}

	firstRunBlock := ``
	if user != nil {
		snapshot := a.buildFirstCallSnapshot(user)
		firstRunBlock = `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>First-Run Progress</h2><p>Use this to verify that your account can complete the full caller loop.</p>` + renderOnboardingChecklist("First-Run Progress", snapshot.Tasks) + `<p><a href="/first-call">Open guided first-call session</a></p></article></section>`
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>` + htmlEscape(a.siteDisplayName()) + ` Showcase</title></head><body>
<h1>` + htmlEscape(a.siteDisplayName()) + ` Product Showcase</h1>
<p>` + nav + `</p>
<p>Role lane: <strong>` + htmlEscape(roleLabel) + `</strong></p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(boardCount) + `</strong><span>boards configured</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(doorCount) + `</strong><span>doors available</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(onlineCount) + `</strong><span>callers online now</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(eventCount) + `</strong><span>upcoming events (14d)</span></article></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Experience Lanes</h2><div class="wolfbbs-action-grid"><a class="wolfbbs-action-card" href="/connect"><strong>Terminal + Web connect</strong><span>SSH presets, browser terminal, and practical connection checks</span></a><a class="wolfbbs-action-card" href="/boards"><strong>Boards + Mail</strong><span>classic long-form and private reply loop</span></a><a class="wolfbbs-action-card" href="/chat"><strong>Live chat + IRC</strong><span>shared channels with moderation controls</span></a><a class="wolfbbs-action-card" href="/gateway?view=files"><strong>FileBase + queue</strong><span>tag search, batch zip, and one-time ticket downloads</span></a><a class="wolfbbs-action-card" href="/offline"><strong>Offline packets</strong><span>QWK export/import for disconnected reading</span></a><a class="wolfbbs-action-card" href="/topx"><strong>TopX momentum</strong><span>weekly/monthly/all-time caller activity ranking</span></a></div></article><article class="wolfbbs-card"><h2>Operator Toolkit</h2><ul class="wolfbbs-list-clean"><li><a href="/admin/setup">Setup wizard</a> for identity, safety, and bootstrap.</li><li><a href="/admin/ops">Ops center</a> for runtime triage and audit visibility.</li><li><a href="/admin/release">Release dashboard</a> for QA evidence and checklist gating.</li><li><a href="/admin/plugins">Plugin contracts</a> with starter SDK pack export.</li><li><a href="/admin/files">File ops</a> for upload intake, review queue, and ticket issuance.</li></ul></article></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>10-minute smoke flow</h2><ol><li>Open <a href="/connect">/connect</a> and verify your preferred client route.</li><li>Create one board post in <a href="/boards">/boards</a> and one chat line in <a href="/chat">/chat</a>.</li><li>Open <a href="/gateway?view=files">/gateway?view=files</a> and issue a download ticket from queue.</li><li>Run one door from <a href="/doors">/doors</a> and confirm <a href="/scores">/scores</a> updates.</li><li>If you are sysop, confirm <a href="/status">/status</a> and <a href="/admin/ops">/admin/ops</a> are clean.</li></ol></article><article class="wolfbbs-card"><h2>Docs map</h2><ul class="wolfbbs-list-clean"><li><code>docs/PRODUCT_GUIDE.md</code></li><li><code>docs/feature-reference.md</code></li><li><code>docs/OPERATOR_PLAYBOOK.md</code></li><li><code>docs/LAUNCH_CHECKLIST.md</code></li><li><code>docs/EXTENSION_SDK.md</code></li></ul></article></section>
` + firstRunBlock + `
<p class="wolfbbs-muted">This page is designed as a practical capability map, not a marketing splash. Every section links to a working route.</p>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(strings.TrimSpace(page)))
}
