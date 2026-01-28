package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"microgrid-cloud/internal/audit"
	"microgrid-cloud/internal/auth"
	strategyapp "microgrid-cloud/internal/strategy/application"
)

const timeLayout = time.RFC3339

// Handler serves strategy endpoints.
type Handler struct {
	service        *strategyapp.Service
	stationChecker auth.StationTenantChecker
	auditLogger    audit.Logger
}

// NewHandler constructs a Handler.
func NewHandler(service *strategyapp.Service, stationChecker auth.StationTenantChecker, auditLogger audit.Logger) (*Handler, error) {
	if service == nil {
		return nil, errors.New("strategy handler: nil service")
	}
	return &Handler{service: service, stationChecker: stationChecker, auditLogger: auditLogger}, nil
}

// ServeHTTP routes strategy requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/api/v1/strategies/") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/strategies/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	stationID := parts[0]
	tenantID := auth.TenantIDFromContext(r.Context())
	if tenantID != "" {
		if err := ensureStationTenant(r, h.stationChecker, tenantID, stationID); err != nil {
			respondTenantError(w, err)
			return
		}
	}

	if len(parts) == 2 && parts[1] == "mode" && r.Method == http.MethodPost {
		h.handleMode(w, r, stationID)
		return
	}
	if len(parts) == 2 && parts[1] == "enable" && r.Method == http.MethodPost {
		h.handleEnable(w, r, stationID)
		return
	}
	if len(parts) == 2 && parts[1] == "calendar" && r.Method == http.MethodPost {
		h.handleCalendar(w, r, stationID)
		return
	}
	if len(parts) == 2 && parts[1] == "runs" && r.Method == http.MethodGet {
		h.handleRuns(w, r, stationID)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (h *Handler) handleMode(w http.ResponseWriter, r *http.Request, stationID string) {
	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	resp, err := h.service.SetMode(r.Context(), stationID, req.Mode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
	h.logAudit(r, stationID, "strategy.mode.set", map[string]any{"mode": req.Mode})
}

func (h *Handler) handleEnable(w http.ResponseWriter, r *http.Request, stationID string) {
	var req struct {
		Enabled        bool           `json:"enabled"`
		TemplateType   string         `json:"template_type"`
		TemplateParams map[string]any `json:"template_params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	resp, err := h.service.SetEnabled(r.Context(), stationID, req.Enabled, req.TemplateType, req.TemplateParams)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
	h.logAudit(r, stationID, "strategy.enabled.set", map[string]any{"enabled": req.Enabled, "template_type": req.TemplateType})
}

func (h *Handler) handleCalendar(w http.ResponseWriter, r *http.Request, stationID string) {
	var req struct {
		Date      string `json:"date"`
		Enabled   bool   `json:"enabled"`
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		http.Error(w, "date must be YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	start, err := parseClock(req.StartTime)
	if err != nil {
		http.Error(w, "start_time must be HH:MM", http.StatusBadRequest)
		return
	}
	end, err := parseClock(req.EndTime)
	if err != nil {
		http.Error(w, "end_time must be HH:MM", http.StatusBadRequest)
		return
	}
	if err := h.service.SetCalendar(r.Context(), stationID, date.UTC(), req.Enabled, start, end); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	h.logAudit(r, stationID, "strategy.calendar.set", map[string]any{
		"date":       req.Date,
		"enabled":    req.Enabled,
		"start_time": req.StartTime,
		"end_time":   req.EndTime,
	})
}

func (h *Handler) handleRuns(w http.ResponseWriter, r *http.Request, stationID string) {
	from, err := parseTimeQuery(r, "from")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	to, err := parseTimeQuery(r, "to")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !to.After(from) {
		http.Error(w, "to must be after from", http.StatusBadRequest)
		return
	}
	list, err := h.service.ListRuns(r.Context(), stationID, from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (h *Handler) logAudit(r *http.Request, stationID, action string, meta map[string]any) {
	if h.auditLogger == nil {
		return
	}
	tenantID := auth.TenantIDFromContext(r.Context())
	if tenantID == "" {
		return
	}
	payload, _ := json.Marshal(meta)
	_ = h.auditLogger.Log(r.Context(), audit.Entry{
		TenantID:     tenantID,
		Actor:        auth.SubjectFromContext(r.Context()),
		Role:         string(auth.RoleFromContext(r.Context())),
		Action:       action,
		ResourceType: "strategy",
		ResourceID:   stationID,
		StationID:    stationID,
		Metadata:     payload,
		IP:           audit.ClientIP(r),
		UserAgent:    r.UserAgent(),
	})
}

func ensureStationTenant(r *http.Request, checker auth.StationTenantChecker, tenantID, stationID string) error {
	if checker == nil || tenantID == "" || stationID == "" {
		return nil
	}
	return checker.EnsureStationTenant(r.Context(), tenantID, stationID)
}

func respondTenantError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, auth.ErrTenantMismatch) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if errors.Is(err, auth.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.Error(w, "tenant check failed", http.StatusInternalServerError)
}

func parseTimeQuery(r *http.Request, key string) (time.Time, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return time.Time{}, errors.New(key + " is required")
	}
	parsed, err := time.Parse(timeLayout, value)
	if err != nil {
		return time.Time{}, errors.New(key + " must be RFC3339")
	}
	return parsed.UTC(), nil
}

func parseClock(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("empty time")
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}
