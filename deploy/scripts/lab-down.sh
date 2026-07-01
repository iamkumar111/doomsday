#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
docker compose --profile attacker --profile conductor --profile vectors --profile monitoring down -v 2>/dev/null || true
pkill -f 'bin/(dashboard|conductor|h2-thrasher|l7-abuser|quic-burner)' 2>/dev/null || true
echo "lab torn down"