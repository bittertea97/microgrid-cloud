package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	commands "microgrid-cloud/internal/commands/domain"
)

const defaultCommandsTable = "commands"

// CommandRepository is a Postgres implementation for commands.
type CommandRepository struct {
	db    *sql.DB
	table string
}

// NewCommandRepository constructs a repository.
func NewCommandRepository(db *sql.DB) *CommandRepository {
	return &CommandRepository{db: db, table: defaultCommandsTable}
}

// FindByIdempotencyKey finds a command by idempotency key within a time window.
func (r *CommandRepository) FindByIdempotencyKey(ctx context.Context, tenantID, key string, since time.Time) (*commands.Command, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("command repo: nil db")
	}
	if tenantID == "" || key == "" {
		return nil, errors.New("command repo: invalid idempotency query")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT command_id, tenant_id, station_id, device_id, command_type, payload, idempotency_key,
	status, created_at, sent_at, acked_at, error
FROM commands
WHERE tenant_id = $1 AND idempotency_key = $2 AND created_at >= $3
ORDER BY created_at DESC
LIMIT 1`, tenantID, key, since)

	return scanCommand(row)
}

// GetByID fetches a command by id.
func (r *CommandRepository) GetByID(ctx context.Context, id string) (*commands.Command, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("command repo: nil db")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT command_id, tenant_id, station_id, device_id, command_type, payload, idempotency_key,
	status, created_at, sent_at, acked_at, error
FROM commands
WHERE command_id = $1
LIMIT 1`, id)
	return scanCommand(row)
}

// Create inserts a command.
func (r *CommandRepository) Create(ctx context.Context, cmd *commands.Command) error {
	if r == nil || r.db == nil {
		return errors.New("command repo: nil db")
	}
	if cmd == nil {
		return errors.New("command repo: nil command")
	}
	payload := cmd.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	if !json.Valid(payload) {
		return errors.New("command repo: invalid payload")
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO commands (
	command_id, tenant_id, station_id, device_id, command_type, payload, idempotency_key,
	status, created_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9
)`, cmd.CommandID, cmd.TenantID, cmd.StationID, cmd.DeviceID, cmd.CommandType, payload, cmd.IdempotencyKey, cmd.Status, cmd.CreatedAt)
	return err
}

// MarkSent marks command as sent.
func (r *CommandRepository) MarkSent(ctx context.Context, id string, sentAt time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("command repo: nil db")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE commands
SET status = $1, sent_at = $2
WHERE command_id = $3`, commands.StatusSent, sentAt, id)
	return err
}

// MarkAcked marks command as acked.
func (r *CommandRepository) MarkAcked(ctx context.Context, id string, ackedAt time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("command repo: nil db")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE commands
SET status = $1, acked_at = $2
WHERE command_id = $3`, commands.StatusAcked, ackedAt, id)
	return err
}

// MarkFailed marks command as failed.
func (r *CommandRepository) MarkFailed(ctx context.Context, id string, errMsg string) error {
	if r == nil || r.db == nil {
		return errors.New("command repo: nil db")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE commands
SET status = $1, error = $2
WHERE command_id = $3`, commands.StatusFailed, errMsg, id)
	return err
}

// MarkTimeoutBefore marks timed-out commands.
func (r *CommandRepository) MarkTimeoutBefore(ctx context.Context, before time.Time) (int, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("command repo: nil db")
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE commands
SET status = $1, error = $2
WHERE status = $3 AND sent_at < $4`, commands.StatusTimeout, "timeout", commands.StatusSent, before)
	if err != nil {
		return 0, err
	}
	count, _ := result.RowsAffected()
	return int(count), nil
}

// ListByStationAndTime lists commands for a station in a time range.
func (r *CommandRepository) ListByStationAndTime(ctx context.Context, tenantID, stationID string, from, to time.Time) ([]commands.Command, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("command repo: nil db")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT command_id, tenant_id, station_id, device_id, command_type, payload, idempotency_key,
	status, created_at, sent_at, acked_at, error
FROM commands
WHERE tenant_id = $1 AND station_id = $2 AND created_at >= $3 AND created_at < $4
ORDER BY created_at ASC`, tenantID, stationID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []commands.Command
	for rows.Next() {
		cmd, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *cmd)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCommand(row rowScanner) (*commands.Command, error) {
	var cmd commands.Command
	var payload []byte
	var sentAt sql.NullTime
	var ackedAt sql.NullTime
	var errMsg sql.NullString
	if err := row.Scan(
		&cmd.CommandID,
		&cmd.TenantID,
		&cmd.StationID,
		&cmd.DeviceID,
		&cmd.CommandType,
		&payload,
		&cmd.IdempotencyKey,
		&cmd.Status,
		&cmd.CreatedAt,
		&sentAt,
		&ackedAt,
		&errMsg,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	cmd.Payload = payload
	if sentAt.Valid {
		cmd.SentAt = sentAt.Time.UTC()
	}
	if ackedAt.Valid {
		cmd.AckedAt = ackedAt.Time.UTC()
	}
	if errMsg.Valid {
		cmd.Error = errMsg.String
	}
	cmd.CreatedAt = cmd.CreatedAt.UTC()
	return &cmd, nil
}
