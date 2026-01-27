package auth

// Role represents a user role.
type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

// NormalizeRole validates and normalizes a role string.
func NormalizeRole(value string) (Role, bool) {
	switch Role(value) {
	case RoleViewer, RoleOperator, RoleAdmin:
		return Role(value), true
	default:
		return "", false
	}
}

// RoleAtLeast returns true when role satisfies required role.
func RoleAtLeast(role Role, required Role) bool {
	return roleRank(role) >= roleRank(required)
}

func roleRank(role Role) int {
	switch role {
	case RoleViewer:
		return 1
	case RoleOperator:
		return 2
	case RoleAdmin:
		return 3
	default:
		return 0
	}
}
