-- 014_shadowrun_alerts.sql

BEGIN;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'system_alerts'
  ) THEN
    EXECUTE 'ALTER TABLE system_alerts RENAME TO shadowrun_alerts';
  END IF;

  IF EXISTS (
    SELECT 1 FROM pg_class WHERE relname = ''idx_system_alerts_station''
  ) THEN
    EXECUTE 'ALTER INDEX idx_system_alerts_station RENAME TO idx_shadowrun_alerts_station';
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.views
    WHERE table_schema = 'public' AND table_name = 'system_alerts'
  ) THEN
    IF EXISTS (
      SELECT 1 FROM information_schema.tables
      WHERE table_schema = 'public' AND table_name = 'shadowrun_alerts'
    ) THEN
      EXECUTE 'CREATE VIEW system_alerts AS SELECT * FROM shadowrun_alerts';
    END IF;
  END IF;
END $$;

COMMIT;
