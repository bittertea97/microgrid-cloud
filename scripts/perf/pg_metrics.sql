-- Capture baseline and post-run metrics, then compute deltas / duration for TPS.

-- Core DB counters
SELECT
  now() AS sampled_at,
  datname,
  xact_commit,
  xact_rollback,
  tup_returned,
  tup_fetched,
  tup_inserted,
  tup_updated,
  tup_deleted,
  blks_read,
  blks_hit,
  temp_files,
  temp_bytes,
  deadlocks
FROM pg_stat_database
WHERE datname = current_database();

-- Outbox backlog (queue depth)
SELECT status, COUNT(*) AS total
FROM event_outbox
GROUP BY status
ORDER BY status;

-- DLQ count
SELECT COUNT(*) AS dead_letter_total
FROM dead_letter_events;

-- Command status distribution (optional)
SELECT status, COUNT(*) AS total
FROM commands
GROUP BY status
ORDER BY status;

-- Analytics stats count (optional)
SELECT COUNT(*) AS analytics_stat_rows
FROM analytics_statistics;

-- If pg_stat_statements is enabled, top statements by total time.
-- SELECT query, calls, mean_exec_time, rows
-- FROM pg_stat_statements
-- ORDER BY total_exec_time DESC
-- LIMIT 10;
