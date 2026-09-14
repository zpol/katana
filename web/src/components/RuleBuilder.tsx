import { useMemo, useState } from 'react'
import type { MatchCriteria, Policy } from '../types'
import {
  MESSAGE_VARS,
  POLICY_TEMPLATES,
  applyTemplate,
  cleanMatch,
  detectTemplate,
} from '../policyTemplates'

interface Props {
  draft: Policy
  onChange: (p: Policy) => void
}

type RuleKind = 'image' | 'pod_security' | 'registry' | 'namespace' | 'custom'

function inferRuleKind(match: MatchCriteria): RuleKind {
  if (match.unsafe_pod_security || match.privileged || match.run_as_root || match.allow_privilege_escalation) {
    return 'pod_security'
  }
  if (match.registry_allowlist?.length) return 'registry'
  if (match.namespace_allowlist?.length) return 'namespace'
  if (match.severity || match.environment || match.scanned !== undefined) return 'image'
  return 'custom'
}

export function RuleBuilder({ draft, onChange }: Props) {
  const [templateId, setTemplateId] = useState(() => detectTemplate(draft))
  const [ruleKind, setRuleKind] = useState<RuleKind>(() => inferRuleKind(draft.match))
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [jsonErr, setJsonErr] = useState('')

  const update = (patch: Partial<Policy>) => onChange({ ...draft, ...patch })

  const updateMatch = (patch: Partial<MatchCriteria>) => {
    onChange({ ...draft, match: cleanMatch({ ...draft.match, ...patch }) })
  }

  const templateOptions = useMemo(() => POLICY_TEMPLATES, [])

  const onTemplateChange = (id: string) => {
    setTemplateId(id)
    const next = applyTemplate(id, draft)
    onChange(next)
    setRuleKind(inferRuleKind(next.match))
  }

  const onRuleKindChange = (kind: RuleKind) => {
    setRuleKind(kind)
    setTemplateId('custom')
    const base: MatchCriteria = {}
    switch (kind) {
      case 'image':
        updateMatch({ ...base, severity: 'critical' })
        break
      case 'pod_security':
        updateMatch({ ...base, unsafe_pod_security: true })
        break
      case 'registry':
        updateMatch({
          ...base,
          registry_allowlist: ['artifactory.example.com', '123456789012.dkr.ecr.us-east-1.amazonaws.com'],
        })
        break
      case 'namespace':
        updateMatch({
          ...base,
          namespace_allowlist: ['kube-system', 'kube-public', 'kube-node-lease', 'katana-system'],
        })
        break
      default:
        updateMatch(base)
    }
  }

  const listField = (label: string, key: 'namespace_allowlist' | 'registry_allowlist') => (
    <div className="field">
      <label>{label}</label>
      <textarea
        value={(draft.match[key] ?? []).join('\n')}
        onChange={(e) =>
          updateMatch({
            [key]: e.target.value
              .split(/[\n,]+/)
              .map((s) => s.trim())
              .filter(Boolean),
          })
        }
      />
    </div>
  )

  return (
    <div className="rule-builder">
      <div className="field">
        <label>Start from template</label>
        <select value={templateId} onChange={(e) => onTemplateChange(e.target.value)}>
          {templateOptions.map((t) => (
            <option key={t.id} value={t.id}>
              {t.label}
            </option>
          ))}
        </select>
      </div>

      <div className="field">
        <label>Rule type</label>
        <select value={ruleKind} onChange={(e) => onRuleKindChange(e.target.value as RuleKind)}>
          <option value="image">Image / Xray vulnerability</option>
          <option value="pod_security">Pod security context</option>
          <option value="registry">Registry allowlist</option>
          <option value="namespace">Namespace allowlist</option>
          <option value="custom">Custom (JSON)</option>
        </select>
      </div>

      {ruleKind === 'image' && (
        <div className="rule-section">
          <h3>When image matches</h3>
          <div className="field row">
            <div className="field">
              <label>Severity</label>
              <select
                value={draft.match.severity ?? ''}
                onChange={(e) => updateMatch({ severity: e.target.value || undefined })}
              >
                <option value="">Any</option>
                <option value="critical">Critical</option>
                <option value="high">High</option>
                <option value="medium">Medium</option>
                <option value="low">Low</option>
              </select>
            </div>
            <div className="field">
              <label>Environment</label>
              <select
                value={draft.match.environment ?? ''}
                onChange={(e) => updateMatch({ environment: e.target.value || undefined })}
              >
                <option value="">Any</option>
                <option value="prod">Production</option>
                <option value="dev">Development</option>
                <option value="staging">Staging</option>
              </select>
            </div>
          </div>
          <div className="field checkbox-field">
            <label>
              <input
                type="checkbox"
                checked={draft.match.scanned === false}
                onChange={(e) =>
                  updateMatch({ scanned: e.target.checked ? false : undefined })
                }
              />
              Image is not scanned in Xray (scanned=false)
            </label>
          </div>
        </div>
      )}

      {ruleKind === 'pod_security' && (
        <div className="rule-section">
          <h3>When pod security matches</h3>
          <div className="field checkbox-field">
            <label>
              <input
                type="checkbox"
                checked={draft.match.unsafe_pod_security === true}
                onChange={(e) =>
                  updateMatch({ unsafe_pod_security: e.target.checked ? true : undefined })
                }
              />
              Any unsafe pod security (root, privileged, privilege escalation)
            </label>
          </div>
          <div className="field checkbox-field">
            <label>
              <input
                type="checkbox"
                checked={draft.match.privileged === true}
                onChange={(e) => updateMatch({ privileged: e.target.checked ? true : undefined })}
              />
              Privileged container
            </label>
          </div>
          <div className="field checkbox-field">
            <label>
              <input
                type="checkbox"
                checked={draft.match.run_as_root === true}
                onChange={(e) => updateMatch({ run_as_root: e.target.checked ? true : undefined })}
              />
              Runs as root (UID 0)
            </label>
          </div>
          <div className="field checkbox-field">
            <label>
              <input
                type="checkbox"
                checked={draft.match.allow_privilege_escalation === true}
                onChange={(e) =>
                  updateMatch({ allow_privilege_escalation: e.target.checked ? true : undefined })
                }
              />
              allowPrivilegeEscalation is true
            </label>
          </div>
        </div>
      )}

      {ruleKind === 'registry' && listField('Approved registries (one per line)', 'registry_allowlist')}
      {ruleKind === 'namespace' && listField('Namespaces (one per line; supports prefix wildcards like platform-*)', 'namespace_allowlist')}

      <div className="rule-section">
        <h3>Developer message</h3>
        <p className="field-hint">
          Shown in <code>kubectl apply</code> when this policy matches. Variables: <code>{MESSAGE_VARS}</code>
        </p>
        {draft.action === 'deny' && (
          <div className="field">
            <label>Deny message</label>
            <textarea
              className="code"
              rows={5}
              value={draft.deny_message ?? ''}
              onChange={(e) => update({ deny_message: e.target.value })}
              placeholder="Image {image} blocked by {policy}. Upgrade the base image."
            />
          </div>
        )}
        {(draft.action === 'warn' || draft.action === 'audit') && (
          <div className="field">
            <label>Warn / audit message</label>
            <textarea
              className="code"
              rows={3}
              value={draft.warn_message ?? ''}
              onChange={(e) => update({ warn_message: e.target.value })}
            />
          </div>
        )}
      </div>

      <div className="field">
        <button type="button" className="btn ghost" onClick={() => setShowAdvanced((v) => !v)}>
          {showAdvanced ? 'Hide' : 'Show'} advanced JSON
        </button>
      </div>

      {showAdvanced && (
        <div className="field">
          <label>Match JSON (power users)</label>
          <textarea
            className="code"
            value={JSON.stringify(draft.match, null, 2)}
            onChange={(e) => {
              try {
                update({ match: JSON.parse(e.target.value) })
                setJsonErr('')
              } catch {
                setJsonErr('invalid match JSON')
              }
            }}
          />
          {jsonErr && <div className="status-line">Error: {jsonErr}</div>}
        </div>
      )}
    </div>
  )
}
