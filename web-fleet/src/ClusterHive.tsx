import { useMemo } from 'react'
import type { Cluster } from './api'

export type HiveTone = 'green' | 'amber' | 'red' | 'muted'

const TONE_FILL: Record<HiveTone, string> = {
  green: '#2f9e72',
  amber: '#d4922e',
  red: '#d45454',
  muted: '#3a4556',
}

/** Worst of Katana + cluster status drives the hex fill. */
export function clusterTone(c: Cluster): HiveTone {
  const ranks: Record<string, number> = { green: 0, amber: 1, red: 2 }
  const a = ranks[c.katana_status] ?? 2
  const b = ranks[c.cluster_status] ?? 2
  const worst = Math.max(a, b)
  if (!c.katana_up) return 'red'
  if (worst >= 2) return 'red'
  if (worst === 1 || c.compliant_pct < 50) return 'amber'
  return 'green'
}

export function shortClusterName(name: string): string {
  if (name.length <= 11) return name
  const parts = name.split(/[-_./]/).filter(Boolean)
  if (parts.length >= 2) {
    const s = parts.map((p) => p.slice(0, 3)).join('-')
    return s.length <= 11 ? s : s.slice(0, 10)
  }
  return name.slice(0, 10) + '…'
}

/**
 * Pointy-top honeycomb — redblobgames "odd-r" layout.
 * Centers share edges; a 2px panel-colored stroke creates the thin gutter
 * between fills (same look as Portainer honeycomb tiles).
 */
const R = 36
const HEX_W = Math.sqrt(3) * R
const HEX_H = 2 * R
const COLS = 10
const DX = HEX_W
const DY = 1.5 * R
const PANEL_BG = '#121821'
const HEX_STROKE = 2

/** Pointy-top corners (vertex up). Angles −30°, 30°, 90°, … */
function pointyTopPoints(r: number): string {
  const pts: string[] = []
  for (let i = 0; i < 6; i++) {
    const a = ((60 * i - 30) * Math.PI) / 180
    pts.push(`${(r * Math.cos(a)).toFixed(3)},${(r * Math.sin(a)).toFixed(3)}`)
  }
  return pts.join(' ')
}

const HEX_POINTS = pointyTopPoints(R)

type Props = {
  clusters: Cluster[]
  selected: string | null
  onSelect: (name: string) => void
}

export function ClusterHive({ clusters, selected, onSelect }: Props) {
  const { width, height, cells } = useMemo(() => {
    const rows = Math.max(1, Math.ceil(clusters.length / COLS))
    const width = Math.ceil(DX * (COLS - 0.5) + HEX_W + 16)
    const height = Math.ceil(DY * (rows - 1) + HEX_H + 16)
    const cells = clusters.map((c, i) => {
      const col = i % COLS
      const row = Math.floor(i / COLS)
      // odd-r: odd rows shifted right by half hex width
      const cx = 8 + HEX_W / 2 + DX * (col + (row % 2 === 1 ? 0.5 : 0))
      const cy = 8 + HEX_H / 2 + DY * row
      return { c, cx, cy, tone: clusterTone(c) }
    })
    return { width, height, cells }
  }, [clusters])

  return (
    <div className="hive" role="list" aria-label="Cluster status map">
      {clusters.length === 0 ? (
        <p className="muted hive-empty">No clusters scraped yet</p>
      ) : (
        <svg className="hive-svg" width={width} height={height} viewBox={`0 0 ${width} ${height}`}>
          {cells.map(({ c, cx, cy, tone }) => {
            const active = selected === c.name
            return (
              <g
                key={c.name}
                role="listitem"
                className={`hex-g${active ? ' on' : ''}`}
                transform={`translate(${cx} ${cy})`}
                onClick={() => onSelect(c.name)}
              >
                <title>
                  {`${c.name}\nKatana: ${c.katana_status} · Cluster: ${c.cluster_status}\nCompliant: ${c.compliant_pct.toFixed(0)}% · ${c.dry_run ? 'dry-run' : 'enforce'}`}
                </title>
                <polygon
                  points={HEX_POINTS}
                  fill={TONE_FILL[tone]}
                  stroke={PANEL_BG}
                  strokeWidth={HEX_STROKE}
                  strokeLinejoin="round"
                />
                <text className="hex-name" textAnchor="middle" dominantBaseline="central" y={-5}>
                  {shortClusterName(c.name)}
                </text>
                <text className="hex-meta" textAnchor="middle" dominantBaseline="central" y={10}>
                  {c.compliant_pct.toFixed(0)}%
                </text>
              </g>
            )
          })}
        </svg>
      )}
    </div>
  )
}
