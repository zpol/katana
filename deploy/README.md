# Deploy KATANA on Kubernetes

KATANA runs as a **ValidatingAdmissionWebhook** inside the cluster. The API server calls it on Pod CREATE/UPDATE (scope depends on `namespaceSelector` — see below).

Auth, users, SSO: [`docs/AUTH.md`](../docs/AUTH.md) · [`docs/SSO.md`](../docs/SSO.md).  
Policy reference: [`docs/policies.md`](../docs/policies.md).  
Fleet / Prometheus: [`docs/FLEET.md`](../docs/FLEET.md).

## Prerequisites

- Cluster-admin (or equivalent) RBAC to create `ValidatingWebhookConfiguration`
- Container image in a registry your cluster can pull
- Egress **TCP/443** from the KATANA pods to your JFrog Xray / Artifactory URL
- TLS certificate for Service DNS `katana.katana-system.svc`

## Step-by-step

### 1. Build and push the image

```bash
docker build -t registry.example.com/<team>/katana:0.2.0 .
docker push registry.example.com/<team>/katana:0.2.0
```

### 2. Configure secrets

Edit [`k8s/katana.yaml`](k8s/katana.yaml) or use External Secrets / Sealed Secrets:

| Secret key | Purpose |
|------------|---------|
| `KATANA_BOOTSTRAP_ADMIN_PASSWORD` | First local admin password (optional if users exist) |
| `KATANA_TOKEN` | Legacy API token (admin; optional with local/SSO login) |
| `KATANA_OIDC_CLIENT_ID` | OIDC client ID (optional) |
| `KATANA_OIDC_CLIENT_SECRET` | OIDC client secret (optional; never in ConfigMap) |
| `JFROG_URL` | e.g. `https://artifactory.example.com` |
| `JFROG_TOKEN` | Xray read token |

### 3. TLS for the webhook

```bash
bash deploy/scripts/gen-webhook-certs.sh deploy/certs
kubectl -n katana-system create secret tls katana-webhook-tls \
  --cert=deploy/certs/tls.crt --key=deploy/certs/tls.key
```

Patch `ValidatingWebhookConfiguration` `caBundle` with:

```bash
base64 -w0 deploy/certs/tls.crt
```

### 4. Choose namespace scope

**Option A — All namespaces (default, recommended for full coverage)**

Leave `namespaceSelector` **commented out** in `k8s/katana.yaml`. Every Pod in the cluster is evaluated. Platform namespaces are still excluded from deny rules via policy `exceptions` (`kube-system`, `katana-system`, `cattle-*`).

**Option B — Opt-in namespaces only (pilot)**

1. Label namespaces to enforce:

   ```bash
   kubectl label ns my-app katana.dev/enforce=true
   ```

2. Uncomment the `namespaceSelector` block at the bottom of `k8s/katana.yaml`.

### 5. Apply manifests

```bash
# Edit image tag + secrets first
kubectl apply -f deploy/k8s/katana.yaml
kubectl -n katana-system set image deploy/katana \
  katana=registry.example.com/<team>/katana:0.2.0
```

### 6. Rollout safely

| Phase | `KATANA_ADMISSION_DRY_RUN` | `failurePolicy` | Behaviour |
|-------|---------------------------|-----------------|-----------|
| Observe | `true` | `Ignore` | Pods always allowed; denials logged in Detections |
| Enforce | `false` | `Ignore` → `Fail` | Matching policies block Pod creation |

```bash
# When ready to enforce:
kubectl -n katana-system set env deploy/katana KATANA_ADMISSION_DRY_RUN=false
```

UI: sign in with bootstrap admin (`admin` + password from Secret) or SSO. Legacy API token: `KATANA_TOKEN`. When admission dry-run is on, the UI shows a **DRY RUN MODE** banner.

Set `KATANA_CLUSTER_NAME` (e.g. `my-cluster`) if you use the fleet dashboard.

## Native ImagePolicy CRDs

Policies can live in Kubernetes as **`ImagePolicy`** CRDs instead of SQLite.

| Step | Command |
|------|---------|
| Install CRD + defaults | `bash deploy/scripts/apply-crd.sh` |
| RBAC | `kubectl apply -f deploy/k8s/katana-rbac-crd.yaml` |
| Enable in pod | `KATANA_POLICY_SOURCE=crd` |

```bash
kubectl get imagepolicies.katana.dev
kubectl describe imagepolicy registry-allowlist
kubectl describe imagepolicy deny-unsafe-pod-security
```

**Default policies (6):** Block Critical, Block High in Prod, Require Scanned Image, Allowlist System NS (audit only — see [policy 4](../docs/policies.md#allowlist-system-ns-policy-4)), Registry Allowlist, **Deny Unsafe Pod Security**. Full reference: [`docs/policies.md`](../docs/policies.md).

- **GitOps:** manage [`deploy/crd/default-imagepolicies.yaml`](crd/default-imagepolicies.yaml) via Argo CD / Flux.
- **Local dev:** keep `KATANA_POLICY_SOURCE=sqlite` (default).
- After checkout: run `go mod tidy` (requires `k8s.io/client-go`).

## Local demo (dummy Xray + optional k3d)

For screenshots and admission demos without a real Artifactory, use the canned Xray mock under [`../demo/`](../demo/):

```bash
# Mock only
docker compose -f demo/docker-compose.yml up --build

# Full k3d cluster (Katana webhook + dummy-jfrog)
bash deploy/scripts/demo-k3d.sh
kubectl -n katana-system port-forward svc/katana 8443:443
```

Details and demo image matrix: [`../demo/README.md`](../demo/README.md).

## Files

| Path | Purpose |
|------|---------|
| [`k8s/katana.yaml`](k8s/katana.yaml) | Namespace, Deployment, Service, webhook |
| [`k8s/katana-rbac-crd.yaml`](k8s/katana-rbac-crd.yaml) | RBAC for ImagePolicy CRD watch/write + nodes |
| [`crd/imagepolicy-crd.yaml`](crd/imagepolicy-crd.yaml) | ImagePolicy CRD definition |
| [`crd/default-imagepolicies.yaml`](crd/default-imagepolicies.yaml) | Default policy CRs (GitOps) |
| [`scripts/gen-webhook-certs.sh`](scripts/gen-webhook-certs.sh) | Dev/lab TLS cert generator |
| [`scripts/apply-crd.sh`](scripts/apply-crd.sh) | Install CRD + default policies |
| [`scripts/demo-k3d.sh`](scripts/demo-k3d.sh) | Local k3d demo (Katana + dummy-jfrog) |
| [`scripts/list-jfrog-docker-repos.sh`](scripts/list-jfrog-docker-repos.sh) | List Docker repos from `$JFROG_URL` |

Full architecture and env var reference: [README.md](../README.md).
