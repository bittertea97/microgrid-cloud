package eventing

import (
	"context"

	"microgrid-cloud/internal/analytics/application/eventbus"
)

// Publisher writes events to outbox and triggers dispatch.
type Publisher struct {
	outbox   OutboxWriter
	dispatch *Dispatcher
	tenantID string
	sub      Subscriber
}

// OutboxWriter inserts outbox records.
type OutboxWriter interface {
	Insert(ctx context.Context, env Envelope) (string, error)
}

// Subscriber registers handlers.
type Subscriber interface {
	Subscribe(eventType string, handler eventbus.EventHandler)
}

// NewPublisher constructs a publisher.
func NewPublisher(outbox OutboxWriter, dispatch *Dispatcher, tenantID string, sub Subscriber) *Publisher {
	return &Publisher{outbox: outbox, dispatch: dispatch, tenantID: tenantID, sub: sub}
}

// Publish writes the event to outbox and triggers dispatch.
func (p *Publisher) Publish(ctx context.Context, event any) error {
	if p == nil || p.outbox == nil {
		return nil
	}
	meta := MetaFromContext(ctx, p.tenantID)
	env, err := BuildEnvelope(event, meta)
	if err != nil {
		return err
	}
	if _, err := p.outbox.Insert(ctx, env); err != nil {
		return err
	}
	if p.dispatch != nil {
		_ = p.dispatch.Dispatch(ctx, 1)
	}
	return nil
}

// Subscribe delegates to the underlying subscriber when available.
func (p *Publisher) Subscribe(eventType string, handler eventbus.EventHandler) {
	if p == nil || p.sub == nil {
		return
	}
	p.sub.Subscribe(eventType, handler)
}
