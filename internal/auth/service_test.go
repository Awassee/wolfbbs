package auth_test

import (
	"testing"

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
