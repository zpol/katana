export type PolicyAction = 'deny' | 'warn' | 'audit'

export interface MatchCriteria {
  severity?: string
  environment?: string
  scanned?: boolean
  namespace_allowlist?: string[]
  registry_allowlist?: string[]
  unsafe_pod_security?: boolean
  privileged?: boolean
  run_as_root?: boolean
  allow_privilege_escalation?: boolean
}

export interface Policy {
  id: string
  name: string
  description: string
  enabled: boolean
  action: PolicyAction
  match: MatchCriteria
  exceptions: string[]
  deny_message?: string
  warn_message?: string
  created_at: string
  updated_at: string
}

export interface Detection {
  id: string
  title: string
  description: string
  severity: string
  environment: string
  namespace: string
  registry: string
  image: string
  scanned: boolean
  source: string
  policy_action?: string
  deployed?: boolean
  dry_run?: boolean
  created_at: string
}

export interface StatsSummary {
  total_events: number
  admission_events: number
  deployed: number
  blocked: number
  dry_run_would_deny: number
  deployed_pct: number
  blocked_pct: number
  by_severity: Record<string, number>
  by_namespace: Record<string, number>
  by_policy_action: Record<string, number>
  scanned_count: number
  unscanned_count: number
}
