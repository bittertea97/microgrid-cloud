# Performance Report - <DATE>

## Environment
- App version / commit:
- Host spec (CPU / RAM / OS):
- DB spec (version / CPU / RAM / disk):
- Deployment mode (local / container / k8s):
- Runtime config (GOMAXPROCS, env overrides):
- TB RPC server: fake (addr/latency/status rates)

## Artifacts (Raw Data)
- k6 summary JSON:
- k6 metrics JSON:
- DB metrics before/after:
- /metrics snapshot:

## Workloads
### Ingest QPS
- Station count:
- Devices per station:
- Points per device:
- Target ingest RPS:
- Duration:
- Notes:

### Stats query concurrency
- Station count:
- Concurrency (VUs):
- From/To range:
- Granularity:
- Duration:

### Statement export concurrency
- Statement IDs count:
- Formats:
- Concurrency (VUs):
- Duration:

### Command issuance concurrency
- Station count:
- Devices per station:
- Concurrency (VUs):
- Command type:
- Duration:

## Results (Key Metrics)
### Throughput + Latency
- Ingest: throughput (req/s) = ___, P95 = ___ ms
- Stats: throughput (req/s) = ___, P95 = ___ ms
- Statement export: throughput (req/s) = ___, P95 = ___ ms
- Commands: throughput (req/s) = ___, P95 = ___ ms

### CPU / Memory
- App CPU avg / peak:
- App memory avg / peak:
- DB CPU avg / peak:
- DB memory avg / peak:

### DB Write TPS
- xact/s: ___
- write tuples/s: ___
- Notes:

### Queue Backlog
- event_outbox pending: ___
- event_outbox sent/failed: ___
- dead_letter_events: ___
- Notes:

## Capacity Recommendations
- Sustained ingest RPS (p95 target met): ___
- Recommended stations per instance: ___
- Recommended points per instance: ___
- Recommended QPS per instance: ___

DB parameters (placeholders):
- max_connections / pool size: TODO
- work_mem: TODO
- shared_buffers: TODO
- effective_cache_size: TODO
- max_wal_size / checkpoint_timeout: TODO

## Risks / Follow-ups
- 
- 
