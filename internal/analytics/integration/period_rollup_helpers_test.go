package integration_test

import (
	"context"
	"errors"
	"time"

	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/analytics/application/events"
	domainstatistic "microgrid-cloud/internal/analytics/domain/statistic"
)

var (
	errRollupAlreadyCompleted = errors.New("rollup: already completed")
	errIncompleteChildren     = errors.New("rollup: incomplete child aggregates")
	errChildNotCompleted      = errors.New("rollup: child aggregates not completed")
)

type dayToMonthRollupHandler struct {
	repo         domainstatistic.StatisticRepository
	clock        domainstatistic.Clock
	bus          eventbus.EventBus
	expectedDays int
}

func newDayToMonthRollupHandler(repo domainstatistic.StatisticRepository, clock domainstatistic.Clock, bus eventbus.EventBus, expectedDays int) *dayToMonthRollupHandler {
	if clock == nil {
		clock = domainstatistic.SystemClock{}
	}
	if expectedDays <= 0 {
		expectedDays = 1
	}
	return &dayToMonthRollupHandler{
		repo:         repo,
		clock:        clock,
		bus:          bus,
		expectedDays: expectedDays,
	}
}

func (h *dayToMonthRollupHandler) HandleStatisticCalculated(ctx context.Context, event any) error {
	evt, ok := event.(events.StatisticCalculated)
	if !ok {
		return nil
	}
	if evt.Granularity != domainstatistic.GranularityDay {
		return nil
	}

	monthStart := time.Date(evt.PeriodStart.Year(), evt.PeriodStart.Month(), 1, 0, 0, 0, 0, evt.PeriodStart.Location())
	agg, err := rollupDaysToMonth(ctx, h.repo, monthStart, h.expectedDays, h.clock, evt.Recalculate)
	if err != nil {
		if errors.Is(err, errRollupAlreadyCompleted) ||
			errors.Is(err, errIncompleteChildren) ||
			errors.Is(err, errChildNotCompleted) {
			return nil
		}
		return err
	}
	if agg == nil {
		return nil
	}
	if err := h.repo.Save(ctx, agg); err != nil {
		return err
	}
	if h.bus == nil {
		return nil
	}

	return h.bus.Publish(ctx, events.StatisticCalculated{
		StationID:   evt.StationID,
		StatisticID: agg.ID(),
		Granularity: domainstatistic.GranularityMonth,
		PeriodStart: agg.PeriodStart(),
		OccurredAt:  evt.OccurredAt,
		Recalculate: evt.Recalculate,
	})
}

type monthToYearRollupHandler struct {
	repo           domainstatistic.StatisticRepository
	clock          domainstatistic.Clock
	bus            eventbus.EventBus
	expectedMonths int
}

func newMonthToYearRollupHandler(repo domainstatistic.StatisticRepository, clock domainstatistic.Clock, bus eventbus.EventBus, expectedMonths int) *monthToYearRollupHandler {
	if clock == nil {
		clock = domainstatistic.SystemClock{}
	}
	if expectedMonths <= 0 {
		expectedMonths = 1
	}
	return &monthToYearRollupHandler{
		repo:           repo,
		clock:          clock,
		bus:            bus,
		expectedMonths: expectedMonths,
	}
}

func (h *monthToYearRollupHandler) HandleStatisticCalculated(ctx context.Context, event any) error {
	evt, ok := event.(events.StatisticCalculated)
	if !ok {
		return nil
	}
	if evt.Granularity != domainstatistic.GranularityMonth {
		return nil
	}

	yearStart := time.Date(evt.PeriodStart.Year(), time.January, 1, 0, 0, 0, 0, evt.PeriodStart.Location())
	agg, err := rollupMonthsToYear(ctx, h.repo, yearStart, h.expectedMonths, h.clock, evt.Recalculate)
	if err != nil {
		if errors.Is(err, errRollupAlreadyCompleted) ||
			errors.Is(err, errIncompleteChildren) ||
			errors.Is(err, errChildNotCompleted) {
			return nil
		}
		return err
	}
	if agg == nil {
		return nil
	}
	if err := h.repo.Save(ctx, agg); err != nil {
		return err
	}
	if h.bus == nil {
		return nil
	}

	return h.bus.Publish(ctx, events.StatisticCalculated{
		StationID:   evt.StationID,
		StatisticID: agg.ID(),
		Granularity: domainstatistic.GranularityYear,
		PeriodStart: agg.PeriodStart(),
		OccurredAt:  evt.OccurredAt,
		Recalculate: evt.Recalculate,
	})
}

func rollupDaysToMonth(
	ctx context.Context,
	repo domainstatistic.StatisticRepository,
	monthStart time.Time,
	expectedDays int,
	clock domainstatistic.Clock,
	force bool,
) (*domainstatistic.StatisticAggregate, error) {
	if monthStart.IsZero() {
		return nil, domainstatistic.ErrInvalidPeriodStart
	}
	if expectedDays <= 0 {
		expectedDays = 1
	}
	if clock == nil {
		clock = domainstatistic.SystemClock{}
	}

	monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, monthStart.Location())
	monthID, err := domainstatistic.BuildStatisticID(domainstatistic.GranularityMonth, monthStart)
	if err != nil {
		return nil, err
	}

	current, err := repo.Get(ctx, monthID)
	if err != nil && !errors.Is(err, domainstatistic.ErrStatisticNotFound) {
		return nil, err
	}
	if current != nil && current.IsCompleted() && !force {
		return nil, errRollupAlreadyCompleted
	}

	monthEnd := monthStart.AddDate(0, 0, expectedDays)
	days, err := repo.ListByGranularityAndPeriod(ctx, domainstatistic.GranularityDay, monthStart, monthEnd)
	if err != nil {
		return nil, err
	}

	factByDay := make(map[time.Time]domainstatistic.StatisticFact, expectedDays)
	for _, dayAgg := range days {
		if dayAgg == nil {
			continue
		}
		if dayAgg.Granularity() != domainstatistic.GranularityDay {
			continue
		}
		period := dayAgg.PeriodStart()
		if period.Before(monthStart) || !period.Before(monthEnd) {
			continue
		}
		fact, ok := dayAgg.Fact()
		if !ok {
			return nil, errChildNotCompleted
		}
		if err := fact.Validate(); err != nil {
			return nil, err
		}
		factByDay[period] = fact
	}

	if len(factByDay) < expectedDays {
		return nil, errIncompleteChildren
	}

	var sum domainstatistic.StatisticFact
	for i := 0; i < expectedDays; i++ {
		period := monthStart.AddDate(0, 0, i)
		fact, ok := factByDay[period]
		if !ok {
			return nil, errIncompleteChildren
		}
		sum.ChargeKWh += fact.ChargeKWh
		sum.DischargeKWh += fact.DischargeKWh
		sum.Earnings += fact.Earnings
		sum.CarbonReduction += fact.CarbonReduction
	}

	monthAgg, err := domainstatistic.NewStatisticAggregate(monthID, domainstatistic.GranularityMonth, monthStart)
	if err != nil {
		return nil, err
	}
	if err := monthAgg.Complete(sum, clock.Now()); err != nil {
		return nil, err
	}

	return monthAgg, nil
}

func rollupMonthsToYear(
	ctx context.Context,
	repo domainstatistic.StatisticRepository,
	yearStart time.Time,
	expectedMonths int,
	clock domainstatistic.Clock,
	force bool,
) (*domainstatistic.StatisticAggregate, error) {
	if yearStart.IsZero() {
		return nil, domainstatistic.ErrInvalidPeriodStart
	}
	if expectedMonths <= 0 {
		expectedMonths = 1
	}
	if clock == nil {
		clock = domainstatistic.SystemClock{}
	}

	yearStart = time.Date(yearStart.Year(), time.January, 1, 0, 0, 0, 0, yearStart.Location())
	yearID, err := domainstatistic.BuildStatisticID(domainstatistic.GranularityYear, yearStart)
	if err != nil {
		return nil, err
	}

	current, err := repo.Get(ctx, yearID)
	if err != nil && !errors.Is(err, domainstatistic.ErrStatisticNotFound) {
		return nil, err
	}
	if current != nil && current.IsCompleted() && !force {
		return nil, errRollupAlreadyCompleted
	}

	yearEnd := yearStart.AddDate(0, expectedMonths, 0)
	months, err := repo.ListByGranularityAndPeriod(ctx, domainstatistic.GranularityMonth, yearStart, yearEnd)
	if err != nil {
		return nil, err
	}

	factByMonth := make(map[time.Time]domainstatistic.StatisticFact, expectedMonths)
	for _, monthAgg := range months {
		if monthAgg == nil {
			continue
		}
		if monthAgg.Granularity() != domainstatistic.GranularityMonth {
			continue
		}
		period := monthAgg.PeriodStart()
		if period.Before(yearStart) || !period.Before(yearEnd) {
			continue
		}
		fact, ok := monthAgg.Fact()
		if !ok {
			return nil, errChildNotCompleted
		}
		if err := fact.Validate(); err != nil {
			return nil, err
		}
		factByMonth[period] = fact
	}

	if len(factByMonth) < expectedMonths {
		return nil, errIncompleteChildren
	}

	var sum domainstatistic.StatisticFact
	for i := 0; i < expectedMonths; i++ {
		period := yearStart.AddDate(0, i, 0)
		fact, ok := factByMonth[period]
		if !ok {
			return nil, errIncompleteChildren
		}
		sum.ChargeKWh += fact.ChargeKWh
		sum.DischargeKWh += fact.DischargeKWh
		sum.Earnings += fact.Earnings
		sum.CarbonReduction += fact.CarbonReduction
	}

	yearAgg, err := domainstatistic.NewStatisticAggregate(yearID, domainstatistic.GranularityYear, yearStart)
	if err != nil {
		return nil, err
	}
	if err := yearAgg.Complete(sum, clock.Now()); err != nil {
		return nil, err
	}

	return yearAgg, nil
}
