#!/usr/bin/env bash
# Build/push Katana + sample image, deploy to EKS, smoke-test, print how to destroy.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TF_DIR="${ROOT}/deploy/e2e-aws"
STATE="${ROOT}/tmp/e2e-aws/terraform.tfstate"
CREDS_BOOTSTRAP="${ROOT}/tmp/c.txt"
GOOD="${ROOT}/tmp/good.txt"
CERT_DIR="${ROOT}/tmp/e2e-aws/certs"
KUBECONFIG_OUT="${ROOT}/tmp/e2e-aws/kubeconfig"
NS=katana-system
REGION="${AWS_REGION:-${AWS_DEFAULT_REGION:-eu-west-1}}"

need() { command -v "$1" >/dev/null || { echo "missing $1" >&2; exit 1; }; }
need aws; need kubectl; need docker; need terraform; need openssl

[[ -f "$STATE" ]] || { echo "run apply.sh first" >&2; exit 1; }
[[ -f "$CREDS_BOOTSTRAP" ]] || { echo "missing $CREDS_BOOTSTRAP" >&2; exit 1; }
# shellcheck disable=SC1090
source "$CREDS_BOOTSTRAP"
export AWS_REGION="$REGION" AWS_DEFAULT_REGION="$REGION"

REPO=$(terraform -chdir="$TF_DIR" output -state="$STATE" -raw ecr_repository_url)
NAME=$(terraform -chdir="$TF_DIR" output -state="$STATE" -raw ecr_repository_name)
ACCT=$(terraform -chdir="$TF_DIR" output -state="$STATE" -raw aws_account_id)
CLUSTER=$(terraform -chdir="$TF_DIR" output -state="$STATE" -raw eks_cluster_name)
IRSA=$(terraform -chdir="$TF_DIR" output -state="$STATE" -raw katana_irsa_role_arn)
KATANA_IMG="${REPO}:katana"
SAMPLE_IMG="${REPO}:e2e"

echo "==> kubeconfig → ${KUBECONFIG_OUT}"
aws eks update-kubeconfig --name "$CLUSTER" --region "$REGION" --kubeconfig "$KUBECONFIG_OUT"
export KUBECONFIG="$KUBECONFIG_OUT"

echo "==> wait for node Ready"
kubectl wait --for=condition=Ready nodes --all --timeout=600s

echo "==> ECR login + push Katana image"
aws ecr get-login-password --region "$REGION" \
  | docker login --username AWS --password-stdin "${ACCT}.dkr.ecr.${REGION}.amazonaws.com"
docker build -t "$KATANA_IMG" --build-arg KATANA_VERSION=e2e "$ROOT"
docker push "$KATANA_IMG"

SAMPLE_SRC="${KATANA_E2E_SAMPLE_IMAGE:-docker.io/library/alpine:3.20}"

echo "==> push sample alpine for scan findings"
# ECR docker login can break public.ecr.aws pulls — use docker hub alpine by default
docker pull "$SAMPLE_SRC"
docker tag "$SAMPLE_SRC" "$SAMPLE_IMG"
docker push "$SAMPLE_IMG"

echo "==> webhook TLS"
mkdir -p "$CERT_DIR"
bash "${ROOT}/deploy/scripts/gen-webhook-certs.sh" "$CERT_DIR" "$NS"
CABUNDLE=$(base64 -w0 "${CERT_DIR}/tls.crt")

echo "==> deploy Katana"
kubectl apply -f "${ROOT}/deploy/k8s/katana-rbac-crd.yaml" || true
kubectl apply -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: ${NS}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: katana
  namespace: ${NS}
  annotations:
    eks.amazonaws.com/role-arn: ${IRSA}
---
apiVersion: v1
kind: Secret
metadata:
  name: katana-secrets
  namespace: ${NS}
type: Opaque
stringData:
  KATANA_TOKEN: "e2e-admin-token"
  JFROG_URL: ""
  JFROG_TOKEN: ""
---
apiVersion: v1
kind: Secret
metadata:
  name: katana-webhook-tls
  namespace: ${NS}
type: kubernetes.io/tls
data:
  tls.crt: $(base64 -w0 "${CERT_DIR}/tls.crt")
  tls.key: $(base64 -w0 "${CERT_DIR}/tls.key")
EOF

# Patch deploy from stock manifest: image, IRSA SA already set, AWS_REGION, dry-run false for real denies optional
kubectl apply -f "${ROOT}/deploy/k8s/katana.yaml"
kubectl -n "$NS" set image deploy/katana katana="${KATANA_IMG}"
kubectl -n "$NS" set env deploy/katana \
  AWS_REGION="$REGION" \
  AWS_DEFAULT_REGION="$REGION" \
  KATANA_ADMISSION_DRY_RUN=true \
  KATANA_CLUSTER_NAME="$CLUSTER"
# Ensure SA annotation survived apply of stock yaml (stock recreates SA without IRSA)
kubectl -n "$NS" annotate sa katana "eks.amazonaws.com/role-arn=${IRSA}" --overwrite
kubectl -n "$NS" set env deploy/katana KATANA_TOKEN=e2e-admin-token
kubectl -n "$NS" patch validatingwebhookconfiguration katana-image-policy --type=json \
  -p="[{\"op\":\"replace\",\"path\":\"/webhooks/0/clientConfig/caBundle\",\"value\":\"${CABUNDLE}\"}]"

kubectl -n "$NS" rollout restart deploy/katana
kubectl -n "$NS" rollout status deploy/katana --timeout=300s

echo "==> health"
kubectl -n "$NS" wait --for=condition=ready pod -l app=katana --timeout=180s
POD=$(kubectl -n "$NS" get pod -l app=katana -o jsonpath='{.items[0].metadata.name}')
kubectl -n "$NS" exec "$POD" -- /katana -h >/dev/null 2>&1 || true

# Port-forward health + evaluate
kubectl -n "$NS" port-forward svc/katana 18443:443 >/tmp/katana-pf.log 2>&1 &
PF_PID=$!
cleanup_pf() { kill "$PF_PID" 2>/dev/null || true; }
trap cleanup_pf EXIT
sleep 3

HEALTH=$(curl -sk https://127.0.0.1:18443/api/v1/health || true)
echo "health: $HEALTH"
echo "$HEALTH" | grep -q '"status":"ok"' || { echo "health failed"; cat /tmp/katana-pf.log; exit 1; }

ECR_ST=$(curl -sk -H "X-Katana-Token: e2e-admin-token" https://127.0.0.1:18443/api/v1/integrations/ecr || true)
echo "ecr status: $ECR_ST"

# Enable ECR scanner via settings API (admin token)
curl -sk -X PUT -H "X-Katana-Token: e2e-admin-token" -H "Content-Type: application/json" \
  https://127.0.0.1:18443/api/v1/settings/integrations \
  -d "{\"allowed_registries\":[\"${ACCT}.dkr.ecr.${REGION}.amazonaws.com\"],\"scanners\":{\"jfrog\":{\"enabled\":false},\"ecr_inspector\":{\"enabled\":true}}}" \
  | tee /tmp/katana-integ.json

echo "==> wait briefly for ECR scan on sample (up to 3m)"
for i in $(seq 1 18); do
  EVAL=$(curl -sk -X POST -H "X-Katana-Token: e2e-admin-token" -H "Content-Type: application/json" \
    https://127.0.0.1:18443/api/v1/evaluate \
    -d "{\"image\":\"${SAMPLE_IMG}\",\"namespace\":\"default\",\"environment\":\"dev\"}" || true)
  echo "evaluate try $i: $(echo "$EVAL" | head -c 300)"
  if echo "$EVAL" | grep -q '"lookup_status":"indexed"'; then
    echo "SCAN OK (indexed)"
    break
  fi
  if echo "$EVAL" | grep -q '"lookup_status":"not_indexed"'; then
    echo "image known to ECR but scan not ready yet…"
  fi
  sleep 10
done

echo
echo "E2E deploy smoke finished."
echo "  cluster: $CLUSTER"
echo "  katana image: $KATANA_IMG"
echo "  sample image: $SAMPLE_IMG"
echo "  kubeconfig: $KUBECONFIG_OUT"
echo "DESTROY WHEN DONE:  bash deploy/e2e-aws/destroy.sh"
