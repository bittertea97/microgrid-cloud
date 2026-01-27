import http from 'k6/http';
import { check } from 'k6';

const baseUrl = __ENV.BASE_URL || 'http://localhost:8080';
const stationPrefix = __ENV.STATION_PREFIX || 'station-perf-';
const stationCount = intEnv('STATION_COUNT', 10);
const vus = intEnv('VUS', 20);
const duration = __ENV.DURATION || '5m';
const granularity = __ENV.GRANULARITY || 'hour';

const now = new Date();
const toTime = __ENV.TO_TS || now.toISOString();
const fromTime = __ENV.FROM_TS || new Date(now.getTime() - 24 * 60 * 60 * 1000).toISOString();

export const options = {
  vus: vus,
  duration: duration,
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<300'],
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

function pickStationId() {
  const stationIndex = Math.floor(Math.random() * stationCount) + 1;
  return `${stationPrefix}${pad(stationIndex, 4)}`;
}

export default function () {
  const stationId = pickStationId();
  const url = `${baseUrl}/api/v1/stats?station_id=${encodeURIComponent(stationId)}&from=${encodeURIComponent(fromTime)}&to=${encodeURIComponent(toTime)}&granularity=${encodeURIComponent(granularity)}`;
  const res = http.get(url);
  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}
