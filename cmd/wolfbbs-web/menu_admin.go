package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (a *webApp) menuRootPath() string {
	root := filepath.Clean(strings.TrimSpace(a.menuRoot))
	if root == "" || root == "." {
		root = "menus"
	}
	return root
}

func (a *webApp) defaultMenuFile() string {
	root := a.menuRootPath()
	path := filepath.Join(root, "main.hjson")
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.ToSlash(path)
}

func (a *webApp) normalizeMenuFilePath(raw string) (string, error) {
	root := a.menuRootPath()
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	clean := filepath.Clean(strings.TrimSpace(raw))
	if clean == "" || clean == "." {
		clean = a.defaultMenuFile()
	}
	if !filepath.IsAbs(clean) {
		if filepath.IsAbs(root) {
			clean = filepath.Join(root, clean)
		} else {
			rootPrefix := filepath.ToSlash(root) + "/"
			cleanSlash := filepath.ToSlash(clean)
			if !strings.HasPrefix(cleanSlash, rootPrefix) {
				clean = filepath.Join(root, clean)
			}
		}
	}
	if strings.ToLower(filepath.Ext(clean)) != ".hjson" {
		return "", fmt.Errorf("menu file must end with .hjson")
	}

	absFile, err := filepath.Abs(clean)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, absFile)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("menu file must be within %s", root)
	}
	if filepath.IsAbs(clean) {
		return clean, nil
	}
	return filepath.ToSlash(clean), nil
}

func (a *webApp) listMenuFiles(selected string) []string {
	root := a.menuRootPath()
	files := make([]string, 0)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) != ".hjson" {
			return nil
		}
		if filepath.IsAbs(path) {
			files = append(files, path)
		} else {
			files = append(files, filepath.ToSlash(path))
		}
		return nil
	})
	if normalized, err := a.normalizeMenuFilePath(selected); err == nil {
		selected = normalized
	}
	if strings.TrimSpace(selected) != "" {
		found := false
		for _, row := range files {
			if row == selected {
				found = true
				break
			}
		}
		if !found {
			files = append(files, selected)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i]) < strings.ToLower(files[j])
	})
	return files
}

func (a *webApp) loadMenuFile(path string) (string, error) {
	path, err := a.normalizeMenuFilePath(path)
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (a *webApp) saveMenuFile(path, body string) error {
	path, err := a.normalizeMenuFilePath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	existing, readErr := os.ReadFile(path)
	if readErr == nil {
		backup := fmt.Sprintf("%s.%s.bak", path, time.Now().UTC().Format("20060102-150405"))
		if writeErr := os.WriteFile(backup, existing, 0o644); writeErr != nil {
			return fmt.Errorf("backup existing menu: %w", writeErr)
		}
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func (a *webApp) renderAdminConfigPage(w http.ResponseWriter, r *http.Request, statusCode int, menuFile, menuBody, menuNotice, menuErr string) {
	if statusCode <= 0 {
		statusCode = http.StatusOK
	}
	if strings.TrimSpace(menuFile) == "" {
		menuFile = a.defaultMenuFile()
	}
	if strings.TrimSpace(menuBody) == "" {
		if loaded, err := a.loadMenuFile(menuFile); err == nil {
			menuBody = loaded
		} else if strings.TrimSpace(menuErr) == "" {
			menuErr = "menu load failed: " + err.Error()
		}
	}

	noticeBlock := ""
	if strings.TrimSpace(menuNotice) != "" {
		noticeBlock = `<p><strong>Menu:</strong> ` + htmlEscape(menuNotice) + `</p>`
	}
	errBlock := ""
	if strings.TrimSpace(menuErr) != "" {
		errBlock = `<p><strong>Menu error:</strong> ` + htmlEscape(menuErr) + `</p>`
	}

	menuRows := strings.Builder{}
	for _, path := range a.listMenuFiles(menuFile) {
		menuRows.WriteString(`<li><a href="/admin/config?menu_file=` + url.QueryEscape(path) + `">` + htmlEscape(path) + `</a></li>`)
	}
	if menuRows.Len() == 0 {
		menuRows.WriteString(`<li>No menu files found under ` + htmlEscape(a.menuRootPath()) + `</li>`)
	}

	csrf := a.csrfHiddenInput(r)
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Runtime Configuration</title></head><body><h1>Runtime Configuration</h1><p><a href="/admin">back</a> | <a href="/admin/setup">setup</a> | <a href="/help">help</a></p>` +
		`<p>These values are persisted in system settings and applied on service startup. Environment values remain fallback defaults.</p>` +
		`<h2>Site Text</h2><form method="POST">` + csrf +
		`<input type="hidden" name="action" value="save_text">` +
		`<label>MOTD<br><textarea name="motd" rows="4" cols="90">` + htmlEscape(a.motd) + `</textarea></label><br>` +
		`<label>Announcement<br><textarea name="announcement" rows="4" cols="90">` + htmlEscape(a.announcement) + `</textarea></label><br>` +
		`<button type="submit">Save Text</button></form>` +
		`<h2>Feature and Safety Flags</h2><form method="POST">` + csrf +
		`<input type="hidden" name="action" value="save_flags">` +
		`<label><input type="checkbox" name="read_only"` + checkedIf(a.readOnly) + `> Read-only mode (block admin writes)</label><br>` +
		`<label><input type="checkbox" name="web_onramp"` + checkedIf(a.modernOnRamp) + `> Enable web connect on-ramp</label><br>` +
		`<label><input type="checkbox" name="guest_tour"` + checkedIf(a.guestTour) + `> Enable guided guest tour</label><br>` +
		`<label><input type="checkbox" name="discover"` + checkedIf(a.discover) + `> Enable discover/newscan web page</label><br>` +
		`<label><input type="checkbox" name="quick_jump"` + checkedIf(a.quickJump) + `> Enable quick jump commands (TUI/web)</label><br>` +
		`<label><input type="checkbox" name="classic_search"` + checkedIf(a.classicSearch) + `> Enable classic deep search presentation</label><br>` +
		`<button type="submit">Save Flags</button></form>` +
		`<h2>ANSI Menu Runtime</h2>` +
		noticeBlock +
		errBlock +
		`<form method="POST">` + csrf +
		`<input type="hidden" name="action" value="save_menu_settings">` +
		`<label><input type="checkbox" name="menu_enabled"` + checkedIf(a.runtimeCfg.Menu.Enabled) + `> Enable HJSON menu runtime in SSH sessions</label><br>` +
		`<label>Menu file <input name="menu_file" value="` + htmlEscape(menuFile) + `" size="64"></label> ` +
		`<button type="submit">Save Menu Runtime</button></form>` +
		`<p>Available menu files under <code>` + htmlEscape(a.menuRootPath()) + `</code>:</p><ul>` + menuRows.String() + `</ul>` +
		`<form method="POST">` + csrf +
		`<label>Editing file <input name="menu_file" value="` + htmlEscape(menuFile) + `" size="64"></label><br>` +
		`<textarea name="menu_body" rows="26" cols="120">` + htmlEscape(menuBody) + `</textarea><br>` +
		`<button type="submit" name="action" value="validate_menu">Validate Menu</button> ` +
		`<button type="submit" name="action" value="save_menu">Save Menu File</button></form>` +
		`<p>Menu edits are validated via HJSON schema checks before save. Reconnect SSH sessions after saving to load changes.</p></body></html>`
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(page))
}
