import type { Detection } from '../types'

export function deployOutcome(d: Detection): { label: string; cls: string; title: string } {
  if (d.source === 'evaluate') {
    if (d.policy_action === 'deny') {
      return {
        label: 'Blocked',
        cls: 'outcome-eval-blocked',
        title: 'Evaluate tab: policy would deny this image (no pod was sent to admission)',
      }
    }
    return {
      label: 'Allowed',
      cls: 'outcome-eval-allow',
      title: 'Evaluate tab: policy would allow this image (dry-run only)',
    }
  }
  if (d.source !== 'admission' || d.deployed === undefined) {
    return { label: 'N/A', cls: 'outcome-na', title: 'Legacy or seed event — no admission outcome' }
  }
  if (d.dry_run && d.policy_action === 'deny') {
    return {
      label: 'Dry-run',
      cls: 'outcome-dryrun',
      title: 'Pod was allowed (dry-run mode) but policy would deny',
    }
  }
  if (d.deployed) {
    return { label: 'Deployed', cls: 'outcome-deployed', title: 'Pod was allowed and created in the cluster' }
  }
  return { label: 'Blocked', cls: 'outcome-blocked', title: 'Admission webhook denied the pod' }
}

interface DonutProps {
  segments: { label: string; value: number; color: string }[]
  center?: string
}

export function DonutChart({ segments, center }: DonutProps) {
  const total = segments.reduce((s, x) => s + x.value, 0)
  if (total === 0) {
    return (
      <div className="donut-wrap">
        <div className="donut donut-empty">
          <div className="donut-hole">0</div>
        </div>
      </div>
    )
  }
  let acc = 0
  const gradient = segments
    .filter((s) => s.value > 0)
    .map((s) => {
      const start = (acc / total) * 100
      acc += s.value
      const end = (acc / total) * 100
      return `${s.color} ${start}% ${end}%`
    })
    .join(', ')

  return (
    <div className="donut-wrap">
      <div className="donut" style={{ background: `conic-gradient(${gradient})` }}>
        <div className="donut-hole">{center ?? total}</div>
      </div>
      <ul className="donut-legend">
        {segments.map((s) => (
          <li key={s.label}>
            <span className="dot" style={{ background: s.color }} />
            {s.label} <strong>{s.value}</strong>
            <span className="pct">({total ? Math.round((s.value / total) * 100) : 0}%)</span>
          </li>
        ))}
      </ul>
    </div>
  )
}

interface BarProps {
  items: { label: string; value: number; color?: string }[]
}

export function BarChart({ items }: BarProps) {
  const max = Math.max(...items.map((i) => i.value), 1)
  return (
    <div className="bar-chart">
      {items.map((item) => (
        <div className="bar-row" key={item.label}>
          <span className="bar-label">{item.label}</span>
          <div className="bar-track">
            <div
              className="bar-fill"
              style={{
                width: `${(item.value / max) * 100}%`,
                background: item.color ?? 'var(--blue)',
              }}
            />
          </div>
          <span className="bar-value">{item.value}</span>
        </div>
      ))}
    </div>
  )
}
