#!/usr/bin/env bash
# Generate a self-signed cert for local/dev ValidatingWebhookConfiguration.
set -euo pipefail
OUT_DIR="${1:-./deploy/certs}"
NS="${2:-katana-system}"
SVC="katana.${NS}.svc"
mkdir -p "$OUT_DIR"
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$OUT_DIR/tls.key" \
  -out "$OUT_DIR/tls.crt" \
  -days 365 \
  -subj "/CN=${SVC}" \
  -addext "subjectAltName=DNS:${SVC},DNS:${SVC}.cluster.local,DNS:localhost,IP:127.0.0.1"
echo "Wrote $OUT_DIR/tls.crt and $OUT_DIR/tls.key"
echo "Base64 CA (paste into ValidatingWebhookConfiguration.caBundle):"
base64 -w0 "$OUT_DIR/tls.crt"; echo
