package integration_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"microgrid-cloud/internal/analytics/application"
	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/analytics/application/events"
	analyticsrepo "microgrid-cloud/internal/analytics/infrastructure/postgres"
	masterdatarepo "microgrid-cloud/internal/masterdata/infrastructure/postgres"
	telemetryadapters "microgrid-cloud/internal/telemetry/adapters/analytics"
	telemetry "microgrid-cloud/internal/telemetry/domain"
	telemetrypostgres "microgrid-cloud/internal/telemetry/infrastructure/postgres"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestTelemetryPointMappings_FactorAndIgnoreUnmapped(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if !tableExists(db, "telemetry_points") ||
		!tableExists(db, "analytics_statistics") ||
		!tableExists(db, "stations") ||
		!tableExists(db, "point_mappings") {
		t.Skip("missing tables; run migrations")
	}

	ctx := context.Background()
	tenantID := "tenant-map"
	stationID := "station-map-001"
	deviceID := "device-map-001"
	hourStart := time.Date(2026, time.January, 22, 6, 0, 0, 0, time.UTC)

	_, _ = db.ExecContext(ctx, "DELETE FROM analytics_statistics WHERE subject_id = $1", stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM telemetry_points WHERE tenant_id = $1 AND station_id = $2", tenantID, stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM point_mappings WHERE station_id = $1", stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM stations WHERE id = $1", stationID)

	if err := seedStationMappingsWithFactor(ctx, db, tenantID, stationID); err != nil {
		t.Fatalf("seed mappings: %v", err)
	}

	telemetryRepo := telemetrypostgres.NewTelemetryRepository(db)
	telemetryQuery := telemetrypostgres.NewTelemetryQuery(db)
	pointMappingRepo := masterdatarepo.NewPointMappingRepository(db)

	queryAdapter, err := telemetryadapters.NewQueryAdapter(tenantID, telemetryQuery, pointMappingRepo)
	if err != nil {
		t.Fatalf("new telemetry query adapter: %v", err)
	}

	statsRepo := analyticsrepo.NewPostgresStatisticRepository(db, stationID)
	bus := eventbus.NewInMemoryBus()
	clock := fixedClock{now: hourStart.Add(2 * time.Hour)}

	hourlyService := application.NewHourlyStatisticAppService(
		statsRepo,
		queryAdapter,
		telemetryadapters.SumStatisticCalculator{},
		bus,
		hourStatisticIDFactory{},
		clock,
	)
	application.WireAnalyticsEventBus(bus, hourlyService, nil, nil)

	if err := insertMappedMeasurements(ctx, telemetryRepo, tenantID, stationID, deviceID, hourStart); err != nil {
		t.Fatalf("insert measurements: %v", err)
	}

	err = bus.Publish(ctx, events.TelemetryWindowClosed{
		StationID:   stationID,
		WindowStart: hourStart,
		WindowEnd:   hourStart.Add(time.Hour),
		OccurredAt:  hourStart.Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("publish telemetry window closed: %v", err)
	}

	agg, err := statsRepo.FindByStationHour(ctx, stationID, hourStart)
	if err != nil {
		t.Fatalf("find hour statistic: %v", err)
	}
	if agg == nil {
		t.Fatalf("hour statistic missing")
	}
	fact, ok := agg.Fact()
	if !ok {
		t.Fatalf("hour statistic not completed")
	}

	assertFloat(t, fact.ChargeKWh, 2.0, "charge_kwh")
	assertFloat(t, fact.DischargeKWh, 2.0, "discharge_kwh")
	assertFloat(t, fact.Earnings, 3.0, "earnings")
	assertFloat(t, fact.CarbonReduction, 0.4, "carbon_reduction")
}

func seedStationMappingsWithFactor(ctx context.Context, db *sql.DB, tenantID, stationID string) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO stations (id, tenant_id, name, timezone, station_type, region)
VALUES ($1, $2, $3, 'UTC', 'microgrid', 'lab')`, stationID, tenantID, stationID+"-name")
	if err != nil {
		return err
	}

	type mapping struct {
		id       string
		pointKey string
		semantic string
		unit     string
		factor   float64
	}
	mappings := []mapping{
		{id: stationID + "-map-charge", pointKey: "tb_charge", semantic: "charge_power_kw", unit: "kW", factor: 2.0},
		{id: stationID + "-map-discharge", pointKey: "tb_discharge", semantic: "discharge_power_kw", unit: "kW", factor: 1.0},
		{id: stationID + "-map-earnings", pointKey: "tb_earnings", semantic: "earnings", unit: "CNY", factor: 1.0},
		{id: stationID + "-map-carbon", pointKey: "tb_carbon", semantic: "carbon_reduction", unit: "kg", factor: 0.1},
	}
	for _, item := range mappings {
		if _, err := db.ExecContext(ctx, `
INSERT INTO point_mappings (id, station_id, point_key, semantic, unit, factor)
VALUES ($1, $2, $3, $4, $5, $6)`, item.id, stationID, item.pointKey, item.semantic, item.unit, item.factor); err != nil {
			return err
		}
	}
	return nil
}

func insertMappedMeasurements(ctx context.Context, repo *telemetrypostgres.TelemetryRepository, tenantID, stationID, deviceID string, hourStart time.Time) error {
	ts := hourStart.Add(5 * time.Minute)
	charge := 1.0
	discharge := 2.0
	earnings := 3.0
	carbon := 4.0
	noise := 100.0

	measurements := []telemetry.Measurement{
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "tb_charge",
			TS:           ts,
			ValueNumeric: &charge,
			Quality:      "good",
		},
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "tb_discharge",
			TS:           ts,
			ValueNumeric: &discharge,
			Quality:      "good",
		},
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "tb_earnings",
			TS:           ts,
			ValueNumeric: &earnings,
			Quality:      "good",
		},
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "tb_carbon",
			TS:           ts,
			ValueNumeric: &carbon,
			Quality:      "good",
		},
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "unmapped_noise",
			TS:           ts,
			ValueNumeric: &noise,
			Quality:      "good",
		},
	}
	return repo.InsertMeasurements(ctx, measurements)
}

func assertFloat(t *testing.T, got, want float64, label string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s mismatch: got=%v want=%v", label, got, want)
	}
}
