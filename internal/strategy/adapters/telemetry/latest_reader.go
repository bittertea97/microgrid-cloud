package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	masterdata "microgrid-cloud/internal/masterdata/domain"
)

// LatestReader reads latest telemetry for a semantic.
type LatestReader struct {
	db *sql.DB
}

// LatestValue represents a latest semantic value.
type LatestValue struct {
	Value     float64
	Timestamp time.Time
	Points    []string
}

// NewLatestReader constructs a LatestReader.
func NewLatestReader(db *sql.DB) *LatestReader {
	return &LatestReader{db: db}
}

// LatestSemantic returns the latest semantic value for a station.
func (r *LatestReader) LatestSemantic(ctx context.Context, tenantID, stationID string, semantic masterdata.Semantic) (LatestValue, error) {
	if r == nil || r.db == nil {
		return LatestValue{}, errors.New("strategy telemetry latest: nil db")
	}
	if tenantID == "" || stationID == "" || semantic == "" {
		return LatestValue{}, errors.New("strategy telemetry latest: invalid arguments")
	}

	mappings, err := r.loadMappings(ctx, stationID, string(semantic))
	if err != nil {
		return LatestValue{}, err
	}
	if len(mappings) == 0 {
		return LatestValue{}, errors.New("strategy telemetry latest: no mappings")
	}

	var (
		total float64
		ts    time.Time
		keys  []string
	)
	for _, mapping := range mappings {
		value, pointTS, ok, err := r.loadLatestPoint(ctx, tenantID, stationID, mapping.PointKey, mapping.DeviceID)
		if err != nil {
			return LatestValue{}, err
		}
		if !ok {
			continue
		}
		total += value * mapping.Factor
		if pointTS.After(ts) {
			ts = pointTS
		}
		keys = append(keys, buildPointLabel(mapping.PointKey, mapping.DeviceID))
	}

	if ts.IsZero() {
		return LatestValue{}, errors.New("strategy telemetry latest: no telemetry points")
	}
	return LatestValue{Value: total, Timestamp: ts.UTC(), Points: keys}, nil
}

type mapping struct {
	PointKey string
	DeviceID string
	Factor   float64
}

func (r *LatestReader) loadMappings(ctx context.Context, stationID, semantic string) ([]mapping, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT point_key, device_id, factor
FROM point_mappings
WHERE station_id = $1 AND semantic = $2
ORDER BY point_key ASC`, stationID, semantic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []mapping
	for rows.Next() {
		var m mapping
		var deviceID sql.NullString
		if err := rows.Scan(&m.PointKey, &deviceID, &m.Factor); err != nil {
			return nil, err
		}
		if deviceID.Valid {
			m.DeviceID = deviceID.String
		}
		if m.Factor == 0 {
			m.Factor = 1
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *LatestReader) loadLatestPoint(ctx context.Context, tenantID, stationID, pointKey, deviceID string) (float64, time.Time, bool, error) {
	if deviceID == "" {
		row := r.db.QueryRowContext(ctx, `
SELECT ts, value_numeric
FROM telemetry_points
WHERE tenant_id = $1 AND station_id = $2 AND point_key = $3
ORDER BY ts DESC
LIMIT 1`, tenantID, stationID, pointKey)
		var ts time.Time
		var value sql.NullFloat64
		if err := row.Scan(&ts, &value); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, time.Time{}, false, nil
			}
			return 0, time.Time{}, false, err
		}
		if !value.Valid {
			return 0, time.Time{}, false, nil
		}
		return value.Float64, ts.UTC(), true, nil
	}

	row := r.db.QueryRowContext(ctx, `
SELECT ts, value_numeric
FROM telemetry_points
WHERE tenant_id = $1 AND station_id = $2 AND device_id = $3 AND point_key = $4
ORDER BY ts DESC
LIMIT 1`, tenantID, stationID, deviceID, pointKey)
	var ts time.Time
	var value sql.NullFloat64
	if err := row.Scan(&ts, &value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, time.Time{}, false, nil
		}
		return 0, time.Time{}, false, err
	}
	if !value.Valid {
		return 0, time.Time{}, false, nil
	}
	return value.Float64, ts.UTC(), true, nil
}

func buildPointLabel(pointKey, deviceID string) string {
	if deviceID == "" {
		return pointKey
	}
	return fmt.Sprintf("%s@%s", pointKey, deviceID)
}
