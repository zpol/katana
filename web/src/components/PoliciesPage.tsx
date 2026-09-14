import type { Policy } from '../types'
import { ToggleSwitch } from './ToggleSwitch'

interface Props {
  policies: Policy[]
  isAdmin: boolean
  onEdit: (p: Policy) => void
  onToggle: (p: Policy) => void
}

export function PoliciesPage({ policies, isAdmin, onEdit, onToggle }: Props) {
  return (
    <div className="table-wrap">
      <table className="data-table policies-table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Action</th>
            <th>Enabled</th>
            <th>Developer message</th>
            <th>Description</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {policies.map((p) => (
            <tr key={p.id}>
              <td>{p.name}</td>
              <td><span className={`badge ${p.action}`}>{p.action}</span></td>
              <td>
                <ToggleSwitch
                  checked={p.enabled}
                  disabled={!isAdmin}
                  label={`${p.enabled ? 'Disable' : 'Enable'} policy ${p.name}`}
                  onChange={() => onToggle(p)}
                />
              </td>
              <td className="clip" title={p.deny_message || p.warn_message || ''}>
                {p.deny_message || p.warn_message ? 'custom' : 'auto'}
              </td>
              <td>{p.description}</td>
              <td style={{ whiteSpace: 'nowrap' }}>
                {isAdmin ? (
                  <button className="btn" onClick={() => onEdit(p)}>Edit</button>
                ) : (
                  <span className="muted-inline">read-only</span>
                )}
              </td>
            </tr>
          ))}
          {policies.length === 0 && (
            <tr><td colSpan={6}>No policies found.</td></tr>
          )}
        </tbody>
      </table>
    </div>
  )
}