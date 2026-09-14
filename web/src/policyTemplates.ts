import type { MatchCriteria, Policy, PolicyAction } from './types'

export interface PolicyTemplate {
  id: string
  label: string
  description: string
  action: PolicyAction
  match: MatchCriteria
  deny_message?: string
  warn_message?: string
  exceptions: string[]
}

const defaultExceptions = ['kube-system', 'kube-public', 'kube-node-lease', 'katana-system']

export const MESSAGE_VARS = '{policy} {image} {severity} {environment} {namespace} {registry} {reason}'

export const POLICY_TEMPLATES: PolicyTemplate[] = [
  {
    id: 'block-critical',
    label: 'Block Critical CVEs',
    description: 'Deny workloads with any Critical CVE (system NS excepted)',
    action: 'deny',
    match: { severity: 'critical' },
    deny_message: `Image {image} has CRITICAL vulnerabilities (JFrog Xray). Policy: {policy}.
Upgrade the base image or contact your platform security team for an exception.`,
    exceptions: [...defaultExceptions],
  },
  {
    id: 'block-high-prod',
    label: 'Block High in Production',
    description: 'Deny High severity findings in production environments',
    action: 'deny',
    match: { severity: 'high', environment: 'prod' },
    deny_message: `Image {image} has HIGH severity findings in production (env: {environment}). Policy: {policy}.
Use a patched image version or deploy to a non-prod namespace for testing.`,
    exceptions: [...defaultExceptions],
  },
  {
    id: 'require-scanned',
    label: 'Require Scanned Image',
    description: 'Deny images with no Xray scan result',
    action: 'deny',
    match: { scanned: false },
    deny_message: `Image {image} is not indexed in JFrog Xray (scanned=false). Policy: {policy}.
Ensure the image is scanned in Artifactory/Xray before deploying.`,
    exceptions: [...defaultExceptions],
  },
  {
    id: 'registry-allowlist',
    label: 'Registry Allowlist',
    description: 'Deny images not from approved registries',
    action: 'deny',
    match: {
      registry_allowlist: [
        'artifactory.example.com',
        '123456789012.dkr.ecr.us-east-1.amazonaws.com',
      ],
    },
    deny_message: `Image {image} uses registry "{registry}" which is not on the approved list. Policy: {policy}.
Pull from artifactory.example.com or an approved registry.`,
    exceptions: [...defaultExceptions],
  },
  {
    id: 'pod-security',
    label: 'Deny Unsafe Pod Security',
    description: 'Deny privileged containers, root (UID 0), or privilege escalation',
    action: 'deny',
    match: { unsafe_pod_security: true },
    deny_message: `Pod security violation in namespace {namespace}. Policy: {policy}. {reason}
Do not run as root (UID 0), privileged, or with allowPrivilegeEscalation.`,
    exceptions: [...defaultExceptions],
  },
  {
    id: 'system-ns-audit',
    label: 'Audit System Namespaces',
    description: 'Audit-only log in platform namespaces. Does not allow or block; deny exceptions do that.',
    action: 'audit',
    match: {
      namespace_allowlist: ['kube-system', 'kube-public', 'kube-node-lease', 'katana-system'],
    },
    warn_message: 'System namespace activity recorded. Policy: {policy}.',
    exceptions: [],
  },
  {
    id: 'custom',
    label: 'Custom rule',
    description: 'Build your own match criteria',
    action: 'deny',
    match: {},
    deny_message: '',
    exceptions: [...defaultExceptions],
  },
]

export function applyTemplate(templateId: string, current: Policy): Policy {
  const t = POLICY_TEMPLATES.find((x) => x.id === templateId)
  if (!t || templateId === 'custom') return current
  return {
    ...current,
    name: t.label,
    description: t.description,
    action: t.action,
    match: { ...t.match },
    exceptions: [...t.exceptions],
    deny_message: t.deny_message ?? '',
    warn_message: t.warn_message ?? '',
  }
}

export function detectTemplate(policy: Policy): string {
  for (const t of POLICY_TEMPLATES) {
    if (t.id === 'custom') continue
    if (JSON.stringify(t.match) === JSON.stringify(policy.match) && t.action === policy.action) {
      return t.id
    }
  }
  return 'custom'
}

export function cleanMatch(match: MatchCriteria): MatchCriteria {
  const out: MatchCriteria = {}
  if (match.severity) out.severity = match.severity
  if (match.environment) out.environment = match.environment
  if (match.scanned !== undefined) out.scanned = match.scanned
  if (match.unsafe_pod_security !== undefined) out.unsafe_pod_security = match.unsafe_pod_security
  if (match.privileged !== undefined) out.privileged = match.privileged
  if (match.run_as_root !== undefined) out.run_as_root = match.run_as_root
  if (match.allow_privilege_escalation !== undefined) {
    out.allow_privilege_escalation = match.allow_privilege_escalation
  }
  if (match.namespace_allowlist?.length) out.namespace_allowlist = [...match.namespace_allowlist]
  if (match.registry_allowlist?.length) out.registry_allowlist = [...match.registry_allowlist]
  return out
}
