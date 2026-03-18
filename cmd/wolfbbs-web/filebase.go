package main

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/rbac"
	"wolfbbs/internal/repository"
)

const (
	maxSidecarBytes  = 8192
	defaultTicketTTL = 15 * time.Minute
)

func (a *webApp) indexAreaFiles(area domain.FileArea, uploaderID int64) (int, int, error) {
	if a.adminRepo == nil {
		return 0, 0, errors.New("admin repository unavailable")
	}
	root := filepath.Clean(strings.TrimSpace(area.Path))
	if root == "" || root == "." {
		return 0, 0, errors.New("invalid area path")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, 0, err
	}
	var indexed int
	var failed int
	for _, row := range entries {
		if row.IsDir() {
			continue
		}
		name := strings.TrimSpace(row.Name())
		if name == "" {
			continue
		}
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".diz") || strings.HasSuffix(lower, ".nfo") {
			continue
		}
		fullPath := filepath.Join(root, name)
		info, err := row.Info()
		if err != nil {
			failed++
			continue
		}
		sum, err := fileSHA256(fullPath)
		if err != nil {
			failed++
			continue
		}
		desc, tags := fileMetadata(root, name)
		entry := &domain.FileEntry{
			AreaID:      area.ID,
			Name:        name,
			Path:        fullPath,
			Description: desc,
			Tags:        tags,
			SHA256:      sum,
			SizeBytes:   info.Size(),
			UploaderID:  uploaderID,
			UploadedAt:  info.ModTime().UTC(),
		}
		if err := a.adminRepo.UpsertFileEntry(entry); err != nil {
			failed++
			continue
		}
		indexed++
	}
	return indexed, failed, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fileMetadata(root, filename string) (string, []string) {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	candidates := []string{
		filepath.Join(root, base+".diz"),
		filepath.Join(root, base+".DIZ"),
		filepath.Join(root, base+".nfo"),
		filepath.Join(root, base+".NFO"),
	}
	description := ""
	for _, path := range candidates {
		if text := readSidecarText(path); text != "" {
			description = text
			break
		}
	}
	return description, deriveTags(filename, description)
}

func readSidecarText(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	reader := io.LimitReader(f, maxSidecarBytes)
	scanner := bufio.NewScanner(reader)
	lines := make([]string, 0, 8)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) >= 3 {
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines, " "))
}

func deriveTags(filename, description string) []string {
	out := map[string]struct{}{}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	if ext != "" {
		out[ext] = struct{}{}
	}
	stem := strings.ToLower(strings.TrimSuffix(filename, filepath.Ext(filename)))
	for _, part := range strings.FieldsFunc(stem, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		part = strings.TrimSpace(part)
		if len(part) >= 3 {
			out[part] = struct{}{}
		}
	}
	for _, part := range strings.Fields(strings.ToLower(description)) {
		part = strings.TrimFunc(part, func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
		})
		if len(part) >= 4 {
			out[part] = struct{}{}
		}
	}
	tags := make([]string, 0, len(out))
	for tag := range out {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	if len(tags) > 8 {
		tags = tags[:8]
	}
	return tags
}

func mergeFileTags(existing []string, raw string) []string {
	out := map[string]struct{}{}
	for _, tag := range existing {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag != "" {
			out[tag] = struct{}{}
		}
	}
	for _, tag := range strings.Split(raw, ",") {
		tag = strings.ToLower(strings.TrimSpace(tag))
		tag = strings.TrimFunc(tag, func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' && r != '_'
		})
		if len(tag) >= 2 {
			out[tag] = struct{}{}
		}
	}
	tags := make([]string, 0, len(out))
	for tag := range out {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	if len(tags) > 10 {
		tags = tags[:10]
	}
	return tags
}

func sanitizeUploadFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '.', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, name)
	name = strings.Trim(name, "._")
	if name == "" {
		return ""
	}
	return name
}

func nextAvailableUploadPath(root, name string) (string, string) {
	name = sanitizeUploadFilename(name)
	if name == "" {
		return "", ""
	}
	destPath := filepath.Join(root, name)
	if _, err := os.Stat(destPath); errors.Is(err, os.ErrNotExist) {
		return destPath, name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 2; i <= 200; i++ {
		candidate := fmt.Sprintf("%s-%d%s", stem, i, ext)
		destPath = filepath.Join(root, candidate)
		if _, err := os.Stat(destPath); errors.Is(err, os.ErrNotExist) {
			return destPath, candidate
		}
	}
	return "", ""
}

func (a *webApp) importUploadedFile(r *http.Request, area domain.FileArea, uploader *domain.User, description, rawTags string) (*domain.FileEntry, error) {
	if a.adminRepo == nil {
		return nil, errors.New("admin repository unavailable")
	}
	if uploader == nil || uploader.ID <= 0 {
		return nil, errors.New("uploader is required")
	}
	root := filepath.Clean(strings.TrimSpace(area.Path))
	if root == "" || root == "." {
		return nil, errors.New("invalid area path")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, fmt.Errorf("parse upload: %w", err)
	}
	src, header, err := r.FormFile("upload_file")
	if err != nil {
		return nil, err
	}
	defer src.Close()
	destPath, displayName := nextAvailableUploadPath(root, header.Filename)
	if destPath == "" || displayName == "" {
		return nil, errors.New("invalid upload filename")
	}
	dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(destPath)
		return nil, err
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(destPath)
		return nil, err
	}
	sum, err := fileSHA256(destPath)
	if err != nil {
		_ = os.Remove(destPath)
		return nil, err
	}
	autoDesc, autoTags := fileMetadata(root, displayName)
	finalDesc := strings.TrimSpace(description)
	if finalDesc == "" {
		finalDesc = autoDesc
	}
	info, err := os.Stat(destPath)
	if err != nil {
		_ = os.Remove(destPath)
		return nil, err
	}
	entry := &domain.FileEntry{
		AreaID:      area.ID,
		Name:        displayName,
		Path:        destPath,
		Description: finalDesc,
		Tags:        mergeFileTags(autoTags, rawTags),
		SHA256:      sum,
		SizeBytes:   info.Size(),
		UploaderID:  uploader.ID,
		UploadedAt:  info.ModTime().UTC(),
	}
	if err := a.adminRepo.UpsertFileEntry(entry); err != nil {
		_ = os.Remove(destPath)
		return nil, err
	}
	return entry, nil
}

func (a *webApp) deleteIndexedFile(entry *domain.FileEntry) error {
	if entry == nil {
		return errors.New("file entry is required")
	}
	if a.adminRepo == nil {
		return errors.New("admin repository unavailable")
	}
	if path := strings.TrimSpace(entry.Path); path != "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return a.adminRepo.DeleteFileEntry(entry.ID)
}

func (a *webApp) createDownloadTicket(userID, fileID int64, ttl time.Duration) (*domain.DownloadTicket, error) {
	if a.adminRepo == nil {
		return nil, errors.New("admin repository unavailable")
	}
	if userID <= 0 || fileID <= 0 {
		return nil, errors.New("user id and file id are required")
	}
	if ttl <= 0 {
		ttl = defaultTicketTTL
	}
	token := "dl_" + randomToken(40)
	ticket := &domain.DownloadTicket{
		Token:     token,
		UserID:    userID,
		FileID:    fileID,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(ttl),
	}
	if err := a.adminRepo.CreateDownloadTicket(ticket); err != nil {
		return nil, err
	}
	return ticket, nil
}

func (a *webApp) resolveDownloadPath(entry *domain.FileEntry) (string, error) {
	if entry == nil {
		return "", repository.ErrNotFound
	}
	candidate := filepath.Clean(strings.TrimSpace(entry.Path))
	if candidate != "" && candidate != "." {
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return candidate, nil
		}
	}
	areas, err := a.adminRepo.ListFileAreas()
	if err != nil {
		return "", err
	}
	for _, area := range areas {
		if area.ID != entry.AreaID {
			continue
		}
		root := filepath.Clean(strings.TrimSpace(area.Path))
		if root == "" || root == "." {
			return "", errors.New("invalid file area path")
		}
		next := filepath.Clean(filepath.Join(root, entry.Path))
		rel, relErr := filepath.Rel(root, next)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			return "", errors.New("invalid file path")
		}
		if stat, err := os.Stat(next); err == nil && !stat.IsDir() {
			return next, nil
		}
		break
	}
	return "", fmt.Errorf("file path missing for entry %d", entry.ID)
}

func (a *webApp) handleGatewayFileAction(w http.ResponseWriter, r *http.Request, user *domain.User) bool {
	action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
	if action == "" || action == "fetch" {
		return false
	}
	redirectPath := "/gateway?view=files"
	if candidate := strings.TrimSpace(r.FormValue("return_to")); strings.HasPrefix(candidate, "/") {
		redirectPath = candidate
	}
	if a.adminRepo == nil {
		redirectWithError(w, r, redirectPath, "FileBase is unavailable.")
		return true
	}
	redirectURL := redirectPath
	notice := ""
	errMsg := ""
	switch action {
	case "rate_file":
		fileID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
		rating := parseInt(r.FormValue("rating"), 0)
		if fileID > 0 && rating > 0 {
			if !a.fileVisibleToCallers(fileID) {
				errMsg = "File is still in review."
				break
			}
			if err := a.adminRepo.SetFileRating(user.ID, fileID, rating); err != nil {
				errMsg = "Could not save rating."
				break
			}
			notice = "Rating saved."
		} else {
			errMsg = "Select a file and rating before submitting."
		}
	case "save_filter":
		filter := &domain.FileFilter{
			UserID: user.ID,
			Name:   strings.TrimSpace(r.FormValue("name")),
			Query:  strings.TrimSpace(r.FormValue("query")),
		}
		for _, tag := range strings.Split(strings.TrimSpace(r.FormValue("tags")), ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				filter.Tags = append(filter.Tags, tag)
			}
		}
		if strings.TrimSpace(filter.Name) == "" {
			errMsg = "Filter name is required."
			break
		}
		if err := a.adminRepo.SaveFileFilter(filter); err != nil {
			errMsg = "Could not save filter."
			break
		}
		notice = "Saved filter '" + filter.Name + "'."
	case "queue_add":
		fileID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
		if fileID > 0 {
			if !a.fileVisibleToCallers(fileID) {
				errMsg = "File is still in review."
				break
			}
			if err := a.adminRepo.EnqueueDownload(user.ID, fileID); err != nil {
				errMsg = "Could not add file to queue."
				break
			}
			notice = "Added file to download queue."
		} else {
			errMsg = "Select a file before queueing."
		}
	case "queue_remove":
		fileID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
		if fileID > 0 {
			if err := a.adminRepo.DequeueDownload(user.ID, fileID); err != nil {
				errMsg = "Could not remove file from queue."
				break
			}
			notice = "Removed file from queue."
		} else {
			errMsg = "Select a file before removing."
		}
	case "ticket":
		fileID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
		ttlMinutes := parseInt(r.FormValue("ttl_minutes"), 15)
		if ttlMinutes <= 0 {
			ttlMinutes = 15
		}
		if fileID > 0 {
			if !a.fileVisibleToCallers(fileID) {
				errMsg = "File is still in review."
				break
			}
			ticket, err := a.createDownloadTicket(user.ID, fileID, time.Duration(ttlMinutes)*time.Minute)
			if err == nil && ticket != nil {
				redirectURL += "&issued_token=" + url.QueryEscape(ticket.Token)
				notice = "Download ticket issued."
			} else {
				errMsg = "Could not issue download ticket."
			}
		} else {
			errMsg = "Select a file before issuing a ticket."
		}
	default:
		redirectWithError(w, r, redirectPath, "Unsupported file action.")
		return true
	}
	if errMsg != "" {
		redirectWithError(w, r, redirectPath, errMsg)
		return true
	}
	if notice != "" {
		redirectWithNotice(w, r, redirectURL, notice)
		return true
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
	return true
}

func (a *webApp) relatedVisibleFiles(entry *domain.FileEntry, limit int) []domain.FileEntry {
	if a.adminRepo == nil || entry == nil {
		return nil
	}
	if limit <= 0 {
		limit = 6
	}
	all, err := a.adminRepo.ListFileEntries(0, "", nil, 400)
	if err != nil {
		return nil
	}
	all = a.filterVisibleFiles(all)
	type scoredEntry struct {
		entry domain.FileEntry
		score int
	}
	tagSet := map[string]struct{}{}
	for _, tag := range entry.Tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag != "" {
			tagSet[tag] = struct{}{}
		}
	}
	scored := make([]scoredEntry, 0, len(all))
	for _, row := range all {
		if row.ID == entry.ID {
			continue
		}
		score := 0
		if row.AreaID == entry.AreaID {
			score += 3
		}
		for _, tag := range row.Tags {
			if _, ok := tagSet[strings.ToLower(strings.TrimSpace(tag))]; ok {
				score += 2
			}
		}
		if score == 0 {
			continue
		}
		scored = append(scored, scoredEntry{entry: row, score: score})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].entry.RatingAvg != scored[j].entry.RatingAvg {
			return scored[i].entry.RatingAvg > scored[j].entry.RatingAvg
		}
		return scored[i].entry.UploadedAt.After(scored[j].entry.UploadedAt)
	})
	out := make([]domain.FileEntry, 0, limit)
	for _, row := range scored {
		out = append(out, row.entry)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (a *webApp) renderGatewayFiles(w http.ResponseWriter, r *http.Request, user *domain.User) {
	if a.adminRepo == nil {
		http.Error(w, "filebase unavailable", http.StatusServiceUnavailable)
		return
	}
	fileAreaID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("area")), 10, 64)
	fileID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	tagsRaw := strings.TrimSpace(r.URL.Query().Get("tags"))
	tags := make([]string, 0)
	for _, tag := range strings.Split(tagsRaw, ",") {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	files, _ := a.adminRepo.ListFileEntries(fileAreaID, query, tags, 250)
	files = a.filterVisibleFiles(files)
	filters, _ := a.adminRepo.ListFileFilters(user.ID)
	queue, _ := a.adminRepo.ListDownloadQueue(user.ID, 200)
	areas, _ := a.adminRepo.ListFileAreas()
	areaNames := map[int64]string{}
	for _, area := range areas {
		areaNames[area.ID] = area.Name
	}
	queueNames := map[int64]string{}
	for _, row := range queue {
		if entry, err := a.adminRepo.GetFileEntry(row.FileID); err == nil && entry != nil && a.fileVisibleToCallers(entry.ID) {
			queueNames[row.FileID] = entry.Name
		}
	}
	issuedToken := strings.TrimSpace(r.URL.Query().Get("issued_token"))
	csrf := a.csrfHiddenInput(r)
	messageBlock := pageMessageBlock(r)
	returnTo := "/gateway?view=files"
	if raw := r.URL.RawQuery; strings.TrimSpace(raw) != "" {
		returnTo += "&" + raw
	}

	fileRows := strings.Builder{}
	for _, row := range files {
		tagsText := htmlEscape(strings.Join(row.Tags, ","))
		fileRows.WriteString(`<tr><td>` + strconv.FormatInt(row.ID, 10) + `</td><td>` + htmlEscape(areaNames[row.AreaID]) + `</td><td><a href="/gateway?view=files&id=` + strconv.FormatInt(row.ID, 10) + `">` + htmlEscape(row.Name) + `</a><br><span class="wolfbbs-muted">` + htmlEscape(cleanOneLiner(defaultIfBlank(row.Description, "No description yet."), 120)) + `</span></td><td>` + tagsText + `</td><td>` + fmt.Sprintf("%.2f", row.RatingAvg) + ` (` + strconv.Itoa(row.RatingCount) + `)</td><td>`)
		fileRows.WriteString(`<form method="POST" action="/gateway">` + csrf + `<input type="hidden" name="action" value="rate_file"><input type="hidden" name="file_id" value="` + strconv.FormatInt(row.ID, 10) + `"><input type="hidden" name="return_to" value="` + htmlEscape(returnTo) + `"><input name="rating" size="2" value="5"><button type="submit">rate</button></form>`)
		fileRows.WriteString(`<form method="POST" action="/gateway">` + csrf + `<input type="hidden" name="action" value="queue_add"><input type="hidden" name="file_id" value="` + strconv.FormatInt(row.ID, 10) + `"><input type="hidden" name="return_to" value="` + htmlEscape(returnTo) + `"><button type="submit">queue</button></form></td></tr>`)
	}
	if fileRows.Len() == 0 {
		fileRows.WriteString(`<tr><td colspan="6">No files matched.</td></tr>`)
	}

	filterRows := strings.Builder{}
	for _, row := range filters {
		filterRows.WriteString(`<tr><td>` + htmlEscape(row.Name) + `</td><td>` + htmlEscape(row.Query) + `</td><td>` + htmlEscape(strings.Join(row.Tags, ",")) + `</td></tr>`)
	}
	if filterRows.Len() == 0 {
		filterRows.WriteString(`<tr><td colspan="3">No saved filters.</td></tr>`)
	}

	queueRows := strings.Builder{}
	for _, row := range queue {
		if !a.fileVisibleToCallers(row.FileID) {
			continue
		}
		name := queueNames[row.FileID]
		if name == "" {
			name = "file #" + strconv.FormatInt(row.FileID, 10)
		}
		queueRows.WriteString(`<tr><td>` + htmlEscape(name) + `</td><td>` + row.CreatedAt.Local().Format(time.RFC3339) + `</td><td>`)
		queueRows.WriteString(`<form method="POST" action="/gateway">` + csrf + `<input type="hidden" name="action" value="queue_remove"><input type="hidden" name="file_id" value="` + strconv.FormatInt(row.FileID, 10) + `"><button type="submit">remove</button></form>`)
		queueRows.WriteString(`<form method="POST" action="/gateway">` + csrf + `<input type="hidden" name="action" value="ticket"><input type="hidden" name="file_id" value="` + strconv.FormatInt(row.FileID, 10) + `"><input name="ttl_minutes" size="4" value="15"><button type="submit">ticket</button></form>`)
		queueRows.WriteString(`</td></tr>`)
	}
	if queueRows.Len() == 0 {
		queueRows.WriteString(`<tr><td colspan="3">Queue is empty.</td></tr>`)
	}

	issuedBlock := ""
	if issuedToken != "" {
		issuedBlock = `<p><strong>Ticket issued:</strong> <code>` + htmlEscape(issuedToken) + `</code><br><a href="/gateway?download=` + url.QueryEscape(issuedToken) + `">/gateway?download=` + url.QueryEscape(issuedToken) + `</a></p>`
	}

	previewBlock := ``
	if fileID > 0 {
		if entry, err := a.adminRepo.GetFileEntry(fileID); err == nil && entry != nil && a.fileVisibleToCallers(entry.ID) {
			relatedRows := strings.Builder{}
			for _, related := range a.relatedVisibleFiles(entry, 6) {
				relatedRows.WriteString(`<li><a href="/gateway?view=files&id=` + strconv.FormatInt(related.ID, 10) + `">` + htmlEscape(related.Name) + `</a> <span class="wolfbbs-muted">` + htmlEscape(strings.Join(related.Tags, ", ")) + `</span></li>`)
			}
			if relatedRows.Len() == 0 {
				relatedRows.WriteString(`<li>No related uploads yet.</li>`)
			}
			tagLinks := strings.Builder{}
			for _, tag := range entry.Tags {
				clean := strings.TrimSpace(tag)
				if clean == "" {
					continue
				}
				if tagLinks.Len() > 0 {
					tagLinks.WriteString(` `)
				}
				tagLinks.WriteString(`<a href="/gateway?view=files&tags=` + url.QueryEscape(clean) + `">#` + htmlEscape(clean) + `</a>`)
			}
			if tagLinks.Len() == 0 {
				tagLinks.WriteString(`<span class="wolfbbs-muted">No tags yet.</span>`)
			}
			previewBlock = `<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>File Preview</h2><p><strong>` + htmlEscape(entry.Name) + `</strong> in ` + htmlEscape(areaNames[entry.AreaID]) + `</p><p>` + htmlEscape(defaultIfBlank(entry.Description, "No description provided yet.")) + `</p><ul class="wolfbbs-list-clean"><li>Size: ` + strconv.FormatInt(entry.SizeBytes, 10) + ` bytes</li><li>Uploaded: ` + entry.UploadedAt.Local().Format("2006-01-02 15:04") + `</li><li>Rating: ` + fmt.Sprintf("%.2f", entry.RatingAvg) + ` from ` + strconv.Itoa(entry.RatingCount) + ` votes</li><li>SHA-256: <code>` + htmlEscape(entry.SHA256) + `</code></li></ul><p><strong>Tags:</strong> ` + tagLinks.String() + `</p><div class="wolfbbs-inline-actions"><form method="POST" action="/gateway">` + csrf + `<input type="hidden" name="action" value="queue_add"><input type="hidden" name="file_id" value="` + strconv.FormatInt(entry.ID, 10) + `"><input type="hidden" name="return_to" value="/gateway?view=files&id=` + strconv.FormatInt(entry.ID, 10) + `"><button type="submit">Add To Queue</button></form><form method="POST" action="/gateway">` + csrf + `<input type="hidden" name="action" value="ticket"><input type="hidden" name="file_id" value="` + strconv.FormatInt(entry.ID, 10) + `"><input type="hidden" name="return_to" value="/gateway?view=files&id=` + strconv.FormatInt(entry.ID, 10) + `"><input name="ttl_minutes" size="4" value="15"><button type="submit">Issue Ticket</button></form></div></article><article class="wolfbbs-card"><h2>Related Uploads</h2><ul>` + relatedRows.String() + `</ul></article></section>`
		}
	}

	page := `<html><body>
	<h1>Gateway FileBase</h1>
	<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/chat">chat</a> | <a href="/gateway">gateway</a> | <a href="/status">status</a> | <a href="/config">config</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>` +
		messageBlock +
		issuedBlock +
		previewBlock +
		`<p><strong>Tip:</strong> Queue files first, then issue one-time tickets or download the batch ZIP.</p>` +
		`<form method="GET" action="/gateway">
			<input type="hidden" name="view" value="files">
			<label>Area ID <input name="area" value="` + strconv.FormatInt(fileAreaID, 10) + `" size="6"></label>
			<label>Query <input name="q" value="` + htmlEscape(query) + `" size="24"></label>
			<label>Tags <input name="tags" value="` + htmlEscape(tagsRaw) + `" size="24" placeholder="tag1,tag2"></label>
			<button type="submit">search</button>
		</form>
		<table border="1"><tr><th>ID</th><th>Area</th><th>Name</th><th>Tags</th><th>Rating</th><th>Actions</th></tr>` + fileRows.String() + `</table>
		<h2>Saved Filters</h2>
		<form method="POST" action="/gateway">` + csrf + `
			<input type="hidden" name="action" value="save_filter">
			<label>Name <input name="name" size="16"></label>
			<label>Query <input name="query" size="24"></label>
			<label>Tags <input name="tags" size="24"></label>
			<button type="submit">save filter</button>
		</form>
		<table border="1"><tr><th>Name</th><th>Query</th><th>Tags</th></tr>` + filterRows.String() + `</table>
		<h2>Download Queue</h2>
		<p><a href="/gateway?view=files&batch=1">Download queue as ZIP</a></p>
		<table border="1"><tr><th>File</th><th>Queued</th><th>Actions</th></tr>` + queueRows.String() + `</table>
	</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) serveGatewayBatchZip(w http.ResponseWriter, r *http.Request, user *domain.User) bool {
	if strings.TrimSpace(r.URL.Query().Get("batch")) != "1" {
		return false
	}
	if a.adminRepo == nil {
		redirectWithError(w, r, "/gateway?view=files", "FileBase is unavailable.")
		return true
	}
	queue, err := a.adminRepo.ListDownloadQueue(user.ID, 500)
	if err != nil {
		redirectWithError(w, r, "/gateway?view=files", "Download queue is unavailable.")
		return true
	}
	if len(queue) == 0 {
		redirectWithError(w, r, "/gateway?view=files", "Download queue is empty.")
		return true
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="wolfbbs-batch-`+stamp+`.zip"`)
	zw := zip.NewWriter(w)
	nameCount := map[string]int{}
	for _, row := range queue {
		entry, err := a.adminRepo.GetFileEntry(row.FileID)
		if err != nil || entry == nil || !a.fileVisibleToCallers(entry.ID) {
			continue
		}
		path, err := a.resolveDownloadPath(entry)
		if err != nil {
			continue
		}
		name := filepath.Base(entry.Name)
		if name == "" || name == "." {
			name = fmt.Sprintf("file-%d.bin", entry.ID)
		}
		count := nameCount[name]
		nameCount[name] = count + 1
		if count > 0 {
			ext := filepath.Ext(name)
			stem := strings.TrimSuffix(name, ext)
			name = fmt.Sprintf("%s-%d%s", stem, count+1, ext)
		}
		fileHeader, err := zip.FileInfoHeader(fileInfoOrNil(path))
		if err == nil && fileHeader != nil {
			fileHeader.Name = name
			fileHeader.Method = zip.Deflate
		}
		var zipWriter io.Writer
		if fileHeader != nil {
			zipWriter, err = zw.CreateHeader(fileHeader)
		} else {
			zipWriter, err = zw.Create(name)
		}
		if err != nil {
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		_, _ = io.Copy(zipWriter, f)
		_ = f.Close()
		_ = a.adminRepo.DequeueDownload(user.ID, row.FileID)
	}
	_ = zw.Close()
	return true
}

func fileInfoOrNil(path string) os.FileInfo {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	return info
}

func (a *webApp) serveGatewayDownload(w http.ResponseWriter, r *http.Request, user *domain.User, token string) bool {
	if strings.TrimSpace(token) == "" {
		return false
	}
	if a.adminRepo == nil {
		http.Error(w, "filebase unavailable", http.StatusServiceUnavailable)
		return true
	}
	ticket, err := a.adminRepo.GetDownloadTicket(token, time.Now().UTC())
	if err != nil || ticket == nil {
		http.Error(w, "download ticket invalid or expired", http.StatusNotFound)
		return true
	}
	if ticket.UserID != user.ID && rbac.NormalizeRole(user.Role) != roleAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return true
	}
	entry, err := a.adminRepo.GetFileEntry(ticket.FileID)
	if err != nil || entry == nil {
		http.Error(w, "file entry not found", http.StatusNotFound)
		return true
	}
	if !a.fileVisibleToCallers(entry.ID) && rbac.NormalizeRole(user.Role) != roleAdmin {
		http.Error(w, "file still in review", http.StatusForbidden)
		return true
	}
	path, err := a.resolveDownloadPath(entry)
	if err != nil {
		http.Error(w, "file path unavailable", http.StatusNotFound)
		return true
	}
	_ = a.adminRepo.MarkDownloadTicketUsed(ticket.Token, time.Now().UTC())
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(entry.Name)+`"`)
	http.ServeFile(w, r, path)
	return true
}
