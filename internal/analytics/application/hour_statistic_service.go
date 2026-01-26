package application

import (
	"context"
	"errors"
	"time"

	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/analytics/application/events"
	"microgrid-cloud/internal/analytics/domain/statistic"
)

// HourlyStatisticAppService handles hourly statistic use case.
type HourlyStatisticAppService interface {
	HandleTelemetryWindowClosed(ctx context.Context, evt events.TelemetryWindowClosed) error
}

// HourlyStatisticCalculator is a domain service that encapsulates all calculation rules.
type HourlyStatisticCalculator interface {
	CalculateHour(ctx context.Context, stationID string, periodStart time.Time, telemetry []TelemetryPoint) (statistic.StatisticFact, error)
}

// HourlyStatisticRepository persists hourly statistic aggregates.
type HourlyStatisticRepository interface {
	FindByStationHour(ctx context.Context, stationID string, hourStart time.Time) (*statistic.StatisticAggregate, error)
	Save(ctx context.Context, agg *statistic.StatisticAggregate) error
}

// TelemetryQuery fetches telemetry data for a time window.
type TelemetryQuery interface {
	QueryHour(ctx context.Context, stationID string, hourStart, hourEnd time.Time) ([]TelemetryPoint, error)
}

// StatisticIDFactory builds deterministic statistic IDs.
type StatisticIDFactory interface {
	HourID(stationID string, hourStart time.Time) (statistic.StatisticID, error)
}

// Clock provides time.
type Clock interface {
	Now() time.Time
}

// TelemetryPoint is a minimal telemetry value object used by the calculator.
type TelemetryPoint struct {
	At               time.Time
	ChargePowerKW    float64
	DischargePowerKW float64
	Earnings         float64
	CarbonReduction  float64
}

// ErrDuplicateStatistic is returned when a statistic already exists (idempotency).
var ErrDuplicateStatistic = errors.New("analytics: statistic already exists")

// HourlyStatisticAppServiceImpl is the default application service implementation.
type HourlyStatisticAppServiceImpl struct {
	repo       HourlyStatisticRepository
	telemetry  TelemetryQuery
	calculator HourlyStatisticCalculator
	bus        eventbus.EventBus
	idFactory  StatisticIDFactory
	clock      Clock
}

// NewHourlyStatisticAppService builds a HourlyStatisticAppServiceImpl.
func NewHourlyStatisticAppService(
	repo HourlyStatisticRepository,
	telemetry TelemetryQuery,
	calculator HourlyStatisticCalculator,
	bus eventbus.EventBus,
	idFactory StatisticIDFactory,
	clock Clock,
) *HourlyStatisticAppServiceImpl {
	return &HourlyStatisticAppServiceImpl{
		repo:       repo,
		telemetry:  telemetry,
		calculator: calculator,
		bus:        bus,
		idFactory:  idFactory,
		clock:      clock,
	}
}

// HandleTelemetryWindowClosed orchestrates the hourly statistic calculation.
func (s *HourlyStatisticAppServiceImpl) HandleTelemetryWindowClosed(ctx context.Context, evt events.TelemetryWindowClosed) error {
	if evt.StationID == "" || evt.WindowStart.IsZero() || evt.WindowEnd.IsZero() {
		return errors.New("analytics: invalid telemetry window")
	}

	existing, err := s.repo.FindByStationHour(ctx, evt.StationID, evt.WindowStart)
	if err != nil {
		return err
	}
	if existing != nil && !evt.Recalculate {
		return nil
	}

	telemetry, err := s.telemetry.QueryHour(ctx, evt.StationID, evt.WindowStart, evt.WindowEnd)
	if err != nil {
		return err
	}

	fact, err := s.calculator.CalculateHour(ctx, evt.StationID, evt.WindowStart, telemetry)
	if err != nil {
		return err
	}

	statID, err := s.idFactory.HourID(evt.StationID, evt.WindowStart)
	if err != nil {
		return err
	}

	agg, err := statistic.NewStatisticAggregate(statID, statistic.GranularityHour, evt.WindowStart)
	if err != nil {
		return err
	}
	completedAt := s.clock.Now()
	if err := agg.Complete(fact, completedAt); err != nil {
		return err
	}

	if err := s.repo.Save(ctx, agg); err != nil {
		if errors.Is(err, ErrDuplicateStatistic) {
			return nil
		}
		return err
	}

	if s.bus == nil {
		return nil
	}

	return s.bus.Publish(ctx, events.StatisticCalculated{
		StationID:   evt.StationID,
		StatisticID: statID,
		Granularity: statistic.GranularityHour,
		PeriodStart: evt.WindowStart,
		OccurredAt:  completedAt,
		Recalculate: evt.Recalculate,
	})
}

