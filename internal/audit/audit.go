package audit

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Entry represents an audit log entry.
type Entry struct {
	ID            string
	TenantID      string
	Actor         string
	Role          string
	Action        string
	ResourceType  string
	ResourceID    string
	StationID     string
	Metadata      json.RawMessage
	PayloadDigest string
	IP            string
	UserAgent     string
	CreatedAt     time.Time
}

// Logger writes audit entries.
type Logger interface {
	Log(ctx context.Context, entry Entry) error
}

// NewID generates a random audit id.
func NewID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return "audit-" + hex.EncodeToString(buf)
}

// DigestJSON computes a SHA256 hex digest for metadata payloads.
func DigestJSON(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
