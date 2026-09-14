export type Segment = { label: string; value: number; pct: number; color?: string }

export type Cluster = {
  name: string
  katana_status: string
  cluster_status: string
  katana_up: boolean
  policies_ready: boolean
  jfrog_up: boolean
  dry_run: boolean
  version: string
  k8s_version: string
  nodes_ready: number
  nodes_not_ready: number
  compliant: number
  non_compliant: number
  compliant_pct: number
  last_seen_unix: number
}

export type Overview = {
  window: string
  generated_at: string
  prometheus_ok: boolean
  prometheus_error?: string
  fleet_health_pct: number
  katana_up: number
  katana_down: number
  compliant_pct: number
  non_compliant_pct: number
  deny_rate: number
  xray_unavailable: number
  nodes_not_ready: number
  version_drift: number
  posture: Segment[]
  cluster_health: Segment[]
  scan_coverage: Segment[]
  policies: Segment[]
  severity: Segment[]
  mode: Segment[]
  nodes: Segment[]
  clusters: Cluster[]
}

export type HistoryEvent = {
  id: number
  ts: string
  cluster: string
  from_version: string
  to_version: string
}

export async function getOverview(window: string): Promise<Overview> {
  const r = await fetch(`/api/v1/fleet/overview?window=${encodeURIComponent(window)}`)
  if (!r.ok) throw new Error(`overview ${r.status}`)
  return r.json()
}

export async function getHistory(): Promise<HistoryEvent[]> {
  const r = await fetch('/api/v1/history')
  if (!r.ok) throw new Error(`history ${r.status}`)
  const d = await r.json()
  return d.deploys || []
}
