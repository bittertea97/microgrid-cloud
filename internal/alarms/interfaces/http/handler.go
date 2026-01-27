package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	alarmapp "microgrid-cloud/internal/alarms/application"
	alarms "microgrid-cloud/internal/alarms/domain"
	"microgrid-cloud/internal/auth"
)

const timeLayout = time.RFC3339

// Handler provides alarm HTTP endpoints.
type Handler struct {
	service        *alarmapp.Service
	stationChecker auth.StationTenantChecker
}

// NewHandler constructs a handler.
func NewHandler(service *alarmapp.Service, stationChecker auth.StationTenantChecker) (*Handler, error) {
	if service == nil {
		return nil, errors.New("alarms handler: nil service")
	}
	return &Handler{service: service, stationChecker: stationChecker}, nil
}

// ServeHTTP handles /api/v1/alarms and subroutes.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/v1/alarms":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		h.handleList(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/api/v1/alarms/"):
		h.handleAction(w, r)
		return
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	stationID := r.URL.Query().Get("station_id")
	if stationID == "" {
		http.Error(w, "station_id is required", http.StatusBadRequest)
		return
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
	status := r.URL.Query().Get("status")

	tenantID := auth.TenantIDFromContext(r.Context())
	if tenantID != "" {
		if err := ensureStationTenant(r, h.stationChecker, tenantID, stationID); err != nil {
			respondTenantError(w, err)
			return
		}
	}

	list, err := h.service.ListAlarms(r.Context(), stationID, status, from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (h *Handler) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/alarms/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := parts[0]
	action := parts[1]

	var (
		alarm *alarms.Alarm
		err   error
	)
	switch action {
	case "ack":
		alarm, err = h.service.AckAlarm(r.Context(), id)
	case "clear":
		alarm, err = h.service.ClearAlarm(r.Context(), id)
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		if errors.Is(err, alarms.ErrNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if errors.Is(err, auth.ErrTenantMismatch) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(alarm)
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
