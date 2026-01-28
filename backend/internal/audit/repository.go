package audit

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Repository writes audit logs.
type Repository struct {
	db *sql.DB
}

// NewRepository constructs an audit repository.
func NewRepository(db *sql.DB) *Repository {
	if db == nil {
		return nil
	}
	return &Repository{db: db}
}

// Log writes an audit entry.
func (r *Repository) Log(ctx context.Context, entry Entry) error {
	if r == nil || r.db == nil {
		return errors.New("audit repo: nil db")
	}
	if entry.ID == "" {
		entry.ID = NewID()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	if entry.PayloadDigest == "" {
		entry.PayloadDigest = DigestJSON(entry.Metadata)
	}

	_, err := r.db.ExecContext(ctx, `
INSERT INTO audit_logs (
	id, tenant_id, actor, role, action, resource_type, resource_id, station_id,
	metadata, payload_digest, ip, user_agent, created_at
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13
)`, entry.ID, entry.TenantID, entry.Actor, entry.Role, entry.Action, entry.ResourceType, entry.ResourceID, entry.StationID,
		entry.Metadata, entry.PayloadDigest, entry.IP, entry.UserAgent, entry.CreatedAt)
	return err
}
