package repository

import (
	"errors"
	"strings"
	"sync"
	"time"

	"wolfbbs/internal/domain"
)

var ErrUserNotFound = errors.New("user not found")

type UserRepository interface {
	Create(user *domain.User) error
	GetByHandle(handle string) (*domain.User, error)
	Update(user *domain.User) error
<<<<<<< ours
	List() ([]domain.User, error)
=======
>>>>>>> theirs
}

type InMemoryUserRepository struct {
	mu      sync.RWMutex
	nextID  int64
	byID    map[int64]*domain.User
	byLower map[string]int64
}

func NewInMemoryUserRepository() *InMemoryUserRepository {
	return &InMemoryUserRepository{
		nextID:  1,
		byID:    map[int64]*domain.User{},
		byLower: map[string]int64{},
	}
}

func (r *InMemoryUserRepository) Create(user *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := strings.ToLower(strings.TrimSpace(user.Handle))
	if key == "" {
		return errors.New("handle is required")
	}
	if _, exists := r.byLower[key]; exists {
		return errors.New("handle already exists")
	}
	now := time.Now().UTC()
	user.ID = r.nextID
	r.nextID++
	if user.Theme == "" {
		user.Theme = "retro-amber"
	}
	user.CreatedAt = now
	user.UpdatedAt = now
	copy := *user
	r.byID[copy.ID] = &copy
	r.byLower[key] = copy.ID
	return nil
}

func (r *InMemoryUserRepository) GetByHandle(handle string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byLower[strings.ToLower(strings.TrimSpace(handle))]
	if !ok {
		return nil, ErrUserNotFound
	}
	user := *r.byID[id]
	return &user, nil
}

<<<<<<< ours
func (r *InMemoryUserRepository) List() ([]domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.User, 0, len(r.byID))
	for _, user := range r.byID {
		out = append(out, *user)
	}
	return out, nil
}

=======
>>>>>>> theirs
func (r *InMemoryUserRepository) Update(user *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[user.ID]; !ok {
		return ErrUserNotFound
	}
	user.UpdatedAt = time.Now().UTC()
	copy := *user
	r.byID[user.ID] = &copy
	return nil
}
