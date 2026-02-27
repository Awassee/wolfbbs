package auth_test

import (
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/repository"
)

func TestRegisterAndLogin(t *testing.T) {
	repo := repository.NewInMemoryUserRepository()
	svc := auth.NewService(repo)

	_, err := svc.Register("sysop", "supersecure")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	user, err := svc.Login("sysop", "supersecure")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if user.Handle != "sysop" {
		t.Fatalf("unexpected handle: %s", user.Handle)
	}
}

func TestAuthenticateWithSecondFactorRecovery(t *testing.T) {
	repo := repository.NewInMemoryUserRepository()
	svc := auth.NewService(repo)

	user, err := svc.Register("sysop", "supersecure")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	codebook, err := auth.GenerateRecoveryCodes(1)
	if err != nil {
		t.Fatalf("generate recovery code: %v", err)
	}
	user.TOTPSecret = "JBSWY3DPEHPK3PXP"
	user.RecoveryCodes = codebook
	if err := repo.Update(user); err != nil {
		t.Fatalf("seed user fields: %v", err)
	}
	user, err = svc.Authenticate("sysop", "supersecure", codebook[0])
	if err != nil {
		t.Fatalf("authenticate with recovery code failed: %v", err)
	}
	if user.RecoveryCodes != nil && len(user.RecoveryCodes) != 0 {
		t.Fatalf("recovery code should be consumed: %#v", user.RecoveryCodes)
	}
}

func TestSetPreferences(t *testing.T) {
	repo := repository.NewInMemoryUserRepository()
	svc := auth.NewService(repo)

	_, err := svc.Register("prefs", "password123")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if err := svc.SetPreferences("prefs", "teal", false, false, true); err != nil {
		t.Fatalf("set preferences failed: %v", err)
	}
	user, err := svc.GetUser("prefs")
	if err != nil {
		t.Fatalf("get user failed: %v", err)
	}
	if user.Theme != "teal" || user.ANSIEnabled || user.PagingEnabled || !user.TimeFormat24h {
		t.Fatalf("unexpected preferences: %+v", user)
	}
}

func TestPBKDF2RegisterAndLogin(t *testing.T) {
	repo := repository.NewInMemoryUserRepository()
	svc := auth.NewServiceWithPolicy(repo, auth.HashPolicy{
		Algorithm:        auth.HashPBKDF2SHA256,
		PBKDF2Iterations: 1000,
		PBKDF2SaltBytes:  16,
		UpgradeOnLogin:   true,
	})

	user, err := svc.Register("pbkdf2-user", "password123")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if user == nil || user.PasswordHash == "" {
		t.Fatal("expected password hash")
	}
	if !strings.HasPrefix(user.PasswordHash, auth.HashPBKDF2SHA256+"$") {
		t.Fatalf("expected pbkdf2 hash prefix, got %q", user.PasswordHash)
	}
	if _, err := svc.Login("pbkdf2-user", "password123"); err != nil {
		t.Fatalf("login failed: %v", err)
	}
}

func TestBcryptLoginUpgradesToPBKDF2(t *testing.T) {
	repo := repository.NewInMemoryUserRepository()
	legacy := auth.NewServiceWithPolicy(repo, auth.HashPolicy{Algorithm: auth.HashBcrypt})
	if _, err := legacy.Register("legacy-user", "password123"); err != nil {
		t.Fatalf("legacy register failed: %v", err)
	}

	modern := auth.NewServiceWithPolicy(repo, auth.HashPolicy{
		Algorithm:        auth.HashPBKDF2SHA256,
		PBKDF2Iterations: 1000,
		PBKDF2SaltBytes:  16,
		UpgradeOnLogin:   true,
	})
	if _, err := modern.Login("legacy-user", "password123"); err != nil {
		t.Fatalf("modern login failed: %v", err)
	}
	upgraded, err := modern.GetUser("legacy-user")
	if err != nil {
		t.Fatalf("get upgraded user failed: %v", err)
	}
	if upgraded == nil || upgraded.PasswordHash == "" {
		t.Fatal("expected upgraded hash")
	}
	if !strings.HasPrefix(upgraded.PasswordHash, auth.HashPBKDF2SHA256+"$") {
		t.Fatalf("expected upgraded pbkdf2 hash, got %q", upgraded.PasswordHash)
	}
}

func TestPasswordResetTokenFlow(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	resetRepo := repository.NewInMemoryPasswordResetRepository()
	svc := auth.NewServiceWithPolicy(userRepo, auth.HashPolicy{
		Algorithm:        auth.HashPBKDF2SHA256,
		PBKDF2Iterations: 1000,
		PBKDF2SaltBytes:  16,
		UpgradeOnLogin:   true,
	})
	svc.SetPasswordResetRepository(resetRepo)
	if _, err := svc.Register("reset-user", "oldpassword123"); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	token, err := svc.IssuePasswordReset("reset-user", 5*time.Minute)
	if err != nil {
		t.Fatalf("issue reset failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected reset token")
	}
	if err := svc.ResetPasswordWithToken(token, "newpassword123"); err != nil {
		t.Fatalf("complete reset failed: %v", err)
	}
	if _, err := svc.Login("reset-user", "oldpassword123"); err == nil {
		t.Fatal("expected old password to fail")
	}
	if _, err := svc.Login("reset-user", "newpassword123"); err != nil {
		t.Fatalf("expected new password to work: %v", err)
	}
	if err := svc.ResetPasswordWithToken(token, "thirdpassword123"); err == nil {
		t.Fatal("expected token reuse to fail")
	}
}
