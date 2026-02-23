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

type AdminRepository interface {
	AddAudit(entry *domain.AdminAudit) error
	ListAudit(limit int) ([]domain.AdminAudit, error)
	SetMailOutboundPolicy(handle string, disabled bool) error
	GetMailOutboundPolicy(handle string) (*domain.MailOutboundPolicy, error)
	ListMailOutboundPolicies() ([]domain.MailOutboundPolicy, error)
	UpsertGatewaySettings(settings *domain.GatewaySettings) error
	GetGatewaySettings() (*domain.GatewaySettings, error)
	ListFileAreas() ([]domain.FileArea, error)
	CreateFileArea(area *domain.FileArea) error
	DeleteFileArea(id int64) error
}

type InMemoryAdminRepository struct {
	mu              sync.Mutex
	nextAuditID     int64
	nextFileAreaID  int64
	audit           []domain.AdminAudit
	policies        map[string]domain.MailOutboundPolicy
	gatewaySettings domain.GatewaySettings
	hasGatewayCfg   bool
	fileAreas       map[int64]domain.FileArea
}

func NewInMemoryAdminRepository() *InMemoryAdminRepository {
	return &InMemoryAdminRepository{
		nextAuditID:    1,
		nextFileAreaID: 1,
		audit:          []domain.AdminAudit{},
		policies:       map[string]domain.MailOutboundPolicy{},
		fileAreas:      map[int64]domain.FileArea{},
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

var _ BoardRepository = (*InMemoryBoardRepository)(nil)
var _ PrivateMailRepository = (*InMemoryPrivateMailRepository)(nil)
var _ AdminRepository = (*InMemoryAdminRepository)(nil)
