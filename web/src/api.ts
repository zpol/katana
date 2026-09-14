import type { Detection, Policy, StatsSummary } from './types'

const TOKEN_KEY = 'katana_token'

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) || ''
}

export function setToken(token: string) {
  if (token) {
    localStorage.setItem(TOKEN_KEY, token)
  } else {
    localStorage.removeItem(TOKEN_KEY)
  }
}

export interface AppConfig {
  admission_dry_run: boolean
  version?: string
  auth_mode?: string
  local_login_enabled?: boolean
  oidc_enabled?: boolean
}

export interface AuthUser {
  user_id?: string
  username: string
  role: 'admin' | 'readonly'
  source: string
}

export interface SSOSettings {
  enabled: boolean
  issuer: string
  client_id: string
  redirect_uri: string
  scopes: string[]
  group_claim: string
  email_claim: string
  admin_groups: string[]
  readonly_groups: string[]
  client_secret_configured: boolean
}

export interface KatanaUser {
  id: string
  username: string
  role: string
  source: string
  email?: string
  has_password: boolean
}

function networkError(path: string, cause: unknown): Error {
  const hint =
    'Check: (1) kubectl port-forward svc/katana 8443:443 and open https://127.0.0.1:8443; ' +
    '(2) sign in with your KATANA account or legacy API token in Settings.'
  const detail = cause instanceof Error ? cause.message : String(cause)
  return new Error(`Cannot reach KATANA API (${path}). ${hint} (${detail})`)
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  const legacy = getToken()
  if (legacy) {
    headers.set('X-Katana-Token', legacy)
  }
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  let res: Response
  try {
    res = await fetch(path, { ...init, headers, credentials: 'include' })
  } catch (e) {
    throw networkError(path, e)
  }
  if (res.status === 401) {
    throw new Error('authentication required')
  }
  if (!res.ok) {
    const text = await res.text()
    if (res.status === 504) {
      throw new Error(
        text ||
          'Evaluate timed out waiting for JFrog Xray (can take up to 90s). Retry or use a cached/indexed image.',
      )
    }
    throw new Error(text || res.statusText)
  }
  if (res.status === 204) {
    return undefined as T
  }
  return res.json() as Promise<T>
}

function evaluateTimeoutSignal(ms = 120_000): AbortSignal {
  if (typeof AbortSignal !== 'undefined' && 'timeout' in AbortSignal) {
    return AbortSignal.timeout(ms)
  }
  const ctrl = new AbortController()
  setTimeout(() => ctrl.abort(), ms)
  return ctrl.signal
}

export interface JFrogStatus {
  configured: boolean
  base_url?: string
  reachable: boolean
  message: string
}

export interface EvaluateResult {
  image: string
  registry: string
  namespace: string
  scan: {
    scanned: boolean
    lookup_status?: string
    critical: number
    high: number
    medium: number
    low: number
    unknown?: number
    violations?: string[]
    error?: string
    artifact?: {
      repo?: string
      path?: string
      sha?: string
      image_ref?: string
    }
  }
  decision: {
    allowed: boolean
    action: string
    matched_policy?: string
    user_message?: string
    reasons?: string[]
  }
  input: {
    severity: string
    environment: string
    scanned: boolean
    namespace: string
    registry: string
    image: string
  }
}

export const api = {
  config: () => fetch('/api/v1/config').then((r) => r.json() as Promise<AppConfig>),
  me: () => request<AuthUser>('/api/v1/auth/me'),
  login: (username: string, password: string) =>
    request<AuthUser>('/api/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  logout: () => request<void>('/api/v1/auth/logout', { method: 'POST' }),
  listPolicies: () => request<Policy[]>('/api/v1/policies'),
  createPolicy: (body: Partial<Policy>) =>
    request<Policy>('/api/v1/policies', { method: 'POST', body: JSON.stringify(body) }),
  getPolicy: (id: string) => request<Policy>(`/api/v1/policies/${id}`),
  updatePolicy: (id: string, body: Partial<Policy>) =>
    request<Policy>(`/api/v1/policies/${id}`, { method: 'PUT', body: JSON.stringify({ ...body, id }) }),
  patchPolicy: (id: string, body: Partial<Policy>) =>
    request<Policy>(`/api/v1/policies/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  deletePolicy: (id: string) =>
    request<void>(`/api/v1/policies/${id}`, { method: 'DELETE' }),
  listDetections: (params: Record<string, string>) => {
    const qs = new URLSearchParams()
    Object.entries(params).forEach(([k, v]) => {
      if (v) qs.set(k, v)
    })
    const suffix = qs.toString() ? `?${qs}` : ''
    return request<Detection[]>(`/api/v1/detections${suffix}`)
  },
  getStats: () => request<StatsSummary>('/api/v1/stats/summary'),
  evaluate: (body: { image: string; namespace?: string; environment?: string; record?: boolean }) =>
    request<EvaluateResult>('/api/v1/evaluate', {
      method: 'POST',
      body: JSON.stringify(body),
      signal: evaluateTimeoutSignal(120_000),
    }),
  jfrogStatus: () => request<JFrogStatus>('/api/v1/integrations/jfrog'),
  health: () => fetch('/api/v1/health').then((r) => r.json() as Promise<{ status: string }>),
  listUsers: () => request<KatanaUser[]>('/api/v1/users'),
  createUser: (body: { username: string; password: string; role: string }) =>
    request<KatanaUser>('/api/v1/users', { method: 'POST', body: JSON.stringify(body) }),
  patchUser: (id: string, body: { role?: string; password?: string }) =>
    request<KatanaUser>(`/api/v1/users/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  deleteUser: (id: string) => request<void>(`/api/v1/users/${id}`, { method: 'DELETE' }),
  getSSOSettings: () => request<SSOSettings>('/api/v1/settings/sso'),
  saveSSOSettings: (body: SSOSettings) =>
    request<SSOSettings>('/api/v1/settings/sso', { method: 'PUT', body: JSON.stringify(body) }),
  testSSOSettings: () =>
    request<{ status: string; message: string }>('/api/v1/settings/sso/test', { method: 'POST' }),
}
