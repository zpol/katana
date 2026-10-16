#!/usr/bin/env bash
# Local k3d (k3s) smoke test for the KATANA Helm chart + dummy Xray.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
CLUSTER="${KATANA_LOCAL_CLUSTER:-katana-local}"
NS=katana-system
CERT_DIR="${ROOT}/tmp/k3d-helm-certs"
export PATH="${HOME}/bin:${PATH}"

need() { command -v "$1" >/dev/null || { echo "missing $1" >&2; exit 1; }; }
need docker; need k3d; need kubectl; need helm; need openssl

cd "$ROOT"

echo "==> k3d cluster '${CLUSTER}'"
if k3d cluster list 2>/dev/null | grep -q "^${CLUSTER} "; then
  echo "    already exists"
else
  k3d cluster create "${CLUSTER}" --wait
fi
kubectl config use-context "k3d-${CLUSTER}" >/dev/null

echo "==> build images"
docker build -t katana:local --build-arg KATANA_VERSION=local .
docker build -t dummy-jfrog:local "${ROOT}/demo/dummy-jfrog"

echo "==> import into k3d"
k3d image import katana:local dummy-jfrog:local -c "${CLUSTER}"

echo "==> namespace + dummy-jfrog"
kubectl create ns "${NS}" --dry-run=client -o yaml | kubectl apply -f -
# dummy-jfrog manifest hardcodes namespace katana-system
kubectl apply -f "${ROOT}/demo/k8s/dummy-jfrog.yaml"
kubectl -n "${NS}" rollout status deploy/dummy-jfrog --timeout=120s

echo "==> webhook TLS"
mkdir -p "${CERT_DIR}"
bash "${ROOT}/deploy/scripts/gen-webhook-certs.sh" "${CERT_DIR}" "${NS}"
CABUNDLE="$(base64 -w0 "${CERT_DIR}/tls.crt")"
CERT="$(base64 -w0 "${CERT_DIR}/tls.crt")"
KEY="$(base64 -w0 "${CERT_DIR}/tls.key")"

echo "==> helm upgrade --install katana"
helm upgrade --install katana "${ROOT}/deploy/charts/katana" \
  --namespace "${NS}" --create-namespace \
  --set image.repository=katana \
  --set image.tag=local \
  --set image.pullPolicy=Never \
  --set webhook.createSecret=true \
  --set-string webhook.tls.cert="${CERT}" \
  --set-string webhook.tls.key="${KEY}" \
  --set-string webhook.caBundle="${CABUNDLE}" \
  --set secrets.create=true \
  --set secrets.katanaToken=local-e2e-token \
  --set secrets.jfrogUrl="http://dummy-jfrog.${NS}.svc:8081" \
  --set secrets.jfrogToken=demo-token \
  --set secrets.bootstrapAdminUser=admin \
  --set secrets.bootstrapAdminPassword=admin \
  --set config.admissionDryRun=true \
  --set config.policySource=sqlite \
  --set config.clusterName="${CLUSTER}" \
  --wait --timeout=180s

kubectl -n "${NS}" rollout status deploy/katana --timeout=180s
kubectl -n "${NS}" get pods -o wide

echo "==> smoke tests"
kubectl -n "${NS}" port-forward svc/katana 18443:443 >/tmp/katana-k3d-pf.log 2>&1 &
PF=$!
cleanup() { kill "$PF" 2>/dev/null || true; }
trap cleanup EXIT
sleep 3

TOKEN=local-e2e-token
echo -n "health: "
curl -sk https://127.0.0.1:18443/api/v1/health
echo
echo -n "jfrog: "
curl -sk -H "X-Katana-Token: ${TOKEN}" https://127.0.0.1:18443/api/v1/integrations/jfrog
echo

# Enable jfrog scanner + allow demo registry via Settings API
curl -sk -X PUT -H "X-Katana-Token: ${TOKEN}" -H 'Content-Type: application/json' \
  https://127.0.0.1:18443/api/v1/settings/integrations \
  -d '{"allowed_registries":["artifactory.example.com"],"scanners":{"jfrog":{"enabled":true},"ecr_inspector":{"enabled":false}}}'
echo

echo -n "evaluate clean: "
curl -sk -X POST -H "X-Katana-Token: ${TOKEN}" -H 'Content-Type: application/json' \
  https://127.0.0.1:18443/api/v1/evaluate \
  -d '{"image":"artifactory.example.com/demo/clean:1.0","namespace":"default","environment":"dev"}' \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);s=d["scan"];print(s.get("scanner"),s.get("lookup_status"),s.get("scanned"),"allowed=",d["decision"]["allowed"],"policy=",d["decision"].get("matched_policy"))'

echo -n "evaluate critical: "
curl -sk -X POST -H "X-Katana-Token: ${TOKEN}" -H 'Content-Type: application/json' \
  https://127.0.0.1:18443/api/v1/evaluate \
  -d '{"image":"artifactory.example.com/demo/critical:1.0","namespace":"default","environment":"prod"}' \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);s=d["scan"];print(s.get("scanner"),s.get("lookup_status"),"crit=",s.get("critical"),"allowed=",d["decision"]["allowed"],"policy=",d["decision"].get("matched_policy"))'

echo
echo "OK — local k3d + Helm smoke finished."
echo "  UI:  kubectl -n ${NS} port-forward svc/katana 8443:443"
echo "       https://127.0.0.1:8443  (admin/admin or X-Katana-Token: ${TOKEN})"
echo "  Tear down: k3d cluster delete ${CLUSTER}"
