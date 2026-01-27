package interfaces

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"microgrid-cloud/internal/audit"
	"microgrid-cloud/internal/auth"
	"microgrid-cloud/internal/observability/metrics"
	statementapp "microgrid-cloud/internal/settlement/application"
	settlement "microgrid-cloud/internal/settlement/domain"
)

// StatementHandler handles statement APIs.
type StatementHandler struct {
	service        *statementapp.StatementService
	stationChecker auth.StationTenantChecker
	auditLogger    audit.Logger
}

// NewStatementHandler constructs a handler.
func NewStatementHandler(service *statementapp.StatementService, stationChecker auth.StationTenantChecker, auditLogger audit.Logger) (*StatementHandler, error) {
	if service == nil {
		return nil, errors.New("statement handler: nil service")
	}
	return &StatementHandler{service: service, stationChecker: stationChecker, auditLogger: auditLogger}, nil
}

// ServeHTTP handles statement routes under /api/v1/statements.
func (h *StatementHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/api/v1/statements/generate" && r.Method == http.MethodPost {
		h.handleGenerate(w, r)
		return
	}
	if path == "/api/v1/statements" && r.Method == http.MethodGet {
		h.handleList(w, r)
		return
	}
	if strings.HasPrefix(path, "/api/v1/statements/") {
		rest := strings.TrimPrefix(path, "/api/v1/statements/")
		h.handleByID(w, r, rest)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (h *StatementHandler) handleGenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID   string `json:"tenant_id"`
		StationID  string `json:"station_id"`
		Month      string `json:"month"`
		Category   string `json:"category"`
		Regenerate bool   `json:"regenerate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	tenantID := auth.TenantIDFromContext(r.Context())
	if tenantID != "" && req.TenantID != "" && req.TenantID != tenantID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if tenantID != "" {
		if err := ensureStationTenant(r, h.stationChecker, tenantID, req.StationID); err != nil {
			respondTenantError(w, err)
			return
		}
	}
	stmt, err := h.service.Generate(r.Context(), req.StationID, req.Month, req.Category, req.Regenerate)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	resp := map[string]any{
		"statement_id": stmt.ID,
		"status":       stmt.Status,
		"version":      stmt.Version,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
	action := "statement.generate"
	if req.Regenerate {
		action = "statement.regenerate"
	}
	h.logAudit(r, req.StationID, stmt.ID, action, map[string]any{
		"category":   req.Category,
		"month":      req.Month,
		"regenerate": req.Regenerate,
	})
}

func (h *StatementHandler) handleList(w http.ResponseWriter, r *http.Request) {
	stationID := r.URL.Query().Get("station_id")
	month := r.URL.Query().Get("month")
	category := r.URL.Query().Get("category")
	tenantID := auth.TenantIDFromContext(r.Context())
	if tenantID != "" {
		if err := ensureStationTenant(r, h.stationChecker, tenantID, stationID); err != nil {
			respondTenantError(w, err)
			return
		}
	}
	list, err := h.service.List(r.Context(), stationID, month, category)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (h *StatementHandler) handleByID(w http.ResponseWriter, r *http.Request, rest string) {
	if rest == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	parts := strings.Split(rest, "/")
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		h.handleGet(w, r, id)
		return
	}
	if len(parts) == 2 {
		switch parts[1] {
		case "freeze":
			if r.Method == http.MethodPost {
				h.handleFreeze(w, r, id)
				return
			}
		case "void":
			if r.Method == http.MethodPost {
				h.handleVoid(w, r, id)
				return
			}
		case "export.pdf":
			if r.Method == http.MethodGet {
				h.handleExportPDF(w, r, id)
				return
			}
		case "export.xlsx":
			if r.Method == http.MethodGet {
				h.handleExportXLSX(w, r, id)
				return
			}
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

func (h *StatementHandler) handleGet(w http.ResponseWriter, r *http.Request, id string) {
	stmt, items, err := h.service.Get(r.Context(), id)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	resp := struct {
		Statement *settlement.StatementAggregate `json:"statement"`
		Items     []settlement.StatementItem     `json:"items"`
	}{Statement: stmt, Items: items}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *StatementHandler) handleFreeze(w http.ResponseWriter, r *http.Request, id string) {
	stmt, err := h.service.Freeze(r.Context(), id)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	resp := map[string]any{
		"statement_id":  stmt.ID,
		"status":        stmt.Status,
		"version":       stmt.Version,
		"snapshot_hash": stmt.SnapshotHash,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
	h.logAudit(r, stmt.StationID, stmt.ID, "statement.freeze", map[string]any{
		"status": stmt.Status,
	})
}

func (h *StatementHandler) handleVoid(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	stmt, err := h.service.Void(r.Context(), id, req.Reason)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	resp := map[string]any{
		"statement_id": stmt.ID,
		"status":       stmt.Status,
		"version":      stmt.Version,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
	h.logAudit(r, stmt.StationID, stmt.ID, "statement.void", map[string]any{
		"reason": req.Reason,
	})
}

func (h *StatementHandler) handleExportPDF(w http.ResponseWriter, r *http.Request, id string) {
	start := time.Now()
	result := metrics.ResultSuccess
	defer func() {
		metrics.ObserveStatementExport("pdf", result, time.Since(start))
	}()

	stmt, items, err := h.service.Get(r.Context(), id)
	if err != nil {
		result = metrics.ResultError
		respondServiceError(w, err)
		return
	}
	data, err := BuildStatementPDF(stmt, items)
	if err != nil {
		result = metrics.ResultError
		http.Error(w, "export pdf error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	h.logAudit(r, stmt.StationID, stmt.ID, "statement.export", map[string]any{"format": "pdf"})
}

func (h *StatementHandler) handleExportXLSX(w http.ResponseWriter, r *http.Request, id string) {
	start := time.Now()
	result := metrics.ResultSuccess
	defer func() {
		metrics.ObserveStatementExport("xlsx", result, time.Since(start))
	}()

	stmt, items, err := h.service.Get(r.Context(), id)
	if err != nil {
		result = metrics.ResultError
		respondServiceError(w, err)
		return
	}
	data, err := BuildStatementXLSX(stmt, items)
	if err != nil {
		result = metrics.ResultError
		http.Error(w, "export xlsx error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	h.logAudit(r, stmt.StationID, stmt.ID, "statement.export", map[string]any{"format": "xlsx"})
}

func (h *StatementHandler) logAudit(r *http.Request, stationID, statementID, action string, meta map[string]any) {
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
		ResourceType: "statement",
		ResourceID:   statementID,
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

func respondServiceError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, auth.ErrTenantMismatch) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}
