package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	settlement "microgrid-cloud/internal/settlement/domain"
)

const (
	defaultStatementsTable       = "settlement_statements"
	defaultStatementItemsTable   = "settlement_statement_items"
	defaultStatementExportsTable = "statement_exports"
)

// StatementRepository persists settlement statements.
type StatementRepository struct {
	db *sql.DB
}

// NewStatementRepository constructs a repository.
func NewStatementRepository(db *sql.DB) *StatementRepository {
	return &StatementRepository{db: db}
}

// FindLatestActive returns latest draft/frozen statement.
func (r *StatementRepository) FindLatestActive(ctx context.Context, tenantID, stationID string, month time.Time, category string) (*settlement.StatementAggregate, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("statement repo: nil db")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT id, tenant_id, station_id, statement_month, category, status, version,
	total_energy_kwh, total_amount, currency, snapshot_hash, void_reason,
	created_at, updated_at, frozen_at, voided_at
FROM settlement_statements
WHERE tenant_id = $1 AND station_id = $2 AND statement_month = $3 AND category = $4
	AND status IN ('draft','frozen')
ORDER BY version DESC
LIMIT 1`, tenantID, stationID, month, category)
	return scanStatement(row)
}

// NextVersion returns next version for station+month+category.
func (r *StatementRepository) NextVersion(ctx context.Context, tenantID, stationID string, month time.Time, category string) (int, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("statement repo: nil db")
	}
	var maxVersion sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
SELECT MAX(version)
FROM settlement_statements
WHERE tenant_id = $1 AND station_id = $2 AND statement_month = $3 AND category = $4`, tenantID, stationID, month, category).Scan(&maxVersion)
	if err != nil {
		return 0, err
	}
	if !maxVersion.Valid {
		return 1, nil
	}
	return int(maxVersion.Int64) + 1, nil
}

// CreateWithItems inserts statement and items.
func (r *StatementRepository) CreateWithItems(ctx context.Context, stmt *settlement.StatementAggregate, items []settlement.StatementItem) error {
	if r == nil || r.db == nil {
		return errors.New("statement repo: nil db")
	}
	if stmt == nil {
		return errors.New("statement repo: nil statement")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO settlement_statements (
	id, tenant_id, station_id, statement_month, category, status, version,
	total_energy_kwh, total_amount, currency, snapshot_hash, void_reason, created_at, updated_at
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14
)`,
		stmt.ID, stmt.TenantID, stmt.StationID, stmt.StatementMonth, stmt.Category, stmt.Status, stmt.Version,
		stmt.TotalEnergyKWh, stmt.TotalAmount, stmt.Currency, stmt.SnapshotHash, stmt.VoidReason, stmt.CreatedAt, stmt.UpdatedAt,
	)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
INSERT INTO settlement_statement_items (
	statement_id, day_start, energy_kwh, amount, currency, created_at
) VALUES ($1,$2,$3,$4,$5,$6)`,
			stmt.ID, item.DayStart, item.EnergyKWh, item.Amount, item.Currency, item.CreatedAt)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// GetByID fetches a statement.
func (r *StatementRepository) GetByID(ctx context.Context, id string) (*settlement.StatementAggregate, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("statement repo: nil db")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT id, tenant_id, station_id, statement_month, category, status, version,
	total_energy_kwh, total_amount, currency, snapshot_hash, void_reason,
	created_at, updated_at, frozen_at, voided_at
FROM settlement_statements
WHERE id = $1
LIMIT 1`, id)
	return scanStatement(row)
}

// ListByStationMonthCategory lists all versions for a month.
func (r *StatementRepository) ListByStationMonthCategory(ctx context.Context, tenantID, stationID string, month time.Time, category string) ([]settlement.StatementAggregate, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("statement repo: nil db")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, tenant_id, station_id, statement_month, category, status, version,
	total_energy_kwh, total_amount, currency, snapshot_hash, void_reason,
	created_at, updated_at, frozen_at, voided_at
FROM settlement_statements
WHERE tenant_id = $1 AND station_id = $2 AND statement_month = $3 AND category = $4
ORDER BY version ASC`, tenantID, stationID, month, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []settlement.StatementAggregate
	for rows.Next() {
		stmt, err := scanStatement(rows)
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			result = append(result, *stmt)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ListItems returns items for a statement.
func (r *StatementRepository) ListItems(ctx context.Context, statementID string) ([]settlement.StatementItem, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("statement repo: nil db")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT statement_id, day_start, energy_kwh, amount, currency, created_at
FROM settlement_statement_items
WHERE statement_id = $1
ORDER BY day_start ASC`, statementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []settlement.StatementItem
	for rows.Next() {
		var item settlement.StatementItem
		if err := rows.Scan(&item.StatementID, &item.DayStart, &item.EnergyKWh, &item.Amount, &item.Currency, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.DayStart = item.DayStart.UTC()
		item.CreatedAt = item.CreatedAt.UTC()
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// MarkFrozen marks statement as frozen.
func (r *StatementRepository) MarkFrozen(ctx context.Context, id, hash string, frozenAt time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("statement repo: nil db")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE settlement_statements
SET status = $1, snapshot_hash = $2, frozen_at = $3, updated_at = $3
WHERE id = $4`, settlement.StatementStatusFrozen, hash, frozenAt, id)
	return err
}

// MarkVoided marks statement as voided.
func (r *StatementRepository) MarkVoided(ctx context.Context, id, reason string, voidedAt time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("statement repo: nil db")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE settlement_statements
SET status = $1, void_reason = $2, voided_at = $3, updated_at = $3
WHERE id = $4`, settlement.StatementStatusVoided, reason, voidedAt, id)
	return err
}

// BuildItemsFromSettlements loads settlements_day and builds items/totals.
func (r *StatementRepository) BuildItemsFromSettlements(ctx context.Context, tenantID, stationID string, monthStart time.Time) ([]settlement.StatementItem, struct {
	TotalEnergyKWh float64
	TotalAmount    float64
}, string, error) {
	if r == nil || r.db == nil {
		return nil, struct {
			TotalEnergyKWh float64
			TotalAmount    float64
		}{}, "", errors.New("statement repo: nil db")
	}
	monthEnd := monthStart.AddDate(0, 1, 0)
	rows, err := r.db.QueryContext(ctx, `
SELECT day_start, energy_kwh, amount, currency
FROM settlements_day
WHERE tenant_id = $1 AND station_id = $2 AND day_start >= $3 AND day_start < $4
ORDER BY day_start ASC`, tenantID, stationID, monthStart, monthEnd)
	if err != nil {
		return nil, struct {
			TotalEnergyKWh float64
			TotalAmount    float64
		}{}, "", err
	}
	defer rows.Close()

	var items []settlement.StatementItem
	var totalEnergy float64
	var totalAmount float64
	currency := ""
	for rows.Next() {
		var dayStart time.Time
		var energy float64
		var amount float64
		var cur string
		if err := rows.Scan(&dayStart, &energy, &amount, &cur); err != nil {
			return nil, struct {
				TotalEnergyKWh float64
				TotalAmount    float64
			}{}, "", err
		}
		if currency == "" {
			currency = cur
		}
		item := settlement.StatementItem{
			StatementID: "",
			DayStart:    dayStart.UTC(),
			EnergyKWh:   energy,
			Amount:      amount,
			Currency:    cur,
			CreatedAt:   time.Now().UTC(),
		}
		items = append(items, item)
		totalEnergy += energy
		totalAmount += amount
	}
	if err := rows.Err(); err != nil {
		return nil, struct {
			TotalEnergyKWh float64
			TotalAmount    float64
		}{}, "", err
	}
	if currency == "" {
		currency = "CNY"
	}
	totals := struct {
		TotalEnergyKWh float64
		TotalAmount    float64
	}{TotalEnergyKWh: totalEnergy, TotalAmount: totalAmount}
	return items, totals, currency, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanStatement(row rowScanner) (*settlement.StatementAggregate, error) {
	var stmt settlement.StatementAggregate
	var snapshot sql.NullString
	var voidReason sql.NullString
	var frozenAt sql.NullTime
	var voidedAt sql.NullTime
	err := row.Scan(
		&stmt.ID,
		&stmt.TenantID,
		&stmt.StationID,
		&stmt.StatementMonth,
		&stmt.Category,
		&stmt.Status,
		&stmt.Version,
		&stmt.TotalEnergyKWh,
		&stmt.TotalAmount,
		&stmt.Currency,
		&snapshot,
		&voidReason,
		&stmt.CreatedAt,
		&stmt.UpdatedAt,
		&frozenAt,
		&voidedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if snapshot.Valid {
		stmt.SnapshotHash = snapshot.String
	}
	if voidReason.Valid {
		stmt.VoidReason = voidReason.String
	}
	if frozenAt.Valid {
		stmt.FrozenAt = frozenAt.Time.UTC()
	}
	if voidedAt.Valid {
		stmt.VoidedAt = voidedAt.Time.UTC()
	}
	stmt.StatementMonth = stmt.StatementMonth.UTC()
	stmt.CreatedAt = stmt.CreatedAt.UTC()
	stmt.UpdatedAt = stmt.UpdatedAt.UTC()
	return &stmt, nil
}

// RecordExport stores an export record (optional).
func (r *StatementRepository) RecordExport(ctx context.Context, statementID, format, status, path string) error {
	if r == nil || r.db == nil {
		return errors.New("statement repo: nil db")
	}
	id := fmt.Sprintf("exp-%d", time.Now().UTC().UnixNano())
	_, err := r.db.ExecContext(ctx, `
INSERT INTO statement_exports (id, statement_id, format, status, path_or_key)
VALUES ($1,$2,$3,$4,$5)`, id, statementID, format, status, path)
	return err
}
