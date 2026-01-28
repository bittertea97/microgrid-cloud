package interfaces

import (
	"context"

	"microgrid-cloud/internal/eventing"
	"microgrid-cloud/internal/settlement/application"
)

// OutboxPublisher writes settlement calculated events to outbox.
type OutboxPublisher struct {
	publisher *eventing.Publisher
	tenantID  string
}

// NewOutboxPublisher constructs an outbox publisher.
func NewOutboxPublisher(publisher *eventing.Publisher, tenantID string) *OutboxPublisher {
	return &OutboxPublisher{publisher: publisher, tenantID: tenantID}
}

// PublishSettlementCalculated writes event to outbox.
func (p *OutboxPublisher) PublishSettlementCalculated(ctx context.Context, event application.SettlementCalculated) error {
	if p == nil || p.publisher == nil {
		return nil
	}
	ctx = eventing.WithTenantID(ctx, p.tenantID)
	return p.publisher.Publish(ctx, event)
}
