package auth

import "context"

type contextKey string

const (
	contextKeyTenant  contextKey = "auth.tenant_id"
	contextKeyRole    contextKey = "auth.role"
	contextKeySubject contextKey = "auth.subject"
)

// WithIdentity stores auth identity details in context.
func WithIdentity(ctx context.Context, tenantID string, role Role, subject string) context.Context {
	ctx = context.WithValue(ctx, contextKeyTenant, tenantID)
	ctx = context.WithValue(ctx, contextKeyRole, role)
	ctx = context.WithValue(ctx, contextKeySubject, subject)
	return ctx
}

// TenantIDFromContext extracts tenant id from context.
func TenantIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value := ctx.Value(contextKeyTenant)
	if tenantID, ok := value.(string); ok {
		return tenantID
	}
	return ""
}

// RoleFromContext extracts role from context.
func RoleFromContext(ctx context.Context) Role {
	if ctx == nil {
		return ""
	}
	value := ctx.Value(contextKeyRole)
	if role, ok := value.(Role); ok {
		return role
	}
	if role, ok := value.(string); ok {
		if normalized, valid := NormalizeRole(role); valid {
			return normalized
		}
	}
	return ""
}

// SubjectFromContext extracts subject from context.
func SubjectFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value := ctx.Value(contextKeySubject)
	if subject, ok := value.(string); ok {
		return subject
	}
	return ""
}
