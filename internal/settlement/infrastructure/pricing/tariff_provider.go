package pricing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	defaultTariffPlansTable = "tariff_plans"
	defaultTariffRulesTable = "tariff_rules"
)

// TariffProvider resolves price per kWh from tariff plans/rules.
type TariffProvider struct {
	db         *sql.DB
	tenantID   string
	plansTable string
	rulesTable string
}

// TariffOption configures the provider.
type TariffOption func(*TariffProvider)

// WithTariffPlansTable overrides the plans table name.
func WithTariffPlansTable(table string) TariffOption {
	return func(p *TariffProvider) {
		if table != "" {
			p.plansTable = table
		}
	}
}

// WithTariffRulesTable overrides the rules table name.
func WithTariffRulesTable(table string) TariffOption {
	return func(p *TariffProvider) {
		if table != "" {
			p.rulesTable = table
		}
	}
}

// WithTenantID sets the tenant id scope.
func WithTenantID(tenantID string) TariffOption {
	return func(p *TariffProvider) {
		if tenantID != "" {
			p.tenantID = tenantID
		}
	}
}

// NewTariffProvider constructs a provider.
func NewTariffProvider(db *sql.DB, opts ...TariffOption) *TariffProvider {
	p := &TariffProvider{
		db:         db,
		plansTable: defaultTariffPlansTable,
		rulesTable: defaultTariffRulesTable,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// PriceAt returns price per kWh for a station at a specific time.
func (p *TariffProvider) PriceAt(ctx context.Context, stationID string, at time.Time) (float64, error) {
	if p == nil || p.db == nil {
		return 0, errors.New("tariff provider: nil db")
	}
	if p.tenantID == "" {
		return 0, errors.New("tariff provider: empty tenant id")
	}
	if stationID == "" {
		return 0, errors.New("tariff provider: empty station id")
	}
	if at.IsZero() {
		return 0, errors.New("tariff provider: invalid timestamp")
	}

	month := time.Date(at.UTC().Year(), at.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)

	planID, mode, err := p.loadPlan(ctx, stationID, month)
	if err != nil {
		return 0, err
	}

	minute := at.UTC().Hour()*60 + at.UTC().Minute()
	price, err := p.loadRulePrice(ctx, planID, minute)
	if err != nil {
		return 0, err
	}

	if mode != "fixed" && mode != "tou" {
		return 0, errors.New("tariff provider: unknown mode")
	}
	return price, nil
}

func (p *TariffProvider) loadPlan(ctx context.Context, stationID string, month time.Time) (string, string, error) {
	query := fmt.Sprintf(`
SELECT id, mode
FROM %s
WHERE tenant_id = $1 AND station_id = $2 AND effective_month = $3
LIMIT 1`, p.plansTable)

	var planID string
	var mode string
	if err := p.db.QueryRowContext(ctx, query, p.tenantID, stationID, month).Scan(&planID, &mode); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", errors.New("tariff provider: plan not found")
		}
		return "", "", err
	}
	return planID, mode, nil
}

func (p *TariffProvider) loadRulePrice(ctx context.Context, planID string, minute int) (float64, error) {
	query := fmt.Sprintf(`
SELECT price_per_kwh
FROM %s
WHERE plan_id = $1 AND start_minute <= $2 AND end_minute > $2
ORDER BY start_minute ASC
LIMIT 1`, p.rulesTable)

	var price float64
	if err := p.db.QueryRowContext(ctx, query, planID, minute).Scan(&price); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errors.New("tariff provider: rule not found")
		}
		return 0, err
	}
	return price, nil
}
