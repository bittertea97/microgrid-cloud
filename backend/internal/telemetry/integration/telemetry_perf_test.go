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

func TestTelemetryPerf_30dInsert_7dQuery(t *testing.T) {
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
	tenantID := "tenant-perf"
	stationID := "station-perf"
	deviceID := "device-perf"

	start := time.Now().UTC().AddDate(0, 0, -30).Truncate(24 * time.Hour)
	end := time.Now().UTC().Truncate(24 * time.Hour)

	_, _ = db.ExecContext(ctx, `
DELETE FROM telemetry_points
WHERE tenant_id = $1 AND station_id = $2 AND ts >= $3 AND ts < $4`, tenantID, stationID, start, end)

	repo := telemetrypostgres.NewTelemetryRepository(db)

	insertStart := time.Now()
	for day := 0; day < 30; day++ {
		dayStart := start.AddDate(0, 0, day)
		measurements := make([]telemetry.Measurement, 0, 48)
		for hour := 0; hour < 24; hour++ {
			ts := dayStart.Add(time.Duration(hour) * time.Hour)
			v1 := float64(hour) + 10
			v2 := float64(hour) + 20
			measurements = append(measurements,
				telemetry.Measurement{
					TenantID:     tenantID,
					StationID:    stationID,
					DeviceID:     deviceID,
					PointKey:     "charge_power_kw",
					TS:           ts,
					ValueNumeric: &v1,
				},
				telemetry.Measurement{
					TenantID:     tenantID,
					StationID:    stationID,
					DeviceID:     deviceID,
					PointKey:     "discharge_power_kw",
					TS:           ts,
					ValueNumeric: &v2,
				},
			)
		}
		if err := repo.InsertMeasurements(ctx, measurements); err != nil {
			t.Fatalf("insert measurements: %v", err)
		}
	}
	insertElapsed := time.Since(insertStart)

	queryStart := time.Now()
	queryFrom := end.AddDate(0, 0, -7)
	rows, err := db.QueryContext(ctx, `
SELECT ts, point_key, value_numeric
FROM telemetry_points
WHERE tenant_id = $1 AND station_id = $2 AND ts >= $3 AND ts < $4
ORDER BY ts ASC`, tenantID, stationID, queryFrom, end)
	if err != nil {
		t.Fatalf("query curve: %v", err)
	}
	count := 0
	for rows.Next() {
		var ts time.Time
		var key string
		var value sql.NullFloat64
		if err := rows.Scan(&ts, &key, &value); err != nil {
			rows.Close()
			t.Fatalf("scan curve: %v", err)
		}
		count++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	curveElapsed := time.Since(queryStart)

	statStart := time.Now()
	statRow := db.QueryRowContext(ctx, `
SELECT point_key, avg(value_numeric)
FROM telemetry_points
WHERE tenant_id = $1 AND station_id = $2 AND ts >= $3 AND ts < $4
GROUP BY point_key`, tenantID, stationID, queryFrom, end)
	var key string
	var avg sql.NullFloat64
	_ = statRow.Scan(&key, &avg)
	statElapsed := time.Since(statStart)

	t.Logf("perf insert 30d rows=%d elapsed=%s", 30*24*2, insertElapsed)
	t.Logf("perf query 7d curve rows=%d elapsed=%s", count, curveElapsed)
	t.Logf("perf query 7d avg elapsed=%s", statElapsed)
}
