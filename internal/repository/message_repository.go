package repository

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"wolfbbs/internal/domain"
)

type MessageRepository interface {
	CreateMessage(msg *domain.Message) error
	GetMessage(id int64) (*domain.Message, error)
	ListByBoard(boardID int64) ([]domain.Message, error)
	DeleteMessage(id int64) error
	MoveThread(threadID, toBoardID int64) error
	SetThreadLocked(threadID int64, locked bool) error
	IsThreadLocked(threadID int64) (bool, error)
	CreateReport(report *domain.MessageReport) error
	ListReports(limit int, status string) ([]domain.MessageReport, error)
	ResolveReport(id int64, resolvedBy string, resolvedAt time.Time) error
	GetPointer(userID, boardID int64) (*domain.MessagePointer, error)
	SetPointer(userID, boardID, lastReadID int64, lastReadAt time.Time) error
}

type InMemoryMessageRepository struct {
	mu           sync.Mutex
	nextID       int64
	nextReportID int64
	byID         map[int64]domain.Message
	byBoard      map[int64][]int64
	ptrs         map[[2]int64]domain.MessagePointer
	threadLocks  map[int64]bool
	reports      map[int64]domain.MessageReport
}

func NewInMemoryMessageRepository() *InMemoryMessageRepository {
	return &InMemoryMessageRepository{
		nextID:       1,
		nextReportID: 1,
		byID:         map[int64]domain.Message{},
		byBoard:      map[int64][]int64{},
		ptrs:         map[[2]int64]domain.MessagePointer{},
		threadLocks:  map[int64]bool{},
		reports:      map[int64]domain.MessageReport{},
	}
}

func (r *InMemoryMessageRepository) CreateMessage(msg *domain.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg == nil {
		return errors.New("message is required")
	}
	if msg.BoardID <= 0 {
		return errors.New("board id is required")
	}
	if msg.AuthorID <= 0 {
		return errors.New("author id is required")
	}
	msg.ID = r.nextID
	r.nextID++
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	if msg.ParentID > 0 {
		parent, ok := r.byID[msg.ParentID]
		if !ok || parent.BoardID != msg.BoardID {
			return errors.New("parent message not found in board")
		}
		if parent.ThreadID > 0 {
			msg.ThreadID = parent.ThreadID
		} else {
			msg.ThreadID = parent.ID
		}
	}
	if msg.ThreadID <= 0 {
		msg.ThreadID = msg.ID
	}
	if r.threadLocks[msg.ThreadID] {
		return errors.New("thread is locked")
	}
	r.byID[msg.ID] = *msg
	r.byBoard[msg.BoardID] = append(r.byBoard[msg.BoardID], msg.ID)
	return nil
}

func (r *InMemoryMessageRepository) GetMessage(id int64) (*domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.byID[id]
	if !ok {
		return nil, errors.New("message not found")
	}
	copy := m
	return &copy, nil
}

func (r *InMemoryMessageRepository) ListByBoard(boardID int64) ([]domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := r.byBoard[boardID]
	out := make([]domain.Message, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.byID[id])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ThreadID == out[j].ThreadID {
			if out[i].ParentID == out[j].ParentID {
				if out[i].CreatedAt.Equal(out[j].CreatedAt) {
					return out[i].ID < out[j].ID
				}
				return out[i].CreatedAt.Before(out[j].CreatedAt)
			}
			if out[i].ParentID == 0 {
				return true
			}
			if out[j].ParentID == 0 {
				return false
			}
			return out[i].ParentID < out[j].ParentID
		}
		return out[i].ThreadID < out[j].ThreadID
	})
	return out, nil
}

func (r *InMemoryMessageRepository) DeleteMessage(id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	msg, ok := r.byID[id]
	if !ok {
		return ErrNotFound
	}
	delete(r.byID, id)
	ids := r.byBoard[msg.BoardID]
	filtered := make([]int64, 0, len(ids))
	for _, messageID := range ids {
		if messageID != id {
			filtered = append(filtered, messageID)
		}
	}
	r.byBoard[msg.BoardID] = filtered
	return nil
}

func (r *InMemoryMessageRepository) MoveThread(threadID, toBoardID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if threadID <= 0 || toBoardID <= 0 {
		return errors.New("thread id and destination board id are required")
	}
	moved := 0
	for id, row := range r.byID {
		if row.ThreadID == threadID {
			row.BoardID = toBoardID
			r.byID[id] = row
			moved++
		}
	}
	if moved == 0 {
		return ErrNotFound
	}
	// Rebuild board index.
	r.byBoard = map[int64][]int64{}
	for id, row := range r.byID {
		r.byBoard[row.BoardID] = append(r.byBoard[row.BoardID], id)
	}
	return nil
}

func (r *InMemoryMessageRepository) SetThreadLocked(threadID int64, locked bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if threadID <= 0 {
		return errors.New("thread id is required")
	}
	if locked {
		r.threadLocks[threadID] = true
	} else {
		delete(r.threadLocks, threadID)
	}
	return nil
}

func (r *InMemoryMessageRepository) IsThreadLocked(threadID int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if threadID <= 0 {
		return false, errors.New("thread id is required")
	}
	return r.threadLocks[threadID], nil
}

func (r *InMemoryMessageRepository) CreateReport(report *domain.MessageReport) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if report == nil {
		return errors.New("report is required")
	}
	if report.MessageID <= 0 || report.ReporterID <= 0 {
		return errors.New("message id and reporter id are required")
	}
	report.ID = r.nextReportID
	r.nextReportID++
	if report.Status == "" {
		report.Status = "open"
	}
	if report.CreatedAt.IsZero() {
		report.CreatedAt = time.Now().UTC()
	}
	r.reports[report.ID] = *report
	return nil
}

func (r *InMemoryMessageRepository) ListReports(limit int, status string) ([]domain.MessageReport, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 {
		limit = 200
	}
	status = strings.TrimSpace(strings.ToLower(status))
	out := make([]domain.MessageReport, 0, len(r.reports))
	for _, report := range r.reports {
		if status != "" && strings.ToLower(report.Status) != status {
			continue
		}
		out = append(out, report)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryMessageRepository) ResolveReport(id int64, resolvedBy string, resolvedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	report, ok := r.reports[id]
	if !ok {
		return ErrNotFound
	}
	if resolvedAt.IsZero() {
		resolvedAt = time.Now().UTC()
	}
	report.Status = "resolved"
	report.ResolvedBy = strings.TrimSpace(resolvedBy)
	report.ResolvedAt = &resolvedAt
	r.reports[id] = report
	return nil
}

func (r *InMemoryMessageRepository) GetPointer(userID, boardID int64) (*domain.MessagePointer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if userID <= 0 || boardID <= 0 {
		return nil, ErrNotFound
	}
	key := [2]int64{userID, boardID}
	row, ok := r.ptrs[key]
	if !ok {
		return nil, ErrNotFound
	}
	copy := row
	return &copy, nil
}

func (r *InMemoryMessageRepository) SetPointer(userID, boardID, lastReadID int64, lastReadAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if userID <= 0 || boardID <= 0 || lastReadID <= 0 {
		return errors.New("user id, board id, and last read id are required")
	}
	now := time.Now().UTC()
	if lastReadAt.IsZero() {
		lastReadAt = now
	}
	key := [2]int64{userID, boardID}
	existing, ok := r.ptrs[key]
	if ok {
		if lastReadID < existing.LastReadID {
			lastReadID = existing.LastReadID
		}
	}
	r.ptrs[key] = domain.MessagePointer{
		UserID:     userID,
		BoardID:    boardID,
		LastReadID: lastReadID,
		LastReadAt: lastReadAt.UTC(),
		UpdatedAt:  now,
	}
	return nil
}

var _ MessageRepository = (*InMemoryMessageRepository)(nil)
