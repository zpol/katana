import { useEffect, useState } from 'react'
import { api, type EvaluateResult } from '../api'
import type { Detection } from '../types'
import { deployOutcome } from './charts'
import { XrayProgressBar } from './XrayProgressBar'

interface Props {
  detection: Detection
  onClose: () => void
}

function formatTime(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toISOString().replace('T', ' ').slice(0, 19) + ' UTC'
}

export function ImageDetailModal({ detection, onClose }: Props) {
  const [busy, setBusy] = useState(true)
  const [error, setError] = useState('')
  const [result, setResult] = useState<EvaluateResult | null>(null)

  useEffect(() => {
    let cancelled = false
    const run = async () => {
      setBusy(true)
      setError('')
      try {
        const res = await api.evaluate({
          image: detection.image,
          namespace: detection.namespace,
          environment: detection.environment,
          record: false,
        })
        if (!cancelled) setResult(res)
      } catch (e) {
        if (!cancelled) {
          setResult(null)
          setError(e instanceof Error ? e.message : 'Xray lookup failed')
        }
      } finally {
        if (!cancelled) setBusy(false)
      }
    }
    void run()
    return () => {
      cancelled = true
    }
  }, [detection.image, detection.namespace, detection.environment])

  const scan = result?.scan
  const outcome = deployOutcome(detection)
  const counts = [
    { label: 'Critical', value: scan?.critical ?? 0, cls: 'critical' },
    { label: 'High', value: scan?.high ?? 0, cls: 'high' },
    { label: 'Medium', value: scan?.medium ?? 0, cls: 'medium' },
    { label: 'Low', value: scan?.low ?? 0, cls: 'low' },
  ]

  return (
    <div className="modal-backdrop" onClick={onClose} role="presentation">
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="image-detail-title"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-head">
          <div>
            <h2 id="image-detail-title">Image details</h2>
            <p className="modal-sub">Detection snapshot + live JFrog Xray lookup</p>
          </div>
          <button className="btn ghost" onClick={onClose}>Close</button>
        </div>

        <p className="image-ref" title={detection.image}>{detection.image}</p>

        <div className="kv">
          <span>Time</span><span>{formatTime(detection.created_at)}</span>
          <span>Severity</span>
          <span><span className={`badge ${detection.severity}`}>{detection.severity}</span></span>
          <span>Scanned (event)</span>
          <span>
            <span className={`badge ${detection.scanned ? 'scanned-yes' : 'scanned-no'}`}>
              {detection.scanned ? 'yes' : 'no'}
            </span>
          </span>
          <span>Namespace</span><span>{detection.namespace || '—'}</span>
          <span>Environment</span><span>{detection.environment || '—'}</span>
          <span>Registry</span><span className="mono">{detection.registry || '—'}</span>
          <span>Source</span><span>{detection.source || '—'}</span>
          <span>Outcome</span>
          <span>
            <span className={`badge outcome ${outcome.cls}`} title={outcome.title}>{outcome.label}</span>
          </span>
          <span>Finding</span><span>{detection.title}</span>
          <span>Notes</span><span>{detection.description || '—'}</span>
        </div>

        <h3 className="modal-section">JFrog Xray</h3>
        <XrayProgressBar active={busy} />
        {error && <div className="status-line">Error: {error}</div>}
        {!busy && result && (
          <>
            <div className="kv">
              <span>Live scanned</span>
              <span>
                <span className={`badge ${scan?.scanned ? 'scanned-yes' : 'scanned-no'}`}>
                  {scan?.scanned ? 'yes' : 'no'}
                </span>
              </span>
              <span>Decision</span>
              <span>
                <span className={`badge ${result.decision.allowed ? 'audit' : 'deny'}`}>
                  {result.decision.allowed ? 'allow' : result.decision.action}
                </span>
                {result.decision.matched_policy ? ` · ${result.decision.matched_policy}` : ''}
              </span>
              <span>Reasons</span>
              <span>{(result.decision.reasons || []).join(' · ') || '—'}</span>
              <span>Artifact path</span>
              <span className="mono">{scan?.artifact?.path || '—'}</span>
              <span>SHA</span>
              <span className="mono">{scan?.artifact?.sha || '—'}</span>
            </div>
            {scan?.error && <p className="status-line">Xray: {scan.error}</p>}
            <div className="sev-grid">
              {counts.map((c) => (
                <div key={c.label} className="sev-card">
                  <div className={`badge ${c.cls}`}>{c.label}</div>
                  <div className="sev-count">{c.value}</div>
                </div>
              ))}
            </div>
            <h3 className="modal-section">Issues</h3>
            <div className="violation-list">
              {(scan?.violations || []).length === 0 && (
                <div className="status-line">No issue labels returned.</div>
              )}
              {(scan?.violations || []).map((v) => (
                <div key={v} className="violation-row">{v}</div>
              ))}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
