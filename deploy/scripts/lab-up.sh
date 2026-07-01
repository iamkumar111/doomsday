#!/usr/bin/env bash
# One-command lab stack: bootstrap runtime.env then docker compose up.
set -euo pipefail
cd "$(dirname "$0")/../.."

if [[ ! -f data/runtime.env ]]; then
  cp data/runtime.env.example data/runtime.env
  echo "lab-up: created data/runtime.env from example"
fi

# Append any keys from example that are missing (keeps DASHBOARD_TOKEN etc.)
while IFS= read -r line; do
  [[ -z "$line" || "$line" =~ ^# ]] && continue
  key="${line%%=*}"
  [[ -z "$key" ]] && continue
  if ! grep -q "^${key}=" data/runtime.env 2>/dev/null; then
    echo "$line" >> data/runtime.env
    echo "lab-up: added ${key} to data/runtime.env"
  fi
done < data/runtime.env.example

docker compose --profile attacker --profile dashboard --profile vectors --profile monitoring up -d --build

bind="${DASHBOARD_HOST_BIND:-127.0.0.1}"
port="8089"
token="$(grep '^DASHBOARD_TOKEN=' data/runtime.env 2>/dev/null | cut -d= -f2- || true)"
echo ""
echo "lab-up: stack running"
echo "  Dashboard: http://${bind}:${port}/"
if [[ -n "$token" && "$token" != "change-me-run-openssl-rand-hex-32" ]]; then
  echo "  With token: http://${bind}:${port}/?token=${token}"
fi
echo "  Victim:    http://127.0.0.1:8443/"
echo "  Workers:   l7-abuser, h2-thrasher, ws-flood, slowloris, quic-burner"