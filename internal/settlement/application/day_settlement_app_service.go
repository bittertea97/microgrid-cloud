package application

import (
	"context"
	"errors"
	"time"

	"microgrid-cloud/internal/settlement/domain"
)

// DayEnergyCalculated represents the day settlement trigger from analytics.
type DayEnergyCalculated struct {
	SubjectID   string
	DayStart    time.Time
	Recalculate bool
	OccurredAt  time.Time
}

// SettlementCalculated is emitted when a day settlement is first created.
type SettlementCalculated struct {
	SubjectID  string
	DayStart   time.Time
	Amount     float64
	OccurredAt time.Time
}

// HourEnergy represents a single hour energy bucket.
type HourEnergy struct {
	HourStart time.Time
	EnergyKWh float64
}

// DayHourEnergyReader loads hourly energy values for a day.
type DayHourEnergyReader interface {
	ListDayHourEnergy(ctx context.Context, subjectID string, dayStart time.Time) ([]HourEnergy, error)
}

// TariffProvider provides the price per kWh at a given timestamp.
type TariffProvider interface {
	PriceAt(ctx context.Context, subjectID string, at time.Time) (float64, error)
}

// SettlementPublisher emits settlement calculated events.
type SettlementPublisher interface {
	PublishSettlementCalculated(ctx context.Context, event SettlementCalculated) error
}

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// SystemClock uses time.Now.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

// DaySettlementApplicationService handles day settlement use cases.
type DaySettlementApplicationService struct {
	repo      settlement.Repository
	energy    DayHourEnergyReader
	pricing   TariffProvider
	publisher SettlementPublisher
	clock     Clock
}

// NewDaySettlementApplicationService constructs the service.
func NewDaySettlementApplicationService(
	repo settlement.Repository,
	energy DayHourEnergyReader,
	pricing TariffProvider,
	publisher SettlementPublisher,
	clock Clock,
) (*DaySettlementApplicationService, error) {
	if repo == nil {
		return nil, errors.New("day settlement app service: nil repository")
	}
	if energy == nil {
		return nil, errors.New("day settlement app service: nil day energy reader")
	}
	if pricing == nil {
		return nil, errors.New("day settlement app service: nil tariff provider")
	}
	if clock == nil {
		clock = SystemClock{}
	}

	return &DaySettlementApplicationService{
		repo:      repo,
		energy:    energy,
		pricing:   pricing,
		publisher: publisher,
		clock:     clock,
	}, nil
}

// HandleDayEnergyCalculated recalculates day settlement amounts.
func (s *DaySettlementApplicationService) HandleDayEnergyCalculated(ctx context.Context, event DayEnergyCalculated) error {
	if event.SubjectID == "" {
		return settlement.ErrEmptySubjectID
	}
	if event.DayStart.IsZero() {
		return settlement.ErrInvalidDayStart
	}

	hourly, err := s.energy.ListDayHourEnergy(ctx, event.SubjectID, event.DayStart)
	if err != nil {
		return err
	}

	var energyKWh float64
	var amount float64
	for _, hour := range hourly {
		price, err := s.pricing.PriceAt(ctx, event.SubjectID, hour.HourStart)
		if err != nil {
			return err
		}
		energyKWh += hour.EnergyKWh
		amount += hour.EnergyKWh * price
	}

	agg, err := s.repo.FindBySubjectAndDay(ctx, event.SubjectID, event.DayStart)
	if err != nil {
		return err
	}
	if agg == nil {
		agg, err = settlement.NewDaySettlementAggregate(event.SubjectID, event.DayStart)
		if err != nil {
			return err
		}
	}
	wasNew := agg.IsNew()

	if err := agg.Recalculate(energyKWh, amount); err != nil {
		return err
	}

	if err := s.repo.Save(ctx, agg); err != nil {
		return err
	}

	if !wasNew || s.publisher == nil {
		return nil
	}

	occurredAt := event.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = s.clock.Now()
	}

	return s.publisher.PublishSettlementCalculated(ctx, SettlementCalculated{
		SubjectID:  event.SubjectID,
		DayStart:   event.DayStart,
		Amount:     amount,
		OccurredAt: occurredAt,
	})
}
