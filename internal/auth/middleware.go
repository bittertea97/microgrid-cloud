package auth

import (
	"net/http"
	"strings"
)

// Middleware validates JWTs and enforces RBAC.
type Middleware struct {
	Secret []byte
	Policy Policy
}

// NewMiddleware constructs an auth middleware.
func NewMiddleware(secret []byte, policy Policy) *Middleware {
	return &Middleware{Secret: secret, Policy: policy}
}

// Wrap applies auth and RBAC to the handler.
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	if m == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.Policy.IsExempt(r) {
			next.ServeHTTP(w, r)
			return
		}

		required, ok := m.Policy.RequiredRole(r)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		token := extractBearer(r)
		claims, err := ParseJWT(token, m.Secret)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		role, _ := NormalizeRole(claims.Role)
		ctx := WithIdentity(r.Context(), claims.TenantID, role, claims.Subject)
		if !RoleAtLeast(role, required) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func extractBearer(r *http.Request) string {
	if r == nil {
		return ""
	}
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}
	parts := strings.Fields(header)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}
