package statistic

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Clock provides time for domain services.
type Clock interface {
	Now() time.Time
}

// SystemClock uses time.Now.
type SystemClock struct{}

// Now returns current time.
func (SystemClock) Now() time.Time { return time.Now() }

// StatisticRepository defines the read/write capabilities needed for rollups.
type StatisticRepository interface {
	Get(ctx context.Context, id StatisticID) (*StatisticAggregate, error)
	ListByGranularityAndPeriod(ctx context.Context, granularity Granularity, startInclusive, endExclusive time.Time) ([]*StatisticAggregate, error)
	Save(ctx context.Context, aggregate *StatisticAggregate) error
}

// DailyRollupService performs day rollups from hour statistics.
type DailyRollupService struct {
	repo          StatisticRepository
	clock         Clock
	expectedHours int
}

// NewDailyRollupService constructs a DailyRollupService.
func NewDailyRollupService(repo StatisticRepository, clock Clock, expectedHours int) (*DailyRollupService, error) {
	if repo == nil {
		return nil, errors.New("statistic: nil repository")
	}
	if clock == nil {
		clock = SystemClock{}
	}
	if expectedHours <= 0 {
		expectedHours = 24
	}

	return &DailyRollupService{
		repo:          repo,
		clock:         clock,
		expectedHours: expectedHours,
	}, nil
}

// RollupDay aggregates all hour statistics for the day.
// If force is true, a completed day aggregate will be recalculated and overwritten.
func (s *DailyRollupService) RollupDay(ctx context.Context, dayStart time.Time, force bool) (*StatisticAggregate, error) {
	if dayStart.IsZero() {
		return nil, ErrInvalidPeriodStart
	}
	dayStart = truncateToDay(dayStart)

	dayID, err := BuildStatisticID(GranularityDay, dayStart)
	if err != nil {
		return nil, err
	}

	current, err := s.repo.Get(ctx, dayID)
	if err != nil && !errors.Is(err, ErrStatisticNotFound) {
		return nil, err
	}
	if current != nil && current.IsCompleted() && !force {
		return nil, ErrDayAlreadyCompleted
	}

	dayEnd := dayStart.Add(time.Duration(s.expectedHours) * time.Hour)
	hours, err := s.repo.ListByGranularityAndPeriod(ctx, GranularityHour, dayStart, dayEnd)
	if err != nil {
		return nil, err
	}

	factByHour := make(map[time.Time]StatisticFact, s.expectedHours)
	for _, hourAgg := range hours {
		if hourAgg == nil {
			continue
		}
		if hourAgg.Granularity() != GranularityHour {
			continue
		}
		period := hourAgg.PeriodStart()
		if period.Before(dayStart) || !period.Before(dayEnd) {
			continue
		}
		fact, ok := hourAgg.Fact()
		if !ok {
			return nil, ErrHourStatisticsNotCompleted
		}
		if err := fact.Validate(); err != nil {
			return nil, err
		}
		factByHour[period] = fact
	}

	if len(factByHour) < s.expectedHours {
		return nil, ErrIncompleteHourStatistics
	}

	var sum StatisticFact
	for i := 0; i < s.expectedHours; i++ {
		period := dayStart.Add(time.Duration(i) * time.Hour)
		fact, ok := factByHour[period]
		if !ok {
			return nil, ErrIncompleteHourStatistics
		}
		sum.ChargeKWh += fact.ChargeKWh
		sum.DischargeKWh += fact.DischargeKWh
		sum.Earnings += fact.Earnings
		sum.CarbonReduction += fact.CarbonReduction
	}

	dayAgg, err := NewStatisticAggregate(dayID, GranularityDay, dayStart)
	if err != nil {
		return nil, err
	}
	if err := dayAgg.Complete(sum, s.clock.Now()); err != nil {
		return nil, err
	}

	return dayAgg, nil
}

// BuildStatisticID creates a deterministic id for a granularity + period start.
func BuildStatisticID(granularity Granularity, periodStart time.Time) (StatisticID, error) {
	if !granularity.IsValid() {
		return "", ErrInvalidGranularity
	}
	if periodStart.IsZero() {
		return "", ErrInvalidPeriodStart
	}

	layout, err := timeKeyLayout(TimeType(granularity))
	if err != nil {
		return "", err
	}

	return StatisticID(fmt.Sprintf("%s:%s", granularity, periodStart.Format(layout))), nil
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
