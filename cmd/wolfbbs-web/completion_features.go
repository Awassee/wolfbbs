package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/mailflow"
)

func (a *webApp) loadMailTemplates(handle string) []mailflow.Template {
	return mailflow.LoadTemplates(a.adminRepo, handle)
}

func (a *webApp) mailTemplate(handle, id string) (mailflow.Template, bool) {
	return mailflow.FindTemplate(a.loadMailTemplates(handle), id)
}

func (a *webApp) saveMailTemplate(handle, id, name, subject, body, urgency string) error {
	_, err := mailflow.UpsertTemplate(a.adminRepo, handle, mailflow.Template{
		ID:      strings.TrimSpace(id),
		Name:    strings.TrimSpace(name),
		Subject: strings.TrimSpace(subject),
		Body:    strings.TrimSpace(body),
		Urgency: strings.TrimSpace(urgency),
	})
	return err
}

func (a *webApp) deleteMailTemplate(handle, id string) error {
	_, err := mailflow.DeleteTemplate(a.adminRepo, handle, id)
	return err
}

func (a *webApp) markMailTemplateUsed(handle, id string) {
	_ = mailflow.RecordUse(a.adminRepo, handle, id)
}

func renderMailSavedTemplateOptions(templates []mailflow.Template, current string) string {
	rows := strings.Builder{}
	rows.WriteString(`<option value="">built-in kits</option>`)
	for _, row := range templates {
		rows.WriteString(`<option value="` + htmlEscape(row.ID) + `"` + selectedIf(strings.TrimSpace(current) == row.ID) + `>` + htmlEscape(row.Name) + `</option>`)
	}
	return rows.String()
}

func renderMailTemplateManager(handle, csrf, selectedID string, templates []mailflow.Template) string {
	rows := strings.Builder{}
	for _, row := range templates {
		urgency := row.Urgency
		if urgency == "" {
			urgency = "normal"
		}
		rows.WriteString(`<tr><td><strong>` + htmlEscape(row.Name) + `</strong><br><span class="wolfbbs-muted">used ` + strconv.Itoa(row.UseCount) + `x</span></td><td>` + htmlEscape(row.Subject) + `</td><td>` + htmlEscape(urgency) + `</td><td>` + row.UpdatedAt.Local().Format("2006-01-02 15:04") + `</td><td><a href="/mail?saved_template=` + urlQueryEscape(row.ID) + `">use</a> <form method="POST" action="/mail" class="wolfbbs-inline-form">` + csrf + `<input type="hidden" name="action" value="delete_template"><input type="hidden" name="template_id" value="` + htmlEscape(row.ID) + `"><button type="submit">delete</button></form></td></tr>`)
	}
	if rows.Len() == 0 {
		rows.WriteString(`<tr><td colspan="5">No saved reply kits yet.</td></tr>`)
	}
	selectedName := ""
	selectedSubject := ""
	selectedBody := ""
	selectedUrgency := "normal"
	if row, ok := mailflow.FindTemplate(templates, selectedID); ok {
		selectedName = row.Name
		selectedSubject = row.Subject
		selectedBody = row.Body
		if row.Urgency != "" {
			selectedUrgency = row.Urgency
		}
	}
	return `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Saved Reply Kits</h2><p>Keep your strongest recurring replies ready: follow-ups, door invites, moderator handoffs, and welcome notes.</p><form method="POST" action="/mail"><input type="hidden" name="action" value="save_template">` + csrf + `<input type="hidden" name="template_id" value="` + htmlEscape(selectedID) + `"><label>Name <input name="template_name" size="20" value="` + htmlEscape(selectedName) + `" placeholder="Door invite"></label><label>Urgency <select name="template_urgency">` + mailUrgencyOptionRows(selectedUrgency) + `</select></label><br><label>Subject <input name="template_subject" size="56" value="` + htmlEscape(selectedSubject) + `" placeholder="Meet me in the Door Hub"></label><br><label>Body<br><textarea name="template_body" rows="6" cols="72" placeholder="A short reusable note that still feels personal.">` + htmlEscape(selectedBody) + `</textarea></label><br><button type="submit">` + map[bool]string{true: "Update kit", false: "Save reply kit"}[selectedID != ""] + `</button></form></article><article class="wolfbbs-card"><h2>Saved Kit Library</h2><table border="1"><tr><th>Kit</th><th>Subject</th><th>Urgency</th><th>Updated</th><th>Action</th></tr>` + rows.String() + `</table></article></section>`
}

func urlQueryEscape(value string) string {
	replacer := strings.NewReplacer("%", "%25", " ", "%20", "&", "%26", "?", "%3F", "=", "%3D", "#", "%23", "+", "%2B")
	return replacer.Replace(value)
}

type fileWorkflowSummary struct {
	OpenRequests        int
	ClaimedRequests     int
	ReviewItems         int
	ReadyDrafts         int
	FeaturedCollections int
	RepairIssues        int
	Recommendations     []string
}

func buildFileWorkflowSummary(requests []fileRequestItem, drafts []uploadDraftItem, reviewQueue map[int64]fileReviewItem, collections []featuredFileCollection, repairIssues []fileRepairIssue) fileWorkflowSummary {
	out := fileWorkflowSummary{FeaturedCollections: len(collections), RepairIssues: len(repairIssues)}
	for _, row := range requests {
		switch row.Status {
		case "open":
			out.OpenRequests++
		case "claimed":
			out.ClaimedRequests++
		}
	}
	for _, row := range drafts {
		if row.Status == "review" || row.Status == "ready" || row.Status == "approved" {
			out.ReviewItems++
		}
		if row.Status == "ready" || row.Status == "approved" {
			out.ReadyDrafts++
		}
	}
	for _, row := range reviewQueue {
		if row.Status != "approved" {
			out.ReviewItems++
		}
	}
	if out.OpenRequests > 0 {
		out.Recommendations = append(out.Recommendations, fmt.Sprintf("Claim %d open request(s) so callers can see who owns the backlog.", out.OpenRequests))
	}
	if out.ReviewItems > 0 {
		out.Recommendations = append(out.Recommendations, fmt.Sprintf("Review %d held upload or draft item(s) before they become silent backlog.", out.ReviewItems))
	}
	if out.ReadyDrafts > 0 {
		out.Recommendations = append(out.Recommendations, fmt.Sprintf("Publish or feature %d ready draft(s) so the file area feels alive.", out.ReadyDrafts))
	}
	if out.FeaturedCollections == 0 {
		out.Recommendations = append(out.Recommendations, "Create at least one featured collection so callers do not land on an empty curator shelf.")
	}
	if out.RepairIssues > 0 {
		out.Recommendations = append(out.Recommendations, fmt.Sprintf("Resolve %d repair issue(s) before promoting the affected files.", out.RepairIssues))
	}
	if len(out.Recommendations) == 0 {
		out.Recommendations = append(out.Recommendations, "File workflow looks healthy. Keep requests moving into collections instead of letting drafts pile up.")
	}
	return out
}

func renderFileCuratorLane(summary fileWorkflowSummary) string {
	recoRows := strings.Builder{}
	for _, row := range summary.Recommendations {
		recoRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	return `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Curator Lane</h2><p>Treat FileBase like a living collection: intake, review, publish, and feature should be one visible loop.</p><div class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(summary.OpenRequests) + `</strong><span>open requests</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(summary.ReviewItems) + `</strong><span>review workload</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(summary.ReadyDrafts) + `</strong><span>ready to publish</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(summary.FeaturedCollections) + `</strong><span>featured shelves</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(summary.RepairIssues) + `</strong><span>repair risks</span></article></div></article><article class="wolfbbs-card"><h2>Next Curator Actions</h2><ul>` + recoRows.String() + `</ul></article></section>`
}

type eventCampaignSummary struct {
	UpcomingSeries  int
	UpcomingEvents  int
	MissingJoinPath int
	MissingRecaps   int
	LowAttendance   int
	Recommendations []string
}

func buildEventCampaignSummary(events []communityEvent, recaps map[string]eventRecap, attendance map[string]int, now time.Time) eventCampaignSummary {
	seriesSeen := map[string]struct{}{}
	out := eventCampaignSummary{}
	for _, row := range events {
		if row.StartsAt.After(now) {
			out.UpcomingEvents++
			if strings.TrimSpace(row.Link) == "" {
				out.MissingJoinPath++
			}
			if strings.TrimSpace(row.Recurrence) != "" {
				seriesID := strings.TrimSpace(row.SeriesID)
				if seriesID == "" {
					seriesID = row.ID
				}
				if _, ok := seriesSeen[seriesID]; !ok {
					seriesSeen[seriesID] = struct{}{}
					out.UpcomingSeries++
				}
			}
			continue
		}
		if row.EndsAt.IsZero() || row.EndsAt.Before(now.Add(-14*24*time.Hour)) {
			continue
		}
		if _, ok := recaps[row.ID]; !ok {
			out.MissingRecaps++
		}
		if attendance[row.ID] == 0 {
			out.LowAttendance++
		}
	}
	if out.UpcomingEvents == 0 {
		out.Recommendations = append(out.Recommendations, "Schedule at least one concrete event so callers have a reason to come back this week.")
	}
	if out.MissingJoinPath > 0 {
		out.Recommendations = append(out.Recommendations, fmt.Sprintf("Add direct join paths to %d upcoming event(s); calendar entries should not force callers to guess.", out.MissingJoinPath))
	}
	if out.MissingRecaps > 0 {
		out.Recommendations = append(out.Recommendations, fmt.Sprintf("Close the loop on %d recent event(s) with a recap so the calendar builds momentum instead of amnesia.", out.MissingRecaps))
	}
	if out.LowAttendance > 0 {
		out.Recommendations = append(out.Recommendations, fmt.Sprintf("%d recent event(s) logged no attendance. Rework the timing, host, or promotional path.", out.LowAttendance))
	}
	if len(out.Recommendations) == 0 {
		out.Recommendations = append(out.Recommendations, "Campaign cadence looks healthy. Keep recaps and series links current so the community loop stays visible.")
	}
	return out
}

func renderEventCampaignHealth(summary eventCampaignSummary) string {
	rows := strings.Builder{}
	for _, row := range summary.Recommendations {
		rows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	return `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Campaign Health</h2><p>Events should feel like campaigns with memory: repeatable cadence, easy join paths, and post-event follow-through.</p><div class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(summary.UpcomingSeries) + `</strong><span>active series</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(summary.UpcomingEvents) + `</strong><span>upcoming events</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(summary.MissingJoinPath) + `</strong><span>missing join paths</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(summary.MissingRecaps) + `</strong><span>missing recaps</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(summary.LowAttendance) + `</strong><span>low-attendance recents</span></article></div></article><article class="wolfbbs-card"><h2>Campaign Recommendations</h2><ul>` + rows.String() + `</ul></article></section>`
}

func renderOperatorConfidenceBlock(a *webApp, user *domain.User, readiness statusSnapshot, runtime statusSnapshot, errors []appErrorEntry) string {
	signals := a.collectOperatorSignals(7)
	manualRows := manualAcceptanceSummaryRows(filepath.Join("docs", "manual-acceptance-latest.md"))
	releaseRows := latestReleaseMetadata(3)
	backupRows := a.listBackupArtifacts(8)
	backupOK := 0
	backupWarn := 0
	for _, row := range backupRows {
		switch strings.ToLower(strings.TrimSpace(row.Status)) {
		case "ok", "pass":
			backupOK++
		default:
			backupWarn++
		}
	}
	manualList := strings.Builder{}
	for _, row := range manualRows {
		manualList.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	releaseList := strings.Builder{}
	for _, row := range releaseRows {
		releaseList.WriteString(`<li><code>` + htmlEscape(row.Name) + `</code> <span class="wolfbbs-muted">` + htmlEscape(row.Updated.Local().Format("2006-01-02 15:04")) + `</span></li>`)
	}
	if releaseList.Len() == 0 {
		releaseList.WriteString(`<li>No release notes found.</li>`)
	}
	backupTable := strings.Builder{}
	for _, row := range backupRows {
		backupTable.WriteString(`<tr><td><code>` + htmlEscape(filepath.Base(row.Path)) + `</code></td><td>` + htmlEscape(row.Kind) + `</td><td><span class="status-` + backupStatusClass(row.Status) + `">` + htmlEscape(row.Status) + `</span></td><td>` + htmlEscape(cleanOneLiner(row.Detail, 96)) + `</td></tr>`)
	}
	if backupTable.Len() == 0 {
		backupTable.WriteString(`<tr><td colspan="4">No backup or offline artifacts detected.</td></tr>`)
	}
	recommendations := buildOperatorSignalRecommendations(signals)
	if len(errors) > 0 {
		recommendations = append([]string{fmt.Sprintf("%d runtime error(s) are still present. Treat green health indicators as provisional until those are explained.", len(errors))}, recommendations...)
	}
	if readiness.Summary.Warn > 0 || runtime.Summary.Warn > 0 {
		recommendations = append(recommendations, "Launch and runtime warnings still exist. Use this block as the operator confidence answer before calling the board ready.")
	}
	recoRows := strings.Builder{}
	for _, row := range recommendations {
		recoRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	return `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Operator Confidence</h2><div class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.FirstCallCompletes) + `</strong><span>first-call proofs (7d)</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.ActiveBoards) + `</strong><span>active boards (7d)</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.ActiveChannels) + `</strong><span>active channels (7d)</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(backupOK) + `</strong><span>healthy artifacts</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(backupWarn) + `</strong><span>artifact warnings</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(readiness.Summary.Pass) + `/` + strconv.Itoa(readiness.Summary.Total) + `</strong><span>launch readiness</span></article></div><h3>Next Confidence Actions</h3><ul>` + recoRows.String() + `</ul></article><article class="wolfbbs-card"><h2>Proof Pack</h2><h3>Latest Acceptance Summary</h3><ul>` + manualList.String() + `</ul><h3>Recent Release Notes</h3><ul>` + releaseList.String() + `</ul></article></section>` +
		`<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Artifact Health</h2><table border="1"><tr><th>Artifact</th><th>Kind</th><th>Status</th><th>Detail</th></tr>` + backupTable.String() + `</table></article></section>`
}
