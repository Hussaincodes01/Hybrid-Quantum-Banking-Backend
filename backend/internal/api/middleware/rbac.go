package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
)

type Role string

const (
	RoleCustomer          Role = "customer"
	RoleAgent             Role = "agent"
	RoleComplianceOfficer Role = "compliance_officer"
	RoleAdmin             Role = "admin"
	RoleAuditor           Role = "auditor"
)

var roleHierarchy = map[Role]int{
	RoleCustomer:          1,
	RoleAgent:             2,
	RoleComplianceOfficer: 3,
	RoleAdmin:             4,
	RoleAuditor:           4,
}

type roleKeyType struct{}

var roleContextKey = roleKeyType{}

func RoleFromContext(ctx context.Context) (Role, bool) {
	r, ok := ctx.Value(roleContextKey).(Role)
	return r, ok
}

func SetRoleInContext(ctx context.Context, role Role) context.Context {
	return context.WithValue(ctx, roleContextKey, role)
}

// RequireRole ensures the authenticated user has one of the specified roles.
// All role validation is SERVER-SIDE only. The frontend never checks permissions.
func RequireRole(allowedRoles ...Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := RoleFromContext(r.Context())
			if !ok {
				http.Error(w, `{"error":"role not found in context — ensure Auth middleware sets RoleFromContext"}`, http.StatusForbidden)
				return
			}
			for _, allowed := range allowedRoles {
				if role == allowed {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, `{"error":"insufficient permissions for this endpoint"}`, http.StatusForbidden)
		})
	}
}

// RequireMinRole ensures the user's role is at or above the minimum level.
func RequireMinRole(minRole Role) func(http.Handler) http.Handler {
	return RequireRole(RoleFunc(minRole)...)
}

func RoleFunc(minRole Role) []Role {
	minLevel := roleHierarchy[minRole]
	roles := make([]Role, 0, len(roleHierarchy))
	for role, level := range roleHierarchy {
		if level >= minLevel {
			roles = append(roles, role)
		}
	}
	return roles
}

func ParseRole(s string) Role {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "agent":
		return RoleAgent
	case "compliance_officer", "compliance":
		return RoleComplianceOfficer
	case "admin":
		return RoleAdmin
	case "auditor":
		return RoleAuditor
	default:
		// An empty string is the normal "no role provided" case; a non-empty
		// unrecognized role is worth surfacing before we downgrade to customer.
		if s != "" {
			slog.Warn("unrecognized role string, defaulting to customer", "role", s)
		}
		return RoleCustomer
	}
}
