# KATANA authentication and authorization

KATANA supports **local users** (username/password stored in SQLite), **OIDC SSO** (any OIDC provider), and an optional **legacy API token** for automation.

## Roles

| Role | Capabilities |
|------|----------------|
| **admin** | Full access: create/edit/delete policies, manage users, configure SSO, record evaluate results |
| **readonly** | View dashboard, policies, detections; run evaluate (without recording); view SSO status |

Write operations return `403 Forbidden` for readonly users.

## Authentication methods

### 1. Browser session (recommended for UI)

After sign-in (local or SSO), KATANA sets an HttpOnly cookie `katana_session`. The UI sends `credentials: 'include'` on API requests.

- Secure flag is enabled when TLS is configured (`KATANA_TLS_CERT` + `KATANA_TLS_KEY`).
- Session TTL: `KATANA_SESSION_TTL` (default `8h`).

### 2. Local login

Users are stored in SQLite (`users` table). Passwords are bcrypt-hashed; never stored in plain text.

**First admin (bootstrap)** — set once via environment/Secret on first deploy when no local users exist:

```yaml
KATANA_BOOTSTRAP_ADMIN_USER: admin
KATANA_BOOTSTRAP_ADMIN_PASSWORD: # from Secret
```

Additional users can be created in **Settings → Users** (admin only).

### 3. OIDC SSO

See [SSO.md](./SSO.md) for OIDC configuration, group mapping, and env vs Secret matrix.

### 4. Legacy API token (optional)

`KATANA_TOKEN` + header `X-Katana-Token` grants **admin** access for scripts, CI, or port-forward workflows without a browser session.

This is optional when local users or SSO are configured. Store in Kubernetes Secret, never in ConfigMap.

## Auth mode

`KATANA_AUTH_MODE` controls which login methods are offered:

| Value | Local login | OIDC SSO |
|-------|-------------|----------|
| `local` (default) | yes | only if OIDC env fully set |
| `oidc` | no | yes (when configured) |
| `hybrid` | yes | yes (when configured) |

## Startup requirements

KATANA starts when at least one of these is true:

1. Bootstrap admin credentials are set (for first-time local user creation)
2. At least one user already exists in the database
3. `KATANA_TOKEN` is set (legacy)
4. OIDC is fully configured (`issuer`, `client_id`, `client_secret`, `redirect_uri`)

## API endpoints

| Endpoint | Auth | Role |
|----------|------|------|
| `GET /api/v1/config` | public | — |
| `GET /api/v1/health` | public | — |
| `POST /api/v1/auth/login` | public | — |
| `GET /api/v1/auth/oidc/login` | public | — |
| `GET /api/v1/auth/oidc/callback` | public | — |
| `GET /api/v1/auth/me` | session/token | any |
| `POST /api/v1/auth/logout` | session/token | any |
| `GET/POST /api/v1/policies` (write) | session/token | admin |
| `POST /api/v1/evaluate` with `record: true` | session/token | admin |
| `GET/PUT /api/v1/settings/sso` | session/token | GET any; PUT admin |
| `GET/POST/DELETE /api/v1/users` | session/token | admin |

Admission webhook (`POST /validate`) remains unauthenticated (cluster TLS + network policy).

## Dry-run mode banner

When `KATANA_ADMISSION_DRY_RUN=true`, the UI shows a persistent **DRY RUN MODE** banner. The flag is exposed via `GET /api/v1/config`:

```json
{
  "admission_dry_run": true,
  "auth_mode": "local",
  "local_login_enabled": true,
  "oidc_enabled": false
}
```

Admission dry-run affects the **Kubernetes webhook only** (audit vs enforce). It is independent of the Evaluate page.

## Local development

```bash
make dev
```

Set in `.env` or shell:

```bash
KATANA_BOOTSTRAP_ADMIN_USER=admin
KATANA_BOOTSTRAP_ADMIN_PASSWORD=admin
KATANA_AUTH_MODE=local
# optional legacy token for curl/scripts:
KATANA_TOKEN=change-me-dev-token
```

Open http://localhost:8080 and sign in with `admin` / `admin`.

## Security notes

- Never commit passwords, OIDC client secrets, or JFrog tokens.
- Prefer Kubernetes Secrets for `KATANA_BOOTSTRAP_ADMIN_PASSWORD`, `KATANA_OIDC_CLIENT_SECRET`, `KATANA_TOKEN`, `JFROG_TOKEN`.
- The last admin user cannot be deleted.
- OIDC users without a matching admin/readonly group are denied login.
