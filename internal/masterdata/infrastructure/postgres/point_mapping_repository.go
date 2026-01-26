package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	masterdata "microgrid-cloud/internal/masterdata/domain"
)

const defaultPointMappingsTable = "point_mappings"

// PointMappingRepository is a Postgres implementation for point mappings.
type PointMappingRepository struct {
	db    *sql.DB
	table string
}

// NewPointMappingRepository constructs a repository.
func NewPointMappingRepository(db *sql.DB, opts ...PointMappingOption) *PointMappingRepository {
	repo := &PointMappingRepository{db: db, table: defaultPointMappingsTable}
	for _, opt := range opts {
		opt(repo)
	}
	return repo
}

// PointMappingOption configures the repository.
type PointMappingOption func(*PointMappingRepository)

// WithPointMappingTable overrides the table name.
func WithPointMappingTable(table string) PointMappingOption {
	return func(repo *PointMappingRepository) {
		if table != "" {
			repo.table = table
		}
	}
}

// ListByStation loads mappings for a station.
func (r *PointMappingRepository) ListByStation(ctx context.Context, stationID string) ([]masterdata.PointMapping, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("point mapping repo: nil db")
	}
	if stationID == "" {
		return nil, errors.New("point mapping repo: empty station id")
	}

	query := fmt.Sprintf(`
SELECT id, station_id, device_id, point_key, semantic, unit, factor, created_at, updated_at
FROM %s
WHERE station_id = $1
ORDER BY point_key ASC`, r.table)

	rows, err := r.db.QueryContext(ctx, query, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []masterdata.PointMapping
	for rows.Next() {
		var mapping masterdata.PointMapping
		var deviceID sql.NullString
		if err := rows.Scan(
			&mapping.ID,
			&mapping.StationID,
			&deviceID,
			&mapping.PointKey,
			&mapping.Semantic,
			&mapping.Unit,
			&mapping.Factor,
			&mapping.CreatedAt,
			&mapping.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if deviceID.Valid {
			mapping.DeviceID = deviceID.String
		}
		mapping.CreatedAt = mapping.CreatedAt.UTC()
		mapping.UpdatedAt = mapping.UpdatedAt.UTC()
		result = append(result, mapping)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// Save upserts a mapping.
func (r *PointMappingRepository) Save(ctx context.Context, mapping *masterdata.PointMapping) error {
	if r == nil || r.db == nil {
		return errors.New("point mapping repo: nil db")
	}
	if mapping == nil {
		return errors.New("point mapping repo: nil mapping")
	}
	if err := mapping.Validate(); err != nil {
		return err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id,
	station_id,
	device_id,
	point_key,
	semantic,
	unit,
	factor
) VALUES (
	$1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (id)
DO UPDATE SET
	station_id = EXCLUDED.station_id,
	device_id = EXCLUDED.device_id,
	point_key = EXCLUDED.point_key,
	semantic = EXCLUDED.semantic,
	unit = EXCLUDED.unit,
	factor = EXCLUDED.factor,
	updated_at = NOW()`, r.table)

	var deviceID sql.NullString
	if mapping.DeviceID != "" {
		deviceID = sql.NullString{String: mapping.DeviceID, Valid: true}
	}

	_, err := r.db.ExecContext(
		ctx,
		query,
		mapping.ID,
		mapping.StationID,
		deviceID,
		mapping.PointKey,
		mapping.Semantic,
		mapping.Unit,
		mapping.Factor,
	)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if mapping.CreatedAt.IsZero() {
		mapping.CreatedAt = now
	}
	mapping.UpdatedAt = now
	return nil
}
