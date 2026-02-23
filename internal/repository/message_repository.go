package repository

import (
	"errors"
	"sort"
	"sync"
	"time"

	"wolfbbs/internal/domain"
)

type MessageRepository interface {
	CreateMessage(msg *domain.Message) error
	GetMessage(id int64) (*domain.Message, error)
	ListByBoard(boardID int64) ([]domain.Message, error)
}

type InMemoryMessageRepository struct {
	mu      sync.Mutex
	nextID  int64
	byID    map[int64]domain.Message
	byBoard map[int64][]int64
}

func NewInMemoryMessageRepository() *InMemoryMessageRepository {
	return &InMemoryMessageRepository{nextID: 1, byID: map[int64]domain.Message{}, byBoard: map[int64][]int64{}}
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

var _ MessageRepository = (*InMemoryMessageRepository)(nil)
