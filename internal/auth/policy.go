package auth

import (
	"net/http"
	"strings"
)

// Policy determines required roles by request.
type Policy struct {
	ExemptPaths    map[string]struct{}
	ExemptPrefixes []string
}

// NewDefaultPolicy builds a default policy with exemptions.
func NewDefaultPolicy(exemptPaths []string, exemptPrefixes []string) Policy {
	set := make(map[string]struct{}, len(exemptPaths))
	for _, path := range exemptPaths {
		set[path] = struct{}{}
	}
	return Policy{ExemptPaths: set, ExemptPrefixes: exemptPrefixes}
}

// IsExempt returns true when a request should skip auth/RBAC.
func (p Policy) IsExempt(r *http.Request) bool {
	if r == nil {
		return true
	}
	if _, ok := p.ExemptPaths[r.URL.Path]; ok {
		return true
	}
	for _, prefix := range p.ExemptPrefixes {
		if strings.HasPrefix(r.URL.Path, prefix) {
			return true
		}
	}
	return false
}

// RequiredRole resolves required role for the request.
func (p Policy) RequiredRole(r *http.Request) (Role, bool) {
	if r == nil {
		return "", false
	}
	path := r.URL.Path
	method := r.Method

	switch {
	case path == "/api/v1/provisioning/stations":
		return RoleAdmin, true
	case path == "/api/v1/commands":
		if method == http.MethodPost {
			return RoleOperator, true
		}
		return RoleViewer, true
	case path == "/api/v1/alarms":
		return RoleViewer, true
	case path == "/api/v1/alarms/stream":
		return RoleViewer, true
	case strings.HasPrefix(path, "/api/v1/alarms/") && method == http.MethodPost:
		return RoleOperator, true
	case strings.HasPrefix(path, "/api/v1/strategies/"):
		if method == http.MethodGet {
			return RoleViewer, true
		}
		return RoleOperator, true
	case path == "/api/v1/stats":
		return RoleViewer, true
	case path == "/api/v1/settlements":
		return RoleViewer, true
	case path == "/api/v1/exports/settlements.csv":
		return RoleViewer, true
	case path == "/api/v1/statements/generate":
		return RoleAdmin, true
	case path == "/api/v1/statements":
		return RoleViewer, true
	case strings.HasPrefix(path, "/api/v1/statements/"):
		if method == http.MethodGet {
			if strings.Contains(path, "/export.") {
				return RoleAdmin, true
			}
			return RoleViewer, true
		}
		return RoleAdmin, true
	case strings.HasPrefix(path, "/api/v1/shadowrun/"):
		if method == http.MethodGet {
			return RoleViewer, true
		}
		return RoleAdmin, true
	case path == "/analytics/window-close":
		return RoleAdmin, true
	}

	if strings.HasPrefix(path, "/api/") {
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			return RoleViewer, true
		}
		return RoleOperator, true
	}
	return "", false
}
