package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"microgrid-cloud/internal/audit"
	"microgrid-cloud/internal/auth"
	commandsapp "microgrid-cloud/internal/commands/application"
)

// Handler provides command HTTP endpoints.
type Handler struct {
	service        *commandsapp.Service
	stationChecker auth.StationTenantChecker
	auditLogger    audit.Logger
}

// NewHandler constructs a handler.
func NewHandler(service *commandsapp.Service, stationChecker auth.StationTenantChecker, auditLogger audit.Logger) (*Handler, error) {
	if service == nil {
		return nil, errors.New("commands handler: nil service")
	}
	return &Handler{service: service, stationChecker: stationChecker, auditLogger: auditLogger}, nil
}

// ServeHTTP handles POST/GET /api/v1/commands.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handlePost(w, r)
	case http.MethodGet:
		h.handleGet(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handlePost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body error", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req commandsapp.IssueRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	tenantID := auth.TenantIDFromContext(r.Context())
	if tenantID != "" && req.TenantID != "" && req.TenantID != tenantID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if tenantID != "" {
		req.TenantID = tenantID
		if err := ensureStationTenant(r, h.stationChecker, tenantID, req.StationID); err != nil {
			respondTenantError(w, err)
			return
		}
	}

	resp, err := h.service.IssueCommand(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)

	h.logAudit(r, tenantID, resp.CommandID, resp.StationID, resp.DeviceID, resp.CommandType)
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	stationID := r.URL.Query().Get("station_id")
	fromValue := r.URL.Query().Get("from")
	toValue := r.URL.Query().Get("to")
	if stationID == "" || fromValue == "" || toValue == "" {
		http.Error(w, "station_id/from/to required", http.StatusBadRequest)
		return
	}
	from, err := time.Parse(time.RFC3339, fromValue)
	if err != nil {
		http.Error(w, "from must be RFC3339", http.StatusBadRequest)
		return
	}
	to, err := time.Parse(time.RFC3339, toValue)
	if err != nil {
		http.Error(w, "to must be RFC3339", http.StatusBadRequest)
		return
	}
	if !to.After(from) {
		http.Error(w, "to must be after from", http.StatusBadRequest)
		return
	}

	tenantID := auth.TenantIDFromContext(r.Context())
	if tenantID != "" {
		if err := ensureStationTenant(r, h.stationChecker, tenantID, stationID); err != nil {
			respondTenantError(w, err)
			return
		}
	}

	list, err := h.service.ListCommands(r.Context(), stationID, from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (h *Handler) logAudit(r *http.Request, tenantID, commandID, stationID, deviceID, commandType string) {
	if h.auditLogger == nil || tenantID == "" {
		return
	}
	meta, _ := json.Marshal(map[string]any{
		"device_id":    deviceID,
		"command_type": commandType,
	})
	_ = h.auditLogger.Log(r.Context(), audit.Entry{
		TenantID:     tenantID,
		Actor:        auth.SubjectFromContext(r.Context()),
		Role:         string(auth.RoleFromContext(r.Context())),
		Action:       "command.issue",
		ResourceType: "command",
		ResourceID:   commandID,
		StationID:    stationID,
		Metadata:     meta,
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
