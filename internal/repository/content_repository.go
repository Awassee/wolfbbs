package repository

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"wolfbbs/internal/domain"
)

var ErrNotFound = errors.New("not found")

type BoardRepository interface {
	Create(board *domain.Board) error
	Get(id int64) (*domain.Board, error)
	List() ([]domain.Board, error)
	Update(board *domain.Board) error
	Delete(id int64) error
}

type InMemoryBoardRepository struct {
	mu     sync.Mutex
	nextID int64
	byID   map[int64]domain.Board
}

func NewInMemoryBoardRepository() *InMemoryBoardRepository {
	return &InMemoryBoardRepository{
		nextID: 1,
		byID:   map[int64]domain.Board{},
	}
}

func (r *InMemoryBoardRepository) Create(board *domain.Board) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if board == nil {
		return errors.New("board is required")
	}
	name := strings.TrimSpace(board.Name)
	if name == "" {
		return errors.New("board name is required")
	}
	board.Name = name
	board.Conference = defaultConference(board.Conference)
	board.ReadACS = strings.TrimSpace(board.ReadACS)
	board.WriteACS = strings.TrimSpace(board.WriteACS)
	if board.CreatedAt.IsZero() {
		board.CreatedAt = time.Now().UTC()
	}
	board.ID = r.nextID
	r.nextID++
	r.byID[board.ID] = *board
	return nil
}

func (r *InMemoryBoardRepository) Get(id int64) (*domain.Board, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	board, ok := r.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	out := board
	return &out, nil
}

func (r *InMemoryBoardRepository) List() ([]domain.Board, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Board, 0, len(r.byID))
	for _, board := range r.byID {
		out = append(out, board)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *InMemoryBoardRepository) Update(board *domain.Board) error {
	if board == nil {
		return errors.New("board is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.byID[board.ID]
	if !ok {
		return ErrNotFound
	}
	name := strings.TrimSpace(board.Name)
	if name == "" {
		return errors.New("board name is required")
	}
	existing.Name = name
	existing.Description = strings.TrimSpace(board.Description)
	existing.Conference = defaultConference(board.Conference)
	existing.ReadACS = strings.TrimSpace(board.ReadACS)
	existing.WriteACS = strings.TrimSpace(board.WriteACS)
	r.byID[board.ID] = existing
	return nil
}

func (r *InMemoryBoardRepository) Delete(id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return ErrNotFound
	}
	delete(r.byID, id)
	return nil
}

type PrivateMailRepository interface {
	CreateMail(mail *domain.PrivateMail) error
	GetMail(id int64) (*domain.PrivateMail, error)
	ListInbox(userID int64, limit int) ([]domain.PrivateMail, error)
	ListOutbox(userID int64, limit int) ([]domain.PrivateMail, error)
	MarkRead(id int64, readAt time.Time) error
	DeleteMail(id int64) error
}

type InMemoryPrivateMailRepository struct {
	mu     sync.Mutex
	nextID int64
	byID   map[int64]domain.PrivateMail
}

func NewInMemoryPrivateMailRepository() *InMemoryPrivateMailRepository {
	return &InMemoryPrivateMailRepository{
		nextID: 1,
		byID:   map[int64]domain.PrivateMail{},
	}
}

func (r *InMemoryPrivateMailRepository) CreateMail(mail *domain.PrivateMail) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if mail == nil {
		return errors.New("mail is required")
	}
	if strings.TrimSpace(mail.Subject) == "" {
		return errors.New("mail subject is required")
	}
	if strings.TrimSpace(mail.Body) == "" {
		return errors.New("mail body is required")
	}
	if mail.FromUserID <= 0 {
		return errors.New("from user id is required")
	}
	if mail.ToUserID <= 0 && mail.ExternalTo == nil {
		return errors.New("to user id or external recipient is required")
	}
	if mail.CreatedAt.IsZero() {
		mail.CreatedAt = time.Now().UTC()
	}
	mail.ID = r.nextID
	r.nextID++
	r.byID[mail.ID] = *mail
	return nil
}

func (r *InMemoryPrivateMailRepository) GetMail(id int64) (*domain.PrivateMail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	out := row
	return &out, nil
}

func (r *InMemoryPrivateMailRepository) ListInbox(userID int64, limit int) ([]domain.PrivateMail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 {
		limit = 100
	}
	out := make([]domain.PrivateMail, 0)
	for _, row := range r.byID {
		if row.ToUserID == userID {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryPrivateMailRepository) ListOutbox(userID int64, limit int) ([]domain.PrivateMail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 {
		limit = 100
	}
	out := make([]domain.PrivateMail, 0)
	for _, row := range r.byID {
		if row.FromUserID == userID {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryPrivateMailRepository) MarkRead(id int64, readAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.byID[id]
	if !ok {
		return ErrNotFound
	}
	if readAt.IsZero() {
		readAt = time.Now().UTC()
	}
	row.ReadAt = &readAt
	r.byID[id] = row
	return nil
}

func (r *InMemoryPrivateMailRepository) DeleteMail(id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return ErrNotFound
	}
	delete(r.byID, id)
	return nil
}

type AdminRepository interface {
	AddAudit(entry *domain.AdminAudit) error
	ListAudit(limit int) ([]domain.AdminAudit, error)
	SetMailOutboundPolicy(handle string, disabled bool) error
	GetMailOutboundPolicy(handle string) (*domain.MailOutboundPolicy, error)
	ListMailOutboundPolicies() ([]domain.MailOutboundPolicy, error)
	UpsertGatewaySettings(settings *domain.GatewaySettings) error
	GetGatewaySettings() (*domain.GatewaySettings, error)
	UpsertSystemSetting(key, value string) error
	GetSystemSetting(key string) (string, error)
	ListSystemSettings() (map[string]string, error)
	UpsertNodeSession(session *domain.NodeSession) error
	DeleteNodeSession(sessionID string) error
	ListNodeSessions(limit int) ([]domain.NodeSession, error)
	AddCallerHistory(entry *domain.CallerHistory) error
	ListCallerHistory(limit int) ([]domain.CallerHistory, error)
	ListFileAreas() ([]domain.FileArea, error)
	CreateFileArea(area *domain.FileArea) error
	DeleteFileArea(id int64) error
	GetFileEntry(id int64) (*domain.FileEntry, error)
	UpsertFileEntry(entry *domain.FileEntry) error
	DeleteFileEntry(id int64) error
	ListFileEntries(areaID int64, query string, tags []string, limit int) ([]domain.FileEntry, error)
	SetFileRating(userID, fileID int64, rating int) error
	SaveFileFilter(filter *domain.FileFilter) error
	ListFileFilters(userID int64) ([]domain.FileFilter, error)
	EnqueueDownload(userID, fileID int64) error
	ListDownloadQueue(userID int64, limit int) ([]domain.DownloadQueueItem, error)
	DequeueDownload(userID, fileID int64) error
	CreateDownloadTicket(ticket *domain.DownloadTicket) error
	GetDownloadTicket(token string, now time.Time) (*domain.DownloadTicket, error)
	MarkDownloadTicketUsed(token string, usedAt time.Time) error
}

type InMemoryAdminRepository struct {
	mu              sync.Mutex
	nextAuditID     int64
	nextFileAreaID  int64
	audit           []domain.AdminAudit
	policies        map[string]domain.MailOutboundPolicy
	gatewaySettings domain.GatewaySettings
	hasGatewayCfg   bool
	systemSettings  map[string]string
	nodeSessions    map[string]domain.NodeSession
	callerHistory   []domain.CallerHistory
	nextCallerID    int64
	fileAreas       map[int64]domain.FileArea
	nextFileEntryID int64
	nextFilterID    int64
	nextQueueID     int64
	fileEntries     map[int64]domain.FileEntry
	fileRatings     map[[2]int64]int
	fileFilters     map[int64]domain.FileFilter
	downloadQueue   map[int64]domain.DownloadQueueItem
	downloadTickets map[string]domain.DownloadTicket
}

func NewInMemoryAdminRepository() *InMemoryAdminRepository {
	return &InMemoryAdminRepository{
		nextAuditID:     1,
		nextFileAreaID:  1,
		audit:           []domain.AdminAudit{},
		policies:        map[string]domain.MailOutboundPolicy{},
		systemSettings:  map[string]string{},
		nodeSessions:    map[string]domain.NodeSession{},
		callerHistory:   []domain.CallerHistory{},
		nextCallerID:    1,
		fileAreas:       map[int64]domain.FileArea{},
		nextFileEntryID: 1,
		nextFilterID:    1,
		nextQueueID:     1,
		fileEntries:     map[int64]domain.FileEntry{},
		fileRatings:     map[[2]int64]int{},
		fileFilters:     map[int64]domain.FileFilter{},
		downloadQueue:   map[int64]domain.DownloadQueueItem{},
		downloadTickets: map[string]domain.DownloadTicket{},
	}
}

func (r *InMemoryAdminRepository) AddAudit(entry *domain.AdminAudit) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry == nil {
		return errors.New("audit entry is required")
	}
	entry.ID = r.nextAuditID
	r.nextAuditID++
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	r.audit = append(r.audit, *entry)
	if len(r.audit) > 2000 {
		r.audit = r.audit[len(r.audit)-2000:]
	}
	return nil
}

func (r *InMemoryAdminRepository) ListAudit(limit int) ([]domain.AdminAudit, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := len(r.audit)
	if limit <= 0 || limit > total {
		limit = total
	}
	start := total - limit
	if start < 0 {
		start = 0
	}
	out := make([]domain.AdminAudit, 0, limit)
	for i := total - 1; i >= start; i-- {
		out = append(out, r.audit[i])
	}
	return out, nil
}

func (r *InMemoryAdminRepository) SetMailOutboundPolicy(handle string, disabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	handle = strings.ToLower(strings.TrimSpace(handle))
	if handle == "" {
		return errors.New("handle is required")
	}
	row := domain.MailOutboundPolicy{
		Handle:           handle,
		OutboundDisabled: disabled,
		UpdatedAt:        time.Now().UTC(),
	}
	r.policies[handle] = row
	return nil
}

func (r *InMemoryAdminRepository) GetMailOutboundPolicy(handle string) (*domain.MailOutboundPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	handle = strings.ToLower(strings.TrimSpace(handle))
	row, ok := r.policies[handle]
	if !ok {
		return nil, ErrNotFound
	}
	out := row
	return &out, nil
}

func (r *InMemoryAdminRepository) ListMailOutboundPolicies() ([]domain.MailOutboundPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.MailOutboundPolicy, 0, len(r.policies))
	for _, row := range r.policies {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Handle < out[j].Handle })
	return out, nil
}

func (r *InMemoryAdminRepository) UpsertGatewaySettings(settings *domain.GatewaySettings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if settings == nil {
		return errors.New("settings are required")
	}
	cfg := *settings
	cfg.UpdatedAt = time.Now().UTC()
	r.gatewaySettings = cfg
	r.hasGatewayCfg = true
	return nil
}

func (r *InMemoryAdminRepository) GetGatewaySettings() (*domain.GatewaySettings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.hasGatewayCfg {
		return nil, ErrNotFound
	}
	cfg := r.gatewaySettings
	return &cfg, nil
}

func (r *InMemoryAdminRepository) UpsertSystemSetting(key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return errors.New("setting key is required")
	}
	r.systemSettings[key] = value
	return nil
}

func (r *InMemoryAdminRepository) GetSystemSetting(key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return "", ErrNotFound
	}
	value, ok := r.systemSettings[key]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (r *InMemoryAdminRepository) ListSystemSettings() (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.systemSettings))
	for k, v := range r.systemSettings {
		out[k] = v
	}
	return out, nil
}

func (r *InMemoryAdminRepository) UpsertNodeSession(session *domain.NodeSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if session == nil {
		return errors.New("node session is required")
	}
	sessionID := strings.TrimSpace(session.SessionID)
	if sessionID == "" {
		return errors.New("session id is required")
	}
	copy := *session
	copy.SessionID = sessionID
	if copy.UpdatedAt.IsZero() {
		copy.UpdatedAt = time.Now().UTC()
	}
	r.nodeSessions[sessionID] = copy
	return nil
}

func (r *InMemoryAdminRepository) DeleteNodeSession(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ErrNotFound
	}
	if _, ok := r.nodeSessions[sessionID]; !ok {
		return ErrNotFound
	}
	delete(r.nodeSessions, sessionID)
	return nil
}

func (r *InMemoryAdminRepository) ListNodeSessions(limit int) ([]domain.NodeSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.NodeSession, 0, len(r.nodeSessions))
	for _, row := range r.nodeSessions {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].NodeID == out[j].NodeID {
			return out[i].SessionID < out[j].SessionID
		}
		return out[i].NodeID < out[j].NodeID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryAdminRepository) AddCallerHistory(entry *domain.CallerHistory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry == nil {
		return errors.New("caller history entry is required")
	}
	copy := *entry
	if copy.CreatedAt.IsZero() {
		copy.CreatedAt = time.Now().UTC()
	}
	copy.ID = r.nextCallerID
	r.nextCallerID++
	r.callerHistory = append([]domain.CallerHistory{copy}, r.callerHistory...)
	if len(r.callerHistory) > 2000 {
		r.callerHistory = r.callerHistory[:2000]
	}
	return nil
}

func (r *InMemoryAdminRepository) ListCallerHistory(limit int) ([]domain.CallerHistory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 || limit > len(r.callerHistory) {
		limit = len(r.callerHistory)
	}
	out := make([]domain.CallerHistory, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, r.callerHistory[i])
	}
	return out, nil
}

func (r *InMemoryAdminRepository) ListFileAreas() ([]domain.FileArea, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.FileArea, 0, len(r.fileAreas))
	for _, area := range r.fileAreas {
		out = append(out, area)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *InMemoryAdminRepository) CreateFileArea(area *domain.FileArea) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if area == nil {
		return errors.New("file area is required")
	}
	area.Name = strings.TrimSpace(area.Name)
	if area.Name == "" {
		return errors.New("file area name is required")
	}
	area.Path = strings.TrimSpace(area.Path)
	if area.Path == "" {
		return errors.New("file area path is required")
	}
	area.ID = r.nextFileAreaID
	r.nextFileAreaID++
	if area.CreatedAt.IsZero() {
		area.CreatedAt = time.Now().UTC()
	}
	r.fileAreas[area.ID] = *area
	return nil
}

func (r *InMemoryAdminRepository) DeleteFileArea(id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.fileAreas[id]; !ok {
		return ErrNotFound
	}
	delete(r.fileAreas, id)
	return nil
}

func (r *InMemoryAdminRepository) GetFileEntry(id int64) (*domain.FileEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.fileEntries[id]
	if !ok {
		return nil, ErrNotFound
	}
	ratingSum := 0
	ratingCount := 0
	for key, score := range r.fileRatings {
		if key[1] != row.ID {
			continue
		}
		ratingSum += score
		ratingCount++
	}
	if ratingCount > 0 {
		row.RatingAvg = float64(ratingSum) / float64(ratingCount)
		row.RatingCount = ratingCount
	}
	copy := row
	return &copy, nil
}

func (r *InMemoryAdminRepository) UpsertFileEntry(entry *domain.FileEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry == nil {
		return errors.New("file entry is required")
	}
	entry.Name = strings.TrimSpace(entry.Name)
	entry.Path = strings.TrimSpace(entry.Path)
	entry.Description = strings.TrimSpace(entry.Description)
	entry.SHA256 = strings.ToLower(strings.TrimSpace(entry.SHA256))
	if entry.Name == "" || entry.Path == "" {
		return errors.New("file name and path are required")
	}
	if entry.AreaID <= 0 {
		return errors.New("file area id is required")
	}
	now := time.Now().UTC()
	if entry.UploadedAt.IsZero() {
		entry.UploadedAt = now
	}
	for id, existing := range r.fileEntries {
		if entry.ID > 0 && existing.ID == entry.ID {
			entry.CreatedAt = existing.CreatedAt
			entry.UpdatedAt = now
			r.fileEntries[id] = *entry
			return nil
		}
		if entry.SHA256 != "" && existing.SHA256 == entry.SHA256 {
			entry.ID = existing.ID
			entry.CreatedAt = existing.CreatedAt
			entry.UpdatedAt = now
			r.fileEntries[id] = *entry
			return nil
		}
	}
	entry.ID = r.nextFileEntryID
	r.nextFileEntryID++
	entry.CreatedAt = now
	entry.UpdatedAt = now
	r.fileEntries[entry.ID] = *entry
	return nil
}

func (r *InMemoryAdminRepository) DeleteFileEntry(id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.fileEntries[id]; !ok {
		return ErrNotFound
	}
	delete(r.fileEntries, id)
	for key := range r.fileRatings {
		if key[1] == id {
			delete(r.fileRatings, key)
		}
	}
	for queueID, row := range r.downloadQueue {
		if row.FileID == id {
			delete(r.downloadQueue, queueID)
		}
	}
	for token, row := range r.downloadTickets {
		if row.FileID == id {
			delete(r.downloadTickets, token)
		}
	}
	return nil
}

func (r *InMemoryAdminRepository) ListFileEntries(areaID int64, query string, tags []string, limit int) ([]domain.FileEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	query = strings.ToLower(strings.TrimSpace(query))
	tagSet := map[string]struct{}{}
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		tagSet[tag] = struct{}{}
	}
	out := make([]domain.FileEntry, 0, len(r.fileEntries))
	for _, row := range r.fileEntries {
		if areaID > 0 && row.AreaID != areaID {
			continue
		}
		if query != "" {
			hay := strings.ToLower(row.Name + " " + row.Description + " " + row.Path + " " + strings.Join(row.Tags, " "))
			if !strings.Contains(hay, query) {
				continue
			}
		}
		if len(tagSet) > 0 {
			match := true
			owned := map[string]struct{}{}
			for _, tag := range row.Tags {
				owned[strings.ToLower(strings.TrimSpace(tag))] = struct{}{}
			}
			for tag := range tagSet {
				if _, ok := owned[tag]; !ok {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}
		ratingSum := 0
		ratingCount := 0
		for key, score := range r.fileRatings {
			if key[1] != row.ID {
				continue
			}
			ratingSum += score
			ratingCount++
		}
		if ratingCount > 0 {
			row.RatingAvg = float64(ratingSum) / float64(ratingCount)
			row.RatingCount = ratingCount
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UploadedAt.Equal(out[j].UploadedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].UploadedAt.After(out[j].UploadedAt)
	})
	if limit <= 0 {
		limit = 200
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryAdminRepository) SetFileRating(userID, fileID int64, rating int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if userID <= 0 || fileID <= 0 {
		return errors.New("user id and file id are required")
	}
	if rating < 1 || rating > 5 {
		return errors.New("rating must be between 1 and 5")
	}
	if _, ok := r.fileEntries[fileID]; !ok {
		return ErrNotFound
	}
	r.fileRatings[[2]int64{userID, fileID}] = rating
	return nil
}

func (r *InMemoryAdminRepository) SaveFileFilter(filter *domain.FileFilter) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if filter == nil {
		return errors.New("file filter is required")
	}
	if filter.UserID <= 0 {
		return errors.New("user id is required")
	}
	filter.Name = strings.TrimSpace(filter.Name)
	if filter.Name == "" {
		return errors.New("filter name is required")
	}
	now := time.Now().UTC()
	if filter.ID > 0 {
		existing, ok := r.fileFilters[filter.ID]
		if !ok || existing.UserID != filter.UserID {
			return ErrNotFound
		}
		filter.CreatedAt = existing.CreatedAt
		filter.UpdatedAt = now
		r.fileFilters[filter.ID] = *filter
		return nil
	}
	filter.ID = r.nextFilterID
	r.nextFilterID++
	filter.CreatedAt = now
	filter.UpdatedAt = now
	r.fileFilters[filter.ID] = *filter
	return nil
}

func (r *InMemoryAdminRepository) ListFileFilters(userID int64) ([]domain.FileFilter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.FileFilter, 0)
	for _, row := range r.fileFilters {
		if userID > 0 && row.UserID != userID {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *InMemoryAdminRepository) EnqueueDownload(userID, fileID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if userID <= 0 || fileID <= 0 {
		return errors.New("user id and file id are required")
	}
	if _, ok := r.fileEntries[fileID]; !ok {
		return ErrNotFound
	}
	for _, row := range r.downloadQueue {
		if row.UserID == userID && row.FileID == fileID {
			return nil
		}
	}
	now := time.Now().UTC()
	row := domain.DownloadQueueItem{
		ID:        r.nextQueueID,
		UserID:    userID,
		FileID:    fileID,
		CreatedAt: now,
	}
	r.nextQueueID++
	r.downloadQueue[row.ID] = row
	return nil
}

func (r *InMemoryAdminRepository) ListDownloadQueue(userID int64, limit int) ([]domain.DownloadQueueItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.DownloadQueueItem, 0)
	for _, row := range r.downloadQueue {
		if userID > 0 && row.UserID != userID {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryAdminRepository) DequeueDownload(userID, fileID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, row := range r.downloadQueue {
		if row.UserID == userID && row.FileID == fileID {
			delete(r.downloadQueue, id)
			return nil
		}
	}
	return ErrNotFound
}

func (r *InMemoryAdminRepository) CreateDownloadTicket(ticket *domain.DownloadTicket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ticket == nil {
		return errors.New("download ticket is required")
	}
	ticket.Token = strings.TrimSpace(ticket.Token)
	if ticket.Token == "" || ticket.UserID <= 0 || ticket.FileID <= 0 {
		return errors.New("ticket token, user id, and file id are required")
	}
	now := time.Now().UTC()
	if ticket.CreatedAt.IsZero() {
		ticket.CreatedAt = now
	}
	r.downloadTickets[ticket.Token] = *ticket
	return nil
}

func (r *InMemoryAdminRepository) GetDownloadTicket(token string, now time.Time) (*domain.DownloadTicket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrNotFound
	}
	row, ok := r.downloadTickets[token]
	if !ok {
		return nil, ErrNotFound
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if row.ExpiresAt.Before(now.UTC()) || row.UsedAt != nil {
		return nil, ErrNotFound
	}
	copy := row
	return &copy, nil
}

func (r *InMemoryAdminRepository) MarkDownloadTicketUsed(token string, usedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrNotFound
	}
	row, ok := r.downloadTickets[token]
	if !ok {
		return ErrNotFound
	}
	if usedAt.IsZero() {
		usedAt = time.Now().UTC()
	}
	row.UsedAt = &usedAt
	r.downloadTickets[token] = row
	return nil
}

var _ BoardRepository = (*InMemoryBoardRepository)(nil)
var _ PrivateMailRepository = (*InMemoryPrivateMailRepository)(nil)
var _ AdminRepository = (*InMemoryAdminRepository)(nil)
