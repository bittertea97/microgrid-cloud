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

func TestStatisticCalculated_DayToMonthRollup_ClosedLoop(t *testing.T) {
	ctx := context.Background()

	stationID := "station-integration-month-001"
	monthStart := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	clock := fixedClock{now: monthStart.Add(72 * time.Hour)}

	expectedDays := 2

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
	bus.Subscribe(eventbus.EventTypeOf[events.StatisticCalculated](), newDayToMonthRollupHandler(repo, clock, bus, expectedDays).HandleStatisticCalculated)
	bus.Subscribe(eventbus.EventTypeOf[events.StatisticCalculated](), recorder.HandleStatisticCalculated)

	expectedMonth := domainstatistic.StatisticFact{}
	dayFacts := make(map[time.Time]domainstatistic.StatisticFact, expectedDays)

	for i := 0; i < expectedDays; i++ {
		dayStart := monthStart.AddDate(0, 0, i)
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
		dayFacts[dayStart] = fact
		expectedMonth = addFacts(expectedMonth, fact)
	}

	for i := 0; i < expectedDays; i++ {
		dayStart := monthStart.AddDate(0, 0, i)
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

	monthAgg := waitForAggregate(t, ctx, repo, domainstatistic.GranularityMonth, monthStart, 2*time.Second)
	if monthAgg == nil {
		t.Fatalf("month aggregate missing")
	}
	assertSingleMonthAggregate(t, ctx, repo, monthStart, expectedMonth)

	_, dayCount, monthCount, _ := recorder.Counts()
	if dayCount != expectedDays {
		t.Fatalf("expected %d day statistic events, got %d", expectedDays, dayCount)
	}
	if monthCount != 1 {
		t.Fatalf("expected 1 month statistic event, got %d", monthCount)
	}

	// Backfill one day.
	backfillDay := monthStart.AddDate(0, 0, 1)
	oldFact := dayFacts[backfillDay]

	newPoint := application.TelemetryPoint{
		At:               backfillDay.Add(15 * time.Minute),
		ChargePowerKW:    100,
		DischargePowerKW: 50,
		Earnings:         25,
		CarbonReduction:  12,
	}
	telemetry.SetHour(backfillDay, []application.TelemetryPoint{newPoint})
	newFact := domainstatistic.StatisticFact{
		ChargeKWh:       newPoint.ChargePowerKW,
		DischargeKWh:    newPoint.DischargePowerKW,
		Earnings:        newPoint.Earnings,
		CarbonReduction: newPoint.CarbonReduction,
	}

	expectedMonthAfter := expectedMonth
	expectedMonthAfter.ChargeKWh += newFact.ChargeKWh - oldFact.ChargeKWh
	expectedMonthAfter.DischargeKWh += newFact.DischargeKWh - oldFact.DischargeKWh
	expectedMonthAfter.Earnings += newFact.Earnings - oldFact.Earnings
	expectedMonthAfter.CarbonReduction += newFact.CarbonReduction - oldFact.CarbonReduction

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

	monthAggAfter := waitForAggregate(t, ctx, repo, domainstatistic.GranularityMonth, monthStart, 2*time.Second)
	if monthAggAfter == nil {
		t.Fatalf("month aggregate missing after backfill")
	}
	assertSingleMonthAggregate(t, ctx, repo, monthStart, expectedMonthAfter)

	_, dayCount, monthCount, _ = recorder.Counts()
	if dayCount != 1 {
		t.Fatalf("expected 1 day statistic event in backfill, got %d", dayCount)
	}
	if monthCount != 1 {
		t.Fatalf("expected 1 month statistic event in backfill, got %d", monthCount)
	}
}
