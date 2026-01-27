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

	apihttp "microgrid-cloud/internal/api/http"
	"microgrid-cloud/internal/auth"

	"github.com/golang-jwt/jwt/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestCrossTenantQueryForbidden(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := applyTenantMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	ctx := context.Background()
	tenantA := "tenant-a"
	tenantB := "tenant-b"
	stationID := "station-a-001"

	_, _ = db.ExecContext(ctx, "DELETE FROM stations WHERE id = $1", stationID)
	_, err = db.ExecContext(ctx, `
INSERT INTO stations (id, tenant_id, name, timezone, station_type, region)
VALUES ($1,$2,$3,$4,$5,$6)`, stationID, tenantA, "demo", "UTC", "microgrid", "lab")
	if err != nil {
		t.Fatalf("insert station: %v", err)
	}

	stationChecker := auth.NewStationChecker(db)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/stats", apihttp.NewStatsHandler(db, stationChecker))

	secret := []byte("test-secret")
	policy := auth.NewDefaultPolicy(nil, nil)
	mw := auth.NewMiddleware(secret, policy)
	server := httptest.NewServer(mw.Wrap(mux))
	defer server.Close()

	token := mustToken(t, secret, tenantB, "viewer")
	from := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/stats?station_id="+stationID+"&from="+from+"&to="+to+"&granularity=hour", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func applyTenantMigrations(db *sql.DB) error {
	root := projectRoot()
	files := []string{
		filepath.Join(root, "migrations", "001_init.sql"),
		filepath.Join(root, "migrations", "003_masterdata.sql"),
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

func mustToken(t *testing.T, secret []byte, tenantID, role string) string {
	t.Helper()
	claims := auth.Claims{
		TenantID: tenantID,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}
