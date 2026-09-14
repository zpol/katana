import type { Segment } from './api'

export function Donut({ segments, center, caption }: { segments: Segment[]; center: string; caption: string }) {
  const total = segments.reduce((s, x) => s + (x.value > 0 ? x.value : 0), 0)
  const colors = segments.map((s, i) => s.color || PALETTE[i % PALETTE.length])
  let acc = 0
  const gradient =
    total <= 0
      ? 'var(--line) 0 100%'
      : segments
          .map((s, i) => {
            if (s.value <= 0) return ''
            const start = (acc / total) * 100
            acc += s.value
            const end = (acc / total) * 100
            return `${colors[i]} ${start}% ${end}%`
          })
          .filter(Boolean)
          .join(', ')

  return (
    <div className="chart-card">
      <h3>{caption}</h3>
      <div className="donut-wrap">
        <div className={`donut ${total <= 0 ? 'empty' : ''}`} style={{ background: `conic-gradient(${gradient})` }}>
          <div className="donut-hole">
            <strong>{center}</strong>
          </div>
        </div>
        <ul className="legend">
          {segments.map((s, i) => (
            <li key={s.label}>
              <span className="dot" style={{ background: colors[i] }} />
              <span className="lbl">{s.label.replace(/_/g, ' ')}</span>
              <span className="val">{formatNum(s.value)}</span>
              <span className="pct">{s.pct.toFixed(0)}%</span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}

export function HBar({ segments, title }: { segments: Segment[]; title: string }) {
  const max = Math.max(...segments.map((s) => s.value), 1)
  return (
    <div className="chart-card">
      <h3>{title}</h3>
      <div className="hbar">
        {segments.map((s, i) => (
          <div className="hbar-row" key={s.label}>
            <span className="lbl">{s.label.replace(/_/g, ' ')}</span>
            <div className="track">
              <div
                className="fill"
                style={{
                  width: `${(s.value / max) * 100}%`,
                  background: s.color || PALETTE[i % PALETTE.length],
                }}
              />
            </div>
            <span className="val">
              {formatNum(s.value)} · {s.pct.toFixed(0)}%
            </span>
          </div>
        ))}
        {segments.length === 0 && <p className="muted">No data in this window</p>}
      </div>
    </div>
  )
}

export function ComplianceBar({ pct, label }: { pct: number; label: string }) {
  const cls = pct >= 90 ? 'ok' : pct >= 70 ? 'warn' : 'bad'
  return (
    <div className="cbar">
      <div className="cbar-meta">
        <span>{label}</span>
        <strong className={cls}>{pct.toFixed(0)}%</strong>
      </div>
      <div className="cbar-track">
        <div className={`cbar-fill ${cls}`} style={{ width: `${Math.min(100, Math.max(0, pct))}%` }} />
      </div>
    </div>
  )
}

const PALETTE = ['#2d7dff', '#3cbe8c', '#e6a23c', '#e35d5d', '#c5ccd6', '#9aa3b0']

function formatNum(n: number) {
  if (n >= 1000) return n.toFixed(0)
  if (n >= 10) return n.toFixed(0)
  return n.toFixed(n >= 1 ? 0 : 1)
}
