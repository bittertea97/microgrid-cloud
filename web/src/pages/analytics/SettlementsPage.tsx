import React, { useMemo, useState } from 'react'
import { fetchSettlements } from '../../api/settlementsApi'
import type { SettlementRecord } from '../../types/api'

const formatCell = (value: unknown) => {
  if (value === null || value === undefined || value === '') {
    return '-'
  }
  return String(value)
}

export const AnalyticsSettlementsPage: React.FC = () => {
  const [stationId, setStationId] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [rows, setRows] = useState<SettlementRecord[]>([])
  const [status, setStatus] = useState<'idle' | 'loading' | 'error' | 'empty' | 'ready'>('idle')
  const [error, setError] = useState('')
  const [expandedRows, setExpandedRows] = useState<Set<number>>(() => new Set())

  const toggleRow = (index: number) => {
    setExpandedRows(prev => {
      const next = new Set(prev)
      if (next.has(index)) {
        next.delete(index)
      } else {
        next.add(index)
      }
      return next
    })
  }

  const columnValues = useMemo(() => {
    return rows.map(row => ({
      dayKey: formatCell(row.day_start ?? row.time_key),
      energy: formatCell(row.energy_kwh),
      amount: formatCell(row.amount),
      currency: formatCell(row.currency),
      version: formatCell(row.version),
      updatedAt: formatCell(row.updated_at)
    }))
  }, [rows])

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    setStatus('loading')
    setError('')

    try {
      const data = await fetchSettlements({
        station_id: stationId || undefined,
        from: from || undefined,
        to: to || undefined
      })
      setRows(data)
      if (data.length === 0) {
        setStatus('empty')
      } else {
        setStatus('ready')
      }
    } catch (err) {
      setStatus('error')
      setError(err instanceof Error ? err.message : 'Failed to load settlements.')
    }
  }

  return (
    <div style={{ padding: '24px' }}>
      <h2>Analytics · Settlements</h2>
      <form onSubmit={handleSubmit} style={{ display: 'flex', gap: '12px', flexWrap: 'wrap' }}>
        <label>
          Station ID
          <input
            type="text"
            value={stationId}
            onChange={event => setStationId(event.target.value)}
            placeholder="station-123"
            style={{ display: 'block', marginTop: '4px' }}
          />
        </label>
        <label>
          From
          <input
            type="datetime-local"
            value={from}
            onChange={event => setFrom(event.target.value)}
            style={{ display: 'block', marginTop: '4px' }}
          />
        </label>
        <label>
          To
          <input
            type="datetime-local"
            value={to}
            onChange={event => setTo(event.target.value)}
            style={{ display: 'block', marginTop: '4px' }}
          />
        </label>
        <button type="submit" style={{ alignSelf: 'flex-end', height: '32px' }}>
          Query
        </button>
      </form>

      {status === 'loading' && <p style={{ marginTop: '16px' }}>Loading settlements…</p>}
      {status === 'error' && (
        <p style={{ marginTop: '16px', color: '#b42318' }}>Error: {error}</p>
      )}
      {status === 'empty' && <p style={{ marginTop: '16px' }}>No settlements found.</p>}

      {status === 'ready' && (
        <div style={{ marginTop: '16px', overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                <th style={{ textAlign: 'left', padding: '8px', borderBottom: '1px solid #ddd' }}>
                  Day Start / Time Key
                </th>
                <th style={{ textAlign: 'left', padding: '8px', borderBottom: '1px solid #ddd' }}>
                  Energy (kWh)
                </th>
                <th style={{ textAlign: 'left', padding: '8px', borderBottom: '1px solid #ddd' }}>
                  Amount
                </th>
                <th style={{ textAlign: 'left', padding: '8px', borderBottom: '1px solid #ddd' }}>
                  Currency
                </th>
                <th style={{ textAlign: 'left', padding: '8px', borderBottom: '1px solid #ddd' }}>
                  Version
                </th>
                <th style={{ textAlign: 'left', padding: '8px', borderBottom: '1px solid #ddd' }}>
                  Updated At
                </th>
                <th style={{ textAlign: 'left', padding: '8px', borderBottom: '1px solid #ddd' }}>
                  Raw
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row, index) => {
                const values = columnValues[index]
                const expanded = expandedRows.has(index)
                return (
                  <React.Fragment key={index}>
                    <tr>
                      <td style={{ padding: '8px', borderBottom: '1px solid #eee' }}>{values.dayKey}</td>
                      <td style={{ padding: '8px', borderBottom: '1px solid #eee' }}>{values.energy}</td>
                      <td style={{ padding: '8px', borderBottom: '1px solid #eee' }}>{values.amount}</td>
                      <td style={{ padding: '8px', borderBottom: '1px solid #eee' }}>{values.currency}</td>
                      <td style={{ padding: '8px', borderBottom: '1px solid #eee' }}>{values.version}</td>
                      <td style={{ padding: '8px', borderBottom: '1px solid #eee' }}>{values.updatedAt}</td>
                      <td style={{ padding: '8px', borderBottom: '1px solid #eee' }}>
                        <button type="button" onClick={() => toggleRow(index)}>
                          {expanded ? 'Hide' : 'Show'} JSON
                        </button>
                      </td>
                    </tr>
                    {expanded && (
                      <tr>
                        <td colSpan={7} style={{ padding: '8px', background: '#f9fafb' }}>
                          <pre style={{ margin: 0, fontSize: '12px' }}>
                            {JSON.stringify(row, null, 2)}
                          </pre>
                        </td>
                      </tr>
                    )}
                  </React.Fragment>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
