#!/usr/bin/env bash
# Push a tiny public image into the E2E ECR repo (uses bootstrap creds from tmp/c.txt)
# so Inspector/ECR can produce scan findings for Katana to read (via tmp/good.txt).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
CREDS_BOOTSTRAP="${ROOT}/tmp/c.txt"
GOOD="${ROOT}/tmp/good.txt"
STATE_DIR="${ROOT}/tmp/e2e-aws"
TF_DIR="${ROOT}/deploy/e2e-aws"
REGION="${AWS_REGION:-${AWS_DEFAULT_REGION:-eu-west-1}}"
SAMPLE_SRC="${KATANA_E2E_SAMPLE_IMAGE:-public.ecr.aws/docker/library/alpine:3.20}"

need() { command -v "$1" >/dev/null || { echo "missing $1" >&2; exit 1; }; }
need aws
need docker
need terraform

[[ -f "$CREDS_BOOTSTRAP" ]] || { echo "missing $CREDS_BOOTSTRAP" >&2; exit 1; }
[[ -f "${STATE_DIR}/terraform.tfstate" ]] || { echo "apply terraform first" >&2; exit 1; }

# shellcheck disable=SC1090
source "$CREDS_BOOTSTRAP"
export AWS_DEFAULT_REGION="$REGION"
export AWS_REGION="$REGION"

REPO=$(terraform -chdir="$TF_DIR" output -state="${STATE_DIR}/terraform.tfstate" -raw ecr_repository_url)
ACCT=$(terraform -chdir="$TF_DIR" output -state="${STATE_DIR}/terraform.tfstate" -raw aws_account_id)
TAG="${REPO}:e2e"

echo "==> Login to ECR"
aws ecr get-login-password --region "$REGION" \
  | docker login --username AWS --password-stdin "${ACCT}.dkr.ecr.${REGION}.amazonaws.com"

echo "==> Pull ${SAMPLE_SRC} and push as ${TAG}"
docker pull "$SAMPLE_SRC"
docker tag "$SAMPLE_SRC" "$TAG"
docker push "$TAG"

DIGEST=$(aws ecr describe-images --repository-name "${REPO##*/}" --image-ids imageTag=e2e \
  --query 'imageDetails[0].imageDigest' --output text)
echo "Pushed ${TAG}"
echo "Digest: ${DIGEST}"
echo "Full ref for Katana Evaluate: ${REPO}@${DIGEST}"
echo
echo "Wait 1–3 minutes for enhanced scan, then:"
echo "  source tmp/good.txt"
echo "  export JFROG_URL= JFROG_TOKEN=   # optional: ECR-only"
echo "  # enable ecr_inspector in Settings → Integrations"
echo "  # evaluate image: ${REPO}:e2e"
