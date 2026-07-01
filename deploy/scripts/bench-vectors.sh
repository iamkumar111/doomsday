#!/usr/bin/env bash
# Phase 1: test every vector standalone (no Redis). Starts victim if needed.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
export PATH="${HOME}/.local/go/bin:${PATH}"

DURATION="${BENCH_DURATION:-10s}"
TARGET="${BENCH_TARGET:-http://127.0.0.1:8443}"
VICTIM_PID=""

cleanup() {
  [[ -n "$VICTIM_PID" ]] && kill "$VICTIM_PID" 2>/dev/null || true
}
trap cleanup EXIT

if ! curl -sf --max-time 2 "$TARGET" >/dev/null 2>&1; then
  echo "starting native victim on :8443..."
  python3 deploy/scripts/native-victim.py &
  VICTIM_PID=$!
  sleep 1
fi

make build-all >/dev/null
echo "=== SH-MVDoS vector bench (no Redis) target=$TARGET duration=$DURATION ==="
./bin/vector-bench -all -target "$TARGET" -duration "$DURATION" -json | tee /tmp/shmv-bench.json
echo ""
./bin/vector-bench -all -target "$TARGET" -duration "$DURATION"