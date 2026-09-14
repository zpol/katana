#!/usr/bin/env bash
# Install ImagePolicy CRD + default policies (cluster-admin required for CRD).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

echo "==> Apply CRD"
kubectl apply -f deploy/crd/imagepolicy-crd.yaml

echo "==> Wait for CRD Established"
kubectl wait --for=condition=Established crd/imagepolicies.katana.dev --timeout=120s

echo "==> Apply default ImagePolicy resources"
kubectl apply -f deploy/crd/default-imagepolicies.yaml

echo "==> Apply RBAC (if not already)"
kubectl apply -f deploy/k8s/katana-rbac-crd.yaml

kubectl get imagepolicies.katana.dev
