import { useEffect, useMemo, useState } from 'react'
import { api, type AppConfig, type AuthUser, type JFrogStatus } from './api'
import type { Detection, Policy } from './types'
import { PolicyDrawer } from './components/PolicyDrawer'
import { PoliciesPage } from './components/PoliciesPage'
import { DetectionsPage } from './components/DetectionsPage'
import { DashboardPage } from './components/DashboardPage'
import { SettingsPage } from './components/SettingsPage'
import { EvaluatePage } from './components/EvaluatePage'
import { LoginPage } from './components/LoginPage'
import { DryRunBanner } from './components/DryRunBanner'

type Tab = 'dashboard' | 'policies' | 'detections' | 'evaluate' | 'settings'

const emptyPolicy = (): Policy => ({
  id: '',
  name: 'Block Critical CVEs',
  description: 'Deny workloads with any Critical CVE (system NS excepted)',
  enabled: true,
  action: 'deny',
  match: { severity: 'critical' },
  exceptions: ['kube-system', 'katana-system', 'katana-poc-system', 'cattle-*'],
  deny_message: `Image {image} has CRITICAL vulnerabilities (JFrog Xray). Policy: {policy}.
Upgrade the base image or contact your platform security team for an exception.`,
  created_at: '',
  updated_at: '',
})

export default function App() {
  const [tab, setTab] = useState<Tab>('dashboard')
  const [policies, setPolicies] = useState<Policy[]>([])
  const [detections, setDetections] = useState<Detection[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<Policy | null>(null)
  const [creating, setCreating] = useState(false)
  const [config, setConfig] = useState<AppConfig | null>(null)
  const [jfrog, setJfrog] = useState<JFrogStatus | null>(null)
  const [user, setUser] = useState<AuthUser | null>(null)
  const [authChecked, setAuthChecked] = useState(false)
  const [detFilters, setDetFilters] = useState({
    severity: '',
    environment: '',
    namespace: '',
    registry: '',
    outcome: '',
    q: '',
  })

  const isAdmin = user?.role === 'admin'

  const refreshAuth = async () => {
    try {
      const me = await api.me()
      setUser(me)
    } catch {
      setUser(null)
    } finally {
      setAuthChecked(true)
    }
  }

  useEffect(() => {
    void api.config().then(setConfig).catch(() => setConfig(null))
    void refreshAuth()
  }, [])

  useEffect(() => {
    if (!user) return
    let cancelled = false
    const load = () => {
      void api
        .jfrogStatus()
        .then((s) => {
          if (!cancelled) setJfrog(s)
        })
        .catch(() => {
          if (!cancelled) setJfrog({ configured: false, reachable: false, message: 'status unavailable' })
        })
    }
    load()
    const id = window.setInterval(load, 30_000)
    return () => {
      cancelled = true
      window.clearInterval(id)
    }
  }, [user])

  const loadPolicies = async () => {
    setLoading(true)
    setError('')
    try {
      setPolicies(await api.listPolicies())
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed to load policies')
    } finally {
      setLoading(false)
    }
  }

  const loadDetections = async () => {
    setLoading(true)
    setError('')
    try {
      setDetections(await api.listDetections(detFilters))
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed to load detections')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (!user) return
    if (tab === 'policies') void loadPolicies()
    if (tab === 'detections') void loadDetections()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab, user])

  useEffect(() => {
    if (!user || tab !== 'detections') return
    void loadDetections()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detFilters])

  const subtitle = useMemo(() => {
    if (tab === 'dashboard') return 'Admission metrics, deploy outcomes, and policy activity'
    if (tab === 'policies') return 'Admission rules and allowlists'
    if (tab === 'detections') return 'Latest evaluation findings with deploy outcome'
    if (tab === 'evaluate') return 'Dry-run image against policies + Xray'
    return 'Integrations, users, and SSO'
  }, [tab])

  const tabTitle = tab === 'dashboard' ? 'Dashboard' : tab[0].toUpperCase() + tab.slice(1)

  const savePolicy = async (next: Policy) => {
    if (creating || !next.id) {
      await api.createPolicy({
        name: next.name,
        description: next.description,
        enabled: next.enabled,
        action: next.action,
        match: next.match,
        exceptions: next.exceptions,
        deny_message: next.deny_message,
        warn_message: next.warn_message,
      })
    } else {
      await api.updatePolicy(next.id, next)
    }
    setEditing(null)
    setCreating(false)
    await loadPolicies()
  }

  const logout = async () => {
    try {
      await api.logout()
    } finally {
      setUser(null)
    }
  }

  if (!authChecked) {
    return <div className="login-shell"><div className="status-line">Loading…</div></div>
  }

  if (!user) {
    return (
      <LoginPage
        localLoginEnabled={config?.local_login_enabled !== false}
        oidcEnabled={!!config?.oidc_enabled}
        version={config?.version}
        onLoggedIn={() => void refreshAuth()}
      />
    )
  }

  return (
    <div className="app-shell">
      <div className="app-chrome">
      {config?.admission_dry_run && <DryRunBanner />}
      <header className="topbar">
        <div className="brand">
          <img className="brand-logo" src="/logo.png" alt="KATANA" />
        </div>
        <nav className="nav" aria-label="Primary">
          <button className={tab === 'dashboard' ? 'active' : ''} onClick={() => setTab('dashboard')}>
            Dashboard
          </button>
          <button className={tab === 'policies' ? 'active' : ''} onClick={() => setTab('policies')}>
            Policies
          </button>
          <button className={tab === 'detections' ? 'active' : ''} onClick={() => setTab('detections')}>
            Detections
          </button>
          <button className={tab === 'evaluate' ? 'active' : ''} onClick={() => setTab('evaluate')}>
            Evaluate
          </button>
          <button className={tab === 'settings' ? 'active' : ''} onClick={() => setTab('settings')}>
            Settings
          </button>
        </nav>
        <div className="topbar-user">
          <span
            className={`jfrog-pill ${jfrog?.reachable ? 'ok' : 'down'}`}
            title={jfrog?.message || 'JFrog status unknown'}
          >
            <span className="jfrog-dot" aria-hidden />
            JFrog {jfrog?.reachable ? 'reachable' : jfrog?.configured ? 'unreachable' : 'offline'}
          </span>
          <span className="user-pill">
            {user.username}
            <em>{user.role}</em>
          </span>
          <button className="btn ghost" onClick={() => void logout()}>
            Sign out
          </button>
        </div>
      </header>
      </div>

      <main className="content">
        <section className="panel">
          <div className="panel-head">
            <div>
              <h1>{tabTitle}</h1>
              <p>{subtitle}</p>
            </div>
            <div style={{ display: 'flex', gap: '0.5rem' }}>
              {tab === 'policies' && isAdmin && (
                <>
                  <button
                    className="btn primary"
                    onClick={() => {
                      setCreating(true)
                      setEditing(emptyPolicy())
                    }}
                  >
                    New policy
                  </button>
                  <button className="btn ghost" onClick={() => void loadPolicies()}>
                    Refresh
                  </button>
                </>
              )}
              {tab === 'policies' && !isAdmin && (
                <button className="btn ghost" onClick={() => void loadPolicies()}>
                  Refresh
                </button>
              )}
              {tab === 'detections' && (
                <button className="btn ghost" onClick={() => void loadDetections()}>
                  Refresh
                </button>
              )}
            </div>
          </div>

          {error && <div className="status-line">Error: {error}</div>}
          {loading && tab !== 'dashboard' && tab !== 'evaluate' && tab !== 'settings' && (
            <div className="status-line">Loading…</div>
          )}

          {tab === 'dashboard' && <DashboardPage />}
          {tab === 'policies' && !loading && (
            <PoliciesPage
              policies={policies}
              isAdmin={isAdmin}
              onEdit={(p) => {
                setCreating(false)
                setEditing(p)
              }}
              onToggle={async (p) => {
                await api.patchPolicy(p.id, { enabled: !p.enabled })
                await loadPolicies()
              }}
            />
          )}
          {tab === 'detections' && (
            <DetectionsPage detections={detections} filters={detFilters} onFilters={setDetFilters} />
          )}
          {tab === 'evaluate' && <EvaluatePage />}
          {tab === 'settings' && <SettingsPage isAdmin={isAdmin} user={user} />}
        </section>
      </main>

      {editing && isAdmin && (
        <PolicyDrawer
          policy={editing}
          isNew={creating}
          onClose={() => {
            setEditing(null)
            setCreating(false)
          }}
          onSave={savePolicy}
        />
      )}

      <footer className="app-footer">
        KATANA {config?.version ? `v${config.version}` : 'dev'}
      </footer>
    </div>
  )
}
