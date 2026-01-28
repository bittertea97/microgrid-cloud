package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	// NOTE: replace the module path with your actual module name.
	domainstatistic "microgrid-cloud/internal/analytics/domain/statistic"
)

const defaultStatisticTable = "analytics_statistics"

// PostgresStatisticRepository is a Postgres implementation for statistic aggregates.
// It is subject-scoped: all read/write operations are bound to a subject_id.
type PostgresStatisticRepository struct {
	db        *sql.DB
	table     string
	subjectID string
}

// NewPostgresStatisticRepository creates a repository using the default table name.
func NewPostgresStatisticRepository(db *sql.DB, subjectID string, opts ...RepositoryOption) *PostgresStatisticRepository {
	repo := &PostgresStatisticRepository{
		db:        db,
		table:     defaultStatisticTable,
		subjectID: subjectID,
	}
	for _, opt := range opts {
		opt(repo)
	}
	return repo
}

// RepositoryOption configures the repository.
type RepositoryOption func(*PostgresStatisticRepository)

// WithTable overrides the default table name.
func WithTable(table string) RepositoryOption {
	return func(repo *PostgresStatisticRepository) {
		if table != "" {
			repo.table = table
		}
	}
}

// WithSubjectID overrides the default subject id.
func WithSubjectID(subjectID string) RepositoryOption {
	return func(repo *PostgresStatisticRepository) {
		if subjectID != "" {
			repo.subjectID = subjectID
		}
	}
}

// FindByStationHour fetches a statistic aggregate for a station + hour window.
func (r *PostgresStatisticRepository) FindByStationHour(ctx context.Context, stationID string, hourStart time.Time) (*domainstatistic.StatisticAggregate, error) {
	subjectID, err := r.resolveSubjectID(stationID)
	if err != nil {
		return nil, err
	}
	if hourStart.IsZero() {
		return nil, domainstatistic.ErrInvalidPeriodStart
	}

	timeKey, err := domainstatistic.NewTimeKey(domainstatistic.TimeTypeHour, hourStart)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
SELECT
	time_type,
	period_start,
	statistic_id,
	is_completed,
	completed_at,
	charge_kwh,
	discharge_kwh,
	earnings,
	carbon_reduction
FROM %s
WHERE subject_id = $1
	AND time_type = $2
	AND time_key = $3
LIMIT 1`, r.table)

	row := r.db.QueryRowContext(ctx, query, subjectID, string(domainstatistic.GranularityHour), timeKey.String())
	agg, err := scanAggregate(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return agg, nil
}

// Get fetches a statistic aggregate by id within the subject scope.
func (r *PostgresStatisticRepository) Get(ctx context.Context, id domainstatistic.StatisticID) (*domainstatistic.StatisticAggregate, error) {
	subjectID, err := r.resolveSubjectID("")
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, domainstatistic.ErrEmptyID
	}

	query := fmt.Sprintf(`
SELECT
	time_type,
	period_start,
	statistic_id,
	is_completed,
	completed_at,
	charge_kwh,
	discharge_kwh,
	earnings,
	carbon_reduction
FROM %s
WHERE subject_id = $1
	AND statistic_id = $2
LIMIT 1`, r.table)

	row := r.db.QueryRowContext(ctx, query, subjectID, string(id))
	agg, err := scanAggregate(row)
	if err == sql.ErrNoRows {
		return nil, domainstatistic.ErrStatisticNotFound
	}
	if err != nil {
		return nil, err
	}
	return agg, nil
}

// ListByGranularityAndPeriod lists statistics within a period range.
func (r *PostgresStatisticRepository) ListByGranularityAndPeriod(ctx context.Context, granularity domainstatistic.Granularity, startInclusive, endExclusive time.Time) ([]*domainstatistic.StatisticAggregate, error) {
	subjectID, err := r.resolveSubjectID("")
	if err != nil {
		return nil, err
	}
	if !granularity.IsValid() {
		return nil, domainstatistic.ErrInvalidGranularity
	}

	query := fmt.Sprintf(`
SELECT
	time_type,
	period_start,
	statistic_id,
	is_completed,
	completed_at,
	charge_kwh,
	discharge_kwh,
	earnings,
	carbon_reduction
FROM %s
WHERE subject_id = $1
	AND time_type = $2
	AND period_start >= $3
	AND period_start < $4
ORDER BY period_start ASC`, r.table)

	rows, err := r.db.QueryContext(ctx, query, subjectID, string(granularity), startInclusive, endExclusive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*domainstatistic.StatisticAggregate
	for rows.Next() {
		agg, err := scanAggregate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, agg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// Save upserts a statistic aggregate for the current subject.
func (r *PostgresStatisticRepository) Save(ctx context.Context, agg *domainstatistic.StatisticAggregate) error {
	subjectID, err := r.resolveSubjectID("")
	if err != nil {
		return err
	}
	if agg == nil {
		return errors.New("statistic repo: nil aggregate")
	}
	if !agg.Granularity().IsValid() {
		return domainstatistic.ErrInvalidGranularity
	}

	timeKey, err := domainstatistic.NewTimeKey(domainstatistic.TimeType(agg.Granularity()), agg.PeriodStart())
	if err != nil {
		return err
	}

	fact, completed := agg.Fact()
	completedAt, ok := agg.CompletedAt()
	completedAtValue := sql.NullTime{}
	if ok {
		completedAtValue = sql.NullTime{Time: completedAt, Valid: true}
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	subject_id,
	time_type,
	time_key,
	period_start,
	statistic_id,
	is_completed,
	completed_at,
	charge_kwh,
	discharge_kwh,
	earnings,
	carbon_reduction
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
ON CONFLICT (subject_id, time_type, time_key)
DO UPDATE SET
	period_start = EXCLUDED.period_start,
	statistic_id = EXCLUDED.statistic_id,
	is_completed = EXCLUDED.is_completed,
	completed_at = EXCLUDED.completed_at,
	charge_kwh = EXCLUDED.charge_kwh,
	discharge_kwh = EXCLUDED.discharge_kwh,
	earnings = EXCLUDED.earnings,
	carbon_reduction = EXCLUDED.carbon_reduction,
	updated_at = NOW()`, r.table)

	_, err = r.db.ExecContext(
		ctx,
		query,
		subjectID,
		string(agg.Granularity()),
		timeKey.String(),
		agg.PeriodStart(),
		string(agg.ID()),
		completed,
		completedAtValue,
		fact.ChargeKWh,
		fact.DischargeKWh,
		fact.Earnings,
		fact.CarbonReduction,
	)
	return err
}

func (r *PostgresStatisticRepository) resolveSubjectID(subjectID string) (string, error) {
	if subjectID != "" {
		if r.subjectID != "" && r.subjectID != subjectID {
			return "", errors.New("statistic repo: subject id mismatch")
		}
		return subjectID, nil
	}
	if r.subjectID == "" {
		return "", errors.New("statistic repo: empty subject id")
	}
	return r.subjectID, nil
}

func scanAggregate(scanner interface{ Scan(dest ...any) error }) (*domainstatistic.StatisticAggregate, error) {
	var (
		timeType       string
		periodStart    time.Time
		statisticID    string
		isCompleted    bool
		completedAt    sql.NullTime
		chargeKWh      float64
		dischargeKWh   float64
		earnings       float64
		carbonReduction float64
	)

	if err := scanner.Scan(
		&timeType,
		&periodStart,
		&statisticID,
		&isCompleted,
		&completedAt,
		&chargeKWh,
		&dischargeKWh,
		&earnings,
		&carbonReduction,
	); err != nil {
		return nil, err
	}

	granularity := domainstatistic.Granularity(timeType)
	if !granularity.IsValid() {
		return nil, domainstatistic.ErrInvalidGranularity
	}

	agg, err := domainstatistic.NewStatisticAggregate(domainstatistic.StatisticID(statisticID), granularity, periodStart)
	if err != nil {
		return nil, err
	}

	if isCompleted {
		if !completedAt.Valid {
			return nil, domainstatistic.ErrInvalidCompletedAt
		}
		fact := domainstatistic.StatisticFact{
			ChargeKWh:       chargeKWh,
			DischargeKWh:    dischargeKWh,
			Earnings:        earnings,
			CarbonReduction: carbonReduction,
		}
		if err := agg.Complete(fact, completedAt.Time); err != nil {
			return nil, err
		}
	}

	return agg, nil
}

