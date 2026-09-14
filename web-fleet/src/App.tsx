import { useCallback, useEffect, useState } from 'react'
import { getHistory, getOverview, type Cluster, type HistoryEvent, type Overview } from './api'
import { ComplianceBar, Donut, HBar } from './charts'

type WindowSel = '1h' | '24h' | '7d'

export default function App() {
  const [windowSel, setWindowSel] = useState<WindowSel>('24h')
  const [ov, setOv] = useState<Overview | null>(null)
  const [history, setHistory] = useState<HistoryEvent[]>([])
  const [error, setError] = useState('')
  const [selected, setSelected] = useState<string | null>(null)
  const [tick, setTick] = useState(0)

  const load = useCallback(async () => {
    try {
      const [o, h] = await Promise.all([getOverview(windowSel), getHistory()])
      setOv(o)
      setHistory(h)
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'load failed')
    }
  }, [windowSel])

  useEffect(() => {
    void load()
    const id = setInterval(() => {
      setTick((t) => t + 1)
      void load()
    }, 15000)
    return () => clearInterval(id)
  }, [load])

  const cluster = ov?.clusters.find((c) => c.name === selected) ?? null

  return (
    <div className="shell">
      <header className="top">
        <div className="brand">
          <span className="mark" />
          <div>
            <h1>KATANA FLEET</h1>
            <p>Multi-cluster admission control · live posture</p>
          </div>
        </div>
        <div className="top-right">
          <div className={`live ${ov?.prometheus_ok ? 'on' : 'off'}`}>
            <span className="pulse" />
            {ov?.prometheus_ok ? 'Prometheus connected' : 'Prometheus degraded'}
          </div>
          <div className="windows">
            {(['1h', '24h', '7d'] as WindowSel[]).map((w) => (
              <button key={w} className={w === windowSel ? 'on' : ''} onClick={() => setWindowSel(w)}>
                {w}
              </button>
            ))}
          </div>
          <button className="ghost" onClick={() => void load()}>
            Refresh
          </button>
        </div>
      </header>

      {error && <div className="banner err">{error}</div>}
      {ov?.prometheus_error && <div className="banner warn">{ov.prometheus_error}</div>}

      <section className="kpis">
        <KPI label="Fleet health" value={`${(ov?.fleet_health_pct ?? 0).toFixed(0)}%`} hint={`${ov?.katana_up ?? 0} / ${(ov?.katana_up ?? 0) + (ov?.katana_down ?? 0)} Katana up`} tone="ok" />
        <KPI label="Compliant deploys" value={`${(ov?.compliant_pct ?? 0).toFixed(0)}%`} hint={`${windowSel} admission posture`} tone="ok" />
        <KPI label="Non-compliant" value={`${(ov?.non_compliant_pct ?? 0).toFixed(0)}%`} hint="blocked · dry-run · unscanned · error" tone="bad" />
        <KPI label="Deny rate" value={`${(ov?.deny_rate ?? 0).toFixed(0)}%`} hint="enforce blocks" tone="warn" />
        <KPI label="Katana down" value={String(ov?.katana_down ?? 0)} hint="clusters without scrape" tone={ov?.katana_down ? 'bad' : 'ok'} />
        <KPI label="Nodes NotReady" value={String(ov?.nodes_not_ready ?? 0)} hint="fleet sum" tone={ov?.nodes_not_ready ? 'warn' : 'ok'} />
      </section>

      <section className="pies">
        <Donut
          caption="Deploy posture"
          center={`${(ov?.compliant_pct ?? 0).toFixed(0)}%`}
          segments={ov?.posture ?? []}
        />
        <Donut caption="Katana health" center={`${ov?.katana_up ?? 0}`} segments={ov?.cluster_health ?? []} />
        <Donut caption="Xray coverage" center="scan" segments={ov?.scan_coverage ?? []} />
      </section>

      <section className="bars">
        <HBar title="Policy denials" segments={ov?.policies ?? []} />
        <HBar title="Severity mix" segments={ov?.severity ?? []} />
        <HBar title="Enforce vs dry-run" segments={ov?.mode ?? []} />
        <HBar title="Node readiness" segments={ov?.nodes ?? []} />
      </section>

      <section className="split">
        <div className="panel">
          <div className="panel-h">
            <h2>Clusters</h2>
            <span className="muted">{ov?.clusters.length ?? 0} registered · tick {tick}</span>
          </div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Cluster</th>
                  <th>Katana</th>
                  <th>Cluster</th>
                  <th>K8s</th>
                  <th>Version</th>
                  <th>Nodes</th>
                  <th>Mode</th>
                  <th>Compliant</th>
                </tr>
              </thead>
              <tbody>
                {(ov?.clusters ?? []).map((c) => (
                  <tr key={c.name} className={selected === c.name ? 'sel' : ''} onClick={() => setSelected(c.name)}>
                    <td className="name">{c.name}</td>
                    <td><StatusDot status={c.katana_status} /></td>
                    <td><StatusDot status={c.cluster_status} /></td>
                    <td className="mono">{c.k8s_version || '—'}</td>
                    <td className="mono">{c.version || '—'}</td>
                    <td className="mono">{c.nodes_ready}/{c.nodes_ready + c.nodes_not_ready}</td>
                    <td>{c.dry_run ? 'dry-run' : 'enforce'}</td>
                    <td><ComplianceBar pct={c.compliant_pct} label="" /></td>
                  </tr>
                ))}
                {(ov?.clusters.length ?? 0) === 0 && (
                  <tr>
                    <td colSpan={8} className="muted">
                      Waiting for Prometheus scrape of Katana /metrics
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <div className="cstack">
            {(ov?.clusters ?? []).map((c) => (
              <ComplianceBar key={c.name} pct={c.compliant_pct} label={c.name} />
            ))}
          </div>
        </div>

        <aside className="panel detail">
          <div className="panel-h">
            <h2>Cluster detail</h2>
          </div>
          {cluster ? <ClusterDetail c={cluster} /> : <p className="muted">Select a cluster</p>}
        </aside>
      </section>

      <section className="panel">
        <div className="panel-h">
          <h2>Deploy history</h2>
          <span className="muted">Katana version changes across the fleet</span>
        </div>
        <ol className="timeline">
          {history.map((h) => (
            <li key={h.id}>
              <span className="when">{formatTs(h.ts)}</span>
              <span className="who">{h.cluster}</span>
              <span className="mono">
                {h.from_version} → {h.to_version}
              </span>
            </li>
          ))}
          {history.length === 0 && <li className="muted">No version changes recorded yet</li>}
        </ol>
      </section>

      <footer>
        Generated {ov?.generated_at ? formatTs(ov.generated_at) : '—'} · window {ov?.window ?? windowSel}
        {ov?.version_drift ? ` · version drift ${ov.version_drift}` : ''}
      </footer>
    </div>
  )
}

function KPI({ label, value, hint, tone }: { label: string; value: string; hint: string; tone: string }) {
  return (
    <div className={`kpi ${tone}`}>
      <span className="k-label">{label}</span>
      <span className="k-value">{value}</span>
      <span className="k-hint">{hint}</span>
    </div>
  )
}

function StatusDot({ status }: { status: string }) {
  return (
    <span className={`sdot ${status}`}>
      <i />
      {status}
    </span>
  )
}

function ClusterDetail({ c }: { c: Cluster }) {
  return (
    <dl className="kv">
      <dt>Name</dt><dd className="mono">{c.name}</dd>
      <dt>Katana</dt><dd><StatusDot status={c.katana_status} /> {c.katana_up ? 'up' : 'down'}</dd>
      <dt>Policies</dt><dd>{c.policies_ready ? 'ready' : 'not ready'}</dd>
      <dt>JFrog</dt><dd>{c.jfrog_up ? 'configured' : 'down'}</dd>
      <dt>Mode</dt><dd>{c.dry_run ? 'dry-run' : 'enforce'}</dd>
      <dt>Katana ver</dt><dd className="mono">{c.version || '—'}</dd>
      <dt>Kubernetes</dt><dd className="mono">{c.k8s_version || '—'}</dd>
      <dt>Nodes</dt><dd>{c.nodes_ready} ready / {c.nodes_not_ready} not ready</dd>
      <dt>Compliant</dt><dd>{c.compliant_pct.toFixed(0)}%</dd>
    </dl>
  )
}

function formatTs(iso: string) {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toISOString().replace('T', ' ').slice(0, 19) + 'Z'
}
