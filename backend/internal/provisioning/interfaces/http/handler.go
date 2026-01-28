package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"microgrid-cloud/internal/audit"
	"microgrid-cloud/internal/auth"
	provisioning "microgrid-cloud/internal/provisioning/application"
)

// StationProvisioningHandler handles station provisioning requests.
type StationProvisioningHandler struct {
	service     *provisioning.Service
	auditLogger audit.Logger
}

// NewStationProvisioningHandler constructs a handler.
func NewStationProvisioningHandler(service *provisioning.Service, auditLogger audit.Logger) (*StationProvisioningHandler, error) {
	if service == nil {
		return nil, errors.New("provisioning handler: nil service")
	}
	return &StationProvisioningHandler{service: service, auditLogger: auditLogger}, nil
}

// ServeHTTP handles POST /api/v1/provisioning/stations.
func (h *StationProvisioningHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body error", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req provisioning.ProvisionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	tenantID := auth.TenantIDFromContext(r.Context())
	if tenantID != "" && req.Station.TenantID != "" && req.Station.TenantID != tenantID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if tenantID != "" {
		req.Station.TenantID = tenantID
	}

	resp, err := h.service.ProvisionStation(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
	h.logAudit(r, req.Station.TenantID, resp.StationID)
}

func (h *StationProvisioningHandler) logAudit(r *http.Request, tenantID, stationID string) {
	if h.auditLogger == nil || tenantID == "" {
		return
	}
	_ = h.auditLogger.Log(r.Context(), audit.Entry{
		TenantID:     tenantID,
		Actor:        auth.SubjectFromContext(r.Context()),
		Role:         string(auth.RoleFromContext(r.Context())),
		Action:       "provision.station",
		ResourceType: "station",
		ResourceID:   stationID,
		StationID:    stationID,
		Metadata:     nil,
		IP:           audit.ClientIP(r),
		UserAgent:    r.UserAgent(),
	})
}
