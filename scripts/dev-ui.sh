#!/bin/bash
set -euo pipefail
export PATH="$HOME/.local/node/bin:$PATH"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/web"
exec npm run dev -- --host 127.0.0.1
