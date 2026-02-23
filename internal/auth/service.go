package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

var (
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrMissingSecondFactor = errors.New("missing second-factor code")
	ErrInvalidSecondFactor = errors.New("invalid second-factor code")
)

type Service struct {
	users repository.UserRepository
}

func NewService(users repository.UserRepository) *Service {
	return &Service{users: users}
}

func (s *Service) Register(handle, password string) (*domain.User, error) {
	handle = strings.TrimSpace(handle)
	if len(handle) < 3 {
		return nil, errors.New("handle must be at least 3 characters")
	}
	if len(password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := &domain.User{
		Handle:        handle,
		PasswordHash:  string(hash),
		Enabled:       true,
		ANSIEnabled:   true,
		PagingEnabled: true,
		Theme:         "retro-amber",
		Role:          "user",
	}
	if err := s.users.Create(user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) Login(handle, password string) (*domain.User, error) {
	user, err := s.users.GetByHandle(handle)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if !user.Enabled {
		return nil, ErrInvalidCredentials
	}
	if user.Banned {
		return nil, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	now := time.Now().UTC()
	user.LastLoginAt = &now
	_ = s.users.Update(user)
	return user, nil
}

func (s *Service) Authenticate(handle, password, secondFactor string) (*domain.User, error) {
	user, err := s.users.GetByHandle(handle)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if !user.Enabled {
		return nil, ErrInvalidCredentials
	}
	if user.Banned {
		return nil, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	if err := s.VerifySecondFactorForUser(user, secondFactor); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	user.LastLoginAt = &now
	_ = s.users.Update(user)
	return user, nil
}

func (s *Service) VerifySecondFactor(handle, code string) error {
	user, err := s.users.GetByHandle(handle)
	if err != nil {
		return ErrInvalidCredentials
	}
	return s.VerifySecondFactorForUser(user, code)
}

func (s *Service) VerifySecondFactorForUser(user *domain.User, code string) error {
	if user == nil {
		return ErrInvalidCredentials
	}
	code = strings.TrimSpace(code)
	if user.TOTPSecret == "" {
		return nil
	}
	if code == "" {
		return ErrMissingSecondFactor
	}
	if VerifyTOTP(user.TOTPSecret, code, time.Now()) {
		return nil
	}
	for idx, candidate := range user.RecoveryCodes {
		if len(candidate) == 0 {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(candidate)), []byte(code)) == 1 {
			user.RecoveryCodes = append(append([]string{}, user.RecoveryCodes[:idx]...), user.RecoveryCodes[idx+1:]...)
			_ = s.users.Update(user)
			return nil
		}
	}
	return ErrInvalidSecondFactor
}

func GenerateRecoveryCodes(count int) ([]string, error) {
	if count <= 0 {
		count = 8
	}
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	out := make([]string, count)
	b := make([]byte, count*8)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	for i := 0; i < count; i++ {
		start := i * 8
		chunk := b[start : start+8]
		var code strings.Builder
		for _, v := range chunk {
			code.WriteByte(alphabet[int(v)%len(alphabet)])
		}
		out[i] = code.String()
	}
	return out, nil
}

func RecoveryCodesString(codes []string) string {
	if len(codes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(codes))
	for _, code := range codes {
		clean := strings.TrimSpace(code)
		if clean == "" {
			continue
		}
		parts = append(parts, clean)
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, ","))
}

func (s *Service) GetUser(handle string) (*domain.User, error) {
	return s.users.GetByHandle(handle)
}

func (s *Service) ListUsers() ([]domain.User, error) {
	return s.users.List()
}

func (s *Service) SetEnabled(handle string, enabled bool) error {
	user, err := s.users.GetByHandle(handle)
	if err != nil {
		return err
	}
	user.Enabled = enabled
	return s.users.Update(user)
}

func (s *Service) SetBanned(handle string, banned bool) error {
	user, err := s.users.GetByHandle(handle)
	if err != nil {
		return err
	}
	user.Banned = banned
	return s.users.Update(user)
}

func (s *Service) SetPassword(handle, newPassword string) error {
	if len(newPassword) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user, err := s.users.GetByHandle(handle)
	if err != nil {
		return err
	}
	user.PasswordHash = string(hash)
	user.ForceReset = false
	return s.users.Update(user)
}

func (s *Service) SetForceReset(handle string, reset bool) error {
	user, err := s.users.GetByHandle(handle)
	if err != nil {
		return err
	}
	user.ForceReset = reset
	return s.users.Update(user)
}

func (s *Service) SetRole(handle string, role string) error {
	user, err := s.users.GetByHandle(handle)
	if err != nil {
		return err
	}
	user.Role = role
	return s.users.Update(user)
}

func (s *Service) SetVerified(handle string, verified bool) error {
	user, err := s.users.GetByHandle(handle)
	if err != nil {
		return err
	}
	user.Verified = verified
	return s.users.Update(user)
}

func (s *Service) SetTOTPSecret(handle, secret string) error {
	user, err := s.users.GetByHandle(handle)
	if err != nil {
		return err
	}
	user.TOTPSecret = secret
	return s.users.Update(user)
}

func (s *Service) SetRecoveryCodes(handle string, codes []string) error {
	user, err := s.users.GetByHandle(handle)
	if err != nil {
		return err
	}
	user.RecoveryCodes = codes
	return s.users.Update(user)
}
