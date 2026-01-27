import http from 'k6/http';
import { check } from 'k6';

const baseUrl = __ENV.BASE_URL || 'http://localhost:8080';
const tenantId = __ENV.TENANT_ID || 'tenant-demo';
const stationPrefix = __ENV.STATION_PREFIX || 'station-perf-';
const devicePrefix = __ENV.DEVICE_PREFIX || 'device-perf-';
const stationCount = intEnv('STATION_COUNT', 10);
const devicesPerStation = intEnv('DEVICES_PER_STATION', 5);
const commandType = __ENV.COMMAND_TYPE || 'setPower';
const vus = intEnv('VUS', 20);
const duration = __ENV.DURATION || '5m';

export const options = {
  vus: vus,
  duration: duration,
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500'],
  },
};

function intEnv(name, fallback) {
  const raw = __ENV[name];
  const value = parseInt(raw, 10);
  if (!Number.isFinite(value) || value <= 0) {
    return fallback;
  }
  return value;
}

function pad(num, width) {
  const str = String(num);
  return str.length >= width ? str : '0'.repeat(width - str.length) + str;
}

function pickStationDevice() {
  const stationIndex = Math.floor(Math.random() * stationCount) + 1;
  const deviceIndex = Math.floor(Math.random() * devicesPerStation) + 1;
  return {
    stationId: `${stationPrefix}${pad(stationIndex, 4)}`,
    deviceId: `${devicePrefix}${pad(deviceIndex, 4)}`,
  };
}

export default function () {
  const ids = pickStationDevice();
  const payloadValue = Math.round(Math.random() * 1000) / 10;
  const body = {
    tenant_id: tenantId,
    station_id: ids.stationId,
    device_id: ids.deviceId,
    command_type: commandType,
    payload: { value: payloadValue },
    idempotency_key: `perf-${__VU}-${__ITER}-${Date.now()}-${Math.random()}`,
  };

  const res = http.post(`${baseUrl}/api/v1/commands`, JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}
