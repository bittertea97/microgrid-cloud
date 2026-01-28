package memory

import (
	"context"
	"sync"
	"time"

	"microgrid-cloud/internal/settlement/domain"
)

// SettlementRepository is an in-memory repository for settlements.
type SettlementRepository struct {
	mu   sync.RWMutex
	data map[string]*settlement.SettlementAggregate
}

// NewSettlementRepository constructs a repository.
func NewSettlementRepository() *SettlementRepository {
	return &SettlementRepository{data: make(map[string]*settlement.SettlementAggregate)}
}

// FindBySubjectAndDay loads a day aggregate.
func (r *SettlementRepository) FindBySubjectAndDay(ctx context.Context, subjectID string, dayStart time.Time) (*settlement.SettlementAggregate, error) {
	_ = ctx
	id, err := settlement.BuildSettlementID(subjectID, dayStart)
	if err != nil {
		return nil, err
	}

	r.mu.RLock()
	agg := r.data[string(id)]
	r.mu.RUnlock()
	if agg == nil {
		return nil, nil
	}
	return agg.Clone(), nil
}

// Save persists an aggregate (overwrites existing).
func (r *SettlementRepository) Save(ctx context.Context, aggregate *settlement.SettlementAggregate) error {
	_ = ctx
	if aggregate == nil {
		return settlement.ErrNilAggregate
	}
	id := aggregate.ID()
	if id == "" {
		return settlement.ErrEmptySubjectID
	}

	copy := aggregate.Clone()
	r.mu.Lock()
	r.data[string(id)] = copy
	r.mu.Unlock()

	aggregate.MarkPersisted()
	return nil
}

// ListBySubjectAndDay returns the single record for assertion convenience.
func (r *SettlementRepository) ListBySubjectAndDay(ctx context.Context, subjectID string, dayStart time.Time) ([]*settlement.SettlementAggregate, error) {
	_ = ctx
	id, err := settlement.BuildSettlementID(subjectID, dayStart)
	if err != nil {
		return nil, err
	}

	r.mu.RLock()
	agg := r.data[string(id)]
	r.mu.RUnlock()
	if agg == nil {
		return nil, nil
	}
	return []*settlement.SettlementAggregate{agg.Clone()}, nil
}
