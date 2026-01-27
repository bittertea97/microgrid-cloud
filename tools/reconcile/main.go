package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const timeLayout = time.RFC3339

type config struct {
	dbURL          string
	tenantID       string
	stationID      string
	month          string
	outDir         string
	legacyHourPath string
	pricePerKWh    float64
}

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
	ID           string
	StartMinute  int
	EndMinute    int
	PricePerKWh  float64
}

type legacyHour struct {
	HourStart time.Time
	EnergyKWh float64
	Amount    float64
}

func main() {
	cfg, err := parseFlags()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	if err := os.MkdirAll(cfg.outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "create out dir:", err)
		os.Exit(2)
	}

	db, err := sql.Open("pgx", cfg.dbURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db open:", err)
		os.Exit(2)
	}
	defer db.Close()

	ctx := context.Background()
	monthStart, monthEnd, err := parseMonth(cfg.month)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	plan, rules, err := loadTariff(ctx, db, cfg.tenantID, cfg.stationID, monthStart)
	if err != nil {
		if cfg.pricePerKWh <= 0 {
			fmt.Fprintln(os.Stderr, "tariff:", err)
			os.Exit(2)
		}
		plan = &tariffPlan{ID: "fixed", Mode: "fixed", Currency: "CNY"}
		rules = []tariffRule{{
			ID:          "fixed",
			StartMinute: 0,
			EndMinute:   1440,
			PricePerKWh: cfg.pricePerKWh,
		}}
	}

	hours, err := loadHourStats(ctx, db, cfg.stationID, monthStart, monthEnd, plan, rules)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load hour stats:", err)
		os.Exit(2)
	}

	days, err := loadDayStats(ctx, db, cfg.stationID, monthStart, monthEnd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load day stats:", err)
		os.Exit(2)
	}

	settlements, err := loadSettlements(ctx, db, cfg.tenantID, cfg.stationID, monthStart, monthEnd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load settlements:", err)
		os.Exit(2)
	}

	statements, err := loadStatements(ctx, db, cfg.tenantID, cfg.stationID, monthStart)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load statements:", err)
		os.Exit(2)
	}

	if err := writeHourStats(cfg.outDir, hours); err != nil {
		fmt.Fprintln(os.Stderr, "write hour stats:", err)
		os.Exit(2)
	}
	if err := writeDayStats(cfg.outDir, days); err != nil {
		fmt.Fprintln(os.Stderr, "write day stats:", err)
		os.Exit(2)
	}
	if err := writeSettlements(cfg.outDir, settlements); err != nil {
		fmt.Fprintln(os.Stderr, "write settlements:", err)
		os.Exit(2)
	}
	if err := writeStatementSummary(cfg.outDir, statements); err != nil {
		fmt.Fprintln(os.Stderr, "write statement summary:", err)
		os.Exit(2)
	}

	if cfg.legacyHourPath != "" {
		semantics, _ := loadSemantics(ctx, db, cfg.stationID)
		legacyRows, err := loadLegacyHours(cfg.legacyHourPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "load legacy hours:", err)
			os.Exit(2)
		}
		if err := writeDiffReport(cfg.outDir, hours, legacyRows, semantics); err != nil {
			fmt.Fprintln(os.Stderr, "write diff report:", err)
			os.Exit(2)
		}
	}

	fmt.Printf("Reconciliation outputs written to %s\n", cfg.outDir)
}

func parseFlags() (config, error) {
	var cfg config
	flag.StringVar(&cfg.dbURL, "db", getenvDefault("DATABASE_URL", getenvDefault("PG_DSN", "")), "Postgres DSN")
	flag.StringVar(&cfg.tenantID, "tenant", getenvDefault("TENANT_ID", ""), "tenant id")
	flag.StringVar(&cfg.stationID, "station", "", "station id")
	flag.StringVar(&cfg.month, "month", "", "month in YYYY-MM")
	flag.StringVar(&cfg.outDir, "out", "./out", "output directory")
	flag.StringVar(&cfg.legacyHourPath, "legacy-hour-csv", "", "legacy hour CSV path (optional)")
	flag.Float64Var(&cfg.pricePerKWh, "price-per-kwh", getenvFloatDefault("PRICE_PER_KWH", 0), "fallback fixed price per kWh when no tariff plan")
	flag.Parse()

	if cfg.dbURL == "" {
		return cfg, errors.New("missing --db or DATABASE_URL/PG_DSN")
	}
	if cfg.tenantID == "" {
		return cfg, errors.New("missing --tenant or TENANT_ID")
	}
	if cfg.stationID == "" {
		return cfg, errors.New("missing --station")
	}
	if cfg.month == "" {
		return cfg, errors.New("missing --month (YYYY-MM)")
	}
	return cfg, nil
}

func getenvDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getenvFloatDefault(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseMonth(value string) (time.Time, time.Time, error) {
	t, err := time.Parse("2006-01", value)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("month must be YYYY-MM")
	}
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	return start, end, nil
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
			formatInt(row.Version),
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
			formatInt(row.Version),
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

func loadSemantics(ctx context.Context, db *sql.DB, stationID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT semantic
FROM point_mappings
WHERE station_id = $1
ORDER BY semantic ASC`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var semantics []string
	for rows.Next() {
		var semantic string
		if err := rows.Scan(&semantic); err != nil {
			return nil, err
		}
		if semantic != "" {
			semantics = append(semantics, semantic)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return semantics, nil
}

func loadLegacyHours(path string) ([]legacyHour, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 1 {
		return nil, errors.New("legacy csv: empty")
	}
	header := make(map[string]int)
	for i, name := range records[0] {
		header[strings.ToLower(strings.TrimSpace(name))] = i
	}
	timeIdx := findHeader(header, "hour_start", "period_start", "time", "datetime", "ts")
	energyIdx := findHeader(header, "energy_kwh", "energy", "kwh")
	amountIdx := findHeader(header, "amount", "total_amount")
	if timeIdx < 0 || energyIdx < 0 || amountIdx < 0 {
		return nil, errors.New("legacy csv requires headers: hour_start, energy_kwh, amount")
	}

	var result []legacyHour
	for _, row := range records[1:] {
		if timeIdx >= len(row) {
			continue
		}
		ts, err := parseLegacyTime(row[timeIdx])
		if err != nil {
			return nil, err
		}
		energy, err := parseFloat(row[energyIdx])
		if err != nil {
			return nil, err
		}
		amount, err := parseFloat(row[amountIdx])
		if err != nil {
			return nil, err
		}
		result = append(result, legacyHour{
			HourStart: ts.UTC(),
			EnergyKWh: energy,
			Amount:    amount,
		})
	}
	return result, nil
}

func writeDiffReport(outDir string, local []hourStat, legacy []legacyHour, semantics []string) error {
	path := filepath.Join(outDir, "diff_report.csv")
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{
		"day_start",
		"hour_start",
		"energy_kwh_local",
		"energy_kwh_legacy",
		"energy_diff",
		"amount_local",
		"amount_legacy",
		"amount_diff",
		"tariff_rule_id",
		"rule_start_minute",
		"rule_end_minute",
		"price_per_kwh",
		"semantics",
	}); err != nil {
		return err
	}

	localMap := make(map[time.Time]hourStat)
	for _, row := range local {
		localMap[row.PeriodStart] = row
	}
	legacyMap := make(map[time.Time]legacyHour)
	for _, row := range legacy {
		legacyMap[row.HourStart] = row
	}

	var keys []time.Time
	for k := range localMap {
		keys = append(keys, k)
	}
	for k := range legacyMap {
		if _, ok := localMap[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })

	semanticList := strings.Join(semantics, "|")
	for _, hourStart := range keys {
		localRow, hasLocal := localMap[hourStart]
		legacyRow, hasLegacy := legacyMap[hourStart]
		var energyLocal, energyLegacy, amountLocal, amountLegacy float64
		if hasLocal {
			energyLocal = localRow.EnergyKWh
			amountLocal = localRow.Amount
		}
		if hasLegacy {
			energyLegacy = legacyRow.EnergyKWh
			amountLegacy = legacyRow.Amount
		}
		energyDiff := energyLocal - energyLegacy
		amountDiff := amountLocal - amountLegacy
		dayStart := time.Date(hourStart.Year(), hourStart.Month(), hourStart.Day(), 0, 0, 0, 0, time.UTC)
		if err := writer.Write([]string{
			formatTime(dayStart),
			formatTime(hourStart),
			formatFloat(energyLocal),
			formatFloat(energyLegacy),
			formatFloat(energyDiff),
			formatFloat(amountLocal),
			formatFloat(amountLegacy),
			formatFloat(amountDiff),
			localRow.TariffRuleID,
			formatOptionalInt(localRow.RuleStartMinute),
			formatOptionalInt(localRow.RuleEndMinute),
			formatFloat(localRow.PricePerKWh),
			semanticList,
		}); err != nil {
			return err
		}
	}
	return nil
}

func findHeader(headers map[string]int, names ...string) int {
	for _, name := range names {
		if idx, ok := headers[strings.ToLower(name)]; ok {
			return idx
		}
	}
	return -1
}

func parseLegacyTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("legacy csv: empty time")
	}
	if epoch, err := strconv.ParseInt(value, 10, 64); err == nil {
		if epoch > 1_000_000_000_000 {
			return time.UnixMilli(epoch).UTC(), nil
		}
		return time.Unix(epoch, 0).UTC(), nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("legacy csv: unsupported time format %q", value)
}

func parseFloat(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return strconv.ParseFloat(value, 64)
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

func formatInt(value int) string {
	return strconv.Itoa(value)
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
