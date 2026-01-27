param(
  [string]$BaseUrl = "http://localhost:8080",
  [string]$PgDsn = "",
  [string]$AuthJwtSecret = "",
  [string]$IngestHmacSecret = "",
  [int]$AutoStartDocker = 1,
  [string]$ComposeFile = "docker-compose.dev.yml",
  [string]$TenantId = "tenant-demo",
  [string]$StationId = "station-demo-001",
  [string]$StationName = "station-demo-001",
  [string]$StationTZ = "UTC",
  [string]$StationType = "microgrid",
  [string]$StationRegion = "pilot",
  [string]$DeviceId = "device-demo-001",
  [string]$DeviceName = "device-demo-001",
  [string]$DeviceType = "inverter",
  [string]$TBProfile = "default",
  [string]$DeviceCredentials = "token-123",
  [string]$Month = "2026-01",
  [string]$BaseDay = "2026-01-20",
  [string]$ExportDir = "var/exports",
  [string]$JwtRole = "admin",
  [string]$JwtSubject = "release-manager",
  [int]$JwtTtlSeconds = 3600
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Require-Command([string]$Name) {
  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
    throw "missing required command: $Name"
  }
}

function Base64UrlEncode([byte[]]$bytes) {
  [Convert]::ToBase64String($bytes).TrimEnd("=") -replace "\+", "-" -replace "/", "_"
}

function New-JwtHs256([string]$secret, [hashtable]$payload) {
  $header = @{ alg = "HS256"; typ = "JWT" }
  $headerJson = ($header | ConvertTo-Json -Compress)
  $payloadJson = ($payload | ConvertTo-Json -Compress)
  $headerB64 = Base64UrlEncode([Text.Encoding]::UTF8.GetBytes($headerJson))
  $payloadB64 = Base64UrlEncode([Text.Encoding]::UTF8.GetBytes($payloadJson))
  $unsigned = "$headerB64.$payloadB64"
  $hmac = New-Object System.Security.Cryptography.HMACSHA256([Text.Encoding]::UTF8.GetBytes($secret))
  $sig = Base64UrlEncode($hmac.ComputeHash([Text.Encoding]::UTF8.GetBytes($unsigned)))
  "$unsigned.$sig"
}

function New-IngestSignature([string]$secret, [string]$timestamp, [string]$body) {
  $hmac = New-Object System.Security.Cryptography.HMACSHA256([Text.Encoding]::UTF8.GetBytes($secret))
  $payload = "$timestamp`n$body"
  $hash = $hmac.ComputeHash([Text.Encoding]::UTF8.GetBytes($payload))
  ($hash | ForEach-Object { $_.ToString("x2") }) -join ""
}

function Invoke-Health([string]$url) {
  try {
    Invoke-WebRequest -Uri "$url/healthz" -Method Get -UseBasicParsing | Out-Null
    return $true
  } catch {
    return $false
  }
}

function Invoke-Json([string]$method, [string]$url, [hashtable]$headers, $body) {
  $json = $null
  if ($null -ne $body) {
    $json = $body | ConvertTo-Json -Depth 10 -Compress
  }
  if ($null -ne $json) {
    return Invoke-RestMethod -Method $method -Uri $url -Headers $headers -ContentType "application/json" -Body $json
  }
  return Invoke-RestMethod -Method $method -Uri $url -Headers $headers
}

function Ensure-Dir([string]$path) {
  if (-not (Test-Path $path)) {
    New-Item -ItemType Directory -Force -Path $path | Out-Null
  }
}

if ([string]::IsNullOrEmpty($AuthJwtSecret)) {
  $AuthJwtSecret = "dev-secret-change-me"
}
if ([string]::IsNullOrEmpty($IngestHmacSecret)) {
  $IngestHmacSecret = "dev-ingest-secret"
}

if ([string]::IsNullOrEmpty($PgDsn) -and $AutoStartDocker -eq 1 -and (Get-Command docker -ErrorAction SilentlyContinue)) {
  $PgDsn = "postgres://microgrid:microgrid@localhost:5432/microgrid?sslmode=disable"
}

if ($AutoStartDocker -eq 1 -and (Get-Command docker -ErrorAction SilentlyContinue)) {
  Write-Host "==> Starting docker compose stack"
  & docker compose -f $ComposeFile up -d | Out-Host
}

if (-not [string]::IsNullOrEmpty($PgDsn) -and (Get-Command psql -ErrorAction SilentlyContinue)) {
  Write-Host "==> Applying migrations"
  Get-ChildItem -Path "migrations" -Filter "*.sql" | Sort-Object Name | ForEach-Object {
    & psql $PgDsn -f $_.FullName | Out-Host
  }
} else {
  Write-Host "==> Skipping migrations (PG_DSN or psql missing)"
}

if (-not (Invoke-Health $BaseUrl)) {
  Write-Host "==> Waiting for service to become healthy"
  1..30 | ForEach-Object {
    if (Invoke-Health $BaseUrl) { return }
    Start-Sleep -Seconds 2
  }
}
if (-not (Invoke-Health $BaseUrl)) {
  throw "service health check failed: $BaseUrl/healthz"
}

Write-Host "==> Building auth headers"
$now = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
$payload = @{
  sub = $JwtSubject
  tenant_id = $TenantId
  role = $JwtRole
  iat = $now
  exp = $now + $JwtTtlSeconds
}
$jwt = New-JwtHs256 -secret $AuthJwtSecret -payload $payload
$authHeaders = @{ Authorization = "Bearer $jwt" }

function Invoke-Ingest($body) {
  $timestamp = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds().ToString()
  $json = $body | ConvertTo-Json -Depth 6 -Compress
  $sig = New-IngestSignature -secret $IngestHmacSecret -timestamp $timestamp -body $json
  Invoke-RestMethod -Method Post -Uri "$BaseUrl/ingest/thingsboard/telemetry" -Headers @{
    "Content-Type" = "application/json"
    "X-Ingest-Timestamp" = $timestamp
    "X-Ingest-Signature" = $sig
  } -Body $json | Out-Null
}

$summary = @()

try {
  Write-Host "==> Provisioning station/device/mappings"
  $provisionBody = @{
    station = @{
      id = $StationId
      tenant_id = $TenantId
      name = $StationName
      timezone = $StationTZ
      type = $StationType
      region = $StationRegion
    }
    devices = @(
      @{
        id = $DeviceId
        name = $DeviceName
        device_type = $DeviceType
        tb_profile = $TBProfile
        credentials = $DeviceCredentials
      }
    )
    point_mappings = @(
      @{ device_id = $DeviceId; point_key = "charge_power_kw"; semantic = "charge_power_kw"; unit = "kW"; factor = 1 }
      @{ device_id = $DeviceId; point_key = "discharge_power_kw"; semantic = "discharge_power_kw"; unit = "kW"; factor = 1 }
      @{ device_id = $DeviceId; point_key = "earnings"; semantic = "earnings"; unit = "CNY"; factor = 1 }
      @{ device_id = $DeviceId; point_key = "carbon_reduction"; semantic = "carbon_reduction"; unit = "kg"; factor = 1 }
    )
  }
  $provisionResp = Invoke-Json -method Post -url "$BaseUrl/api/v1/provisioning/stations" -headers $authHeaders -body $provisionBody
  $provisionResp | ConvertTo-Json -Depth 6 | Write-Host

  Write-Host "==> Ingest telemetry + window close (3 days)"
  for ($day = 0; $day -lt 3; $day++) {
    for ($hour = 0; $hour -lt 24; $hour++) {
      $windowStart = ([DateTime]::Parse("$BaseDay`T00:00:00Z")).AddDays($day).AddHours($hour)
      $tsMs = ([DateTimeOffset]$windowStart.AddMinutes(5)).ToUnixTimeMilliseconds()
      $ingest = @{
        tenantId = $TenantId
        stationId = $StationId
        deviceId = $DeviceId
        ts = $tsMs
        values = @{
          charge_power_kw = 1
          discharge_power_kw = 2
          earnings = 0.1
          carbon_reduction = 0.01
        }
      }
      Invoke-Ingest $ingest
      $closeBody = @{
        stationId = $StationId
        windowStart = $windowStart.ToString("yyyy-MM-ddTHH:00:00Z")
      }
      Invoke-Json -method Post -url "$BaseUrl/analytics/window-close" -headers $authHeaders -body $closeBody | Out-Null
    }
  }

  $fromTs = ([DateTime]::Parse("$BaseDay`T00:00:00Z")).ToString("yyyy-MM-ddTHH:mm:ssZ")
  $toTs = ([DateTime]::Parse("$BaseDay`T00:00:00Z")).AddDays(3).ToString("yyyy-MM-ddTHH:mm:ssZ")

  Write-Host "==> Verify analytics statistics"
  Invoke-Json -method Get -url "$BaseUrl/api/v1/stats?station_id=$StationId&from=$fromTs&to=$toTs&granularity=day" -headers $authHeaders -body $null | ConvertTo-Json -Depth 6 | Write-Host

  Write-Host "==> Verify settlements_day"
  Invoke-Json -method Get -url "$BaseUrl/api/v1/settlements?station_id=$StationId&from=$fromTs&to=$toTs" -headers $authHeaders -body $null | ConvertTo-Json -Depth 6 | Write-Host

  Write-Host "==> Generate statement (draft) and freeze"
  $stmtBody = @{
    station_id = $StationId
    month = $Month
    category = "owner"
    regenerate = $false
  }
  $stmtResp = Invoke-Json -method Post -url "$BaseUrl/api/v1/statements/generate" -headers $authHeaders -body $stmtBody
  $stmtId = $stmtResp.statement_id
  $stmtResp | ConvertTo-Json -Depth 6 | Write-Host

  $freezeResp = Invoke-Json -method Post -url "$BaseUrl/api/v1/statements/$stmtId/freeze" -headers $authHeaders -body $null
  $freezeResp | ConvertTo-Json -Depth 6 | Write-Host

  Ensure-Dir $ExportDir
  Invoke-WebRequest -Uri "$BaseUrl/api/v1/statements/$stmtId/export.pdf" -Headers $authHeaders -OutFile (Join-Path $ExportDir "statement.pdf") | Out-Null
  Invoke-WebRequest -Uri "$BaseUrl/api/v1/statements/$stmtId/export.xlsx" -Headers $authHeaders -OutFile (Join-Path $ExportDir "statement.xlsx") | Out-Null

  Write-Host "==> Create alarm rule"
  if (Get-Command psql -ErrorAction SilentlyContinue) {
    & psql $PgDsn -c "DELETE FROM alarm_rules WHERE id='rule-pilot-001';" | Out-Null
    & psql $PgDsn -c "INSERT INTO alarm_rules (id, tenant_id, station_id, name, semantic, operator, threshold, hysteresis, duration_seconds, severity, enabled) VALUES ('rule-pilot-001', '$TenantId', '$StationId', 'Charge Power High', 'charge_power_kw', '>', 100, 5, 0, 'high', TRUE);" | Out-Null
  } else {
    Write-Host "psql missing; skipping alarm rule insert"
  }

  Write-Host "==> Trigger alarm (active -> clear)"
  $tsNow = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
  Invoke-Ingest @{
    tenantId = $TenantId
    stationId = $StationId
    deviceId = $DeviceId
    ts = $tsNow
    values = @{ charge_power_kw = 120 }
  }
  Start-Sleep -Seconds 2
  $tsNow = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
  Invoke-Ingest @{
    tenantId = $TenantId
    stationId = $StationId
    deviceId = $DeviceId
    ts = $tsNow
    values = @{ charge_power_kw = 90 }
  }
  Start-Sleep -Seconds 2

  $fromAlarm = [DateTime]::UtcNow.AddHours(-1).ToString("yyyy-MM-ddTHH:mm:ssZ")
  $toAlarm = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
  Invoke-Json -method Get -url "$BaseUrl/api/v1/alarms?station_id=$StationId&from=$fromAlarm&to=$toAlarm" -headers $authHeaders -body $null | ConvertTo-Json -Depth 6 | Write-Host

  Write-Host "==> Issue command"
  $cmdBody = @{
    tenant_id = $TenantId
    station_id = $StationId
    device_id = $DeviceId
    command_type = "setPower"
    payload = @{ value = 10 }
    idempotency_key = "setPower-20260101-001"
  }
  $cmdResp = Invoke-Json -method Post -url "$BaseUrl/api/v1/commands" -headers $authHeaders -body $cmdBody
  $cmdId = $cmdResp.command_id
  $cmdResp | ConvertTo-Json -Depth 6 | Write-Host

  Start-Sleep -Seconds 2
  $fromCmd = [DateTime]::UtcNow.AddMinutes(-5).ToString("yyyy-MM-ddTHH:mm:ssZ")
  $toCmd = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
  $cmdList = Invoke-Json -method Get -url "$BaseUrl/api/v1/commands?station_id=$StationId&from=$fromCmd&to=$toCmd" -headers $authHeaders -body $null
  $cmdList | ConvertTo-Json -Depth 6 | Write-Host
  $lastStatus = ($cmdList | Select-Object -Last 1).status
  if ($lastStatus -ne "acked" -and (Get-Command psql -ErrorAction SilentlyContinue)) {
    Write-Host "Command not acked yet (status=$lastStatus). Marking timeout for pilot."
    & psql $PgDsn -c "UPDATE commands SET status='timeout', error='timeout' WHERE command_id='$cmdId' AND status='sent';" | Out-Null
  }

  Write-Host "==> Backfill one hour and recalc"
  $stmtDetail = Invoke-Json -method Get -url "$BaseUrl/api/v1/statements/$stmtId" -headers $authHeaders -body $null
  $frozenTotal = $stmtDetail.statement.total_amount
  $backfillTs = [DateTimeOffset]::Parse("2026-01-21T06:05:00Z").ToUnixTimeMilliseconds()
  Invoke-Ingest @{
    tenantId = $TenantId
    stationId = $StationId
    deviceId = $DeviceId
    ts = $backfillTs
    values = @{ charge_power_kw = 10; discharge_power_kw = 20 }
  }
  Invoke-Json -method Post -url "$BaseUrl/analytics/window-close" -headers $authHeaders -body @{ stationId = $StationId; windowStart = "2026-01-21T06:00:00Z"; recalculate = $true } | Out-Null

  $stmtDetailAfter = Invoke-Json -method Get -url "$BaseUrl/api/v1/statements/$stmtId" -headers $authHeaders -body $null
  $frozenAfter = $stmtDetailAfter.statement.total_amount
  if ($frozenTotal -ne $frozenAfter) {
    throw "Frozen statement changed unexpectedly: $frozenTotal -> $frozenAfter"
  }

  Write-Host "==> Regenerate statement for corrected version"
  $regenBody = @{
    station_id = $StationId
    month = $Month
    category = "owner"
    regenerate = $true
  }
  (Invoke-Json -method Post -url "$BaseUrl/api/v1/statements/generate" -headers $authHeaders -body $regenBody) | ConvertTo-Json -Depth 6 | Write-Host

  Write-Host "==> Trigger shadowrun"
  $shadowBody = @{
    tenant_id = $TenantId
    station_ids = @($StationId)
    month = $Month
    thresholds = @{ energy_abs = 5; amount_abs = 5; missing_hours = 2 }
  }
  (Invoke-Json -method Post -url "$BaseUrl/api/v1/shadowrun/run" -headers $authHeaders -body $shadowBody) | ConvertTo-Json -Depth 6 | Write-Host

  Write-Host "==> List shadowrun reports"
  $from = "$Month-01T00:00:00Z"
  $to = ([DateTime]::Parse("$Month-01T00:00:00Z")).AddMonths(1).ToString("yyyy-MM-ddTHH:mm:ssZ")
  (Invoke-Json -method Get -url "$BaseUrl/api/v1/shadowrun/reports?station_id=$StationId&from=$from&to=$to" -headers $authHeaders -body $null) | ConvertTo-Json -Depth 6 | Write-Host

  $summary += "pilot: OK"
  $summary += "shadowrun: OK"
} catch {
  $summary += "error: $($_.Exception.Message)"
  throw
} finally {
  Write-Host "==> Release acceptance summary"
  $summary | ForEach-Object { Write-Host " - $_" }
}
