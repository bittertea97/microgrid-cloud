package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"microgrid-cloud/internal/auth"
	shadowapp "microgrid-cloud/internal/shadowrun/application"
	shadowrepo "microgrid-cloud/internal/shadowrun/infrastructure/postgres"
)

const timeLayout = time.RFC3339

// Handler provides shadowrun APIs.
type Handler struct {
	runner         *shadowapp.Runner
	repo           *shadowrepo.Repository
	tenantID       string
	stationChecker auth.StationTenantChecker
}

// NewHandler constructs a handler.
func NewHandler(runner *shadowapp.Runner, repo *shadowrepo.Repository, tenantID string, stationChecker auth.StationTenantChecker) (*Handler, error) {
	if runner == nil || repo == nil {
		return nil, errors.New("shadowrun handler: nil dependency")
	}
	return &Handler{runner: runner, repo: repo, tenantID: tenantID, stationChecker: stationChecker}, nil
}

// ServeHTTP routes shadowrun endpoints.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/v1/shadowrun/run" && r.Method == http.MethodPost:
		h.handleRun(w, r)
		return
	case r.URL.Path == "/api/v1/shadowrun/reports" && r.Method == http.MethodGet:
		h.handleReports(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/api/v1/shadowrun/reports/"):
		h.handleReportByID(w, r)
		return
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (h *Handler) handleRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID   string                `json:"tenant_id"`
		StationIDs []string              `json:"station_ids"`
		Month      string                `json:"month"`
		Thresholds *shadowapp.Thresholds `json:"thresholds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	tenantID := auth.TenantIDFromContext(r.Context())
	if tenantID == "" {
		tenantID = req.TenantID
	}
	if tenantID == "" {
		tenantID = h.tenantID
	}
	if tenantID == "" {
		http.Error(w, "tenant_id required", http.StatusBadRequest)
		return
	}
	if len(req.StationIDs) == 0 {
		http.Error(w, "station_ids required", http.StatusBadRequest)
		return
	}
	month, err := parseMonth(req.Month)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	jobDate := time.Now().UTC()
	var results []map[string]any
	for _, stationID := range req.StationIDs {
		if tenantID != "" {
			if err := ensureStationTenant(r, h.stationChecker, tenantID, stationID); err != nil {
				results = append(results, map[string]any{
					"station_id": stationID,
					"error":      tenantErrorMessage(err),
				})
				continue
			}
		}
		report, err := h.runner.Run(r.Context(), tenantID, stationID, month, jobDate, req.Thresholds)
		if err != nil {
			results = append(results, map[string]any{
				"station_id": stationID,
				"error":      err.Error(),
			})
			continue
		}
		if report != nil {
			results = append(results, map[string]any{
				"station_id": stationID,
				"report_id":  report.ID,
				"status":     report.Status,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

func (h *Handler) handleReports(w http.ResponseWriter, r *http.Request) {
	stationID := r.URL.Query().Get("station_id")
	if stationID == "" {
		http.Error(w, "station_id required", http.StatusBadRequest)
		return
	}
	tenantID := auth.TenantIDFromContext(r.Context())
	if tenantID != "" {
		if err := ensureStationTenant(r, h.stationChecker, tenantID, stationID); err != nil {
			respondTenantError(w, err)
			return
		}
	}
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
	reports, err := h.repo.ListReports(r.Context(), stationID, from, to)
	if err != nil {
		http.Error(w, "query reports error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reports)
}

func (h *Handler) handleReportByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/shadowrun/reports/")
	parts := strings.Split(path, "/")
	reportID := parts[0]
	if reportID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		h.handleReportGet(w, r, reportID)
		return
	}
	if len(parts) == 2 {
		switch parts[1] {
		case "download":
			if r.Method == http.MethodGet {
				h.handleDownload(w, r, reportID)
				return
			}
		case "replay":
			if r.Method == http.MethodPost {
				h.handleReplay(w, r, reportID)
				return
			}
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

func (h *Handler) handleReportGet(w http.ResponseWriter, r *http.Request, reportID string) {
	report, err := h.repo.GetReport(r.Context(), reportID)
	if err != nil {
		http.Error(w, "report not found", http.StatusNotFound)
		return
	}
	if report == nil {
		http.Error(w, "report not found", http.StatusNotFound)
		return
	}
	tenantID := auth.TenantIDFromContext(r.Context())
	if tenantID != "" && report.TenantID != tenantID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	resp := map[string]any{
		"id":                 report.ID,
		"job_id":             report.JobID,
		"station_id":         report.StationID,
		"month":              report.Month.Format("2006-01"),
		"report_date":        report.ReportDate.Format("2006-01-02"),
		"status":             report.Status,
		"location":           report.Location,
		"diff_summary":       json.RawMessage(report.DiffSummary),
		"recommended_action": report.RecommendedAction,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request, reportID string) {
	report, err := h.repo.GetReport(r.Context(), reportID)
	if err != nil || report == nil {
		http.Error(w, "report not found", http.StatusNotFound)
		return
	}
	tenantID := auth.TenantIDFromContext(r.Context())
	if tenantID != "" && report.TenantID != tenantID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, report.Location)
}

func (h *Handler) handleReplay(w http.ResponseWriter, r *http.Request, reportID string) {
	report, err := h.repo.GetReport(r.Context(), reportID)
	if err != nil || report == nil {
		http.Error(w, "report not found", http.StatusNotFound)
		return
	}
	tenantID := auth.TenantIDFromContext(r.Context())
	if tenantID != "" && report.TenantID != tenantID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	jobDate := time.Now().UTC()
	job, err := h.repo.CreateJob(r.Context(), &shadowrepo.Job{
		ID:        "replay-" + report.ID,
		TenantID:  report.TenantID,
		StationID: report.StationID,
		Month:     report.Month,
		JobDate:   jobDate,
		JobType:   "replay",
		Status:    "created",
	})
	if err == nil && job != nil {
		_ = h.repo.UpdateJobStatus(r.Context(), job.ID, "failed", "TODO: replay not implemented", nil, nil, true)
	}
	resp := map[string]any{
		"report_id": reportID,
		"status":    "todo",
		"message":   "replay not implemented; job recorded",
		"job_id":    "replay-" + report.ID,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(resp)
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

func parseMonth(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("month required")
	}
	t, err := time.Parse("2006-01", value)
	if err != nil {
		return time.Time{}, errors.New("month must be YYYY-MM")
	}
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC), nil
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

func tenantErrorMessage(err error) string {
	switch {
	case errors.Is(err, auth.ErrTenantMismatch):
		return "forbidden"
	case errors.Is(err, auth.ErrNotFound):
		return "not found"
	case err != nil:
		return err.Error()
	default:
		return ""
	}
}
