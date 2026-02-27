package repository

import (
	"errors"
	"strings"
	"sync"
	"time"

	"wolfbbs/internal/domain"
)

type PasswordResetRepository interface {
	Create(token *domain.PasswordResetToken) error
	Get(tokenHash string) (*domain.PasswordResetToken, error)
	MarkConsumed(tokenHash string, consumedAt time.Time) error
	DeleteExpired(now time.Time) error
}

type InMemoryPasswordResetRepository struct {
	mu      sync.Mutex
	byToken map[string]domain.PasswordResetToken
}

func NewInMemoryPasswordResetRepository() *InMemoryPasswordResetRepository {
	return &InMemoryPasswordResetRepository{
		byToken: map[string]domain.PasswordResetToken{},
	}
}

func (r *InMemoryPasswordResetRepository) Create(token *domain.PasswordResetToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if token == nil {
		return errors.New("token is required")
	}
	key := strings.TrimSpace(token.TokenHash)
	if key == "" {
		return errors.New("token hash is required")
	}
	handle := strings.TrimSpace(token.Handle)
	if handle == "" {
		return errors.New("handle is required")
	}
	if token.ExpiresAt.IsZero() {
		return errors.New("expiry is required")
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now().UTC()
	}
	token.TokenHash = key
	token.Handle = handle
	row := *token
	r.byToken[key] = row
	return nil
}

func (r *InMemoryPasswordResetRepository) Get(tokenHash string) (*domain.PasswordResetToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.byToken[strings.TrimSpace(tokenHash)]
	if !ok {
		return nil, ErrNotFound
	}
	copy := row
	return &copy, nil
}

func (r *InMemoryPasswordResetRepository) MarkConsumed(tokenHash string, consumedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := strings.TrimSpace(tokenHash)
	row, ok := r.byToken[key]
	if !ok {
		return ErrNotFound
	}
	if consumedAt.IsZero() {
		consumedAt = time.Now().UTC()
	}
	row.ConsumedAt = &consumedAt
	r.byToken[key] = row
	return nil
}

func (r *InMemoryPasswordResetRepository) DeleteExpired(now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	for key, row := range r.byToken {
		if row.ExpiresAt.Before(now) || row.ConsumedAt != nil {
			delete(r.byToken, key)
		}
	}
	return nil
}
