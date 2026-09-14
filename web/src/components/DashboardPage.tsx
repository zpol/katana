import { useEffect, useState } from 'react'
import { api } from '../api'
import type { StatsSummary } from '../types'
import { BarChart, DonutChart } from './charts'

const SEV_COLORS: Record<string, string> = {
  critical: '#e35d5d',
  high: '#e35d5d',
  medium: '#c5ccd6',
  low: '#4d94ff',
  info: '#3cbe8c',
  unknown: '#8b95a5',
}

export function DashboardPage() {
  const [stats, setStats] = useState<StatsSummary | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      setStats(await api.getStats())
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed to load stats')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  if (loading && !stats) {
    return <div className="status-line">Loading dashboard…</div>
  }
  if (error && !stats) {
    return <div className="status-line">Error: {error}</div>
  }
  if (!stats) return null

  const admissionTotal = stats.deployed + stats.blocked
  const severitySegments = Object.entries(stats.by_severity)
    .sort((a, b) => b[1] - a[1])
    .map(([label, value]) => ({ label, value, color: SEV_COLORS[label] ?? '#8b95a5' }))

  const namespaceItems = Object.entries(stats.by_namespace)
    .sort((a, b) => b[1] - a[1])
    .map(([label, value]) => ({ label, value, color: 'var(--blue-bright)' }))

  const actionSegments = Object.entries(stats.by_policy_action).map(([label, value]) => ({
    label,
    value,
    color: label === 'deny' ? '#e35d5d' : label === 'warn' ? '#e6a23c' : '#4d94ff',
  }))

  return (
    <div className="dashboard">
      <div className="dash-cards">
        <div className="dash-card">
          <span className="dash-card-label">Admission events</span>
          <span className="dash-card-value">{stats.admission_events}</span>
          <span className="dash-card-hint">Pod create/update via webhook</span>
        </div>
        <div className="dash-card dash-card-ok">
          <span className="dash-card-label">Deployed</span>
          <span className="dash-card-value">{stats.deployed}</span>
          <span className="dash-card-hint">{stats.deployed_pct.toFixed(0)}% of admission</span>
        </div>
        <div className="dash-card dash-card-danger">
          <span className="dash-card-label">Blocked</span>
          <span className="dash-card-value">{stats.blocked}</span>
          <span className="dash-card-hint">{stats.blocked_pct.toFixed(0)}% of admission</span>
        </div>
        <div className="dash-card dash-card-warn">
          <span className="dash-card-label">Dry-run would deny</span>
          <span className="dash-card-value">{stats.dry_run_would_deny}</span>
          <span className="dash-card-hint">Allowed but policy = deny</span>
        </div>
        <div className="dash-card">
          <span className="dash-card-label">Total events</span>
          <span className="dash-card-value">{stats.total_events}</span>
          <span className="dash-card-hint">All detections in DB</span>
        </div>
      </div>

      <div className="dash-grid">
        <section className="dash-panel">
          <h3>Deploy outcome</h3>
          <p className="dash-sub">Allowed vs blocked admission decisions ({admissionTotal} total)</p>
          <DonutChart
            center={String(admissionTotal)}
            segments={[
              { label: 'Deployed', value: stats.deployed, color: '#3cbe8c' },
              { label: 'Blocked', value: stats.blocked, color: '#e35d5d' },
            ]}
          />
        </section>

        <section className="dash-panel">
          <h3>Scan coverage</h3>
          <p className="dash-sub">Xray scan status across all events</p>
          <DonutChart
            segments={[
              { label: 'Scanned', value: stats.scanned_count, color: '#3cbe8c' },
              { label: 'Unscanned', value: stats.unscanned_count, color: '#e6a23c' },
            ]}
          />
        </section>

        <section className="dash-panel">
          <h3>By severity</h3>
          <p className="dash-sub">Finding severity distribution</p>
          {severitySegments.length > 0 ? (
            <DonutChart segments={severitySegments} />
          ) : (
            <p className="dash-empty">No data yet</p>
          )}
        </section>

        <section className="dash-panel">
          <h3>Policy actions</h3>
          <p className="dash-sub">Matched policy action (deny / warn / audit)</p>
          {actionSegments.length > 0 ? (
            <DonutChart segments={actionSegments} />
          ) : (
            <p className="dash-empty">No policy actions recorded yet</p>
          )}
        </section>

        <section className="dash-panel dash-panel-wide">
          <h3>Top namespaces</h3>
          <p className="dash-sub">Most active namespaces by event count</p>
          {namespaceItems.length > 0 ? (
            <BarChart items={namespaceItems} />
          ) : (
            <p className="dash-empty">No namespace data yet</p>
          )}
        </section>
      </div>

      <p className="dash-footnote">
        Metrics are derived from KATANA admission events and evaluations stored locally.
        Live cluster pod counts are not queried — redeploy after upgrading to backfill deploy outcome on new events.
      </p>
    </div>
  )
}
