package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/ui"
)

const (
	sysSettingPluginManifests            = "extensions.plugins.manifests"
	sysSettingThemeMarketplaceBundles    = "themes.marketplace.bundles"
	sysSettingThemeMarketplaceActiveID   = "themes.marketplace.active.id"
	sysSettingThemeMarketplaceActiveFile = "themes.marketplace.active.file"
	sysSettingWebhookBridgeConfig        = "integrations.webhook.bridge.config"
	sysSettingWebhookBridgeDeliveries    = "integrations.webhook.bridge.deliveries"
	sysSettingReleaseChecklist146155     = "release.checklist.146_155"
	sysSettingDigestWeekdayPrefsRoot     = "web.digest_weekday."
	sysSettingSeasonMissions             = "community.season_missions"
	sysSettingSeasonMissionCompletions   = "community.season_mission_completions"
	maxWebhookDeliveryLogEntries         = 120
	maxThemeMarketplaceBundles           = 48
	maxSeasonMissions                    = 64
	themeMarketplaceDefaultSubdir        = "themes/marketplace"
	pluginStarterDefaultID               = "mod_sample"
)

var (
	pluginIDExpr         = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)
	themeBundleIDExpr    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)
	seasonMissionIDExpr  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)
	pluginCapabilityList = []string{
		"board.read",
		"board.write",
		"chat.read",
		"chat.write",
		"mail.read",
		"mail.write",
		"door.run",
		"events.publish",
		"http.outbound",
	}
)

var pluginCapabilitySet = func() map[string]struct{} {
	out := make(map[string]struct{}, len(pluginCapabilityList))
	for _, row := range pluginCapabilityList {
		out[row] = struct{}{}
	}
	return out
}()

type pluginManifest struct {
	ID             string    `json:"id"`
	Entrypoint     string    `json:"entrypoint"`
	Capabilities   []string  `json:"capabilities,omitempty"`
	RetentionDays  int       `json:"retention_days"`
	SandboxProfile string    `json:"sandbox_profile"`
	Enabled        bool      `json:"enabled"`
	UpdatedBy      string    `json:"updated_by,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type themeBundleManifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Author      string `json:"author,omitempty"`
	Description string `json:"description,omitempty"`
}

type themeBundleTheme struct {
	Name     string `json:"name"`
	StatusFg string `json:"status_fg,omitempty"`
	StatusBg string `json:"status_bg,omitempty"`
	BodyFg   string `json:"body_fg,omitempty"`
	AccentFg string `json:"accent_fg,omitempty"`
	WarnFg   string `json:"warn_fg,omitempty"`
	ErrorFg  string `json:"error_fg,omitempty"`
	MutedFg  string `json:"muted_fg,omitempty"`
}

type themeBundle struct {
	Manifest   themeBundleManifest `json:"manifest"`
	Themes     []themeBundleTheme  `json:"themes"`
	FilePath   string              `json:"file_path"`
	ImportedBy string              `json:"imported_by,omitempty"`
	ImportedAt time.Time           `json:"imported_at"`
}

type webhookBridgeConfig struct {
	Enabled        bool      `json:"enabled"`
	Endpoint       string    `json:"endpoint"`
	Token          string    `json:"token,omitempty"`
	Events         []string  `json:"events,omitempty"`
	RetryAttempts  int       `json:"retry_attempts"`
	BackoffMS      int       `json:"backoff_ms"`
	TimeoutSeconds int       `json:"timeout_seconds"`
	UpdatedBy      string    `json:"updated_by,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type webhookDeliveryLog struct {
	At         time.Time `json:"at"`
	Event      string    `json:"event"`
	Endpoint   string    `json:"endpoint"`
	Success    bool      `json:"success"`
	Attempts   int       `json:"attempts"`
	StatusCode int       `json:"status_code"`
	DurationMS int       `json:"duration_ms"`
	Error      string    `json:"error,omitempty"`
}

type analyticsWindow struct {
	Label                string
	Days                 int
	BoardPosts           int
	ChatMessages         int
	DoorRuns             int
	EventCheckins        int
	ActiveCallers        int
	ReturningCallers     int
	FirstCallCompletions int
	FeedbackItems        int
}

type callerActivity struct {
	Handle        string
	CurrentStreak int
	LongestStreak int
	ActiveDays14  int
	Board7        int
	Chat7         int
	Door7         int
	LastActiveAt  time.Time
}

type weekdayDigestPrefs struct {
	Sunday    int `json:"sunday"`
	Monday    int `json:"monday"`
	Tuesday   int `json:"tuesday"`
	Wednesday int `json:"wednesday"`
	Thursday  int `json:"thursday"`
	Friday    int `json:"friday"`
	Saturday  int `json:"saturday"`
}

type seasonMission struct {
	ID               string    `json:"id"`
	Season           string    `json:"season"`
	Title            string    `json:"title"`
	Description      string    `json:"description,omitempty"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
	TargetBoardPosts int       `json:"target_board_posts"`
	TargetChatPosts  int       `json:"target_chat_posts"`
	TargetDoorRuns   int       `json:"target_door_runs"`
	Active           bool      `json:"active"`
	UpdatedBy        string    `json:"updated_by,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type missionCompletion struct {
	MissionID string    `json:"mission_id"`
	Handle    string    `json:"handle"`
	ClaimedAt time.Time `json:"claimed_at"`
}

type missionProgress struct {
	BoardPosts int
	ChatPosts  int
	DoorRuns   int
}

type releaseChecklistState struct {
	Checks map[string]bool `json:"checks"`
}

func splitLowerCSV(raw string, limit int) []string {
	if limit <= 0 {
		limit = 64
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, limit)
	for _, part := range strings.Split(raw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func cleanRouteEventName(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	return raw
}

func normalizePluginManifest(row pluginManifest) (pluginManifest, []string) {
	errs := make([]string, 0, 8)
	row.ID = strings.ToLower(strings.TrimSpace(row.ID))
	row.Entrypoint = strings.TrimSpace(row.Entrypoint)
	row.SandboxProfile = strings.ToLower(strings.TrimSpace(row.SandboxProfile))
	row.UpdatedBy = normalizeHandleKey(row.UpdatedBy)
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	if !pluginIDExpr.MatchString(row.ID) {
		errs = append(errs, "id must be lowercase [a-z0-9_-], 2-64 chars")
	}
	if row.Entrypoint == "" {
		errs = append(errs, "entrypoint is required")
	}
	if row.RetentionDays <= 0 {
		row.RetentionDays = 30
	}
	if row.RetentionDays > 3650 {
		row.RetentionDays = 3650
	}
	switch row.SandboxProfile {
	case "strict", "network-limited", "filesystem-readonly", "none":
	default:
		errs = append(errs, "sandbox profile must be strict, network-limited, filesystem-readonly, or none")
	}
	caps := make([]string, 0, len(row.Capabilities))
	seen := map[string]struct{}{}
	for _, capName := range row.Capabilities {
		capName = strings.ToLower(strings.TrimSpace(capName))
		if capName == "" {
			continue
		}
		if _, ok := pluginCapabilitySet[capName]; !ok {
			errs = append(errs, "unsupported capability: "+capName)
			continue
		}
		if _, ok := seen[capName]; ok {
			continue
		}
		seen[capName] = struct{}{}
		caps = append(caps, capName)
	}
	if len(caps) == 0 {
		errs = append(errs, "at least one capability is required")
	}
	sort.Strings(caps)
	row.Capabilities = caps
	return row, errs
}

func (a *webApp) loadPluginManifests() []pluginManifest {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingPluginManifests)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := []pluginManifest{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("extensions.plugins", fmt.Errorf("decode plugin manifests: %w", err))
		return nil
	}
	out := make([]pluginManifest, 0, len(decoded))
	for _, row := range decoded {
		normalized, errs := normalizePluginManifest(row)
		if len(errs) > 0 {
			continue
		}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (a *webApp) persistPluginManifests(rows []pluginManifest) {
	if a.adminRepo == nil {
		return
	}
	clean := make([]pluginManifest, 0, len(rows))
	for _, row := range rows {
		normalized, errs := normalizePluginManifest(row)
		if len(errs) > 0 {
			continue
		}
		clean = append(clean, normalized)
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i].UpdatedAt.Equal(clean[j].UpdatedAt) {
			return clean[i].ID < clean[j].ID
		}
		return clean[i].UpdatedAt.After(clean[j].UpdatedAt)
	})
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			a.addAppError("extensions.plugins", fmt.Errorf("encode plugin manifests: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingPluginManifests, body)
}

func pluginCapabilityEnabled(row pluginManifest, capability string) bool {
	capability = strings.ToLower(strings.TrimSpace(capability))
	for _, c := range row.Capabilities {
		if strings.EqualFold(c, capability) {
			return true
		}
	}
	return false
}

func normalizePluginStarterID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if pluginIDExpr.MatchString(raw) {
		return raw
	}
	return pluginStarterDefaultID
}

type pluginStarterFile struct {
	Name string
	Body string
}

func pluginStarterArchive(id string) ([]byte, error) {
	id = normalizePluginStarterID(id)
	manifest := pluginManifest{
		ID:             id,
		Entrypoint:     "/app/plugins/" + id + "/entrypoint.sh",
		Capabilities:   []string{"board.read", "chat.read"},
		RetentionDays:  30,
		SandboxProfile: "strict",
		Enabled:        false,
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	files := []pluginStarterFile{
		{
			Name: id + "/manifest.json",
			Body: string(manifestJSON) + "\n",
		},
		{
			Name: id + "/entrypoint.sh",
			Body: `#!/usr/bin/env sh
set -eu

event="${1:-}"
payload="${2:-}"

case "$event" in
  message.posted)
    echo "[` + id + `] observed board event payload: $payload"
    ;;
  chat.post)
    echo "[` + id + `] observed chat event payload: $payload"
    ;;
  *)
    echo "[` + id + `] no-op for event: $event"
    ;;
esac
`,
		},
		{
			Name: id + "/event.sample.json",
			Body: `{
  "name": "message.posted",
  "fields": {
    "board_id": "1",
    "author": "sysop",
    "subject": "starter message"
  }
}
`,
		},
		{
			Name: id + "/README.md",
			Body: "# " + id + " starter\n\n" +
				"1. Update `manifest.json` capabilities and entrypoint path.\n" +
				"2. Implement your handler logic in `entrypoint.sh`.\n" +
				"3. Validate + save the manifest in WolfBBS at `/admin/plugins`.\n" +
				"4. Keep sandbox profile explicit and least-privilege.\n",
		},
	}
	buf := bytes.NewBuffer(nil)
	zw := zip.NewWriter(buf)
	for _, row := range files {
		row.Name = strings.TrimSpace(row.Name)
		if row.Name == "" {
			continue
		}
		w, err := zw.Create(row.Name)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(w, row.Body); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (a *webApp) handleAdminPluginStarter(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := normalizePluginStarterID(r.URL.Query().Get("id"))
	payload, err := pluginStarterArchive(id)
	if err != nil {
		a.addAppError("extensions.plugins", fmt.Errorf("build starter archive: %w", err))
		http.Error(w, "starter archive unavailable", http.StatusInternalServerError)
		return
	}
	a.recordAdminAction(user.Handle, id, "download_plugin_starter", "")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="wolfbbs-plugin-`+id+`-starter.zip"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (a *webApp) handleAdminPlugins(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method == http.MethodPost {
		if !a.requireAdminWrite(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		switch action {
		case "save_plugin":
			candidate := pluginManifest{
				ID:             r.FormValue("id"),
				Entrypoint:     r.FormValue("entrypoint"),
				Capabilities:   splitLowerCSV(r.FormValue("capabilities"), 32),
				RetentionDays:  parseIntWithFallback(r.FormValue("retention_days"), 30),
				SandboxProfile: r.FormValue("sandbox_profile"),
				Enabled:        formHasValue(r, "enabled"),
				UpdatedBy:      user.Handle,
				UpdatedAt:      time.Now().UTC(),
			}
			if raw := strings.TrimSpace(r.FormValue("manifest_json")); raw != "" {
				if err := json.Unmarshal([]byte(raw), &candidate); err != nil {
					redirectWithError(w, r, "/admin/plugins", "Manifest JSON is invalid: "+err.Error())
					return
				}
				candidate.UpdatedBy = user.Handle
				candidate.UpdatedAt = time.Now().UTC()
			}
			normalized, errs := normalizePluginManifest(candidate)
			if len(errs) > 0 {
				redirectWithError(w, r, "/admin/plugins", "Manifest rejected: "+strings.Join(errs, "; "))
				return
			}
			rows := a.loadPluginManifests()
			updated := false
			for i := range rows {
				if rows[i].ID == normalized.ID {
					rows[i] = normalized
					updated = true
					break
				}
			}
			if !updated {
				rows = append(rows, normalized)
			}
			a.persistPluginManifests(rows)
			a.recordAdminAction(user.Handle, normalized.ID, "save_plugin_manifest", "capabilities="+strings.Join(normalized.Capabilities, ","))
			redirectWithNotice(w, r, "/admin/plugins", "Plugin manifest saved.")
			return
		case "delete_plugin":
			id := strings.ToLower(strings.TrimSpace(r.FormValue("id")))
			if id == "" {
				redirectWithError(w, r, "/admin/plugins", "Plugin id is required.")
				return
			}
			rows := a.loadPluginManifests()
			next := make([]pluginManifest, 0, len(rows))
			deleted := false
			for _, row := range rows {
				if row.ID == id {
					deleted = true
					continue
				}
				next = append(next, row)
			}
			if !deleted {
				redirectWithError(w, r, "/admin/plugins", "Plugin manifest not found.")
				return
			}
			a.persistPluginManifests(next)
			a.recordAdminAction(user.Handle, id, "delete_plugin_manifest", "")
			redirectWithNotice(w, r, "/admin/plugins", "Plugin manifest deleted.")
			return
		default:
			redirectWithError(w, r, "/admin/plugins", "Unsupported plugin action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rows := a.loadPluginManifests()
	manifestRows := strings.Builder{}
	for _, row := range rows {
		manifestRows.WriteString(`<tr><td><code>` + htmlEscape(row.ID) + `</code></td><td><code>` + htmlEscape(row.Entrypoint) + `</code></td><td>` + htmlEscape(strings.Join(row.Capabilities, ", ")) + `</td><td>` + htmlEscape(row.SandboxProfile) + `</td><td>` + strconv.Itoa(row.RetentionDays) + `</td><td>` + boolToText(row.Enabled) + `</td><td><form method="POST" action="/admin/plugins" style="display:inline">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="delete_plugin"><input type="hidden" name="id" value="` + htmlEscape(row.ID) + `"><button type="submit">Delete</button></form></td></tr>`)
	}
	if manifestRows.Len() == 0 {
		manifestRows.WriteString(`<tr><td colspan="7">No plugin manifests saved yet.</td></tr>`)
	}
	matrixRows := strings.Builder{}
	for _, capName := range pluginCapabilityList {
		matrixRows.WriteString(`<tr><td><code>` + htmlEscape(capName) + `</code></td>`)
		for _, row := range rows {
			state := "-"
			if pluginCapabilityEnabled(row, capName) {
				state = "Y"
			}
			matrixRows.WriteString(`<td>` + state + `</td>`)
		}
		matrixRows.WriteString(`</tr>`)
	}
	if matrixRows.Len() == 0 {
		matrixRows.WriteString(`<tr><td>No capabilities to render.</td></tr>`)
	}
	matrixHeader := `<tr><th>Capability</th>`
	for _, row := range rows {
		matrixHeader += `<th>` + htmlEscape(row.ID) + `</th>`
	}
	matrixHeader += `</tr>`

	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Plugin Contracts</title></head><body>
<p><a href="/admin">admin</a> | <a href="/admin/plugins/starter">starter pack</a> | <a href="/admin/themes">themes</a> | <a href="/admin/webhooks">webhooks</a> | <a href="/admin/analytics">analytics</a> | <a href="/admin/release">release</a></p>
` + pageMessageBlock(r) + `
<h1>Plugin Manifest Contracts</h1>
<p>Roadmap 146: manifest schema + capability + sandbox contract with explicit validation errors and auditable capability matrix.</p>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Save Manifest</h2><form method="POST" action="/admin/plugins">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="save_plugin"><label>ID <input name="id" size="26" placeholder="mod_sync"></label><br><label>Entrypoint <input name="entrypoint" size="54" placeholder="/app/plugins/mod-sync"></label><br><label>Capabilities (comma list)<input name="capabilities" size="82" placeholder="board.read,board.write,http.outbound"></label><br><label>Sandbox profile <select name="sandbox_profile"><option value="strict">strict</option><option value="network-limited">network-limited</option><option value="filesystem-readonly">filesystem-readonly</option><option value="none">none</option></select></label><br><label>Retention days <input name="retention_days" value="30" inputmode="numeric"></label><br><label><input type="checkbox" name="enabled" value="1" checked> Enabled</label><br><label>Manifest JSON (optional override)<br><textarea name="manifest_json" rows="8" cols="96" placeholder='{"id":"mod_sync","entrypoint":"/app/plugins/mod-sync","capabilities":["board.read","board.write"],"retention_days":30,"sandbox_profile":"strict","enabled":true}'></textarea></label><br><button type="submit">Validate + Save</button></form></article><article class="wolfbbs-card"><h2>Validation Contract</h2><ul><li>ID must be lowercase, safe, and stable.</li><li>Entrypoint is required for operator auditability.</li><li>Sandbox profile must be explicit.</li><li>Capabilities must be from the approved list.</li><li>Retention days are bounded and persisted.</li></ul></article><article class="wolfbbs-card"><h2>Starter SDK Pack</h2><p>Generate a ready-to-edit manifest + shell entrypoint bundle, then register it here.</p><form method="GET" action="/admin/plugins/starter"><label>Plugin ID <input name="id" value="` + pluginStarterDefaultID + `" size="24"></label><button type="submit">Download Starter ZIP</button></form><p>Reference: <code>docs/EXTENSION_SDK.md</code></p></article></section>
<h2>Saved Plugin Manifests</h2><table border="1"><tr><th>ID</th><th>Entrypoint</th><th>Capabilities</th><th>Sandbox</th><th>Retention</th><th>Enabled</th><th>Action</th></tr>` + manifestRows.String() + `</table>
<h2>Capability Matrix</h2><table border="1">` + matrixHeader + matrixRows.String() + `</table>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func sanitizeBundleID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if themeBundleIDExpr.MatchString(raw) {
		return raw
	}
	clean := strings.Builder{}
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			clean.WriteRune(r)
		case r >= '0' && r <= '9':
			clean.WriteRune(r)
		case r == '-' || r == '_':
			clean.WriteRune(r)
		}
	}
	out := clean.String()
	if !themeBundleIDExpr.MatchString(out) {
		return ""
	}
	return out
}

func normalizeThemeBundle(row themeBundle) (themeBundle, []string) {
	errs := make([]string, 0, 8)
	row.Manifest.ID = sanitizeBundleID(row.Manifest.ID)
	row.Manifest.Name = cleanOneLiner(row.Manifest.Name, 80)
	row.Manifest.Version = cleanOneLiner(row.Manifest.Version, 32)
	row.Manifest.Author = cleanOneLiner(row.Manifest.Author, 48)
	row.Manifest.Description = cleanOneLiner(row.Manifest.Description, 220)
	row.ImportedBy = normalizeHandleKey(row.ImportedBy)
	if row.ImportedAt.IsZero() {
		row.ImportedAt = time.Now().UTC()
	}
	if row.Manifest.ID == "" {
		errs = append(errs, "bundle manifest id is invalid")
	}
	if row.Manifest.Name == "" {
		errs = append(errs, "bundle manifest name is required")
	}
	if row.Manifest.Version == "" {
		errs = append(errs, "bundle manifest version is required")
	}
	cleanThemes := make([]themeBundleTheme, 0, len(row.Themes))
	seen := map[string]struct{}{}
	for _, th := range row.Themes {
		th.Name = strings.ToLower(strings.TrimSpace(th.Name))
		if th.Name == "" {
			continue
		}
		if _, ok := seen[th.Name]; ok {
			continue
		}
		seen[th.Name] = struct{}{}
		cleanThemes = append(cleanThemes, th)
	}
	if len(cleanThemes) == 0 {
		errs = append(errs, "bundle must include at least one theme")
	}
	row.Themes = cleanThemes
	row.FilePath = strings.TrimSpace(row.FilePath)
	return row, errs
}

func themeMarketplaceRoot() string {
	if custom := strings.TrimSpace(os.Getenv("WOLFBBS_THEME_BUNDLE_DIR")); custom != "" {
		return filepath.Clean(custom)
	}
	return filepath.Join(installPrefixPath(), themeMarketplaceDefaultSubdir)
}

func (a *webApp) loadThemeMarketplaceBundles() []themeBundle {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingThemeMarketplaceBundles)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := []themeBundle{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("themes.marketplace", fmt.Errorf("decode bundles: %w", err))
		return nil
	}
	out := make([]themeBundle, 0, len(decoded))
	for _, row := range decoded {
		normalized, errs := normalizeThemeBundle(row)
		if len(errs) > 0 {
			continue
		}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ImportedAt.Equal(out[j].ImportedAt) {
			return out[i].Manifest.ID < out[j].Manifest.ID
		}
		return out[i].ImportedAt.After(out[j].ImportedAt)
	})
	return out
}

func (a *webApp) persistThemeMarketplaceBundles(rows []themeBundle) {
	if a.adminRepo == nil {
		return
	}
	clean := make([]themeBundle, 0, len(rows))
	for _, row := range rows {
		normalized, errs := normalizeThemeBundle(row)
		if len(errs) > 0 {
			continue
		}
		clean = append(clean, normalized)
	}
	if len(clean) > maxThemeMarketplaceBundles {
		clean = clean[:maxThemeMarketplaceBundles]
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i].ImportedAt.Equal(clean[j].ImportedAt) {
			return clean[i].Manifest.ID < clean[j].Manifest.ID
		}
		return clean[i].ImportedAt.After(clean[j].ImportedAt)
	})
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			a.addAppError("themes.marketplace", fmt.Errorf("encode bundles: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingThemeMarketplaceBundles, body)
}

func (a *webApp) activeThemeMarketplaceBundleID() string {
	if a.adminRepo == nil {
		return ""
	}
	value, err := a.adminRepo.GetSystemSetting(sysSettingThemeMarketplaceActiveID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func (a *webApp) activeThemeMarketplaceFile() string {
	if a.adminRepo == nil {
		return ""
	}
	value, err := a.adminRepo.GetSystemSetting(sysSettingThemeMarketplaceActiveFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func themeBundleToThemeFile(row themeBundle) []byte {
	payload := map[string]interface{}{
		"themes": row.Themes,
	}
	raw, _ := json.MarshalIndent(payload, "", "  ")
	return raw
}

func saveThemeBundleThemeFile(row themeBundle) (string, error) {
	root := themeMarketplaceRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	id := sanitizeBundleID(row.Manifest.ID)
	if id == "" {
		return "", fmt.Errorf("bundle id is invalid")
	}
	version := strings.TrimSpace(strings.ReplaceAll(row.Manifest.Version, "/", "-"))
	if version == "" {
		version = "v0"
	}
	filename := id + "_" + version + ".hjson"
	path := filepath.Join(root, filename)
	if err := os.WriteFile(path, themeBundleToThemeFile(row), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (a *webApp) applyThemeBundleByID(id string) error {
	id = sanitizeBundleID(id)
	if id == "" {
		return fmt.Errorf("bundle id is required")
	}
	bundles := a.loadThemeMarketplaceBundles()
	for _, row := range bundles {
		if row.Manifest.ID != id {
			continue
		}
		if strings.TrimSpace(row.FilePath) == "" {
			return fmt.Errorf("bundle file path missing")
		}
		if _, err := ui.LoadThemeFile(row.FilePath); err != nil {
			return err
		}
		a.persistSystemSetting(sysSettingThemeMarketplaceActiveID, row.Manifest.ID)
		a.persistSystemSetting(sysSettingThemeMarketplaceActiveFile, row.FilePath)
		return nil
	}
	return fmt.Errorf("bundle not found")
}

func (a *webApp) applyPersistedThemeBundle(settings map[string]string) {
	if len(settings) == 0 {
		return
	}
	path := strings.TrimSpace(settings[sysSettingThemeMarketplaceActiveFile])
	if path == "" {
		return
	}
	if _, err := ui.LoadThemeFile(path); err != nil {
		a.addAppError("themes.marketplace", fmt.Errorf("load active theme bundle %s: %w", path, err))
	}
}

func (a *webApp) handleAdminThemes(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method == http.MethodPost {
		if !a.requireAdminWrite(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		switch action {
		case "import_bundle":
			raw := strings.TrimSpace(r.FormValue("bundle_json"))
			if raw == "" {
				redirectWithError(w, r, "/admin/themes", "Theme bundle JSON is required.")
				return
			}
			row := themeBundle{}
			if err := json.Unmarshal([]byte(raw), &row); err != nil {
				redirectWithError(w, r, "/admin/themes", "Bundle JSON is invalid: "+err.Error())
				return
			}
			row.ImportedBy = user.Handle
			row.ImportedAt = time.Now().UTC()
			normalized, errs := normalizeThemeBundle(row)
			if len(errs) > 0 {
				redirectWithError(w, r, "/admin/themes", "Bundle rejected: "+strings.Join(errs, "; "))
				return
			}
			path, err := saveThemeBundleThemeFile(normalized)
			if err != nil {
				redirectWithError(w, r, "/admin/themes", "Could not save bundle file: "+err.Error())
				return
			}
			normalized.FilePath = path
			if _, err := ui.LoadThemeFile(path); err != nil {
				redirectWithError(w, r, "/admin/themes", "Bundle failed validation during load: "+err.Error())
				return
			}
			rows := a.loadThemeMarketplaceBundles()
			updated := false
			for i := range rows {
				if rows[i].Manifest.ID == normalized.Manifest.ID {
					rows[i] = normalized
					updated = true
					break
				}
			}
			if !updated {
				rows = append(rows, normalized)
			}
			a.persistThemeMarketplaceBundles(rows)
			a.recordAdminAction(user.Handle, normalized.Manifest.ID, "import_theme_bundle", normalized.Manifest.Version)
			redirectWithNotice(w, r, "/admin/themes", "Theme bundle imported and validated.")
			return
		case "apply_bundle":
			id := strings.TrimSpace(r.FormValue("id"))
			if err := a.applyThemeBundleByID(id); err != nil {
				redirectWithError(w, r, "/admin/themes", "Apply failed: "+err.Error())
				return
			}
			a.recordAdminAction(user.Handle, id, "apply_theme_bundle", "")
			redirectWithNotice(w, r, "/admin/themes", "Theme bundle applied.")
			return
		case "delete_bundle":
			id := sanitizeBundleID(r.FormValue("id"))
			if id == "" {
				redirectWithError(w, r, "/admin/themes", "Bundle id is required.")
				return
			}
			rows := a.loadThemeMarketplaceBundles()
			next := make([]themeBundle, 0, len(rows))
			deletedFile := ""
			for _, row := range rows {
				if row.Manifest.ID == id {
					deletedFile = row.FilePath
					continue
				}
				next = append(next, row)
			}
			if len(next) == len(rows) {
				redirectWithError(w, r, "/admin/themes", "Bundle not found.")
				return
			}
			a.persistThemeMarketplaceBundles(next)
			if a.activeThemeMarketplaceBundleID() == id {
				a.persistSystemSetting(sysSettingThemeMarketplaceActiveID, "")
				a.persistSystemSetting(sysSettingThemeMarketplaceActiveFile, "")
			}
			if deletedFile != "" {
				_ = os.Remove(deletedFile)
			}
			a.recordAdminAction(user.Handle, id, "delete_theme_bundle", "")
			redirectWithNotice(w, r, "/admin/themes", "Theme bundle deleted.")
			return
		default:
			redirectWithError(w, r, "/admin/themes", "Unsupported theme action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rows := a.loadThemeMarketplaceBundles()
	activeID := a.activeThemeMarketplaceBundleID()
	activeFile := a.activeThemeMarketplaceFile()
	bundleRows := strings.Builder{}
	for _, row := range rows {
		active := ""
		if row.Manifest.ID == activeID {
			active = `<strong>active</strong>`
		}
		themeNames := make([]string, 0, len(row.Themes))
		for _, th := range row.Themes {
			themeNames = append(themeNames, th.Name)
		}
		sort.Strings(themeNames)
		bundleRows.WriteString(`<tr><td><code>` + htmlEscape(row.Manifest.ID) + `</code></td><td>` + htmlEscape(row.Manifest.Name) + `</td><td>` + htmlEscape(row.Manifest.Version) + `</td><td>` + htmlEscape(strings.Join(themeNames, ", ")) + `</td><td><code>` + htmlEscape(row.FilePath) + `</code></td><td>` + active + `</td><td><form method="POST" action="/admin/themes" style="display:inline">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="apply_bundle"><input type="hidden" name="id" value="` + htmlEscape(row.Manifest.ID) + `"><button type="submit">Apply</button></form> <form method="POST" action="/admin/themes" style="display:inline">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="delete_bundle"><input type="hidden" name="id" value="` + htmlEscape(row.Manifest.ID) + `"><button type="submit">Delete</button></form></td></tr>`)
	}
	if bundleRows.Len() == 0 {
		bundleRows.WriteString(`<tr><td colspan="7">No imported bundles yet.</td></tr>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Theme Marketplace</title></head><body>
<p><a href="/admin">admin</a> | <a href="/admin/plugins">plugins</a> | <a href="/admin/themes">themes</a> | <a href="/admin/webhooks">webhooks</a> | <a href="/admin/release">release</a></p>
` + pageMessageBlock(r) + `
<h1>Theme Marketplace / Import</h1>
<p>Roadmap 147: importable bundle format, safe extraction path, and preview/apply workflow.</p>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Import Bundle</h2><form method="POST" action="/admin/themes">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="import_bundle"><textarea name="bundle_json" rows="14" cols="104" placeholder='{"manifest":{"id":"retro_night","name":"Retro Night","version":"1.0.0","author":"sysop"},"themes":[{"name":"retro-night","status_fg":"fg-black","status_bg":"bg-gray","body_fg":"fg-yellow","accent_fg":"fg-orange","warn_fg":"fg-red","error_fg":"fg-magenta","muted_fg":"fg-green"}]}'></textarea><br><button type="submit">Import + Validate</button></form></article><article class="wolfbbs-card"><h2>Runtime State</h2><ul><li>Active bundle id: <code>` + htmlEscape(defaultIfBlank(activeID, "(none)")) + `</code></li><li>Active bundle file: <code>` + htmlEscape(defaultIfBlank(activeFile, "(none)")) + `</code></li><li>Theme root: <code>` + htmlEscape(themeMarketplaceRoot()) + `</code></li><li>Loaded theme names: ` + htmlEscape(strings.Join(ui.ThemeNames(), ", ")) + `</li></ul></article></section>
<h2>Imported Bundles</h2><table border="1"><tr><th>ID</th><th>Name</th><th>Version</th><th>Themes</th><th>Extracted file</th><th>Status</th><th>Actions</th></tr>` + bundleRows.String() + `</table>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func normalizeWebhookBridgeConfig(row webhookBridgeConfig) (webhookBridgeConfig, []string) {
	errs := make([]string, 0, 8)
	row.Endpoint = strings.TrimSpace(row.Endpoint)
	row.Token = strings.TrimSpace(row.Token)
	row.UpdatedBy = normalizeHandleKey(row.UpdatedBy)
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	if row.RetryAttempts <= 0 {
		row.RetryAttempts = 3
	}
	if row.RetryAttempts > 6 {
		row.RetryAttempts = 6
	}
	if row.BackoffMS <= 0 {
		row.BackoffMS = 300
	}
	if row.BackoffMS > 5000 {
		row.BackoffMS = 5000
	}
	if row.TimeoutSeconds <= 0 {
		row.TimeoutSeconds = 4
	}
	if row.TimeoutSeconds > 30 {
		row.TimeoutSeconds = 30
	}
	cleanEvents := make([]string, 0, len(row.Events))
	seen := map[string]struct{}{}
	for _, ev := range row.Events {
		ev = cleanRouteEventName(ev)
		if ev == "" {
			continue
		}
		if _, ok := seen[ev]; ok {
			continue
		}
		seen[ev] = struct{}{}
		cleanEvents = append(cleanEvents, ev)
	}
	sort.Strings(cleanEvents)
	row.Events = cleanEvents
	if row.Enabled {
		if row.Endpoint == "" {
			errs = append(errs, "endpoint is required when webhook bridge is enabled")
		} else {
			parsed, err := url.Parse(row.Endpoint)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || strings.TrimSpace(parsed.Host) == "" {
				errs = append(errs, "endpoint must be absolute http(s) URL")
			}
		}
	}
	return row, errs
}

func (a *webApp) loadWebhookBridgeConfig() webhookBridgeConfig {
	if a.adminRepo == nil {
		return webhookBridgeConfig{}
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingWebhookBridgeConfig)
	if err != nil || strings.TrimSpace(raw) == "" {
		cfg, _ := normalizeWebhookBridgeConfig(webhookBridgeConfig{})
		return cfg
	}
	cfg := webhookBridgeConfig{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		a.addAppError("webhook.bridge", fmt.Errorf("decode config: %w", err))
		cfg, _ = normalizeWebhookBridgeConfig(webhookBridgeConfig{})
		return cfg
	}
	normalized, _ := normalizeWebhookBridgeConfig(cfg)
	return normalized
}

func (a *webApp) persistWebhookBridgeConfig(row webhookBridgeConfig) {
	if a.adminRepo == nil {
		return
	}
	normalized, errs := normalizeWebhookBridgeConfig(row)
	if len(errs) > 0 {
		a.addAppError("webhook.bridge", fmt.Errorf("config rejected: %s", strings.Join(errs, "; ")))
		return
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		a.addAppError("webhook.bridge", fmt.Errorf("encode config: %w", err))
		return
	}
	a.persistSystemSetting(sysSettingWebhookBridgeConfig, string(raw))
}

func (a *webApp) loadWebhookDeliveryLogs() []webhookDeliveryLog {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingWebhookBridgeDeliveries)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	rows := []webhookDeliveryLog{}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("webhook.bridge", fmt.Errorf("decode delivery logs: %w", err))
		return nil
	}
	out := make([]webhookDeliveryLog, 0, len(rows))
	for _, row := range rows {
		if row.At.IsZero() {
			continue
		}
		row.Event = cleanRouteEventName(row.Event)
		row.Endpoint = strings.TrimSpace(row.Endpoint)
		row.Error = cleanOneLiner(row.Error, 260)
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].At.After(out[j].At)
	})
	if len(out) > maxWebhookDeliveryLogEntries {
		out = out[:maxWebhookDeliveryLogEntries]
	}
	return out
}

func (a *webApp) persistWebhookDeliveryLogs(rows []webhookDeliveryLog) {
	if a.adminRepo == nil {
		return
	}
	clean := make([]webhookDeliveryLog, 0, len(rows))
	for _, row := range rows {
		if row.At.IsZero() {
			continue
		}
		row.Event = cleanRouteEventName(row.Event)
		row.Endpoint = strings.TrimSpace(row.Endpoint)
		row.Error = cleanOneLiner(row.Error, 260)
		clean = append(clean, row)
	}
	sort.Slice(clean, func(i, j int) bool {
		return clean[i].At.After(clean[j].At)
	})
	if len(clean) > maxWebhookDeliveryLogEntries {
		clean = clean[:maxWebhookDeliveryLogEntries]
	}
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			a.addAppError("webhook.bridge", fmt.Errorf("encode delivery logs: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingWebhookBridgeDeliveries, body)
}

func (a *webApp) appendWebhookDeliveryLog(row webhookDeliveryLog) {
	rows := a.loadWebhookDeliveryLogs()
	rows = append([]webhookDeliveryLog{row}, rows...)
	a.persistWebhookDeliveryLogs(rows)
}

func webhookEventAllowed(cfg webhookBridgeConfig, eventName string) bool {
	if len(cfg.Events) == 0 {
		return true
	}
	eventName = cleanRouteEventName(eventName)
	for _, row := range cfg.Events {
		if strings.EqualFold(row, eventName) {
			return true
		}
	}
	return false
}

func (a *webApp) dispatchWebhookEvent(eventName string, fields map[string]string) {
	cfg := a.loadWebhookBridgeConfig()
	if !cfg.Enabled {
		return
	}
	eventName = cleanRouteEventName(eventName)
	if eventName == "" || !webhookEventAllowed(cfg, eventName) {
		return
	}
	payload := map[string]interface{}{
		"event": eventName,
		"site":  a.siteDisplayName(),
		"host":  a.siteHost(),
		"at":    time.Now().UTC().Format(time.RFC3339Nano),
		"data":  fields,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		a.addAppError("webhook.bridge", fmt.Errorf("encode payload: %w", err))
		return
	}
	attempts := cfg.RetryAttempts
	if attempts <= 0 {
		attempts = 1
	}
	start := time.Now()
	statusCode := 0
	lastErr := ""
	success := false
	for i := 0; i < attempts; i++ {
		req, err := http.NewRequest(http.MethodPost, cfg.Endpoint, bytes.NewReader(raw))
		if err != nil {
			lastErr = err.Error()
			break
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-WolfBBS-Event", eventName)
		if cfg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
			req.Header.Set("X-WolfBBS-Token", cfg.Token)
		}
		client := &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err.Error()
		} else {
			statusCode = resp.StatusCode
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
				success = true
				break
			}
			lastErr = fmt.Sprintf("http %d", resp.StatusCode)
		}
		if i+1 < attempts {
			time.Sleep(time.Duration(cfg.BackoffMS*(i+1)) * time.Millisecond)
		}
	}
	logRow := webhookDeliveryLog{
		At:         time.Now().UTC(),
		Event:      eventName,
		Endpoint:   cfg.Endpoint,
		Success:    success,
		Attempts:   attempts,
		StatusCode: statusCode,
		DurationMS: int(time.Since(start).Milliseconds()),
		Error:      lastErr,
	}
	a.appendWebhookDeliveryLog(logRow)
	if !success && lastErr != "" {
		a.addAppError("webhook.bridge", fmt.Errorf("deliver %s: %s", eventName, lastErr))
	}
}

func (a *webApp) handleAdminWebhooks(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method == http.MethodPost {
		if !a.requireAdminWrite(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		switch action {
		case "save_webhook":
			cfg := webhookBridgeConfig{
				Enabled:        formHasValue(r, "enabled"),
				Endpoint:       r.FormValue("endpoint"),
				Token:          r.FormValue("token"),
				Events:         splitLowerCSV(r.FormValue("events"), 32),
				RetryAttempts:  parseIntWithFallback(r.FormValue("retry_attempts"), 3),
				BackoffMS:      parseIntWithFallback(r.FormValue("backoff_ms"), 300),
				TimeoutSeconds: parseIntWithFallback(r.FormValue("timeout_seconds"), 4),
				UpdatedBy:      user.Handle,
				UpdatedAt:      time.Now().UTC(),
			}
			normalized, errs := normalizeWebhookBridgeConfig(cfg)
			if len(errs) > 0 {
				redirectWithError(w, r, "/admin/webhooks", "Webhook config rejected: "+strings.Join(errs, "; "))
				return
			}
			a.persistWebhookBridgeConfig(normalized)
			a.recordAdminAction(user.Handle, "webhook.bridge", "save_config", normalized.Endpoint)
			redirectWithNotice(w, r, "/admin/webhooks", "Webhook bridge settings saved.")
			return
		case "send_test":
			a.dispatchWebhookEvent("board.test", map[string]string{
				"board_id":   "0",
				"board":      "Test Board",
				"conference": "Test",
				"actor":      user.Handle,
				"subject":    "Webhook smoke message",
			})
			a.recordAdminAction(user.Handle, "webhook.bridge", "send_test", "board.test")
			redirectWithNotice(w, r, "/admin/webhooks", "Webhook test event dispatched.")
			return
		case "clear_logs":
			a.persistWebhookDeliveryLogs(nil)
			a.recordAdminAction(user.Handle, "webhook.bridge", "clear_logs", "")
			redirectWithNotice(w, r, "/admin/webhooks", "Webhook delivery logs cleared.")
			return
		default:
			redirectWithError(w, r, "/admin/webhooks", "Unsupported webhook action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	cfg := a.loadWebhookBridgeConfig()
	logs := a.loadWebhookDeliveryLogs()
	logRows := strings.Builder{}
	for _, row := range logs {
		status := "ok"
		if !row.Success {
			status = "failed"
		}
		logRows.WriteString(`<tr><td>` + htmlEscape(row.At.Local().Format("2006-01-02 15:04:05")) + `</td><td><code>` + htmlEscape(row.Event) + `</code></td><td>` + htmlEscape(status) + `</td><td>` + strconv.Itoa(row.Attempts) + `</td><td>` + strconv.Itoa(row.StatusCode) + `</td><td>` + strconv.Itoa(row.DurationMS) + `ms</td><td>` + htmlEscape(defaultIfBlank(row.Error, "-")) + `</td></tr>`)
	}
	if logRows.Len() == 0 {
		logRows.WriteString(`<tr><td colspan="7">No webhook deliveries logged yet.</td></tr>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Webhook Bridge</title></head><body>
<p><a href="/admin">admin</a> | <a href="/admin/plugins">plugins</a> | <a href="/admin/themes">themes</a> | <a href="/admin/webhooks">webhooks</a> | <a href="/admin/release">release</a></p>
` + pageMessageBlock(r) + `
<h1>External Webhook Bridge</h1>
<p>Roadmap 148: board event schema + retry/backoff + delivery audit trail.</p>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Bridge Config</h2><form method="POST" action="/admin/webhooks">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="save_webhook"><label><input type="checkbox" name="enabled" value="1" ` + checkedAttr(cfg.Enabled) + `> Enable bridge</label><br><label>Endpoint <input name="endpoint" size="72" value="` + htmlEscape(cfg.Endpoint) + `" placeholder="https://hooks.example.org/wolfbbs"></label><br><label>Token <input name="token" size="48" value="` + htmlEscape(cfg.Token) + `" placeholder="shared token"></label><br><label>Events (comma list)<input name="events" size="72" value="` + htmlEscape(strings.Join(cfg.Events, ",")) + `" placeholder="board.created,board.updated,board.deleted,message.posted"></label><br><label>Retry attempts <input name="retry_attempts" value="` + strconv.Itoa(cfg.RetryAttempts) + `" inputmode="numeric"></label><br><label>Backoff ms <input name="backoff_ms" value="` + strconv.Itoa(cfg.BackoffMS) + `" inputmode="numeric"></label><br><label>Timeout sec <input name="timeout_seconds" value="` + strconv.Itoa(cfg.TimeoutSeconds) + `" inputmode="numeric"></label><br><button type="submit">Save Bridge</button></form><form method="POST" action="/admin/webhooks" style="margin-top:0.8rem">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="send_test"><button type="submit">Send Test Event</button></form></article><article class="wolfbbs-card"><h2>Event Schema</h2><pre>{
  "event": "board.created",
  "site": "WolfBBS",
  "host": "bbs.example.org",
  "at": "2026-03-18T20:30:00Z",
  "data": {
    "board_id": "42",
    "board": "General",
    "conference": "Public",
    "actor": "sysop",
    "message_id": "99",
    "subject": "Welcome"
  }
}</pre></article></section>
<h2>Delivery Log</h2><form method="POST" action="/admin/webhooks">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="clear_logs"><button type="submit">Clear Delivery Log</button></form><table border="1"><tr><th>At</th><th>Event</th><th>Status</th><th>Attempts</th><th>HTTP</th><th>Latency</th><th>Error</th></tr>` + logRows.String() + `</table>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) collectWindowActivity(windowDays int) analyticsWindow {
	if windowDays <= 0 {
		windowDays = 1
	}
	now := time.Now().UTC()
	since := now.Add(-time.Duration(windowDays) * 24 * time.Hour)
	active := map[string]map[string]struct{}{}
	ensureDay := func(handle, day string) {
		handle = normalizeHandleKey(handle)
		day = strings.TrimSpace(day)
		if handle == "" || day == "" {
			return
		}
		if active[handle] == nil {
			active[handle] = map[string]struct{}{}
		}
		active[handle][day] = struct{}{}
	}
	idToHandle := map[int64]string{}
	if a.authSvc != nil {
		if users, err := a.authSvc.ListUsers(); err == nil {
			for _, row := range users {
				idToHandle[row.ID] = row.Handle
			}
		}
	}
	out := analyticsWindow{Days: windowDays}
	switch windowDays {
	case 1:
		out.Label = "Daily"
	case 7:
		out.Label = "Weekly"
	case 30:
		out.Label = "Monthly"
	default:
		out.Label = strconv.Itoa(windowDays) + "-day"
	}
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
					out.BoardPosts++
					handle := idToHandle[msg.AuthorID]
					ensureDay(handle, msg.CreatedAt.UTC().Format("2006-01-02"))
				}
			}
		}
	}
	if a.chatSvc != nil {
		for _, channel := range a.chatSvc.ListChannels() {
			for _, msg := range a.chatSvc.History(channel, 5000) {
				if msg.CreatedAt.Before(since) {
					continue
				}
				out.ChatMessages++
				ensureDay(msg.From, msg.CreatedAt.UTC().Format("2006-01-02"))
			}
		}
	}
	if a.doorRegistry != nil {
		for _, door := range a.doorRegistry.Doors() {
			rows, err := a.doorRegistry.ListEvents(door.ID, 0, 5000)
			if err != nil {
				continue
			}
			for _, ev := range rows {
				if ev.CreatedAt.Before(since) {
					continue
				}
				switch strings.ToLower(strings.TrimSpace(ev.EventType)) {
				case "start", "connector_launch", "poll_vote", "poll_create":
				default:
					continue
				}
				out.DoorRuns++
				handle := idToHandle[ev.UserID]
				ensureDay(handle, ev.CreatedAt.UTC().Format("2006-01-02"))
			}
		}
	}
	for _, rows := range a.loadEventAttendance() {
		for handle, at := range rows {
			if at.Before(since) {
				continue
			}
			out.EventCheckins++
			ensureDay(handle, at.UTC().Format("2006-01-02"))
		}
	}
	for _, row := range a.loadOperatorInsightEvents() {
		if row.At.Before(since) {
			continue
		}
		switch row.Kind {
		case "first_call.complete":
			out.FirstCallCompletions++
			ensureDay(row.Handle, row.At.UTC().Format("2006-01-02"))
		case "feedback.sent":
			out.FeedbackItems++
			ensureDay(row.Handle, row.At.UTC().Format("2006-01-02"))
		}
	}
	out.ActiveCallers = len(active)
	for _, days := range active {
		if len(days) >= 2 {
			out.ReturningCallers++
		}
	}
	return out
}

func buildAnalyticsRecommendations(daily, weekly, monthly analyticsWindow) []string {
	out := make([]string, 0, 6)
	if monthly.ActiveCallers == 0 {
		out = append(out, "No active callers detected in the last 30 days. Focus first on onboarding and launch messaging.")
		return out
	}
	if daily.ActiveCallers < maxInt(1, monthly.ActiveCallers/8) {
		out = append(out, "Daily active callers are low relative to monthly participation. Promote /today, /events, and /challenges in your login flow.")
	}
	if weekly.ReturningCallers < maxInt(1, weekly.ActiveCallers/3) {
		out = append(out, "Weekly return rate is weak. Use streaks, missions, and spotlight routes to create return loops.")
	}
	if weekly.DoorRuns < weekly.BoardPosts/2 {
		out = append(out, "Door activity is trailing board posting. Feature door ladders and direct links from /today and /next.")
	}
	if weekly.EventCheckins == 0 {
		out = append(out, "No event check-ins recorded this week. Promote recurring events and make check-in visible in announcements.")
	}
	if weekly.FirstCallCompletions == 0 {
		out = append(out, "No first-call completions landed this week. Run a real new-user path and tighten onboarding before you chase more traffic.")
	}
	if weekly.FeedbackItems == 0 {
		out = append(out, "No feedback reached the sysop inbox this week. Add a visible ask on /today or /showcase so callers know where to send friction notes.")
	}
	if len(out) == 0 {
		out = append(out, "Engagement profile is balanced. Keep cadence with weekly events, spotlights, and release notes.")
	}
	return out
}

func (a *webApp) handleAdminAnalytics(w http.ResponseWriter, r *http.Request) {
	_, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	daily := a.collectWindowActivity(1)
	weekly := a.collectWindowActivity(7)
	monthly := a.collectWindowActivity(30)
	signals := a.collectOperatorSignals(7)
	recommendations := buildAnalyticsRecommendations(daily, weekly, monthly)
	recommendations = append(recommendations, buildOperatorSignalRecommendations(signals)...)
	recoRows := strings.Builder{}
	for _, row := range recommendations {
		recoRows.WriteString(`<li>` + htmlEscape(row) + `</li>`)
	}
	windowRows := func(row analyticsWindow) string {
		return `<tr><td>` + htmlEscape(row.Label) + `</td><td>` + strconv.Itoa(row.ActiveCallers) + `</td><td>` + strconv.Itoa(row.ReturningCallers) + `</td><td>` + strconv.Itoa(row.BoardPosts) + `</td><td>` + strconv.Itoa(row.ChatMessages) + `</td><td>` + strconv.Itoa(row.DoorRuns) + `</td><td>` + strconv.Itoa(row.EventCheckins) + `</td><td>` + strconv.Itoa(row.FirstCallCompletions) + `</td><td>` + strconv.Itoa(row.FeedbackItems) + `</td></tr>`
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Product Analytics Summary</title></head><body>
<p><a href="/admin">admin</a> | <a href="/admin/analytics">analytics</a> | <a href="/admin/challenges">challenges</a> | <a href="/admin/missions">missions</a> | <a href="/admin/release">release</a></p>
` + pageMessageBlock(r) + `
<h1>Embedded Product Analytics</h1>
<p>Roadmap 149: bounded daily/weekly/monthly KPI summaries for boards/chat/doors/events plus onboarding and feedback loops, with no new PII exposure.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(monthly.ActiveCallers) + `</strong><span>monthly active callers</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(weekly.ReturningCallers) + `</strong><span>weekly returners</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(weekly.BoardPosts+weekly.ChatMessages+weekly.DoorRuns) + `</strong><span>weekly core actions</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(weekly.EventCheckins) + `</strong><span>weekly event check-ins</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(weekly.FirstCallCompletions) + `</strong><span>weekly first-call completes</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(weekly.FeedbackItems) + `</strong><span>weekly feedback items</span></article></section>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.ActiveBoards) + `</strong><span>active boards (7d)</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.ActiveChannels) + `</strong><span>active chat channels (7d)</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.SetupFailures) + `</strong><span>setup failures (7d)</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.UpgradeFailures) + `</strong><span>upgrade failures (7d)</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.UpgradeSuccesses) + `</strong><span>upgrade successes (7d)</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(signals.UserCreates) + `</strong><span>caller accounts created (7d)</span></article></section>
<table border="1"><tr><th>Window</th><th>Active callers</th><th>Returning callers</th><th>Board posts</th><th>Chat messages</th><th>Door runs</th><th>Event check-ins</th><th>First-call completes</th><th>Feedback items</th></tr>` + windowRows(daily) + windowRows(weekly) + windowRows(monthly) + `</table>
<h2>Operator Signals</h2><table border="1"><tr><th>Window</th><th>First-call completes</th><th>Active boards</th><th>Active channels</th><th>Setup actions</th><th>Setup failures</th><th>Caller accounts created</th><th>Upgrade successes</th><th>Upgrade failures</th></tr><tr><td>7-day</td><td>` + strconv.Itoa(signals.FirstCallCompletes) + `</td><td>` + strconv.Itoa(signals.ActiveBoards) + `</td><td>` + strconv.Itoa(signals.ActiveChannels) + `</td><td>` + strconv.Itoa(signals.SetupActions) + `</td><td>` + strconv.Itoa(signals.SetupFailures) + `</td><td>` + strconv.Itoa(signals.UserCreates) + `</td><td>` + strconv.Itoa(signals.UpgradeSuccesses) + `</td><td>` + strconv.Itoa(signals.UpgradeFailures) + `</td></tr></table>
<h2>Recommendations</h2><ul>` + recoRows.String() + `</ul>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func weekdayDigestSettingKey(handle string) string {
	handle = normalizeHandleKey(handle)
	if handle == "" {
		return ""
	}
	return sysSettingDigestWeekdayPrefsRoot + handle
}

func defaultWeekdayDigestPrefs(base int) weekdayDigestPrefs {
	base = clampInt(base, 4, 40)
	return weekdayDigestPrefs{
		Sunday:    base,
		Monday:    base,
		Tuesday:   base,
		Wednesday: base,
		Thursday:  base,
		Friday:    base,
		Saturday:  base,
	}
}

func normalizeWeekdayDigestPrefs(row weekdayDigestPrefs, fallback int) weekdayDigestPrefs {
	if fallback <= 0 {
		fallback = 12
	}
	def := defaultWeekdayDigestPrefs(fallback)
	norm := row
	if norm.Sunday <= 0 {
		norm.Sunday = def.Sunday
	}
	if norm.Monday <= 0 {
		norm.Monday = def.Monday
	}
	if norm.Tuesday <= 0 {
		norm.Tuesday = def.Tuesday
	}
	if norm.Wednesday <= 0 {
		norm.Wednesday = def.Wednesday
	}
	if norm.Thursday <= 0 {
		norm.Thursday = def.Thursday
	}
	if norm.Friday <= 0 {
		norm.Friday = def.Friday
	}
	if norm.Saturday <= 0 {
		norm.Saturday = def.Saturday
	}
	norm.Sunday = clampInt(norm.Sunday, 4, 40)
	norm.Monday = clampInt(norm.Monday, 4, 40)
	norm.Tuesday = clampInt(norm.Tuesday, 4, 40)
	norm.Wednesday = clampInt(norm.Wednesday, 4, 40)
	norm.Thursday = clampInt(norm.Thursday, 4, 40)
	norm.Friday = clampInt(norm.Friday, 4, 40)
	norm.Saturday = clampInt(norm.Saturday, 4, 40)
	return norm
}

func (a *webApp) loadWeekdayDigestPrefs(handle string, fallback int) weekdayDigestPrefs {
	key := weekdayDigestSettingKey(handle)
	if key == "" || a.adminRepo == nil {
		return defaultWeekdayDigestPrefs(fallback)
	}
	raw, err := a.adminRepo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return defaultWeekdayDigestPrefs(fallback)
	}
	decoded := weekdayDigestPrefs{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("digest.weekday", fmt.Errorf("decode digest weekday prefs: %w", err))
		return defaultWeekdayDigestPrefs(fallback)
	}
	return normalizeWeekdayDigestPrefs(decoded, fallback)
}

func (a *webApp) persistWeekdayDigestPrefs(handle string, row weekdayDigestPrefs, fallback int) {
	key := weekdayDigestSettingKey(handle)
	if key == "" || a.adminRepo == nil {
		return
	}
	normalized := normalizeWeekdayDigestPrefs(row, fallback)
	raw, err := json.Marshal(normalized)
	if err != nil {
		a.addAppError("digest.weekday", fmt.Errorf("encode digest weekday prefs: %w", err))
		return
	}
	a.persistSystemSetting(key, string(raw))
}

func (a *webApp) digestMaxItemsForDate(handle string, fallback int, now time.Time) int {
	prefs := a.loadWeekdayDigestPrefs(handle, fallback)
	switch now.Weekday() {
	case time.Sunday:
		return prefs.Sunday
	case time.Monday:
		return prefs.Monday
	case time.Tuesday:
		return prefs.Tuesday
	case time.Wednesday:
		return prefs.Wednesday
	case time.Thursday:
		return prefs.Thursday
	case time.Friday:
		return prefs.Friday
	case time.Saturday:
		return prefs.Saturday
	default:
		return clampInt(fallback, 4, 40)
	}
}

func (a *webApp) handleDigestPreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	basePref := normalizeDigestPreferences(a.loadDigestPreferences(user.Handle))
	if r.Method == http.MethodPost {
		if !a.requireCSRF(w, r) {
			return
		}
		row := weekdayDigestPrefs{
			Sunday:    parseIntWithFallback(r.FormValue("sunday"), basePref.MaxItems),
			Monday:    parseIntWithFallback(r.FormValue("monday"), basePref.MaxItems),
			Tuesday:   parseIntWithFallback(r.FormValue("tuesday"), basePref.MaxItems),
			Wednesday: parseIntWithFallback(r.FormValue("wednesday"), basePref.MaxItems),
			Thursday:  parseIntWithFallback(r.FormValue("thursday"), basePref.MaxItems),
			Friday:    parseIntWithFallback(r.FormValue("friday"), basePref.MaxItems),
			Saturday:  parseIntWithFallback(r.FormValue("saturday"), basePref.MaxItems),
		}
		a.persistWeekdayDigestPrefs(user.Handle, row, basePref.MaxItems)
		redirectWithNotice(w, r, "/digest/preferences", "Weekday digest preferences saved.")
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	prefs := a.loadWeekdayDigestPrefs(user.Handle, basePref.MaxItems)
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Digest Weekday Preferences</title></head><body>
<p><a href="/settings">settings</a> | <a href="/digest">digest</a> | <a href="/today">today</a> | <a href="/next">next</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Digest Preferences By Weekday</h1>
<p>Roadmap 155: choose weekday digest size so heavier days can carry larger summaries without changing global defaults.</p>
<form method="POST" action="/digest/preferences">` + a.csrfHiddenInput(r) + `<table border="1"><tr><th>Weekday</th><th>Max digest items</th></tr><tr><td>Sunday</td><td><input name="sunday" value="` + strconv.Itoa(prefs.Sunday) + `" inputmode="numeric"></td></tr><tr><td>Monday</td><td><input name="monday" value="` + strconv.Itoa(prefs.Monday) + `" inputmode="numeric"></td></tr><tr><td>Tuesday</td><td><input name="tuesday" value="` + strconv.Itoa(prefs.Tuesday) + `" inputmode="numeric"></td></tr><tr><td>Wednesday</td><td><input name="wednesday" value="` + strconv.Itoa(prefs.Wednesday) + `" inputmode="numeric"></td></tr><tr><td>Thursday</td><td><input name="thursday" value="` + strconv.Itoa(prefs.Thursday) + `" inputmode="numeric"></td></tr><tr><td>Friday</td><td><input name="friday" value="` + strconv.Itoa(prefs.Friday) + `" inputmode="numeric"></td></tr><tr><td>Saturday</td><td><input name="saturday" value="` + strconv.Itoa(prefs.Saturday) + `" inputmode="numeric"></td></tr></table><button type="submit">Save Weekday Preferences</button></form>
<p>Global fallback from /settings is <strong>` + strconv.Itoa(basePref.MaxItems) + `</strong> items.</p>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) callerActivities(days int) []callerActivity {
	if days <= 0 {
		days = 45
	}
	now := time.Now().UTC()
	start := now.Add(-time.Duration(days) * 24 * time.Hour)
	idToHandle := map[int64]string{}
	handleSet := map[string]struct{}{}
	if a.authSvc != nil {
		if users, err := a.authSvc.ListUsers(); err == nil {
			for _, row := range users {
				h := normalizeHandleKey(row.Handle)
				if h == "" {
					continue
				}
				idToHandle[row.ID] = h
				handleSet[h] = struct{}{}
			}
		}
	}
	type dayActivity struct {
		board bool
		chat  bool
		door  bool
	}
	byHandleDay := map[string]map[string]dayActivity{}
	setActivity := func(handle string, at time.Time, surface string) {
		handle = normalizeHandleKey(handle)
		if handle == "" || at.Before(start) {
			return
		}
		if byHandleDay[handle] == nil {
			byHandleDay[handle] = map[string]dayActivity{}
		}
		day := at.UTC().Format("2006-01-02")
		row := byHandleDay[handle][day]
		switch surface {
		case "board":
			row.board = true
		case "chat":
			row.chat = true
		case "door":
			row.door = true
		}
		byHandleDay[handle][day] = row
		handleSet[handle] = struct{}{}
	}
	if a.boardRepo != nil && a.msgRepo != nil {
		if boards, err := a.boardRepo.List(); err == nil {
			for _, board := range boards {
				msgs, err := a.msgRepo.ListByBoard(board.ID)
				if err != nil {
					continue
				}
				for _, msg := range msgs {
					setActivity(idToHandle[msg.AuthorID], msg.CreatedAt, "board")
				}
			}
		}
	}
	if a.chatSvc != nil {
		for _, channel := range a.chatSvc.ListChannels() {
			for _, msg := range a.chatSvc.History(channel, 7000) {
				setActivity(msg.From, msg.CreatedAt, "chat")
			}
		}
	}
	if a.doorRegistry != nil {
		for _, door := range a.doorRegistry.Doors() {
			events, err := a.doorRegistry.ListEvents(door.ID, 0, 7000)
			if err != nil {
				continue
			}
			for _, ev := range events {
				switch strings.ToLower(strings.TrimSpace(ev.EventType)) {
				case "start", "connector_launch", "poll_vote", "poll_create":
				default:
					continue
				}
				setActivity(idToHandle[ev.UserID], ev.CreatedAt, "door")
			}
		}
	}

	out := make([]callerActivity, 0, len(handleSet))
	for handle := range handleSet {
		daysMap := byHandleDay[handle]
		if len(daysMap) == 0 {
			continue
		}
		row := callerActivity{Handle: handle}
		for i := 0; i < 14; i++ {
			day := now.Add(-time.Duration(i) * 24 * time.Hour).Format("2006-01-02")
			if _, ok := daysMap[day]; ok {
				row.ActiveDays14++
			}
		}
		for i := 0; i < 7; i++ {
			day := now.Add(-time.Duration(i) * 24 * time.Hour).Format("2006-01-02")
			if activity, ok := daysMap[day]; ok {
				if activity.board {
					row.Board7++
				}
				if activity.chat {
					row.Chat7++
				}
				if activity.door {
					row.Door7++
				}
			}
		}
		for i := 0; i < days; i++ {
			day := now.Add(-time.Duration(i) * 24 * time.Hour).Format("2006-01-02")
			if _, ok := daysMap[day]; !ok {
				if i == 0 {
					row.CurrentStreak = 0
				}
				break
			}
			row.CurrentStreak++
		}
		best := 0
		run := 0
		for i := days - 1; i >= 0; i-- {
			day := now.Add(-time.Duration(i) * 24 * time.Hour).Format("2006-01-02")
			if _, ok := daysMap[day]; ok {
				run++
				if run > best {
					best = run
				}
			} else {
				run = 0
			}
		}
		row.LongestStreak = best
		for day := range daysMap {
			if at, err := time.Parse("2006-01-02", day); err == nil {
				if at.After(row.LastActiveAt) {
					row.LastActiveAt = at
				}
			}
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CurrentStreak != out[j].CurrentStreak {
			return out[i].CurrentStreak > out[j].CurrentStreak
		}
		if out[i].ActiveDays14 != out[j].ActiveDays14 {
			return out[i].ActiveDays14 > out[j].ActiveDays14
		}
		return out[i].Handle < out[j].Handle
	})
	return out
}

func displayHandle(handle string) string {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return "caller"
	}
	return handle
}

func (a *webApp) handleStreaks(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rows := a.callerActivities(60)
	me := callerActivity{Handle: normalizeHandleKey(user.Handle)}
	for _, row := range rows {
		if strings.EqualFold(row.Handle, user.Handle) {
			me = row
			break
		}
	}
	leaderRows := strings.Builder{}
	for idx, row := range rows {
		leaderRows.WriteString(`<tr><td>` + strconv.Itoa(idx+1) + `</td><td>` + htmlEscape(displayHandle(row.Handle)) + `</td><td>` + strconv.Itoa(row.CurrentStreak) + `</td><td>` + strconv.Itoa(row.LongestStreak) + `</td><td>` + strconv.Itoa(row.ActiveDays14) + `</td><td>` + strconv.Itoa(row.Board7) + ` / ` + strconv.Itoa(row.Chat7) + ` / ` + strconv.Itoa(row.Door7) + `</td></tr>`)
		if idx >= 19 {
			break
		}
	}
	if leaderRows.Len() == 0 {
		leaderRows.WriteString(`<tr><td colspan="6">No streak activity yet.</td></tr>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Daily Streaks</title></head><body>
<p><a href="/start">start</a> | <a href="/today">today</a> | <a href="/next">next</a> | <a href="/spotlights">spotlights</a> | <a href="/missions">missions</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Daily Streaks</h1>
<p>Roadmap 151: daily return streaks across boards, chat, and doors.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(me.CurrentStreak) + `</strong><span>your current streak</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(me.LongestStreak) + `</strong><span>your best streak</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(me.ActiveDays14) + `/14</strong><span>your active days</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(rows)) + `</strong><span>callers on board</span></article></section>
<table border="1"><tr><th>Rank</th><th>Caller</th><th>Current</th><th>Best</th><th>Active(14d)</th><th>7d surfaces (B/C/D)</th></tr>` + leaderRows.String() + `</table>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) unreadMailCountForUser(user *domain.User) int {
	if user == nil || a.mailRepo == nil {
		return 0
	}
	rows, err := a.mailRepo.ListInbox(user.ID, 200)
	if err != nil {
		return 0
	}
	count := 0
	for _, row := range rows {
		if row.ReadAt == nil {
			count++
		}
	}
	return count
}

func (a *webApp) myCallerActivity(handle string) callerActivity {
	rows := a.callerActivities(60)
	for _, row := range rows {
		if strings.EqualFold(row.Handle, handle) {
			return row
		}
	}
	return callerActivity{Handle: normalizeHandleKey(handle)}
}

func (a *webApp) handleNextActions(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	activity := a.myCallerActivity(user.Handle)
	now := time.Now().UTC()
	actions := make([]attentionActionRow, 0, 8)
	if unread := a.unreadMailCountForUser(user); unread > 0 {
		actions = append(actions, attentionActionRow{Label: "Clear unread mail", Href: "/mail", Meta: strconv.Itoa(unread) + " unread"})
	}
	if activity.Board7 == 0 {
		actions = append(actions, attentionActionRow{Label: "Post on a board", Href: "/boards", Meta: "No board activity in last 7 days"})
	}
	if activity.Chat7 == 0 {
		actions = append(actions, attentionActionRow{Label: "Check in to lobby chat", Href: "/chat", Meta: "No chat messages in last 7 days"})
	}
	if activity.Door7 == 0 {
		actions = append(actions, attentionActionRow{Label: "Run a door", Href: "/doors", Meta: "No door runs in last 7 days"})
	}
	if activity.CurrentStreak == 0 {
		actions = append(actions, attentionActionRow{Label: "Restart your streak", Href: "/streaks", Meta: "One action today restores momentum"})
	}
	upcoming := a.upcomingCommunityEvents(3, now)
	if len(upcoming) > 0 {
		actions = append(actions, attentionActionRow{Label: "RSVP/check-in upcoming event", Href: "/events", Meta: strconv.Itoa(len(upcoming)) + " events in queue"})
	}
	if len(actions) == 0 {
		actions = append(actions,
			attentionActionRow{Label: "Maintain your current streak", Href: "/streaks", Meta: "You are fully caught up"},
			attentionActionRow{Label: "Check missions for bonus progress", Href: "/missions", Meta: "Seasonal missions are active"},
		)
	}
	actionCards := strings.Builder{}
	for _, row := range actions {
		actionCards.WriteString(`<a class="wolfbbs-action-card" href="` + htmlEscape(row.Href) + `"><strong>` + htmlEscape(row.Label) + `</strong><span>` + htmlEscape(row.Meta) + `</span></a>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Next Best Actions</title></head><body>
<p><a href="/start">start</a> | <a href="/today">today</a> | <a href="/next">next</a> | <a href="/streaks">streaks</a> | <a href="/missions">missions</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Next Best Actions</h1>
<p>Roadmap 152: personalized actions that reduce time-to-value after login.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(activity.CurrentStreak) + `</strong><span>current streak</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(activity.ActiveDays14) + `/14</strong><span>active days</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(activity.Board7+activity.Chat7+activity.Door7) + `</strong><span>7-day engagement signals</span></article></section>
<section class="wolfbbs-action-grid">` + actionCards.String() + `</section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleSpotlights(w http.ResponseWriter, r *http.Request) {
	_, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rows := a.callerActivities(45)
	cards := strings.Builder{}
	for idx, row := range rows {
		reason := fmt.Sprintf("%d-day streak, %d active days in last 14", row.CurrentStreak, row.ActiveDays14)
		cards.WriteString(`<article class="wolfbbs-card"><h2>` + htmlEscape(displayHandle(row.Handle)) + `</h2><p>` + htmlEscape(reason) + `</p><p class="wolfbbs-muted">7-day surfaces: boards ` + strconv.Itoa(row.Board7) + `, chat ` + strconv.Itoa(row.Chat7) + `, doors ` + strconv.Itoa(row.Door7) + `</p><p><a href="/directory?handle=` + url.QueryEscape(displayHandle(row.Handle)) + `">open profile</a></p></article>`)
		if idx >= 9 {
			break
		}
	}
	if cards.Len() == 0 {
		cards.WriteString(`<article class="wolfbbs-card"><p>No spotlight candidates yet.</p></article>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Returning Caller Spotlights</title></head><body>
<p><a href="/start">start</a> | <a href="/spotlights">spotlights</a> | <a href="/streaks">streaks</a> | <a href="/next">next</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Returning Caller Spotlights</h1>
<p>Roadmap 153: highlight returners so momentum feels visible and social.</p>
<div class="wolfbbs-grid">` + cards.String() + `</div>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func normalizeSeasonMission(row seasonMission) (seasonMission, []string) {
	errs := make([]string, 0, 8)
	row.ID = sanitizeBundleID(row.ID)
	row.Season = cleanOneLiner(row.Season, 48)
	row.Title = cleanOneLiner(row.Title, 96)
	row.Description = cleanOneLiner(row.Description, 240)
	row.UpdatedBy = normalizeHandleKey(row.UpdatedBy)
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	if row.ID == "" {
		row.ID = "mission-" + randomEventID()
	}
	if !seasonMissionIDExpr.MatchString(row.ID) {
		errs = append(errs, "mission id must be lowercase safe token")
	}
	if row.Title == "" {
		errs = append(errs, "mission title is required")
	}
	if row.StartsAt.IsZero() {
		row.StartsAt = time.Now().UTC().Add(-24 * time.Hour)
	}
	if row.EndsAt.IsZero() || !row.EndsAt.After(row.StartsAt) {
		row.EndsAt = row.StartsAt.Add(30 * 24 * time.Hour)
	}
	row.TargetBoardPosts = clampInt(row.TargetBoardPosts, 0, 500)
	row.TargetChatPosts = clampInt(row.TargetChatPosts, 0, 500)
	row.TargetDoorRuns = clampInt(row.TargetDoorRuns, 0, 500)
	if row.TargetBoardPosts == 0 && row.TargetChatPosts == 0 && row.TargetDoorRuns == 0 {
		errs = append(errs, "at least one mission target must be > 0")
	}
	if row.Season == "" {
		row.Season = "current"
	}
	return row, errs
}

func (a *webApp) loadSeasonMissions() []seasonMission {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingSeasonMissions)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	decoded := []seasonMission{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("missions", fmt.Errorf("decode missions: %w", err))
		return nil
	}
	out := make([]seasonMission, 0, len(decoded))
	for _, row := range decoded {
		normalized, errs := normalizeSeasonMission(row)
		if len(errs) > 0 {
			continue
		}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartsAt.Equal(out[j].StartsAt) {
			return out[i].Title < out[j].Title
		}
		return out[i].StartsAt.After(out[j].StartsAt)
	})
	return out
}

func (a *webApp) persistSeasonMissions(rows []seasonMission) {
	if a.adminRepo == nil {
		return
	}
	clean := make([]seasonMission, 0, len(rows))
	for _, row := range rows {
		normalized, errs := normalizeSeasonMission(row)
		if len(errs) > 0 {
			continue
		}
		clean = append(clean, normalized)
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i].StartsAt.Equal(clean[j].StartsAt) {
			return clean[i].Title < clean[j].Title
		}
		return clean[i].StartsAt.After(clean[j].StartsAt)
	})
	if len(clean) > maxSeasonMissions {
		clean = clean[:maxSeasonMissions]
	}
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			a.addAppError("missions", fmt.Errorf("encode missions: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingSeasonMissions, body)
}

func (a *webApp) loadMissionCompletions() []missionCompletion {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingSeasonMissionCompletions)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	rows := []missionCompletion{}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("missions", fmt.Errorf("decode mission completions: %w", err))
		return nil
	}
	out := make([]missionCompletion, 0, len(rows))
	for _, row := range rows {
		row.MissionID = strings.TrimSpace(row.MissionID)
		row.Handle = normalizeHandleKey(row.Handle)
		if row.MissionID == "" || row.Handle == "" || row.ClaimedAt.IsZero() {
			continue
		}
		out = append(out, row)
	}
	return out
}

func (a *webApp) persistMissionCompletions(rows []missionCompletion) {
	if a.adminRepo == nil {
		return
	}
	clean := make([]missionCompletion, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		row.MissionID = strings.TrimSpace(row.MissionID)
		row.Handle = normalizeHandleKey(row.Handle)
		if row.MissionID == "" || row.Handle == "" || row.ClaimedAt.IsZero() {
			continue
		}
		key := row.MissionID + "|" + row.Handle
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		clean = append(clean, row)
	}
	sort.Slice(clean, func(i, j int) bool {
		if clean[i].ClaimedAt.Equal(clean[j].ClaimedAt) {
			if clean[i].MissionID == clean[j].MissionID {
				return clean[i].Handle < clean[j].Handle
			}
			return clean[i].MissionID < clean[j].MissionID
		}
		return clean[i].ClaimedAt.After(clean[j].ClaimedAt)
	})
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			a.addAppError("missions", fmt.Errorf("encode mission completions: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingSeasonMissionCompletions, body)
}

func (a *webApp) missionProgressForUser(user *domain.User, mission seasonMission) missionProgress {
	if user == nil {
		return missionProgress{}
	}
	out := missionProgress{}
	inWindow := func(at time.Time) bool {
		if at.IsZero() {
			return false
		}
		at = at.UTC()
		return !at.Before(mission.StartsAt.UTC()) && !at.After(mission.EndsAt.UTC())
	}
	if a.boardRepo != nil && a.msgRepo != nil {
		if boards, err := a.boardRepo.List(); err == nil {
			for _, board := range boards {
				msgs, err := a.msgRepo.ListByBoard(board.ID)
				if err != nil {
					continue
				}
				for _, msg := range msgs {
					if msg.AuthorID == user.ID && inWindow(msg.CreatedAt) {
						out.BoardPosts++
					}
				}
			}
		}
	}
	if a.chatSvc != nil {
		for _, channel := range a.chatSvc.ListChannels() {
			for _, msg := range a.chatSvc.History(channel, 8000) {
				if strings.EqualFold(msg.From, user.Handle) && inWindow(msg.CreatedAt) {
					out.ChatPosts++
				}
			}
		}
	}
	if a.doorRegistry != nil {
		for _, door := range a.doorRegistry.Doors() {
			events, err := a.doorRegistry.ListEvents(door.ID, user.ID, 8000)
			if err != nil {
				continue
			}
			for _, ev := range events {
				switch strings.ToLower(strings.TrimSpace(ev.EventType)) {
				case "start", "connector_launch", "poll_vote", "poll_create":
					if inWindow(ev.CreatedAt) {
						out.DoorRuns++
					}
				}
			}
		}
	}
	return out
}

func missionProgressSatisfied(progress missionProgress, mission seasonMission) bool {
	if mission.TargetBoardPosts > 0 && progress.BoardPosts < mission.TargetBoardPosts {
		return false
	}
	if mission.TargetChatPosts > 0 && progress.ChatPosts < mission.TargetChatPosts {
		return false
	}
	if mission.TargetDoorRuns > 0 && progress.DoorRuns < mission.TargetDoorRuns {
		return false
	}
	return true
}

func (a *webApp) userMissionClaimed(missionID, handle string) bool {
	missionID = strings.TrimSpace(missionID)
	handle = normalizeHandleKey(handle)
	if missionID == "" || handle == "" {
		return false
	}
	for _, row := range a.loadMissionCompletions() {
		if row.MissionID == missionID && row.Handle == handle {
			return true
		}
	}
	return false
}

func (a *webApp) claimMission(missionID, handle string) {
	missionID = strings.TrimSpace(missionID)
	handle = normalizeHandleKey(handle)
	if missionID == "" || handle == "" {
		return
	}
	rows := a.loadMissionCompletions()
	rows = append(rows, missionCompletion{MissionID: missionID, Handle: handle, ClaimedAt: time.Now().UTC()})
	a.persistMissionCompletions(rows)
}

func (a *webApp) handleMissions(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method == http.MethodPost {
		if !a.requireCSRF(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		if action != "claim" {
			redirectWithError(w, r, "/missions", "Unsupported mission action.")
			return
		}
		missionID := strings.TrimSpace(r.FormValue("mission_id"))
		if missionID == "" {
			redirectWithError(w, r, "/missions", "Mission id is required.")
			return
		}
		missions := a.loadSeasonMissions()
		for _, mission := range missions {
			if mission.ID != missionID {
				continue
			}
			if !mission.Active {
				redirectWithError(w, r, "/missions", "Mission is not active.")
				return
			}
			if a.userMissionClaimed(mission.ID, user.Handle) {
				redirectWithNotice(w, r, "/missions", "Mission already claimed.")
				return
			}
			progress := a.missionProgressForUser(user, mission)
			if !missionProgressSatisfied(progress, mission) {
				redirectWithError(w, r, "/missions", "Mission goals not complete yet.")
				return
			}
			a.claimMission(mission.ID, user.Handle)
			a.recordAdminAction(user.Handle, mission.ID, "claim_mission", "")
			redirectWithNotice(w, r, "/missions", "Mission claimed.")
			return
		}
		redirectWithError(w, r, "/missions", "Mission not found.")
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	now := time.Now().UTC()
	rows := a.loadSeasonMissions()
	cards := strings.Builder{}
	for _, mission := range rows {
		if !mission.Active {
			continue
		}
		if now.Before(mission.StartsAt.UTC()) || now.After(mission.EndsAt.UTC()) {
			continue
		}
		progress := a.missionProgressForUser(user, mission)
		claimed := a.userMissionClaimed(mission.ID, user.Handle)
		claimBlock := `<p><em>Complete goals to claim.</em></p>`
		if claimed {
			claimBlock = `<p><strong>Claimed</strong></p>`
		} else if missionProgressSatisfied(progress, mission) {
			claimBlock = `<form method="POST" action="/missions">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="claim"><input type="hidden" name="mission_id" value="` + htmlEscape(mission.ID) + `"><button type="submit">Claim Mission</button></form>`
		}
		cards.WriteString(`<article class="wolfbbs-card"><h2>` + htmlEscape(mission.Title) + `</h2><p class="wolfbbs-muted">` + htmlEscape(mission.Season) + ` • ` + mission.StartsAt.Local().Format("2006-01-02") + ` to ` + mission.EndsAt.Local().Format("2006-01-02") + `</p><p>` + htmlEscape(defaultIfBlank(mission.Description, "Complete the mission goals during the active season.")) + `</p><ul><li>Boards: ` + strconv.Itoa(progress.BoardPosts) + ` / ` + strconv.Itoa(mission.TargetBoardPosts) + `</li><li>Chat: ` + strconv.Itoa(progress.ChatPosts) + ` / ` + strconv.Itoa(mission.TargetChatPosts) + `</li><li>Doors: ` + strconv.Itoa(progress.DoorRuns) + ` / ` + strconv.Itoa(mission.TargetDoorRuns) + `</li></ul>` + claimBlock + `</article>`)
	}
	if cards.Len() == 0 {
		cards.WriteString(`<article class="wolfbbs-card"><p>No active missions right now. Ask sysop to configure missions in <a href="/admin/missions">/admin/missions</a>.</p></article>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Seasonal Missions</title></head><body>
<p><a href="/start">start</a> | <a href="/missions">missions</a> | <a href="/streaks">streaks</a> | <a href="/next">next</a> | <a href="/challenges">challenges</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Seasonal Missions</h1>
<p>Roadmap 154: medium-term goals with per-caller completion tracking.</p>
<div class="wolfbbs-grid">` + cards.String() + `</div>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleAdminMissions(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method == http.MethodPost {
		if !a.requireAdminWrite(w, r) {
			return
		}
		action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
		switch action {
		case "save_mission":
			startsAt, err := parseLocalDateTime(r.FormValue("starts_at"))
			if err != nil {
				redirectWithError(w, r, "/admin/missions", "Mission start must be a valid local date/time.")
				return
			}
			endsAt, err := parseLocalDateTime(r.FormValue("ends_at"))
			if err != nil || !endsAt.After(startsAt) {
				redirectWithError(w, r, "/admin/missions", "Mission end must be after mission start.")
				return
			}
			mission := seasonMission{
				ID:               r.FormValue("id"),
				Season:           r.FormValue("season"),
				Title:            r.FormValue("title"),
				Description:      r.FormValue("description"),
				StartsAt:         startsAt.UTC(),
				EndsAt:           endsAt.UTC(),
				TargetBoardPosts: parseIntWithFallback(r.FormValue("target_board_posts"), 0),
				TargetChatPosts:  parseIntWithFallback(r.FormValue("target_chat_posts"), 0),
				TargetDoorRuns:   parseIntWithFallback(r.FormValue("target_door_runs"), 0),
				Active:           formHasValue(r, "active"),
				UpdatedBy:        user.Handle,
				UpdatedAt:        time.Now().UTC(),
			}
			normalized, errs := normalizeSeasonMission(mission)
			if len(errs) > 0 {
				redirectWithError(w, r, "/admin/missions", "Mission rejected: "+strings.Join(errs, "; "))
				return
			}
			rows := a.loadSeasonMissions()
			updated := false
			for i := range rows {
				if rows[i].ID == normalized.ID {
					rows[i] = normalized
					updated = true
					break
				}
			}
			if !updated {
				rows = append(rows, normalized)
			}
			a.persistSeasonMissions(rows)
			a.recordAdminAction(user.Handle, normalized.ID, "save_mission", normalized.Title)
			redirectWithNotice(w, r, "/admin/missions", "Mission saved.")
			return
		case "delete_mission":
			id := sanitizeBundleID(r.FormValue("id"))
			if id == "" {
				redirectWithError(w, r, "/admin/missions", "Mission id is required.")
				return
			}
			rows := a.loadSeasonMissions()
			next := make([]seasonMission, 0, len(rows))
			deleted := false
			for _, row := range rows {
				if row.ID == id {
					deleted = true
					continue
				}
				next = append(next, row)
			}
			if !deleted {
				redirectWithError(w, r, "/admin/missions", "Mission not found.")
				return
			}
			a.persistSeasonMissions(next)
			a.recordAdminAction(user.Handle, id, "delete_mission", "")
			redirectWithNotice(w, r, "/admin/missions", "Mission deleted.")
			return
		default:
			redirectWithError(w, r, "/admin/missions", "Unsupported mission action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rows := a.loadSeasonMissions()
	tableRows := strings.Builder{}
	for _, row := range rows {
		tableRows.WriteString(`<tr><td><code>` + htmlEscape(row.ID) + `</code></td><td>` + htmlEscape(row.Season) + `</td><td>` + htmlEscape(row.Title) + `</td><td>` + htmlEscape(row.StartsAt.Local().Format("2006-01-02")) + `</td><td>` + htmlEscape(row.EndsAt.Local().Format("2006-01-02")) + `</td><td>` + strconv.Itoa(row.TargetBoardPosts) + ` / ` + strconv.Itoa(row.TargetChatPosts) + ` / ` + strconv.Itoa(row.TargetDoorRuns) + `</td><td>` + boolToText(row.Active) + `</td><td><form method="POST" action="/admin/missions" style="display:inline">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="delete_mission"><input type="hidden" name="id" value="` + htmlEscape(row.ID) + `"><button type="submit">Delete</button></form></td></tr>`)
	}
	if tableRows.Len() == 0 {
		tableRows.WriteString(`<tr><td colspan="8">No missions configured yet.</td></tr>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Missions Admin</title></head><body>
<p><a href="/admin">admin</a> | <a href="/admin/challenges">challenges</a> | <a href="/admin/missions">missions</a> | <a href="/missions">public missions</a> | <a href="/admin/analytics">analytics</a></p>
` + pageMessageBlock(r) + `
<h1>Seasonal Missions Admin</h1>
<p>Roadmap 154 control plane for mission templates and completion tracking.</p>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Create / Update Mission</h2><form method="POST" action="/admin/missions">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="save_mission"><label>ID <input name="id" size="28" placeholder="spring-return-1"></label><br><label>Season <input name="season" size="28" placeholder="Spring 2026"></label><br><label>Title <input name="title" size="64" placeholder="Post-Chat-Play Triple"></label><br><label>Description <input name="description" size="88" placeholder="Complete all three activity lanes this season."></label><br><label>Starts <input type="datetime-local" name="starts_at" value="` + formatLocalDateTimeValue(time.Now().UTC()) + `"></label><br><label>Ends <input type="datetime-local" name="ends_at" value="` + formatLocalDateTimeValue(time.Now().UTC().Add(30*24*time.Hour)) + `"></label><br><label>Target board posts <input name="target_board_posts" value="3" inputmode="numeric"></label><br><label>Target chat posts <input name="target_chat_posts" value="5" inputmode="numeric"></label><br><label>Target door runs <input name="target_door_runs" value="2" inputmode="numeric"></label><br><label><input type="checkbox" name="active" value="1" checked> Active</label><br><button type="submit">Save Mission</button></form></article><article class="wolfbbs-card"><h2>Guidelines</h2><ul><li>Use clear mission titles callers can finish in 1-4 weeks.</li><li>Keep targets bounded to avoid burnout.</li><li>Pair missions with /spotlights for social visibility.</li><li>Mission claims are idempotent and audit logged.</li></ul></article></section>
<table border="1"><tr><th>ID</th><th>Season</th><th>Title</th><th>Start</th><th>End</th><th>Targets (B/C/D)</th><th>Active</th><th>Action</th></tr>` + tableRows.String() + `</table>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func releaseChecklistDefaults() []string {
	return []string{
		"go_test_all",
		"verify_fast",
		"verify_smoke",
		"manual_acceptance_auto",
		"release_notes_updated",
		"package_dist_built",
		"function_registry_updated",
	}
}

func releaseChecklistLabel(key string) string {
	switch key {
	case "go_test_all":
		return "go test ./..."
	case "verify_fast":
		return "scripts/verify.sh --fast"
	case "verify_smoke":
		return "scripts/verify.sh --smoke"
	case "manual_acceptance_auto":
		return "manual-acceptance auto"
	case "release_notes_updated":
		return "release notes updated"
	case "package_dist_built":
		return "distribution package built"
	case "function_registry_updated":
		return "function registry + UI matrix updated"
	default:
		return strings.ReplaceAll(strings.TrimSpace(key), "_", " ")
	}
}

func (a *webApp) loadReleaseChecklistState() releaseChecklistState {
	state := releaseChecklistState{Checks: map[string]bool{}}
	if a.adminRepo == nil {
		return state
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingReleaseChecklist146155)
	if err != nil || strings.TrimSpace(raw) == "" {
		return state
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		a.addAppError("release.checklist", fmt.Errorf("decode checklist: %w", err))
		return releaseChecklistState{Checks: map[string]bool{}}
	}
	if state.Checks == nil {
		state.Checks = map[string]bool{}
	}
	return state
}

func (a *webApp) persistReleaseChecklistState(state releaseChecklistState) {
	if a.adminRepo == nil {
		return
	}
	if state.Checks == nil {
		state.Checks = map[string]bool{}
	}
	raw, err := json.Marshal(state)
	if err != nil {
		a.addAppError("release.checklist", fmt.Errorf("encode checklist: %w", err))
		return
	}
	a.persistSystemSetting(sysSettingReleaseChecklist146155, string(raw))
}

func latestReleaseMetadata(limit int) []releaseArtifactRow {
	rows := releaseArtifacts(limit)
	return rows
}

func packageArtifactRows(limit int) []releaseArtifactRow {
	root := filepath.Join("dist", "releases")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	out := make([]releaseArtifactRow, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".txt") && !strings.HasSuffix(name, ".sha256") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, releaseArtifactRow{Name: entry.Name(), Path: filepath.Join(root, entry.Name()), Updated: info.ModTime().UTC()})
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

func (a *webApp) handleReleaseChecklistUpdate(r *http.Request, user *domain.User) error {
	if user == nil {
		return fmt.Errorf("missing user")
	}
	action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
	state := a.loadReleaseChecklistState()
	switch action {
	case "toggle_check":
		key := strings.TrimSpace(r.FormValue("key"))
		if key == "" {
			return fmt.Errorf("checklist key is required")
		}
		state.Checks[key] = formHasValue(r, "done")
		a.persistReleaseChecklistState(state)
		a.recordAdminAction(user.Handle, "release.checklist", "toggle", key+"="+boolToText(state.Checks[key]))
		return nil
	case "clear_checks":
		a.persistReleaseChecklistState(releaseChecklistState{Checks: map[string]bool{}})
		a.recordAdminAction(user.Handle, "release.checklist", "clear", "")
		return nil
	default:
		return fmt.Errorf("unsupported release action")
	}
}

func (a *webApp) digestMaxItemsForUser(handle string, fallback int, now time.Time) int {
	return a.digestMaxItemsForDate(handle, fallback, now)
}

func (a *webApp) themedMenuLinks() string {
	return `<a href="/admin/plugins">plugins</a> | <a href="/admin/themes">themes</a> | <a href="/admin/webhooks">webhooks</a> | <a href="/admin/analytics">analytics</a> | <a href="/admin/missions">missions</a>`
}

func (a *webApp) digestWeeklyOverrideSummary(handle string, fallback int) string {
	prefs := a.loadWeekdayDigestPrefs(handle, fallback)
	parts := []string{
		"Sun " + strconv.Itoa(prefs.Sunday),
		"Mon " + strconv.Itoa(prefs.Monday),
		"Tue " + strconv.Itoa(prefs.Tuesday),
		"Wed " + strconv.Itoa(prefs.Wednesday),
		"Thu " + strconv.Itoa(prefs.Thursday),
		"Fri " + strconv.Itoa(prefs.Friday),
		"Sat " + strconv.Itoa(prefs.Saturday),
	}
	return strings.Join(parts, " | ")
}
