package integration_test

import (
	"context"
	"database/sql"
	"io"
	"log"
	"math"
	"os"
	"testing"
	"time"

	"microgrid-cloud/internal/analytics/application"
	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/analytics/application/events"
	appstatistic "microgrid-cloud/internal/analytics/application/statistic"
	domainstatistic "microgrid-cloud/internal/analytics/domain/statistic"
	analyticsrepo "microgrid-cloud/internal/analytics/infrastructure/postgres"
	masterdatarepo "microgrid-cloud/internal/masterdata/infrastructure/postgres"
	settlementadapters "microgrid-cloud/internal/settlement/adapters/analytics"
	settlementapp "microgrid-cloud/internal/settlement/application"
	settlementrepo "microgrid-cloud/internal/settlement/infrastructure/postgres"
	settlementpricing "microgrid-cloud/internal/settlement/infrastructure/pricing"
	settlementinterfaces "microgrid-cloud/internal/settlement/interfaces"
	telemetryadapters "microgrid-cloud/internal/telemetry/adapters/analytics"
	telemetry "microgrid-cloud/internal/telemetry/domain"
	telemetrypostgres "microgrid-cloud/internal/telemetry/infrastructure/postgres"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestM2_DaySettlementClosedLoop_Postgres(t *testing.T) {
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
		!tableExists(db, "settlements_day") ||
		!tableExists(db, "stations") ||
		!tableExists(db, "point_mappings") {
		t.Skip("missing tables; run migrations")
	}

	ctx := context.Background()
	tenantID := "tenant-m2"
	stationID := "station-m2-001"
	deviceID := "device-m2-001"
	dayStart := time.Date(2026, time.January, 20, 0, 0, 0, 0, time.UTC)
	expectedHours := 24
	unitPrice := 1.2

	_, _ = db.ExecContext(ctx, "DELETE FROM settlements_day WHERE tenant_id = $1 AND station_id = $2", tenantID, stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM analytics_statistics WHERE subject_id = $1", stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM telemetry_points WHERE tenant_id = $1 AND station_id = $2", tenantID, stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM point_mappings WHERE station_id = $1", stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM stations WHERE id = $1", stationID)

	if err := seedStationAndMappings(ctx, db, tenantID, stationID); err != nil {
		t.Fatalf("seed masterdata: %v", err)
	}

	telemetryRepo := telemetrypostgres.NewTelemetryRepository(db)
	telemetryQuery := telemetrypostgres.NewTelemetryQuery(db)
	pointMappingRepo := masterdatarepo.NewPointMappingRepository(db)
	queryAdapter, err := telemetryadapters.NewQueryAdapter(tenantID, telemetryQuery, pointMappingRepo)
	if err != nil {
		t.Fatalf("new telemetry query adapter: %v", err)
	}

	bus := eventbus.NewInMemoryBus()
	statsRepo := analyticsrepo.NewPostgresStatisticRepository(db, stationID)

	clock := fixedClock{now: dayStart.Add(7 * 24 * time.Hour)}
	hourlyService := application.NewHourlyStatisticAppService(
		statsRepo,
		queryAdapter,
		telemetryadapters.SumStatisticCalculator{},
		bus,
		hourStatisticIDFactory{},
		clock,
	)
	rollupService, err := domainstatistic.NewDailyRollupService(statsRepo, clock, expectedHours)
	if err != nil {
		t.Fatalf("new daily rollup service: %v", err)
	}
	dailyApp, err := appstatistic.NewDailyRollupAppService(rollupService, statsRepo, bus, clock)
	if err != nil {
		t.Fatalf("new daily rollup app service: %v", err)
	}
	application.WireAnalyticsEventBus(bus, hourlyService, dailyApp, nil)

	dayEnergyReader := settlementadapters.NewDayHourEnergyReader(db)
	priceProvider, err := settlementpricing.NewFixedPriceProvider(unitPrice)
	if err != nil {
		t.Fatalf("new price provider: %v", err)
	}
	settlementRepo := settlementrepo.NewSettlementRepository(db, settlementrepo.WithTenantID(tenantID), settlementrepo.WithCurrency("CNY"))
	publisher := newSettlementRecorder()
	settlementApp, err := settlementapp.NewDaySettlementApplicationService(settlementRepo, dayEnergyReader, priceProvider, publisher, clock)
	if err != nil {
		t.Fatalf("new day settlement app service: %v", err)
	}
	settlementHandler, err := settlementinterfaces.NewDayStatisticCalculatedHandler(settlementApp, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("new settlement handler: %v", err)
	}
	bus.Subscribe(eventbus.EventTypeOf[events.StatisticCalculated](), settlementHandler.HandleStatisticCalculated)

	baseCharge := 1.0
	baseDischarge := 2.0
	baseEarnings := 0.1
	baseCarbon := 0.01

	for d := 0; d < 3; d++ {
		currentDay := dayStart.AddDate(0, 0, d)
		for h := 0; h < expectedHours; h++ {
			hourStart := currentDay.Add(time.Duration(h) * time.Hour)
			if err := insertHourMeasurements(ctx, telemetryRepo, tenantID, stationID, deviceID, hourStart, baseCharge, baseDischarge, baseEarnings, baseCarbon); err != nil {
				t.Fatalf("insert measurements: %v", err)
			}
			err := bus.Publish(ctx, events.TelemetryWindowClosed{
				StationID:   stationID,
				WindowStart: hourStart,
				WindowEnd:   hourStart.Add(time.Hour),
				OccurredAt:  hourStart.Add(30 * time.Minute),
			})
			if err != nil {
				t.Fatalf("publish telemetry window closed: %v", err)
			}
		}
	}

	expectedEnergy := (baseCharge + baseDischarge) * float64(expectedHours)
	expectedAmount := expectedEnergy * unitPrice

	settlements, err := loadSettlements(ctx, db, tenantID, stationID)
	if err != nil {
		t.Fatalf("load settlements: %v", err)
	}
	if len(settlements) != 3 {
		t.Fatalf("expected 3 settlement records, got %d", len(settlements))
	}
	for i, row := range settlements {
		expectedDay := dayStart.AddDate(0, 0, i)
		if !row.DayStart.Equal(expectedDay) {
			t.Fatalf("day start mismatch: got=%s want=%s", row.DayStart, expectedDay)
		}
		assertFloat(t, row.EnergyKWh, expectedEnergy, "energy")
		assertFloat(t, row.Amount, expectedAmount, "amount")
		if row.Version != 1 {
			t.Fatalf("expected version 1, got %d", row.Version)
		}
	}
	if publisher.Count() != 3 {
		t.Fatalf("expected SettlementCalculated 3 times, got %d", publisher.Count())
	}

	backfillDay := dayStart.AddDate(0, 0, 1)
	backfillHour := backfillDay.Add(6 * time.Hour)
	newCharge := 10.0
	newDischarge := 20.0

	if err := insertHourMeasurements(ctx, telemetryRepo, tenantID, stationID, deviceID, backfillHour, newCharge, newDischarge, baseEarnings, baseCarbon); err != nil {
		t.Fatalf("insert backfill measurements: %v", err)
	}
	err = bus.Publish(ctx, events.TelemetryWindowClosed{
		StationID:   stationID,
		WindowStart: backfillHour,
		WindowEnd:   backfillHour.Add(time.Hour),
		OccurredAt:  backfillHour.Add(40 * time.Minute),
		Recalculate: true,
	})
	if err != nil {
		t.Fatalf("publish backfill telemetry window closed: %v", err)
	}

	expectedEnergyAfter := expectedEnergy + (newCharge + newDischarge) - (baseCharge + baseDischarge)
	expectedAmountAfter := expectedEnergyAfter * unitPrice

	dayEnergy, err := loadDayEnergy(ctx, db, stationID, backfillDay)
	if err != nil {
		t.Fatalf("load day energy: %v", err)
	}
	assertFloat(t, dayEnergy, expectedEnergyAfter, "day energy")

	updated, err := loadSettlement(ctx, db, tenantID, stationID, backfillDay)
	if err != nil {
		t.Fatalf("load settlement after backfill: %v", err)
	}
	assertFloat(t, updated.EnergyKWh, expectedEnergyAfter, "settlement energy")
	assertFloat(t, updated.Amount, expectedAmountAfter, "settlement amount")
	if updated.Version != 2 {
		t.Fatalf("expected version 2 after backfill, got %d", updated.Version)
	}
	if publisher.Count() != 3 {
		t.Fatalf("expected SettlementCalculated to stay at 3, got %d", publisher.Count())
	}

	dayID, err := domainstatistic.BuildStatisticID(domainstatistic.GranularityDay, backfillDay)
	if err != nil {
		t.Fatalf("build day statistic id: %v", err)
	}
	err = bus.Publish(ctx, events.StatisticCalculated{
		StationID:   stationID,
		StatisticID: dayID,
		Granularity: domainstatistic.GranularityDay,
		PeriodStart: backfillDay,
		OccurredAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("publish duplicate day event: %v", err)
	}

	count, err := countSettlements(ctx, db, tenantID, stationID)
	if err != nil {
		t.Fatalf("count settlements: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 settlement rows after duplicate event, got %d", count)
	}
}

type hourStatisticIDFactory struct{}

func (hourStatisticIDFactory) HourID(stationID string, hourStart time.Time) (domainstatistic.StatisticID, error) {
	_ = stationID
	return domainstatistic.BuildStatisticID(domainstatistic.GranularityHour, hourStart)
}

type settlementRow struct {
	DayStart  time.Time
	EnergyKWh float64
	Amount    float64
	Version   int
}

type settlementRecorder struct {
	count int
}

func newSettlementRecorder() *settlementRecorder {
	return &settlementRecorder{}
}

func (r *settlementRecorder) PublishSettlementCalculated(ctx context.Context, event settlementapp.SettlementCalculated) error {
	_ = ctx
	_ = event
	r.count++
	return nil
}

func (r *settlementRecorder) Count() int { return r.count }

func insertHourMeasurements(ctx context.Context, repo *telemetrypostgres.TelemetryRepository, tenantID, stationID, deviceID string, hourStart time.Time, charge, discharge, earnings, carbon float64) error {
	ts := hourStart.Add(5 * time.Minute)
	measurements := []telemetry.Measurement{
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "charge_power_kw",
			TS:           ts,
			ValueNumeric: &charge,
			Quality:      "good",
		},
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "discharge_power_kw",
			TS:           ts,
			ValueNumeric: &discharge,
			Quality:      "good",
		},
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "earnings",
			TS:           ts,
			ValueNumeric: &earnings,
			Quality:      "good",
		},
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "carbon_reduction",
			TS:           ts,
			ValueNumeric: &carbon,
			Quality:      "good",
		},
	}
	return repo.InsertMeasurements(ctx, measurements)
}

func loadSettlements(ctx context.Context, db *sql.DB, tenantID, stationID string) ([]settlementRow, error) {
	rows, err := db.QueryContext(ctx, `
SELECT day_start, energy_kwh, amount, version
FROM settlements_day
WHERE tenant_id = $1 AND station_id = $2
ORDER BY day_start ASC`, tenantID, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []settlementRow
	for rows.Next() {
		var row settlementRow
		if err := rows.Scan(&row.DayStart, &row.EnergyKWh, &row.Amount, &row.Version); err != nil {
			return nil, err
		}
		row.DayStart = row.DayStart.UTC()
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func loadSettlement(ctx context.Context, db *sql.DB, tenantID, stationID string, dayStart time.Time) (settlementRow, error) {
	var row settlementRow
	err := db.QueryRowContext(ctx, `
SELECT day_start, energy_kwh, amount, version
FROM settlements_day
WHERE tenant_id = $1 AND station_id = $2 AND day_start = $3
LIMIT 1`, tenantID, stationID, dayStart.UTC()).Scan(&row.DayStart, &row.EnergyKWh, &row.Amount, &row.Version)
	if err != nil {
		return settlementRow{}, err
	}
	row.DayStart = row.DayStart.UTC()
	return row, nil
}

func loadDayEnergy(ctx context.Context, db *sql.DB, subjectID string, dayStart time.Time) (float64, error) {
	var charge float64
	var discharge float64
	err := db.QueryRowContext(ctx, `
SELECT charge_kwh, discharge_kwh
FROM analytics_statistics
WHERE subject_id = $1 AND time_type = 'DAY' AND period_start = $2
LIMIT 1`, subjectID, dayStart.UTC()).Scan(&charge, &discharge)
	if err != nil {
		return 0, err
	}
	return charge + discharge, nil
}

func countSettlements(ctx context.Context, db *sql.DB, tenantID, stationID string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM settlements_day
WHERE tenant_id = $1 AND station_id = $2`, tenantID, stationID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
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

func seedStationAndMappings(ctx context.Context, db *sql.DB, tenantID, stationID string) error {
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
	}
	mappings := []mapping{
		{id: stationID + "-map-charge", pointKey: "charge_power_kw", semantic: "charge_power_kw", unit: "kW"},
		{id: stationID + "-map-discharge", pointKey: "discharge_power_kw", semantic: "discharge_power_kw", unit: "kW"},
		{id: stationID + "-map-earnings", pointKey: "earnings", semantic: "earnings", unit: "CNY"},
		{id: stationID + "-map-carbon", pointKey: "carbon_reduction", semantic: "carbon_reduction", unit: "kg"},
	}
	for _, item := range mappings {
		if _, err := db.ExecContext(ctx, `
INSERT INTO point_mappings (id, station_id, point_key, semantic, unit, factor)
VALUES ($1, $2, $3, $4, $5, 1)`, item.id, stationID, item.pointKey, item.semantic, item.unit); err != nil {
			return err
		}
	}
	return nil
}

func assertFloat(t *testing.T, got, want float64, label string) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s mismatch: got=%v want=%v", label, got, want)
	}
}
