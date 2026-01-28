package integration_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	telemetry "microgrid-cloud/internal/telemetry/domain"
	telemetrypostgres "microgrid-cloud/internal/telemetry/infrastructure/postgres"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestTelemetryQuery_Postgres(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if !tableExists(db, "telemetry_points") {
		t.Skip("telemetry_points missing; run migrations")
	}

	ctx := context.Background()
	tenantID := "tenant-it"
	stationID := "station-it"
	deviceID := "device-it"
	hourStart := time.Date(2026, time.January, 21, 9, 0, 0, 0, time.UTC)

	_, _ = db.ExecContext(ctx, "DELETE FROM telemetry_points WHERE tenant_id = $1 AND station_id = $2", tenantID, stationID)

	repo := telemetrypostgres.NewTelemetryRepository(db)
	query := telemetrypostgres.NewTelemetryQuery(db)

	charge := 1.2
	discharge := 0.7

	measurements := []telemetry.Measurement{
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "charge_power_kw",
			TS:           hourStart.Add(5 * time.Minute),
			ValueNumeric: &charge,
			Quality:      "good",
		},
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "discharge_power_kw",
			TS:           hourStart.Add(5 * time.Minute),
			ValueNumeric: &discharge,
			Quality:      "good",
		},
	}

	if err := repo.InsertMeasurements(ctx, measurements); err != nil {
		t.Fatalf("insert measurements: %v", err)
	}

	points, err := query.QueryHour(ctx, tenantID, stationID, hourStart, hourStart.Add(time.Hour))
	if err != nil {
		t.Fatalf("query hour: %v", err)
	}
	if len(points) == 0 {
		t.Fatalf("expected telemetry points")
	}
	if points[0].Values["charge_power_kw"] != charge {
		t.Fatalf("charge value mismatch: got=%v want=%v", points[0].Values["charge_power_kw"], charge)
	}
	if points[0].Values["discharge_power_kw"] != discharge {
		t.Fatalf("discharge value mismatch: got=%v want=%v", points[0].Values["discharge_power_kw"], discharge)
	}
}

func tableExists(db *sql.DB, table string) bool {
	var exists bool
	err := db.QueryRow(`
SELECT EXISTS (
	SELECT 1
	FROM information_schema.tables
	WHERE table_schema = 'public' AND table_name = $1
)`, table).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}
