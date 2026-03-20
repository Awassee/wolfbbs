package main

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type releaseArtifactRow struct {
	Name    string
	Path    string
	Updated time.Time
}

func roadmapTrancheRows(path string, start, end int) []string {
	file, err := os.Open(path)
	if err != nil {
		return []string{"Roadmap file unavailable: " + path}
	}
	defer file.Close()

	lines := make([]string, 0, (end-start+1)*2)
	scanner := bufio.NewScanner(file)
	var collectStatus bool
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		idx := parseRoadmapIndex(line)
		if idx >= start && idx <= end {
			lines = append(lines, line)
			collectStatus = true
			continue
		}
		if collectStatus && strings.HasPrefix(strings.ToLower(line), "status:") {
			lines = append(lines, line)
			collectStatus = false
		}
	}
	if len(lines) == 0 {
		return []string{"No roadmap entries found for items " + strconv.Itoa(start) + "-" + strconv.Itoa(end) + "."}
	}
	return lines
}

func parseRoadmapIndex(line string) int {
	line = strings.TrimSpace(line)
	if line == "" {
		return 0
	}
	parts := strings.SplitN(line, ".", 2)
	if len(parts) != 2 {
		return 0
	}
	return parseInt(strings.TrimSpace(parts[0]), 0)
}

func manualAcceptanceSummaryRows(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return []string{"Manual acceptance report unavailable: " + path}
	}
	defer file.Close()

	out := make([]string, 0, 8)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "- Date:") || strings.HasPrefix(line, "- Mode:") || strings.HasPrefix(line, "- PASS:") || strings.HasPrefix(line, "- FAIL:") || strings.HasPrefix(line, "- SKIPPED:") {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		return []string{"Manual acceptance summary not found in report."}
	}
	return out
}

func releaseArtifacts(limit int) []releaseArtifactRow {
	root := filepath.Join("docs", "releases")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	out := make([]releaseArtifactRow, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, releaseArtifactRow{
			Name:    entry.Name(),
			Path:    path,
			Updated: info.ModTime().UTC(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Updated.Equal(out[j].Updated) {
			return out[i].Name > out[j].Name
		}
		return out[i].Updated.After(out[j].Updated)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (a *webApp) handleAdminReleaseDashboard(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method == http.MethodPost {
		if !a.requireAdminWrite(w, r) {
			return
		}
		if err := a.handleReleaseChecklistUpdate(r, user); err != nil {
			redirectWithError(w, r, "/admin/release", err.Error())
			return
		}
		redirectWithNotice(w, r, "/admin/release", "Release checklist updated.")
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	trancheRows := roadmapTrancheRows(filepath.Join("docs", "ROADMAP_101_150.md"), 146, 150)
	manualRows := manualAcceptanceSummaryRows(filepath.Join("docs", "manual-acceptance-latest.md"))
	releaseRows := latestReleaseMetadata(10)
	packageRows := packageArtifactRows(20)
	checklist := a.loadReleaseChecklistState()

	trancheList := strings.Builder{}
	for _, row := range trancheRows {
		trancheList.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	manualList := strings.Builder{}
	for _, row := range manualRows {
		manualList.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	releaseTable := strings.Builder{}
	for _, row := range releaseRows {
		releaseTable.WriteString(`<tr><td><code>` + htmlEscape(row.Name) + `</code></td><td><code>` + htmlEscape(row.Path) + `</code></td><td>` + htmlEscape(row.Updated.Local().Format("2006-01-02 15:04")) + `</td></tr>`)
	}
	if releaseTable.Len() == 0 {
		releaseTable.WriteString(`<tr><td colspan="3">No release notes found under docs/releases.</td></tr>`)
	}
	packageTable := strings.Builder{}
	for _, row := range packageRows {
		packageTable.WriteString(`<tr><td><code>` + htmlEscape(row.Name) + `</code></td><td><code>` + htmlEscape(row.Path) + `</code></td><td>` + htmlEscape(row.Updated.Local().Format("2006-01-02 15:04")) + `</td></tr>`)
	}
	if packageTable.Len() == 0 {
		packageTable.WriteString(`<tr><td colspan="3">No package artifacts found under dist/releases.</td></tr>`)
	}
	checkRows := strings.Builder{}
	total := 0
	done := 0
	for _, key := range releaseChecklistDefaults() {
		total++
		active := checklist.Checks[key]
		if active {
			done++
		}
		checkRows.WriteString(`<tr><td>` + htmlEscape(releaseChecklistLabel(key)) + `</td><td>` + boolToText(active) + `</td><td><form method="POST" action="/admin/release" style="display:inline">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="toggle_check"><input type="hidden" name="key" value="` + htmlEscape(key) + `"><input type="hidden" name="done" value="` + map[bool]string{true: "0", false: "1"}[active] + `"><button type="submit">` + map[bool]string{true: "Mark pending", false: "Mark done"}[active] + `</button></form></td></tr>`)
	}

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Release Dashboard</title></head><body>
<p><a href="/admin">admin</a> | <a href="/admin/launch">launch</a> | <a href="/admin/ops">ops</a> | <a href="/admin/release">release</a> | <a href="/admin/analytics">analytics</a> | <a href="/admin/plugins">plugins</a> | <a href="/admin/themes">themes</a> | <a href="/admin/webhooks">webhooks</a> | <a href="/admin/system">system</a> | <a href="/status">status</a> | <a href="/help">help</a></p>
` + pageMessageBlock(r) + `
<h1>Release Dashboard</h1>
<p>Roadmap 150: release cockpit linking roadmap, QA evidence, docs, artifact inspection, and checklist state.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(trancheRows)/2) + `</strong><span>roadmap items tracked (146-150)</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(releaseRows)) + `</strong><span>release note artifacts</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(packageRows)) + `</strong><span>package artifacts</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(done) + `/` + strconv.Itoa(total) + `</strong><span>release checklist</span></article></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Roadmap Tranche 146-150</h2><ul>` + trancheList.String() + `</ul><p><code>docs/ROADMAP_101_150.md</code></p></article><article class="wolfbbs-card"><h2>Latest Manual Acceptance</h2><ul>` + manualList.String() + `</ul><p><code>docs/manual-acceptance-latest.md</code></p></article></section>
<h2>Release Checklist</h2><form method="POST" action="/admin/release">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="clear_checks"><button type="submit">Clear Checklist</button></form><table border="1"><tr><th>Check</th><th>State</th><th>Action</th></tr>` + checkRows.String() + `</table>
<h2>Release Metadata</h2>
<table border="1"><tr><th>Tag File</th><th>Path</th><th>Updated</th></tr>` + releaseTable.String() + `</table>
<h2>Package Artifact Inspection</h2>
<table border="1"><tr><th>Name</th><th>Path</th><th>Updated</th></tr>` + packageTable.String() + `</table>
<h2>QA + Packaging Commands</h2>
<pre>go test ./...
scripts/verify.sh --fast
scripts/verify.sh --smoke
scripts/security-audit.sh
scripts/manual-acceptance.sh --auto --no-smoke --report docs/manual-acceptance-latest.md
scripts/package-dist.sh
scripts/release.sh --version vX.Y.Z --full-qa</pre>
<h2>Reference Artifacts</h2>
<ul><li><code>docs/function-registry.json</code></li><li><code>docs/UI_COVERAGE_MATRIX.md</code></li><li><code>docs/ACCEPTANCE_SPEC.md</code></li><li><code>docs/releases/</code></li><li><code>dist/releases/</code></li></ul>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}
