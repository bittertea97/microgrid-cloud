package eventing

import (
	"context"
	"log"
	"reflect"
	"time"

	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/observability/metrics"
)

// Publisher writes events to outbox.
type Publisher struct {
	outbox   OutboxWriter
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
func NewPublisher(outbox OutboxWriter, tenantID string, sub Subscriber) *Publisher {
	return &Publisher{outbox: outbox, tenantID: tenantID, sub: sub}
}

// Publish writes the event to outbox.
func (p *Publisher) Publish(ctx context.Context, event any) error {
	start := time.Now()
	result := metrics.ResultSuccess
	if p == nil || p.outbox == nil {
		metrics.ObserveOutboxPublish(result, time.Since(start))
		return nil
	}
	meta := MetaFromContext(ctx, p.tenantID)
	env, err := BuildEnvelope(event, meta)
	if err != nil {
		result = metrics.ResultError
		metrics.ObserveOutboxPublish(result, time.Since(start))
		return err
	}
	if _, err := p.outbox.Insert(ctx, env); err != nil {
		result = metrics.ResultError
		metrics.ObserveOutboxPublish(result, time.Since(start))
		return err
	}
	duration := time.Since(start)
	metrics.ObserveOutboxPublish(result, duration)
	if duration > 50*time.Millisecond {
		log.Printf("outbox_publish duration_ms=%d result=%s event_type=%s",
			duration.Milliseconds(),
			result,
			reflect.TypeOf(event).String(),
		)
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
