package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"microgrid-cloud/internal/telemetry/domain"
)

const defaultTelemetryTable = "telemetry_points"

// TelemetryRepository is a Postgres implementation for telemetry measurements.
type TelemetryRepository struct {
	db    *sql.DB
	table string
}

// NewTelemetryRepository constructs a repository with default table name.
func NewTelemetryRepository(db *sql.DB, opts ...RepositoryOption) *TelemetryRepository {
	repo := &TelemetryRepository{db: db, table: defaultTelemetryTable}
	for _, opt := range opts {
		opt(repo)
	}
	return repo
}

// RepositoryOption configures the repository.
type RepositoryOption func(*TelemetryRepository)

// WithTable overrides the default table name.
func WithTable(table string) RepositoryOption {
	return func(repo *TelemetryRepository) {
		if table != "" {
			repo.table = table
		}
	}
}

// InsertMeasurements upserts telemetry measurements.
func (r *TelemetryRepository) InsertMeasurements(ctx context.Context, measurements []telemetry.Measurement) error {
	if r == nil || r.db == nil {
		return errors.New("telemetry repo: nil db")
	}
	if len(measurements) == 0 {
		return nil
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	tenant_id,
	station_id,
	device_id,
	point_key,
	ts,
	value_numeric,
	value_text,
	quality
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (tenant_id, station_id, device_id, point_key, ts)
DO UPDATE SET
	value_numeric = EXCLUDED.value_numeric,
	value_text = EXCLUDED.value_text,
	quality = EXCLUDED.quality,
	updated_at = NOW()`, r.table)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, m := range measurements {
		if m.TenantID == "" || m.StationID == "" || m.DeviceID == "" || m.PointKey == "" || m.TS.IsZero() {
			_ = tx.Rollback()
			return errors.New("telemetry repo: invalid measurement")
		}

		valueNumeric := sql.NullFloat64{}
		if m.ValueNumeric != nil {
			valueNumeric = sql.NullFloat64{Float64: *m.ValueNumeric, Valid: true}
		}
		valueText := sql.NullString{}
		if m.ValueText != nil {
			valueText = sql.NullString{String: *m.ValueText, Valid: true}
		}

		if _, err := stmt.ExecContext(
			ctx,
			m.TenantID,
			m.StationID,
			m.DeviceID,
			m.PointKey,
			m.TS,
			valueNumeric,
			valueText,
			m.Quality,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}
