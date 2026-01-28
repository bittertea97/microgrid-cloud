package integration_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	shadowapp "microgrid-cloud/internal/shadowrun/application"
	shadowrepo "microgrid-cloud/internal/shadowrun/infrastructure/postgres"
	shadownotify "microgrid-cloud/internal/shadowrun/notify"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestShadowrun_ReportAndAlert(t *testing.T) {
	db := openDB(t)
	defer db.Close()

	if err := applyShadowMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	ctx := context.Background()
	cleanupShadowTables(ctx, db)

	webhook := newFakeWebhook()
	server := httptest.NewServer(webhook)
	defer server.Close()

	cfg := shadowapp.Config{
		Defaults: shadowapp.Thresholds{
			EnergyAbs:    5,
			AmountAbs:    5,
			MissingHours: 2,
		},
		StorageRoot:   t.TempDir(),
		WebhookURL:    server.URL,
		PublicBaseURL: "http://localhost:8080",
		FallbackPrice: 1.0,
	}
	repo := shadowrepo.NewRepository(db)
	notifier := shadownotify.NewWebhookNotifier(server.URL)
	runner := shadowapp.NewRunner(repo, db, cfg, notifier, nil, nil)

	month := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	jobDate := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)

	// OK station
	if err := seedHourAndSettlement(ctx, db, "tenant-shadow", "station-ok", month.AddDate(0, 0, 1), 24, 1, 24); err != nil {
		t.Fatalf("seed ok: %v", err)
	}
	if _, err := runner.Run(ctx, "tenant-shadow", "station-ok", month, jobDate, nil); err != nil {
		t.Fatalf("run ok: %v", err)
	}

	// Diff station (settlement amount mismatch)
	if err := seedHourAndSettlement(ctx, db, "tenant-shadow", "station-diff", month.AddDate(0, 0, 2), 24, 1, 100); err != nil {
		t.Fatalf("seed diff: %v", err)
	}
	if _, err := runner.Run(ctx, "tenant-shadow", "station-diff", month, jobDate, nil); err != nil {
		t.Fatalf("run diff: %v", err)
	}

	// Missing hours station
	if err := seedHourAndSettlement(ctx, db, "tenant-shadow", "station-miss", month.AddDate(0, 0, 3), 10, 1, 10); err != nil {
		t.Fatalf("seed miss: %v", err)
	}
	if _, err := runner.Run(ctx, "tenant-shadow", "station-miss", month, jobDate, nil); err != nil {
		t.Fatalf("run miss: %v", err)
	}

	// alerts should be raised for diff and missing
	if webhook.count() < 2 {
		t.Fatalf("expected webhook calls >=2, got %d", webhook.count())
	}
}

func seedHourAndSettlement(ctx context.Context, db *sql.DB, tenantID, stationID string, dayStart time.Time, hours int, perHourEnergy float64, settlementAmount float64) error {
	dayStart = time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day(), 0, 0, 0, 0, time.UTC)
	for i := 0; i < hours; i++ {
		periodStart := dayStart.Add(time.Duration(i) * time.Hour)
		timeKey := periodStart.Format(time.RFC3339)
		_, err := db.ExecContext(ctx, `
INSERT INTO analytics_statistics (
	subject_id, time_type, time_key, period_start, statistic_id, is_completed,
	charge_kwh, discharge_kwh, earnings, carbon_reduction
) VALUES ($1,'HOUR',$2,$3,$4,TRUE,$5,$6,0,0)
ON CONFLICT (subject_id, time_type, time_key)
DO UPDATE SET charge_kwh = EXCLUDED.charge_kwh, discharge_kwh = EXCLUDED.discharge_kwh, updated_at = NOW()`,
			stationID, timeKey, periodStart, "stat-"+stationID, perHourEnergy, 0)
		if err != nil {
			return err
		}
	}
	_, err := db.ExecContext(ctx, `
INSERT INTO settlements_day (
	tenant_id, station_id, day_start, energy_kwh, amount, currency, status, version
) VALUES ($1,$2,$3,$4,$5,'CNY','CALCULATED',1)
ON CONFLICT (tenant_id, station_id, day_start)
DO UPDATE SET energy_kwh = EXCLUDED.energy_kwh, amount = EXCLUDED.amount, updated_at = NOW()`,
		tenantID, stationID, dayStart, float64(hours)*perHourEnergy, settlementAmount)
	return err
}

func cleanupShadowTables(ctx context.Context, db *sql.DB) {
	_, _ = db.ExecContext(ctx, "DELETE FROM shadowrun_reports")
	_, _ = db.ExecContext(ctx, "DELETE FROM shadowrun_jobs")
	_, _ = db.ExecContext(ctx, "DELETE FROM shadowrun_alerts")
	_, _ = db.ExecContext(ctx, "DELETE FROM analytics_statistics")
	_, _ = db.ExecContext(ctx, "DELETE FROM settlements_day")
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

func applyShadowMigrations(db *sql.DB) error {
	root := projectRoot()
	files := []string{
		filepath.Join(root, "migrations", "001_init.sql"),
		filepath.Join(root, "migrations", "002_settlement.sql"),
		filepath.Join(root, "migrations", "004_tariff.sql"),
		filepath.Join(root, "migrations", "008_statements.sql"),
		filepath.Join(root, "migrations", "011_shadowrun.sql"),
		filepath.Join(root, "migrations", "014_shadowrun_alerts.sql"),
	}
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(content)); err != nil {
			return err
		}
	}
	return nil
}

func projectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return filepath.Clean(filepath.Join(dir, "..", "..", ".."))
}

type fakeWebhook struct {
	mu    sync.Mutex
	calls int
}

func newFakeWebhook() *fakeWebhook {
	return &fakeWebhook{}
}

func (f *fakeWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (f *fakeWebhook) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}
