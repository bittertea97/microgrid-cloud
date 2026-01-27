package integration_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	settlementapp "microgrid-cloud/internal/settlement/application"
	settlementrepo "microgrid-cloud/internal/settlement/infrastructure/postgres"
	settlementinterfaces "microgrid-cloud/internal/settlement/interfaces"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestStatement_GenerateFreezeRegenerateAndExport(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := applyStatementMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	ctx := context.Background()
	tenantID := "tenant-stmt"
	stationID := "station-stmt-001"
	monthStart := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	_, _ = db.ExecContext(ctx, "DELETE FROM settlement_statement_items")
	_, _ = db.ExecContext(ctx, "DELETE FROM settlement_statements")
	_, _ = db.ExecContext(ctx, "DELETE FROM settlements_day WHERE tenant_id = $1 AND station_id = $2", tenantID, stationID)

	if err := seedSettlementsDay(ctx, db, tenantID, stationID, monthStart, []float64{10, 12, 14}, []float64{100, 120, 140}); err != nil {
		t.Fatalf("seed settlements: %v", err)
	}

	stmtRepo := settlementrepo.NewStatementRepository(db)
	stmtService, err := settlementapp.NewStatementService(stmtRepo, tenantID)
	if err != nil {
		t.Fatalf("statement service: %v", err)
	}

	stmt, err := stmtService.Generate(ctx, stationID, "2026-01", "owner", false)
	if err != nil {
		t.Fatalf("generate statement: %v", err)
	}
	if stmt.Status != "draft" {
		t.Fatalf("expected draft, got %s", stmt.Status)
	}
	if stmt.TotalAmount != 360 {
		t.Fatalf("total amount mismatch: %v", stmt.TotalAmount)
	}

	frozen, err := stmtService.Freeze(ctx, stmt.ID)
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if frozen.Status != "frozen" || frozen.SnapshotHash == "" {
		t.Fatalf("freeze failed or missing hash")
	}

	// backfill update one day
	_, err = db.ExecContext(ctx, `
UPDATE settlements_day
SET energy_kwh = $1, amount = $2, updated_at = NOW()
WHERE tenant_id = $3 AND station_id = $4 AND day_start = $5`,
		20.0, 200.0, tenantID, stationID, monthStart.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("backfill update: %v", err)
	}

	newStmt, err := stmtService.Generate(ctx, stationID, "2026-01", "owner", true)
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if newStmt.Version != stmt.Version+1 {
		t.Fatalf("expected version bump")
	}
	if newStmt.TotalAmount != 420 {
		t.Fatalf("regenerated total mismatch: %v", newStmt.TotalAmount)
	}

	// frozen statement unchanged
	frozenAgain, _, err := stmtService.Get(ctx, stmt.ID)
	if err != nil {
		t.Fatalf("get frozen: %v", err)
	}
	if frozenAgain.TotalAmount != 360 {
		t.Fatalf("frozen statement changed")
	}

	handler, err := settlementinterfaces.NewStatementHandler(stmtService, nil, nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/statements", handler)
	mux.Handle("/api/v1/statements/", handler)
	mux.Handle("/api/v1/statements/generate", handler)

	pdfReq := httptest.NewRequest(http.MethodGet, "/api/v1/statements/"+stmt.ID+"/export.pdf", nil)
	pdfResp := httptest.NewRecorder()
	mux.ServeHTTP(pdfResp, pdfReq)
	if pdfResp.Code != http.StatusOK {
		t.Fatalf("pdf status %d", pdfResp.Code)
	}
	if pdfResp.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("pdf content-type mismatch")
	}
	if len(pdfResp.Body.Bytes()) == 0 {
		t.Fatalf("pdf empty")
	}

	xlsxReq := httptest.NewRequest(http.MethodGet, "/api/v1/statements/"+stmt.ID+"/export.xlsx", nil)
	xlsxResp := httptest.NewRecorder()
	mux.ServeHTTP(xlsxResp, xlsxReq)
	if xlsxResp.Code != http.StatusOK {
		t.Fatalf("xlsx status %d", xlsxResp.Code)
	}
	if xlsxResp.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("xlsx content-type mismatch")
	}
	if len(xlsxResp.Body.Bytes()) == 0 {
		t.Fatalf("xlsx empty")
	}
}

func applyStatementMigrations(db *sql.DB) error {
	root := projectRoot()
	files := []string{
		filepath.Join(root, "migrations", "002_settlement.sql"),
		filepath.Join(root, "migrations", "008_statements.sql"),
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

func seedSettlementsDay(ctx context.Context, db *sql.DB, tenantID, stationID string, dayStart time.Time, energy []float64, amount []float64) error {
	for i := range energy {
		_, err := db.ExecContext(ctx, `
INSERT INTO settlements_day (
	tenant_id, station_id, day_start, energy_kwh, amount, currency, status, version
) VALUES ($1,$2,$3,$4,$5,'CNY','CALCULATED',1)
ON CONFLICT (tenant_id, station_id, day_start)
DO UPDATE SET energy_kwh = EXCLUDED.energy_kwh, amount = EXCLUDED.amount, updated_at = NOW()`,
			tenantID, stationID, dayStart.AddDate(0, 0, i), energy[i], amount[i])
		if err != nil {
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
