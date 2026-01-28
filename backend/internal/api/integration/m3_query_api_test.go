package integration_test

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	domainstatistic "microgrid-cloud/internal/analytics/domain/statistic"
	apihttp "microgrid-cloud/internal/api/http"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestM3_QueryAPI_JSONAndCSV(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := applyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	ctx := context.Background()
	tenantID := "tenant-m3"
	stationID := "station-m3-001"

	_, _ = db.ExecContext(ctx, "DELETE FROM settlements_day WHERE tenant_id = $1 AND station_id = $2", tenantID, stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM analytics_statistics WHERE subject_id = $1", stationID)

	dayStart := time.Date(2026, time.January, 20, 0, 0, 0, 0, time.UTC)
	hourStart := dayStart.Add(6 * time.Hour)

	if err := insertStatisticRow(ctx, db, stationID, domainstatistic.GranularityHour, hourStart, 1.1, 2.2, 0.3, 0.04); err != nil {
		t.Fatalf("insert hour statistic: %v", err)
	}
	if err := insertStatisticRow(ctx, db, stationID, domainstatistic.GranularityDay, dayStart, 24.0, 48.0, 3.0, 0.4); err != nil {
		t.Fatalf("insert day statistic: %v", err)
	}

	if err := insertSettlementRow(ctx, db, tenantID, stationID, dayStart, 72.0, 72.0, "CNY", "CALCULATED", 1); err != nil {
		t.Fatalf("insert settlement: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/stats", apihttp.NewStatsHandler(db, nil))
	mux.Handle("/api/v1/settlements", apihttp.NewSettlementsHandler(db, tenantID, nil))
	mux.Handle("/api/v1/exports/settlements.csv", apihttp.NewExportSettlementsCSVHandler(db, tenantID, nil))

	server := httptest.NewServer(mux)
	defer server.Close()

	from := dayStart.Format(time.RFC3339)
	to := dayStart.Add(24 * time.Hour).Format(time.RFC3339)

	statsURL := server.URL + "/api/v1/stats?station_id=" + stationID + "&from=" + from + "&to=" + to + "&granularity=hour"
	statsResp, err := http.Get(statsURL)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	defer statsResp.Body.Close()
	if statsResp.StatusCode != http.StatusOK {
		t.Fatalf("stats status: %d", statsResp.StatusCode)
	}

	var stats []statResponse
	if err := json.NewDecoder(statsResp.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 hour stat, got %d", len(stats))
	}
	if stats[0].SubjectID != stationID {
		t.Fatalf("subject_id mismatch: got=%s", stats[0].SubjectID)
	}
	if stats[0].TimeType != "HOUR" {
		t.Fatalf("time_type mismatch: got=%s", stats[0].TimeType)
	}
	if stats[0].ChargeKWh != 1.1 {
		t.Fatalf("charge_kwh mismatch: got=%v", stats[0].ChargeKWh)
	}

	settleURL := server.URL + "/api/v1/settlements?station_id=" + stationID + "&from=" + from + "&to=" + to
	settleResp, err := http.Get(settleURL)
	if err != nil {
		t.Fatalf("get settlements: %v", err)
	}
	defer settleResp.Body.Close()
	if settleResp.StatusCode != http.StatusOK {
		t.Fatalf("settlements status: %d", settleResp.StatusCode)
	}

	var settlements []settlementResponse
	if err := json.NewDecoder(settleResp.Body).Decode(&settlements); err != nil {
		t.Fatalf("decode settlements: %v", err)
	}
	if len(settlements) != 1 {
		t.Fatalf("expected 1 settlement, got %d", len(settlements))
	}
	if settlements[0].StationID != stationID {
		t.Fatalf("station_id mismatch: got=%s", settlements[0].StationID)
	}
	if settlements[0].EnergyKWh != 72.0 || settlements[0].Amount != 72.0 {
		t.Fatalf("settlement amount mismatch: energy=%v amount=%v", settlements[0].EnergyKWh, settlements[0].Amount)
	}

	csvURL := server.URL + "/api/v1/exports/settlements.csv?station_id=" + stationID + "&from=" + from + "&to=" + to
	csvResp, err := http.Get(csvURL)
	if err != nil {
		t.Fatalf("get csv: %v", err)
	}
	defer csvResp.Body.Close()
	if csvResp.StatusCode != http.StatusOK {
		t.Fatalf("csv status: %d", csvResp.StatusCode)
	}

	reader := csv.NewReader(csvResp.Body)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 csv rows (header + 1), got %d", len(records))
	}
	if records[0][0] != "tenant_id" || records[0][1] != "station_id" || records[0][2] != "day_start" {
		t.Fatalf("csv header mismatch: %v", records[0])
	}
	if records[1][1] != stationID {
		t.Fatalf("csv station_id mismatch: %v", records[1][1])
	}
}

type statResponse struct {
	SubjectID string  `json:"subject_id"`
	TimeType  string  `json:"time_type"`
	ChargeKWh float64 `json:"charge_kwh"`
}

type settlementResponse struct {
	StationID string  `json:"station_id"`
	EnergyKWh float64 `json:"energy_kwh"`
	Amount    float64 `json:"amount"`
}

func applyMigrations(db *sql.DB) error {
	root := projectRoot()
	files := []string{
		filepath.Join(root, "migrations", "001_init.sql"),
		filepath.Join(root, "migrations", "002_settlement.sql"),
	}
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(content)); err != nil {
			return err
		}
	}
	return nil
}

func projectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return filepath.Clean(filepath.Join(dir, "..", "..", ".."))
}

func insertStatisticRow(ctx context.Context, db *sql.DB, subjectID string, granularity domainstatistic.Granularity, periodStart time.Time, charge, discharge, earnings, carbon float64) error {
	timeKey, err := domainstatistic.NewTimeKey(domainstatistic.TimeType(granularity), periodStart)
	if err != nil {
		return err
	}
	statID, err := domainstatistic.BuildStatisticID(granularity, periodStart)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO analytics_statistics (
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
	carbon_reduction
) VALUES (
	$1, $2, $3, $4, $5, TRUE, $6, $7, $8, $9, $10
)`, subjectID, string(granularity), timeKey.String(), periodStart.UTC(), string(statID), periodStart.Add(time.Hour).UTC(), charge, discharge, earnings, carbon)
	return err
}

func insertSettlementRow(ctx context.Context, db *sql.DB, tenantID, stationID string, dayStart time.Time, energy, amount float64, currency, status string, version int) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO settlements_day (
	tenant_id,
	station_id,
	day_start,
	energy_kwh,
	amount,
	currency,
	status,
	version
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8
)`, tenantID, stationID, dayStart.UTC(), energy, amount, currency, status, version)
	return err
}
