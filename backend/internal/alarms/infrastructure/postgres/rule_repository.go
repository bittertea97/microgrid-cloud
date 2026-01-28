package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	alarms "microgrid-cloud/internal/alarms/domain"
	"microgrid-cloud/internal/audit"
	"microgrid-cloud/internal/auth"
)

const defaultAlarmRulesTable = "alarm_rules"

// AlarmRuleRepository is a Postgres repository for alarm rules.
type AlarmRuleRepository struct {
	db    *sql.DB
	table string
}

// NewAlarmRuleRepository constructs a repository.
func NewAlarmRuleRepository(db *sql.DB) *AlarmRuleRepository {
	return &AlarmRuleRepository{db: db, table: defaultAlarmRulesTable}
}

// Create inserts an alarm rule.
func (r *AlarmRuleRepository) Create(ctx context.Context, rule *alarms.AlarmRule) error {
	if r == nil || r.db == nil {
		return errors.New("alarm rule repo: nil db")
	}
	if rule == nil {
		return errors.New("alarm rule repo: nil rule")
	}
	if err := rule.Validate(); err != nil {
		return err
	}
	if rule.Severity == "" {
		rule.Severity = "medium"
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now().UTC()
	}
	if rule.UpdatedAt.IsZero() {
		rule.UpdatedAt = rule.CreatedAt
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO alarm_rules (
	id, tenant_id, station_id, name, semantic, operator, threshold, hysteresis,
	duration_seconds, severity, enabled, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8,
	$9, $10, $11, $12, $13
)`, rule.ID, rule.TenantID, rule.StationID, rule.Name, rule.Semantic, string(rule.Operator),
		rule.Threshold, rule.Hysteresis, rule.DurationSeconds, rule.Severity, rule.Enabled,
		rule.CreatedAt, rule.UpdatedAt)
	if err != nil {
		return err
	}
	logAlarmRuleAudit(ctx, r.db, rule)
	return nil
}

// GetByID loads a rule by id.
func (r *AlarmRuleRepository) GetByID(ctx context.Context, tenantID, ruleID string) (*alarms.AlarmRule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("alarm rule repo: nil db")
	}
	if tenantID == "" || ruleID == "" {
		return nil, errors.New("alarm rule repo: invalid query")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT id, tenant_id, station_id, name, semantic, operator, threshold, hysteresis,
	duration_seconds, severity, enabled, created_at, updated_at
FROM alarm_rules
WHERE tenant_id = $1 AND id = $2
LIMIT 1`, tenantID, ruleID)
	var rule alarms.AlarmRule
	var op string
	if err := row.Scan(
		&rule.ID,
		&rule.TenantID,
		&rule.StationID,
		&rule.Name,
		&rule.Semantic,
		&op,
		&rule.Threshold,
		&rule.Hysteresis,
		&rule.DurationSeconds,
		&rule.Severity,
		&rule.Enabled,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	rule.Operator = alarms.Operator(op)
	rule.CreatedAt = rule.CreatedAt.UTC()
	rule.UpdatedAt = rule.UpdatedAt.UTC()
	return &rule, nil
}

// ListEnabledByStation returns enabled rules for a station.
func (r *AlarmRuleRepository) ListEnabledByStation(ctx context.Context, tenantID, stationID string) ([]alarms.AlarmRule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("alarm rule repo: nil db")
	}
	if tenantID == "" || stationID == "" {
		return nil, errors.New("alarm rule repo: invalid query")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, tenant_id, station_id, name, semantic, operator, threshold, hysteresis,
	duration_seconds, severity, enabled, created_at, updated_at
FROM alarm_rules
WHERE tenant_id = $1 AND station_id = $2 AND enabled = TRUE
ORDER BY created_at ASC`, tenantID, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []alarms.AlarmRule
	for rows.Next() {
		var rule alarms.AlarmRule
		var op string
		if err := rows.Scan(
			&rule.ID,
			&rule.TenantID,
			&rule.StationID,
			&rule.Name,
			&rule.Semantic,
			&op,
			&rule.Threshold,
			&rule.Hysteresis,
			&rule.DurationSeconds,
			&rule.Severity,
			&rule.Enabled,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rule.Operator = alarms.Operator(op)
		rule.CreatedAt = rule.CreatedAt.UTC()
		rule.UpdatedAt = rule.UpdatedAt.UTC()
		result = append(result, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func logAlarmRuleAudit(ctx context.Context, db *sql.DB, rule *alarms.AlarmRule) {
	if db == nil || rule == nil {
		return
	}
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID == "" {
		tenantID = rule.TenantID
	}
	if tenantID == "" {
		return
	}
	meta, _ := json.Marshal(map[string]any{
		"station_id":       rule.StationID,
		"name":             rule.Name,
		"semantic":         rule.Semantic,
		"operator":         rule.Operator,
		"threshold":        rule.Threshold,
		"hysteresis":       rule.Hysteresis,
		"duration_seconds": rule.DurationSeconds,
		"severity":         rule.Severity,
		"enabled":          rule.Enabled,
	})
	repo := audit.NewRepository(db)
	if repo == nil {
		return
	}
	_ = repo.Log(ctx, audit.Entry{
		TenantID:     tenantID,
		Actor:        auth.SubjectFromContext(ctx),
		Role:         string(auth.RoleFromContext(ctx)),
		Action:       "alarm_rule.create",
		ResourceType: "alarm_rule",
		ResourceID:   rule.ID,
		StationID:    rule.StationID,
		Metadata:     meta,
		CreatedAt:    time.Now().UTC(),
	})
}
