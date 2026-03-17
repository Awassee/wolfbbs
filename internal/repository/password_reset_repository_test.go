package repository

import (
	"testing"
	"time"

	"wolfbbs/internal/domain"
)

func TestInMemoryPasswordResetRepositoryLifecycle(t *testing.T) {
	repo := NewInMemoryPasswordResetRepository()
	now := time.Now().UTC()
	row := &domain.PasswordResetToken{
		TokenHash: "abc123",
		Handle:    "sysop",
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	}
	if err := repo.Create(row); err != nil {
		t.Fatalf("create reset token: %v", err)
	}
	loaded, err := repo.Get("abc123")
	if err != nil {
		t.Fatalf("get reset token: %v", err)
	}
	if loaded.Handle != "sysop" {
		t.Fatalf("unexpected handle %q", loaded.Handle)
	}
	if err := repo.MarkConsumed("abc123", now.Add(time.Minute)); err != nil {
		t.Fatalf("mark consumed: %v", err)
	}
	loaded, err = repo.Get("abc123")
	if err != nil {
		t.Fatalf("get consumed token: %v", err)
	}
	if loaded.ConsumedAt == nil {
		t.Fatal("expected consumed_at")
	}
	if err := repo.DeleteExpired(now.Add(2 * time.Minute)); err != nil {
		t.Fatalf("delete expired: %v", err)
	}
	if _, err := repo.Get("abc123"); err == nil {
		t.Fatal("expected consumed token to be deleted")
	}
}
