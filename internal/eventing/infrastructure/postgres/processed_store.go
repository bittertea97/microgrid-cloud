package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const defaultProcessedTable = "processed_events"

// ProcessedStore is a Postgres implementation for processed events.
type ProcessedStore struct {
	db    *sql.DB
	table string
}

// NewProcessedStore constructs a processed store.
func NewProcessedStore(db *sql.DB, opts ...ProcessedOption) *ProcessedStore {
	store := &ProcessedStore{db: db, table: defaultProcessedTable}
	for _, opt := range opts {
		opt(store)
	}
	return store
}

// ProcessedOption configures the processed store.
type ProcessedOption func(*ProcessedStore)

// WithProcessedTable overrides table name.
func WithProcessedTable(table string) ProcessedOption {
	return func(store *ProcessedStore) {
		if table != "" {
			store.table = table
		}
	}
}

// HasProcessed checks if event was already processed.
func (s *ProcessedStore) HasProcessed(ctx context.Context, eventID, consumerName string) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("processed store: nil db")
	}
	if eventID == "" || consumerName == "" {
		return false, errors.New("processed store: invalid arguments")
	}
	query := fmt.Sprintf(`
SELECT EXISTS (
	SELECT 1 FROM %s WHERE event_id = $1 AND consumer_name = $2
)`, s.table)
	var exists bool
	if err := s.db.QueryRowContext(ctx, query, eventID, consumerName).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// MarkProcessed records an event as processed.
func (s *ProcessedStore) MarkProcessed(ctx context.Context, eventID, consumerName string) error {
	if s == nil || s.db == nil {
		return errors.New("processed store: nil db")
	}
	if eventID == "" || consumerName == "" {
		return errors.New("processed store: invalid arguments")
	}
	query := fmt.Sprintf(`
INSERT INTO %s (event_id, consumer_name, processed_at)
VALUES ($1, $2, $3)
ON CONFLICT (event_id, consumer_name)
DO NOTHING`, s.table)
	_, err := s.db.ExecContext(ctx, query, eventID, consumerName, time.Now().UTC())
	return err
}
