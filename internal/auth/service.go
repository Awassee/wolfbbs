package auth

import (
<<<<<<< ours
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/events"
	"wolfbbs/internal/rbac"
=======
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"wolfbbs/internal/domain"
>>>>>>> theirs
	"wolfbbs/internal/repository"
)

var (
<<<<<<< ours
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrMissingSecondFactor = errors.New("missing second-factor code")
	ErrInvalidSecondFactor = errors.New("invalid second-factor code")
	ErrResetNotEnabled     = errors.New("password reset is not enabled")
	ErrInvalidResetToken   = errors.New("invalid reset token")
)

type Service struct {
	users       repository.UserRepository
	hashPolicy  HashPolicy
	resetTokens repository.PasswordResetRepository
	bus         *events.Bus
}

func NewService(users repository.UserRepository) *Service {
	return NewServiceWithPolicy(users, hashPolicyFromEnv())
}

func NewServiceWithPolicy(users repository.UserRepository, policy HashPolicy) *Service {
	return &Service{
		users:      users,
		hashPolicy: policy.normalize(),
	}
}

func (s *Service) SetEventBus(bus *events.Bus) {
	s.bus = bus
=======
	ErrInvalidCredentials = errors.New("invalid credentials")
)

type Service struct {
	users repository.UserRepository
}

func NewService(users repository.UserRepository) *Service {
	return &Service{users: users}
>>>>>>> theirs
}

func (s *Service) Register(handle, password string) (*domain.User, error) {
	handle = strings.TrimSpace(handle)
	if len(handle) < 3 {
		return nil, errors.New("handle must be at least 3 characters")
	}
	if len(password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}
<<<<<<< ours
	hash, err := hashPassword(password, s.hashPolicy)
=======
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
>>>>>>> theirs
	if err != nil {
		return nil, err
	}
	user := &domain.User{
		Handle:        handle,
		PasswordHash:  string(hash),
<<<<<<< ours
		Enabled:       true,
		ANSIEnabled:   true,
		PagingEnabled: true,
		Theme:         "retro-amber",
		Role:          rbac.RoleUser,
=======
		ANSIEnabled:   true,
		PagingEnabled: true,
		Theme:         "retro-amber",
>>>>>>> theirs
	}
	if err := s.users.Create(user); err != nil {
		return nil, err
	}
<<<<<<< ours
	s.publish("auth.registered", map[string]string{"handle": user.Handle})
=======
>>>>>>> theirs
	return user, nil
}

func (s *Service) Login(handle, password string) (*domain.User, error) {
	user, err := s.users.GetByHandle(handle)
	if err != nil {
<<<<<<< ours
		s.publish("auth.login_failed", map[string]string{"handle": handle, "reason": "unknown_user"})
		return nil, ErrInvalidCredentials
	}
	if !user.Enabled {
		s.publish("auth.login_failed", map[string]string{"handle": handle, "reason": "disabled"})
		return nil, ErrInvalidCredentials
	}
	if user.Banned {
		s.publish("auth.login_failed", map[string]string{"handle": handle, "reason": "banned"})
		return nil, ErrInvalidCredentials
	}
	ok, err := s.verifyPasswordWithUpgrade(user, password)
	if err != nil {
		s.publish("auth.login_failed", map[string]string{"handle": handle, "reason": "verify_error"})
		return nil, ErrInvalidCredentials
	}
	if !ok {
		s.publish("auth.login_failed", map[string]string{"handle": handle, "reason": "bad_password"})
		return nil, ErrInvalidCredentials
	}
	previousLogin := cloneTimePtr(user.LastLoginAt)
	now := time.Now().UTC()
	user.LastLoginAt = &now
	_ = s.users.Update(user)
	s.publish("auth.login_success", map[string]string{"handle": user.Handle})
	user.LastLoginAt = previousLogin
	return user, nil
}

func (s *Service) Authenticate(handle, password, secondFactor string) (*domain.User, error) {
	user, err := s.users.GetByHandle(handle)
	if err != nil {
		s.publish("auth.login_failed", map[string]string{"handle": handle, "reason": "unknown_user"})
		return nil, ErrInvalidCredentials
	}
	if !user.Enabled {
		s.publish("auth.login_failed", map[string]string{"handle": handle, "reason": "disabled"})
		return nil, ErrInvalidCredentials
	}
	if user.Banned {
		s.publish("auth.login_failed", map[string]string{"handle": handle, "reason": "banned"})
		return nil, ErrInvalidCredentials
	}
	ok, err := s.verifyPasswordWithUpgrade(user, password)
	if err != nil {
		s.publish("auth.login_failed", map[string]string{"handle": handle, "reason": "verify_error"})
		return nil, ErrInvalidCredentials
	}
	if !ok {
		s.publish("auth.login_failed", map[string]string{"handle": handle, "reason": "bad_password"})
		return nil, ErrInvalidCredentials
	}
	if err := s.VerifySecondFactorForUser(user, secondFactor); err != nil {
		s.publish("auth.login_failed", map[string]string{"handle": handle, "reason": "2fa"})
		return nil, err
	}
	previousLogin := cloneTimePtr(user.LastLoginAt)
	now := time.Now().UTC()
	user.LastLoginAt = &now
	_ = s.users.Update(user)
	s.publish("auth.login_success", map[string]string{"handle": user.Handle})
	user.LastLoginAt = previousLogin
	return user, nil
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := value.UTC()
	return &clone
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
	hash, err := hashPassword(newPassword, s.hashPolicy)
	if err != nil {
		return err
	}
	user, err := s.users.GetByHandle(handle)
	if err != nil {
		return err
	}
	user.PasswordHash = string(hash)
	user.ForceReset = false
	if err := s.users.Update(user); err != nil {
		return err
	}
	s.publish("auth.password_changed", map[string]string{"handle": user.Handle})
	return nil
}

func (s *Service) SetPasswordResetRepository(repo repository.PasswordResetRepository) {
	s.resetTokens = repo
}

func (s *Service) IssuePasswordReset(handle string, ttl time.Duration) (string, error) {
	if s.resetTokens == nil {
		return "", ErrResetNotEnabled
	}
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return "", errors.New("handle is required")
	}
	user, err := s.users.GetByHandle(handle)
	if err != nil || user == nil {
		return "", ErrInvalidCredentials
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	rawToken, err := randomToken(32)
	if err != nil {
		return "", err
	}
	tokenHash := hashResetToken(rawToken)
	now := time.Now().UTC()
	if err := s.resetTokens.Create(&domain.PasswordResetToken{
		TokenHash: tokenHash,
		Handle:    user.Handle,
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
	}); err != nil {
		return "", err
	}
	_ = s.resetTokens.DeleteExpired(now)
	s.publish("auth.password_reset_issued", map[string]string{"handle": user.Handle})
	return rawToken, nil
}

func (s *Service) ResetPasswordWithToken(token, newPassword string) error {
	if s.resetTokens == nil {
		return ErrResetNotEnabled
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidResetToken
	}
	tokenHash := hashResetToken(token)
	row, err := s.resetTokens.Get(tokenHash)
	if err != nil || row == nil {
		return ErrInvalidResetToken
	}
	now := time.Now().UTC()
	if row.ConsumedAt != nil || row.ExpiresAt.Before(now) {
		return ErrInvalidResetToken
	}
	if err := s.SetPassword(row.Handle, newPassword); err != nil {
		return err
	}
	if err := s.resetTokens.MarkConsumed(tokenHash, now); err != nil {
		return err
	}
	s.publish("auth.password_reset_completed", map[string]string{"handle": row.Handle})
	return nil
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
	user.Role = rbac.NormalizeRole(role)
	if err := s.users.Update(user); err != nil {
		return err
	}
	s.publish("auth.role_changed", map[string]string{"handle": user.Handle, "role": user.Role})
	return nil
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

func (s *Service) SetPreferences(handle, theme string, ansiEnabled, pagingEnabled, timeFormat24h bool) error {
	user, err := s.users.GetByHandle(handle)
	if err != nil {
		return err
	}
	theme = strings.TrimSpace(theme)
	if theme == "" {
		theme = user.Theme
	}
	if theme == "" {
		theme = "retro-amber"
	}
	user.Theme = theme
	user.ANSIEnabled = ansiEnabled
	user.PagingEnabled = pagingEnabled
	user.TimeFormat24h = timeFormat24h
	return s.users.Update(user)
}

func (s *Service) verifyPasswordWithUpgrade(user *domain.User, password string) (bool, error) {
	ok, err := verifyPassword(user.PasswordHash, password)
	if err != nil || !ok {
		return ok, err
	}
	if shouldUpgradeHash(user.PasswordHash, s.hashPolicy) {
		upgraded, hashErr := hashPassword(password, s.hashPolicy)
		if hashErr == nil {
			user.PasswordHash = upgraded
			_ = s.users.Update(user)
		}
	}
	return true, nil
}

func hashPolicyFromEnv() HashPolicy {
	policy := defaultHashPolicy()
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_PASSWORD_HASH")); raw != "" {
		policy.Algorithm = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_PASSWORD_ALGORITHM")); raw != "" {
		policy.Algorithm = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_PBKDF2_ITERATIONS")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			policy.PBKDF2Iterations = v
		}
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_PBKDF2_SALT_BYTES")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			policy.PBKDF2SaltBytes = v
		}
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_PASSWORD_UPGRADE_ON_LOGIN")); raw != "" {
		lower := strings.ToLower(raw)
		policy.UpgradeOnLogin = !(lower == "0" || lower == "false" || lower == "no")
	}
	return policy.normalize()
}

func randomToken(bytesLen int) (string, error) {
	if bytesLen <= 0 {
		bytesLen = 32
	}
	raw := make([]byte, bytesLen)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func hashResetToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func (s *Service) publish(name string, fields map[string]string) {
	if s == nil || s.bus == nil {
		return
	}
	s.bus.Publish(name, fields)
}
=======
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
>>>>>>> theirs
