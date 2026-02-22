package repository

import (
	"errors"
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
	mu     sync.Mutex
	nextID int64
	byID   map[int64]domain.Message
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
	msg.ID = r.nextID
	r.nextID++
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
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
	return out, nil
}

var _ MessageRepository = (*InMemoryMessageRepository)(nil)
