package analytics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	settlementapp "microgrid-cloud/internal/settlement/application"
)

const defaultStatisticsTable = "analytics_statistics"

// DayHourEnergyReader loads hour statistics for a day.
type DayHourEnergyReader struct {
	db            *sql.DB
	table         string
	expectedHours int
}

// NewDayHourEnergyReader constructs a reader.
func NewDayHourEnergyReader(db *sql.DB, opts ...ReaderOption) *DayHourEnergyReader {
	reader := &DayHourEnergyReader{db: db, table: defaultStatisticsTable, expectedHours: 24}
	for _, opt := range opts {
		opt(reader)
	}
	return reader
}

// ReaderOption configures the reader.
type ReaderOption func(*DayHourEnergyReader)

// WithTable overrides the statistics table name.
func WithTable(table string) ReaderOption {
	return func(reader *DayHourEnergyReader) {
		if reader != nil && table != "" {
			reader.table = table
		}
	}
}

// WithExpectedHours overrides the expected hour count.
func WithExpectedHours(expected int) ReaderOption {
	return func(reader *DayHourEnergyReader) {
		if reader != nil && expected > 0 {
			reader.expectedHours = expected
		}
	}
}

// ListDayHourEnergy returns hour energy (charge + discharge) for a station/day.
func (r *DayHourEnergyReader) ListDayHourEnergy(ctx context.Context, subjectID string, dayStart time.Time) ([]settlementapp.HourEnergy, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("day hour energy reader: nil db")
	}
	if subjectID == "" {
		return nil, errors.New("day hour energy reader: empty subject id")
	}
	if dayStart.IsZero() {
		return nil, errors.New("day hour energy reader: invalid day start")
	}

	dayStart = time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)

	query := fmt.Sprintf(`
SELECT period_start, charge_kwh, discharge_kwh, is_completed
FROM %s
WHERE subject_id = $1 AND time_type = 'HOUR' AND period_start >= $2 AND period_start < $3
ORDER BY period_start ASC`, r.table)

	rows, err := r.db.QueryContext(ctx, query, subjectID, dayStart.UTC(), dayEnd.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []settlementapp.HourEnergy
	for rows.Next() {
		var periodStart time.Time
		var charge float64
		var discharge float64
		var completed bool
		if err := rows.Scan(&periodStart, &charge, &discharge, &completed); err != nil {
			return nil, err
		}
		if !completed {
			return nil, errors.New("day hour energy reader: hour statistic not completed")
		}
		result = append(result, settlementapp.HourEnergy{
			HourStart: periodStart.UTC(),
			EnergyKWh: charge + discharge,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(result) < r.expectedHours {
		return nil, errors.New("day hour energy reader: incomplete hour statistics")
	}
	return result, nil
}
