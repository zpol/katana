import { useState } from 'react'
import { api } from '../api'

interface Props {
  localLoginEnabled: boolean
  oidcEnabled: boolean
  version?: string
  onLoggedIn: () => void
}

export function LoginPage({ localLoginEnabled, oidcEnabled, version, onLoggedIn }: Props) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.login(username, password)
      onLoggedIn()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'login failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login-shell">
      <div className="login-card">
        <img className="brand-logo" src="/logo.png" alt="KATANA" />
        <h1>Sign in</h1>
        <p className="login-sub">Kubernetes admission policy console</p>

        {localLoginEnabled && (
          <form onSubmit={(e) => void submit(e)}>
            <div className="field">
              <label>Username</label>
              <input
                autoComplete="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                required
              />
            </div>
            <div className="field">
              <label>Password</label>
              <input
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
            </div>
            {error && <div className="status-line">Error: {error}</div>}
            <button className="btn primary login-btn" type="submit" disabled={busy}>
              {busy ? 'Signing in…' : 'Sign in'}
            </button>
          </form>
        )}

        {oidcEnabled && (
          <div style={{ marginTop: localLoginEnabled ? '1rem' : 0 }}>
            <a className="btn login-btn" href="/api/v1/auth/oidc/login">
              Sign in with SSO
            </a>
          </div>
        )}

        {!localLoginEnabled && !oidcEnabled && (
          <div className="status-line">No login methods configured on the server.</div>
        )}
      </div>
      <footer className="app-footer">KATANA {version ? `v${version}` : 'dev'}</footer>
    </div>
  )
}
