package integration_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	settlementadapters "microgrid-cloud/internal/settlement/adapters/analytics"
	settlementapp "microgrid-cloud/internal/settlement/application"
	settlementrepo "microgrid-cloud/internal/settlement/infrastructure/postgres"
	settlementpricing "microgrid-cloud/internal/settlement/infrastructure/pricing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestTariffPricing_FixedModeMatchesLegacy(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if !tableExists(db, "analytics_statistics") ||
		!tableExists(db, "settlements_day") ||
		!tableExists(db, "tariff_plans") ||
		!tableExists(db, "tariff_rules") {
		t.Skip("missing tables; run migrations")
	}

	ctx := context.Background()
	tenantID := "tenant-tariff"
	stationID := "station-tariff-001"
	dayStart := time.Date(2026, time.January, 20, 0, 0, 0, 0, time.UTC)
	effectiveMonth := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	_, _ = db.ExecContext(ctx, "DELETE FROM settlements_day WHERE tenant_id = $1 AND station_id = $2", tenantID, stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM analytics_statistics WHERE subject_id = $1 AND period_start >= $2 AND period_start < $3", stationID, dayStart, dayStart.Add(24*time.Hour))
	_, _ = db.ExecContext(ctx, "DELETE FROM tariff_rules WHERE plan_id IN (SELECT id FROM tariff_plans WHERE tenant_id = $1 AND station_id = $2)", tenantID, stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM tariff_plans WHERE tenant_id = $1 AND station_id = $2", tenantID, stationID)

	if err := seedFixedTariffPlan(ctx, db, tenantID, stationID, effectiveMonth, 1.5); err != nil {
		t.Fatalf("seed fixed tariff: %v", err)
	}

	if err := seedHourlyStats(ctx, db, stationID, dayStart, 1.0, 2.0); err != nil {
		t.Fatalf("seed hourly stats: %v", err)
	}

	reader := settlementadapters.NewDayHourEnergyReader(db)
	provider := settlementpricing.NewTariffProvider(db, settlementpricing.WithTenantID(tenantID))
	repo := settlementrepo.NewSettlementRepository(db, settlementrepo.WithTenantID(tenantID))
	app, err := settlementapp.NewDaySettlementApplicationService(repo, reader, provider, nil, settlementapp.SystemClock{})
	if err != nil {
		t.Fatalf("new settlement app: %v", err)
	}

	if err := app.HandleDayEnergyCalculated(ctx, settlementapp.DayEnergyCalculated{
		SubjectID: stationID,
		DayStart:  dayStart,
	}); err != nil {
		t.Fatalf("handle day settlement: %v", err)
	}

	got, err := loadSettlement(ctx, db, tenantID, stationID, dayStart)
	if err != nil {
		t.Fatalf("load settlement: %v", err)
	}

	expectedEnergy := float64(24) * (1.0 + 2.0)
	expectedAmount := expectedEnergy * 1.5

	assertFloat(t, got.EnergyKWh, expectedEnergy, "energy")
	assertFloat(t, got.Amount, expectedAmount, "amount")
}

func TestTariffPricing_TouMode(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if !tableExists(db, "analytics_statistics") ||
		!tableExists(db, "settlements_day") ||
		!tableExists(db, "tariff_plans") ||
		!tableExists(db, "tariff_rules") {
		t.Skip("missing tables; run migrations")
	}

	ctx := context.Background()
	tenantID := "tenant-tariff"
	stationID := "station-tariff-002"
	dayStart := time.Date(2026, time.January, 21, 0, 0, 0, 0, time.UTC)
	effectiveMonth := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	_, _ = db.ExecContext(ctx, "DELETE FROM settlements_day WHERE tenant_id = $1 AND station_id = $2", tenantID, stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM analytics_statistics WHERE subject_id = $1 AND period_start >= $2 AND period_start < $3", stationID, dayStart, dayStart.Add(24*time.Hour))
	_, _ = db.ExecContext(ctx, "DELETE FROM tariff_rules WHERE plan_id IN (SELECT id FROM tariff_plans WHERE tenant_id = $1 AND station_id = $2)", tenantID, stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM tariff_plans WHERE tenant_id = $1 AND station_id = $2", tenantID, stationID)

	if err := seedTouTariffPlan(ctx, db, tenantID, stationID, effectiveMonth); err != nil {
		t.Fatalf("seed tou tariff: %v", err)
	}

	if err := seedHourlyStats(ctx, db, stationID, dayStart, 1.0, 0.0); err != nil {
		t.Fatalf("seed hourly stats: %v", err)
	}

	reader := settlementadapters.NewDayHourEnergyReader(db)
	provider := settlementpricing.NewTariffProvider(db, settlementpricing.WithTenantID(tenantID))
	repo := settlementrepo.NewSettlementRepository(db, settlementrepo.WithTenantID(tenantID))
	app, err := settlementapp.NewDaySettlementApplicationService(repo, reader, provider, nil, settlementapp.SystemClock{})
	if err != nil {
		t.Fatalf("new settlement app: %v", err)
	}

	if err := app.HandleDayEnergyCalculated(ctx, settlementapp.DayEnergyCalculated{
		SubjectID: stationID,
		DayStart:  dayStart,
	}); err != nil {
		t.Fatalf("handle day settlement: %v", err)
	}

	got, err := loadSettlement(ctx, db, tenantID, stationID, dayStart)
	if err != nil {
		t.Fatalf("load settlement: %v", err)
	}

	expectedAmount := 8*0.5 + 10*1.5 + 6*0.8
	assertFloat(t, got.Amount, expectedAmount, "amount")
}

func seedFixedTariffPlan(ctx context.Context, db *sql.DB, tenantID, stationID string, effectiveMonth time.Time, price float64) error {
	planID := stationID + "-fixed-" + effectiveMonth.Format("200601")
	_, err := db.ExecContext(ctx, `
INSERT INTO tariff_plans (id, tenant_id, station_id, effective_month, currency, mode)
VALUES ($1, $2, $3, $4, 'CNY', 'fixed')`, planID, tenantID, stationID, effectiveMonth)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO tariff_rules (id, plan_id, start_minute, end_minute, price_per_kwh)
VALUES ($1, $2, 0, 1440, $3)`, planID+"-rule", planID, price)
	return err
}

func seedTouTariffPlan(ctx context.Context, db *sql.DB, tenantID, stationID string, effectiveMonth time.Time) error {
	planID := stationID + "-tou-" + effectiveMonth.Format("200601")
	_, err := db.ExecContext(ctx, `
INSERT INTO tariff_plans (id, tenant_id, station_id, effective_month, currency, mode)
VALUES ($1, $2, $3, $4, 'CNY', 'tou')`, planID, tenantID, stationID, effectiveMonth)
	if err != nil {
		return err
	}

	type rule struct {
		id          string
		startMinute int
		endMinute   int
		price       float64
	}
	rules := []rule{
		{id: planID + "-r1", startMinute: 0, endMinute: 480, price: 0.5},
		{id: planID + "-r2", startMinute: 480, endMinute: 1080, price: 1.5},
		{id: planID + "-r3", startMinute: 1080, endMinute: 1440, price: 0.8},
	}
	for _, r := range rules {
		if _, err := db.ExecContext(ctx, `
INSERT INTO tariff_rules (id, plan_id, start_minute, end_minute, price_per_kwh)
VALUES ($1, $2, $3, $4, $5)`, r.id, planID, r.startMinute, r.endMinute, r.price); err != nil {
			return err
		}
	}
	return nil
}

func seedHourlyStats(ctx context.Context, db *sql.DB, stationID string, dayStart time.Time, charge, discharge float64) error {
	for i := 0; i < 24; i++ {
		hourStart := dayStart.Add(time.Duration(i) * time.Hour)
		if err := insertHourStat(ctx, db, stationID, hourStart, charge, discharge); err != nil {
			return err
		}
	}
	return nil
}

func insertHourStat(ctx context.Context, db *sql.DB, stationID string, hourStart time.Time, charge, discharge float64) error {
	timeKey := hourStart.UTC().Format("20060102T15")
	statID := "HOUR:" + timeKey
	_, err := db.ExecContext(ctx, `
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
	$1, 'HOUR', $2, $3, $4, TRUE, $5, $6, $7, 0, 0
)
ON CONFLICT (subject_id, time_type, time_key)
DO UPDATE SET
	period_start = EXCLUDED.period_start,
	statistic_id = EXCLUDED.statistic_id,
	is_completed = EXCLUDED.is_completed,
	completed_at = EXCLUDED.completed_at,
	charge_kwh = EXCLUDED.charge_kwh,
	discharge_kwh = EXCLUDED.discharge_kwh,
	updated_at = NOW()`, stationID, timeKey, hourStart.UTC(), statID, hourStart.Add(time.Hour).UTC(), charge, discharge)
	return err
}
