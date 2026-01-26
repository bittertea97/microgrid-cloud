package apihttp

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

const timeLayout = time.RFC3339

// StatsHandler serves analytics statistics queries.
type StatsHandler struct {
	db *sql.DB
}

// NewStatsHandler constructs a StatsHandler.
func NewStatsHandler(db *sql.DB) *StatsHandler {
	return &StatsHandler{db: db}
}

// ServeHTTP handles GET /api/v1/stats.
func (h *StatsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h == nil || h.db == nil {
		http.Error(w, "server not ready", http.StatusServiceUnavailable)
		return
	}

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

	granularity := r.URL.Query().Get("granularity")
	timeType, err := resolveTimeType(granularity)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	stats, err := queryStats(r.Context(), h.db, stationID, timeType, from, to)
	if err != nil {
		http.Error(w, "query stats error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

// SettlementsHandler serves day settlement queries.
type SettlementsHandler struct {
	db       *sql.DB
	tenantID string
}

// NewSettlementsHandler constructs a SettlementsHandler.
func NewSettlementsHandler(db *sql.DB, tenantID string) *SettlementsHandler {
	return &SettlementsHandler{db: db, tenantID: tenantID}
}

// ServeHTTP handles GET /api/v1/settlements.
func (h *SettlementsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h == nil || h.db == nil {
		http.Error(w, "server not ready", http.StatusServiceUnavailable)
		return
	}
	if h.tenantID == "" {
		http.Error(w, "tenant_id is required", http.StatusServiceUnavailable)
		return
	}

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

	rows, err := querySettlements(r.Context(), h.db, h.tenantID, stationID, from, to)
	if err != nil {
		http.Error(w, "query settlements error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rows)
}

// ExportSettlementsCSVHandler serves settlement CSV exports.
type ExportSettlementsCSVHandler struct {
	db       *sql.DB
	tenantID string
}

// NewExportSettlementsCSVHandler constructs a ExportSettlementsCSVHandler.
func NewExportSettlementsCSVHandler(db *sql.DB, tenantID string) *ExportSettlementsCSVHandler {
	return &ExportSettlementsCSVHandler{db: db, tenantID: tenantID}
}

// ServeHTTP handles GET /api/v1/exports/settlements.csv.
func (h *ExportSettlementsCSVHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h == nil || h.db == nil {
		http.Error(w, "server not ready", http.StatusServiceUnavailable)
		return
	}
	if h.tenantID == "" {
		http.Error(w, "tenant_id is required", http.StatusServiceUnavailable)
		return
	}

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

	rows, err := querySettlements(r.Context(), h.db, h.tenantID, stationID, from, to)
	if err != nil {
		http.Error(w, "query settlements error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{
		"tenant_id",
		"station_id",
		"day_start",
		"energy_kwh",
		"amount",
		"currency",
		"status",
		"version",
		"created_at",
		"updated_at",
	})
	for _, row := range rows {
		_ = writer.Write([]string{
			row.TenantID,
			row.StationID,
			row.DayStart.Format(timeLayout),
			formatFloat(row.EnergyKWh),
			formatFloat(row.Amount),
			row.Currency,
			row.Status,
			formatInt(row.Version),
			formatTime(row.CreatedAt),
			formatTime(row.UpdatedAt),
		})
	}
	writer.Flush()
}

type statRow struct {
	SubjectID       string     `json:"subject_id"`
	TimeType        string     `json:"time_type"`
	TimeKey         string     `json:"time_key"`
	PeriodStart     time.Time  `json:"period_start"`
	StatisticID     string     `json:"statistic_id"`
	IsCompleted     bool       `json:"is_completed"`
	CompletedAt     *time.Time `json:"completed_at"`
	ChargeKWh       float64    `json:"charge_kwh"`
	DischargeKWh    float64    `json:"discharge_kwh"`
	Earnings        float64    `json:"earnings"`
	CarbonReduction float64    `json:"carbon_reduction"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type settlementRow struct {
	TenantID  string    `json:"tenant_id"`
	StationID string    `json:"station_id"`
	DayStart  time.Time `json:"day_start"`
	EnergyKWh float64   `json:"energy_kwh"`
	Amount    float64   `json:"amount"`
	Currency  string    `json:"currency"`
	Status    string    `json:"status"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func queryStats(ctx context.Context, db *sql.DB, stationID, timeType string, from, to time.Time) ([]statRow, error) {
	rows, err := db.QueryContext(ctx, `
SELECT
	subject_id,
	time_type,
	time_key,
	period_start,
	statistic_id,
	is_completed,
	completed_at,
	charge_kwh,
	discharge_kwh,
	earnings,
	carbon_reduction,
	created_at,
	updated_at
FROM analytics_statistics
WHERE subject_id = $1
	AND time_type = $2
	AND period_start >= $3
	AND period_start < $4
ORDER BY period_start ASC`, stationID, timeType, from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []statRow
	for rows.Next() {
		var row statRow
		var completedAt sql.NullTime
		if err := rows.Scan(
			&row.SubjectID,
			&row.TimeType,
			&row.TimeKey,
			&row.PeriodStart,
			&row.StatisticID,
			&row.IsCompleted,
			&completedAt,
			&row.ChargeKWh,
			&row.DischargeKWh,
			&row.Earnings,
			&row.CarbonReduction,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		row.PeriodStart = row.PeriodStart.UTC()
		row.CreatedAt = row.CreatedAt.UTC()
		row.UpdatedAt = row.UpdatedAt.UTC()
		if completedAt.Valid {
			t := completedAt.Time.UTC()
			row.CompletedAt = &t
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func querySettlements(ctx context.Context, db *sql.DB, tenantID, stationID string, from, to time.Time) ([]settlementRow, error) {
	rows, err := db.QueryContext(ctx, `
SELECT
	tenant_id,
	station_id,
	day_start,
	energy_kwh,
	amount,
	currency,
	status,
	version,
	created_at,
	updated_at
FROM settlements_day
WHERE tenant_id = $1
	AND station_id = $2
	AND day_start >= $3
	AND day_start < $4
ORDER BY day_start ASC`, tenantID, stationID, from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []settlementRow
	for rows.Next() {
		var row settlementRow
		if err := rows.Scan(
			&row.TenantID,
			&row.StationID,
			&row.DayStart,
			&row.EnergyKWh,
			&row.Amount,
			&row.Currency,
			&row.Status,
			&row.Version,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		row.DayStart = row.DayStart.UTC()
		row.CreatedAt = row.CreatedAt.UTC()
		row.UpdatedAt = row.UpdatedAt.UTC()
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
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

func resolveTimeType(granularity string) (string, error) {
	switch granularity {
	case "hour":
		return "HOUR", nil
	case "day":
		return "DAY", nil
	default:
		return "", errors.New("granularity must be hour or day")
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(timeLayout)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func formatInt(value int) string {
	return strconv.Itoa(value)
}
