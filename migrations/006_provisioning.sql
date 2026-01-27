-- 006_provisioning.sql

ALTER TABLE stations
	ADD COLUMN IF NOT EXISTS tb_asset_id TEXT,
	ADD COLUMN IF NOT EXISTS tb_tenant_id TEXT;
