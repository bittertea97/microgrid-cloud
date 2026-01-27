package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	alarms "microgrid-cloud/internal/alarms/domain"
)

const defaultAlarmsTable = "alarms"

// AlarmRepository is a Postgres repository for alarms.
type AlarmRepository struct {
	db    *sql.DB
	table string
}

// NewAlarmRepository constructs a repository.
func NewAlarmRepository(db *sql.DB) *AlarmRepository {
	return &AlarmRepository{db: db, table: defaultAlarmsTable}
}

// Create inserts a new alarm.
func (r *AlarmRepository) Create(ctx context.Context, alarm *alarms.Alarm) error {
	if r == nil || r.db == nil {
		return errors.New("alarm repo: nil db")
	}
	if alarm == nil {
		return errors.New("alarm repo: nil alarm")
	}
	if alarm.ID == "" || alarm.TenantID == "" || alarm.RuleID == "" || alarm.StationID == "" {
		return errors.New("alarm repo: missing fields")
	}
	if alarm.CreatedAt.IsZero() {
		alarm.CreatedAt = time.Now().UTC()
	}
	if alarm.UpdatedAt.IsZero() {
		alarm.UpdatedAt = alarm.CreatedAt
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO alarms (
	id, tenant_id, station_id, originator_type, originator_id, rule_id, status,
	start_at, end_at, last_value, acked_at, cleared_at, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13, $14
)`,
		alarm.ID,
		alarm.TenantID,
		alarm.StationID,
		alarm.OriginatorType,
		alarm.OriginatorID,
		alarm.RuleID,
		alarm.Status,
		alarm.StartAt,
		nullableTime(alarm.EndAt),
		sql.NullFloat64{Float64: alarm.LastValue, Valid: true},
		nullableTime(alarm.AckedAt),
		nullableTime(alarm.ClearedAt),
		alarm.CreatedAt,
		alarm.UpdatedAt,
	)
	return err
}

// GetByID fetches an alarm by id.
func (r *AlarmRepository) GetByID(ctx context.Context, id string) (*alarms.Alarm, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("alarm repo: nil db")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT id, tenant_id, station_id, originator_type, originator_id, rule_id, status,
	start_at, end_at, last_value, acked_at, cleared_at, created_at, updated_at
FROM alarms
WHERE id = $1`, id)
	return scanAlarm(row)
}

// FindOpenByRuleOriginator returns active or acknowledged alarm for a rule originator.
func (r *AlarmRepository) FindOpenByRuleOriginator(ctx context.Context, tenantID, ruleID, originatorType, originatorID string) (*alarms.Alarm, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("alarm repo: nil db")
	}
	if tenantID == "" || ruleID == "" || originatorID == "" {
		return nil, errors.New("alarm repo: invalid query")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT id, tenant_id, station_id, originator_type, originator_id, rule_id, status,
	start_at, end_at, last_value, acked_at, cleared_at, created_at, updated_at
FROM alarms
WHERE tenant_id = $1 AND rule_id = $2 AND originator_type = $3 AND originator_id = $4
	AND status IN ('active', 'acknowledged')
ORDER BY created_at DESC
LIMIT 1`, tenantID, ruleID, originatorType, originatorID)
	return scanAlarm(row)
}

// UpdateLastValue updates the last value and updated_at.
func (r *AlarmRepository) UpdateLastValue(ctx context.Context, id string, value float64, updatedAt time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("alarm repo: nil db")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE alarms
SET last_value = $1, updated_at = $2
WHERE id = $3`, value, updatedAt, id)
	return err
}

// MarkAcknowledged marks an alarm as acknowledged.
func (r *AlarmRepository) MarkAcknowledged(ctx context.Context, id string, ackedAt time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("alarm repo: nil db")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE alarms
SET status = $1, acked_at = $2, updated_at = $3
WHERE id = $4`, alarms.StatusAcknowledged, ackedAt, ackedAt, id)
	return err
}

// MarkCleared marks an alarm as cleared.
func (r *AlarmRepository) MarkCleared(ctx context.Context, id string, value float64, clearedAt time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("alarm repo: nil db")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE alarms
SET status = $1, last_value = $2, end_at = $3, cleared_at = $4, updated_at = $5
WHERE id = $6`, alarms.StatusCleared, value, clearedAt, clearedAt, clearedAt, id)
	return err
}

// ListByStationStatusAndTime lists alarms for station within time window.
func (r *AlarmRepository) ListByStationStatusAndTime(ctx context.Context, tenantID, stationID, status string, from, to time.Time) ([]alarms.Alarm, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("alarm repo: nil db")
	}
	if tenantID == "" || stationID == "" {
		return nil, errors.New("alarm repo: invalid query")
	}
	query := `
SELECT id, tenant_id, station_id, originator_type, originator_id, rule_id, status,
	start_at, end_at, last_value, acked_at, cleared_at, created_at, updated_at
FROM alarms
WHERE tenant_id = $1 AND station_id = $2 AND start_at >= $3 AND start_at < $4`
	args := []any{tenantID, stationID, from, to}
	if status != "" {
		query += " AND status = $5"
		args = append(args, status)
	}
	query += " ORDER BY start_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []alarms.Alarm
	for rows.Next() {
		alarm, err := scanAlarm(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *alarm)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

type alarmScanner interface {
	Scan(dest ...any) error
}

func scanAlarm(row alarmScanner) (*alarms.Alarm, error) {
	var alarm alarms.Alarm
	var endAt sql.NullTime
	var ackedAt sql.NullTime
	var clearedAt sql.NullTime
	var lastValue sql.NullFloat64
	if err := row.Scan(
		&alarm.ID,
		&alarm.TenantID,
		&alarm.StationID,
		&alarm.OriginatorType,
		&alarm.OriginatorID,
		&alarm.RuleID,
		&alarm.Status,
		&alarm.StartAt,
		&endAt,
		&lastValue,
		&ackedAt,
		&clearedAt,
		&alarm.CreatedAt,
		&alarm.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	alarm.StartAt = alarm.StartAt.UTC()
	alarm.CreatedAt = alarm.CreatedAt.UTC()
	alarm.UpdatedAt = alarm.UpdatedAt.UTC()
	if endAt.Valid {
		alarm.EndAt = endAt.Time.UTC()
	}
	if ackedAt.Valid {
		alarm.AckedAt = ackedAt.Time.UTC()
	}
	if clearedAt.Valid {
		alarm.ClearedAt = clearedAt.Time.UTC()
	}
	if lastValue.Valid {
		alarm.LastValue = lastValue.Float64
	}
	return &alarm, nil
}

func nullableTime(value time.Time) sql.NullTime {
	if value.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value, Valid: true}
}
