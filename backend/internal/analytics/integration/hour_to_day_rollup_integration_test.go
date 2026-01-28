package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"microgrid-cloud/internal/analytics/application"
	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/analytics/application/events"
	appstatistic "microgrid-cloud/internal/analytics/application/statistic"
	domainstatistic "microgrid-cloud/internal/analytics/domain/statistic"
	"microgrid-cloud/internal/analytics/infrastructure/memory"
	analyticsrepo "microgrid-cloud/internal/analytics/infrastructure/postgres"
	masterdatarepo "microgrid-cloud/internal/masterdata/infrastructure/postgres"
	telemetryadapters "microgrid-cloud/internal/telemetry/adapters/analytics"
	telemetry "microgrid-cloud/internal/telemetry/domain"
	telemetrypostgres "microgrid-cloud/internal/telemetry/infrastructure/postgres"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestTelemetryWindowClosed_HourToDayRollup_ClosedLoop(t *testing.T) {
	ctx := context.Background()

	stationID := "station-integration-001"
	dayStart := time.Date(2026, time.January, 20, 0, 0, 0, 0, time.UTC)
	clock := fixedClock{now: dayStart.Add(48 * time.Hour)}

	repo := newRecalcStatisticRepository()
	bus := eventbus.NewInMemoryBus()
	telemetry := newTelemetryStore()
	recorder := newEventRecorder()

	hourlyApp := application.NewHourlyStatisticAppService(
		repo,
		telemetry,
		sumStatisticCalculator{},
		bus,
		hourStatisticIDFactory{},
		clock,
	)

	rollupService, err := domainstatistic.NewDailyRollupService(repo, clock, 24)
	if err != nil {
		t.Fatalf("new daily rollup service: %v", err)
	}
	dailyApp, err := appstatistic.NewDailyRollupAppService(rollupService, repo, bus, clock)
	if err != nil {
		t.Fatalf("new daily rollup app service: %v", err)
	}

	application.WireAnalyticsEventBus(bus, hourlyApp, dailyApp, nil)
	bus.Subscribe(eventbus.EventTypeOf[events.StatisticCalculated](), recorder.HandleStatisticCalculated)

	expectedDay := domainstatistic.StatisticFact{}
	hourFacts := make(map[time.Time]domainstatistic.StatisticFact, 24)

	for i := 0; i < 24; i++ {
		hourStart := dayStart.Add(time.Duration(i) * time.Hour)
		point := application.TelemetryPoint{
			At:               hourStart.Add(10 * time.Minute),
			ChargePowerKW:    float64(i + 1),
			DischargePowerKW: float64(i+1) * 0.5,
			Earnings:         float64(i+1) * 0.1,
			CarbonReduction:  float64(i+1) * 0.01,
		}
		telemetry.SetHour(hourStart, []application.TelemetryPoint{point})

		fact := domainstatistic.StatisticFact{
			ChargeKWh:       point.ChargePowerKW,
			DischargeKWh:    point.DischargePowerKW,
			Earnings:        point.Earnings,
			CarbonReduction: point.CarbonReduction,
		}
		hourFacts[hourStart] = fact
		expectedDay = addFacts(expectedDay, fact)
	}

	for i := 0; i < 24; i++ {
		hourStart := dayStart.Add(time.Duration(i) * time.Hour)
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

	dayAgg := waitForDayAggregate(t, ctx, repo, dayStart, 2*time.Second)
	if dayAgg == nil {
		t.Fatalf("day aggregate missing")
	}
	assertSingleDayAggregate(t, ctx, repo, dayStart, expectedDay)

	hourCount, dayCount, _, _ := recorder.Counts()
	if hourCount != 24 {
		t.Fatalf("expected 24 hour statistic events, got %d", hourCount)
	}
	if dayCount != 1 {
		t.Fatalf("expected 1 day statistic event, got %d", dayCount)
	}

	// Backfill one hour with new telemetry input.
	backfillHour := dayStart.Add(7 * time.Hour)
	oldFact := hourFacts[backfillHour]

	newPoint := application.TelemetryPoint{
		At:               backfillHour.Add(15 * time.Minute),
		ChargePowerKW:    100,
		DischargePowerKW: 50,
		Earnings:         25,
		CarbonReduction:  12,
	}
	telemetry.SetHour(backfillHour, []application.TelemetryPoint{newPoint})
	newFact := domainstatistic.StatisticFact{
		ChargeKWh:       newPoint.ChargePowerKW,
		DischargeKWh:    newPoint.DischargePowerKW,
		Earnings:        newPoint.Earnings,
		CarbonReduction: newPoint.CarbonReduction,
	}

	expectedDayAfter := expectedDay
	expectedDayAfter.ChargeKWh += newFact.ChargeKWh - oldFact.ChargeKWh
	expectedDayAfter.DischargeKWh += newFact.DischargeKWh - oldFact.DischargeKWh
	expectedDayAfter.Earnings += newFact.Earnings - oldFact.Earnings
	expectedDayAfter.CarbonReduction += newFact.CarbonReduction - oldFact.CarbonReduction

	repo.ForceRecalculateHour(backfillHour)
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

	recorder.Reset()

	hourID, err := domainstatistic.BuildStatisticID(domainstatistic.GranularityHour, backfillHour)
	if err != nil {
		t.Fatalf("build hour statistic id: %v", err)
	}
	err = bus.Publish(ctx, events.StatisticCalculated{
		StationID:   stationID,
		StatisticID: hourID,
		Granularity: domainstatistic.GranularityHour,
		PeriodStart: backfillHour,
		OccurredAt:  clock.Now(),
		Recalculate: true,
	})
	if err != nil {
		t.Fatalf("publish recalculation hour event: %v", err)
	}

	dayAggAfter := waitForDayAggregate(t, ctx, repo, dayStart, 2*time.Second)
	if dayAggAfter == nil {
		t.Fatalf("day aggregate missing after backfill")
	}
	assertSingleDayAggregate(t, ctx, repo, dayStart, expectedDayAfter)

	_, dayCount, _, _ = recorder.Counts()
	if dayCount != 1 {
		t.Fatalf("expected 1 day statistic event in backfill, got %d", dayCount)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

type hourStatisticIDFactory struct{}

func (hourStatisticIDFactory) HourID(stationID string, hourStart time.Time) (domainstatistic.StatisticID, error) {
	_ = stationID
	return domainstatistic.BuildStatisticID(domainstatistic.GranularityHour, hourStart)
}

type sumStatisticCalculator struct{}

func (sumStatisticCalculator) CalculateHour(ctx context.Context, stationID string, periodStart time.Time, telemetry []application.TelemetryPoint) (domainstatistic.StatisticFact, error) {
	_ = ctx
	_ = stationID
	_ = periodStart

	var fact domainstatistic.StatisticFact
	for _, point := range telemetry {
		fact.ChargeKWh += point.ChargePowerKW
		fact.DischargeKWh += point.DischargePowerKW
		fact.Earnings += point.Earnings
		fact.CarbonReduction += point.CarbonReduction
	}
	return fact, nil
}

type telemetryStore struct {
	mu   sync.RWMutex
	data map[time.Time][]application.TelemetryPoint
}

func newTelemetryStore() *telemetryStore {
	return &telemetryStore{
		data: make(map[time.Time][]application.TelemetryPoint),
	}
}

func (s *telemetryStore) SetHour(hourStart time.Time, points []application.TelemetryPoint) {
	s.mu.Lock()
	defer s.mu.Unlock()

	copied := make([]application.TelemetryPoint, len(points))
	copy(copied, points)
	s.data[hourStart] = copied
}

func (s *telemetryStore) QueryHour(ctx context.Context, stationID string, hourStart, hourEnd time.Time) ([]application.TelemetryPoint, error) {
	_ = ctx
	_ = stationID
	_ = hourEnd

	s.mu.RLock()
	defer s.mu.RUnlock()
	points := s.data[hourStart]
	if points == nil {
		return nil, nil
	}
	copied := make([]application.TelemetryPoint, len(points))
	copy(copied, points)
	return copied, nil
}

type recalcStatisticRepository struct {
	*memory.StatisticRepository
	mu    sync.RWMutex
	force map[time.Time]bool
}

func newRecalcStatisticRepository() *recalcStatisticRepository {
	return &recalcStatisticRepository{
		StatisticRepository: memory.NewStatisticRepository(),
		force:               make(map[time.Time]bool),
	}
}

func (r *recalcStatisticRepository) ForceRecalculateHour(hourStart time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.force[hourStart] = true
}

func (r *recalcStatisticRepository) FindByStationHour(ctx context.Context, stationID string, hourStart time.Time) (*domainstatistic.StatisticAggregate, error) {
	r.mu.RLock()
	force := r.force[hourStart]
	r.mu.RUnlock()
	if force {
		return nil, nil
	}
	agg, err := r.StatisticRepository.FindByStationHour(ctx, stationID, hourStart)
	if err != nil && errors.Is(err, domainstatistic.ErrStatisticNotFound) {
		return nil, nil
	}
	return agg, err
}

type eventRecorder struct {
	mu         sync.RWMutex
	hourCount  int
	dayCount   int
	monthCount int
	yearCount  int
}

func newEventRecorder() *eventRecorder {
	return &eventRecorder{}
}

func (r *eventRecorder) HandleStatisticCalculated(ctx context.Context, event any) error {
	_ = ctx

	var evt events.StatisticCalculated
	switch e := event.(type) {
	case events.StatisticCalculated:
		evt = e
	case *events.StatisticCalculated:
		if e == nil {
			return nil
		}
		evt = *e
	default:
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	switch evt.Granularity {
	case domainstatistic.GranularityHour:
		r.hourCount++
	case domainstatistic.GranularityDay:
		r.dayCount++
	case domainstatistic.GranularityMonth:
		r.monthCount++
	case domainstatistic.GranularityYear:
		r.yearCount++
	}
	return nil
}

func (r *eventRecorder) Counts() (int, int, int, int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hourCount, r.dayCount, r.monthCount, r.yearCount
}

func (r *eventRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hourCount = 0
	r.dayCount = 0
	r.monthCount = 0
	r.yearCount = 0
}

func waitForDayAggregate(t *testing.T, ctx context.Context, repo domainstatistic.StatisticRepository, dayStart time.Time, timeout time.Duration) *domainstatistic.StatisticAggregate {
	t.Helper()

	return waitForAggregate(t, ctx, repo, domainstatistic.GranularityDay, dayStart, timeout)
}

func waitForAggregate(t *testing.T, ctx context.Context, repo domainstatistic.StatisticRepository, granularity domainstatistic.Granularity, periodStart time.Time, timeout time.Duration) *domainstatistic.StatisticAggregate {
	t.Helper()

	aggID, err := domainstatistic.BuildStatisticID(granularity, periodStart)
	if err != nil {
		t.Fatalf("build statistic id: %v", err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		agg, err := repo.Get(ctx, aggID)
		if err == nil && agg != nil {
			return agg
		}
		if err != nil && !errors.Is(err, domainstatistic.ErrStatisticNotFound) {
			t.Fatalf("get aggregate: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("aggregate not found within %s", timeout)
	return nil
}

func assertSingleDayAggregate(t *testing.T, ctx context.Context, repo domainstatistic.StatisticRepository, dayStart time.Time, expected domainstatistic.StatisticFact) {
	t.Helper()

	assertSingleAggregate(t, ctx, repo, domainstatistic.GranularityDay, dayStart, dayStart.Add(24*time.Hour), expected)
}

func assertSingleMonthAggregate(t *testing.T, ctx context.Context, repo domainstatistic.StatisticRepository, monthStart time.Time, expected domainstatistic.StatisticFact) {
	t.Helper()

	assertSingleAggregate(t, ctx, repo, domainstatistic.GranularityMonth, monthStart, monthStart.AddDate(0, 1, 0), expected)
}

func assertSingleYearAggregate(t *testing.T, ctx context.Context, repo domainstatistic.StatisticRepository, yearStart time.Time, expected domainstatistic.StatisticFact) {
	t.Helper()

	assertSingleAggregate(t, ctx, repo, domainstatistic.GranularityYear, yearStart, yearStart.AddDate(1, 0, 0), expected)
}

func assertSingleAggregate(
	t *testing.T,
	ctx context.Context,
	repo domainstatistic.StatisticRepository,
	granularity domainstatistic.Granularity,
	periodStart time.Time,
	periodEnd time.Time,
	expected domainstatistic.StatisticFact,
) {
	t.Helper()

	aggID, err := domainstatistic.BuildStatisticID(granularity, periodStart)
	if err != nil {
		t.Fatalf("build statistic id: %v", err)
	}
	expectedKey, err := domainstatistic.NewTimeKey(domainstatistic.TimeType(granularity), periodStart)
	if err != nil {
		t.Fatalf("build time key: %v", err)
	}

	agg, err := repo.Get(ctx, aggID)
	if err != nil {
		t.Fatalf("get aggregate: %v", err)
	}
	if agg == nil {
		t.Fatalf("aggregate missing")
	}
	if agg.ID() != aggID {
		t.Fatalf("aggregate id mismatch: got=%s want=%s", agg.ID(), aggID)
	}
	if agg.Granularity() != granularity {
		t.Fatalf("aggregate granularity mismatch: %s", agg.Granularity())
	}
	if !agg.IsCompleted() {
		t.Fatalf("aggregate not completed")
	}
	if agg.PeriodStart() != periodStart {
		t.Fatalf("aggregate period mismatch: got=%s want=%s", agg.PeriodStart(), periodStart)
	}
	timeKey, err := agg.TimeKey()
	if err != nil {
		t.Fatalf("aggregate time key error: %v", err)
	}
	if timeKey != expectedKey {
		t.Fatalf("aggregate time key mismatch: got=%s want=%s", timeKey, expectedKey)
	}

	fact, ok := agg.Fact()
	if !ok {
		t.Fatalf("aggregate missing fact")
	}
	assertFactClose(t, fact, expected)

	aggs, err := repo.ListByGranularityAndPeriod(ctx, granularity, periodStart, periodEnd)
	if err != nil {
		t.Fatalf("list aggregates: %v", err)
	}
	if len(aggs) != 1 {
		t.Fatalf("expected 1 aggregate, got %d", len(aggs))
	}
	if aggs[0].ID() != aggID {
		t.Fatalf("aggregate list id mismatch: got=%s want=%s", aggs[0].ID(), aggID)
	}
}

func addFacts(a, b domainstatistic.StatisticFact) domainstatistic.StatisticFact {
	return domainstatistic.StatisticFact{
		ChargeKWh:       a.ChargeKWh + b.ChargeKWh,
		DischargeKWh:    a.DischargeKWh + b.DischargeKWh,
		Earnings:        a.Earnings + b.Earnings,
		CarbonReduction: a.CarbonReduction + b.CarbonReduction,
	}
}

func assertFactClose(t *testing.T, got, want domainstatistic.StatisticFact) {
	t.Helper()

	const eps = 1e-9
	if !floatClose(got.ChargeKWh, want.ChargeKWh, eps) ||
		!floatClose(got.DischargeKWh, want.DischargeKWh, eps) ||
		!floatClose(got.Earnings, want.Earnings, eps) ||
		!floatClose(got.CarbonReduction, want.CarbonReduction, eps) {
		t.Fatalf("fact mismatch: got=%+v want=%+v", got, want)
	}
}

func floatClose(a, b, eps float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= eps
}

func TestTelemetryWindowClosed_HourlyStatistic_PostgresTelemetry(t *testing.T) {
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
		t.Skip("required tables missing; run migrations")
	}

	ctx := context.Background()
	tenantID := "tenant-it-telemetry"
	stationID := "station-it-telemetry"
	deviceID := "device-it-telemetry"
	hourStart := time.Date(2026, time.January, 21, 10, 0, 0, 0, time.UTC)

	_, _ = db.ExecContext(ctx, "DELETE FROM telemetry_points WHERE tenant_id = $1 AND station_id = $2", tenantID, stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM analytics_statistics WHERE subject_id = $1", stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM point_mappings WHERE station_id = $1", stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM stations WHERE id = $1", stationID)

	if err := seedStationAndMappings(ctx, db, tenantID, stationID); err != nil {
		t.Fatalf("seed masterdata: %v", err)
	}

	telemetryRepo := telemetrypostgres.NewTelemetryRepository(db)
	telemetryQuery := telemetrypostgres.NewTelemetryQuery(db)

	charge := 5.0
	discharge := 2.0
	earnings := 1.5
	carbon := 0.3

	measurements := []telemetry.Measurement{
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "charge_power_kw",
			TS:           hourStart.Add(10 * time.Minute),
			ValueNumeric: &charge,
			Quality:      "good",
		},
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "discharge_power_kw",
			TS:           hourStart.Add(10 * time.Minute),
			ValueNumeric: &discharge,
			Quality:      "good",
		},
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "earnings",
			TS:           hourStart.Add(10 * time.Minute),
			ValueNumeric: &earnings,
			Quality:      "good",
		},
		{
			TenantID:     tenantID,
			StationID:    stationID,
			DeviceID:     deviceID,
			PointKey:     "carbon_reduction",
			TS:           hourStart.Add(10 * time.Minute),
			ValueNumeric: &carbon,
			Quality:      "good",
		},
	}

	if err := telemetryRepo.InsertMeasurements(ctx, measurements); err != nil {
		t.Fatalf("insert telemetry: %v", err)
	}

	pointMappingRepo := masterdatarepo.NewPointMappingRepository(db)
	queryAdapter, err := telemetryadapters.NewQueryAdapter(tenantID, telemetryQuery, pointMappingRepo)
	if err != nil {
		t.Fatalf("new query adapter: %v", err)
	}

	statsRepo := analyticsrepo.NewPostgresStatisticRepository(db, stationID)
	bus := eventbus.NewInMemoryBus()
	clock := fixedClock{now: hourStart.Add(2 * time.Hour)}

	hourlyApp := application.NewHourlyStatisticAppService(
		statsRepo,
		queryAdapter,
		telemetryadapters.SumStatisticCalculator{},
		bus,
		hourStatisticIDFactory{},
		clock,
	)

	application.WireAnalyticsEventBus(bus, hourlyApp, nil, nil)

	if err := bus.Publish(ctx, events.TelemetryWindowClosed{
		StationID:   stationID,
		WindowStart: hourStart,
		WindowEnd:   hourStart.Add(time.Hour),
		OccurredAt:  hourStart.Add(30 * time.Minute),
	}); err != nil {
		t.Fatalf("publish telemetry window closed: %v", err)
	}

	agg, err := statsRepo.FindByStationHour(ctx, stationID, hourStart)
	if err != nil {
		t.Fatalf("find hour statistic: %v", err)
	}
	if agg == nil {
		t.Fatalf("hour statistic not found")
	}

	fact, ok := agg.Fact()
	if !ok {
		t.Fatalf("statistic fact missing")
	}

	want := domainstatistic.StatisticFact{
		ChargeKWh:       charge,
		DischargeKWh:    discharge,
		Earnings:        earnings,
		CarbonReduction: carbon,
	}
	assertFactClose(t, fact, want)
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
