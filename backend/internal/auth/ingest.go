package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// IngestAuthMiddleware validates ThingsBoard ingest signatures.
type IngestAuthMiddleware struct {
	Secret  []byte
	MaxSkew time.Duration
}

// NewIngestAuthMiddleware constructs ingest auth middleware.
func NewIngestAuthMiddleware(secret []byte, maxSkew time.Duration) *IngestAuthMiddleware {
	return &IngestAuthMiddleware{Secret: secret, MaxSkew: maxSkew}
}

// Wrap enforces ingest signature validation.
func (m *IngestAuthMiddleware) Wrap(next http.Handler) http.Handler {
	if m == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(m.Secret) == 0 {
			http.Error(w, "ingest auth not configured", http.StatusUnauthorized)
			return
		}
		timestamp := strings.TrimSpace(r.Header.Get("X-Ingest-Timestamp"))
		signature := strings.TrimSpace(r.Header.Get("X-Ingest-Signature"))
		if timestamp == "" || signature == "" {
			http.Error(w, "missing ingest signature", http.StatusUnauthorized)
			return
		}
		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			http.Error(w, "invalid ingest timestamp", http.StatusUnauthorized)
			return
		}
		skew := time.Since(time.Unix(ts, 0))
		if skew < 0 {
			skew = -skew
		}
		if m.MaxSkew > 0 && skew > m.MaxSkew {
			http.Error(w, "ingest signature expired", http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body error", http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()

		expected := computeIngestSignature(m.Secret, timestamp, body)
		if !hmac.Equal([]byte(strings.ToLower(signature)), []byte(expected)) {
			http.Error(w, "invalid ingest signature", http.StatusUnauthorized)
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

func computeIngestSignature(secret []byte, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
