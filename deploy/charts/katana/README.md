# KATANA Helm chart

Installs the KATANA admission controller + admin UI (Deployment, Service, ServiceAccount, Secret, ValidatingWebhookConfiguration, optional CRD RBAC).

## Install (lab)

```bash
# from repo root
bash deploy/scripts/gen-webhook-certs.sh /tmp/katana-certs katana-system
CABUNDLE=$(base64 -w0 /tmp/katana-certs/tls.crt)
CERT=$(base64 -w0 /tmp/katana-certs/tls.crt)
KEY=$(base64 -w0 /tmp/katana-certs/tls.key)

helm upgrade --install katana deploy/charts/katana \
  --namespace katana-system --create-namespace \
  --set image.repository=registry.example.com/team/katana \
  --set image.tag=0.5.0 \
  --set webhook.createSecret=true \
  --set-string webhook.tls.cert="$CERT" \
  --set-string webhook.tls.key="$KEY" \
  --set-string webhook.caBundle="$CABUNDLE" \
  --set secrets.katanaToken=change-me \
  --set secrets.jfrogUrl=https://artifactory.example.com \
  --set secrets.jfrogToken=REPLACE \
  --set config.admissionDryRun=true \
  --set config.awsRegion=eu-west-1 \
  --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"=arn:aws:iam::123456789012:role/katana-ecr-read
```

## ECR-only example

```bash
helm upgrade --install katana deploy/charts/katana \
  --namespace katana-system --create-namespace \
  --set image.repository=... \
  --set image.tag=0.5.0 \
  --set secrets.jfrogUrl="" \
  --set secrets.jfrogToken="" \
  --set config.awsRegion=eu-west-1 \
  --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"=arn:aws:iam::ACCOUNT:role/katana-ecr-read \
  # + webhook TLS values as above
```

Then in the UI: **Settings → Integrations** → enable Amazon Inspector, set allowed ECR registries.

## Values of interest

| Key | Default | Notes |
|-----|---------|-------|
| `config.admissionDryRun` | `true` | Enforce when `false` |
| `config.policySource` | `sqlite` | Set `crd` + apply ImagePolicy CRD |
| `config.awsRegion` | `""` | ECR Inspector |
| `webhook.failurePolicy` | `Ignore` | Use `Fail` only when latency proven |
| `webhook.enforceLabel` | `""` | Opt-in NS label if set |
| `persistence.enabled` | `false` | PVC for SQLite |
| `rbac.create` | `true` | ImagePolicy + nodes |

Full scanner docs: [`docs/SCANNERS.md`](../../../docs/SCANNERS.md).

## Lint / template

```bash
helm lint deploy/charts/katana
helm template katana deploy/charts/katana --namespace katana-system \
  --set webhook.caBundle=dGVzdA== | head
```
