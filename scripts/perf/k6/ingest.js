import http from 'k6/http';
import { check } from 'k6';

const baseUrl = __ENV.BASE_URL || 'http://localhost:8080';
const tenantId = __ENV.TENANT_ID || 'tenant-demo';
const stationPrefix = __ENV.STATION_PREFIX || 'station-perf-';
const devicePrefix = __ENV.DEVICE_PREFIX || 'device-perf-';
const stationCount = intEnv('STATION_COUNT', 10);
const devicesPerStation = intEnv('DEVICES_PER_STATION', 5);
const pointsPerDevice = intEnv('POINTS_PER_DEVICE', 20);
const quality = __ENV.QUALITY || 'good';
const rate = intEnv('INGEST_RPS', 200);
const duration = __ENV.DURATION || '5m';
const preAllocatedVUs = intEnv('PREALLOCATED_VUS', Math.min(rate, 200));
const maxVUs = intEnv('MAX_VUS', Math.max(preAllocatedVUs, 400));

const pointKeys = buildPointKeys(pointsPerDevice);

export const options = {
  scenarios: {
    ingest_qps: {
      executor: 'constant-arrival-rate',
      rate: rate,
      timeUnit: '1s',
      duration: duration,
      preAllocatedVUs: preAllocatedVUs,
      maxVUs: maxVUs,
    },
  },
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

function buildPointKeys(count) {
  const baseKeys = ['charge_power_kw', 'discharge_power_kw', 'earnings', 'carbon_reduction'];
  const keys = [];
  for (let i = 0; i < count; i += 1) {
    if (i < baseKeys.length) {
      keys.push(baseKeys[i]);
    } else {
      keys.push(`p_${pad(i + 1, 4)}`);
    }
  }
  return keys;
}

function buildValues() {
  const values = {};
  for (let i = 0; i < pointKeys.length; i += 1) {
    const value = Math.round((Math.random() * 100 + i) * 1000) / 1000;
    values[pointKeys[i]] = value;
  }
  return values;
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
  const payload = JSON.stringify({
    tenantId: tenantId,
    stationId: ids.stationId,
    deviceId: ids.deviceId,
    ts: Date.now(),
    values: buildValues(),
    quality: quality,
  });

  const res = http.post(`${baseUrl}/ingest/thingsboard/telemetry`, payload, {
    headers: { 'Content-Type': 'application/json' },
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}
