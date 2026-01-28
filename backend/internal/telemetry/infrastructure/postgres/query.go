package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"microgrid-cloud/internal/telemetry/domain"
)

// TelemetryQuery is a Postgres query implementation.
type TelemetryQuery struct {
	db    *sql.DB
	table string
}

// NewTelemetryQuery constructs a query with default table name.
func NewTelemetryQuery(db *sql.DB, opts ...QueryOption) *TelemetryQuery {
	query := &TelemetryQuery{db: db, table: defaultTelemetryTable}
	for _, opt := range opts {
		opt(query)
	}
	return query
}

// QueryHour returns telemetry points within [start, end).
func (q *TelemetryQuery) QueryHour(ctx context.Context, tenantID, stationID string, start, end time.Time) ([]telemetry.TelemetryPoint, error) {
	if q == nil || q.db == nil {
		return nil, errors.New("telemetry query: nil db")
	}
	if tenantID == "" || stationID == "" || start.IsZero() || end.IsZero() {
		return nil, errors.New("telemetry query: invalid arguments")
	}

	query := fmt.Sprintf(`
SELECT ts, point_key, value_numeric
FROM %s
WHERE tenant_id = $1
	AND station_id = $2
	AND ts >= $3
	AND ts < $4
ORDER BY ts ASC`, q.table)

	rows, err := q.db.QueryContext(ctx, query, tenantID, stationID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byTime := make(map[time.Time]map[string]float64)
	order := make([]time.Time, 0)

	for rows.Next() {
		var ts time.Time
		var pointKey string
		var value sql.NullFloat64
		if err := rows.Scan(&ts, &pointKey, &value); err != nil {
			return nil, err
		}
		if !value.Valid {
			continue
		}
		metrics := byTime[ts]
		if metrics == nil {
			metrics = make(map[string]float64)
			byTime[ts] = metrics
			order = append(order, ts)
		}
		metrics[pointKey] = value.Float64
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(order, func(i, j int) bool { return order[i].Before(order[j]) })
	points := make([]telemetry.TelemetryPoint, 0, len(order))
	for _, ts := range order {
		points = append(points, telemetry.TelemetryPoint{At: ts, Values: byTime[ts]})
	}
	return points, nil
}

// QueryOption configures the telemetry query.
type QueryOption func(*TelemetryQuery)

// WithQueryTable overrides the default table name for queries.
func WithQueryTable(table string) QueryOption {
	return func(query *TelemetryQuery) {
		if query != nil && table != "" {
			query.table = table
		}
	}
}
