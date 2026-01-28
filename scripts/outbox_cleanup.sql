-- Outbox cleanup helper (DEV ONLY).
-- Usage examples:
--   psql "$PG_DSN" -v action=stats -f scripts/outbox_cleanup.sql
--   psql "$PG_DSN" -v action=cleanup_type -v event_type='events.StatisticCalculated' -v confirm=YES -f scripts/outbox_cleanup.sql
--   psql "$PG_DSN" -v action=cleanup_all -v confirm=YES -f scripts/outbox_cleanup.sql
--
-- Actions:
--   stats        : show counts by status and pending by event_type
--   cleanup_type : delete pending rows by event_type (requires event_type + confirm=YES)
--   cleanup_all  : delete all pending rows (requires confirm=YES)
\set ON_ERROR_STOP on

\if :{?action}
\else
\set action 'stats'
\endif
\if :{?confirm}
\else
\set confirm ''
\endif
\if :{?event_type}
\else
\set event_type ''
\endif

\echo ''
SELECT (:'action' = 'stats') AS is_stats,
       (:'action' = 'cleanup_type') AS is_cleanup_type,
       (:'action' = 'cleanup_all') AS is_cleanup_all,
       (:'confirm' = 'YES') AS confirm_yes,
       (length(:'event_type') = 0) AS event_type_empty
\gset

\echo ''
\echo '==> Outbox status counts'
SELECT status, count(*) AS total
FROM event_outbox
GROUP BY status
ORDER BY status;

\echo ''
\echo '==> Pending counts by event_type'
SELECT event_type, count(*) AS pending
FROM event_outbox
WHERE status = 'pending'
GROUP BY event_type
ORDER BY pending DESC, event_type ASC;

\if :is_cleanup_type
  \if :event_type_empty
    \echo 'ERROR: missing -v event_type'
    \quit 1
  \endif

  \echo ''
  \echo '==> Preview pending rows to delete (by event_type)'
  SELECT count(*) AS pending_to_delete
  FROM event_outbox
  WHERE status = 'pending' AND event_type = :'event_type';

  \if :confirm_yes
    \echo ''
    \echo '==> Deleting pending rows (by event_type)'
    WITH deleted AS (
      DELETE FROM event_outbox
      WHERE status = 'pending' AND event_type = :'event_type'
      RETURNING 1
    )
    SELECT count(*) AS deleted
    FROM deleted;
  \else
    \echo 'Dry run only. Re-run with -v confirm=YES to execute.'
  \endif
\elif :is_cleanup_all
  \echo ''
  \echo '==> Preview ALL pending rows to delete'
  SELECT count(*) AS pending_to_delete
  FROM event_outbox
  WHERE status = 'pending';

  \if :confirm_yes
    \echo ''
    \echo '==> Deleting ALL pending rows'
    WITH deleted AS (
      DELETE FROM event_outbox
      WHERE status = 'pending'
      RETURNING 1
    )
    SELECT count(*) AS deleted
    FROM deleted;
  \else
    \echo 'Dry run only. Re-run with -v confirm=YES to execute.'
  \endif
\elif :is_stats
  \echo ''
  \echo '==> Stats only (no deletions).'
\else
  \echo 'ERROR: unknown action. Use stats|cleanup_type|cleanup_all'
  \quit 1
\endif
