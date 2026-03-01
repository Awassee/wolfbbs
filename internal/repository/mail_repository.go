package repository

import (
	"sync"
	"time"

	"wolfbbs/internal/domain"
)

type MailRepository interface {
	Send(mail *domain.PrivateMail) error
	ListInbox(userID int64) ([]domain.PrivateMail, error)
	ListOutbox(userID int64) ([]domain.PrivateMail, error)
}

type InMemoryMailRepository struct {
	mu        sync.RWMutex
	nextID    int64
	mails     map[int64]*domain.PrivateMail
	inboxByID map[int64][]int64
	outByID   map[int64][]int64
}

func NewInMemoryMailRepository() *InMemoryMailRepository {
	return &InMemoryMailRepository{
		nextID:    1,
		mails:     map[int64]*domain.PrivateMail{},
		inboxByID: map[int64][]int64{},
		outByID:   map[int64][]int64{},
	}
}

func (r *InMemoryMailRepository) Send(mail *domain.PrivateMail) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	mail.ID = r.nextID
	r.nextID++
	mail.CreatedAt = time.Now().UTC()
	copy := *mail
	r.mails[copy.ID] = &copy
	r.inboxByID[mail.ToUserID] = append(r.inboxByID[mail.ToUserID], copy.ID)
	r.outByID[mail.FromUserID] = append(r.outByID[mail.FromUserID], copy.ID)
	return nil
}

func (r *InMemoryMailRepository) ListInbox(userID int64) ([]domain.PrivateMail, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.inboxByID[userID]
	out := make([]domain.PrivateMail, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		out = append(out, *r.mails[ids[i]])
	}
	return out, nil
}

func (r *InMemoryMailRepository) ListOutbox(userID int64) ([]domain.PrivateMail, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.outByID[userID]
	out := make([]domain.PrivateMail, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		out = append(out, *r.mails[ids[i]])
	}
	return out, nil
}
