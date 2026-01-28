package statistic

import (
	"context"
	"errors"
	"time"

	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/analytics/application/events"
	domainstatistic "microgrid-cloud/internal/analytics/domain/statistic"
)

// DailyRollupAppService handles day rollup application use cases.
type DailyRollupAppService struct {
	rollup    *domainstatistic.DailyRollupService
	repo      domainstatistic.StatisticRepository
	bus       eventbus.EventBus
	clock     domainstatistic.Clock
}

// NewDailyRollupAppService constructs the application service.
func NewDailyRollupAppService(
	rollup *domainstatistic.DailyRollupService,
	repo domainstatistic.StatisticRepository,
	bus eventbus.EventBus,
	clock domainstatistic.Clock,
) (*DailyRollupAppService, error) {
	if rollup == nil {
		return nil, errors.New("daily rollup app service: nil rollup service")
	}
	if repo == nil {
		return nil, errors.New("daily rollup app service: nil repository")
	}
	if clock == nil {
		clock = domainstatistic.SystemClock{}
	}

	return &DailyRollupAppService{
		rollup:    rollup,
		repo:      repo,
		bus:       bus,
		clock:     clock,
	}, nil
}

// HandleStatisticCalculated reacts to HOUR statistics and performs day rollups.
func (s *DailyRollupAppService) HandleStatisticCalculated(ctx context.Context, event events.StatisticCalculated) error {
	if event.Granularity != domainstatistic.GranularityHour {
		return nil
	}

	period := event.PeriodStart
	if period.IsZero() {
		return domainstatistic.ErrInvalidPeriodStart
	}
	dayStart := time.Date(period.Year(), period.Month(), period.Day(), 0, 0, 0, 0, period.Location())

	dayAggregate, err := s.rollup.RollupDay(ctx, dayStart, event.Recalculate)
	if err != nil {
		if errors.Is(err, domainstatistic.ErrDayAlreadyCompleted) ||
			errors.Is(err, domainstatistic.ErrIncompleteHourStatistics) ||
			errors.Is(err, domainstatistic.ErrHourStatisticsNotCompleted) {
			return nil
		}
		return err
	}
	if dayAggregate == nil {
		return nil
	}

	if err := s.repo.Save(ctx, dayAggregate); err != nil {
		return err
	}

	occurredAt := event.OccurredAt
	if occurredAt.IsZero() {
		if completedAt, ok := dayAggregate.CompletedAt(); ok {
			occurredAt = completedAt
		} else {
			occurredAt = s.clock.Now()
		}
	}

	if s.bus == nil {
		return nil
	}

	return s.bus.Publish(ctx, events.StatisticCalculated{
		StationID:   event.StationID,
		StatisticID: dayAggregate.ID(),
		Granularity: domainstatistic.GranularityDay,
		PeriodStart: dayAggregate.PeriodStart(),
		OccurredAt:  occurredAt,
		Recalculate: event.Recalculate,
	})
}

