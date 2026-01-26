package memory

import (
	"context"
	"errors"
	"sync"
	"time"

	"microgrid-cloud/internal/analytics/domain/statistic"
)

// StatisticRepository is an in-memory repository for demo/testing.
// It implements both hourly and rollup repository interfaces.
type StatisticRepository struct {
	mu   sync.RWMutex
	data map[string]*statistic.StatisticAggregate
}

// NewStatisticRepository constructs a repository.
func NewStatisticRepository() *StatisticRepository {
	return &StatisticRepository{
		data: make(map[string]*statistic.StatisticAggregate),
	}
}

// FindByStationHour checks if an hour aggregate exists.
func (r *StatisticRepository) FindByStationHour(ctx context.Context, stationID string, hourStart time.Time) (*statistic.StatisticAggregate, error) {
	_ = ctx
	if hourStart.IsZero() {
		return nil, statistic.ErrInvalidPeriodStart
	}

	id, err := statistic.BuildStatisticID(statistic.GranularityHour, hourStart)
	if err != nil {
		return nil, err
	}

	return r.Get(ctx, id)
}

// Get loads an aggregate by id.
func (r *StatisticRepository) Get(ctx context.Context, id statistic.StatisticID) (*statistic.StatisticAggregate, error) {
	_ = ctx
	if id == "" {
		return nil, statistic.ErrEmptyID
	}

	key := string(id)
	r.mu.RLock()
	defer r.mu.RUnlock()
	agg := r.data[key]
	if agg == nil {
		return nil, statistic.ErrStatisticNotFound
	}
	return agg, nil
}

// ListByGranularityAndPeriod returns aggregates in the given range.
func (r *StatisticRepository) ListByGranularityAndPeriod(ctx context.Context, granularity statistic.Granularity, startInclusive, endExclusive time.Time) ([]*statistic.StatisticAggregate, error) {
	_ = ctx
	if !granularity.IsValid() {
		return nil, statistic.ErrInvalidGranularity
	}
	if startInclusive.IsZero() || endExclusive.IsZero() {
		return nil, statistic.ErrInvalidPeriodStart
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*statistic.StatisticAggregate, 0, len(r.data))
	for _, agg := range r.data {
		if agg == nil {
			continue
		}
		if agg.Granularity() != granularity {
			continue
		}
		period := agg.PeriodStart()
		if period.Before(startInclusive) || !period.Before(endExclusive) {
			continue
		}
		result = append(result, agg)
	}
	return result, nil
}

// Save persists an aggregate.
func (r *StatisticRepository) Save(ctx context.Context, agg *statistic.StatisticAggregate) error {
	_ = ctx
	if agg == nil {
		return errors.New("memory statistic repo: nil aggregate")
	}
	if agg.ID() == "" {
		return statistic.ErrEmptyID
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[string(agg.ID())] = agg
	return nil
}

