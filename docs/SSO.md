# SSO with OIDC

KATANA uses standard **OpenID Connect** authorization code flow. Any OIDC-compliant identity provider works when exposed as an OIDC issuer (generic OIDC, Okta, Microsoft Entra ID, PingOne, Keycloak, Auth0, and others).

## Configuration sources

| Setting | Deploy via | Editable in UI (Settings → SSO) | Stored in DB |
|---------|------------|----------------------------------|--------------|
| Client secret | **Secret only** (`KATANA_OIDC_CLIENT_SECRET`) | No (shows `client_secret_configured: true/false`) | Never |
| Issuer URL | Env or Secret | Yes (admin) | Yes (`app_settings`) |
| Client ID | Env or Secret | Yes (admin) | Yes |
| Redirect URI | Env | Yes (admin) | Yes |
| Scopes | Env | Yes (admin) | Yes |
| Group claim name | Env | Yes (admin) | Yes |
| Admin groups | Env | Yes (admin) | Yes |
| Read-only groups | Env | Yes (admin) | Yes |

**Precedence:** Secrets always come from environment at pod start. Non-secret values can be overridden in the UI; effective config merges env bootstrap + DB overrides.

**Important:** Changing issuer/client/secret at runtime requires a **pod restart** to re-initialize the OIDC provider. UI saves persist group mappings and non-secret fields for the next restart.

## Environment variables

### Required for OIDC login

```bash
KATANA_AUTH_MODE=hybrid          # or oidc
KATANA_OIDC_ENABLED=true
KATANA_OIDC_ISSUER=https://idp.example.com
KATANA_OIDC_CLIENT_ID=<client-id>
KATANA_OIDC_CLIENT_SECRET=<from-secret>
KATANA_OIDC_REDIRECT_URI=https://<katana-host>/api/v1/auth/oidc/callback
```

### Group → role mapping

```bash
KATANA_OIDC_GROUP_CLAIM=groups
KATANA_OIDC_ADMIN_GROUPS=katana-admins,platform-admins
KATANA_OIDC_READONLY_GROUPS=katana-readonly
```

Evaluation order:

1. If user is in any **admin** group → `admin`
2. Else if in any **readonly** group → `readonly`
3. Else → login denied (`403`)

Group matching is case-insensitive.

### Optional

```bash
KATANA_OIDC_SCOPES=openid profile email
KATANA_OIDC_EMAIL_CLAIM=email
```

## Kubernetes example

```yaml
env:
  - name: KATANA_AUTH_MODE
    value: hybrid
  - name: KATANA_OIDC_ENABLED
    value: "true"
  - name: KATANA_OIDC_ISSUER
    value: "https://idp.example.com"
  - name: KATANA_OIDC_CLIENT_ID
    valueFrom:
      secretKeyRef:
        name: katana-secrets
        key: KATANA_OIDC_CLIENT_ID
  - name: KATANA_OIDC_CLIENT_SECRET
    valueFrom:
      secretKeyRef:
        name: katana-secrets
        key: KATANA_OIDC_CLIENT_SECRET
  - name: KATANA_OIDC_REDIRECT_URI
    value: "https://katana.example.com/api/v1/auth/oidc/callback"
  - name: KATANA_OIDC_ADMIN_GROUPS
    value: "katana-admins"
  - name: KATANA_OIDC_READONLY_GROUPS
    value: "katana-readonly"
```

Register the redirect URI with your IdP exactly as deployed (including path `/api/v1/auth/oidc/callback`).

## OIDC provider setup checklist

1. Create an OIDC web application in your IdP (generic OIDC, Okta, Microsoft Entra ID, PingOne, Keycloak, …).
2. Set redirect URI to `https://<katana-external-url>/api/v1/auth/oidc/callback`.
3. Enable scopes: `openid`, `profile`, `email`, and any scope required for groups.
4. Configure groups claim in token (attribute name → `KATANA_OIDC_GROUP_CLAIM`).
5. Assign users to groups listed in `KATANA_OIDC_ADMIN_GROUPS` or `KATANA_OIDC_READONLY_GROUPS`.
6. Store client secret in Kubernetes Secret; apply manifest and restart KATANA.
7. In KATANA UI: **Settings → SSO → Test connection** (validates issuer discovery).
8. Click **Sign in with SSO** on the login page.

## OIDC flow

```
Browser → GET /api/v1/auth/oidc/login
       → Redirect to IdP authorize URL
       → User authenticates
       → Redirect to /api/v1/auth/oidc/callback?code=...&state=...
       → KATANA exchanges code, reads groups, maps role, creates session cookie
       → Redirect to /
```

## Settings tab (admin)

Admins can view and edit non-secret SSO settings and run **Test connection** (OIDC discovery GET). Readonly users see the current configuration but cannot save.

## Troubleshooting

| Symptom | Check |
|---------|--------|
| "oidc not configured" | `KATANA_OIDC_CLIENT_SECRET`, issuer, client ID, redirect URI all set; restart pod |
| "no matching OIDC group" | User groups in token vs `KATANA_OIDC_*_GROUPS`; claim name |
| "invalid oidc state" | Cookie blocked; ensure HTTPS and SameSite; retry login |
| Discovery test fails | Issuer URL reachable from pod; HTTP/HTTPS proxy or custom CA if needed |
| SSO button hidden | `GET /api/v1/config` → `oidc_enabled: true` |

## Local break-glass

Keep at least one **local admin** user (bootstrap or Settings → Users) in case the IdP is unavailable. Set `KATANA_AUTH_MODE=hybrid` so local login remains available alongside SSO.
