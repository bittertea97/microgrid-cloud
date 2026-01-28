import React, { useState } from 'react'
import { downloadSettlementsCsv } from '../../api/exportsApi'
import { sanitizeFilename, saveBlob } from '../../utils/download'

export const AnalyticsExportsPage: React.FC = () => {
  const [stationId, setStationId] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [status, setStatus] = useState<'idle' | 'loading' | 'error' | 'done'>('idle')
  const [error, setError] = useState('')

  const handleDownload = async () => {
    setStatus('loading')
    setError('')

    try {
      const blob = await downloadSettlementsCsv({
        station_id: stationId || undefined,
        from: from || undefined,
        to: to || undefined
      })

      const rawName = `settlements_${stationId}_${from}_${to}.csv`
      const filename = sanitizeFilename(rawName) || 'settlements.csv'
      saveBlob(blob, filename)
      setStatus('done')
    } catch (err) {
      setStatus('error')
      setError(err instanceof Error ? err.message : 'Failed to download CSV.')
    }
  }

  return (
    <div style={{ padding: '24px' }}>
      <h2>Analytics · Exports</h2>
      <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap', marginBottom: '12px' }}>
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
      </div>
      <button type="button" onClick={handleDownload} disabled={status === 'loading'}>
        {status === 'loading' ? 'Downloading…' : 'Download settlements CSV'}
      </button>

      {status === 'error' && (
        <p style={{ marginTop: '12px', color: '#b42318' }}>Error: {error}</p>
      )}
      {status === 'done' && <p style={{ marginTop: '12px' }}>Download started.</p>}
    </div>
  )
}
