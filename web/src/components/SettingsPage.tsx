import { useEffect, useState } from 'react'
import { api, getToken, setToken, type AuthUser, type KatanaUser, type SSOSettings } from '../api'

interface Props {
  isAdmin: boolean
  user: AuthUser
}

type SettingsTab = 'general' | 'users' | 'sso'

export function SettingsPage({ isAdmin, user }: Props) {
  const [settingsTab, setSettingsTab] = useState<SettingsTab>('general')
  const [localToken, setLocalToken] = useState(getToken())
  const [jf, setJf] = useState<Awaited<ReturnType<typeof api.jfrogStatus>> | null>(null)
  const [jfErr, setJfErr] = useState('')
  const [users, setUsers] = useState<KatanaUser[]>([])
  const [usersErr, setUsersErr] = useState('')
  const [sso, setSso] = useState<SSOSettings | null>(null)
  const [ssoMsg, setSsoMsg] = useState('')
  const [newUser, setNewUser] = useState({ username: '', password: '', role: 'readonly' })

  const refreshJf = async () => {
    setJfErr('')
    try {
      setJf(await api.jfrogStatus())
    } catch (e) {
      setJf(null)
      setJfErr(e instanceof Error ? e.message : 'status failed')
    }
  }

  const refreshUsers = async () => {
    if (!isAdmin) return
    setUsersErr('')
    try {
      setUsers(await api.listUsers())
    } catch (e) {
      setUsers([])
      setUsersErr(e instanceof Error ? e.message : 'failed to load users')
    }
  }

  const refreshSSO = async () => {
    try {
      const raw = await api.getSSOSettings()
      setSso(normalizeSSO(raw))
    } catch (e) {
      setSso(null)
      setSsoMsg(e instanceof Error ? e.message : 'failed to load SSO settings')
    }
  }

  const listField = (items?: string[] | null) => (items ?? []).join(', ')

  useEffect(() => {
    void refreshJf()
    void refreshSSO()
  }, [])

  useEffect(() => {
    if (settingsTab === 'users') void refreshUsers()
  }, [settingsTab, isAdmin])

  const saveSSO = async () => {
    if (!sso) return
    setSsoMsg('')
    try {
      setSso(await api.saveSSOSettings(sso))
      setSsoMsg('Saved. Restart KATANA if you changed issuer/client settings loaded at startup.')
    } catch (e) {
      setSsoMsg(e instanceof Error ? e.message : 'save failed')
    }
  }

  const testSSO = async () => {
    setSsoMsg('')
    try {
      const res = await api.testSSOSettings()
      setSsoMsg(res.message)
    } catch (e) {
      setSsoMsg(e instanceof Error ? e.message : 'test failed')
    }
  }

  const createUser = async () => {
    setUsersErr('')
    try {
      await api.createUser(newUser)
      setNewUser({ username: '', password: '', role: 'readonly' })
      await refreshUsers()
    } catch (e) {
      setUsersErr(e instanceof Error ? e.message : 'create failed')
    }
  }

  return (
    <div>
      <div className="settings-tabs">
        <button className={settingsTab === 'general' ? 'active' : ''} onClick={() => setSettingsTab('general')}>
          General
        </button>
        {isAdmin && (
          <button className={settingsTab === 'users' ? 'active' : ''} onClick={() => setSettingsTab('users')}>
            Users
          </button>
        )}
        <button className={settingsTab === 'sso' ? 'active' : ''} onClick={() => setSettingsTab('sso')}>
          SSO
        </button>
      </div>

      {settingsTab === 'general' && (
        <div className="settings-grid">
          <div className="settings-card">
            <h3>Signed in as</h3>
            <p>
              <strong>{user.username}</strong> · role <strong>{user.role}</strong> · source {user.source}
            </p>
          </div>

          <div className="settings-card">
            <h3>Legacy API token (optional)</h3>
            <p>
              For scripts and automation only. Browser sessions use cookies after sign-in.
            </p>
            <div className="field">
              <label>X-Katana-Token</label>
              <input
                type="password"
                autoComplete="off"
                value={localToken}
                onChange={(e) => setLocalToken(e.target.value)}
              />
            </div>
            <div style={{ marginTop: '0.8rem' }}>
              <button className="btn primary" onClick={() => setToken(localToken)}>
                Save token locally
              </button>
            </div>
          </div>

          <div className="settings-card">
            <h3>JFrog Artifactory / Xray</h3>
            <p>Server-side only via `JFROG_URL` / `JFROG_TOKEN`.</p>
            {jfErr && <div className="status-line">Error: {jfErr}</div>}
            {jf && (
              <>
                <p>
                  configured=<strong>{String(jf.configured)}</strong> · reachable=
                  <strong>{String(jf.reachable)}</strong>
                </p>
                <p style={{ color: 'var(--muted)' }}>
                  {jf.base_url || '(no URL)'} — {jf.message}
                </p>
              </>
            )}
            <div style={{ marginTop: '0.8rem' }}>
              <button className="btn" onClick={() => void refreshJf()}>
                Refresh status
              </button>
            </div>
          </div>
        </div>
      )}

      {settingsTab === 'users' && isAdmin && (
        <div className="settings-grid">
          <div className="settings-card">
            <h3>Local users</h3>
            {usersErr && <div className="status-line">Error: {usersErr}</div>}
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>Username</th>
                    <th>Role</th>
                    <th>Source</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((u) => (
                    <tr key={u.id}>
                      <td>{u.username}</td>
                      <td>{u.role}</td>
                      <td>{u.source}</td>
                      <td>
                        {u.source === 'local' && (
                          <button
                            className="btn ghost"
                            onClick={() =>
                              void api.deleteUser(u.id).then(refreshUsers).catch((e) =>
                                setUsersErr(e instanceof Error ? e.message : 'delete failed'),
                              )
                            }
                          >
                            Delete
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          <div className="settings-card">
            <h3>Create user</h3>
            <div className="field">
              <label>Username</label>
              <input value={newUser.username} onChange={(e) => setNewUser({ ...newUser, username: e.target.value })} />
            </div>
            <div className="field">
              <label>Password</label>
              <input
                type="password"
                value={newUser.password}
                onChange={(e) => setNewUser({ ...newUser, password: e.target.value })}
              />
            </div>
            <div className="field">
              <label>Role</label>
              <select value={newUser.role} onChange={(e) => setNewUser({ ...newUser, role: e.target.value })}>
                <option value="readonly">readonly</option>
                <option value="admin">admin</option>
              </select>
            </div>
            <button className="btn primary" onClick={() => void createUser()}>
              Create user
            </button>
          </div>
        </div>
      )}

      {settingsTab === 'sso' && !sso && (
        <div className="status-line">{ssoMsg || 'Loading SSO settings…'}</div>
      )}

      {settingsTab === 'sso' && sso && (
        <div className="settings-grid">
          <div className="settings-card">
            <h3>SSO / OIDC</h3>
            <p>
              Non-secret settings can be saved here. Client secret must be supplied via Kubernetes Secret
              (`KATANA_OIDC_CLIENT_SECRET`) and is never stored in the database.
            </p>
            {!isAdmin && <p className="status-line">Read-only view — contact an admin to change SSO settings.</p>}
            <div className="field">
              <label>
                <input
                  type="checkbox"
                  checked={sso.enabled}
                  disabled={!isAdmin}
                  onChange={(e) => setSso({ ...sso, enabled: e.target.checked })}
                />{' '}
                OIDC enabled
              </label>
            </div>
            <div className="field">
              <label>Issuer URL</label>
              <input
                value={sso.issuer}
                disabled={!isAdmin}
                onChange={(e) => setSso({ ...sso, issuer: e.target.value })}
              />
            </div>
            <div className="field">
              <label>Client ID</label>
              <input
                value={sso.client_id}
                disabled={!isAdmin}
                onChange={(e) => setSso({ ...sso, client_id: e.target.value })}
              />
            </div>
            <div className="field">
              <label>Redirect URI</label>
              <input
                value={sso.redirect_uri}
                disabled={!isAdmin}
                onChange={(e) => setSso({ ...sso, redirect_uri: e.target.value })}
              />
            </div>
            <div className="field">
              <label>Scopes (comma-separated)</label>
              <input
                value={listField(sso.scopes)}
                disabled={!isAdmin}
                onChange={(e) =>
                  setSso({
                    ...sso,
                    scopes: e.target.value.split(',').map((s) => s.trim()).filter(Boolean),
                  })
                }
              />
            </div>
            <div className="field">
              <label>Group claim</label>
              <input
                value={sso.group_claim}
                disabled={!isAdmin}
                onChange={(e) => setSso({ ...sso, group_claim: e.target.value })}
              />
            </div>
            <div className="field">
              <label>Admin groups (comma-separated)</label>
              <input
                value={listField(sso.admin_groups)}
                disabled={!isAdmin}
                onChange={(e) =>
                  setSso({
                    ...sso,
                    admin_groups: e.target.value.split(',').map((s) => s.trim()).filter(Boolean),
                  })
                }
              />
            </div>
            <div className="field">
              <label>Read-only groups (comma-separated)</label>
              <input
                value={listField(sso.readonly_groups)}
                disabled={!isAdmin}
                onChange={(e) =>
                  setSso({
                    ...sso,
                    readonly_groups: e.target.value.split(',').map((s) => s.trim()).filter(Boolean),
                  })
                }
              />
            </div>
            <p style={{ color: 'var(--muted)' }}>
              client_secret configured: <strong>{String(sso.client_secret_configured)}</strong>
            </p>
            {ssoMsg && <div className="status-line">{ssoMsg}</div>}
            {isAdmin && (
              <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.8rem' }}>
                <button className="btn primary" onClick={() => void saveSSO()}>
                  Save SSO settings
                </button>
                <button className="btn" onClick={() => void testSSO()}>
                  Test connection
                </button>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function normalizeSSO(raw: SSOSettings): SSOSettings {
  return {
    ...raw,
    issuer: raw.issuer ?? '',
    client_id: raw.client_id ?? '',
    redirect_uri: raw.redirect_uri ?? '',
    group_claim: raw.group_claim ?? 'groups',
    email_claim: raw.email_claim ?? 'email',
    scopes: raw.scopes ?? [],
    admin_groups: raw.admin_groups ?? [],
    readonly_groups: raw.readonly_groups ?? [],
  }
}
