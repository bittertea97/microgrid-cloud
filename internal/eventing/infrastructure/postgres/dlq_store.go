package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"microgrid-cloud/internal/eventing"
)

const defaultDLQTable = "dead_letter_events"

// DLQStore is a Postgres implementation for dead letter events.
type DLQStore struct {
	db    *sql.DB
	table string
}

// NewDLQStore constructs a DLQ store.
func NewDLQStore(db *sql.DB, opts ...DLQOption) *DLQStore {
	store := &DLQStore{db: db, table: defaultDLQTable}
	for _, opt := range opts {
		opt(store)
	}
	return store
}

// DLQOption configures the DLQ store.
type DLQOption func(*DLQStore)

// WithDLQTable overrides the table name.
func WithDLQTable(table string) DLQOption {
	return func(store *DLQStore) {
		if table != "" {
			store.table = table
		}
	}
}

// RecordFailure inserts or updates a DLQ record.
func (s *DLQStore) RecordFailure(ctx context.Context, env eventing.Envelope, err error) error {
	if s == nil || s.db == nil {
		return errors.New("dlq store: nil db")
	}
	if env.EventID == "" {
		return errors.New("dlq store: empty event id")
	}
	payload, marshalErr := json.Marshal(env)
	if marshalErr != nil {
		return marshalErr
	}
	message := ""
	if err != nil {
		message = err.Error()
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	event_id,
	event_type,
	payload,
	error,
	first_seen_at,
	last_seen_at,
	attempts
) VALUES (
	$1, $2, $3, $4, $5, $5, 1
)
ON CONFLICT (event_id)
DO UPDATE SET
	event_type = EXCLUDED.event_type,
	payload = EXCLUDED.payload,
	error = EXCLUDED.error,
	last_seen_at = EXCLUDED.last_seen_at,
	attempts = %s.attempts + 1`, s.table, s.table)

	now := time.Now().UTC()
	_, execErr := s.db.ExecContext(ctx, query, env.EventID, env.EventType, payload, message, now)
	return execErr
}
