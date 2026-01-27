# Security (Auth, RBAC, Tenant Isolation, Audit)

## Authentication (JWT)
- All `/api/*` and `/analytics/*` endpoints require a JWT signed with HS256.
- Public endpoints: `/healthz`, `/metrics`, `/ingest/*` (signed HMAC, see below).
- Set `AUTH_JWT_SECRET` (or `JWT_SECRET`) in the environment.

Required JWT claims:
- `tenant_id` (string)
- `role` (string: `viewer`, `operator`, `admin`)
- `sub` (subject/user id, string)
- `exp` (unix timestamp, seconds)
- `iat` (unix timestamp, seconds)

Example payload:
```
{
  "sub": "user-123",
  "tenant_id": "tenant-demo",
  "role": "operator",
  "iat": 1700000000,
  "exp": 1700003600
}
```

Authorization header:
```
Authorization: Bearer <jwt>
```

## RBAC Matrix
Roles are hierarchical: `admin` > `operator` > `viewer`.

| Capability | viewer | operator | admin |
| --- | --- | --- | --- |
| Read-only APIs (GET) | ✅ | ✅ | ✅ |
| Commands (POST `/api/v1/commands`) | ❌ | ✅ | ✅ |
| Alarm + Strategy config (POST) | ❌ | ✅ | ✅ |
| Statement freeze/void/regenerate | ❌ | ❌ | ✅ |
| Statement export (PDF/XLSX) | ❌ | ❌ | ✅ |
| Provisioning | ❌ | ❌ | ✅ |

## Tenant Isolation
- `tenant_id` is derived from the JWT and is enforced in handlers/services.
- If a request targets another tenant’s station/resource, the API returns **403**.

## Ingest Signature (ThingsBoard)
Ingest does **not** use JWT. Use an independent HMAC secret.

Environment:
- `INGEST_HMAC_SECRET` (required to accept ingest)
- `INGEST_MAX_SKEW_SECONDS` (default `300`)

Headers:
- `X-Ingest-Timestamp`: unix timestamp (seconds)
- `X-Ingest-Signature`: hex HMAC-SHA256 of `timestamp + "\n" + raw_body`

Signing rule (pseudo):
```
signature = hex(hmac_sha256(INGEST_HMAC_SECRET, timestamp + "\n" + body))
```

Requests with missing/expired/invalid signatures are rejected with **401**.

## Audit Logging
Audit events are recorded in `audit_logs` for:
- Command create/issue
- Statement generate/regenerate/freeze/void/export
- Strategy mode/enable/calendar updates
- Provisioning
- Alarm rule creation (when invoked)

Audit fields (standardized):
- `actor` (who)
- `tenant_id`
- `station_id`
- `action`
- `resource_type`
- `resource_id`
- `payload_digest` (SHA256 of metadata payload)
- `created_at`
