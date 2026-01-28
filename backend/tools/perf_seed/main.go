package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	timeKeyHourLayout = "20060102T15"
	timeKeyDayLayout  = "20060102"
)

type config struct {
	dsn                string
	baseURL            string
	tenantID           string
	stationPrefix      string
	stationCount       int
	startDate          string
	days               int
	seedHourly         bool
	seedDaily          bool
	seedSettlements    bool
	generateStatements bool
	statementMonth     string
	statementCategory  string
	statementIDsOut    string
}

func main() {
	cfg := parseConfig()
	if cfg.dsn == "" {
		log.Fatal("PG_DSN or DATABASE_URL is required")
	}
	if cfg.stationCount <= 0 {
		log.Fatal("station-count must be > 0")
	}
	if cfg.days <= 0 {
		log.Fatal("days must be > 0")
	}

	start, err := parseStartDate(cfg.startDate)
	if err != nil {
		log.Fatalf("invalid start-date: %v", err)
	}
	if cfg.statementMonth == "" {
		cfg.statementMonth = start.Format("2006-01")
	}

	stationIDs := buildStationIDs(cfg.stationPrefix, cfg.stationCount)

	db, err := sql.Open("pgx", cfg.dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	if cfg.seedHourly || cfg.seedDaily {
		log.Printf("seeding analytics_statistics: stations=%d days=%d hourly=%v daily=%v", cfg.stationCount, cfg.days, cfg.seedHourly, cfg.seedDaily)
		if err := seedAnalytics(ctx, db, stationIDs, start, cfg.days, cfg.seedHourly, cfg.seedDaily); err != nil {
			log.Fatalf("seed analytics: %v", err)
		}
	}

	if cfg.seedSettlements {
		log.Printf("seeding settlements_day: stations=%d days=%d tenant=%s", cfg.stationCount, cfg.days, cfg.tenantID)
		if err := seedSettlements(ctx, db, stationIDs, cfg.tenantID, start, cfg.days); err != nil {
			log.Fatalf("seed settlements: %v", err)
		}
	}

	if cfg.generateStatements {
		if cfg.baseURL == "" {
			log.Fatal("base-url is required when generate-statements is enabled")
		}
		log.Printf("generating statements: month=%s category=%s stations=%d", cfg.statementMonth, cfg.statementCategory, cfg.stationCount)
		ids, err := generateStatements(ctx, cfg.baseURL, stationIDs, cfg.statementMonth, cfg.statementCategory)
		if err != nil {
			log.Fatalf("generate statements: %v", err)
		}
		if cfg.statementIDsOut != "" {
			if err := writeLines(cfg.statementIDsOut, ids); err != nil {
				log.Fatalf("write statement ids: %v", err)
			}
			log.Printf("statement ids written to %s", cfg.statementIDsOut)
		}
	}

	log.Printf("perf seed completed")
}

func parseConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.dsn, "pg-dsn", envOrDefault("PG_DSN", envOrDefault("DATABASE_URL", "")), "Postgres DSN")
	flag.StringVar(&cfg.baseURL, "base-url", envOrDefault("BASE_URL", ""), "API base URL for statement generation")
	flag.StringVar(&cfg.tenantID, "tenant-id", envOrDefault("TENANT_ID", "tenant-demo"), "tenant id used in settlements_day")
	flag.StringVar(&cfg.stationPrefix, "station-prefix", envOrDefault("STATION_PREFIX", "station-perf-"), "station id prefix")
	flag.IntVar(&cfg.stationCount, "station-count", envOrInt("STATION_COUNT", 10), "number of stations to seed")
	flag.StringVar(&cfg.startDate, "start-date", envOrDefault("START_DATE", ""), "start date (YYYY-MM-DD or RFC3339)")
	flag.IntVar(&cfg.days, "days", envOrInt("DAYS", 7), "number of days to seed")
	flag.BoolVar(&cfg.seedHourly, "seed-hourly", envOrBool("SEED_HOURLY", true), "seed hourly analytics statistics")
	flag.BoolVar(&cfg.seedDaily, "seed-daily", envOrBool("SEED_DAILY", true), "seed daily analytics statistics")
	flag.BoolVar(&cfg.seedSettlements, "seed-settlements", envOrBool("SEED_SETTLEMENTS", true), "seed settlements_day")
	flag.BoolVar(&cfg.generateStatements, "generate-statements", envOrBool("GENERATE_STATEMENTS", false), "generate statements via API")
	flag.StringVar(&cfg.statementMonth, "statement-month", envOrDefault("STATEMENT_MONTH", ""), "statement month (YYYY-MM)")
	flag.StringVar(&cfg.statementCategory, "statement-category", envOrDefault("STATEMENT_CATEGORY", "owner"), "statement category")
	flag.StringVar(&cfg.statementIDsOut, "statement-ids-out", envOrDefault("STATEMENT_IDS_OUT", ""), "output file for statement IDs")
	flag.Parse()
	return cfg
}

func parseStartDate(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Now().UTC().AddDate(0, 0, -7).Truncate(24 * time.Hour), nil
	}
	value = strings.TrimSpace(value)
	if strings.Contains(value, "T") {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return time.Time{}, err
		}
		return parsed.UTC(), nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func buildStationIDs(prefix string, count int) []string {
	list := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		list = append(list, fmt.Sprintf("%s%04d", prefix, i))
	}
	return list
}

func seedAnalytics(ctx context.Context, db *sql.DB, stations []string, start time.Time, days int, hourly bool, daily bool) error {
	const insertSQL = `
INSERT INTO analytics_statistics (
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
	carbon_reduction,
	created_at,
	updated_at
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13
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
	updated_at = EXCLUDED.updated_at`

	now := time.Now().UTC()
	for idx, stationID := range stations {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		stmt, err := tx.PrepareContext(ctx, insertSQL)
		if err != nil {
			_ = tx.Rollback()
			return err
		}

		base := float64((idx % 10) + 1)
		for day := 0; day < days; day++ {
			dayStart := start.AddDate(0, 0, day)
			if daily {
				charge := base*10 + float64(day+1)
				discharge := base*5 + float64(day%7)
				earnings := charge * 0.12
				carbon := charge * 0.02
				timeKey := dayStart.UTC().Format(timeKeyDayLayout)
				statID := fmt.Sprintf("stat-%s-D-%s", stationID, timeKey)
				if _, err := stmt.ExecContext(
					ctx,
					stationID,
					"DAY",
					timeKey,
					dayStart.UTC(),
					statID,
					true,
					dayStart.Add(24*time.Hour).UTC(),
					charge,
					discharge,
					earnings,
					carbon,
					now,
					now,
				); err != nil {
					_ = stmt.Close()
					_ = tx.Rollback()
					return err
				}
			}

			if hourly {
				for hour := 0; hour < 24; hour++ {
					periodStart := dayStart.Add(time.Duration(hour) * time.Hour).UTC()
					charge := base + float64(hour+1)
					discharge := base/2 + float64(hour%6)
					earnings := charge * 0.08
					carbon := charge * 0.01
					timeKey := periodStart.Format(timeKeyHourLayout)
					statID := fmt.Sprintf("stat-%s-H-%s", stationID, timeKey)
					if _, err := stmt.ExecContext(
						ctx,
						stationID,
						"HOUR",
						timeKey,
						periodStart,
						statID,
						true,
						periodStart.Add(time.Hour).UTC(),
						charge,
						discharge,
						earnings,
						carbon,
						now,
						now,
					); err != nil {
						_ = stmt.Close()
						_ = tx.Rollback()
						return err
					}
				}
			}
		}

		if err := stmt.Close(); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		log.Printf("seeded analytics station %s (%d/%d)", stationID, idx+1, len(stations))
	}
	return nil
}

func seedSettlements(ctx context.Context, db *sql.DB, stations []string, tenantID string, start time.Time, days int) error {
	const insertSQL = `
INSERT INTO settlements_day (
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
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9,$10
)
ON CONFLICT (tenant_id, station_id, day_start)
DO UPDATE SET
	energy_kwh = EXCLUDED.energy_kwh,
	amount = EXCLUDED.amount,
	currency = EXCLUDED.currency,
	status = EXCLUDED.status,
	version = EXCLUDED.version,
	updated_at = EXCLUDED.updated_at`

	now := time.Now().UTC()
	for idx, stationID := range stations {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		stmt, err := tx.PrepareContext(ctx, insertSQL)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		base := float64((idx % 10) + 1)
		for day := 0; day < days; day++ {
			dayStart := start.AddDate(0, 0, day).UTC()
			energy := base*50 + float64(day+1)
			amount := energy * 0.6
			if _, err := stmt.ExecContext(
				ctx,
				tenantID,
				stationID,
				dayStart,
				energy,
				amount,
				"CNY",
				"CALCULATED",
				1,
				now,
				now,
			); err != nil {
				_ = stmt.Close()
				_ = tx.Rollback()
				return err
			}
		}
		if err := stmt.Close(); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		log.Printf("seeded settlements station %s (%d/%d)", stationID, idx+1, len(stations))
	}
	return nil
}

func generateStatements(ctx context.Context, baseURL string, stations []string, month string, category string) ([]string, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("base url required")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	baseURL = strings.TrimRight(baseURL, "/")
	ids := make([]string, 0, len(stations))
	for _, stationID := range stations {
		body := map[string]any{
			"station_id": stationID,
			"month":      month,
			"category":   category,
			"regenerate": false,
		}
		payload, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/statements/generate", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		var respBody struct {
			StatementID string `json:"statement_id"`
		}
		if resp.StatusCode >= 300 {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("generate statement failed for %s: http %d", stationID, resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
			_ = resp.Body.Close()
			return nil, err
		}
		_ = resp.Body.Close()
		if respBody.StatementID == "" {
			return nil, fmt.Errorf("empty statement id for %s", stationID)
		}
		ids = append(ids, respBody.StatementID)
	}
	return ids, nil
}

func writeLines(path string, lines []string) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	content := strings.Join(lines, "\n")
	return os.WriteFile(path, []byte(content), 0o644)
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func envOrBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
