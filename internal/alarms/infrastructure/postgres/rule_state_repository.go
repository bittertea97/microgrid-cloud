package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	alarms "microgrid-cloud/internal/alarms/domain"
)

const defaultAlarmRuleStatesTable = "alarm_rule_states"

// AlarmRuleStateRepository stores pending rule evaluations.
type AlarmRuleStateRepository struct {
	db    *sql.DB
	table string
}

// NewAlarmRuleStateRepository constructs a repository.
func NewAlarmRuleStateRepository(db *sql.DB) *AlarmRuleStateRepository {
	return &AlarmRuleStateRepository{db: db, table: defaultAlarmRuleStatesTable}
}

// Get fetches a pending rule state.
func (r *AlarmRuleStateRepository) Get(ctx context.Context, tenantID, ruleID, originatorType, originatorID string) (*alarms.AlarmRuleState, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("alarm state repo: nil db")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT tenant_id, station_id, rule_id, originator_type, originator_id, pending_since, last_value, updated_at
FROM alarm_rule_states
WHERE tenant_id = $1 AND rule_id = $2 AND originator_type = $3 AND originator_id = $4`, tenantID, ruleID, originatorType, originatorID)

	var state alarms.AlarmRuleState
	var lastValue sql.NullFloat64
	if err := row.Scan(
		&state.TenantID,
		&state.StationID,
		&state.RuleID,
		&state.OriginatorType,
		&state.OriginatorID,
		&state.PendingSince,
		&lastValue,
		&state.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	state.PendingSince = state.PendingSince.UTC()
	state.UpdatedAt = state.UpdatedAt.UTC()
	if lastValue.Valid {
		state.LastValue = lastValue.Float64
	}
	return &state, nil
}

// Upsert inserts or updates a pending rule state.
func (r *AlarmRuleStateRepository) Upsert(ctx context.Context, state *alarms.AlarmRuleState) error {
	if r == nil || r.db == nil {
		return errors.New("alarm state repo: nil db")
	}
	if state == nil {
		return errors.New("alarm state repo: nil state")
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO alarm_rule_states (
	tenant_id, station_id, rule_id, originator_type, originator_id,
	pending_since, last_value, updated_at
) VALUES (
	$1, $2, $3, $4, $5,
	$6, $7, $8
)
ON CONFLICT (tenant_id, rule_id, originator_type, originator_id)
DO UPDATE SET
	station_id = EXCLUDED.station_id,
	pending_since = EXCLUDED.pending_since,
	last_value = EXCLUDED.last_value,
	updated_at = EXCLUDED.updated_at`,
		state.TenantID,
		state.StationID,
		state.RuleID,
		state.OriginatorType,
		state.OriginatorID,
		state.PendingSince,
		sql.NullFloat64{Float64: state.LastValue, Valid: true},
		state.UpdatedAt,
	)
	return err
}

// Clear deletes a pending rule state.
func (r *AlarmRuleStateRepository) Clear(ctx context.Context, tenantID, ruleID, originatorType, originatorID string) error {
	if r == nil || r.db == nil {
		return errors.New("alarm state repo: nil db")
	}
	_, err := r.db.ExecContext(ctx, `
DELETE FROM alarm_rule_states
WHERE tenant_id = $1 AND rule_id = $2 AND originator_type = $3 AND originator_id = $4`, tenantID, ruleID, originatorType, originatorID)
	return err
}
