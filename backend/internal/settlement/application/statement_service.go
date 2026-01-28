package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"time"

	"microgrid-cloud/internal/auth"
	"microgrid-cloud/internal/observability/metrics"
	settlement "microgrid-cloud/internal/settlement/domain"
	statementrepo "microgrid-cloud/internal/settlement/infrastructure/postgres"
)

// StatementService handles settlement statement workflows.
type StatementService struct {
	repo     *statementrepo.StatementRepository
	tenantID string
}

// NewStatementService constructs a service.
func NewStatementService(repo *statementrepo.StatementRepository, tenantID string) (*StatementService, error) {
	if repo == nil {
		return nil, errors.New("statement service: nil repo")
	}
	if tenantID == "" {
		return nil, errors.New("statement service: empty tenant id")
	}
	return &StatementService{repo: repo, tenantID: tenantID}, nil
}

// Generate creates or returns a statement draft.
func (s *StatementService) Generate(ctx context.Context, stationID, month, category string, regenerate bool) (*settlement.StatementAggregate, error) {
	start := time.Now()
	result := metrics.ResultSuccess
	defer func() {
		metrics.ObserveStatementGenerate(result, time.Since(start))
	}()

	if stationID == "" {
		result = metrics.ResultError
		return nil, errors.New("statement service: station_id required")
	}
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID == "" {
		tenantID = s.tenantID
	}
	monthStart, err := parseMonth(month)
	if err != nil {
		result = metrics.ResultError
		return nil, err
	}
	if category == "" {
		category = "owner"
	}

	if !regenerate {
		existing, err := s.repo.FindLatestActive(ctx, tenantID, stationID, monthStart, category)
		if err != nil {
			result = metrics.ResultError
			return nil, err
		}
		if existing != nil {
			if tenantID != "" && existing.TenantID != tenantID {
				result = metrics.ResultError
				return nil, auth.ErrTenantMismatch
			}
			return existing, nil
		}
	}

	version, err := s.repo.NextVersion(ctx, tenantID, stationID, monthStart, category)
	if err != nil {
		result = metrics.ResultError
		return nil, err
	}

	items, totals, currency, err := s.repo.BuildItemsFromSettlements(ctx, tenantID, stationID, monthStart)
	if err != nil {
		result = metrics.ResultError
		return nil, err
	}
	statementID := buildStatementID(stationID, monthStart, category, version)
	now := time.Now().UTC()

	stmt := &settlement.StatementAggregate{
		ID:             statementID,
		TenantID:       tenantID,
		StationID:      stationID,
		StatementMonth: monthStart,
		Category:       category,
		Status:         settlement.StatementStatusDraft,
		Version:        version,
		TotalEnergyKWh: totals.TotalEnergyKWh,
		TotalAmount:    totals.TotalAmount,
		Currency:       currency,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repo.CreateWithItems(ctx, stmt, items); err != nil {
		result = metrics.ResultError
		return nil, err
	}
	return stmt, nil
}

// Freeze freezes a statement and computes snapshot hash.
func (s *StatementService) Freeze(ctx context.Context, id string) (*settlement.StatementAggregate, error) {
	start := time.Now()
	result := metrics.ResultSuccess
	defer func() {
		metrics.ObserveStatementFreeze(result, time.Since(start))
	}()

	stmt, err := s.repo.GetByID(ctx, id)
	if err != nil {
		result = metrics.ResultError
		return nil, err
	}
	if stmt == nil {
		result = metrics.ResultError
		return nil, errors.New("statement service: not found")
	}
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID == "" {
		tenantID = s.tenantID
	}
	if tenantID != "" && stmt.TenantID != tenantID {
		result = metrics.ResultError
		return nil, auth.ErrTenantMismatch
	}
	if stmt.Status == settlement.StatementStatusFrozen {
		return stmt, nil
	}
	if stmt.Status == settlement.StatementStatusVoided {
		result = metrics.ResultError
		return nil, errors.New("statement service: statement is voided")
	}

	items, err := s.repo.ListItems(ctx, id)
	if err != nil {
		result = metrics.ResultError
		return nil, err
	}
	hash, err := computeSnapshotHash(stmt, items)
	if err != nil {
		result = metrics.ResultError
		return nil, err
	}
	now := time.Now().UTC()
	if err := s.repo.MarkFrozen(ctx, id, hash, now); err != nil {
		result = metrics.ResultError
		return nil, err
	}
	stmt.Status = settlement.StatementStatusFrozen
	stmt.SnapshotHash = hash
	stmt.FrozenAt = now
	stmt.UpdatedAt = now
	return stmt, nil
}

// Void voids a statement.
func (s *StatementService) Void(ctx context.Context, id, reason string) (*settlement.StatementAggregate, error) {
	stmt, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if stmt == nil {
		return nil, errors.New("statement service: not found")
	}
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID == "" {
		tenantID = s.tenantID
	}
	if tenantID != "" && stmt.TenantID != tenantID {
		return nil, auth.ErrTenantMismatch
	}
	if stmt.Status == settlement.StatementStatusVoided {
		return stmt, nil
	}
	now := time.Now().UTC()
	if err := s.repo.MarkVoided(ctx, id, reason, now); err != nil {
		return nil, err
	}
	stmt.Status = settlement.StatementStatusVoided
	stmt.VoidReason = reason
	stmt.VoidedAt = now
	stmt.UpdatedAt = now
	return stmt, nil
}

// Get returns a statement with items.
func (s *StatementService) Get(ctx context.Context, id string) (*settlement.StatementAggregate, []settlement.StatementItem, error) {
	stmt, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if stmt == nil {
		return nil, nil, errors.New("statement service: not found")
	}
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID == "" {
		tenantID = s.tenantID
	}
	if tenantID != "" && stmt.TenantID != tenantID {
		return nil, nil, auth.ErrTenantMismatch
	}
	items, err := s.repo.ListItems(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return stmt, items, nil
}

// List returns statements for a station month/category.
func (s *StatementService) List(ctx context.Context, stationID, month, category string) ([]settlement.StatementAggregate, error) {
	if stationID == "" {
		return nil, errors.New("statement service: station_id required")
	}
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID == "" {
		tenantID = s.tenantID
	}
	monthStart, err := parseMonth(month)
	if err != nil {
		return nil, err
	}
	if category == "" {
		category = "owner"
	}
	return s.repo.ListByStationMonthCategory(ctx, tenantID, stationID, monthStart, category)
}

func parseMonth(month string) (time.Time, error) {
	if month == "" {
		return time.Time{}, errors.New("statement service: month required")
	}
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return time.Time{}, errors.New("statement service: month must be YYYY-MM")
	}
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC), nil
}

type totals struct {
	TotalEnergyKWh float64
	TotalAmount    float64
}

func computeSnapshotHash(stmt *settlement.StatementAggregate, items []settlement.StatementItem) (string, error) {
	if stmt == nil {
		return "", errors.New("statement service: nil statement")
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].DayStart.Before(items[j].DayStart)
	})
	payload := struct {
		Statement *settlement.StatementAggregate `json:"statement"`
		Items     []settlement.StatementItem     `json:"items"`
	}{
		Statement: stmt,
		Items:     items,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func buildStatementID(stationID string, month time.Time, category string, version int) string {
	base := stationID + "|" + month.Format("2006-01") + "|" + category + "|" + strconv.Itoa(version)
	hash := sha256.Sum256([]byte(base))
	return "stmt-" + hex.EncodeToString(hash[:8])
}
