#!/usr/bin/env bash
# Spin up a local k3d cluster with KATANA + dummy-jfrog for demos.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
CLUSTER="${KATANA_DEMO_CLUSTER:-katana-demo}"
NS=katana-system
CERT_DIR="${ROOT}/deploy/certs"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required binary: $1" >&2
    exit 1
  }
}

need docker
need k3d
need kubectl

cd "$ROOT"

echo "==> Ensuring k3d cluster '${CLUSTER}'"
if k3d cluster list 2>/dev/null | grep -q "^${CLUSTER}"; then
  echo "    cluster already exists"
else
  k3d cluster create "${CLUSTER}" --wait
fi
kubectl config use-context "k3d-${CLUSTER}" >/dev/null

echo "==> Building images"
docker build -t katana:local .
docker build -t dummy-jfrog:local "${ROOT}/demo/dummy-jfrog"

echo "==> Importing images into k3d"
k3d image import katana:local dummy-jfrog:local -c "${CLUSTER}"

echo "==> Generating webhook TLS"
bash "${ROOT}/deploy/scripts/gen-webhook-certs.sh" "${CERT_DIR}" "${NS}"
CABUNDLE="$(base64 -w0 "${CERT_DIR}/tls.crt")"

echo "==> Applying demo manifests"
kubectl apply -f "${ROOT}/demo/k8s/katana-demo.yaml"
kubectl apply -f "${ROOT}/demo/k8s/dummy-jfrog.yaml"

kubectl -n "${NS}" create secret tls katana-webhook-tls \
  --cert="${CERT_DIR}/tls.crt" --key="${CERT_DIR}/tls.key" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl patch validatingwebhookconfiguration katana-image-policy --type=json \
  -p="[{\"op\":\"replace\",\"path\":\"/webhooks/0/clientConfig/caBundle\",\"value\":\"${CABUNDLE}\"}]"

kubectl create ns katana-demo --dry-run=client -o yaml | kubectl apply -f -
kubectl label ns katana-demo katana.dev/enforce=true --overwrite

echo "==> Waiting for rollouts"
kubectl -n "${NS}" rollout status deploy/dummy-jfrog --timeout=120s
kubectl -n "${NS}" rollout status deploy/katana --timeout=180s

echo
echo "Demo ready."
echo "  UI:        kubectl -n ${NS} port-forward svc/katana 8443:443"
echo "             open https://127.0.0.1:8443  (admin / admin)"
echo "  Evaluate images:"
echo "    artifactory.example.com/demo/clean:1.0"
echo "    artifactory.example.com/demo/high:1.0"
echo "    artifactory.example.com/demo/critical:1.0"
echo "    artifactory.example.com/demo/unscanned:1.0"
echo
echo "  Admission test (dry-run on by default; use non-root pods so CVE policies fire):"
echo "    kubectl apply -f demo/k8s/demo-pods.yaml"
echo "    # ErrImagePull is expected — images are fictional; check Detections in the UI"
echo
echo "  Tear down: k3d cluster delete ${CLUSTER}"
