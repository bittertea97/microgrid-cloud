package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	strategy "microgrid-cloud/internal/strategy/domain"
)

// Repository persists strategies and related tables.
type Repository struct {
	db *sql.DB
}

// NewRepository constructs a Repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// GetStrategy loads a strategy.
func (r *Repository) GetStrategy(ctx context.Context, stationID string) (*strategy.Strategy, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("strategy repo: nil db")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT station_id, mode, enabled, template_id, version, created_at, updated_at
FROM strategies
WHERE station_id = $1`, stationID)

	var s strategy.Strategy
	if err := row.Scan(
		&s.StationID,
		&s.Mode,
		&s.Enabled,
		&s.TemplateID,
		&s.Version,
		&s.CreatedAt,
		&s.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	s.CreatedAt = s.CreatedAt.UTC()
	s.UpdatedAt = s.UpdatedAt.UTC()
	return &s, nil
}

// UpsertStrategy creates or updates a strategy.
func (r *Repository) UpsertStrategy(ctx context.Context, s *strategy.Strategy) error {
	if r == nil || r.db == nil {
		return errors.New("strategy repo: nil db")
	}
	if s == nil {
		return errors.New("strategy repo: nil strategy")
	}
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
INSERT INTO strategies (
	station_id, mode, enabled, template_id, version, created_at, updated_at
) VALUES (
	$1,$2,$3,$4,1,$5,$5
)
ON CONFLICT (station_id)
DO UPDATE SET
	mode = EXCLUDED.mode,
	enabled = EXCLUDED.enabled,
	template_id = EXCLUDED.template_id,
	version = strategies.version + 1,
	updated_at = $5`,
		s.StationID, s.Mode, s.Enabled, s.TemplateID, now,
	)
	return err
}

// ListAutoEnabled returns strategies that are auto + enabled.
func (r *Repository) ListAutoEnabled(ctx context.Context) ([]strategy.Strategy, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("strategy repo: nil db")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT station_id, mode, enabled, template_id, version, created_at, updated_at
FROM strategies
WHERE enabled = TRUE AND mode = 'auto'
ORDER BY station_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []strategy.Strategy
	for rows.Next() {
		var s strategy.Strategy
		if err := rows.Scan(
			&s.StationID,
			&s.Mode,
			&s.Enabled,
			&s.TemplateID,
			&s.Version,
			&s.CreatedAt,
			&s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		s.CreatedAt = s.CreatedAt.UTC()
		s.UpdatedAt = s.UpdatedAt.UTC()
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// UpsertTemplate creates or updates a template.
func (r *Repository) UpsertTemplate(ctx context.Context, t *strategy.Template) error {
	if r == nil || r.db == nil {
		return errors.New("strategy repo: nil db")
	}
	if t == nil {
		return errors.New("strategy repo: nil template")
	}
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
INSERT INTO strategy_templates (
	id, type, params, created_at, updated_at
) VALUES (
	$1,$2,$3,$4,$4
)
ON CONFLICT (id)
DO UPDATE SET
	type = EXCLUDED.type,
	params = EXCLUDED.params,
	updated_at = $4`,
		t.ID, t.Type, t.Params, now,
	)
	return err
}

// GetTemplate loads a template by id.
func (r *Repository) GetTemplate(ctx context.Context, id string) (*strategy.Template, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("strategy repo: nil db")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT id, type, params, created_at, updated_at
FROM strategy_templates
WHERE id = $1`, id)
	var t strategy.Template
	if err := row.Scan(&t.ID, &t.Type, &t.Params, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	t.CreatedAt = t.CreatedAt.UTC()
	t.UpdatedAt = t.UpdatedAt.UTC()
	return &t, nil
}

// UpsertCalendar sets the calendar window for a date.
func (r *Repository) UpsertCalendar(ctx context.Context, cal *strategy.Calendar) error {
	if r == nil || r.db == nil {
		return errors.New("strategy repo: nil db")
	}
	if cal == nil {
		return errors.New("strategy repo: nil calendar")
	}
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
INSERT INTO strategy_calendar (
	strategy_id, date, enabled, start_time, end_time, created_at, updated_at
) VALUES (
	$1,$2,$3,$4,$5,$6,$6
)
ON CONFLICT (strategy_id, date)
DO UPDATE SET
	enabled = EXCLUDED.enabled,
	start_time = EXCLUDED.start_time,
	end_time = EXCLUDED.end_time,
	updated_at = $6`,
		cal.StrategyID, cal.Date, cal.Enabled, cal.StartTime, cal.EndTime, now,
	)
	return err
}

// GetCalendar loads calendar for a date.
func (r *Repository) GetCalendar(ctx context.Context, strategyID string, date time.Time) (*strategy.Calendar, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("strategy repo: nil db")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT strategy_id, date, enabled, start_time, end_time, created_at, updated_at
FROM strategy_calendar
WHERE strategy_id = $1 AND date = $2`, strategyID, date)
	var cal strategy.Calendar
	if err := row.Scan(
		&cal.StrategyID,
		&cal.Date,
		&cal.Enabled,
		&cal.StartTime,
		&cal.EndTime,
		&cal.CreatedAt,
		&cal.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	cal.Date = cal.Date.UTC()
	cal.StartTime = cal.StartTime.UTC()
	cal.EndTime = cal.EndTime.UTC()
	cal.CreatedAt = cal.CreatedAt.UTC()
	cal.UpdatedAt = cal.UpdatedAt.UTC()
	return &cal, nil
}

// InsertRun inserts a strategy run.
func (r *Repository) InsertRun(ctx context.Context, run *strategy.Run) error {
	if r == nil || r.db == nil {
		return errors.New("strategy repo: nil db")
	}
	if run == nil {
		return errors.New("strategy repo: nil run")
	}
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
INSERT INTO strategy_runs (
	strategy_id, ts, decision, command_id, status, created_at
) VALUES (
	$1,$2,$3,$4,$5,$6
)
ON CONFLICT (strategy_id, ts)
DO UPDATE SET
	decision = EXCLUDED.decision,
	command_id = EXCLUDED.command_id,
	status = EXCLUDED.status`,
		run.StrategyID, run.TS, run.Decision, run.CommandID, run.Status, now,
	)
	return err
}

// ListRuns returns runs in a time range.
func (r *Repository) ListRuns(ctx context.Context, strategyID string, from, to time.Time) ([]strategy.Run, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("strategy repo: nil db")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT strategy_id, ts, decision, command_id, status, created_at
FROM strategy_runs
WHERE strategy_id = $1 AND ts >= $2 AND ts < $3
ORDER BY ts ASC`, strategyID, from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []strategy.Run
	for rows.Next() {
		var run strategy.Run
		if err := rows.Scan(&run.StrategyID, &run.TS, &run.Decision, &run.CommandID, &run.Status, &run.CreatedAt); err != nil {
			return nil, err
		}
		run.TS = run.TS.UTC()
		run.CreatedAt = run.CreatedAt.UTC()
		result = append(result, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
