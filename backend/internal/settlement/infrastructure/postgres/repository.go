package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"microgrid-cloud/internal/settlement/domain"
)

const defaultSettlementTable = "settlements_day"

// SettlementRepository is a Postgres implementation for settlements.
type SettlementRepository struct {
	db       *sql.DB
	table    string
	tenantID string
	currency string
	status   string
}

// NewSettlementRepository constructs a repository with defaults.
func NewSettlementRepository(db *sql.DB, opts ...RepositoryOption) *SettlementRepository {
	repo := &SettlementRepository{
		db:       db,
		table:    defaultSettlementTable,
		currency: "CNY",
		status:   "CALCULATED",
	}
	for _, opt := range opts {
		opt(repo)
	}
	return repo
}

// RepositoryOption configures the repository.
type RepositoryOption func(*SettlementRepository)

// WithTable overrides the default table.
func WithTable(table string) RepositoryOption {
	return func(repo *SettlementRepository) {
		if table != "" {
			repo.table = table
		}
	}
}

// WithTenantID sets the tenant id.
func WithTenantID(tenantID string) RepositoryOption {
	return func(repo *SettlementRepository) {
		if tenantID != "" {
			repo.tenantID = tenantID
		}
	}
}

// WithCurrency sets the currency code.
func WithCurrency(currency string) RepositoryOption {
	return func(repo *SettlementRepository) {
		if currency != "" {
			repo.currency = currency
		}
	}
}

// WithStatus sets the status string.
func WithStatus(status string) RepositoryOption {
	return func(repo *SettlementRepository) {
		if status != "" {
			repo.status = status
		}
	}
}

// FindBySubjectAndDay loads a settlement aggregate.
func (r *SettlementRepository) FindBySubjectAndDay(ctx context.Context, subjectID string, dayStart time.Time) (*settlement.SettlementAggregate, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("settlement repo: nil db")
	}
	if r.tenantID == "" {
		return nil, errors.New("settlement repo: empty tenant id")
	}
	if subjectID == "" {
		return nil, settlement.ErrEmptySubjectID
	}
	if dayStart.IsZero() {
		return nil, settlement.ErrInvalidDayStart
	}

	query := fmt.Sprintf(`
SELECT day_start, energy_kwh, amount
FROM %s
WHERE tenant_id = $1 AND station_id = $2 AND day_start = $3
LIMIT 1`, r.table)

	var storedDay time.Time
	var energy float64
	var amount float64
	row := r.db.QueryRowContext(ctx, query, r.tenantID, subjectID, dayStart.UTC())
	if err := row.Scan(&storedDay, &energy, &amount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	agg, err := settlement.NewDaySettlementAggregate(subjectID, storedDay)
	if err != nil {
		return nil, err
	}
	if err := agg.Recalculate(energy, amount); err != nil {
		return nil, err
	}
	agg.MarkPersisted()
	return agg, nil
}

// Save upserts the settlement aggregate.
func (r *SettlementRepository) Save(ctx context.Context, aggregate *settlement.SettlementAggregate) error {
	if r == nil || r.db == nil {
		return errors.New("settlement repo: nil db")
	}
	if aggregate == nil {
		return settlement.ErrNilAggregate
	}
	if r.tenantID == "" {
		return errors.New("settlement repo: empty tenant id")
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	tenant_id,
	station_id,
	day_start,
	energy_kwh,
	amount,
	currency,
	status,
	version
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, 1
)
ON CONFLICT (tenant_id, station_id, day_start)
DO UPDATE SET
	energy_kwh = EXCLUDED.energy_kwh,
	amount = EXCLUDED.amount,
	currency = EXCLUDED.currency,
	status = EXCLUDED.status,
	version = %s.version + 1,
	updated_at = NOW()`, r.table, r.table)

	_, err := r.db.ExecContext(
		ctx,
		query,
		r.tenantID,
		aggregate.SubjectID(),
		aggregate.DayStart().UTC(),
		aggregate.EnergyKWh(),
		aggregate.Amount(),
		r.currency,
		r.status,
	)
	if err != nil {
		return err
	}

	aggregate.MarkPersisted()
	return nil
}
