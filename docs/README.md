# KATANA documentation

| Document | Description |
|----------|-------------|
| [AUTH.md](./AUTH.md) | Users, roles (admin/readonly), sessions, legacy API token, dry-run banner |
| [SSO.md](./SSO.md) | OIDC setup (generic IdP examples), env vs Secret, group mapping |
| [policies.md](./policies.md) | ImagePolicy rules, match fields, exceptions, kubectl deny messages |
| [SCANNERS.md](./SCANNERS.md) | JFrog Xray + Amazon Inspector (ECR), Settings → Integrations, IRSA |
| [FLEET.md](./FLEET.md) | Prometheus exporter, fleet dashboard, adding clusters |

## Quick reference — auth env vars

```bash
# Mode: local | oidc | hybrid
KATANA_AUTH_MODE=local

# First admin (Secret recommended)
KATANA_BOOTSTRAP_ADMIN_USER=admin
KATANA_BOOTSTRAP_ADMIN_PASSWORD=

# Optional legacy automation token (Secret)
KATANA_TOKEN=

# Session
KATANA_SESSION_TTL=8h

# Admission dry-run (UI banner when true)
KATANA_ADMISSION_DRY_RUN=true

# OIDC — see SSO.md
KATANA_OIDC_ENABLED=false
KATANA_OIDC_ISSUER=
KATANA_OIDC_CLIENT_ID=
KATANA_OIDC_CLIENT_SECRET=
KATANA_OIDC_REDIRECT_URI=
KATANA_OIDC_ADMIN_GROUPS=
KATANA_OIDC_READONLY_GROUPS=

# Fleet identity (Prometheus label) — see FLEET.md
# KATANA_CLUSTER_NAME=my-cluster

# Scanners — see SCANNERS.md
# JFROG_URL= / JFROG_TOKEN=          # optional if ECR-only
# AWS_REGION=eu-west-1               # ECR Inspector (IRSA in cluster)
```

## Deploy docs

Kubernetes manifests: [../deploy/README.md](../deploy/README.md).  
Scan backends (JFrog / ECR): [SCANNERS.md](./SCANNERS.md).  
Prometheus exporter and fleet console: [FLEET.md](./FLEET.md).
