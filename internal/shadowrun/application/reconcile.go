package application

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

const timeLayout = time.RFC3339

type hourStat struct {
	SubjectID       string
	TimeType        string
	TimeKey         string
	PeriodStart     time.Time
	StatisticID     string
	IsCompleted     bool
	ChargeKWh       float64
	DischargeKWh    float64
	Earnings        float64
	CarbonReduction float64
	CreatedAt       time.Time
	UpdatedAt       time.Time
	EnergyKWh       float64
	Amount          float64
	TariffPlanID    string
	TariffMode      string
	TariffRuleID    string
	RuleStartMinute int
	RuleEndMinute   int
	PricePerKWh     float64
}

type dayStat struct {
	SubjectID       string
	TimeType        string
	TimeKey         string
	PeriodStart     time.Time
	StatisticID     string
	IsCompleted     bool
	ChargeKWh       float64
	DischargeKWh    float64
	Earnings        float64
	CarbonReduction float64
	CreatedAt       time.Time
	UpdatedAt       time.Time
	EnergyKWh       float64
}

type settlementRow struct {
	TenantID  string
	StationID string
	DayStart  time.Time
	EnergyKWh float64
	Amount    float64
	Currency  string
	Status    string
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type statementSummary struct {
	ID             string
	TenantID       string
	StationID      string
	StatementMonth time.Time
	Category       string
	Status         string
	Version        int
	TotalEnergyKWh float64
	TotalAmount    float64
	Currency       string
	SnapshotHash   string
	VoidReason     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	FrozenAt       *time.Time
	VoidedAt       *time.Time
}

type tariffPlan struct {
	ID       string
	Mode     string
	Currency string
}

type tariffRule struct {
	ID          string
	StartMinute int
	EndMinute   int
	PricePerKWh float64
}

type reconcileResult struct {
	Hours       []hourStat
	Days        []dayStat
	Settlements []settlementRow
	Statements  []statementSummary
}

func reconcile(ctx context.Context, db *sql.DB, tenantID, stationID string, monthStart, monthEnd time.Time, fallbackPrice float64) (reconcileResult, *tariffPlan, []tariffRule, error) {
	plan, rules, err := loadTariff(ctx, db, tenantID, stationID, monthStart)
	if err != nil {
		if fallbackPrice <= 0 {
			return reconcileResult{}, nil, nil, err
		}
		plan = &tariffPlan{ID: "fixed", Mode: "fixed", Currency: "CNY"}
		rules = []tariffRule{{ID: "fixed", StartMinute: 0, EndMinute: 1440, PricePerKWh: fallbackPrice}}
	}

	hours, err := loadHourStats(ctx, db, stationID, monthStart, monthEnd, plan, rules)
	if err != nil {
		return reconcileResult{}, nil, nil, err
	}
	days, err := loadDayStats(ctx, db, stationID, monthStart, monthEnd)
	if err != nil {
		return reconcileResult{}, nil, nil, err
	}
	settlements, err := loadSettlements(ctx, db, tenantID, stationID, monthStart, monthEnd)
	if err != nil {
		return reconcileResult{}, nil, nil, err
	}
	statements, err := loadStatements(ctx, db, tenantID, stationID, monthStart)
	if err != nil {
		return reconcileResult{}, nil, nil, err
	}

	return reconcileResult{
		Hours:       hours,
		Days:        days,
		Settlements: settlements,
		Statements:  statements,
	}, plan, rules, nil
}

func loadTariff(ctx context.Context, db *sql.DB, tenantID, stationID string, month time.Time) (*tariffPlan, []tariffRule, error) {
	var plan tariffPlan
	err := db.QueryRowContext(ctx, `
SELECT id, mode, currency
FROM tariff_plans
WHERE tenant_id = $1 AND station_id = $2 AND effective_month = $3
LIMIT 1`, tenantID, stationID, month).Scan(&plan.ID, &plan.Mode, &plan.Currency)
	if err != nil {
		return nil, nil, err
	}

	rows, err := db.QueryContext(ctx, `
SELECT id, start_minute, end_minute, price_per_kwh
FROM tariff_rules
WHERE plan_id = $1
ORDER BY start_minute ASC`, plan.ID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var rules []tariffRule
	for rows.Next() {
		var r tariffRule
		if err := rows.Scan(&r.ID, &r.StartMinute, &r.EndMinute, &r.PricePerKWh); err != nil {
			return nil, nil, err
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return &plan, rules, nil
}

func matchRule(rules []tariffRule, minute int) (tariffRule, bool) {
	for _, rule := range rules {
		if rule.StartMinute <= minute && rule.EndMinute > minute {
			return rule, true
		}
	}
	return tariffRule{}, false
}

func loadHourStats(ctx context.Context, db *sql.DB, stationID string, from, to time.Time, plan *tariffPlan, rules []tariffRule) ([]hourStat, error) {
	rows, err := db.QueryContext(ctx, `
SELECT
	subject_id,
	time_type,
	time_key,
	period_start,
	statistic_id,
	is_completed,
	charge_kwh,
	discharge_kwh,
	earnings,
	carbon_reduction,
	created_at,
	updated_at
FROM analytics_statistics
WHERE subject_id = $1
	AND time_type = 'HOUR'
	AND period_start >= $2
	AND period_start < $3
ORDER BY period_start ASC`, stationID, from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []hourStat
	for rows.Next() {
		var row hourStat
		if err := rows.Scan(
			&row.SubjectID,
			&row.TimeType,
			&row.TimeKey,
			&row.PeriodStart,
			&row.StatisticID,
			&row.IsCompleted,
			&row.ChargeKWh,
			&row.DischargeKWh,
			&row.Earnings,
			&row.CarbonReduction,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		row.PeriodStart = row.PeriodStart.UTC()
		row.CreatedAt = row.CreatedAt.UTC()
		row.UpdatedAt = row.UpdatedAt.UTC()
		row.EnergyKWh = row.ChargeKWh + row.DischargeKWh
		row.RuleStartMinute = -1
		row.RuleEndMinute = -1

		if plan != nil {
			row.TariffPlanID = plan.ID
			row.TariffMode = plan.Mode
			minute := row.PeriodStart.Hour() * 60
			if rule, ok := matchRule(rules, minute); ok {
				row.TariffRuleID = rule.ID
				row.RuleStartMinute = rule.StartMinute
				row.RuleEndMinute = rule.EndMinute
				row.PricePerKWh = rule.PricePerKWh
				row.Amount = row.EnergyKWh * rule.PricePerKWh
			}
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func loadDayStats(ctx context.Context, db *sql.DB, stationID string, from, to time.Time) ([]dayStat, error) {
	rows, err := db.QueryContext(ctx, `
SELECT
	subject_id,
	time_type,
	time_key,
	period_start,
	statistic_id,
	is_completed,
	charge_kwh,
	discharge_kwh,
	earnings,
	carbon_reduction,
	created_at,
	updated_at
FROM analytics_statistics
WHERE subject_id = $1
	AND time_type = 'DAY'
	AND period_start >= $2
	AND period_start < $3
ORDER BY period_start ASC`, stationID, from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []dayStat
	for rows.Next() {
		var row dayStat
		if err := rows.Scan(
			&row.SubjectID,
			&row.TimeType,
			&row.TimeKey,
			&row.PeriodStart,
			&row.StatisticID,
			&row.IsCompleted,
			&row.ChargeKWh,
			&row.DischargeKWh,
			&row.Earnings,
			&row.CarbonReduction,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		row.PeriodStart = row.PeriodStart.UTC()
		row.CreatedAt = row.CreatedAt.UTC()
		row.UpdatedAt = row.UpdatedAt.UTC()
		row.EnergyKWh = row.ChargeKWh + row.DischargeKWh
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func loadSettlements(ctx context.Context, db *sql.DB, tenantID, stationID string, from, to time.Time) ([]settlementRow, error) {
	rows, err := db.QueryContext(ctx, `
SELECT
	tenant_id,
	station_id,
	day_start,
	energy_kwh,
	amount,
	currency,
	status,
	version,
	created_at,
	updated_at
FROM settlements_day
WHERE tenant_id = $1
	AND station_id = $2
	AND day_start >= $3
	AND day_start < $4
ORDER BY day_start ASC`, tenantID, stationID, from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []settlementRow
	for rows.Next() {
		var row settlementRow
		if err := rows.Scan(
			&row.TenantID,
			&row.StationID,
			&row.DayStart,
			&row.EnergyKWh,
			&row.Amount,
			&row.Currency,
			&row.Status,
			&row.Version,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		row.DayStart = row.DayStart.UTC()
		row.CreatedAt = row.CreatedAt.UTC()
		row.UpdatedAt = row.UpdatedAt.UTC()
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func loadStatements(ctx context.Context, db *sql.DB, tenantID, stationID string, month time.Time) ([]statementSummary, error) {
	rows, err := db.QueryContext(ctx, `
SELECT
	id,
	tenant_id,
	station_id,
	statement_month,
	category,
	status,
	version,
	total_energy_kwh,
	total_amount,
	currency,
	snapshot_hash,
	void_reason,
	created_at,
	updated_at,
	frozen_at,
	voided_at
FROM settlement_statements
WHERE tenant_id = $1 AND station_id = $2 AND statement_month = $3
ORDER BY version ASC`, tenantID, stationID, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []statementSummary
	for rows.Next() {
		var row statementSummary
		var frozenAt sql.NullTime
		var voidedAt sql.NullTime
		var snapshot sql.NullString
		var voidReason sql.NullString
		if err := rows.Scan(
			&row.ID,
			&row.TenantID,
			&row.StationID,
			&row.StatementMonth,
			&row.Category,
			&row.Status,
			&row.Version,
			&row.TotalEnergyKWh,
			&row.TotalAmount,
			&row.Currency,
			&snapshot,
			&voidReason,
			&row.CreatedAt,
			&row.UpdatedAt,
			&frozenAt,
			&voidedAt,
		); err != nil {
			return nil, err
		}
		row.StatementMonth = row.StatementMonth.UTC()
		row.CreatedAt = row.CreatedAt.UTC()
		row.UpdatedAt = row.UpdatedAt.UTC()
		if snapshot.Valid {
			row.SnapshotHash = snapshot.String
		}
		if voidReason.Valid {
			row.VoidReason = voidReason.String
		}
		if frozenAt.Valid {
			t := frozenAt.Time.UTC()
			row.FrozenAt = &t
		}
		if voidedAt.Valid {
			t := voidedAt.Time.UTC()
			row.VoidedAt = &t
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func writeReports(outDir string, result reconcileResult) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if err := writeHourStats(outDir, result.Hours); err != nil {
		return err
	}
	if err := writeDayStats(outDir, result.Days); err != nil {
		return err
	}
	if err := writeSettlements(outDir, result.Settlements); err != nil {
		return err
	}
	if err := writeStatementSummary(outDir, result.Statements); err != nil {
		return err
	}
	return nil
}

func writeArchive(outDir string) (string, error) {
	archivePath := filepath.Join(outDir, "report.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	entries := []string{
		"hour_stats.csv",
		"day_stats.csv",
		"settlements_day.csv",
		"statement_summary.csv",
		"diff_summary.json",
	}

	for _, name := range entries {
		path := filepath.Join(outDir, name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		fw, err := zipWriter.Create(name)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if _, err := fw.Write(data); err != nil {
			return "", err
		}
	}
	return archivePath, nil
}

func writeHourStats(outDir string, rows []hourStat) error {
	path := filepath.Join(outDir, "hour_stats.csv")
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{
		"subject_id",
		"time_type",
		"time_key",
		"period_start",
		"statistic_id",
		"is_completed",
		"charge_kwh",
		"discharge_kwh",
		"energy_kwh",
		"earnings",
		"carbon_reduction",
		"tariff_plan_id",
		"tariff_mode",
		"tariff_rule_id",
		"rule_start_minute",
		"rule_end_minute",
		"price_per_kwh",
		"amount",
		"created_at",
		"updated_at",
	}); err != nil {
		return err
	}

	for _, row := range rows {
		if err := writer.Write([]string{
			row.SubjectID,
			row.TimeType,
			row.TimeKey,
			formatTime(row.PeriodStart),
			row.StatisticID,
			formatBool(row.IsCompleted),
			formatFloat(row.ChargeKWh),
			formatFloat(row.DischargeKWh),
			formatFloat(row.EnergyKWh),
			formatFloat(row.Earnings),
			formatFloat(row.CarbonReduction),
			row.TariffPlanID,
			row.TariffMode,
			row.TariffRuleID,
			formatOptionalInt(row.RuleStartMinute),
			formatOptionalInt(row.RuleEndMinute),
			formatFloat(row.PricePerKWh),
			formatFloat(row.Amount),
			formatTime(row.CreatedAt),
			formatTime(row.UpdatedAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

func writeDayStats(outDir string, rows []dayStat) error {
	path := filepath.Join(outDir, "day_stats.csv")
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{
		"subject_id",
		"time_type",
		"time_key",
		"period_start",
		"statistic_id",
		"is_completed",
		"charge_kwh",
		"discharge_kwh",
		"energy_kwh",
		"earnings",
		"carbon_reduction",
		"created_at",
		"updated_at",
	}); err != nil {
		return err
	}

	for _, row := range rows {
		if err := writer.Write([]string{
			row.SubjectID,
			row.TimeType,
			row.TimeKey,
			formatTime(row.PeriodStart),
			row.StatisticID,
			formatBool(row.IsCompleted),
			formatFloat(row.ChargeKWh),
			formatFloat(row.DischargeKWh),
			formatFloat(row.EnergyKWh),
			formatFloat(row.Earnings),
			formatFloat(row.CarbonReduction),
			formatTime(row.CreatedAt),
			formatTime(row.UpdatedAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

func writeSettlements(outDir string, rows []settlementRow) error {
	path := filepath.Join(outDir, "settlements_day.csv")
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{
		"tenant_id",
		"station_id",
		"day_start",
		"energy_kwh",
		"amount",
		"currency",
		"status",
		"version",
		"created_at",
		"updated_at",
	}); err != nil {
		return err
	}

	for _, row := range rows {
		if err := writer.Write([]string{
			row.TenantID,
			row.StationID,
			formatTime(row.DayStart),
			formatFloat(row.EnergyKWh),
			formatFloat(row.Amount),
			row.Currency,
			row.Status,
			strconv.Itoa(row.Version),
			formatTime(row.CreatedAt),
			formatTime(row.UpdatedAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

func writeStatementSummary(outDir string, rows []statementSummary) error {
	path := filepath.Join(outDir, "statement_summary.csv")
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{
		"id",
		"tenant_id",
		"station_id",
		"statement_month",
		"category",
		"status",
		"version",
		"total_energy_kwh",
		"total_amount",
		"currency",
		"snapshot_hash",
		"void_reason",
		"created_at",
		"updated_at",
		"frozen_at",
		"voided_at",
	}); err != nil {
		return err
	}

	for _, row := range rows {
		if err := writer.Write([]string{
			row.ID,
			row.TenantID,
			row.StationID,
			formatDate(row.StatementMonth),
			row.Category,
			row.Status,
			strconv.Itoa(row.Version),
			formatFloat(row.TotalEnergyKWh),
			formatFloat(row.TotalAmount),
			row.Currency,
			row.SnapshotHash,
			row.VoidReason,
			formatTime(row.CreatedAt),
			formatTime(row.UpdatedAt),
			formatOptionalTime(row.FrozenAt),
			formatOptionalTime(row.VoidedAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

type diffDay struct {
	DayStart     time.Time `json:"day_start"`
	EnergyHour   float64   `json:"energy_hour"`
	EnergySettle float64   `json:"energy_settlement"`
	EnergyDiff   float64   `json:"energy_diff"`
	AmountHour   float64   `json:"amount_hour"`
	AmountSettle float64   `json:"amount_settlement"`
	AmountDiff   float64   `json:"amount_diff"`
	MissingHours int       `json:"missing_hours"`
}

type diffSummary struct {
	Month             string     `json:"month"`
	StationID         string     `json:"station_id"`
	DiffEnergyMax     float64    `json:"diff_energy_max"`
	DiffAmountMax     float64    `json:"diff_amount_max"`
	MissingHoursTotal int        `json:"missing_hours_total"`
	LateDataCount     int        `json:"late_data_count"`
	GeneratedAt       string     `json:"generated_at"`
	DayDiffs          []diffDay  `json:"day_diffs"`
	Thresholds        Thresholds `json:"thresholds"`
}

func buildDiffSummary(result reconcileResult, monthStart, monthEnd, jobDate time.Time, thresholds Thresholds) (diffSummary, error) {
	hourByDay := make(map[time.Time][]hourStat)
	for _, row := range result.Hours {
		day := time.Date(row.PeriodStart.Year(), row.PeriodStart.Month(), row.PeriodStart.Day(), 0, 0, 0, 0, time.UTC)
		hourByDay[day] = append(hourByDay[day], row)
	}
	settlementByDay := make(map[time.Time]settlementRow)
	for _, row := range result.Settlements {
		day := time.Date(row.DayStart.Year(), row.DayStart.Month(), row.DayStart.Day(), 0, 0, 0, 0, time.UTC)
		settlementByDay[day] = row
	}

	endDate := monthEnd
	if jobDate.Before(monthEnd) && jobDate.After(monthStart) {
		endDate = time.Date(jobDate.Year(), jobDate.Month(), jobDate.Day(), 0, 0, 0, 0, time.UTC)
	}

	var diffs []diffDay
	var maxEnergy float64
	var maxAmount float64
	var missingTotal int

	for day := monthStart; day.Before(endDate); day = day.AddDate(0, 0, 1) {
		hours := hourByDay[day]
		settle := settlementByDay[day]
		var energyHour float64
		var amountHour float64
		for _, hr := range hours {
			energyHour += hr.EnergyKWh
			amountHour += hr.Amount
		}
		energyDiff := energyHour - settle.EnergyKWh
		amountDiff := amountHour - settle.Amount

		missing := 24 - len(hours)
		if missing < 0 {
			missing = 0
		}
		missingTotal += missing

		if abs(energyDiff) > maxEnergy {
			maxEnergy = abs(energyDiff)
		}
		if abs(amountDiff) > maxAmount {
			maxAmount = abs(amountDiff)
		}

		diffs = append(diffs, diffDay{
			DayStart:     day,
			EnergyHour:   energyHour,
			EnergySettle: settle.EnergyKWh,
			EnergyDiff:   energyDiff,
			AmountHour:   amountHour,
			AmountSettle: settle.Amount,
			AmountDiff:   amountDiff,
			MissingHours: missing,
		})
	}

	sort.Slice(diffs, func(i, j int) bool { return diffs[i].DayStart.Before(diffs[j].DayStart) })

	return diffSummary{
		Month:             monthStart.Format("2006-01"),
		StationID:         result.SettlementsStationID(),
		DiffEnergyMax:     maxEnergy,
		DiffAmountMax:     maxAmount,
		MissingHoursTotal: missingTotal,
		LateDataCount:     0,
		GeneratedAt:       time.Now().UTC().Format(timeLayout),
		DayDiffs:          diffs,
		Thresholds:        thresholds,
	}, nil
}

func (r reconcileResult) SettlementsStationID() string {
	if len(r.Settlements) > 0 {
		return r.Settlements[0].StationID
	}
	if len(r.Days) > 0 {
		return r.Days[0].SubjectID
	}
	if len(r.Hours) > 0 {
		return r.Hours[0].SubjectID
	}
	return ""
}

func writeSummaryJSON(outDir string, summary diffSummary) error {
	path := filepath.Join(outDir, "diff_summary.json")
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(summary)
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(timeLayout)
}

func formatDate(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02")
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(timeLayout)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func formatOptionalInt(value int) string {
	if value < 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func formatBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func validateMonth(monthStart, monthEnd time.Time) error {
	if monthStart.IsZero() || monthEnd.IsZero() {
		return errors.New("invalid month range")
	}
	if !monthEnd.After(monthStart) {
		return errors.New("invalid month range")
	}
	return nil
}
