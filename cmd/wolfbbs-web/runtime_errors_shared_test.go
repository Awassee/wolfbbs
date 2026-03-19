package main

import (
	"fmt"
	"testing"

	"wolfbbs/internal/repository"
)

func TestSharedRuntimeErrorsPersistAndClear(t *testing.T) {
	adminRepo := repository.NewInMemoryAdminRepository()
	app := &webApp{adminRepo: adminRepo}

	app.addAppError("mail", fmt.Errorf("first failure"))
	app.addAppError("chat", fmt.Errorf("second failure"))

	rows := app.loadSharedRuntimeErrors()
	if len(rows) != 2 {
		t.Fatalf("expected 2 shared runtime errors, got %d", len(rows))
	}
	if rows[0].Area != "mail" || rows[1].Area != "chat" {
		t.Fatalf("unexpected shared runtime errors: %+v", rows)
	}

	latest := app.latestErrors(1)
	if len(latest) != 1 || latest[0].Message != "second failure" {
		t.Fatalf("expected latest shared runtime error, got %+v", latest)
	}

	if cleared := app.clearAppErrors(); cleared != 2 {
		t.Fatalf("expected 2 cleared errors, got %d", cleared)
	}
	if len(app.loadSharedRuntimeErrors()) != 0 {
		t.Fatalf("expected shared runtime errors cleared, got %+v", app.loadSharedRuntimeErrors())
	}
}
