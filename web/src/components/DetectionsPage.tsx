import { useState } from 'react'
import type { Detection } from '../types'
import { deployOutcome } from './charts'
import { ImageDetailModal } from './ImageDetailModal'

interface Filters {
  severity: string
  environment: string
  namespace: string
  registry: string
  outcome: string
  q: string
}

interface Props {
  detections: Detection[]
  filters: Filters
  onFilters: (f: Filters) => void
}

function formatTime(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function DetectionsPage({ detections, filters, onFilters }: Props) {
  const [selected, setSelected] = useState<Detection | null>(null)

  return (
    <>
      <div className="filters">
        <input
          placeholder="Search title, image, description"
          value={filters.q}
          onChange={(e) => onFilters({ ...filters, q: e.target.value })}
          style={{ minWidth: 260, flex: 1 }}
        />
        <select
          value={filters.outcome}
          onChange={(e) => onFilters({ ...filters, outcome: e.target.value })}
        >
          <option value="">All outcomes</option>
          <option value="deployed">Deployed</option>
          <option value="blocked">Blocked (admission)</option>
          <option value="eval-blocked">Blocked (evaluate)</option>
          <option value="dry-run">Dry-run (would deny)</option>
        </select>
        <select
          value={filters.severity}
          onChange={(e) => onFilters({ ...filters, severity: e.target.value })}
        >
          <option value="">All severities</option>
          <option value="critical">critical</option>
          <option value="high">high</option>
          <option value="medium">medium</option>
          <option value="low">low</option>
          <option value="info">info</option>
        </select>
        <input
          placeholder="Environment"
          value={filters.environment}
          onChange={(e) => onFilters({ ...filters, environment: e.target.value })}
        />
        <input
          placeholder="Namespace"
          value={filters.namespace}
          onChange={(e) => onFilters({ ...filters, namespace: e.target.value })}
        />
        <input
          placeholder="Registry"
          value={filters.registry}
          onChange={(e) => onFilters({ ...filters, registry: e.target.value })}
        />
        <span className="table-meta">{detections.length.toLocaleString()} events</span>
      </div>
      <div className="table-wrap">
        <table className="data-table">
          <colgroup>
            <col style={{ width: '10.5rem' }} />
            <col style={{ width: '7.5rem' }} />
            <col style={{ width: '6.8rem' }} />
            <col style={{ width: '6.2rem' }} />
            <col style={{ width: '20%' }} />
            <col style={{ width: '24%' }} />
            <col style={{ width: '9rem' }} />
            <col style={{ width: '6.5rem' }} />
            <col style={{ width: '14%' }} />
            <col style={{ width: '7rem' }} />
          </colgroup>
          <thead>
            <tr>
              <th>Time</th>
              <th>Outcome</th>
              <th>Severity</th>
              <th>Scanned</th>
              <th>Title</th>
              <th>Image</th>
              <th>Namespace</th>
              <th>Env</th>
              <th>Registry</th>
              <th>Source</th>
            </tr>
          </thead>
          <tbody>
            {detections.map((d) => {
              const tip = [d.title, d.description].filter(Boolean).join(' — ')
              const outcome = deployOutcome(d)
              return (
                <tr key={d.id}>
                  <td className="mono" title={d.created_at}>{formatTime(d.created_at)}</td>
                  <td>
                    <span className={`badge outcome ${outcome.cls}`} title={outcome.title}>
                      {outcome.label}
                    </span>
                  </td>
                  <td><span className={`badge ${d.severity}`}>{d.severity}</span></td>
                  <td>
                    <span className={`badge ${d.scanned ? 'scanned-yes' : 'scanned-no'}`}>
                      {d.scanned ? 'yes' : 'no'}
                    </span>
                  </td>
                  <td className="clip" title={tip}>{d.title}</td>
                  <td className="clip mono" title={`${d.image} — click for Xray details`}>
                    <button className="image-link" type="button" onClick={() => setSelected(d)}>
                      {d.image}
                    </button>
                  </td>
                  <td className="clip" title={d.namespace}>{d.namespace || '—'}</td>
                  <td className="clip" title={d.environment}>{d.environment || '—'}</td>
                  <td className="clip mono" title={d.registry}>{d.registry || '—'}</td>
                  <td className="clip" title={d.source}>{d.source || '—'}</td>
                </tr>
              )
            })}
            {detections.length === 0 && (
              <tr><td colSpan={10}>No detections match filters.</td></tr>
            )}
          </tbody>
        </table>
      </div>
      {selected && (
        <ImageDetailModal detection={selected} onClose={() => setSelected(null)} />
      )}
    </>
  )
}
