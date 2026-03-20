package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"wolfbbs/internal/domain"
)

type fileRequestItem struct {
	ID          string    `json:"id"`
	Requester   string    `json:"requester"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	DesiredArea int64     `json:"desired_area,omitempty"`
	Status      string    `json:"status"`
	Owner       string    `json:"owner,omitempty"`
	FileID      int64     `json:"file_id,omitempty"`
	Note        string    `json:"note,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ResolvedAt  time.Time `json:"resolved_at,omitempty"`
}

type uploadDraftItem struct {
	ID          string    `json:"id"`
	AreaID      int64     `json:"area_id,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags,omitempty"`
	SourceHint  string    `json:"source_hint,omitempty"`
	Reviewer    string    `json:"reviewer,omitempty"`
	Status      string    `json:"status"`
	Notes       string    `json:"notes,omitempty"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type fileCuratorNote struct {
	FileID    int64     `json:"file_id"`
	Curator   string    `json:"curator"`
	Note      string    `json:"note"`
	UpdatedAt time.Time `json:"updated_at"`
}

type featuredFileCollection struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	FileIDs     []int64   `json:"file_ids,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Curator     string    `json:"curator,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type fileRepairIssue struct {
	Kind   string
	FileID int64
	Label  string
	Detail string
	Action string
}

type uploaderTierInfo struct {
	Label    string
	Approved int
	Avg      float64
	Trusted  bool
}

type offlinePacket struct {
	GeneratedAt string               `json:"generated_at"`
	Handle      string               `json:"handle"`
	Boards      []offlinePacketBoard `json:"boards"`
}

type offlinePacketBoard struct {
	BoardID     int64                  `json:"board_id"`
	Name        string                 `json:"name"`
	Conference  string                 `json:"conference,omitempty"`
	MessageRows []offlinePacketMessage `json:"messages"`
}

type offlinePacketMessage struct {
	ID        int64  `json:"id"`
	Subject   string `json:"subject"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
	Body      string `json:"body"`
}

type offlineMailReply struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Urgency string `json:"urgency,omitempty"`
}

type offlineMailReplyPayload struct {
	Replies []offlineMailReply `json:"replies"`
}

func normalizeFileRequestStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "open", "claimed", "fulfilled", "closed":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "open"
	}
}

func normalizeUploadDraftStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "draft", "review", "ready", "approved", "rejected":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "draft"
	}
}

func normalizeFileList(raw string, limit int) []int64 {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeTagList(raw string, limit int) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t'
	})
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := strings.ToLower(strings.TrimSpace(part))
		clean = strings.TrimFunc(clean, func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' && r != '_'
		})
		if len(clean) < 2 {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	sort.Strings(out)
	return out
}

func (a *webApp) loadFileRequests() []fileRequestItem {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingFileRequests)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []fileRequestItem
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("files.requests", fmt.Errorf("decode file requests: %w", err))
		return nil
	}
	for i := range rows {
		rows[i].ID = strings.TrimSpace(rows[i].ID)
		rows[i].Requester = normalizeHandleKey(rows[i].Requester)
		rows[i].Owner = normalizeHandleKey(rows[i].Owner)
		rows[i].Status = normalizeFileRequestStatus(rows[i].Status)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	return rows
}

func (a *webApp) persistFileRequests(rows []fileRequestItem) {
	if a.adminRepo == nil {
		return
	}
	filtered := make([]fileRequestItem, 0, len(rows))
	for _, row := range rows {
		row.ID = strings.TrimSpace(row.ID)
		row.Title = strings.TrimSpace(row.Title)
		row.Description = strings.TrimSpace(row.Description)
		row.Requester = normalizeHandleKey(row.Requester)
		row.Owner = normalizeHandleKey(row.Owner)
		row.Status = normalizeFileRequestStatus(row.Status)
		if row.ID == "" || row.Requester == "" || row.Title == "" || row.Description == "" {
			continue
		}
		filtered = append(filtered, row)
	}
	body := ""
	if len(filtered) > 0 {
		raw, err := json.Marshal(filtered)
		if err != nil {
			a.addAppError("files.requests", fmt.Errorf("encode file requests: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingFileRequests, body)
}

func (a *webApp) upsertFileRequest(row fileRequestItem) {
	rows := a.loadFileRequests()
	if strings.TrimSpace(row.ID) == "" {
		row.ID = randomEventID()
		if row.CreatedAt.IsZero() {
			row.CreatedAt = time.Now().UTC()
		}
	}
	row.UpdatedAt = time.Now().UTC()
	for idx := range rows {
		if rows[idx].ID == row.ID {
			if row.CreatedAt.IsZero() {
				row.CreatedAt = rows[idx].CreatedAt
			}
			rows[idx] = row
			a.persistFileRequests(rows)
			return
		}
	}
	rows = append(rows, row)
	a.persistFileRequests(rows)
}

func (a *webApp) loadUploadDrafts() []uploadDraftItem {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingUploadDrafts)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []uploadDraftItem
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("files.drafts", fmt.Errorf("decode upload drafts: %w", err))
		return nil
	}
	for i := range rows {
		rows[i].ID = strings.TrimSpace(rows[i].ID)
		rows[i].Status = normalizeUploadDraftStatus(rows[i].Status)
		rows[i].CreatedBy = normalizeHandleKey(rows[i].CreatedBy)
		rows[i].Reviewer = normalizeHandleKey(rows[i].Reviewer)
		rows[i].Tags = normalizeTagList(strings.Join(rows[i].Tags, ","), 10)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UpdatedAt.Equal(rows[j].UpdatedAt) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
	})
	return rows
}

func (a *webApp) persistUploadDrafts(rows []uploadDraftItem) {
	filtered := make([]uploadDraftItem, 0, len(rows))
	for _, row := range rows {
		row.ID = strings.TrimSpace(row.ID)
		row.Name = strings.TrimSpace(row.Name)
		row.Description = strings.TrimSpace(row.Description)
		row.Status = normalizeUploadDraftStatus(row.Status)
		row.CreatedBy = normalizeHandleKey(row.CreatedBy)
		row.Reviewer = normalizeHandleKey(row.Reviewer)
		row.Tags = normalizeTagList(strings.Join(row.Tags, ","), 10)
		if row.ID == "" || row.Name == "" {
			continue
		}
		filtered = append(filtered, row)
	}
	body := ""
	if len(filtered) > 0 {
		raw, err := json.Marshal(filtered)
		if err != nil {
			a.addAppError("files.drafts", fmt.Errorf("encode upload drafts: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingUploadDrafts, body)
}

func (a *webApp) upsertUploadDraft(row uploadDraftItem) {
	rows := a.loadUploadDrafts()
	if strings.TrimSpace(row.ID) == "" {
		row.ID = randomEventID()
		if row.CreatedAt.IsZero() {
			row.CreatedAt = time.Now().UTC()
		}
	}
	row.UpdatedAt = time.Now().UTC()
	for idx := range rows {
		if rows[idx].ID == row.ID {
			if row.CreatedAt.IsZero() {
				row.CreatedAt = rows[idx].CreatedAt
			}
			rows[idx] = row
			a.persistUploadDrafts(rows)
			return
		}
	}
	rows = append(rows, row)
	a.persistUploadDrafts(rows)
}

func (a *webApp) deleteUploadDraft(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	rows := a.loadUploadDrafts()
	filtered := make([]uploadDraftItem, 0, len(rows))
	for _, row := range rows {
		if row.ID != id {
			filtered = append(filtered, row)
		}
	}
	a.persistUploadDrafts(filtered)
}

func (a *webApp) loadCuratorNotes() map[int64]fileCuratorNote {
	out := map[int64]fileCuratorNote{}
	if a.adminRepo == nil {
		return out
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingCuratorNotes)
	if err != nil || strings.TrimSpace(raw) == "" {
		return out
	}
	decoded := map[string]fileCuratorNote{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		a.addAppError("files.curator_notes", fmt.Errorf("decode curator notes: %w", err))
		return out
	}
	for rawID, row := range decoded {
		fileID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || fileID <= 0 {
			continue
		}
		row.FileID = fileID
		row.Curator = normalizeHandleKey(row.Curator)
		row.Note = strings.TrimSpace(row.Note)
		if row.Note == "" {
			continue
		}
		out[fileID] = row
	}
	return out
}

func (a *webApp) persistCuratorNotes(rows map[int64]fileCuratorNote) {
	encoded := map[string]fileCuratorNote{}
	for fileID, row := range rows {
		if fileID <= 0 || strings.TrimSpace(row.Note) == "" {
			continue
		}
		row.FileID = fileID
		row.Curator = normalizeHandleKey(row.Curator)
		encoded[strconv.FormatInt(fileID, 10)] = row
	}
	body := ""
	if len(encoded) > 0 {
		raw, err := json.Marshal(encoded)
		if err != nil {
			a.addAppError("files.curator_notes", fmt.Errorf("encode curator notes: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingCuratorNotes, body)
}

func (a *webApp) setCuratorNote(fileID int64, note fileCuratorNote) {
	if fileID <= 0 {
		return
	}
	rows := a.loadCuratorNotes()
	if strings.TrimSpace(note.Note) == "" {
		delete(rows, fileID)
	} else {
		note.FileID = fileID
		note.Curator = normalizeHandleKey(note.Curator)
		note.UpdatedAt = time.Now().UTC()
		rows[fileID] = note
	}
	a.persistCuratorNotes(rows)
}

func (a *webApp) loadFeaturedCollections() []featuredFileCollection {
	if a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingFeaturedCollections)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []featuredFileCollection
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("files.collections", fmt.Errorf("decode featured collections: %w", err))
		return nil
	}
	for i := range rows {
		rows[i].ID = strings.TrimSpace(rows[i].ID)
		rows[i].Title = strings.TrimSpace(rows[i].Title)
		rows[i].Description = strings.TrimSpace(rows[i].Description)
		rows[i].Curator = normalizeHandleKey(rows[i].Curator)
		rows[i].Tags = normalizeTagList(strings.Join(rows[i].Tags, ","), 8)
		rows[i].FileIDs = normalizeFileList(int64ListText(rows[i].FileIDs), 40)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UpdatedAt.Equal(rows[j].UpdatedAt) {
			return rows[i].Title < rows[j].Title
		}
		return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
	})
	return rows
}

func (a *webApp) persistFeaturedCollections(rows []featuredFileCollection) {
	filtered := make([]featuredFileCollection, 0, len(rows))
	for _, row := range rows {
		row.ID = strings.TrimSpace(row.ID)
		row.Title = strings.TrimSpace(row.Title)
		row.Description = strings.TrimSpace(row.Description)
		row.Curator = normalizeHandleKey(row.Curator)
		row.Tags = normalizeTagList(strings.Join(row.Tags, ","), 8)
		row.FileIDs = normalizeFileList(int64ListText(row.FileIDs), 40)
		if row.ID == "" || row.Title == "" {
			continue
		}
		filtered = append(filtered, row)
	}
	body := ""
	if len(filtered) > 0 {
		raw, err := json.Marshal(filtered)
		if err != nil {
			a.addAppError("files.collections", fmt.Errorf("encode featured collections: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingFeaturedCollections, body)
}

func (a *webApp) upsertFeaturedCollection(row featuredFileCollection) {
	rows := a.loadFeaturedCollections()
	if strings.TrimSpace(row.ID) == "" {
		row.ID = randomEventID()
	}
	row.UpdatedAt = time.Now().UTC()
	for idx := range rows {
		if rows[idx].ID == row.ID {
			rows[idx] = row
			a.persistFeaturedCollections(rows)
			return
		}
	}
	rows = append(rows, row)
	a.persistFeaturedCollections(rows)
}

func (a *webApp) deleteFeaturedCollection(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	rows := a.loadFeaturedCollections()
	filtered := make([]featuredFileCollection, 0, len(rows))
	for _, row := range rows {
		if row.ID != id {
			filtered = append(filtered, row)
		}
	}
	a.persistFeaturedCollections(filtered)
}

func int64ListText(values []int64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value > 0 {
			parts = append(parts, strconv.FormatInt(value, 10))
		}
	}
	return strings.Join(parts, ",")
}

func normalizedFileStem(name string) string {
	stem := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(name, filepath.Ext(name))))
	stem = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, stem)
	stem = strings.Trim(stem, "_")
	return stem
}

func (a *webApp) visibleFileEntries(limit int) []domain.FileEntry {
	if a.adminRepo == nil {
		return nil
	}
	if limit <= 0 {
		limit = 400
	}
	rows, err := a.adminRepo.ListFileEntries(0, "", nil, limit)
	if err != nil {
		return nil
	}
	return a.filterVisibleFiles(rows)
}

func (a *webApp) fileUploaderTier(userID int64) uploaderTierInfo {
	if userID <= 0 {
		return uploaderTierInfo{Label: "unknown"}
	}
	rows := a.visibleFileEntries(500)
	approved := 0
	totalRating := 0.0
	rated := 0
	for _, row := range rows {
		if row.UploaderID != userID {
			continue
		}
		approved++
		if row.RatingCount > 0 {
			totalRating += row.RatingAvg
			rated++
		}
	}
	avg := 0.0
	if rated > 0 {
		avg = totalRating / float64(rated)
	}
	info := uploaderTierInfo{Label: "newcomer", Approved: approved, Avg: avg}
	switch {
	case approved >= 8 && avg >= 4.25:
		info.Label = "curator"
		info.Trusted = true
	case approved >= 3 && avg >= 3.75:
		info.Label = "trusted"
		info.Trusted = true
	case approved >= 1:
		info.Label = "regular"
	}
	return info
}

func (a *webApp) fileDuplicateCandidates(entry *domain.FileEntry, limit int) []domain.FileEntry {
	if a.adminRepo == nil || entry == nil {
		return nil
	}
	if limit <= 0 {
		limit = 6
	}
	all, err := a.adminRepo.ListFileEntries(0, "", nil, 500)
	if err != nil {
		return nil
	}
	type scored struct {
		row   domain.FileEntry
		score int
	}
	entryStem := normalizedFileStem(entry.Name)
	tagSet := map[string]struct{}{}
	for _, tag := range entry.Tags {
		clean := strings.ToLower(strings.TrimSpace(tag))
		if clean != "" {
			tagSet[clean] = struct{}{}
		}
	}
	scoredRows := make([]scored, 0, len(all))
	for _, row := range all {
		if row.ID == entry.ID {
			continue
		}
		score := 0
		if strings.TrimSpace(entry.SHA256) != "" && strings.EqualFold(strings.TrimSpace(entry.SHA256), strings.TrimSpace(row.SHA256)) {
			score += 10
		}
		if entryStem != "" && entryStem == normalizedFileStem(row.Name) {
			score += 6
		}
		for _, tag := range row.Tags {
			if _, ok := tagSet[strings.ToLower(strings.TrimSpace(tag))]; ok {
				score += 2
			}
		}
		if score == 0 {
			continue
		}
		scoredRows = append(scoredRows, scored{row: row, score: score})
	}
	sort.Slice(scoredRows, func(i, j int) bool {
		if scoredRows[i].score != scoredRows[j].score {
			return scoredRows[i].score > scoredRows[j].score
		}
		return scoredRows[i].row.UploadedAt.After(scoredRows[j].row.UploadedAt)
	})
	out := make([]domain.FileEntry, 0, limit)
	for _, row := range scoredRows {
		out = append(out, row.row)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (a *webApp) relatedDoorsForFile(entry *domain.FileEntry, limit int) []string {
	if entry == nil || a.doorRegistry == nil {
		return nil
	}
	if limit <= 0 {
		limit = 4
	}
	tokens := normalizeTagList(strings.Join(entry.Tags, ",")+","+normalizedFileStem(entry.Name), 20)
	type scored struct {
		label string
		score int
	}
	scoredRows := make([]scored, 0, len(a.doorRegistry.Doors()))
	for _, door := range a.doorRegistry.Doors() {
		haystack := strings.ToLower(strings.Join([]string{door.ID, door.Name, door.Category, door.Description}, " "))
		score := 0
		for _, token := range tokens {
			if token != "" && strings.Contains(haystack, token) {
				score += 2
			}
		}
		if strings.Contains(strings.ToLower(door.Category), "file") {
			score++
		}
		if score == 0 {
			continue
		}
		scoredRows = append(scoredRows, scored{label: door.Name + " (" + door.ID + ")", score: score})
	}
	sort.Slice(scoredRows, func(i, j int) bool {
		if scoredRows[i].score != scoredRows[j].score {
			return scoredRows[i].score > scoredRows[j].score
		}
		return scoredRows[i].label < scoredRows[j].label
	})
	out := make([]string, 0, limit)
	for _, row := range scoredRows {
		out = append(out, row.label)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (a *webApp) relatedEventsForFile(entry *domain.FileEntry, limit int) []communityEvent {
	if entry == nil {
		return nil
	}
	if limit <= 0 {
		limit = 4
	}
	tokens := normalizeTagList(strings.Join(entry.Tags, ",")+","+normalizedFileStem(entry.Name), 20)
	type scored struct {
		row   communityEvent
		score int
	}
	scoredRows := make([]scored, 0, 8)
	for _, row := range a.upcomingCommunityEvents(24, time.Now().UTC()) {
		haystack := strings.ToLower(strings.Join([]string{row.Title, row.Description, row.Category, row.Location, row.Link}, " "))
		score := 0
		for _, token := range tokens {
			if token != "" && strings.Contains(haystack, token) {
				score += 2
			}
		}
		if strings.Contains(strings.ToLower(row.Link), "/doors") || strings.Contains(strings.ToLower(row.Link), "/scores") {
			score++
		}
		if score == 0 {
			continue
		}
		scoredRows = append(scoredRows, scored{row: row, score: score})
	}
	sort.Slice(scoredRows, func(i, j int) bool {
		if scoredRows[i].score != scoredRows[j].score {
			return scoredRows[i].score > scoredRows[j].score
		}
		return scoredRows[i].row.StartsAt.Before(scoredRows[j].row.StartsAt)
	})
	out := make([]communityEvent, 0, limit)
	for _, row := range scoredRows {
		out = append(out, row.row)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (a *webApp) collectionsContainingFile(fileID int64) []featuredFileCollection {
	if fileID <= 0 {
		return nil
	}
	out := []featuredFileCollection{}
	for _, row := range a.loadFeaturedCollections() {
		for _, candidate := range row.FileIDs {
			if candidate == fileID {
				out = append(out, row)
				break
			}
		}
	}
	return out
}

func (a *webApp) featuredCollectionsForDisplay() []featuredFileCollection {
	collections := a.loadFeaturedCollections()
	if len(collections) > 0 {
		return collections
	}
	files := a.visibleFileEntries(120)
	if len(files) == 0 {
		return nil
	}
	tagToFiles := map[string][]int64{}
	for _, row := range files {
		for _, tag := range row.Tags {
			clean := strings.ToLower(strings.TrimSpace(tag))
			if len(clean) < 3 {
				continue
			}
			tagToFiles[clean] = append(tagToFiles[clean], row.ID)
		}
	}
	type scored struct {
		tag   string
		count int
	}
	tags := make([]scored, 0, len(tagToFiles))
	for tag, ids := range tagToFiles {
		if len(ids) < 2 {
			continue
		}
		tags = append(tags, scored{tag: tag, count: len(ids)})
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].count != tags[j].count {
			return tags[i].count > tags[j].count
		}
		return tags[i].tag < tags[j].tag
	})
	out := make([]featuredFileCollection, 0, 4)
	for _, row := range tags {
		out = append(out, featuredFileCollection{
			ID:          "auto-" + row.tag,
			Title:       strings.Title(row.tag) + " Picks",
			Description: "Auto-curated from the most common visible file tags.",
			FileIDs:     normalizeFileList(int64ListText(tagToFiles[row.tag]), 6),
			Tags:        []string{row.tag},
			Curator:     "system",
			UpdatedAt:   time.Now().UTC(),
		})
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func (a *webApp) fileRepairIssues() []fileRepairIssue {
	issues := []fileRepairIssue{}
	if a.adminRepo == nil {
		return issues
	}
	areas, _ := a.adminRepo.ListFileAreas()
	areaSet := map[int64]bool{}
	for _, area := range areas {
		areaSet[area.ID] = true
	}
	files, _ := a.adminRepo.ListFileEntries(0, "", nil, 500)
	for _, row := range files {
		if row.AreaID > 0 && !areaSet[row.AreaID] {
			issues = append(issues, fileRepairIssue{Kind: "missing_area", FileID: row.ID, Label: row.Name, Detail: "File entry points at an area that no longer exists.", Action: "repair_remove_file"})
			continue
		}
		if strings.TrimSpace(row.Path) == "" {
			issues = append(issues, fileRepairIssue{Kind: "missing_path", FileID: row.ID, Label: row.Name, Detail: "File entry has no path.", Action: "repair_remove_file"})
			continue
		}
		if _, err := os.Stat(row.Path); err != nil {
			issues = append(issues, fileRepairIssue{Kind: "missing_file", FileID: row.ID, Label: row.Name, Detail: "Indexed file is missing on disk.", Action: "repair_remove_file"})
			continue
		}
		if strings.TrimSpace(row.SHA256) == "" {
			issues = append(issues, fileRepairIssue{Kind: "missing_sha", FileID: row.ID, Label: row.Name, Detail: "Indexed file is missing a SHA-256 hash.", Action: "repair_hash"})
		}
	}
	for fileID, row := range a.loadFileReviewQueue() {
		entry, err := a.adminRepo.GetFileEntry(fileID)
		if err == nil && entry != nil {
			continue
		}
		issues = append(issues, fileRepairIssue{Kind: "orphan_review", FileID: fileID, Label: row.Name, Detail: "Review queue row exists without a backing file entry.", Action: "repair_clear_review"})
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Kind != issues[j].Kind {
			return issues[i].Kind < issues[j].Kind
		}
		return issues[i].FileID < issues[j].FileID
	})
	return issues
}

func (a *webApp) buildOfflineBoardPacket(user *domain.User, limitBoards, limitMessages int) offlinePacket {
	packet := offlinePacket{GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if user == nil {
		return packet
	}
	packet.Handle = user.Handle
	if limitBoards <= 0 {
		limitBoards = 10
	}
	if limitMessages <= 0 {
		limitMessages = 12
	}
	boards, err := a.boardRepo.List()
	if err != nil {
		return packet
	}
	handles := a.userHandleLookup()
	selected := a.boardSubscriptionIDs(user.Handle, boardSubscriptionWatch, boardSubscriptionDigest)
	selectedBoards := make([]domain.Board, 0, len(boards))
	for _, board := range boards {
		if !a.canReadBoard(user, &board) {
			continue
		}
		if len(selected) > 0 {
			if !selected[board.ID] {
				continue
			}
		}
		selectedBoards = append(selectedBoards, board)
	}
	if len(selectedBoards) == 0 {
		for _, board := range boards {
			if a.canReadBoard(user, &board) {
				selectedBoards = append(selectedBoards, board)
			}
			if len(selectedBoards) >= limitBoards {
				break
			}
		}
	}
	for _, board := range selectedBoards {
		msgs, err := a.msgRepo.ListByBoard(board.ID)
		if err != nil || len(msgs) == 0 {
			continue
		}
		sort.Slice(msgs, func(i, j int) bool { return msgs[i].CreatedAt.After(msgs[j].CreatedAt) })
		rows := make([]offlinePacketMessage, 0, limitMessages)
		for _, msg := range msgs {
			rows = append(rows, offlinePacketMessage{
				ID:        msg.ID,
				Subject:   cleanOneLiner(msg.Subject, 96),
				Author:    handles[msg.AuthorID],
				CreatedAt: msg.CreatedAt.UTC().Format(time.RFC3339),
				Body:      msg.Body,
			})
			if len(rows) >= limitMessages {
				break
			}
		}
		if len(rows) == 0 {
			continue
		}
		packet.Boards = append(packet.Boards, offlinePacketBoard{
			BoardID:     board.ID,
			Name:        board.Name,
			Conference:  board.Conference,
			MessageRows: rows,
		})
		if len(packet.Boards) >= limitBoards {
			break
		}
	}
	return packet
}

func renderOfflinePacketText(packet offlinePacket) string {
	var out strings.Builder
	out.WriteString("WolfBBS Offline Packet\n")
	out.WriteString("Generated: " + packet.GeneratedAt + "\n")
	out.WriteString("Handle: " + packet.Handle + "\n\n")
	for _, board := range packet.Boards {
		out.WriteString("== [" + defaultConferenceValue(board.Conference) + "] " + board.Name + " ==\n")
		for _, msg := range board.MessageRows {
			out.WriteString("#" + strconv.FormatInt(msg.ID, 10) + " " + msg.Subject + "\n")
			out.WriteString("From: " + defaultIfBlank(msg.Author, "unknown") + " @ " + msg.CreatedAt + "\n")
			out.WriteString(msg.Body + "\n\n")
		}
	}
	if len(packet.Boards) == 0 {
		out.WriteString("No watched or digest boards have packet content yet.\n")
	}
	return out.String()
}

func parseOfflineReplyPayload(raw string) ([]offlineMailReply, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("payload is required")
	}
	var payload offlineMailReplyPayload
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &payload.Replies); err != nil {
			return nil, err
		}
	} else {
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, err
		}
	}
	if len(payload.Replies) == 0 {
		return nil, fmt.Errorf("no replies found")
	}
	return payload.Replies, nil
}

func (a *webApp) importOfflineMailReplies(user *domain.User, payload string) (int, error) {
	if user == nil {
		return 0, fmt.Errorf("user is required")
	}
	replies, err := parseOfflineReplyPayload(payload)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range replies {
		to := strings.TrimSpace(row.To)
		subject := strings.TrimSpace(row.Subject)
		body := strings.TrimSpace(row.Body)
		if to == "" || subject == "" || body == "" {
			continue
		}
		if strings.Contains(to, "@") {
			continue
		}
		target, err := a.authSvc.GetUser(to)
		if err != nil || target == nil || !directoryVisibleUser(target) {
			continue
		}
		msg := &domain.PrivateMail{
			FromUserID: user.ID,
			ToUserID:   target.ID,
			Subject:    applyMailUrgency(subject, normalizeMailUrgency(row.Urgency)),
			Body:       body,
		}
		if err := a.mailRepo.CreateMail(msg); err != nil {
			continue
		}
		count++
		if count >= 50 {
			break
		}
	}
	if count == 0 {
		return 0, fmt.Errorf("no valid local replies imported")
	}
	return count, nil
}

func (a *webApp) handleCollections(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if !a.canReadFiles(user, "browse") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	collections := a.featuredCollectionsForDisplay()
	files := a.visibleFileEntries(300)
	fileByID := map[int64]domain.FileEntry{}
	for _, row := range files {
		fileByID[row.ID] = row
	}
	cards := strings.Builder{}
	for _, row := range collections {
		items := strings.Builder{}
		count := 0
		for _, fileID := range row.FileIDs {
			entry, ok := fileByID[fileID]
			if !ok {
				continue
			}
			items.WriteString(`<li><a href="/gateway?view=files&id=` + strconv.FormatInt(entry.ID, 10) + `">` + htmlEscape(entry.Name) + `</a> <span class="wolfbbs-muted">` + htmlEscape(strings.Join(entry.Tags, ", ")) + `</span></li>`)
			count++
			if count >= 5 {
				break
			}
		}
		if items.Len() == 0 {
			items.WriteString(`<li>No visible files matched this collection yet.</li>`)
		}
		tagLine := "auto"
		if len(row.Tags) > 0 {
			tagLine = strings.Join(row.Tags, ", ")
		}
		cards.WriteString(`<article class="wolfbbs-card"><h2>` + htmlEscape(row.Title) + `</h2><p>` + htmlEscape(defaultIfBlank(row.Description, "Curated file bundle for callers who want a guided browse.")) + `</p><p class="wolfbbs-muted">Curator: ` + htmlEscape(defaultIfBlank(row.Curator, "system")) + ` | tags: ` + htmlEscape(tagLine) + `</p><ul>` + items.String() + `</ul><p><a href="/gateway?view=files&tags=` + url.QueryEscape(strings.Join(row.Tags, ",")) + `">Browse matching files</a></p></article>`)
	}
	if cards.Len() == 0 {
		cards.WriteString(`<article class="wolfbbs-card"><p>No featured collections yet. Use <a href="/admin/files">Files Admin</a> to curate one.</p></article>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Featured Collections</title></head><body>
<p><a href="/boards">boards</a> | <a href="/newfiles">newfiles</a> | <a href="/gateway?view=files">filebase</a> | <a href="/offline">offline</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Featured Collections</h1>
<p>Curated file bundles for callers who want a guided browse instead of raw directory listings.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(collections)) + `</strong><span>collections</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(files)) + `</strong><span>visible files</span></article></section>
<div class="wolfbbs-grid">` + cards.String() + `</div>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleOffline(w http.ResponseWriter, r *http.Request) {
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
		switch action {
		case "import_mail_replies":
			count, err := a.importOfflineMailReplies(user, r.FormValue("payload"))
			if err != nil {
				redirectWithError(w, r, "/offline", err.Error())
				return
			}
			redirectWithNotice(w, r, "/offline", fmt.Sprintf("Imported %d offline mail replie(s).", count))
			return
		default:
			redirectWithError(w, r, "/offline", "Unsupported offline action.")
			return
		}
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	packet := a.buildOfflineBoardPacket(user, 8, 10)
	if strings.TrimSpace(r.URL.Query().Get("download")) == "watched" {
		format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
		if format == "text" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"wolfbbs-watched-%s.txt\"", normalizeHandleKey(user.Handle)))
			_, _ = w.Write([]byte(renderOfflinePacketText(packet)))
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"wolfbbs-watched-%s.json\"", normalizeHandleKey(user.Handle)))
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(packet)
		return
	}
	previewRows := strings.Builder{}
	for _, board := range packet.Boards {
		previewRows.WriteString(`<li><strong>` + htmlEscape(board.Name) + `</strong> <span class="wolfbbs-muted">` + strconv.Itoa(len(board.MessageRows)) + ` messages</span></li>`)
	}
	if previewRows.Len() == 0 {
		previewRows.WriteString(`<li>No watched or digest boards are ready for an offline packet yet.</li>`)
	}
	samplePayload := `{
  "replies": [
    {
      "to": "sysop",
      "subject": "Re: packet catch-up",
      "body": "Reply drafted offline and imported later.",
      "urgency": "normal"
    }
  ]
}`
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Offline Center</title></head><body>
<p><a href="/boards">boards</a> | <a href="/mail">mail</a> | <a href="/gateway?view=files">filebase</a> | <a href="/collections">collections</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
` + pageMessageBlock(r) + `
<h1>Offline Center</h1>
<p>Build watched-board packets for asynchronous reading, then import local mail replies when you reconnect.</p>
<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(packet.Boards)) + `</strong><span>packet boards</span></article><article class="wolfbbs-kpi-card"><strong>` + normalizeHandleKey(user.Handle) + `</strong><span>packet owner</span></article></section>
<section class="wolfbbs-grid"><article class="wolfbbs-card"><h2>Watched Board Packet</h2><p><a href="/offline?download=watched&format=json">Download JSON packet</a> | <a href="/offline?download=watched&format=text">Download text packet</a></p><ul>` + previewRows.String() + `</ul></article><article class="wolfbbs-card"><h2>Offline Mail Reply Import</h2><form method="POST" action="/offline">` + a.csrfHiddenInput(r) + `<input type="hidden" name="action" value="import_mail_replies"><label>Reply payload<br><textarea name="payload" rows="14" cols="84">` + htmlEscape(samplePayload) + `</textarea></label><br><button type="submit">Import Replies</button></form></article></section>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) handleGatewayExtendedFileAction(w http.ResponseWriter, r *http.Request, user *domain.User) bool {
	action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
	if action != "request_file" {
		return false
	}
	redirectPath := safeLocalRedirectPath(r.FormValue("return_to"), "/gateway?view=files")
	title := strings.TrimSpace(r.FormValue("title"))
	description := strings.TrimSpace(r.FormValue("description"))
	desiredArea, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("desired_area")), 10, 64)
	if title == "" || description == "" {
		redirectWithError(w, r, redirectPath, "File request title and description are required.")
		return true
	}
	a.upsertFileRequest(fileRequestItem{
		Requester:   user.Handle,
		Title:       title,
		Description: description,
		DesiredArea: desiredArea,
		Status:      "open",
		CreatedAt:   time.Now().UTC(),
	})
	redirectWithNotice(w, r, redirectPath, "File request queued for review.")
	return true
}

func (a *webApp) adminFileWorkflowAction(user *domain.User, r *http.Request, redirectURL *string) bool {
	action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
	if action == "" {
		return false
	}
	now := time.Now().UTC()
	switch action {
	case "request_claim", "request_fulfill", "request_close":
		requestID := strings.TrimSpace(r.FormValue("request_id"))
		if requestID == "" {
			return true
		}
		rows := a.loadFileRequests()
		for idx := range rows {
			if rows[idx].ID != requestID {
				continue
			}
			rows[idx].UpdatedAt = now
			rows[idx].Owner = user.Handle
			rows[idx].Note = strings.TrimSpace(r.FormValue("note"))
			switch action {
			case "request_claim":
				rows[idx].Status = "claimed"
			case "request_fulfill":
				rows[idx].Status = "fulfilled"
				rows[idx].ResolvedAt = now
				rows[idx].FileID, _ = strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
			case "request_close":
				rows[idx].Status = "closed"
				rows[idx].ResolvedAt = now
			}
			break
		}
		a.persistFileRequests(rows)
		a.recordAdminAction(user.Handle, requestID, action, strings.TrimSpace(r.FormValue("note")))
		return true
	case "save_upload_draft":
		row := uploadDraftItem{
			ID:          strings.TrimSpace(r.FormValue("draft_id")),
			AreaID:      int64(parseInt(r.FormValue("draft_area_id"), 0)),
			Name:        strings.TrimSpace(r.FormValue("draft_name")),
			Description: strings.TrimSpace(r.FormValue("draft_description")),
			Tags:        normalizeTagList(r.FormValue("draft_tags"), 10),
			SourceHint:  strings.TrimSpace(r.FormValue("draft_source")),
			Reviewer:    strings.TrimSpace(r.FormValue("draft_reviewer")),
			Status:      normalizeUploadDraftStatus(r.FormValue("draft_status")),
			Notes:       strings.TrimSpace(r.FormValue("draft_notes")),
			CreatedBy:   user.Handle,
			CreatedAt:   now,
		}
		a.upsertUploadDraft(row)
		a.recordAdminAction(user.Handle, defaultIfBlank(row.ID, row.Name), "save_upload_draft", row.Status)
		return true
	case "delete_upload_draft":
		a.deleteUploadDraft(r.FormValue("draft_id"))
		a.recordAdminAction(user.Handle, strings.TrimSpace(r.FormValue("draft_id")), "delete_upload_draft", "")
		return true
	case "save_curator_note":
		fileID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
		if fileID > 0 {
			a.setCuratorNote(fileID, fileCuratorNote{Curator: user.Handle, Note: strings.TrimSpace(r.FormValue("curator_note"))})
			a.recordAdminAction(user.Handle, strconv.FormatInt(fileID, 10), "save_curator_note", "")
			if redirectURL != nil {
				*redirectURL = "/admin/files?view_file=" + strconv.FormatInt(fileID, 10)
			}
		}
		return true
	case "save_collection":
		row := featuredFileCollection{
			ID:          strings.TrimSpace(r.FormValue("collection_id")),
			Title:       strings.TrimSpace(r.FormValue("collection_title")),
			Description: strings.TrimSpace(r.FormValue("collection_description")),
			FileIDs:     normalizeFileList(r.FormValue("collection_file_ids"), 40),
			Tags:        normalizeTagList(r.FormValue("collection_tags"), 8),
			Curator:     user.Handle,
		}
		a.upsertFeaturedCollection(row)
		a.recordAdminAction(user.Handle, defaultIfBlank(row.ID, row.Title), "save_collection", row.Title)
		return true
	case "delete_collection":
		a.deleteFeaturedCollection(r.FormValue("collection_id"))
		a.recordAdminAction(user.Handle, strings.TrimSpace(r.FormValue("collection_id")), "delete_collection", "")
		return true
	case "repair_hash":
		fileID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
		if fileID > 0 && a.adminRepo != nil {
			if entry, err := a.adminRepo.GetFileEntry(fileID); err == nil && entry != nil {
				if sum, err := fileSHA256(entry.Path); err == nil {
					if info, statErr := os.Stat(entry.Path); statErr == nil {
						entry.SHA256 = sum
						entry.SizeBytes = info.Size()
						_ = a.adminRepo.UpsertFileEntry(entry)
						a.recordAdminAction(user.Handle, strconv.FormatInt(fileID, 10), "repair_hash", "")
					}
				}
			}
		}
		return true
	case "repair_remove_file":
		fileID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
		if fileID > 0 && a.adminRepo != nil {
			if entry, err := a.adminRepo.GetFileEntry(fileID); err == nil && entry != nil {
				_ = a.deleteIndexedFile(entry)
			}
			a.removeFileReviewItem(fileID)
			a.recordAdminAction(user.Handle, strconv.FormatInt(fileID, 10), "repair_remove_file", "")
		}
		return true
	case "repair_clear_review":
		fileID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("file_id")), 10, 64)
		if fileID > 0 {
			a.removeFileReviewItem(fileID)
			a.recordAdminAction(user.Handle, strconv.FormatInt(fileID, 10), "repair_clear_review", "")
		}
		return true
	default:
		return false
	}
}

func renderAdminRequestStatusOptions(current string) string {
	current = normalizeFileRequestStatus(current)
	options := []string{"open", "claimed", "fulfilled", "closed"}
	var out strings.Builder
	for _, option := range options {
		out.WriteString(`<option value="` + option + `"` + selectedIf(option == current) + `>` + option + `</option>`)
	}
	return out.String()
}

func renderUploadDraftStatusOptions(current string) string {
	current = normalizeUploadDraftStatus(current)
	options := []string{"draft", "review", "ready", "approved", "rejected"}
	var out strings.Builder
	for _, option := range options {
		out.WriteString(`<option value="` + option + `"` + selectedIf(option == current) + `>` + option + `</option>`)
	}
	return out.String()
}

func (a *webApp) renderAdminFileWorkflowSections(r *http.Request, user *domain.User, areaOptionRows string, areas map[int64]string) string {
	csrf := a.csrfHiddenInput(r)
	requests := a.loadFileRequests()
	requestRows := strings.Builder{}
	openRequests := 0
	for _, row := range requests {
		if row.Status == "open" || row.Status == "claimed" {
			openRequests++
		}
		fileLink := htmlEscape(defaultIfBlank(strconv.FormatInt(row.FileID, 10), "n/a"))
		if row.FileID > 0 {
			fileLink = `<a href="/gateway?view=files&id=` + strconv.FormatInt(row.FileID, 10) + `">` + strconv.FormatInt(row.FileID, 10) + `</a>`
		}
		requestRows.WriteString(`<tr><td>` + htmlEscape(row.Requester) + `</td><td>` + htmlEscape(row.Title) + `</td><td>` + htmlEscape(row.Description) + `</td><td>` + htmlEscape(defaultIfBlank(areas[row.DesiredArea], "any area")) + `</td><td>` + htmlEscape(row.Status) + `</td><td>` + htmlEscape(defaultIfBlank(row.Owner, "unassigned")) + `</td><td>` + fileLink + `</td><td><form method="POST" action="/admin/files" class="wolfbbs-inline-form">` + csrf + `<input type="hidden" name="action" value="request_claim"><input type="hidden" name="request_id" value="` + htmlEscape(row.ID) + `"><input name="note" size="18" value="` + htmlEscape(row.Note) + `" placeholder="owner note"><button type="submit">claim</button></form><form method="POST" action="/admin/files" class="wolfbbs-inline-form">` + csrf + `<input type="hidden" name="action" value="request_fulfill"><input type="hidden" name="request_id" value="` + htmlEscape(row.ID) + `"><input name="file_id" size="6" value="` + htmlEscape(defaultIfBlank(strconv.FormatInt(row.FileID, 10), "")) + `" placeholder="file id"><input name="note" size="14" value="` + htmlEscape(row.Note) + `" placeholder="resolution"><button type="submit">fulfill</button></form><form method="POST" action="/admin/files" class="wolfbbs-inline-form">` + csrf + `<input type="hidden" name="action" value="request_close"><input type="hidden" name="request_id" value="` + htmlEscape(row.ID) + `"><input name="note" size="14" value="` + htmlEscape(row.Note) + `" placeholder="close note"><button type="submit">close</button></form></td></tr>`)
	}
	if requestRows.Len() == 0 {
		requestRows.WriteString(`<tr><td colspan="8">No file requests yet.</td></tr>`)
	}
	drafts := a.loadUploadDrafts()
	draftRows := strings.Builder{}
	for _, row := range drafts {
		draftRows.WriteString(`<tr><td>` + htmlEscape(row.Name) + `</td><td>` + htmlEscape(defaultIfBlank(areas[row.AreaID], "any area")) + `</td><td>` + htmlEscape(row.Status) + `</td><td>` + htmlEscape(defaultIfBlank(row.Reviewer, "unassigned")) + `</td><td>` + htmlEscape(row.SourceHint) + `</td><td>` + htmlEscape(strings.Join(row.Tags, ",")) + `</td><td>` + htmlEscape(cleanOneLiner(row.Notes, 96)) + `</td><td><form method="POST" action="/admin/files" class="wolfbbs-inline-form">` + csrf + `<input type="hidden" name="action" value="delete_upload_draft"><input type="hidden" name="draft_id" value="` + htmlEscape(row.ID) + `"><button type="submit">delete</button></form></td></tr>`)
	}
	if draftRows.Len() == 0 {
		draftRows.WriteString(`<tr><td colspan="8">No upload drafts yet.</td></tr>`)
	}
	collections := a.featuredCollectionsForDisplay()
	collectionRows := strings.Builder{}
	for _, row := range collections {
		collectionRows.WriteString(`<tr><td>` + htmlEscape(row.Title) + `</td><td>` + htmlEscape(strings.Join(row.Tags, ",")) + `</td><td>` + htmlEscape(int64ListText(row.FileIDs)) + `</td><td>` + htmlEscape(defaultIfBlank(row.Curator, "system")) + `</td><td><a href="/collections">open</a> <form method="POST" action="/admin/files" class="wolfbbs-inline-form">` + csrf + `<input type="hidden" name="action" value="delete_collection"><input type="hidden" name="collection_id" value="` + htmlEscape(row.ID) + `"><button type="submit">delete</button></form></td></tr>`)
	}
	if collectionRows.Len() == 0 {
		collectionRows.WriteString(`<tr><td colspan="5">No featured collections yet.</td></tr>`)
	}
	repairIssues := a.fileRepairIssues()
	repairRows := strings.Builder{}
	for _, row := range repairIssues {
		repairRows.WriteString(`<tr><td>` + htmlEscape(row.Kind) + `</td><td>` + strconv.FormatInt(row.FileID, 10) + `</td><td>` + htmlEscape(row.Label) + `</td><td>` + htmlEscape(row.Detail) + `</td><td><form method="POST" action="/admin/files">` + csrf + `<input type="hidden" name="action" value="` + htmlEscape(row.Action) + `"><input type="hidden" name="file_id" value="` + strconv.FormatInt(row.FileID, 10) + `"><button type="submit">run</button></form></td></tr>`)
	}
	if repairRows.Len() == 0 {
		repairRows.WriteString(`<tr><td colspan="5">No repair issues detected.</td></tr>`)
	}
	summary := buildFileWorkflowSummary(requests, drafts, a.loadFileReviewQueue(), collections, repairIssues)
	return renderFileCuratorLane(summary) +
		`<section class="wolfbbs-kpi-grid"><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(openRequests) + `</strong><span>open file requests</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(drafts)) + `</strong><span>upload drafts</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(collections)) + `</strong><span>featured collections</span></article><article class="wolfbbs-kpi-card"><strong>` + strconv.Itoa(len(repairIssues)) + `</strong><span>repair issues</span></article></section>` +
		`<h2>File Request Queue</h2><table border="1"><tr><th>Requester</th><th>Title</th><th>Description</th><th>Desired Area</th><th>Status</th><th>Owner</th><th>File</th><th>Actions</th></tr>` + requestRows.String() + `</table>` +
		`<h2>Upload Drafts</h2><form method="POST" action="/admin/files">` + csrf + `<input type="hidden" name="action" value="save_upload_draft"><label>Name <input name="draft_name" size="22"></label> <label>Area <select name="draft_area_id"><option value="0">any area</option>` + areaOptionRows + `</select></label> <label>Status <select name="draft_status">` + renderUploadDraftStatusOptions("draft") + `</select></label> <label>Reviewer <input name="draft_reviewer" size="14"></label><br><label>Description <input name="draft_description" size="42"></label> <label>Tags <input name="draft_tags" size="18" placeholder="ansi,retro"></label> <label>Source <input name="draft_source" size="18" placeholder="request, upload, event"></label><br><label>Notes <input name="draft_notes" size="52" placeholder="prep work, missing file, waiting on scan"></label> <button type="submit">Save Draft</button></form><table border="1"><tr><th>Name</th><th>Area</th><th>Status</th><th>Reviewer</th><th>Source</th><th>Tags</th><th>Notes</th><th>Action</th></tr>` + draftRows.String() + `</table>` +
		`<h2>Featured Collections</h2><form method="POST" action="/admin/files">` + csrf + `<input type="hidden" name="action" value="save_collection"><label>Title <input name="collection_title" size="24"></label> <label>Tags <input name="collection_tags" size="18" placeholder="ansi,docs"></label> <label>File IDs <input name="collection_file_ids" size="22" placeholder="1,2,3"></label><br><label>Description <input name="collection_description" size="64" placeholder="What this bundle is for and why callers should open it"></label> <button type="submit">Save Collection</button></form><table border="1"><tr><th>Title</th><th>Tags</th><th>Files</th><th>Curator</th><th>Action</th></tr>` + collectionRows.String() + `</table>` +
		`<h2>Recovery Assistant</h2><table border="1"><tr><th>Kind</th><th>File ID</th><th>Label</th><th>Detail</th><th>Action</th></tr>` + repairRows.String() + `</table>`
}
