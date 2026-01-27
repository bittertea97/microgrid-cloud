#!/usr/bin/env bash
set -euo pipefail

jwt_b64url() {
  openssl base64 -A | tr '+/' '-_' | tr -d '='
}

jwt_token_hs256() {
  local secret="$1"
  local tenant_id="$2"
  local role="$3"
  local subject="$4"
  local ttl_seconds="$5"

  if [[ -z "$secret" || -z "$tenant_id" || -z "$role" || -z "$subject" ]]; then
    echo "jwt_token_hs256: missing args" >&2
    return 1
  fi

  local now
  now=$(date +%s)
  local exp=$((now + ttl_seconds))

  local header payload signature
  header=$(printf '{"alg":"HS256","typ":"JWT"}' | jwt_b64url)
  payload=$(printf '{"tenant_id":"%s","role":"%s","sub":"%s","iat":%d,"exp":%d}' "$tenant_id" "$role" "$subject" "$now" "$exp" | jwt_b64url)
  signature=$(printf '%s.%s' "$header" "$payload" | openssl dgst -sha256 -hmac "$secret" -binary | jwt_b64url)
  printf '%s.%s.%s' "$header" "$payload" "$signature"
}

ingest_signature() {
  local secret="$1"
  local timestamp="$2"
  local body="$3"
  printf '%s\n%s' "$timestamp" "$body" | openssl dgst -sha256 -hmac "$secret" -hex | awk '{print $2}'
}
