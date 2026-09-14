#!/bin/bash
set -euo pipefail
export PATH="$HOME/.local/node/bin:$PATH"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p data
export KATANA_TOKEN="${KATANA_TOKEN:-change-me-dev-token}"
export KATANA_ADDR="${KATANA_ADDR:-0.0.0.0:8080}"
export KATANA_DB_PATH="${KATANA_DB_PATH:-./data/katana.db}"
export KATANA_ADMISSION_DRY_RUN="${KATANA_ADMISSION_DRY_RUN:-true}"
exec go run ./cmd/katana
