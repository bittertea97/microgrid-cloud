package integration_test

import (
	"context"
	"testing"
	"time"

	"microgrid-cloud/internal/analytics/application"
	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/analytics/application/events"
	appstatistic "microgrid-cloud/internal/analytics/application/statistic"
	domainstatistic "microgrid-cloud/internal/analytics/domain/statistic"
)

func TestStatisticCalculated_MonthToYearRollup_ClosedLoop(t *testing.T) {
	ctx := context.Background()

	stationID := "station-integration-year-001"
	yearStart := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	clock := fixedClock{now: yearStart.Add(72 * time.Hour)}

	expectedMonths := 2
	expectedDaysPerMonth := 1

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

	rollupService, err := domainstatistic.NewDailyRollupService(repo, clock, 1)
	if err != nil {
		t.Fatalf("new daily rollup service: %v", err)
	}
	dailyApp, err := appstatistic.NewDailyRollupAppService(rollupService, repo, bus, clock)
	if err != nil {
		t.Fatalf("new daily rollup app service: %v", err)
	}

	application.WireAnalyticsEventBus(bus, hourlyApp, dailyApp, nil)
	bus.Subscribe(eventbus.EventTypeOf[events.StatisticCalculated](), newDayToMonthRollupHandler(repo, clock, bus, expectedDaysPerMonth).HandleStatisticCalculated)
	bus.Subscribe(eventbus.EventTypeOf[events.StatisticCalculated](), newMonthToYearRollupHandler(repo, clock, bus, expectedMonths).HandleStatisticCalculated)
	bus.Subscribe(eventbus.EventTypeOf[events.StatisticCalculated](), recorder.HandleStatisticCalculated)

	expectedYear := domainstatistic.StatisticFact{}
	monthFacts := make(map[time.Time]domainstatistic.StatisticFact, expectedMonths)

	for i := 0; i < expectedMonths; i++ {
		monthStart := yearStart.AddDate(0, i, 0)
		dayStart := monthStart
		point := application.TelemetryPoint{
			At:               dayStart.Add(10 * time.Minute),
			ChargePowerKW:    float64(i + 1),
			DischargePowerKW: float64(i+1) * 0.5,
			Earnings:         float64(i+1) * 0.1,
			CarbonReduction:  float64(i+1) * 0.01,
		}
		telemetry.SetHour(dayStart, []application.TelemetryPoint{point})

		fact := domainstatistic.StatisticFact{
			ChargeKWh:       point.ChargePowerKW,
			DischargeKWh:    point.DischargePowerKW,
			Earnings:        point.Earnings,
			CarbonReduction: point.CarbonReduction,
		}
		monthFacts[monthStart] = fact
		expectedYear = addFacts(expectedYear, fact)
	}

	for i := 0; i < expectedMonths; i++ {
		dayStart := yearStart.AddDate(0, i, 0)
		err := bus.Publish(ctx, events.TelemetryWindowClosed{
			StationID:   stationID,
			WindowStart: dayStart,
			WindowEnd:   dayStart.Add(time.Hour),
			OccurredAt:  dayStart.Add(30 * time.Minute),
		})
		if err != nil {
			t.Fatalf("publish telemetry window closed: %v", err)
		}
	}

	yearAgg := waitForAggregate(t, ctx, repo, domainstatistic.GranularityYear, yearStart, 2*time.Second)
	if yearAgg == nil {
		t.Fatalf("year aggregate missing")
	}
	assertSingleYearAggregate(t, ctx, repo, yearStart, expectedYear)

	_, _, monthCount, yearCount := recorder.Counts()
	if monthCount != expectedMonths {
		t.Fatalf("expected %d month statistic events, got %d", expectedMonths, monthCount)
	}
	if yearCount != 1 {
		t.Fatalf("expected 1 year statistic event, got %d", yearCount)
	}

	// Backfill one month.
	backfillMonth := yearStart.AddDate(0, 1, 0)
	backfillDay := backfillMonth
	oldFact := monthFacts[backfillMonth]

	newPoint := application.TelemetryPoint{
		At:               backfillDay.Add(15 * time.Minute),
		ChargePowerKW:    120,
		DischargePowerKW: 60,
		Earnings:         30,
		CarbonReduction:  14,
	}
	telemetry.SetHour(backfillDay, []application.TelemetryPoint{newPoint})
	newFact := domainstatistic.StatisticFact{
		ChargeKWh:       newPoint.ChargePowerKW,
		DischargeKWh:    newPoint.DischargePowerKW,
		Earnings:        newPoint.Earnings,
		CarbonReduction: newPoint.CarbonReduction,
	}

	expectedYearAfter := expectedYear
	expectedYearAfter.ChargeKWh += newFact.ChargeKWh - oldFact.ChargeKWh
	expectedYearAfter.DischargeKWh += newFact.DischargeKWh - oldFact.DischargeKWh
	expectedYearAfter.Earnings += newFact.Earnings - oldFact.Earnings
	expectedYearAfter.CarbonReduction += newFact.CarbonReduction - oldFact.CarbonReduction

	recorder.Reset()
	repo.ForceRecalculateHour(backfillDay)
	err = bus.Publish(ctx, events.TelemetryWindowClosed{
		StationID:   stationID,
		WindowStart: backfillDay,
		WindowEnd:   backfillDay.Add(time.Hour),
		OccurredAt:  backfillDay.Add(40 * time.Minute),
		Recalculate: true,
	})
	if err != nil {
		t.Fatalf("publish backfill telemetry window closed: %v", err)
	}

	yearAggAfter := waitForAggregate(t, ctx, repo, domainstatistic.GranularityYear, yearStart, 2*time.Second)
	if yearAggAfter == nil {
		t.Fatalf("year aggregate missing after backfill")
	}
	assertSingleYearAggregate(t, ctx, repo, yearStart, expectedYearAfter)

	_, _, monthCount, yearCount = recorder.Counts()
	if monthCount != 1 {
		t.Fatalf("expected 1 month statistic event in backfill, got %d", monthCount)
	}
	if yearCount != 1 {
		t.Fatalf("expected 1 year statistic event in backfill, got %d", yearCount)
	}
}
