import { useState } from 'react'
import { api, type EvaluateResult } from '../api'
import { XrayProgressBar } from './XrayProgressBar'

export function EvaluatePage() {
  const [image, setImage] = useState('evil.example/app:1.0')
  const [namespace, setNamespace] = useState('demo')
  const [environment, setEnvironment] = useState('dev')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<EvaluateResult | null>(null)

  const run = async () => {
    setBusy(true)
    setError('')
    try {
      setResult(await api.evaluate({ image, namespace, environment, record: true }))
    } catch (e) {
      setResult(null)
      setError(e instanceof Error ? e.message : 'evaluate failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="settings-grid">
      <div className="settings-card">
        <h3>Evaluate image</h3>
        <p>Runs policy evaluation using Xray data when JFROG_* is configured on the server.</p>
        <p className="field-hint">
          Xray lookup can take 30–90s depending on the image. Wait for the result; do not refresh.
          Port-forward: <code>kubectl -n katana-system port-forward svc/katana 8443:443</code> then open{' '}
          <code>https://127.0.0.1:8443</code> (HTTPS).
        </p>
        <div className="field">
          <label>Image</label>
          <input value={image} onChange={(e) => setImage(e.target.value)} />
        </div>
        <div className="field row" style={{ marginTop: '0.6rem' }}>
          <div className="field">
            <label>Namespace</label>
            <input value={namespace} onChange={(e) => setNamespace(e.target.value)} />
          </div>
          <div className="field">
            <label>Environment</label>
            <input value={environment} onChange={(e) => setEnvironment(e.target.value)} />
          </div>
        </div>
        <div style={{ marginTop: '0.8rem' }}>
          <button className="btn primary" disabled={busy || !image} onClick={() => void run()}>
            {busy ? 'Evaluating…' : 'Evaluate'}
          </button>
        </div>
        <XrayProgressBar active={busy} />
        {error && <div className="status-line">Error: {error}</div>}
      </div>

      {result && (
        <div className="settings-card">
          <h3>Decision</h3>
          <p>
            <span className={`badge ${result.decision.allowed ? 'audit' : 'deny'}`}>
              {result.decision.allowed ? 'allow' : 'deny'}
            </span>{' '}
            action=<strong>{result.decision.action}</strong>
            {result.decision.matched_policy ? ` · ${result.decision.matched_policy}` : ''}
          </p>
          <p style={{ color: 'var(--muted)' }}>
            {result.decision.user_message || (result.decision.reasons || []).join(' · ')}
          </p>
          <h3 style={{ marginTop: '1rem' }}>Scan</h3>
          <p>
            status={result.scan?.lookup_status || 'unknown'} · scanned={String(result.scan?.scanned)} · C/H/M/L=
            {result.scan?.critical}/{result.scan?.high}/{result.scan?.medium}/{result.scan?.low}
          </p>
          {result.scan?.error && <p style={{ color: 'var(--danger)' }}>{result.scan.error}</p>}
          {(result.scan?.violations || []).slice(0, 8).map((v) => (
            <div key={v} style={{ fontSize: '0.85rem', color: 'var(--muted)' }}>
              · {v}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
