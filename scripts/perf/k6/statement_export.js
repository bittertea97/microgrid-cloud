import http from 'k6/http';
import { check } from 'k6';

const baseUrl = __ENV.BASE_URL || 'http://localhost:8080';
const vus = intEnv('VUS', 10);
const duration = __ENV.DURATION || '5m';

const statementIds = loadStatementIds();
const formats = loadFormats();

export const options = {
  vus: vus,
  duration: duration,
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<1500'],
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

function loadStatementIds() {
  const filePath = (__ENV.STATEMENT_IDS_FILE || '').trim();
  if (filePath.length > 0) {
    const content = open(filePath);
    const list = content
      .split(/[\r\n,]+/)
      .map((value) => value.trim())
      .filter((value) => value.length > 0);
    if (list.length > 0) {
      return list;
    }
  }

  const list = (__ENV.STATEMENT_IDS || '')
    .split(',')
    .map((value) => value.trim())
    .filter((value) => value.length > 0);
  if (list.length > 0) {
    return list;
  }
  const single = (__ENV.STATEMENT_ID || '').trim();
  if (single.length > 0) {
    return [single];
  }
  throw new Error('STATEMENT_ID or STATEMENT_IDS is required');
}

function loadFormats() {
  const raw = (__ENV.EXPORT_FORMATS || __ENV.EXPORT_FORMAT || 'pdf').split(',');
  return raw.map((value) => value.trim()).filter((value) => value.length > 0);
}

export default function () {
  const id = statementIds[Math.floor(Math.random() * statementIds.length)];
  const format = formats[Math.floor(Math.random() * formats.length)];
  const url = `${baseUrl}/api/v1/statements/${encodeURIComponent(id)}/export.${encodeURIComponent(format)}`;
  const res = http.get(url);
  check(res, {
    'status is 200': (r) => r.status === 200,
    'body not empty': (r) => r.body && r.body.length > 0,
  });
}
