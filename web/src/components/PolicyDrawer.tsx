import { useState } from 'react'
import type { Policy, PolicyAction } from '../types'
import { RuleBuilder } from './RuleBuilder'
import { ToggleSwitch } from './ToggleSwitch'

interface Props {
  policy: Policy
  isNew?: boolean
  onClose: () => void
  onSave: (p: Policy) => Promise<void>
}

export function PolicyDrawer({ policy, isNew, onClose, onSave }: Props) {
  const [draft, setDraft] = useState<Policy>({
    ...policy,
    exceptions: policy.exceptions ?? [],
    deny_message: policy.deny_message ?? '',
    warn_message: policy.warn_message ?? '',
  })
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState('')

  const submit = async () => {
    setSaving(true)
    setErr('')
    try {
      await onSave(draft)
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'save failed')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <aside className="drawer drawer-wide" onClick={(e) => e.stopPropagation()}>
        <h2>{isNew ? 'Create policy' : 'Edit policy'}</h2>
        <div className="field">
          <label>Name</label>
          <input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
        </div>
        <div className="field">
          <label>Description (internal)</label>
          <textarea
            value={draft.description}
            onChange={(e) => setDraft({ ...draft, description: e.target.value })}
          />
        </div>
        <div className="field row">
          <div className="field">
            <label>Action</label>
            <select
              value={draft.action}
              onChange={(e) => setDraft({ ...draft, action: e.target.value as PolicyAction })}
            >
              <option value="deny">deny — block pod</option>
              <option value="warn">warn — allow with warning</option>
              <option value="audit">audit — log only</option>
            </select>
          </div>
          <div className="field">
            <label>Enabled</label>
            <ToggleSwitch
              checked={draft.enabled}
              label={draft.enabled ? 'Disable policy' : 'Enable policy'}
              caption={draft.enabled ? 'On' : 'Off'}
              onChange={(enabled) => setDraft({ ...draft, enabled })}
            />
          </div>
        </div>
        <div className="field">
          <label>Exceptions (namespaces, comma or newline; supports prefix wildcards like platform-*)</label>
          <textarea
            value={(draft.exceptions || []).join('\n')}
            onChange={(e) =>
              setDraft({
                ...draft,
                exceptions: e.target.value
                  .split(/[\n,]+/)
                  .map((s) => s.trim())
                  .filter(Boolean),
              })
            }
          />
        </div>

        <RuleBuilder draft={draft} onChange={setDraft} />

        {err && <div className="status-line">Error: {err}</div>}
        <div className="drawer-actions">
          <button className="btn primary" disabled={saving || !!err} onClick={() => void submit()}>
            {saving ? 'Saving…' : 'Save'}
          </button>
          <button className="btn ghost" onClick={onClose}>
            Cancel
          </button>
        </div>
      </aside>
    </div>
  )
}
