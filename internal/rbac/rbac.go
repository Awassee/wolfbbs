package rbac

import "strings"

const (
	RoleUser      = "user"
	RoleModerator = "moderator"
	RoleSysop     = "sysop"
)

func NormalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", RoleUser:
		return RoleUser
	case RoleModerator:
		return RoleModerator
	case "admin", RoleSysop:
		return RoleSysop
	default:
		return RoleUser
	}
}

func Rank(role string) int {
	switch NormalizeRole(role) {
	case RoleSysop:
		return 3
	case RoleModerator:
		return 2
	default:
		return 1
	}
}

func AtLeast(role, minimum string) bool {
	return Rank(role) >= Rank(minimum)
}
