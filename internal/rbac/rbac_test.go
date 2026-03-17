package rbac

import "testing"

func TestNormalizeRoleAlias(t *testing.T) {
	if got := NormalizeRole("admin"); got != RoleSysop {
		t.Fatalf("expected admin alias to map to sysop, got %q", got)
	}
	if got := NormalizeRole("sysop"); got != RoleSysop {
		t.Fatalf("expected sysop role, got %q", got)
	}
	if got := NormalizeRole("moderator"); got != RoleModerator {
		t.Fatalf("expected moderator role, got %q", got)
	}
}

func TestAtLeast(t *testing.T) {
	if !AtLeast("sysop", "moderator") {
		t.Fatal("sysop should satisfy moderator minimum")
	}
	if AtLeast("user", "sysop") {
		t.Fatal("user should not satisfy sysop minimum")
	}
	if !AtLeast("admin", "sysop") {
		t.Fatal("admin alias should satisfy sysop minimum")
	}
}
