#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
set -a
# shellcheck disable=SC1091
. "$ROOT/.env"
set +a

curl -sk -H "Authorization: Bearer $JFROG_TOKEN" \
  "${JFROG_URL%/}/artifactory/api/repositories" \
  | python3 - <<'PY'
import sys, json
repos = json.load(sys.stdin)
docker = [r["key"] for r in repos if r.get("packageType") == "Docker"]
local = [r["key"] for r in repos if r.get("packageType") == "Docker" and r.get("type") == "LOCAL"]
print("LOCAL docker repos:")
for k in sorted(local):
    print(" ", k)
print(f"\nAll docker repos ({len(docker)}):")
for k in sorted(docker)[:40]:
    print(" ", k)
PY
