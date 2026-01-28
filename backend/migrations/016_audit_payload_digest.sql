-- 016_audit_payload_digest.sql

ALTER TABLE audit_logs
  ADD COLUMN IF NOT EXISTS payload_digest TEXT;
