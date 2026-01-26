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

const defaultOutboxTable = "event_outbox"

// OutboxStore is a Postgres implementation for outbox records.
type OutboxStore struct {
	db    *sql.DB
	table string
}

// NewOutboxStore constructs an outbox store.
func NewOutboxStore(db *sql.DB, opts ...OutboxOption) *OutboxStore {
	store := &OutboxStore{db: db, table: defaultOutboxTable}
	for _, opt := range opts {
		opt(store)
	}
	return store
}

// OutboxOption configures the outbox store.
type OutboxOption func(*OutboxStore)

// WithOutboxTable overrides the table name.
func WithOutboxTable(table string) OutboxOption {
	return func(store *OutboxStore) {
		if table != "" {
			store.table = table
		}
	}
}

// Insert writes an envelope to outbox.
func (s *OutboxStore) Insert(ctx context.Context, env eventing.Envelope) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("outbox store: nil db")
	}
	payload, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	outboxID := eventing.NewEventID()
	query := fmt.Sprintf(`
INSERT INTO %s (
	id,
	event_id,
	event_type,
	payload,
	status,
	attempts
) VALUES (
	$1, $2, $3, $4, 'pending', 0
)
ON CONFLICT (id)
DO NOTHING`, s.table)

	_, err = s.db.ExecContext(ctx, query, outboxID, env.EventID, env.EventType, payload)
	if err != nil {
		return "", err
	}
	return outboxID, nil
}

// ListPending returns pending outbox records.
func (s *OutboxStore) ListPending(ctx context.Context, limit int) ([]eventing.OutboxRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("outbox store: nil db")
	}
	if limit <= 0 {
		limit = 50
	}
	query := fmt.Sprintf(`
SELECT id, payload
FROM %s
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT $1`, s.table)

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []eventing.OutboxRecord
	for rows.Next() {
		var id string
		var payload []byte
		if err := rows.Scan(&id, &payload); err != nil {
			return nil, err
		}
		var env eventing.Envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return nil, err
		}
		result = append(result, eventing.OutboxRecord{ID: id, Envelope: env})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// MarkSent marks outbox record as sent.
func (s *OutboxStore) MarkSent(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return errors.New("outbox store: nil db")
	}
	query := fmt.Sprintf(`
UPDATE %s
SET status = 'sent', sent_at = $1
WHERE id = $2`, s.table)
	_, err := s.db.ExecContext(ctx, query, time.Now().UTC(), id)
	return err
}

// MarkFailed marks outbox record as failed and increments attempts.
func (s *OutboxStore) MarkFailed(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return errors.New("outbox store: nil db")
	}
	query := fmt.Sprintf(`
UPDATE %s
SET status = 'failed', attempts = attempts + 1
WHERE id = $1`, s.table)
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}
