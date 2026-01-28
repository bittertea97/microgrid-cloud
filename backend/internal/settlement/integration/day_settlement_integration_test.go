package integration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	appsettlement "microgrid-cloud/internal/settlement/application"
	"microgrid-cloud/internal/settlement/infrastructure/memory"
)

func TestDaySettlement_RecalculateOnEnergyBackfill(t *testing.T) {
	ctx := context.Background()

	subjectID := "subject-settlement-001"
	dayStart := time.Date(2026, time.January, 20, 0, 0, 0, 0, time.UTC)
	unitPrice := 1.0

	repo := memory.NewSettlementRepository()
	energyStore := newHourEnergyStore()
	pricing := fixedPrice{unit: unitPrice}
	publisher := newSettlementEventRecorder()
	clock := fixedClock{now: dayStart.Add(2 * time.Hour)}

	app := newDaySettlementAppService(t, repo, energyStore, pricing, publisher, clock)

	energyStore.SetDayEnergy(subjectID, dayStart, 100)
	err := app.HandleDayEnergyCalculated(ctx, appsettlement.DayEnergyCalculated{
		SubjectID:  subjectID,
		DayStart:   dayStart,
		OccurredAt: dayStart.Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("handle day settlement: %v", err)
	}

	energyStore.SetDayEnergy(subjectID, dayStart, 120)
	err = app.HandleDayEnergyCalculated(ctx, appsettlement.DayEnergyCalculated{
		SubjectID:   subjectID,
		DayStart:    dayStart,
		Recalculate: true,
		OccurredAt:  dayStart.Add(90 * time.Minute),
	})
	if err != nil {
		t.Fatalf("handle day settlement backfill: %v", err)
	}

	settlements, err := repo.ListBySubjectAndDay(ctx, subjectID, dayStart)
	if err != nil {
		t.Fatalf("list settlements: %v", err)
	}
	if len(settlements) != 1 {
		t.Fatalf("expected 1 settlement record, got %d", len(settlements))
	}

	got := settlements[0]
	expectedKey := dayTimeKey(dayStart)
	expectedAmount := 120 * unitPrice

	if got.TimeKey() != expectedKey {
		t.Fatalf("time key mismatch: got=%s want=%s", got.TimeKey(), expectedKey)
	}
	if got.Amount() != expectedAmount {
		t.Fatalf("amount mismatch: got=%v want=%v", got.Amount(), expectedAmount)
	}
	if got.Amount() == 220*unitPrice {
		t.Fatalf("amount should be overwritten, not accumulated")
	}

	if publisher.Count() != 1 {
		t.Fatalf("expected SettlementCalculated event once, got %d", publisher.Count())
	}
}

func newDaySettlementAppService(
	t *testing.T,
	repo *memory.SettlementRepository,
	energy appsettlement.DayHourEnergyReader,
	pricing appsettlement.TariffProvider,
	publisher appsettlement.SettlementPublisher,
	clock appsettlement.Clock,
) *appsettlement.DaySettlementApplicationService {
	t.Helper()

	app, err := appsettlement.NewDaySettlementApplicationService(repo, energy, pricing, publisher, clock)
	if err != nil {
		t.Fatalf("new day settlement app service: %v", err)
	}
	return app
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

type fixedPrice struct {
	unit float64
}

func (p fixedPrice) PriceAt(ctx context.Context, subjectID string, at time.Time) (float64, error) {
	_ = ctx
	_ = subjectID
	_ = at
	return p.unit, nil
}

type hourEnergyStore struct {
	mu   sync.RWMutex
	data map[string]float64
}

func newHourEnergyStore() *hourEnergyStore {
	return &hourEnergyStore{data: make(map[string]float64)}
}

func (s *hourEnergyStore) SetDayEnergy(subjectID string, dayStart time.Time, energy float64) {
	key := settlementKey(subjectID, dayStart)
	s.mu.Lock()
	s.data[key] = energy
	s.mu.Unlock()
}

func (s *hourEnergyStore) ListDayHourEnergy(ctx context.Context, subjectID string, dayStart time.Time) ([]appsettlement.HourEnergy, error) {
	_ = ctx
	key := settlementKey(subjectID, dayStart)

	s.mu.RLock()
	energy, ok := s.data[key]
	s.mu.RUnlock()
	if !ok {
		return nil, nil
	}
	return []appsettlement.HourEnergy{
		{HourStart: dayStart, EnergyKWh: energy},
	}, nil
}

type settlementEventRecorder struct {
	mu    sync.RWMutex
	count int
}

func newSettlementEventRecorder() *settlementEventRecorder {
	return &settlementEventRecorder{}
}

func (r *settlementEventRecorder) PublishSettlementCalculated(ctx context.Context, event appsettlement.SettlementCalculated) error {
	_ = ctx
	_ = event

	r.mu.Lock()
	r.count++
	r.mu.Unlock()
	return nil
}

func (r *settlementEventRecorder) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count
}

func dayTimeKey(dayStart time.Time) string {
	return dayStart.UTC().Format("20060102")
}

func settlementKey(subjectID string, dayStart time.Time) string {
	return subjectID + "|" + dayTimeKey(dayStart)
}
