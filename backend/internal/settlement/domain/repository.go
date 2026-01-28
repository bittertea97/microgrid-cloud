package settlement

import (
	"context"
	"time"
)

// Repository persists settlement aggregates.
type Repository interface {
	FindBySubjectAndDay(ctx context.Context, subjectID string, dayStart time.Time) (*SettlementAggregate, error)
	Save(ctx context.Context, aggregate *SettlementAggregate) error
}
