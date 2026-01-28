import React, { useEffect, useMemo, useState } from 'react';

const defaultBaseUrl = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8081';
const defaultStationId = 'station-demo-001';

const toIso = (value) => {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toISOString();
};

const toLocalInput = (date) => {
  const pad = (n) => String(n).padStart(2, '0');
  const year = date.getFullYear();
  const month = pad(date.getMonth() + 1);
  const day = pad(date.getDate());
  const hour = pad(date.getHours());
  const minute = pad(date.getMinutes());
  return `${year}-${month}-${day}T${hour}:${minute}`;
};

const formatNumber = (value, digits = 2) => {
  if (value === null || value === undefined) return '--';
  const num = Number(value);
  if (Number.isNaN(num)) return '--';
  return num.toFixed(digits);
};

const formatDate = (value) => {
  if (!value) return '--';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '--';
  return date.toLocaleString();
};

const fetchJson = async (url, token) => {
  const resp = await fetch(url, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`${resp.status} ${resp.statusText} - ${text}`);
  }
  return resp.json();
};

const Sparkline = ({ values }) => {
  const width = 240;
  const height = 64;
  const padding = 6;
  const safeValues = values.filter((v) => Number.isFinite(v));
  if (safeValues.length === 0) {
    return (
      <div className="sparkline-empty">暂无数据</div>
    );
  }
  const min = Math.min(...safeValues);
  const max = Math.max(...safeValues);
  const range = max - min || 1;
  const step = (width - padding * 2) / (values.length - 1 || 1);
  const points = values.map((value, index) => {
    const x = padding + index * step;
    const normalized = Number.isFinite(value) ? (value - min) / range : 0;
    const y = height - padding - normalized * (height - padding * 2);
    return `${x},${y}`;
  });

  return (
    <svg className="sparkline" viewBox={`0 0 ${width} ${height}`} role="img">
      <polyline
        fill="none"
        stroke="var(--accent)"
        strokeWidth="2"
        strokeLinejoin="round"
        strokeLinecap="round"
        points={points.join(' ')}
      />
    </svg>
  );
};

const LineChart = ({ series, height = 160 }) => {
  const width = 520;
  const padding = 16;
  const allValues = series.flatMap((s) => s.values).filter((v) => Number.isFinite(v));
  if (allValues.length === 0) {
    return <div className="sparkline-empty">暂无数据</div>;
  }
  const min = Math.min(...allValues);
  const max = Math.max(...allValues);
  const range = max - min || 1;
  const maxLen = Math.max(...series.map((s) => s.values.length));
  const step = (width - padding * 2) / (maxLen - 1 || 1);

  return (
    <svg className="linechart" viewBox={`0 0 ${width} ${height}`} role="img">
      {series.map((s, idx) => {
        const points = s.values.map((value, index) => {
          const x = padding + index * step;
          const normalized = Number.isFinite(value) ? (value - min) / range : 0;
          const y = height - padding - normalized * (height - padding * 2);
          return `${x},${y}`;
        });
        return (
          <polyline
            key={`${s.label}-${idx}`}
            fill="none"
            stroke={s.color}
            strokeWidth="2"
            strokeLinejoin="round"
            strokeLinecap="round"
            points={points.join(' ')}
          />
        );
      })}
    </svg>
  );
};

export default function App() {
  const now = new Date();
  const defaultTo = toLocalInput(now);
  const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000);
  const defaultFrom = toLocalInput(yesterday);
  const defaultStations = ['station-demo-001'];

  const [baseUrl, setBaseUrl] = useState(() => localStorage.getItem('mg_base_url') || defaultBaseUrl);
  const [stationId, setStationId] = useState(() => localStorage.getItem('mg_station_id') || defaultStationId);
  const [stations, setStations] = useState(() => {
    const raw = localStorage.getItem('mg_station_list');
    if (!raw) return defaultStations;
    try {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed) && parsed.length) return parsed;
      return defaultStations;
    } catch {
      return defaultStations;
    }
  });
  const [newStation, setNewStation] = useState('');
  const [token, setToken] = useState(() => localStorage.getItem('mg_token') || '');
  const [from, setFrom] = useState(defaultFrom);
  const [to, setTo] = useState(defaultTo);
  const [windowStart, setWindowStart] = useState(defaultFrom);

  const [stats, setStats] = useState([]);
  const [settlements, setSettlements] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [windowCloseStatus, setWindowCloseStatus] = useState('');

  useEffect(() => {
    localStorage.setItem('mg_base_url', baseUrl);
  }, [baseUrl]);

  useEffect(() => {
    localStorage.setItem('mg_station_id', stationId);
  }, [stationId]);

  useEffect(() => {
    localStorage.setItem('mg_station_list', JSON.stringify(stations));
    if (!stations.includes(stationId)) {
      setStationId(stations[0] || '');
    }
  }, [stations]);

  useEffect(() => {
    localStorage.setItem('mg_token', token);
  }, [token]);

  const loadData = async () => {
    setLoading(true);
    setError('');
    try {
      const fromIso = toIso(from);
      const toIsoValue = toIso(to);
      const statsUrl = `${baseUrl}/api/v1/stats?station_id=${encodeURIComponent(stationId)}&from=${encodeURIComponent(fromIso)}&to=${encodeURIComponent(toIsoValue)}&granularity=hour`;
      const settlementsUrl = `${baseUrl}/api/v1/settlements?station_id=${encodeURIComponent(stationId)}&from=${encodeURIComponent(fromIso)}&to=${encodeURIComponent(toIsoValue)}`;
      const [statsResp, settlementResp] = await Promise.all([
        fetchJson(statsUrl, token),
        fetchJson(settlementsUrl, token)
      ]);
      setStats(statsResp || []);
      setSettlements(settlementResp || []);
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setLoading(false);
    }
  };

  const handleWindowClose = async () => {
    setWindowCloseStatus('');
    setError('');
    try {
      const resp = await fetch(`${baseUrl}/analytics/window-close`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { Authorization: `Bearer ${token}` } : {})
        },
        body: JSON.stringify({
          stationId: stationId,
          windowStart: toIso(windowStart)
        })
      });
      if (!resp.ok) {
        const text = await resp.text();
        throw new Error(`${resp.status} ${resp.statusText} - ${text}`);
      }
      const data = await resp.json();
      setWindowCloseStatus(`ok: ${data.windowStart} ~ ${data.windowEnd}`);
      await loadData();
    } catch (err) {
      setWindowCloseStatus('');
      setError(err.message || String(err));
    }
  };

  const addStation = () => {
    const trimmed = newStation.trim();
    if (!trimmed) return;
    if (!stations.includes(trimmed)) {
      setStations((prev) => [...prev, trimmed]);
    }
    setNewStation('');
  };

  const removeStation = (id) => {
    setStations((prev) => prev.filter((station) => station !== id));
  };

  useEffect(() => {
    loadData();
  }, []);

  const kpi = useMemo(() => {
    if (!stats.length) return null;
    const totalCharge = stats.reduce((sum, row) => sum + (row.charge_kwh || 0), 0);
    const totalDischarge = stats.reduce((sum, row) => sum + (row.discharge_kwh || 0), 0);
    const totalEarnings = stats.reduce((sum, row) => sum + (row.earnings || 0), 0);
    const totalCarbon = stats.reduce((sum, row) => sum + (row.carbon_reduction || 0), 0);
    return { totalCharge, totalDischarge, totalEarnings, totalCarbon };
  }, [stats]);

  const hourlySeries = useMemo(() => {
    const sorted = [...stats].sort((a, b) => new Date(a.period_start) - new Date(b.period_start));
    return [
      {
        label: 'Charge',
        color: 'var(--accent)',
        values: sorted.map((row) => row.charge_kwh || 0)
      },
      {
        label: 'Discharge',
        color: 'var(--accent-strong)',
        values: sorted.map((row) => row.discharge_kwh || 0)
      }
    ];
  }, [stats]);

  const settlementSeries = useMemo(() => {
    const sorted = [...settlements].sort((a, b) => new Date(a.day_start) - new Date(b.day_start));
    return [
      {
        label: 'Amount',
        color: 'var(--warning)',
        values: sorted.map((row) => row.amount || 0)
      }
    ];
  }, [settlements]);

  return (
    <div className="app">
      <header className="hero">
        <div>
          <p className="eyebrow">Microgrid Cloud / Dev Console</p>
          <h1>站点运行看板</h1>
          <p className="subtitle">最小可交付：数据链路、统计、结算与窗口触发</p>
        </div>
        <div className="hero-card">
          <div className="status-dot" />
          <div>
            <p className="hero-label">当前环境</p>
            <p className="hero-value">{baseUrl}</p>
          </div>
        </div>
      </header>

      <section className="panel grid-3">
        <div className="panel-card">
          <h3>站点配置</h3>
          <label>
            API Base URL
            <input value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} />
          </label>
          <label>
            Station ID
            <select value={stationId} onChange={(e) => setStationId(e.target.value)}>
              {stations.map((station) => (
                <option key={station} value={station}>
                  {station}
                </option>
              ))}
            </select>
          </label>
          <div className="station-input">
            <input
              placeholder="添加站点 ID"
              value={newStation}
              onChange={(e) => setNewStation(e.target.value)}
            />
            <button className="ghost" onClick={addStation}>添加</button>
          </div>
          <div className="station-tags">
            {stations.map((station) => (
              <button key={station} className={`tag-button ${station === stationId ? 'active' : ''}`} onClick={() => setStationId(station)}>
                {station}
                <span
                  className="remove"
                  onClick={(e) => {
                    e.stopPropagation();
                    removeStation(station);
                  }}
                >
                  ×
                </span>
              </button>
            ))}
          </div>
        </div>
        <div className="panel-card">
          <h3>时间范围</h3>
          <label>
            From
            <input type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} />
          </label>
          <label>
            To
            <input type="datetime-local" value={to} onChange={(e) => setTo(e.target.value)} />
          </label>
          <button className="primary" onClick={loadData} disabled={loading}>
            {loading ? '加载中...' : '刷新统计'}
          </button>
        </div>
        <div className="panel-card">
          <h3>认证</h3>
          <label>
            JWT Token
            <textarea rows="4" value={token} onChange={(e) => setToken(e.target.value)} placeholder="粘贴 Bearer token" />
          </label>
        </div>
      </section>

      <section className="panel grid-4">
        <div className="kpi">
          <p>累计充电量 (kWh)</p>
          <h2>{formatNumber(kpi?.totalCharge)}</h2>
        </div>
        <div className="kpi">
          <p>累计放电量 (kWh)</p>
          <h2>{formatNumber(kpi?.totalDischarge)}</h2>
        </div>
        <div className="kpi">
          <p>收益 (CNY)</p>
          <h2>{formatNumber(kpi?.totalEarnings)}</h2>
        </div>
        <div className="kpi">
          <p>减排量</p>
          <h2>{formatNumber(kpi?.totalCarbon)}</h2>
        </div>
      </section>

      <section className="panel grid-2">
        <div className="panel-card">
          <div className="card-header">
            <h3>小时能量趋势</h3>
            <span className="tag">charge / discharge</span>
          </div>
          <LineChart series={hourlySeries} />
          <p className="hint">数据来自 hourly stats</p>
        </div>
        <div className="panel-card">
          <h3>窗口关闭</h3>
          <label>
            Window Start
            <input type="datetime-local" value={windowStart} onChange={(e) => setWindowStart(e.target.value)} />
          </label>
          <button className="primary" onClick={handleWindowClose}>
            触发 window-close
          </button>
          {windowCloseStatus && <p className="ok">{windowCloseStatus}</p>}
        </div>
      </section>

      <section className="panel grid-2">
        <div className="panel-card">
          <h3>小时统计列表</h3>
          <div className="table">
            <div className="row header">
              <span>Period</span>
              <span>Charge</span>
              <span>Discharge</span>
              <span>Earnings</span>
            </div>
            {stats.slice(0, 8).map((row) => (
              <div className="row" key={row.statistic_id}>
                <span>{formatDate(row.period_start)}</span>
                <span>{formatNumber(row.charge_kwh)}</span>
                <span>{formatNumber(row.discharge_kwh)}</span>
                <span>{formatNumber(row.earnings)}</span>
              </div>
            ))}
            {!stats.length && <p className="hint">暂无统计数据</p>}
          </div>
        </div>
        <div className="panel-card">
          <div className="card-header">
            <h3>结算趋势</h3>
            <span className="tag">day amount</span>
          </div>
          <LineChart series={settlementSeries} height={140} />
          <p className="hint">结算金额趋势</p>
        </div>
      </section>

      <section className="panel grid-2">
        <div className="panel-card">
          <h3>结算列表</h3>
          <div className="table">
            <div className="row header">
              <span>Day</span>
              <span>Energy</span>
              <span>Amount</span>
              <span>Status</span>
            </div>
            {settlements.slice(0, 8).map((row) => (
              <div className="row" key={`${row.station_id}-${row.day_start}`}>
                <span>{formatDate(row.day_start)}</span>
                <span>{formatNumber(row.energy_kwh)}</span>
                <span>{formatNumber(row.amount)}</span>
                <span>{row.status || '--'}</span>
              </div>
            ))}
            {!settlements.length && <p className="hint">暂无结算数据</p>}
          </div>
        </div>
      </section>

      {error && (
        <section className="panel">
          <div className="error">{error}</div>
        </section>
      )}

      <footer className="footer">
        <p>Microgrid Cloud Dev Console · React + Vite</p>
      </footer>
    </div>
  );
}
