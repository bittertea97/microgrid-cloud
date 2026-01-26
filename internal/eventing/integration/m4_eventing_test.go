package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/analytics/application/events"
	"microgrid-cloud/internal/eventing"
	eventingrepo "microgrid-cloud/internal/eventing/infrastructure/postgres"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestEventing_IdempotentConsumer(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if !tableExists(db, "event_outbox") ||
		!tableExists(db, "processed_events") ||
		!tableExists(db, "dead_letter_events") {
		t.Skip("missing tables; run migrations")
	}

	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM processed_events")
	_, _ = db.ExecContext(ctx, "DELETE FROM dead_letter_events")
	_, _ = db.ExecContext(ctx, "DELETE FROM event_outbox")

	baseBus := eventbus.NewInMemoryBus()
	registry := eventing.NewRegistry()
	registry.Register(events.TelemetryWindowClosed{})

	outboxStore := eventingrepo.NewOutboxStore(db)
	processedStore := eventingrepo.NewProcessedStore(db)
	dlqStore := eventingrepo.NewDLQStore(db)
	dispatcher := eventing.NewDispatcher(baseBus, outboxStore, registry, dlqStore)
	publisher := eventing.NewPublisher(outboxStore, dispatcher, "tenant-test", baseBus)

	count := 0
	eventing.Subscribe(baseBus, eventbus.EventTypeOf[events.TelemetryWindowClosed](), "consumer-a", func(ctx context.Context, event any) error {
		count++
		return nil
	}, processedStore)

	eventID := "evt-dup-001"
	ctx = eventing.WithEventID(ctx, eventID)
	ctx = eventing.WithTenantID(ctx, "tenant-test")

	payload := events.TelemetryWindowClosed{
		StationID:   "station-1",
		WindowStart: time.Date(2026, time.January, 25, 10, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, time.January, 25, 11, 0, 0, 0, time.UTC),
		OccurredAt:  time.Date(2026, time.January, 25, 11, 0, 0, 0, time.UTC),
	}

	if err := publisher.Publish(ctx, payload); err != nil {
		t.Fatalf("publish event: %v", err)
	}
	if err := publisher.Publish(ctx, payload); err != nil {
		t.Fatalf("publish duplicate: %v", err)
	}

	_ = dispatcher.Dispatch(ctx, 10)

	if count != 1 {
		t.Fatalf("expected handler once, got %d", count)
	}
}

func TestEventing_DLQOnFailure(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if !tableExists(db, "event_outbox") ||
		!tableExists(db, "processed_events") ||
		!tableExists(db, "dead_letter_events") {
		t.Skip("missing tables; run migrations")
	}

	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM processed_events")
	_, _ = db.ExecContext(ctx, "DELETE FROM dead_letter_events")
	_, _ = db.ExecContext(ctx, "DELETE FROM event_outbox")

	baseBus := eventbus.NewInMemoryBus()
	registry := eventing.NewRegistry()
	registry.Register(events.TelemetryWindowClosed{})

	outboxStore := eventingrepo.NewOutboxStore(db)
	processedStore := eventingrepo.NewProcessedStore(db)
	dlqStore := eventingrepo.NewDLQStore(db)
	dispatcher := eventing.NewDispatcher(baseBus, outboxStore, registry, dlqStore)
	publisher := eventing.NewPublisher(outboxStore, dispatcher, "tenant-test", baseBus)

	eventing.Subscribe(baseBus, eventbus.EventTypeOf[events.TelemetryWindowClosed](), "consumer-fail", func(ctx context.Context, event any) error {
		return errors.New("boom")
	}, processedStore)

	payload := events.TelemetryWindowClosed{
		StationID:   "station-2",
		WindowStart: time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, time.January, 25, 13, 0, 0, 0, time.UTC),
		OccurredAt:  time.Date(2026, time.January, 25, 13, 0, 0, 0, time.UTC),
	}

	if err := publisher.Publish(ctx, payload); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	_ = dispatcher.Dispatch(ctx, 10)

	var dlqCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM dead_letter_events").Scan(&dlqCount); err != nil {
		t.Fatalf("count dlq: %v", err)
	}
	if dlqCount != 1 {
		t.Fatalf("expected 1 dlq record, got %d", dlqCount)
	}
}

func tableExists(db *sql.DB, table string) bool {
	var exists bool
	err := db.QueryRow(`
SELECT EXISTS (
	SELECT 1
	FROM information_schema.tables
	WHERE table_schema = 'public' AND table_name = $1
)`, table).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}
