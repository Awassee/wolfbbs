package sshserver

import (
	"testing"

	"wolfbbs/internal/domain"
)

func TestLegacyHotkeyToAction(t *testing.T) {
	if got := legacyHotkeyToAction("m"); got != "boards.open" {
		t.Fatalf("expected boards.open, got %q", got)
	}
	if got := legacyHotkeyToAction("n"); got != "system.newscan" {
		t.Fatalf("expected system.newscan, got %q", got)
	}
	if got := legacyHotkeyToAction("?"); got != "" {
		t.Fatalf("expected empty action for help key, got %q", got)
	}
	if got := legacyHotkeyToAction("x"); got != "system.config_center" {
		t.Fatalf("expected system.config_center, got %q", got)
	}
	if got := legacyHotkeyToAction("y"); got != "system.status_center" {
		t.Fatalf("expected system.status_center, got %q", got)
	}
	if got := legacyHotkeyToAction("/"); got != "system.quick_jump" {
		t.Fatalf("expected system.quick_jump, got %q", got)
	}
}

func TestQuickJumpToAction(t *testing.T) {
	if got := quickJumpToAction("boards"); got != "boards.open" {
		t.Fatalf("expected boards.open, got %q", got)
	}
	if got := quickJumpToAction("status"); got != "system.status_center" {
		t.Fatalf("expected system.status_center, got %q", got)
	}
	if got := quickJumpToAction("config"); got != "system.config_center" {
		t.Fatalf("expected system.config_center, got %q", got)
	}
	if got := quickJumpToAction("unknown"); got != "" {
		t.Fatalf("expected empty action for unknown jump target, got %q", got)
	}
}

func TestEvaluateAccess(t *testing.T) {
	user := &domain.User{
		Handle:   "tester",
		Role:     "moderator",
		Verified: true,
	}
	if ok := evaluateAccess("role=moderator and verified", user, "", map[string]string{"area": "boards"}, true, nil); !ok {
		t.Fatal("expected moderator verified user to pass")
	}
	if ok := evaluateAccess("role=sysop", user, "", nil, true, nil); ok {
		t.Fatal("expected moderator to fail sysop-only rule")
	}
	if ok := evaluateAccess("role=admin", user, "", nil, true, nil); ok {
		t.Fatal("expected moderator to fail admin/sysop-only alias rule")
	}
	if ok := evaluateAccess("(role=admin", user, "", nil, false, nil); !ok {
		t.Fatal("expected invalid expression to allow when strict is false")
	}
	if ok := evaluateAccess("(role=admin", user, "", nil, true, nil); ok {
		t.Fatal("expected invalid expression to deny when strict is true")
	}
}
