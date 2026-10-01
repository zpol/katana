#!/usr/bin/env bash
# Destroy ephemeral E2E AWS resources (ECR repo, IAM user/keys, scanning rule).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TF_DIR="${ROOT}/deploy/e2e-aws"
STATE_DIR="${ROOT}/tmp/e2e-aws"
CREDS_BOOTSTRAP="${ROOT}/tmp/c.txt"
REGION="${AWS_REGION:-${AWS_DEFAULT_REGION:-eu-west-1}}"

need() { command -v "$1" >/dev/null || { echo "missing $1" >&2; exit 1; }; }
need terraform

if [[ ! -f "${STATE_DIR}/terraform.tfstate" ]]; then
  echo "no state at ${STATE_DIR}/terraform.tfstate — nothing to destroy" >&2
  exit 1
fi

if [[ -f "$CREDS_BOOTSTRAP" ]]; then
  # shellcheck disable=SC1090
  source "$CREDS_BOOTSTRAP"
fi
export AWS_DEFAULT_REGION="$REGION"
export AWS_REGION="$REGION"

cd "$TF_DIR"
terraform destroy -input=false -auto-approve \
  -var="aws_region=${REGION}" \
  -state="${STATE_DIR}/terraform.tfstate"

rm -f "${ROOT}/tmp/good.txt" "${ROOT}/good.txt"
echo "Destroyed. good.txt / tmp/good.txt removed."
