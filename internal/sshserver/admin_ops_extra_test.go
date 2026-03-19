package sshserver

import (
	"testing"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

func TestEnsureMailbotServiceUser(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	authSvc := auth.NewService(userRepo)

	ensureMailbotServiceUser(authSvc)

	user, err := authSvc.GetUser("mailbot")
	if err != nil || user == nil {
		t.Fatalf("expected mailbot user, got err=%v", err)
	}
	if user.Enabled {
		t.Fatalf("expected mailbot disabled, got %+v", *user)
	}
	if !user.Verified {
		t.Fatalf("expected mailbot verified, got %+v", *user)
	}
	if user.Role != "user" {
		t.Fatalf("expected mailbot role user, got %q", user.Role)
	}
}

func TestSeedDefaultBoardsSSH(t *testing.T) {
	boardRepo := repository.NewInMemoryBoardRepository()

	created, err := seedDefaultBoardsSSH(boardRepo)
	if err != nil {
		t.Fatalf("seed default boards: %v", err)
	}
	if created != 3 {
		t.Fatalf("expected 3 boards created, got %d", created)
	}

	created, err = seedDefaultBoardsSSH(boardRepo)
	if err != nil {
		t.Fatalf("seed rerun: %v", err)
	}
	if created != 0 {
		t.Fatalf("expected idempotent rerun, got %d", created)
	}
}

func TestLockedChannelsRoundTripSSH(t *testing.T) {
	adminRepo := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: adminRepo}

	if err := srv.setChannelLockSSH("#ops", true); err != nil {
		t.Fatalf("lock channel: %v", err)
	}
	if !srv.isChannelLockedSSH("#ops") {
		t.Fatal("expected #ops to be locked")
	}
	if err := srv.setChannelLockSSH("#ops", false); err != nil {
		t.Fatalf("unlock channel: %v", err)
	}
	if srv.isChannelLockedSSH("#ops") {
		t.Fatal("expected #ops to be unlocked")
	}
}

func TestEffectiveGatewayConfigUsesAdminOverrides(t *testing.T) {
	adminRepo := repository.NewInMemoryAdminRepository()
	if err := adminRepo.UpsertGatewaySettings(&domain.GatewaySettings{
		SMTPHost:        "smtp.example.com",
		SMTPPort:        2525,
		SMTPUser:        "mailer",
		SMTPPass:        "secret",
		FromDomain:      "example.com",
		MaxRecipients:   7,
		MaxMessageBytes: 77777,
	}); err != nil {
		t.Fatalf("seed gateway settings: %v", err)
	}
	srv := &Server{admin: adminRepo}

	cfg := srv.effectiveGatewayConfig()
	if cfg.Host != "smtp.example.com" || cfg.Port != 2525 {
		t.Fatalf("unexpected smtp settings: %+v", cfg)
	}
	if cfg.MaxRecipients != 7 || cfg.MaxMessageBytes != 77777 {
		t.Fatalf("unexpected limits: %+v", cfg)
	}
}
